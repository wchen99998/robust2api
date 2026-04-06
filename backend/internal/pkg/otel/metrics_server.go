package otel

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
)

// MetricsServer serves Prometheus metrics and pprof on an internal port.
type MetricsServer struct {
	server *http.Server
}

// NewMetricsServer creates a metrics server bound to the given port.
// Pass port=0 to auto-select an available port.
func NewMetricsServer(port int, promExp *promexporter.Exporter) *MetricsServer {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	if promExp != nil {
		mux.Handle("/metrics", promhttp.Handler())
	}

	// pprof endpoints
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Health check for the metrics server itself
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &MetricsServer{
		server: &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Start begins listening. It blocks until the server stops.
func (s *MetricsServer) Start() error {
	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("metrics server listen: %w", err)
	}
	log.Printf("Metrics server listening on %s", ln.Addr().String())
	return s.server.Serve(ln)
}

// Shutdown gracefully stops the metrics server.
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
