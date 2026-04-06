package otel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// runtimeStartOnce ensures runtime.Start is called at most once across all Init invocations
// (e.g. multiple test cases that each call Init).
var runtimeStartOnce sync.Once

// Provider holds the initialized OTel providers and exposes a Shutdown method.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	promExporter   *promexporter.Exporter
	enabled        bool
}

func (p *Provider) TracerProvider() *sdktrace.TracerProvider {
	return p.tracerProvider
}

func (p *Provider) MeterProvider() *sdkmetric.MeterProvider {
	return p.meterProvider
}

func (p *Provider) PrometheusExporter() *promexporter.Exporter {
	return p.promExporter
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if !p.enabled {
		return nil
	}
	// Best-effort shutdown: flush in-flight spans/metrics.
	// Export errors (e.g. collector unreachable) are intentionally ignored
	// so that application shutdown is not blocked or failed by telemetry issues.
	if p.tracerProvider != nil {
		_ = p.tracerProvider.Shutdown(ctx)
	}
	if p.meterProvider != nil {
		_ = p.meterProvider.Shutdown(ctx)
	}
	return nil
}

// Init initializes OTel tracing and metrics providers.
// Traces are exported via OTLP/gRPC. Metrics are exposed via Prometheus scrape only (no OTLP push).
// If cfg.Enabled is false, returns a no-op Provider.
func Init(ctx context.Context, cfg *config.OtelConfig) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{enabled: false}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// --- Tracer ---
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating trace exporter: %w", err)
	}

	sampler := sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(cfg.TraceSampleRate),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)

	// --- Meter (Prometheus scrape only, no OTLP push) ---
	promExp, err := promexporter.New()
	if err != nil {
		return nil, fmt.Errorf("creating prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExp),
	)
	otel.SetMeterProvider(mp)

	// Start Go runtime metrics collection (goroutines, memory, GC).
	// Guarded by sync.Once so repeated calls (e.g. in tests) don't return an error.
	var runtimeErr error
	runtimeStartOnce.Do(func() {
		runtimeErr = runtime.Start(runtime.WithMinimumReadMemStatsInterval(15 * time.Second))
	})
	if runtimeErr != nil {
		return nil, fmt.Errorf("starting runtime metrics: %w", runtimeErr)
	}

	return &Provider{
		tracerProvider: tp,
		meterProvider:  mp,
		promExporter:   promExp,
		enabled:        true,
	}, nil
}

// ProvideOtel is a Wire provider that initializes the OTel SDK.
func ProvideOtel(cfg *config.Config) (*Provider, error) {
	return Init(context.Background(), &cfg.Otel)
}

// ProvideMetricsServer is a Wire provider for the internal metrics server.
// It only constructs the server; the caller (main.go) is responsible for starting it
// so that no goroutine is orphaned if a later Wire provider fails.
func ProvideMetricsServer(cfg *config.Config, provider *Provider) *MetricsServer {
	if !cfg.Otel.Enabled {
		return nil
	}
	return NewMetricsServer(cfg.Otel.MetricsPort, provider.PrometheusExporter())
}
