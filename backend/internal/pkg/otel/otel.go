package otel

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

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
	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.Endpoint+"/v1/traces"),
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

	// --- Meter ---
	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(cfg.Endpoint+"/v1/metrics"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating metric exporter: %w", err)
	}

	promExp, err := promexporter.New()
	if err != nil {
		return nil, fmt.Errorf("creating prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithReader(promExp),
	)
	otel.SetMeterProvider(mp)

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

// ProvideMetrics is a Wire provider for application metrics.
func ProvideMetrics() (*Metrics, error) {
	return NewMetrics()
}

// ProvideMetricsServer is a Wire provider for the internal metrics server.
func ProvideMetricsServer(cfg *config.Config, provider *Provider) *MetricsServer {
	if !cfg.Otel.Enabled {
		return nil
	}
	srv := NewMetricsServer(cfg.Otel.MetricsPort, provider.PrometheusExporter())
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()
	return srv
}
