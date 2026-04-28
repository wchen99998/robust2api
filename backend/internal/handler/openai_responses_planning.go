package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/adapters"
	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/Wei-Shaw/sub2api/internal/gateway/ingress"
	"github.com/Wei-Shaw/sub2api/internal/gateway/planning"
	openai "github.com/Wei-Shaw/sub2api/internal/gateway/provider/openai"
	"github.com/Wei-Shaw/sub2api/internal/gateway/scheduler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openAIPlanningSubject struct {
	apiKeyID int64
	userID   int64
	groupID  int64
}

type openAIResponsesPlanningInput struct {
	body                   []byte
	subject                openAIPlanningSubject
	transport              domain.TransportKind
	previousResponseID     string
	sessionKey             string
	requestedModelOverride string
	excludedAccountIDs     map[int64]struct{}
}

type openAIResponsesPlanningResult struct {
	Plan           domain.RoutingPlan
	Account        *service.Account
	NormalizedBody []byte
	ScheduleResult *scheduler.ScheduleResult
}

type openAIResponsesPlanningHelper struct {
	gatewayService     *service.OpenAIGatewayService
	maxAccountSwitches int
	selectAccount      func(input openAIResponsesPlanningInput) (*openAIResponsesPlanningResult, error)
}

func (h openAIResponsesPlanningHelper) planAndSelect(
	c *gin.Context,
	input openAIResponsesPlanningInput,
) (*openAIResponsesPlanningResult, error) {
	if h.selectAccount != nil {
		return h.selectAccount(input)
	}
	if c == nil || c.Request == nil {
		return nil, errors.New("openai responses planning: request is nil")
	}

	subject := domain.Subject{
		APIKey: domain.APIKeySnapshot{
			ID:      input.subject.apiKeyID,
			UserID:  input.subject.userID,
			GroupID: input.subject.groupID,
			Policy:  domain.GroupPolicy{GroupID: input.subject.groupID},
		},
		User:  domain.UserSnapshot{ID: input.subject.userID},
		Group: domain.GroupSnapshot{ID: input.subject.groupID, Platform: domain.PlatformOpenAI},
	}

	ingressRequest, err := ingress.BuildOpenAIResponses(ingress.OpenAIResponsesInput{
		Request:   c.Request,
		Body:      input.body,
		Transport: input.transport,
		Subject:   subject,
	})
	if err != nil {
		return nil, err
	}

	parsed, err := openai.ParseResponses(ingressRequest)
	if err != nil {
		return nil, err
	}

	requestedModel := applyOpenAIResponsesRequestedModelOverride(&parsed, input.requestedModelOverride)

	plan := planning.BuildOpenAIResponsesPlan(planning.OpenAIResponsesPlanInput{
		Ingress:            ingressRequest,
		Subject:            subject,
		Parsed:             parsed,
		MaxAccountSwitches: h.maxAccountSwitches,
		CreatedAt:          time.Now().UTC(),
	})

	if h.gatewayService == nil {
		return nil, errors.New("openai responses planning: gateway service is nil")
	}

	groupID := input.subject.groupID
	var groupIDPtr *int64
	if groupID > 0 {
		groupIDPtr = &groupID
	}
	sessionKey := strings.TrimSpace(input.sessionKey)
	if sessionKey == "" {
		sessionKey = strings.TrimSpace(plan.Session.Key)
	}
	previousResponseID := strings.TrimSpace(input.previousResponseID)
	if previousResponseID == "" {
		previousResponseID = strings.TrimSpace(parsed.PreviousResponseID)
	}

	ports := adapters.OpenAISchedulerPorts{
		Bridge:         h.gatewayService,
		GroupID:        groupIDPtr,
		RequestedModel: requestedModel,
		Excluded:       input.excludedAccountIDs,
	}
	scheduleResult, err := scheduler.NewOpenAIScheduler(ports).Select(contextFromGin(c), scheduler.ScheduleRequest{
		GroupID:            groupID,
		SessionKey:         sessionKey,
		PreviousResponseID: previousResponseID,
		RequestedModel:     requestedModel,
		RequiredTransport:  openAIRequiredTransportForScheduler(input.transport),
		ExcludedAccountIDs: input.excludedAccountIDs,
	})
	if err != nil {
		return nil, err
	}
	if scheduleResult == nil {
		return nil, errors.New("openai responses planning: scheduler returned nil result")
	}

	account, ok := scheduleResult.Account.Legacy.(*service.Account)
	if !ok || account == nil {
		return nil, errors.New("openai responses planning: selected account missing legacy service account")
	}

	plan.Account = domain.AccountDecision{
		Layer:   scheduleResult.Layer,
		Account: scheduleResult.Account.Snapshot,
		Reservation: domain.AccountReservation{
			AccountID: scheduleResult.Reservation.AccountID,
		},
		WaitPlan: scheduleResult.WaitPlan,
	}
	plan.Diagnostics = scheduleResult.Diagnostics
	if plan.Session.Enabled {
		plan.Session.AccountID = account.ID
	}

	return &openAIResponsesPlanningResult{
		Plan:           plan,
		Account:        account,
		NormalizedBody: append([]byte(nil), parsed.NormalizedBody...),
		ScheduleResult: scheduleResult,
	}, nil
}

func contextFromGin(c *gin.Context) context.Context {
	if c != nil && c.Request != nil && c.Request.Context() != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func openAIPlanningSubjectFromService(apiKey *service.APIKey, userID int64) openAIPlanningSubject {
	if apiKey == nil {
		return openAIPlanningSubject{userID: userID}
	}
	if userID == 0 {
		userID = apiKey.UserID
	}
	subject := openAIPlanningSubject{
		apiKeyID: apiKey.ID,
		userID:   userID,
	}
	if apiKey.GroupID != nil {
		subject.groupID = *apiKey.GroupID
	} else if apiKey.Group != nil {
		subject.groupID = apiKey.Group.ID
	}
	return subject
}

func applyOpenAIResponsesRequestedModelOverride(parsed *openai.ResponsesParseResult, override string) string {
	trimmedOverride := strings.TrimSpace(override)
	if parsed == nil {
		return trimmedOverride
	}

	requestedModel := strings.TrimSpace(parsed.Canonical.RequestedModel)
	if trimmedOverride == "" {
		return requestedModel
	}

	requestedModel = trimmedOverride
	parsed.Canonical.RequestedModel = requestedModel
	parsed.Canonical.Model.Requested = requestedModel
	parsed.Canonical.Model.Canonical = requestedModel

	normalizedBody := service.ReplaceModelInBody(parsed.NormalizedBody, requestedModel)
	parsed.NormalizedBody = append([]byte(nil), normalizedBody...)
	parsed.Canonical.Body = append([]byte(nil), normalizedBody...)
	parsed.BodySHA256 = openAIResponsesBodySHA256(parsed.NormalizedBody)

	return requestedModel
}

func openAIResponsesBodySHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func openAIRequiredTransportForScheduler(transport domain.TransportKind) domain.TransportKind {
	if transport == domain.TransportWebSocket {
		return domain.TransportWebSocket
	}
	return domain.TransportHTTP
}
