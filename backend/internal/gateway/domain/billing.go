package domain

type BillingMode string

const (
	BillingModeNone       BillingMode = "none"
	BillingModeToken      BillingMode = "token"
	BillingModeStreaming  BillingMode = "streaming"
	BillingModePerRequest BillingMode = "per_request"
	BillingModeImage      BillingMode = "image"
)

type BillingLifecyclePlan struct {
	Mode          BillingMode        `json:"mode"`
	Model         string             `json:"model,omitempty"`
	Multiplier    float64            `json:"multiplier,omitempty"`
	ReserveTokens int                `json:"reserve_tokens,omitempty"`
	Events        []BillingEventKind `json:"events,omitempty"`
}

type BillingEventKind string

const (
	BillingEventCharge   BillingEventKind = "charge"
	BillingEventReserve  BillingEventKind = "reserve"
	BillingEventFinalize BillingEventKind = "finalize"
	BillingEventRelease  BillingEventKind = "release"
)

type UsageEvent struct {
	Kind             BillingEventKind `json:"kind"`
	APIKeyID         int64            `json:"api_key_id"`
	AccountID        int64            `json:"account_id"`
	GroupID          int64            `json:"group_id"`
	Model            string           `json:"model,omitempty"`
	PromptTokens     int              `json:"prompt_tokens,omitempty"`
	CompletionTokens int              `json:"completion_tokens,omitempty"`
	TotalTokens      int              `json:"total_tokens,omitempty"`
	Cost             float64          `json:"cost,omitempty"`
}

type BillingExecutionReport struct {
	Mode        BillingMode        `json:"mode"`
	Events      []BillingEventKind `json:"events,omitempty"`
	Reserved    bool               `json:"reserved"`
	Finalized   bool               `json:"finalized"`
	Released    bool               `json:"released"`
	ChargedCost float64            `json:"charged_cost,omitempty"`
}
