package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	openai "github.com/Wei-Shaw/sub2api/internal/gateway/provider/openai"
	"github.com/Wei-Shaw/sub2api/internal/gateway/scheduler"
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

func TestOpenAIResponsesPlanningResultConvertsSchedulerSelectionToLegacySelection(t *testing.T) {
	account := &service.Account{
		ID:          22,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Concurrency: 5,
	}

	release := func() {}
	selection := openAIPlanningResultToLegacySelection(&openAIResponsesPlanningResult{
		Account: account,
		ScheduleResult: &scheduler.ScheduleResult{
			Account: scheduler.Account{
				Snapshot: domain.AccountSnapshot{ID: 22},
			},
			Layer: domain.AccountDecisionLoadBalance,
			Reservation: scheduler.Reservation{
				AccountID: 22,
				Acquired:  true,
				Release:   release,
			},
		},
	})

	require.NotNil(t, selection)
	require.Same(t, account, selection.Account)
	require.True(t, selection.Acquired)
	require.NotNil(t, selection.ReleaseFunc)
}

func TestOpenAIResponsesPlanningResultConvertsWaitPlanLimitsToLegacySelection(t *testing.T) {
	account := &service.Account{
		ID:          22,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Concurrency: 5,
	}

	selection := openAIPlanningResultToLegacySelection(&openAIResponsesPlanningResult{
		Account: account,
		ScheduleResult: &scheduler.ScheduleResult{
			Account: scheduler.Account{
				Snapshot: domain.AccountSnapshot{ID: 22, Concurrency: 7},
			},
			Layer: domain.AccountDecisionLoadBalance,
			Reservation: scheduler.Reservation{
				AccountID: 22,
				Acquired:  false,
			},
			WaitPlan: domain.AccountWaitPlan{
				Required:       true,
				Reason:         "account_busy",
				Timeout:        12 * time.Second,
				MaxConcurrency: 7,
				MaxWaiting:     42,
			},
		},
	})

	require.NotNil(t, selection)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(22), selection.WaitPlan.AccountID)
	require.Equal(t, 7, selection.WaitPlan.MaxConcurrency)
	require.Equal(t, 12*time.Second, selection.WaitPlan.Timeout)
	require.Equal(t, 42, selection.WaitPlan.MaxWaiting)
}

func TestOpenAIResponsesPlanningHelperAppliesRequestedModelOverrideToNormalizedBodies(t *testing.T) {
	parsed, err := openai.ParseResponses(domain.IngressRequest{
		Subpath: "/compact",
		Body:    []byte(`{"model":{"value":"gpt-5.1"},"input":{"value":"hello"}}`),
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.1","input":"hello"}`, string(parsed.NormalizedBody))

	requestedModel := applyOpenAIResponsesRequestedModelOverride(&parsed, " gpt-5.1-mini ")

	require.Equal(t, "gpt-5.1-mini", requestedModel)
	require.Equal(t, "gpt-5.1-mini", parsed.Canonical.RequestedModel)
	require.Equal(t, "gpt-5.1-mini", parsed.Canonical.Model.Requested)
	require.Equal(t, "gpt-5.1-mini", parsed.Canonical.Model.Canonical)
	require.JSONEq(t, `{"model":"gpt-5.1-mini","input":"hello"}`, string(parsed.NormalizedBody))
	require.JSONEq(t, string(parsed.NormalizedBody), string(parsed.Canonical.Body))
	require.Equal(t, openAIResponsesPlanningTestSHA256(parsed.NormalizedBody), parsed.BodySHA256)
}

func openAIResponsesPlanningTestSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
