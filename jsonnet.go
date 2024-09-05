package cflog2otel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/fujiwara/ssm-lookup/ssm"
	jsonnet "github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/samber/oops"
)

type jsonnetOptions struct {
	awsCfg *aws.Config
	ctx    context.Context
	cache  *sync.Map
}

type JsonnetOption func(*jsonnetOptions)

func WithAWSConfig(cfg aws.Config) JsonnetOption {
	return func(o *jsonnetOptions) {
		o.awsCfg = &cfg
	}
}

func WithContext(ctx context.Context) JsonnetOption {
	return func(o *jsonnetOptions) {
		o.ctx = ctx
	}
}

func WithCache(cache *sync.Map) JsonnetOption {
	return func(o *jsonnetOptions) {
		o.cache = cache
	}
}

func MakeVM(opts ...JsonnetOption) *jsonnet.VM {
	options := jsonnetOptions{
		ctx:   context.Background(),
		cache: &sync.Map{},
	}
	for _, opt := range opts {
		opt(&options)
	}
	vm := jsonnet.MakeVM()
	for _, nf := range NativeFunctions {
		vm.NativeFunction(nf)
	}
	if options.awsCfg == nil {
		awsCfg, err := config.LoadDefaultConfig(options.ctx)
		if err != nil {
			slog.WarnContext(options.ctx, "failed to load AWS config", "error", err)
		}
		options.awsCfg = &awsCfg
	}
	ssmlookup := ssm.New(*options.awsCfg, options.cache)
	for _, nf := range ssmlookup.JsonnetNativeFuncs(options.ctx) {
		vm.NativeFunction(nf)
	}
	return vm
}

var NativeFunctions = []*jsonnet.NativeFunction{
	MustEnvNativeFunction,
	EnvNativeFunction,
	JsonescapeNativeFunction,
	Base64EncodeNativeFunction,
	CELCapableNativeFunction,
	SwitchNativeFunction,
}

var MustEnvNativeFunction = &jsonnet.NativeFunction{
	Name:   "must_env",
	Params: []ast.Identifier{"name"},
	Func: func(args []interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, oops.Errorf("must_env: invalid arguments length expected 1 got %d", len(args))
		}
		key, ok := args[0].(string)
		if !ok {
			return nil, oops.Errorf("must_env: invalid arguments, expected string got %T", args[0])
		}
		val, ok := os.LookupEnv(key)
		if !ok {
			return nil, oops.Errorf("must_env: %s not set", key)
		}
		return val, nil
	},
}
var EnvNativeFunction = &jsonnet.NativeFunction{
	Name:   "env",
	Params: []ast.Identifier{"name", "default"},
	Func: func(args []interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, oops.Errorf("env: invalid arguments length expected 2 got %d", len(args))
		}
		key, ok := args[0].(string)
		if !ok {
			return nil, oops.Errorf("env: invalid 1st arguments, expected string got %T", args[0])
		}
		val := os.Getenv(key)
		if val == "" {
			return args[1], nil
		}
		return val, nil
	},
}

var JsonescapeNativeFunction = &jsonnet.NativeFunction{
	Name:   "json_escape",
	Params: []ast.Identifier{"str"},
	Func: func(args []interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, oops.Errorf("jsonescape: invalid arguments length expected 1 got %d", len(args))
		}
		str, ok := args[0].(string)
		if !ok {
			return nil, oops.Errorf("jsonescape: invalid arguments, expected string got %T", args[0])
		}
		bs, err := json.Marshal(str)
		if err != nil {
			return nil, oops.Wrapf(err, "jsonescape")
		}
		return string(bs), nil
	},
}

var Base64EncodeNativeFunction = &jsonnet.NativeFunction{
	Name:   "base64_encode",
	Params: []ast.Identifier{"data"},
	Func: func(args []interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, oops.Errorf("base64_encode: invalid arguments length expected 1 got %d", len(args))
		}
		var data []byte
		str, ok := args[0].(string)
		if ok {
			data = []byte(str)
		} else {
			data, ok = args[0].([]byte)
			if !ok {
				return nil, oops.Errorf("base64_encode: invalid arguments, expected string or []byte got %T", args[0])
			}
		}
		return base64.StdEncoding.EncodeToString(data), nil
	},
}

var CELCapableNativeFunction = &jsonnet.NativeFunction{
	Name:   "cel",
	Params: []ast.Identifier{"expr"},
	Func: func(args []interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, oops.Errorf("cel: invalid arguments length expected 1 got %d", len(args))
		}
		str, ok := args[0].(string)
		if !ok {
			return nil, oops.Errorf("cel: invalid arguments, expected string got %T", args[0])
		}
		return map[string]interface{}{
			"expr": str,
		}, nil
	},
}

var SwitchNativeFunction = &jsonnet.NativeFunction{
	Name:   "switch",
	Params: []ast.Identifier{"cases"},
	Func: func(args []interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, oops.Errorf("switch: invalid arguments length expected 1 got %d", len(args))
		}
		cases, ok := args[0].([]any)
		if !ok {
			return nil, oops.Errorf("switch: invalid arguments, expected string got %T", args[0])
		}
		defaultCount := 0
		for i, c := range cases {
			v, ok := c.(map[string]any)
			if !ok {
				return nil, oops.Errorf("switch: invalid arguments, expected map[string]interface{} got %T", c)
			}
			caseField, ok := v["case"]
			if !ok {
				defaultField, ok := v["default"]
				if !ok {
					return nil, oops.Errorf("switch: invalid arguments, expected string case")
				}
				defaultCount++
				if defaultExpr, ok := castCELExpr(defaultField); ok {
					cases[i] = map[string]any{
						"default_expr": defaultExpr,
					}
				}
				continue
			}
			caseExpr, ok := castCELExpr(caseField)
			if !ok {
				return nil, oops.Errorf("switch: case must be a CEL expression")
			}
			valueField, ok := v["value"]
			if !ok {
				return nil, oops.Errorf("cel: invalid arguments, need value")
			}

			if valueExpr, ok := castCELExpr(valueField); ok {
				cases[i] = map[string]any{
					"case":       caseExpr,
					"value_expr": valueExpr,
				}
				continue
			}
			cases[i] = map[string]any{
				"case":  caseExpr,
				"value": valueField,
			}
		}
		if defaultCount > 1 {
			return nil, oops.Errorf("cel: multiple default values in switch")
		}
		return map[string]interface{}{
			"switch": cases,
		}, nil
	},
}

func castCELExpr(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	m, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	str, ok := m["expr"].(string)
	if !ok {
		return "", false
	}
	return str, ok
}
