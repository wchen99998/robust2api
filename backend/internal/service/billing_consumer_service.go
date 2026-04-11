package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	"go.uber.org/zap"
)

type BillingStatusSnapshot struct {
	StreamKey          string
	Group              string
	PendingCount       int64
	OldestPendingAge   time.Duration
	DLQDepth           int64
	LastApplySuccessAt *time.Time
	LastApplyFailureAt *time.Time
}

// BillingConsumerService orchestrates the billing event consumption loop.
// It reads events from a BillingEventConsumer, applies billing+usage persistence
// transactionally, and then updates cache/auth projections after commit.
type BillingConsumerService struct {
	consumer         BillingEventConsumer
	billingRepo      UsageBillingRepository
	billingCache     *BillingCacheService
	deferredService  *DeferredService
	authInvalidator  APIKeyAuthCacheInvalidator
	statusCancel     context.CancelFunc
	statusWG         sync.WaitGroup
	lastApplyOKUnix  atomic.Int64
	lastApplyErrUnix atomic.Int64
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
	svc.startStatusPolling()
	logger.L().Info("billing consumer service started",
		zap.String("component", "service.billing_consumer"),
	)
	return svc
}

// Stop gracefully shuts down the consumer.
func (s *BillingConsumerService) Stop() {
	if s.statusCancel != nil {
		s.statusCancel()
		s.statusWG.Wait()
	}
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
		s.recordApplyFailure(time.Now().UTC())
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

	s.recordApplySuccess(time.Now().UTC())
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

func (s *BillingConsumerService) recordApplySuccess(now time.Time) {
	unix := now.UTC().Unix()
	s.lastApplyOKUnix.Store(unix)
	appelotel.M().RecordBillingApply(context.Background(), "success")
	appelotel.M().SetBillingLastApplySuccessTimestamp(context.Background(), unix)
}

func (s *BillingConsumerService) recordApplyFailure(now time.Time) {
	unix := now.UTC().Unix()
	s.lastApplyErrUnix.Store(unix)
	appelotel.M().RecordBillingApply(context.Background(), "failure")
	appelotel.M().SetBillingLastApplyFailureTimestamp(context.Background(), unix)
}

func (s *BillingConsumerService) startStatusPolling() {
	if s.consumer == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.statusCancel = cancel
	s.statusWG.Add(1)
	go func() {
		defer s.statusWG.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		s.refreshStatusMetrics(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshStatusMetrics(ctx)
			}
		}
	}()
}

func (s *BillingConsumerService) refreshStatusMetrics(ctx context.Context) {
	if s.consumer == nil {
		return
	}
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := s.consumer.Status(statusCtx)
	if err != nil {
		if ctx.Err() == nil {
			logger.L().Warn("billing consumer: status refresh failed",
				zap.String("component", "service.billing_consumer"),
				zap.Error(err),
			)
		}
		return
	}
	if status == nil {
		return
	}
	appelotel.M().SetBillingPendingMessages(context.Background(), status.PendingCount)
	appelotel.M().SetBillingPendingOldestAge(context.Background(), status.OldestPendingAge.Seconds())
	appelotel.M().SetBillingDLQMessages(context.Background(), status.DLQDepth)
}

func (s *BillingConsumerService) StatusSnapshot(ctx context.Context) (*BillingStatusSnapshot, error) {
	var (
		status *BillingConsumerStatus
		err    error
	)
	if s.consumer != nil {
		status, err = s.consumer.Status(ctx)
		if err != nil {
			return nil, err
		}
	}

	snapshot := &BillingStatusSnapshot{}
	if status != nil {
		snapshot.StreamKey = status.StreamKey
		snapshot.Group = status.Group
		snapshot.PendingCount = status.PendingCount
		snapshot.OldestPendingAge = status.OldestPendingAge
		snapshot.DLQDepth = status.DLQDepth
	}
	if unix := s.lastApplyOKUnix.Load(); unix > 0 {
		ts := time.Unix(unix, 0).UTC()
		snapshot.LastApplySuccessAt = &ts
	}
	if unix := s.lastApplyErrUnix.Load(); unix > 0 {
		ts := time.Unix(unix, 0).UTC()
		snapshot.LastApplyFailureAt = &ts
	}
	return snapshot, nil
}
