package domain

import (
	"encoding/json"
	"net/http"
)

type CanonicalRequest struct {
	RequestedModel string          `json:"requested_model,omitempty"`
	Headers        http.Header     `json:"headers,omitempty"`
	Body           json.RawMessage `json:"body,omitempty"`
	Model          ModelResolution `json:"model"`
	Session        SessionInput    `json:"session"`
	Mutation       RequestMutation `json:"mutation"`
}

type ModelResolution struct {
	Requested     string             `json:"requested,omitempty"`
	Canonical     string             `json:"canonical,omitempty"`
	Upstream      string             `json:"upstream,omitempty"`
	Billing       string             `json:"billing,omitempty"`
	BillingSource BillingModelSource `json:"billing_source,omitempty"`
}

type BillingModelSource string

const (
	BillingModelSourceRequested     BillingModelSource = "requested"
	BillingModelSourceChannelMapped BillingModelSource = "channel_mapped"
	BillingModelSourceAccountMapped BillingModelSource = "account_mapped"
	BillingModelSourceUpstream      BillingModelSource = "upstream"
)

type SessionInput struct {
	Key    string        `json:"key,omitempty"`
	Source SessionSource `json:"source"`
}

type SessionSource string

const (
	SessionSourceNone               SessionSource = "none"
	SessionSourceHeader             SessionSource = "header"
	SessionSourcePromptCacheKey     SessionSource = "prompt_cache_key"
	SessionSourcePreviousResponseID SessionSource = "previous_response_id"
)

type SessionDecision struct {
	Enabled   bool          `json:"enabled"`
	Key       string        `json:"key,omitempty"`
	Source    SessionSource `json:"source"`
	Sticky    bool          `json:"sticky"`
	AccountID int64         `json:"account_id,omitempty"`
}

type RequestMutation struct {
	TargetPath string      `json:"target_path,omitempty"`
	Headers    http.Header `json:"headers,omitempty"`
	Model      string      `json:"model,omitempty"`
	Streaming  *bool       `json:"streaming,omitempty"`
}
