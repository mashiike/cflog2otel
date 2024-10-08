package cflog2otel

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mashiike/slogutils"
	"github.com/samber/oops"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type App struct {
	cfg        *Config
	client     S3APIClient
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

type S3APIClient interface {
	manager.DownloadAPIClient
	s3.ListObjectsV2APIClient
}

func NewWithClient(cfg *Config, client S3APIClient) (*App, error) {
	return &App{
		cfg:        cfg,
		client:     client,
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
	celVariables, logs, err := app.GetVariablesAndLogs(ctx, notification)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to get variables and logs")
	}
	if len(logs) == 0 {
		slog.InfoContext(ctx, "no logs to process")
		return []*metricdata.ResourceMetrics{}, nil
	}
	resourceMetrics, err := Aggregate(ctx, app.cfg, celVariables, logs)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to aggregate metrics")
	}
	return resourceMetrics, nil
}

func (app *App) GetVariablesAndLogs(ctx context.Context, notification events.S3EventRecord) (*CELVariables, []CELVariablesLog, error) {
	prefix, distributionID, datehour, _, err := ParseCFStandardLogObjectKey(notification.S3.Object.Key)
	if err != nil {
		if app.cfg.NoSkip {
			return nil, nil, oops.Wrapf(err, "parse object key[%s]", notification.S3.Object.Key)
		}
		slog.WarnContext(ctx, "skipping object", "reason", err.Error())
		return nil, []CELVariablesLog{}, nil
	}
	reader, err := NewS3ObjectReader(ctx, app.downloader, notification.S3.Bucket.Name, notification.S3.Object.Key)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "failed to create object reader")
	}
	logs, err := ParseCloudFrontLog(ctx, reader)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "failed to parse cloudfront log")
	}
	if app.cfg.Backfill.Enabled {
		currentObjectLines := len(logs)
		skipLines := 0
		backfilTotalLines := 0
		eventTime := notification.EventTime
		p := s3.NewListObjectsV2Paginator(app.client, &s3.ListObjectsV2Input{
			Bucket: &notification.S3.Bucket.Name,
			Prefix: aws.String(fmt.Sprintf("%s%s.%s.", prefix, distributionID, datehour)),
		})
		timeTolerance := app.cfg.Backfill.TimeToleranceDuration()
		for p.HasMorePages() {
			out, err := p.NextPage(ctx)
			if err != nil {
				return nil, nil, oops.Wrapf(err, "failed to list objects")
			}
			for _, obj := range out.Contents {
				if *obj.Key == notification.S3.Object.Key {
					continue
				}
				if d := eventTime.Sub(*obj.LastModified); d > timeTolerance {
					slog.InfoContext(ctx, "skipping backfill object", "key", *obj.Key, "last_modified", *obj.LastModified, "time_tolerance", timeTolerance, "since", d)
					continue
				}
				reader, err := NewS3ObjectReader(ctx, app.downloader, notification.S3.Bucket.Name, *obj.Key)
				if err != nil {
					return nil, nil, oops.Wrapf(err, "failed to create object reader")
				}
				currentLogs, err := ParseCloudFrontLog(ctx, reader)
				if err != nil {
					return nil, nil, oops.Wrapf(err, "failed to parse cloudfront log")
				}
				backfilTotalLines += len(currentLogs)
				for _, currentLog := range currentLogs {
					if d := eventTime.Sub(currentLog.Timestamp); d > timeTolerance {
						skipLines++
						slog.DebugContext(ctx, "skipping backfill log", "timestamp", currentLog.Timestamp, "time_tolerance", timeTolerance, "since", d)
						continue
					}
					logs = append(logs, currentLog)
				}
			}
		}
		slices.SortStableFunc(logs, func(i, j CELVariablesLog) int {
			return i.Timestamp.Compare(j.Timestamp)
		})
		slog.InfoContext(ctx, "backfill logs", "total", backfilTotalLines+currentObjectLines, "skipped", skipLines)
	}
	celVariables := NewCELVariables(notification, distributionID)
	return celVariables, logs, nil
}

func NewS3ObjectReader(ctx context.Context, downloader *manager.Downloader, bucket, key string) (io.Reader, error) {
	buffer := NewWriteAtBuffer()
	n, err := downloader.Download(ctx, buffer, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "failed to download object")
	}
	slog.InfoContext(ctx, "downloaded object", "size", n)
	data := buffer.Bytes()
	var reader io.Reader
	reader = bytes.NewReader(data)
	if IsGzipped(data) {
		reader, err = gzip.NewReader(reader)
		if err != nil {
			return nil, oops.Wrapf(err, "failed to create gzip reader")
		}
	}
	return reader, nil
}

func IsGzipped(data []byte) bool {
	return len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b
}

func ToAttribute(ctx context.Context, key string, value any) (attribute.KeyValue, bool) {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v), true
	case int64:
		return attribute.Int64(key, v), true
	case float64:
		return attribute.Float64(key, v), true
	case bool:
		return attribute.Bool(key, v), true
	default:
		slog.WarnContext(ctx, "unsupported attribute type", "key", key, "value", value)
		return attribute.KeyValue{}, false
	}
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
		attr, ok := ToAttribute(ctx, cfg.Key, val)
		if !ok {
			continue
		}
		attrs = append(attrs, attr)
	}
	return attrs, nil
}

func ParseCFStandardLogObjectKey(str string) (string, string, string, string, error) {
	if !strings.HasSuffix(str, ".gz") {
		return "", "", "", "", errors.New("object key is not gzipped")
	}
	name := strings.TrimSuffix(filepath.Base(str), ".gz")
	parts := strings.SplitN(name, ".", 3)
	if len(parts) != 3 {
		return "", "", "", "", errors.New("invalid object key")
	}
	prefix := strings.TrimPrefix(filepath.Dir(str), "/")
	if prefix == "." {
		prefix = ""
	} else {
		prefix = strings.TrimSuffix(prefix, "/") + "/"
	}
	return prefix, parts[0], parts[1], parts[2], nil
}
