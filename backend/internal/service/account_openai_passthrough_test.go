package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsCodexCLIOnlyEnabled(t *testing.T) {
	t.Run("OpenAI OAuth 开启", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only": true,
			},
		}
		require.True(t, account.IsCodexCLIOnlyEnabled())
	})

	t.Run("OpenAI OAuth 关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only": false,
			},
		}
		require.False(t, account.IsCodexCLIOnlyEnabled())
	})

	t.Run("字段缺失默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		}
		require.False(t, account.IsCodexCLIOnlyEnabled())
	})

	t.Run("类型非法默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only": "true",
			},
		}
		require.False(t, account.IsCodexCLIOnlyEnabled())
	})

	t.Run("非 OAuth 账号始终关闭", func(t *testing.T) {
		apiKeyAccount := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"codex_cli_only": true,
			},
		}
		require.False(t, apiKeyAccount.IsCodexCLIOnlyEnabled())

		otherPlatform := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only": true,
			},
		}
		require.False(t, otherPlatform.IsCodexCLIOnlyEnabled())
	})
}

func TestAccount_IsOpenAIResponsesWebSocketV2Enabled(t *testing.T) {
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}).IsOpenAIResponsesWebSocketV2Enabled())
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"responses_websockets_v2_enabled": false}}).IsOpenAIResponsesWebSocketV2Enabled())
	require.False(t, (&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{"responses_websockets_v2_enabled": true}}).IsOpenAIResponsesWebSocketV2Enabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: "unknown_type"}).IsOpenAIResponsesWebSocketV2Enabled())
}

func TestAccount_OpenAIWSExtraFlags(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_ws_force_http": true,
		},
	}
	require.True(t, account.IsOpenAIWSForceHTTPEnabled())

	off := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}
	require.False(t, off.IsOpenAIWSForceHTTPEnabled())
}
