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
