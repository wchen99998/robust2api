package service

import (
	"context"

	"github.com/wchen99998/robust2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// BillingConsumerService orchestrates the billing event consumption loop.
// It reads events from a BillingEventConsumer, applies billing+usage persistence
// transactionally, and then updates cache/auth projections after commit.
type BillingConsumerService struct {
	consumer        BillingEventConsumer
	billingRepo     UsageBillingRepository
	billingCache    *BillingCacheService
	deferredService *DeferredService
	authInvalidator APIKeyAuthCacheInvalidator
}

// NewBillingConsumerService creates a new billing consumer orchestrator.
func NewBillingConsumerService(
	consumer BillingEventConsumer,
	billingRepo UsageBillingRepository,
	billingCache *BillingCacheService,
	deferredService *DeferredService,
	authInvalidator APIKeyAuthCacheInvalidator,
) *BillingConsumerService {
	svc := &BillingConsumerService{
		consumer:        consumer,
		billingRepo:     billingRepo,
		billingCache:    billingCache,
		deferredService: deferredService,
		authInvalidator: authInvalidator,
	}
	svc.consumer.Start(svc.handleEvent)
	logger.L().Info("billing consumer service started",
		zap.String("component", "service.billing_consumer"),
	)
	return svc
}

// Stop gracefully shuts down the consumer.
func (s *BillingConsumerService) Stop() {
	if s.consumer != nil {
		s.consumer.Stop()
	}
}

// handleEvent processes a single billing event from the stream.
func (s *BillingConsumerService) handleEvent(ctx context.Context, event *BillingEvent) error {
	if event == nil || event.Command == nil {
		return nil
	}

	cmd := event.Command
	cmd.Normalize()

	// Apply billing mutations and usage log persistence in a single transaction.
	result, err := s.billingRepo.ApplyUsageCharge(ctx, event)
	if err != nil {
		logger.L().Error("billing consumer: apply failed",
			zap.String("component", "service.billing_consumer"),
			zap.String("request_id", cmd.RequestID),
			zap.Int64("user_id", cmd.UserID),
			zap.Error(err),
		)
		return err
	}

	if result == nil {
		result = &UsageBillingApplyResult{}
	}

	// Repair auth projection after commit. This is safe to repeat on replay.
	if result.NeedsAPIKeyAuthCacheInvalidation && result.APIKeyAuthCacheKey != "" && s.authInvalidator != nil {
		s.authInvalidator.InvalidateAuthCacheByKey(ctx, result.APIKeyAuthCacheKey)
	}

	// Cache updates must not reapply additive deltas on duplicate delivery.
	if result.Applied {
		s.updateBillingCache(event, cmd)
	} else {
		s.repairBillingCache(ctx, event, cmd)
	}

	// Deferred account touch is idempotent enough to repeat after replay.
	if s.deferredService != nil {
		s.deferredService.ScheduleLastUsedUpdate(cmd.AccountID)
	}

	return nil
}

// updateBillingCache queues cache updates so that eligibility checks reflect this charge.
func (s *BillingConsumerService) updateBillingCache(event *BillingEvent, cmd *UsageBillingCommand) {
	if s.billingCache == nil {
		return
	}

	// Subscription usage or balance deduction.
	if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil && event.GroupID > 0 {
		s.billingCache.QueueUpdateSubscriptionUsage(cmd.UserID, event.GroupID, cmd.SubscriptionCost)
	} else if cmd.BalanceCost > 0 {
		s.billingCache.QueueDeductBalance(cmd.UserID, cmd.BalanceCost)
	}

	// API key rate limit usage.
	if cmd.APIKeyRateLimitCost > 0 && event.HasAPIKeyRateLimitUsage {
		s.billingCache.QueueUpdateAPIKeyRateLimitUsage(cmd.APIKeyID, cmd.APIKeyRateLimitCost)
	}
}

func (s *BillingConsumerService) repairBillingCache(ctx context.Context, event *BillingEvent, cmd *UsageBillingCommand) {
	if s.billingCache == nil {
		return
	}

	if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil && event.GroupID > 0 {
		if err := s.billingCache.InvalidateSubscription(ctx, cmd.UserID, event.GroupID); err != nil {
			logger.L().Warn("billing consumer: subscription cache repair failed",
				zap.String("component", "service.billing_consumer"),
				zap.String("request_id", cmd.RequestID),
				zap.Int64("user_id", cmd.UserID),
				zap.Int64("group_id", event.GroupID),
				zap.Error(err),
			)
		}
	} else if cmd.BalanceCost > 0 {
		if err := s.billingCache.InvalidateUserBalance(ctx, cmd.UserID); err != nil {
			logger.L().Warn("billing consumer: balance cache repair failed",
				zap.String("component", "service.billing_consumer"),
				zap.String("request_id", cmd.RequestID),
				zap.Int64("user_id", cmd.UserID),
				zap.Error(err),
			)
		}
	}

	if cmd.APIKeyRateLimitCost > 0 && event.HasAPIKeyRateLimitUsage {
		if err := s.billingCache.InvalidateAPIKeyRateLimit(ctx, cmd.APIKeyID); err != nil {
			logger.L().Warn("billing consumer: api key rate-limit cache repair failed",
				zap.String("component", "service.billing_consumer"),
				zap.String("request_id", cmd.RequestID),
				zap.Int64("api_key_id", cmd.APIKeyID),
				zap.Error(err),
			)
		}
	}
}
