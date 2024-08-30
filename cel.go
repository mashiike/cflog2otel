package cflog2otel

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"github.com/samber/oops"
)

var defaultCELEnv *cel.Env

type CELVariables struct {
	Bucket     CELVariablesS3Bucket   `json:"bucket" cel:"bucket"`
	Object     CELVariablesS3Object   `json:"object" cel:"object"`
	CloudFront CELVariablesCloudFront `json:"cloudfront" cel:"cloudfront"`
	Log        CELVariablesLog        `json:"log" cel:"log"`
}

type CELVariablesS3Bucket struct {
	Name          string                     `json:"name" cel:"name"`
	OwnerIdentity CELVariablesS3UserIdentity `json:"ownerIdentity" cel:"ownerIdentity"`
	Arn           string                     `json:"arn" cel:"arn"`
}

type CELVariablesCloudFront struct {
	DistributionId string `json:"distributionId" cel:"distributionId"`
}

type CELVariablesS3UserIdentity struct {
	PrincipalId string `json:"principalId" cel:"principalId"`
}

type CELVariablesS3Object struct {
	Key       string `json:"key" cel:"key"`
	Size      int64  `json:"size" cel:"size"`
	ETag      string `json:"eTag" cel:"eTag"`
	VersionId string `json:"versionId" cel:"versionId"`
	Sequencer string `json:"sequencer" cel:"sequencer"`
}

func NewCELVariables(record events.S3EventRecord, distributionID string, logLine CELVariablesLog) *CELVariables {
	return &CELVariables{
		Bucket: CELVariablesS3Bucket{
			Name: record.S3.Bucket.Name,
			OwnerIdentity: CELVariablesS3UserIdentity{
				PrincipalId: record.S3.Bucket.OwnerIdentity.PrincipalID,
			},
			Arn: record.S3.Bucket.Arn,
		},
		Object: CELVariablesS3Object{
			Key:       record.S3.Object.Key,
			Size:      record.S3.Object.Size,
			ETag:      record.S3.Object.ETag,
			VersionId: record.S3.Object.VersionID,
			Sequencer: record.S3.Object.Sequencer,
		},
		CloudFront: CELVariablesCloudFront{
			DistributionId: distributionID,
		},
		Log: logLine,
	}
}

func (v *CELVariables) MarshalMap() map[string]interface{} {
	if v == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"bucket":     v.Bucket,
		"object":     v.Object,
		"cloudfront": v.CloudFront,
		"log":        v.Log,
	}
}

func init() {
	var err error
	var variables CELVariables
	m := variables.MarshalMap()
	opts := make([]cel.EnvOption, 0, len(m)*2)
	for k, v := range m {
		rt := reflect.TypeOf(v)
		var pkgPath string
		paths := strings.Split(rt.PkgPath(), "/")
		if len(paths) != 0 {
			pkgPath = paths[len(paths)-1]
		}
		objectName := fmt.Sprintf("%s.%s", pkgPath, rt.Name())
		opts = append(
			opts,
			cel.Variable(k, cel.ObjectType(objectName)),
			ext.NativeTypes(rt, ext.ParseStructTags(true)),
		)
	}
	defaultCELEnv, err = cel.NewEnv(opts...)
	if err != nil {
		panic(oops.Wrapf(err, "failed to create CEL environment"))
	}
}

type celCapableField[T any] struct {
	Expr   string                     `json:"expr,omitempty"`
	Switch []celCapableFieldSwitch[T] `json:"switch,omitempty"`
}

type celCapableFieldSwitch[T any] struct {
	Case    string `json:"case,omitempty"`
	Value   T      `json:"value,omitempty"`
	Default T      `json:"default,omitempty"`
}

type CELCapable[T any] struct {
	raw              json.RawMessage
	value            T
	prog             cel.Program
	switchCases      []cel.Program
	switchCaseValues []T
	switchDefault    T
}

func (expr *CELCapable[T]) MarshalJSON() ([]byte, error) {
	return expr.raw, nil
}

func (expr *CELCapable[T]) UnmarshalJSON(data []byte) error {
	expr.raw = data
	var field celCapableField[T]
	fallback := func() error {
		var value T
		if err := json.Unmarshal(data, &value); err != nil {
			return oops.Wrapf(err, "failed to unmarshal CEL expression")
		}
		expr.value = value
		return nil
	}
	if err := json.Unmarshal(data, &field); err != nil {
		return fallback()
	}
	if field.Expr == "" && field.Switch == nil {
		return fallback()
	}
	dummyVariables := &CELVariables{}
	if field.Expr != "" {
		ast, iss := defaultCELEnv.Compile(field.Expr)
		if iss.Err() != nil {
			return oops.Wrapf(iss.Err(), "failed to compile CEL expression")
		}
		prog, err := defaultCELEnv.Program(ast)
		if err != nil {
			return oops.Wrapf(err, "failed to create CEL program")
		}
		expr.prog = prog
		ctx := context.Background()
		if _, err := expr.Eval(ctx, dummyVariables); err != nil {
			return oops.Wrapf(err, "check CEL expression type")
		}
		return nil
	}
	var defaultCount int
	expr.switchCases = make([]cel.Program, 0, len(field.Switch))
	expr.switchCaseValues = make([]T, 0, len(field.Switch))
	for i, s := range field.Switch {
		if s.Case == "" {
			expr.switchDefault = s.Default
			defaultCount++
			continue
		}
		ast, iss := defaultCELEnv.Compile(s.Case)
		if iss.Err() != nil {
			return oops.Wrapf(iss.Err(), "failed to compile CEL expression")
		}
		prog, err := defaultCELEnv.Program(ast)
		if err != nil {
			return oops.Wrapf(err, "failed to create CEL program")
		}
		out, _, err := prog.Eval(dummyVariables.MarshalMap())
		if err != nil {
			return oops.Wrapf(err, "switch case[%d] evaluation failed", i)
		}
		if out.Type() != cel.BoolType {
			return oops.Errorf("switch case[%d] must return boolean type", i)
		}
		expr.switchCases = append(expr.switchCases, prog)
		expr.switchCaseValues = append(expr.switchCaseValues, s.Value)
	}
	if defaultCount > 1 {
		return oops.Errorf("multiple default values in switch")
	}
	if len(expr.switchCases) == 0 {
		return oops.Errorf("no switch cases")
	}
	return nil
}

func (expr *CELCapable[T]) Eval(ctx context.Context, vars *CELVariables) (T, error) {
	if expr.prog == nil && len(expr.switchCases) == 0 {
		return expr.value, nil
	}
	variables := vars.MarshalMap()
	if expr.prog != nil {
		out, _, err := expr.prog.ContextEval(ctx, variables)
		if err != nil {
			var zero T
			return zero, oops.Wrapf(err, "failed to evaluate CEL expression")
		}
		value, ok := out.Value().(T)
		if !ok {
			var zero T
			return zero, oops.Errorf("failed to convert CEL expression value to %T", zero)
		}
		return value, nil
	}
	for i, prog := range expr.switchCases {
		out, _, err := prog.ContextEval(ctx, variables)
		if err != nil {
			return expr.switchDefault, oops.Wrapf(err, "switch case[%d] evaluation failed", i)
		}
		if out.Type() != cel.BoolType {
			return expr.switchDefault, oops.Errorf("switch case[%d] must return boolean type", i)
		}
		if out.Value().(bool) {
			return expr.switchCaseValues[i], nil
		}
	}
	return expr.switchDefault, nil
}
