package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type controlAuthRepoStub struct {
	ControlAuthRepository
	bundle           *IdentityBundle
	ensureCalled     bool
	session          *SessionRecord
	createSessionErr error
	getSessionErr    error
}

func (s *controlAuthRepoStub) GetIdentityBundleBySubjectID(_ context.Context, subjectID string) (*IdentityBundle, error) {
	if s.bundle == nil || s.bundle.Subject == nil || s.bundle.Subject.SubjectID != subjectID {
		return nil, ErrSubjectNotFound
	}
	return s.bundle, nil
}

func (s *controlAuthRepoStub) EnsureSubjectAccount(_ context.Context, _ *User) (*IdentityBundle, error) {
	s.ensureCalled = true
	if s.bundle == nil {
		return nil, errors.New("identity bundle not configured")
	}
	return s.bundle, nil
}

func (s *controlAuthRepoStub) SyncLocalCredentialState(_ context.Context, _ *User) (*IdentityBundle, error) {
	s.ensureCalled = true
	if s.bundle == nil {
		return nil, errors.New("identity bundle not configured")
	}
	return s.bundle, nil
}

func (s *controlAuthRepoStub) CreateSession(_ context.Context, session *SessionRecord, _ *RefreshTokenRecord) error {
	if s.createSessionErr != nil {
		return s.createSessionErr
	}
	s.session = session
	return nil
}

func (s *controlAuthRepoStub) GetSession(_ context.Context, sessionID string) (*SessionRecord, error) {
	if s.getSessionErr != nil {
		return nil, s.getSessionErr
	}
	if s.session == nil || s.session.SessionID != sessionID {
		return nil, ErrSessionNotFound
	}
	return s.session, nil
}

type controlAuthUserRepoStub struct {
	UserRepository
	user *User
}

func (s *controlAuthUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	if s.user == nil || s.user.ID != id {
		return nil, ErrUserNotFound
	}
	return s.user, nil
}

type controlAuthSessionCacheStub struct {
	SessionSnapshotCache
	snapshot *SessionSnapshot
	getErr   error
	setErr   error
	setCalls int
}

func (s *controlAuthSessionCacheStub) GetSessionSnapshot(_ context.Context, sessionID string) (*SessionSnapshot, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.snapshot == nil || s.snapshot.SessionID != sessionID {
		return nil, nil
	}
	return s.snapshot, nil
}

func (s *controlAuthSessionCacheStub) SetSessionSnapshot(_ context.Context, snapshot *SessionSnapshot, _ time.Duration) error {
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.snapshot = snapshot
	return nil
}

func newTestControlSigningKey(t *testing.T) *controlSigningKey {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	signingKey, err := buildSigningKey(privateKey)
	require.NoError(t, err)

	return signingKey
}

func TestControlAuthService_AuthenticateAccessToken_UsesReadOnlyIdentityLookup(t *testing.T) {
	signingKey := newTestControlSigningKey(t)

	now := time.Now()
	snapshot := &SessionSnapshot{
		SessionID:         "0fa843c7-7882-4986-af28-d6a5c59106b3",
		SubjectID:         "50ee32fe-4372-443e-acb0-5f65c9c7a2bf",
		LegacyUserID:      42,
		Status:            StatusActive,
		AMR:               "pwd",
		AuthVersion:       7,
		ExpiresAt:         now.Add(10 * time.Minute),
		AbsoluteExpiresAt: now.Add(24 * time.Hour),
		LastSeenAt:        now,
	}

	repo := &controlAuthRepoStub{
		bundle: &IdentityBundle{
			Subject: &SubjectRecord{
				SubjectID:    snapshot.SubjectID,
				LegacyUserID: snapshot.LegacyUserID,
				Email:        "test@example.com",
				Status:       StatusActive,
				AuthVersion:  snapshot.AuthVersion,
			},
			Profile: &SubjectProfileRecord{
				SubjectID:    snapshot.SubjectID,
				LegacyUserID: snapshot.LegacyUserID,
				Email:        "test@example.com",
				Username:     "tester",
			},
			Roles: []string{RoleUser},
			TOTP:  &TOTPFactorRecord{SubjectID: snapshot.SubjectID},
		},
	}

	svc := &ControlAuthService{
		userRepo: &controlAuthUserRepoStub{
			user: &User{
				ID:       snapshot.LegacyUserID,
				Email:    "test@example.com",
				Username: "tester",
				Role:     RoleUser,
				Status:   StatusActive,
			},
		},
		authRepo:         repo,
		sessionCache:     &controlAuthSessionCacheStub{snapshot: snapshot},
		issuer:           "https://control.example.test",
		audience:         "https://control.example.test",
		activeSigningKey: signingKey,
		verificationKeys: map[string]*ecdsa.PublicKey{
			signingKey.kid: signingKey.publicKey,
		},
	}

	token, _, err := svc.signAccessToken(snapshot.SubjectID, snapshot.SessionID, snapshot.AuthVersion, snapshot.AMR, now)
	require.NoError(t, err)

	identity, err := svc.AuthenticateAccessToken(context.Background(), token)
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, snapshot.SubjectID, identity.SubjectID)
	require.Equal(t, snapshot.SessionID, identity.SessionID)
	require.False(t, repo.ensureCalled)
}

func TestControlAuthService_GetSessionSnapshot_FallsBackToRepositoryWhenCacheFails(t *testing.T) {
	now := time.Now()
	sessionRecord := &SessionRecord{
		SessionID:         "0fa843c7-7882-4986-af28-d6a5c59106b3",
		SubjectID:         "50ee32fe-4372-443e-acb0-5f65c9c7a2bf",
		LegacyUserID:      42,
		Status:            StatusActive,
		AMR:               "pwd",
		AuthVersion:       7,
		ExpiresAt:         now.Add(10 * time.Minute),
		AbsoluteExpiresAt: now.Add(24 * time.Hour),
		LastSeenAt:        now,
	}

	cache := &controlAuthSessionCacheStub{
		getErr: errors.New("redis unavailable"),
		setErr: errors.New("redis unavailable"),
	}
	svc := &ControlAuthService{
		authRepo:     &controlAuthRepoStub{session: sessionRecord},
		sessionCache: cache,
	}

	snapshot, err := svc.getSessionSnapshot(context.Background(), sessionRecord.SessionID)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, sessionRecord.SessionID, snapshot.SessionID)
	require.Equal(t, sessionRecord.SubjectID, snapshot.SubjectID)
	require.Equal(t, 1, cache.setCalls)
}

func TestControlAuthService_CreateAuthenticatedSession_IgnoresSnapshotCacheFailure(t *testing.T) {
	signingKey := newTestControlSigningKey(t)
	repo := &controlAuthRepoStub{
		bundle: &IdentityBundle{
			Subject: &SubjectRecord{
				SubjectID:    "50ee32fe-4372-443e-acb0-5f65c9c7a2bf",
				LegacyUserID: 42,
				Email:        "test@example.com",
				Status:       StatusActive,
				AuthVersion:  7,
			},
			Roles: []string{RoleUser},
		},
	}
	cache := &controlAuthSessionCacheStub{setErr: errors.New("redis unavailable")}
	user := &User{
		ID:       42,
		Email:    "test@example.com",
		Username: "tester",
		Role:     RoleUser,
		Status:   StatusActive,
	}
	svc := &ControlAuthService{
		authRepo:         repo,
		sessionCache:     cache,
		issuer:           "https://control.example.test",
		audience:         "https://control.example.test",
		activeSigningKey: signingKey,
		verificationKeys: map[string]*ecdsa.PublicKey{
			signingKey.kid: signingKey.publicKey,
		},
	}

	identity, tokens, err := svc.createAuthenticatedSession(context.Background(), repo.bundle, user, "pwd")
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.NotNil(t, tokens)
	require.Equal(t, repo.bundle.Subject.SubjectID, identity.SubjectID)
	require.Equal(t, 1, cache.setCalls)
	require.NotNil(t, repo.session)
	require.NotEmpty(t, tokens.RefreshToken)
	require.NotEmpty(t, tokens.AccessToken)
}
