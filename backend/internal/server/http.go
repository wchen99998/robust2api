// Package server provides HTTP server initialization and configuration.
package server

import (
	"log"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/health"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// GatewayProviderSet provides server-layer dependencies for the gateway binary.
var GatewayProviderSet = wire.NewSet(
	ProvideGatewayRouter,
	ProvideHTTPServer,
)

// ControlProviderSet provides server-layer dependencies for the control binary.
var ControlProviderSet = wire.NewSet(
	ProvideControlRouter,
	ProvideHTTPServer,
)

// ProviderSet 提供服务器层的依赖
var ProviderSet = wire.NewSet(
	ProvideRouter,
	ProvideHTTPServer,
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
	r := newEngine(cfg)
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
	r := newEngine(cfg)
	return SetupControlRouter(r, handlers, jwtAuth, adminAuth, settingService, buildInfo, cfg, redisClient, healthChecker)
}

// ProvideRouter 提供路由器
func ProvideRouter(
	cfg *config.Config,
	handlers *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	settingService *service.SettingService,
	buildInfo service.BuildInfo,
	redisClient *redis.Client,
	healthChecker *health.Checker,
) *gin.Engine {
	r := newEngine(cfg)
	SetupControlRouter(r, &handler.ControlHandlers{
		Auth:         handlers.Auth,
		User:         handlers.User,
		APIKey:       handlers.APIKey,
		Usage:        handlers.Usage,
		Redeem:       handlers.Redeem,
		Subscription: handlers.Subscription,
		Announcement: handlers.Announcement,
		Admin:        handlers.Admin,
		Setting:      handlers.Setting,
		Totp:         handlers.Totp,
	}, jwtAuth, adminAuth, settingService, buildInfo, cfg, redisClient, healthChecker)
	routes.RegisterGatewayCompatRoutes(r)
	routes.RegisterGatewayRoutes(r, handlers, apiKeyAuth, apiKeyService, subscriptionService, settingService, cfg)
	return r
}

func newEngine(cfg *config.Config) *gin.Engine {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware2.Recovery())
	if len(cfg.Server.TrustedProxies) > 0 {
		if err := r.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
			log.Printf("Failed to set trusted proxies: %v", err)
		}
	} else {
		if err := r.SetTrustedProxies(nil); err != nil {
			log.Printf("Failed to disable trusted proxies: %v", err)
		}
		if cfg.Server.Mode == "release" {
			log.Printf("Warning: server.trusted_proxies is empty in release mode; client IP trust chain is disabled")
		}
	}
	return r
}

// ProvideHTTPServer 提供 HTTP 服务器
func ProvideHTTPServer(cfg *config.Config, router *gin.Engine) *http.Server {
	httpHandler := http.Handler(router)

	globalMaxSize := cfg.Server.MaxRequestBodySize
	if globalMaxSize <= 0 && cfg.Gateway.MaxBodySize > 0 {
		globalMaxSize = cfg.Gateway.MaxBodySize
	}
	if globalMaxSize > 0 {
		httpHandler = http.MaxBytesHandler(httpHandler, globalMaxSize)
		log.Printf("Global max request body size: %d bytes (%.2f MB)", globalMaxSize, float64(globalMaxSize)/(1<<20))
	}

	// 根据配置决定是否启用 H2C
	if cfg.Server.H2C.Enabled {
		h2cConfig := cfg.Server.H2C
		httpHandler = h2c.NewHandler(router, &http2.Server{
			MaxConcurrentStreams:         h2cConfig.MaxConcurrentStreams,
			IdleTimeout:                  time.Duration(h2cConfig.IdleTimeout) * time.Second,
			MaxReadFrameSize:             uint32(h2cConfig.MaxReadFrameSize),
			MaxUploadBufferPerConnection: int32(h2cConfig.MaxUploadBufferPerConnection),
			MaxUploadBufferPerStream:     int32(h2cConfig.MaxUploadBufferPerStream),
		})
		log.Printf("HTTP/2 Cleartext (h2c) enabled: max_concurrent_streams=%d, idle_timeout=%ds, max_read_frame_size=%d, max_upload_buffer_per_connection=%d, max_upload_buffer_per_stream=%d",
			h2cConfig.MaxConcurrentStreams,
			h2cConfig.IdleTimeout,
			h2cConfig.MaxReadFrameSize,
			h2cConfig.MaxUploadBufferPerConnection,
			h2cConfig.MaxUploadBufferPerStream,
		)
	}

	return &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: httpHandler,
		// ReadHeaderTimeout: 读取请求头的超时时间，防止慢速请求头攻击
		ReadHeaderTimeout: time.Duration(cfg.Server.ReadHeaderTimeout) * time.Second,
		// IdleTimeout: 空闲连接超时时间，释放不活跃的连接资源
		IdleTimeout: time.Duration(cfg.Server.IdleTimeout) * time.Second,
		// 注意：不设置 WriteTimeout，因为流式响应可能持续十几分钟
		// 不设置 ReadTimeout，因为大请求体可能需要较长时间读取
	}
}
