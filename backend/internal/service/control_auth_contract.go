package service

import "context"

const (
	ControlAuthModeLocal         = "local"
	ControlAuthModeExternalOIDC  = "external_oidc"
	ControlAuthModeExternalAuth0 = "external_auth0"
	ControlAuthModeExternalClerk = "external_clerk"

	ControlAuthProviderLocal = "local"

	ControlAuthProviderTypeOIDC  = "oidc"
	ControlAuthProviderTypeOAuth = "oauth"
	ControlAuthProviderTypeClerk = "clerk"
)

// ControlAuthCapabilities describes which auth features are currently owned by the control plane.
// Future external IdP adapters can return different capability sets without changing the frontend API.
type ControlAuthCapabilities struct {
	Provider                  string `json:"provider"`
	PasswordLoginEnabled      bool   `json:"password_login_enabled"`
	RegistrationEnabled       bool   `json:"registration_enabled"`
	EmailVerificationEnabled  bool   `json:"email_verification_enabled"`
	PasswordResetEnabled      bool   `json:"password_reset_enabled"`
	PasswordChangeEnabled     bool   `json:"password_change_enabled"`
	MFASelfServiceEnabled     bool   `json:"mfa_self_service_enabled"`
	ProfileSelfServiceEnabled bool   `json:"profile_self_service_enabled"`
}

type ControlAuthProviderDescriptor struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	StartPath   string `json:"start_path"`
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
	LogoutSession(ctx context.Context, sessionID string) error
	LogoutAllSessions(ctx context.Context, identity *AuthenticatedIdentity) error
	RefreshSession(ctx context.Context, rawRefreshToken string) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	RotateCurrentSession(ctx context.Context, identity *AuthenticatedIdentity, amr string) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	IssueEmbedToken(ctx context.Context, identity *AuthenticatedIdentity) (*ControlEmbedToken, error)
	CreateAuthFlow(ctx context.Context, provider, purpose, issuer, redirectTo string, codeVerifier, nonce *string) (*AuthFlowRecord, string, error)
	ConsumeAuthFlow(ctx context.Context, flowID, state string) (*AuthFlowRecord, error)
}

type ControlLocalCredentialService interface {
	Login(ctx context.Context, email, password, turnstileToken, remoteIP string) (*ControlLoginResult, error)
	CompleteLoginTOTP(ctx context.Context, challengeID, totpCode string) (*ControlLoginResult, error)
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, email, token, newPassword string) error
	ChangePassword(ctx context.Context, identity *AuthenticatedIdentity, currentPassword, newPassword string) (*AuthenticatedIdentity, *ControlSessionTokens, error)
}

// ControlRegistrationService captures local registration and external-login completion provisioning.
type ControlRegistrationService interface {
	RegistrationPreflight(ctx context.Context, email, promoCode, invitationCode string) (*RegistrationPreflightResult, error)
	SendRegistrationEmailCode(ctx context.Context, email string) error
	Register(ctx context.Context, input *ControlRegistrationInput) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	CompleteExternalLogin(ctx context.Context, input *ControlExternalLoginRequest) (*ControlExternalLoginResult, error)
	CompleteOAuthRegistration(ctx context.Context, challengeID, invitationCode string) (*AuthenticatedIdentity, *ControlSessionTokens, error)
	GetRegistrationChallenge(ctx context.Context, challengeID string) (*RegistrationChallengeRecord, error)
}

type ControlProfileService interface {
	UpdateProfile(ctx context.Context, identity *AuthenticatedIdentity, username *string) (*AuthenticatedIdentity, error)
}

type ControlLocalMFAService interface {
	GetStatus(ctx context.Context, userID int64) (*TotpStatus, error)
	SendVerifyCode(ctx context.Context, userID int64) error
	InitiateSetup(ctx context.Context, userID int64, emailCode, password string) (*TotpSetupResponse, error)
	CompleteSetup(ctx context.Context, userID int64, totpCode, setupToken string) error
	Disable(ctx context.Context, userID int64, emailCode, password string) error
	GetVerificationMethod(ctx context.Context) *VerificationMethod
}

type ExternalIdentityProviderRegistry interface {
	Providers(ctx context.Context) []ControlAuthProviderDescriptor
	StartLogin(ctx context.Context, input *ControlExternalLoginStartRequest) (*ControlExternalLoginStartResult, error)
	CompleteLogin(ctx context.Context, input *ControlExternalLoginCallbackRequest) (*ControlExternalLoginCallbackResult, error)
}

// ControlBrowserAuthService is the aggregate contract backed by the local auth runtime.
type ControlBrowserAuthService interface {
	ControlSessionAuthService
	ControlLocalCredentialService
	ControlRegistrationService
	ControlProfileService
}
