//go:build unit

package server

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/health"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRoleRoutersExposeOnlyOwnedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Server.Mode = "release"
	cfg.Server.TrustedProxies = []string{}

	healthChecker := health.NewChecker(nil, nil)

	gatewayRouter := gin.New()
	SetupGatewayRouter(
		gatewayRouter,
		&handler.GatewayHandlers{},
		middleware2.APIKeyAuthMiddleware(func(c *gin.Context) { c.Next() }),
		nil,
		nil,
		nil,
		cfg,
		healthChecker,
	)

	controlRouter := gin.New()
	SetupControlRouter(
		controlRouter,
		&handler.ControlHandlers{Admin: &handler.AdminHandlers{}},
		middleware2.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		middleware2.AdminAuthMiddleware(func(c *gin.Context) { c.Next() }),
		nil,
		service.BuildInfo{},
		cfg,
		nil,
		healthChecker,
	)

	gatewayRoutes := routeIndex(gatewayRouter)
	controlRoutes := routeIndex(controlRouter)

	require.Contains(t, gatewayRoutes, "POST /v1/messages")
	require.Contains(t, gatewayRoutes, "GET /health")
	require.NotContains(t, gatewayRoutes, "GET /api/v1/bootstrap")
	require.NotContains(t, gatewayRoutes, "GET /api/v1/admin/dashboard/models")

	require.Contains(t, controlRoutes, "GET /api/v1/bootstrap")
	require.Contains(t, controlRoutes, "POST /api/v1/session/login")
	require.Contains(t, controlRoutes, "GET /api/v1/admin/dashboard/models")
	require.Contains(t, controlRoutes, "GET /health")
	require.NotContains(t, controlRoutes, "POST /v1/messages")
	require.NotContains(t, controlRoutes, "POST /responses")
}

func routeIndex(r *gin.Engine) map[string]struct{} {
	out := make(map[string]struct{}, len(r.Routes()))
	for _, route := range r.Routes() {
		out[route.Method+" "+route.Path] = struct{}{}
	}
	return out
}
