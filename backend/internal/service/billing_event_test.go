package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUsageChargeEventJSONSchemaUsesExplicitFieldNames(t *testing.T) {
	now := time.Unix(1_710_000_000, 0).UTC()
	serviceTier := "priority"
	userAgent := "codex-cli/1.0"

	event := NewUsageChargeEvent(
		&UsageBillingCommand{
			RequestID:          "req-123",
			APIKeyID:           12,
			RequestFingerprint: "fp-123",
			RequestPayloadHash: "payload-123",
			UserID:             34,
			AccountID:          56,
			AccountType:        AccountTypeAPIKey,
			Model:              "gpt-5.1",
			ServiceTier:        serviceTier,
			BillingType:        BillingTypeBalance,
			InputTokens:        10,
			OutputTokens:       5,
			BalanceCost:        1.25,
		},
		&UsageLog{
			UserID:          34,
			APIKeyID:        12,
			AccountID:       56,
			RequestID:       "req-123",
			Model:           "gpt-5.1",
			RequestedModel:  "gpt-5.1",
			InputTokens:     10,
			OutputTokens:    5,
			BillingType:     BillingTypeBalance,
			UserAgent:       &userAgent,
			ServiceTier:     &serviceTier,
			CreatedAt:       now,
			RateMultiplier:  1.1,
			RequestType:     RequestTypeSync,
			ImageCount:      0,
			ImageOutputCost: 0,
		},
		78,
		true,
	)
	event.EventID = "evt-123"
	event.OccurredAt = now

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	command, ok := decoded["command"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "charge", decoded["kind"])
	require.Contains(t, command, "request_id")
	require.Contains(t, command, "api_key_id")
	require.NotContains(t, command, "RequestID")

	usageLog, ok := decoded["usage_log"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, usageLog, "user_id")
	require.Contains(t, usageLog, "created_at")
	require.NotContains(t, usageLog, "User")
	require.NotContains(t, usageLog, "APIKey")

	var roundTripped UsageChargeEvent
	require.NoError(t, json.Unmarshal(data, &roundTripped))
	require.Equal(t, UsageChargeEventKindCharge, roundTripped.Kind)
	require.NotNil(t, roundTripped.Command)
	require.Equal(t, "req-123", roundTripped.Command.RequestID)
	require.NotNil(t, roundTripped.UsageLog)
	require.Equal(t, "req-123", roundTripped.UsageLog.RequestID)
	require.Equal(t, RequestTypeSync, roundTripped.UsageLog.RequestType)
}
