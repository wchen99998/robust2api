DROP TABLE IF EXISTS ops_retry_attempts CASCADE;
DROP TABLE IF EXISTS ops_error_logs CASCADE;

ALTER TABLE error_passthrough_rules
DROP COLUMN IF EXISTS skip_monitoring;
