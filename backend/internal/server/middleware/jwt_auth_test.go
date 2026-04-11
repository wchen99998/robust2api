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

func newJWTTestEnv(controlAuthService service.ControlAccessTokenAuthenticator) *gin.Engine {
	gin.SetMode(gin.TestMode)

	mw := NewJWTAuthMiddleware(controlAuthService)

	r := gin.New()
	r.Use(gin.HandlerFunc(mw))
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestJWTAuth_MissingAuthorizationHeader(t *testing.T) {
	router := newJWTTestEnv(nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "UNAUTHORIZED", body.Code)
}

func TestJWTAuth_InvalidHeaderFormatWithoutCookie(t *testing.T) {
	router := newJWTTestEnv(nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Token abc123")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "UNAUTHORIZED", body.Code)
}

func TestJWTAuth_WithBearerTokenAndNilServiceReturnsInternalError(t *testing.T) {
	router := newJWTTestEnv(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "INTERNAL_ERROR", body.Code)
}

func TestExtractControlAccessToken_PrefersCookieOverAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer from-header")
	req.AddCookie(&http.Cookie{Name: service.ControlAccessCookieName, Value: "from-cookie"})
	c.Request = req

	require.Equal(t, "from-cookie", extractControlAccessToken(c))
}

func TestAbortControlAuthError_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "access token expired", err: service.ErrAccessTokenExpired, wantStatus: http.StatusUnauthorized, wantCode: "TOKEN_EXPIRED"},
		{name: "token revoked", err: service.ErrTokenRevoked, wantStatus: http.StatusUnauthorized, wantCode: "TOKEN_REVOKED"},
		{name: "user inactive", err: service.ErrUserNotActive, wantStatus: http.StatusUnauthorized, wantCode: "USER_INACTIVE"},
		{name: "invalid token", err: service.ErrInvalidToken, wantStatus: http.StatusUnauthorized, wantCode: "INVALID_TOKEN"},
		{name: "internal", err: http.ErrNoCookie, wantStatus: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)

			abortControlAuthError(c, tt.err)

			require.Equal(t, tt.wantStatus, rec.Code)
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tt.wantCode, body.Code)
		})
	}
}
