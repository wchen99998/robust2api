package middleware

import (
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewJWTAuthMiddleware creates the control-plane authenticated-user middleware.
func NewJWTAuthMiddleware(controlAuthService service.ControlAccessTokenAuthenticator) JWTAuthMiddleware {
	return JWTAuthMiddleware(jwtAuth(controlAuthService))
}

func jwtAuth(controlAuthService service.ControlAccessTokenAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractControlAccessToken(c)
		if tokenString == "" {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
			return
		}

		identity, err := authenticateControlIdentity(c, controlAuthService, tokenString)
		if err != nil {
			abortControlAuthError(c, err)
			return
		}

		c.Set(string(ContextKeyUser), AuthSubject{
			SubjectID:    identity.SubjectID,
			SessionID:    identity.SessionID,
			UserID:       identity.LegacyUserID,
			Concurrency:  identity.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), identity.PrimaryRole)
		c.Next()
	}
}

func extractControlAccessToken(c *gin.Context) string {
	if c == nil {
		return ""
	}

	if cookie, err := c.Request.Cookie(service.ControlAccessCookieName); err == nil && cookie != nil {
		if token := strings.TrimSpace(cookie.Value); token != "" {
			return token
		}
	}

	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func authenticateControlIdentity(c *gin.Context, controlAuthService service.ControlAccessTokenAuthenticator, tokenString string) (*service.AuthenticatedIdentity, error) {
	if controlAuthService == nil {
		return nil, errors.New("control auth service is not configured")
	}
	return controlAuthService.AuthenticateAccessToken(c.Request.Context(), tokenString)
}

func abortControlAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAccessTokenExpired):
		AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
	case errors.Is(err, service.ErrTokenRevoked):
		AbortWithError(c, 401, "TOKEN_REVOKED", "Token has been revoked")
	case errors.Is(err, service.ErrUserNotActive):
		AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
	case errors.Is(err, service.ErrInvalidToken):
		AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
	default:
		AbortWithError(c, 500, "INTERNAL_ERROR", "Internal server error")
	}
}
