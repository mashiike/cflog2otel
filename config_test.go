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
