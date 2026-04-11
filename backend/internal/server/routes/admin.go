// Package routes provides HTTP route registration and handlers.
package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes 注册管理员路由
func RegisterAdminRoutes(
	v1 *gin.RouterGroup,
	h *handler.ControlHandlers,
	adminAuth middleware.AdminAuthMiddleware,
	buildInfo service.BuildInfo,
) {
	admin := v1.Group("/admin")
	admin.Use(gin.HandlerFunc(adminAuth))
	{
		// 仪表盘 (usage analytics used by the Usage page)
		registerDashboardRoutes(admin, h.Admin)

		// 用户管理
		registerUserManagementRoutes(admin, h.Admin)

		// 分组管理
		registerGroupRoutes(admin, h.Admin)

		// 账号管理
		registerAccountRoutes(admin, h.Admin)

		// 公告管理
		registerAnnouncementRoutes(admin, h.Admin)

		// OpenAI OAuth
		registerOpenAIOAuthRoutes(admin, h.Admin)

		// Gemini OAuth
		registerGeminiOAuthRoutes(admin, h.Admin)

		// Antigravity OAuth
		registerAntigravityOAuthRoutes(admin, h.Admin)

		// 代理管理
		registerProxyRoutes(admin, h.Admin)

		// 卡密管理
		registerRedeemCodeRoutes(admin, h.Admin)

		// 优惠码管理
		registerPromoCodeRoutes(admin, h.Admin)

		// 系统设置
		registerSettingsRoutes(admin, h.Admin)

		// 系统管理
		registerSystemRoutes(admin, buildInfo)

		// 订阅管理
		registerSubscriptionRoutes(admin, h.Admin)

		// 使用记录管理
		registerUsageRoutes(admin, h.Admin)

		// 用户属性管理
		registerUserAttributeRoutes(admin, h.Admin)

		// 错误透传规则管理
		registerErrorPassthroughRoutes(admin, h.Admin)

		// TLS 指纹模板管理
		registerTLSFingerprintProfileRoutes(admin, h.Admin)

		// API Key 管理
		registerAdminAPIKeyRoutes(admin, h.Admin)

		// 定时测试计划
		registerScheduledTestRoutes(admin, h.Admin)

		// 渠道管理
		registerChannelRoutes(admin, h.Admin)
	}
}

func registerAdminAPIKeyRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	apiKeys := admin.Group("/api-keys")
	{
		apiKeys.PUT("/:id", h.APIKey.UpdateGroup)
	}
}

func registerDashboardRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	dashboard := admin.Group("/dashboard")
	{
		dashboard.GET("/snapshot-v2", h.Dashboard.GetSnapshotV2)
		dashboard.GET("/models", h.Dashboard.GetModelStats)
		dashboard.GET("/groups", h.Dashboard.GetGroupStats)
	}
}

func registerUserManagementRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	users := admin.Group("/users")
	{
		users.GET("", h.User.List)
		users.GET("/:id", h.User.GetByID)
		users.POST("", h.User.Create)
		users.PUT("/:id", h.User.Update)
		users.DELETE("/:id", h.User.Delete)
		users.POST("/:id/balance", h.User.UpdateBalance)
		users.GET("/:id/api-keys", h.User.GetUserAPIKeys)
		users.GET("/:id/usage", h.User.GetUserUsage)
		users.GET("/:id/balance-history", h.User.GetBalanceHistory)
		users.POST("/:id/replace-group", h.User.ReplaceGroup)

		// User attribute values
		users.GET("/:id/attributes", h.UserAttribute.GetUserAttributes)
		users.PUT("/:id/attributes", h.UserAttribute.UpdateUserAttributes)
	}
}

func registerGroupRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	groups := admin.Group("/groups")
	{
		groups.GET("", h.Group.List)
		groups.GET("/all", h.Group.GetAll)
		groups.GET("/usage-summary", h.Group.GetUsageSummary)
		groups.GET("/capacity-summary", h.Group.GetCapacitySummary)
		groups.PUT("/sort-order", h.Group.UpdateSortOrder)
		groups.GET("/:id", h.Group.GetByID)
		groups.POST("", h.Group.Create)
		groups.PUT("/:id", h.Group.Update)
		groups.DELETE("/:id", h.Group.Delete)
		groups.GET("/:id/stats", h.Group.GetStats)
		groups.GET("/:id/rate-multipliers", h.Group.GetGroupRateMultipliers)
		groups.PUT("/:id/rate-multipliers", h.Group.BatchSetGroupRateMultipliers)
		groups.DELETE("/:id/rate-multipliers", h.Group.ClearGroupRateMultipliers)
		groups.GET("/:id/api-keys", h.Group.GetGroupAPIKeys)
	}
}

func registerAccountRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	accounts := admin.Group("/accounts")
	{
		accounts.GET("", h.Account.List)
		accounts.GET("/:id", h.Account.GetByID)
		accounts.POST("", h.Account.Create)
		accounts.POST("/check-mixed-channel", h.Account.CheckMixedChannel)
		accounts.POST("/sync/crs", h.Account.SyncFromCRS)
		accounts.POST("/sync/crs/preview", h.Account.PreviewFromCRS)
		accounts.PUT("/:id", h.Account.Update)
		accounts.DELETE("/:id", h.Account.Delete)
		accounts.POST("/:id/test", h.Account.Test)
		accounts.POST("/:id/recover-state", h.Account.RecoverState)
		accounts.POST("/:id/refresh", h.Account.Refresh)
		accounts.POST("/:id/set-privacy", h.Account.SetPrivacy)
		accounts.POST("/:id/refresh-tier", h.Account.RefreshTier)
		accounts.GET("/:id/stats", h.Account.GetStats)
		accounts.POST("/:id/clear-error", h.Account.ClearError)
		accounts.GET("/:id/usage", h.Account.GetUsage)
		accounts.GET("/:id/today-stats", h.Account.GetTodayStats)
		accounts.POST("/today-stats/batch", h.Account.GetBatchTodayStats)
		accounts.POST("/:id/clear-rate-limit", h.Account.ClearRateLimit)
		accounts.POST("/:id/reset-quota", h.Account.ResetQuota)
		accounts.GET("/:id/temp-unschedulable", h.Account.GetTempUnschedulable)
		accounts.DELETE("/:id/temp-unschedulable", h.Account.ClearTempUnschedulable)
		accounts.POST("/:id/schedulable", h.Account.SetSchedulable)
		accounts.GET("/:id/models", h.Account.GetAvailableModels)
		accounts.POST("/batch", h.Account.BatchCreate)
		accounts.GET("/data", h.Account.ExportData)
		accounts.POST("/data", h.Account.ImportData)
		accounts.POST("/batch-update-credentials", h.Account.BatchUpdateCredentials)
		accounts.POST("/batch-refresh-tier", h.Account.BatchRefreshTier)
		accounts.POST("/bulk-update", h.Account.BulkUpdate)
		accounts.POST("/batch-clear-error", h.Account.BatchClearError)
		accounts.POST("/batch-refresh", h.Account.BatchRefresh)

		// Antigravity 默认模型映射
		accounts.GET("/antigravity/default-model-mapping", h.Account.GetAntigravityDefaultModelMapping)

		// Claude OAuth routes
		accounts.POST("/generate-auth-url", h.OAuth.GenerateAuthURL)
		accounts.POST("/generate-setup-token-url", h.OAuth.GenerateSetupTokenURL)
		accounts.POST("/exchange-code", h.OAuth.ExchangeCode)
		accounts.POST("/exchange-setup-token-code", h.OAuth.ExchangeSetupTokenCode)
		accounts.POST("/cookie-auth", h.OAuth.CookieAuth)
		accounts.POST("/setup-token-cookie-auth", h.OAuth.SetupTokenCookieAuth)
	}
}

func registerAnnouncementRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	announcements := admin.Group("/announcements")
	{
		announcements.GET("", h.Announcement.List)
		announcements.POST("", h.Announcement.Create)
		announcements.GET("/:id", h.Announcement.GetByID)
		announcements.PUT("/:id", h.Announcement.Update)
		announcements.DELETE("/:id", h.Announcement.Delete)
		announcements.GET("/:id/read-status", h.Announcement.ListReadStatus)
	}
}

func registerOpenAIOAuthRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	openai := admin.Group("/openai")
	{
		openai.POST("/generate-auth-url", h.OpenAIOAuth.GenerateAuthURL)
		openai.POST("/exchange-code", h.OpenAIOAuth.ExchangeCode)
		openai.POST("/refresh-token", h.OpenAIOAuth.RefreshToken)
		openai.POST("/accounts/:id/refresh", h.OpenAIOAuth.RefreshAccountToken)
		openai.POST("/create-from-oauth", h.OpenAIOAuth.CreateAccountFromOAuth)
	}
}

func registerGeminiOAuthRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	gemini := admin.Group("/gemini")
	{
		gemini.POST("/oauth/auth-url", h.GeminiOAuth.GenerateAuthURL)
		gemini.POST("/oauth/exchange-code", h.GeminiOAuth.ExchangeCode)
		gemini.GET("/oauth/capabilities", h.GeminiOAuth.GetCapabilities)
	}
}

func registerAntigravityOAuthRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	antigravity := admin.Group("/antigravity")
	{
		antigravity.POST("/oauth/auth-url", h.AntigravityOAuth.GenerateAuthURL)
		antigravity.POST("/oauth/exchange-code", h.AntigravityOAuth.ExchangeCode)
		antigravity.POST("/oauth/refresh-token", h.AntigravityOAuth.RefreshToken)
	}
}

func registerProxyRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	proxies := admin.Group("/proxies")
	{
		proxies.GET("", h.Proxy.List)
		proxies.GET("/all", h.Proxy.GetAll)
		proxies.GET("/data", h.Proxy.ExportData)
		proxies.POST("/data", h.Proxy.ImportData)
		proxies.GET("/:id", h.Proxy.GetByID)
		proxies.POST("", h.Proxy.Create)
		proxies.PUT("/:id", h.Proxy.Update)
		proxies.DELETE("/:id", h.Proxy.Delete)
		proxies.POST("/:id/test", h.Proxy.Test)
		proxies.POST("/:id/quality-check", h.Proxy.CheckQuality)
		proxies.GET("/:id/stats", h.Proxy.GetStats)
		proxies.GET("/:id/accounts", h.Proxy.GetProxyAccounts)
		proxies.POST("/batch-delete", h.Proxy.BatchDelete)
		proxies.POST("/batch", h.Proxy.BatchCreate)
	}
}

func registerRedeemCodeRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	codes := admin.Group("/redeem-codes")
	{
		codes.GET("", h.Redeem.List)
		codes.GET("/stats", h.Redeem.GetStats)
		codes.GET("/export", h.Redeem.Export)
		codes.GET("/:id", h.Redeem.GetByID)
		codes.POST("/create-and-redeem", h.Redeem.CreateAndRedeem)
		codes.POST("/generate", h.Redeem.Generate)
		codes.DELETE("/:id", h.Redeem.Delete)
		codes.POST("/batch-delete", h.Redeem.BatchDelete)
		codes.POST("/:id/expire", h.Redeem.Expire)
	}
}

func registerPromoCodeRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	promoCodes := admin.Group("/promo-codes")
	{
		promoCodes.GET("", h.Promo.List)
		promoCodes.GET("/:id", h.Promo.GetByID)
		promoCodes.POST("", h.Promo.Create)
		promoCodes.PUT("/:id", h.Promo.Update)
		promoCodes.DELETE("/:id", h.Promo.Delete)
		promoCodes.GET("/:id/usages", h.Promo.GetUsages)
	}
}

func registerSettingsRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	adminSettings := admin.Group("/settings")
	{
		adminSettings.GET("", h.Setting.GetSettings)
		adminSettings.PUT("", h.Setting.UpdateSettings)
		adminSettings.POST("/test-smtp", h.Setting.TestSMTPConnection)
		adminSettings.POST("/send-test-email", h.Setting.SendTestEmail)
		// 529过载冷却配置
		adminSettings.GET("/overload-cooldown", h.Setting.GetOverloadCooldownSettings)
		adminSettings.PUT("/overload-cooldown", h.Setting.UpdateOverloadCooldownSettings)
		// 流超时处理配置
		adminSettings.GET("/stream-timeout", h.Setting.GetStreamTimeoutSettings)
		adminSettings.PUT("/stream-timeout", h.Setting.UpdateStreamTimeoutSettings)
		// 请求整流器配置
		adminSettings.GET("/rectifier", h.Setting.GetRectifierSettings)
		adminSettings.PUT("/rectifier", h.Setting.UpdateRectifierSettings)
		// Beta 策略配置
		adminSettings.GET("/beta-policy", h.Setting.GetBetaPolicySettings)
		adminSettings.PUT("/beta-policy", h.Setting.UpdateBetaPolicySettings)
	}
}

func registerSystemRoutes(admin *gin.RouterGroup, buildInfo service.BuildInfo) {
	system := admin.Group("/system")
	{
		system.GET("/version", func(c *gin.Context) {
			response.Success(c, gin.H{"version": buildInfo.Version})
		})
	}
}

func registerSubscriptionRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	subscriptions := admin.Group("/subscriptions")
	{
		subscriptions.GET("", h.Subscription.List)
		subscriptions.GET("/:id", h.Subscription.GetByID)
		subscriptions.GET("/:id/progress", h.Subscription.GetProgress)
		subscriptions.POST("/assign", h.Subscription.Assign)
		subscriptions.POST("/bulk-assign", h.Subscription.BulkAssign)
		subscriptions.POST("/:id/extend", h.Subscription.Extend)
		subscriptions.POST("/:id/reset-quota", h.Subscription.ResetQuota)
		subscriptions.DELETE("/:id", h.Subscription.Revoke)
	}

	// 分组下的订阅列表
	admin.GET("/groups/:id/subscriptions", h.Subscription.ListByGroup)

	// 用户下的订阅列表
	admin.GET("/users/:id/subscriptions", h.Subscription.ListByUser)
}

func registerUsageRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	usage := admin.Group("/usage")
	{
		usage.GET("", h.Usage.List)
		usage.GET("/stats", h.Usage.Stats)
		usage.GET("/search-users", h.Usage.SearchUsers)
		usage.GET("/search-api-keys", h.Usage.SearchAPIKeys)
		usage.GET("/cleanup-tasks", h.Usage.ListCleanupTasks)
		usage.POST("/cleanup-tasks", h.Usage.CreateCleanupTask)
		usage.POST("/cleanup-tasks/:id/cancel", h.Usage.CancelCleanupTask)
		usage.GET("/user-breakdown", h.Usage.GetUserBreakdown)
		usage.POST("/users-usage", h.Usage.GetBatchUsersUsage)
	}
}

func registerUserAttributeRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	attrs := admin.Group("/user-attributes")
	{
		attrs.GET("", h.UserAttribute.ListDefinitions)
		attrs.POST("", h.UserAttribute.CreateDefinition)
		attrs.POST("/batch", h.UserAttribute.GetBatchUserAttributes)
		attrs.PUT("/reorder", h.UserAttribute.ReorderDefinitions)
		attrs.PUT("/:id", h.UserAttribute.UpdateDefinition)
		attrs.DELETE("/:id", h.UserAttribute.DeleteDefinition)
	}
}

func registerScheduledTestRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	plans := admin.Group("/scheduled-test-plans")
	{
		plans.POST("", h.ScheduledTest.Create)
		plans.PUT("/:id", h.ScheduledTest.Update)
		plans.DELETE("/:id", h.ScheduledTest.Delete)
		plans.GET("/:id/results", h.ScheduledTest.ListResults)
	}
	// Nested under accounts
	admin.GET("/accounts/:id/scheduled-test-plans", h.ScheduledTest.ListByAccount)
}

func registerErrorPassthroughRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	rules := admin.Group("/error-passthrough-rules")
	{
		rules.GET("", h.ErrorPassthrough.List)
		rules.GET("/:id", h.ErrorPassthrough.GetByID)
		rules.POST("", h.ErrorPassthrough.Create)
		rules.PUT("/:id", h.ErrorPassthrough.Update)
		rules.DELETE("/:id", h.ErrorPassthrough.Delete)
	}
}

func registerTLSFingerprintProfileRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	profiles := admin.Group("/tls-fingerprint-profiles")
	{
		profiles.GET("", h.TLSFingerprintProfile.List)
		profiles.GET("/:id", h.TLSFingerprintProfile.GetByID)
		profiles.POST("", h.TLSFingerprintProfile.Create)
		profiles.PUT("/:id", h.TLSFingerprintProfile.Update)
		profiles.DELETE("/:id", h.TLSFingerprintProfile.Delete)
	}
}

func registerChannelRoutes(admin *gin.RouterGroup, h *handler.AdminHandlers) {
	channels := admin.Group("/channels")
	{
		channels.GET("", h.Channel.List)
		channels.GET("/model-pricing", h.Channel.GetModelDefaultPricing)
		channels.GET("/:id", h.Channel.GetByID)
		channels.POST("", h.Channel.Create)
		channels.PUT("/:id", h.Channel.Update)
		channels.DELETE("/:id", h.Channel.Delete)
	}
}
