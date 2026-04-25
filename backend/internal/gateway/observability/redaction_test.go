package observability

import (
	"net/http"
	"strings"
	"testing"
)

func TestSafeHeadersRedactsSecrets(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer secret")
	headers.Set("Cookie", "sid=secret")
	headers.Set("X-Goog-Api-Key", "AIza-secret")
	headers.Set("Content-Type", " application/json ")

	got := SafeHeaders(headers)
	if got["Authorization"] != "Bearer [redacted]" {
		t.Fatalf("authorization was not redacted: %q", got["Authorization"])
	}
	if got["Cookie"] != "[redacted]" {
		t.Fatalf("cookie was not redacted: %q", got["Cookie"])
	}
	if got["X-Goog-Api-Key"] != "[redacted]" {
		t.Fatalf("google api key was not redacted: %q", got["X-Goog-Api-Key"])
	}
	if got["Content-Type"] != "application/json" {
		t.Fatalf("content type = %q", got["Content-Type"])
	}
}

func TestBodyFingerprintIsBounded(t *testing.T) {
	got := BodyFingerprint([]byte(strings.Repeat("x", 4096)))
	if strings.Contains(got, strings.Repeat("x", 32)) {
		t.Fatalf("fingerprint leaked body content: %q", got)
	}
	if !strings.Contains(got, "len=4096") {
		t.Fatalf("fingerprint missing length: %q", got)
	}
}
