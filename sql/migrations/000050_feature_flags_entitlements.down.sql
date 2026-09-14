-- Rollback Migration 050: capability catalogue and tenant feature flags.

DROP TRIGGER IF EXISTS trg_feature_flag_events_immutable ON feature_flag_change_events;
DROP FUNCTION IF EXISTS prevent_feature_flag_event_mutation();
DROP TABLE IF EXISTS feature_flag_change_events;
DROP TRIGGER IF EXISTS trg_tenant_feature_flags_updated_at ON tenant_feature_flag_overrides;
DROP TABLE IF EXISTS tenant_feature_flag_overrides;
DROP TRIGGER IF EXISTS trg_product_capabilities_updated_at ON product_capabilities;
DROP TABLE IF EXISTS product_capabilities;
