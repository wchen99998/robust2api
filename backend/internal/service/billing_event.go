package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const usageChargeEventVersion = 1

var ErrBillingEventPublisherUnavailable = errors.New("billing event publisher is unavailable")

// UsageChargeEvent is the versioned message published to the billing stream.
// It contains immutable usage facts and computed charge amounts for one request.
type UsageChargeEvent struct {
	Version int    `json:"version"`
	EventID string `json:"event_id"`

	RequestID  string    `json:"request_id"`
	OccurredAt time.Time `json:"occurred_at"`

	// Command holds the billing transaction data (balance/subscription/quota deductions).
	Command *UsageBillingCommand `json:"command"`

	// UsageLog holds the authoritative usage record to persist transactionally.
	UsageLog *UsageLog `json:"usage_log,omitempty"`

	// Projection hints used only after the billing transaction commits.
	GroupID                 int64 `json:"group_id,omitempty"`
	HasAPIKeyRateLimitUsage bool  `json:"has_api_key_rate_limit_usage"`
}

// BillingEvent is kept as an alias while the codebase migrates to UsageChargeEvent.
type BillingEvent = UsageChargeEvent

// NewUsageChargeEvent constructs a UsageChargeEvent from the billing params available at the gateway.
func NewUsageChargeEvent(cmd *UsageBillingCommand, usageLog *UsageLog, groupID int64, hasRateLimits bool) *UsageChargeEvent {
	requestID := ""
	occurredAt := time.Now().UTC()
	if cmd != nil {
		cmd.Normalize()
		requestID = cmd.RequestID
	}
	if usageLog != nil {
		if usageLog.RequestID != "" {
			requestID = usageLog.RequestID
		}
		if !usageLog.CreatedAt.IsZero() {
			occurredAt = usageLog.CreatedAt.UTC()
		}
	}

	return &UsageChargeEvent{
		Version:                 usageChargeEventVersion,
		EventID:                 uuid.NewString(),
		RequestID:               requestID,
		OccurredAt:              occurredAt,
		Command:                 cmd,
		UsageLog:                usageLog,
		GroupID:                 groupID,
		HasAPIKeyRateLimitUsage: hasRateLimits,
	}
}

// NewBillingEvent is kept as a compatibility wrapper for existing call sites and tests.
func NewBillingEvent(cmd *UsageBillingCommand, usageLog *UsageLog, groupID int64, hasRateLimits bool) *BillingEvent {
	return NewUsageChargeEvent(cmd, usageLog, groupID, hasRateLimits)
}

// BillingEventPublisher publishes billing events to a durable stream.
type BillingEventPublisher interface {
	Publish(ctx context.Context, event *UsageChargeEvent) error
}

// BillingEventConsumerHandler processes a single billing event.
type BillingEventConsumerHandler func(ctx context.Context, event *UsageChargeEvent) error

// BillingEventConsumer reads billing events from a durable stream.
type BillingEventConsumer interface {
	// Start begins consuming events in background goroutines, calling handler for each event.
	Start(handler BillingEventConsumerHandler)
	// Stop gracefully shuts down the consumer, draining in-flight events.
	Stop()
}
