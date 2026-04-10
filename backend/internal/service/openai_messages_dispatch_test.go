package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIMessagesDispatchModelConfig(t *testing.T) {
	cfg := normalizeOpenAIMessagesDispatchModelConfig(OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   " gpt-5.4 ",
		SonnetMappedModel: " gpt-5.3-codex ",
		HaikuMappedModel:  " gpt-5.4-mini-high ",
		ExactModelMappings: map[string]string{
			" claude-opus-4-6 ":              " gpt-5.4 ",
			"claude-sonnet-4-5-20250929":     " gpt-5.4-mini-high ",
			"claude-haiku-4-5-20251001":      " ",
			"":                               "gpt-5.2",
			"claude-3-5-sonnet-20241022":     "",
			"claude-3-5-haiku-20241022":      " gpt-5.4-mini ",
			" claude-3-7-sonnet-20250219 ":   " gpt-5.3-codex ",
			"claude-opus-4-6-20251102":       "gpt-5.4",
			"claude-unknown-family-20251001": "gpt-5.2",
		},
	})

	require.Equal(t, "gpt-5.4", cfg.OpusMappedModel)
	require.Equal(t, "gpt-5.3-codex", cfg.SonnetMappedModel)
	require.Equal(t, "gpt-5.4-mini", cfg.HaikuMappedModel)
	require.Equal(t, map[string]string{
		"claude-opus-4-6":                "gpt-5.4",
		"claude-sonnet-4-5-20250929":     "gpt-5.4-mini",
		"claude-3-5-haiku-20241022":      "gpt-5.4-mini",
		"claude-3-7-sonnet-20250219":     "gpt-5.3-codex",
		"claude-opus-4-6-20251102":       "gpt-5.4",
		"claude-unknown-family-20251001": "gpt-5.2",
	}, cfg.ExactModelMappings)
}

func TestGroupResolveMessagesDispatchModel(t *testing.T) {
	t.Run("exact override wins over family mapping", func(t *testing.T) {
		group := &Group{
			MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
				SonnetMappedModel: "gpt-5.2",
				ExactModelMappings: map[string]string{
					"claude-sonnet-4-5-20250929": "gpt-5.4-mini-high",
				},
			},
		}

		require.Equal(t, "gpt-5.4-mini", group.ResolveMessagesDispatchModel("claude-sonnet-4-5-20250929"))
	})

	t.Run("uses configured family mapping", func(t *testing.T) {
		group := &Group{
			MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
				OpusMappedModel:  "gpt-5.2",
				HaikuMappedModel: "gpt-5.4-mini",
			},
		}

		require.Equal(t, "gpt-5.2", group.ResolveMessagesDispatchModel("claude-opus-4-6"))
		require.Equal(t, "gpt-5.4-mini", group.ResolveMessagesDispatchModel("claude-haiku-4-5-20251001"))
	})

	t.Run("falls back to built-in family defaults", func(t *testing.T) {
		group := &Group{}

		require.Equal(t, "gpt-5.4", group.ResolveMessagesDispatchModel("claude-opus-4-6"))
		require.Equal(t, "gpt-5.3-codex", group.ResolveMessagesDispatchModel("claude-sonnet-4-5-20250929"))
		require.Equal(t, "gpt-5.4-mini", group.ResolveMessagesDispatchModel("claude-haiku-4-5-20251001"))
	})

	t.Run("returns empty for non claude models", func(t *testing.T) {
		group := &Group{}

		require.Empty(t, group.ResolveMessagesDispatchModel("gpt-5.4"))
		require.Empty(t, group.ResolveMessagesDispatchModel(""))
		require.Empty(t, (*Group)(nil).ResolveMessagesDispatchModel("claude-sonnet-4-5-20250929"))
	})
}

func TestSanitizeGroupMessagesDispatchFields(t *testing.T) {
	group := &Group{
		Platform:              PlatformAnthropic,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.4",
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel: "gpt-5.2",
		},
	}

	sanitizeGroupMessagesDispatchFields(group)

	require.False(t, group.AllowMessagesDispatch)
	require.Empty(t, group.DefaultMappedModel)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{}, group.MessagesDispatchModelConfig)
}
