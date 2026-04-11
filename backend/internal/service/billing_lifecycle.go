package service

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

type StreamingBillingLifecycleInput struct {
	RequestID          string
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription
	Model              string
	ServiceTier        *string
	ReasoningEffort    *string
	RequestPayloadHash string
	OccurredAt         time.Time
}

func resolveStreamingBillingType(apiKey *APIKey, subscription *UserSubscription) int8 {
	if subscription != nil && apiKey != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		return BillingTypeSubscription
	}
	return BillingTypeBalance
}

func trimmedStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func buildStreamingLifecycleEvent(ctx context.Context, kind UsageChargeEventKind, input *StreamingBillingLifecycleInput) *UsageChargeEvent {
	if input == nil || input.APIKey == nil || input.Account == nil {
		return nil
	}
	user := input.User
	if user == nil {
		user = input.APIKey.User
	}
	if user == nil {
		return nil
	}

	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		requestID = resolveUsageBillingRequestID(ctx, "")
	}
	cmd := &UsageBillingCommand{
		RequestID:          requestID,
		APIKeyID:           input.APIKey.ID,
		RequestPayloadHash: strings.TrimSpace(input.RequestPayloadHash),
		UserID:             user.ID,
		AccountID:          input.Account.ID,
		AccountType:        strings.TrimSpace(input.Account.Type),
		Model:              strings.TrimSpace(input.Model),
		ServiceTier:        trimmedStringValue(input.ServiceTier),
		ReasoningEffort:    trimmedStringValue(input.ReasoningEffort),
		BillingType:        resolveStreamingBillingType(input.APIKey, input.Subscription),
	}
	if input.Subscription != nil {
		cmd.SubscriptionID = &input.Subscription.ID
	}
	cmd.Normalize()

	var groupID int64
	if input.APIKey.GroupID != nil {
		groupID = *input.APIKey.GroupID
	}
	event := NewUsageChargeEventWithKind(kind, cmd, nil, groupID, input.APIKey.HasRateLimits())
	if !input.OccurredAt.IsZero() {
		event.OccurredAt = input.OccurredAt.UTC()
	}
	return event
}

func publishStreamingLifecycleEvent(ctx context.Context, publisher BillingEventPublisher, component string, event *UsageChargeEvent, userID int64) error {
	if event == nil {
		return nil
	}
	if publisher == nil {
		slog.Error("billing event publisher unavailable",
			"component", component,
			"request_id", event.RequestID,
			"user_id", userID,
			"kind", event.Kind,
		)
		return ErrBillingEventPublisherUnavailable
	}
	if err := publisher.Publish(ctx, event); err != nil {
		slog.Error("billing event publish failed",
			"component", component,
			"request_id", event.RequestID,
			"user_id", userID,
			"kind", event.Kind,
			"error", err,
		)
		return err
	}
	return nil
}

func (s *GatewayService) PublishStreamingReserve(ctx context.Context, input *StreamingBillingLifecycleInput) error {
	event := buildStreamingLifecycleEvent(ctx, UsageChargeEventKindReserve, input)
	if event == nil {
		return nil
	}
	return publishStreamingLifecycleEvent(ctx, s.billingPublisher, "service.gateway", event, event.Command.UserID)
}

func (s *GatewayService) PublishStreamingRelease(ctx context.Context, input *StreamingBillingLifecycleInput) error {
	event := buildStreamingLifecycleEvent(ctx, UsageChargeEventKindRelease, input)
	if event == nil {
		return nil
	}
	return publishStreamingLifecycleEvent(ctx, s.billingPublisher, "service.gateway", event, event.Command.UserID)
}

func (s *OpenAIGatewayService) PublishStreamingReserve(ctx context.Context, input *StreamingBillingLifecycleInput) error {
	event := buildStreamingLifecycleEvent(ctx, UsageChargeEventKindReserve, input)
	if event == nil {
		return nil
	}
	return publishStreamingLifecycleEvent(ctx, s.billingPublisher, "service.openai_gateway", event, event.Command.UserID)
}

func (s *OpenAIGatewayService) PublishStreamingRelease(ctx context.Context, input *StreamingBillingLifecycleInput) error {
	event := buildStreamingLifecycleEvent(ctx, UsageChargeEventKindRelease, input)
	if event == nil {
		return nil
	}
	return publishStreamingLifecycleEvent(ctx, s.billingPublisher, "service.openai_gateway", event, event.Command.UserID)
}
