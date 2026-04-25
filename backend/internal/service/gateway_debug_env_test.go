package service

import "testing"

func TestParseDebugEnvBool(t *testing.T) {
	t.Run("empty is false", func(t *testing.T) {
		if parseDebugEnvBool("") {
			t.Fatalf("expected false for empty string")
		}
	})

	t.Run("true-like values", func(t *testing.T) {
		for _, value := range []string{"1", "true", "TRUE", "yes", "on"} {
			t.Run(value, func(t *testing.T) {
				if !parseDebugEnvBool(value) {
					t.Fatalf("expected true for %q", value)
				}
			})
		}
	})

	t.Run("false-like values", func(t *testing.T) {
		for _, value := range []string{"0", "false", "off", "debug"} {
			t.Run(value, func(t *testing.T) {
				if parseDebugEnvBool(value) {
					t.Fatalf("expected false for %q", value)
				}
			})
		}
	})
}

func TestSafeHeaderValueForLogRedactsSensitiveHeaders(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		value  string
		expect string
	}{
		{name: "authorization bearer", key: "Authorization", value: "Bearer abc123", expect: "Bearer [redacted]"},
		{name: "api key", key: "X-API-Key", value: "sk-test", expect: "[redacted]"},
		{name: "google api key", key: "X-Goog-Api-Key", value: "AIza-secret", expect: "[redacted]"},
		{name: "cookie", key: "Cookie", value: "session=secret", expect: "[redacted]"},
		{name: "proxy authorization", key: "Proxy-Authorization", value: "Basic abc", expect: "[redacted]"},
		{name: "token substring", key: "X-Access-Token", value: "token-secret", expect: "[redacted]"},
		{name: "session substring", key: "X-Claude-Code-Session-Id", value: "session-id", expect: "[redacted]"},
		{name: "safe header", key: "Content-Type", value: " application/json ", expect: "application/json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeHeaderValueForLog(tt.key, tt.value); got != tt.expect {
				t.Fatalf("safeHeaderValueForLog(%q, %q) = %q, want %q", tt.key, tt.value, got, tt.expect)
			}
		})
	}
}
