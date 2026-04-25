package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const defaultPreviewBytes = 256

func SafeHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		out[http.CanonicalHeaderKey(key)] = SafeHeaderValue(key, strings.Join(values, ","))
	}
	return out
}

func SafeHeaderValue(key, value string) string {
	if SensitiveHeader(key) {
		value = strings.TrimSpace(value)
		if strings.HasPrefix(strings.ToLower(value), "bearer ") {
			return "Bearer [redacted]"
		}
		if value == "" {
			return ""
		}
		return "[redacted]"
	}
	return strings.TrimSpace(value)
}

func SensitiveHeader(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "authorization",
		"proxy-authorization",
		"cookie",
		"set-cookie",
		"x-api-key",
		"x-goog-api-key",
		"x-auth-token",
		"x-csrf-token",
		"x-xsrf-token",
		"apikey",
		"api-key":
		return true
	}
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "session") ||
		strings.HasSuffix(key, "-api-key")
}

func BodyFingerprint(body []byte) string {
	if len(body) == 0 {
		return "sha256:e3b0c44298fc len=0"
	}
	sum := sha256.Sum256(body)
	return fmt.Sprintf("sha256:%s len=%d", hex.EncodeToString(sum[:8]), len(body))
}

func HeaderFingerprint(headers http.Header) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, strings.ToLower(key))
	}
	sort.Strings(keys)
	if len(keys) > defaultPreviewBytes {
		keys = keys[:defaultPreviewBytes]
	}
	return strings.Join(keys, ",")
}
