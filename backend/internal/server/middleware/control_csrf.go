package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func ControlCSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			c.Next()
			return
		}
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			c.Next()
			return
		}

		headerValue := strings.TrimSpace(c.GetHeader(service.ControlCSRFHeaderName))
		cookie, err := c.Request.Cookie(service.ControlCSRFCookieName)
		cookieValue := ""
		if err == nil && cookie != nil {
			cookieValue = strings.TrimSpace(cookie.Value)
		}

		if headerValue == "" || cookieValue == "" {
			AbortWithError(c, http.StatusForbidden, "CSRF_TOKEN_REQUIRED", "csrf token is required")
			return
		}
		if headerValue != cookieValue {
			AbortWithError(c, http.StatusForbidden, "CSRF_TOKEN_INVALID", "csrf token is invalid")
			return
		}

		c.Next()
	}
}
