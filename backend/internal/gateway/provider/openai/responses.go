package openai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

type PreviousResponseKind string

const (
	PreviousResponseKindNone     PreviousResponseKind = "none"
	PreviousResponseKindResponse PreviousResponseKind = "response"
	PreviousResponseKindMessage  PreviousResponseKind = "message"
	PreviousResponseKindOther    PreviousResponseKind = "other"
)

type ResponsesParseResult struct {
	Canonical            domain.CanonicalRequest
	NormalizedBody       []byte
	Stream               bool
	Compact              bool
	CompactSessionSeed   string
	PreviousResponseID   string
	PreviousResponseKind PreviousResponseKind
	ServiceTier          string
	ReasoningEffort      string
	BodySHA256           string
}

func ParseResponses(req domain.IngressRequest) (ResponsesParseResult, error) {
	if len(bytes.TrimSpace(req.Body)) == 0 {
		return ResponsesParseResult{}, errors.New("Request body is empty")
	}
	if !gjson.ValidBytes(req.Body) {
		return ResponsesParseResult{}, errors.New("Failed to parse request body")
	}

	compact := isCompactSubpath(req.Subpath)
	normalizedBody, changed, err := normalizeCompactBody(req.Body, compact)
	if err != nil {
		return ResponsesParseResult{}, fmt.Errorf("Failed to parse request body: %w", err)
	}
	if !changed {
		normalizedBody = append([]byte(nil), req.Body...)
	}

	modelResult := gjson.GetBytes(normalizedBody, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || strings.TrimSpace(modelResult.String()) == "" {
		return ResponsesParseResult{}, errors.New("model is required")
	}
	requestedModel := strings.TrimSpace(modelResult.String())

	streamResult := gjson.GetBytes(normalizedBody, "stream")
	if streamResult.Exists() && streamResult.Type != gjson.True && streamResult.Type != gjson.False {
		return ResponsesParseResult{}, errors.New("invalid stream field type")
	}
	stream := streamResult.Bool()

	compactSessionSeed := strings.TrimSpace(gjson.GetBytes(normalizedBody, "prompt_cache_key").String())
	previousResponseID := strings.TrimSpace(gjson.GetBytes(normalizedBody, "previous_response_id").String())
	previousResponseKind := classifyPreviousResponseID(previousResponseID)
	if previousResponseKind == PreviousResponseKindMessage {
		return ResponsesParseResult{}, errors.New("previous_response_id must be a response.id (resp_*), not a message id")
	}

	bodySHA256 := sha256.Sum256(normalizedBody)
	headers := cloneHeaders(req.Header)
	canonicalBody := append([]byte(nil), normalizedBody...)

	return ResponsesParseResult{
		Canonical: domain.CanonicalRequest{
			RequestedModel: requestedModel,
			Headers:        headers,
			Body:           canonicalBody,
			Model: domain.ModelResolution{
				Requested: requestedModel,
				Canonical: requestedModel,
			},
			Session: resolveSession(headers, compactSessionSeed, previousResponseID),
		},
		NormalizedBody:       append([]byte(nil), normalizedBody...),
		Stream:               stream,
		Compact:              compact,
		CompactSessionSeed:   compactSessionSeed,
		PreviousResponseID:   previousResponseID,
		PreviousResponseKind: previousResponseKind,
		ServiceTier:          strings.TrimSpace(gjson.GetBytes(normalizedBody, "service_tier").String()),
		ReasoningEffort:      strings.TrimSpace(gjson.GetBytes(normalizedBody, "reasoning.effort").String()),
		BodySHA256:           hex.EncodeToString(bodySHA256[:]),
	}, nil
}

func isCompactSubpath(subpath string) bool {
	normalized := strings.TrimPrefix(strings.TrimSpace(subpath), "/")
	return normalized == "compact" || strings.HasPrefix(normalized, "compact/")
}

func normalizeCompactBody(body []byte, compact bool) ([]byte, bool, error) {
	if !compact {
		return nil, false, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, false, err
	}

	changed := false
	for _, key := range []string{"model", "input", "instructions", "previous_response_id"} {
		raw, ok := object[key]
		if !ok {
			continue
		}

		value, ok, err := compactValue(raw)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		object[key] = value
		changed = true
	}

	if !changed {
		return nil, false, nil
	}

	normalized, err := json.Marshal(object)
	if err != nil {
		return nil, false, err
	}
	return normalized, true, nil
}

func compactValue(raw json.RawMessage) (json.RawMessage, bool, error) {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, false, nil
	}

	value, ok := wrapper["value"]
	if !ok {
		return nil, false, nil
	}
	return append(json.RawMessage(nil), value...), true, nil
}

func classifyPreviousResponseID(previousResponseID string) PreviousResponseKind {
	switch {
	case previousResponseID == "":
		return PreviousResponseKindNone
	case strings.HasPrefix(previousResponseID, "resp_"):
		return PreviousResponseKindResponse
	case strings.HasPrefix(previousResponseID, "msg_"):
		return PreviousResponseKindMessage
	default:
		return PreviousResponseKindOther
	}
}

func resolveSession(headers http.Header, compactSeed, previousID string) domain.SessionInput {
	if sessionID := firstHeader(headers, "session_id", "session-id"); sessionID != "" {
		return domain.SessionInput{Key: sessionID, Source: domain.SessionSourceHeader}
	}
	if conversationID := firstHeader(headers, "conversation_id", "conversation-id"); conversationID != "" {
		return domain.SessionInput{Key: conversationID, Source: domain.SessionSourceHeader}
	}
	if compactSeed = strings.TrimSpace(compactSeed); compactSeed != "" {
		return domain.SessionInput{Key: compactSeed, Source: domain.SessionSourcePromptCacheKey}
	}
	if previousID = strings.TrimSpace(previousID); previousID != "" {
		return domain.SessionInput{Key: previousID, Source: domain.SessionSourcePreviousResponseID}
	}
	return domain.SessionInput{Source: domain.SessionSourceNone}
}

func firstHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func cloneHeaders(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	return headers.Clone()
}
