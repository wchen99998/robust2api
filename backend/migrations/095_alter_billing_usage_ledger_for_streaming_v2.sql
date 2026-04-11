ALTER TABLE billing_usage_ledger
    ADD COLUMN IF NOT EXISTS kind VARCHAR(32);

UPDATE billing_usage_ledger
SET kind = 'charge'
WHERE kind IS NULL OR kind = '';

ALTER TABLE billing_usage_ledger
    ALTER COLUMN kind SET NOT NULL;

DROP INDEX IF EXISTS billing_usage_ledger_request_api_key_uidx;

CREATE UNIQUE INDEX IF NOT EXISTS billing_usage_ledger_request_api_key_kind_uidx
    ON billing_usage_ledger (request_id, api_key_id, kind);
