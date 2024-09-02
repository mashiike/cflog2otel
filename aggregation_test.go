package cflog2otel_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/mashiike/cflog2otel"
	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

var testAggregations = testConfigs

func TestAggreagation(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("testdata/fixtures"),
		goldie.WithNameSuffix(".golden.json"),
	)
	for _, c := range testAggregations {
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

func TestAppendValueToHistogramDataPoint(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		dp       metricdata.HistogramDataPoint[float64]
		noMinMax bool
		expected metricdata.HistogramDataPoint[float64]
	}{
		{
			name:  "basic case",
			value: 7.5,
			dp: metricdata.HistogramDataPoint[float64]{
				Count:        1,
				Sum:          5.0,
				Min:          metricdata.NewExtrema(5.0),
				Max:          metricdata.NewExtrema(5.0),
				Bounds:       []float64{0, 5, 10, 15},
				BucketCounts: []uint64{0, 1, 0, 0, 0},
			},
			noMinMax: false,
			expected: metricdata.HistogramDataPoint[float64]{
				Count:        2,
				Sum:          12.5,
				Min:          metricdata.NewExtrema(5.0),
				Max:          metricdata.NewExtrema(7.5),
				Bounds:       []float64{0, 5, 10, 15},
				BucketCounts: []uint64{0, 1, 1, 0, 0},
			},
		},
		{
			name:  "no min max",
			value: 3.0,
			dp: metricdata.HistogramDataPoint[float64]{
				Count:        1,
				Sum:          2.0,
				Min:          metricdata.NewExtrema(2.0),
				Max:          metricdata.NewExtrema(2.0),
				Bounds:       []float64{0, 5, 10, 15},
				BucketCounts: []uint64{0, 1, 0, 0, 0},
			},
			noMinMax: true,
			expected: metricdata.HistogramDataPoint[float64]{
				Count:        2,
				Sum:          5.0,
				Min:          metricdata.NewExtrema(2.0),
				Max:          metricdata.NewExtrema(2.0),
				Bounds:       []float64{0, 5, 10, 15},
				BucketCounts: []uint64{0, 2, 0, 0, 0},
			},
		},
		{
			name:  "value in last bucket",
			value: 20.0,
			dp: metricdata.HistogramDataPoint[float64]{
				Count:        1,
				Sum:          10.0,
				Min:          metricdata.NewExtrema(10.0),
				Max:          metricdata.NewExtrema(10.0),
				Bounds:       []float64{0, 5, 10, 15},
				BucketCounts: []uint64{0, 0, 0, 0, 1},
			},
			noMinMax: false,
			expected: metricdata.HistogramDataPoint[float64]{
				Count:        2,
				Sum:          30.0,
				Min:          metricdata.NewExtrema(10.0),
				Max:          metricdata.NewExtrema(20.0),
				Bounds:       []float64{0, 5, 10, 15},
				BucketCounts: []uint64{0, 0, 0, 0, 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cflog2otel.AppendValueToHistogramDataPoint(tt.value, tt.dp, tt.noMinMax)
			require.EqualValues(t, tt.expected, result)
		})
	}
}
