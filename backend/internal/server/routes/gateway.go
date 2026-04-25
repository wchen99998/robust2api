package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterGatewayRoutes 注册 API 网关路由（Claude/OpenAI/Gemini 兼容）
func RegisterGatewayRoutes(
	r *gin.Engine,
	h *handler.GatewayHandlers,
	apiKeyAuth middleware.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	settingService *service.SettingService,
	cfg *config.Config,
) {
	bodyLimit := middleware.RequestBodyLimit(cfg.Gateway.MaxBodySize)
	clientRequestID := middleware.ClientRequestID()
	endpointNorm := handler.InboundEndpointMiddleware()

	// 未分组 Key 拦截中间件（按协议格式区分错误响应）
	requireGroupAnthropic := middleware.RequireGroupAssignment(settingService, middleware.AnthropicErrorWriter)
	requireGroupGoogle := middleware.RequireGroupAssignment(settingService, middleware.GoogleErrorWriter)

	// API网关（Claude API兼容）
	gateway := r.Group("/v1")
	gateway.Use(bodyLimit)
	gateway.Use(clientRequestID)
	gateway.Use(endpointNorm)
	gateway.Use(gin.HandlerFunc(apiKeyAuth))
	gateway.Use(requireGroupAnthropic)
	{
		// /v1/messages: auto-route based on group platform
		gateway.POST("/messages", h.CoreGateway.Handle)
		gateway.POST("/messages/count_tokens", h.CoreGateway.Handle)
		gateway.GET("/models", h.CoreGateway.Handle)
		gateway.GET("/usage", h.CoreGateway.Handle)
		// OpenAI Responses API: auto-route based on group platform
		gateway.POST("/responses", h.CoreGateway.Handle)
		gateway.POST("/responses/*subpath", h.CoreGateway.Handle)
		gateway.GET("/responses", h.CoreGateway.HandleWebSocket)
		// OpenAI Chat Completions API: auto-route based on group platform
		gateway.POST("/chat/completions", h.CoreGateway.Handle)
	}

	// Gemini 原生 API 兼容层（Gemini SDK/CLI 直连）
	gemini := r.Group("/v1beta")
	gemini.Use(bodyLimit)
	gemini.Use(clientRequestID)
	gemini.Use(endpointNorm)
	gemini.Use(middleware.APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, cfg))
	gemini.Use(requireGroupGoogle)
	{
		gemini.GET("/models", h.CoreGateway.Handle)
		gemini.GET("/models/:model", h.CoreGateway.Handle)
		// Gin treats ":" as a param marker, but Gemini uses "{model}:{action}" in the same segment.
		gemini.POST("/models/*modelAction", h.CoreGateway.Handle)
	}

	// OpenAI Responses API（不带v1前缀的别名）— auto-route based on group platform
	responsesHandler := h.CoreGateway.Handle
	r.POST("/responses", bodyLimit, clientRequestID, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, responsesHandler)
	r.POST("/responses/*subpath", bodyLimit, clientRequestID, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, responsesHandler)
	r.GET("/responses", bodyLimit, clientRequestID, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, h.CoreGateway.HandleWebSocket)
	// OpenAI Chat Completions API（不带v1前缀的别名）— auto-route based on group platform
	r.POST("/chat/completions", bodyLimit, clientRequestID, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, h.CoreGateway.Handle)

	openaiV1 := r.Group("/openai/v1")
	openaiV1.Use(bodyLimit)
	openaiV1.Use(clientRequestID)
	openaiV1.Use(endpointNorm)
	openaiV1.Use(gin.HandlerFunc(apiKeyAuth))
	openaiV1.Use(requireGroupAnthropic)
	{
		openaiV1.POST("/responses", h.CoreGateway.Handle)
		openaiV1.POST("/responses/*subpath", h.CoreGateway.Handle)
		openaiV1.GET("/responses", h.CoreGateway.HandleWebSocket)
		openaiV1.POST("/chat/completions", h.CoreGateway.Handle)
	}

	// Antigravity 模型列表
	r.GET("/antigravity/models", gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, h.CoreGateway.Handle)

	// Antigravity 专用路由（仅使用 antigravity 账户，不混合调度）
	antigravityV1 := r.Group("/antigravity/v1")
	antigravityV1.Use(bodyLimit)
	antigravityV1.Use(clientRequestID)
	antigravityV1.Use(endpointNorm)
	antigravityV1.Use(middleware.ForcePlatform(service.PlatformAntigravity))
	antigravityV1.Use(gin.HandlerFunc(apiKeyAuth))
	antigravityV1.Use(requireGroupAnthropic)
	{
		antigravityV1.POST("/messages", h.CoreGateway.Handle)
		antigravityV1.POST("/messages/count_tokens", h.CoreGateway.Handle)
		antigravityV1.GET("/models", h.CoreGateway.Handle)
		antigravityV1.GET("/usage", h.CoreGateway.Handle)
	}

	antigravityV1Beta := r.Group("/antigravity/v1beta")
	antigravityV1Beta.Use(bodyLimit)
	antigravityV1Beta.Use(clientRequestID)
	antigravityV1Beta.Use(endpointNorm)
	antigravityV1Beta.Use(middleware.ForcePlatform(service.PlatformAntigravity))
	antigravityV1Beta.Use(middleware.APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, cfg))
	antigravityV1Beta.Use(requireGroupGoogle)
	{
		antigravityV1Beta.GET("/models", h.CoreGateway.Handle)
		antigravityV1Beta.GET("/models/:model", h.CoreGateway.Handle)
		antigravityV1Beta.POST("/models/*modelAction", h.CoreGateway.Handle)
	}

}
