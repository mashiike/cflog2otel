package cflog2otel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mashiike/slogutils"
	"github.com/samber/oops"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

type App struct {
	cfg        *Config
	downloader *manager.Downloader
}

func New(ctx context.Context, cfg *Config) (*App, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to load AWS config")
	}
	client := s3.NewFromConfig(awsCfg)
	return NewWithClient(cfg, client)
}

func NewWithClient(cfg *Config, client manager.DownloadAPIClient) (*App, error) {
	return &App{
		cfg:        cfg,
		downloader: manager.NewDownloader(client),
	}, nil
}

func unwrapSQSEvent(ctx context.Context, eventIter iter.Seq[json.RawMessage]) iter.Seq[json.RawMessage] {
	return func(yield func(json.RawMessage) bool) {
		for event := range eventIter {
			slog.DebugContext(ctx, "unwrapping SQS event")
			var sqsEvent events.SQSEvent
			if err := json.Unmarshal(event, &sqsEvent); err != nil {
				slog.DebugContext(ctx, "event is not an SQS event")
				if !yield(event) {
					return
				}
				continue
			}
			if len(sqsEvent.Records) == 0 {
				slog.DebugContext(ctx, "no records in SQS event")
				var sqsEventRecord events.SQSMessage
				if err := json.Unmarshal(event, &sqsEventRecord); err != nil {
					slog.DebugContext(ctx, "event is not an SQS message")
					if !yield(event) {
						return
					}
					continue
				}
				if sqsEventRecord.EventSource == "aws:sqs" {
					slog.DebugContext(ctx, "unwrapped SQS message", "message_id", sqsEventRecord.MessageId, "body", sqsEventRecord.Body)
					if !yield([]byte(sqsEventRecord.Body)) {
						return
					}
					continue
				}
				if !yield(event) {
					return
				}
				continue
			}
			for _, record := range sqsEvent.Records {
				if record.EventSource != "aws:sqs" {
					slog.DebugContext(ctx, "eventSource is not aws:sqs", "source", record.EventSource)
					if !yield(event) {
						return
					}
					break
				}
				slog.DebugContext(ctx, "unwrapped SQS event", "message_id", record.MessageId, "body", record.Body)
				if !yield([]byte(record.Body)) {
					return
				}
			}
		}
	}
}

func unwrapSNSEvent(ctx context.Context, eventIter iter.Seq[json.RawMessage]) iter.Seq[json.RawMessage] {
	return func(yield func(json.RawMessage) bool) {
		for event := range eventIter {
			slog.DebugContext(ctx, "unwrapping SNS event")
			var snsEvent events.SNSEvent
			if err := json.Unmarshal(event, &snsEvent); err != nil {
				slog.DebugContext(ctx, "event is not an SNS event")
				if !yield(event) {
					return
				}
				continue
			}
			if len(snsEvent.Records) == 0 {
				slog.DebugContext(ctx, "no records in SNS event")
				var snsEventRecord events.SNSEventRecord
				if err := json.Unmarshal(event, &snsEventRecord); err != nil {
					slog.DebugContext(ctx, "event is not an SNS event record")
					if !yield(event) {
						return
					}
					continue
				}
				if snsEventRecord.EventSource == "aws:sns" {
					slog.DebugContext(ctx, "unwrapped SNS event record", "message_id", snsEventRecord.SNS.MessageID, "message", snsEventRecord.SNS.Message)
					if !yield([]byte(snsEventRecord.SNS.Message)) {
						return
					}
					continue
				}
				var entity events.SNSEntity
				if err := json.Unmarshal(event, &entity); err != nil {
					slog.DebugContext(ctx, "event is not an SNS entity")
					if !yield(event) {
						return
					}
					continue
				}
				if entity.MessageID != "" {
					slog.DebugContext(ctx, "unwrapped SNS entity", "message_id", entity.MessageID, "message", entity.Message)
					if !yield([]byte(entity.Message)) {
						return
					}
					continue
				}
				if !yield(event) {
					return
				}
				continue
			}
			for _, record := range snsEvent.Records {
				if record.EventSource != "aws:sns" {
					slog.DebugContext(ctx, "eventSource is not aws:sns", "source", record.EventSource)
					if !yield(event) {
						return
					}
					break
				}
				slog.DebugContext(ctx, "unwrapped SNS event", "message_id", record.SNS.MessageID, "message", record.SNS.Message)
				if !yield([]byte(record.SNS.Message)) {
					return
				}
			}
		}
	}
}

func UnwrapEvent(ctx context.Context, event json.RawMessage) func(yield func(json.RawMessage) bool) {
	return unwrapSNSEvent(ctx, unwrapSQSEvent(ctx, slices.Values([]json.RawMessage{event})))
}

func (app *App) Invoke(ctx context.Context, event json.RawMessage) (any, error) {
	if lambCtx, ok := lambdacontext.FromContext(ctx); ok {
		ctx = slogutils.With(ctx,
			"aws_request_id", lambCtx.AwsRequestID,
		)
	}
	slog.InfoContext(ctx, "received invoke request")
	s3Notifications := make([]events.S3EventRecord, 0)
	for event := range UnwrapEvent(ctx, event) {
		var s3Event events.S3Event
		if err := json.Unmarshal(event, &s3Event); err != nil {
			slog.WarnContext(ctx, "event is not an S3 event, skipping", "event", string(event))
			continue
		}
		s3Notifications = append(s3Notifications, s3Event.Records...)
	}
	slog.InfoContext(ctx, "s3 notifications", "count", len(s3Notifications))
	if len(s3Notifications) == 0 {
		slog.InfoContext(ctx, "no s3 notifications, skipping")
		return nil, nil
	}
	if err := app.Process(ctx, s3Notifications); err != nil {
		return nil, err
	}
	return nil, nil
}

func (app *App) Process(ctx context.Context, notifications []events.S3EventRecord) error {
	exporter, endpointURL, err := newOtelExporter(ctx, app.cfg.Otel)
	if err != nil {
		return oops.Wrapf(err, "failed to create OTLP exporter")
	}
	slog.InfoContext(ctx, "starting export to otel metrics", "endpoint", endpointURL)
	defer func() {
		if err := exporter.Shutdown(ctx); err != nil {
			slog.WarnContext(ctx, "failed to shutdown exporter", "error", err)
		}
	}()
	recourceMetrics := make([]*metricdata.ResourceMetrics, 0)
	for _, notification := range notifications {
		slog.InfoContext(ctx, "processing notification", "bucket", notification.S3.Bucket.Name, "key", notification.S3.Object.Key)
		metrics, err := app.generateMetrics(ctx, notification)
		if err != nil {
			return oops.Wrapf(err, "failed to generate metrics[s3://%s/%s]", notification.S3.Bucket.Name, notification.S3.Object.Key)
		}
		recourceMetrics = append(recourceMetrics, metrics...)
	}
	if len(recourceMetrics) == 0 {
		slog.InfoContext(ctx, "no metrics to export")
		return nil
	}
	var errs []error
	for _, metrics := range recourceMetrics {
		if err := exporter.Export(ctx, metrics); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return oops.Wrapf(errors.Join(errs...), "failed to export metrics")
	}
	slog.InfoContext(ctx, "exported metrics", "count", len(recourceMetrics))
	return nil
}

func newOtelExporter(ctx context.Context, oc OtelConfig) (*otlpmetricgrpc.Exporter, string, error) {
	opts := make([]otlpmetricgrpc.Option, 0)
	if len(oc.Headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(oc.Headers))
	}
	if oc.GZip {
		opts = append(opts, otlpmetricgrpc.WithCompressor("gzip"))
	}
	endpointURL := oc.EndpointURL().String()
	opts = append(opts, otlpmetricgrpc.WithEndpointURL(endpointURL))
	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, "", err
	}
	return exporter, endpointURL, nil
}

// WriteAtBuffer is an in-memory buffer implementing io.WriterAt
type WriteAtBuffer struct {
	buf *bytes.Buffer
}

// NewWriteAtBuffer creates a new WriteAtBuffer
func NewWriteAtBuffer() *WriteAtBuffer {
	return &WriteAtBuffer{buf: &bytes.Buffer{}}
}

// WriteAt writes bytes to the buffer at the specified offset
func (w *WriteAtBuffer) WriteAt(p []byte, off int64) (n int, err error) {
	if off != int64(w.buf.Len()) {
		return 0, oops.Errorf("unsupported offset in WriteAtBuffer")
	}
	return w.buf.Write(p)
}

// Bytes returns the contents of the buffer
func (w *WriteAtBuffer) Bytes() []byte {
	return w.buf.Bytes()
}

func (app *App) generateMetrics(ctx context.Context, notification events.S3EventRecord) ([]*metricdata.ResourceMetrics, error) {
	ctx = slogutils.With(ctx,
		"bucket_name", notification.S3.Bucket.Name,
		"object_key", notification.S3.Object.Key,
	)
	slog.InfoContext(ctx, "starting metrics generation")
	buffer := NewWriteAtBuffer()
	n, err := app.downloader.Download(ctx, buffer, &s3.GetObjectInput{
		Bucket: &notification.S3.Bucket.Name,
		Key:    &notification.S3.Object.Key,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "failed to download object")
	}
	slog.InfoContext(ctx, "downloaded object", "size", n)
	resourceMetrics, err := Aggregate(ctx, app.cfg, notification, bytes.NewReader(buffer.Bytes()))
	if err != nil {
		return nil, oops.Wrapf(err, "failed to aggregate metrics")
	}
	return resourceMetrics, nil
}

func ToAttributes(ctx context.Context, cfgs []AttributeConfig, celVariables *CELVariables) ([]attribute.KeyValue, error) {
	attrs := make([]attribute.KeyValue, 0)
	for _, cfg := range cfgs {
		val, err := cfg.Value.Eval(ctx, celVariables)
		if err != nil {
			return nil, oops.Wrapf(err, "failed to evaluate attribute")
		}
		if val == nil {
			continue
		}
		switch v := val.(type) {
		case string:
			attrs = append(attrs, attribute.String(cfg.Key, v))
		case int64:
			attrs = append(attrs, attribute.Int64(cfg.Key, v))
		case float64:
			attrs = append(attrs, attribute.Float64(cfg.Key, v))
		case bool:
			attrs = append(attrs, attribute.Bool(cfg.Key, v))
		default:
			slog.WarnContext(ctx, "unsupported attribute type", "key", cfg.Key, "value", val)
		}
	}
	return attrs, nil
}

func Aggregate(ctx context.Context, cfg *Config, notification events.S3EventRecord, reader io.Reader) ([]*metricdata.ResourceMetrics, error) {
	distributionID, datehour, uniqueID, err := ParseCFStandardLogObjectKey(notification.S3.Object.Key)
	if err != nil {
		return nil, oops.Wrapf(err, "parse object key[%s]", notification.S3.Object.Key)
	}
	ctx = slogutils.With(ctx,
		"distribution_id", distributionID,
		"datehour", datehour,
		"unique_id", uniqueID,
	)
	logs, err := ParseCloudFrontLog(ctx, reader)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to parse cloudfront log")
	}
	resourceMetrics := make([]*metricdata.ResourceMetrics, 0)
	for _, l := range logs {
		celVariables := NewCELVariables(notification, distributionID, l)
		attrs, err := ToAttributes(ctx, cfg.ResourceAttributes, celVariables)
		if err != nil {
			return nil, oops.Wrapf(err, "failed to convert attributes")
		}
		var found bool
		attrSet := attribute.NewSet(attrs...)
		var target *metricdata.ResourceMetrics
		for i, r := range resourceMetrics {
			set := attribute.NewSet(r.Resource.Attributes()...)
			if !set.Equals(&attrSet) {
				continue
			}
			target = resourceMetrics[i]
			found = true
		}
		if !found {
			target = &metricdata.ResourceMetrics{
				Resource: resource.NewSchemaless(attrs...),
				ScopeMetrics: []metricdata.ScopeMetrics{
					{
						Scope: instrumentation.Scope{
							Name:      cfg.Scope.Name,
							Version:   cfg.Scope.Version,
							SchemaURL: cfg.Scope.SchemaURL,
						},
						Metrics: make([]metricdata.Metrics, 0, len(cfg.Metrics)),
					},
				},
			}
			resourceMetrics = append(resourceMetrics, target)
		}
		for _, mcfg := range cfg.Metrics {
			var found bool
			var metricsIndex int
			for i, metric := range target.ScopeMetrics[0].Metrics {
				if mcfg.Name != metric.Name {
					continue
				}
				metricsIndex = i
				found = true
				break
			}
			if !found {
				target.ScopeMetrics[0].Metrics = append(target.ScopeMetrics[0].Metrics, metricdata.Metrics{
					Name:        mcfg.Name,
					Description: mcfg.Description,
					Unit:        mcfg.Unit,
				})
				metricsIndex = len(target.ScopeMetrics[0].Metrics) - 1
			}
			target.ScopeMetrics[0].Metrics[metricsIndex], err = aggregateMetric(ctx, target.ScopeMetrics[0].Metrics[metricsIndex], mcfg, celVariables)
			if err != nil {
				return nil, oops.Wrapf(err, "failed to aggregate metric %q", mcfg.Name)
			}
		}
	}
	resp := make([]*metricdata.ResourceMetrics, 0, len(resourceMetrics))
	for _, r := range resourceMetrics {
		if r == nil {
			continue
		}
		metrics := make([]metricdata.Metrics, 0, len(r.ScopeMetrics[0].Metrics))
		for i, m := range r.ScopeMetrics[0].Metrics {
			if LenDataPoints(m.Data) == 0 {
				continue
			}
			metrics = append(metrics, r.ScopeMetrics[0].Metrics[i])
		}
		if len(metrics) == 0 {
			continue
		}
		r.ScopeMetrics[0].Metrics = metrics
		resp = append(resp, r)
	}
	return resp, nil
}

func LenDataPoints(data metricdata.Aggregation) int {
	if data == nil {
		return 0
	}
	switch data := data.(type) {
	case metricdata.Sum[int64]:
		return len(data.DataPoints)
	case metricdata.Sum[float64]:
		return len(data.DataPoints)
	default:
		return 0
	}
}

func aggregateMetric(ctx context.Context, metrics metricdata.Metrics, config MetricsConfig, vars *CELVariables) (metricdata.Metrics, error) {
	switch config.Type {
	case AggregationTypeCounter:
		return aggregateForCounterMetric(ctx, metrics, config, vars)
	default:
		return metricdata.Metrics{}, oops.Errorf("unsupported aggregation type")
	}
}

func aggregateForCounterMetric(ctx context.Context, metrics metricdata.Metrics, config MetricsConfig, vars *CELVariables) (metricdata.Metrics, error) {
	if metrics.Data == nil {
		metrics.Data = metricdata.Sum[int64]{
			DataPoints:  make([]metricdata.DataPoint[int64], 0),
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
		}
	}
	data, ok := metrics.Data.(metricdata.Sum[int64])
	if !ok {
		return metrics, oops.Errorf("unsupported data type for counter")
	}
	startTime := vars.Log.Timestamp.Truncate(time.Minute)
	attrs, err := ToAttributes(ctx, config.Attributes, vars)
	if err != nil {
		return metrics, oops.Wrapf(err, "failed to convert attributes")
	}
	attrSet := attribute.NewSet(attrs...)
	var found bool
	for i, dp := range data.DataPoints {
		if !dp.StartTime.Equal(startTime) {
			continue
		}
		if !dp.Attributes.Equals(&attrSet) {
			continue
		}
		data.DataPoints[i].Value++
		found = true
		break
	}
	if !found {
		data.DataPoints = append(data.DataPoints, metricdata.DataPoint[int64]{
			StartTime:  startTime,
			Value:      1,
			Attributes: attrSet,
		})
	}
	metrics.Data = data
	return metrics, nil
}

func ParseCFStandardLogObjectKey(str string) (string, string, string, error) {
	name := strings.TrimSuffix(filepath.Base(str), ".gz")
	parts := strings.SplitN(name, ".", 3)
	if len(parts) != 3 {
		return "", "", "", errors.New("invalid object key")
	}
	return parts[0], parts[1], parts[2], nil
}
