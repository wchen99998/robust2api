package middleware

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestMetricsPlatformPrefersRequestContextPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/v1/messages", nil)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Platform, service.PlatformOpenAI))

	c.Set("platform", service.PlatformAnthropic)

	require.Equal(t, service.PlatformOpenAI, requestMetricsPlatform(c))
}

func TestRequestMetricsPlatformFallsBackToGroupPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/v1/messages", nil)
	c.Set(string(ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{Platform: service.PlatformAnthropic},
	})

	require.Equal(t, service.PlatformAnthropic, requestMetricsPlatform(c))
}
