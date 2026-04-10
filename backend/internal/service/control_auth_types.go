package service

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSessionNotFound               = errors.New("session not found")
	ErrSubjectNotFound               = errors.New("subject not found")
	ErrFederatedIdentityNotFound     = errors.New("federated identity not found")
	ErrRegistrationChallengeNotFound = errors.New("registration challenge not found")
	ErrAuthFlowNotFound              = errors.New("auth flow not found")
	ErrEmailVerificationNotFound     = errors.New("email verification not found")
	ErrPasswordResetTokenNotFound    = errors.New("password reset token not found")
)

type SubjectRecord struct {
	SubjectID    string
	LegacyUserID int64
	Email        string
	Status       string
	AuthVersion  int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type SubjectProfileRecord struct {
	SubjectID    string
	LegacyUserID int64
	Email        string
	Username     string
	Notes        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type FederatedIdentityRecord struct {
	ID              int64
	SubjectID       string
	Provider        string
	Issuer          string
	ExternalSubject string
	Email           string
	Username        string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type TOTPFactorRecord struct {
	SubjectID       string
	SecretEncrypted *string
	Enabled         bool
	EnabledAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type SessionRecord struct {
	SessionID               string
	SubjectID               string
	LegacyUserID            int64
	Status                  string
	AMR                     string
	LastSeenAt              time.Time
	ExpiresAt               time.Time
	AbsoluteExpiresAt       time.Time
	RevokedAt               *time.Time
	CurrentRefreshTokenHash string
	AuthVersion             int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type RefreshTokenRecord struct {
	TokenHash           string
	SessionID           string
	SubjectID           string
	LegacyUserID        int64
	CreatedAt           time.Time
	IdleExpiresAt       time.Time
	AbsoluteExpiresAt   time.Time
	RotatedAt           *time.Time
	RevokedAt           *time.Time
	ReplacedByTokenHash *string
}

type EmailVerificationRecord struct {
	VerificationID string
	SubjectID      *string
	Purpose        string
	Email          string
	CodeHash       string
	ExpiresAt      time.Time
	ConsumedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type PasswordResetTokenRecord struct {
	ResetID    string
	SubjectID  string
	Email      string
	TokenHash  string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type AuthFlowRecord struct {
	FlowID       string
	Provider     string
	Purpose      string
	Issuer       string
	StateHash    string
	CodeVerifier *string
	Nonce        *string
	RedirectTo   string
	ExpiresAt    time.Time
	ConsumedAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type RegistrationChallengeRecord struct {
	ChallengeID       string
	Provider          string
	Issuer            string
	ExternalSubject   string
	Email             string
	RegistrationEmail string
	Username          string
	RedirectTo        string
	ExpiresAt         time.Time
	ConsumedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type IdentityBundle struct {
	Subject *SubjectRecord
	Profile *SubjectProfileRecord
	Roles   []string
	TOTP    *TOTPFactorRecord
}

type ControlAuthRepository interface {
	EnsureSubjectShadow(ctx context.Context, user *User) (*IdentityBundle, error)
	GetIdentityBundleBySubjectID(ctx context.Context, subjectID string) (*IdentityBundle, error)
	GetIdentityBundleByLegacyUserID(ctx context.Context, userID int64) (*IdentityBundle, error)
	GetIdentityBundleByFederatedIdentity(ctx context.Context, provider, issuer, externalSubject string) (*IdentityBundle, error)
	LinkFederatedIdentity(ctx context.Context, identity *FederatedIdentityRecord) error

	CreateSession(ctx context.Context, session *SessionRecord, refreshToken *RefreshTokenRecord) error
	GetSession(ctx context.Context, sessionID string) (*SessionRecord, error)
	GetSessionByRefreshTokenHash(ctx context.Context, tokenHash string) (*SessionRecord, *RefreshTokenRecord, error)
	RotateSessionRefreshToken(ctx context.Context, session *SessionRecord, currentTokenHash string, nextToken *RefreshTokenRecord, rotatedAt time.Time) error
	UpdateSession(ctx context.Context, session *SessionRecord) error
	RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error
	RevokeAllSessions(ctx context.Context, subjectID string, revokedAt time.Time) error
	UpdateSubjectAuthVersion(ctx context.Context, subjectID string, authVersion int64) error

	GetTOTPFactor(ctx context.Context, subjectID string) (*TOTPFactorRecord, error)
	UpsertTOTPFactor(ctx context.Context, factor *TOTPFactorRecord) error

	CreateEmailVerification(ctx context.Context, record *EmailVerificationRecord) error
	ConsumeEmailVerification(ctx context.Context, purpose, email, codeHash string, now time.Time, subjectID *string) (*EmailVerificationRecord, error)

	CreatePasswordResetToken(ctx context.Context, record *PasswordResetTokenRecord) error
	ConsumePasswordResetToken(ctx context.Context, email, tokenHash string, now time.Time) (*PasswordResetTokenRecord, error)

	CreateAuthFlow(ctx context.Context, flow *AuthFlowRecord) error
	GetAuthFlow(ctx context.Context, flowID string) (*AuthFlowRecord, error)
	ConsumeAuthFlow(ctx context.Context, flowID, stateHash string, now time.Time) (*AuthFlowRecord, error)

	CreateRegistrationChallenge(ctx context.Context, challenge *RegistrationChallengeRecord) error
	GetRegistrationChallenge(ctx context.Context, challengeID string) (*RegistrationChallengeRecord, error)
	ConsumeRegistrationChallenge(ctx context.Context, challengeID string, now time.Time) (*RegistrationChallengeRecord, error)
}

type SessionSnapshot struct {
	SessionID         string
	SubjectID         string
	LegacyUserID      int64
	Status            string
	AMR               string
	AuthVersion       int64
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
	RevokedAt         *time.Time
	LastSeenAt        time.Time
}

type SessionSnapshotCache interface {
	GetSessionSnapshot(ctx context.Context, sessionID string) (*SessionSnapshot, error)
	SetSessionSnapshot(ctx context.Context, snapshot *SessionSnapshot, ttl time.Duration) error
	DeleteSessionSnapshot(ctx context.Context, sessionID string) error
}

type ControlSessionTokens struct {
	AccessToken       string
	RefreshToken      string
	AccessExpiresAt   time.Time
	IdleRefreshExpiry time.Time
	AbsoluteExpiry    time.Time
}

type ControlLoginResult struct {
	RequiresMFA      bool
	LoginChallengeID string
	MaskedEmail      string
	Identity         *AuthenticatedIdentity
	Tokens           *ControlSessionTokens
}

type AuthenticatedIdentity struct {
	SubjectID         string
	SessionID         string
	LegacyUserID      int64
	AMR               string
	AuthVersion       int64
	Roles             []string
	PrimaryRole       string
	User              *User
	SessionExpiresAt  time.Time
	SessionAbsoluteAt time.Time
	SessionLastSeenAt time.Time
}

type RegistrationPreflightResult struct {
	RegistrationEnabled       bool     `json:"registration_enabled"`
	EmailVerificationRequired bool     `json:"email_verification_required"`
	InvitationRequired        bool     `json:"invitation_required"`
	EmailSuffixAllowed        bool     `json:"email_suffix_allowed"`
	PromoStatus               string   `json:"promo_status"`
	InvitationStatus          string   `json:"invitation_status"`
	Errors                    []string `json:"errors"`
}
