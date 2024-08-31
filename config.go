package cflog2otel

import (
	"encoding/json"
	"log/slog"
	"net/url"

	"github.com/samber/oops"
)

type Config struct {
	Otel               OtelConfig        `json:"otel,omitempty"`
	ResourceAttributes []AttributeConfig `json:"resource_attributes,omitempty"`
	Scope              ScopeConfig       `json:"scope,omitempty"`
	Metrics            []MetricsConfig   `json:"metrics,omitempty"`
}

type OtelConfig struct {
	Headers  map[string]string `json:"headers,omitempty"`
	Endpoint string            `json:"endpoint,omitempty"`
	GZip     bool              `json:"gzip,omitempty"`
	endpoint *url.URL          `json:"-"`
}

type AttributeConfig struct {
	Key   string           `json:"key,omitempty"`
	Value *CELCapable[any] `json:"value,omitempty"`
}

type ScopeConfig struct {
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	SchemaURL string `json:"schema_url,omitempty"`
}

type MetricsConfig struct {
	Name         string               `json:"name,omitempty"`
	Description  string               `json:"description,omitempty"`
	Unit         string               `json:"unit,omitempty"`
	Type         AggregationType      `json:"type,omitempty"`
	Attributes   []AttributeConfig    `json:"attributes,omitempty"`
	Filter       *CELCapable[bool]    `json:"filter,omitempty"`
	Value        *CELCapable[float64] `json:"value,omitempty"`
	IsMonotonic  bool                 `json:"is_monotonic,omitempty"`
	IsCumulative bool                 `json:"is_cumulative,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{}
}

func (c *Config) Load(path string, opts ...JsonnetOption) error {
	vm := MakeVM(opts...)
	jsonStr, err := vm.EvaluateFile(path)
	if err != nil {
		return oops.Errorf("failed to evaluate JSONnet file: %w", err)
	}
	err = json.Unmarshal([]byte(jsonStr), c)
	if err != nil {
		return oops.Errorf("failed to unmarshal JSON: %w", err)
	}
	return c.Validate()
}

func (c *Config) Validate() error {
	if err := c.Otel.Validate(); err != nil {
		return oops.Wrapf(err, "otel")
	}
	for i, a := range c.ResourceAttributes {
		if err := a.Validate(); err != nil {
			return oops.Wrapf(err, "resource_attributes[%d]", i)
		}
	}
	if err := c.Scope.Validate(); err != nil {
		return oops.Wrapf(err, "scope")
	}
	for i, m := range c.Metrics {
		if err := m.Validate(); err != nil {
			return oops.Wrapf(err, "metrics[%d]", i)
		}
	}
	return nil
}

func (c *ScopeConfig) Validate() error {
	return nil
}

func (c *MetricsConfig) Validate() error {
	if c.Name == "" {
		return oops.Errorf("name is required")
	}
	for i, a := range c.Attributes {
		if err := a.Validate(); err != nil {
			return oops.Wrapf(err, "attributes[%d]", i)
		}
	}
	switch c.Type {
	case AggregationTypeCount:
		if c.Value != nil {
			slog.Warn("value is ignored for metric type \"Count\"")
		}
	case AggregationTypeSum:
		if c.Value == nil {
			return oops.Errorf("value is required for metric type \"Sum\"")
		}
	default:
		return oops.Errorf("unsupported metric type: %s", c.Type)
	}
	return nil
}

func (c *AttributeConfig) Validate() error {
	if c.Key == "" {
		return oops.Errorf("key is required")
	}
	if c.Value == nil {
		return oops.Errorf("value is required")
	}
	return nil
}

func (c *OtelConfig) SetEndpointURL(endpoint string) error {
	if endpoint == "" {
		return oops.Errorf("endpoint is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return oops.Wrapf(err, "endpoint")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return oops.Errorf("endpoint must be http or https")
	}
	c.endpoint = u
	return nil
}

func (c *OtelConfig) EndpointURL() *url.URL {
	return c.endpoint
}

func (c *OtelConfig) Validate() error {
	if c.Endpoint == "" {
		c.Endpoint = "http://localhost:4317"
	}
	if err := c.SetEndpointURL(c.Endpoint); err != nil {
		return err
	}
	return nil
}
