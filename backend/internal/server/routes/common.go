package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/health"

	"github.com/gin-gonic/gin"
)

// RegisterHealthRoutes registers shared probe endpoints for a role.
func RegisterHealthRoutes(r gin.IRoutes, healthChecker *health.Checker) {
	r.GET("/livez", gin.WrapF(healthChecker.Livez))
	r.GET("/readyz", gin.WrapF(healthChecker.Readyz))
	r.GET("/startupz", gin.WrapF(healthChecker.Startupz))
	r.GET("/health", gin.WrapF(healthChecker.Readyz))
}

// RegisterGatewayCompatRoutes registers gateway-only compatibility endpoints.
func RegisterGatewayCompatRoutes(r gin.IRoutes) {
	// Claude Code telemetry is intentionally accepted and ignored.
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}
