// Package server provides HTTP server initialization and configuration.
package server

import (
	"github.com/wchen99998/robust2api/internal/config"
	"github.com/wchen99998/robust2api/internal/handler"
	"github.com/wchen99998/robust2api/internal/health"
	platformhttp "github.com/wchen99998/robust2api/internal/platform/httpserver"
	middleware2 "github.com/wchen99998/robust2api/internal/server/middleware"
	"github.com/wchen99998/robust2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

// GatewayProviderSet provides server-layer dependencies for the gateway binary.
var GatewayProviderSet = wire.NewSet(
	ProvideGatewayRouter,
)

// ControlProviderSet provides server-layer dependencies for the control binary.
var ControlProviderSet = wire.NewSet(
	ProvideControlRouter,
)

// ProvideGatewayRouter provides the gateway-only router.
func ProvideGatewayRouter(
	cfg *config.Config,
	handlers *handler.GatewayHandlers,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	settingService *service.SettingService,
	healthChecker *health.Checker,
) *gin.Engine {
	r := platformhttp.NewEngine(cfg)
	return SetupGatewayRouter(r, handlers, apiKeyAuth, apiKeyService, subscriptionService, settingService, cfg, healthChecker)
}

// ProvideControlRouter provides the control-plane router.
func ProvideControlRouter(
	cfg *config.Config,
	handlers *handler.ControlHandlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	settingService *service.SettingService,
	buildInfo service.BuildInfo,
	redisClient *redis.Client,
	healthChecker *health.Checker,
) *gin.Engine {
	r := platformhttp.NewEngine(cfg)
	return SetupControlRouter(r, handlers, jwtAuth, adminAuth, settingService, buildInfo, cfg, redisClient, healthChecker)
}
