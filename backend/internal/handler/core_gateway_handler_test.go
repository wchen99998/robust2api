package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type coreCapture struct {
	req core.IngressRequest
}

func (c *coreCapture) Handle(_ context.Context, req core.IngressRequest) (*core.GatewayResult, error) {
	c.req = req
	return &core.GatewayResult{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"content-type": {"application/json"}},
		Body:       []byte(`{"ok":true}`),
	}, nil
}

type coreError struct{}

func (coreError) Handle(context.Context, core.IngressRequest) (*core.GatewayResult, error) {
	return nil, errors.New("bad request")
}

func TestCoreGatewayHandlerHandleBuildsIngressRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &coreCapture{}
	h := NewCoreGatewayHandler(capture)
	router := gin.New()
	router.POST("/openai/v1/responses", func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      2,
			GroupID: &groupID,
			User:    &service.User{ID: 3},
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI},
		})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 3, Concurrency: 1})
		h.Handle(c)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	req.Header.Set("User-Agent", "codex")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"ok":true}`, rec.Body.String())
	require.Equal(t, "/openai/v1/responses", capture.req.Path)
	require.Equal(t, core.EndpointResponses, capture.req.Endpoint)
	require.Equal(t, int64(2), capture.req.APIKey.ID)
	require.Equal(t, int64(3), capture.req.User.ID)
	require.JSONEq(t, `{"model":"gpt-5"}`, string(capture.req.Body))
}

func TestCoreGatewayHandlerHandleWebSocketRequiresUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewCoreGatewayHandler(&coreCapture{})
	router := gin.New()
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		h.HandleWebSocket(c)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUpgradeRequired, rec.Code)
	require.Contains(t, rec.Body.String(), "Expected WebSocket upgrade")
}

func TestCoreGatewayHandlerUsesGoogleErrorForGeminiCoreErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewCoreGatewayHandler(coreError{})
	router := gin.New()
	router.POST("/v1beta/models/gemini:generateContent", func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      2,
			GroupID: &groupID,
			User:    &service.User{ID: 3},
			Group:   &service.Group{ID: groupID, Platform: service.PlatformGemini},
		})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 3, Concurrency: 1})
		h.Handle(c)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:generateContent", strings.NewReader(`{`))
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.JSONEq(t, `{"error":{"code":502,"message":"bad request","status":"Bad Gateway"}}`, rec.Body.String())
}
