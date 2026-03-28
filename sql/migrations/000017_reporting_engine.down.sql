-- Rollback Migration 017: Drop reporting engine tables and types.

-- Drop RLS policies
DROP POLICY IF EXISTS report_runs_tenant_select ON report_runs;
DROP POLICY IF EXISTS report_runs_tenant_insert ON report_runs;
DROP POLICY IF EXISTS report_runs_tenant_update ON report_runs;
DROP POLICY IF EXISTS report_runs_tenant_delete ON report_runs;

DROP POLICY IF EXISTS report_sched_tenant_select ON report_schedules;
DROP POLICY IF EXISTS report_sched_tenant_insert ON report_schedules;
DROP POLICY IF EXISTS report_sched_tenant_update ON report_schedules;
DROP POLICY IF EXISTS report_sched_tenant_delete ON report_schedules;

DROP POLICY IF EXISTS report_def_tenant_select ON report_definitions;
DROP POLICY IF EXISTS report_def_tenant_insert ON report_definitions;
DROP POLICY IF EXISTS report_def_tenant_update ON report_definitions;
DROP POLICY IF EXISTS report_def_tenant_delete ON report_definitions;

-- Drop triggers
DROP TRIGGER IF EXISTS trg_report_schedules_updated_at ON report_schedules;
DROP TRIGGER IF EXISTS trg_report_definitions_updated_at ON report_definitions;

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS report_runs CASCADE;
DROP TABLE IF EXISTS report_schedules CASCADE;
DROP TABLE IF EXISTS report_definitions CASCADE;

-- Drop enum types
DROP TYPE IF EXISTS report_run_status;
DROP TYPE IF EXISTS report_schedule_frequency;
DROP TYPE IF EXISTS report_format;
DROP TYPE IF EXISTS report_type;
