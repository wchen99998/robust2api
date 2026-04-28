package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesPlanningHelperReturnsNormalizedBodyAndLegacyAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &service.Account{
		ID:          9,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 4,
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/responses/compact", bytes.NewReader([]byte(`{"model":{"value":"gpt-5.1"}}`)))
	c.Request = req

	body := []byte(`{"model":{"value":"gpt-5.1"}}`)
	helper := openAIResponsesPlanningHelper{
		selectAccount: func(input openAIResponsesPlanningInput) (*openAIResponsesPlanningResult, error) {
			require.Equal(t, body, input.body)
			require.Equal(t, openAIPlanningSubject{apiKeyID: 1, userID: 2, groupID: 3}, input.subject)
			require.Equal(t, domain.TransportHTTP, input.transport)

			return &openAIResponsesPlanningResult{
				Plan: domain.RoutingPlan{
					Request: domain.IngressRequest{RequestID: "req-test"},
				},
				Account:        account,
				NormalizedBody: []byte(`{"model":"gpt-5.1"}`),
			}, nil
		},
	}

	result, err := helper.planAndSelect(c, openAIResponsesPlanningInput{
		body:      body,
		subject:   openAIPlanningSubject{apiKeyID: 1, userID: 2, groupID: 3},
		transport: domain.TransportHTTP,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Same(t, account, result.Account)
	require.JSONEq(t, `{"model":"gpt-5.1"}`, string(result.NormalizedBody))
}
