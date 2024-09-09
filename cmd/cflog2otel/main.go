package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/fatih/color"
	"github.com/fujiwara/lamblocal"
	"github.com/ken39arg/go-flagx"
	"github.com/mashiike/cflog2otel"
	"github.com/mashiike/cflog2otel/otlptest"
	"github.com/mashiike/slogutils"
	"github.com/samber/oops"
)

func main() {
	ctx := context.Background()
	if err := _main(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to run", "details", err)
		os.Exit(1)
	}
}

func _main(ctx context.Context) error {
	var (
		logLevel           string
		logPrettify        bool
		configPath         string
		configValidateOnly bool
		renderConfig       bool
		localExporter      bool
		s3URL              string
	)
	flag.StringVar(&logLevel, "log-level", "info", "log level ($LOG_LEVEL)")
	flag.BoolVar(&logPrettify, "log-prettify", false, "log prettify ($LOG_PRETTIFY)")
	flag.StringVar(&configPath, "config", "cflog2otel.jsonnet", "config file path ($CONFIG)")
	flag.BoolVar(&configValidateOnly, "config-validate-only", false, "validate config only ($CONFIG_VALIDATE_ONLY)")
	flag.BoolVar(&renderConfig, "render-config", false, "render config only ($RENDER_CONFIG)")
	flag.StringVar(&s3URL, "s3-url", "", "s3 notification url ($S3_URL)")
	flag.BoolVar(&localExporter, "local-collector", false, "use with test collector, export to stdout ($LOCAL_COLLECTOR)")
	flag.VisitAll(flagx.EnvToFlag)
	flag.Parse()

	setupLogger(logLevel, logPrettify)
	cfg := cflog2otel.DefaultConfig()
	if err := cfg.Load(configPath, cflog2otel.WithContext(ctx)); err != nil {
		return oops.Wrapf(err, "failed to load config")
	}
	if configValidateOnly {
		return nil
	}
	if renderConfig {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cfg); err != nil {
			return oops.Wrapf(err, "failed to render config")
		}
		return nil
	}
	if localExporter {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		collector := otlptest.NewMetricsCollector(otlptest.NewExporterWithWriter(os.Stdout))
		cfg.Otel = cflog2otel.OtelConfig{
			Endpoint: collector.URL,
			GZip:     true,
		}
		if err := cfg.Otel.Validate(); err != nil {
			return oops.Wrapf(err, "failed to validate otel config")
		}
		slog.Info("start local collector", "url", collector.URL)
	}
	app, err := cflog2otel.New(ctx, cfg)
	if err != nil {
		return oops.Wrapf(err, "failed to create app")
	}
	if s3URL != "" {
		u, err := url.Parse(s3URL)
		if err != nil {
			return oops.Wrapf(err, "failed to parse s3 url")
		}
		if u.Scheme != "s3" {
			return oops.Errorf("invalid s3 url")
		}
		dummyEvent, err := generateDummyS3Notification(u)
		if err != nil {
			return oops.Wrapf(err, "failed to generate dummy s3 notification")
		}
		lamblocal.CLISrc = strings.NewReader(dummyEvent)
	}
	return lamblocal.RunWithError(ctx, app.Invoke)
}

func setupLogger(logLevel string, logPrettify bool) {
	var level slog.Level
	var parseErr error
	if parseErr = level.UnmarshalText([]byte(logLevel)); parseErr != nil {
		level = slog.LevelInfo
	}
	modifiers := map[slog.Level]slogutils.ModifierFunc{
		slog.LevelDebug: slogutils.Color(color.FgBlack),
		slog.LevelInfo:  nil,
		slog.LevelWarn:  slogutils.Color(color.FgYellow),
		slog.LevelError: slogutils.Color(color.FgRed, color.Bold),
	}
	if logPrettify {
		prettifyMiddleware := func(m slogutils.ModifierFunc) slogutils.ModifierFunc {
			if m == nil {
				m = func(b []byte) []byte {
					return b
				}
			}
			return func(b []byte) []byte {
				if !json.Valid(b) {
					return m(b)
				}
				var buf bytes.Buffer
				if err := json.Indent(&buf, b, "", "  "); err != nil {
					return m(b)
				}
				return m(buf.Bytes())
			}
		}
		for k, v := range modifiers {
			modifiers[k] = prettifyMiddleware(v)
		}
	}
	middleware := slogutils.NewMiddleware(
		slog.NewJSONHandler,
		slogutils.MiddlewareOptions{
			ModifierFuncs: modifiers,
			Writer:        os.Stderr,
			HandlerOptions: &slog.HandlerOptions{
				Level: level,
			},
		},
	)
	slog.SetDefault(slog.New(middleware))
	if parseErr != nil {
		slog.Warn("failed to parse log level,fallback to info", "details", parseErr, "log_level", logLevel)
	}
}

func generateDummyS3Notification(u *url.URL) (string, error) {
	e := events.S3Event{
		Records: []events.S3EventRecord{
			{
				EventVersion: "2.1",
				EventSource:  "aws:s3",
				AWSRegion:    os.Getenv("AWS_REGION"),
				EventTime:    time.Now(),
				EventName:    "ObjectCreated:Put",
				S3: events.S3Entity{
					SchemaVersion:   "1.0",
					ConfigurationID: "testConfigRule",
					Bucket: events.S3Bucket{
						Name: u.Host,
						Arn:  fmt.Sprintf("arn:aws:s3:::%s", u.Host),
					},
					Object: events.S3Object{
						Key:       strings.TrimPrefix(u.Path, "/"),
						Size:      1024,
						ETag:      "0123456789abcdef0123456789abcdef",
						VersionID: "096fKKXTRTtl3on89fVO.nfljtsv6qko",
						Sequencer: "0A1B2C3D4E5F678901",
					},
				},
			},
		},
	}
	bs, err := json.Marshal(e)
	if err != nil {
		return "", oops.Wrapf(err, "failed to marshal s3 event")
	}
	return string(bs), nil
}
