package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BackendModeUserGuard blocks non-admin users from accessing user routes when backend mode is enabled.
// Must be placed AFTER JWT auth middleware so that the user role is available in context.
func BackendModeUserGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settingService == nil || !settingService.IsBackendModeEnabled(c.Request.Context()) {
			c.Next()
			return
		}
		role, _ := GetUserRoleFromContext(c)
		if role == "admin" {
			c.Next()
			return
		}
		response.Forbidden(c, "Backend mode is active. User self-service is disabled.")
		c.Abort()
	}
}

// BackendModeAuthGuard selectively blocks unauthenticated auth endpoints when backend mode is enabled.
// Allows bootstrap, session establishment/rotation, and external OAuth login/callback.
// Blocks registration, password reset, and other public self-service auth flows.
func BackendModeAuthGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settingService == nil || !settingService.IsBackendModeEnabled(c.Request.Context()) {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		allowedSuffixes := []string{
			"/bootstrap",
			"/session/login",
			"/session/login/totp",
			"/session/refresh",
		}
		for _, suffix := range allowedSuffixes {
			if strings.HasSuffix(path, suffix) {
				c.Next()
				return
			}
		}
		if strings.Contains(path, "/oauth/") &&
			(strings.HasSuffix(path, "/start") || strings.HasSuffix(path, "/callback")) {
			c.Next()
			return
		}

		response.Forbidden(c, "Backend mode is active. Registration and self-service auth flows are disabled.")
		c.Abort()
	}
}
