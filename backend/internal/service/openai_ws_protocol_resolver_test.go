package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSProtocolResolver_Resolve(t *testing.T) {
	baseCfg := &config.Config{}
	baseCfg.Gateway.OpenAIWS.Enabled = true
	baseCfg.Gateway.OpenAIWS.OAuthEnabled = true
	baseCfg.Gateway.OpenAIWS.APIKeyEnabled = true
	baseCfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	baseCfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool

	baseAccount := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool,
		},
	}

	t.Run("ctx_pool mode routes to ws v2", func(t *testing.T) {
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(baseAccount)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_mode_ctx_pool", decision.Reason)
	})

	t.Run("passthrough mode routes to ws v2", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
			Extra: map[string]any{
				"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
			},
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_mode_passthrough", decision.Reason)
	})

	t.Run("invalid or off mode routes to http", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
			Extra: map[string]any{
				"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
			},
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_mode_off", decision.Reason)
	})

	t.Run("non-positive concurrency is rejected", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool,
			},
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_concurrency_invalid", decision.Reason)
	})

	t.Run("feature disabled routes to http", func(t *testing.T) {
		cfg := *baseCfg
		cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = false
		decision := NewOpenAIWSProtocolResolver(&cfg).Resolve(baseAccount)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "feature_disabled", decision.Reason)
	})

	t.Run("force http and auth-type gates still apply", func(t *testing.T) {
		cfg := *baseCfg
		cfg.Gateway.OpenAIWS.OAuthEnabled = false
		decision := NewOpenAIWSProtocolResolver(&cfg).Resolve(baseAccount)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "oauth_disabled", decision.Reason)

		account := *baseAccount
		account.Extra = map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool,
			"openai_ws_force_http":                      true,
		}
		decision = NewOpenAIWSProtocolResolver(baseCfg).Resolve(&account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_force_http", decision.Reason)
	})
}
