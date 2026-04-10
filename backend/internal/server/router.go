package server

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/health"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// SetupGatewayRouter configures the gateway-only HTTP surface.
func SetupGatewayRouter(
	r *gin.Engine,
	handlers *handler.GatewayHandlers,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	settingService *service.SettingService,
	cfg *config.Config,
	healthChecker *health.Checker,
) *gin.Engine {
	applySharedRouterMiddleware(r, cfg)

	routes.RegisterHealthRoutes(r, healthChecker)
	routes.RegisterGatewayCompatRoutes(r)
	routes.RegisterGatewayRoutes(r, handlers, apiKeyAuth, apiKeyService, subscriptionService, settingService, cfg)

	return r
}

// SetupControlRouter configures the control-plane HTTP surface.
func SetupControlRouter(
	r *gin.Engine,
	handlers *handler.ControlHandlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	settingService *service.SettingService,
	buildInfo service.BuildInfo,
	cfg *config.Config,
	redisClient *redis.Client,
	healthChecker *health.Checker,
) *gin.Engine {
	applySharedRouterMiddleware(r, cfg)
	r.Use(middleware2.CORS(cfg.CORS))
	r.Use(middleware2.SecurityHeaders(cfg.Security.CSP, nil))

	routes.RegisterHealthRoutes(r, healthChecker)

	v1 := r.Group("/api/v1")
	routes.RegisterAuthRoutes(v1, handlers, jwtAuth, redisClient, settingService)
	routes.RegisterUserRoutes(v1, handlers, jwtAuth, settingService)
	routes.RegisterAdminRoutes(v1, handlers, adminAuth, buildInfo)

	return r
}

func applySharedRouterMiddleware(r *gin.Engine, cfg *config.Config) {
	if cfg.Otel.Enabled {
		r.Use(otelgin.Middleware("sub2api",
			otelgin.WithFilter(func(r *http.Request) bool {
				p := r.URL.Path
				return p != "/livez" && p != "/readyz" && p != "/startupz" && p != "/health"
			}),
		))
	}
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	if cfg.Otel.Enabled {
		r.Use(middleware2.TraceIDHeader())
		r.Use(middleware2.RequestMetrics())
	}
}
