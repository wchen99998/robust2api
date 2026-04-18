package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestMetricsPrometheusScrapeIncludesObservabilityContracts(t *testing.T) {
	m, meterProvider, registry := newTestMetrics(t)

	ctx := context.Background()
	m.RecordRequest(ctx, http.MethodPost, "/v1/responses", http.StatusOK, "openai")
	m.RecordDuration(ctx, 16, http.MethodPost, "/v1/responses", http.StatusOK, "openai")
	m.RecordTTFT(ctx, 2.5, "openai", "gpt-5.1")
	m.RecordTokens(ctx, 100, "input", "openai", "gpt-5.1")
	m.RecordTokens(ctx, 200, "output", "openai", "gpt-5.1")
	m.RecordUpstreamDuration(ctx, 1.25, "openai", "ws", "502", "http_error")
	m.SetConcurrencyQueueDepth(ctx, 0)
	m.RecordBillingPublish(ctx, "success", 0.05)
	m.RecordBillingApply(ctx, "failure")
	m.SetBillingPendingMessages(ctx, 7)

	require.NoError(t, meterProvider.ForceFlush(ctx))

	body := scrapePrometheus(t, registry)
	require.Contains(t, body, `sub2api_tokens_total{direction="input",model="gpt-5.1",platform="openai"} 100`)
	require.Contains(t, body, `sub2api_tokens_total{direction="output",model="gpt-5.1",platform="openai"} 200`)
	require.Contains(t, body, `sub2api_http_request_duration_seconds_bucket{http_method="POST",http_route="/v1/responses",http_status_code="200",platform="openai",le="15"} 0`)
	require.Contains(t, body, `sub2api_http_request_duration_seconds_bucket{http_method="POST",http_route="/v1/responses",http_status_code="200",platform="openai",le="20"} 1`)
	require.Contains(t, body, `sub2api_http_request_ttft_seconds_bucket{model="gpt-5.1",platform="openai",le="3"} 1`)
	require.Contains(t, body, `sub2api_upstream_request_duration_seconds_bucket{outcome="http_error",platform="openai",status_code="502",transport="ws",le="2.5"} 1`)
	require.Contains(t, body, "sub2api_concurrency_queue_depth 0")
	require.Contains(t, body, `sub2api_billing_publish_total{outcome="success"} 1`)
	require.Contains(t, body, `sub2api_billing_apply_failures_total{outcome="failure"} 1`)
	require.Contains(t, body, "sub2api_billing_pending_messages 7")
}

func newTestMetrics(t *testing.T) (*Metrics, *sdkmetric.MeterProvider, *prometheus.Registry) {
	t.Helper()

	registry := prometheus.NewRegistry()
	exporter, err := promexporter.New(
		promexporter.WithRegisterer(registry),
		promexporter.WithoutScopeInfo(),
		promexporter.WithoutTargetInfo(),
	)
	require.NoError(t, err)

	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(meterProvider)

	globalMetrics = nil
	globalMetricsOnce = sync.Once{}

	t.Cleanup(func() {
		require.NoError(t, meterProvider.Shutdown(context.Background()))
		otel.SetMeterProvider(previousProvider)
		globalMetrics = nil
		globalMetricsOnce = sync.Once{}
	})

	m, err := NewMetrics()
	require.NoError(t, err)
	return m, meterProvider, registry
}

func scrapePrometheus(t *testing.T, registry *prometheus.Registry) string {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	return strings.TrimSpace(recorder.Body.String())
}
