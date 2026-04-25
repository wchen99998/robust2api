package service

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

var sensitiveQueryParamRegex = regexp.MustCompile(`(?i)([?&](?:key|client_secret|access_token|refresh_token)=)[^&"\s]+`)

func truncateForLog(b []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 2048
	}
	if len(b) > maxBytes {
		b = b[:maxBytes]
	}
	s := string(b)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

func sanitizeUpstreamErrorMessage(msg string) string {
	if msg == "" {
		return msg
	}
	return sensitiveQueryParamRegex.ReplaceAllString(msg, `$1***`)
}

func ExtractUpstreamErrorMessage(body []byte) string {
	return extractUpstreamErrorMessage(body)
}

func extractUpstreamErrorMessage(body []byte) string {
	if m := gjson.GetBytes(body, "error.message").String(); strings.TrimSpace(m) != "" {
		inner := strings.TrimSpace(m)
		if strings.HasPrefix(inner, "{") {
			if innerMsg := gjson.Get(inner, "error.message").String(); strings.TrimSpace(innerMsg) != "" {
				return innerMsg
			}
		}
		return m
	}
	if d := gjson.GetBytes(body, "detail").String(); strings.TrimSpace(d) != "" {
		return d
	}
	return gjson.GetBytes(body, "message").String()
}

func extractUpstreamErrorCode(body []byte) string {
	if code := strings.TrimSpace(gjson.GetBytes(body, "error.code").String()); code != "" {
		return code
	}

	inner := strings.TrimSpace(gjson.GetBytes(body, "error.message").String())
	if !strings.HasPrefix(inner, "{") {
		return ""
	}
	if code := strings.TrimSpace(gjson.Get(inner, "error.code").String()); code != "" {
		return code
	}
	if lastBrace := strings.LastIndex(inner, "}"); lastBrace >= 0 {
		if code := strings.TrimSpace(gjson.Get(inner[:lastBrace+1], "error.code").String()); code != "" {
			return code
		}
	}
	return ""
}

func extractUpstreamErrorType(body []byte) string {
	if errType := strings.TrimSpace(gjson.GetBytes(body, "error.type").String()); errType != "" {
		return errType
	}
	inner := strings.TrimSpace(gjson.GetBytes(body, "error.message").String())
	if strings.HasPrefix(inner, "{") {
		return strings.TrimSpace(gjson.Get(inner, "error.type").String())
	}
	return ""
}

func hashSensitiveValueForLog(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
