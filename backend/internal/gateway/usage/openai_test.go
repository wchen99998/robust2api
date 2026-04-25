package usage

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestFromOpenAIForwardResult(t *testing.T) {
	now := time.Date(2026, 4, 25, 1, 2, 3, 0, time.UTC)
	plan := core.RoutingPlan{
		Billing: core.BillingPlan{
			Model:              "gpt-5.4-billing",
			IdempotencyKey:     "req_123",
			PayloadFingerprint: "payload",
		},
	}
	apiKey := &service.APIKey{ID: 7, UserID: 8}
	account := &service.Account{ID: 9}
	result := &service.OpenAIForwardResult{
		RequestID: "req_123",
		Model:     "gpt-5.4",
		Usage: service.OpenAIUsage{
			InputTokens:              10,
			OutputTokens:             20,
			CacheCreationInputTokens: 3,
			CacheReadInputTokens:     4,
			ImageOutputTokens:        5,
		},
		Duration: time.Second,
	}

	event := FromOpenAIForwardResult(plan, apiKey, account, result, now)
	if event.APIKeyID != 7 || event.UserID != 8 || event.AccountID != 9 {
		t.Fatalf("wrong ownership fields: %+v", event)
	}
	if event.BillingModel != "gpt-5.4-billing" {
		t.Fatalf("billing model = %q", event.BillingModel)
	}
	if event.InputTokens != 10 || event.OutputTokens != 20 || event.CacheCreationTokens != 3 || event.CacheReadTokens != 4 || event.ImageOutputTokens != 5 {
		t.Fatalf("wrong token fields: %+v", event)
	}
	if event.BillingIdempotencyKey != "req_123" || event.PayloadFingerprint != "payload" {
		t.Fatalf("wrong idempotency fields: %+v", event)
	}
}
