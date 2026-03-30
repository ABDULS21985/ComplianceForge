# GRC Compliance Management Solution — 100 Master Prompts

## BATCH 6 — Exception Management, Evidence Template Library, Third-Party Risk Questionnaires, Data Classification & ROPA, Executive Board Reporting Portal

**Stack:** Golang 1.22+ | PostgreSQL 16 | Redis 7 | Next.js 14 | PDF/XLSX Generation
**Prerequisite:** All previous batches (Prompts 1–25) completed
**Deliverable:** Compliance exception lifecycle, automated evidence collection templates, vendor security questionnaires, GDPR data mapping/ROPA, and board-ready reporting portal

---

### PROMPT 26 OF 100 — Exception Management & Compensating Controls

```
You are a senior Golang backend engineer building the compliance exception management module for "ComplianceForge" — a GRC platform targeting European enterprises.

OBJECTIVE:
Build a complete exception management system that allows organisations to formally document, justify, approve, and track exceptions to compliance controls. When an organisation cannot implement a control as required (due to technical limitations, cost, legacy systems, or business constraints), they must document the exception with a risk assessment, compensating controls, and an expiry date. This is a critical audit requirement — auditors check that every non-compliant control has either a remediation plan OR an approved exception. The system must integrate with the workflow engine (Prompt 16) for approval chains and the notification engine (Prompt 11) for expiry alerts.

DATABASE SCHEMA — Create migration 021:

TABLE compliance_exceptions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - exception_ref VARCHAR(20) NOT NULL UNIQUE — EXC-2026-0001
  - title VARCHAR(500) NOT NULL
  - description TEXT NOT NULL — detailed justification
  - exception_type ENUM('permanent', 'temporary', 'conditional')
  - status ENUM('draft', 'pending_risk_assessment', 'pending_approval', 'approved', 'rejected', 'expired', 'revoked', 'renewal_pending')
  - priority ENUM('critical', 'high', 'medium', 'low')
  
  -- What is excepted
  - scope_type ENUM('single_control', 'control_group', 'framework_domain', 'policy_requirement', 'standard_requirement')
  - control_implementation_ids UUID[] — specific control implementations this exception covers
  - framework_control_codes TEXT[] — human-readable: ['A.8.9', 'CM-2']
  - policy_id UUID FK → policies — if exception is to a policy requirement
  - scope_description TEXT — human-readable scope
  
  -- Risk Assessment
  - risk_justification TEXT NOT NULL — why the control cannot be implemented
  - residual_risk_description TEXT — what risk remains with this exception
  - residual_risk_level ENUM('critical', 'high', 'medium', 'low', 'very_low')
  - risk_assessment_id UUID FK → risk_assessments — formal risk assessment if conducted
  - risk_accepted_by UUID FK → users — who accepted the residual risk
  - risk_accepted_at TIMESTAMPTZ
  
  -- Compensating Controls
  - has_compensating_controls BOOLEAN DEFAULT false
  - compensating_controls_description TEXT
  - compensating_control_ids UUID[] — FK → control_implementations providing compensation
  - compensating_effectiveness ENUM('full', 'partial', 'minimal', 'none')
  
  -- Lifecycle
  - requested_by UUID FK → users NOT NULL
  - requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  - approved_by UUID FK → users
  - approved_at TIMESTAMPTZ
  - approval_comments TEXT
  - rejection_reason TEXT
  - workflow_instance_id UUID FK → workflow_instances — approval workflow
  
  -- Validity
  - effective_date DATE NOT NULL
  - expiry_date DATE — NULL for permanent exceptions (requires annual review)
  - review_frequency_months INT DEFAULT 12 — even permanent exceptions must be reviewed
  - next_review_date DATE
  - last_review_date DATE
  - last_reviewed_by UUID FK → users
  - renewal_count INT DEFAULT 0
  
  -- Audit Trail
  - conditions TEXT — conditions under which this exception is valid (e.g., "only for legacy System X")
  - business_impact_if_implemented TEXT — cost/impact of actually implementing the control
  - regulatory_notification_required BOOLEAN DEFAULT false — does the regulator need to know?
  - regulator_notified_at TIMESTAMPTZ
  - audit_evidence_path TEXT — supporting documentation
  
  - tags TEXT[]
  - metadata JSONB
  - created_at, updated_at, deleted_at

TABLE exception_reviews:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - exception_id UUID FK → compliance_exceptions
  - review_type ENUM('periodic', 'triggered', 'audit', 'renewal')
  - review_date DATE NOT NULL
  - reviewer_user_id UUID FK → users
  - outcome ENUM('continue', 'modify', 'revoke', 'renew', 'escalate')
  - risk_reassessment TEXT — has the risk level changed?
  - new_risk_level risk_level — updated risk level if changed
  - compensating_control_effective BOOLEAN — are compensating controls still working?
  - conditions_still_valid BOOLEAN — are the exception conditions still met?
  - review_notes TEXT
  - next_review_date DATE
  - evidence_path TEXT
  - created_at

TABLE exception_audit_trail:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - exception_id UUID FK → compliance_exceptions
  - action VARCHAR(100) — 'created', 'submitted_for_approval', 'risk_assessed', 'approved', 'rejected', 'renewed', 'revoked', 'expired', 'reviewed', 'modified'
  - performed_by UUID FK → users
  - previous_status exception_status
  - new_status exception_status
  - details TEXT
  - metadata JSONB
  - created_at — immutable

GOLANG IMPLEMENTATION:

1. internal/service/exception_service.go:

   ExceptionService struct with methods:

   - CreateException(ctx, orgID, req) → creates exception in 'draft' status:
     * Auto-generate ref: EXC-{YYYY}-{NNNN}
     * Validate that referenced control_implementation_ids exist and belong to the org
     * Calculate initial risk level based on the controls being excepted
     * Create audit trail entry
   
   - SubmitForApproval(ctx, orgID, exceptionID) → transitions to 'pending_risk_assessment':
     * Validate exception has required fields (justification, scope, compensating controls)
     * Start the exception approval workflow (from Prompt 16 seed workflow):
       Step 1: Risk assessment by Risk Manager
       Step 2: Approval by CISO (for high/critical) or Compliance Officer (for medium/low)
       Step 3: If compensating controls exist, verify they are actually implemented
       Step 4: Final approval and activation
     * Create audit trail entry
   
   - ApproveException(ctx, orgID, exceptionID, approverID, comments) → transitions to 'approved':
     * Set approved_by, approved_at
     * Calculate expiry_date (if temporary) and next_review_date
     * Update linked control_implementations status to include exception note
     * The control's compliance_score contribution should reflect the exception:
       - Approved exception with full compensating controls → treated as "partially implemented"
       - Approved exception with no compensating controls → treated as "not implemented (excepted)"
     * Emit 'exception.approved' notification event
     * Create audit trail entry
   
   - RejectException(ctx, orgID, exceptionID, rejectorID, reason) → transitions to 'rejected'
   
   - RevokeException(ctx, orgID, exceptionID, revokerID, reason) → transitions to 'revoked':
     * When an exception is revoked, the underlying controls revert to their actual status
     * Creates a remediation action: "Implement control {code} — exception EXC-XXXX revoked"
     * Emit 'exception.revoked' notification
   
   - RenewException(ctx, orgID, exceptionID, newExpiryDate, justification) → extends exception:
     * Requires a fresh review
     * Increments renewal_count
     * Resets next_review_date
     * Requires re-approval through workflow
   
   - ReviewException(ctx, orgID, exceptionID, review) → periodic review:
     * Assess: is the exception still necessary? Are compensating controls effective?
     * Outcomes: continue (no change), modify (update conditions), revoke, renew, escalate
     * If risk level has increased: trigger escalation notification
   
   - GetExpiringExceptions(ctx, orgID, withinDays) → exceptions expiring within N days
   - GetExceptionDashboard(ctx, orgID) → metrics:
     * Total active exceptions
     * By risk level distribution
     * Expiring within 30/60/90 days
     * Overdue for review
     * Average exception age
     * Top excepted frameworks/controls
   
   - CalculateComplianceImpact(ctx, orgID, exceptionID) → what is the compliance score impact:
     * "This exception affects 3 controls across ISO 27001 and PCI DSS"
     * "Approving this exception will reduce ISO 27001 score from 82.3% to 79.1%"
     * "Compensating controls provide 65% coverage, effective impact: -1.2%"

2. internal/worker/exception_scheduler.go:
   - Daily check: exceptions approaching expiry (30d, 14d, 7d, 1d, expired)
   - Daily check: exceptions overdue for periodic review
   - Emit notification events: 'exception.expiring', 'exception.expired', 'exception.review_overdue'
   - Auto-expire: when expiry_date passes, change status to 'expired', revert control status

3. internal/handler/exception_handler.go — API Endpoints:
   - GET /exceptions — list exceptions (filterable by status, risk_level, framework)
   - POST /exceptions — create exception
   - GET /exceptions/{id} — exception detail with reviews and audit trail
   - PUT /exceptions/{id} — update exception (only in draft status)
   - POST /exceptions/{id}/submit — submit for approval
   - POST /exceptions/{id}/approve — approve (via workflow)
   - POST /exceptions/{id}/reject — reject
   - POST /exceptions/{id}/revoke — revoke active exception
   - POST /exceptions/{id}/renew — request renewal
   - POST /exceptions/{id}/review — submit periodic review
   - GET /exceptions/dashboard — exception metrics
   - GET /exceptions/expiring — exceptions nearing expiry
   - GET /exceptions/impact/{id} — compliance score impact analysis

4. INTEGRATION WITH EXISTING MODULES:
   - Workflow Engine (Prompt 16): exception approval uses the seeded "Exception Request Workflow"
   - Notification Engine (Prompt 11): expiry alerts, approval notifications, review reminders
   - Compliance Engine: exceptions factor into compliance score calculations
   - Audit Module: exceptions are flagged in audit findings — auditors see "Control A.8.9: Not Implemented — Exception EXC-2026-0001 (Approved, expires 2026-12-31)"
   - Risk Module: exceptions linked to risk assessments, residual risk tracked
   - Reporting Engine (Prompt 12): exception register included in compliance reports

5. NEXT.JS FRONTEND — /exceptions:
   - Exception Dashboard:
     * KPIs: active count, expiring within 30 days (amber), expired (red), overdue reviews, avg age
     * Risk level distribution donut chart
     * Framework impact chart (which frameworks have most exceptions)
   - Exception List:
     * DataTable: Ref, Title, Type badge, Status badge, Risk Level badge, Affected Controls (count), Expiry Date (red if <30d), Compensating (Yes/No badge), Last Reviewed
     * Filters: status, risk level, framework, type, expiring within
   - Create Exception Form (multi-step):
     * Step 1: Select controls to except (search + select from org's control implementations)
     * Step 2: Provide justification (rich text, mandatory)
     * Step 3: Risk assessment (risk level selector, residual risk description)
     * Step 4: Compensating controls (search + select existing implemented controls, describe effectiveness)
     * Step 5: Set validity (effective date, expiry date, review frequency)
     * Step 6: Review & submit for approval
   - Exception Detail Page:
     * Header: ref, title, status badge, risk level, type, expiry countdown
     * Affected controls list with framework badges
     * Justification and risk assessment sections
     * Compensating controls with effectiveness rating
     * Compliance impact analysis: "Score impact: ISO 27001 82.3% → 79.1%"
     * Review history timeline
     * Audit trail (immutable)
     * Action buttons: Submit, Approve, Reject, Renew, Revoke (based on status and user role)
   - For auditors: exception register view showing all exceptions with compliance impact summary

CRITICAL REQUIREMENTS:
- Exceptions MUST have approval workflow — no self-approval
- Expired exceptions automatically revert control status (enforced by scheduler)
- Compliance score calculation MUST account for exceptions:
  * Exception with full compensating controls: control counts as "partial" (50% credit)
  * Exception without compensating controls: control counts as "not implemented" (0% credit)
  * Exception with partial compensating controls: proportional credit based on compensating_effectiveness
- Renewal requires fresh risk assessment and re-approval
- Maximum 2 renewals for temporary exceptions — after that, must implement or make permanent
- Permanent exceptions require annual review (enforced, not optional)
- All state transitions logged immutably in exception_audit_trail
- Auditor view: comprehensive exception register showing all exceptions that affect compliance posture
- Integration test: create exception → approve → verify compliance score changes → expire → verify score reverts

OUTPUT: Complete Golang code for exception service, scheduler, handlers, migration, workflow integration, and Next.js exception management pages. Include unit tests for compliance impact calculation and expiry handling.
```

---

### PROMPT 27 OF 100 — Evidence Template Library & Automated Evidence Testing

```
You are a senior Golang backend engineer building the evidence template library and automated evidence testing system for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a library of evidence templates that define what evidence is required for each control, how to collect it, how to validate it, and how often it needs refreshing. The system provides pre-built evidence requirements for all 593 seeded controls, supports automated evidence validation testing, and generates evidence collection schedules. This eliminates the most time-consuming part of compliance: figuring out what evidence auditors want and collecting it.

DATABASE SCHEMA — Create migration 022:

TABLE evidence_templates:
  - id UUID PK
  - organization_id UUID FK (RLS, NULL for system templates)
  - framework_control_code VARCHAR(50) NOT NULL — e.g., 'A.8.9', 'AC-6', '8.4.1'
  - framework_code VARCHAR(20) NOT NULL — 'ISO27001', 'NIST_800_53', 'PCI_DSS_4'
  - name VARCHAR(300) NOT NULL — e.g., 'Configuration Baseline Documentation'
  - description TEXT NOT NULL — what this evidence proves
  - evidence_category ENUM('document', 'screenshot', 'configuration_export', 'log_extract', 'scan_report', 'interview_record', 'test_result', 'certification', 'training_record', 'policy_document', 'procedure_document', 'meeting_minutes', 'email_confirmation', 'system_report', 'audit_trail')
  
  -- Collection Guidance
  - collection_method ENUM('manual_upload', 'api_automated', 'script_automated', 'screenshot_capture', 'system_export', 'interview', 'observation')
  - collection_instructions TEXT NOT NULL — step-by-step how to collect this evidence
  - collection_frequency ENUM('once', 'daily', 'weekly', 'monthly', 'quarterly', 'semi_annually', 'annually', 'on_change')
  - typical_collection_time_minutes INT — estimated time to collect
  
  -- Validation
  - validation_rules JSONB — automated checks: [{"type": "file_not_empty"}, {"type": "date_within", "field": "report_date", "max_age_days": 90}, {"type": "contains_text", "text": "PASS"}]
  - acceptance_criteria TEXT — human-readable criteria
  - common_rejection_reasons TEXT[] — e.g., ['Screenshot is outdated', 'Report does not cover all systems']
  
  -- Template content
  - template_fields JSONB — structured fields to fill: [{"name": "system_name", "type": "text", "required": true}, {"name": "scan_date", "type": "date", "required": true}, {"name": "result", "type": "select", "options": ["pass", "fail"]}]
  - sample_evidence_description TEXT — what a good evidence artifact looks like
  - sample_file_path TEXT — link to a sample/example evidence document
  
  -- Metadata
  - applicable_to TEXT[] — ['all'] or specific entity types: ['cloud_services', 'on_premise', 'saas']
  - difficulty ENUM('easy', 'moderate', 'complex') — how hard is it to collect this evidence
  - auditor_priority ENUM('must_have', 'should_have', 'nice_to_have') — how important for auditors
  - is_system BOOLEAN DEFAULT false
  - tags TEXT[]
  - created_at, updated_at

TABLE evidence_requirements:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - control_implementation_id UUID FK → control_implementations
  - evidence_template_id UUID FK → evidence_templates
  - status ENUM('not_started', 'in_progress', 'collected', 'validated', 'expired', 'rejected')
  - is_mandatory BOOLEAN DEFAULT true
  - collection_frequency_override ENUM (overrides template default if set)
  - assigned_to UUID FK → users
  - due_date DATE
  - last_collected_at TIMESTAMPTZ
  - last_validated_at TIMESTAMPTZ
  - last_evidence_id UUID FK → control_evidence — most recent evidence artifact
  - validation_status ENUM('pending', 'passed', 'failed', 'warning')
  - validation_results JSONB — results of automated validation checks
  - next_collection_due DATE — calculated from frequency
  - consecutive_failures INT DEFAULT 0
  - notes TEXT
  - created_at, updated_at

TABLE evidence_test_suites:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL
  - description TEXT
  - test_type ENUM('automated_validation', 'control_effectiveness', 'configuration_check', 'compliance_verification')
  - schedule_cron VARCHAR(100) — when to run
  - is_active BOOLEAN DEFAULT true
  - last_run_at TIMESTAMPTZ
  - last_run_status ENUM('passed', 'failed', 'partial', 'error')
  - pass_threshold_percent DECIMAL(5,2) DEFAULT 80.0
  - created_at, updated_at

TABLE evidence_test_cases:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - test_suite_id UUID FK → evidence_test_suites
  - name VARCHAR(300) NOT NULL
  - description TEXT
  - test_type ENUM('evidence_exists', 'evidence_current', 'evidence_valid', 'field_check', 'file_check', 'api_check', 'script_check')
  - target_control_code VARCHAR(50)
  - target_evidence_template_id UUID FK → evidence_templates
  - test_config JSONB NOT NULL — test-specific configuration:
    * evidence_exists: {"control_implementation_id": "..."}
    * evidence_current: {"max_age_days": 90}
    * field_check: {"field": "scan_result", "operator": "equals", "value": "pass"}
    * api_check: {"url": "...", "expected_status": 200, "expected_body_contains": "compliant"}
  - expected_result VARCHAR(100)
  - sort_order INT
  - is_critical BOOLEAN DEFAULT false
  - created_at, updated_at

TABLE evidence_test_runs:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - test_suite_id UUID FK → evidence_test_suites
  - status ENUM('running', 'completed', 'failed', 'error')
  - started_at TIMESTAMPTZ
  - completed_at TIMESTAMPTZ
  - total_tests INT
  - passed INT
  - failed INT
  - skipped INT
  - errors INT
  - pass_rate DECIMAL(5,2)
  - threshold_met BOOLEAN
  - results JSONB — [{test_case_id, status, message, duration_ms}]
  - triggered_by ENUM('schedule', 'manual', 'ci_cd', 'pre_audit')
  - triggered_by_user UUID FK → users
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/evidence_template_service.go:

   - GetTemplatesForControl(ctx, controlCode, frameworkCode) → returns all evidence templates for a control
   - GetTemplatesForFramework(ctx, orgID, frameworkCode) → all templates for a framework
   - GenerateEvidenceRequirements(ctx, orgID, frameworkID) → for all control implementations in the framework:
     * Look up evidence templates matching each control code
     * Create evidence_requirements records for each
     * Calculate initial due dates based on collection frequency
     * Assign to control owners
     * Return summary: total requirements, by category, by difficulty
   - ValidateEvidence(ctx, orgID, requirementID, evidenceID) → run validation rules against uploaded evidence:
     * Check file_not_empty
     * Check date_within (is the evidence recent enough?)
     * Check contains_text (does a scan report contain "PASS"?)
     * Check file_type (is it a PDF, not a random file?)
     * Check file_size (reasonable size for the evidence type?)
     * Return: pass/fail per rule, overall status
   - GetEvidenceGaps(ctx, orgID) → controls missing required evidence:
     * No evidence uploaded
     * Evidence expired (past collection frequency)
     * Evidence validation failed
     * Grouped by framework, sorted by auditor_priority
   - GetCollectionSchedule(ctx, orgID) → upcoming evidence collection tasks:
     * What needs collecting this week/month
     * Who is assigned
     * What's overdue

2. internal/service/evidence_test_runner.go:

   - RunTestSuite(ctx, orgID, suiteID, triggeredBy) → execute all test cases:
     * For each test case:
       - evidence_exists: check if control has current, validated evidence
       - evidence_current: check if evidence age is within acceptable range
       - evidence_valid: re-run validation rules on the latest evidence
       - field_check: verify a specific field value in evidence metadata
       - api_check: call an external API and verify response
       - script_check: execute a validation script
     * Record results in evidence_test_runs
     * Calculate pass rate
     * If pass_rate < threshold: emit 'evidence.test_suite_failed' notification
     * Return results summary
   
   - RunPreAuditChecks(ctx, orgID, frameworkID) → comprehensive pre-audit evidence verification:
     * Run all evidence checks for the specified framework
     * Identify: missing evidence, expired evidence, failed validations
     * Generate a "readiness report" showing what needs attention before the audit
     * This is the killer feature for pre-audit preparation

3. internal/handler/evidence_template_handler.go — API Endpoints:
   - GET /evidence/templates — browse evidence templates (filterable by framework, control, category)
   - GET /evidence/templates/{id} — template detail
   - POST /evidence/templates — create custom template (org-specific)
   - GET /evidence/requirements — list evidence requirements for the org
   - POST /evidence/requirements/generate — generate requirements from templates for a framework
   - PUT /evidence/requirements/{id} — update requirement (assign, reschedule)
   - POST /evidence/requirements/{id}/validate — validate uploaded evidence against rules
   - GET /evidence/gaps — evidence gaps analysis
   - GET /evidence/schedule — upcoming collection schedule
   
   - GET /evidence/test-suites — list test suites
   - POST /evidence/test-suites — create test suite
   - POST /evidence/test-suites/{id}/run — run test suite
   - GET /evidence/test-suites/{id}/results — test run history
   - POST /evidence/pre-audit-check — run pre-audit evidence verification
   - GET /evidence/pre-audit-check/{id}/report — get readiness report

4. SEED DATA — System Evidence Templates (200+ templates):

   Create evidence templates for the most commonly audited controls:
   
   ISO 27001 (93 controls × 1-3 templates each = ~150 templates):
   - A.5.1 Policies → "Information Security Policy Document" (document, annually)
   - A.5.2 Roles → "RACI Matrix for Security Roles" (document, annually)
   - A.5.7 Threat Intelligence → "Threat Intelligence Report Subscription" (system_report, monthly)
   - A.5.15 Access Control → "Access Control Policy" + "User Access Review Report" (document + system_report, quarterly)
   - A.5.24 Incident Response → "Incident Response Plan" + "IR Test Results" (document + test_result, annually)
   - A.6.3 Training → "Security Awareness Training Completion Records" (training_record, annually)
   - A.8.2 Privileged Access → "Privileged Account Inventory" + "PAM Configuration Export" (system_report, quarterly)
   - A.8.5 Authentication → "MFA Enforcement Configuration Screenshot" (screenshot, quarterly)
   - A.8.7 Malware → "Anti-Malware Deployment Report" + "Malware Scan Results" (scan_report, monthly)
   - A.8.8 Vulnerability → "Vulnerability Scan Report" (scan_report, monthly)
   - A.8.9 Configuration → "Configuration Baseline Documentation" + "Configuration Compliance Report" (configuration_export, quarterly)
   - A.8.13 Backup → "Backup Configuration" + "Backup Restoration Test Results" (test_result, quarterly)
   - A.8.15 Logging → "Log Aggregation Configuration" + "Log Review Records" (log_extract, monthly)
   - A.8.16 Monitoring → "SIEM Dashboard Screenshot" + "Monitoring Alert Configuration" (screenshot, quarterly)
   - A.8.20 Network Security → "Firewall Rule Set Export" + "Network Diagram" (configuration_export, semi_annually)
   - A.8.24 Cryptography → "Encryption Configuration Report" + "Certificate Inventory" (system_report, quarterly)
   - A.8.25 SDLC → "Secure Development Policy" + "Code Review Records" (document + system_report, quarterly)
   - A.8.29 Security Testing → "Penetration Test Report" (scan_report, annually)
   - ... (continue for all 93 controls)
   
   PCI DSS (key controls, ~40 templates):
   - 1.2.1 → "Firewall Rule Review Report"
   - 6.3.3 → "Patch Management Report"
   - 8.4.1 → "MFA Configuration Evidence"
   - 10.2.1 → "Audit Log Configuration Evidence"
   - 11.3.1 → "Internal Vulnerability Scan Report"
   - 11.4.1 → "Penetration Test Report"
   - ...

   NIST 800-53 (key controls, ~50 templates):
   - AC-2 → "Account Inventory and Review Report"
   - AU-2 → "Audit Event Configuration"
   - CM-2 → "Baseline Configuration Documentation"
   - IA-2 → "Authentication Mechanism Configuration"
   - ...

   Each template includes: name, description, collection_instructions (detailed!), validation_rules, collection_frequency, auditor_priority, difficulty, sample_evidence_description

5. NEXT.JS FRONTEND:
   - /evidence/templates — browsable template library:
     * Search and filter by framework, control, category, difficulty, priority
     * Template cards: name, framework badge, control code, category badge, frequency, difficulty badge
     * Template detail: full instructions, validation rules, sample evidence description
   - /evidence/requirements — evidence tracking dashboard:
     * Summary: total requirements, collected %, validated %, expired %, gaps count
     * Calendar view: what's due this week/month
     * Kanban: Not Started → In Progress → Collected → Validated
     * Gap list: controls missing evidence, sorted by auditor priority
   - /evidence/testing — test suite management:
     * Suite list with last run status (green/red badge)
     * "Run Pre-Audit Check" button → shows progress → displays readiness report
     * Readiness report: pass/fail per control with details, overall readiness percentage
     * Historical test results chart (pass rate over time)
   - Control implementation page enhancement:
     * "Evidence Requirements" tab showing what evidence is needed
     * Each requirement: template name, status badge, due date, "Upload" button
     * Validation results shown inline after upload (green checks / red crosses)

CRITICAL REQUIREMENTS:
- Evidence templates cover ALL 93 ISO 27001 controls (the primary framework audited in Europe)
- Templates include specific, actionable collection instructions (not generic "collect evidence")
- Validation rules are automatable: system can check without human intervention
- Pre-audit check is the flagship feature: "Are we ready for the auditor?" with a clear YES/NO answer
- Evidence freshness is tracked: expired evidence = compliance gap
- Templates are extensible: orgs can create custom templates, marketplace packages can include templates
- The collection schedule integrates with the notification engine: reminders before due dates
- Evidence validation runs are logged and reportable (auditors want to see testing evidence)

OUTPUT: Complete Golang code for evidence template service, test runner, handlers, migration, 200+ seed evidence templates (comprehensive for ISO 27001, key for PCI DSS and NIST 800-53), and Next.js evidence management pages. Include unit tests for the validation rule engine.
```

---

### PROMPT 28 OF 100 — Third-Party Risk Assessment Questionnaires (TPRM)

```
You are a senior Golang backend engineer building the third-party risk management questionnaire system for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a complete system for creating, distributing, collecting, and scoring vendor security assessment questionnaires. European enterprises must assess their vendors' security posture per GDPR Article 28 (processor requirements), NIS2 Article 21(d) (supply chain security), and ISO 27001 A.5.19-A.5.23 (supplier relationships). The system must support: pre-built questionnaire templates (SIG Lite, CAIQ, custom), vendor self-service portal for responses, automated risk scoring, response comparison across vendors, and integration with the vendor module (Prompt 8).

DATABASE SCHEMA — Create migration 023:

TABLE assessment_questionnaires:
  - id UUID PK
  - organization_id UUID FK (RLS, NULL for system templates)
  - name VARCHAR(300) NOT NULL
  - description TEXT
  - questionnaire_type ENUM('security_assessment', 'privacy_assessment', 'gdpr_article_28', 'nis2_supply_chain', 'iso27001_supplier', 'pci_dss_tpsp', 'custom')
  - version INT DEFAULT 1
  - status ENUM('draft', 'active', 'deprecated')
  - total_questions INT
  - total_sections INT
  - estimated_completion_minutes INT
  - scoring_method ENUM('weighted_average', 'pass_fail', 'maturity_model', 'risk_rated')
  - pass_threshold DECIMAL(5,2) — minimum score to "pass"
  - risk_tier_thresholds JSONB — {"critical": [0, 25], "high": [25, 50], "medium": [50, 75], "low": [75, 100]}
  - applicable_vendor_tiers TEXT[] — e.g., ['critical', 'high'] — only send to these tiers
  - is_system BOOLEAN DEFAULT false
  - is_template BOOLEAN DEFAULT false
  - created_by UUID FK → users
  - created_at, updated_at

TABLE questionnaire_sections:
  - id UUID PK
  - questionnaire_id UUID FK → assessment_questionnaires
  - name VARCHAR(200) NOT NULL — e.g., 'Access Control', 'Data Protection', 'Incident Response'
  - description TEXT
  - sort_order INT
  - weight DECIMAL(5,2) DEFAULT 1.0 — section weight in scoring
  - framework_domain_code VARCHAR(50) — mapped ISO/NIST domain
  - created_at

TABLE questionnaire_questions:
  - id UUID PK
  - section_id UUID FK → questionnaire_sections
  - question_text TEXT NOT NULL
  - question_type ENUM('yes_no', 'yes_no_na', 'multiple_choice', 'single_choice', 'rating_scale', 'text', 'date', 'file_upload', 'multi_select')
  - options JSONB — for choice questions: [{"value": "yes", "label": "Yes", "score": 100}, {"value": "partial", "label": "Partially Implemented", "score": 50}, ...]
  - is_required BOOLEAN DEFAULT true
  - weight DECIMAL(5,2) DEFAULT 1.0 — question weight in section scoring
  - risk_impact ENUM('critical', 'high', 'medium', 'low') — impact if answered negatively
  - guidance_text TEXT — help text for the vendor filling it out
  - evidence_required BOOLEAN DEFAULT false — must attach evidence for certain answers
  - evidence_guidance TEXT — what evidence to upload
  - mapped_control_codes TEXT[] — e.g., ['A.5.15', 'AC-3'] — which controls this verifies
  - conditional_on JSONB — only show if another question has a specific answer: {"question_id": "...", "answer": "yes"}
  - sort_order INT
  - tags TEXT[]
  - created_at

TABLE vendor_assessments:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - vendor_id UUID FK → vendors
  - questionnaire_id UUID FK → assessment_questionnaires
  - assessment_ref VARCHAR(20) — VA-2026-0001
  - status ENUM('draft', 'sent', 'in_progress', 'submitted', 'under_review', 'completed', 'expired', 'cancelled')
  
  -- Distribution
  - sent_at TIMESTAMPTZ
  - sent_to_email VARCHAR(300) — vendor contact email
  - sent_to_name VARCHAR(200)
  - access_token_hash VARCHAR(128) — SHA-256 of the unique access link token
  - reminder_count INT DEFAULT 0
  - last_reminder_at TIMESTAMPTZ
  - due_date DATE NOT NULL
  - submitted_at TIMESTAMPTZ
  
  -- Scoring
  - overall_score DECIMAL(5,2) — 0–100
  - risk_rating ENUM('critical', 'high', 'medium', 'low')
  - section_scores JSONB — [{"section_id": "...", "section_name": "...", "score": 72.5, "max_score": 100}]
  - critical_findings INT DEFAULT 0 — questions with critical risk_impact answered negatively
  - high_findings INT DEFAULT 0
  - pass_fail ENUM('pass', 'fail', 'conditional_pass')
  
  -- Review
  - reviewed_by UUID FK → users
  - reviewed_at TIMESTAMPTZ
  - review_notes TEXT
  - reviewer_override_score DECIMAL(5,2) — reviewer can override calculated score
  - reviewer_override_reason TEXT
  
  -- Follow-up
  - follow_up_required BOOLEAN DEFAULT false
  - follow_up_items JSONB — [{"question_id": "...", "issue": "...", "required_action": "...", "due_date": "..."}]
  - next_assessment_date DATE
  
  - metadata JSONB
  - created_at, updated_at

TABLE vendor_assessment_responses:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - assessment_id UUID FK → vendor_assessments
  - question_id UUID FK → questionnaire_questions
  - answer_value TEXT — the raw answer
  - answer_score DECIMAL(5,2) — calculated score for this answer
  - evidence_paths TEXT[] — uploaded evidence files
  - evidence_notes TEXT — vendor's explanation
  - reviewer_comment TEXT — org reviewer's comment on this answer
  - reviewer_flag ENUM('accepted', 'flagged', 'requires_evidence', 'requires_follow_up')
  - created_at, updated_at

TABLE vendor_portal_sessions:
  - id UUID PK
  - assessment_id UUID FK → vendor_assessments
  - access_token_hash VARCHAR(128)
  - vendor_email VARCHAR(300)
  - ip_address VARCHAR(45)
  - user_agent TEXT
  - started_at TIMESTAMPTZ
  - last_activity_at TIMESTAMPTZ
  - completed_at TIMESTAMPTZ
  - progress_percentage DECIMAL(5,2)
  - is_active BOOLEAN DEFAULT true

GOLANG IMPLEMENTATION:

1. internal/service/questionnaire_service.go:
   
   - CreateQuestionnaire(ctx, orgID, req) — create custom questionnaire with sections and questions
   - CloneTemplate(ctx, orgID, templateID) — clone a system template for org customisation
   - CalculateScore(ctx, assessmentID) → calculate scores per section and overall:
     * weighted_average: Σ(answer_score × question_weight × section_weight) / Σ(max_possible × weights)
     * pass_fail: pass if all critical questions answered positively
     * risk_rated: map overall score to risk tier thresholds
   - CompareVendors(ctx, orgID, assessmentIDs) → side-by-side comparison of vendor scores:
     * Section-by-section comparison
     * Strengths/weaknesses per vendor
     * Recommendation ranking

2. internal/service/vendor_assessment_service.go:
   
   - SendAssessment(ctx, orgID, vendorID, questionnaireID, dueDate, contactEmail) → distribute:
     * Generate unique access token (32 bytes random, URL-safe base64)
     * Store SHA-256 hash (never store the token plaintext)
     * Build access URL: https://app.complianceforge.io/vendor-portal/{token}
     * Send email with instructions and access link
     * Set status to 'sent'
   
   - SubmitAssessment(ctx, token) → vendor submits completed questionnaire:
     * Validate token against stored hash
     * Validate all required questions answered
     * Calculate scores
     * Set status to 'submitted'
     * Notify org assessor
   
   - ReviewAssessment(ctx, orgID, assessmentID, review) → org reviews responses:
     * Review each answer: accept, flag, require evidence, require follow-up
     * Override score if needed (with documented reason)
     * Determine pass/fail
     * Generate follow-up items for flagged responses
     * Complete assessment
   
   - SendReminder(ctx, orgID, assessmentID) → send reminder email to vendor
   
   - GetAssessmentDashboard(ctx, orgID) → metrics:
     * Total assessments: sent, in-progress, submitted, completed, overdue
     * Average vendor score by tier
     * Critical findings across all vendors
     * Vendors requiring follow-up
     * Score distribution histogram

3. internal/handler/vendor_portal_handler.go — Vendor Self-Service Portal (PUBLIC, no auth):
   
   These endpoints are accessed by EXTERNAL vendors using the access token — no JWT required:
   
   - GET /vendor-portal/{token} → validate token, return assessment metadata and questions
   - PUT /vendor-portal/{token}/responses — save partial responses (auto-save)
   - POST /vendor-portal/{token}/responses/{questionId}/evidence — upload evidence file
   - POST /vendor-portal/{token}/submit — submit completed assessment
   - GET /vendor-portal/{token}/progress — get completion progress
   
   Security:
   - Token validated against SHA-256 hash on every request
   - Token expires with the assessment due date
   - Rate limited: 60 requests/minute per token
   - File upload limited: 10MB per file, 50MB total
   - Session tracking for audit purposes

4. internal/handler/questionnaire_handler.go — Internal API Endpoints:
   - GET /questionnaires — list questionnaires
   - POST /questionnaires — create questionnaire
   - GET /questionnaires/{id} — questionnaire with sections and questions
   - PUT /questionnaires/{id} — update questionnaire
   - POST /questionnaires/{id}/clone — clone for customisation
   
   - GET /vendor-assessments — list assessments (filterable)
   - POST /vendor-assessments — create and send assessment
   - GET /vendor-assessments/{id} — assessment detail with responses
   - POST /vendor-assessments/{id}/review — submit review
   - POST /vendor-assessments/{id}/reminder — send reminder
   - GET /vendor-assessments/compare — side-by-side vendor comparison
   - GET /vendor-assessments/dashboard — TPRM dashboard

5. SEED — System Questionnaire Templates:

   a. "ComplianceForge Standard Security Assessment" (comprehensive, ~80 questions):
      Sections: Governance & Policies (10q), Access Control (10q), Data Protection (12q), Network Security (8q), Incident Response (8q), Business Continuity (6q), Vulnerability Management (8q), Change Management (6q), Physical Security (4q), Human Resources (4q), Compliance (4q)
      
      Example questions:
      - "Do you have a documented information security policy?" (yes_no, weight: 1.0, risk: high)
      - "How frequently is the security policy reviewed?" (single_choice: annually/biennially/ad_hoc/never, weight: 0.8)
      - "Is multi-factor authentication enforced for all administrative access?" (yes_no_na, weight: 1.5, risk: critical)
      - "Do you conduct annual penetration testing by an independent third party?" (yes_no, evidence_required: true, weight: 1.2, risk: high)
      - "What is your average time to patch critical vulnerabilities?" (single_choice: <24h/<7d/<30d/<90d/>90d, weight: 1.0, risk: critical)
      - "Do you have a documented incident response plan?" (yes_no, evidence_required: true, weight: 1.0, risk: high)
      - "Have you experienced a data breach in the last 24 months?" (yes_no, weight: 0.5, risk: critical)
      
   b. "GDPR Article 28 Processor Assessment" (~40 questions):
      Focused on data protection: lawful basis, data minimisation, retention, transfers, DPIA, DPO, breach notification, sub-processor management
      
   c. "NIS2 Supply Chain Security Assessment" (~30 questions):
      Focused on: cyber hygiene, incident reporting capability, business continuity, access management, encryption, vulnerability handling
      
   d. "Quick Security Assessment" (~20 questions):
      Abbreviated version for low-risk vendors: essential controls only

6. NEXT.JS FRONTEND:
   - /vendor-assessments — TPRM dashboard:
     * Summary cards: sent, in-progress, submitted, completed, overdue
     * Score distribution chart across all vendors
     * Critical findings alert panel
     * Assessment list table with vendor name, questionnaire, status, score, due date
   - Assessment detail page:
     * Response review: accordion by section, each question with vendor's answer and reviewer controls
     * Score breakdown by section (bar chart)
     * "Flag" / "Accept" / "Request Evidence" buttons per answer
     * Follow-up item generator
   - Vendor comparison page:
     * Select 2-4 vendors → side-by-side section scores
     * Radar chart comparing vendors
     * Strengths/weaknesses highlights
   - Questionnaire builder:
     * Drag-and-drop sections and questions
     * Question type selector with preview
     * Weight and risk impact configuration
     * Conditional logic builder (show question if...)
     * Preview mode (see it as the vendor would)
   - Vendor portal (separate layout, no sidebar — public-facing):
     * Clean, professional design — this is the vendor's experience of your brand
     * Progress indicator (Section 3 of 8 — 42% complete)
     * Auto-save every 30 seconds
     * Evidence upload per question
     * "Save & Continue Later" + "Submit" buttons
     * Mobile-responsive (vendor might fill this on a tablet)

CRITICAL REQUIREMENTS:
- Vendor portal is UNAUTHENTICATED — accessed via unique token only
- Vendor responses are stored within the ORG's tenant (not the vendor's)
- Assessment tokens use SHA-256 hashing — plaintext token shown to vendor once via email, never stored
- Scoring is deterministic: given the same responses, the same score is always calculated
- Evidence files uploaded by vendors stored in the org's storage (local/S3), not shared across orgs
- Questionnaire versioning: if a questionnaire is updated, in-progress assessments continue with the old version
- GDPR considerations: vendor contact data (name, email) in the assessment is personal data — handle accordingly
- Email delivery: assessment invitation + reminders use the notification engine (Prompt 11)
- Follow-up items integrate with the risk treatment module: critical vendor findings → risk treatments
- Automated re-assessment scheduling: after completion, schedule next assessment based on vendor risk tier

OUTPUT: Complete Golang code for questionnaire service, vendor assessment service, vendor portal handlers, internal handlers, migration, 4 seed questionnaire templates with full question content, and Next.js TPRM pages + vendor portal. Include unit tests for score calculation and token validation.
```

---

### PROMPT 29 OF 100 — Data Classification, Data Mapping & ROPA (Records of Processing Activities)

```
You are a senior Golang backend engineer building the data classification and ROPA module for "ComplianceForge" — a GRC platform. This module addresses GDPR Article 30 (Records of Processing Activities), UK GDPR equivalent, ISO 27001 A.5.12 (Classification of information), A.5.13 (Labelling of information), and A.8.11 (Data masking).

OBJECTIVE:
Build a complete data governance module that: classifies all organisational data assets by sensitivity, maps personal data flows across the organisation, generates and maintains the legally-required ROPA, tracks data processing activities, manages data retention policies, identifies cross-border transfers, and integrates with the vendor module for processor data mapping. The ROPA is the single most requested document by data protection authorities during inspections — getting this right is critical.

DATABASE SCHEMA — Create migration 024:

TABLE data_classifications:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(100) NOT NULL — 'Public', 'Internal', 'Confidential', 'Restricted', 'Top Secret'
  - level INT NOT NULL — 0, 1, 2, 3, 4 (higher = more sensitive)
  - description TEXT NOT NULL
  - handling_requirements TEXT — how data at this level must be handled
  - encryption_required BOOLEAN DEFAULT false
  - access_restriction_required BOOLEAN DEFAULT false
  - data_masking_required BOOLEAN DEFAULT false
  - retention_policy TEXT
  - disposal_method TEXT — 'standard_delete', 'secure_wipe', 'physical_destruction'
  - color_hex VARCHAR(7) — for UI badges
  - is_system BOOLEAN DEFAULT false
  - sort_order INT
  - created_at, updated_at

TABLE data_categories:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL — 'Name', 'Email Address', 'Financial Data', 'Health Data', etc.
  - category_type ENUM('personal_data', 'special_category', 'financial', 'technical', 'business', 'public', 'proprietary')
  - gdpr_special_category BOOLEAN DEFAULT false — Article 9 special categories
  - gdpr_article_9_basis TEXT — legal basis for processing special category data
  - description TEXT
  - examples TEXT[] — e.g., ['first name', 'last name', 'maiden name']
  - classification_id UUID FK → data_classifications — default classification
  - retention_period_months INT — default retention period
  - is_system BOOLEAN DEFAULT false
  - created_at, updated_at

TABLE processing_activities:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - activity_ref VARCHAR(20) NOT NULL — PA-001
  - name VARCHAR(500) NOT NULL — e.g., 'Employee Payroll Processing'
  - description TEXT NOT NULL — detailed description of the processing
  - purpose TEXT NOT NULL — GDPR Article 30(1)(b): purposes of the processing
  - legal_basis ENUM('consent', 'contract', 'legal_obligation', 'vital_interests', 'public_task', 'legitimate_interests')
  - legal_basis_detail TEXT — specific legal provision or legitimate interest description
  - status ENUM('active', 'under_review', 'suspended', 'retired')
  
  -- Controller/Processor Info (GDPR Art 30)
  - role ENUM('controller', 'joint_controller', 'processor')
  - joint_controller_details TEXT — if joint controller, who and what arrangement
  
  -- Data Subjects
  - data_subject_categories TEXT[] NOT NULL — e.g., ['employees', 'customers', 'job_applicants', 'website_visitors']
  - estimated_data_subjects_count INT
  
  -- Data Categories
  - data_category_ids UUID[] FK → data_categories
  - special_categories_processed BOOLEAN DEFAULT false
  - special_categories_legal_basis TEXT
  
  -- Recipients
  - recipient_categories TEXT[] — e.g., ['HR department', 'payroll provider', 'tax authority']
  - recipient_vendor_ids UUID[] FK → vendors — which vendors receive this data
  
  -- International Transfers (GDPR Art 30(1)(e))
  - involves_international_transfer BOOLEAN DEFAULT false
  - transfer_countries TEXT[] — e.g., ['US', 'IN']
  - transfer_safeguards ENUM('adequacy_decision', 'sccs', 'bcrs', 'derogation', 'other')
  - transfer_safeguards_detail TEXT
  - tia_conducted BOOLEAN DEFAULT false — Transfer Impact Assessment
  - tia_date DATE
  - tia_document_path TEXT
  
  -- Retention (GDPR Art 30(1)(f))
  - retention_period_months INT
  - retention_justification TEXT
  - deletion_method TEXT
  - deletion_responsible_user_id UUID FK → users
  
  -- Systems & Assets
  - system_ids UUID[] FK → assets — which systems process this data
  - storage_locations TEXT[] — where the data is stored
  
  -- DPIA
  - dpia_required BOOLEAN DEFAULT false — High-risk processing requires DPIA per Art 35
  - dpia_status ENUM('not_required', 'pending', 'in_progress', 'completed', 'review_due')
  - dpia_document_path TEXT
  - dpia_conducted_date DATE
  
  -- Risk & Controls
  - security_measures TEXT — technical and organisational measures (Art 30(1)(g))
  - linked_control_codes TEXT[] — linked ISO 27001 / NIST controls
  - risk_level ENUM('high', 'medium', 'low')
  
  -- Ownership
  - data_steward_user_id UUID FK → users
  - department VARCHAR(200)
  - process_owner_user_id UUID FK → users
  - last_review_date DATE
  - next_review_date DATE
  - review_frequency_months INT DEFAULT 12
  
  - metadata JSONB
  - created_at, updated_at, deleted_at

TABLE data_flow_maps:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - processing_activity_id UUID FK → processing_activities
  - name VARCHAR(300)
  - flow_type ENUM('collection', 'storage', 'processing', 'sharing', 'transfer', 'deletion')
  - source_type ENUM('data_subject', 'internal_system', 'vendor', 'partner', 'public_source')
  - source_name VARCHAR(300)
  - source_entity_id UUID — FK to asset, vendor, etc.
  - destination_type ENUM('internal_system', 'vendor', 'partner', 'regulator', 'data_subject', 'archive', 'deletion')
  - destination_name VARCHAR(300)
  - destination_entity_id UUID
  - destination_country VARCHAR(5)
  - data_category_ids UUID[] FK → data_categories
  - transfer_method VARCHAR(200) — 'API', 'SFTP', 'email', 'manual', 'database_replication'
  - encryption_in_transit BOOLEAN DEFAULT false
  - encryption_at_rest BOOLEAN DEFAULT false
  - volume_description VARCHAR(200) — 'high', 'medium', 'low' or specific counts
  - frequency VARCHAR(100) — 'real_time', 'daily_batch', 'weekly', 'on_demand'
  - legal_basis VARCHAR(100) — inherited from processing activity or specific
  - notes TEXT
  - sort_order INT
  - created_at, updated_at

TABLE ropa_exports:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - export_ref VARCHAR(20) — ROPA-2026-Q1
  - export_date TIMESTAMPTZ
  - format ENUM('pdf', 'xlsx', 'csv', 'json')
  - file_path TEXT
  - activities_included INT
  - exported_by UUID FK → users
  - export_reason VARCHAR(200) — 'regulatory_request', 'periodic_review', 'dpa_inspection', 'internal_audit'
  - notes TEXT
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/data_classification_service.go:
   - ManageClassifications(ctx, orgID) — CRUD for classification levels
   - ManageDataCategories(ctx, orgID) — CRUD for personal data categories
   - SuggestClassification(ctx, dataDescription) → AI-assisted classification suggestion

2. internal/service/ropa_service.go:
   - CreateProcessingActivity(ctx, orgID, req) → with auto-ref, validate legal basis, flag DPIA if high risk
   - UpdateProcessingActivity(ctx, orgID, activityID, req)
   - MapDataFlow(ctx, orgID, activityID, flow) → add a data flow step to the activity
   - GenerateROPA(ctx, orgID, format) → export the complete ROPA document:
     * PDF: formatted table per Article 30 requirements
     * XLSX: one row per processing activity, all fields as columns, with data validation
     * Must include ALL Article 30(1) required fields for controllers:
       a. Name and contact details of the controller (and DPO)
       b. Purposes of the processing
       c. Categories of data subjects and personal data
       d. Categories of recipients
       e. Transfers to third countries and safeguards
       f. Retention periods
       g. Description of technical and organisational security measures
   - GetROPADashboard(ctx, orgID) → metrics:
     * Total processing activities
     * Activities by legal basis (pie chart)
     * Special category processing count
     * International transfers count and destinations
     * DPIA status breakdown
     * Activities overdue for review
     * Data categories heat map (which categories are most processed)
   - IdentifyHighRiskProcessing(ctx, orgID) → flag activities that require DPIA:
     * Large-scale processing of special categories (Art 9)
     * Systematic monitoring of public areas
     * Automated decision-making with legal effects (Art 22)
     * Large-scale profiling
     * Processing of vulnerable data subjects (children, employees)
     * Innovative use of new technologies
   - DataSubjectImpactMap(ctx, orgID, subjectCategory) → for a given subject type (e.g., 'customers'):
     * What data is collected
     * Where it flows
     * Who has access
     * How long it's retained
     * What rights mechanisms exist
     * This powers DSAR responses (Prompt 13)

3. internal/handler/ropa_handler.go — API Endpoints:
   - GET /data/classifications — list data classification levels
   - POST /data/classifications — create level
   - GET /data/categories — list data categories
   - POST /data/categories — create category
   
   - GET /data/processing-activities — list processing activities (ROPA view)
   - POST /data/processing-activities — create processing activity
   - GET /data/processing-activities/{id} — activity detail with data flows
   - PUT /data/processing-activities/{id} — update activity
   - POST /data/processing-activities/{id}/flows — add data flow
   - GET /data/processing-activities/{id}/flow-diagram — visual flow data
   
   - POST /data/ropa/export — generate ROPA export (PDF/XLSX)
   - GET /data/ropa/exports — list past exports
   - GET /data/ropa/exports/{id}/download — download export
   - GET /data/ropa/dashboard — ROPA metrics dashboard
   
   - GET /data/high-risk — identify high-risk processing activities
   - GET /data/subject-map/{category} — data subject impact map
   - GET /data/transfers — international transfer register

4. SEED — Default Data Categories (30+):
   Personal data: Name, Email Address, Phone Number, Postal Address, Date of Birth, National ID Number, Passport Number, IP Address, Cookie Data, Location Data, Biometric Data, Photograph
   Special categories (Art 9): Health Data, Genetic Data, Racial/Ethnic Origin, Political Opinions, Religious Beliefs, Trade Union Membership, Sexual Orientation, Criminal Records
   Financial: Bank Account Details, Credit Card Number, Salary Information, Credit Score, Tax Records
   Employment: Employment History, Performance Reviews, Disciplinary Records, Training Records

   Default Classification Levels:
   - Public (0): freely shareable, no restrictions
   - Internal (1): org use only, standard access controls
   - Confidential (2): restricted access, encryption required
   - Restricted (3): need-to-know, strong encryption, audit logging
   - Top Secret (4): highest sensitivity, additional physical controls

5. NEXT.JS FRONTEND:
   - /data — Data Governance Hub:
     * ROPA Dashboard: total activities, by legal basis, special categories flag, transfers map, DPIA status, overdue reviews
     * Processing Activity list: ref, name, purpose, legal basis badge, data subjects, special category flag, transfers flag, DPIA status, review status
   - Processing Activity Detail:
     * Form with all Article 30 fields
     * Data flow diagram: visual flowchart showing data path from collection → storage → processing → sharing → deletion
     * Connected vendors (from vendor module)
     * Connected assets (from asset module)
     * DPIA section with trigger assessment
   - Data Flow Diagram builder:
     * Drag-and-drop nodes: Data Subject, Internal System, Vendor, Partner, Regulator
     * Connect nodes with flow arrows
     * Annotate each flow: data categories, transfer method, encryption, legal basis
     * Export as SVG/PNG for documentation
   - International Transfer Map:
     * World map showing data transfer destinations
     * Each destination: country, adequacy status, safeguard mechanism
     * Alert for transfers to countries without adequacy (require SCCs/BCRs)
   - ROPA Export page:
     * "Generate ROPA" button → format selection → download
     * Export history with download links
   - Data Subject Rights integration:
     * "View as Data Subject" mode: select a subject category, see all processing that affects them
     * This data feeds DSR responses (Prompt 13)

CRITICAL REQUIREMENTS:
- ROPA is a LEGAL REQUIREMENT under GDPR Article 30 — the export MUST include all legally required fields
- ROPA export must be in a format acceptable to supervisory authorities (PDF with official formatting, or Excel)
- International transfers must flag adequacy status per current EU adequacy decisions
- DPIA trigger assessment must follow EDPB Guidelines (WP248 rev.01) criteria
- Data flow maps must show encryption status at each step (visual indicator)
- Processing activities link to vendors (Prompt 8): when a vendor is a processor, the data mapping flows through them
- Processing activities link to DSR module (Prompt 13): when a DSAR is received, show all processing involving that subject category
- Processing activities link to assets (Prompt 8): when an asset processes personal data, it appears here
- Data categories seed includes ALL categories commonly requested by EU supervisory authorities
- Classification levels are configurable per org (some orgs use 3 levels, some use 5)
- Review cycle: processing activities must be reviewed at least annually (notification engine triggers)
- Deletion: when retention period expires, system flags the activity for data deletion confirmation

OUTPUT: Complete Golang code for data classification service, ROPA service, handlers, migration, seed data (30+ categories, 5 classifications), and Next.js data governance pages with data flow diagram builder. Include unit tests for DPIA trigger assessment and ROPA completeness validation.
```

---

### PROMPT 30 OF 100 — Executive Board Reporting Portal & Governance Dashboards

```
You are a senior full-stack engineer building the executive board reporting portal for "ComplianceForge" — a GRC platform. This module serves the board of directors, C-suite executives, and governance committees who need compliance visibility without the operational detail.

OBJECTIVE:
Build a separate, simplified executive portal that presents board-level governance dashboards, compliance posture summaries, risk appetite monitoring, regulatory compliance status, and key decisions requiring board attention. The portal must support board-specific access control (read-only, time-limited), branded PDF board packs, and a "board decisions" tracker for governance accountability per ISO 27001 Clause 5 (Leadership) and NIS2 Article 20 (Governance).

DATABASE SCHEMA — Create migration 025:

TABLE board_members:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - user_id UUID FK → users (NULL if external board member without platform account)
  - name VARCHAR(200) NOT NULL
  - title VARCHAR(200) — 'Non-Executive Director', 'Chair of Audit Committee', 'CISO'
  - email VARCHAR(300) NOT NULL
  - member_type ENUM('executive_director', 'non_executive_director', 'independent_director', 'committee_chair', 'observer', 'secretary')
  - committees TEXT[] — ['audit_committee', 'risk_committee', 'infosec_committee', 'data_protection_committee']
  - is_active BOOLEAN DEFAULT true
  - portal_access_enabled BOOLEAN DEFAULT false
  - portal_access_token_hash VARCHAR(128) — for external board members without accounts
  - portal_access_expires_at TIMESTAMPTZ
  - last_portal_access_at TIMESTAMPTZ
  - created_at, updated_at

TABLE board_meetings:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - meeting_ref VARCHAR(20) — BM-2026-Q1
  - title VARCHAR(300) NOT NULL
  - meeting_type ENUM('scheduled_board', 'extraordinary', 'committee_audit', 'committee_risk', 'committee_infosec', 'committee_data_protection', 'agm')
  - date DATE NOT NULL
  - time TIME
  - location VARCHAR(300)
  - status ENUM('planned', 'agenda_set', 'in_progress', 'completed', 'minutes_approved')
  - agenda_items JSONB — [{order, title, presenter, duration_minutes, type: 'information'|'discussion'|'decision'}]
  - board_pack_document_path TEXT — the compiled board pack PDF
  - board_pack_generated_at TIMESTAMPTZ
  - minutes_document_path TEXT
  - minutes_approved_at TIMESTAMPTZ
  - minutes_approved_by UUID FK → users
  - attendees UUID[] FK → board_members
  - apologies UUID[] FK → board_members
  - created_at, updated_at

TABLE board_decisions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - meeting_id UUID FK → board_meetings
  - decision_ref VARCHAR(20) — BD-2026-001
  - title VARCHAR(500) NOT NULL
  - description TEXT NOT NULL
  - decision_type ENUM('risk_acceptance', 'policy_approval', 'budget_approval', 'strategy_direction', 'compliance_action', 'incident_response', 'exception_approval', 'audit_response', 'regulatory_response', 'other')
  - decision ENUM('approved', 'rejected', 'deferred', 'conditional_approval')
  - conditions TEXT — conditions if conditional approval
  - vote_for INT
  - vote_against INT
  - vote_abstain INT
  - rationale TEXT
  
  -- Linkage
  - linked_entity_type VARCHAR(50) — 'risk', 'policy', 'exception', 'incident', 'remediation_plan'
  - linked_entity_id UUID
  
  -- Follow-up
  - action_required BOOLEAN DEFAULT false
  - action_description TEXT
  - action_owner_user_id UUID FK → users
  - action_due_date DATE
  - action_status ENUM('not_started', 'in_progress', 'completed', 'overdue')
  - action_completed_at TIMESTAMPTZ
  
  - decided_at TIMESTAMPTZ
  - decided_by VARCHAR(200) — meeting attendees who decided
  - tags TEXT[]
  - created_at, updated_at

TABLE board_reports:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - meeting_id UUID FK → board_meetings (NULL for ad-hoc reports)
  - report_type ENUM('compliance_summary', 'risk_appetite_status', 'incident_summary', 'vendor_risk_summary', 'nis2_governance', 'gdpr_status', 'board_pack', 'quarterly_review', 'annual_report')
  - title VARCHAR(300)
  - period_start DATE
  - period_end DATE
  - file_path TEXT
  - file_format ENUM('pdf', 'xlsx')
  - generated_by UUID FK → users
  - generated_at TIMESTAMPTZ
  - classification VARCHAR(50) DEFAULT 'board_confidential'
  - page_count INT
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/board_service.go:

   - ManageBoardMembers(ctx, orgID) — CRUD for board members
   - ManageMeetings(ctx, orgID) — CRUD for board meetings with agenda management
   - RecordDecision(ctx, orgID, meetingID, decision) → record board decision:
     * Link to entity (risk, policy, exception, etc.)
     * Create follow-up action if needed
     * Update linked entity status (e.g., risk → 'accepted', policy → 'approved')
     * Trigger the relevant approval workflow completion
   
   - GenerateBoardPack(ctx, orgID, meetingID) → compile a board-ready PDF:
     This is the premium deliverable — a single PDF containing:
     
     a. Cover page: org logo, meeting date, classification "BOARD CONFIDENTIAL"
     b. Agenda
     c. Executive Compliance Summary (2 pages):
        - Overall compliance score with trend arrow
        - Framework scores bar chart
        - Top 3 compliance achievements this period
        - Top 3 compliance risks/concerns
        - Comparison to industry benchmark percentile
     d. Risk Appetite Dashboard (2 pages):
        - Risk heatmap (5×5)
        - Risks exceeding appetite threshold (red zone)
        - New risks registered this period
        - Risk treatment completion rate
        - Financial exposure summary
     e. Incident & Breach Report (1-2 pages):
        - Incident count by severity
        - Data breaches (if any): timeline, subjects affected, DPA notification status
        - NIS2 reportable incidents
        - Mean resolution time trend
     f. Regulatory Update (1 page):
        - New regulatory changes affecting the org
        - Upcoming compliance deadlines
        - Regulatory enforcement actions in the industry
     g. Key Decisions Required (1 page):
        - Items requiring board approval or direction
        - Recommended actions with rationale
     h. Appendix: detailed metrics tables
   
   - GenerateNIS2GovernanceReport(ctx, orgID) → NIS2 Article 20 compliance:
     * Board members' cybersecurity training status
     * Risk management measures approved by management body
     * Evidence of board oversight of cybersecurity
   
   - GetBoardDashboard(ctx, orgID) → executive portal home page data:
     * Compliance score gauge (0–100)
     * Risk appetite status (within/approaching/exceeding)
     * Open incidents requiring board attention
     * Upcoming decisions/actions due
     * Regulatory horizon (upcoming changes)
     * Peer benchmark position

2. internal/handler/board_handler.go — API Endpoints:
   
   Internal (authenticated):
   - GET /board/members — list board members
   - POST /board/members — add board member
   - PUT /board/members/{id} — update member
   - GET /board/meetings — list meetings
   - POST /board/meetings — create meeting
   - PUT /board/meetings/{id} — update meeting
   - POST /board/meetings/{id}/generate-pack — generate board pack PDF
   - GET /board/meetings/{id}/download-pack — download board pack
   - POST /board/decisions — record decision
   - GET /board/decisions — list decisions (with action tracking)
   - PUT /board/decisions/{id}/action — update decision follow-up status
   - GET /board/reports — list reports
   - POST /board/reports/generate — generate ad-hoc report
   - GET /board/dashboard — executive dashboard data
   - GET /board/nis2-governance — NIS2 governance report
   
   Board Portal (limited auth, for external board members):
   - GET /board-portal/{token} — validate access, return dashboard data
   - GET /board-portal/{token}/meetings — past and upcoming meetings
   - GET /board-portal/{token}/meetings/{id}/pack — download board pack
   - GET /board-portal/{token}/decisions — decisions and follow-up status

3. NEXT.JS FRONTEND:

   - /board — Board Management (internal):
     * Board members list with committee assignments
     * Meeting calendar and management
     * Decision tracker with action follow-up
     * "Generate Board Pack" wizard:
       Step 1: Select meeting
       Step 2: Select period (default: since last meeting)
       Step 3: Select sections to include
       Step 4: Preview (show what each section will contain)
       Step 5: Generate → download PDF
   
   - /board/portal — Executive Board Portal (simplified UI):
     * SEPARATE LAYOUT: no sidebar, minimal navigation, clean executive aesthetic
     * Landing page: compliance gauge, risk appetite gauge, key alerts, decisions pending
     * "Board Packs" section: past meeting packs with download
     * "Decisions" section: decisions made, with follow-up status tracking
     * Read-only: no editing capabilities
     * Session timeout: 30 minutes (board-level security)
   
   - Board pack PDF must look PREMIUM:
     * Professional typography and layout
     * Charts rendered as high-resolution images embedded in PDF
     * Consistent colour scheme matching org branding (if configured)
     * Page numbers, headers, footers with classification
     * Table of contents with page references
     * This document goes to the board of directors — it must look as good as McKinsey output

CRITICAL REQUIREMENTS:
- Board portal access is TIME-LIMITED: tokens expire after the configured period
- Board packs are classified "BOARD CONFIDENTIAL" by default
- Decision tracking creates audit evidence for ISO 27001 Clause 5 (Leadership) and NIS2 Article 20
- External board members access via unique token (similar to vendor portal) — no full platform account needed
- Board pack generation must complete within 30 seconds (pre-cache heavy calculations)
- The executive dashboard shows ONLY high-level metrics — no operational detail that would overwhelm board members
- NIS2 governance report must demonstrate management body's active involvement in cybersecurity
- Meeting minutes are immutable once approved — append-only corrections
- All board portal access logged in audit trail
- Board pack includes a "Compared to Industry" section using anonymised benchmarks (from Prompt 25)

OUTPUT: Complete Golang code for board service, board pack generator, handlers, migration, and Next.js board management pages + executive portal. Include the complete board pack PDF template design (page-by-page specification).
```

---

## BATCH 6 SUMMARY

| Prompt | Focus Area | New Tables | New Endpoints | Key Capabilities |
|--------|-----------|------------|---------------|------------------|
| 26 | Exception Management | 3 (exceptions, reviews, audit trail) | ~13 | Full exception lifecycle (request→assess→approve→monitor→expire→revoke), compensating controls, compliance score impact calculation, workflow integration, expiry scheduler, auditor register view |
| 27 | Evidence Templates | 5 (templates, requirements, test suites, test cases, test runs) | ~15 | 200+ pre-built evidence templates for ISO 27001/PCI DSS/NIST, automated evidence validation rules, pre-audit readiness checker, evidence collection scheduling, test suite runner |
| 28 | TPRM Questionnaires | 6 (questionnaires, sections, questions, assessments, responses, portal sessions) | ~18 | Questionnaire builder, vendor self-service portal, automated scoring, vendor comparison, 4 seed templates (80/40/30/20 questions), GDPR Art.28 and NIS2 supply chain assessments |
| 29 | Data Classification & ROPA | 5 (classifications, categories, processing activities, data flows, ropa exports) | ~16 | Data classification levels, 30+ personal data categories, ROPA per GDPR Art.30, data flow mapping, international transfer tracking, DPIA trigger assessment, data subject impact map |
| 30 | Board Reporting Portal | 4 (board members, meetings, decisions, reports) | ~16 | Executive portal with simplified UI, board pack PDF generator, decision tracker, NIS2 governance reporting, time-limited external board member access, meeting management |

**Running Total: 30/100 Prompts | ~120 Tables | ~325+ API Endpoints | Complete GRC + Governance Platform**

---

> **NEXT BATCH (Prompts 31–35):** Compliance Calendar & Deadline Management, Advanced Search & Knowledge Base, Collaboration & Comments System, Mobile App API & Push Notifications, and Tenant White-Labelling & Custom Branding.
>
> Type **"next"** to continue.
