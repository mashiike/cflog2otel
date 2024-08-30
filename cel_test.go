package cflog2otel_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/mashiike/cflog2otel"
	"github.com/stretchr/testify/require"
)

type testCaseCELCapable[T any] struct {
	name             string
	expr             string
	variables        *cflog2otel.CELVariables
	want             T
	wantUnmarshalErr string
	wantEvalErr      string
}

func (tc testCaseCELCapable[T]) Name() string {
	return tc.name
}

func (tc testCaseCELCapable[T]) Run(t *testing.T) {
	var expr cflog2otel.CELCapable[T]
	vm := cflog2otel.MakeVM(cflog2otel.WithAWSConfig(aws.Config{}))
	jsonStr, err := vm.EvaluateAnonymousSnippet("field.jsonnet", fmt.Sprintf("local cel = std.native('cel');local cel_switch = std.native('cel_switch');\n%s", tc.expr))
	require.NoError(t, err)
	t.Log(jsonStr)
	err = json.Unmarshal([]byte(jsonStr), &expr)
	if tc.wantUnmarshalErr != "" {
		require.ErrorContains(t, err, tc.wantUnmarshalErr)
		return
	}
	require.NoError(t, err)
	ctx := context.Background()
	got, err := expr.Eval(ctx, tc.variables)
	if tc.wantEvalErr != "" {
		require.ErrorContains(t, err, tc.wantEvalErr)
		return
	}
	require.NoError(t, err)
	require.EqualValues(t, tc.want, got)
}

type testCase interface {
	Name() string
	Run(t *testing.T)
}

func TestCELCapable(t *testing.T) {
	cases := []testCase{
		testCaseCELCapable[string]{
			name:      "empty",
			expr:      `''`,
			variables: &cflog2otel.CELVariables{},
			want:      "",
		},
		testCaseCELCapable[string]{
			name: "use cel expr",
			expr: `cel('bucket.name')`,
			variables: &cflog2otel.CELVariables{
				Bucket: cflog2otel.CELVariablesS3Bucket{
					Name: "my-bucket",
				},
			},
			want: "my-bucket",
		},
		testCaseCELCapable[string]{
			name:      "use cel expr but variable empty",
			expr:      `cel('bucket.name')`,
			variables: &cflog2otel.CELVariables{},
			want:      "",
		},
		testCaseCELCapable[int64]{
			name:      "raw int value",
			expr:      `1`,
			variables: &cflog2otel.CELVariables{},
			want:      1,
		},
		testCaseCELCapable[string]{
			name:      "raw string value",
			expr:      `'1'`,
			variables: &cflog2otel.CELVariables{},
			want:      "1",
		},
		testCaseCELCapable[int64]{
			name:             "type missmatch",
			expr:             `'hogehoge'`,
			variables:        &cflog2otel.CELVariables{},
			wantUnmarshalErr: "json: cannot unmarshal string into Go value of type int64",
		},
		testCaseCELCapable[int64]{
			name:             "type missmatch expr type",
			expr:             `cel('bucket.name')`,
			variables:        &cflog2otel.CELVariables{},
			wantUnmarshalErr: `failed to convert CEL expression value to int64`,
		},
		testCaseCELCapable[string]{
			name: "switch case yes",
			expr: `cel_switch([
  {
    case: 'bucket.name == "my-bucket"',
    value: 'yes',
  },
  {
    default: 'no',
  },
])
`,
			variables: &cflog2otel.CELVariables{
				Bucket: cflog2otel.CELVariablesS3Bucket{
					Name: "my-bucket",
				},
			},
			want: "yes",
		},
		testCaseCELCapable[string]{
			name: "switch case no",
			expr: `cel_switch([
  {
    case: 'bucket.name == "my-bucket"',
    value: 'yes',
  },
  {
    default: 'no',
  },
])
`,
			variables: &cflog2otel.CELVariables{
				Bucket: cflog2otel.CELVariablesS3Bucket{
					Name: "my-bucket2",
				},
			},
			want: "no",
		},
		testCaseCELCapable[string]{
			name:             "missing cel variables",
			expr:             `cel('bucket.hoge')`,
			variables:        &cflog2otel.CELVariables{},
			wantUnmarshalErr: "undefined field 'hoge'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.Name(), tc.Run)
	}
}
