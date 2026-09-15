-- A downgrade would remove immutable governance evidence and make timed/SoD
-- access perpetual. Refuse it once these semantics are in use; never silently
-- erase evidence or widen active authorization. Empty upgrades are reversible.
ALTER TABLE user_roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE access_review_campaigns NO FORCE ROW LEVEL SECURITY;
ALTER TABLE access_sod_rules NO FORCE ROW LEVEL SECURITY;
ALTER TABLE access_governance_events NO FORCE ROW LEVEL SECURITY;
DO $guard$ BEGIN
    IF EXISTS (SELECT 1 FROM user_roles WHERE expires_at IS NOT NULL OR valid_from<>'-infinity'::timestamptz)
        OR EXISTS (SELECT 1 FROM access_review_campaigns)
        OR EXISTS (SELECT 1 FROM access_sod_rules)
        OR EXISTS (SELECT 1 FROM access_governance_events) THEN
        RAISE EXCEPTION 'Access governance downgrade would lose evidence or access restrictions';
    END IF;
END $guard$;
DROP VIEW effective_user_roles;
DROP TRIGGER governed_role_assignment ON user_roles;
DROP FUNCTION enforce_governed_role_assignment();
DROP TABLE access_review_decisions,access_review_items,access_review_campaigns,
    access_sod_violations,access_sod_exceptions,access_sod_rules,access_governance_events;
DROP FUNCTION validate_access_governance_scope();
ALTER TABLE user_roles DROP CONSTRAINT chk_user_roles_window,
    DROP CONSTRAINT IF EXISTS uq_user_roles_assignment_id,DROP COLUMN IF EXISTS assignment_id,
    DROP CONSTRAINT chk_user_roles_version,DROP CONSTRAINT chk_user_roles_timed_reason,
    DROP CONSTRAINT chk_user_roles_approval,DROP CONSTRAINT fk_user_roles_window_approver,
    DROP COLUMN valid_from,DROP COLUMN expires_at,DROP COLUMN version,DROP COLUMN assignment_reason,DROP COLUMN window_approved_by;
ALTER TABLE user_roles FORCE ROW LEVEL SECURITY;
