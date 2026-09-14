-- Migration 040: bootstrap compatibility repairs and deterministic seed history

-- Databases that applied the original migrations 014 and 030 had both triggers
-- attached to one generate_exception_ref() function. Re-establish the two
-- domain-specific functions/triggers before removing the ambiguous function.
CREATE OR REPLACE FUNCTION generate_policy_exception_ref()
RETURNS TRIGGER AS $$
DECLARE
    next_num INT;
BEGIN
    IF NEW.exception_ref IS NULL OR NEW.exception_ref = '' THEN
        SELECT COALESCE(MAX(
            CASE WHEN exception_ref ~ '^EXC-[0-9]+$'
                 THEN CAST(SUBSTRING(exception_ref FROM 5) AS INT)
                 ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM policy_exceptions
        WHERE organization_id = NEW.organization_id;

        NEW.exception_ref := 'EXC-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION generate_compliance_exception_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.exception_ref IS NULL OR NEW.exception_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN exception_ref ~ ('^EXC-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(exception_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM compliance_exceptions
        WHERE organization_id = NEW.organization_id;

        NEW.exception_ref := 'EXC-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_exceptions_generate_ref ON policy_exceptions;
CREATE TRIGGER trg_exceptions_generate_ref
    BEFORE INSERT ON policy_exceptions
    FOR EACH ROW EXECUTE FUNCTION generate_policy_exception_ref();

DROP TRIGGER IF EXISTS trg_comp_exceptions_generate_ref ON compliance_exceptions;
CREATE TRIGGER trg_comp_exceptions_generate_ref
    BEFORE INSERT ON compliance_exceptions
    FOR EACH ROW EXECUTE FUNCTION generate_compliance_exception_ref();

DROP FUNCTION IF EXISTS generate_exception_ref();

-- Compatibility repair for databases where the original NIS2 seed created
-- this table outside the migration lifecycle. Fresh databases create it in
-- migration 019; IF NOT EXISTS keeps upgrades safe.
CREATE TABLE IF NOT EXISTS nis2_measure_definitions (
    measure_code              VARCHAR(20) PRIMARY KEY,
    measure_title             VARCHAR(500) NOT NULL,
    measure_description       TEXT NOT NULL,
    article_reference         VARCHAR(50) NOT NULL,
    iso27001_control_codes    TEXT[] NOT NULL
);

CREATE TABLE IF NOT EXISTS bootstrap_seed_history (
    seed_name   TEXT PRIMARY KEY,
    checksum    CHAR(64) NOT NULL,
    position    INT NOT NULL UNIQUE,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_bootstrap_seed_name CHECK (
        seed_name <> '' AND seed_name = BTRIM(seed_name)
    ),
    CONSTRAINT chk_bootstrap_seed_checksum CHECK (
        checksum ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_bootstrap_seed_position CHECK (
        position > 0
    )
);

COMMENT ON TABLE bootstrap_seed_history IS
    'Checksums of ordered reference-data seeds applied by cmd/seed. Seed files are immutable after application.';
