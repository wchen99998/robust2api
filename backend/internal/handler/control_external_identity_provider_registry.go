package handler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type controlExternalIdentityProviderRegistry struct {
	cfg                *config.Config
	settingSvc         *service.SettingService
	controlSessionAuth service.ControlSessionAuthService
	controlRegistration service.ControlRegistrationService
}

func newControlExternalIdentityProviderRegistry(
	cfg *config.Config,
	settingSvc *service.SettingService,
	controlSessionAuth service.ControlSessionAuthService,
	controlRegistration service.ControlRegistrationService,
) service.ExternalIdentityProviderRegistry {
	if controlSessionAuth == nil || controlRegistration == nil {
		return nil
	}
	return &controlExternalIdentityProviderRegistry{
		cfg:                 cfg,
		settingSvc:          settingSvc,
		controlSessionAuth:  controlSessionAuth,
		controlRegistration: controlRegistration,
	}
}

func (r *controlExternalIdentityProviderRegistry) Providers(ctx context.Context) []service.ControlAuthProviderDescriptor {
	if r == nil {
		return nil
	}

	providers := make([]service.ControlAuthProviderDescriptor, 0, 2)
	if _, err := loadLinuxDoOAuthConfig(ctx, r.cfg, r.settingSvc); err == nil {
		providers = append(providers, service.ControlAuthProviderDescriptor{
			ID:          "linuxdo",
			Type:        service.ControlAuthProviderTypeOAuth,
			DisplayName: "Linux.do",
			StartPath:   "/api/v1/oauth/linuxdo/start",
		})
	}
	if cfg, err := loadOIDCOAuthConfig(ctx, r.cfg, r.settingSvc); err == nil {
		displayName := strings.TrimSpace(cfg.ProviderName)
		authMode := ""
		if r.cfg != nil {
			authMode = strings.TrimSpace(r.cfg.ControlAuth.Mode)
		}
		if displayName == "" && authMode == service.ControlAuthModeExternalAuth0 {
			displayName = "Auth0"
		}
		if displayName == "" {
			displayName = "OIDC"
		}
		providers = append(providers, service.ControlAuthProviderDescriptor{
			ID:          "oidc",
			Type:        service.ControlAuthProviderTypeOIDC,
			DisplayName: displayName,
			StartPath:   "/api/v1/oauth/oidc/start",
		})
	}
	return providers
}

func (r *controlExternalIdentityProviderRegistry) StartLogin(ctx context.Context, input *service.ControlExternalLoginStartRequest) (*service.ControlExternalLoginStartResult, error) {
	if r == nil || r.controlSessionAuth == nil {
		return nil, infraerrors.InternalServer("CONTROL_AUTH_NOT_READY", "control auth service is not configured")
	}
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_REQUEST", "external login request is required")
	}

	provider := strings.TrimSpace(input.Provider)
	redirectTo := sanitizeFrontendRedirectPath(input.RedirectTo)
	if redirectTo == "" {
		redirectTo = controlDefaultFrontendRedirect
	}

	switch provider {
	case "linuxdo":
		cfg, err := loadLinuxDoOAuthConfig(ctx, r.cfg, r.settingSvc)
		if err != nil {
			return nil, err
		}
		issuer := firstNonEmpty(strings.TrimSpace(cfg.AuthorizeURL), provider)

		var codeVerifier *string
		challenge := ""
		if cfg.UsePKCE {
			verifier, err := oauth.GenerateCodeVerifier()
			if err != nil {
				return nil, infraerrors.InternalServer("OAUTH_PKCE_GEN_FAILED", "failed to generate pkce verifier").WithCause(err)
			}
			challenge = oauth.GenerateCodeChallenge(verifier)
			codeVerifier = &verifier
		}

		flow, state, err := r.controlSessionAuth.CreateAuthFlow(ctx, provider, "login", issuer, redirectTo, codeVerifier, nil)
		if err != nil {
			return nil, err
		}
		authURL, err := buildLinuxDoAuthorizeURL(cfg, state, challenge, strings.TrimSpace(cfg.RedirectURL))
		if err != nil {
			return nil, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err)
		}
		return &service.ControlExternalLoginStartResult{
			AuthURL: authURL,
			FlowID:  flow.FlowID,
		}, nil
	case "oidc":
		cfg, err := loadOIDCOAuthConfig(ctx, r.cfg, r.settingSvc)
		if err != nil {
			return nil, err
		}
		issuer := firstNonEmpty(strings.TrimSpace(cfg.IssuerURL), provider)

		var codeVerifier *string
		challenge := ""
		if cfg.UsePKCE {
			verifier, err := oauth.GenerateCodeVerifier()
			if err != nil {
				return nil, infraerrors.InternalServer("OAUTH_PKCE_GEN_FAILED", "failed to generate pkce verifier").WithCause(err)
			}
			challenge = oauth.GenerateCodeChallenge(verifier)
			codeVerifier = &verifier
		}

		var nonce *string
		if cfg.ValidateIDToken {
			value, err := oauth.GenerateState()
			if err != nil {
				return nil, infraerrors.InternalServer("OAUTH_NONCE_GEN_FAILED", "failed to generate oauth nonce").WithCause(err)
			}
			nonce = &value
		}

		flow, state, err := r.controlSessionAuth.CreateAuthFlow(ctx, provider, "login", issuer, redirectTo, codeVerifier, nonce)
		if err != nil {
			return nil, err
		}
		authURL, err := buildOIDCAuthorizeURL(cfg, state, derefString(nonce), challenge, strings.TrimSpace(cfg.RedirectURL))
		if err != nil {
			return nil, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err)
		}
		return &service.ControlExternalLoginStartResult{
			AuthURL: authURL,
			FlowID:  flow.FlowID,
		}, nil
	default:
		return nil, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
	}
}

func (r *controlExternalIdentityProviderRegistry) CompleteLogin(ctx context.Context, input *service.ControlExternalLoginCallbackRequest) (*service.ControlExternalLoginCallbackResult, error) {
	if r == nil || r.controlSessionAuth == nil || r.controlRegistration == nil {
		return nil, infraerrors.InternalServer("CONTROL_AUTH_NOT_READY", "control auth service is not configured")
	}
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_REQUEST", "external login callback is required")
	}

	provider := strings.TrimSpace(input.Provider)
	result := &service.ControlExternalLoginCallbackResult{
		FrontendCallback: controlDefaultFrontendRedirect,
		RedirectTo:       controlDefaultFrontendRedirect,
	}

	switch provider {
	case "linuxdo":
		cfg, err := loadLinuxDoOAuthConfig(ctx, r.cfg, r.settingSvc)
		if err != nil {
			return result, err
		}
		result.FrontendCallback = oauthFrontendCallbackForProvider(provider, cfg.FrontendRedirectURL)
		if providerErr := strings.TrimSpace(input.ProviderError); providerErr != "" {
			return result, infraerrors.BadRequest(providerErr, firstNonEmpty(strings.TrimSpace(input.ProviderErrorMessage), providerErr))
		}
		if strings.TrimSpace(input.FlowID) == "" || strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.State) == "" {
			return result, infraerrors.BadRequest("INVALID_STATE", "missing code or state")
		}

		flow, err := r.controlSessionAuth.ConsumeAuthFlow(ctx, input.FlowID, input.State)
		if err != nil {
			return result, infraerrors.BadRequest("INVALID_STATE", "invalid oauth state").WithCause(err)
		}
		result.RedirectTo = firstNonEmpty(sanitizeFrontendRedirectPath(flow.RedirectTo), controlDefaultFrontendRedirect)

		tokenResp, err := linuxDoExchangeCode(ctx, cfg, input.Code, strings.TrimSpace(cfg.RedirectURL), derefString(flow.CodeVerifier))
		if err != nil {
			return result, infraerrors.BadRequest("TOKEN_EXCHANGE_FAILED", "failed to exchange oauth code").WithCause(err)
		}
		email, username, subject, err := linuxDoFetchUserInfo(ctx, cfg, tokenResp)
		if err != nil {
			return result, infraerrors.BadRequest("USERINFO_FAILED", "failed to fetch user info").WithCause(err)
		}
		registrationEmail := strings.TrimSpace(email)
		if subject != "" {
			email = linuxDoSyntheticEmail(subject)
		}

		loginResult, err := r.controlRegistration.CompleteExternalLogin(ctx, &service.ControlExternalLoginRequest{
			Identity: &service.ExternalIdentityProfile{
				Provider:          provider,
				Issuer:            firstNonEmpty(strings.TrimSpace(flow.Issuer), strings.TrimSpace(cfg.AuthorizeURL), provider),
				Subject:           subject,
				LoginHint:         email,
				RegistrationEmail: registrationEmail,
				Username:          username,
			},
			RedirectTo: result.RedirectTo,
			AMR:        provider,
		})
		if err != nil {
			return result, err
		}
		result.Result = loginResult
		return result, nil
	case "oidc":
		cfg, err := loadOIDCOAuthConfig(ctx, r.cfg, r.settingSvc)
		if err != nil {
			return result, err
		}
		result.FrontendCallback = oauthFrontendCallbackForProvider(provider, cfg.FrontendRedirectURL)
		if providerErr := strings.TrimSpace(input.ProviderError); providerErr != "" {
			return result, infraerrors.BadRequest(providerErr, firstNonEmpty(strings.TrimSpace(input.ProviderErrorMessage), providerErr))
		}
		if strings.TrimSpace(input.FlowID) == "" || strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.State) == "" {
			return result, infraerrors.BadRequest("INVALID_STATE", "missing code or state")
		}

		flow, err := r.controlSessionAuth.ConsumeAuthFlow(ctx, input.FlowID, input.State)
		if err != nil {
			return result, infraerrors.BadRequest("INVALID_STATE", "invalid oauth state").WithCause(err)
		}
		result.RedirectTo = firstNonEmpty(sanitizeFrontendRedirectPath(flow.RedirectTo), controlDefaultFrontendRedirect)

		tokenResp, err := oidcExchangeCode(ctx, cfg, input.Code, strings.TrimSpace(cfg.RedirectURL), derefString(flow.CodeVerifier))
		if err != nil {
			return result, infraerrors.BadRequest("TOKEN_EXCHANGE_FAILED", "failed to exchange oauth code").WithCause(err)
		}

		idClaims := &oidcIDTokenClaims{}
		if cfg.ValidateIDToken {
			if strings.TrimSpace(tokenResp.IDToken) == "" {
				return result, infraerrors.BadRequest("MISSING_ID_TOKEN", "missing id_token")
			}
			idClaims, err = oidcParseAndValidateIDToken(ctx, cfg, tokenResp.IDToken, derefString(flow.Nonce))
			if err != nil {
				return result, infraerrors.BadRequest("INVALID_ID_TOKEN", "failed to validate id_token").WithCause(err)
			}
		}

		userInfoClaims, err := oidcFetchUserInfo(ctx, cfg, tokenResp)
		if err != nil {
			return result, infraerrors.BadRequest("USERINFO_FAILED", "failed to fetch user info").WithCause(err)
		}

		subject := strings.TrimSpace(idClaims.Subject)
		if subject == "" {
			subject = strings.TrimSpace(userInfoClaims.Subject)
		}
		if subject == "" {
			return result, infraerrors.BadRequest("MISSING_SUBJECT", "missing subject claim")
		}
		issuer := strings.TrimSpace(idClaims.Issuer)
		if issuer == "" {
			issuer = firstNonEmpty(strings.TrimSpace(flow.Issuer), strings.TrimSpace(cfg.IssuerURL), provider)
		}
		emailVerified := userInfoClaims.EmailVerified
		if emailVerified == nil {
			emailVerified = idClaims.EmailVerified
		}
		if cfg.RequireEmailVerified && (emailVerified == nil || !*emailVerified) {
			return result, infraerrors.BadRequest("EMAIL_NOT_VERIFIED", "email is not verified")
		}

		loginEmail := oidcSelectLoginEmail(oidcIdentityKey(issuer, subject))
		registrationEmail := strings.TrimSpace(firstNonEmpty(userInfoClaims.Email, idClaims.Email))
		username := firstNonEmpty(
			userInfoClaims.Username,
			idClaims.PreferredUsername,
			idClaims.Name,
			oidcFallbackUsername(subject),
		)

		loginResult, err := r.controlRegistration.CompleteExternalLogin(ctx, &service.ControlExternalLoginRequest{
			Identity: &service.ExternalIdentityProfile{
				Provider:          provider,
				Issuer:            issuer,
				Subject:           subject,
				LoginHint:         loginEmail,
				RegistrationEmail: registrationEmail,
				Username:          username,
			},
			RedirectTo: result.RedirectTo,
			AMR:        provider,
		})
		if err != nil {
			return result, err
		}
		result.Result = loginResult
		return result, nil
	default:
		return result, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
	}
}

func loadLinuxDoOAuthConfig(ctx context.Context, cfg *config.Config, settingSvc *service.SettingService) (config.LinuxDoConnectConfig, error) {
	if settingSvc != nil {
		return settingSvc.GetLinuxDoConnectOAuthConfig(ctx)
	}
	if cfg == nil {
		return config.LinuxDoConnectConfig{}, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
	}
	if !cfg.LinuxDo.Enabled {
		return config.LinuxDoConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
	}
	return cfg.LinuxDo, nil
}

func loadOIDCOAuthConfig(ctx context.Context, cfg *config.Config, settingSvc *service.SettingService) (config.OIDCConnectConfig, error) {
	if settingSvc != nil {
		return settingSvc.GetOIDCConnectOAuthConfig(ctx)
	}
	if cfg == nil {
		return config.OIDCConnectConfig{}, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
	}
	if !cfg.OIDC.Enabled {
		return config.OIDCConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
	}
	return cfg.OIDC, nil
}

func oauthFrontendCallbackForProvider(provider, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured
	}
	switch provider {
	case "linuxdo":
		return controlLinuxDoFrontendCallback
	case "oidc":
		return controlOIDCFrontendCallback
	default:
		return controlDefaultFrontendRedirect
	}
}
