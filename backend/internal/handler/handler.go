package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
)

// AdminHandlers contains all admin-related HTTP handlers
type AdminHandlers struct {
	Dashboard             *admin.DashboardHandler
	User                  *admin.UserHandler
	Group                 *admin.GroupHandler
	Account               *admin.AccountHandler
	Announcement          *admin.AnnouncementHandler
	OAuth                 *admin.OAuthHandler
	OpenAIOAuth           *admin.OpenAIOAuthHandler
	GeminiOAuth           *admin.GeminiOAuthHandler
	AntigravityOAuth      *admin.AntigravityOAuthHandler
	Proxy                 *admin.ProxyHandler
	Redeem                *admin.RedeemHandler
	Promo                 *admin.PromoHandler
	Setting               *admin.SettingHandler
	Subscription          *admin.SubscriptionHandler
	Usage                 *admin.UsageHandler
	UserAttribute         *admin.UserAttributeHandler
	ErrorPassthrough      *admin.ErrorPassthroughHandler
	TLSFingerprintProfile *admin.TLSFingerprintProfileHandler
	APIKey                *admin.AdminAPIKeyHandler
	ScheduledTest         *admin.ScheduledTestHandler
	Channel               *admin.ChannelHandler
}

// GatewayHandlers contains only gateway-facing HTTP handlers.
type GatewayHandlers struct {
	CoreGateway *CoreGatewayHandler
}

// ControlHandlers contains control-plane HTTP handlers.
type ControlHandlers struct {
	Auth         *AuthHandler
	User         *UserHandler
	APIKey       *APIKeyHandler
	Usage        *UsageHandler
	Redeem       *RedeemHandler
	Subscription *SubscriptionHandler
	Announcement *AnnouncementHandler
	Admin        *AdminHandlers
	Setting      *SettingHandler
	Totp         *TotpHandler
}
