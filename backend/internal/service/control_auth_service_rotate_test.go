//go:build unit

package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type rotateControlAuthRepoStub struct {
	syncBundle     *IdentityBundle
	calls          []string
	txSeen         map[string]bool
	updatedVersion int64
	createdSession *SessionRecord
}

func (s *rotateControlAuthRepoStub) EnsureSubjectAccount(context.Context, *User) (*IdentityBundle, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) SyncLocalCredentialState(ctx context.Context, user *User) (*IdentityBundle, error) {
	s.calls = append(s.calls, "sync_local")
	s.txSeen["sync_local"] = dbent.TxFromContext(ctx) != nil
	return s.syncBundle, nil
}

func (s *rotateControlAuthRepoStub) GetIdentityBundleBySubjectID(ctx context.Context, subjectID string) (*IdentityBundle, error) {
	s.calls = append(s.calls, "get_bundle")
	s.txSeen["get_bundle"] = dbent.TxFromContext(ctx) != nil
	return s.syncBundle, nil
}

func (s *rotateControlAuthRepoStub) GetIdentityBundleByLegacyUserID(context.Context, int64) (*IdentityBundle, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) GetIdentityBundleByFederatedIdentity(context.Context, string, string, string) (*IdentityBundle, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) LinkFederatedIdentity(context.Context, *FederatedIdentityRecord) error {
	return nil
}

func (s *rotateControlAuthRepoStub) CreateSession(ctx context.Context, session *SessionRecord, refreshToken *RefreshTokenRecord) error {
	s.calls = append(s.calls, "create_session")
	s.txSeen["create_session"] = dbent.TxFromContext(ctx) != nil
	copySession := *session
	s.createdSession = &copySession
	return nil
}

func (s *rotateControlAuthRepoStub) GetSession(context.Context, string) (*SessionRecord, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) GetSessionByRefreshTokenHash(context.Context, string) (*SessionRecord, *RefreshTokenRecord, error) {
	return nil, nil, nil
}

func (s *rotateControlAuthRepoStub) RotateSessionRefreshToken(context.Context, *SessionRecord, string, *RefreshTokenRecord, time.Time) error {
	return nil
}

func (s *rotateControlAuthRepoStub) UpdateSession(context.Context, *SessionRecord) error {
	return nil
}

func (s *rotateControlAuthRepoStub) RevokeSession(context.Context, string, time.Time) error {
	return nil
}

func (s *rotateControlAuthRepoStub) RevokeAllSessions(ctx context.Context, subjectID string, revokedAt time.Time) error {
	s.calls = append(s.calls, "revoke_all")
	s.txSeen["revoke_all"] = dbent.TxFromContext(ctx) != nil
	return nil
}

func (s *rotateControlAuthRepoStub) UpdateSubjectAuthVersion(ctx context.Context, subjectID string, authVersion int64) error {
	s.calls = append(s.calls, "update_auth_version")
	s.txSeen["update_auth_version"] = dbent.TxFromContext(ctx) != nil
	s.updatedVersion = authVersion
	return nil
}

func (s *rotateControlAuthRepoStub) GetTOTPFactor(context.Context, string) (*TOTPFactorRecord, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) UpsertTOTPFactor(context.Context, *TOTPFactorRecord) error {
	return nil
}

func (s *rotateControlAuthRepoStub) CreateEmailVerification(context.Context, *EmailVerificationRecord) error {
	return nil
}

func (s *rotateControlAuthRepoStub) ConsumeEmailVerification(context.Context, string, string, string, time.Time, *string) (*EmailVerificationRecord, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) CreatePasswordResetToken(context.Context, *PasswordResetTokenRecord) error {
	return nil
}

func (s *rotateControlAuthRepoStub) ConsumePasswordResetToken(context.Context, string, string, time.Time) (*PasswordResetTokenRecord, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) CreateAuthFlow(context.Context, *AuthFlowRecord) error {
	return nil
}

func (s *rotateControlAuthRepoStub) GetAuthFlow(context.Context, string) (*AuthFlowRecord, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) ConsumeAuthFlow(context.Context, string, string, time.Time) (*AuthFlowRecord, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) CreateRegistrationChallenge(context.Context, *RegistrationChallengeRecord) error {
	return nil
}

func (s *rotateControlAuthRepoStub) GetRegistrationChallenge(context.Context, string) (*RegistrationChallengeRecord, error) {
	return nil, nil
}

func (s *rotateControlAuthRepoStub) ConsumeRegistrationChallenge(context.Context, string, time.Time) (*RegistrationChallengeRecord, error) {
	return nil, nil
}

type rotateSessionCacheStub struct {
	deleted []string
	stored  []*SessionSnapshot
}

func (s *rotateSessionCacheStub) GetSessionSnapshot(context.Context, string) (*SessionSnapshot, error) {
	return nil, nil
}

func (s *rotateSessionCacheStub) SetSessionSnapshot(ctx context.Context, snapshot *SessionSnapshot, ttl time.Duration) error {
	copySnapshot := *snapshot
	s.stored = append(s.stored, &copySnapshot)
	return nil
}

func (s *rotateSessionCacheStub) DeleteSessionSnapshot(ctx context.Context, sessionID string) error {
	s.deleted = append(s.deleted, sessionID)
	return nil
}

type rotateUserRepoStub struct {
	mockUserRepo
	user *User
}

func (s *rotateUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

func newRotateControlAuthService(t *testing.T, authRepo ControlAuthRepository, userRepo UserRepository, sessionCache SessionSnapshotCache) *ControlAuthService {
	t.Helper()

	dbName := fmt.Sprintf("file:control_auth_rotate_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signingKey, err := buildSigningKey(privateKey)
	require.NoError(t, err)

	return &ControlAuthService{
		entClient:        client,
		authRepo:         authRepo,
		userRepo:         userRepo,
		sessionCache:     sessionCache,
		issuer:           controlIssuerDefault,
		audience:         controlAudienceDefault,
		activeSigningKey: signingKey,
		verificationKeys: map[string]*ecdsa.PublicKey{
			signingKey.kid: signingKey.publicKey,
		},
	}
}

func TestControlAuthServiceRotateCurrentSessionBumpsAuthVersionAndUsesTransaction(t *testing.T) {
	repo := &rotateControlAuthRepoStub{
		syncBundle: &IdentityBundle{
			Subject: &SubjectRecord{
				SubjectID:    "subject-1",
				LegacyUserID: 42,
				Email:        "user@example.com",
				Status:       StatusActive,
				AuthVersion:  7,
			},
			Profile: &SubjectProfileRecord{
				SubjectID: "subject-1",
				Email:     "user@example.com",
			},
			Roles: []string{RoleUser},
			TOTP: &TOTPFactorRecord{
				SubjectID: "subject-1",
			},
		},
		txSeen: map[string]bool{},
	}
	cache := &rotateSessionCacheStub{}
	userRepo := &rotateUserRepoStub{
		user: &User{
			ID:          42,
			Email:       "user@example.com",
			Role:        RoleUser,
			Status:      StatusActive,
			Concurrency: 5,
		},
	}
	svc := newRotateControlAuthService(t, repo, userRepo, cache)

	identity := &AuthenticatedIdentity{
		SubjectID:    "subject-1",
		SessionID:    "session-old",
		LegacyUserID: 42,
		AMR:          "pwd",
		AuthVersion:  7,
	}

	nextIdentity, tokens, err := svc.RotateCurrentSession(context.Background(), identity, "")
	require.NoError(t, err)
	require.Equal(t, []string{"sync_local", "update_auth_version", "revoke_all", "create_session"}, repo.calls)
	require.Equal(t, int64(8), repo.updatedVersion)
	require.NotNil(t, repo.createdSession)
	require.Equal(t, int64(8), repo.createdSession.AuthVersion)
	require.NotEqual(t, identity.SessionID, repo.createdSession.SessionID)
	require.Equal(t, []string{"session-old"}, cache.deleted)
	require.Len(t, cache.stored, 1)
	require.Equal(t, repo.createdSession.SessionID, cache.stored[0].SessionID)
	require.NotNil(t, nextIdentity)
	require.Equal(t, int64(8), nextIdentity.AuthVersion)
	require.NotNil(t, tokens)
	require.NotEmpty(t, tokens.AccessToken)
	require.NotEmpty(t, tokens.RefreshToken)
	for _, step := range []string{"sync_local", "update_auth_version", "revoke_all", "create_session"} {
		require.True(t, repo.txSeen[step], "expected %s to run inside a transaction", step)
	}
}
