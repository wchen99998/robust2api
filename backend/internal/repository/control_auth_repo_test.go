package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestControlAuthRepositoryConsumeEmailVerificationUsesAtomicUpdate(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &controlAuthRepository{sql: db}

	now := time.Date(2026, 4, 11, 8, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	createdAt := now.Add(-5 * time.Minute)
	updatedAt := now.Add(-2 * time.Minute)
	subjectID := "subject-1"

	mock.ExpectQuery("UPDATE auth_email_verifications").
		WithArgs("register", "user@example.com", "code-hash", subjectID, now).
		WillReturnRows(sqlmock.NewRows([]string{
			"verification_id", "subject_id", "purpose", "email", "code_hash", "expires_at", "consumed_at", "created_at", "updated_at",
		}).AddRow("verification-1", subjectID, "register", "user@example.com", "code-hash", expiresAt, now, createdAt, updatedAt))

	record, err := repo.ConsumeEmailVerification(context.Background(), "register", "user@example.com", "code-hash", now, &subjectID)
	require.NoError(t, err)
	require.Equal(t, "verification-1", record.VerificationID)
	require.Equal(t, subjectID, *record.SubjectID)
	require.NotNil(t, record.ConsumedAt)
	require.Equal(t, now, *record.ConsumedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestControlAuthRepositoryConsumeEmailVerificationNotFound(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &controlAuthRepository{sql: db}

	now := time.Date(2026, 4, 11, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery("UPDATE auth_email_verifications").
		WithArgs("register", "user@example.com", "code-hash", nil, now).
		WillReturnRows(sqlmock.NewRows([]string{
			"verification_id", "subject_id", "purpose", "email", "code_hash", "expires_at", "consumed_at", "created_at", "updated_at",
		}))

	record, err := repo.ConsumeEmailVerification(context.Background(), "register", "user@example.com", "code-hash", now, nil)
	require.Nil(t, record)
	require.ErrorIs(t, err, service.ErrEmailVerificationNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestControlAuthRepositoryConsumePasswordResetTokenUsesAtomicUpdate(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &controlAuthRepository{sql: db}

	now := time.Date(2026, 4, 11, 8, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	createdAt := now.Add(-5 * time.Minute)
	updatedAt := now.Add(-2 * time.Minute)

	mock.ExpectQuery("UPDATE auth_password_reset_tokens").
		WithArgs("user@example.com", "reset-hash", now).
		WillReturnRows(sqlmock.NewRows([]string{
			"reset_id", "subject_id", "email", "token_hash", "expires_at", "consumed_at", "created_at", "updated_at",
		}).AddRow("reset-1", "subject-1", "user@example.com", "reset-hash", expiresAt, now, createdAt, updatedAt))

	record, err := repo.ConsumePasswordResetToken(context.Background(), "user@example.com", "reset-hash", now)
	require.NoError(t, err)
	require.Equal(t, "reset-1", record.ResetID)
	require.NotNil(t, record.ConsumedAt)
	require.Equal(t, now, *record.ConsumedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestControlAuthRepositoryConsumePasswordResetTokenNotFound(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &controlAuthRepository{sql: db}

	now := time.Date(2026, 4, 11, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery("UPDATE auth_password_reset_tokens").
		WithArgs("user@example.com", "reset-hash", now).
		WillReturnRows(sqlmock.NewRows([]string{
			"reset_id", "subject_id", "email", "token_hash", "expires_at", "consumed_at", "created_at", "updated_at",
		}))

	record, err := repo.ConsumePasswordResetToken(context.Background(), "user@example.com", "reset-hash", now)
	require.Nil(t, record)
	require.ErrorIs(t, err, service.ErrPasswordResetTokenNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestControlAuthRepositoryConsumeAuthFlowUsesAtomicUpdate(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &controlAuthRepository{sql: db}

	now := time.Date(2026, 4, 11, 8, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	createdAt := now.Add(-5 * time.Minute)
	updatedAt := now.Add(-2 * time.Minute)
	codeVerifier := "verifier"
	nonce := "nonce"

	mock.ExpectQuery("UPDATE auth_flows").
		WithArgs("flow-1", "state-hash", now).
		WillReturnRows(sqlmock.NewRows([]string{
			"flow_id", "provider", "purpose", "issuer", "state_hash", "code_verifier", "nonce", "redirect_to", "expires_at", "consumed_at", "created_at", "updated_at",
		}).AddRow("flow-1", "google", "login", "issuer", "state-hash", codeVerifier, nonce, "/dashboard", expiresAt, now, createdAt, updatedAt))

	record, err := repo.ConsumeAuthFlow(context.Background(), "flow-1", "state-hash", now)
	require.NoError(t, err)
	require.Equal(t, "flow-1", record.FlowID)
	require.NotNil(t, record.CodeVerifier)
	require.Equal(t, codeVerifier, *record.CodeVerifier)
	require.NotNil(t, record.Nonce)
	require.Equal(t, nonce, *record.Nonce)
	require.NotNil(t, record.ConsumedAt)
	require.Equal(t, now, *record.ConsumedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestControlAuthRepositoryConsumeAuthFlowNotFound(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &controlAuthRepository{sql: db}

	now := time.Date(2026, 4, 11, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery("UPDATE auth_flows").
		WithArgs("flow-1", "state-hash", now).
		WillReturnRows(sqlmock.NewRows([]string{
			"flow_id", "provider", "purpose", "issuer", "state_hash", "code_verifier", "nonce", "redirect_to", "expires_at", "consumed_at", "created_at", "updated_at",
		}))

	record, err := repo.ConsumeAuthFlow(context.Background(), "flow-1", "state-hash", now)
	require.Nil(t, record)
	require.ErrorIs(t, err, service.ErrAuthFlowNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestControlAuthRepositoryEnsureSubjectAccountUsesAtomicUpsert(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &controlAuthRepository{sql: db}

	user := &service.User{
		ID:       42,
		Email:    "user@example.com",
		Status:   service.StatusActive,
		Username: "tester",
		Notes:    "hello",
		Role:     service.RoleUser,
	}
	subjectID := "subject-1"
	authVersion := int64(3)
	createdAt := time.Date(2026, 4, 11, 8, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)

	mock.ExpectQuery("INSERT INTO auth_subjects").
		WithArgs(sqlmock.AnyArg(), user.ID, user.Email, user.Status, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{
			"subject_id", "legacy_user_id", "email", "status", "auth_version", "created_at", "updated_at",
		}).AddRow(subjectID, user.ID, user.Email, user.Status, authVersion, createdAt, updatedAt))

	mock.ExpectExec("UPDATE users").
		WithArgs(user.ID, subjectID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec("INSERT INTO control_user_profiles").
		WithArgs(subjectID, user.ID, user.Email, user.Username, user.Notes).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec("DELETE FROM control_subject_roles").
		WithArgs(subjectID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO control_subject_roles").
		WithArgs(subjectID, user.Role).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery("SELECT subject_id, legacy_user_id, email, username, notes, created_at, updated_at\\s+FROM control_user_profiles").
		WithArgs(subjectID).
		WillReturnRows(sqlmock.NewRows([]string{
			"subject_id", "legacy_user_id", "email", "username", "notes", "created_at", "updated_at",
		}).AddRow(subjectID, user.ID, user.Email, user.Username, user.Notes, createdAt, updatedAt))

	mock.ExpectQuery("SELECT role\\s+FROM control_subject_roles").
		WithArgs(subjectID).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(user.Role))

	mock.ExpectQuery("SELECT subject_id, secret_encrypted, enabled, enabled_at, created_at, updated_at\\s+FROM auth_mfa_totp_factors").
		WithArgs(subjectID).
		WillReturnRows(sqlmock.NewRows([]string{
			"subject_id", "secret_encrypted", "enabled", "enabled_at", "created_at", "updated_at",
		}))

	bundle, err := repo.EnsureSubjectAccount(context.Background(), user)
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Subject)
	require.Equal(t, subjectID, bundle.Subject.SubjectID)
	require.Equal(t, authVersion, bundle.Subject.AuthVersion)
	require.NotNil(t, bundle.Profile)
	require.Equal(t, user.Email, bundle.Profile.Email)
	require.Equal(t, []string{user.Role}, bundle.Roles)
	require.NotNil(t, bundle.TOTP)
	require.Equal(t, subjectID, bundle.TOTP.SubjectID)
	require.NoError(t, mock.ExpectationsWereMet())
}
