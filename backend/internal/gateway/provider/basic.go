package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type BasicAdapter struct {
	platform string
}

func NewOpenAIAdapter() *BasicAdapter {
	return &BasicAdapter{platform: service.PlatformOpenAI}
}

func NewAnthropicAdapter() *BasicAdapter {
	return &BasicAdapter{platform: service.PlatformAnthropic}
}

func NewGeminiAdapter() *BasicAdapter {
	return &BasicAdapter{platform: service.PlatformGemini}
}

func NewAntigravityAdapter() *BasicAdapter {
	return &BasicAdapter{platform: service.PlatformAntigravity}
}

func (a *BasicAdapter) Provider() string {
	if a == nil {
		return ""
	}
	return a.platform
}

func (a *BasicAdapter) Parse(_ context.Context, req core.IngressRequest) (*core.CanonicalRequest, error) {
	if a == nil || a.platform == "" {
		return nil, fmt.Errorf("provider adapter is not configured")
	}
	model, stream, err := parseJSONModelStream(req.Body)
	if err != nil {
		return nil, err
	}
	stream = stream || req.IsWebSocket
	endpoint := req.Endpoint
	if endpoint == core.EndpointUnknown {
		endpoint = detectEndpoint(req.Path)
	}
	return &core.CanonicalRequest{
		RequestID:      req.RequestID,
		Endpoint:       endpoint,
		Provider:       a.platform,
		RequestedModel: model,
		Stream:         stream,
		Body:           append([]byte(nil), req.Body...),
		Headers:        req.Headers.Clone(),
		Subpath:        responsesSubpath(req.RawPath),
		Session: core.SessionInput{
			ClientIP:  req.ClientIP,
			UserAgent: req.Headers.Get("User-Agent"),
		},
	}, nil
}

func (a *BasicAdapter) Prepare(_ context.Context, plan core.RoutingPlan, account *service.Account) (*core.UpstreamRequest, error) {
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	targetURL, err := a.upstreamURL(plan, account)
	if err != nil {
		return nil, err
	}
	body := []byte(nil)
	if raw, ok := plan.Meta["body"].(string); ok {
		body = []byte(raw)
	}
	body = a.normalizeBody(plan, body)
	if mapped := strings.TrimSpace(account.GetMappedModel(plan.Model.UpstreamModel)); mapped != "" && mapped != plan.Model.UpstreamModel && len(body) > 0 && json.Valid(body) {
		if rewritten, err := sjson.SetBytes(body, "model", mapped); err == nil {
			body = rewritten
		}
	}
	headers := a.upstreamHeaders(plan, account)
	return &core.UpstreamRequest{
		Method:    plan.Transport.Method,
		URL:       targetURL,
		Headers:   headers,
		Body:      body,
		AccountID: account.ID,
		ProxyURL:  accountProxyURL(account),
	}, nil
}

func (a *BasicAdapter) Decode(_ context.Context, upstream *core.UpstreamResult) (*core.GatewayResult, error) {
	if upstream == nil {
		return nil, fmt.Errorf("upstream result is required")
	}
	return &core.GatewayResult{
		StatusCode: upstream.StatusCode,
		Headers:    upstream.Headers.Clone(),
		Body:       append([]byte(nil), upstream.Body...),
		Stream:     upstream.Stream,
	}, nil
}

func (a *BasicAdapter) ClassifyError(_ context.Context, upstream *core.UpstreamResult) core.UpstreamErrorDecision {
	if upstream == nil {
		return core.UpstreamErrorDecision{Retryable: true, FailoverAccount: true}
	}
	switch upstream.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return core.UpstreamErrorDecision{Retryable: true, FailoverAccount: true}
	default:
		return core.UpstreamErrorDecision{}
	}
}

func (a *BasicAdapter) upstreamURL(plan core.RoutingPlan, account *service.Account) (string, error) {
	if explicit := strings.TrimSpace(plan.Transport.UpstreamURL); explicit != "" {
		return explicit, nil
	}
	switch a.platform {
	case service.PlatformOpenAI:
		if account.Type == service.AccountTypeOAuth {
			return "https://chatgpt.com/backend-api/codex/responses", nil
		}
		base := account.GetOpenAIBaseURL()
		if base == "" {
			base = "https://api.openai.com"
		}
		validated, err := urlvalidator.ValidateURLFormat(base, false)
		if err != nil {
			return "", err
		}
		switch plan.Endpoint {
		case core.EndpointChatCompletions:
			return appendPath(validated, "/v1/chat/completions"), nil
		default:
			return appendPath(validated, "/v1/responses"), nil
		}
	case service.PlatformAnthropic:
		base := account.GetBaseURL()
		if base == "" {
			base = "https://api.anthropic.com"
		}
		validated, err := urlvalidator.ValidateURLFormat(base, false)
		if err != nil {
			return "", err
		}
		if plan.Endpoint == core.EndpointCountTokens {
			return appendPath(validated, "/v1/messages/count_tokens"), nil
		}
		return appendPath(validated, "/v1/messages?beta=true"), nil
	case service.PlatformGemini:
		base := account.GetGeminiBaseURL("https://generativelanguage.googleapis.com")
		validated, err := urlvalidator.ValidateURLFormat(base, false)
		if err != nil {
			return "", err
		}
		if rawPath, ok := plan.Meta["raw_path"].(string); ok && strings.Contains(rawPath, "/v1beta/models") {
			return appendPath(validated, rawPath[strings.Index(rawPath, "/v1beta/models"):]), nil
		}
		return appendPath(validated, "/v1beta/models"), nil
	case service.PlatformAntigravity:
		base := account.GetBaseURL()
		if base == "" {
			base = "https://generativelanguage.googleapis.com/antigravity"
		}
		validated, err := urlvalidator.ValidateURLFormat(base, false)
		if err != nil {
			return "", err
		}
		if rawPath, ok := plan.Meta["raw_path"].(string); ok && strings.Contains(rawPath, "/v1beta/models") {
			return appendPath(validated, rawPath[strings.Index(rawPath, "/v1beta/models"):]), nil
		}
		return appendPath(validated, "/v1/messages"), nil
	default:
		return "", fmt.Errorf("unsupported provider: %s", a.platform)
	}
}

func (a *BasicAdapter) upstreamHeaders(plan core.RoutingPlan, account *service.Account) http.Header {
	headers := http.Header{}
	headers.Set("content-type", "application/json")
	switch a.platform {
	case service.PlatformOpenAI:
		token := account.GetOpenAIApiKey()
		if token == "" {
			token = account.GetOpenAIAccessToken()
		}
		if token != "" {
			headers.Set("authorization", "Bearer "+token)
		}
		if account.Type == service.AccountTypeOAuth {
			headers.Set("OpenAI-Beta", "responses=experimental")
			if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
				headers.Set("chatgpt-account-id", chatgptAccountID)
			}
		}
	case service.PlatformAnthropic:
		token := account.GetCredential("api_key")
		if token == "" {
			token = account.GetCredential("access_token")
		}
		if token != "" {
			if strings.EqualFold(account.GetCredential("token_type"), "bearer") {
				headers.Set("authorization", "Bearer "+token)
			} else {
				headers.Set("x-api-key", token)
			}
		}
		headers.Set("anthropic-version", "2023-06-01")
	case service.PlatformGemini, service.PlatformAntigravity:
		if token := account.GetCredential("api_key"); token != "" {
			headers.Set("x-goog-api-key", token)
		}
	}
	if plan.Transport.Stream {
		headers.Set("accept", "text/event-stream")
	}
	return headers
}

func (a *BasicAdapter) normalizeBody(plan core.RoutingPlan, body []byte) []byte {
	if len(body) == 0 || !json.Valid(body) {
		return body
	}
	switch {
	case a.platform == service.PlatformOpenAI && plan.Endpoint == core.EndpointMessages:
		if converted, err := anthropicMessagesToOpenAIResponses(body); err == nil {
			return converted
		}
	case a.platform == service.PlatformAnthropic && plan.Endpoint == core.EndpointChatCompletions:
		if converted, err := openAIChatToAnthropicMessages(body); err == nil {
			return converted
		}
	case a.platform == service.PlatformAnthropic && plan.Endpoint == core.EndpointResponses:
		if converted, err := openAIResponsesToAnthropicMessages(body); err == nil {
			return converted
		}
	}
	return body
}

func anthropicMessagesToOpenAIResponses(body []byte) ([]byte, error) {
	var input map[string]any
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, err
	}
	output := map[string]any{}
	copyIfPresent(output, input, "model", "model")
	copyIfPresent(output, input, "stream", "stream")
	copyIfPresent(output, input, "temperature", "temperature")
	copyIfPresent(output, input, "top_p", "top_p")
	copyIfPresent(output, input, "metadata", "metadata")
	copyIfPresent(output, input, "tools", "tools")
	copyIfPresent(output, input, "tool_choice", "tool_choice")
	if maxTokens, ok := input["max_tokens"]; ok {
		output["max_output_tokens"] = maxTokens
	}
	if system, ok := input["system"]; ok {
		output["instructions"] = system
	}
	if messages, ok := input["messages"]; ok {
		output["input"] = messages
	}
	return json.Marshal(output)
}

func openAIChatToAnthropicMessages(body []byte) ([]byte, error) {
	var input map[string]any
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, err
	}
	output := map[string]any{}
	copyIfPresent(output, input, "model", "model")
	copyIfPresent(output, input, "stream", "stream")
	copyIfPresent(output, input, "temperature", "temperature")
	copyIfPresent(output, input, "top_p", "top_p")
	copyIfPresent(output, input, "metadata", "metadata")
	if maxTokens, ok := input["max_tokens"]; ok {
		output["max_tokens"] = maxTokens
	} else {
		output["max_tokens"] = 4096
	}
	messages, system := splitOpenAIMessages(input["messages"])
	output["messages"] = messages
	if len(system) == 1 {
		output["system"] = system[0]
	} else if len(system) > 1 {
		output["system"] = strings.Join(system, "\n\n")
	}
	return json.Marshal(output)
}

func openAIResponsesToAnthropicMessages(body []byte) ([]byte, error) {
	var input map[string]any
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, err
	}
	output := map[string]any{}
	copyIfPresent(output, input, "model", "model")
	copyIfPresent(output, input, "stream", "stream")
	copyIfPresent(output, input, "temperature", "temperature")
	copyIfPresent(output, input, "top_p", "top_p")
	copyIfPresent(output, input, "metadata", "metadata")
	if maxTokens, ok := input["max_output_tokens"]; ok {
		output["max_tokens"] = maxTokens
	} else {
		output["max_tokens"] = 4096
	}
	if instructions, ok := input["instructions"]; ok {
		output["system"] = instructions
	}
	if messages, ok := input["input"]; ok {
		output["messages"] = normalizeAnthropicMessages(messages)
	} else {
		output["messages"] = []any{}
	}
	return json.Marshal(output)
}

func splitOpenAIMessages(raw any) ([]any, []string) {
	items, ok := raw.([]any)
	if !ok {
		return []any{}, nil
	}
	messages := make([]any, 0, len(items))
	system := make([]string, 0)
	for _, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		if role == "system" || role == "developer" {
			if text := contentAsText(message["content"]); text != "" {
				system = append(system, text)
			}
			continue
		}
		if role == "assistant" || role == "user" {
			messages = append(messages, map[string]any{
				"role":    role,
				"content": message["content"],
			})
		}
	}
	return messages, system
}

func normalizeAnthropicMessages(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case string:
		return []any{map[string]any{"role": "user", "content": typed}}
	default:
		return []any{}
	}
}

func copyIfPresent(dst, src map[string]any, from, to string) {
	if value, ok := src[from]; ok {
		dst[to] = value
	}
}

func contentAsText(raw any) string {
	switch typed := raw.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := block["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func appendPath(base, suffix string) string {
	trimmed := strings.TrimRight(base, "/")
	if strings.HasSuffix(trimmed, strings.TrimSuffix(suffix, "?beta=true")) || strings.HasSuffix(trimmed, suffix) {
		return trimmed
	}
	return trimmed + suffix
}

func accountProxyURL(account *service.Account) string {
	if account == nil || account.Proxy == nil {
		return ""
	}
	return account.Proxy.URL()
}

func parseJSONModelStream(body []byte) (string, bool, error) {
	if len(body) == 0 {
		return "", false, nil
	}
	if !json.Valid(body) {
		return "", false, fmt.Errorf("invalid JSON request body")
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	stream := gjson.GetBytes(body, "stream").Bool()
	return model, stream, nil
}

func detectEndpoint(path string) core.EndpointKind {
	switch {
	case strings.Contains(path, "/messages/count_tokens"):
		return core.EndpointCountTokens
	case strings.Contains(path, "/messages"):
		return core.EndpointMessages
	case strings.Contains(path, "/chat/completions"):
		return core.EndpointChatCompletions
	case strings.Contains(path, "/responses"):
		return core.EndpointResponses
	case strings.Contains(path, "/v1beta/models"):
		return core.EndpointGeminiModels
	case strings.HasSuffix(path, "/models"):
		return core.EndpointModels
	case strings.HasSuffix(path, "/usage"):
		return core.EndpointUsage
	default:
		return core.EndpointUnknown
	}
}

func responsesSubpath(rawPath string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(rawPath), "/")
	idx := strings.LastIndex(trimmed, "/responses")
	if idx < 0 {
		return ""
	}
	return trimmed[idx+len("/responses"):]
}
