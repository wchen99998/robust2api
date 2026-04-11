package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wchen99998/robust2api/internal/config"
	"github.com/wchen99998/robust2api/internal/pkg/tlsfingerprint"
	"github.com/wchen99998/robust2api/internal/server/middleware"
	"github.com/wchen99998/robust2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOpenAIHandlers_CreateRootAndAuthSpansOnAuthFailure(t *testing.T) {
	t.Run("responses", func(t *testing.T) {
		assertHandlerRootAndAuthSpans(t, "gateway.responses", "http", http.StatusUnauthorized, func(h *OpenAIGatewayHandler, c *gin.Context) {
			h.Responses(c)
		})
	})

	t.Run("chat_completions", func(t *testing.T) {
		assertHandlerRootAndAuthSpans(t, "gateway.chat_completions", "http", http.StatusUnauthorized, func(h *OpenAIGatewayHandler, c *gin.Context) {
			h.ChatCompletions(c)
		})
	})

	t.Run("responses_ws", func(t *testing.T) {
		assertHandlerRootAndAuthSpans(t, "gateway.responses_ws", "ws", http.StatusUnauthorized, func(h *OpenAIGatewayHandler, c *gin.Context) {
			c.Request.Header.Set("Upgrade", "websocket")
			c.Request.Header.Set("Connection", "Upgrade")
			h.ResponsesWebSocket(c)
		})
	})
}

func TestOpenAIResponses_RootSpanIncludesRetryEventsAndChildSpans(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider()
	traceProvider.RegisterSpanProcessor(recorder)
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(traceProvider)
	defer func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
		otel.SetTracerProvider(previousProvider)
	}()

	groupID := int64(88)
	account := service.Account{
		ID:          9001,
		Name:        "openai-pool-mode",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
		Credentials: map[string]any{
			"api_key":               "sk-test",
			"base_url":              "http://openai.internal",
			"pool_mode":             true,
			"pool_mode_retry_count": 1,
		},
	}

	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, cfg)
	defer billingCache.Stop()

	concurrencyService := service.NewConcurrencyService(handlerStubConcurrencyCache{})
	gatewayService := service.NewOpenAIGatewayService(
		handlerStubOpenAIAccountRepo{accounts: []service.Account{account}},
		nil,
		nil,
		&handlerStubGatewayCache{},
		cfg,
		nil,
		concurrencyService,
		nil,
		nil,
		billingCache,
		&handlerHTTPUpstreamSequenceRecorder{
			responses: []*http.Response{
				{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"rate limited"}}`))),
				},
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"rid-handler-trace"}},
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"resp_handler_trace"}`))),
				},
			},
		},
		nil,
		nil,
		nil,
		nil,
	)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      7001,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID},
		User:    &service.User{ID: 42},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      42,
		Concurrency: 1,
	})

	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCache,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(concurrencyService, SSEPingFormatNone, time.Second),
		maxAccountSwitches:  2,
		cfg:                 cfg,
	}

	h.Responses(c)

	require.Equal(t, http.StatusOK, w.Code)

	rootSpan := findEndedSpanByName(t, recorder.Ended(), "gateway.responses")
	require.True(t, spanHasEvent(rootSpan, "robust2api.failover_candidate_detected"))
	require.True(t, spanHasEvent(rootSpan, "robust2api.same_account_retry"))

	authSpan := findEndedSpanByName(t, recorder.Ended(), "gateway.auth")
	billingSpan := findEndedSpanByName(t, recorder.Ended(), "gateway.billing")
	selectSpan := findEndedSpanByName(t, recorder.Ended(), "gateway.select_account")
	queueUserSpan := findEndedSpanByName(t, recorder.Ended(), "gateway.queue_wait.user")
	queueAccountSpan := findEndedSpanByName(t, recorder.Ended(), "gateway.queue_wait.account")
	usageSpan := findEndedSpanByName(t, recorder.Ended(), "gateway.record_usage")

	require.Equal(t, rootSpan.SpanContext().SpanID(), authSpan.Parent().SpanID())
	require.Equal(t, rootSpan.SpanContext().SpanID(), billingSpan.Parent().SpanID())
	require.Equal(t, rootSpan.SpanContext().SpanID(), selectSpan.Parent().SpanID())
	require.Equal(t, rootSpan.SpanContext().SpanID(), queueUserSpan.Parent().SpanID())
	require.Equal(t, rootSpan.SpanContext().SpanID(), queueAccountSpan.Parent().SpanID())
	require.Equal(t, rootSpan.SpanContext().SpanID(), usageSpan.Parent().SpanID())
}

func assertHandlerRootAndAuthSpans(
	t *testing.T,
	rootName string,
	wantTransport string,
	wantStatus int,
	invoke func(h *OpenAIGatewayHandler, c *gin.Context),
) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider()
	traceProvider.RegisterSpanProcessor(recorder)
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(traceProvider)
	defer func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
		otel.SetTracerProvider(previousProvider)
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/test", nil)

	h := &OpenAIGatewayHandler{}
	invoke(h, c)

	require.Equal(t, wantStatus, w.Code)

	rootSpan := findEndedSpanByName(t, recorder.Ended(), rootName)
	authSpan := findEndedSpanByName(t, recorder.Ended(), "gateway.auth")
	require.Equal(t, rootSpan.SpanContext().SpanID(), authSpan.Parent().SpanID())
	require.Equal(t, wantTransport, spanAttributeValue(rootSpan, "robust2api.transport"))
}

func findEndedSpanByName(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("span %q not found", name)
	return nil
}

func spanAttributeValue(span sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			if value, ok := attr.Value.AsInterface().(string); ok {
				return value
			}
		}
	}
	return ""
}

func spanHasEvent(span sdktrace.ReadOnlySpan, name string) bool {
	for _, event := range span.Events() {
		if event.Name == name {
			return true
		}
	}
	return false
}

type handlerStubOpenAIAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r handlerStubOpenAIAccountRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, io.EOF
}

func (r handlerStubOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	var accounts []service.Account
	for _, account := range r.accounts {
		if account.Platform == platform {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r handlerStubOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByGroupIDAndPlatform(ctx, 0, platform)
}

func (r handlerStubOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

type handlerStubGatewayCache struct{}

func (c *handlerStubGatewayCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, io.EOF
}

func (c *handlerStubGatewayCache) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (c *handlerStubGatewayCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *handlerStubGatewayCache) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

type handlerStubConcurrencyCache struct {
	service.ConcurrencyCache
}

func (handlerStubConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (handlerStubConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error {
	return nil
}

func (handlerStubConcurrencyCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (handlerStubConcurrencyCache) ReleaseUserSlot(context.Context, int64, string) error {
	return nil
}

func (handlerStubConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	loads := make(map[int64]*service.AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		loads[account.ID] = &service.AccountLoadInfo{AccountID: account.ID, LoadRate: 0}
	}
	return loads, nil
}

func (handlerStubConcurrencyCache) GetUsersLoadBatch(ctx context.Context, users []service.UserWithConcurrency) (map[int64]*service.UserLoadInfo, error) {
	loads := make(map[int64]*service.UserLoadInfo, len(users))
	for _, user := range users {
		loads[user.ID] = &service.UserLoadInfo{UserID: user.ID, LoadRate: 0}
	}
	return loads, nil
}

type handlerHTTPUpstreamSequenceRecorder struct {
	responses []*http.Response
	callCount int
}

func (u *handlerHTTPUpstreamSequenceRecorder) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	index := u.callCount
	u.callCount++
	if index >= len(u.responses) {
		return u.responses[len(u.responses)-1], nil
	}
	return u.responses[index], nil
}

func (u *handlerHTTPUpstreamSequenceRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}
