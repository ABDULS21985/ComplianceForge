-- Additive tenant profile controls. Historical migrations are not rewritten.
-- Contact addresses are informational; this does not configure mail delivery,
-- SSO domains, data residency, billing, or administrator delegation.

CREATE FUNCTION public.valid_organization_profile_contacts(contacts JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql IMMUTABLE STRICT PARALLEL SAFE
SET search_path = pg_catalog, public, pg_temp
AS $contacts$
DECLARE
    entry JSONB;
    purpose TEXT;
    address TEXT;
    purposes TEXT[] := ARRAY[]::TEXT[];
BEGIN
    IF jsonb_typeof(contacts) <> 'array' THEN
        RETURN FALSE;
    END IF;
    IF jsonb_array_length(contacts) > 4
       OR octet_length(contacts::TEXT) > 2048 THEN
        RETURN FALSE;
    END IF;
    FOR entry IN SELECT value FROM jsonb_array_elements(contacts) LOOP
        IF jsonb_typeof(entry) <> 'object' THEN
            RETURN FALSE;
        END IF;
        IF (SELECT count(*) FROM jsonb_object_keys(entry)) <> 2
           OR NOT entry ?& ARRAY['purpose', 'email']::TEXT[]
           OR jsonb_typeof(entry->'purpose') <> 'string'
           OR jsonb_typeof(entry->'email') <> 'string' THEN
            RETURN FALSE;
        END IF;
        purpose := entry->>'purpose';
        address := entry->>'email';
        IF purpose NOT IN ('security', 'privacy', 'billing', 'support')
           OR purpose = ANY(purposes)
           OR octet_length(address) NOT BETWEEN 3 AND 254
           OR address ~ '[^!-~]'
           OR address !~ '^[^@[:space:]]+@[^@[:space:]]+$' THEN
            RETURN FALSE;
        END IF;
        purposes := array_append(purposes, purpose);
    END LOOP;
    RETURN TRUE;
END;
$contacts$;

ALTER TABLE public.organizations
    ADD COLUMN profile_version BIGINT NOT NULL DEFAULT 1
        CHECK (profile_version BETWEEN 1 AND 9007199254740991),
    ADD COLUMN fiscal_year_start_month SMALLINT NOT NULL DEFAULT 1,
    ADD COLUMN fiscal_year_start_day SMALLINT NOT NULL DEFAULT 1,
    ADD COLUMN enterprise_contacts JSONB NOT NULL DEFAULT '[]'::JSONB,
    ADD CONSTRAINT organizations_fiscal_year_start_valid CHECK (
        fiscal_year_start_month BETWEEN 1 AND 12
        AND fiscal_year_start_day BETWEEN 1 AND CASE
            WHEN fiscal_year_start_month = 2 THEN 28
            WHEN fiscal_year_start_month IN (4, 6, 9, 11) THEN 30
            ELSE 31 END
    ),
    ADD CONSTRAINT organizations_enterprise_contacts_valid CHECK (
        public.valid_organization_profile_contacts(enterprise_contacts)
    );

CREATE FUNCTION public.guard_organization_profile_version()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public, pg_temp
AS $version$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.profile_version IS DISTINCT FROM 1::BIGINT THEN
            RAISE EXCEPTION 'organization profile initial version must be one'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        IF NEW.id IS DISTINCT FROM OLD.id THEN
            RAISE EXCEPTION 'organization identity is immutable'
                USING ERRCODE = '23514';
        END IF;
        IF NEW.profile_version IS DISTINCT FROM OLD.profile_version THEN
            RAISE EXCEPTION 'organization profile version is database managed'
                USING ERRCODE = '23514';
        END IF;
        IF OLD.profile_version >= 9007199254740991 THEN
            RAISE EXCEPTION 'organization profile version is exhausted'
                USING ERRCODE = '22003';
        END IF;
        NEW.profile_version := OLD.profile_version + 1;
    END IF;
    RETURN NEW;
END;
$version$;

CREATE TRIGGER organizations_profile_version_guard
BEFORE INSERT OR UPDATE ON public.organizations
FOR EACH ROW EXECUTE FUNCTION public.guard_organization_profile_version();

COMMENT ON COLUMN public.organizations.profile_version IS
    'Database-managed JavaScript-safe integer CAS version, incremented by every organization update, including legacy writers.';
COMMENT ON COLUMN public.organizations.enterprise_contacts IS
    'At most one informational ASCII email address per security/privacy/billing/support purpose; no mail-delivery or billing authority.';
COMMENT ON COLUMN public.organizations.fiscal_year_start_day IS
    'Recurring fiscal-year start in a non-leap calendar; February 29 is deliberately unsupported.';

REVOKE ALL ON FUNCTION public.valid_organization_profile_contacts(JSONB) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.guard_organization_profile_version() FROM PUBLIC;

-- The reviewed post-migration runtime manifest grants the scalar invoker
-- CHECK helper. Trigger execution needs no caller EXECUTE privilege.
