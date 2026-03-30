-- Migration 032 DOWN: TPRM Questionnaires & Vendor Assessments
-- ComplianceForge GRC Platform
-- Drop everything in reverse dependency order

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

-- vendor_assessment_responses
DROP POLICY IF EXISTS vendor_responses_tenant_delete ON vendor_assessment_responses;
DROP POLICY IF EXISTS vendor_responses_tenant_update ON vendor_assessment_responses;
DROP POLICY IF EXISTS vendor_responses_tenant_insert ON vendor_assessment_responses;
DROP POLICY IF EXISTS vendor_responses_tenant_select ON vendor_assessment_responses;

-- vendor_assessments
DROP POLICY IF EXISTS vendor_assess_tenant_delete ON vendor_assessments;
DROP POLICY IF EXISTS vendor_assess_tenant_update ON vendor_assessments;
DROP POLICY IF EXISTS vendor_assess_tenant_insert ON vendor_assessments;
DROP POLICY IF EXISTS vendor_assess_tenant_select ON vendor_assessments;

-- assessment_questionnaires
DROP POLICY IF EXISTS assess_quest_tenant_delete ON assessment_questionnaires;
DROP POLICY IF EXISTS assess_quest_tenant_update ON assessment_questionnaires;
DROP POLICY IF EXISTS assess_quest_tenant_insert ON assessment_questionnaires;
DROP POLICY IF EXISTS assess_quest_tenant_select ON assessment_questionnaires;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_vendor_assess_generate_ref ON vendor_assessments;
DROP TRIGGER IF EXISTS trg_vendor_assess_updated_at ON vendor_assessments;
DROP TRIGGER IF EXISTS trg_vendor_responses_updated_at ON vendor_assessment_responses;
DROP TRIGGER IF EXISTS trg_assess_quest_updated_at ON assessment_questionnaires;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

DROP TABLE IF EXISTS vendor_portal_sessions;
DROP TABLE IF EXISTS vendor_assessment_responses;
DROP TABLE IF EXISTS vendor_assessments;
DROP TABLE IF EXISTS questionnaire_questions;
DROP TABLE IF EXISTS questionnaire_sections;
DROP TABLE IF EXISTS assessment_questionnaires;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS generate_vendor_assessment_ref();
