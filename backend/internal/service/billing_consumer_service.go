package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// BillingConsumerService orchestrates the billing event consumption loop.
// It reads events from a BillingEventConsumer, applies billing via UsageBillingRepository,
// writes usage logs, and updates the billing cache.
type BillingConsumerService struct {
	consumer         BillingEventConsumer
	billingRepo      UsageBillingRepository
	usageLogRepo     UsageLogRepository
	billingCache     *BillingCacheService
	deferredService  *DeferredService
	authInvalidator  APIKeyAuthCacheInvalidator
}

// NewBillingConsumerService creates a new billing consumer orchestrator.
func NewBillingConsumerService(
	consumer BillingEventConsumer,
	billingRepo UsageBillingRepository,
	usageLogRepo UsageLogRepository,
	billingCache *BillingCacheService,
	deferredService *DeferredService,
	authInvalidator APIKeyAuthCacheInvalidator,
) *BillingConsumerService {
	svc := &BillingConsumerService{
		consumer:        consumer,
		billingRepo:     billingRepo,
		usageLogRepo:    usageLogRepo,
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

	// 1. Apply billing transaction (idempotent via usage_billing_dedup table).
	result, err := s.billingRepo.Apply(ctx, cmd)
	if err != nil {
		logger.L().Error("billing consumer: apply failed",
			zap.String("component", "service.billing_consumer"),
			zap.String("request_id", cmd.RequestID),
			zap.Int64("user_id", cmd.UserID),
			zap.Error(err),
		)
		return err
	}

	// If this was a duplicate (already applied), just schedule the account update and return.
	if result == nil || !result.Applied {
		if s.deferredService != nil {
			s.deferredService.ScheduleLastUsedUpdate(cmd.AccountID)
		}
		return nil
	}

	// 2. Invalidate API key auth cache if quota was exhausted by this billing.
	if result.APIKeyQuotaExhausted && event.APIKeyKey != "" && s.authInvalidator != nil {
		s.authInvalidator.InvalidateAuthCacheByKey(ctx, event.APIKeyKey)
	}

	// 3. Write usage log.
	if event.UsageLog != nil && s.usageLogRepo != nil {
		s.writeUsageLog(ctx, event.UsageLog)
	}

	// 4. Update billing cache so eligibility checks reflect this charge.
	s.updateBillingCache(event, cmd)

	// 5. Schedule deferred account last-used update.
	if s.deferredService != nil {
		s.deferredService.ScheduleLastUsedUpdate(cmd.AccountID)
	}

	return nil
}

// writeUsageLog persists the usage log record with best-effort retry.
func (s *BillingConsumerService) writeUsageLog(ctx context.Context, usageLog *UsageLog) {
	writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if writer, ok := s.usageLogRepo.(usageLogBestEffortWriter); ok {
		if err := writer.CreateBestEffort(writeCtx, usageLog); err != nil {
			logger.L().Error("billing consumer: usage log best-effort write failed",
				zap.String("component", "service.billing_consumer"),
				zap.String("request_id", usageLog.RequestID),
				zap.Error(err),
			)
			if !IsUsageLogCreateDropped(err) {
				if _, syncErr := s.usageLogRepo.Create(writeCtx, usageLog); syncErr != nil {
					logger.L().Error("billing consumer: usage log sync fallback failed",
						zap.String("component", "service.billing_consumer"),
						zap.String("request_id", usageLog.RequestID),
						zap.Error(syncErr),
					)
				}
			}
		}
		return
	}

	if _, err := s.usageLogRepo.Create(writeCtx, usageLog); err != nil {
		logger.L().Error("billing consumer: usage log write failed",
			zap.String("component", "service.billing_consumer"),
			zap.String("request_id", usageLog.RequestID),
			zap.Error(err),
		)
	}
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
	if cmd.APIKeyRateLimitCost > 0 && event.HasRateLimits {
		s.billingCache.QueueUpdateAPIKeyRateLimitUsage(cmd.APIKeyID, cmd.APIKeyRateLimitCost)
	}
}
