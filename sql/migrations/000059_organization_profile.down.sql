-- Refuse to silently erase configured contact/fiscal data or CAS history.
BEGIN;

-- The ordinary migration owner must inspect every tenant, not an empty FORCE
-- RLS projection. This owner-only ACCESS EXCLUSIVE change is transaction-local;
-- an error rolls it back and a successful downgrade restores FORCE before commit.
ALTER TABLE public.organizations NO FORCE ROW LEVEL SECURITY;
DO $loss_guard$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.organizations
        WHERE profile_version <> 1
           OR fiscal_year_start_month <> 1
           OR fiscal_year_start_day <> 1
           OR enterprise_contacts <> '[]'::JSONB
    ) THEN
        RAISE EXCEPTION 'organization profile rollback would erase configured data or version history; reviewed export/rollback is required';
    END IF;
END;
$loss_guard$;

DROP TRIGGER organizations_profile_version_guard ON public.organizations;
DROP FUNCTION public.guard_organization_profile_version();
ALTER TABLE public.organizations
    DROP CONSTRAINT organizations_enterprise_contacts_valid,
    DROP CONSTRAINT organizations_fiscal_year_start_valid,
    DROP COLUMN enterprise_contacts,
    DROP COLUMN fiscal_year_start_day,
    DROP COLUMN fiscal_year_start_month,
    DROP COLUMN profile_version;
DROP FUNCTION public.valid_organization_profile_contacts(JSONB);
ALTER TABLE public.organizations FORCE ROW LEVEL SECURITY;

COMMIT;
