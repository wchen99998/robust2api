package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const usageChargeEventVersion = 2

type UsageChargeEventKind string

const (
	UsageChargeEventKindCharge   UsageChargeEventKind = "charge"
	UsageChargeEventKindReserve  UsageChargeEventKind = "reserve"
	UsageChargeEventKindFinalize UsageChargeEventKind = "finalize"
	UsageChargeEventKindRelease  UsageChargeEventKind = "release"
)

func normalizeUsageChargeEventKind(kind UsageChargeEventKind) UsageChargeEventKind {
	switch kind {
	case UsageChargeEventKindReserve, UsageChargeEventKindFinalize, UsageChargeEventKindRelease:
		return kind
	default:
		return UsageChargeEventKindCharge
	}
}

var ErrBillingEventPublisherUnavailable = errors.New("billing event publisher is unavailable")

// UsageChargeEvent is the versioned message published to the billing stream.
// It contains immutable usage facts and computed charge amounts for one request.
type UsageChargeEvent struct {
	Version int                  `json:"version"`
	Kind    UsageChargeEventKind `json:"kind"`
	EventID string               `json:"event_id"`

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

// NewUsageChargeEvent constructs a UsageChargeEvent from the billing params available at the gateway.
func NewUsageChargeEvent(cmd *UsageBillingCommand, usageLog *UsageLog, groupID int64, hasRateLimits bool) *UsageChargeEvent {
	return NewUsageChargeEventWithKind(UsageChargeEventKindCharge, cmd, usageLog, groupID, hasRateLimits)
}

func NewUsageChargeEventWithKind(kind UsageChargeEventKind, cmd *UsageBillingCommand, usageLog *UsageLog, groupID int64, hasRateLimits bool) *UsageChargeEvent {
	requestID := ""
	occurredAt := time.Now().UTC()
	if cmd != nil {
		cmd.Normalize()
		requestID = cmd.RequestID
	}
	if usageLog != nil {
		// Prefer the caller-supplied command RequestID so streaming lifecycle
		// events (reserve/finalize/release) stay correlated across phases.
		// Fall back to usageLog.RequestID only when the command did not
		// supply one.
		if requestID == "" && usageLog.RequestID != "" {
			requestID = usageLog.RequestID
		}
		if !usageLog.CreatedAt.IsZero() {
			occurredAt = usageLog.CreatedAt.UTC()
		}
	}

	return &UsageChargeEvent{
		Version:                 usageChargeEventVersion,
		Kind:                    normalizeUsageChargeEventKind(kind),
		EventID:                 uuid.NewString(),
		RequestID:               requestID,
		OccurredAt:              occurredAt,
		Command:                 cmd,
		UsageLog:                usageLog,
		GroupID:                 groupID,
		HasAPIKeyRateLimitUsage: hasRateLimits,
	}
}

// BillingEventPublisher publishes billing events to a durable stream.
type BillingEventPublisher interface {
	Publish(ctx context.Context, event *UsageChargeEvent) error
}

// BillingEventConsumerHandler processes a single billing event.
type BillingEventConsumerHandler func(ctx context.Context, event *UsageChargeEvent) error

type BillingConsumerStatus struct {
	StreamKey        string
	Group            string
	DLQKey           string
	PendingCount     int64
	OldestPendingAge time.Duration
	DLQDepth         int64
}

// BillingEventConsumer reads billing events from a durable stream.
type BillingEventConsumer interface {
	// Start begins consuming events in background goroutines, calling handler for each event.
	Start(handler BillingEventConsumerHandler)
	// Stop gracefully shuts down the consumer, draining in-flight events.
	Stop()
	// Status returns a point-in-time operational snapshot of the consumer state.
	Status(ctx context.Context) (*BillingConsumerStatus, error)
}
