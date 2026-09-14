-- Rollback Migration 048: tenant-isolated vendor management.

DROP FUNCTION IF EXISTS vendor_due_tenants(INTEGER);
ALTER TABLE assets DROP CONSTRAINT IF EXISTS fk_assets_vendor_tenant;
ALTER TABLE vendor_assessments DROP CONSTRAINT IF EXISTS fk_vendor_assessments_vendor_tenant;
DROP TABLE IF EXISTS vendor_events;
DROP TABLE IF EXISTS vendor_subprocessors;
DROP TABLE IF EXISTS vendor_certifications;
DROP TABLE IF EXISTS vendor_contracts;
DROP TABLE IF EXISTS vendor_contacts;
DROP TABLE IF EXISTS vendors;
DROP TABLE IF EXISTS vendor_reference_sequences;
DROP FUNCTION IF EXISTS vendor_text_array_is_valid(TEXT[], INTEGER);
