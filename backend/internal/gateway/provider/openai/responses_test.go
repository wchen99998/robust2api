package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

func TestParseResponsesValidHTTP(t *testing.T) {
	body := []byte(`{ "model":"gpt-5.1", "stream":true, "previous_response_id":"resp_123", "service_tier":"priority" }`)
	headers := http.Header{}
	headers.Set("Session-Id", "session-1")
	headers.Set("Authorization", "Bearer secret")

	got, err := ParseResponses(domain.IngressRequest{
		Header: headers,
		Body:   body,
	})
	require.NoError(t, err)

	require.Equal(t, "gpt-5.1", got.Canonical.RequestedModel)
	require.Equal(t, "gpt-5.1", got.Canonical.Model.Requested)
	require.Equal(t, "gpt-5.1", got.Canonical.Model.Canonical)
	require.True(t, got.Stream)
	require.False(t, got.Compact)
	require.Equal(t, "resp_123", got.PreviousResponseID)
	require.Equal(t, PreviousResponseKindResponse, got.PreviousResponseKind)
	require.Equal(t, "priority", got.ServiceTier)
	require.Equal(t, domain.SessionSourceHeader, got.Canonical.Session.Source)
	require.Equal(t, "session-1", got.Canonical.Session.Key)
	require.Equal(t, "Bearer secret", got.Canonical.Headers.Get("Authorization"))
	require.JSONEq(t, string(body), string(got.NormalizedBody))
	require.JSONEq(t, string(body), string(got.Canonical.Body))
	require.Equal(t, sha256Hex(body), got.BodySHA256)

	headers.Set("Authorization", "Bearer changed")
	body[0] = '['
	require.Equal(t, "Bearer secret", got.Canonical.Headers.Get("Authorization"))
	require.NotEqual(t, '[', got.NormalizedBody[0])
	require.NotEqual(t, '[', got.Canonical.Body[0])
}

func TestParseResponsesCompactNormalizesBody(t *testing.T) {
	body := []byte(`{
		"model":{"value":"gpt-5.1"},
		"input":{"value":"hello"},
		"instructions":{"value":"be brief"},
		"previous_response_id":{"value":"resp_abc"},
		"prompt_cache_key":" compact-seed "
	}`)

	got, err := ParseResponses(domain.IngressRequest{
		Subpath: "/compact",
		Body:    body,
	})
	require.NoError(t, err)

	require.True(t, got.Compact)
	require.Equal(t, "compact-seed", got.CompactSessionSeed)
	require.Equal(t, "gpt-5.1", got.Canonical.RequestedModel)
	require.Equal(t, "resp_abc", got.PreviousResponseID)
	require.Equal(t, PreviousResponseKindResponse, got.PreviousResponseKind)
	require.Equal(t, domain.SessionSourcePromptCacheKey, got.Canonical.Session.Source)
	require.Equal(t, "compact-seed", got.Canonical.Session.Key)
	require.JSONEq(t, `{
		"model":"gpt-5.1",
		"input":"hello",
		"instructions":"be brief",
		"previous_response_id":"resp_abc",
		"prompt_cache_key":" compact-seed "
	}`, string(got.NormalizedBody))
	require.JSONEq(t, string(got.NormalizedBody), string(got.Canonical.Body))
	require.Equal(t, sha256Hex(got.NormalizedBody), got.BodySHA256)
}

func TestParseResponsesCompactSubpathForms(t *testing.T) {
	for _, subpath := range []string{"/compact", "compact", "compact/detail", "/compact/detail"} {
		t.Run(subpath, func(t *testing.T) {
			got, err := ParseResponses(domain.IngressRequest{
				Subpath: subpath,
				Body:    []byte(`{"model":{"value":"gpt-5.1"}}`),
			})
			require.NoError(t, err)
			require.True(t, got.Compact)
			require.Equal(t, "gpt-5.1", got.Canonical.RequestedModel)
		})
	}
}

func TestParseResponsesRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		wantErr string
	}{
		{
			name:    "empty",
			body:    nil,
			wantErr: "Request body is empty",
		},
		{
			name:    "invalid json",
			body:    []byte(`{invalid json`),
			wantErr: "Failed to parse request body",
		},
		{
			name:    "missing model",
			body:    []byte(`{"stream":false}`),
			wantErr: "model is required",
		},
		{
			name:    "non-string model",
			body:    []byte(`{"model":123}`),
			wantErr: "model is required",
		},
		{
			name:    "blank model",
			body:    []byte(`{"model":"   "}`),
			wantErr: "model is required",
		},
		{
			name:    "invalid stream",
			body:    []byte(`{"model":"gpt-5.1","stream":"true"}`),
			wantErr: "invalid stream field type",
		},
		{
			name:    "message previous response id",
			body:    []byte(`{"model":"gpt-5.1","previous_response_id":"msg_123"}`),
			wantErr: "previous_response_id must be a response.id (resp_*), not a message id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseResponses(domain.IngressRequest{Body: tt.body})
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestClassifyPreviousResponseID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want PreviousResponseKind
	}{
		{
			name: "valid response",
			id:   "resp_abc",
			want: PreviousResponseKindResponse,
		},
		{
			name: "malformed response prefix",
			id:   "resp_",
			want: PreviousResponseKindOther,
		},
		{
			name: "message prefix",
			id:   "message_abc",
			want: PreviousResponseKindMessage,
		},
		{
			name: "item prefix",
			id:   "item_abc",
			want: PreviousResponseKindMessage,
		},
		{
			name: "chat completion prefix",
			id:   "chatcmpl_abc",
			want: PreviousResponseKindMessage,
		},
		{
			name: "uppercase message prefix",
			id:   "MSG_abc",
			want: PreviousResponseKindMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyPreviousResponseID(tt.id))
		})
	}
}

func TestParseResponsesPreviousResponseIDClassification(t *testing.T) {
	got, err := ParseResponses(domain.IngressRequest{
		Body: []byte(`{"model":"gpt-5.1","previous_response_id":"resp_"}`),
	})
	require.NoError(t, err)
	require.Equal(t, "resp_", got.PreviousResponseID)
	require.Equal(t, PreviousResponseKindOther, got.PreviousResponseKind)

	for _, previousID := range []string{"message_abc", "item_abc", "chatcmpl_abc", "MSG_abc"} {
		t.Run(previousID, func(t *testing.T) {
			_, err := ParseResponses(domain.IngressRequest{
				Body: []byte(`{"model":"gpt-5.1","previous_response_id":"` + previousID + `"}`),
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "previous_response_id must be a response.id (resp_*), not a message id")
		})
	}
}

func TestParseResponsesSessionPriority(t *testing.T) {
	tests := []struct {
		name       string
		headers    http.Header
		compact    string
		previousID string
		wantSource domain.SessionSource
		wantKey    string
	}{
		{
			name: "session header first",
			headers: http.Header{
				"Session-Id":      []string{" session-1 "},
				"Conversation-Id": []string{"conversation-1"},
			},
			compact:    "compact-1",
			previousID: "resp_1",
			wantSource: domain.SessionSourceHeader,
			wantKey:    "session-1",
		},
		{
			name: "conversation header second",
			headers: http.Header{
				"Conversation-Id": []string{" conversation-1 "},
			},
			compact:    "compact-1",
			previousID: "resp_1",
			wantSource: domain.SessionSourceHeader,
			wantKey:    "conversation-1",
		},
		{
			name:       "compact seed third",
			headers:    http.Header{},
			compact:    "compact-1",
			previousID: "resp_1",
			wantSource: domain.SessionSourcePromptCacheKey,
			wantKey:    "compact-1",
		},
		{
			name:       "previous response fourth",
			headers:    http.Header{},
			previousID: "resp_1",
			wantSource: domain.SessionSourcePreviousResponseID,
			wantKey:    "resp_1",
		},
		{
			name:       "none",
			headers:    http.Header{},
			wantSource: domain.SessionSourceNone,
			wantKey:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"model":"gpt-5.1"`
			if tt.compact != "" {
				body += `,"prompt_cache_key":"` + tt.compact + `"`
			}
			if tt.previousID != "" {
				body += `,"previous_response_id":"` + tt.previousID + `"`
			}
			body += `}`

			got, err := ParseResponses(domain.IngressRequest{
				Subpath: "/compact",
				Header:  tt.headers,
				Body:    []byte(body),
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantSource, got.Canonical.Session.Source)
			require.Equal(t, tt.wantKey, got.Canonical.Session.Key)
		})
	}
}

func TestParseResponsesExtractsReasoningEffortAndClassifiesOtherPreviousID(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","previous_response_id":"legacy-1","reasoning":{"effort":"high"}}`)

	got, err := ParseResponses(domain.IngressRequest{Body: body})
	require.NoError(t, err)

	require.Equal(t, "high", got.ReasoningEffort)
	require.Equal(t, "legacy-1", got.PreviousResponseID)
	require.Equal(t, PreviousResponseKindOther, got.PreviousResponseKind)
	require.Equal(t, domain.SessionSourcePreviousResponseID, got.Canonical.Session.Source)
	require.Equal(t, sha256Hex(body), got.BodySHA256)
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return strings.ToLower(hex.EncodeToString(sum[:]))
}
