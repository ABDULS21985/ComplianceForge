-- Rollback Migration 014: Drop policy operations tables.

-- Drop RLS policies
DROP POLICY IF EXISTS pcm_tenant_select ON policy_control_mappings;
DROP POLICY IF EXISTS pcm_tenant_insert ON policy_control_mappings;
DROP POLICY IF EXISTS pcm_tenant_update ON policy_control_mappings;
DROP POLICY IF EXISTS pcm_tenant_delete ON policy_control_mappings;

DROP POLICY IF EXISTS comment_tenant_select ON policy_comments;
DROP POLICY IF EXISTS comment_tenant_insert ON policy_comments;
DROP POLICY IF EXISTS comment_tenant_update ON policy_comments;
DROP POLICY IF EXISTS comment_tenant_delete ON policy_comments;

DROP POLICY IF EXISTS review_tenant_select ON policy_reviews;
DROP POLICY IF EXISTS review_tenant_insert ON policy_reviews;
DROP POLICY IF EXISTS review_tenant_update ON policy_reviews;
DROP POLICY IF EXISTS review_tenant_delete ON policy_reviews;

DROP POLICY IF EXISTS exc_tenant_select ON policy_exceptions;
DROP POLICY IF EXISTS exc_tenant_insert ON policy_exceptions;
DROP POLICY IF EXISTS exc_tenant_update ON policy_exceptions;
DROP POLICY IF EXISTS exc_tenant_delete ON policy_exceptions;

DROP POLICY IF EXISTS attest_tenant_select ON policy_attestations;
DROP POLICY IF EXISTS attest_tenant_insert ON policy_attestations;
DROP POLICY IF EXISTS attest_tenant_update ON policy_attestations;
DROP POLICY IF EXISTS attest_tenant_delete ON policy_attestations;

DROP POLICY IF EXISTS campaign_tenant_select ON policy_attestation_campaigns;
DROP POLICY IF EXISTS campaign_tenant_insert ON policy_attestation_campaigns;
DROP POLICY IF EXISTS campaign_tenant_update ON policy_attestation_campaigns;
DROP POLICY IF EXISTS campaign_tenant_delete ON policy_attestation_campaigns;

DROP POLICY IF EXISTS step_tenant_select ON policy_approval_steps;
DROP POLICY IF EXISTS step_tenant_insert ON policy_approval_steps;
DROP POLICY IF EXISTS step_tenant_update ON policy_approval_steps;
DROP POLICY IF EXISTS step_tenant_delete ON policy_approval_steps;

DROP POLICY IF EXISTS wf_tenant_select ON policy_approval_workflows;
DROP POLICY IF EXISTS wf_tenant_insert ON policy_approval_workflows;
DROP POLICY IF EXISTS wf_tenant_update ON policy_approval_workflows;
DROP POLICY IF EXISTS wf_tenant_delete ON policy_approval_workflows;

-- Drop triggers and functions
DROP TRIGGER IF EXISTS trg_policy_control_mappings_updated_at ON policy_control_mappings;
DROP TRIGGER IF EXISTS trg_policy_comments_updated_at ON policy_comments;
DROP TRIGGER IF EXISTS trg_policy_reviews_updated_at ON policy_reviews;
DROP TRIGGER IF EXISTS trg_exceptions_updated_at ON policy_exceptions;
DROP TRIGGER IF EXISTS trg_exceptions_generate_ref ON policy_exceptions;
DROP TRIGGER IF EXISTS trg_campaigns_updated_at ON policy_attestation_campaigns;
DROP TRIGGER IF EXISTS trg_approval_steps_updated_at ON policy_approval_steps;
DROP TRIGGER IF EXISTS trg_policy_workflows_updated_at ON policy_approval_workflows;

DROP FUNCTION IF EXISTS generate_exception_ref();

-- Drop tables in dependency order
DROP TABLE IF EXISTS policy_control_mappings CASCADE;
DROP TABLE IF EXISTS policy_comments CASCADE;
DROP TABLE IF EXISTS policy_reviews CASCADE;
DROP TABLE IF EXISTS policy_exceptions CASCADE;
DROP TABLE IF EXISTS policy_attestations CASCADE;
DROP TABLE IF EXISTS policy_attestation_campaigns CASCADE;
DROP TABLE IF EXISTS policy_approval_steps CASCADE;
DROP TABLE IF EXISTS policy_approval_workflows CASCADE;
