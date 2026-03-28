-- Seed Data: Default Workflow Definitions
-- ComplianceForge GRC Platform
--
-- System-default workflow definitions with pre-configured steps.
-- Available to all organizations (organization_id = NULL, is_system = true).
-- Organizations can clone and customize these for their own needs.
--
-- UUID pattern: g0000000-0000-0000-0000-XXXXXXXXXXXX
--   Definitions: g0000000-0000-0000-0000-000000000001 through 000000000005
--   Steps:       g0000000-0000-0000-0001-XXYY (XX = definition, YY = step)

BEGIN;

-- ============================================================================
-- WORKFLOW 1: Policy Approval
-- 3 steps: Compliance Review -> Approver Approval -> Auto-Action Status Update
-- ============================================================================

INSERT INTO workflow_definitions (id, organization_id, name, description, workflow_type, entity_type, version, status, trigger_conditions, sla_config, is_system, created_by) VALUES
('g0000000-0000-0000-0000-000000000001', NULL,
 'Policy Approval Workflow',
 'Standard three-step policy approval process. A compliance team member reviews the policy for regulatory alignment, then a designated approver grants formal approval, and finally the system automatically updates the policy status to approved.',
 'policy_approval', 'policy', 1, 'active',
 '{"on_status_change": "pending_approval", "entity_type": "policy"}',
 '{"total_hours": 120, "warning_threshold_pct": 75}',
 true, NULL);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000100000001', 'g0000000-0000-0000-0000-000000000001', NULL, 1,
 'Compliance Review',
 'Compliance team reviews the policy for regulatory alignment, completeness, and consistency with existing policies.',
 'review', 'role', 'any_one', 48, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000100000002', 'g0000000-0000-0000-0000-000000000001', NULL, 2,
 'Approver Approval',
 'Designated policy approver reviews and formally approves or rejects the policy. Rejection returns the policy to draft status with feedback.',
 'approval', 'entity_owner', 'any_one', 72, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, auto_action) VALUES
('g0000000-0000-0000-0001-000100000003', 'g0000000-0000-0000-0000-000000000001', NULL, 3,
 'Update Policy Status',
 'Automatically updates the policy status to approved and sets the effective date.',
 'auto_action',
 '{"action": "update_status", "target": "entity", "value": "approved", "additional": {"set_effective_date": true}}');

-- ============================================================================
-- WORKFLOW 2: Risk Acceptance
-- Conditional workflow: CISO approval for critical/high, Risk Manager for medium/low
-- 3 steps: Condition -> CISO/Risk Manager Approval -> Auto-Action Record Acceptance
-- ============================================================================

INSERT INTO workflow_definitions (id, organization_id, name, description, workflow_type, entity_type, version, status, trigger_conditions, sla_config, is_system, created_by) VALUES
('g0000000-0000-0000-0000-000000000002', NULL,
 'Risk Acceptance Workflow',
 'Conditional risk acceptance workflow that routes to the CISO for critical and high risks, or to the Risk Manager for medium and low risks. Ensures appropriate authority level for risk acceptance decisions.',
 'risk_acceptance', 'risk', 1, 'active',
 '{"on_action": "request_acceptance", "entity_type": "risk"}',
 '{"total_hours": 168, "warning_threshold_pct": 75}',
 true, NULL);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, condition_expression, condition_true_step_id, condition_false_step_id) VALUES
('g0000000-0000-0000-0001-000200000001', 'g0000000-0000-0000-0000-000000000002', NULL, 1,
 'Risk Level Evaluation',
 'Evaluates the risk level to determine the required approval authority. Critical and high risks require CISO approval; medium and low risks require Risk Manager approval.',
 'condition',
 '{"field": "risk_level", "operator": "in", "value": ["critical", "high"]}',
 'g0000000-0000-0000-0001-000200000002',
 'g0000000-0000-0000-0001-000200000003');

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000200000002', 'g0000000-0000-0000-0000-000000000002', NULL, 2,
 'CISO Approval',
 'CISO reviews and approves or rejects the risk acceptance for critical/high risks. Requires documented justification and compensating controls.',
 'approval', 'role', 'any_one', 120, false);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000200000003', 'g0000000-0000-0000-0000-000000000002', NULL, 3,
 'Risk Manager Approval',
 'Risk Manager reviews and approves or rejects the risk acceptance for medium/low risks.',
 'approval', 'role', 'any_one', 72, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, auto_action) VALUES
('g0000000-0000-0000-0001-000200000004', 'g0000000-0000-0000-0000-000000000002', NULL, 4,
 'Record Risk Acceptance',
 'Automatically records the risk acceptance decision, sets the acceptance expiry date, and updates the risk treatment status.',
 'auto_action',
 '{"action": "update_status", "target": "entity", "value": "accepted", "additional": {"set_acceptance_date": true, "set_review_date_months": 12}}');

-- ============================================================================
-- WORKFLOW 3: Exception Request
-- 4 steps: Control Owner Review -> Risk Assessment Task -> Conditional CISO/Compliance
--          Approval -> Auto-Action Create Exception
-- ============================================================================

INSERT INTO workflow_definitions (id, organization_id, name, description, workflow_type, entity_type, version, status, trigger_conditions, sla_config, is_system, created_by) VALUES
('g0000000-0000-0000-0000-000000000003', NULL,
 'Exception Request Workflow',
 'Four-step exception request process. The control owner reviews the request, a risk assessment is performed, then conditional routing sends critical/high exceptions to the CISO and medium/low to compliance for approval. Finally, an approved exception record is automatically created.',
 'exception_request', 'exception', 1, 'active',
 '{"on_action": "request_exception", "entity_type": "control_implementation"}',
 '{"total_hours": 240, "warning_threshold_pct": 75}',
 true, NULL);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000300000001', 'g0000000-0000-0000-0000-000000000003', NULL, 1,
 'Control Owner Review',
 'Control owner reviews the exception request for technical validity, assesses the impact of the exception on control effectiveness, and provides a recommendation.',
 'review', 'entity_owner', 'any_one', 48, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, task_description, task_assignee_type, sla_hours) VALUES
('g0000000-0000-0000-0001-000300000002', 'g0000000-0000-0000-0000-000000000003', NULL, 2,
 'Risk Assessment',
 'Risk team performs a risk assessment of the exception to quantify the residual risk and identify compensating controls.',
 'task',
 'Perform risk assessment for the requested exception. Document residual risk, identify compensating controls, and provide a risk rating for the exception period.',
 'role', 72);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, condition_expression, condition_true_step_id, condition_false_step_id) VALUES
('g0000000-0000-0000-0001-000300000003', 'g0000000-0000-0000-0000-000000000003', NULL, 3,
 'Exception Severity Routing',
 'Routes the exception to the appropriate approver based on the assessed risk level. Critical/high exceptions require CISO approval; medium/low require compliance approval.',
 'condition',
 '{"field": "assessed_risk_level", "operator": "in", "value": ["critical", "high"]}',
 'g0000000-0000-0000-0001-000300000004',
 'g0000000-0000-0000-0001-000300000005');

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000300000004', 'g0000000-0000-0000-0000-000000000003', NULL, 4,
 'CISO Exception Approval',
 'CISO reviews and approves or rejects the high/critical risk exception. Requires documented compensating controls and an expiry date.',
 'approval', 'role', 'any_one', 96, false);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000300000005', 'g0000000-0000-0000-0000-000000000003', NULL, 5,
 'Compliance Exception Approval',
 'Compliance team reviews and approves or rejects the medium/low risk exception.',
 'approval', 'role', 'any_one', 72, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, auto_action) VALUES
('g0000000-0000-0000-0001-000300000006', 'g0000000-0000-0000-0000-000000000003', NULL, 6,
 'Create Exception Record',
 'Automatically creates the exception record with the approved parameters, sets the expiry date, and links compensating controls.',
 'auto_action',
 '{"action": "create_exception", "target": "control_implementation", "additional": {"copy_compensating_controls": true, "set_expiry_from_request": true, "notify_control_owner": true}}');

-- ============================================================================
-- WORKFLOW 4: Finding Remediation
-- 3 steps: Assign Remediation Task -> Evidence Review -> Verification
-- ============================================================================

INSERT INTO workflow_definitions (id, organization_id, name, description, workflow_type, entity_type, version, status, trigger_conditions, sla_config, is_system, created_by) VALUES
('g0000000-0000-0000-0000-000000000004', NULL,
 'Finding Remediation Workflow',
 'Three-step finding remediation process. A remediation task is assigned to the responsible party, evidence of remediation is reviewed, and finally verification confirms the finding is resolved.',
 'finding_remediation', 'finding', 1, 'active',
 '{"on_status_change": "open", "entity_type": "finding"}',
 '{"total_hours": 720, "warning_threshold_pct": 75}',
 true, NULL);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, task_description, task_assignee_type, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000400000001', 'g0000000-0000-0000-0000-000000000004', NULL, 1,
 'Assign Remediation Task',
 'Remediation task is assigned to the finding owner or responsible system owner. They must implement the corrective action and upload evidence.',
 'task',
 'Implement the corrective action described in the finding. Upload evidence of remediation (screenshots, configuration exports, test results) when complete.',
 'entity_owner', 480, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000400000002', 'g0000000-0000-0000-0000-000000000004', NULL, 2,
 'Evidence Review',
 'Compliance or audit team reviews the submitted remediation evidence to confirm it adequately addresses the finding.',
 'review', 'role', 'any_one', 120, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000400000003', 'g0000000-0000-0000-0000-000000000004', NULL, 3,
 'Verification',
 'Independent verification that the remediation is effective. The verifier confirms the control is operating as intended and closes the finding.',
 'approval', 'role', 'any_one', 120, true);

-- ============================================================================
-- WORKFLOW 5: Vendor Onboarding
-- 3 steps: Parallel Security + Legal Review -> Final Approval -> Auto-Action Activate
-- ============================================================================

INSERT INTO workflow_definitions (id, organization_id, name, description, workflow_type, entity_type, version, status, trigger_conditions, sla_config, is_system, created_by) VALUES
('g0000000-0000-0000-0000-000000000005', NULL,
 'Vendor Onboarding Workflow',
 'Three-step vendor onboarding process. Security and legal teams review the vendor in parallel (both must complete before proceeding), then a final approval is granted, and the vendor is automatically activated in the system.',
 'vendor_onboarding', 'vendor', 1, 'active',
 '{"on_action": "request_onboarding", "entity_type": "vendor"}',
 '{"total_hours": 336, "warning_threshold_pct": 75}',
 true, NULL);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, minimum_approvals, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000500000001', 'g0000000-0000-0000-0000-000000000005', NULL, 1,
 'Parallel Security & Legal Review',
 'Security team assesses the vendor''s security posture (questionnaire, certifications, pen test results) while legal reviews contractual terms, data processing agreements, and liability clauses. Both reviews must complete before proceeding.',
 'parallel_gate', 'role', 'all_required', 2, 168, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, approver_type, approval_mode, sla_hours, can_delegate) VALUES
('g0000000-0000-0000-0001-000500000002', 'g0000000-0000-0000-0000-000000000005', NULL, 2,
 'Final Approval',
 'Procurement or vendor management lead grants final approval for the vendor onboarding, considering both security and legal review outcomes.',
 'approval', 'role', 'any_one', 72, true);

INSERT INTO workflow_steps (id, workflow_definition_id, organization_id, step_order, name, description, step_type, auto_action) VALUES
('g0000000-0000-0000-0001-000500000003', 'g0000000-0000-0000-0000-000000000005', NULL, 3,
 'Activate Vendor',
 'Automatically activates the vendor in the system, creates the vendor record with approved risk tier, and schedules the first periodic assessment.',
 'auto_action',
 '{"action": "activate_vendor", "target": "entity", "additional": {"set_status": "active", "schedule_first_assessment_months": 12, "notify_requester": true, "create_vendor_record": true}}');

COMMIT;
