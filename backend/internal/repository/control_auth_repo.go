package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

type controlAuthRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewControlAuthRepository(client *dbent.Client, sqlDB *sql.DB) service.ControlAuthRepository {
	return &controlAuthRepository{
		client: client,
		sql:    sqlDB,
	}
}

func (r *controlAuthRepository) EnsureSubjectShadow(ctx context.Context, user *service.User) (*service.IdentityBundle, error) {
	if user == nil {
		return nil, fmt.Errorf("nil user")
	}

	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}

	subject, err := r.getSubjectByLegacyUserID(ctx, exec, user.ID)
	if err != nil && !errors.Is(err, service.ErrSubjectNotFound) {
		return nil, err
	}
	if errors.Is(err, service.ErrSubjectNotFound) {
		subject = &service.SubjectRecord{
			SubjectID:    uuid.NewString(),
			LegacyUserID: user.ID,
			Email:        user.Email,
			Status:       user.Status,
			AuthVersion:  1,
		}
		if _, err := exec.ExecContext(
			ctx,
			`INSERT INTO auth_subjects (subject_id, legacy_user_id, email, status, auth_version, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`,
			subject.SubjectID, subject.LegacyUserID, subject.Email, subject.Status, subject.AuthVersion,
		); err != nil {
			return nil, fmt.Errorf("insert auth_subject: %w", err)
		}
	}

	changedAuthVersion := false
	currentAuthVersion := subject.AuthVersion

	passwordChanged, err := r.syncPasswordCredential(ctx, exec, subject.SubjectID, user.PasswordHash)
	if err != nil {
		return nil, err
	}
	if passwordChanged {
		currentAuthVersion++
		changedAuthVersion = true
	}

	totpChanged, err := r.syncTOTPFactor(ctx, exec, subject.SubjectID, user)
	if err != nil {
		return nil, err
	}
	if totpChanged {
		currentAuthVersion++
		changedAuthVersion = true
	}

	if _, err := exec.ExecContext(
		ctx,
		`UPDATE auth_subjects
		    SET email = $2,
		        status = $3,
		        auth_version = $4,
		        updated_at = NOW()
		  WHERE subject_id = $1`,
		subject.SubjectID, user.Email, user.Status, currentAuthVersion,
	); err != nil {
		return nil, fmt.Errorf("update auth_subject: %w", err)
	}

	if _, err := exec.ExecContext(
		ctx,
		`UPDATE users
		    SET subject_id = $2
		  WHERE id = $1
		    AND (subject_id IS NULL OR subject_id <> $2)`,
		user.ID, subject.SubjectID,
	); err != nil {
		return nil, fmt.Errorf("update users.subject_id: %w", err)
	}

	if err := r.upsertProfile(ctx, exec, &service.SubjectProfileRecord{
		SubjectID:    subject.SubjectID,
		LegacyUserID: user.ID,
		Email:        user.Email,
		Username:     user.Username,
		Notes:        user.Notes,
	}); err != nil {
		return nil, err
	}

	if err := r.replaceRoles(ctx, exec, subject.SubjectID, []string{user.Role}); err != nil {
		return nil, err
	}

	subject.Email = user.Email
	subject.Status = user.Status
	subject.AuthVersion = currentAuthVersion

	profile, err := r.getProfile(ctx, exec, subject.SubjectID)
	if err != nil {
		return nil, err
	}
	totp, err := r.getTOTPFactor(ctx, exec, subject.SubjectID)
	if err != nil && !errors.Is(err, service.ErrSubjectNotFound) {
		return nil, err
	}
	if errors.Is(err, service.ErrSubjectNotFound) {
		totp = &service.TOTPFactorRecord{
			SubjectID: subject.SubjectID,
			Enabled:   false,
		}
	}

	if changedAuthVersion {
		subject.UpdatedAt = time.Now()
	}

	return &service.IdentityBundle{
		Subject: subject,
		Profile: profile,
		Roles:   []string{user.Role},
		TOTP:    totp,
	}, nil
}

func (r *controlAuthRepository) GetIdentityBundleBySubjectID(ctx context.Context, subjectID string) (*service.IdentityBundle, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	subject, err := r.getSubjectByID(ctx, exec, subjectID)
	if err != nil {
		return nil, err
	}
	return r.loadIdentityBundle(ctx, exec, subject)
}

func (r *controlAuthRepository) GetIdentityBundleByLegacyUserID(ctx context.Context, userID int64) (*service.IdentityBundle, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	subject, err := r.getSubjectByLegacyUserID(ctx, exec, userID)
	if err != nil {
		return nil, err
	}
	return r.loadIdentityBundle(ctx, exec, subject)
}

func (r *controlAuthRepository) GetIdentityBundleByFederatedIdentity(ctx context.Context, provider, issuer, externalSubject string) (*service.IdentityBundle, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}

	var subjectID string
	row := queryRowContext(
		ctx,
		exec,
		`SELECT subject_id
		   FROM auth_federated_identities
		  WHERE provider = $1 AND issuer = $2 AND external_subject = $3`,
		provider, issuer, externalSubject,
	)
	if err := row.Scan(&subjectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrFederatedIdentityNotFound
		}
		return nil, fmt.Errorf("query federated identity: %w", err)
	}

	subject, err := r.getSubjectByID(ctx, exec, subjectID)
	if err != nil {
		return nil, err
	}
	return r.loadIdentityBundle(ctx, exec, subject)
}

func (r *controlAuthRepository) LinkFederatedIdentity(ctx context.Context, identity *service.FederatedIdentityRecord) error {
	if identity == nil {
		return fmt.Errorf("nil federated identity")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_federated_identities
		    (subject_id, provider, issuer, external_subject, email, username, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		 ON CONFLICT (provider, issuer, external_subject)
		 DO UPDATE SET
		    subject_id = EXCLUDED.subject_id,
		    email = EXCLUDED.email,
		    username = EXCLUDED.username,
		    updated_at = NOW()`,
		identity.SubjectID,
		identity.Provider,
		identity.Issuer,
		identity.ExternalSubject,
		identity.Email,
		identity.Username,
	)
	if err != nil {
		return fmt.Errorf("upsert federated identity: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) CreateSession(ctx context.Context, session *service.SessionRecord, refreshToken *service.RefreshTokenRecord) error {
	if session == nil || refreshToken == nil {
		return fmt.Errorf("session and refresh token are required")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_sessions
		    (sid, subject_id, legacy_user_id, status, amr, last_seen_at, expires_at, absolute_expires_at, revoked_at, current_refresh_token_hash, auth_version, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())`,
		session.SessionID,
		session.SubjectID,
		session.LegacyUserID,
		session.Status,
		session.AMR,
		session.LastSeenAt,
		session.ExpiresAt,
		session.AbsoluteExpiresAt,
		session.RevokedAt,
		session.CurrentRefreshTokenHash,
		session.AuthVersion,
	)
	if err != nil {
		return fmt.Errorf("insert auth_session: %w", err)
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_refresh_tokens
		    (token_hash, sid, subject_id, legacy_user_id, created_at, idle_expires_at, absolute_expires_at, rotated_at, revoked_at, replaced_by_token_hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		refreshToken.TokenHash,
		refreshToken.SessionID,
		refreshToken.SubjectID,
		refreshToken.LegacyUserID,
		refreshToken.CreatedAt,
		refreshToken.IdleExpiresAt,
		refreshToken.AbsoluteExpiresAt,
		refreshToken.RotatedAt,
		refreshToken.RevokedAt,
		refreshToken.ReplacedByTokenHash,
	)
	if err != nil {
		return fmt.Errorf("insert auth_refresh_token: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) GetSession(ctx context.Context, sessionID string) (*service.SessionRecord, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	return r.getSession(ctx, exec, sessionID)
}

func (r *controlAuthRepository) GetSessionByRefreshTokenHash(ctx context.Context, tokenHash string) (*service.SessionRecord, *service.RefreshTokenRecord, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, nil, err
	}
	row := queryRowContext(
		ctx,
		exec,
		`SELECT
		    s.sid,
		    s.subject_id,
		    s.legacy_user_id,
		    s.status,
		    s.amr,
		    s.last_seen_at,
		    s.expires_at,
		    s.absolute_expires_at,
		    s.revoked_at,
		    s.current_refresh_token_hash,
		    s.auth_version,
		    s.created_at,
		    s.updated_at,
		    rt.token_hash,
		    rt.created_at,
		    rt.idle_expires_at,
		    rt.absolute_expires_at,
		    rt.rotated_at,
		    rt.revoked_at,
		    rt.replaced_by_token_hash
		   FROM auth_refresh_tokens rt
		   JOIN auth_sessions s ON s.sid = rt.sid
		  WHERE rt.token_hash = $1`,
		tokenHash,
	)

	session, refresh, err := scanSessionAndRefresh(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, service.ErrSessionNotFound
		}
		return nil, nil, err
	}
	return session, refresh, nil
}

func (r *controlAuthRepository) RotateSessionRefreshToken(ctx context.Context, session *service.SessionRecord, currentTokenHash string, nextToken *service.RefreshTokenRecord, rotatedAt time.Time) error {
	if session == nil || nextToken == nil {
		return fmt.Errorf("session and next refresh token are required")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_refresh_tokens
		    SET rotated_at = $2,
		        replaced_by_token_hash = $3
		  WHERE token_hash = $1`,
		currentTokenHash,
		rotatedAt,
		nextToken.TokenHash,
	)
	if err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_refresh_tokens
		    (token_hash, sid, subject_id, legacy_user_id, created_at, idle_expires_at, absolute_expires_at, rotated_at, revoked_at, replaced_by_token_hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NULL, NULL, NULL)`,
		nextToken.TokenHash,
		nextToken.SessionID,
		nextToken.SubjectID,
		nextToken.LegacyUserID,
		nextToken.CreatedAt,
		nextToken.IdleExpiresAt,
		nextToken.AbsoluteExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert rotated refresh token: %w", err)
	}
	session.CurrentRefreshTokenHash = nextToken.TokenHash
	if err := r.UpdateSession(ctx, session); err != nil {
		return err
	}
	return nil
}

func (r *controlAuthRepository) UpdateSession(ctx context.Context, session *service.SessionRecord) error {
	if session == nil {
		return fmt.Errorf("nil session")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_sessions
		    SET status = $2,
		        amr = $3,
		        last_seen_at = $4,
		        expires_at = $5,
		        absolute_expires_at = $6,
		        revoked_at = $7,
		        current_refresh_token_hash = $8,
		        auth_version = $9,
		        updated_at = NOW()
		  WHERE sid = $1`,
		session.SessionID,
		session.Status,
		session.AMR,
		session.LastSeenAt,
		session.ExpiresAt,
		session.AbsoluteExpiresAt,
		session.RevokedAt,
		session.CurrentRefreshTokenHash,
		session.AuthVersion,
	)
	if err != nil {
		return fmt.Errorf("update auth_session: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_sessions
		    SET status = 'revoked',
		        revoked_at = $2,
		        updated_at = NOW()
		  WHERE sid = $1`,
		sessionID, revokedAt,
	)
	if err != nil {
		return fmt.Errorf("revoke auth_session: %w", err)
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_refresh_tokens
		    SET revoked_at = $2
		  WHERE sid = $1 AND revoked_at IS NULL`,
		sessionID, revokedAt,
	)
	if err != nil {
		return fmt.Errorf("revoke auth_refresh_tokens: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) RevokeAllSessions(ctx context.Context, subjectID string, revokedAt time.Time) error {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_sessions
		    SET status = 'revoked',
		        revoked_at = $2,
		        updated_at = NOW()
		  WHERE subject_id = $1 AND revoked_at IS NULL`,
		subjectID, revokedAt,
	)
	if err != nil {
		return fmt.Errorf("revoke subject sessions: %w", err)
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_refresh_tokens
		    SET revoked_at = $2
		  WHERE subject_id = $1 AND revoked_at IS NULL`,
		subjectID, revokedAt,
	)
	if err != nil {
		return fmt.Errorf("revoke subject refresh tokens: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) UpdateSubjectAuthVersion(ctx context.Context, subjectID string, authVersion int64) error {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_subjects
		    SET auth_version = $2,
		        updated_at = NOW()
		  WHERE subject_id = $1`,
		subjectID, authVersion,
	)
	if err != nil {
		return fmt.Errorf("update auth_version: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) GetTOTPFactor(ctx context.Context, subjectID string) (*service.TOTPFactorRecord, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	return r.getTOTPFactor(ctx, exec, subjectID)
}

func (r *controlAuthRepository) UpsertTOTPFactor(ctx context.Context, factor *service.TOTPFactorRecord) error {
	if factor == nil {
		return fmt.Errorf("nil totp factor")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_mfa_totp_factors
		    (subject_id, secret_encrypted, enabled, enabled_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())
		 ON CONFLICT (subject_id)
		 DO UPDATE SET
		    secret_encrypted = EXCLUDED.secret_encrypted,
		    enabled = EXCLUDED.enabled,
		    enabled_at = EXCLUDED.enabled_at,
		    updated_at = NOW()`,
		factor.SubjectID, factor.SecretEncrypted, factor.Enabled, factor.EnabledAt,
	)
	if err != nil {
		return fmt.Errorf("upsert totp factor: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) CreateEmailVerification(ctx context.Context, record *service.EmailVerificationRecord) error {
	if record == nil {
		return fmt.Errorf("nil email verification")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_email_verifications
		    (verification_id, subject_id, purpose, email, code_hash, expires_at, consumed_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`,
		record.VerificationID, record.SubjectID, record.Purpose, record.Email, record.CodeHash, record.ExpiresAt, record.ConsumedAt,
	)
	if err != nil {
		return fmt.Errorf("insert email verification: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) ConsumeEmailVerification(ctx context.Context, purpose, email, codeHash string, now time.Time, subjectID *string) (*service.EmailVerificationRecord, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	row := queryRowContext(
		ctx,
		exec,
		`SELECT verification_id, subject_id, purpose, email, code_hash, expires_at, consumed_at, created_at, updated_at
		   FROM auth_email_verifications
		  WHERE purpose = $1
		    AND email = $2
		    AND code_hash = $3
		    AND consumed_at IS NULL
		  ORDER BY created_at DESC
		  LIMIT 1`,
		purpose, email, codeHash,
	)
	record, err := scanEmailVerification(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrEmailVerificationNotFound
		}
		return nil, err
	}
	if now.After(record.ExpiresAt) {
		return nil, service.ErrEmailVerificationNotFound
	}
	if subjectID != nil && record.SubjectID != nil && *record.SubjectID != *subjectID {
		return nil, service.ErrEmailVerificationNotFound
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_email_verifications
		    SET consumed_at = $2,
		        updated_at = NOW()
		  WHERE verification_id = $1`,
		record.VerificationID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("consume email verification: %w", err)
	}
	record.ConsumedAt = &now
	return record, nil
}

func (r *controlAuthRepository) CreatePasswordResetToken(ctx context.Context, record *service.PasswordResetTokenRecord) error {
	if record == nil {
		return fmt.Errorf("nil password reset token")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_password_reset_tokens
		    (reset_id, subject_id, email, token_hash, expires_at, consumed_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		record.ResetID, record.SubjectID, record.Email, record.TokenHash, record.ExpiresAt, record.ConsumedAt,
	)
	if err != nil {
		return fmt.Errorf("insert password reset token: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) ConsumePasswordResetToken(ctx context.Context, email, tokenHash string, now time.Time) (*service.PasswordResetTokenRecord, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	row := queryRowContext(
		ctx,
		exec,
		`SELECT reset_id, subject_id, email, token_hash, expires_at, consumed_at, created_at, updated_at
		   FROM auth_password_reset_tokens
		  WHERE email = $1
		    AND token_hash = $2
		    AND consumed_at IS NULL
		  ORDER BY created_at DESC
		  LIMIT 1`,
		email, tokenHash,
	)
	record, err := scanPasswordResetToken(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrPasswordResetTokenNotFound
		}
		return nil, err
	}
	if now.After(record.ExpiresAt) {
		return nil, service.ErrPasswordResetTokenNotFound
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_password_reset_tokens
		    SET consumed_at = $2,
		        updated_at = NOW()
		  WHERE reset_id = $1`,
		record.ResetID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("consume password reset token: %w", err)
	}
	record.ConsumedAt = &now
	return record, nil
}

func (r *controlAuthRepository) CreateAuthFlow(ctx context.Context, flow *service.AuthFlowRecord) error {
	if flow == nil {
		return fmt.Errorf("nil auth flow")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_flows
		    (flow_id, provider, purpose, issuer, state_hash, code_verifier, nonce, redirect_to, expires_at, consumed_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`,
		flow.FlowID, flow.Provider, flow.Purpose, flow.Issuer, flow.StateHash, flow.CodeVerifier, flow.Nonce, flow.RedirectTo, flow.ExpiresAt, flow.ConsumedAt,
	)
	if err != nil {
		return fmt.Errorf("insert auth flow: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) GetAuthFlow(ctx context.Context, flowID string) (*service.AuthFlowRecord, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	row := queryRowContext(
		ctx,
		exec,
		`SELECT flow_id, provider, purpose, issuer, state_hash, code_verifier, nonce, redirect_to, expires_at, consumed_at, created_at, updated_at
		   FROM auth_flows
		  WHERE flow_id = $1`,
		flowID,
	)
	record, err := scanAuthFlow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrAuthFlowNotFound
		}
		return nil, err
	}
	return record, nil
}

func (r *controlAuthRepository) ConsumeAuthFlow(ctx context.Context, flowID, stateHash string, now time.Time) (*service.AuthFlowRecord, error) {
	record, err := r.GetAuthFlow(ctx, flowID)
	if err != nil {
		return nil, err
	}
	if record.StateHash != stateHash || record.ConsumedAt != nil || now.After(record.ExpiresAt) {
		return nil, service.ErrAuthFlowNotFound
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_flows
		    SET consumed_at = $2,
		        updated_at = NOW()
		  WHERE flow_id = $1`,
		flowID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("consume auth flow: %w", err)
	}
	record.ConsumedAt = &now
	return record, nil
}

func (r *controlAuthRepository) CreateRegistrationChallenge(ctx context.Context, challenge *service.RegistrationChallengeRecord) error {
	if challenge == nil {
		return fmt.Errorf("nil registration challenge")
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(
		ctx,
		`INSERT INTO auth_registration_challenges
		    (challenge_id, provider, issuer, external_subject, email, registration_email, username, redirect_to, expires_at, consumed_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`,
		challenge.ChallengeID,
		challenge.Provider,
		challenge.Issuer,
		challenge.ExternalSubject,
		challenge.Email,
		challenge.RegistrationEmail,
		challenge.Username,
		challenge.RedirectTo,
		challenge.ExpiresAt,
		challenge.ConsumedAt,
	)
	if err != nil {
		return fmt.Errorf("insert registration challenge: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) GetRegistrationChallenge(ctx context.Context, challengeID string) (*service.RegistrationChallengeRecord, error) {
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	row := queryRowContext(
		ctx,
		exec,
		`SELECT challenge_id, provider, issuer, external_subject, email, registration_email, username, redirect_to, expires_at, consumed_at, created_at, updated_at
		   FROM auth_registration_challenges
		  WHERE challenge_id = $1`,
		challengeID,
	)
	record, err := scanRegistrationChallenge(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrRegistrationChallengeNotFound
		}
		return nil, err
	}
	return record, nil
}

func (r *controlAuthRepository) ConsumeRegistrationChallenge(ctx context.Context, challengeID string, now time.Time) (*service.RegistrationChallengeRecord, error) {
	record, err := r.GetRegistrationChallenge(ctx, challengeID)
	if err != nil {
		return nil, err
	}
	if record.ConsumedAt != nil || now.After(record.ExpiresAt) {
		return nil, service.ErrRegistrationChallengeNotFound
	}
	exec, err := sqlExecutorFromContext(ctx, r.sql)
	if err != nil {
		return nil, err
	}
	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_registration_challenges
		    SET consumed_at = $2,
		        updated_at = NOW()
		  WHERE challenge_id = $1`,
		challengeID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("consume registration challenge: %w", err)
	}
	record.ConsumedAt = &now
	return record, nil
}

func (r *controlAuthRepository) loadIdentityBundle(ctx context.Context, exec sqlExecutor, subject *service.SubjectRecord) (*service.IdentityBundle, error) {
	profile, err := r.getProfile(ctx, exec, subject.SubjectID)
	if err != nil {
		return nil, err
	}
	roles, err := r.getRoles(ctx, exec, subject.SubjectID)
	if err != nil {
		return nil, err
	}
	totp, err := r.getTOTPFactor(ctx, exec, subject.SubjectID)
	if err != nil && !errors.Is(err, service.ErrSubjectNotFound) {
		return nil, err
	}
	if errors.Is(err, service.ErrSubjectNotFound) {
		totp = &service.TOTPFactorRecord{SubjectID: subject.SubjectID}
	}
	return &service.IdentityBundle{
		Subject: subject,
		Profile: profile,
		Roles:   roles,
		TOTP:    totp,
	}, nil
}

func (r *controlAuthRepository) getSubjectByID(ctx context.Context, exec sqlExecutor, subjectID string) (*service.SubjectRecord, error) {
	row := queryRowContext(
		ctx,
		exec,
		`SELECT subject_id, legacy_user_id, email, status, auth_version, created_at, updated_at
		   FROM auth_subjects
		  WHERE subject_id = $1`,
		subjectID,
	)
	return scanSubject(row)
}

func (r *controlAuthRepository) getSubjectByLegacyUserID(ctx context.Context, exec sqlExecutor, userID int64) (*service.SubjectRecord, error) {
	row := queryRowContext(
		ctx,
		exec,
		`SELECT subject_id, legacy_user_id, email, status, auth_version, created_at, updated_at
		   FROM auth_subjects
		  WHERE legacy_user_id = $1`,
		userID,
	)
	return scanSubject(row)
}

func (r *controlAuthRepository) syncPasswordCredential(ctx context.Context, exec sqlExecutor, subjectID, passwordHash string) (bool, error) {
	var existingHash string
	row := queryRowContext(
		ctx,
		exec,
		`SELECT password_hash
		   FROM auth_password_credentials
		  WHERE subject_id = $1`,
		subjectID,
	)
	switch err := row.Scan(&existingHash); {
	case err == nil:
		if existingHash == passwordHash {
			return false, nil
		}
		_, err = exec.ExecContext(
			ctx,
			`UPDATE auth_password_credentials
			    SET password_hash = $2,
			        changed_at = NOW(),
			        updated_at = NOW()
			  WHERE subject_id = $1`,
			subjectID, passwordHash,
		)
		if err != nil {
			return false, fmt.Errorf("update auth_password_credentials: %w", err)
		}
		return true, nil
	case errors.Is(err, sql.ErrNoRows):
		_, err = exec.ExecContext(
			ctx,
			`INSERT INTO auth_password_credentials
			    (subject_id, password_hash, created_at, updated_at, changed_at)
			 VALUES ($1, $2, NOW(), NOW(), NOW())`,
			subjectID, passwordHash,
		)
		if err != nil {
			return false, fmt.Errorf("insert auth_password_credentials: %w", err)
		}
		return false, nil
	default:
		return false, fmt.Errorf("query auth_password_credentials: %w", err)
	}
}

func (r *controlAuthRepository) syncTOTPFactor(ctx context.Context, exec sqlExecutor, subjectID string, user *service.User) (bool, error) {
	current, err := r.getTOTPFactor(ctx, exec, subjectID)
	if err != nil && !errors.Is(err, service.ErrSubjectNotFound) {
		return false, err
	}
	if errors.Is(err, service.ErrSubjectNotFound) {
		_, err = exec.ExecContext(
			ctx,
			`INSERT INTO auth_mfa_totp_factors
			    (subject_id, secret_encrypted, enabled, enabled_at, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
			subjectID, user.TotpSecretEncrypted, user.TotpEnabled, user.TotpEnabledAt,
		)
		if err != nil {
			return false, fmt.Errorf("insert auth_mfa_totp_factors: %w", err)
		}
		return false, nil
	}

	if nullableStringEquals(current.SecretEncrypted, user.TotpSecretEncrypted) &&
		current.Enabled == user.TotpEnabled &&
		nullableTimeEqual(current.EnabledAt, user.TotpEnabledAt) {
		return false, nil
	}

	_, err = exec.ExecContext(
		ctx,
		`UPDATE auth_mfa_totp_factors
		    SET secret_encrypted = $2,
		        enabled = $3,
		        enabled_at = $4,
		        updated_at = NOW()
		  WHERE subject_id = $1`,
		subjectID, user.TotpSecretEncrypted, user.TotpEnabled, user.TotpEnabledAt,
	)
	if err != nil {
		return false, fmt.Errorf("update auth_mfa_totp_factors: %w", err)
	}
	return true, nil
}

func (r *controlAuthRepository) upsertProfile(ctx context.Context, exec sqlExecutor, profile *service.SubjectProfileRecord) error {
	_, err := exec.ExecContext(
		ctx,
		`INSERT INTO control_user_profiles
		    (subject_id, legacy_user_id, email, username, notes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (subject_id)
		 DO UPDATE SET
		    legacy_user_id = EXCLUDED.legacy_user_id,
		    email = EXCLUDED.email,
		    username = EXCLUDED.username,
		    notes = EXCLUDED.notes,
		    updated_at = NOW()`,
		profile.SubjectID, profile.LegacyUserID, profile.Email, profile.Username, profile.Notes,
	)
	if err != nil {
		return fmt.Errorf("upsert control_user_profiles: %w", err)
	}
	return nil
}

func (r *controlAuthRepository) getProfile(ctx context.Context, exec sqlExecutor, subjectID string) (*service.SubjectProfileRecord, error) {
	row := queryRowContext(
		ctx,
		exec,
		`SELECT subject_id, legacy_user_id, email, username, notes, created_at, updated_at
		   FROM control_user_profiles
		  WHERE subject_id = $1`,
		subjectID,
	)
	var profile service.SubjectProfileRecord
	if err := row.Scan(
		&profile.SubjectID,
		&profile.LegacyUserID,
		&profile.Email,
		&profile.Username,
		&profile.Notes,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrSubjectNotFound
		}
		return nil, fmt.Errorf("scan control_user_profile: %w", err)
	}
	return &profile, nil
}

func (r *controlAuthRepository) replaceRoles(ctx context.Context, exec sqlExecutor, subjectID string, roles []string) error {
	if _, err := exec.ExecContext(ctx, `DELETE FROM control_subject_roles WHERE subject_id = $1`, subjectID); err != nil {
		return fmt.Errorf("delete control_subject_roles: %w", err)
	}
	for _, role := range roles {
		if role == "" {
			continue
		}
		if _, err := exec.ExecContext(
			ctx,
			`INSERT INTO control_subject_roles (subject_id, role, created_at, updated_at)
			 VALUES ($1, $2, NOW(), NOW())`,
			subjectID, role,
		); err != nil {
			return fmt.Errorf("insert control_subject_role: %w", err)
		}
	}
	return nil
}

func (r *controlAuthRepository) getRoles(ctx context.Context, exec sqlExecutor, subjectID string) ([]string, error) {
	rows, err := exec.QueryContext(
		ctx,
		`SELECT role
		   FROM control_subject_roles
		  WHERE subject_id = $1
		  ORDER BY role`,
		subjectID,
	)
	if err != nil {
		return nil, fmt.Errorf("query control_subject_roles: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan control_subject_role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

func (r *controlAuthRepository) getTOTPFactor(ctx context.Context, exec sqlExecutor, subjectID string) (*service.TOTPFactorRecord, error) {
	row := queryRowContext(
		ctx,
		exec,
		`SELECT subject_id, secret_encrypted, enabled, enabled_at, created_at, updated_at
		   FROM auth_mfa_totp_factors
		  WHERE subject_id = $1`,
		subjectID,
	)
	var factor service.TOTPFactorRecord
	var secret sql.NullString
	var enabledAt sql.NullTime
	if err := row.Scan(
		&factor.SubjectID,
		&secret,
		&factor.Enabled,
		&enabledAt,
		&factor.CreatedAt,
		&factor.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrSubjectNotFound
		}
		return nil, fmt.Errorf("scan auth_mfa_totp_factor: %w", err)
	}
	if secret.Valid {
		value := secret.String
		factor.SecretEncrypted = &value
	}
	if enabledAt.Valid {
		value := enabledAt.Time
		factor.EnabledAt = &value
	}
	return &factor, nil
}

func (r *controlAuthRepository) getSession(ctx context.Context, exec sqlExecutor, sessionID string) (*service.SessionRecord, error) {
	row := queryRowContext(
		ctx,
		exec,
		`SELECT sid, subject_id, legacy_user_id, status, amr, last_seen_at, expires_at, absolute_expires_at, revoked_at, current_refresh_token_hash, auth_version, created_at, updated_at
		   FROM auth_sessions
		  WHERE sid = $1`,
		sessionID,
	)
	return scanSession(row)
}

func scanSubject(row rowScanner) (*service.SubjectRecord, error) {
	var subject service.SubjectRecord
	if err := row.Scan(
		&subject.SubjectID,
		&subject.LegacyUserID,
		&subject.Email,
		&subject.Status,
		&subject.AuthVersion,
		&subject.CreatedAt,
		&subject.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrSubjectNotFound
		}
		return nil, fmt.Errorf("scan auth_subject: %w", err)
	}
	return &subject, nil
}

func scanSession(row rowScanner) (*service.SessionRecord, error) {
	var session service.SessionRecord
	var revokedAt sql.NullTime
	if err := row.Scan(
		&session.SessionID,
		&session.SubjectID,
		&session.LegacyUserID,
		&session.Status,
		&session.AMR,
		&session.LastSeenAt,
		&session.ExpiresAt,
		&session.AbsoluteExpiresAt,
		&revokedAt,
		&session.CurrentRefreshTokenHash,
		&session.AuthVersion,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrSessionNotFound
		}
		return nil, fmt.Errorf("scan auth_session: %w", err)
	}
	if revokedAt.Valid {
		value := revokedAt.Time
		session.RevokedAt = &value
	}
	return &session, nil
}

func scanSessionAndRefresh(row rowScanner) (*service.SessionRecord, *service.RefreshTokenRecord, error) {
	session := &service.SessionRecord{}
	refresh := &service.RefreshTokenRecord{}
	var sessionRevokedAt sql.NullTime
	var refreshRotatedAt sql.NullTime
	var refreshRevokedAt sql.NullTime
	var replacedBy sql.NullString
	if err := row.Scan(
		&session.SessionID,
		&session.SubjectID,
		&session.LegacyUserID,
		&session.Status,
		&session.AMR,
		&session.LastSeenAt,
		&session.ExpiresAt,
		&session.AbsoluteExpiresAt,
		&sessionRevokedAt,
		&session.CurrentRefreshTokenHash,
		&session.AuthVersion,
		&session.CreatedAt,
		&session.UpdatedAt,
		&refresh.TokenHash,
		&refresh.CreatedAt,
		&refresh.IdleExpiresAt,
		&refresh.AbsoluteExpiresAt,
		&refreshRotatedAt,
		&refreshRevokedAt,
		&replacedBy,
	); err != nil {
		return nil, nil, err
	}
	refresh.SessionID = session.SessionID
	refresh.SubjectID = session.SubjectID
	refresh.LegacyUserID = session.LegacyUserID
	if sessionRevokedAt.Valid {
		value := sessionRevokedAt.Time
		session.RevokedAt = &value
	}
	if refreshRotatedAt.Valid {
		value := refreshRotatedAt.Time
		refresh.RotatedAt = &value
	}
	if refreshRevokedAt.Valid {
		value := refreshRevokedAt.Time
		refresh.RevokedAt = &value
	}
	if replacedBy.Valid {
		value := replacedBy.String
		refresh.ReplacedByTokenHash = &value
	}
	return session, refresh, nil
}

func scanEmailVerification(row rowScanner) (*service.EmailVerificationRecord, error) {
	var record service.EmailVerificationRecord
	var subjectID sql.NullString
	var consumedAt sql.NullTime
	if err := row.Scan(
		&record.VerificationID,
		&subjectID,
		&record.Purpose,
		&record.Email,
		&record.CodeHash,
		&record.ExpiresAt,
		&consumedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if subjectID.Valid {
		value := subjectID.String
		record.SubjectID = &value
	}
	if consumedAt.Valid {
		value := consumedAt.Time
		record.ConsumedAt = &value
	}
	return &record, nil
}

func scanPasswordResetToken(row rowScanner) (*service.PasswordResetTokenRecord, error) {
	var record service.PasswordResetTokenRecord
	var consumedAt sql.NullTime
	if err := row.Scan(
		&record.ResetID,
		&record.SubjectID,
		&record.Email,
		&record.TokenHash,
		&record.ExpiresAt,
		&consumedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if consumedAt.Valid {
		value := consumedAt.Time
		record.ConsumedAt = &value
	}
	return &record, nil
}

func scanAuthFlow(row rowScanner) (*service.AuthFlowRecord, error) {
	var record service.AuthFlowRecord
	var codeVerifier sql.NullString
	var nonce sql.NullString
	var consumedAt sql.NullTime
	if err := row.Scan(
		&record.FlowID,
		&record.Provider,
		&record.Purpose,
		&record.Issuer,
		&record.StateHash,
		&codeVerifier,
		&nonce,
		&record.RedirectTo,
		&record.ExpiresAt,
		&consumedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if codeVerifier.Valid {
		value := codeVerifier.String
		record.CodeVerifier = &value
	}
	if nonce.Valid {
		value := nonce.String
		record.Nonce = &value
	}
	if consumedAt.Valid {
		value := consumedAt.Time
		record.ConsumedAt = &value
	}
	return &record, nil
}

func scanRegistrationChallenge(row rowScanner) (*service.RegistrationChallengeRecord, error) {
	var record service.RegistrationChallengeRecord
	var consumedAt sql.NullTime
	if err := row.Scan(
		&record.ChallengeID,
		&record.Provider,
		&record.Issuer,
		&record.ExternalSubject,
		&record.Email,
		&record.RegistrationEmail,
		&record.Username,
		&record.RedirectTo,
		&record.ExpiresAt,
		&consumedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if consumedAt.Valid {
		value := consumedAt.Time
		record.ConsumedAt = &value
	}
	return &record, nil
}

func nullableStringEquals(a, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func nullableTimeEqual(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Equal(*b)
	}
}
