-- Rollback Migration 041: integration lifecycle and API-key tenant resolution

DROP FUNCTION IF EXISTS resolve_api_key_tenant(TEXT);

DROP TRIGGER IF EXISTS trg_api_keys_tenant_index ON api_keys;
DROP FUNCTION IF EXISTS maintain_api_key_tenant_index();
DROP TABLE IF EXISTS api_key_tenant_index;

DROP INDEX IF EXISTS idx_integrations_org_active;
DROP INDEX IF EXISTS uq_api_keys_hash;
ALTER TABLE api_keys
    DROP CONSTRAINT IF EXISTS chk_api_keys_expiry,
    DROP CONSTRAINT IF EXISTS chk_api_keys_hash_format,
    DROP CONSTRAINT IF EXISTS chk_api_keys_rate_limit;
ALTER TABLE integrations
    DROP CONSTRAINT IF EXISTS chk_integrations_deleted_state,
    DROP CONSTRAINT IF EXISTS chk_integrations_error_count,
    DROP CONSTRAINT IF EXISTS chk_integrations_sync_frequency;
ALTER TABLE integrations DROP COLUMN IF EXISTS deleted_at;
