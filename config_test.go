package cflog2otel_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/mashiike/cflog2otel"
	"github.com/stretchr/testify/require"
)

var testConfigs = []string{
	`testdata/request_count_by_status_category.jsonnet`,
	`testdata/request_count_for_5xx.jsonnet`,
	`testdata/switch_case.jsonnet`,
	`testdata/multi_metrics.jsonnet`,
	`testdata/request_time_histogram.jsonnet`,
	`testdata/request_count_for_5xx_is_cumlative.jsonnet`,
	`testdata/backfil_config.jsonnet`,
	`testdata/request_time_histogram_custom_buckets.jsonnet`,
}

func TestConfigLoad__Success(t *testing.T) {
	for _, c := range testConfigs {
		t.Run(c, func(t *testing.T) {
			cfg := cflog2otel.DefaultConfig()
			err := cfg.Load(c, cflog2otel.WithAWSConfig(aws.Config{}))
			require.NoError(t, err)
		})
	}

}

func TestConfigLoad__Failed(t *testing.T) {
	testFailedConfig := [][]string{
		{`testdata/not_found.jsonnet`, `open testdata/not_found.jsonnet: no such file or directory`},
		{`testdata/invalid_syntax.jsonnet`, `Expected , or ; but got end of file`},
		{`testdata/invalid_unknown_field.jsonnet`, `unknown field "fiter"`},
		{`testdata/invalid_cel.jsonnet`, `undefined field 'csURIStem'`},
	}
	for _, c := range testFailedConfig {
		t.Run(c[0], func(t *testing.T) {
			cfg := cflog2otel.DefaultConfig()
			err := cfg.Load(c[0])
			require.Error(t, err)
			t.Log(err)
			require.Contains(t, err.Error(), c[1])
		})
	}
}
