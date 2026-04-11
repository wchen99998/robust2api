//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wchen99998/robust2api/internal/config"
	"github.com/wchen99998/robust2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// testBillingPublisher is a stub publisher for tests.
type testBillingPublisher struct {
	events []*BillingEvent
	err    error
}

func (p *testBillingPublisher) Publish(_ context.Context, event *BillingEvent) error {
	p.events = append(p.events, event)
	return p.err
}

func newGatewayRecordUsageServiceForTest(usageRepo UsageLogRepository) (*GatewayService, *testBillingPublisher) {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.1
	publisher := &testBillingPublisher{}
	svc := NewGatewayService(
		nil,
		nil,
		usageRepo,
		publisher,
		nil,
		nil,
		cfg,
		nil,
		nil,
		NewBillingService(cfg, nil),
		nil,
		&BillingCacheService{},
		nil,
		nil,
		&DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return svc, publisher
}

func newGatewayRecordUsageServiceWithPublisherForTest(usageRepo UsageLogRepository, publisher BillingEventPublisher) *GatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.1
	return NewGatewayService(
		nil,
		nil,
		usageRepo,
		publisher,
		nil,
		nil,
		cfg,
		nil,
		nil,
		NewBillingService(cfg, nil),
		nil,
		&BillingCacheService{},
		nil,
		nil,
		&DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

type openAIRecordUsageBestEffortLogRepoStub struct {
	UsageLogRepository

	bestEffortErr   error
	createErr       error
	bestEffortCalls int
	createCalls     int
	lastLog         *UsageLog
	lastCtxErr      error
}

func (s *openAIRecordUsageBestEffortLogRepoStub) CreateBestEffort(ctx context.Context, log *UsageLog) error {
	s.bestEffortCalls++
	s.lastLog = log
	s.lastCtxErr = ctx.Err()
	return s.bestEffortErr
}

func (s *openAIRecordUsageBestEffortLogRepoStub) Create(ctx context.Context, log *UsageLog) (bool, error) {
	s.createCalls++
	s.lastLog = log
	s.lastCtxErr = ctx.Err()
	return false, s.createErr
}

func TestGatewayServiceRecordUsage_PublishesBillingEvent(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testBillingPublisher{}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, publisher)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_publish_test",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    501,
			Quota: 100,
		},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].Command)
	require.Equal(t, int64(601), publisher.events[0].Command.UserID)
}

func TestGatewayServiceRecordUsage_ReturnsErrorWhenBillingPublisherMissing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, nil)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_publish_missing",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    501,
			Quota: 100,
		},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
	})

	require.ErrorIs(t, err, ErrBillingEventPublisherUnavailable)
}

func TestGatewayServiceRecordUsage_BillingFingerprintIncludesRequestPayloadHash(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testBillingPublisher{}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, publisher)

	payloadHash := HashUsageRequestPayload([]byte(`{"messages":[{"role":"user","content":"hello"}]}`))
	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_payload_hash",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:             &APIKey{ID: 501, Quota: 100},
		User:               &User{ID: 601},
		Account:            &Account{ID: 701},
		RequestPayloadHash: payloadHash,
	})
	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, payloadHash, publisher.events[0].Command.RequestPayloadHash)
}

func TestGatewayServiceRecordUsage_BillingFingerprintFallsBackToContextRequestID(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testBillingPublisher{}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, publisher)

	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "req-local-123")
	err := svc.RecordUsage(ctx, &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_payload_fallback",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 501, Quota: 100},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
	})
	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, "local:req-local-123", publisher.events[0].Command.RequestPayloadHash)
}

func TestGatewayServiceRecordUsage_PreservesRequestedAndUpstreamModels(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	publisher := &testBillingPublisher{}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, publisher)
	mappedModel := "claude-sonnet-4-20250514"

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:     "gateway_models_split",
			Usage:         ClaudeUsage{InputTokens: 10, OutputTokens: 6},
			Model:         "claude-sonnet-4",
			UpstreamModel: mappedModel,
			Duration:      time.Second,
		},
		APIKey:  &APIKey{ID: 501, Quota: 100},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	usageLog := publisher.events[0].UsageLog
	require.NotNil(t, usageLog)
	require.Equal(t, "claude-sonnet-4", usageLog.Model)
	require.Equal(t, "claude-sonnet-4", usageLog.RequestedModel)
	require.NotNil(t, usageLog.UpstreamModel)
	require.Equal(t, mappedModel, *usageLog.UpstreamModel)
}

func TestGatewayServiceRecordUsage_UsageLogWriteErrorDoesNotSkipBilling(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: MarkUsageLogCreateNotPersisted(context.Canceled)}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc, publisher := newGatewayRecordUsageServiceForTest(usageRepo)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_not_persisted",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    503,
			Quota: 100,
		},
		User:          &User{ID: 603},
		Account:       &Account{ID: 703},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
}

func TestGatewayServiceRecordUsageWithLongContext_BillingUsesDetachedContext(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: context.DeadlineExceeded}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc, publisher := newGatewayRecordUsageServiceForTest(usageRepo)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsageWithLongContext(reqCtx, &RecordUsageLongContextInput{
		Result: &ForwardResult{
			RequestID: "gateway_long_context_detached_ctx",
			Usage: ClaudeUsage{
				InputTokens:  12,
				OutputTokens: 8,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    502,
			Quota: 100,
		},
		User:                  &User{ID: 602},
		Account:               &Account{ID: 702},
		LongContextThreshold:  200000,
		LongContextMultiplier: 2,
		APIKeyService:         quotaSvc,
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
}

func TestGatewayServiceRecordUsage_ZeroCostStillPublishesBillingEvent(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	svc, publisher := newGatewayRecordUsageServiceForTest(usageRepo)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_zero_cost_event",
			Usage: ClaudeUsage{
				InputTokens:  0,
				OutputTokens: 0,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 520},
		User:    &User{ID: 620},
		Account: &Account{ID: 720},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].Command)
	require.Equal(t, 0, usageRepo.calls)
}

func TestGatewayServiceRecordUsage_UsesFallbackRequestIDForUsageLog(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	svc, publisher := newGatewayRecordUsageServiceForTest(usageRepo)

	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "gateway-local-fallback")
	err := svc.RecordUsage(ctx, &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 504},
		User:    &User{ID: 604},
		Account: &Account{ID: 704},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, "local:gateway-local-fallback", publisher.events[0].UsageLog.RequestID)
}

func TestGatewayServiceRecordUsage_PrefersClientRequestIDOverUpstreamRequestID(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testBillingPublisher{}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, publisher)

	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-stable-123")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "req-local-ignored")
	err := svc.RecordUsage(ctx, &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "upstream-volatile-456",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 506},
		User:    &User{ID: 606},
		Account: &Account{ID: 706},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, "client:client-stable-123", publisher.events[0].Command.RequestID)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, "client:client-stable-123", publisher.events[0].UsageLog.RequestID)
}

func TestGatewayServiceRecordUsage_GeneratesRequestIDWhenAllSourcesMissing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testBillingPublisher{}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, publisher)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 507},
		User:    &User{ID: 607},
		Account: &Account{ID: 707},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.True(t, strings.HasPrefix(publisher.events[0].Command.RequestID, "generated:"))
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, publisher.events[0].Command.RequestID, publisher.events[0].UsageLog.RequestID)
}

func TestGatewayServiceRecordUsage_DroppedUsageLogDoesNotSyncFallback(t *testing.T) {
	usageRepo := &openAIRecordUsageBestEffortLogRepoStub{
		bestEffortErr: MarkUsageLogCreateDropped(errors.New("usage log best-effort queue full")),
	}
	publisher := &testBillingPublisher{}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, publisher)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_drop_usage_log",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 508},
		User:    &User{ID: 608},
		Account: &Account{ID: 708},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, 0, usageRepo.bestEffortCalls)
	require.Equal(t, 0, usageRepo.createCalls)
}

func TestGatewayServiceRecordUsage_BillingErrorSkipsUsageLogWrite(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testBillingPublisher{err: context.DeadlineExceeded}
	svc := newGatewayRecordUsageServiceWithPublisherForTest(usageRepo, publisher)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_billing_fail",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 505},
		User:    &User{ID: 605},
		Account: &Account{ID: 705},
	})

	require.Error(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, 0, usageRepo.calls)
}

func TestGatewayServiceRecordUsage_ReasoningEffortPersisted(t *testing.T) {
	usageRepo := &openAIRecordUsageBestEffortLogRepoStub{}
	svc, publisher := newGatewayRecordUsageServiceForTest(usageRepo)

	effort := "max"
	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "effort_test",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
			Model:           "claude-opus-4-6",
			Duration:        time.Second,
			ReasoningEffort: &effort,
		},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 1},
		Account: &Account{ID: 1},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.NotNil(t, publisher.events[0].UsageLog.ReasoningEffort)
	require.Equal(t, "max", *publisher.events[0].UsageLog.ReasoningEffort)
}

func TestGatewayServiceRecordUsage_ReasoningEffortNil(t *testing.T) {
	usageRepo := &openAIRecordUsageBestEffortLogRepoStub{}
	svc, publisher := newGatewayRecordUsageServiceForTest(usageRepo)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "no_effort_test",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 1},
		Account: &Account{ID: 1},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Nil(t, publisher.events[0].UsageLog.ReasoningEffort)
}
