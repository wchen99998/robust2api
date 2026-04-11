//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type authBFFSettingRepoStub struct {
	values map[string]string
}

func (s *authBFFSettingRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	if value, ok := s.values[key]; ok {
		return &service.Setting{Key: key, Value: value}, nil
	}
	return nil, service.ErrSettingNotFound
}

func (s *authBFFSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}

func (s *authBFFSettingRepoStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *authBFFSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *authBFFSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *authBFFSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *authBFFSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

type authBFFProviderRegistryStub struct {
	providers []service.ControlAuthProviderDescriptor
}

func (s *authBFFProviderRegistryStub) Providers(_ context.Context) []service.ControlAuthProviderDescriptor {
	return append([]service.ControlAuthProviderDescriptor(nil), s.providers...)
}

func (s *authBFFProviderRegistryStub) StartLogin(_ context.Context, _ *service.ControlExternalLoginStartRequest) (*service.ControlExternalLoginStartResult, error) {
	return nil, errors.New("not implemented")
}

func (s *authBFFProviderRegistryStub) CompleteLogin(_ context.Context, _ *service.ControlExternalLoginCallbackRequest) (*service.ControlExternalLoginCallbackResult, error) {
	return nil, errors.New("not implemented")
}

type authBFFLocalMFAStub struct{}

func (authBFFLocalMFAStub) GetStatus(_ context.Context, _ int64) (*service.TotpStatus, error) {
	return &service.TotpStatus{}, nil
}

func (authBFFLocalMFAStub) SendVerifyCode(_ context.Context, _ int64) error {
	return nil
}

func (authBFFLocalMFAStub) InitiateSetup(_ context.Context, _ int64, _, _ string) (*service.TotpSetupResponse, error) {
	return &service.TotpSetupResponse{}, nil
}

func (authBFFLocalMFAStub) CompleteSetup(_ context.Context, _ int64, _, _ string) error {
	return nil
}

func (authBFFLocalMFAStub) Disable(_ context.Context, _ int64, _, _ string) error {
	return nil
}

func (authBFFLocalMFAStub) GetVerificationMethod(_ context.Context) *service.VerificationMethod {
	return &service.VerificationMethod{Method: "email"}
}

type authBFFSessionAuthStub struct {
	capabilities *service.ControlAuthCapabilities
}

func (s *authBFFSessionAuthStub) AuthenticateAccessToken(_ context.Context, _ string) (*service.AuthenticatedIdentity, error) {
	return nil, service.ErrInvalidToken
}

func (s *authBFFSessionAuthStub) JWKS() *service.ControlJWKS {
	return &service.ControlJWKS{}
}

func (s *authBFFSessionAuthStub) AuthCapabilities(_ context.Context) *service.ControlAuthCapabilities {
	return s.capabilities
}

func (s *authBFFSessionAuthStub) LogoutSession(_ context.Context, _ string) error {
	return nil
}

func (s *authBFFSessionAuthStub) LogoutAllSessions(_ context.Context, _ *service.AuthenticatedIdentity) error {
	return nil
}

func (s *authBFFSessionAuthStub) RefreshSession(_ context.Context, _ string) (*service.AuthenticatedIdentity, *service.ControlSessionTokens, error) {
	return nil, nil, errors.New("not implemented")
}

func (s *authBFFSessionAuthStub) RotateCurrentSession(_ context.Context, _ *service.AuthenticatedIdentity, _ string) (*service.AuthenticatedIdentity, *service.ControlSessionTokens, error) {
	return nil, nil, errors.New("not implemented")
}

func (s *authBFFSessionAuthStub) IssueEmbedToken(_ context.Context, _ *service.AuthenticatedIdentity) (*service.ControlEmbedToken, error) {
	return nil, errors.New("not implemented")
}

func (s *authBFFSessionAuthStub) CreateAuthFlow(_ context.Context, _, _, _, _ string, _ *string, _ *string) (*service.AuthFlowRecord, string, error) {
	return nil, "", errors.New("not implemented")
}

func (s *authBFFSessionAuthStub) ConsumeAuthFlow(_ context.Context, _, _ string) (*service.AuthFlowRecord, error) {
	return nil, errors.New("not implemented")
}

type authBFFLocalCredentialStub struct {
	loginErr         error
	passwordResetErr error
}

func (s *authBFFLocalCredentialStub) Login(_ context.Context, _, _, _, _ string) (*service.ControlLoginResult, error) {
	return nil, s.loginErr
}

func (s *authBFFLocalCredentialStub) CompleteLoginTOTP(_ context.Context, _, _ string) (*service.ControlLoginResult, error) {
	return nil, errors.New("not implemented")
}

func (s *authBFFLocalCredentialStub) RequestPasswordReset(_ context.Context, _ string) error {
	return s.passwordResetErr
}

func (s *authBFFLocalCredentialStub) ResetPassword(_ context.Context, _, _, _ string) error {
	return errors.New("not implemented")
}

func (s *authBFFLocalCredentialStub) ChangePassword(_ context.Context, _ *service.AuthenticatedIdentity, _, _ string) (*service.AuthenticatedIdentity, *service.ControlSessionTokens, error) {
	return nil, nil, errors.New("not implemented")
}

type authBFFRegistrationStub struct {
	emailCodeErr error
}

func (s *authBFFRegistrationStub) RegistrationPreflight(_ context.Context, _, _, _ string) (*service.RegistrationPreflightResult, error) {
	return nil, errors.New("not implemented")
}

func (s *authBFFRegistrationStub) SendRegistrationEmailCode(_ context.Context, _ string) error {
	return s.emailCodeErr
}

func (s *authBFFRegistrationStub) Register(_ context.Context, _ *service.ControlRegistrationInput) (*service.AuthenticatedIdentity, *service.ControlSessionTokens, error) {
	return nil, nil, errors.New("not implemented")
}

func (s *authBFFRegistrationStub) CompleteExternalLogin(_ context.Context, _ *service.ControlExternalLoginRequest) (*service.ControlExternalLoginResult, error) {
	return nil, errors.New("not implemented")
}

func (s *authBFFRegistrationStub) CompleteOAuthRegistration(_ context.Context, _, _ string) (*service.AuthenticatedIdentity, *service.ControlSessionTokens, error) {
	return nil, nil, errors.New("not implemented")
}

func (s *authBFFRegistrationStub) GetRegistrationChallenge(_ context.Context, _ string) (*service.RegistrationChallengeRecord, error) {
	return nil, service.ErrRegistrationChallengeNotFound
}

func authCapabilitiesForMode(mode string, settings map[string]string) *service.ControlAuthCapabilities {
	localMode := mode == service.ControlAuthModeLocal
	registrationEnabled := localMode && settings[service.SettingKeyRegistrationEnabled] == "true"
	emailVerifyEnabled := localMode && settings[service.SettingKeyEmailVerifyEnabled] == "true"
	passwordResetEnabled := localMode && settings[service.SettingKeyPasswordResetEnabled] == "true"
	totpEnabled := localMode && settings[service.SettingKeyTotpEnabled] == "true"

	return &service.ControlAuthCapabilities{
		Provider:                  mode,
		PasswordLoginEnabled:      localMode,
		RegistrationEnabled:       registrationEnabled,
		EmailVerificationEnabled:  emailVerifyEnabled,
		PasswordResetEnabled:      passwordResetEnabled,
		PasswordChangeEnabled:     localMode,
		MFASelfServiceEnabled:     totpEnabled,
		ProfileSelfServiceEnabled: true,
	}
}

func newAuthBFFTestHandler(mode string, settingValues map[string]string, providers []service.ControlAuthProviderDescriptor) *AuthHandler {
	cfg := &config.Config{
		RunMode: config.RunModeStandard,
		ControlAuth: config.ControlAuthConfig{
			Mode: mode,
		},
	}

	var settingSvc *service.SettingService
	if settingValues != nil {
		settingSvc = service.NewSettingService(&authBFFSettingRepoStub{values: settingValues}, cfg)
	}

	return &AuthHandler{
		cfg:                cfg,
		controlSessionAuth: &authBFFSessionAuthStub{capabilities: authCapabilitiesForMode(mode, settingValues)},
		controlLocalCredentials: &authBFFLocalCredentialStub{
			loginErr:         service.ErrPasswordLoginDisabled,
			passwordResetErr: service.ErrPasswordResetDisabled,
		},
		controlRegistration: &authBFFRegistrationStub{
			emailCodeErr: service.ErrRegDisabled,
		},
		controlLocalMFA: authBFFLocalMFAStub{},
		settingSvc:      settingSvc,
		externalIdentityProviders: &authBFFProviderRegistryStub{
			providers: providers,
		},
	}
}

func newHandlerTestContext(method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	return c, rec
}

func decodeHandlerResponse(t *testing.T, rec *httptest.ResponseRecorder) response.Response {
	t.Helper()
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func TestBootstrap_ExposesAuthoritativeAuthCapabilitiesAndProviders(t *testing.T) {
	handler := newAuthBFFTestHandler(service.ControlAuthModeLocal, map[string]string{
		service.SettingKeyRegistrationEnabled:  "true",
		service.SettingKeyEmailVerifyEnabled:   "true",
		service.SettingKeyPasswordResetEnabled: "true",
		service.SettingKeyTotpEnabled:          "true",
		service.SettingKeySiteName:             "Sub2API",
		service.SettingKeyBackendModeEnabled:   "false",
	}, []service.ControlAuthProviderDescriptor{
		{
			ID:          "oidc",
			Type:        service.ControlAuthProviderTypeOIDC,
			DisplayName: "Auth0",
			StartPath:   "/api/v1/oauth/oidc/start",
		},
	})

	c, rec := newHandlerTestContext(http.MethodGet, "/api/v1/bootstrap", "")
	handler.Bootstrap(c)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeHandlerResponse(t, rec)
	payload, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	caps, ok := payload["auth_capabilities"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.ControlAuthModeLocal, caps["provider"])
	require.Equal(t, true, caps["password_login_enabled"])
	require.Equal(t, true, caps["registration_enabled"])
	require.Equal(t, true, caps["email_verification_enabled"])
	require.Equal(t, true, caps["password_reset_enabled"])
	require.Equal(t, true, caps["password_change_enabled"])
	require.Equal(t, true, caps["mfa_self_service_enabled"])
	require.Equal(t, true, caps["profile_self_service_enabled"])

	providers, ok := payload["auth_providers"].([]any)
	require.True(t, ok)
	require.Len(t, providers, 1)
	firstProvider, ok := providers[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "oidc", firstProvider["id"])
	require.Equal(t, "/api/v1/oauth/oidc/start", firstProvider["start_path"])
}

func TestSessionLogin_DisabledCredentialModesReturnForbidden(t *testing.T) {
	handler := newAuthBFFTestHandler(service.ControlAuthModeExternalOIDC, nil, nil)

	c, rec := newHandlerTestContext(http.MethodPost, "/api/v1/session/login", `{"email":"user@example.com","password":"secret123"}`)
	handler.SessionLogin(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	resp := decodeHandlerResponse(t, rec)
	require.Equal(t, "PASSWORD_LOGIN_DISABLED", resp.Reason)
}

func TestRegistrationEmailCode_DisabledCredentialModesReturnForbidden(t *testing.T) {
	handler := newAuthBFFTestHandler(service.ControlAuthModeExternalOIDC, nil, nil)

	c, rec := newHandlerTestContext(http.MethodPost, "/api/v1/registration/email-code", `{"email":"user@example.com"}`)
	handler.RegistrationEmailCode(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	resp := decodeHandlerResponse(t, rec)
	require.Equal(t, "REGISTRATION_DISABLED", resp.Reason)
}

func TestPasswordForgot_DisabledCredentialModesReturnForbidden(t *testing.T) {
	handler := newAuthBFFTestHandler(service.ControlAuthModeExternalOIDC, nil, nil)

	c, rec := newHandlerTestContext(http.MethodPost, "/api/v1/password/forgot", `{"email":"user@example.com"}`)
	handler.PasswordForgot(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	resp := decodeHandlerResponse(t, rec)
	require.Equal(t, "PASSWORD_RESET_DISABLED", resp.Reason)
}

func TestGetMyTOTP_DisabledCredentialModesReturnForbidden(t *testing.T) {
	handler := newAuthBFFTestHandler(service.ControlAuthModeExternalOIDC, nil, nil)

	c, rec := newHandlerTestContext(http.MethodGet, "/api/v1/me/mfa/totp", "")
	handler.GetMyTOTP(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	resp := decodeHandlerResponse(t, rec)
	require.Equal(t, "MFA_SELF_SERVICE_DISABLED", resp.Reason)
}
