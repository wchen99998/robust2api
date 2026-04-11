ALTER TABLE users
    ADD COLUMN IF NOT EXISTS subject_id UUID;

CREATE UNIQUE INDEX IF NOT EXISTS users_subject_id_unique_idx
    ON users (subject_id)
    WHERE subject_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS auth_subjects (
    subject_id UUID PRIMARY KEY,
    legacy_user_id BIGINT NOT NULL UNIQUE REFERENCES users(id),
    email TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    auth_version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS auth_subjects_email_idx
    ON auth_subjects (LOWER(email));

CREATE TABLE IF NOT EXISTS auth_password_credentials (
    subject_id UUID PRIMARY KEY REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS auth_federated_identities (
    id BIGSERIAL PRIMARY KEY,
    subject_id UUID NOT NULL REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    issuer TEXT NOT NULL,
    external_subject TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT auth_federated_identities_provider_identity_unique
        UNIQUE (provider, issuer, external_subject)
);

CREATE INDEX IF NOT EXISTS auth_federated_identities_subject_id_idx
    ON auth_federated_identities (subject_id);

CREATE TABLE IF NOT EXISTS auth_sessions (
    sid UUID PRIMARY KEY,
    subject_id UUID NOT NULL REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    legacy_user_id BIGINT NOT NULL REFERENCES users(id),
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    amr TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    current_refresh_token_hash TEXT NOT NULL,
    auth_version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS auth_sessions_subject_status_idx
    ON auth_sessions (subject_id, status);

CREATE INDEX IF NOT EXISTS auth_sessions_legacy_user_id_idx
    ON auth_sessions (legacy_user_id);

CREATE TABLE IF NOT EXISTS auth_refresh_tokens (
    token_hash TEXT PRIMARY KEY,
    sid UUID NOT NULL REFERENCES auth_sessions(sid) ON DELETE CASCADE,
    subject_id UUID NOT NULL REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    legacy_user_id BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    idle_expires_at TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    rotated_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    replaced_by_token_hash TEXT NULL
);

CREATE INDEX IF NOT EXISTS auth_refresh_tokens_sid_idx
    ON auth_refresh_tokens (sid);

CREATE INDEX IF NOT EXISTS auth_refresh_tokens_subject_id_idx
    ON auth_refresh_tokens (subject_id);

CREATE TABLE IF NOT EXISTS auth_mfa_totp_factors (
    subject_id UUID PRIMARY KEY REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    secret_encrypted TEXT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    enabled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS auth_email_verifications (
    verification_id UUID PRIMARY KEY,
    subject_id UUID NULL REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    purpose VARCHAR(64) NOT NULL,
    email TEXT NOT NULL,
    code_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS auth_email_verifications_lookup_idx
    ON auth_email_verifications (email, purpose, consumed_at);

CREATE TABLE IF NOT EXISTS auth_password_reset_tokens (
    reset_id UUID PRIMARY KEY,
    subject_id UUID NOT NULL REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS auth_password_reset_tokens_subject_idx
    ON auth_password_reset_tokens (subject_id, consumed_at);

CREATE TABLE IF NOT EXISTS auth_flows (
    flow_id UUID PRIMARY KEY,
    provider VARCHAR(64) NOT NULL,
    purpose VARCHAR(64) NOT NULL,
    issuer TEXT NOT NULL DEFAULT '',
    state_hash TEXT NOT NULL UNIQUE,
    code_verifier TEXT NULL,
    nonce TEXT NULL,
    redirect_to TEXT NOT NULL DEFAULT '/',
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS auth_registration_challenges (
    challenge_id UUID PRIMARY KEY,
    provider VARCHAR(64) NOT NULL,
    issuer TEXT NOT NULL DEFAULT '',
    external_subject TEXT NOT NULL,
    email TEXT NOT NULL,
    registration_email TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    redirect_to TEXT NOT NULL DEFAULT '/',
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS auth_registration_challenges_active_idx
    ON auth_registration_challenges (expires_at, consumed_at);

CREATE TABLE IF NOT EXISTS control_user_profiles (
    subject_id UUID PRIMARY KEY REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    legacy_user_id BIGINT NOT NULL UNIQUE REFERENCES users(id),
    email TEXT NOT NULL,
    username VARCHAR(100) NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS control_user_profiles_email_idx
    ON control_user_profiles (LOWER(email));

CREATE TABLE IF NOT EXISTS control_subject_roles (
    subject_id UUID NOT NULL REFERENCES auth_subjects(subject_id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subject_id, role)
);

CREATE INDEX IF NOT EXISTS control_subject_roles_role_idx
    ON control_subject_roles (role);
