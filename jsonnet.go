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
	MastEnvNativeFunction,
	EnvNativeFunction,
	JsonescapeNativeFunction,
	Base64EncodeNativeFunction,
	CELCapableNativeFunction,
	CELSwitchNativeFunction,
}

var MastEnvNativeFunction = &jsonnet.NativeFunction{
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

var CELSwitchNativeFunction = &jsonnet.NativeFunction{
	Name:   "cel_switch",
	Params: []ast.Identifier{"cases"},
	Func: func(args []interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, oops.Errorf("cel: invalid arguments length expected 1 got %d", len(args))
		}
		cases, ok := args[0].([]any)
		if !ok {
			return nil, oops.Errorf("cel: invalid arguments, expected string got %T", args[0])
		}
		defaultCount := 0
		for _, c := range cases {
			v, ok := c.(map[string]interface{})
			if !ok {
				return nil, oops.Errorf("cel: invalid arguments, expected map[string]interface{} got %T", c)
			}
			if _, ok := v["case"].(string); !ok {
				if _, ok := v["default"]; !ok {
					return nil, oops.Errorf("cel: invalid arguments, expected string case")
				}
				defaultCount++
				continue
			}
			if _, ok := v["value"]; !ok {
				return nil, oops.Errorf("cel: invalid arguments, need value")
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
