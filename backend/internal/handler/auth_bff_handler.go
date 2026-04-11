package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	controlSessionCookiePath        = "/"
	controlOAuthFlowCookieMaxAgeSec = 10 * 60
	controlPendingCookieMaxAgeSec   = 15 * 60
	controlCSRFCookieMaxAgeSec      = 30 * 24 * 60 * 60
	controlDefaultFrontendRedirect  = "/dashboard"
	controlLinuxDoFrontendCallback  = "/auth/linuxdo/callback"
	controlOIDCFrontendCallback     = "/auth/oidc/callback"
)

type bootstrapSubjectResponse struct {
	SubjectID   string `json:"subject_id"`
	SessionID   string `json:"session_id"`
	AuthVersion int64  `json:"auth_version"`
	AMR         string `json:"amr"`
}

type bootstrapSessionResponse struct {
	SessionID         string    `json:"session_id"`
	ExpiresAt         time.Time `json:"expires_at"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
}

type bootstrapMFAResponse struct {
	TotpEnabled        bool                        `json:"totp_enabled"`
	TotpEnabledAt      *int64                      `json:"totp_enabled_at,omitempty"`
	FeatureEnabled     bool                        `json:"feature_enabled"`
	VerificationMethod *service.VerificationMethod `json:"verification_method,omitempty"`
}

type bootstrapPendingRegistrationResponse struct {
	ChallengeID       string    `json:"challenge_id"`
	Provider          string    `json:"provider"`
	Email             string    `json:"email"`
	RegistrationEmail string    `json:"registration_email,omitempty"`
	Username          string    `json:"username,omitempty"`
	RedirectTo        string    `json:"redirect_to,omitempty"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type bootstrapResponse struct {
	Settings            dto.PublicSettings                    `json:"settings"`
	AuthCapabilities    *service.ControlAuthCapabilities      `json:"auth_capabilities,omitempty"`
	CSRFToken           string                                `json:"csrf_token"`
	AccessToken         string                                `json:"access_token,omitempty"`
	RunMode             string                                `json:"run_mode"`
	Authenticated       bool                                  `json:"authenticated"`
	RefreshAvailable    bool                                  `json:"refresh_available"`
	Subject             *bootstrapSubjectResponse             `json:"subject,omitempty"`
	Profile             *dto.User                             `json:"profile,omitempty"`
	Roles               []string                              `json:"roles,omitempty"`
	PrimaryRole         string                                `json:"primary_role,omitempty"`
	MFA                 *bootstrapMFAResponse                 `json:"mfa,omitempty"`
	Session             *bootstrapSessionResponse             `json:"session,omitempty"`
	PendingRegistration *bootstrapPendingRegistrationResponse `json:"pending_registration,omitempty"`
}

type sessionLoginRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required"`
	TurnstileToken string `json:"turnstile_token"`
}

type sessionLoginTOTPRequest struct {
	LoginChallengeID string `json:"login_challenge_id" binding:"required"`
	TOTPCode         string `json:"totp_code" binding:"required,len=6"`
}

type registrationPreflightRequest struct {
	Email                    string `json:"email"`
	PromoCode                string `json:"promo_code"`
	InvitationCode           string `json:"invitation_code"`
	ProviderRegistrationHint string `json:"provider_registration_context"`
}

type registrationEmailCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type registrationRequest struct {
	Email            string `json:"email" binding:"required,email"`
	Password         string `json:"password" binding:"required,min=6"`
	VerificationCode string `json:"verification_code"`
	PromoCode        string `json:"promo_code"`
	InvitationCode   string `json:"invitation_code"`
	TurnstileToken   string `json:"turnstile_token"`
}

type registrationCompleteRequest struct {
	InvitationCode string `json:"invitation_code" binding:"required"`
}

type passwordForgotRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type passwordResetRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type patchMeRequest struct {
	Username *string `json:"username"`
}

type changeMyPasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type meTotpSetupRequest struct {
	EmailCode string `json:"email_code"`
	Password  string `json:"password"`
}

type meTotpEnableRequest struct {
	TOTPCode   string `json:"totp_code" binding:"required,len=6"`
	SetupToken string `json:"setup_token" binding:"required"`
}

type meTotpDisableRequest struct {
	EmailCode string `json:"email_code"`
	Password  string `json:"password"`
}

func (h *AuthHandler) Bootstrap(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	csrfToken, err := h.ensureCSRFCookie(c, false)
	if err != nil {
		response.InternalError(c, "failed to initialize csrf token")
		return
	}

	refreshAvailable := h.cookieValue(c, service.ControlRefreshCookieName) != ""
	identity, authErr := h.currentIdentity(c)
	switch {
	case authErr == nil:
	case errors.Is(authErr, service.ErrAccessTokenExpired), errors.Is(authErr, service.ErrInvalidToken), errors.Is(authErr, service.ErrTokenRevoked), errors.Is(authErr, service.ErrUserNotActive):
		h.clearAccessCookie(c)
	default:
		response.ErrorFrom(c, authErr)
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), identity, csrfToken, refreshAvailable)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if identity != nil {
		payload.AccessToken = h.currentAccessToken(c)
	}
	if identity != nil {
		h.clearPendingRegistrationCookie(c)
	} else if pending := h.pendingRegistrationPayload(c); pending != nil {
		payload.PendingRegistration = pending
	}

	response.Success(c, payload)
}

func (h *AuthHandler) SessionLogin(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req sessionLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.controlSessionAuth.Login(c.Request.Context(), req.Email, req.Password, req.TurnstileToken, ip.GetClientIP(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if result.RequiresMFA {
		response.Success(c, gin.H{
			"mfa_required":       true,
			"login_challenge_id": result.LoginChallengeID,
			"masked_email":       result.MaskedEmail,
			"user_email_masked":  result.MaskedEmail,
		})
		return
	}

	csrfToken, err := h.setSessionCookies(c, result.Tokens)
	if err != nil {
		response.InternalError(c, "failed to establish session")
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), result.Identity, csrfToken, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload.AccessToken = result.Tokens.AccessToken
	response.Success(c, payload)
}

func (h *AuthHandler) SessionLoginTOTP(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req sessionLoginTOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.controlSessionAuth.CompleteLoginTOTP(c.Request.Context(), req.LoginChallengeID, req.TOTPCode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	csrfToken, err := h.setSessionCookies(c, result.Tokens)
	if err != nil {
		response.InternalError(c, "failed to establish session")
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), result.Identity, csrfToken, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload.AccessToken = result.Tokens.AccessToken
	response.Success(c, payload)
}

func (h *AuthHandler) SessionLogout(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.controlSessionAuth.LogoutSession(c.Request.Context(), identity.SessionID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	h.clearSessionCookies(c)
	response.Success(c, gin.H{"success": true})
}

func (h *AuthHandler) SessionsLogoutAll(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.controlSessionAuth.LogoutAllSessions(c.Request.Context(), identity); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	h.clearSessionCookies(c)
	response.Success(c, gin.H{"success": true})
}

func (h *AuthHandler) SessionRefresh(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	refreshToken := h.cookieValue(c, service.ControlRefreshCookieName)
	identity, tokens, err := h.controlSessionAuth.RefreshSession(c.Request.Context(), refreshToken)
	if err != nil {
		if errors.Is(err, service.ErrRefreshTokenInvalid) || errors.Is(err, service.ErrRefreshTokenExpired) || errors.Is(err, service.ErrRefreshTokenReused) || errors.Is(err, service.ErrTokenRevoked) {
			h.clearSessionCookies(c)
		}
		response.ErrorFrom(c, err)
		return
	}

	csrfToken, err := h.setSessionCookies(c, tokens)
	if err != nil {
		response.InternalError(c, "failed to refresh session")
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), identity, csrfToken, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload.AccessToken = tokens.AccessToken
	response.Success(c, payload)
}

func (h *AuthHandler) RegistrationPreflight(c *gin.Context) {
	if h.controlLocalAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req registrationPreflightRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.controlLocalAuth.RegistrationPreflight(c.Request.Context(), req.Email, req.PromoCode, req.InvitationCode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AuthHandler) RegistrationEmailCode(c *gin.Context) {
	if h.controlLocalAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req registrationEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.controlLocalAuth.SendRegistrationEmailCode(c.Request.Context(), req.Email); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"success": true})
}

func (h *AuthHandler) Registration(c *gin.Context) {
	if h.controlSessionAuth == nil || h.controlLocalAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req registrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	identity, tokens, err := h.controlLocalAuth.Register(c.Request.Context(), &service.ControlRegistrationInput{
		Email:            req.Email,
		Password:         req.Password,
		VerificationCode: req.VerificationCode,
		PromoCode:        req.PromoCode,
		InvitationCode:   req.InvitationCode,
		TurnstileToken:   req.TurnstileToken,
		RemoteIP:         ip.GetClientIP(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	csrfToken, err := h.setSessionCookies(c, tokens)
	if err != nil {
		response.InternalError(c, "failed to establish session")
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), identity, csrfToken, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload.AccessToken = tokens.AccessToken
	response.Success(c, payload)
}

func (h *AuthHandler) RegistrationComplete(c *gin.Context) {
	if h.controlSessionAuth == nil || h.controlLocalAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req registrationCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	challengeID := h.cookieValue(c, service.ControlPendingRegistrationCookieName)
	if strings.TrimSpace(challengeID) == "" {
		response.ErrorFrom(c, service.ErrRegistrationChallengeNotFound)
		return
	}

	identity, tokens, err := h.controlLocalAuth.CompleteOAuthRegistration(c.Request.Context(), challengeID, req.InvitationCode)
	if err != nil {
		if errors.Is(err, service.ErrRegistrationChallengeNotFound) {
			h.clearPendingRegistrationCookie(c)
		}
		response.ErrorFrom(c, err)
		return
	}

	h.clearPendingRegistrationCookie(c)
	csrfToken, err := h.setSessionCookies(c, tokens)
	if err != nil {
		response.InternalError(c, "failed to establish session")
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), identity, csrfToken, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload.AccessToken = tokens.AccessToken
	response.Success(c, payload)
}

func (h *AuthHandler) PasswordForgot(c *gin.Context) {
	if h.controlLocalAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req passwordForgotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.controlLocalAuth.RequestPasswordReset(c.Request.Context(), req.Email); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"message": "If your email is registered, you will receive a password reset link shortly.",
	})
}

func (h *AuthHandler) PasswordReset(c *gin.Context) {
	if h.controlLocalAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req passwordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.controlLocalAuth.ResetPassword(c.Request.Context(), req.Email, req.Token, req.NewPassword); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"message": "Your password has been reset successfully. You can now log in with your new password.",
	})
}

func (h *AuthHandler) PatchMe(c *gin.Context) {
	if h.controlLocalAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req patchMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	updatedIdentity, err := h.controlLocalAuth.UpdateProfile(c.Request.Context(), identity, req.Username)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"subject":      h.newBootstrapSubject(updatedIdentity),
		"profile":      dto.UserFromService(updatedIdentity.User),
		"roles":        updatedIdentity.Roles,
		"primary_role": updatedIdentity.PrimaryRole,
	})
}

func (h *AuthHandler) ChangeMyPassword(c *gin.Context) {
	if h.controlSessionAuth == nil || h.controlLocalAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	var req changeMyPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	nextIdentity, tokens, err := h.controlLocalAuth.ChangePassword(c.Request.Context(), identity, req.OldPassword, req.NewPassword)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	csrfToken, err := h.setSessionCookies(c, tokens)
	if err != nil {
		response.InternalError(c, "failed to rotate session")
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), nextIdentity, csrfToken, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload.AccessToken = tokens.AccessToken
	response.Success(c, payload)
}

func (h *AuthHandler) GetMyTOTP(c *gin.Context) {
	if h.totpService == nil {
		response.InternalError(c, "totp service is not configured")
		return
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	status, err := h.totpService.GetStatus(c.Request.Context(), identity.LegacyUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	resp := gin.H{
		"enabled":             status.Enabled,
		"feature_enabled":     status.FeatureEnabled,
		"verification_method": h.totpService.GetVerificationMethod(c.Request.Context()),
	}
	if status.EnabledAt != nil {
		ts := status.EnabledAt.Unix()
		resp["enabled_at"] = ts
	}
	response.Success(c, resp)
}

func (h *AuthHandler) SendMyTOTPCode(c *gin.Context) {
	if h.totpService == nil {
		response.InternalError(c, "totp service is not configured")
		return
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if err := h.totpService.SendVerifyCode(c.Request.Context(), identity.LegacyUserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"success": true})
}

func (h *AuthHandler) SetupMyTOTP(c *gin.Context) {
	if h.totpService == nil {
		response.InternalError(c, "totp service is not configured")
		return
	}

	var req meTotpSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = meTotpSetupRequest{}
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.totpService.InitiateSetup(c.Request.Context(), identity.LegacyUserID, req.EmailCode, req.Password)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AuthHandler) EnableMyTOTP(c *gin.Context) {
	if h.totpService == nil || h.controlSessionAuth == nil {
		response.InternalError(c, "totp service is not configured")
		return
	}

	var req meTotpEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.totpService.CompleteSetup(c.Request.Context(), identity.LegacyUserID, req.TOTPCode, req.SetupToken); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	nextIdentity, tokens, err := h.controlSessionAuth.RotateCurrentSession(c.Request.Context(), identity, identity.AMR)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	csrfToken, err := h.setSessionCookies(c, tokens)
	if err != nil {
		response.InternalError(c, "failed to rotate session")
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), nextIdentity, csrfToken, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload.AccessToken = tokens.AccessToken
	response.Success(c, payload)
}

func (h *AuthHandler) DisableMyTOTP(c *gin.Context) {
	if h.totpService == nil || h.controlSessionAuth == nil {
		response.InternalError(c, "totp service is not configured")
		return
	}

	var req meTotpDisableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	identity, err := h.currentIdentity(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.totpService.Disable(c.Request.Context(), identity.LegacyUserID, req.EmailCode, req.Password); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	nextIdentity, tokens, err := h.controlSessionAuth.RotateCurrentSession(c.Request.Context(), identity, identity.AMR)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	csrfToken, err := h.setSessionCookies(c, tokens)
	if err != nil {
		response.InternalError(c, "failed to rotate session")
		return
	}

	payload, err := h.buildBootstrapPayload(c.Request.Context(), nextIdentity, csrfToken, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload.AccessToken = tokens.AccessToken
	response.Success(c, payload)
}

func (h *AuthHandler) OAuthStart(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	provider := strings.TrimSpace(c.Param("provider"))
	redirectTo := sanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirectTo == "" {
		redirectTo = controlDefaultFrontendRedirect
	}

	var (
		authURL      string
		issuer       string
		codeVerifier *string
		nonce        *string
		err          error
	)

	switch provider {
	case "linuxdo":
		cfg, cfgErr := h.getLinuxDoOAuthConfig(c.Request.Context())
		if cfgErr != nil {
			response.ErrorFrom(c, cfgErr)
			return
		}
		issuer = firstNonEmpty(strings.TrimSpace(cfg.AuthorizeURL), provider)

		var verifier string
		challenge := ""
		if cfg.UsePKCE {
			verifier, err = oauth.GenerateCodeVerifier()
			if err != nil {
				response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_PKCE_GEN_FAILED", "failed to generate pkce verifier").WithCause(err))
				return
			}
			challenge = oauth.GenerateCodeChallenge(verifier)
			codeVerifier = &verifier
		}

		flow, state, err := h.controlSessionAuth.CreateAuthFlow(c.Request.Context(), provider, "login", issuer, redirectTo, codeVerifier, nil)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		h.setAuthFlowCookie(c, flow.FlowID)

		authURL, err = buildLinuxDoAuthorizeURL(cfg, state, challenge, strings.TrimSpace(cfg.RedirectURL))
		if err != nil {
			response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
			return
		}
	case "oidc":
		cfg, cfgErr := h.getOIDCOAuthConfig(c.Request.Context())
		if cfgErr != nil {
			response.ErrorFrom(c, cfgErr)
			return
		}
		issuer = firstNonEmpty(strings.TrimSpace(cfg.IssuerURL), provider)

		var verifier string
		challenge := ""
		if cfg.UsePKCE {
			verifier, err = oauth.GenerateCodeVerifier()
			if err != nil {
				response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_PKCE_GEN_FAILED", "failed to generate pkce verifier").WithCause(err))
				return
			}
			challenge = oauth.GenerateCodeChallenge(verifier)
			codeVerifier = &verifier
		}

		if cfg.ValidateIDToken {
			value, nonceErr := oauth.GenerateState()
			if nonceErr != nil {
				response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_NONCE_GEN_FAILED", "failed to generate oauth nonce").WithCause(nonceErr))
				return
			}
			nonce = &value
		}

		flow, state, err := h.controlSessionAuth.CreateAuthFlow(c.Request.Context(), provider, "login", issuer, redirectTo, codeVerifier, nonce)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		h.setAuthFlowCookie(c, flow.FlowID)

		authURL, err = buildOIDCAuthorizeURL(cfg, state, derefString(nonce), challenge, strings.TrimSpace(cfg.RedirectURL))
		if err != nil {
			response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
			return
		}
	default:
		response.NotFound(c, "OAuth provider not found")
		return
	}

	c.Redirect(http.StatusFound, authURL)
}

func (h *AuthHandler) OAuthCallback(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}

	provider := strings.TrimSpace(c.Param("provider"))
	flowID := h.cookieValue(c, service.ControlAuthFlowCookieName)
	h.clearAuthFlowCookie(c)

	var (
		redirectTo       = controlDefaultFrontendRedirect
		frontendCallback string
		result           *service.ControlOAuthLoginResult
		err              error
	)

	switch provider {
	case "linuxdo":
		cfg, cfgErr := h.getLinuxDoOAuthConfig(c.Request.Context())
		if cfgErr != nil {
			response.ErrorFrom(c, cfgErr)
			return
		}
		frontendCallback = h.oauthFrontendCallback(provider, cfg.FrontendRedirectURL)

		if providerErr := strings.TrimSpace(c.Query("error")); providerErr != "" {
			h.redirectOAuthQueryError(c, frontendCallback, "provider_error", providerErr, c.Query("error_description"), redirectTo)
			return
		}
		code := strings.TrimSpace(c.Query("code"))
		state := strings.TrimSpace(c.Query("state"))
		if flowID == "" || code == "" || state == "" {
			h.redirectOAuthQueryError(c, frontendCallback, "invalid_state", "missing code/state", "", redirectTo)
			return
		}

		flow, flowErr := h.controlSessionAuth.ConsumeAuthFlow(c.Request.Context(), flowID, state)
		if flowErr != nil {
			h.redirectOAuthQueryError(c, frontendCallback, "invalid_state", "invalid oauth state", "", redirectTo)
			return
		}
		redirectTo = firstNonEmpty(sanitizeFrontendRedirectPath(flow.RedirectTo), controlDefaultFrontendRedirect)

		tokenResp, exchangeErr := linuxDoExchangeCode(c.Request.Context(), cfg, code, strings.TrimSpace(cfg.RedirectURL), derefString(flow.CodeVerifier))
		if exchangeErr != nil {
			h.redirectOAuthQueryError(c, frontendCallback, "token_exchange_failed", "failed to exchange oauth code", singleLine(exchangeErr.Error()), redirectTo)
			return
		}
		email, username, subject, userInfoErr := linuxDoFetchUserInfo(c.Request.Context(), cfg, tokenResp)
		if userInfoErr != nil {
			h.redirectOAuthQueryError(c, frontendCallback, "userinfo_failed", "failed to fetch user info", "", redirectTo)
			return
		}
		registrationEmail := strings.TrimSpace(email)
		if subject != "" {
			email = linuxDoSyntheticEmail(subject)
		}

		result, err = h.controlSessionAuth.HandleOAuthLogin(c.Request.Context(), &service.ControlOAuthLoginInput{
			Provider:          provider,
			Issuer:            firstNonEmpty(strings.TrimSpace(flow.Issuer), strings.TrimSpace(cfg.AuthorizeURL), provider),
			ExternalSubject:   subject,
			LoginEmail:        email,
			RegistrationEmail: registrationEmail,
			Username:          username,
			RedirectTo:        redirectTo,
			AMR:               provider,
		})
	case "oidc":
		cfg, cfgErr := h.getOIDCOAuthConfig(c.Request.Context())
		if cfgErr != nil {
			response.ErrorFrom(c, cfgErr)
			return
		}
		frontendCallback = h.oauthFrontendCallback(provider, cfg.FrontendRedirectURL)

		if providerErr := strings.TrimSpace(c.Query("error")); providerErr != "" {
			h.redirectOAuthQueryError(c, frontendCallback, "provider_error", providerErr, c.Query("error_description"), redirectTo)
			return
		}
		code := strings.TrimSpace(c.Query("code"))
		state := strings.TrimSpace(c.Query("state"))
		if flowID == "" || code == "" || state == "" {
			h.redirectOAuthQueryError(c, frontendCallback, "invalid_state", "missing code/state", "", redirectTo)
			return
		}

		flow, flowErr := h.controlSessionAuth.ConsumeAuthFlow(c.Request.Context(), flowID, state)
		if flowErr != nil {
			h.redirectOAuthQueryError(c, frontendCallback, "invalid_state", "invalid oauth state", "", redirectTo)
			return
		}
		redirectTo = firstNonEmpty(sanitizeFrontendRedirectPath(flow.RedirectTo), controlDefaultFrontendRedirect)

		tokenResp, exchangeErr := oidcExchangeCode(c.Request.Context(), cfg, code, strings.TrimSpace(cfg.RedirectURL), derefString(flow.CodeVerifier))
		if exchangeErr != nil {
			h.redirectOAuthQueryError(c, frontendCallback, "token_exchange_failed", "failed to exchange oauth code", singleLine(exchangeErr.Error()), redirectTo)
			return
		}

		idClaims := &oidcIDTokenClaims{}
		if cfg.ValidateIDToken {
			if strings.TrimSpace(tokenResp.IDToken) == "" {
				h.redirectOAuthQueryError(c, frontendCallback, "missing_id_token", "missing id_token", "", redirectTo)
				return
			}
			idClaims, err = oidcParseAndValidateIDToken(c.Request.Context(), cfg, tokenResp.IDToken, derefString(flow.Nonce))
			if err != nil {
				h.redirectOAuthQueryError(c, frontendCallback, "invalid_id_token", "failed to validate id_token", "", redirectTo)
				return
			}
		}

		userInfoClaims, userInfoErr := oidcFetchUserInfo(c.Request.Context(), cfg, tokenResp)
		if userInfoErr != nil {
			h.redirectOAuthQueryError(c, frontendCallback, "userinfo_failed", "failed to fetch user info", "", redirectTo)
			return
		}

		subject := strings.TrimSpace(idClaims.Subject)
		if subject == "" {
			subject = strings.TrimSpace(userInfoClaims.Subject)
		}
		if subject == "" {
			h.redirectOAuthQueryError(c, frontendCallback, "missing_subject", "missing subject claim", "", redirectTo)
			return
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
			h.redirectOAuthQueryError(c, frontendCallback, "email_not_verified", "email is not verified", "", redirectTo)
			return
		}

		loginEmail := oidcSelectLoginEmail(oidcIdentityKey(issuer, subject))
		registrationEmail := strings.TrimSpace(firstNonEmpty(userInfoClaims.Email, idClaims.Email))
		username := firstNonEmpty(
			userInfoClaims.Username,
			idClaims.PreferredUsername,
			idClaims.Name,
			oidcFallbackUsername(subject),
		)

		result, err = h.controlSessionAuth.HandleOAuthLogin(c.Request.Context(), &service.ControlOAuthLoginInput{
			Provider:          provider,
			Issuer:            issuer,
			ExternalSubject:   subject,
			LoginEmail:        loginEmail,
			RegistrationEmail: registrationEmail,
			Username:          username,
			RedirectTo:        redirectTo,
			AMR:               provider,
		})
	default:
		response.NotFound(c, "OAuth provider not found")
		return
	}

	if err != nil {
		h.redirectOAuthQueryError(c, frontendCallback, "login_failed", infraerrors.Reason(err), infraerrors.Message(err), redirectTo)
		return
	}
	if result == nil {
		h.redirectOAuthQueryError(c, frontendCallback, "login_failed", "oauth login failed", "", redirectTo)
		return
	}

	if result.Challenge != nil {
		h.setPendingRegistrationCookie(c, result.Challenge.ChallengeID, result.Challenge.ExpiresAt)
		h.redirectToFrontend(c, frontendCallback, redirectTo, true)
		return
	}

	if _, err := h.setSessionCookies(c, result.Tokens); err != nil {
		h.redirectOAuthQueryError(c, frontendCallback, "login_failed", "failed to establish session", "", redirectTo)
		return
	}
	h.redirectToFrontend(c, redirectTo, redirectTo, false)
}

func (h *AuthHandler) JWKS(c *gin.Context) {
	if h.controlSessionAuth == nil {
		response.InternalError(c, "control auth service is not configured")
		return
	}
	c.JSON(http.StatusOK, h.controlSessionAuth.JWKS())
}

func (h *AuthHandler) buildBootstrapPayload(ctx context.Context, identity *service.AuthenticatedIdentity, csrfToken string, refreshAvailable bool) (*bootstrapResponse, error) {
	settings, err := h.publicSettings(ctx)
	if err != nil {
		return nil, err
	}

	payload := &bootstrapResponse{
		Settings:         settings,
		AuthCapabilities: h.controlSessionAuth.AuthCapabilities(ctx),
		CSRFToken:        csrfToken,
		RunMode:          h.currentRunMode(),
		Authenticated:    identity != nil,
		RefreshAvailable: refreshAvailable,
	}

	if identity == nil {
		return payload, nil
	}
	if identity.User == nil {
		user, err := h.userService.GetByID(ctx, identity.LegacyUserID)
		if err != nil {
			return nil, err
		}
		identity.User = user
	}

	payload.Subject = h.newBootstrapSubject(identity)
	payload.Profile = dto.UserFromService(identity.User)
	payload.Roles = append([]string(nil), identity.Roles...)
	payload.PrimaryRole = identity.PrimaryRole
	payload.Session = &bootstrapSessionResponse{
		SessionID:         identity.SessionID,
		ExpiresAt:         identity.SessionExpiresAt,
		AbsoluteExpiresAt: identity.SessionAbsoluteAt,
		LastSeenAt:        identity.SessionLastSeenAt,
	}

	if h.totpService != nil {
		status, err := h.totpService.GetStatus(ctx, identity.LegacyUserID)
		if err != nil {
			return nil, err
		}
		payload.MFA = &bootstrapMFAResponse{
			TotpEnabled:        status.Enabled,
			FeatureEnabled:     status.FeatureEnabled,
			VerificationMethod: h.totpService.GetVerificationMethod(ctx),
		}
		if status.EnabledAt != nil {
			ts := status.EnabledAt.Unix()
			payload.MFA.TotpEnabledAt = &ts
		}
	}

	return payload, nil
}

func (h *AuthHandler) publicSettings(ctx context.Context) (dto.PublicSettings, error) {
	if h.settingSvc == nil {
		return dto.PublicSettings{Version: h.version}, nil
	}

	settings, err := h.settingSvc.GetPublicSettings(ctx)
	if err != nil {
		return dto.PublicSettings{}, err
	}

	return dto.PublicSettings{
		RegistrationEnabled:              settings.RegistrationEnabled,
		EmailVerifyEnabled:               settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 settings.PromoCodeEnabled,
		PasswordResetEnabled:             settings.PasswordResetEnabled,
		InvitationCodeEnabled:            settings.InvitationCodeEnabled,
		TotpEnabled:                      settings.TotpEnabled,
		TurnstileEnabled:                 settings.TurnstileEnabled,
		TurnstileSiteKey:                 settings.TurnstileSiteKey,
		SiteName:                         settings.SiteName,
		SiteLogo:                         settings.SiteLogo,
		SiteSubtitle:                     settings.SiteSubtitle,
		APIBaseURL:                       settings.APIBaseURL,
		ContactInfo:                      settings.ContactInfo,
		DocURL:                           settings.DocURL,
		HomeContent:                      settings.HomeContent,
		HideCcsImportButton:              settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:      settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:          settings.PurchaseSubscriptionURL,
		GrafanaURL:                       settings.GrafanaURL,
		CustomMenuItems:                  dto.ParseUserVisibleMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                  dto.ParseCustomEndpoints(settings.CustomEndpoints),
		LinuxDoOAuthEnabled:              settings.LinuxDoOAuthEnabled,
		OIDCOAuthEnabled:                 settings.OIDCOAuthEnabled,
		OIDCOAuthProviderName:            settings.OIDCOAuthProviderName,
		BackendModeEnabled:               settings.BackendModeEnabled,
		Version:                          h.version,
	}, nil
}

func (h *AuthHandler) currentIdentity(c *gin.Context) (*service.AuthenticatedIdentity, error) {
	if h.controlSessionAuth == nil {
		return nil, infraerrors.InternalServer("CONTROL_AUTH_NOT_READY", "control auth service is not configured")
	}
	token := extractBearerToken(c)
	if token == "" {
		token = h.cookieValue(c, service.ControlAccessCookieName)
	}
	if token == "" {
		return nil, service.ErrInvalidToken
	}
	return h.controlSessionAuth.AuthenticateAccessToken(c.Request.Context(), token)
}

func (h *AuthHandler) currentAccessToken(c *gin.Context) string {
	if token := extractBearerToken(c); token != "" {
		return token
	}
	return h.cookieValue(c, service.ControlAccessCookieName)
}

func (h *AuthHandler) setSessionCookies(c *gin.Context, tokens *service.ControlSessionTokens) (string, error) {
	if tokens == nil {
		return "", infraerrors.InternalServer("SESSION_TOKENS_MISSING", "session tokens are required")
	}

	secure := isRequestHTTPS(c)
	h.setHTTPOnlyCookie(c, service.ControlAccessCookieName, tokens.AccessToken, int(time.Until(tokens.AccessExpiresAt).Seconds()), secure)
	h.setHTTPOnlyCookie(c, service.ControlRefreshCookieName, tokens.RefreshToken, int(time.Until(tokens.AbsoluteExpiry).Seconds()), secure)
	h.clearPendingRegistrationCookie(c)
	return h.ensureCSRFCookie(c, true)
}

func (h *AuthHandler) clearSessionCookies(c *gin.Context) {
	secure := isRequestHTTPS(c)
	h.clearCookie(c, service.ControlAccessCookieName, true, secure)
	h.clearCookie(c, service.ControlRefreshCookieName, true, secure)
}

func (h *AuthHandler) clearAccessCookie(c *gin.Context) {
	h.clearCookie(c, service.ControlAccessCookieName, true, isRequestHTTPS(c))
}

func (h *AuthHandler) ensureCSRFCookie(c *gin.Context, rotate bool) (string, error) {
	if !rotate {
		if existing := h.cookieValue(c, service.ControlCSRFCookieName); existing != "" {
			return existing, nil
		}
	}

	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	h.setCookie(c, service.ControlCSRFCookieName, token, controlCSRFCookieMaxAgeSec, false, isRequestHTTPS(c))
	return token, nil
}

func (h *AuthHandler) setAuthFlowCookie(c *gin.Context, flowID string) {
	h.setCookie(c, service.ControlAuthFlowCookieName, flowID, controlOAuthFlowCookieMaxAgeSec, true, isRequestHTTPS(c))
}

func (h *AuthHandler) clearAuthFlowCookie(c *gin.Context) {
	h.clearCookie(c, service.ControlAuthFlowCookieName, true, isRequestHTTPS(c))
}

func (h *AuthHandler) setPendingRegistrationCookie(c *gin.Context, challengeID string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge <= 0 {
		maxAge = controlPendingCookieMaxAgeSec
	}
	h.setCookie(c, service.ControlPendingRegistrationCookieName, challengeID, maxAge, true, isRequestHTTPS(c))
}

func (h *AuthHandler) clearPendingRegistrationCookie(c *gin.Context) {
	h.clearCookie(c, service.ControlPendingRegistrationCookieName, true, isRequestHTTPS(c))
}

func (h *AuthHandler) setHTTPOnlyCookie(c *gin.Context, name, value string, maxAge int, secure bool) {
	h.setCookie(c, name, value, maxAge, true, secure)
}

func (h *AuthHandler) setCookie(c *gin.Context, name, value string, maxAge int, httpOnly, secure bool) {
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     controlSessionCookiePath,
		MaxAge:   maxAge,
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearCookie(c *gin.Context, name string, httpOnly, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     controlSessionCookiePath,
		MaxAge:   -1,
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) cookieValue(c *gin.Context, name string) string {
	value, err := c.Cookie(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func (h *AuthHandler) redirectOAuthQueryError(c *gin.Context, frontendCallback, code, message, description, redirectTo string) {
	target := h.resolveFrontendURL(firstNonEmpty(frontendCallback, controlDefaultFrontendRedirect))
	u, err := url.Parse(target)
	if err != nil {
		c.Redirect(http.StatusFound, controlDefaultFrontendRedirect)
		return
	}
	q := u.Query()
	if code = strings.TrimSpace(code); code != "" {
		q.Set("error", code)
	}
	if message = strings.TrimSpace(message); message != "" {
		q.Set("error_message", singleLine(message))
	}
	if description = strings.TrimSpace(description); description != "" {
		q.Set("error_description", singleLine(description))
	}
	if redirect := sanitizeFrontendRedirectPath(redirectTo); redirect != "" {
		q.Set("redirect", redirect)
	}
	u.RawQuery = q.Encode()
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, u.String())
}

func (h *AuthHandler) redirectToFrontend(c *gin.Context, target, redirectTo string, includeRedirectQuery bool) {
	resolved := h.resolveFrontendURL(firstNonEmpty(target, controlDefaultFrontendRedirect))
	if includeRedirectQuery {
		u, err := url.Parse(resolved)
		if err == nil {
			if redirect := sanitizeFrontendRedirectPath(redirectTo); redirect != "" {
				q := u.Query()
				q.Set("redirect", redirect)
				u.RawQuery = q.Encode()
			}
			resolved = u.String()
		}
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, resolved)
}

func (h *AuthHandler) resolveFrontendURL(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return controlDefaultFrontendRedirect
	}
	u, err := url.Parse(target)
	if err != nil {
		return controlDefaultFrontendRedirect
	}
	if u.IsAbs() {
		return u.String()
	}
	if !strings.HasPrefix(target, "/") {
		return controlDefaultFrontendRedirect
	}

	base := ""
	if h.settingSvc != nil {
		base = strings.TrimSuffix(strings.TrimSpace(h.settingSvc.GetFrontendURL(context.Background())), "/")
	}
	if base == "" && h.cfg != nil {
		base = strings.TrimSuffix(strings.TrimSpace(h.cfg.Server.FrontendURL), "/")
	}
	if base == "" {
		return target
	}
	return base + target
}

func (h *AuthHandler) oauthFrontendCallback(provider string, configured string) string {
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

func (h *AuthHandler) currentRunMode() string {
	if h.cfg != nil && strings.TrimSpace(h.cfg.RunMode) != "" {
		return h.cfg.RunMode
	}
	return config.RunModeStandard
}

func (h *AuthHandler) newBootstrapSubject(identity *service.AuthenticatedIdentity) *bootstrapSubjectResponse {
	if identity == nil {
		return nil
	}
	return &bootstrapSubjectResponse{
		SubjectID:   identity.SubjectID,
		SessionID:   identity.SessionID,
		AuthVersion: identity.AuthVersion,
		AMR:         identity.AMR,
	}
}

func (h *AuthHandler) pendingRegistrationPayload(c *gin.Context) *bootstrapPendingRegistrationResponse {
	if h.controlLocalAuth == nil {
		return nil
	}

	challengeID := h.cookieValue(c, service.ControlPendingRegistrationCookieName)
	if challengeID == "" {
		return nil
	}

	challenge, err := h.controlLocalAuth.GetRegistrationChallenge(c.Request.Context(), challengeID)
	if err != nil || challenge == nil || challenge.ConsumedAt != nil || time.Now().After(challenge.ExpiresAt) {
		h.clearPendingRegistrationCookie(c)
		return nil
	}

	return &bootstrapPendingRegistrationResponse{
		ChallengeID:       challenge.ChallengeID,
		Provider:          challenge.Provider,
		Email:             challenge.Email,
		RegistrationEmail: challenge.RegistrationEmail,
		Username:          challenge.Username,
		RedirectTo:        challenge.RedirectTo,
		ExpiresAt:         challenge.ExpiresAt,
	}
}

func extractBearerToken(c *gin.Context) string {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func randomHex(byteLength int) (string, error) {
	if byteLength <= 0 {
		return "", nil
	}
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
