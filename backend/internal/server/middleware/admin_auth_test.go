//go:build unit

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newAdminTestRouter(controlAuthService *service.ControlAuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(controlAuthService)))
	router.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return router
}

func TestAdminAuth_MissingTokenReturnsUnauthorized(t *testing.T) {
	router := newAdminTestRouter(nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "UNAUTHORIZED", body.Code)
}

func TestAdminAuth_WebsocketJWTAndNilServiceReturnsInternalError(t *testing.T) {
	router := newAdminTestRouter(nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt.test-token")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "INTERNAL_ERROR", body.Code)
}

func TestIsWebSocketUpgradeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/admin", nil)
	require.False(t, isWebSocketUpgradeRequest(c))

	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Connection", "keep-alive, Upgrade")
	require.True(t, isWebSocketUpgradeRequest(c))
}

func TestExtractJWTFromWebSocketSubprotocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/admin", nil)

	c.Request.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt.token123")
	require.Equal(t, "token123", extractJWTFromWebSocketSubprotocol(c))

	c.Request.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin")
	require.Empty(t, extractJWTFromWebSocketSubprotocol(c))
}
