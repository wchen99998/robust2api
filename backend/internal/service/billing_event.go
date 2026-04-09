package service

import (
	"context"
	"time"
)

// BillingEvent is the message published to the billing stream.
// It contains everything the billing service needs to apply the billing transaction,
// write the usage log, and update the billing cache.
type BillingEvent struct {
	// Command holds the billing transaction data (balance/subscription/quota deductions).
	Command *UsageBillingCommand `json:"command"`

	// UsageLog holds the usage record to persist after billing succeeds.
	UsageLog *UsageLog `json:"usage_log,omitempty"`

	// Cache update metadata not carried in UsageBillingCommand.
	GroupID       int64  `json:"group_id,omitempty"`
	APIKeyKey     string `json:"api_key_key,omitempty"` // for auth cache invalidation on quota exhaustion
	HasRateLimits bool   `json:"has_rate_limits"`

	PublishedAt int64 `json:"published_at"` // unix milliseconds
}

// NewBillingEvent constructs a BillingEvent from the billing params available at the gateway.
func NewBillingEvent(cmd *UsageBillingCommand, usageLog *UsageLog, groupID int64, apiKeyKey string, hasRateLimits bool) *BillingEvent {
	return &BillingEvent{
		Command:       cmd,
		UsageLog:      usageLog,
		GroupID:       groupID,
		APIKeyKey:     apiKeyKey,
		HasRateLimits: hasRateLimits,
		PublishedAt:   time.Now().UnixMilli(),
	}
}

// BillingEventPublisher publishes billing events to a durable stream.
type BillingEventPublisher interface {
	Publish(ctx context.Context, event *BillingEvent) error
}

// BillingEventConsumerHandler processes a single billing event.
type BillingEventConsumerHandler func(ctx context.Context, event *BillingEvent) error

// BillingEventConsumer reads billing events from a durable stream.
type BillingEventConsumer interface {
	// Start begins consuming events in background goroutines, calling handler for each event.
	Start(handler BillingEventConsumerHandler)
	// Stop gracefully shuts down the consumer, draining in-flight events.
	Stop()
}
