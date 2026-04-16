CREATE TABLE IF NOT EXISTS billing_usage_ledger (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    kind VARCHAR(32) NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    source_event_id UUID NOT NULL,
    user_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    subscription_id BIGINT NULL,
    group_id BIGINT NULL,
    billing_type SMALLINT NOT NULL,
    balance_cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    subscription_cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    api_key_quota_cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    api_key_rate_limit_cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    account_quota_cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    raw_event JSONB NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS billing_usage_ledger_request_api_key_kind_uidx
    ON billing_usage_ledger (request_id, api_key_id, kind);

CREATE INDEX IF NOT EXISTS billing_usage_ledger_user_occurred_idx
    ON billing_usage_ledger (user_id, occurred_at);

CREATE INDEX IF NOT EXISTS billing_usage_ledger_source_event_idx
    ON billing_usage_ledger (source_event_id);

CREATE TABLE IF NOT EXISTS usage_log_tombstones (
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    usage_log_id BIGINT NULL,
    user_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    original_created_at TIMESTAMPTZ NULL,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delete_reason TEXT NOT NULL,
    source_task_id BIGINT NULL,
    PRIMARY KEY (request_id, api_key_id)
);
