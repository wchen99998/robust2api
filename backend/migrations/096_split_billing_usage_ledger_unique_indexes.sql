DROP INDEX IF EXISTS billing_usage_ledger_request_api_key_kind_uidx;

CREATE UNIQUE INDEX IF NOT EXISTS billing_usage_ledger_request_api_key_kind_uidx
    ON billing_usage_ledger (request_id, api_key_id, kind)
    WHERE kind IN ('charge', 'finalize');

CREATE UNIQUE INDEX IF NOT EXISTS billing_usage_ledger_request_api_key_kind_account_uidx
    ON billing_usage_ledger (request_id, api_key_id, kind, account_id)
    WHERE kind IN ('reserve', 'release');
