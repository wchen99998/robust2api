package ingress

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

const (
	openAIResponsesPrefix      = "/v1/responses"
	openAIResponsesAliasPrefix = "/openai/v1/responses"
)

type OpenAIResponsesInput struct {
	Request   *http.Request
	Body      []byte
	Transport domain.TransportKind
	Subject   domain.Subject
}

func BuildOpenAIResponses(input OpenAIResponsesInput) (domain.IngressRequest, error) {
	if input.Request == nil {
		return domain.IngressRequest{}, errors.New("openai responses ingress: request is nil")
	}

	requestID := input.Request.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = uuid.NewString()
	}

	req := input.Request
	return domain.IngressRequest{
		RequestID: requestID,
		Endpoint:  domain.EndpointOpenAIResponses,
		Platform:  domain.PlatformOpenAI,
		Transport: input.Transport,
		Method:    req.Method,
		Path:      req.URL.Path,
		Subpath:   openAIResponsesSubpath(req.URL.Path),
		Header:    req.Header.Clone(),
		Body:      append([]byte(nil), input.Body...),
	}, nil
}

func openAIResponsesSubpath(path string) string {
	for _, prefix := range []string{openAIResponsesAliasPrefix, openAIResponsesPrefix} {
		if path == prefix {
			return ""
		}
		if strings.HasPrefix(path, prefix+"/") {
			return strings.TrimPrefix(path, prefix)
		}
	}
	return ""
}
