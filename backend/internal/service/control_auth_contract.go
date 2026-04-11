package service

import "context"

const (
	ControlAuthProviderLocal = "local"
)

// ControlAuthCapabilities describes which auth features are currently owned by the control plane.
// Future external IdP adapters can return different capability sets without changing the frontend API.
type ControlAuthCapabilities struct {
	Provider              string `json:"provider"`
	PasswordLoginEnabled  bool   `json:"password_login_enabled"`
	PasswordResetEnabled  bool   `json:"password_reset_enabled"`
	MFASelfServiceEnabled bool   `json:"mfa_self_service_enabled"`
}

// ControlAccessTokenAuthenticator verifies a control-plane browser token and returns internal identity.
type ControlAccessTokenAuthenticator interface {
	AuthenticateAccessToken(ctx context.Context, tokenString string) (*AuthenticatedIdentity, error)
}

// ControlSessionAuthService captures the session and external-provider-facing auth concerns.
// Future Auth0/Clerk-style adapters should primarily target this interface.
type ControlSessionAuthService interface {
	ControlAccessTokenAuthenticator
	JWKS() *ControlJWKS
	AuthCapabilities(ctx context.Context) *ControlAuthCapabilities
	Login(ctx context.Context, email, password, turnstileToken, remoteIP string) (*ControlLoginResult, error)
	CompleteLoginTOTP(ctx context.Context, challengeID, totpCode string) (*ControlLoginResult, error)
	LogoutSession(ctx context.Context, sessionID string) error
	LogoutAllSessions(ctx context.Context, identity *AuthenticatedIdentity) error
	RefreshSession(ctx context.Context, rawRefreshToken string) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	RotateCurrentSession(ctx context.Context, identity *AuthenticatedIdentity, amr string) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	CreateAuthFlow(ctx context.Context, provider, purpose, issuer, redirectTo string, codeVerifier, nonce *string) (*AuthFlowRecord, string, error)
	ConsumeAuthFlow(ctx context.Context, flowID, state string) (*AuthFlowRecord, error)
	HandleOAuthLogin(ctx context.Context, input *ControlOAuthLoginInput) (*ControlOAuthLoginResult, error)
}

// ControlLocalIdentityService captures the currently local-only registration and credential lifecycle.
// These behaviors may be disabled or replaced when an external IdP owns passwords, reset, or signup.
type ControlLocalIdentityService interface {
	RegistrationPreflight(ctx context.Context, email, promoCode, invitationCode string) (*RegistrationPreflightResult, error)
	SendRegistrationEmailCode(ctx context.Context, email string) error
	Register(ctx context.Context, input *ControlRegistrationInput) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	CompleteOAuthRegistration(ctx context.Context, challengeID, invitationCode string) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, email, token, newPassword string) error
	UpdateProfile(ctx context.Context, identity *AuthenticatedIdentity, username *string) (*AuthenticatedIdentity, error)
	ChangePassword(ctx context.Context, identity *AuthenticatedIdentity, currentPassword, newPassword string) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	GetRegistrationChallenge(ctx context.Context, challengeID string) (*RegistrationChallengeRecord, error)
}

// ControlBrowserAuthService is the current aggregate contract backed by the local auth runtime.
type ControlBrowserAuthService interface {
	ControlSessionAuthService
	ControlLocalIdentityService
}
