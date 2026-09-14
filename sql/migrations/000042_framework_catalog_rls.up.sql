-- Migration 042: Framework catalog child-table RLS
--
-- framework_domains and framework_controls inherit tenancy from their parent
-- compliance_framework. The original schema relied only on application joins,
-- leaving direct SQL paths able to cross tenant boundaries.

ALTER TABLE framework_domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE framework_domains FORCE ROW LEVEL SECURITY;

CREATE POLICY framework_domains_visible_parent_select
    ON framework_domains FOR SELECT
    USING (EXISTS (
        SELECT 1
        FROM compliance_frameworks framework
        WHERE framework.id = framework_domains.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id IS NULL
              OR framework.organization_id = get_current_tenant()
          )
    ));

CREATE POLICY framework_domains_writable_parent_insert
    ON framework_domains FOR INSERT
    WITH CHECK (EXISTS (
        SELECT 1
        FROM compliance_frameworks framework
        WHERE framework.id = framework_domains.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id = get_current_tenant()
              OR (
                  get_current_tenant() IS NULL
                  AND framework.organization_id IS NULL
                  AND framework.is_system_framework
              )
          )
    ));

CREATE POLICY framework_domains_writable_parent_update
    ON framework_domains FOR UPDATE
    USING (EXISTS (
        SELECT 1 FROM compliance_frameworks framework
        WHERE framework.id = framework_domains.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id = get_current_tenant()
              OR (get_current_tenant() IS NULL AND framework.organization_id IS NULL AND framework.is_system_framework)
          )
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM compliance_frameworks framework
        WHERE framework.id = framework_domains.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id = get_current_tenant()
              OR (get_current_tenant() IS NULL AND framework.organization_id IS NULL AND framework.is_system_framework)
          )
    ));

CREATE POLICY framework_domains_writable_parent_delete
    ON framework_domains FOR DELETE
    USING (EXISTS (
        SELECT 1 FROM compliance_frameworks framework
        WHERE framework.id = framework_domains.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id = get_current_tenant()
              OR (get_current_tenant() IS NULL AND framework.organization_id IS NULL AND framework.is_system_framework)
          )
    ));

ALTER TABLE framework_controls ENABLE ROW LEVEL SECURITY;
ALTER TABLE framework_controls FORCE ROW LEVEL SECURITY;

CREATE POLICY framework_controls_visible_parent_select
    ON framework_controls FOR SELECT
    USING (EXISTS (
        SELECT 1
        FROM compliance_frameworks framework
        WHERE framework.id = framework_controls.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id IS NULL
              OR framework.organization_id = get_current_tenant()
          )
    ));

CREATE POLICY framework_controls_writable_parent_insert
    ON framework_controls FOR INSERT
    WITH CHECK (EXISTS (
        SELECT 1
        FROM compliance_frameworks framework
        WHERE framework.id = framework_controls.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id = get_current_tenant()
              OR (
                  get_current_tenant() IS NULL
                  AND framework.organization_id IS NULL
                  AND framework.is_system_framework
              )
          )
    ));

CREATE POLICY framework_controls_writable_parent_update
    ON framework_controls FOR UPDATE
    USING (EXISTS (
        SELECT 1 FROM compliance_frameworks framework
        WHERE framework.id = framework_controls.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id = get_current_tenant()
              OR (get_current_tenant() IS NULL AND framework.organization_id IS NULL AND framework.is_system_framework)
          )
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM compliance_frameworks framework
        WHERE framework.id = framework_controls.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id = get_current_tenant()
              OR (get_current_tenant() IS NULL AND framework.organization_id IS NULL AND framework.is_system_framework)
          )
    ));

CREATE POLICY framework_controls_writable_parent_delete
    ON framework_controls FOR DELETE
    USING (EXISTS (
        SELECT 1 FROM compliance_frameworks framework
        WHERE framework.id = framework_controls.framework_id
          AND framework.deleted_at IS NULL
          AND (
              framework.organization_id = get_current_tenant()
              OR (get_current_tenant() IS NULL AND framework.organization_id IS NULL AND framework.is_system_framework)
          )
    ));

COMMENT ON POLICY framework_controls_visible_parent_select ON framework_controls IS
    'Controls are visible only when their parent framework is global or belongs to app.current_tenant.';
COMMENT ON POLICY framework_controls_writable_parent_insert ON framework_controls IS
    'Tenant sessions may mutate only private catalog children. An unset tenant is reserved for bootstrap maintenance of system catalog data.';
