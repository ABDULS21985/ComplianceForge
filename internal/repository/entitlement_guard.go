package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrEntitlementLimitExceeded = errors.New("subscription entitlement limit exceeded")

type EntitlementLimitError struct {
	Metric     string
	Limit      int64
	Usage      int64
	Additional int64
}

func (e *EntitlementLimitError) Error() string {
	return fmt.Sprintf("%s: metric=%s limit=%d usage=%d requested=%d",
		ErrEntitlementLimitExceeded, e.Metric, e.Limit, e.Usage, e.Additional)
}

func (e *EntitlementLimitError) Unwrap() error { return ErrEntitlementLimitExceeded }

// EnsureEntitlementCapacity atomically serializes a metered create operation
// for one tenant and metric. Callers must invoke it inside the same transaction
// that inserts the metered record; this closes the race left by UI preflight
// checks while retaining zero as the documented unlimited value.
func EnsureEntitlementCapacity(ctx context.Context, tx pgx.Tx, organizationID, metric string, additional int64) error {
	if tx == nil {
		return errors.New("entitlement capacity check requires a transaction")
	}
	if additional < 1 {
		return errors.New("entitlement capacity increment must be positive")
	}
	switch metric {
	case "users", "frameworks", "risks", "vendors", "storage_bytes":
	default:
		return fmt.Errorf("unknown entitlement metric %q", metric)
	}
	if err := lockEntitlementCapacity(ctx, tx, organizationID, metric); err != nil {
		return err
	}
	return ensureEntitlementCapacityLocked(ctx, tx, organizationID, metric, additional)
}

func lockEntitlementCapacity(ctx context.Context, tx pgx.Tx, organizationID, metric string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || $2,0))`, organizationID, metric); err != nil {
		return fmt.Errorf("locking entitlement capacity: %w", err)
	}
	return nil
}

func ensureEntitlementCapacityLocked(ctx context.Context, tx pgx.Tx, organizationID, metric string, additional int64) error {
	limit, err := entitlementLimit(ctx, tx, organizationID, metric)
	if err != nil {
		return err
	}
	if limit == 0 {
		return nil
	}
	usage, err := entitlementUsage(ctx, tx, organizationID, metric)
	if err != nil {
		return err
	}
	if additional > limit-usage {
		return &EntitlementLimitError{Metric: metric, Limit: limit, Usage: usage, Additional: additional}
	}
	return nil
}

func entitlementLimit(ctx context.Context, tx pgx.Tx, organizationID, metric string) (int64, error) {
	var limit int64
	err := tx.QueryRow(ctx, `
		SELECT CASE $2
			WHEN 'users' THEN COALESCE(plan.max_users,legacy.max_users,
				CASE organization.tier::text WHEN 'starter' THEN 5 WHEN 'professional' THEN 25 WHEN 'enterprise' THEN 100 ELSE 0 END)
			WHEN 'frameworks' THEN COALESCE(plan.max_frameworks,legacy.max_frameworks,
				CASE organization.tier::text WHEN 'starter' THEN 3 WHEN 'professional' THEN 5 WHEN 'enterprise' THEN 9 ELSE 0 END)
			WHEN 'risks' THEN COALESCE(plan.max_risks,
				CASE organization.tier::text WHEN 'starter' THEN 50 ELSE 0 END)
			WHEN 'vendors' THEN COALESCE(plan.max_vendors,
				CASE organization.tier::text WHEN 'starter' THEN 10 WHEN 'professional' THEN 50 ELSE 0 END)
			WHEN 'storage_bytes' THEN COALESCE(plan.max_storage_gb,
				CASE organization.tier::text WHEN 'starter' THEN 5 WHEN 'professional' THEN 25 WHEN 'enterprise' THEN 100 ELSE 0 END) * 1073741824::bigint
		END
		FROM organizations organization
		LEFT JOIN LATERAL (
			SELECT subscription_plan.max_users,subscription_plan.max_frameworks,
				subscription_plan.max_risks,subscription_plan.max_vendors,subscription_plan.max_storage_gb
			FROM organization_subscriptions_v2 subscription
			JOIN subscription_plans subscription_plan ON subscription_plan.id=subscription.plan_id
			WHERE subscription.organization_id=organization.id
			LIMIT 1
		) plan ON true
		LEFT JOIN LATERAL (
			SELECT subscription.max_users,subscription.max_frameworks
			FROM organization_subscriptions subscription
			WHERE subscription.organization_id=organization.id
			ORDER BY subscription.created_at DESC LIMIT 1
		) legacy ON plan.max_users IS NULL
		WHERE organization.id=$1::uuid AND organization.deleted_at IS NULL`, organizationID, metric).Scan(&limit)
	if err != nil {
		return 0, fmt.Errorf("loading %s entitlement limit: %w", metric, err)
	}
	return limit, nil
}

func entitlementUsage(ctx context.Context, tx pgx.Tx, organizationID, metric string) (int64, error) {
	queries := map[string]string{
		"users":         `SELECT COUNT(*) FROM users WHERE organization_id=$1::uuid AND deleted_at IS NULL`,
		"frameworks":    `SELECT COUNT(*) FROM organization_frameworks WHERE organization_id=$1::uuid`,
		"risks":         `SELECT COUNT(*) FROM risks WHERE organization_id=$1::uuid AND deleted_at IS NULL`,
		"vendors":       `SELECT COUNT(*) FROM vendors WHERE organization_id=$1::uuid AND deleted_at IS NULL`,
		"storage_bytes": `SELECT COALESCE(SUM(file_size_bytes),0) FROM control_evidence WHERE organization_id=$1::uuid AND deleted_at IS NULL`,
	}
	var usage int64
	if err := tx.QueryRow(ctx, queries[metric], organizationID).Scan(&usage); err != nil {
		return 0, fmt.Errorf("loading %s entitlement usage: %w", metric, err)
	}
	return usage, nil
}
