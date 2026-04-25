package usage

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func FromOpenAIForwardResult(plan core.RoutingPlan, apiKey *service.APIKey, account *service.Account, result *service.OpenAIForwardResult, now time.Time) *core.UsageEvent {
	if result == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	started := now.Add(-result.Duration)
	if result.Duration <= 0 {
		started = now
	}
	event := &core.UsageEvent{
		RequestID:             result.RequestID,
		Provider:              service.PlatformOpenAI,
		RequestedModel:        result.Model,
		BillingModel:          plan.Billing.Model,
		InputTokens:           int64(result.Usage.InputTokens),
		OutputTokens:          int64(result.Usage.OutputTokens),
		CacheCreationTokens:   int64(result.Usage.CacheCreationInputTokens),
		CacheReadTokens:       int64(result.Usage.CacheReadInputTokens),
		ImageOutputTokens:     int64(result.Usage.ImageOutputTokens),
		Status:                "completed",
		StartedAt:             started,
		CompletedAt:           now,
		PayloadFingerprint:    plan.Billing.PayloadFingerprint,
		BillingIdempotencyKey: plan.Billing.IdempotencyKey,
	}
	if apiKey != nil {
		event.APIKeyID = apiKey.ID
		event.UserID = apiKey.UserID
	}
	if account != nil {
		event.AccountID = account.ID
	}
	if event.BillingModel == "" {
		event.BillingModel = result.BillingModel
	}
	if event.BillingModel == "" {
		event.BillingModel = result.Model
	}
	return event
}
