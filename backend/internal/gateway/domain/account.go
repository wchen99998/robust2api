package domain

import "time"

type AccountSnapshot struct {
	ID               int64               `json:"id"`
	Name             string              `json:"name,omitempty"`
	Platform         Platform            `json:"platform"`
	Type             AccountType         `json:"type"`
	GroupIDs         []int64             `json:"group_ids,omitempty"`
	Priority         int                 `json:"priority,omitempty"`
	Concurrency      int                 `json:"concurrency,omitempty"`
	RateMultiplier   float64             `json:"rate_multiplier,omitempty"`
	Capabilities     AccountCapabilities `json:"capabilities"`
	RateLimitResetAt *time.Time          `json:"rate_limit_reset_at,omitempty"`
}

type AccountCapabilities struct {
	Models     []string        `json:"models,omitempty"`
	Transports []TransportKind `json:"transports,omitempty"`
	Streaming  bool            `json:"streaming,omitempty"`
	Tools      bool            `json:"tools,omitempty"`
	Vision     bool            `json:"vision,omitempty"`
}

type AccountDecisionLayer string

const (
	AccountDecisionNone               AccountDecisionLayer = "none"
	AccountDecisionPreviousResponseID AccountDecisionLayer = "previous_response_id"
	AccountDecisionSessionHash        AccountDecisionLayer = "session_hash"
	AccountDecisionLoadBalance        AccountDecisionLayer = "load_balance"
	AccountDecisionWaitPlan           AccountDecisionLayer = "wait_plan"
)

type AccountDecision struct {
	Layer       AccountDecisionLayer `json:"layer"`
	Account     AccountSnapshot      `json:"account"`
	Reservation AccountReservation   `json:"reservation"`
	WaitPlan    AccountWaitPlan      `json:"wait_plan"`
}

type AccountReservation struct {
	AccountID int64     `json:"account_id"`
	Token     string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AccountWaitPlan struct {
	Required bool          `json:"required"`
	Reason   string        `json:"reason,omitempty"`
	Timeout  time.Duration `json:"timeout"`
}
