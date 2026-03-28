# GRC Compliance Management Solution — 100 Master Prompts

## BATCH 3 — Notification Engine, Advanced Reporting, GDPR DSR, NIS2 Automation & Continuous Monitoring

**Stack:** Golang 1.22+ | PostgreSQL 16 | Redis 7 | Next.js 14 | WebSockets
**Prerequisite:** Backend (Prompts 1–5), Frontend (Prompts 6–9), CI/CD (Prompt 10) all completed
**Deliverable:** Enterprise notification system, PDF/XLSX reporting, GDPR data subject rights automation, NIS2 incident reporting, and scheduled evidence collection

---

### PROMPT 11 OF 100 — Enterprise Notification Engine (Email + In-App + Webhooks + Slack)

```
You are a senior Golang backend engineer building the enterprise notification engine for "ComplianceForge" — a GRC platform. The core platform, database schema, handlers, and job queue are built (Prompts 1–5).

OBJECTIVE:
Build a complete multi-channel notification engine that sends context-aware, compliance-driven notifications via email, in-app real-time, webhooks, and Slack. The engine must be event-driven, configurable per user and per organisation, and must support regulatory notification deadlines (GDPR 72h, NIS2 24h).

ARCHITECTURE:
The notification engine follows an event → rule evaluation → dispatch pipeline:
1. Business events emitted from handlers/services (e.g., incident.created, control.status_changed)
2. Notification rules engine evaluates which notifications to send based on event type, severity, and user preferences
3. Dispatch layer sends via configured channels (email, in-app, webhook, Slack)
4. Delivery tracking with retry for failed deliveries

DATABASE SCHEMA — Create migration 007:

TABLE notification_channels:
  - id UUID PK
  - organization_id UUID FK → organizations (RLS)
  - channel_type ENUM('email', 'in_app', 'webhook', 'slack', 'teams')
  - name VARCHAR(200) NOT NULL
  - configuration JSONB NOT NULL — stores channel-specific config:
    * email: {smtp_host, smtp_port, from_address, from_name}
    * webhook: {url, secret, headers, retry_count}
    * slack: {webhook_url, channel, bot_token}
    * teams: {webhook_url}
  - is_active BOOLEAN DEFAULT true
  - is_default BOOLEAN DEFAULT false
  - created_at, updated_at, deleted_at

TABLE notification_rules:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL
  - event_type VARCHAR(100) NOT NULL — e.g., 'incident.created', 'control.status_changed', 'breach.deadline_approaching', 'policy.review_due', 'finding.overdue', 'vendor.assessment_due', 'risk.threshold_exceeded'
  - severity_filter TEXT[] — e.g., {'critical', 'high'} or NULL for all
  - conditions JSONB — additional filter conditions (e.g., {"is_data_breach": true})
  - channel_ids UUID[] — which channels to notify
  - recipient_type ENUM('role', 'user', 'owner', 'assignee', 'dpo', 'ciso', 'custom')
  - recipient_ids UUID[] — specific user/role IDs if recipient_type='custom'
  - template_id UUID FK → notification_templates
  - is_active BOOLEAN DEFAULT true
  - cooldown_minutes INT DEFAULT 0 — prevent notification spam
  - escalation_after_minutes INT — if unacknowledged, escalate
  - escalation_channel_ids UUID[]
  - created_at, updated_at

TABLE notification_templates:
  - id UUID PK
  - organization_id UUID FK (RLS, NULL for system templates)
  - name VARCHAR(200) NOT NULL
  - event_type VARCHAR(100) NOT NULL
  - subject_template TEXT — Go template syntax: "GDPR Breach Alert — {{.IncidentRef}}"
  - body_html_template TEXT — Full HTML template with Go template variables
  - body_text_template TEXT — Plain text fallback
  - in_app_title_template TEXT — Short title for in-app bell
  - in_app_body_template TEXT — In-app notification body
  - slack_template JSONB — Slack Block Kit JSON template
  - webhook_payload_template JSONB — Webhook payload template
  - variables TEXT[] — List of available template variables
  - is_system BOOLEAN DEFAULT false
  - created_at, updated_at

TABLE notifications:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - rule_id UUID FK → notification_rules
  - event_type VARCHAR(100) NOT NULL
  - event_payload JSONB NOT NULL — full event data
  - recipient_user_id UUID FK → users
  - channel_type notification_channel_type
  - channel_id UUID FK → notification_channels
  - subject TEXT
  - body TEXT
  - status ENUM('pending', 'sent', 'delivered', 'failed', 'bounced')
  - sent_at TIMESTAMPTZ
  - delivered_at TIMESTAMPTZ
  - read_at TIMESTAMPTZ — for in-app notifications
  - acknowledged_at TIMESTAMPTZ — user acknowledged (for escalation tracking)
  - error_message TEXT
  - retry_count INT DEFAULT 0
  - max_retries INT DEFAULT 3
  - next_retry_at TIMESTAMPTZ
  - metadata JSONB
  - created_at

TABLE notification_preferences:
  - id UUID PK
  - user_id UUID FK → users
  - organization_id UUID FK (RLS)
  - event_type VARCHAR(100) NOT NULL — '*' for global default
  - email_enabled BOOLEAN DEFAULT true
  - in_app_enabled BOOLEAN DEFAULT true
  - slack_enabled BOOLEAN DEFAULT false
  - digest_frequency ENUM('immediate', 'hourly', 'daily', 'weekly') DEFAULT 'immediate'
  - quiet_hours_start TIME — e.g., 22:00
  - quiet_hours_end TIME — e.g., 07:00
  - quiet_hours_timezone VARCHAR(50)
  - UNIQUE(user_id, event_type)

GOLANG IMPLEMENTATION:

1. internal/service/notification_engine.go:
   - EventBus: channel-based event publisher/subscriber
   - Event types: incident.created, incident.severity_changed, breach.detected, breach.deadline_approaching (12h, 6h, 1h), breach.deadline_expired, control.status_changed, control.maturity_changed, policy.review_due, policy.review_overdue, policy.published, policy.attestation_required, finding.created, finding.overdue, finding.escalated, risk.created, risk.threshold_exceeded, risk.review_due, vendor.assessment_due, vendor.assessment_overdue, vendor.dpa_missing, user.login_failed_threshold, compliance.score_dropped
   - RuleEvaluator: matches events against active rules, checks severity filters and conditions
   - TemplateRenderer: renders Go templates with event data
   - Dispatcher: sends notifications via the appropriate channel
   - DeliveryTracker: updates notification status, handles retries
   - EscalationManager: checks for unacknowledged critical notifications, escalates after configured timeout

2. internal/service/notification_channels/:
   - email_channel.go: SMTP email delivery with HTML templates
   - inapp_channel.go: stores in-app notification in database
   - webhook_channel.go: HTTP POST with HMAC-SHA256 signature, configurable headers, retry with backoff
   - slack_channel.go: Slack Webhook API with Block Kit formatting

3. internal/handler/notification_handler.go — API endpoints:
   - GET /notifications — user's in-app notifications (paginated, unread count)
   - PUT /notifications/{id}/read — mark as read
   - PUT /notifications/read-all — mark all as read
   - GET /notifications/unread-count — for the bell icon badge
   - GET /notifications/preferences — user's notification preferences
   - PUT /notifications/preferences — update preferences
   - GET /settings/notification-rules — org notification rules (admin)
   - POST /settings/notification-rules — create rule
   - PUT /settings/notification-rules/{id} — update rule
   - DELETE /settings/notification-rules/{id} — delete rule
   - GET /settings/notification-channels — org channels (admin)
   - POST /settings/notification-channels — create channel
   - POST /settings/notification-channels/{id}/test — send test notification

4. SYSTEM DEFAULT TEMPLATES (seed data):
   Create 15+ default notification templates for:
   - GDPR Breach 72h Alert (12h remaining, 6h remaining, 1h remaining, expired)
   - NIS2 24h Early Warning Alert
   - Incident Created (by severity)
   - Control Status Changed
   - Policy Review Due / Overdue
   - Attestation Required
   - Audit Finding Created (critical/high)
   - Finding Remediation Overdue
   - Vendor Assessment Due
   - Vendor Missing DPA
   - Risk Threshold Exceeded
   - Compliance Score Dropped Below Threshold
   - Welcome Email
   - Password Reset

5. REGULATORY NOTIFICATION SCHEDULER (internal/worker/regulatory_scheduler.go):
   - Runs every 15 minutes via the background worker
   - Checks for:
     * Data breaches approaching 72-hour GDPR deadline (emit events at 48h, 12h, 6h, 1h, 0h remaining)
     * NIS2 incidents approaching 24-hour early warning deadline
     * Policies overdue for review
     * Audit findings past remediation due date
     * Vendor assessments overdue
     * Risk reviews due
   - Each check queries the database and emits the appropriate event to the EventBus

6. NEXT.JS FRONTEND INTEGRATION:
   - Notification bell in topbar: fetches GET /notifications/unread-count every 30 seconds
   - Click bell → dropdown with last 10 notifications from GET /notifications?page_size=10
   - "Mark all read" button
   - Notification preferences page in user settings
   - Admin: Notification Rules configuration page under /settings/notifications

CRITICAL REQUIREMENTS:
- Every notification channel must be testable (POST /settings/notification-channels/{id}/test)
- Webhook notifications must include HMAC-SHA256 signature in X-CF-Signature header
- Email notifications must use responsive HTML templates (renders on mobile)
- In-app notifications must show unread count badge on the sidebar navigation
- Breach deadline notifications MUST NOT be suppressible by user preferences (regulatory requirement)
- All notification delivery must be through the Redis job queue (non-blocking)
- Notification templates support Go template syntax with all event data as variables
- Include rate limiting: max 100 notifications per user per hour (configurable)

OUTPUT: Complete Golang code for every file, SQL migration, seed data for templates, and Next.js notification components. Every file must compile. Include unit tests for the rule evaluator and template renderer.
```

---

### PROMPT 12 OF 100 — Advanced Reporting Engine (PDF, XLSX, Scheduled Reports)

```
You are a senior Golang backend engineer building the advanced reporting engine for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a comprehensive reporting system that generates professional PDF and XLSX compliance reports, supports scheduled report delivery, and provides a report builder for custom reports. Reports must be auditor-grade — suitable for presenting to regulators, boards, and external auditors.

DATABASE SCHEMA — Create migration 008:

TABLE report_definitions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL
  - description TEXT
  - report_type ENUM('compliance_status', 'risk_register', 'risk_heatmap', 'audit_summary', 'audit_findings', 'incident_summary', 'breach_register', 'vendor_risk', 'policy_status', 'attestation_report', 'gap_analysis', 'cross_framework_mapping', 'executive_summary', 'kri_dashboard', 'treatment_progress', 'custom')
  - format ENUM('pdf', 'xlsx', 'csv', 'json') DEFAULT 'pdf'
  - filters JSONB — e.g., {"framework_ids": [...], "date_range": {"from": "...", "to": "..."}, "risk_levels": ["critical", "high"]}
  - sections JSONB — ordered list of report sections to include
  - classification VARCHAR(50) DEFAULT 'internal' — document classification stamp
  - include_executive_summary BOOLEAN DEFAULT true
  - include_appendices BOOLEAN DEFAULT true
  - branding JSONB — {logo_url, header_color, company_name}
  - created_by UUID FK → users
  - is_template BOOLEAN DEFAULT false
  - created_at, updated_at

TABLE report_schedules:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - report_definition_id UUID FK → report_definitions
  - name VARCHAR(200)
  - frequency ENUM('daily', 'weekly', 'monthly', 'quarterly', 'annually')
  - day_of_week INT — 0=Sunday (for weekly)
  - day_of_month INT — 1-28 (for monthly)
  - time_of_day TIME DEFAULT '08:00'
  - timezone VARCHAR(50) DEFAULT 'Europe/London'
  - recipient_user_ids UUID[]
  - recipient_emails TEXT[] — external recipients
  - delivery_channel ENUM('email', 'storage', 'both') DEFAULT 'email'
  - is_active BOOLEAN DEFAULT true
  - last_run_at TIMESTAMPTZ
  - next_run_at TIMESTAMPTZ
  - created_at, updated_at

TABLE report_runs:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - report_definition_id UUID FK
  - schedule_id UUID FK (NULL for ad-hoc)
  - status ENUM('pending', 'generating', 'completed', 'failed')
  - format report_format
  - file_path TEXT — path in file storage
  - file_size_bytes BIGINT
  - page_count INT
  - generation_time_ms INT — how long it took
  - parameters JSONB — runtime parameters/filters used
  - generated_by UUID FK → users (NULL for scheduled)
  - error_message TEXT
  - created_at
  - completed_at

GOLANG IMPLEMENTATION:

1. internal/service/report_engine.go — Core report generation:

   ReportEngine struct with methods:
   - GenerateComplianceReport(ctx, orgID, filters) → generates comprehensive compliance status report
   - GenerateRiskReport(ctx, orgID, filters) → risk register with heatmap visualization
   - GenerateAuditReport(ctx, orgID, auditID) → audit findings report
   - GenerateIncidentReport(ctx, orgID, filters) → incident & breach register
   - GenerateVendorReport(ctx, orgID, filters) → vendor risk assessment report
   - GeneratePolicyReport(ctx, orgID, filters) → policy compliance & attestation report
   - GenerateExecutiveSummary(ctx, orgID) → board-level executive summary
   - GenerateGapAnalysis(ctx, orgID, frameworkID) → gap analysis with remediation roadmap
   - GenerateCrossFrameworkReport(ctx, orgID) → cross-mapping coverage analysis
   - GenerateCustomReport(ctx, orgID, definition) → custom report from definition

   Each generate method:
   a. Queries the database for all required data
   b. Builds a ReportData struct with all sections
   c. Passes to the appropriate renderer (PDF or XLSX)

2. internal/pkg/pdf/report_renderer.go — PDF generation using go-pdf or maroto v2:

   Professional PDF reports with:
   - Cover page: report title, organisation name, date, classification stamp, ComplianceForge logo
   - Table of contents with page numbers
   - Executive summary section with key metrics and traffic light indicators
   - Data tables with alternating row colors, column headers, proper pagination
   - Charts rendered as embedded images:
     * Compliance score bar charts
     * Risk heatmap (5×5 colored grid)
     * Donut charts for distributions
     * Trend line charts
   - Headers: report title + page number on every page
   - Footers: classification, generation date, "Generated by ComplianceForge"
   - Appendices: methodology notes, scoring definitions, framework reference
   - Watermark support (DRAFT, CONFIDENTIAL)
   - Page size: A4 (European standard)

3. internal/pkg/xlsx/report_renderer.go — Excel generation using excelize:

   Professional XLSX reports with:
   - Summary sheet with KPI cells, conditional formatting
   - Data sheets for each section with:
     * Frozen header row
     * Auto-filter on all columns
     * Column width auto-sizing
     * Conditional formatting (red/amber/green for scores, statuses)
     * Data validation dropdowns for status columns
   - Charts sheet with embedded Excel charts
   - Pivot table-ready data layout
   - Named ranges for key metrics
   - Print area and page setup configured

4. internal/handler/report_handler.go — API endpoints:

   - POST /reports/generate — ad-hoc report generation (returns job ID)
   - GET /reports/status/{id} — check generation status
   - GET /reports/download/{id} — download generated report file
   - GET /reports/definitions — list saved report definitions
   - POST /reports/definitions — save a report definition
   - PUT /reports/definitions/{id} — update definition
   - DELETE /reports/definitions/{id} — delete definition
   - GET /reports/schedules — list report schedules
   - POST /reports/schedules — create schedule
   - PUT /reports/schedules/{id} — update schedule
   - DELETE /reports/schedules/{id} — delete schedule
   - GET /reports/history — list past report runs (paginated)
   - POST /reports/definitions/{id}/generate — generate from saved definition

5. internal/worker/report_scheduler.go:
   - Runs every minute in the background worker
   - Checks report_schedules for any where next_run_at <= NOW() AND is_active = true
   - Enqueues a report generation job for each due schedule
   - Updates last_run_at and calculates next_run_at

6. REPORT TYPES — Detailed specifications:

   A. COMPLIANCE STATUS REPORT (PDF, 15-25 pages):
      - Page 1: Cover page
      - Page 2: Table of contents
      - Page 3-4: Executive summary with overall score, framework scores, top 5 gaps, key findings
      - Page 5-8: Framework-by-framework breakdown (score, controls implemented/total, maturity level, trend)
      - Page 9-12: Control implementation status tables per framework
      - Page 13-14: Maturity distribution analysis (bar chart + commentary)
      - Page 15-16: Gap analysis with prioritised remediation recommendations
      - Page 17-18: Cross-framework coverage summary
      - Appendix: Methodology, scoring criteria, framework descriptions

   B. RISK REGISTER REPORT (PDF, 10-20 pages):
      - Executive summary: total risks, distribution by level, average residual score, treatment completion rate
      - Risk heatmap (5×5 grid, full page)
      - Top 10 risks table with full details
      - Risk distribution by category (chart + table)
      - Treatment progress summary
      - KRI status dashboard
      - Trend analysis: risk scores over time

   C. EXECUTIVE SUMMARY (PDF, 3-5 pages):
      - One-page compliance dashboard (all KPIs)
      - Risk posture summary
      - Key incidents & breaches (last quarter)
      - Top 5 action items for the board
      - Regulatory compliance status (GDPR, NIS2, PCI DSS)
      - Designed for board presentation

7. NEXT.JS FRONTEND:
   - /reports page with:
     * Quick generate buttons for each standard report type
     * Report builder: select type, filters (frameworks, date range, risk levels), format (PDF/XLSX)
     * Saved definitions list with "Generate Now" buttons
     * Schedule management (CRUD table)
     * Report history with download links and generation time
     * Report preview (embedded PDF viewer for completed reports)
   - Generation status: polling GET /reports/status/{id} every 2 seconds until completed
   - Download: direct link to GET /reports/download/{id}

CRITICAL REQUIREMENTS:
- PDF reports must look professional enough to present to regulators and board members
- All reports include document classification stamping (header + footer)
- Reports include generation metadata: who generated, when, what filters applied
- XLSX reports must be ready for auditor use (sortable, filterable, with proper formatting)
- Large reports (>100 pages) must generate within 60 seconds
- Report files stored in the file storage backend (local/S3) with SHA-256 integrity hash
- Scheduled reports are generated in the background worker, not blocking API requests
- Report history retained for 90 days (configurable), then auto-deleted

OUTPUT: Complete Golang code for the report engine, PDF renderer, XLSX renderer, handlers, scheduler, migration, and Next.js report builder page. Include 3 example report outputs (describe the exact layout page by page).
```

---

### PROMPT 13 OF 100 — GDPR Data Subject Request (DSR) Management Module

```
You are a senior Golang backend engineer building the GDPR Data Subject Request management module for "ComplianceForge" — a GRC platform targeting European enterprises.

OBJECTIVE:
Build a complete system for managing Data Subject Access Requests (DSARs), Right to Erasure (Right to be Forgotten), Right to Rectification, Right to Portability, Right to Restriction, and Right to Object — per GDPR Articles 12–23 and UK GDPR equivalents. This module must track SLA compliance (30-day response deadline, extendable to 90 days for complex requests), route requests to data stewards, and generate response documentation.

REGULATORY CONTEXT:
- GDPR Article 12: Transparent information — respond within 1 month, extendable by 2 months
- GDPR Article 15: Right of access (DSAR) — provide copy of all personal data
- GDPR Article 16: Right to rectification — correct inaccurate data
- GDPR Article 17: Right to erasure — delete personal data (with exceptions)
- GDPR Article 18: Right to restriction — restrict processing
- GDPR Article 20: Right to data portability — provide data in machine-readable format
- GDPR Article 21: Right to object — stop processing for direct marketing, etc.
- UK GDPR mirrors these rights with ICO as supervisory authority

DATABASE SCHEMA — Create migration 009:

TABLE dsr_requests:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - request_ref VARCHAR(20) NOT NULL UNIQUE — e.g., DSR-2026-0001
  - request_type ENUM('access', 'erasure', 'rectification', 'portability', 'restriction', 'objection', 'automated_decision')
  - status ENUM('received', 'identity_verification', 'in_progress', 'extended', 'completed', 'rejected', 'withdrawn')
  - priority ENUM('standard', 'urgent', 'complex')
  
  -- Data Subject Info (encrypted at rest)
  - data_subject_name_encrypted TEXT NOT NULL
  - data_subject_email_encrypted TEXT NOT NULL
  - data_subject_phone_encrypted TEXT
  - data_subject_address_encrypted TEXT
  - data_subject_id_verified BOOLEAN DEFAULT false
  - identity_verification_method VARCHAR(100)
  - identity_verified_at TIMESTAMPTZ
  - identity_verified_by UUID FK → users
  
  -- Request Details
  - request_description TEXT NOT NULL
  - request_source ENUM('email', 'form', 'phone', 'letter', 'in_person', 'portal')
  - received_date DATE NOT NULL
  - acknowledged_at TIMESTAMPTZ
  - response_deadline DATE NOT NULL — received_date + 30 days
  - extended_deadline DATE — received_date + 90 days (if extended)
  - extension_reason TEXT
  - extension_notified_at TIMESTAMPTZ
  
  -- Processing
  - assigned_to UUID FK → users (DPO or data steward)
  - data_systems_affected TEXT[] — e.g., {'CRM', 'HR System', 'Marketing Platform'}
  - data_categories_affected TEXT[] — e.g., {'name', 'email', 'financial', 'health'}
  - third_parties_notified TEXT[] — processors/recipients notified per Art.19
  - processing_notes TEXT
  
  -- Completion
  - completed_at TIMESTAMPTZ
  - completed_by UUID FK → users
  - response_method ENUM('email', 'post', 'portal', 'in_person')
  - response_document_path TEXT — stored in file storage
  - rejection_reason TEXT
  - rejection_legal_basis TEXT — e.g., "Article 17(3)(b) — exercise of legal claims"
  
  -- Compliance Tracking
  - sla_status ENUM('on_track', 'at_risk', 'overdue') — computed
  - days_remaining INT — computed
  - was_extended BOOLEAN DEFAULT false
  - was_completed_on_time BOOLEAN
  
  - metadata JSONB
  - created_at, updated_at, deleted_at

TABLE dsr_tasks:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - dsr_request_id UUID FK → dsr_requests
  - task_type ENUM('verify_identity', 'locate_data', 'extract_data', 'review_data', 'compile_response', 'notify_processors', 'execute_erasure', 'confirm_erasure', 'send_response', 'notify_third_parties')
  - description TEXT NOT NULL
  - system_name VARCHAR(200) — which system this task relates to
  - assigned_to UUID FK → users
  - status ENUM('pending', 'in_progress', 'completed', 'blocked', 'not_applicable')
  - due_date DATE
  - completed_at TIMESTAMPTZ
  - completed_by UUID FK
  - notes TEXT
  - evidence_path TEXT — screenshot/proof of completion
  - sort_order INT
  - created_at, updated_at

TABLE dsr_audit_trail:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - dsr_request_id UUID FK → dsr_requests
  - action VARCHAR(100) NOT NULL — e.g., 'request_received', 'identity_verified', 'task_assigned', 'data_located', 'response_sent'
  - performed_by UUID FK → users
  - description TEXT
  - metadata JSONB
  - created_at — immutable

TABLE dsr_response_templates:
  - id UUID PK
  - organization_id UUID FK (RLS, NULL for system)
  - request_type dsr_request_type
  - name VARCHAR(200)
  - subject TEXT
  - body_html TEXT
  - body_text TEXT
  - is_system BOOLEAN DEFAULT false
  - language VARCHAR(10) DEFAULT 'en'
  - created_at, updated_at

GOLANG IMPLEMENTATION:

1. internal/service/dsr_service.go:
   - CreateRequest(ctx, orgID, req) — create DSR with auto-generated ref (DSR-YYYY-NNNN), auto-calculate deadline, create default task checklist based on request_type, emit 'dsr.received' notification event
   - VerifyIdentity(ctx, orgID, requestID, method, verifiedBy) — record ID verification
   - AssignRequest(ctx, orgID, requestID, assigneeID)
   - ExtendDeadline(ctx, orgID, requestID, reason) — extend by 60 days, record notification to data subject
   - CompleteTask(ctx, orgID, taskID, completedBy, notes, evidencePath)
   - CompleteRequest(ctx, orgID, requestID, responseMethod, documentPath)
   - RejectRequest(ctx, orgID, requestID, reason, legalBasis)
   - GetDSRDashboard(ctx, orgID) — summary metrics: total, by type, by status, overdue count, avg completion days
   - CheckSLACompliance(ctx, orgID) — returns all at-risk and overdue requests

   Auto-generated task checklists by request type:
   - ACCESS: verify_identity → locate_data → extract_data → review_data (redact third-party data) → compile_response → send_response
   - ERASURE: verify_identity → locate_data → review_exemptions → execute_erasure → confirm_erasure → notify_third_parties → send_confirmation
   - RECTIFICATION: verify_identity → locate_data → verify_correction → execute_correction → notify_third_parties → send_confirmation
   - PORTABILITY: verify_identity → locate_data → extract_in_machine_readable_format → review_data → send_response

2. internal/handler/dsr_handler.go — API endpoints:
   - GET /dsr — list all DSR requests (paginated, filterable by type, status)
   - GET /dsr/{id} — get request detail with tasks and audit trail
   - POST /dsr — create new DSR request
   - PUT /dsr/{id} — update request details
   - POST /dsr/{id}/verify-identity — record identity verification
   - POST /dsr/{id}/assign — assign to user
   - POST /dsr/{id}/extend — extend deadline
   - POST /dsr/{id}/complete — mark as completed
   - POST /dsr/{id}/reject — reject request
   - PUT /dsr/{id}/tasks/{taskId} — update task status
   - GET /dsr/dashboard — DSR metrics dashboard
   - GET /dsr/overdue — list overdue requests
   - GET /dsr/templates — response templates

3. internal/worker/dsr_scheduler.go:
   - Runs daily: calculate sla_status and days_remaining for all active DSRs
   - Emit notifications:
     * 'dsr.deadline_approaching' when 7 days remaining
     * 'dsr.deadline_approaching' when 3 days remaining
     * 'dsr.overdue' when deadline passed
   - Update computed fields

4. ENCRYPTION:
   - All data subject PII (name, email, phone, address) encrypted at rest using AES-256-GCM
   - Encryption key from config (different from JWT secret)
   - Decrypt only when displaying to authorised users
   - Log every access to data subject PII in dsr_audit_trail

5. NEXT.JS FRONTEND — /dsr page:
   - DSR Dashboard: total requests, by type (pie chart), by status, SLA compliance rate, avg completion days
   - Request list with DataTable: Ref, Type (badge), Subject Name (partially masked), Status (badge), Received Date, Deadline (red if overdue, amber if <7 days), Assigned To, Days Remaining
   - Create Request form: request_type (select), data_subject details (name, email, phone, address), description, source (select), received_date
   - Request Detail page:
     * Header with ref, type, status, deadline countdown
     * Task checklist (checkboxes with assignee and completion date)
     * Audit trail timeline
     * "Verify Identity", "Extend Deadline", "Complete Request", "Reject Request" action buttons
     * Response letter generator using template
   - SLA dashboard view showing all at-risk and overdue requests

OUTPUT: Complete Golang code (service, handler, worker, migration, seed templates), Next.js frontend pages, and unit tests for SLA calculation logic.
```

---

### PROMPT 14 OF 100 — NIS2 Compliance Automation Module

```
You are a senior Golang backend engineer building the NIS2 Directive compliance automation module for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a comprehensive module for managing compliance with the EU Network and Information Security Directive (NIS2), which came into force in October 2024. The module must automate incident reporting workflows (24h early warning, 72h notification, 1-month final report), entity categorisation (essential vs important), supply chain risk management per NIS2 requirements, and management body accountability tracking.

REGULATORY CONTEXT:
- NIS2 Article 20: Governance — management bodies must approve cybersecurity risk-management measures
- NIS2 Article 21: Cybersecurity risk-management measures — 10 minimum security measures
- NIS2 Article 23: Reporting obligations — 3-phase incident reporting:
  * Phase 1: Early warning within 24 hours of becoming aware
  * Phase 2: Incident notification within 72 hours with initial assessment
  * Phase 3: Final report within 1 month of notification
- NIS2 Article 24: Use of European cybersecurity certification schemes
- NIS2 Article 29: Cybersecurity information-sharing arrangements

DATABASE SCHEMA — Create migration 010:

TABLE nis2_entity_assessment:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - entity_type ENUM('essential', 'important', 'not_applicable')
  - sector VARCHAR(200) — e.g., 'energy', 'transport', 'banking', 'health', 'digital_infrastructure'
  - sub_sector VARCHAR(200)
  - assessment_criteria JSONB — answers to categorisation questionnaire
  - employee_count INT
  - annual_turnover_eur DECIMAL
  - assessment_date DATE
  - assessed_by UUID FK → users
  - is_in_scope BOOLEAN
  - member_state VARCHAR(5) — EU member state code
  - competent_authority VARCHAR(200) — e.g., 'BSI (Germany)', 'ANSSI (France)', 'NCSC (Netherlands)'
  - csirt_name VARCHAR(200) — designated CSIRT for reporting
  - csirt_contact_email VARCHAR(200)
  - csirt_reporting_url TEXT
  - notes TEXT
  - created_at, updated_at

TABLE nis2_incident_reports:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - incident_id UUID FK → incidents
  - report_ref VARCHAR(30) — NIS2-2026-0001
  
  -- Phase 1: Early Warning (24 hours)
  - early_warning_status ENUM('not_required', 'pending', 'submitted', 'overdue')
  - early_warning_deadline TIMESTAMPTZ
  - early_warning_submitted_at TIMESTAMPTZ
  - early_warning_submitted_by UUID FK → users
  - early_warning_content JSONB — {initial_assessment, suspected_cause, cross_border_impact}
  - early_warning_csirt_reference VARCHAR(100)
  
  -- Phase 2: Incident Notification (72 hours)
  - notification_status ENUM('not_required', 'pending', 'submitted', 'overdue')
  - notification_deadline TIMESTAMPTZ
  - notification_submitted_at TIMESTAMPTZ
  - notification_submitted_by UUID FK → users
  - notification_content JSONB — {severity_assessment, impact_assessment, ioc_list, affected_services, mitigation_measures}
  - notification_csirt_reference VARCHAR(100)
  
  -- Phase 3: Final Report (1 month)
  - final_report_status ENUM('not_required', 'pending', 'submitted', 'overdue')
  - final_report_deadline TIMESTAMPTZ
  - final_report_submitted_at TIMESTAMPTZ
  - final_report_submitted_by UUID FK → users
  - final_report_content JSONB — {detailed_description, root_cause_analysis, mitigation_applied, cross_border_impacts, lessons_learned}
  - final_report_document_path TEXT
  
  - created_at, updated_at

TABLE nis2_security_measures:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - measure_code VARCHAR(20) NOT NULL — e.g., 'NIS2-Art21-a' through 'NIS2-Art21-j'
  - measure_title VARCHAR(500) NOT NULL
  - measure_description TEXT
  - article_reference VARCHAR(50)
  - implementation_status ENUM('not_started', 'in_progress', 'implemented', 'verified')
  - owner_user_id UUID FK → users
  - evidence_description TEXT
  - last_assessed_at TIMESTAMPTZ
  - next_assessment_date DATE
  - linked_control_ids UUID[] — links to ISO 27001 / NIST controls
  - notes TEXT
  - created_at, updated_at

TABLE nis2_management_accountability:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - board_member_name VARCHAR(200)
  - board_member_role VARCHAR(200)
  - training_completed BOOLEAN DEFAULT false
  - training_date DATE
  - training_provider VARCHAR(200)
  - training_certificate_path TEXT
  - risk_measures_approved BOOLEAN DEFAULT false
  - approval_date DATE
  - approval_document_path TEXT
  - next_training_due DATE
  - notes TEXT
  - created_at, updated_at

GOLANG IMPLEMENTATION:

1. internal/service/nis2_service.go:
   - AssessEntityType(ctx, orgID, assessment) → determine if essential/important
   - CreateIncidentReport(ctx, orgID, incidentID) → create 3-phase report with auto-calculated deadlines
   - SubmitEarlyWarning(ctx, orgID, reportID, content) → record phase 1
   - SubmitNotification(ctx, orgID, reportID, content) → record phase 2
   - SubmitFinalReport(ctx, orgID, reportID, content, documentPath) → record phase 3
   - GetComplianceDashboard(ctx, orgID) → NIS2 compliance metrics
   - GetSecurityMeasuresStatus(ctx, orgID) → status of 10 Article 21 measures
   - RecordManagementTraining(ctx, orgID, memberID, training)
   - RecordRiskMeasuresApproval(ctx, orgID, memberID, approval)

2. Seed the 10 NIS2 Article 21 Security Measures:
   a) Policies on risk analysis and information system security
   b) Incident handling
   c) Business continuity (backup, disaster recovery, crisis management)
   d) Supply chain security
   e) Security in network and information system acquisition, development and maintenance
   f) Policies and procedures for assessing effectiveness of cybersecurity measures
   g) Basic cyber hygiene practices and cybersecurity training
   h) Policies and procedures for use of cryptography and encryption
   i) Human resources security, access control, asset management
   j) Use of multi-factor authentication, secured communications, secured emergency communications

3. Cross-map NIS2 measures to existing ISO 27001 controls (seed data)

4. internal/handler/nis2_handler.go — API endpoints:
   - GET /nis2/assessment → entity assessment status
   - POST /nis2/assessment → submit entity categorisation
   - GET /nis2/incidents → NIS2 incident reports
   - GET /nis2/incidents/{id} → 3-phase report detail
   - POST /nis2/incidents/{id}/early-warning → submit phase 1
   - POST /nis2/incidents/{id}/notification → submit phase 2
   - POST /nis2/incidents/{id}/final-report → submit phase 3
   - GET /nis2/measures → 10 security measures status
   - PUT /nis2/measures/{id} → update measure implementation
   - GET /nis2/management → management accountability records
   - POST /nis2/management → record training/approval
   - GET /nis2/dashboard → NIS2 compliance dashboard

5. NEXT.JS FRONTEND — /nis2 section:
   - NIS2 Compliance Dashboard: entity type, measure implementation progress (10-bar chart), incident reporting compliance, management training status
   - Entity Assessment wizard: step-by-step questionnaire to determine essential/important
   - Security Measures tracker: 10 cards with implementation status, linked ISO 27001 controls
   - Incident 3-Phase Report manager: visual timeline showing 3 phases with deadlines and status
   - Management Accountability table: board members, training dates, approval records

OUTPUT: Complete Golang code, SQL migration, seed data (10 measures + cross-mappings), Next.js pages, and unit tests.
```

---

### PROMPT 15 OF 100 — Continuous Monitoring & Automated Evidence Collection

```
You are a senior Golang backend engineer building the continuous monitoring and automated evidence collection system for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a system that automatically collects evidence of control implementation, monitors compliance status continuously, detects control failures, and triggers alerts when controls drift out of compliance. This transforms ComplianceForge from a point-in-time assessment tool to a continuous compliance platform — a key differentiator for European enterprises.

ARCHITECTURE OVERVIEW:
The system consists of:
1. Evidence Collection Scheduler — runs collection jobs at configured frequencies
2. Collection Agents — adapters that fetch evidence from various sources (manual, API, file, script)
3. Evidence Validator — checks collected evidence against acceptance criteria
4. Compliance Monitor — continuously evaluates control status based on evidence
5. Drift Detector — detects when controls fall out of compliance
6. Alert Engine — triggers notifications when drift is detected

DATABASE SCHEMA — Create migration 011:

TABLE evidence_collection_configs:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - control_implementation_id UUID FK → control_implementations
  - name VARCHAR(200) NOT NULL
  - collection_method ENUM('manual', 'api_fetch', 'file_watch', 'script_execution', 'email_parse', 'webhook_receive')
  - schedule_cron VARCHAR(100) — cron expression: '0 9 * * 1' (every Monday 9am)
  - schedule_description VARCHAR(200) — human-readable: "Every Monday at 9:00 AM"
  
  -- Method-specific configuration
  - api_config JSONB — {url, method, headers, auth_type, auth_credentials_encrypted, response_path, expected_format}
  - file_config JSONB — {path, pattern, expected_format, hash_verification}
  - script_config JSONB — {interpreter, script_path, arguments, expected_output_format, timeout_seconds}
  - webhook_config JSONB — {secret, expected_payload_schema}
  
  -- Validation Rules
  - acceptance_criteria JSONB NOT NULL — [{field: "status", operator: "equals", value: "enabled"}, {field: "count", operator: "greater_than", value: 0}]
  - failure_threshold INT DEFAULT 1 — number of consecutive failures before alerting
  - auto_update_control_status BOOLEAN DEFAULT false — automatically change control status on collection
  
  - is_active BOOLEAN DEFAULT true
  - last_collection_at TIMESTAMPTZ
  - last_collection_status VARCHAR(50)
  - next_collection_at TIMESTAMPTZ
  - consecutive_failures INT DEFAULT 0
  - created_at, updated_at

TABLE evidence_collection_runs:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - config_id UUID FK → evidence_collection_configs
  - control_implementation_id UUID FK
  - status ENUM('scheduled', 'running', 'success', 'failed', 'timeout', 'validation_failed')
  - started_at TIMESTAMPTZ
  - completed_at TIMESTAMPTZ
  - duration_ms INT
  - collected_data JSONB — the raw data collected
  - validation_results JSONB — [{criteria_index: 0, passed: true, actual_value: "enabled"}]
  - all_criteria_passed BOOLEAN
  - evidence_id UUID FK → control_evidence (if evidence record created)
  - error_message TEXT
  - metadata JSONB
  - created_at

TABLE compliance_monitors:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL
  - monitor_type ENUM('control_effectiveness', 'evidence_freshness', 'kri_threshold', 'policy_attestation', 'vendor_assessment', 'training_completion')
  - target_entity_type VARCHAR(50) — 'control_implementation', 'risk_indicator', 'policy', 'vendor'
  - target_entity_id UUID
  - check_frequency_cron VARCHAR(100)
  - conditions JSONB — monitor-specific conditions
  - alert_on_failure BOOLEAN DEFAULT true
  - alert_severity VARCHAR(20) DEFAULT 'high'
  - is_active BOOLEAN DEFAULT true
  - last_check_at TIMESTAMPTZ
  - last_check_status ENUM('passing', 'failing', 'unknown')
  - consecutive_failures INT DEFAULT 0
  - failure_since TIMESTAMPTZ — when it started failing
  - created_at, updated_at

TABLE compliance_monitor_results:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - monitor_id UUID FK → compliance_monitors
  - status ENUM('passing', 'failing')
  - check_time TIMESTAMPTZ
  - result_data JSONB — what was checked and what the values were
  - message TEXT
  - created_at

TABLE compliance_drift_events:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - drift_type ENUM('control_degraded', 'evidence_expired', 'kri_breached', 'policy_unattested', 'vendor_overdue', 'training_expired', 'score_dropped')
  - severity ENUM('critical', 'high', 'medium', 'low')
  - entity_type VARCHAR(50)
  - entity_id UUID
  - entity_ref VARCHAR(50)
  - description TEXT NOT NULL
  - previous_state VARCHAR(100)
  - current_state VARCHAR(100)
  - detected_at TIMESTAMPTZ
  - acknowledged_at TIMESTAMPTZ
  - acknowledged_by UUID FK → users
  - resolved_at TIMESTAMPTZ
  - resolved_by UUID FK → users
  - resolution_notes TEXT
  - notification_sent BOOLEAN DEFAULT false
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/evidence_collector.go:
   - CollectionScheduler: cron-based scheduler that triggers evidence collection jobs
   - ManualCollector: creates evidence from manual file upload
   - APICollector: fetches data from external APIs (e.g., cloud provider status endpoints)
   - FileCollector: watches for files in a directory (e.g., automated scan reports)
   - ScriptCollector: executes a script and captures output (e.g., compliance check scripts)
   - WebhookReceiver: receives evidence via incoming webhooks
   - EvidenceValidator: evaluates collected data against acceptance_criteria rules
   - Each collector:
     a. Fetches/receives the data
     b. Validates against acceptance criteria
     c. If valid: creates a control_evidence record, optionally updates control status
     d. If invalid: increments consecutive_failures, triggers alert if threshold exceeded
     e. Logs the collection run

2. internal/service/compliance_monitor.go:
   - MonitorScheduler: runs compliance checks per monitor configuration
   - ControlEffectivenessMonitor: checks if control evidence is current and passing
   - EvidenceFreshnessMonitor: alerts when evidence is older than configured max age
   - KRIThresholdMonitor: checks if KRI values exceed red thresholds
   - PolicyAttestationMonitor: checks attestation completion rates
   - VendorAssessmentMonitor: checks if vendor assessments are current
   - TrainingCompletionMonitor: checks if required training is up to date
   - ComplianceScoreMonitor: detects when overall score drops below threshold

3. internal/service/drift_detector.go:
   - Analyses monitor results to detect drift events
   - Creates drift_events records
   - Emits notification events for detected drift
   - Provides drift summary dashboard data

4. internal/handler/monitoring_handler.go — API endpoints:
   - GET /monitoring/configs — list evidence collection configs
   - POST /monitoring/configs — create collection config
   - PUT /monitoring/configs/{id} — update config
   - POST /monitoring/configs/{id}/run-now — trigger immediate collection
   - GET /monitoring/configs/{id}/history — collection run history
   - GET /monitoring/monitors — list compliance monitors
   - POST /monitoring/monitors — create monitor
   - PUT /monitoring/monitors/{id} — update monitor
   - GET /monitoring/monitors/{id}/results — monitor check history
   - GET /monitoring/drift — list active drift events
   - PUT /monitoring/drift/{id}/acknowledge — acknowledge drift
   - PUT /monitoring/drift/{id}/resolve — resolve drift
   - GET /monitoring/dashboard — continuous monitoring dashboard

5. NEXT.JS FRONTEND — /monitoring section:
   - Continuous Monitoring Dashboard:
     * Overall compliance health indicator (green/amber/red)
     * Active drift events count (with severity breakdown)
     * Evidence collection success rate (last 24h, 7d, 30d)
     * Monitor status grid: all monitors with passing/failing status
     * Timeline: recent drift events and collection results
   - Evidence Collection Configs:
     * Table of all collection configs with status, schedule, last collection
     * Create/edit config form with method-specific settings
     * "Run Now" button for immediate collection
     * Collection history log
   - Drift Events:
     * Table: type, severity, entity, description, detected date, status
     * Acknowledge and Resolve workflows
     * Filter by type, severity, status
   - Monitor Status:
     * Cards showing each monitor with current status, consecutive failures, uptime
     * Click to see check history with pass/fail timeline

CRITICAL REQUIREMENTS:
- Evidence collection must never expose API credentials in logs or database (encrypt in config)
- Script execution must be sandboxed (timeout, resource limits, no network access)
- API collection must support: Basic Auth, Bearer Token, API Key, OAuth2 Client Credentials
- Webhook receiver must validate HMAC-SHA256 signatures
- All collection runs are asynchronous (via job queue)
- Drift detection runs every 15 minutes
- Evidence freshness checks consider the collection_method — API evidence expires after 24h, manual evidence after 90 days
- The monitoring dashboard must support real-time updates (polling every 30 seconds or WebSocket)

OUTPUT: Complete Golang code for all services, handlers, workers, migration, and Next.js monitoring pages. Include unit tests for the evidence validator and drift detector.
```

---

## BATCH 3 SUMMARY

| Prompt | Focus Area | Tables Created | Key Capabilities |
|--------|-----------|----------------|------------------|
| 11 | Notification Engine | ~4 tables | Multi-channel notifications (email, in-app, webhook, Slack), event-driven rules engine, regulatory deadline alerts, user preferences, escalation management |
| 12 | Advanced Reporting | ~3 tables | PDF/XLSX report generation, 10+ report types, scheduled delivery, report builder, auditor-grade formatting |
| 13 | GDPR DSR Module | ~4 tables | Data Subject Request management (access, erasure, rectification, portability), 30/90-day SLA tracking, task checklists, PII encryption, response templates |
| 14 | NIS2 Compliance | ~4 tables | Entity categorisation, 3-phase incident reporting (24h/72h/1mo), 10 Article 21 measures, management accountability, CSIRT integration |
| 15 | Continuous Monitoring | ~5 tables | Automated evidence collection (API, file, script, webhook), compliance monitors, drift detection, real-time alerting, evidence validation |

**Running Total: 15/100 Prompts | ~55+ Tables | ~100 API Endpoints | Full Compliance Automation**

---

> **NEXT BATCH (Prompts 16–20):** Compliance Workflow Engine (approval chains, task automation), Integration Hub (SSO/SAML, cloud connectors, SIEM integration), Multi-Language & Localisation (i18n for EU languages), Tenant Onboarding Wizard (self-service signup), and Advanced RBAC with Attribute-Based Access Control (ABAC).
>
> Type **"next"** to continue.
