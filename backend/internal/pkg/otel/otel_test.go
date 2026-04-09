package otel

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInit_Disabled(t *testing.T) {
	cfg := &config.OtelConfig{Enabled: false}
	provider, err := Init(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.NoError(t, provider.Shutdown(context.Background()))
}

func TestInit_Enabled_NoEndpoint(t *testing.T) {
	cfg := &config.OtelConfig{
		Enabled:         true,
		ServiceName:     "test-service",
		Endpoint:        "http://localhost:4318",
		TraceSampleRate: 1.0,
		MetricsPort:     0,
	}
	provider, err := Init(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.NotNil(t, provider.TracerProvider())
	require.NotNil(t, provider.MeterProvider())
	require.NoError(t, provider.Shutdown(context.Background()))
}

func TestFilteredTraceExporter_DropsFastRedisSpans(t *testing.T) {
	capture := &captureSpanExporter{}
	exporter := newFilteredTraceExporter(capture)

	err := exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{
		testSpanStub("redis.get", "github.com/redis/go-redis/extra/redisotel", 500*time.Microsecond, codes.Unset).Snapshot(),
		testSpanStub("redis.dial", "github.com/redis/go-redis/extra/redisotel", 5*time.Millisecond, codes.Unset).Snapshot(),
		testSpanStub("http.request", "sub2api.gateway", 250*time.Microsecond, codes.Unset).Snapshot(),
	})
	require.NoError(t, err)

	require.Equal(t, []string{"http.request"}, capture.names())
}

func TestFilteredTraceExporter_KeepsSlowAndErroredRedisSpans(t *testing.T) {
	capture := &captureSpanExporter{}
	exporter := newFilteredTraceExporter(capture)

	err := exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{
		testSpanStub("redis.evalsha", "github.com/redis/go-redis/extra/redisotel", 2*time.Millisecond, codes.Unset).Snapshot(),
		testSpanStub("redis.get", "github.com/redis/go-redis/extra/redisotel", 250*time.Microsecond, codes.Error).Snapshot(),
		testSpanStub("db.query", "github.com/XSAM/otelsql", 250*time.Microsecond, codes.Unset).Snapshot(),
	})
	require.NoError(t, err)

	require.Equal(t, []string{"redis.evalsha", "redis.get", "db.query"}, capture.names())
}

type captureSpanExporter struct {
	spans []sdktrace.ReadOnlySpan
}

func (e *captureSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *captureSpanExporter) Shutdown(context.Context) error {
	return nil
}

func (e *captureSpanExporter) names() []string {
	names := make([]string, 0, len(e.spans))
	for _, span := range e.spans {
		names = append(names, span.Name())
	}
	return names
}

func testSpanStub(name, scope string, duration time.Duration, status codes.Code) tracetest.SpanStub {
	start := time.Unix(1700000000, 0)
	return tracetest.SpanStub{
		Name:      name,
		StartTime: start,
		EndTime:   start.Add(duration),
		Status: sdktrace.Status{
			Code: status,
		},
		InstrumentationScope: instrumentation.Scope{Name: scope},
	}
}
