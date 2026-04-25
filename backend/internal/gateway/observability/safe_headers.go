package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
)

var secretHeaderNames = map[string]struct{}{
	"authorization":           {},
	"cookie":                  {},
	"set-cookie":              {},
	"x-api-key":               {},
	"api-key":                 {},
	"proxy-authorization":     {},
	"openai-organization":     {},
	"openai-project":          {},
	"x-goog-api-key":          {},
	"x-goog-authuser":         {},
	"x-codex-auth":            {},
	"x-stainless-api-key":     {},
	"anthropic-api-key":       {},
	"cf-access-client-secret": {},
}

// SafeHeaders returns a deterministic, redacted copy of headers suitable for
// diagnostic logs and RoutingPlan fixtures.
func SafeHeaders(headers http.Header) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string][]string, len(headers))
	for key, values := range headers {
		canonical := http.CanonicalHeaderKey(key)
		if _, secret := secretHeaderNames[strings.ToLower(key)]; secret {
			out[canonical] = []string{"[redacted]"}
			continue
		}
		copied := append([]string(nil), values...)
		sort.Strings(copied)
		out[canonical] = copied
	}
	return out
}

// BodyFingerprint is a bounded diagnostic digest for payload logging.
func BodyFingerprint(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
