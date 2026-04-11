package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	ControlAccessCookieName              = "control_access_token"
	ControlRefreshCookieName             = "control_refresh_token"
	ControlCSRFCookieName                = "control_csrf_token"
	ControlCSRFHeaderName                = "X-CSRF-Token"
	ControlAuthFlowCookieName            = "control_auth_flow"
	ControlPendingRegistrationCookieName = "control_pending_registration"

	controlAccessKeyActiveSecret   = "control_access_es256_private_key_active"
	controlAccessKeyPreviousSecret = "control_access_es256_private_key_previous"

	controlAccessTokenTTL           = 5 * time.Minute
	controlRefreshIdleTTL           = 7 * 24 * time.Hour
	controlRefreshAbsoluteTTL       = 30 * 24 * time.Hour
	controlSessionSnapshotTTL       = 5 * time.Minute
	controlEmbedTokenTTL            = 2 * time.Minute
	controlOAuthFlowTTL             = 10 * time.Minute
	controlRegistrationChallengeTTL = 15 * time.Minute
	controlEmailVerificationTTL     = 15 * time.Minute
	controlPasswordResetTTL         = 30 * time.Minute

	controlEmailPurposeRegistration = "registration"
	controlEmailPurposeTotp         = "totp"

	controlIssuerDefault   = "sub2api-control"
	controlAudienceDefault = "sub2api-control"
)

var (
	ErrLoginChallengeNotFound = infraerrors.BadRequest("LOGIN_CHALLENGE_INVALID", "invalid or expired login challenge")
	ErrCSRFTokenRequired      = infraerrors.Forbidden("CSRF_TOKEN_REQUIRED", "csrf token is required")
	ErrCSRFTokenInvalid       = infraerrors.Forbidden("CSRF_TOKEN_INVALID", "csrf token is invalid")
	ErrPasswordLoginDisabled  = infraerrors.Forbidden("PASSWORD_LOGIN_DISABLED", "password login is disabled")
	ErrEmailVerificationOff   = infraerrors.Forbidden("EMAIL_VERIFICATION_DISABLED", "email verification is disabled")
	ErrPasswordResetDisabled  = infraerrors.Forbidden("PASSWORD_RESET_DISABLED", "password reset is disabled")
	ErrPasswordChangeDisabled = infraerrors.Forbidden("PASSWORD_CHANGE_DISABLED", "password change is disabled")
	ErrMFASelfServiceDisabled = infraerrors.Forbidden("MFA_SELF_SERVICE_DISABLED", "mfa self-service is disabled")
)

type controlSigningKey struct {
	kid        string
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
}

type controlAccessClaims struct {
	SessionID   string `json:"sid"`
	AuthVersion int64  `json:"av"`
	AMR         string `json:"amr"`
	jwt.RegisteredClaims
}

type ControlJWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type ControlJWKS struct {
	Keys []ControlJWK `json:"keys"`
}

type ControlRegistrationInput struct {
	Email            string
	Password         string
	VerificationCode string
	PromoCode        string
	InvitationCode   string
	TurnstileToken   string
	RemoteIP         string
}

type ControlPasswordChangeResult struct {
	Identity *AuthenticatedIdentity
	Tokens   *ControlSessionTokens
}

// ExternalIdentityProfile is the normalized identity payload emitted by an external IdP or social provider.
// Future Auth0/Clerk adapters should construct this type directly rather than depending on provider-specific callback fields.
type ExternalIdentityProfile struct {
	Provider          string
	Issuer            string
	Subject           string
	LoginHint         string
	RegistrationEmail string
	Username          string
}

type ControlExternalLoginRequest struct {
	Identity   *ExternalIdentityProfile
	RedirectTo string
	AMR        string
}

type ControlExternalLoginResult struct {
	Identity  *AuthenticatedIdentity
	Tokens    *ControlSessionTokens
	Challenge *RegistrationChallengeRecord
}

type ControlAuthService struct {
	entClient          *dbent.Client
	cfg                *config.Config
	userRepo           UserRepository
	authRepo           ControlAuthRepository
	sessionCache       SessionSnapshotCache
	settingService     *SettingService
	authService        *AuthService
	totpService        *TotpService
	emailService       *EmailService
	promoRepo          PromoCodeRepository
	redeemRepo         RedeemCodeRepository
	defaultSubAssigner DefaultSubscriptionAssigner
	issuer             string
	audience           string
	activeSigningKey   *controlSigningKey
	verificationKeys   map[string]*ecdsa.PublicKey
	jwks               *ControlJWKS
}

func NewControlAuthService(
	entClient *dbent.Client,
	cfg *config.Config,
	userRepo UserRepository,
	authRepo ControlAuthRepository,
	sessionCache SessionSnapshotCache,
	settingService *SettingService,
	authService *AuthService,
	totpService *TotpService,
	emailService *EmailService,
	promoRepo PromoCodeRepository,
	redeemRepo RedeemCodeRepository,
	defaultSubAssigner DefaultSubscriptionAssigner,
) (*ControlAuthService, error) {
	if entClient == nil {
		return nil, fmt.Errorf("nil ent client")
	}
	if userRepo == nil {
		return nil, fmt.Errorf("nil user repo")
	}
	if authRepo == nil {
		return nil, fmt.Errorf("nil control auth repo")
	}

	svc := &ControlAuthService{
		entClient:          entClient,
		cfg:                cfg,
		userRepo:           userRepo,
		authRepo:           authRepo,
		sessionCache:       sessionCache,
		settingService:     settingService,
		authService:        authService,
		totpService:        totpService,
		emailService:       emailService,
		promoRepo:          promoRepo,
		redeemRepo:         redeemRepo,
		defaultSubAssigner: defaultSubAssigner,
		issuer:             controlIssuerDefault,
		audience:           controlAudienceDefault,
	}

	if frontendURL := strings.TrimSpace(svc.frontendBaseURL(context.Background())); frontendURL != "" {
		svc.issuer = frontendURL
		svc.audience = frontendURL
	}

	if err := svc.loadSigningKeys(context.Background()); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *ControlAuthService) JWKS() *ControlJWKS {
	if s == nil || s.jwks == nil {
		return &ControlJWKS{Keys: []ControlJWK{}}
	}
	return s.jwks
}

func (s *ControlAuthService) AuthCapabilities(ctx context.Context) *ControlAuthCapabilities {
	return &ControlAuthCapabilities{
		Provider:                  s.authMode(),
		PasswordLoginEnabled:      s.passwordLoginEnabled(ctx),
		RegistrationEnabled:       s.registrationEnabled(ctx),
		EmailVerificationEnabled:  s.emailVerificationEnabled(ctx),
		PasswordResetEnabled:      s.passwordResetEnabled(ctx),
		PasswordChangeEnabled:     s.passwordChangeEnabled(ctx),
		MFASelfServiceEnabled:     s.mfaSelfServiceEnabled(ctx),
		ProfileSelfServiceEnabled: true,
	}
}

func (s *ControlAuthService) authMode() string {
	if s != nil && s.cfg != nil {
		mode := strings.TrimSpace(s.cfg.ControlAuth.Mode)
		if mode != "" {
			return mode
		}
	}
	return ControlAuthModeLocal
}

func (s *ControlAuthService) localCredentialMode() bool {
	return s.authMode() == ControlAuthModeLocal
}

func (s *ControlAuthService) passwordLoginEnabled(ctx context.Context) bool {
	return s.localCredentialMode()
}

func (s *ControlAuthService) registrationEnabled(ctx context.Context) bool {
	if !s.localCredentialMode() {
		return false
	}
	return s.isRegistrationAllowed(ctx)
}

func (s *ControlAuthService) emailVerificationEnabled(ctx context.Context) bool {
	if !s.localCredentialMode() || s.settingService == nil {
		return false
	}
	return s.settingService.IsEmailVerifyEnabled(ctx)
}

func (s *ControlAuthService) passwordResetEnabled(ctx context.Context) bool {
	if !s.localCredentialMode() || s.settingService == nil {
		return false
	}
	return s.settingService.IsPasswordResetEnabled(ctx)
}

func (s *ControlAuthService) passwordChangeEnabled(ctx context.Context) bool {
	return s.localCredentialMode()
}

func (s *ControlAuthService) mfaSelfServiceEnabled(ctx context.Context) bool {
	if !s.localCredentialMode() || s.settingService == nil {
		return false
	}
	return s.settingService.IsTotpEnabled(ctx)
}

func (s *ControlAuthService) loadSigningKeys(ctx context.Context) error {
	activeKey, err := s.loadOrCreateSigningKey(ctx, controlAccessKeyActiveSecret)
	if err != nil {
		return fmt.Errorf("load active signing key: %w", err)
	}

	keys := map[string]*ecdsa.PublicKey{
		activeKey.kid: activeKey.publicKey,
	}
	jwkList := []ControlJWK{buildControlJWK(activeKey)}

	previous, err := s.loadSigningKey(ctx, controlAccessKeyPreviousSecret)
	if err != nil {
		return fmt.Errorf("load previous signing key: %w", err)
	}
	if previous != nil && previous.kid != activeKey.kid {
		keys[previous.kid] = previous.publicKey
		jwkList = append(jwkList, buildControlJWK(previous))
	}

	s.activeSigningKey = activeKey
	s.verificationKeys = keys
	s.jwks = &ControlJWKS{Keys: jwkList}
	return nil
}

func (s *ControlAuthService) loadOrCreateSigningKey(ctx context.Context, secretKey string) (*controlSigningKey, error) {
	existing, err := s.loadSigningKey(ctx, secretKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ecdsa key: %w", err)
	}

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal ecdsa key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})
	if pemBytes == nil {
		return nil, fmt.Errorf("encode ecdsa pem")
	}

	if err := s.entClient.SecuritySecret.Create().
		SetKey(secretKey).
		SetValue(strings.TrimSpace(string(pemBytes))).
		OnConflictColumns(securitysecret.FieldKey).
		DoNothing().
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("persist signing key: %w", err)
	}

	return s.loadSigningKey(ctx, secretKey)
}

func (s *ControlAuthService) loadSigningKey(ctx context.Context, secretKey string) (*controlSigningKey, error) {
	record, err := s.entClient.SecuritySecret.Query().Where(securitysecret.KeyEQ(secretKey)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query security secret %s: %w", secretKey, err)
	}

	privateKey, err := parseECDSAPrivateKey(record.Value)
	if err != nil {
		return nil, fmt.Errorf("parse security secret %s: %w", secretKey, err)
	}
	return buildSigningKey(privateKey)
}

func (s *ControlAuthService) AuthenticateAccessToken(ctx context.Context, tokenString string) (*AuthenticatedIdentity, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, ErrInvalidToken
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}))
	token, err := parser.ParseWithClaims(tokenString, &controlAccessClaims{}, func(tok *jwt.Token) (any, error) {
		kid, _ := tok.Header["kid"].(string)
		if kid == "" {
			return nil, ErrInvalidToken
		}
		publicKey, ok := s.verificationKeys[kid]
		if !ok {
			return nil, ErrInvalidToken
		}
		return publicKey, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrAccessTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*controlAccessClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Issuer != s.issuer {
		return nil, ErrInvalidToken
	}
	if len(claims.Audience) == 0 || claims.Audience[0] != s.audience {
		return nil, ErrInvalidToken
	}

	snapshot, err := s.getSessionSnapshot(ctx, claims.SessionID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, ErrTokenRevoked
	}

	now := time.Now()
	if snapshot.RevokedAt != nil || now.After(snapshot.ExpiresAt) || now.After(snapshot.AbsoluteExpiresAt) {
		return nil, ErrTokenRevoked
	}
	if snapshot.SubjectID != claims.Subject || snapshot.AuthVersion != claims.AuthVersion || snapshot.AMR != claims.AMR {
		return nil, ErrTokenRevoked
	}

	bundle, err := s.authRepo.GetIdentityBundleBySubjectID(ctx, claims.Subject)
	if err != nil {
		if errors.Is(err, ErrSubjectNotFound) {
			return nil, ErrTokenRevoked
		}
		return nil, fmt.Errorf("load subject bundle: %w", err)
	}
	if bundle == nil || bundle.Subject == nil {
		return nil, ErrTokenRevoked
	}
	if bundle.Subject.SubjectID != claims.Subject || bundle.Subject.LegacyUserID != snapshot.LegacyUserID {
		return nil, ErrTokenRevoked
	}
	if bundle.Subject.AuthVersion != claims.AuthVersion {
		return nil, ErrTokenRevoked
	}
	if !strings.EqualFold(bundle.Subject.Status, StatusActive) {
		return nil, ErrUserNotActive
	}

	return buildAuthenticatedIdentity(bundle, snapshot, s.tryGetLinkedUser(ctx, bundle.Subject.LegacyUserID)), nil
}

func (s *ControlAuthService) Login(ctx context.Context, email, password, turnstileToken, remoteIP string) (*ControlLoginResult, error) {
	if !s.passwordLoginEnabled(ctx) {
		return nil, ErrPasswordLoginDisabled
	}
	if s.authService != nil {
		if err := s.authService.VerifyTurnstile(ctx, turnstileToken, remoteIP); err != nil {
			return nil, err
		}
	}

	user, err := s.userRepo.GetByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, ErrServiceUnavailable
	}
	if !checkPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	if !user.IsActive() {
		return nil, ErrUserNotActive
	}
	if s.settingService != nil && s.settingService.IsBackendModeEnabled(ctx) && !user.IsAdmin() {
		return nil, infraerrors.Forbidden("BACKEND_MODE_ONLY_ADMIN", "backend mode is active. only admin login is allowed")
	}

	bundle, err := s.authRepo.SyncLocalCredentialState(ctx, user)
	if err != nil {
		return nil, err
	}

	if s.shouldRequireTOTP(ctx, bundle, user) {
		if s.totpService == nil {
			return nil, ErrTotpNotEnabled
		}
		challengeID, err := s.totpService.CreateLoginSession(ctx, user.ID, user.Email)
		if err != nil {
			return nil, fmt.Errorf("create login challenge: %w", err)
		}
		return &ControlLoginResult{
			RequiresMFA:      true,
			LoginChallengeID: challengeID,
			MaskedEmail:      MaskEmail(user.Email),
		}, nil
	}

	identity, tokens, err := s.createAuthenticatedSession(ctx, bundle, user, "pwd")
	if err != nil {
		return nil, err
	}
	return &ControlLoginResult{Identity: identity, Tokens: tokens}, nil
}

func (s *ControlAuthService) CompleteLoginTOTP(ctx context.Context, challengeID, totpCode string) (*ControlLoginResult, error) {
	if !s.passwordLoginEnabled(ctx) {
		return nil, ErrPasswordLoginDisabled
	}
	if s.totpService == nil {
		return nil, ErrTotpNotEnabled
	}

	session, err := s.totpService.GetLoginSession(ctx, strings.TrimSpace(challengeID))
	if err != nil || session == nil || time.Now().After(session.TokenExpiry) {
		return nil, ErrLoginChallengeNotFound
	}
	if err := s.totpService.VerifyCode(ctx, session.UserID, totpCode); err != nil {
		return nil, err
	}
	_ = s.totpService.DeleteLoginSession(ctx, challengeID)

	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user for login challenge: %w", err)
	}
	if !user.IsActive() {
		return nil, ErrUserNotActive
	}

	bundle, err := s.authRepo.SyncLocalCredentialState(ctx, user)
	if err != nil {
		return nil, err
	}

	identity, tokens, err := s.createAuthenticatedSession(ctx, bundle, user, "pwd+totp")
	if err != nil {
		return nil, err
	}
	return &ControlLoginResult{Identity: identity, Tokens: tokens}, nil
}

func (s *ControlAuthService) RefreshSession(ctx context.Context, rawRefreshToken string) (*AuthenticatedIdentity, *ControlSessionTokens, error) {
	rawRefreshToken = strings.TrimSpace(rawRefreshToken)
	if rawRefreshToken == "" {
		return nil, nil, ErrRefreshTokenInvalid
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin refresh transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)

	sessionRecord, refreshRecord, err := s.authRepo.GetSessionByRefreshTokenHash(txCtx, hashToken(rawRefreshToken))
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, nil, ErrRefreshTokenInvalid
		}
		return nil, nil, err
	}

	now := time.Now()
	if refreshRecord.RevokedAt != nil || sessionRecord.RevokedAt != nil || sessionRecord.Status != StatusActive {
		return nil, nil, ErrRefreshTokenInvalid
	}
	if refreshRecord.RotatedAt != nil || refreshRecord.ReplacedByTokenHash != nil {
		_ = s.authRepo.RevokeSession(txCtx, sessionRecord.SessionID, now)
		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("commit refresh reuse revocation: %w", err)
		}
		_ = s.sessionCache.DeleteSessionSnapshot(ctx, sessionRecord.SessionID)
		return nil, nil, ErrRefreshTokenReused
	}
	if now.After(refreshRecord.IdleExpiresAt) || now.After(refreshRecord.AbsoluteExpiresAt) {
		return nil, nil, ErrRefreshTokenExpired
	}

	bundle, err := s.authRepo.GetIdentityBundleBySubjectID(txCtx, sessionRecord.SubjectID)
	if err != nil {
		if errors.Is(err, ErrSubjectNotFound) {
			return nil, nil, ErrRefreshTokenInvalid
		}
		return nil, nil, err
	}
	if bundle == nil || bundle.Subject == nil || bundle.Subject.LegacyUserID != sessionRecord.LegacyUserID {
		return nil, nil, ErrRefreshTokenInvalid
	}
	if bundle.Subject.AuthVersion != sessionRecord.AuthVersion || !strings.EqualFold(bundle.Subject.Status, StatusActive) {
		_ = s.authRepo.RevokeSession(txCtx, sessionRecord.SessionID, now)
		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("commit refresh revocation: %w", err)
		}
		_ = s.sessionCache.DeleteSessionSnapshot(ctx, sessionRecord.SessionID)
		return nil, nil, ErrTokenRevoked
	}

	nextRefreshToken, err := randomHexString(32)
	if err != nil {
		return nil, nil, fmt.Errorf("generate refresh token: %w", err)
	}
	nextRefreshHash := hashToken(nextRefreshToken)

	nextIdleExpiry := now.Add(controlRefreshIdleTTL)
	if nextIdleExpiry.After(sessionRecord.AbsoluteExpiresAt) {
		nextIdleExpiry = sessionRecord.AbsoluteExpiresAt
	}

	nextRefreshRecord := &RefreshTokenRecord{
		TokenHash:         nextRefreshHash,
		SessionID:         sessionRecord.SessionID,
		SubjectID:         sessionRecord.SubjectID,
		LegacyUserID:      sessionRecord.LegacyUserID,
		CreatedAt:         now,
		IdleExpiresAt:     nextIdleExpiry,
		AbsoluteExpiresAt: sessionRecord.AbsoluteExpiresAt,
	}

	sessionRecord.CurrentRefreshTokenHash = nextRefreshHash
	sessionRecord.LastSeenAt = now
	sessionRecord.ExpiresAt = nextIdleExpiry
	sessionRecord.UpdatedAt = now

	if err := s.authRepo.RotateSessionRefreshToken(txCtx, sessionRecord, refreshRecord.TokenHash, nextRefreshRecord, now); err != nil {
		if errors.Is(err, ErrRefreshTokenInvalid) || errors.Is(err, ErrRefreshTokenReused) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	snapshot := sessionRecordToSnapshot(sessionRecord)
	accessToken, accessExpiry, err := s.signAccessToken(sessionRecord.SubjectID, sessionRecord.SessionID, sessionRecord.AuthVersion, sessionRecord.AMR, now)
	if err != nil {
		return nil, nil, err
	}
	identity := buildAuthenticatedIdentity(bundle, snapshot, s.tryGetLinkedUser(ctx, bundle.Subject.LegacyUserID))
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit refresh transaction: %w", err)
	}
	s.tryStoreSessionSnapshot(ctx, snapshot)
	return identity, &ControlSessionTokens{
		AccessToken:       accessToken,
		RefreshToken:      nextRefreshToken,
		AccessExpiresAt:   accessExpiry,
		IdleRefreshExpiry: nextIdleExpiry,
		AbsoluteExpiry:    sessionRecord.AbsoluteExpiresAt,
	}, nil
}

func (s *ControlAuthService) LogoutSession(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	now := time.Now()
	if err := s.authRepo.RevokeSession(ctx, sessionID, now); err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}
	return s.sessionCache.DeleteSessionSnapshot(ctx, sessionID)
}

func (s *ControlAuthService) LogoutAllSessions(ctx context.Context, identity *AuthenticatedIdentity) error {
	if identity == nil {
		return nil
	}
	now := time.Now()
	nextAuthVersion := identity.AuthVersion + 1
	if err := s.authRepo.UpdateSubjectAuthVersion(ctx, identity.SubjectID, nextAuthVersion); err != nil {
		return err
	}
	if err := s.authRepo.RevokeAllSessions(ctx, identity.SubjectID, now); err != nil {
		return err
	}
	return s.sessionCache.DeleteSessionSnapshot(ctx, identity.SessionID)
}

func (s *ControlAuthService) RotateCurrentSession(ctx context.Context, identity *AuthenticatedIdentity, amr string) (*AuthenticatedIdentity, *ControlSessionTokens, error) {
	if identity == nil {
		return nil, nil, ErrInvalidToken
	}

	user := s.tryGetLinkedUser(ctx, identity.LegacyUserID)
	var bundle *IdentityBundle
	var err error
	if user != nil {
		bundle, err = s.authRepo.SyncLocalCredentialState(ctx, user)
	} else {
		bundle, err = s.authRepo.GetIdentityBundleBySubjectID(ctx, identity.SubjectID)
	}
	if err != nil {
		return nil, nil, err
	}
	if bundle == nil || bundle.Subject == nil {
		return nil, nil, ErrSubjectNotFound
	}

	now := time.Now()
	if err := s.authRepo.RevokeAllSessions(ctx, bundle.Subject.SubjectID, now); err != nil {
		return nil, nil, err
	}
	_ = s.sessionCache.DeleteSessionSnapshot(ctx, identity.SessionID)

	nextAMR := strings.TrimSpace(amr)
	if nextAMR == "" {
		nextAMR = strings.TrimSpace(identity.AMR)
	}
	if nextAMR == "" {
		nextAMR = "pwd"
	}

	return s.createAuthenticatedSession(ctx, bundle, user, nextAMR)
}

func (s *ControlAuthService) IssueEmbedToken(ctx context.Context, identity *AuthenticatedIdentity) (*ControlEmbedToken, error) {
	if identity == nil {
		return nil, ErrInvalidToken
	}
	token, expiresAt, err := s.signSessionToken(identity.SubjectID, identity.SessionID, identity.AuthVersion, identity.AMR, controlEmbedTokenTTL, time.Now())
	if err != nil {
		return nil, err
	}
	return &ControlEmbedToken{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *ControlAuthService) GetRegistrationChallenge(ctx context.Context, challengeID string) (*RegistrationChallengeRecord, error) {
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return nil, ErrRegistrationChallengeNotFound
	}
	return s.authRepo.GetRegistrationChallenge(ctx, challengeID)
}

func (s *ControlAuthService) CreateAuthFlow(ctx context.Context, provider, purpose, issuer, redirectTo string, codeVerifier, nonce *string) (*AuthFlowRecord, string, error) {
	flowID := newUUIDString()
	state, err := randomHexString(24)
	if err != nil {
		return nil, "", err
	}
	record := &AuthFlowRecord{
		FlowID:       flowID,
		Provider:     provider,
		Purpose:      purpose,
		Issuer:       issuer,
		StateHash:    hashToken(state),
		CodeVerifier: codeVerifier,
		Nonce:        nonce,
		RedirectTo:   redirectTo,
		ExpiresAt:    time.Now().Add(controlOAuthFlowTTL),
	}
	if err := s.authRepo.CreateAuthFlow(ctx, record); err != nil {
		return nil, "", err
	}
	return record, state, nil
}

func (s *ControlAuthService) ConsumeAuthFlow(ctx context.Context, flowID, state string) (*AuthFlowRecord, error) {
	return s.authRepo.ConsumeAuthFlow(ctx, strings.TrimSpace(flowID), hashToken(strings.TrimSpace(state)), time.Now())
}

func (s *ControlAuthService) shouldRequireTOTP(ctx context.Context, bundle *IdentityBundle, user *User) bool {
	if bundle == nil || bundle.TOTP == nil || user == nil {
		return false
	}
	if s.settingService == nil || !s.settingService.IsTotpEnabled(ctx) {
		return false
	}
	return bundle.TOTP.Enabled || user.TotpEnabled
}

func (s *ControlAuthService) createAuthenticatedSession(ctx context.Context, bundle *IdentityBundle, user *User, amr string) (*AuthenticatedIdentity, *ControlSessionTokens, error) {
	now := time.Now()
	sessionRecord, refreshRecord, rawRefreshToken, err := s.newSessionRecords(bundle, user, amr, now)
	if err != nil {
		return nil, nil, err
	}
	accessToken, accessExpiry, err := s.signAccessToken(bundle.Subject.SubjectID, sessionRecord.SessionID, bundle.Subject.AuthVersion, amr, now)
	if err != nil {
		return nil, nil, err
	}
	if err := s.authRepo.CreateSession(ctx, sessionRecord, refreshRecord); err != nil {
		return nil, nil, fmt.Errorf("create session: %w", err)
	}

	snapshot := sessionRecordToSnapshot(sessionRecord)
	s.tryStoreSessionSnapshot(ctx, snapshot)

	return buildAuthenticatedIdentity(bundle, snapshot, user), &ControlSessionTokens{
		AccessToken:       accessToken,
		RefreshToken:      rawRefreshToken,
		AccessExpiresAt:   accessExpiry,
		IdleRefreshExpiry: refreshRecord.IdleExpiresAt,
		AbsoluteExpiry:    refreshRecord.AbsoluteExpiresAt,
	}, nil
}

func (s *ControlAuthService) newSessionRecords(bundle *IdentityBundle, user *User, amr string, now time.Time) (*SessionRecord, *RefreshTokenRecord, string, error) {
	if bundle == nil || bundle.Subject == nil {
		return nil, nil, "", fmt.Errorf("identity bundle is required")
	}
	legacyUserID := bundle.Subject.LegacyUserID
	if user != nil && user.ID > 0 {
		legacyUserID = user.ID
	}
	if legacyUserID <= 0 {
		return nil, nil, "", fmt.Errorf("linked app user is required")
	}
	refreshToken, err := randomHexString(32)
	if err != nil {
		return nil, nil, "", err
	}
	sessionID := newUUIDString()

	absoluteExpiry := now.Add(controlRefreshAbsoluteTTL)
	idleExpiry := now.Add(controlRefreshIdleTTL)

	refreshRecord := &RefreshTokenRecord{
		TokenHash:         hashToken(refreshToken),
		SessionID:         sessionID,
		SubjectID:         bundle.Subject.SubjectID,
		LegacyUserID:      legacyUserID,
		CreatedAt:         now,
		IdleExpiresAt:     idleExpiry,
		AbsoluteExpiresAt: absoluteExpiry,
	}
	sessionRecord := &SessionRecord{
		SessionID:               sessionID,
		SubjectID:               bundle.Subject.SubjectID,
		LegacyUserID:            legacyUserID,
		Status:                  StatusActive,
		AMR:                     amr,
		LastSeenAt:              now,
		ExpiresAt:               idleExpiry,
		AbsoluteExpiresAt:       absoluteExpiry,
		CurrentRefreshTokenHash: refreshRecord.TokenHash,
		AuthVersion:             bundle.Subject.AuthVersion,
	}
	return sessionRecord, refreshRecord, refreshToken, nil
}

func (s *ControlAuthService) signAccessToken(subjectID, sessionID string, authVersion int64, amr string, now time.Time) (string, time.Time, error) {
	return s.signSessionToken(subjectID, sessionID, authVersion, amr, controlAccessTokenTTL, now)
}

func (s *ControlAuthService) signSessionToken(subjectID, sessionID string, authVersion int64, amr string, ttl time.Duration, now time.Time) (string, time.Time, error) {
	if s == nil || s.activeSigningKey == nil || s.activeSigningKey.privateKey == nil {
		return "", time.Time{}, fmt.Errorf("signing key not initialized")
	}

	expiresAt := now.Add(ttl)
	claims := &controlAccessClaims{
		SessionID:   sessionID,
		AuthVersion: authVersion,
		AMR:         amr,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   subjectID,
			Audience:  jwt.ClaimStrings{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = s.activeSigningKey.kid
	signed, err := token.SignedString(s.activeSigningKey.privateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

func (s *ControlAuthService) getSessionSnapshot(ctx context.Context, sessionID string) (*SessionSnapshot, error) {
	if s.sessionCache != nil {
		snapshot, err := s.sessionCache.GetSessionSnapshot(ctx, sessionID)
		if err != nil {
			logger.LegacyPrintf("service.control_auth", "session snapshot cache read failed: sid=%s err=%v", sessionID, err)
		}
		if err == nil && snapshot != nil {
			return snapshot, nil
		}
	}

	sessionRecord, err := s.authRepo.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, nil
		}
		return nil, err
	}
	snapshot := sessionRecordToSnapshot(sessionRecord)
	s.tryStoreSessionSnapshot(ctx, snapshot)
	return snapshot, nil
}

func (s *ControlAuthService) storeSessionSnapshot(ctx context.Context, snapshot *SessionSnapshot) error {
	if s.sessionCache == nil || snapshot == nil {
		return nil
	}
	ttl := time.Until(snapshot.ExpiresAt)
	if ttl <= 0 || ttl > controlSessionSnapshotTTL {
		ttl = controlSessionSnapshotTTL
	}
	return s.sessionCache.SetSessionSnapshot(ctx, snapshot, ttl)
}

func (s *ControlAuthService) tryStoreSessionSnapshot(ctx context.Context, snapshot *SessionSnapshot) {
	if snapshot == nil {
		return
	}
	if err := s.storeSessionSnapshot(ctx, snapshot); err != nil {
		logger.LegacyPrintf("service.control_auth", "session snapshot cache write failed: sid=%s err=%v", snapshot.SessionID, err)
	}
}

func (s *ControlAuthService) tryGetLinkedUser(ctx context.Context, legacyUserID int64) *User {
	if s == nil || s.userRepo == nil || legacyUserID <= 0 {
		return nil
	}
	user, err := s.userRepo.GetByID(ctx, legacyUserID)
	if err != nil {
		if !errors.Is(err, ErrUserNotFound) {
			logger.LegacyPrintf("service.control_auth", "linked user lookup failed: user=%d err=%v", legacyUserID, err)
		}
		return nil
	}
	return user
}

func (s *ControlAuthService) frontendBaseURL(ctx context.Context) string {
	if s.settingService != nil {
		if value := strings.TrimSpace(s.settingService.GetFrontendURL(ctx)); value != "" {
			return strings.TrimSuffix(value, "/")
		}
	}
	if s != nil && s.cfg != nil {
		if value := strings.TrimSpace(s.cfg.Server.FrontendURL); value != "" {
			return strings.TrimSuffix(value, "/")
		}
	}
	return ""
}

func sessionRecordToSnapshot(record *SessionRecord) *SessionSnapshot {
	if record == nil {
		return nil
	}
	return &SessionSnapshot{
		SessionID:         record.SessionID,
		SubjectID:         record.SubjectID,
		LegacyUserID:      record.LegacyUserID,
		Status:            record.Status,
		AMR:               record.AMR,
		AuthVersion:       record.AuthVersion,
		ExpiresAt:         record.ExpiresAt,
		AbsoluteExpiresAt: record.AbsoluteExpiresAt,
		RevokedAt:         record.RevokedAt,
		LastSeenAt:        record.LastSeenAt,
	}
}

func buildAuthenticatedIdentity(bundle *IdentityBundle, snapshot *SessionSnapshot, user *User) *AuthenticatedIdentity {
	if bundle == nil || bundle.Subject == nil || snapshot == nil {
		return nil
	}

	roles := append([]string(nil), bundle.Roles...)
	primaryRole := RoleUser
	if user != nil && user.IsAdmin() {
		primaryRole = RoleAdmin
	}
	if len(roles) > 0 {
		primaryRole = roles[0]
	}
	legacyUserID := bundle.Subject.LegacyUserID
	concurrency := 0
	if user != nil {
		legacyUserID = user.ID
		concurrency = user.Concurrency
	}
	return &AuthenticatedIdentity{
		SubjectID:         bundle.Subject.SubjectID,
		SessionID:         snapshot.SessionID,
		LegacyUserID:      legacyUserID,
		AMR:               snapshot.AMR,
		AuthVersion:       bundle.Subject.AuthVersion,
		Roles:             roles,
		PrimaryRole:       primaryRole,
		Profile:           bundle.Profile,
		Concurrency:       concurrency,
		SessionExpiresAt:  snapshot.ExpiresAt,
		SessionAbsoluteAt: snapshot.AbsoluteExpiresAt,
		SessionLastSeenAt: snapshot.LastSeenAt,
	}
}

func newUUIDString() string {
	return uuid.NewString()
}

func buildSigningKey(privateKey *ecdsa.PrivateKey) (*controlSigningKey, error) {
	if privateKey == nil || privateKey.PublicKey.X == nil || privateKey.PublicKey.Y == nil {
		return nil, fmt.Errorf("nil ecdsa private key")
	}
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	sum := sha256.Sum256(publicKeyBytes)
	return &controlSigningKey{
		kid:        hex.EncodeToString(sum[:8]),
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
	}, nil
}

func buildControlJWK(signingKey *controlSigningKey) ControlJWK {
	return ControlJWK{
		Kty: "EC",
		Use: "sig",
		Alg: jwt.SigningMethodES256.Alg(),
		Kid: signingKey.kid,
		Crv: "P-256",
		X:   encodeBigIntURL(signingKey.publicKey.X, 32),
		Y:   encodeBigIntURL(signingKey.publicKey.Y, 32),
	}
}

func parseECDSAPrivateKey(raw string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		return nil, fmt.Errorf("invalid pem block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		ecdsaKey, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("pem is not ecdsa private key")
		}
		return ecdsaKey, nil
	}
	ecdsaKey, ecErr := x509.ParseECPrivateKey(block.Bytes)
	if ecErr != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return ecdsaKey, nil
}

func encodeBigIntURL(value *big.Int, size int) string {
	buf := make([]byte, size)
	bytes := value.Bytes()
	copy(buf[len(buf)-len(bytes):], bytes)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func checkPassword(password, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)) == nil
}
