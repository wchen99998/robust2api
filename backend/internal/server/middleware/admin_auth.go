// Package middleware provides HTTP middleware for authentication, authorization, and request processing.
package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewAdminAuthMiddleware creates the authenticated-admin middleware.
func NewAdminAuthMiddleware(controlAuthService service.ControlAccessTokenAuthenticator) AdminAuthMiddleware {
	return AdminAuthMiddleware(adminAuth(controlAuthService))
}

func adminAuth(controlAuthService service.ControlAccessTokenAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractControlAccessToken(c)
		if isWebSocketUpgradeRequest(c) {
			if wsToken := extractJWTFromWebSocketSubprotocol(c); wsToken != "" {
				tokenString = wsToken
			}
		}

		if tokenString == "" {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
			return
		}

		identity, err := authenticateControlIdentity(c, controlAuthService, tokenString)
		if err != nil {
			abortControlAuthError(c, err)
			return
		}
		if identity == nil || identity.PrimaryRole != service.RoleAdmin {
			AbortWithError(c, 403, "FORBIDDEN", "Admin access required")
			return
		}

		c.Set(string(ContextKeyUser), AuthSubject{
			SubjectID:    identity.SubjectID,
			SessionID:    identity.SessionID,
			UserID:       identity.LegacyUserID,
			Concurrency:  identity.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), identity.PrimaryRole)
		authMethod := strings.TrimSpace(identity.AMR)
		if authMethod == "" {
			authMethod = "session"
		}
		c.Set("auth_method", authMethod)
		c.Next()
	}
}

func isWebSocketUpgradeRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	upgrade := strings.ToLower(strings.TrimSpace(c.GetHeader("Upgrade")))
	if upgrade != "websocket" {
		return false
	}
	connection := strings.ToLower(c.GetHeader("Connection"))
	return strings.Contains(connection, "upgrade")
}

func extractJWTFromWebSocketSubprotocol(c *gin.Context) string {
	if c == nil {
		return ""
	}
	raw := strings.TrimSpace(c.GetHeader("Sec-WebSocket-Protocol"))
	if raw == "" {
		return ""
	}

	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if strings.HasPrefix(value, "jwt.") {
			token := strings.TrimSpace(strings.TrimPrefix(value, "jwt."))
			if token != "" {
				return token
			}
		}
	}
	return ""
}
