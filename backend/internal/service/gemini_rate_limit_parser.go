package service

import (
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

var retryInRegex = regexp.MustCompile(`Please retry in ([0-9.]+)s`)

func ParseGeminiRateLimitResetTime(body []byte) *int64 {
	errMsg := gjson.GetBytes(body, "error.message").String()
	if looksLikeGeminiDailyQuota(errMsg) {
		if ts := nextGeminiDailyResetUnix(); ts != nil {
			return ts
		}
	}

	var found *int64
	gjson.GetBytes(body, "error.details").ForEach(func(_, detail gjson.Result) bool {
		v := detail.Get("metadata.quotaResetDelay").String()
		if v == "" {
			return true
		}
		if dur, err := time.ParseDuration(v); err == nil {
			ts := time.Now().Unix() + int64(math.Ceil(dur.Seconds()))
			found = &ts
			return false
		}
		return true
	})
	if found != nil {
		return found
	}

	matches := retryInRegex.FindStringSubmatch(string(body))
	if len(matches) == 2 {
		if dur, err := time.ParseDuration(matches[1] + "s"); err == nil {
			ts := time.Now().Unix() + int64(math.Ceil(dur.Seconds()))
			return &ts
		}
	}

	return nil
}

func looksLikeGeminiDailyQuota(message string) bool {
	m := strings.ToLower(message)
	return strings.Contains(m, "per day") ||
		strings.Contains(m, "requests per day") ||
		(strings.Contains(m, "quota") && strings.Contains(m, "per day"))
}

func nextGeminiDailyResetUnix() *int64 {
	reset := geminiDailyResetTime(time.Now())
	ts := reset.Unix()
	return &ts
}
