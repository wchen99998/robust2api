package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type openAIRecordUsageLogRepoStub struct {
	UsageLogRepository

	inserted   bool
	err        error
	calls      int
	lastLog    *UsageLog
	lastCtxErr error
}

func (s *openAIRecordUsageLogRepoStub) Create(ctx context.Context, log *UsageLog) (bool, error) {
	s.calls++
	s.lastLog = log
	s.lastCtxErr = ctx.Err()
	return s.inserted, s.err
}

type testOpenAIBillingPublisher struct {
	events []*UsageChargeEvent
	err    error
}

func (p *testOpenAIBillingPublisher) Publish(_ context.Context, event *UsageChargeEvent) error {
	p.events = append(p.events, event)
	return p.err
}

type openAIRecordUsageUserRepoStub struct {
	UserRepository

	deductCalls int
	deductErr   error
	lastAmount  float64
	lastCtxErr  error
}

func (s *openAIRecordUsageUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	s.deductCalls++
	s.lastAmount = amount
	s.lastCtxErr = ctx.Err()
	return s.deductErr
}

type openAIRecordUsageSubRepoStub struct {
	UserSubscriptionRepository

	incrementCalls int
	incrementErr   error
	lastCtxErr     error
}

func (s *openAIRecordUsageSubRepoStub) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	s.incrementCalls++
	s.lastCtxErr = ctx.Err()
	return s.incrementErr
}

type openAIRecordUsageAPIKeyQuotaStub struct {
	quotaCalls          int
	rateLimitCalls      int
	err                 error
	lastAmount          float64
	lastQuotaCtxErr     error
	lastRateLimitCtxErr error
}

func (s *openAIRecordUsageAPIKeyQuotaStub) UpdateQuotaUsed(ctx context.Context, apiKeyID int64, cost float64) error {
	s.quotaCalls++
	s.lastAmount = cost
	s.lastQuotaCtxErr = ctx.Err()
	return s.err
}

func (s *openAIRecordUsageAPIKeyQuotaStub) UpdateRateLimitUsage(ctx context.Context, apiKeyID int64, cost float64) error {
	s.rateLimitCalls++
	s.lastAmount = cost
	s.lastRateLimitCtxErr = ctx.Err()
	return s.err
}

type openAIUserGroupRateRepoStub struct {
	UserGroupRateRepository

	rate  *float64
	err   error
	calls int
}

func (s *openAIUserGroupRateRepoStub) GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.rate, nil
}

func i64p(v int64) *int64 {
	return &v
}

func newOpenAIRecordUsageServiceForTest(usageRepo UsageLogRepository, rateRepo UserGroupRateRepository) (*OpenAIGatewayService, *testOpenAIBillingPublisher) {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.1
	publisher := &testOpenAIBillingPublisher{}
	_ = usageRepo
	svc := NewOpenAIGatewayService(
		nil,
		publisher,
		rateRepo,
		nil,
		cfg,
		nil,
		nil,
		NewBillingService(cfg, nil),
		nil,
		&BillingCacheService{},
		nil,
		&DeferredService{},
		nil,
		nil,
		nil,
	)
	svc.userGroupRateResolver = newUserGroupRateResolver(
		rateRepo,
		nil,
		resolveUserGroupRateCacheTTL(cfg),
		nil,
		"service.openai_gateway.test",
	)
	return svc, publisher
}

func newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo UsageLogRepository, publisher BillingEventPublisher, rateRepo UserGroupRateRepository) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.1
	_ = usageRepo
	svc := NewOpenAIGatewayService(
		nil,
		publisher,
		rateRepo,
		nil,
		cfg,
		nil,
		nil,
		NewBillingService(cfg, nil),
		nil,
		&BillingCacheService{},
		nil,
		&DeferredService{},
		nil,
		nil,
		nil,
	)
	svc.userGroupRateResolver = newUserGroupRateResolver(
		rateRepo,
		nil,
		resolveUserGroupRateCacheTTL(cfg),
		nil,
		"service.openai_gateway.test",
	)
	return svc
}

func expectedOpenAICost(t *testing.T, svc *OpenAIGatewayService, model string, usage OpenAIUsage, multiplier float64) *CostBreakdown {
	t.Helper()

	cost, err := svc.billingService.CalculateCost(model, UsageTokens{
		InputTokens:         max(usage.InputTokens-usage.CacheReadInputTokens, 0),
		OutputTokens:        usage.OutputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
	}, multiplier)
	require.NoError(t, err)
	return cost
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestOpenAIGatewayServiceRecordUsage_UsesUserSpecificGroupRate(t *testing.T) {
	groupID := int64(11)
	groupRate := 1.4
	userRate := 1.8
	usage := OpenAIUsage{InputTokens: 15, OutputTokens: 4, CacheReadInputTokens: 3}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	rateRepo := &openAIUserGroupRateRepoStub{rate: &userRate}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_user_group_rate",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1001,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
			},
		},
		User:    &User{ID: 2001},
		Account: &Account{ID: 3001},
	})

	require.NoError(t, err)
	require.Equal(t, 1, rateRepo.calls)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, userRate, publisher.events[0].UsageLog.RateMultiplier)
	require.Equal(t, 12, publisher.events[0].UsageLog.InputTokens)
	require.Equal(t, 3, publisher.events[0].UsageLog.CacheReadTokens)

	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, userRate)
	require.InDelta(t, expected.ActualCost, publisher.events[0].UsageLog.ActualCost, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_IncludesEndpointMetadata(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	rateRepo := &openAIUserGroupRateRepoStub{}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_endpoint_metadata",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 2,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    1002,
			Group: &Group{RateMultiplier: 1},
		},
		User:             &User{ID: 2002},
		Account:          &Account{ID: 3002},
		InboundEndpoint:  " /v1/chat/completions ",
		UpstreamEndpoint: " /v1/responses ",
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.NotNil(t, publisher.events[0].UsageLog.InboundEndpoint)
	require.Equal(t, "/v1/chat/completions", *publisher.events[0].UsageLog.InboundEndpoint)
	require.NotNil(t, publisher.events[0].UsageLog.UpstreamEndpoint)
	require.Equal(t, "/v1/responses", *publisher.events[0].UsageLog.UpstreamEndpoint)
}

func TestOpenAIGatewayServiceRecordUsage_FallsBackToGroupDefaultRateOnResolverError(t *testing.T) {
	groupID := int64(12)
	groupRate := 1.6
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 5, CacheReadInputTokens: 2}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	rateRepo := &openAIUserGroupRateRepoStub{err: errors.New("db unavailable")}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_group_default_on_error",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1002,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
			},
		},
		User:    &User{ID: 2002},
		Account: &Account{ID: 3002},
	})

	require.NoError(t, err)
	require.Equal(t, 1, rateRepo.calls)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, groupRate, publisher.events[0].UsageLog.RateMultiplier)

	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, groupRate)
	require.InDelta(t, expected.ActualCost, publisher.events[0].UsageLog.ActualCost, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_FallsBackToGroupDefaultRateWhenResolverMissing(t *testing.T) {
	groupID := int64(13)
	groupRate := 1.25
	usage := OpenAIUsage{InputTokens: 9, OutputTokens: 4, CacheReadInputTokens: 1}

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	svc.userGroupRateResolver = nil

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_group_default_nil_resolver",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1003,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:             groupID,
				RateMultiplier: groupRate,
			},
		},
		User:    &User{ID: 2003},
		Account: &Account{ID: 3003},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, groupRate, publisher.events[0].UsageLog.RateMultiplier)
}

func TestOpenAIGatewayServiceRecordUsage_PublishesBillingEvent(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false}
	publisher := &testOpenAIBillingPublisher{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_publish",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1004},
		User:    &User{ID: 2004},
		Account: &Account{ID: 3004},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].Command)
	require.Equal(t, int64(2004), publisher.events[0].Command.UserID)
}

func TestOpenAIGatewayServiceRecordUsage_PublishesBillingEventWithQuota(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false}
	publisher := &testOpenAIBillingPublisher{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_duplicate_billing_key",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    10045,
			Quota: 100,
		},
		User:          &User{ID: 20045},
		Account:       &Account{ID: 30045},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].Command)
	require.Equal(t, 0, quotaSvc.quotaCalls)
}

func TestOpenAIGatewayServiceRecordUsage_BillsWhenUsageLogCreateReturnsError(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 8, OutputTokens: 4}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: errors.New("usage log batch state uncertain")}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_usage_log_error",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 10041},
		User:    &User{ID: 20041},
		Account: &Account{ID: 30041},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
}

func TestOpenAIGatewayServiceRecordUsage_UsageLogWriteErrorDoesNotSkipBilling(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: MarkUsageLogCreateNotPersisted(context.Canceled)}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_not_persisted",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    10043,
			Quota: 100,
		},
		User:          &User{ID: 20043},
		Account:       &Account{ID: 30043},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
}

func TestOpenAIGatewayServiceRecordUsage_BillingUsesDetachedContext(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 6, CacheReadInputTokens: 2}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: context.DeadlineExceeded}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsage(reqCtx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_detached_billing_ctx",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:    10042,
			Quota: 100,
		},
		User:          &User{ID: 20042},
		Account:       &Account{ID: 30042},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NoError(t, quotaSvc.lastQuotaCtxErr)
}

func TestOpenAIGatewayServiceRecordUsage_ZeroCostStillPublishesBillingEvent(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testOpenAIBillingPublisher{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_zero_cost_event",
			Usage: OpenAIUsage{
				InputTokens:  0,
				OutputTokens: 0,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 100421},
		User:    &User{ID: 200421},
		Account: &Account{ID: 300421},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].Command)
	require.Equal(t, 0, usageRepo.calls)
}

func TestOpenAIGatewayServiceRecordUsage_PublisherUsesDetachedContext(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testOpenAIBillingPublisher{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsage(reqCtx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_detached_billing_repo_ctx",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10046},
		User:    &User{ID: 20046},
		Account: &Account{ID: 30046},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
}

func TestOpenAIGatewayServiceRecordUsage_BillingFingerprintIncludesRequestPayloadHash(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testOpenAIBillingPublisher{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	payloadHash := HashUsageRequestPayload([]byte(`{"model":"gpt-5","input":"hello"}`))
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "openai_payload_hash",
			Usage: OpenAIUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "gpt-5",
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

func TestOpenAIGatewayServiceRecordUsage_ReturnsErrorWhenBillingPublisherMissing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, nil, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "openai_publish_missing",
			Usage: OpenAIUsage{
				InputTokens:  12,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    1001,
			Quota: 100,
		},
		User:    &User{ID: 2001},
		Account: &Account{ID: 3001},
	})

	require.ErrorIs(t, err, ErrBillingEventPublisherUnavailable)
}

func TestOpenAIGatewayServiceRecordUsage_UsesFallbackRequestIDForBillingAndUsageLog(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testOpenAIBillingPublisher{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "req-local-fallback")
	err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10047},
		User:    &User{ID: 20047},
		Account: &Account{ID: 30047},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, "local:req-local-fallback", publisher.events[0].Command.RequestID)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, "local:req-local-fallback", publisher.events[0].UsageLog.RequestID)
}

func TestOpenAIGatewayServiceRecordUsage_PrefersClientRequestIDOverUpstreamRequestID(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testOpenAIBillingPublisher{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-stable-123")
	err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "upstream-openai-volatile-456",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10049},
		User:    &User{ID: 20049},
		Account: &Account{ID: 30049},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, "client:openai-client-stable-123", publisher.events[0].Command.RequestID)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, "client:openai-client-stable-123", publisher.events[0].UsageLog.RequestID)
}

func TestOpenAIGatewayServiceRecordUsage_GeneratesRequestIDWhenAllSourcesMissing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testOpenAIBillingPublisher{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10050},
		User:    &User{ID: 20050},
		Account: &Account{ID: 30050},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.True(t, strings.HasPrefix(publisher.events[0].Command.RequestID, "generated:"))
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, publisher.events[0].Command.RequestID, publisher.events[0].UsageLog.RequestID)
}

func TestOpenAIGatewayServiceRecordUsage_BillingErrorSkipsUsageLogWrite(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	publisher := &testOpenAIBillingPublisher{err: errors.New("billing publish failed")}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_billing_fail",
			Usage: OpenAIUsage{
				InputTokens:  8,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 10048},
		User:    &User{ID: 20048},
		Account: &Account{ID: 30048},
	})

	require.Error(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, 0, usageRepo.calls)
}

func TestOpenAIGatewayServiceRecordUsage_UpdatesAPIKeyQuotaWhenConfigured(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 10, OutputTokens: 6, CacheReadInputTokens: 2}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	publisher := &testOpenAIBillingPublisher{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newOpenAIRecordUsageServiceWithPublisherForTest(usageRepo, publisher, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_quota_update",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:    1005,
			Quota: 100,
		},
		User:          &User{ID: 2005},
		Account:       &Account{ID: 3005},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, 1.1)
	require.InDelta(t, expected.ActualCost, publisher.events[0].Command.APIKeyQuotaCost, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_ClampsActualInputTokensToZero(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_clamp_actual_input",
			Usage: OpenAIUsage{
				InputTokens:          2,
				OutputTokens:         1,
				CacheReadInputTokens: 5,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1006},
		User:    &User{ID: 2006},
		Account: &Account{ID: 3006},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, 0, publisher.events[0].UsageLog.InputTokens)
}

func TestOpenAIGatewayServiceRecordUsage_Gpt54LongContextBillsWholeSession(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_gpt54_long_context",
			Usage: OpenAIUsage{
				InputTokens:  300000,
				OutputTokens: 2000,
			},
			Model:    "gpt-5.4-2026-03-05",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1014},
		User:    &User{ID: 2014},
		Account: &Account{ID: 3014},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)

	expectedInput := 300000 * 2.5e-6 * 2.0
	expectedOutput := 2000 * 15e-6 * 1.5
	require.InDelta(t, expectedInput, publisher.events[0].UsageLog.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, publisher.events[0].UsageLog.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, publisher.events[0].UsageLog.TotalCost, 1e-10)
	require.InDelta(t, (expectedInput+expectedOutput)*1.1, publisher.events[0].UsageLog.ActualCost, 1e-10)
}

func TestOpenAIGatewayServiceRecordUsage_ServiceTierPriorityUsesFastPricing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	serviceTier := "priority"
	usage := OpenAIUsage{InputTokens: 100, OutputTokens: 50}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_service_tier_priority",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
		},
		APIKey:  &APIKey{ID: 1015},
		User:    &User{ID: 2015},
		Account: &Account{ID: 3015},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.NotNil(t, publisher.events[0].UsageLog.ServiceTier)
	require.Equal(t, serviceTier, *publisher.events[0].UsageLog.ServiceTier)

	baseCost, calcErr := svc.billingService.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 100, OutputTokens: 50}, 1.0)
	require.NoError(t, calcErr)
	require.InDelta(t, baseCost.TotalCost*2, publisher.events[0].UsageLog.TotalCost, 1e-10)
}

func TestOpenAIGatewayServiceRecordUsage_ServiceTierFlexHalvesCost(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	serviceTier := "flex"
	usage := OpenAIUsage{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 20}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:   "resp_service_tier_flex",
			ServiceTier: &serviceTier,
			Usage:       usage,
			Model:       "gpt-5.4",
			Duration:    time.Second,
		},
		APIKey:  &APIKey{ID: 1016},
		User:    &User{ID: 2016},
		Account: &Account{ID: 3016},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)

	baseCost, calcErr := svc.billingService.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 80, OutputTokens: 50, CacheReadTokens: 20}, 1.0)
	require.NoError(t, calcErr)
	require.InDelta(t, baseCost.TotalCost*0.5, publisher.events[0].UsageLog.TotalCost, 1e-10)
}

func TestNormalizeOpenAIServiceTier(t *testing.T) {
	t.Run("fast maps to priority", func(t *testing.T) {
		got := normalizeOpenAIServiceTier(" fast ")
		require.NotNil(t, got)
		require.Equal(t, "priority", *got)
	})

	t.Run("default ignored", func(t *testing.T) {
		require.Nil(t, normalizeOpenAIServiceTier("default"))
	})

	t.Run("invalid ignored", func(t *testing.T) {
		require.Nil(t, normalizeOpenAIServiceTier("turbo"))
	})
}

func TestExtractOpenAIServiceTier(t *testing.T) {
	require.Equal(t, "priority", *extractOpenAIServiceTier(map[string]any{"service_tier": "fast"}))
	require.Equal(t, "flex", *extractOpenAIServiceTier(map[string]any{"service_tier": "flex"}))
	require.Nil(t, extractOpenAIServiceTier(map[string]any{"service_tier": 1}))
	require.Nil(t, extractOpenAIServiceTier(nil))
}

func TestExtractOpenAIServiceTierFromBody(t *testing.T) {
	require.Equal(t, "priority", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"fast"}`)))
	require.Equal(t, "flex", *extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"flex"}`)))
	require.Nil(t, extractOpenAIServiceTierFromBody([]byte(`{"service_tier":"default"}`)))
	require.Nil(t, extractOpenAIServiceTierFromBody(nil))
}

func TestOpenAIGatewayServiceRecordUsage_UsesRequestedModelAndUpstreamModelMetadataFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	serviceTier := "priority"
	reasoning := "high"

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:       "resp_billing_model_override",
			BillingModel:    "gpt-5.1-codex",
			Model:           "gpt-5.1",
			UpstreamModel:   "gpt-5.1-codex",
			ServiceTier:     &serviceTier,
			ReasoningEffort: &reasoning,
			Usage: OpenAIUsage{
				InputTokens:  20,
				OutputTokens: 10,
			},
			Duration:     2 * time.Second,
			FirstTokenMs: func() *int { v := 120; return &v }(),
		},
		APIKey:    &APIKey{ID: 10, GroupID: i64p(11), Group: &Group{ID: 11, RateMultiplier: 1.2}},
		User:      &User{ID: 20},
		Account:   &Account{ID: 30},
		UserAgent: "codex-cli/1.0",
		IPAddress: "127.0.0.1",
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, "gpt-5.1", publisher.events[0].UsageLog.Model)
	require.Equal(t, "gpt-5.1", publisher.events[0].UsageLog.RequestedModel)
	require.NotNil(t, publisher.events[0].UsageLog.UpstreamModel)
	require.Equal(t, "gpt-5.1-codex", *publisher.events[0].UsageLog.UpstreamModel)
	require.NotNil(t, publisher.events[0].UsageLog.ServiceTier)
	require.Equal(t, serviceTier, *publisher.events[0].UsageLog.ServiceTier)
	require.NotNil(t, publisher.events[0].UsageLog.ReasoningEffort)
	require.Equal(t, reasoning, *publisher.events[0].UsageLog.ReasoningEffort)
	require.NotNil(t, publisher.events[0].UsageLog.UserAgent)
	require.Equal(t, "codex-cli/1.0", *publisher.events[0].UsageLog.UserAgent)
	require.NotNil(t, publisher.events[0].UsageLog.IPAddress)
	require.Equal(t, "127.0.0.1", *publisher.events[0].UsageLog.IPAddress)
	require.NotNil(t, publisher.events[0].UsageLog.GroupID)
	require.Equal(t, int64(11), *publisher.events[0].UsageLog.GroupID)
}

func TestOpenAIGatewayServiceRecordUsage_BillsMappedRequestsUsingRequestedModel(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10}

	// Billing should use the requested model ("gpt-5.1"), not the upstream mapped model ("gpt-5.1-codex").
	// This ensures pricing is always based on the model the user requested.
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
	}, 1.1)
	require.NoError(t, err)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_upstream_model_billing_fallback",
			Model:         "gpt-5.1",
			UpstreamModel: "gpt-5.1-codex",
			Usage:         usage,
			Duration:      time.Second,
		},
		APIKey:  &APIKey{ID: 10},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, "gpt-5.1", publisher.events[0].UsageLog.Model)
	require.Equal(t, expectedCost.ActualCost, publisher.events[0].UsageLog.ActualCost)
	require.Equal(t, expectedCost.TotalCost, publisher.events[0].UsageLog.TotalCost)
}

func TestOpenAIGatewayServiceRecordUsage_ChannelMappedDoesNotOverrideBillingModelWhenUnmapped(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10}

	// When channel did NOT map the model (ChannelMappedModel == OriginalModel),
	// billing should use result.BillingModel (the actual model used after group
	// DefaultMappedModel resolution), not the unmapped original model.
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
	}, 1.1)
	require.NoError(t, err)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_channel_unmapped_billing",
			Model:         "glm",
			BillingModel:  "gpt-5.1",
			UpstreamModel: "gpt-5.1",
			Usage:         usage,
			Duration:      time.Second,
		},
		APIKey:  &APIKey{ID: 10},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          1,
			OriginalModel:      "glm",
			ChannelMappedModel: "glm", // channel did NOT map
			BillingModelSource: BillingModelSourceChannelMapped,
		},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, expectedCost.ActualCost, publisher.events[0].UsageLog.ActualCost)
	require.True(t, publisher.events[0].UsageLog.ActualCost > 0, "cost must not be zero")
}

func TestOpenAIGatewayServiceRecordUsage_ChannelMappedOverridesBillingModelWhenMapped(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	usage := OpenAIUsage{InputTokens: 20, OutputTokens: 10}

	// When channel DID map the model (ChannelMappedModel != OriginalModel),
	// billing should use the channel-mapped model, honoring admin intent.
	expectedCost, err := svc.billingService.CalculateCost("gpt-5.1", UsageTokens{
		InputTokens:  20,
		OutputTokens: 10,
	}, 1.1)
	require.NoError(t, err)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     "resp_channel_mapped_billing",
			Model:         "glm",
			BillingModel:  "gpt-5.1-codex",
			UpstreamModel: "gpt-5.1-codex",
			Usage:         usage,
			Duration:      time.Second,
		},
		APIKey:  &APIKey{ID: 10},
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          1,
			OriginalModel:      "glm",
			ChannelMappedModel: "gpt-5.1", // channel mapped glm → gpt-5.1
			BillingModelSource: BillingModelSourceChannelMapped,
		},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, expectedCost.ActualCost, publisher.events[0].UsageLog.ActualCost)
	require.True(t, publisher.events[0].UsageLog.ActualCost > 0, "cost must not be zero")
}

func TestOpenAIGatewayServiceRecordUsage_SubscriptionBillingSetsSubscriptionFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	subscription := &UserSubscription{ID: 99}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_subscription_billing",
			Usage:     OpenAIUsage{InputTokens: 10, OutputTokens: 5},
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey:       &APIKey{ID: 100, GroupID: i64p(88), Group: &Group{ID: 88, SubscriptionType: SubscriptionTypeSubscription}},
		User:         &User{ID: 200},
		Account:      &Account{ID: 300},
		Subscription: subscription,
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.NotNil(t, publisher.events[0].UsageLog)
	require.Equal(t, BillingTypeSubscription, publisher.events[0].UsageLog.BillingType)
	require.NotNil(t, publisher.events[0].UsageLog.SubscriptionID)
	require.Equal(t, subscription.ID, *publisher.events[0].UsageLog.SubscriptionID)
}

func TestOpenAIGatewayServiceRecordUsage_SimpleModeStillPublishesBillingEvent(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc, publisher := newOpenAIRecordUsageServiceForTest(usageRepo, nil)
	svc.cfg.RunMode = config.RunModeSimple

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "resp_simple_mode",
			Usage:     OpenAIUsage{InputTokens: 10, OutputTokens: 5},
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 1000},
		User:    &User{ID: 2000},
		Account: &Account{ID: 3000},
	})

	require.NoError(t, err)
	require.Len(t, publisher.events, 1)
	require.Equal(t, 0, usageRepo.calls)
}
