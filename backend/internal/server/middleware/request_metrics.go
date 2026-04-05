package middleware

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	"github.com/gin-gonic/gin"
)

// RequestMetrics records gateway request count and latency once per routed request.
func RequestMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			return
		}

		platform := requestMetricsPlatform(c)
		if platform == "" {
			return
		}

		ctx := c.Request.Context()
		status := c.Writer.Status()
		appelotel.M().RecordRequest(ctx, c.Request.Method, route, status, platform)
		appelotel.M().RecordDuration(ctx, time.Since(start).Seconds(), c.Request.Method, route, status, platform)
	}
}

func requestMetricsPlatform(c *gin.Context) string {
	if c == nil {
		return ""
	}

	if c.Request != nil {
		if platform, _ := c.Request.Context().Value(ctxkey.Platform).(string); strings.TrimSpace(platform) != "" {
			return strings.TrimSpace(platform)
		}
		if platform, _ := c.Request.Context().Value(ctxkey.ForcePlatform).(string); strings.TrimSpace(platform) != "" {
			return strings.TrimSpace(platform)
		}
	}

	if platform, ok := c.Get("platform"); ok {
		if s, ok := platform.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}

	if platform, ok := GetForcePlatformFromContext(c); ok && strings.TrimSpace(platform) != "" {
		return strings.TrimSpace(platform)
	}

	if apiKey, ok := GetAPIKeyFromContext(c); ok && apiKey != nil && apiKey.Group != nil {
		return strings.TrimSpace(apiKey.Group.Platform)
	}

	return ""
}
