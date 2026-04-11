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

// BackendModeAuthGuard selectively blocks auth endpoints when backend mode is enabled.
// Allows: login, login/2fa, logout, refresh, bootstrap, and external OAuth callbacks.
// Blocks: registration, forgot-password, reset-password, and other self-service auth flows.
func BackendModeAuthGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settingService == nil || !settingService.IsBackendModeEnabled(c.Request.Context()) {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		allowedSuffixes := []string{
			"/auth/login",
			"/auth/login/2fa",
			"/auth/logout",
			"/auth/refresh",
			"/bootstrap",
			"/session/login",
			"/session/login/totp",
			"/session/logout",
			"/session/refresh",
			"/oauth/linuxdo/start",
			"/oauth/linuxdo/callback",
			"/oauth/oidc/start",
			"/oauth/oidc/callback",
			"/jwks",
		}
		for _, suffix := range allowedSuffixes {
			if strings.HasSuffix(path, suffix) {
				c.Next()
				return
			}
		}
		response.Forbidden(c, "Backend mode is active. Registration and self-service auth flows are disabled.")
		c.Abort()
	}
}
