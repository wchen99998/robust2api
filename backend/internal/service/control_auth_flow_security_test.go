//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

type securityControlAuthRepoStub struct {
	rotateControlAuthRepoStub
	bundleBySubject    *IdentityBundle
	bundleByFederated  *IdentityBundle
	passwordResetToken *PasswordResetTokenRecord
	sessionRecord      *SessionRecord
	refreshRecord      *RefreshTokenRecord
	revokeSessionErr   error
	revokeAllErr       error
	createSessionErr   error
}

func newSecurityControlAuthRepoStub() *securityControlAuthRepoStub {
	return &securityControlAuthRepoStub{
		rotateControlAuthRepoStub: rotateControlAuthRepoStub{
			txSeen: map[string]bool{},
		},
	}
}

func (s *securityControlAuthRepoStub) GetIdentityBundleBySubjectID(ctx context.Context, subjectID string) (*IdentityBundle, error) {
	s.calls = append(s.calls, "get_bundle")
	s.txSeen["get_bundle"] = dbent.TxFromContext(ctx) != nil
	if s.bundleBySubject != nil {
		return s.bundleBySubject, nil
	}
	return s.syncBundle, nil
}

func (s *securityControlAuthRepoStub) GetIdentityBundleByFederatedIdentity(ctx context.Context, provider, issuer, externalSubject string) (*IdentityBundle, error) {
	s.calls = append(s.calls, "get_federated_bundle")
	s.txSeen["get_federated_bundle"] = dbent.TxFromContext(ctx) != nil
	if s.bundleByFederated == nil {
		return nil, ErrFederatedIdentityNotFound
	}
	return s.bundleByFederated, nil
}

func (s *securityControlAuthRepoStub) GetSessionByRefreshTokenHash(ctx context.Context, tokenHash string) (*SessionRecord, *RefreshTokenRecord, error) {
	s.calls = append(s.calls, "get_session_by_refresh")
	s.txSeen["get_session_by_refresh"] = dbent.TxFromContext(ctx) != nil
	if s.sessionRecord == nil || s.refreshRecord == nil {
		return nil, nil, ErrSessionNotFound
	}
	return s.sessionRecord, s.refreshRecord, nil
}

func (s *securityControlAuthRepoStub) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error {
	s.calls = append(s.calls, "revoke_session")
	s.txSeen["revoke_session"] = dbent.TxFromContext(ctx) != nil
	return s.revokeSessionErr
}

func (s *securityControlAuthRepoStub) RevokeAllSessions(ctx context.Context, subjectID string, revokedAt time.Time) error {
	s.calls = append(s.calls, "revoke_all")
	s.txSeen["revoke_all"] = dbent.TxFromContext(ctx) != nil
	return s.revokeAllErr
}

func (s *securityControlAuthRepoStub) CreateSession(ctx context.Context, session *SessionRecord, refreshToken *RefreshTokenRecord) error {
	s.calls = append(s.calls, "create_session")
	s.txSeen["create_session"] = dbent.TxFromContext(ctx) != nil
	if session != nil {
		copySession := *session
		s.createdSession = &copySession
	}
	return s.createSessionErr
}

func (s *securityControlAuthRepoStub) ConsumePasswordResetToken(ctx context.Context, email, tokenHash string, now time.Time) (*PasswordResetTokenRecord, error) {
	s.calls = append(s.calls, "consume_password_reset")
	s.txSeen["consume_password_reset"] = dbent.TxFromContext(ctx) != nil
	if s.passwordResetToken == nil {
		return nil, ErrPasswordResetTokenNotFound
	}
	return s.passwordResetToken, nil
}

type securitySessionCacheStub struct {
	rotateSessionCacheStub
	snapshot *SessionSnapshot
}

func (s *securitySessionCacheStub) GetSessionSnapshot(context.Context, string) (*SessionSnapshot, error) {
	return s.snapshot, nil
}

type controlAuthSettingRepoStub struct {
	values map[string]string
}

func (s *controlAuthSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (s *controlAuthSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (s *controlAuthSettingRepoStub) Set(context.Context, string, string) error { return nil }
func (s *controlAuthSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (s *controlAuthSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (s *controlAuthSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (s *controlAuthSettingRepoStub) Delete(context.Context, string) error { return nil }

func mustCreateControlAuthTestUser(t *testing.T, svc *ControlAuthService, email, password, role, status, username, notes string, concurrency int) *User {
	t.Helper()

	passwordHash, err := hashPassword(password)
	require.NoError(t, err)

	entity, err := svc.entClient.User.Create().
		SetEmail(email).
		SetPasswordHash(passwordHash).
		SetRole(role).
		SetBalance(0).
		SetConcurrency(concurrency).
		SetStatus(status).
		SetUsername(username).
		SetNotes(notes).
		SetTotpEnabled(false).
		Save(context.Background())
	require.NoError(t, err)

	return controlAuthUserEntityToService(entity)
}

func mustIssueAccessToken(t *testing.T, svc *ControlAuthService, snapshot *SessionSnapshot) string {
	t.Helper()

	token, _, err := svc.signAccessToken(snapshot.SubjectID, snapshot.SessionID, snapshot.AuthVersion, snapshot.AMR, time.Now())
	require.NoError(t, err)
	return token
}

func TestAuthenticateAccessTokenRejectsMissingOrInactiveLiveUser(t *testing.T) {
	t.Run("missing user revokes token", func(t *testing.T) {
		repo := newSecurityControlAuthRepoStub()
		repo.bundleBySubject = &IdentityBundle{
			Subject: &SubjectRecord{
				SubjectID:    "subject-missing",
				LegacyUserID: 42,
				Email:        "missing@example.com",
				Status:       StatusActive,
				AuthVersion:  3,
			},
			Profile: &SubjectProfileRecord{
				SubjectID:    "subject-missing",
				LegacyUserID: 42,
				Email:        "missing@example.com",
			},
			Roles: []string{RoleUser},
		}
		cache := &securitySessionCacheStub{
			snapshot: &SessionSnapshot{
				SessionID:         "session-missing",
				SubjectID:         "subject-missing",
				LegacyUserID:      42,
				Status:            StatusActive,
				AMR:               "pwd",
				AuthVersion:       3,
				ExpiresAt:         time.Now().Add(time.Minute),
				AbsoluteExpiresAt: time.Now().Add(time.Hour),
				LastSeenAt:        time.Now(),
			},
		}
		svc := newRotateControlAuthService(t, repo, &mockUserRepo{}, cache)

		_, err := svc.AuthenticateAccessToken(context.Background(), mustIssueAccessToken(t, svc, cache.snapshot))
		require.ErrorIs(t, err, ErrTokenRevoked)
	})

	t.Run("inactive user is rejected", func(t *testing.T) {
		repo := newSecurityControlAuthRepoStub()
		cache := &securitySessionCacheStub{}
		svc := newRotateControlAuthService(t, repo, &mockUserRepo{}, cache)
		user := mustCreateControlAuthTestUser(t, svc, "disabled@example.com", "old-password", RoleUser, StatusDisabled, "disabled", "", 2)

		repo.bundleBySubject = &IdentityBundle{
			Subject: &SubjectRecord{
				SubjectID:    "subject-disabled",
				LegacyUserID: user.ID,
				Email:        user.Email,
				Status:       StatusActive,
				AuthVersion:  5,
			},
			Profile: &SubjectProfileRecord{
				SubjectID:    "subject-disabled",
				LegacyUserID: user.ID,
				Email:        "stale@example.com",
				Username:     "stale",
			},
			Roles: []string{RoleAdmin},
		}
		cache.snapshot = &SessionSnapshot{
			SessionID:         "session-disabled",
			SubjectID:         "subject-disabled",
			LegacyUserID:      user.ID,
			Status:            StatusActive,
			AMR:               "pwd",
			AuthVersion:       5,
			ExpiresAt:         time.Now().Add(time.Minute),
			AbsoluteExpiresAt: time.Now().Add(time.Hour),
			LastSeenAt:        time.Now(),
		}

		_, err := svc.AuthenticateAccessToken(context.Background(), mustIssueAccessToken(t, svc, cache.snapshot))
		require.ErrorIs(t, err, ErrUserNotActive)
	})
}

func TestAuthenticateAccessTokenUsesLiveUserRoleAndProfile(t *testing.T) {
	repo := newSecurityControlAuthRepoStub()
	cache := &securitySessionCacheStub{}
	svc := newRotateControlAuthService(t, repo, &mockUserRepo{}, cache)
	user := mustCreateControlAuthTestUser(t, svc, "current@example.com", "old-password", RoleUser, StatusActive, "current-user", "live notes", 9)

	repo.bundleBySubject = &IdentityBundle{
		Subject: &SubjectRecord{
			SubjectID:    "subject-current",
			LegacyUserID: user.ID,
			Email:        "stale@example.com",
			Status:       StatusActive,
			AuthVersion:  7,
		},
		Profile: &SubjectProfileRecord{
			SubjectID:    "subject-current",
			LegacyUserID: user.ID,
			Email:        "stale@example.com",
			Username:     "stale-user",
			Notes:        "stale notes",
		},
		Roles: []string{RoleAdmin},
	}
	cache.snapshot = &SessionSnapshot{
		SessionID:         "session-current",
		SubjectID:         "subject-current",
		LegacyUserID:      user.ID,
		Status:            StatusActive,
		AMR:               "pwd",
		AuthVersion:       7,
		ExpiresAt:         time.Now().Add(time.Minute),
		AbsoluteExpiresAt: time.Now().Add(time.Hour),
		LastSeenAt:        time.Now(),
	}

	identity, err := svc.AuthenticateAccessToken(context.Background(), mustIssueAccessToken(t, svc, cache.snapshot))
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, []string{RoleUser}, identity.Roles)
	require.Equal(t, RoleUser, identity.PrimaryRole)
	require.NotNil(t, identity.Profile)
	require.Equal(t, user.Email, identity.Profile.Email)
	require.Equal(t, user.Username, identity.Profile.Username)
	require.Equal(t, user.Notes, identity.Profile.Notes)
	require.Equal(t, user.Concurrency, identity.Concurrency)
}

func TestRefreshSessionRevokesWhenLiveUserInactive(t *testing.T) {
	repo := newSecurityControlAuthRepoStub()
	cache := &securitySessionCacheStub{}
	svc := newRotateControlAuthService(t, repo, &mockUserRepo{}, cache)
	user := mustCreateControlAuthTestUser(t, svc, "refresh@example.com", "old-password", RoleUser, StatusDisabled, "refresh-user", "", 1)

	rawRefreshToken := "refresh-token"
	now := time.Now()
	repo.sessionRecord = &SessionRecord{
		SessionID:               "session-refresh",
		SubjectID:               "subject-refresh",
		LegacyUserID:            user.ID,
		Status:                  StatusActive,
		AMR:                     "pwd",
		LastSeenAt:              now,
		ExpiresAt:               now.Add(time.Hour),
		AbsoluteExpiresAt:       now.Add(24 * time.Hour),
		CurrentRefreshTokenHash: hashToken(rawRefreshToken),
		AuthVersion:             5,
	}
	repo.refreshRecord = &RefreshTokenRecord{
		TokenHash:         hashToken(rawRefreshToken),
		SessionID:         "session-refresh",
		SubjectID:         "subject-refresh",
		LegacyUserID:      user.ID,
		CreatedAt:         now,
		IdleExpiresAt:     now.Add(time.Hour),
		AbsoluteExpiresAt: now.Add(24 * time.Hour),
	}
	repo.bundleBySubject = &IdentityBundle{
		Subject: &SubjectRecord{
			SubjectID:    "subject-refresh",
			LegacyUserID: user.ID,
			Email:        user.Email,
			Status:       StatusActive,
			AuthVersion:  5,
		},
		Profile: &SubjectProfileRecord{
			SubjectID:    "subject-refresh",
			LegacyUserID: user.ID,
			Email:        user.Email,
		},
		Roles: []string{RoleUser},
	}

	identity, tokens, err := svc.RefreshSession(context.Background(), rawRefreshToken)
	require.ErrorIs(t, err, ErrTokenRevoked)
	require.Nil(t, identity)
	require.Nil(t, tokens)
	require.Contains(t, repo.calls, "revoke_session")
	require.True(t, repo.txSeen["get_session_by_refresh"])
	require.True(t, repo.txSeen["get_bundle"])
	require.True(t, repo.txSeen["revoke_session"])
	require.Equal(t, []string{"session-refresh"}, cache.deleted)
}

func TestCompleteExternalLoginRejectsInactiveLinkedUser(t *testing.T) {
	repo := newSecurityControlAuthRepoStub()
	cache := &securitySessionCacheStub{}
	svc := newRotateControlAuthService(t, repo, &mockUserRepo{}, cache)
	user := mustCreateControlAuthTestUser(t, svc, "oauth@example.com", "old-password", RoleUser, StatusDisabled, "oauth-user", "", 1)

	repo.bundleByFederated = &IdentityBundle{
		Subject: &SubjectRecord{
			SubjectID:    "subject-oauth",
			LegacyUserID: user.ID,
			Email:        user.Email,
			Status:       StatusActive,
			AuthVersion:  2,
		},
		Profile: &SubjectProfileRecord{
			SubjectID:    "subject-oauth",
			LegacyUserID: user.ID,
			Email:        user.Email,
		},
		Roles: []string{RoleUser},
	}

	result, err := svc.CompleteExternalLogin(context.Background(), &ControlExternalLoginRequest{
		Identity: &ExternalIdentityProfile{
			Provider: "oidc",
			Issuer:   "https://issuer.example.com",
			Subject:  "external-subject",
		},
		AMR: "oidc",
	})
	require.ErrorIs(t, err, ErrUserNotActive)
	require.Nil(t, result)
	require.Nil(t, repo.createdSession)
	require.NotContains(t, repo.calls, "sync_local")
}

func TestResetPasswordRollsBackOnRevocationFailure(t *testing.T) {
	repo := newSecurityControlAuthRepoStub()
	repo.passwordResetToken = &PasswordResetTokenRecord{
		ResetID:   "reset-1",
		SubjectID: "subject-reset",
		Email:     "reset@example.com",
		TokenHash: hashToken("reset-token"),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	repo.syncBundle = &IdentityBundle{
		Subject: &SubjectRecord{
			SubjectID:    "subject-reset",
			LegacyUserID: 1,
			Email:        "reset@example.com",
			Status:       StatusActive,
			AuthVersion:  4,
		},
		Profile: &SubjectProfileRecord{
			SubjectID: "subject-reset",
			Email:     "reset@example.com",
		},
		Roles: []string{RoleUser},
	}
	repo.revokeAllErr = errors.New("revoke all failed")

	svc := newRotateControlAuthService(t, repo, &mockUserRepo{}, &securitySessionCacheStub{})
	svc.settingService = &SettingService{
		settingRepo: &controlAuthSettingRepoStub{
			values: map[string]string{
				SettingKeyEmailVerifyEnabled:   "true",
				SettingKeyPasswordResetEnabled: "true",
			},
		},
	}
	user := mustCreateControlAuthTestUser(t, svc, "reset@example.com", "old-password", RoleUser, StatusActive, "reset-user", "", 1)
	repo.syncBundle.Subject.LegacyUserID = user.ID

	err := svc.ResetPassword(context.Background(), user.Email, "reset-token", "new-password")
	require.ErrorContains(t, err, "revoke all failed")
	require.True(t, repo.txSeen["consume_password_reset"])
	require.True(t, repo.txSeen["sync_local"])
	require.True(t, repo.txSeen["revoke_all"])

	current, loadErr := svc.getCurrentUserByID(context.Background(), user.ID)
	require.NoError(t, loadErr)
	require.True(t, checkPassword("old-password", current.PasswordHash))
	require.False(t, checkPassword("new-password", current.PasswordHash))
}

func TestChangePasswordRollsBackWhenReplacementSessionCreationFails(t *testing.T) {
	repo := newSecurityControlAuthRepoStub()
	repo.syncBundle = &IdentityBundle{
		Subject: &SubjectRecord{
			SubjectID:    "subject-change",
			LegacyUserID: 1,
			Email:        "change@example.com",
			Status:       StatusActive,
			AuthVersion:  8,
		},
		Profile: &SubjectProfileRecord{
			SubjectID: "subject-change",
			Email:     "change@example.com",
		},
		Roles: []string{RoleUser},
	}
	repo.createSessionErr = errors.New("create session failed")
	cache := &securitySessionCacheStub{}
	svc := newRotateControlAuthService(t, repo, &mockUserRepo{}, cache)
	user := mustCreateControlAuthTestUser(t, svc, "change@example.com", "old-password", RoleUser, StatusActive, "change-user", "", 1)
	repo.syncBundle.Subject.LegacyUserID = user.ID

	nextIdentity, tokens, err := svc.ChangePassword(context.Background(), &AuthenticatedIdentity{
		SubjectID:    "subject-change",
		SessionID:    "session-old",
		LegacyUserID: user.ID,
		AMR:          "pwd",
		AuthVersion:  8,
	}, "old-password", "new-password")
	require.ErrorContains(t, err, "create session failed")
	require.Nil(t, nextIdentity)
	require.Nil(t, tokens)
	require.True(t, repo.txSeen["sync_local"])
	require.True(t, repo.txSeen["revoke_all"])
	require.True(t, repo.txSeen["create_session"])
	require.Empty(t, cache.deleted)
	require.Empty(t, cache.stored)

	current, loadErr := svc.getCurrentUserByID(context.Background(), user.ID)
	require.NoError(t, loadErr)
	require.True(t, checkPassword("old-password", current.PasswordHash))
	require.False(t, checkPassword("new-password", current.PasswordHash))
}
