package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

type usageBillingRepository struct {
	db *sql.DB
}

func NewUsageBillingRepository(billingDB *BillingDB) service.UsageBillingRepository {
	if billingDB == nil || billingDB.DB == nil {
		return &usageBillingRepository{db: nil}
	}
	return &usageBillingRepository{db: billingDB.DB}
}

func (r *usageBillingRepository) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (_ *service.UsageBillingApplyResult, err error) {
	if cmd == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	return r.ApplyUsageCharge(ctx, &service.UsageChargeEvent{Kind: service.UsageChargeEventKindCharge, Command: cmd})
}

func (r *usageBillingRepository) ApplyUsageCharge(ctx context.Context, event *service.UsageChargeEvent) (_ *service.UsageBillingApplyResult, err error) {
	if event == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	kind := normalizeUsageChargeEvent(event)
	cmd := event.Command
	if event.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}
	if kind != service.UsageChargeEventKindReserve && kind != service.UsageChargeEventKindRelease {
		if cmd == nil {
			return &service.UsageBillingApplyResult{}, nil
		}
		if cmd.RequestID == "" {
			return nil, service.ErrUsageBillingRequestIDRequired
		}
	}
	if cmd == nil {
		return nil, service.ErrUsageBillingRequestIDRequired
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	result := &service.UsageBillingApplyResult{}
	if kind == service.UsageChargeEventKindCharge || kind == service.UsageChargeEventKindFinalize {
		applied, err := r.claimUsageBillingKey(ctx, tx, cmd)
		if err != nil {
			return nil, err
		}
		result.Applied = applied
	}
	if err := r.persistBillingLedger(ctx, tx, event); err != nil {
		return nil, err
	}

	if result.Applied {
		if err := r.applyUsageBillingEffects(ctx, tx, cmd, result); err != nil {
			return nil, err
		}
	}
	if (kind == service.UsageChargeEventKindCharge || kind == service.UsageChargeEventKindFinalize) && event.UsageLog != nil {
		tombstoned, err := r.usageLogTombstoned(ctx, tx, event.UsageLog.RequestID, event.UsageLog.APIKeyID)
		if err != nil {
			return nil, err
		}
		if !tombstoned {
			inserted, err := r.persistUsageLog(ctx, tx, event.UsageLog)
			if err != nil {
				return nil, err
			}
			result.UsageLogInserted = inserted
		}
	}
	if cmd.APIKeyQuotaCost > 0 {
		apiKeyKey, shouldInvalidate, err := r.resolveAPIKeyAuthProjection(ctx, tx, cmd.APIKeyID)
		if err != nil {
			return nil, err
		}
		result.APIKeyAuthCacheKey = apiKeyKey
		result.NeedsAPIKeyAuthCacheInvalidation = shouldInvalidate
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func normalizeUsageChargeEvent(event *service.UsageChargeEvent) service.UsageChargeEventKind {
	if event == nil {
		return service.UsageChargeEventKindCharge
	}
	event.Kind = service.UsageChargeEventKind(strings.TrimSpace(string(event.Kind)))
	switch event.Kind {
	case service.UsageChargeEventKindReserve, service.UsageChargeEventKindFinalize, service.UsageChargeEventKindRelease:
	default:
		event.Kind = service.UsageChargeEventKindCharge
	}
	if event.Version <= 0 {
		event.Version = 2
	}
	requestID := strings.TrimSpace(event.RequestID)
	if event.Command != nil {
		event.Command.Normalize()
	}
	if requestID == "" && event.Command != nil {
		requestID = event.Command.RequestID
	}
	if event.UsageLog != nil {
		event.UsageLog.RequestID = strings.TrimSpace(event.UsageLog.RequestID)
		if event.UsageLog.RequestID == "" {
			event.UsageLog.RequestID = requestID
		}
		if requestID == "" {
			requestID = event.UsageLog.RequestID
		}
	}
	if event.Command != nil {
		event.Command.RequestID = requestID
	}
	event.RequestID = requestID
	return event.Kind
}

func billingUsageLedgerConflictClause(kind service.UsageChargeEventKind) string {
	switch kind {
	case service.UsageChargeEventKindReserve, service.UsageChargeEventKindRelease:
		return "ON CONFLICT (request_id, api_key_id, kind, account_id) WHERE kind IN ('reserve', 'release') DO NOTHING"
	default:
		return "ON CONFLICT (request_id, api_key_id, kind) WHERE kind IN ('charge', 'finalize') DO NOTHING"
	}
}

func (r *usageBillingRepository) claimUsageBillingKey(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) (bool, error) {
	if ok, err := r.matchExistingUsageBillingFingerprint(ctx, tx, "usage_billing_dedup", cmd); err != nil || ok {
		return false, err
	}
	if ok, err := r.matchExistingUsageBillingFingerprint(ctx, tx, "usage_billing_dedup_archive", cmd); err != nil || ok {
		return false, err
	}

	var id int64
	err := scanSingleRow(ctx, tx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		VALUES ($1, $2, $3)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id
	`, []any{cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint}, &id)
	if errors.Is(err, sql.ErrNoRows) {
		if ok, err := r.matchExistingUsageBillingFingerprint(ctx, tx, "usage_billing_dedup", cmd); err != nil || ok {
			return false, err
		}
		if ok, err := r.matchExistingUsageBillingFingerprint(ctx, tx, "usage_billing_dedup_archive", cmd); err != nil || ok {
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// usageBillingDedupTables is a compile-time allow-list of tables that may be
// queried by matchExistingUsageBillingFingerprint.  Restricting interpolated
// table names to this set prevents SQL-injection via the `table` parameter.
var usageBillingDedupTables = map[string]struct{}{
	"usage_billing_dedup":         {},
	"usage_billing_dedup_archive": {},
}

func (r *usageBillingRepository) matchExistingUsageBillingFingerprint(ctx context.Context, tx *sql.Tx, table string, cmd *service.UsageBillingCommand) (bool, error) {
	if _, ok := usageBillingDedupTables[table]; !ok {
		return false, fmt.Errorf("usage billing fingerprint check: disallowed table %q", table)
	}
	var existingFingerprint string
	err := scanSingleRow(ctx, tx, `
		SELECT request_fingerprint
		FROM `+table+`
		WHERE request_id = $1 AND api_key_id = $2
	`, []any{cmd.RequestID, cmd.APIKeyID}, &existingFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(existingFingerprint) != strings.TrimSpace(cmd.RequestFingerprint) {
		return false, service.ErrUsageBillingRequestConflict
	}
	return true, nil
}

func (r *usageBillingRepository) applyUsageBillingEffects(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand, result *service.UsageBillingApplyResult) error {
	if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil {
		if err := incrementUsageBillingSubscription(ctx, tx, *cmd.SubscriptionID, cmd.SubscriptionCost); err != nil {
			return err
		}
	}

	if cmd.BalanceCost > 0 {
		if err := deductUsageBillingBalance(ctx, tx, cmd.UserID, cmd.BalanceCost); err != nil {
			return err
		}
	}

	if cmd.APIKeyQuotaCost > 0 {
		exhausted, err := incrementUsageBillingAPIKeyQuota(ctx, tx, cmd.APIKeyID, cmd.APIKeyQuotaCost)
		if err != nil {
			return err
		}
		result.APIKeyQuotaExhausted = exhausted
	}

	if cmd.APIKeyRateLimitCost > 0 {
		if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, cmd.APIKeyID, cmd.APIKeyRateLimitCost); err != nil {
			return err
		}
	}

	if cmd.AccountQuotaCost > 0 && (strings.EqualFold(cmd.AccountType, service.AccountTypeAPIKey) || strings.EqualFold(cmd.AccountType, service.AccountTypeBedrock)) {
		if err := incrementUsageBillingAccountQuota(ctx, tx, cmd.AccountID, cmd.AccountQuotaCost); err != nil {
			return err
		}
	}

	return nil
}

func (r *usageBillingRepository) persistUsageLog(ctx context.Context, tx *sql.Tx, usageLog *service.UsageLog) (bool, error) {
	if usageLog == nil {
		return false, nil
	}
	repo := &usageLogRepository{sql: tx}
	return repo.createSingle(ctx, tx, usageLog)
}

func (r *usageBillingRepository) persistBillingLedger(ctx context.Context, tx *sql.Tx, event *service.UsageChargeEvent) error {
	if event == nil || event.Command == nil {
		return nil
	}
	cmd := event.Command
	rawEvent, err := json.Marshal(event)
	if err != nil {
		return err
	}
	sourceEventID := strings.TrimSpace(event.EventID)
	if sourceEventID == "" {
		sourceEventID = uuid.NewString()
	}
	occurredAt := event.OccurredAt.UTC()
	if occurredAt.IsZero() {
		if event.UsageLog != nil && !event.UsageLog.CreatedAt.IsZero() {
			occurredAt = event.UsageLog.CreatedAt.UTC()
		} else {
			occurredAt = time.Now().UTC()
		}
	}

	query := fmt.Sprintf(`
		INSERT INTO billing_usage_ledger (
			request_id,
			api_key_id,
			kind,
			request_fingerprint,
			source_event_id,
			user_id,
			account_id,
			subscription_id,
			group_id,
			billing_type,
			balance_cost,
			subscription_cost,
			api_key_quota_cost,
			api_key_rate_limit_cost,
			account_quota_cost,
			occurred_at,
			raw_event
		) VALUES (
			$1, $2, $3, $4, $5::uuid, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17::jsonb
		)
		%s
	`, billingUsageLedgerConflictClause(event.Kind))
	_, err = tx.ExecContext(ctx, query,
		cmd.RequestID,
		cmd.APIKeyID,
		strings.TrimSpace(string(event.Kind)),
		cmd.RequestFingerprint,
		sourceEventID,
		cmd.UserID,
		cmd.AccountID,
		nullableInt64Value(cmd.SubscriptionID),
		nullableGroupID(event.GroupID),
		cmd.BillingType,
		cmd.BalanceCost,
		cmd.SubscriptionCost,
		cmd.APIKeyQuotaCost,
		cmd.APIKeyRateLimitCost,
		cmd.AccountQuotaCost,
		occurredAt,
		string(rawEvent),
	)
	return err
}

func (r *usageBillingRepository) usageLogTombstoned(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID int64) (bool, error) {
	var exists bool
	err := scanSingleRow(ctx, tx, `
		SELECT TRUE
		FROM usage_log_tombstones
		WHERE request_id = $1 AND api_key_id = $2
	`, []any{requestID, apiKeyID}, &exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists, nil
}

func nullableInt64Value(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableGroupID(groupID int64) any {
	if groupID <= 0 {
		return nil
	}
	return groupID
}

func (r *usageBillingRepository) resolveAPIKeyAuthProjection(ctx context.Context, tx *sql.Tx, apiKeyID int64) (string, bool, error) {
	if apiKeyID <= 0 {
		return "", false, nil
	}

	var (
		key       string
		status    string
		quota     float64
		quotaUsed float64
	)
	err := tx.QueryRowContext(ctx, `
		SELECT key, status, quota, quota_used
		FROM api_keys
		WHERE id = $1 AND deleted_at IS NULL
	`, apiKeyID).Scan(&key, &status, &quota, &quotaUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, service.ErrAPIKeyNotFound
	}
	if err != nil {
		return "", false, err
	}

	quotaExhausted := quota > 0 && quotaUsed >= quota
	if strings.EqualFold(status, service.StatusAPIKeyQuotaExhausted) {
		quotaExhausted = true
	}
	return key, quotaExhausted, nil
}

func incrementUsageBillingSubscription(ctx context.Context, tx *sql.Tx, subscriptionID int64, costUSD float64) error {
	const updateSQL = `
		UPDATE user_subscriptions us
		SET
			daily_usage_usd = us.daily_usage_usd + $1,
			weekly_usage_usd = us.weekly_usage_usd + $1,
			monthly_usage_usd = us.monthly_usage_usd + $1,
			updated_at = NOW()
		FROM groups g
		WHERE us.id = $2
			AND us.deleted_at IS NULL
			AND us.group_id = g.id
			AND g.deleted_at IS NULL
	`
	res, err := tx.ExecContext(ctx, updateSQL, costUSD, subscriptionID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	return service.ErrSubscriptionNotFound
}

func deductUsageBillingBalance(ctx context.Context, tx *sql.Tx, userID int64, amount float64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, amount, userID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	return service.ErrUserNotFound
}

func incrementUsageBillingAPIKeyQuota(ctx context.Context, tx *sql.Tx, apiKeyID int64, amount float64) (bool, error) {
	var exhausted bool
	err := tx.QueryRowContext(ctx, `
		UPDATE api_keys
		SET quota_used = quota_used + $1,
			status = CASE
				WHEN quota > 0
					AND status = $3
					AND quota_used < quota
					AND quota_used + $1 >= quota
				THEN $4
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING quota > 0 AND quota_used >= quota AND quota_used - $1 < quota
	`, amount, apiKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).Scan(&exhausted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, service.ErrAPIKeyNotFound
	}
	if err != nil {
		return false, err
	}
	return exhausted, nil
}

func incrementUsageBillingAPIKeyRateLimit(ctx context.Context, tx *sql.Tx, apiKeyID int64, cost float64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL '5 hours' <= NOW() THEN $1 ELSE usage_5h + $1 END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL '24 hours' <= NOW() THEN $1 ELSE usage_1d + $1 END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL '7 days' <= NOW() THEN $1 ELSE usage_7d + $1 END,
			window_5h_start = CASE WHEN window_5h_start IS NULL OR window_5h_start + INTERVAL '5 hours' <= NOW() THEN NOW() ELSE window_5h_start END,
			window_1d_start = CASE WHEN window_1d_start IS NULL OR window_1d_start + INTERVAL '24 hours' <= NOW() THEN date_trunc('day', NOW()) ELSE window_1d_start END,
			window_7d_start = CASE WHEN window_7d_start IS NULL OR window_7d_start + INTERVAL '7 days' <= NOW() THEN date_trunc('day', NOW()) ELSE window_7d_start END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, cost, apiKeyID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAPIKeyNotFound
	}
	return nil
}

func incrementUsageBillingAccountQuota(ctx context.Context, tx *sql.Tx, accountID int64, amount float64) error {
	rows, err := tx.QueryContext(ctx,
		`UPDATE accounts SET extra = (
			COALESCE(extra, '{}'::jsonb)
			|| jsonb_build_object('quota_used', COALESCE((extra->>'quota_used')::numeric, 0) + $1)
			|| CASE WHEN COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_daily_used',
					CASE WHEN COALESCE((extra->>'quota_daily_start')::timestamptz, '1970-01-01'::timestamptz)
						+ '24 hours'::interval <= NOW()
					THEN $1
					ELSE COALESCE((extra->>'quota_daily_used')::numeric, 0) + $1 END,
					'quota_daily_start',
					CASE WHEN COALESCE((extra->>'quota_daily_start')::timestamptz, '1970-01-01'::timestamptz)
						+ '24 hours'::interval <= NOW()
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_daily_start', `+nowUTC+`) END
				)
			ELSE '{}'::jsonb END
			|| CASE WHEN COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_weekly_used',
					CASE WHEN COALESCE((extra->>'quota_weekly_start')::timestamptz, '1970-01-01'::timestamptz)
						+ '168 hours'::interval <= NOW()
					THEN $1
					ELSE COALESCE((extra->>'quota_weekly_used')::numeric, 0) + $1 END,
					'quota_weekly_start',
					CASE WHEN COALESCE((extra->>'quota_weekly_start')::timestamptz, '1970-01-01'::timestamptz)
						+ '168 hours'::interval <= NOW()
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_weekly_start', `+nowUTC+`) END
				)
			ELSE '{}'::jsonb END
		), updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING
			COALESCE((extra->>'quota_used')::numeric, 0),
			COALESCE((extra->>'quota_limit')::numeric, 0)`,
		amount, accountID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var newUsed, limit float64
	if rows.Next() {
		if err := rows.Scan(&newUsed, &limit); err != nil {
			return err
		}
	} else {
		if err := rows.Err(); err != nil {
			return err
		}
		return service.ErrAccountNotFound
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if limit > 0 && newUsed >= limit && (newUsed-amount) < limit {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
			logger.LegacyPrintf("repository.usage_billing", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", accountID, err)
			return err
		}
	}
	return nil
}
