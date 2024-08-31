package cflog2otel_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mashiike/cflog2otel"
	"github.com/mashiike/cflog2otel/otlptest"
	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

func TestAggreagation(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("testdata/fixtures"),
		goldie.WithNameSuffix(".golden.json"),
	)
	for _, c := range testConfigs {
		t.Run(c, func(t *testing.T) {
			cfg := cflog2otel.DefaultConfig()
			err := cfg.Load(c, cflog2otel.WithAWSConfig(aws.Config{}))
			require.NoError(t, err)
			ctx := context.Background()
			bs, err := os.ReadFile("testdata/cf_log.txt")
			require.NoError(t, err)
			metrics, err := cflog2otel.Aggregate(ctx, cfg, events.S3EventRecord{
				S3: events.S3Entity{
					Bucket: events.S3Bucket{
						Name: "example-bucket",
					},
					Object: events.S3Object{
						Key: "logs/EMLARXS9EXAMPLE.2019-11-14-20.RT4KCN4SGK9.gz",
					},
				},
			}, strings.NewReader(string(bs)))
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			g.AssertJson(t, strings.TrimSuffix(filepath.Base(c), filepath.Ext(c)), metrics[0])
		})
	}
}

func TestE2E(t *testing.T) {
	ctrl := newMockControler(t)
	defer ctrl.Finish()
	client := newMockDownloadAPIClient(ctrl)
	bs, err := os.ReadFile("testdata/cf_log.txt")
	require.NoError(t, err)
	client.On(
		"GetObject",
		mock.Anything,
		mock.MatchedBy(func(input *s3.GetObjectInput) bool {
			return input.Bucket != nil && *input.Bucket == "example-bucket" && input.Key != nil && *input.Key == "logs/EMLARXS9EXAMPLE.2019-11-14-20.RT4KCN4SGK9.gz"
		}),
	).Return(&s3.GetObjectOutput{
		Body:          io.NopCloser(strings.NewReader(string(bs))),
		ContentLength: aws.Int64(int64(len(bs))),
	}, nil)
	cfg := cflog2otel.DefaultConfig()
	err = cfg.Load("testdata/request_count_by_status_category.jsonnet", cflog2otel.WithAWSConfig(aws.Config{}))
	require.NoError(t, err)
	ctx := context.Background()
	var sended []*collectormetrics.ExportMetricsServiceRequest
	server := otlptest.NewMetricsCollector(otlptest.ExporterFunc(
		func(ctx context.Context, req *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error) {
			sended = append(sended, req)
			return &collectormetrics.ExportMetricsServiceResponse{}, nil
		},
	))
	defer server.Close()
	cfg.Otel.SetEndpointURL(server.URL)
	app, err := cflog2otel.NewWithClient(cfg, client)
	require.NoError(t, err)

	payload, err := os.ReadFile("testdata/s3_notification.json")
	require.NoError(t, err)
	_, err = app.Invoke(ctx, payload)
	require.NoError(t, err)
	require.Len(t, sended, 1)

	g := goldie.New(t, goldie.WithFixtureDir("testdata/fixtures"), goldie.WithNameSuffix(".golden.json"))
	g.AssertJson(t, "e2e", sended[0])
}

func TestUnwrapEvent_S3Notification(t *testing.T) {
	bs, err := os.ReadFile("testdata/s3_notification.json")
	require.NoError(t, err)
	ctx := context.Background()
	actual := slices.Collect(cflog2otel.UnwrapEvent(ctx, bs))
	require.Len(t, actual, 1)
	require.JSONEq(t, string(bs), string(actual[0]))
}

func TestUnwrapEvent_SQSEvent(t *testing.T) {
	bs, err := os.ReadFile("testdata/sqs_event.json")
	require.NoError(t, err)
	ctx := context.Background()
	actual := slices.Collect(cflog2otel.UnwrapEvent(ctx, bs))
	expected, err := os.ReadFile("testdata/s3_notification.json")
	require.NoError(t, err)
	require.Len(t, actual, 1)
	require.JSONEq(t, string(expected), string(actual[0]))
}
