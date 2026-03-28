# GRC Compliance Management Solution — 100 Master Prompts

## BATCH 4 — Workflow Engine, Integration Hub, i18n Localisation, Self-Service Onboarding & Advanced ABAC

**Stack:** Golang 1.22+ | PostgreSQL 16 | Redis 7 | Next.js 14 | SAML/OIDC | WebSockets
**Prerequisite:** All previous batches (Prompts 1–15) completed
**Deliverable:** Approval workflows, third-party integrations, EU language support, self-service tenant setup, and fine-grained access control

---

### PROMPT 16 OF 100 — Compliance Workflow Engine (Approval Chains, Task Automation & SLAs)

```
You are a senior Golang backend engineer building the workflow automation engine for "ComplianceForge" — a GRC platform targeting European enterprises.

OBJECTIVE:
Build a configurable, multi-step workflow engine that automates approval chains, task routing, escalation, and SLA enforcement across all GRC modules. The engine must support: policy approval workflows, risk acceptance sign-off, audit finding remediation tracking, vendor onboarding approval, exception requests, and change management — all with configurable steps, conditional routing, parallel approvals, and deadline enforcement.

This is the operational backbone that turns ComplianceForge from a compliance database into a compliance operations platform.

DATABASE SCHEMA — Create migration 012:

TABLE workflow_definitions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL
  - description TEXT
  - workflow_type ENUM('policy_approval', 'risk_acceptance', 'exception_request', 'finding_remediation', 'vendor_onboarding', 'vendor_assessment', 'change_request', 'access_request', 'evidence_review', 'dsr_processing', 'incident_response', 'custom')
  - entity_type VARCHAR(100) NOT NULL — e.g., 'policy', 'risk', 'exception', 'finding', 'vendor'
  - version INT DEFAULT 1
  - status ENUM('draft', 'active', 'deprecated') DEFAULT 'draft'
  - trigger_conditions JSONB — when to auto-start this workflow: {"on_create": true, "on_status_change": {"from": "draft", "to": "under_review"}, "on_field_change": ["severity"]}
  - sla_config JSONB — {"default_sla_hours": 72, "escalation_after_hours": 48, "auto_approve_after_hours": null, "business_hours_only": true}
  - metadata JSONB
  - created_by UUID FK → users
  - is_system BOOLEAN DEFAULT false — system-provided workflows
  - created_at, updated_at

TABLE workflow_steps:
  - id UUID PK
  - workflow_definition_id UUID FK → workflow_definitions
  - organization_id UUID FK (RLS)
  - step_order INT NOT NULL
  - name VARCHAR(200) NOT NULL
  - description TEXT
  - step_type ENUM('approval', 'review', 'task', 'notification', 'condition', 'parallel_gate', 'timer', 'auto_action')
  
  -- Approval/Review config
  - approver_type ENUM('specific_user', 'role', 'manager_of_owner', 'entity_owner', 'dpo', 'ciso', 'custom_query') 
  - approver_ids UUID[] — specific user or role IDs
  - approval_mode ENUM('any_one', 'all_required', 'majority') DEFAULT 'any_one' — for multiple approvers
  - minimum_approvals INT DEFAULT 1
  
  -- Task config
  - task_description TEXT
  - task_assignee_type ENUM('specific_user', 'role', 'entity_owner', 'previous_step_actor')
  - task_assignee_ids UUID[]
  
  -- Condition config (for branching)
  - condition_expression JSONB — {"field": "risk_level", "operator": "in", "values": ["critical", "high"]}
  - condition_true_step_id UUID — go to this step if condition is true
  - condition_false_step_id UUID — go to this step if false
  
  -- Auto-action config
  - auto_action JSONB — {"action": "update_status", "target_field": "status", "target_value": "approved"} or {"action": "send_notification", "template": "policy_approved"}
  
  -- Timer config
  - timer_hours INT — wait this many hours before proceeding
  - timer_business_hours_only BOOLEAN DEFAULT true
  
  -- SLA
  - sla_hours INT — override per-step SLA
  - escalation_user_ids UUID[] — who to escalate to if SLA breached
  - is_optional BOOLEAN DEFAULT false — can be skipped
  - can_delegate BOOLEAN DEFAULT true — can the approver delegate to someone else
  
  - created_at, updated_at

TABLE workflow_instances:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - workflow_definition_id UUID FK → workflow_definitions
  - entity_type VARCHAR(100) NOT NULL
  - entity_id UUID NOT NULL — the thing being processed (policy ID, risk ID, etc.)
  - entity_ref VARCHAR(50) — human-readable ref (POL-0001, RSK-0003)
  - status ENUM('active', 'completed', 'cancelled', 'suspended', 'failed')
  - current_step_id UUID FK → workflow_steps
  - current_step_order INT
  - started_at TIMESTAMPTZ
  - started_by UUID FK → users
  - completed_at TIMESTAMPTZ
  - completion_outcome ENUM('approved', 'rejected', 'completed', 'cancelled', 'timed_out')
  - total_duration_hours DECIMAL(10,2)
  - sla_status ENUM('on_track', 'at_risk', 'breached')
  - sla_deadline TIMESTAMPTZ
  - metadata JSONB — stores runtime data passed between steps
  - created_at, updated_at

TABLE workflow_step_executions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - workflow_instance_id UUID FK → workflow_instances
  - workflow_step_id UUID FK → workflow_steps
  - step_order INT
  - status ENUM('pending', 'in_progress', 'approved', 'rejected', 'completed', 'skipped', 'escalated', 'delegated', 'timed_out')
  - assigned_to UUID FK → users (resolved at runtime from approver config)
  - delegated_to UUID FK → users
  - delegated_by UUID FK
  - delegated_at TIMESTAMPTZ
  - action_taken_by UUID FK → users
  - action_taken_at TIMESTAMPTZ
  - action ENUM('approve', 'reject', 'complete', 'delegate', 'skip', 'escalate', 'request_info')
  - comments TEXT — approver's comments
  - decision_reason TEXT — required for rejections
  - attachments_paths TEXT[] — supporting documents
  - sla_deadline TIMESTAMPTZ
  - sla_status ENUM('on_track', 'at_risk', 'breached')
  - escalated_at TIMESTAMPTZ
  - escalated_to UUID FK → users
  - started_at TIMESTAMPTZ
  - completed_at TIMESTAMPTZ
  - duration_hours DECIMAL(10,2)
  - created_at

TABLE workflow_delegation_rules:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - delegator_user_id UUID FK → users
  - delegate_user_id UUID FK → users
  - workflow_types TEXT[] — which workflow types can be delegated, NULL = all
  - valid_from DATE NOT NULL
  - valid_until DATE NOT NULL
  - reason TEXT
  - is_active BOOLEAN DEFAULT true
  - created_by UUID FK
  - created_at, updated_at

GOLANG IMPLEMENTATION:

1. internal/service/workflow_engine.go — Core Engine:

   WorkflowEngine struct with methods:
   - StartWorkflow(ctx, orgID, workflowType, entityType, entityID, startedBy) → creates instance, resolves first step assignees, sends notifications
   - ProcessStep(ctx, orgID, executionID, action, actorID, comments, reason) → processes approval/rejection/completion, advances to next step or completes workflow
   - DelegateStep(ctx, orgID, executionID, delegatorID, delegateID) → reassign step
   - EscalateStep(ctx, orgID, executionID) → escalate overdue step
   - CancelWorkflow(ctx, orgID, instanceID, cancelledBy, reason)
   - SuspendWorkflow / ResumeWorkflow
   - GetPendingApprovals(ctx, orgID, userID) → all steps pending this user's action
   - GetWorkflowHistory(ctx, orgID, entityType, entityID) → workflow instances for an entity

   Step Resolution Logic:
   - When advancing to next step:
     a. If step_type='condition': evaluate condition_expression against entity data, route to true/false step
     b. If step_type='parallel_gate': create multiple step_executions (one per approver), wait for approval_mode
     c. If step_type='approval': resolve approver from approver_type config, check delegation rules, create execution, send notification
     d. If step_type='auto_action': execute the configured action immediately, advance to next step
     e. If step_type='timer': schedule a delayed advancement
     f. If step_type='notification': send notification, immediately advance
   
   Approver Resolution:
   - 'specific_user' → use approver_ids directly
   - 'role' → query users with that role in the org
   - 'manager_of_owner' → look up the entity owner's manager
   - 'entity_owner' → look up the owner field on the entity
   - 'dpo' → find user with DPO role
   - 'ciso' → find user with CISO role
   - Check delegation_rules: if the resolved user has an active delegation, use the delegate instead

2. internal/worker/workflow_scheduler.go:
   - Runs every 5 minutes
   - Check all active workflow step_executions where sla_deadline < NOW():
     * If at_risk (within 80% of SLA): update status, send warning notification
     * If breached: escalate to configured escalation users, create new execution for escalatee
   - Check timer steps: if timer has expired, advance the workflow
   - Auto-trigger workflows: check trigger_conditions against recently changed entities

3. internal/handler/workflow_handler.go — API Endpoints:
   - GET /workflows/definitions — list workflow definitions (admin)
   - POST /workflows/definitions — create workflow definition
   - PUT /workflows/definitions/{id} — update definition
   - GET /workflows/definitions/{id}/steps — get workflow steps
   - POST /workflows/definitions/{id}/steps — add step
   - PUT /workflows/definitions/{id}/steps/{stepId} — update step
   - DELETE /workflows/definitions/{id}/steps/{stepId} — remove step
   - POST /workflows/definitions/{id}/activate — activate workflow
   
   - GET /workflows/instances — list workflow instances (filterable by entity, status)
   - GET /workflows/instances/{id} — get instance detail with step executions
   - POST /workflows/start — manually start a workflow for an entity
   - POST /workflows/instances/{id}/cancel — cancel workflow
   
   - GET /workflows/my-approvals — pending approvals for current user (across all workflows)
   - POST /workflows/executions/{id}/approve — approve step
   - POST /workflows/executions/{id}/reject — reject step (requires reason)
   - POST /workflows/executions/{id}/delegate — delegate step
   - POST /workflows/executions/{id}/request-info — request more information
   
   - GET /workflows/delegations — list delegation rules
   - POST /workflows/delegations — create delegation (out-of-office coverage)

4. SEED — Default Workflow Definitions:

   a. Policy Approval Workflow (3 steps):
      Step 1: Review by Compliance Officer (role)
      Step 2: Approval by Policy Approver (entity field: approver_user_id)
      Step 3: Auto-action: update policy status to 'approved', send 'policy_approved' notification
      
   b. Risk Acceptance Workflow (2 steps, conditional):
      Step 1: Condition: if residual_risk_level IN ('critical', 'high') → go to Step 2A, else Step 2B
      Step 2A: Approval by CISO (role) — for critical/high risks
      Step 2B: Approval by Risk Manager (role) — for medium/low risks
      Final: Auto-action: update risk status to 'accepted'
      
   c. Exception Request Workflow (4 steps):
      Step 1: Review by Control Owner
      Step 2: Risk Assessment by Risk Manager
      Step 3: Approval by CISO (if risk is high) or Compliance Officer (if medium/low)
      Step 4: Auto-action: create policy_exception record, set expiry date
      
   d. Audit Finding Remediation Workflow (3 steps):
      Step 1: Assign remediation task to Finding Owner
      Step 2: Review evidence by Auditor
      Step 3: Verification and closure

   e. Vendor Onboarding Approval (3 steps, parallel):
      Step 1: Parallel gate: Security Review (CISO) AND Legal Review (DPO if data_processing=true)
      Step 2: Final Approval by Vendor Manager
      Step 3: Auto-action: update vendor status to 'active'

5. NEXT.JS FRONTEND:
   - /workflows/my-approvals — inbox-style page showing all pending approvals with:
     * Entity context (what is being approved, link to entity)
     * Workflow step name and description
     * SLA deadline with countdown
     * "Approve" / "Reject" / "Delegate" / "Request Info" action buttons
     * Comments field (required for reject)
     * Sorted by SLA deadline (most urgent first)
   - /workflows/definitions — admin page to view/edit workflow definitions
     * Visual workflow builder: drag-and-drop steps into a flowchart
     * Step configuration panel: type, approver, SLA, conditions
     * Preview workflow as a diagram
   - Entity-level workflow panel: on every entity detail page (policy, risk, etc.), show:
     * Current workflow status with visual progress indicator (step 2 of 4)
     * Step history: who approved/rejected at each step, when, with comments
     * If current user is the assigned approver: inline approve/reject buttons
   - /workflows/delegations — manage delegation rules (out-of-office)
   - Dashboard widget: "X approvals pending your action" with link to /workflows/my-approvals

CRITICAL REQUIREMENTS:
- Workflow definitions are versioned: changing an active definition creates a new version, existing instances continue on the old version
- Rejection at any step cascels the entire workflow and reverts entity status
- Parallel gates support 'any_one', 'all_required', or 'majority' approval modes
- Delegation chains are max 2 levels deep (to prevent infinite delegation)
- All workflow actions are immutably logged in workflow_step_executions
- SLA calculations respect business hours (configurable: Mon-Fri, 09:00-18:00, timezone-aware)
- Workflow definitions can be exported/imported as JSON (for sharing between orgs)
- The approval inbox (GET /workflows/my-approvals) must be highly optimised — this is the most frequently accessed page for managers

OUTPUT: Complete Golang code for workflow engine, handler, scheduler, migration, seed workflows, and Next.js approval inbox + workflow builder pages. Include unit tests for step resolution logic, SLA calculation, and conditional branching.
```

---

### PROMPT 17 OF 100 — Integration Hub (SSO/SAML, Cloud Connectors, SIEM & ITSM)

```
You are a senior Golang backend engineer building the integration hub for "ComplianceForge" — a GRC platform. European enterprises require SSO, cloud security posture integrations, and SIEM/ITSM connectivity.

OBJECTIVE:
Build a pluggable integration framework that supports: Single Sign-On (SAML 2.0 / OIDC), cloud provider integrations (AWS, Azure, GCP) for automated evidence collection, SIEM integration for incident ingestion, ITSM integration for ticket synchronisation, and a webhook-based custom integration API. Each integration must be configurable per organisation without code changes.

DATABASE SCHEMA — Create migration 013:

TABLE integrations:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - integration_type ENUM('sso_saml', 'sso_oidc', 'cloud_aws', 'cloud_azure', 'cloud_gcp', 'siem_splunk', 'siem_elastic', 'siem_sentinel', 'itsm_servicenow', 'itsm_jira', 'itsm_freshservice', 'email_smtp', 'email_sendgrid', 'slack', 'teams', 'webhook_inbound', 'webhook_outbound', 'custom_api')
  - name VARCHAR(200) NOT NULL
  - description TEXT
  - status ENUM('active', 'inactive', 'error', 'pending_setup')
  - configuration_encrypted TEXT NOT NULL — AES-256-GCM encrypted JSON config
  - health_status ENUM('healthy', 'degraded', 'unhealthy', 'unknown') DEFAULT 'unknown'
  - last_health_check_at TIMESTAMPTZ
  - last_sync_at TIMESTAMPTZ
  - sync_frequency_minutes INT — 0 = real-time/webhook, >0 = polling interval
  - error_count INT DEFAULT 0
  - last_error_message TEXT
  - capabilities TEXT[] — what this integration can do: {'evidence_collection', 'incident_ingestion', 'user_sync', 'ticket_sync'}
  - created_by UUID FK → users
  - metadata JSONB
  - created_at, updated_at

TABLE integration_sync_logs:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - integration_id UUID FK → integrations
  - sync_type VARCHAR(100) — 'evidence_collection', 'incident_ingestion', 'user_sync', 'health_check'
  - status ENUM('started', 'completed', 'failed', 'partial')
  - records_processed INT
  - records_created INT
  - records_updated INT
  - records_failed INT
  - started_at TIMESTAMPTZ
  - completed_at TIMESTAMPTZ
  - duration_ms INT
  - error_message TEXT
  - details JSONB
  - created_at

TABLE sso_configurations:
  - id UUID PK
  - organization_id UUID FK (RLS) UNIQUE — one SSO config per org
  - protocol ENUM('saml2', 'oidc')
  - is_enabled BOOLEAN DEFAULT false
  - is_enforced BOOLEAN DEFAULT false — force all users through SSO
  
  -- SAML 2.0 config
  - saml_entity_id VARCHAR(500)
  - saml_sso_url TEXT
  - saml_slo_url TEXT
  - saml_certificate TEXT — IdP's X.509 certificate (PEM)
  - saml_name_id_format VARCHAR(200) DEFAULT 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress'
  - saml_attribute_mapping JSONB — {"email": "...", "first_name": "...", "last_name": "...", "groups": "..."}
  
  -- OIDC config
  - oidc_issuer_url TEXT
  - oidc_client_id VARCHAR(500)
  - oidc_client_secret_encrypted TEXT
  - oidc_scopes TEXT[] DEFAULT ARRAY['openid', 'profile', 'email']
  - oidc_claim_mapping JSONB
  
  -- Common
  - auto_provision_users BOOLEAN DEFAULT true — auto-create user on first SSO login
  - default_role_id UUID FK → roles — role assigned to auto-provisioned users
  - allowed_domains TEXT[] — restrict SSO to specific email domains
  - group_to_role_mapping JSONB — {"IdP_Group_Name": "complianceforge_role_slug"}
  - jit_provisioning BOOLEAN DEFAULT true — Just-In-Time user provisioning
  
  - created_at, updated_at

TABLE api_keys:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL
  - key_prefix VARCHAR(10) NOT NULL — first 10 chars for identification (e.g., "cf_live_ab3")
  - key_hash VARCHAR(128) NOT NULL — SHA-256 of the full key
  - permissions TEXT[] — e.g., {'read:risks', 'write:incidents', 'read:compliance'}
  - rate_limit_per_minute INT DEFAULT 60
  - expires_at TIMESTAMPTZ
  - last_used_at TIMESTAMPTZ
  - last_used_ip VARCHAR(45)
  - is_active BOOLEAN DEFAULT true
  - created_by UUID FK → users
  - created_at, updated_at

GOLANG IMPLEMENTATION:

1. internal/service/integration_service.go — Core Integration Framework:
   
   Integration interface:
   ```go
   type Integration interface {
       Type() string
       Connect(ctx context.Context, config json.RawMessage) error
       Disconnect(ctx context.Context) error
       HealthCheck(ctx context.Context) (HealthStatus, error)
       Sync(ctx context.Context, syncType string) (*SyncResult, error)
   }
   ```
   
   IntegrationRegistry: register and lookup integrations by type
   IntegrationManager: CRUD operations, health monitoring, sync scheduling

2. internal/integrations/sso_saml.go — SAML 2.0 SSO:
   - SP metadata endpoint: GET /auth/saml/metadata
   - ACS (Assertion Consumer Service): POST /auth/saml/acs — receives SAML response from IdP
   - SLO (Single Logout): GET /auth/saml/slo
   - Parse SAML assertion, validate signature against IdP certificate
   - Extract user attributes using attribute_mapping
   - JIT provisioning: if user doesn't exist, create with default role
   - Group mapping: sync IdP groups to ComplianceForge roles
   - Use crewjam/saml Go library

3. internal/integrations/sso_oidc.go — OpenID Connect SSO:
   - Initiate flow: GET /auth/oidc/login → redirect to IdP
   - Callback: GET /auth/oidc/callback → exchange code for tokens, validate ID token
   - Extract user info from claims using claim_mapping
   - JIT provisioning with role assignment
   - Token refresh handling
   - Support for: Azure AD, Okta, Google Workspace, Auth0, Keycloak

4. internal/integrations/cloud_aws.go — AWS Integration:
   - Authenticate: AWS IAM Role (cross-account assume role) or Access Key
   - Evidence collection capabilities:
     * AWS Config: compliance rules status → maps to controls
     * AWS SecurityHub: findings → maps to risks/incidents
     * AWS CloudTrail: audit logs → evidence for A.8.15 (Logging)
     * AWS IAM: access review → evidence for A.5.15 (Access Control)
     * AWS KMS: encryption status → evidence for A.8.24 (Cryptography)
     * AWS GuardDuty: threats → incident ingestion
   - Sync method: fetch data from AWS APIs, create evidence records, update control implementation status

5. internal/integrations/cloud_azure.go — Azure Integration:
   - Authenticate: Azure AD App Registration (client credentials)
   - Evidence collection:
     * Azure Security Center / Defender: secure score, recommendations
     * Azure Policy: compliance state
     * Azure AD: user access reviews, MFA status
     * Azure Key Vault: encryption configuration
     * Azure Sentinel: security incidents
   - Map Azure Security Center recommendations to ISO 27001 / NIST controls

6. internal/integrations/siem_splunk.go — Splunk Integration:
   - Connect via Splunk REST API
   - Inbound: query Splunk for security events matching configured searches
   - Create incidents in ComplianceForge from Splunk notable events
   - Outbound: send ComplianceForge incidents to Splunk as events
   - Bidirectional status sync

7. internal/integrations/itsm_servicenow.go — ServiceNow Integration:
   - Connect via ServiceNow REST API (OAuth2 or Basic Auth)
   - Bidirectional sync:
     * Audit findings → ServiceNow incidents/tasks
     * Risk treatments → ServiceNow change requests
     * ComplianceForge status updates ↔ ServiceNow status updates
   - Map ServiceNow priorities to ComplianceForge severities
   - Attachment sync for evidence

8. internal/integrations/itsm_jira.go — Jira Integration:
   - Connect via Jira REST API (API token or OAuth2)
   - Sync audit findings → Jira issues
   - Sync risk treatments → Jira stories/tasks
   - Map Jira statuses to ComplianceForge statuses
   - Webhook receiver for real-time Jira updates

9. internal/middleware/apikey_auth.go — API Key Authentication:
   - Extract API key from X-API-Key header or ?api_key query param
   - Lookup by prefix, verify by SHA-256 hash comparison
   - Check permissions against requested endpoint
   - Rate limit per key
   - Log usage

10. internal/handler/integration_handler.go — API Endpoints:
    - GET /integrations — list all integrations for org
    - POST /integrations — create integration (validates config, tests connection)
    - GET /integrations/{id} — get integration detail
    - PUT /integrations/{id} — update configuration
    - DELETE /integrations/{id} — remove integration
    - POST /integrations/{id}/test — test connectivity
    - POST /integrations/{id}/sync — trigger manual sync
    - GET /integrations/{id}/logs — sync history
    - GET /integrations/{id}/health — health status
    
    - GET /settings/sso — SSO configuration
    - PUT /settings/sso — update SSO configuration
    - GET /auth/saml/metadata — SAML SP metadata XML
    - POST /auth/saml/acs — SAML assertion consumer service
    - GET /auth/oidc/login — initiate OIDC flow
    - GET /auth/oidc/callback — OIDC callback
    
    - GET /settings/api-keys — list API keys
    - POST /settings/api-keys — create API key (returns full key ONCE)
    - DELETE /settings/api-keys/{id} — revoke API key

11. NEXT.JS FRONTEND — /settings/integrations:
    - Integration marketplace page: grid of available integration types with logos
    - Each integration card: type, status indicator (green/amber/red), last sync time
    - Setup wizard per integration type: step-by-step configuration with test connection
    - SSO configuration page: SAML/OIDC setup with metadata upload, attribute mapping
    - API Keys management: create (show key once), list, revoke
    - Integration detail: health status, sync logs, manual sync trigger, configuration edit
    - Cloud integration dashboard: evidence collection status across AWS/Azure/GCP

CRITICAL REQUIREMENTS:
- ALL credentials/secrets stored encrypted (AES-256-GCM) — never in plaintext
- SAML certificate validation: reject expired or self-signed certificates (configurable)
- SSO enforcement: when enabled, password login disabled for all non-admin users
- Cloud integrations use least-privilege IAM roles (document required permissions)
- API key full value shown ONLY at creation time — never retrievable again
- Rate limiting on API key endpoints: per-key configurable (default 60/min)
- Integration health checks run every 5 minutes in the background worker
- All sync operations are idempotent — running twice produces the same result
- Webhook inbound endpoints validate HMAC signatures
- Integration configuration changes logged in audit trail

OUTPUT: Complete Golang code for integration framework, all integration adapters (SSO, cloud, SIEM, ITSM), handlers, middleware, migration, and Next.js integration marketplace pages. Include integration setup documentation.
```

---

### PROMPT 18 OF 100 — Multi-Language & Localisation (i18n for EU Languages)

```
You are a senior full-stack engineer implementing internationalisation (i18n) and localisation (l10n) for "ComplianceForge" — a GRC platform targeting European enterprises.

OBJECTIVE:
Implement complete multi-language support across both the Golang backend and Next.js frontend. The platform must support all major EU business languages with proper date/time formatting, number formatting, currency formatting, right-to-left text (for future Arabic support), and regulatory term localisation. Framework and control names remain in English (as they are standardised), but all UI chrome, messages, notifications, reports, and user-generated content labels are translatable.

SUPPORTED LANGUAGES (Phase 1):
- English (en-GB) — default
- German (de-DE) — largest EU economy
- French (fr-FR) — EU institutional language
- Spanish (es-ES) — large market
- Italian (it-IT) — large market
- Dutch (nl-NL) — key market
- Portuguese (pt-PT) — EU market
- Polish (pl-PL) — growing market
- Swedish (sv-SE) — Nordics entry

BACKEND IMPLEMENTATION:

1. internal/i18n/i18n.go — Translation Engine:
   - Load translations from JSON files: locales/{lang}.json
   - Fallback chain: requested language → en-GB
   - Template variable interpolation: T("notification.breach_alert", map[string]interface{}{"ref": "INC-0001", "hours": 12})
   - Pluralisation rules per language (English: 1=singular, 2+=plural; Polish: complex pluralisation)
   - Thread-safe: translations loaded once at startup, no mutex contention
   - Missing key logging: log.Warn when a translation key is missing

2. Translation Files Structure (locales/):
   - Common categories:
     * common.json: buttons (Save, Cancel, Delete, Edit, Create, Export), status labels, severity labels, pagination
     * navigation.json: sidebar items, breadcrumbs
     * dashboard.json: KPI labels, chart labels
     * frameworks.json: framework-related UI text (NOT control names — those stay English)
     * risks.json: risk register labels, heatmap labels, form labels
     * policies.json: policy management labels
     * audits.json: audit labels
     * incidents.json: incident labels, GDPR breach terminology
     * vendors.json: vendor management labels
     * settings.json: organisation settings, user management labels
     * notifications.json: all notification templates (15+ templates × 9 languages)
     * reports.json: report titles, section headers, footer text
     * errors.json: all API error messages
     * dsr.json: GDPR data subject request labels
     * nis2.json: NIS2 specific terminology

3. API Localisation:
   - Accept-Language header determines response language
   - Error messages returned in the requested language
   - Notification emails sent in the user's preferred language
   - PDF reports generated in the organisation's default language
   - Audit log descriptions stored in English (canonical) but displayed translated

4. Date/Time Localisation:
   - German: 28.03.2026 14:30
   - French: 28/03/2026 14h30
   - English: 28/03/2026 14:30
   - Relative time: "vor 2 Stunden" (de), "il y a 2 heures" (fr), "2 hours ago" (en)
   
5. Number/Currency:
   - German: 1.234.567,89 € 
   - French: 1 234 567,89 €
   - English: €1,234,567.89

FRONTEND IMPLEMENTATION:

6. next-intl Integration:
   - Configure next-intl with App Router
   - Language switcher in topbar (dropdown with flag emojis)
   - User language preference stored in profile (synced to backend)
   - Organisation default language setting
   - URL prefix routing: /en/dashboard, /de/dashboard, /fr/dashboard (optional, configurable)
   - Translation files in messages/{lang}.json matching backend structure
   - useTranslations() hook in every component
   - Server components: use getTranslations()

7. Translation File Example (messages/de.json — partial):
   ```json
   {
     "common": {
       "save": "Speichern",
       "cancel": "Abbrechen",
       "delete": "Löschen",
       "create": "Erstellen",
       "edit": "Bearbeiten",
       "export": "Exportieren",
       "search": "Suchen...",
       "loading": "Wird geladen...",
       "no_results": "Keine Ergebnisse gefunden",
       "showing": "Zeige {from}–{to} von {total} Ergebnissen",
       "confirm_delete": "Sind Sie sicher, dass Sie dies löschen möchten?",
       "status": {
         "active": "Aktiv",
         "inactive": "Inaktiv",
         "draft": "Entwurf",
         "published": "Veröffentlicht",
         "archived": "Archiviert"
       },
       "severity": {
         "critical": "Kritisch",
         "high": "Hoch",
         "medium": "Mittel",
         "low": "Niedrig"
       }
     },
     "dashboard": {
       "title": "Übersicht",
       "compliance_score": "Compliance-Bewertung",
       "open_risks": "Offene Risiken",
       "open_incidents": "Offene Vorfälle",
       "open_findings": "Offene Feststellungen",
       "policies_due": "Fällige Richtlinien",
       "high_risk_vendors": "Hochrisiko-Lieferanten",
       "breach_alert": "DSGVO-Datenschutzverletzung — Meldung erforderlich",
       "breach_deadline": "Meldefrist",
       "hours_remaining": "{hours} Stunden verbleibend",
       "notify_dpa": "Aufsichtsbehörde benachrichtigen"
     },
     "incidents": {
       "title": "Vorfallmanagement",
       "report_incident": "Vorfall melden",
       "data_breach": "Datenschutzverletzung",
       "notification_deadline": "Meldefrist (72 Stunden)",
       "dpa_notified": "Aufsichtsbehörde benachrichtigt",
       "subjects_affected": "{count} betroffene Personen"
     }
   }
   ```

8. Provide COMPLETE translation files for ALL 9 languages covering ALL UI text.

9. RTL Support Preparation:
   - Use logical CSS properties (margin-inline-start instead of margin-left)
   - dir="rtl" on html element when Arabic is selected
   - Tailwind RTL plugin configured

CRITICAL REQUIREMENTS:
- Framework names (ISO 27001, NIST CSF, etc.) and control codes (A.5.1, AC-2) are NEVER translated
- GDPR/NIS2 regulatory terms use official translations from the directives (e.g., "Datenschutz-Grundverordnung" not a casual translation)
- All 15+ notification email templates must be available in all 9 languages
- PDF reports must render correctly in all languages (font support for Polish characters ł, ń, ś, etc.)
- Language preference cascades: user preference → organisation default → browser Accept-Language → en-GB
- Switching language does not require page reload (client-side re-render)
- Translation keys are consistent between backend and frontend (same namespace structure)
- Missing translations fall back gracefully to English (never show raw keys to users)

OUTPUT: Complete Golang i18n package, ALL translation JSON files for 9 languages, Next.js i18n configuration, language switcher component, and updated notification templates in all languages.
```

---

### PROMPT 19 OF 100 — Self-Service Onboarding Wizard & Subscription Management

```
You are a senior full-stack engineer building the self-service onboarding experience for "ComplianceForge" — a GRC SaaS platform.

OBJECTIVE:
Build a complete self-service signup, onboarding wizard, and subscription management system. A new customer should be able to: sign up, choose a plan, complete a guided onboarding wizard that configures their compliance programme, adopt their relevant frameworks, and be productive within 30 minutes. The system must also handle plan upgrades/downgrades, usage metering, and billing portal integration.

DATABASE SCHEMA — Create migration 014:

TABLE subscription_plans:
  - id UUID PK
  - name VARCHAR(100) NOT NULL — 'Starter', 'Professional', 'Enterprise'
  - slug VARCHAR(50) NOT NULL UNIQUE
  - description TEXT
  - tier ENUM('starter', 'professional', 'enterprise', 'unlimited')
  - pricing_eur_monthly DECIMAL(10,2)
  - pricing_eur_annual DECIMAL(10,2) — annual = monthly × 10 (2 months free)
  - max_users INT
  - max_frameworks INT
  - max_risks INT — 0 = unlimited
  - max_vendors INT
  - max_storage_gb INT
  - features JSONB — {"sso": false, "api_access": false, "custom_reports": true, "continuous_monitoring": false, "workflow_engine": false, "integrations": 2}
  - is_active BOOLEAN DEFAULT true
  - sort_order INT
  - created_at, updated_at

TABLE organization_subscriptions:
  - id UUID PK
  - organization_id UUID FK → organizations UNIQUE
  - plan_id UUID FK → subscription_plans
  - status ENUM('trialing', 'active', 'past_due', 'cancelled', 'paused')
  - billing_cycle ENUM('monthly', 'annual')
  - current_period_start TIMESTAMPTZ
  - current_period_end TIMESTAMPTZ
  - trial_ends_at TIMESTAMPTZ — 14-day free trial
  - cancelled_at TIMESTAMPTZ
  - cancel_reason TEXT
  - stripe_customer_id VARCHAR(200)
  - stripe_subscription_id VARCHAR(200)
  - usage_snapshot JSONB — {"users": 5, "frameworks": 3, "risks": 47, "vendors": 12, "storage_gb": 2.1}
  - created_at, updated_at

TABLE onboarding_progress:
  - id UUID PK
  - organization_id UUID FK (RLS) UNIQUE
  - current_step INT DEFAULT 1
  - total_steps INT DEFAULT 7
  - completed_steps JSONB DEFAULT '[]' — [{"step": 1, "completed_at": "..."}, ...]
  - is_completed BOOLEAN DEFAULT false
  - completed_at TIMESTAMPTZ
  - skipped_steps INT[] DEFAULT ARRAY[]::INT[]
  
  -- Step-specific data
  - org_profile_data JSONB — step 1 answers
  - industry_assessment_data JSONB — step 2 answers (used for framework recommendations)
  - selected_framework_ids UUID[] — step 3 selections
  - team_invitations JSONB — step 4: [{email, role, name}]
  - risk_appetite_data JSONB — step 5 answers
  - quick_assessment_data JSONB — step 6 answers
  
  - created_at, updated_at

TABLE usage_events:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - event_type VARCHAR(100) — 'user_added', 'framework_adopted', 'risk_created', 'storage_used', 'api_call', 'report_generated'
  - quantity DECIMAL(10,2) DEFAULT 1
  - metadata JSONB
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/onboarding_wizard.go:

   7-Step Onboarding Wizard:

   Step 1 — Organisation Profile:
   - Collect: org name, legal name, industry, country, employee count range, website
   - Auto-detect timezone from country
   - Store in onboarding_progress.org_profile_data

   Step 2 — Industry & Regulatory Assessment:
   - Interactive questionnaire (10-15 questions):
     * "Do you process payment card data?" → recommend PCI DSS
     * "Do you process personal data of EU residents?" → recommend UK GDPR
     * "Are you a provider of essential services (energy, transport, health, digital infrastructure)?" → recommend NIS2 + NCSC CAF
     * "Do you operate in the UK public sector?" → recommend Cyber Essentials
     * "Does your organisation require ISO certification?" → recommend ISO 27001
     * "Do you have US federal contracts?" → recommend NIST 800-53
     * "What is your target cybersecurity maturity level?" → recommend NIST CSF 2.0
     * "Are you subject to ITIL service management requirements?" → recommend ITIL 4
     * "Does your board require IT governance reporting?" → recommend COBIT 2019
   - Based on answers, generate framework recommendations with priority ranking
   - Explain WHY each framework is recommended based on their answers
   - Store in onboarding_progress.industry_assessment_data

   Step 3 — Framework Selection:
   - Show recommended frameworks (from step 2) with "Recommended" badges
   - Allow selecting additional frameworks
   - Show estimated implementation effort per framework
   - Show cross-framework overlap percentages ("ISO 27001 covers 85% of Cyber Essentials")
   - Enforce plan limits (Starter: max 3, Professional: max 5, Enterprise: 9)
   - Store selected_framework_ids

   Step 4 — Team Setup:
   - Invite team members: email, name, role (from predefined roles)
   - Suggest roles based on selected frameworks:
     * ISO 27001 → need Compliance Officer, Control Owners
     * UK GDPR → need DPO
     * NIST 800-53 → need Security Manager
   - Enforce plan user limits
   - Send invitation emails immediately
   - Store team_invitations

   Step 5 — Risk Appetite Configuration:
   - Configure the 5×5 risk matrix labels and thresholds
   - Set risk appetite: what level is acceptable without treatment?
   - Define risk categories relevant to their industry
   - Quick-start option: "Use default ISO 31000 matrix"
   - Store risk_appetite_data

   Step 6 — Quick Compliance Assessment:
   - For each selected framework, show the top 10 most critical controls
   - Ask: "Is this control implemented?" (Yes / Partial / No / Don't Know)
   - This gives an instant baseline compliance score
   - Show a live compliance score updating as they answer
   - Store quick_assessment_data

   Step 7 — Summary & Launch:
   - Show everything configured: org details, frameworks adopted, team invited, initial compliance score
   - "Launch ComplianceForge" button that:
     a. Creates the organisation (from step 1)
     b. Creates the admin user
     c. Creates the subscription
     d. Adopts selected frameworks (from step 3)
     e. Initialises control implementations for all controls
     f. Applies quick assessment answers (from step 6) to control statuses
     g. Creates risk matrix (from step 5)
     h. Sends team invitations (from step 4)
     i. Marks onboarding as complete
   - All in a SINGLE DATABASE TRANSACTION

2. internal/service/subscription_service.go:
   - CreateSubscription(ctx, orgID, planSlug, billingCycle)
   - UpgradePlan(ctx, orgID, newPlanSlug) — pro-rated
   - DowngradePlan(ctx, orgID, newPlanSlug) — effective at period end
   - CancelSubscription(ctx, orgID, reason)
   - PauseSubscription / ResumeSubscription
   - CheckLimits(ctx, orgID, resource) → returns {current, max, canCreate}
   - RecordUsage(ctx, orgID, eventType, quantity)
   - GetUsageSummary(ctx, orgID) → current usage vs plan limits

3. internal/middleware/plan_limits.go:
   - Middleware that checks plan limits before allowing resource creation:
     * Before POST /settings/users → check max_users
     * Before POST /frameworks/adopt → check max_frameworks
     * Before POST /risks → check max_risks (if limited)
     * Before POST /integrations → check max_integrations
   - Returns 402 Payment Required with upgrade prompt if limit exceeded

4. internal/handler/onboarding_handler.go — API Endpoints:
   - POST /onboard/signup — create account (email + password + org name)
   - GET /onboard/progress — get current onboarding progress
   - PUT /onboard/step/{n} — save step data
   - POST /onboard/step/{n}/skip — skip optional step
   - POST /onboard/complete — finalise onboarding (the big transaction)
   - GET /onboard/recommendations — get framework recommendations based on step 2 answers
   
   - GET /subscription — current subscription details + usage
   - PUT /subscription/plan — change plan
   - POST /subscription/cancel — cancel
   - GET /subscription/plans — available plans with pricing
   - GET /subscription/usage — detailed usage breakdown
   - POST /subscription/portal — get Stripe billing portal URL

5. NEXT.JS FRONTEND:
   - /onboard — full-screen wizard (no sidebar, step-by-step flow):
     * Progress bar showing 7 steps
     * Each step is a distinct page with back/next navigation
     * Step 2: interactive questionnaire with animated framework recommendations appearing
     * Step 3: framework cards with drag-to-reorder priority, cross-mapping percentages
     * Step 4: team invitation form with role suggestions
     * Step 6: quick assessment with live score animation
     * Step 7: summary with animated launch sequence
   - /settings/subscription — subscription management:
     * Current plan details with usage meters (users: 5/25, frameworks: 3/5, etc.)
     * Plan comparison table (Starter vs Professional vs Enterprise)
     * Upgrade/downgrade buttons with confirmation
     * Billing history
     * Cancel flow with retention offers

SEED DATA:
- 3 subscription plans with pricing:
  * Starter: €99/mo (€990/yr) — 5 users, 3 frameworks, basic reports
  * Professional: €299/mo (€2,990/yr) — 25 users, 5 frameworks, all reports, SSO, API access
  * Enterprise: €799/mo (€7,990/yr) — 100 users, 9 frameworks, everything + custom branding + SLA

CRITICAL REQUIREMENTS:
- Onboarding MUST complete within 30 minutes for an average user
- The "Launch" transaction MUST be atomic — if any step fails, roll everything back
- Plan limits enforced at the middleware level (not just frontend validation)
- Trial period: 14 days, full Enterprise features, no credit card required
- Usage metering is eventual-consistent (async event recording)
- Downgrade validation: prevent downgrade if current usage exceeds new plan limits
- Framework recommendations algorithm documented and testable

OUTPUT: Complete Golang code for onboarding wizard service, subscription service, plan limits middleware, migration, seed plans, and Next.js onboarding wizard pages (7 steps). Include unit tests for the recommendation engine and plan limit checks.
```

---

### PROMPT 20 OF 100 — Advanced RBAC with Attribute-Based Access Control (ABAC)

```
You are a senior Golang backend engineer building the advanced access control system for "ComplianceForge" — an enterprise GRC platform.

OBJECTIVE:
Extend the existing RBAC system (Prompt 2) with Attribute-Based Access Control (ABAC) to support fine-grained, context-aware permissions. European enterprises need access control that goes beyond role-based: a Control Owner should only see controls they own, a DPO should see all personal data incidents but not operational incidents, a Regional Compliance Manager should only see entities in their region, and an External Auditor should see audit data read-only during the audit engagement period only.

The ABAC engine must evaluate policies based on: subject attributes (user role, department, region, clearance level), resource attributes (classification, owner, framework, risk level), action (CRUD + approve + export), and environment attributes (time of day, IP range, MFA status).

DATABASE SCHEMA — Create migration 015:

TABLE access_policies:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL
  - description TEXT
  - priority INT DEFAULT 100 — lower number = higher priority
  - effect ENUM('allow', 'deny') NOT NULL
  - is_active BOOLEAN DEFAULT true
  
  -- Subject conditions (WHO)
  - subject_conditions JSONB NOT NULL — [{"attribute": "role", "operator": "in", "values": ["auditor"]}, {"attribute": "department", "operator": "equals", "value": "Internal Audit"}]
  
  -- Resource conditions (WHAT)
  - resource_type VARCHAR(100) NOT NULL — 'risk', 'policy', 'control', 'incident', 'vendor', 'asset', 'audit', 'finding', 'report', '*'
  - resource_conditions JSONB — [{"attribute": "classification", "operator": "in", "values": ["public", "internal"]}, {"attribute": "owner_user_id", "operator": "equals_subject", "subject_attribute": "user_id"}]
  
  -- Action conditions (HOW)
  - actions TEXT[] NOT NULL — ['read', 'create', 'update', 'delete', 'approve', 'export', 'assign']
  
  -- Environment conditions (WHEN/WHERE)
  - environment_conditions JSONB — [{"attribute": "mfa_verified", "operator": "equals", "value": true}, {"attribute": "ip_range", "operator": "in_cidr", "value": "10.0.0.0/8"}, {"attribute": "time", "operator": "between", "values": ["09:00", "18:00"]}]
  
  -- Temporal constraints
  - valid_from TIMESTAMPTZ — policy only active from this date (for temporary access like external auditors)
  - valid_until TIMESTAMPTZ — policy expires after this date
  
  - created_by UUID FK → users
  - created_at, updated_at

TABLE access_policy_assignments:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - access_policy_id UUID FK → access_policies
  - assignee_type ENUM('user', 'role', 'group', 'all_users')
  - assignee_id UUID — user_id or role_id (NULL for 'all_users')
  - valid_from TIMESTAMPTZ
  - valid_until TIMESTAMPTZ
  - created_by UUID FK
  - created_at

TABLE access_audit_log:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - user_id UUID FK
  - action VARCHAR(50)
  - resource_type VARCHAR(100)
  - resource_id UUID
  - decision ENUM('allow', 'deny')
  - matched_policy_id UUID FK → access_policies
  - evaluation_time_us INT — microseconds for the policy evaluation
  - subject_attributes JSONB — snapshot of subject attributes at evaluation time
  - resource_attributes JSONB — snapshot of resource attributes
  - environment_attributes JSONB — snapshot of environment (IP, time, MFA)
  - created_at

TABLE field_level_permissions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - access_policy_id UUID FK → access_policies
  - resource_type VARCHAR(100)
  - field_name VARCHAR(100) — e.g., 'financial_impact_eur', 'data_subject_name', 'password_hash'
  - permission ENUM('visible', 'masked', 'hidden') DEFAULT 'visible'
  - mask_pattern VARCHAR(50) — e.g., '****', 'J*** S****' for names
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/abac_engine.go — Policy Decision Point (PDP):

   ABACEngine struct with methods:
   
   - Evaluate(ctx, request AccessRequest) → AccessDecision
     AccessRequest: {SubjectID, Action, ResourceType, ResourceID}
     AccessDecision: {Effect: allow/deny, PolicyID, Reason}
   
   Evaluation algorithm (DENY-overrides):
   a. Collect all active policies assigned to the user (via direct assignment, role, group)
   b. Filter policies by resource_type matching
   c. Filter by action matching
   d. Filter by temporal constraints (valid_from/valid_until)
   e. For remaining policies, evaluate conditions:
      - Subject conditions: check user attributes against conditions
      - Resource conditions: fetch resource attributes, check against conditions
      - Environment conditions: check current context (IP, time, MFA status)
   f. If ANY matching policy has effect='deny' → DENY (deny-overrides)
   g. If at least one matching policy has effect='allow' → ALLOW
   h. If no matching policy → DENY (default deny)
   i. Log decision in access_audit_log

   Special operators:
   - 'equals_subject': compare resource attribute to a subject attribute (e.g., resource.owner_user_id == subject.user_id)
   - 'in_cidr': check if IP is within a CIDR range
   - 'between': time range check (with timezone awareness)
   - 'contains_any': array intersection check
   - 'not_in': exclusion check

   Performance optimisation:
   - Cache compiled policies in Redis per org (invalidate on policy change)
   - Pre-fetch user attributes and cache for the request duration
   - Lazy-load resource attributes only when needed
   - Target: <1ms average evaluation time

2. internal/middleware/abac.go — Policy Enforcement Point (PEP):

   ABAC middleware that wraps route handlers:
   ```go
   func ABAC(engine *ABACEngine, action string, resourceType string) func(http.Handler) http.Handler
   ```
   
   Usage in router:
   ```go
   r.With(ABAC(abacEngine, "read", "risk")).Get("/risks", riskH.ListRisks)
   r.With(ABAC(abacEngine, "create", "risk")).Post("/risks", riskH.CreateRisk)
   r.With(ABAC(abacEngine, "export", "report")).Get("/reports/compliance", reportH.ComplianceReport)
   ```

   For list endpoints (GET /risks):
   - ABAC evaluates "can this user read risks at all?"
   - If allowed, the repository layer applies resource-level filters (e.g., only risks owned by user)
   - This is done by injecting ABAC filter conditions into the SQL WHERE clause

3. internal/service/abac_filter.go — Resource-Level Filtering:

   ABACFilter: generates SQL WHERE clause fragments based on user's access policies
   
   Example: if user has policy "can read risks WHERE owner_user_id = current_user_id":
   → generates: "AND r.owner_user_id = $N" with user's ID as parameter
   
   Example: if user has policy "can read incidents WHERE classification IN ('public', 'internal')":
   → generates: "AND i.classification IN ('public', 'internal')"
   
   This is injected into repository list queries transparently.

4. internal/service/field_masker.go — Field-Level Security:

   FieldMasker: after fetching data, applies field-level permissions:
   - 'visible': no change
   - 'masked': replace with mask pattern (e.g., "John Smith" → "J*** S****")
   - 'hidden': remove field from JSON response entirely
   
   Applied as a response middleware or in the handler before JSON encoding.
   
   Use cases:
   - External auditors: financial_impact_eur masked in risk register
   - Non-DPO users: data subject names masked in incident details
   - Viewers: cannot see remediation_plan text in findings

5. internal/handler/access_handler.go — API Endpoints:
   - GET /access/policies — list access policies (admin)
   - POST /access/policies — create policy
   - PUT /access/policies/{id} — update policy
   - DELETE /access/policies/{id} — delete policy
   - POST /access/policies/{id}/assignments — assign policy to user/role
   - DELETE /access/policies/{id}/assignments/{assignmentId} — remove assignment
   - POST /access/evaluate — test policy evaluation (admin diagnostic tool)
   - GET /access/audit-log — access decision audit log
   - GET /access/my-permissions — what can the current user do? (for frontend UI rendering)
   - GET /access/field-permissions — field-level permissions for a resource type

6. SEED — Default Access Policies:

   a. "Org Admin — Full Access":
      Subject: role IN ['org_admin']
      Resource: *
      Actions: all
      Effect: allow

   b. "Control Owner — Own Controls Only":
      Subject: role IN ['control_owner']
      Resource: control_implementation WHERE owner_user_id = current_user_id
      Actions: ['read', 'update']
      Effect: allow

   c. "DPO — Privacy Incidents":
      Subject: role IN ['dpo']
      Resource: incident WHERE is_data_breach = true OR incident_type = 'privacy'
      Actions: ['read', 'update', 'approve']
      Effect: allow

   d. "DPO — All DSR Requests":
      Subject: role IN ['dpo']
      Resource: dsr_request
      Actions: all
      Effect: allow

   e. "External Auditor — Read-Only During Engagement":
      Subject: role IN ['external_auditor']
      Resource: audit, finding, control_implementation, control_evidence
      Actions: ['read']
      Effect: allow
      Valid: engagement start date → engagement end date
      Environment: MFA required

   f. "Viewer — Read-Only, No Confidential":
      Subject: role IN ['viewer']
      Resource: * WHERE classification NOT IN ['confidential', 'restricted']
      Actions: ['read']
      Effect: allow

   g. "Export Restriction — No Export After Hours":
      Subject: all_users
      Resource: report
      Actions: ['export']
      Environment: time NOT BETWEEN 09:00 AND 18:00
      Effect: deny

   h. "Field Masking — Financial Data for Non-Managers":
      Resource: risk
      Field: financial_impact_eur
      Permission: masked
      Subject: role NOT IN ['org_admin', 'risk_manager', 'ciso']

7. NEXT.JS FRONTEND:
   - /settings/access-policies — admin page:
     * Policy list with effect (allow/deny badge), assigned users/roles, resource type, actions
     * Policy builder form:
       - Subject conditions builder (add/remove conditions with attribute/operator/value selectors)
       - Resource type selector with resource conditions builder
       - Action checkboxes (read, create, update, delete, approve, export)
       - Environment conditions builder (MFA, IP range, time window)
       - Temporal constraints (valid from/until date pickers)
       - Effect toggle (allow/deny)
     * Assignment panel: assign policy to users or roles
     * Test evaluation: "Can [user] [action] [resource type] [resource ID]?" → shows allow/deny + matched policy
   - Access audit log viewer: filterable by user, action, decision, resource type
   - Frontend permission-aware rendering:
     * Use GET /access/my-permissions to determine what the current user can do
     * Hide UI elements the user cannot interact with (e.g., hide "Delete" button if no delete permission)
     * Disable "Export" button if export not permitted
     * Show masked fields with asterisks (don't hide — show that the data exists but is masked)

CRITICAL REQUIREMENTS:
- DENY always overrides ALLOW (deny-overrides combining algorithm per XACML/ABAC standard)
- Default deny: if no policy explicitly allows an action, it's denied
- Policy evaluation must be <5ms average (cache aggressively)
- The existing RBAC (role-based) continues to work — ABAC is additive, not replacing RBAC
- External auditor access must have temporal boundaries (auto-expire after audit engagement ends)
- All access decisions logged immutably in access_audit_log for regulatory compliance
- Field masking must not affect backend processing — only JSON serialisation to the API consumer
- The ABAC engine must be testable: given a set of policies and a request, assert the decision
- Policy changes take effect within 30 seconds (Redis cache TTL)
- Circular or conflicting policies detected and flagged at creation time

OUTPUT: Complete Golang code for ABAC engine (PDP), middleware (PEP), SQL filter generator, field masker, migration, seed policies, handlers, and Next.js access policy management pages. Include comprehensive unit tests for the evaluation engine covering: basic allow/deny, deny-overrides, subject matching, resource conditions, environment conditions, temporal constraints, field-level masking, and the SQL filter generator.
```

---

## BATCH 4 SUMMARY

| Prompt | Focus Area | New Tables | New Endpoints | Key Capabilities |
|--------|-----------|------------|---------------|------------------|
| 16 | Workflow Engine | 5 (definitions, steps, instances, executions, delegations) | ~18 | Multi-step approval chains, conditional routing, parallel approvals, SLA enforcement, escalation, delegation, 5 pre-built compliance workflows |
| 17 | Integration Hub | 4 (integrations, sync_logs, sso_configs, api_keys) | ~20 | SAML 2.0 / OIDC SSO, AWS/Azure/GCP evidence collection, Splunk/Elastic SIEM, ServiceNow/Jira ITSM, webhook system, API key management |
| 18 | i18n Localisation | 0 (config-based) | ~3 | 9 EU languages (EN, DE, FR, ES, IT, NL, PT, PL, SV), date/number/currency formatting, notification templates in all languages, RTL preparation |
| 19 | Onboarding Wizard | 4 (plans, subscriptions, onboarding_progress, usage_events) | ~12 | 7-step guided onboarding, framework recommendation engine, subscription plans with Stripe, plan limit enforcement middleware, 14-day free trial |
| 20 | Advanced ABAC | 4 (access_policies, assignments, audit_log, field_permissions) | ~10 | Attribute-based access control, deny-overrides evaluation, SQL filter injection for list endpoints, field-level masking, temporal access (external auditors), 8 pre-built policies |

**Running Total: 20/100 Prompts | ~75 Tables | ~160+ API Endpoints | Enterprise-Grade Security & Operations**

---

> **NEXT BATCH (Prompts 21–25):** Compliance Gap Remediation Planner (AI-assisted remediation suggestions), Control Library Marketplace (share/import control frameworks), Regulatory Change Management (track regulation updates), Business Impact Analysis (BIA) Module, and Advanced Analytics & BI Dashboard (compliance trends, predictive risk scoring).
>
> Type **"next"** to continue.
