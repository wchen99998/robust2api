package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/gateway/ingress"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const defaultResponsesURL = "https://api.openai.com/v1/responses"
const defaultChatGPTCodexResponsesURL = "https://chatgpt.com/backend-api/codex/responses"

type Adapter struct{}

func (Adapter) Provider() string {
	return service.PlatformOpenAI
}

func (Adapter) Parse(_ context.Context, req core.IngressRequest) (*core.CanonicalRequest, error) {
	return ingress.ParseOpenAIResponses(req, core.SessionInput{})
}

func (Adapter) Prepare(_ context.Context, plan core.RoutingPlan, account *service.Account) (*core.UpstreamRequest, error) {
	if account == nil {
		return nil, errors.New("account is required")
	}
	targetURL := defaultResponsesURL
	switch account.Type {
	case service.AccountTypeOAuth:
		targetURL = defaultChatGPTCodexResponsesURL
	case service.AccountTypeAPIKey:
		targetURL = buildResponsesURL(account.GetOpenAIBaseURL())
	default:
		return nil, fmt.Errorf("unsupported OpenAI account type: %s", account.Type)
	}
	token := account.GetOpenAIApiKey()
	if account.Type == service.AccountTypeOAuth {
		token = account.GetOpenAIAccessToken()
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("openai account token is required")
	}
	req := &core.UpstreamRequest{
		Method:  http.MethodPost,
		URL:     targetURL,
		Headers: http.Header{},
		Stream:  plan.Endpoint == core.EndpointResponsesWebSocket,
	}
	req.Headers.Set("authorization", "Bearer "+token)
	req.Headers.Set("content-type", "application/json")
	if account.Type == service.AccountTypeOAuth {
		req.Headers.Set("accept", "text/event-stream")
		req.Headers.Set("OpenAI-Beta", "responses=experimental")
		if chatGPTAccountID := account.GetChatGPTAccountID(); strings.TrimSpace(chatGPTAccountID) != "" {
			req.Headers.Set("chatgpt-account-id", chatGPTAccountID)
		}
	}
	if customUA := account.GetOpenAIUserAgent(); strings.TrimSpace(customUA) != "" {
		req.Headers.Set("user-agent", customUA)
	}
	if plan.Model.UpstreamModel != "" {
		req.Headers.Set("x-sub2api-upstream-model", plan.Model.UpstreamModel)
	}
	return req, nil
}

func buildResponsesURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalized == "" {
		return defaultResponsesURL
	}
	if strings.HasSuffix(normalized, "/responses") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/responses"
	}
	return normalized + "/v1/responses"
}

func PrepareResponsesBody(plan core.RoutingPlan, body []byte) []byte {
	targetModel := strings.TrimSpace(plan.Model.ChannelMappedModel)
	if targetModel == "" || targetModel == strings.TrimSpace(plan.Model.RequestedModel) {
		return append([]byte(nil), body...)
	}
	return service.ReplaceModelInBody(body, targetModel)
}

func (Adapter) Decode(_ context.Context, upstream *core.UpstreamResult) (*core.GatewayResult, error) {
	if upstream == nil {
		return nil, errors.New("upstream result is required")
	}
	return &core.GatewayResult{
		Status:  upstream.StatusCode,
		Headers: upstream.Headers.Clone(),
		Body:    append([]byte(nil), upstream.Body...),
	}, nil
}

func (Adapter) ClassifyError(_ context.Context, upstream *core.UpstreamResult) core.UpstreamErrorDecision {
	if upstream == nil {
		return core.UpstreamErrorDecision{
			Retryable:          true,
			ClientStatus:       http.StatusBadGateway,
			ClientErrorType:    "upstream_error",
			ClientErrorMessage: "Upstream request failed",
		}
	}
	retryable := upstream.StatusCode == http.StatusTooManyRequests || upstream.StatusCode >= 500
	return core.UpstreamErrorDecision{
		Retryable:          retryable,
		RetrySameAccount:   retryable,
		ClientStatus:       upstream.StatusCode,
		ClientErrorType:    "upstream_error",
		ClientErrorMessage: "Upstream request failed",
		UpstreamStatus:     upstream.StatusCode,
	}
}
