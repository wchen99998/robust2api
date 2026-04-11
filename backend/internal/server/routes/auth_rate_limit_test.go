package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newAuthRoutesTestRouter(redisClient *redis.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterAuthRoutes(
		v1,
		&handler.ControlHandlers{
			Auth:    &handler.AuthHandler{},
			Setting: &handler.SettingHandler{},
		},
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		redisClient,
		nil,
	)

	return router
}

func TestAuthRoutesRateLimitFailCloseWhenRedisUnavailable(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	router := newAuthRoutesTestRouter(rdb)
	paths := []string{
		"/api/v1/session/login",
		"/api/v1/session/login/totp",
		"/api/v1/session/refresh",
		"/api/v1/registration/preflight",
		"/api/v1/registration",
		"/api/v1/registration/email-code",
		"/api/v1/registration/complete",
		"/api/v1/password/forgot",
		"/api/v1/password/reset",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.10:12345"

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusTooManyRequests, w.Code, "path=%s", path)
		require.Contains(t, w.Body.String(), "rate limit exceeded", "path=%s", path)
	}
}

func TestAuthRoutesRegisterBFFEndpoints(t *testing.T) {
	router := newAuthRoutesTestRouter(nil)
	paths := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/bootstrap"},
		{method: http.MethodGet, path: "/api/v1/jwks"},
		{method: http.MethodDelete, path: "/api/v1/session"},
		{method: http.MethodDelete, path: "/api/v1/sessions"},
		{method: http.MethodGet, path: "/api/v1/oauth/oidc/start"},
		{method: http.MethodGet, path: "/api/v1/oauth/oidc/callback"},
		{method: http.MethodPatch, path: "/api/v1/me"},
		{method: http.MethodGet, path: "/api/v1/me/mfa/totp"},
		{method: http.MethodPost, path: "/api/v1/me/mfa/totp/setup"},
		{method: http.MethodPost, path: "/api/v1/embed-token"},
	}

	for _, tc := range paths {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s", tc.path)
	}
}
