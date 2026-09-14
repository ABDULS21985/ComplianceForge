-- Rollback Migration 047: tenant-isolated enterprise asset inventory.

ALTER TABLE incidents DROP CONSTRAINT IF EXISTS fk_incidents_related_asset_tenant;
DROP TABLE IF EXISTS asset_events;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS asset_reference_sequences;
DROP FUNCTION IF EXISTS asset_tags_are_valid(TEXT[]);
