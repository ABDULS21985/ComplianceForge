-- Migration 034 DOWN: Board Reporting Portal
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- board_reports
DROP POLICY IF EXISTS board_reports_tenant_delete ON board_reports;
DROP POLICY IF EXISTS board_reports_tenant_update ON board_reports;
DROP POLICY IF EXISTS board_reports_tenant_insert ON board_reports;
DROP POLICY IF EXISTS board_reports_tenant_select ON board_reports;

-- board_decisions
DROP POLICY IF EXISTS board_decisions_tenant_delete ON board_decisions;
DROP POLICY IF EXISTS board_decisions_tenant_update ON board_decisions;
DROP POLICY IF EXISTS board_decisions_tenant_insert ON board_decisions;
DROP POLICY IF EXISTS board_decisions_tenant_select ON board_decisions;

-- board_meetings
DROP POLICY IF EXISTS board_meetings_tenant_delete ON board_meetings;
DROP POLICY IF EXISTS board_meetings_tenant_update ON board_meetings;
DROP POLICY IF EXISTS board_meetings_tenant_insert ON board_meetings;
DROP POLICY IF EXISTS board_meetings_tenant_select ON board_meetings;

-- board_members
DROP POLICY IF EXISTS board_members_tenant_delete ON board_members;
DROP POLICY IF EXISTS board_members_tenant_update ON board_members;
DROP POLICY IF EXISTS board_members_tenant_insert ON board_members;
DROP POLICY IF EXISTS board_members_tenant_select ON board_members;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_board_decisions_generate_ref ON board_decisions;
DROP TRIGGER IF EXISTS trg_board_decisions_updated_at ON board_decisions;
DROP TRIGGER IF EXISTS trg_board_meetings_generate_ref ON board_meetings;
DROP TRIGGER IF EXISTS trg_board_meetings_updated_at ON board_meetings;
DROP TRIGGER IF EXISTS trg_board_members_updated_at ON board_members;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS board_reports;
DROP TABLE IF EXISTS board_decisions;
DROP TABLE IF EXISTS board_meetings;
DROP TABLE IF EXISTS board_members;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS generate_board_decision_ref();
DROP FUNCTION IF EXISTS generate_board_meeting_ref();
