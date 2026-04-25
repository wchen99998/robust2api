package observability

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeHeadersRedactsSecretHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization":     {"Bearer secret"},
		"Cookie":            {"session=secret"},
		"Anthropic-Api-Key": {"secret"},
		"User-Agent":        {"b", "a"},
	}

	got := SafeHeaders(headers)

	require.Equal(t, []string{"[redacted]"}, got["Authorization"])
	require.Equal(t, []string{"[redacted]"}, got["Cookie"])
	require.Equal(t, []string{"[redacted]"}, got["Anthropic-Api-Key"])
	require.Equal(t, []string{"a", "b"}, got["User-Agent"])
}

func TestBodyFingerprint(t *testing.T) {
	require.Empty(t, BodyFingerprint(nil))
	require.Equal(t, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", BodyFingerprint([]byte("hello")))
}
