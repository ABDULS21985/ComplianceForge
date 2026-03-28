-- Rollback Migration 018: Drop GDPR Data Subject Request (DSR) tables, triggers, and types.

-- Drop RLS policies
DROP POLICY IF EXISTS dsr_templates_tenant_select ON dsr_response_templates;
DROP POLICY IF EXISTS dsr_templates_tenant_insert ON dsr_response_templates;
DROP POLICY IF EXISTS dsr_templates_tenant_update ON dsr_response_templates;
DROP POLICY IF EXISTS dsr_templates_tenant_delete ON dsr_response_templates;

DROP POLICY IF EXISTS dsr_audit_tenant_select ON dsr_audit_trail;
DROP POLICY IF EXISTS dsr_audit_tenant_insert ON dsr_audit_trail;

DROP POLICY IF EXISTS dsr_tasks_tenant_select ON dsr_tasks;
DROP POLICY IF EXISTS dsr_tasks_tenant_insert ON dsr_tasks;
DROP POLICY IF EXISTS dsr_tasks_tenant_update ON dsr_tasks;
DROP POLICY IF EXISTS dsr_tasks_tenant_delete ON dsr_tasks;

DROP POLICY IF EXISTS dsr_requests_tenant_select ON dsr_requests;
DROP POLICY IF EXISTS dsr_requests_tenant_insert ON dsr_requests;
DROP POLICY IF EXISTS dsr_requests_tenant_update ON dsr_requests;
DROP POLICY IF EXISTS dsr_requests_tenant_delete ON dsr_requests;

-- Drop triggers
DROP TRIGGER IF EXISTS trg_dsr_requests_calc_deadline ON dsr_requests;
DROP TRIGGER IF EXISTS trg_dsr_requests_generate_ref ON dsr_requests;
DROP TRIGGER IF EXISTS trg_dsr_requests_updated_at ON dsr_requests;
DROP TRIGGER IF EXISTS trg_dsr_tasks_updated_at ON dsr_tasks;
DROP TRIGGER IF EXISTS trg_dsr_audit_trail_no_update ON dsr_audit_trail;
DROP TRIGGER IF EXISTS trg_dsr_response_templates_updated_at ON dsr_response_templates;

-- Drop trigger functions
DROP FUNCTION IF EXISTS calculate_dsr_response_deadline();
DROP FUNCTION IF EXISTS generate_dsr_ref();
DROP FUNCTION IF EXISTS dsr_audit_trail_prevent_update();

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS dsr_response_templates CASCADE;
DROP TABLE IF EXISTS dsr_audit_trail CASCADE;
DROP TABLE IF EXISTS dsr_tasks CASCADE;
DROP TABLE IF EXISTS dsr_requests CASCADE;

-- Drop enum types
DROP TYPE IF EXISTS dsr_sla_status;
DROP TYPE IF EXISTS dsr_task_status;
DROP TYPE IF EXISTS dsr_task_type;
DROP TYPE IF EXISTS dsr_priority;
DROP TYPE IF EXISTS dsr_status;
DROP TYPE IF EXISTS dsr_request_type;
