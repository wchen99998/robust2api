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
	bundle       *IdentityBundle
	ensureCalled bool
}

func (s *controlAuthRepoStub) GetIdentityBundleBySubjectID(_ context.Context, subjectID string) (*IdentityBundle, error) {
	if s.bundle == nil || s.bundle.Subject == nil || s.bundle.Subject.SubjectID != subjectID {
		return nil, ErrSubjectNotFound
	}
	return s.bundle, nil
}

func (s *controlAuthRepoStub) EnsureSubjectShadow(_ context.Context, _ *User) (*IdentityBundle, error) {
	s.ensureCalled = true
	return nil, errors.New("EnsureSubjectShadow should not be called during access token authentication")
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
}

func (s *controlAuthSessionCacheStub) GetSessionSnapshot(_ context.Context, sessionID string) (*SessionSnapshot, error) {
	if s.snapshot == nil || s.snapshot.SessionID != sessionID {
		return nil, nil
	}
	return s.snapshot, nil
}

func TestControlAuthService_AuthenticateAccessToken_UsesReadOnlyIdentityLookup(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	signingKey, err := buildSigningKey(privateKey)
	require.NoError(t, err)

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
