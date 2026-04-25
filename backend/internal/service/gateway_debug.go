package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func (s *GatewayService) debugModelRoutingEnabled() bool {
	return s.debugModelRouting.Load()
}

func (s *GatewayService) debugClaudeMimicEnabled() bool {
	return s.debugClaudeMimic.Load()
}

func parseDebugEnvBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func shortSessionHash(sessionHash string) string {
	if sessionHash == "" {
		return ""
	}
	if len(sessionHash) <= 8 {
		return sessionHash
	}
	return sessionHash[:8]
}

func redactAuthHeaderValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// Keep scheme for debugging, redact secret.
	if strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return "Bearer [redacted]"
	}
	return "[redacted]"
}

func safeHeaderValueForLog(key string, v string) string {
	if isSensitiveHeaderForLog(key) {
		return redactAuthHeaderValue(v)
	}
	return strings.TrimSpace(v)
}

func isSensitiveHeaderForLog(key string) bool {
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

func extractSystemPreviewFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sys := gjson.GetBytes(body, "system")
	if !sys.Exists() {
		return ""
	}

	switch {
	case sys.IsArray():
		for _, item := range sys.Array() {
			if !item.IsObject() {
				continue
			}
			if strings.EqualFold(item.Get("type").String(), "text") {
				if t := item.Get("text").String(); strings.TrimSpace(t) != "" {
					return t
				}
			}
		}
		return ""
	case sys.Type == gjson.String:
		return sys.String()
	default:
		return ""
	}
}

func buildClaudeMimicDebugLine(req *http.Request, body []byte, account *Account, tokenType string, mimicClaudeCode bool) string {
	if req == nil {
		return ""
	}

	// Only log a minimal fingerprint to avoid leaking user content.
	interesting := []string{
		"user-agent",
		"x-app",
		"anthropic-dangerous-direct-browser-access",
		"anthropic-version",
		"anthropic-beta",
		"x-stainless-lang",
		"x-stainless-package-version",
		"x-stainless-os",
		"x-stainless-arch",
		"x-stainless-runtime",
		"x-stainless-runtime-version",
		"x-stainless-retry-count",
		"x-stainless-timeout",
		"authorization",
		"x-api-key",
		"content-type",
		"accept",
		"x-stainless-helper-method",
	}

	h := make([]string, 0, len(interesting))
	for _, k := range interesting {
		if v := req.Header.Get(k); v != "" {
			h = append(h, fmt.Sprintf("%s=%q", k, safeHeaderValueForLog(k, v)))
		}
	}

	metaUserID := strings.TrimSpace(gjson.GetBytes(body, "metadata.user_id").String())
	sysPreview := strings.TrimSpace(extractSystemPreviewFromBody(body))

	// Truncate preview to keep logs sane.
	if len(sysPreview) > 300 {
		sysPreview = sysPreview[:300] + "..."
	}
	sysPreview = strings.ReplaceAll(sysPreview, "\n", "\\n")
	sysPreview = strings.ReplaceAll(sysPreview, "\r", "\\r")

	aid := int64(0)
	aname := ""
	if account != nil {
		aid = account.ID
		aname = account.Name
	}

	return fmt.Sprintf(
		"url=%s account=%d(%s) tokenType=%s mimic=%t meta.user_id=%q system.preview=%q headers={%s}",
		req.URL.String(),
		aid,
		aname,
		tokenType,
		mimicClaudeCode,
		metaUserID,
		sysPreview,
		strings.Join(h, " "),
	)
}

func logClaudeMimicDebug(req *http.Request, body []byte, account *Account, tokenType string, mimicClaudeCode bool) {
	line := buildClaudeMimicDebugLine(req, body, account, tokenType, mimicClaudeCode)
	if line == "" {
		return
	}
	logger.LegacyPrintf("service.gateway", "[ClaudeMimicDebug] %s", line)
}

func isClaudeCodeCredentialScopeError(msg string) bool {
	m := strings.ToLower(strings.TrimSpace(msg))
	if m == "" {
		return false
	}
	return strings.Contains(m, "only authorized for use with claude code") &&
		strings.Contains(m, "cannot be used for other api requests")
}
