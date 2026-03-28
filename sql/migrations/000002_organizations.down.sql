-- Rollback Migration 002: Drop organizations and subscriptions.

DROP TRIGGER IF EXISTS trg_org_subscriptions_updated_at ON organization_subscriptions;
DROP TABLE IF EXISTS organization_subscriptions CASCADE;

DROP TRIGGER IF EXISTS trg_organizations_updated_at ON organizations;
DROP TABLE IF EXISTS organizations CASCADE;

DROP FUNCTION IF EXISTS update_updated_at_column();
