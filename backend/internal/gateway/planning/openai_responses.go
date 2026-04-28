package planning

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	openai "github.com/Wei-Shaw/sub2api/internal/gateway/provider/openai"
)

type OpenAIResponsesPlanInput struct {
	Ingress            domain.IngressRequest
	Subject            domain.Subject
	Parsed             openai.ResponsesParseResult
	MaxAccountSwitches int
	CreatedAt          time.Time
}

func BuildOpenAIResponsesPlan(input OpenAIResponsesPlanInput) domain.RoutingPlan {
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	maxAttempts := input.MaxAccountSwitches + 1
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	session := buildOpenAIResponsesSession(input.Parsed.Canonical.Session)
	billing := buildOpenAIResponsesBilling(input.Ingress, input.Parsed)

	return domain.RoutingPlan{
		Request:   input.Ingress,
		Subject:   input.Subject,
		Canonical: input.Parsed.Canonical,
		GroupID:   input.Subject.APIKey.GroupID,
		Session:   session,
		Retry: domain.RetryPlan{
			MaxAttempts:        maxAttempts,
			RetrySameAccount:   true,
			RetryOtherAccounts: true,
		},
		Billing: billing,
		Debug: domain.DebugPlan{
			Enabled: true,
			BodyFingerprint: domain.BodyFingerprint{
				SHA256: input.Parsed.BodySHA256,
				Bytes:  int64(len(input.Parsed.NormalizedBody)),
			},
		},
		CreatedAt: createdAt,
	}
}

func buildOpenAIResponsesSession(sessionInput domain.SessionInput) domain.SessionDecision {
	if sessionInput.Key == "" || sessionInput.Source == "" || sessionInput.Source == domain.SessionSourceNone {
		return domain.SessionDecision{Source: domain.SessionSourceNone}
	}

	return domain.SessionDecision{
		Enabled: true,
		Key:     sessionInput.Key,
		Source:  sessionInput.Source,
		Sticky:  true,
	}
}

func buildOpenAIResponsesBilling(ingress domain.IngressRequest, parsed openai.ResponsesParseResult) domain.BillingLifecyclePlan {
	billing := domain.BillingLifecyclePlan{
		Mode:   domain.BillingModeToken,
		Model:  parsed.Canonical.RequestedModel,
		Events: []domain.BillingEventKind{domain.BillingEventCharge},
	}

	if parsed.Stream || ingress.Transport == domain.TransportWebSocket {
		billing.Mode = domain.BillingModeStreaming
		billing.Events = []domain.BillingEventKind{
			domain.BillingEventReserve,
			domain.BillingEventFinalize,
			domain.BillingEventRelease,
		}
	}

	return billing
}
