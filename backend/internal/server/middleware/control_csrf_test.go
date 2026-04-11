//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCSRFMiddlewareTestEnv() *gin.Engine {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(ControlCSRFMiddleware())
	r.Any("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestControlCSRFMiddleware_SafeMethodBypassesCheck(t *testing.T) {
	router := newCSRFMiddlewareTestEnv()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestControlCSRFMiddleware_RejectsMissingToken(t *testing.T) {
	router := newCSRFMiddlewareTestEnv()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "CSRF_TOKEN_REQUIRED")
}

func TestControlCSRFMiddleware_AllowsMatchingHeaderAndCookie(t *testing.T) {
	router := newCSRFMiddlewareTestEnv()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	req.Header.Set(service.ControlCSRFHeaderName, "csrf-token")
	req.AddCookie(&http.Cookie{Name: service.ControlCSRFCookieName, Value: "csrf-token"})
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestControlCSRFMiddleware_SkipsCheckForBearerRequests(t *testing.T) {
	router := newCSRFMiddlewareTestEnv()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}
