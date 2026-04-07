package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/health"

	"github.com/gin-gonic/gin"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r *gin.Engine, healthChecker *health.Checker) {
	// Health probe endpoints (Kubernetes liveness/readiness/startup)
	r.GET("/livez", gin.WrapF(healthChecker.Livez))
	r.GET("/readyz", gin.WrapF(healthChecker.Readyz))
	r.GET("/startupz", gin.WrapF(healthChecker.Startupz))

	// Backward-compatible liveness alias — Caddy and the frontend VersionBadge poll
	// /health and expect an unconditional 200 even during transient DB/Redis issues.
	r.GET("/health", gin.WrapF(healthChecker.Livez))

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}
