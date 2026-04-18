package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSProtocolResolver_Resolve(t *testing.T) {
	baseCfg := &config.Config{}

	t.Run("openai oauth defaults to ws v2", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_default", decision.Reason)
	})

	t.Run("openai apikey defaults to ws v2", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": false,
				"responses_websockets_v2_enabled":               false,
			},
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_default", decision.Reason)
	})

	t.Run("account force http still wins", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
			Extra: map[string]any{
				"openai_ws_force_http": true,
			},
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_force_http", decision.Reason)
	})

	t.Run("global force http still wins", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Gateway.OpenAIWS.ForceHTTP = true
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
		}
		decision := NewOpenAIWSProtocolResolver(cfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "global_force_http", decision.Reason)
	})

	t.Run("unknown auth type falls back to http", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformOpenAI,
			Type:        "unknown_type",
			Concurrency: 1,
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "unknown_auth_type", decision.Reason)
	})

	t.Run("non-positive concurrency falls back to http", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_concurrency_invalid", decision.Reason)
	})
}
