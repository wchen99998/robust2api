package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	ErrMissingPlanner  = errors.New("gateway core planner is not configured")
	ErrMissingProvider = errors.New("gateway core provider adapter is not configured")
)

type Planner interface {
	Plan(req IngressRequest) (*RoutingPlan, *CanonicalRequest, error)
}

type ProviderRegistry interface {
	Get(platform string) (ProviderAdapter, error)
}

type AccountSource interface {
	ListAccounts(ctx context.Context, plan RoutingPlan) ([]*service.Account, error)
}

type AccountSelector interface {
	Select(ctx context.Context, plan RoutingPlan, accounts []*service.Account, excluded map[int64]struct{}) AccountSelection
}

type AccountSelection struct {
	Account     *service.Account
	Decision    AccountDecision
	Diagnostics CandidateDiagnostics
}

type TransportExecutor interface {
	Do(ctx context.Context, req *UpstreamRequest) (*UpstreamResult, error)
}

type WebSocketTransportExecutor interface {
	Proxy(ctx context.Context, req *UpstreamRequest, client WebSocketConn, firstType WebSocketMessageType, firstPayload []byte) error
}

type Engine struct {
	planner   Planner
	providers ProviderRegistry
	accounts  AccountSource
	selector  AccountSelector
	transport TransportExecutor
	ws        WebSocketTransportExecutor
	usage     UsageExtractor
}

func NewEngine(planner Planner, providers ProviderRegistry) *Engine {
	return &Engine{planner: planner, providers: providers}
}

func NewRuntimeEngine(planner Planner, providers ProviderRegistry, accounts AccountSource, selector AccountSelector, transport TransportExecutor, ws WebSocketTransportExecutor, usage UsageExtractor) *Engine {
	return &Engine{
		planner:   planner,
		providers: providers,
		accounts:  accounts,
		selector:  selector,
		transport: transport,
		ws:        ws,
		usage:     usage,
	}
}

func (e *Engine) Handle(ctx context.Context, req IngressRequest) (*GatewayResult, error) {
	plan, adapter, accounts, selection, err := e.planAndSelect(ctx, req)
	if err != nil {
		return nil, err
	}
	if e.accounts == nil || e.selector == nil || e.transport == nil {
		return &GatewayResult{
			StatusCode: 202,
			Plan:       plan,
		}, nil
	}
	if plan.Endpoint == EndpointModels {
		return modelsResult(plan, accounts), nil
	}
	if plan.Endpoint == EndpointUsage {
		return usageResult(plan, req), nil
	}
	if selection.Account == nil || len(accounts) == 0 {
		body, headers := noAccountsError(plan.Provider)
		return &GatewayResult{
			StatusCode: 503,
			Headers:    headers,
			Body:       body,
			Plan:       plan,
		}, nil
	}
	if plan.Meta == nil {
		plan.Meta = map[string]any{}
	}
	plan.Meta["body"] = string(req.Body)
	return e.executeHTTP(ctx, plan, adapter, accounts, selection)
}

func (e *Engine) HandleWebSocket(ctx context.Context, req IngressRequest, client WebSocketConn) error {
	if client == nil {
		return errors.New("websocket client is required")
	}
	if e == nil || e.ws == nil {
		_ = client.Close(1011, "gateway websocket transport is not configured")
		return errors.New("gateway websocket transport is not configured")
	}
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	firstType, firstPayload, err := client.Read(readCtx)
	cancel()
	if err != nil {
		_ = client.Close(1008, "failed to read initial websocket message")
		return err
	}
	req.Body = append([]byte(nil), firstPayload...)
	req.IsWebSocket = true
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	plan, adapter, _, selection, err := e.planAndSelect(ctx, req)
	if err != nil {
		_ = client.Close(1011, err.Error())
		return err
	}
	if selection.Account == nil {
		_ = client.Close(1013, "No available accounts")
		return errors.New("no available accounts")
	}
	plan.Account = selection.Decision
	if plan.Meta == nil {
		plan.Meta = map[string]any{}
	}
	plan.Meta["body"] = string(req.Body)
	upstreamReq, err := adapter.Prepare(ctx, *plan, selection.Account)
	if err != nil {
		_ = client.Close(1011, "failed to prepare upstream websocket request")
		return err
	}
	return e.ws.Proxy(ctx, upstreamReq, client, firstType, firstPayload)
}

func (e *Engine) executeHTTP(ctx context.Context, plan *RoutingPlan, adapter ProviderAdapter, accounts []*service.Account, initial AccountSelection) (*GatewayResult, error) {
	excluded := map[int64]struct{}{}
	selection := initial
	attempts := plan.Retry.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	var lastResult *GatewayResult
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if selection.Account == nil {
			selection = e.selector.Select(ctx, *plan, accounts, excluded)
			plan.Candidates = selection.Diagnostics
			if selection.Account == nil {
				break
			}
		}
		plan.Account = selection.Decision
		plan.Account.Attempt = attempt
		upstreamReq, err := adapter.Prepare(ctx, *plan, selection.Account)
		if err != nil {
			return nil, err
		}
		upstreamResult, err := e.transport.Do(ctx, upstreamReq)
		if upstreamResult != nil {
			upstreamResult.UpstreamError = err
			decoded, decodeErr := adapter.Decode(ctx, upstreamResult)
			if decodeErr == nil {
				decoded.Plan = plan
				if e.usage != nil {
					decoded.Usage = e.usage.Extract(ctx, *plan, decoded)
				}
				lastResult = decoded
			} else {
				lastErr = decodeErr
			}
		} else {
			lastErr = err
		}
		decision := adapter.ClassifyError(ctx, upstreamResult)
		if err == nil && !decision.Retryable {
			if lastResult != nil {
				return lastResult, nil
			}
			return nil, lastErr
		}
		if !decision.Retryable || attempt == attempts {
			break
		}
		if decision.FailoverAccount && selection.Account != nil {
			excluded[selection.Account.ID] = struct{}{}
			selection = AccountSelection{}
			continue
		}
	}
	if lastResult != nil {
		return lastResult, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	body, headers := noAccountsError(plan.Provider)
	return &GatewayResult{StatusCode: http.StatusServiceUnavailable, Headers: headers, Body: body, Plan: plan}, nil
}

func (e *Engine) planAndSelect(ctx context.Context, req IngressRequest) (*RoutingPlan, ProviderAdapter, []*service.Account, AccountSelection, error) {
	if e == nil || e.planner == nil {
		return nil, nil, nil, AccountSelection{}, ErrMissingPlanner
	}
	plan, canonical, err := e.planner.Plan(req)
	if err != nil {
		return nil, nil, nil, AccountSelection{}, err
	}
	if e.providers == nil {
		return nil, nil, nil, AccountSelection{}, ErrMissingProvider
	}
	adapter, err := e.providers.Get(canonical.Provider)
	if err != nil {
		return nil, nil, nil, AccountSelection{}, err
	}
	parsed, err := adapter.Parse(ctx, req)
	if err != nil {
		return nil, nil, nil, AccountSelection{}, err
	}
	if parsed == nil {
		return nil, nil, nil, AccountSelection{}, fmt.Errorf("provider %s returned nil canonical request", canonical.Provider)
	}
	plan.Provider = parsed.Provider
	plan.Endpoint = parsed.Endpoint
	if e.accounts == nil || e.selector == nil {
		return plan, adapter, nil, AccountSelection{}, nil
	}
	accounts, err := e.accounts.ListAccounts(ctx, *plan)
	if err != nil {
		return nil, nil, nil, AccountSelection{}, err
	}
	if plan.Endpoint == EndpointModels || plan.Endpoint == EndpointUsage {
		return plan, adapter, accounts, AccountSelection{}, nil
	}
	selection := e.selector.Select(ctx, *plan, accounts, nil)
	plan.Candidates = selection.Diagnostics
	if selection.Account != nil {
		plan.Account = selection.Decision
	}
	return plan, adapter, accounts, selection, nil
}

func httpJSONHeaders() http.Header {
	return http.Header{"content-type": {"application/json"}}
}

func noAccountsError(provider string) ([]byte, http.Header) {
	switch provider {
	case service.PlatformGemini, service.PlatformAntigravity:
		return []byte(`{"error":{"code":503,"message":"No available accounts","status":"UNAVAILABLE"}}`), httpJSONHeaders()
	case service.PlatformAnthropic:
		return []byte(`{"type":"error","error":{"type":"api_error","message":"No available accounts"}}`), httpJSONHeaders()
	default:
		return []byte(`{"error":{"type":"api_error","message":"No available accounts"}}`), httpJSONHeaders()
	}
}

func modelsResult(plan *RoutingPlan, accounts []*service.Account) *GatewayResult {
	type modelItem struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		DisplayName string `json:"display_name,omitempty"`
		CreatedAt   string `json:"created_at,omitempty"`
	}
	seen := map[string]struct{}{}
	items := make([]modelItem, 0)
	for _, account := range accounts {
		if account == nil {
			continue
		}
		for model := range account.GetModelMapping() {
			if model == "" || model == "*" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			items = append(items, modelItem{ID: model, Type: "model", DisplayName: model, CreatedAt: "2024-01-01T00:00:00Z"})
		}
	}
	if len(items) == 0 {
		switch plan.Provider {
		case service.PlatformOpenAI:
			body, _ := json.Marshal(map[string]any{"object": "list", "data": openai.DefaultModels})
			return &GatewayResult{StatusCode: http.StatusOK, Headers: httpJSONHeaders(), Body: body, Plan: plan}
		default:
			body, _ := json.Marshal(map[string]any{"object": "list", "data": claude.DefaultModels})
			return &GatewayResult{StatusCode: http.StatusOK, Headers: httpJSONHeaders(), Body: body, Plan: plan}
		}
	}
	body, _ := json.Marshal(map[string]any{"object": "list", "data": items})
	return &GatewayResult{StatusCode: http.StatusOK, Headers: httpJSONHeaders(), Body: body, Plan: plan}
}

func usageResult(plan *RoutingPlan, req IngressRequest) *GatewayResult {
	resp := map[string]any{
		"isValid": true,
		"unit":    "USD",
	}
	apiKey := req.APIKey
	if apiKey == nil {
		resp["isValid"] = false
		body, _ := json.Marshal(resp)
		return &GatewayResult{StatusCode: http.StatusOK, Headers: httpJSONHeaders(), Body: body, Plan: plan}
	}
	resp["status"] = apiKey.Status
	if apiKey.Quota > 0 || apiKey.HasRateLimits() {
		resp["mode"] = "quota_limited"
		resp["isValid"] = apiKey.Status == service.StatusAPIKeyActive ||
			apiKey.Status == service.StatusAPIKeyQuotaExhausted ||
			apiKey.Status == service.StatusAPIKeyExpired
		if apiKey.Quota > 0 {
			remaining := apiKey.GetQuotaRemaining()
			resp["quota"] = map[string]any{
				"limit":     apiKey.Quota,
				"used":      apiKey.QuotaUsed,
				"remaining": remaining,
				"unit":      "USD",
			}
			resp["remaining"] = remaining
		}
		if apiKey.ExpiresAt != nil {
			resp["expires_at"] = apiKey.ExpiresAt
			resp["days_until_expiry"] = apiKey.GetDaysUntilExpiry()
		}
	} else {
		resp["mode"] = "unrestricted"
		if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
			resp["planName"] = apiKey.Group.Name
			if req.Subscription != nil {
				resp["subscription"] = map[string]any{
					"daily_usage_usd":   req.Subscription.DailyUsageUSD,
					"weekly_usage_usd":  req.Subscription.WeeklyUsageUSD,
					"monthly_usage_usd": req.Subscription.MonthlyUsageUSD,
					"daily_limit_usd":   apiKey.Group.DailyLimitUSD,
					"weekly_limit_usd":  apiKey.Group.WeeklyLimitUSD,
					"monthly_limit_usd": apiKey.Group.MonthlyLimitUSD,
					"expires_at":        req.Subscription.ExpiresAt,
				}
			}
		} else {
			resp["planName"] = "钱包余额"
			if apiKey.User != nil {
				resp["remaining"] = apiKey.User.Balance
				resp["balance"] = apiKey.User.Balance
			}
		}
	}
	body, _ := json.Marshal(resp)
	return &GatewayResult{StatusCode: http.StatusOK, Headers: httpJSONHeaders(), Body: body, Plan: plan}
}
