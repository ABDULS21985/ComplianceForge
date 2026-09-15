-- Migration 054 DOWN: return the policy-access schema to migration 024 form.

DROP TRIGGER IF EXISTS trg_access_policy_certifications_no_delete ON access_policy_certifications;
DROP TRIGGER IF EXISTS trg_access_policy_certifications_no_update ON access_policy_certifications;
DROP TABLE IF EXISTS access_policy_certifications;
DROP TRIGGER IF EXISTS trg_access_policy_events_no_delete ON access_policy_change_events;
DROP TRIGGER IF EXISTS trg_access_policy_events_no_update ON access_policy_change_events;
DROP TABLE IF EXISTS access_policy_change_events;

DROP INDEX IF EXISTS idx_access_audit_request;
ALTER TABLE access_audit_log
    DROP CONSTRAINT IF EXISTS chk_access_audit_request_id,
    DROP CONSTRAINT IF EXISTS chk_access_audit_reason_code,
    DROP CONSTRAINT IF EXISTS chk_access_audit_constraint_outcome,
    DROP CONSTRAINT IF EXISTS fk_access_audit_object_grant_tenant,
    DROP CONSTRAINT IF EXISTS fk_access_audit_policy_tenant,
    DROP CONSTRAINT IF EXISTS fk_access_audit_subject_tenant,
    ADD CONSTRAINT access_audit_log_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT access_audit_log_matched_policy_id_fkey FOREIGN KEY (matched_policy_id)
        REFERENCES access_policies(id) ON DELETE SET NULL,
    DROP COLUMN request_id,
    DROP COLUMN matched_object_grant_id,
    DROP COLUMN matched_policy_ids,
    DROP COLUMN reason_code,
    DROP COLUMN constraint_outcome,
    DROP COLUMN rbac_allowed;

DROP TRIGGER IF EXISTS trg_access_audit_no_delete ON access_audit_log;
DROP TRIGGER IF EXISTS trg_access_audit_no_update ON access_audit_log;
CREATE TRIGGER trg_access_audit_no_update
    BEFORE UPDATE ON access_audit_log FOR EACH ROW
    EXECUTE FUNCTION prevent_access_audit_modification();
CREATE TRIGGER trg_access_audit_no_delete
    BEFORE DELETE ON access_audit_log FOR EACH ROW
    EXECUTE FUNCTION prevent_access_audit_modification();

DROP INDEX IF EXISTS idx_entity_permissions_evaluation;
DROP INDEX IF EXISTS uq_entity_permissions_active_grant;
ALTER TABLE user_entity_permissions
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_version,
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_reasons,
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_watermark,
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_decision,
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_sponsorship,
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_window,
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_status,
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_actions,
    DROP CONSTRAINT IF EXISTS chk_entity_permissions_type,
    DROP CONSTRAINT IF EXISTS fk_entity_permissions_revoker_tenant,
    DROP CONSTRAINT IF EXISTS fk_entity_permissions_approver_tenant,
    DROP CONSTRAINT IF EXISTS fk_entity_permissions_sponsor_tenant,
    DROP CONSTRAINT IF EXISTS fk_entity_permissions_subject_tenant,
    DROP CONSTRAINT IF EXISTS uq_user_entity_permissions_org_id,
    ADD CONSTRAINT user_entity_permissions_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT user_entity_permissions_granted_by_fkey FOREIGN KEY (granted_by)
        REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT uq_user_entity_perm UNIQUE (user_id, entity_type, entity_id, permission_level),
    ALTER COLUMN expires_at DROP NOT NULL,
    DROP COLUMN updated_at,
    DROP COLUMN version,
    DROP COLUMN revocation_reason,
    DROP COLUMN revoked_by,
    DROP COLUMN revoked_at,
    DROP COLUMN decision_reason,
    DROP COLUMN grant_reason,
    DROP COLUMN watermark_text,
    DROP COLUMN require_watermark,
    DROP COLUMN allow_download,
    DROP COLUMN valid_from,
    DROP COLUMN approved_at,
    DROP COLUMN approved_by,
    DROP COLUMN status,
    DROP COLUMN actions;

DROP TRIGGER IF EXISTS trg_field_level_permissions_updated_at ON field_level_permissions;
DROP INDEX IF EXISTS idx_field_permissions_evaluation;
ALTER TABLE field_level_permissions
    DROP CONSTRAINT IF EXISTS uq_field_permissions_policy_path,
    DROP CONSTRAINT IF EXISTS chk_field_permissions_mask,
    DROP CONSTRAINT IF EXISTS chk_field_permissions_visibility,
    DROP CONSTRAINT IF EXISTS chk_field_permissions_classification,
    DROP CONSTRAINT IF EXISTS chk_field_permissions_path,
    DROP CONSTRAINT IF EXISTS chk_field_permissions_resource,
    DROP CONSTRAINT IF EXISTS chk_field_permissions_version,
    DROP CONSTRAINT IF EXISTS fk_field_permissions_updater_tenant,
    DROP CONSTRAINT IF EXISTS fk_field_permissions_creator_tenant,
    DROP CONSTRAINT IF EXISTS fk_field_permissions_policy_tenant,
    ADD CONSTRAINT field_level_permissions_access_policy_id_fkey FOREIGN KEY (access_policy_id)
        REFERENCES access_policies(id) ON DELETE CASCADE,
    ADD CONSTRAINT field_level_permissions_permission_check
        CHECK (visibility IN ('visible','masked','hidden')),
    DROP COLUMN updated_at,
    DROP COLUMN updated_by,
    DROP COLUMN created_by,
    DROP COLUMN version,
    DROP COLUMN mask_strategy,
    DROP COLUMN classification;
ALTER TABLE field_level_permissions RENAME COLUMN visibility TO permission;
ALTER TABLE field_level_permissions RENAME COLUMN field_path TO field_name;

DROP INDEX IF EXISTS idx_access_policy_assignments_evaluation;
ALTER TABLE access_policy_assignments
    DROP CONSTRAINT IF EXISTS uq_access_policy_assignments_scope,
    DROP CONSTRAINT IF EXISTS chk_access_policy_assignments_window,
    DROP CONSTRAINT IF EXISTS chk_access_policy_assignments_subject,
    DROP CONSTRAINT IF EXISTS fk_access_policy_assignments_creator_tenant,
    DROP CONSTRAINT IF EXISTS fk_access_policy_assignments_policy_tenant,
    ADD CONSTRAINT access_policy_assignments_access_policy_id_fkey FOREIGN KEY (access_policy_id)
        REFERENCES access_policies(id) ON DELETE CASCADE,
    ADD CONSTRAINT access_policy_assignments_created_by_fkey FOREIGN KEY (created_by)
        REFERENCES users(id) ON DELETE SET NULL,
    ALTER COLUMN created_by DROP NOT NULL;

DROP INDEX IF EXISTS idx_access_policies_evaluation;
DROP INDEX IF EXISTS uq_access_policies_org_name_active;
ALTER TABLE access_policies
    DROP CONSTRAINT IF EXISTS chk_access_policies_valid_window,
    DROP CONSTRAINT IF EXISTS chk_access_policies_condition_shapes,
    DROP CONSTRAINT IF EXISTS chk_access_policies_actions,
    DROP CONSTRAINT IF EXISTS chk_access_policies_resource_type,
    DROP CONSTRAINT IF EXISTS chk_access_policies_priority,
    DROP CONSTRAINT IF EXISTS chk_access_policies_name,
    DROP CONSTRAINT IF EXISTS chk_access_policies_version,
    DROP CONSTRAINT IF EXISTS fk_access_policies_updater_tenant,
    DROP CONSTRAINT IF EXISTS fk_access_policies_creator_tenant,
    DROP CONSTRAINT IF EXISTS uq_access_policies_org_id,
    ADD CONSTRAINT access_policies_created_by_fkey FOREIGN KEY (created_by)
        REFERENCES users(id) ON DELETE SET NULL,
    ALTER COLUMN environment_conditions DROP NOT NULL,
    ALTER COLUMN environment_conditions DROP DEFAULT,
    ALTER COLUMN resource_conditions DROP NOT NULL,
    ALTER COLUMN resource_conditions DROP DEFAULT,
    DROP COLUMN legacy_definition,
    DROP COLUMN deleted_at,
    DROP COLUMN updated_by,
    DROP COLUMN version;

DROP FUNCTION IF EXISTS prevent_policy_access_evidence_modification();
