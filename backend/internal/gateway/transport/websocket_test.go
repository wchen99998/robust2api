package transport

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestURLWithWebSocketScheme(t *testing.T) {
	got, err := URLWithWebSocketScheme("https://api.openai.com/v1/responses")
	require.NoError(t, err)
	require.Equal(t, "wss://api.openai.com/v1/responses", got)

	got, err = URLWithWebSocketScheme("http://localhost:8080/v1/responses")
	require.NoError(t, err)
	require.Equal(t, "ws://localhost:8080/v1/responses", got)

	got, err = URLWithWebSocketScheme("wss://example.test/ws")
	require.NoError(t, err)
	require.Equal(t, "wss://example.test/ws", got)
}

func TestURLWithWebSocketSchemeRejectsUnsupportedScheme(t *testing.T) {
	_, err := URLWithWebSocketScheme("ftp://example.test/ws")
	require.Error(t, err)
}
