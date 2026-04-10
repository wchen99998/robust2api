package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var ErrUsageBillingRequestIDRequired = errors.New("usage billing request_id is required")
var ErrUsageBillingRequestConflict = errors.New("usage billing request fingerprint conflict")

// UsageBillingCommand describes one billable request that must be applied at most once.
type UsageBillingCommand struct {
	RequestID          string `json:"request_id"`
	APIKeyID           int64  `json:"api_key_id"`
	RequestFingerprint string `json:"request_fingerprint"`
	RequestPayloadHash string `json:"request_payload_hash"`

	UserID              int64  `json:"user_id"`
	AccountID           int64  `json:"account_id"`
	SubscriptionID      *int64 `json:"subscription_id,omitempty"`
	AccountType         string `json:"account_type"`
	Model               string `json:"model"`
	ServiceTier         string `json:"service_tier,omitempty"`
	ReasoningEffort     string `json:"reasoning_effort,omitempty"`
	BillingType         int8   `json:"billing_type"`
	InputTokens         int    `json:"input_tokens"`
	OutputTokens        int    `json:"output_tokens"`
	CacheCreationTokens int    `json:"cache_creation_tokens"`
	CacheReadTokens     int    `json:"cache_read_tokens"`
	ImageCount          int    `json:"image_count"`
	MediaType           string `json:"media_type,omitempty"`

	BalanceCost         float64 `json:"balance_cost"`
	SubscriptionCost    float64 `json:"subscription_cost"`
	APIKeyQuotaCost     float64 `json:"api_key_quota_cost"`
	APIKeyRateLimitCost float64 `json:"api_key_rate_limit_cost"`
	AccountQuotaCost    float64 `json:"account_quota_cost"`
}

func (c *UsageBillingCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	if strings.TrimSpace(c.RequestFingerprint) == "" {
		c.RequestFingerprint = buildUsageBillingFingerprint(c)
	}
}

func buildUsageBillingFingerprint(c *UsageBillingCommand) string {
	if c == nil {
		return ""
	}
	raw := fmt.Sprintf(
		"%d|%d|%d|%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|%s|%d|%0.10f|%0.10f|%0.10f|%0.10f|%0.10f",
		c.UserID,
		c.AccountID,
		c.APIKeyID,
		strings.TrimSpace(c.AccountType),
		strings.TrimSpace(c.Model),
		strings.TrimSpace(c.ServiceTier),
		strings.TrimSpace(c.ReasoningEffort),
		c.BillingType,
		c.InputTokens,
		c.OutputTokens,
		c.CacheCreationTokens,
		c.CacheReadTokens,
		c.ImageCount,
		strings.TrimSpace(c.MediaType),
		valueOrZero(c.SubscriptionID),
		c.BalanceCost,
		c.SubscriptionCost,
		c.APIKeyQuotaCost,
		c.APIKeyRateLimitCost,
		c.AccountQuotaCost,
	)
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func HashUsageRequestPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func valueOrZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

type UsageBillingApplyResult struct {
	Applied                          bool
	UsageLogInserted                 bool
	APIKeyQuotaExhausted             bool
	APIKeyAuthCacheKey               string
	NeedsAPIKeyAuthCacheInvalidation bool
}

type UsageBillingRepository interface {
	Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error)
	ApplyUsageCharge(ctx context.Context, event *UsageChargeEvent) (*UsageBillingApplyResult, error)
}
