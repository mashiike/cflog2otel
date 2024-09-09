package otlptest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/samber/oops"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
)

type metricServiceServer struct {
	collectormetrics.UnimplementedMetricsServiceServer
	exporter Exporter
}

type Exporter interface {
	Export(context.Context, *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error)
}

type ExporterFunc func(context.Context, *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error)

func (f ExporterFunc) Export(ctx context.Context, req *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error) {
	return f(ctx, req)
}

func newServiceServer(exporter Exporter) *metricServiceServer {
	return &metricServiceServer{exporter: exporter}
}

// Export implements the gRPC service to handle the export of metrics
func (m *metricServiceServer) Export(ctx context.Context, req *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error) {
	return m.exporter.Export(ctx, req)
}

type MetricsCollector struct {
	URL      string
	Listener net.Listener
	server   *grpc.Server

	mu            sync.Mutex
	closed        bool
	wg            sync.WaitGroup
	serviceServer *metricServiceServer
}

func NewExporterWithWriter(w io.Writer) Exporter {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return ExporterFunc(func(ctx context.Context, req *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error) {
		if err := enc.Encode(req); err != nil {
			return nil, oops.Wrapf(err, "failed to encode request")
		}
		return &collectormetrics.ExportMetricsServiceResponse{}, nil
	})
}

func NewMetricsCollector(exporter Exporter) *MetricsCollector {
	server := NewUnstartedMetricsCollector(exporter)
	server.Start()
	return server
}

func NewUnstartedMetricsCollector(exporter Exporter) *MetricsCollector {
	serviceServer := newServiceServer(exporter)
	server := grpc.NewServer()
	collectormetrics.RegisterMetricsServiceServer(server, serviceServer)
	return &MetricsCollector{
		Listener:      newLocalListener(),
		serviceServer: serviceServer,
		server:        server,
	}
}

func newLocalListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if l, err = net.Listen("tcp6", "[::1]:0"); err != nil {
			panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
		}
	}
	return l
}

func (mc *MetricsCollector) Start() {
	if mc.URL != "" {
		panic("Server already started")
	}
	mc.URL = "http://" + mc.Listener.Addr().String()
	mc.goServe()
}

func (mc *MetricsCollector) Close() {
	mc.mu.Lock()
	if !mc.closed {
		mc.closed = true
		mc.Listener.Close()
		mc.server.GracefulStop()
	}
	mc.mu.Unlock()
	mc.wg.Wait()
}

func (mc *MetricsCollector) goServe() {
	mc.wg.Add(1)
	go func() {
		defer mc.wg.Done()
		if err := mc.server.Serve(mc.Listener); err != nil {
			slog.Error("Failed to serve gRPC server", "error", err)
		}
	}()
}
