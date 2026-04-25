package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBasicAdapterParse(t *testing.T) {
	adapter := NewOpenAIAdapter()

	got, err := adapter.Parse(context.Background(), core.IngressRequest{
		RequestID: "req_1",
		Path:      "/openai/v1/responses/compact",
		RawPath:   "/openai/v1/responses/compact",
		Headers:   http.Header{"User-Agent": {"codex"}},
		Body:      []byte(`{"model":"gpt-5","stream":true}`),
		ClientIP:  "127.0.0.1",
	})

	require.NoError(t, err)
	require.Equal(t, service.PlatformOpenAI, got.Provider)
	require.Equal(t, core.EndpointResponses, got.Endpoint)
	require.Equal(t, "gpt-5", got.RequestedModel)
	require.True(t, got.Stream)
	require.Equal(t, "/compact", got.Subpath)
}

func TestBasicAdapterClassifyError(t *testing.T) {
	decision := NewAnthropicAdapter().ClassifyError(context.Background(), &core.UpstreamResult{
		StatusCode: http.StatusTooManyRequests,
	})

	require.True(t, decision.Retryable)
	require.True(t, decision.FailoverAccount)
}

func TestOpenAIAdapterPrepareBuildsAPIKeyResponsesRequest(t *testing.T) {
	adapter := NewOpenAIAdapter()

	got, err := adapter.Prepare(context.Background(), core.RoutingPlan{
		Endpoint:  core.EndpointResponses,
		Provider:  service.PlatformOpenAI,
		Model:     core.ModelResolution{UpstreamModel: "gpt-5"},
		Transport: core.TransportPlan{Method: http.MethodPost},
		Meta:      map[string]any{"body": `{"model":"gpt-5"}`},
	}, &service.Account{
		ID:       9,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/responses", got.URL)
	require.Equal(t, "Bearer sk-test", got.Headers.Get("authorization"))
	require.JSONEq(t, `{"model":"gpt-5"}`, string(got.Body))
}

func TestAnthropicAdapterPrepareBuildsCountTokensRequest(t *testing.T) {
	adapter := NewAnthropicAdapter()

	got, err := adapter.Prepare(context.Background(), core.RoutingPlan{
		Endpoint:  core.EndpointCountTokens,
		Provider:  service.PlatformAnthropic,
		Transport: core.TransportPlan{Method: http.MethodPost},
		Meta:      map[string]any{"body": `{}`},
	}, &service.Account{
		ID:       10,
		Platform: service.PlatformAnthropic,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-ant",
			"base_url": "https://api.anthropic.com",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "https://api.anthropic.com/v1/messages/count_tokens", got.URL)
	require.Equal(t, "sk-ant", got.Headers.Get("x-api-key"))
	require.Equal(t, "2023-06-01", got.Headers.Get("anthropic-version"))
}

func TestOpenAIAdapterPrepareConvertsAnthropicMessages(t *testing.T) {
	adapter := NewOpenAIAdapter()

	got, err := adapter.Prepare(context.Background(), core.RoutingPlan{
		Endpoint:  core.EndpointMessages,
		Provider:  service.PlatformOpenAI,
		Model:     core.ModelResolution{UpstreamModel: "claude-sonnet-4-5"},
		Transport: core.TransportPlan{Method: http.MethodPost},
		Meta: map[string]any{"body": `{
			"model":"claude-sonnet-4-5",
			"system":"be concise",
			"max_tokens":128,
			"messages":[{"role":"user","content":"hello"}]
		}`},
	}, &service.Account{
		ID:       11,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/responses", got.URL)
	require.JSONEq(t, `{
		"model":"claude-sonnet-4-5",
		"instructions":"be concise",
		"max_output_tokens":128,
		"input":[{"role":"user","content":"hello"}]
	}`, string(got.Body))
}

func TestAnthropicAdapterPrepareConvertsOpenAIChatCompletions(t *testing.T) {
	adapter := NewAnthropicAdapter()

	got, err := adapter.Prepare(context.Background(), core.RoutingPlan{
		Endpoint:  core.EndpointChatCompletions,
		Provider:  service.PlatformAnthropic,
		Model:     core.ModelResolution{UpstreamModel: "gpt-5"},
		Transport: core.TransportPlan{Method: http.MethodPost},
		Meta: map[string]any{"body": `{
			"model":"gpt-5",
			"stream":true,
			"messages":[
				{"role":"system","content":"be concise"},
				{"role":"user","content":"hello"}
			]
		}`},
	}, &service.Account{
		ID:       12,
		Platform: service.PlatformAnthropic,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-ant",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "https://api.anthropic.com/v1/messages?beta=true", got.URL)
	require.JSONEq(t, `{
		"model":"gpt-5",
		"stream":true,
		"max_tokens":4096,
		"system":"be concise",
		"messages":[{"role":"user","content":"hello"}]
	}`, string(got.Body))
}
