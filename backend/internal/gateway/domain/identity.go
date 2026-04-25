package domain

type Subject struct {
	APIKey APIKeySnapshot `json:"api_key"`
	User   UserSnapshot   `json:"user"`
	Group  GroupSnapshot  `json:"group"`
}

type APIKeySnapshot struct {
	ID        int64       `json:"id"`
	KeyID     string      `json:"key_id"`
	Name      string      `json:"name,omitempty"`
	UserID    int64       `json:"user_id"`
	GroupID   int64       `json:"group_id"`
	Quota     float64     `json:"quota,omitempty"`
	UsedQuota float64     `json:"used_quota,omitempty"`
	Policy    GroupPolicy `json:"policy"`
}

type UserSnapshot struct {
	ID       int64  `json:"id"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
}

type GroupSnapshot struct {
	ID   int64  `json:"id"`
	Name string `json:"name,omitempty"`
}

type RateLimitConfig struct {
	Limit5h float64 `json:"limit_5h,omitempty"`
	Limit1d float64 `json:"limit_1d,omitempty"`
	Limit7d float64 `json:"limit_7d,omitempty"`
}

type GroupPolicy struct {
	GroupID   int64           `json:"group_id"`
	RateLimit RateLimitConfig `json:"rate_limit"`
}
