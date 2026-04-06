package otel

import (
	"context"
	"testing"
)

func TestMetrics_RecordRequest(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	// Should not panic when recording metrics
	m.RecordRequest(context.Background(), "POST", "/v1/messages", 200, "anthropic")
	m.RecordDuration(context.Background(), 0.150, "POST", "/v1/messages", 200, "anthropic")
	m.RecordTTFT(context.Background(), 0.050, "anthropic", "claude-sonnet-4-20250514")
	m.RecordTokens(context.Background(), 100, "input", "anthropic", "claude-sonnet-4-20250514")
	m.RecordTokens(context.Background(), 200, "output", "anthropic", "claude-sonnet-4-20250514")
	m.RecordUpstreamError(context.Background(), "anthropic", "429")
	m.RecordAccountFailover(context.Background(), "anthropic")
	m.RecordRateLimitRejection(context.Background(), "api_key")
	m.SetConcurrencyQueueDepth(context.Background(), 42)
	m.SetUpstreamAccountsActive(context.Background(), 10, "anthropic")
}
