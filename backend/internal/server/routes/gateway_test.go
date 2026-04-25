package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayRouteTestCore struct{}

func (gatewayRouteTestCore) Handle(context.Context, core.IngressRequest) (*core.GatewayResult, error) {
	return &core.GatewayResult{StatusCode: http.StatusOK}, nil
}

func newGatewayRoutesTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	RegisterGatewayRoutes(
		router,
		&handler.GatewayHandlers{
			CoreGateway: handler.NewCoreGatewayHandler(gatewayRouteTestCore{}),
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		nil,
		nil,
		nil,
		&config.Config{},
	)

	return router
}

func TestGatewayRoutesOpenAIResponsesCompactPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{"/v1/responses/compact", "/responses/compact"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI responses handler", path)
	}
}

func TestGatewayRoutesPublicSurfaceIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()
	registered := map[string]struct{}{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	expected := []string{
		"POST /v1/messages",
		"POST /v1/messages/count_tokens",
		"GET /v1/models",
		"GET /v1/usage",
		"POST /v1/responses",
		"POST /v1/responses/*subpath",
		"GET /v1/responses",
		"POST /responses",
		"POST /responses/*subpath",
		"GET /responses",
		"POST /v1/chat/completions",
		"POST /chat/completions",
		"POST /openai/v1/responses",
		"POST /openai/v1/responses/*subpath",
		"GET /openai/v1/responses",
		"POST /openai/v1/chat/completions",
		"GET /v1beta/models",
		"GET /v1beta/models/:model",
		"POST /v1beta/models/*modelAction",
		"GET /antigravity/models",
		"POST /antigravity/v1/messages",
		"POST /antigravity/v1/messages/count_tokens",
		"GET /antigravity/v1/models",
		"GET /antigravity/v1/usage",
		"GET /antigravity/v1beta/models",
		"GET /antigravity/v1beta/models/:model",
		"POST /antigravity/v1beta/models/*modelAction",
	}
	for _, route := range expected {
		require.Contains(t, registered, route)
	}
}
