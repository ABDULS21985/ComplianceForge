# GRC Compliance Management Solution — 100 Master Prompts

## BATCH 5 — AI-Assisted Remediation, Control Library Marketplace, Regulatory Change Management, Business Impact Analysis & Advanced Analytics

**Stack:** Golang 1.22+ | PostgreSQL 16 | Redis 7 | Next.js 14 | Claude API (Anthropic) | WebSockets
**Prerequisite:** All previous batches (Prompts 1–20) completed
**Deliverable:** AI-powered compliance intelligence, community-driven control libraries, regulatory horizon scanning, business continuity planning, and predictive risk analytics

---

### PROMPT 21 OF 100 — AI-Assisted Compliance Remediation Planner

```
You are a senior Golang backend engineer building the AI-powered compliance remediation planning system for "ComplianceForge" — a GRC platform targeting European enterprises.

OBJECTIVE:
Build an intelligent remediation planning engine that uses AI (Claude API) to analyse compliance gaps, generate prioritised remediation plans, suggest implementation guidance per control, estimate effort/cost, and recommend evidence templates. This transforms gap analysis from a static report into an actionable, AI-assisted implementation roadmap. The AI layer wraps the Anthropic Claude API and is optional — all features must degrade gracefully when AI is unavailable.

DATABASE SCHEMA — Create migration 016:

TABLE remediation_plans:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - plan_ref VARCHAR(20) NOT NULL UNIQUE — RMP-2026-0001
  - name VARCHAR(300) NOT NULL
  - description TEXT
  - plan_type ENUM('gap_remediation', 'audit_finding_response', 'risk_treatment', 'certification_preparation', 'regulatory_response', 'custom')
  - status ENUM('draft', 'in_review', 'approved', 'in_progress', 'completed', 'cancelled')
  - scope_framework_ids UUID[] — which frameworks this plan covers
  - scope_description TEXT
  - priority ENUM('critical', 'high', 'medium', 'low')
  
  -- AI Generation Metadata
  - ai_generated BOOLEAN DEFAULT false
  - ai_model VARCHAR(100) — e.g., 'claude-sonnet-4-20250514'
  - ai_prompt_summary TEXT — what was asked
  - ai_generation_date TIMESTAMPTZ
  - ai_confidence_score DECIMAL(3,2) — 0.00–1.00
  - human_reviewed BOOLEAN DEFAULT false
  - human_reviewed_by UUID FK → users
  - human_reviewed_at TIMESTAMPTZ
  
  -- Planning
  - target_completion_date DATE
  - estimated_total_hours DECIMAL(10,1)
  - estimated_total_cost_eur DECIMAL(12,2)
  - actual_completion_date DATE
  - completion_percentage DECIMAL(5,2) DEFAULT 0
  
  - owner_user_id UUID FK → users
  - created_by UUID FK → users
  - approved_by UUID FK → users
  - approved_at TIMESTAMPTZ
  - metadata JSONB
  - created_at, updated_at

TABLE remediation_actions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - plan_id UUID FK → remediation_plans
  - action_ref VARCHAR(20) — RMP-0001-A01
  - sort_order INT
  - title VARCHAR(500) NOT NULL
  - description TEXT NOT NULL
  - action_type ENUM('implement_control', 'update_policy', 'deploy_technology', 'conduct_training', 'perform_assessment', 'create_documentation', 'configure_system', 'engage_vendor', 'other')
  
  -- Linkage
  - linked_control_implementation_id UUID FK → control_implementations
  - linked_finding_id UUID FK → audit_findings
  - linked_risk_treatment_id UUID FK → risk_treatments
  - framework_control_code VARCHAR(50) — e.g., 'A.8.9', 'AC-6'
  
  -- Planning
  - priority ENUM('critical', 'high', 'medium', 'low')
  - estimated_hours DECIMAL(8,1)
  - estimated_cost_eur DECIMAL(10,2)
  - required_skills TEXT[] — e.g., {'network_engineering', 'policy_writing', 'cloud_security'}
  - dependencies UUID[] — other action IDs that must complete first
  - assigned_to UUID FK → users
  - target_start_date DATE
  - target_end_date DATE
  
  -- Status
  - status ENUM('not_started', 'in_progress', 'blocked', 'completed', 'cancelled', 'deferred')
  - actual_start_date DATE
  - actual_end_date DATE
  - actual_hours DECIMAL(8,1)
  - actual_cost_eur DECIMAL(10,2)
  - completion_notes TEXT
  - evidence_paths TEXT[]
  
  -- AI Guidance
  - ai_implementation_guidance TEXT — detailed AI-generated step-by-step guidance
  - ai_evidence_suggestions TEXT[] — what evidence to collect
  - ai_tool_recommendations TEXT[] — tools/products that help implement
  - ai_risk_if_deferred TEXT — AI assessment of risk if this action is deferred
  - ai_cross_framework_benefit TEXT — what other frameworks benefit from this action
  
  - created_at, updated_at

TABLE ai_interaction_logs:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - interaction_type VARCHAR(100) — 'remediation_plan', 'control_guidance', 'gap_analysis', 'risk_assessment', 'policy_draft', 'evidence_suggestion'
  - prompt_text TEXT NOT NULL — the prompt sent to Claude (with org data redacted)
  - response_text TEXT NOT NULL — the AI response
  - model VARCHAR(100)
  - input_tokens INT
  - output_tokens INT
  - latency_ms INT
  - cost_eur DECIMAL(8,4) — estimated API cost
  - user_id UUID FK → users
  - rating INT — 1-5 user rating of the response quality
  - feedback TEXT
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/ai_service.go — AI Integration Layer:

   AIService struct wrapping the Anthropic Claude API:
   
   - Client configuration: API key from config (encrypted), model selection, rate limiting
   - Request/response logging: every AI call logged in ai_interaction_logs (with PII scrubbed)
   - Graceful degradation: if AI unavailable, return fallback guidance from a static knowledge base
   - Cost tracking: estimate token costs, enforce per-org monthly budget
   - Rate limiting: max 100 AI calls per org per hour (configurable per plan)
   
   Methods:
   
   a. GenerateRemediationPlan(ctx, orgID, request) → RemediationPlan:
      - Input: list of compliance gaps (control code, title, current status, risk level), frameworks in scope, org industry, org size
      - Prompt engineering: structured prompt that asks Claude to:
        * Prioritise gaps by risk impact and implementation dependencies
        * Group related actions (e.g., implementing A.5.15 and AC-3 can share work)
        * Estimate effort in hours per action (calibrated by org size)
        * Suggest implementation order considering dependencies
        * Identify cross-framework benefits ("implementing this also covers...")
        * Provide specific, actionable steps (not generic advice)
      - Output: structured JSON remediation plan with prioritised actions
      - Parse Claude's response into RemediationPlan struct
      - Store in database with ai_generated=true
   
   b. GenerateControlGuidance(ctx, orgID, controlCode, controlTitle, frameworkCode, orgContext) → ControlGuidance:
      - Input: specific control details, org industry, org size, current implementation status
      - Prompt: "How should a {industry} company of {size} implement {controlCode}: {controlTitle}?"
      - Output: step-by-step implementation guidance, evidence suggestions, tool recommendations, common pitfalls
      - This powers the "AI Assist" button on every control implementation page
   
   c. AnalyseGapImpact(ctx, orgID, gaps) → GapImpactAnalysis:
      - Input: list of unimplemented controls
      - Output: risk assessment of the gaps, regulatory exposure analysis, recommended prioritisation
   
   d. SuggestEvidenceTemplate(ctx, controlCode, controlTitle) → EvidenceTemplate:
      - Input: control details
      - Output: what evidence documents to collect, template outlines, collection frequency guidance
   
   e. DraftPolicySection(ctx, orgID, policyType, section, orgContext) → PolicyDraft:
      - Input: policy type (e.g., "Information Security Policy"), section (e.g., "Access Control"), org context
      - Output: draft policy text suitable for the organisation's context
      - IMPORTANT: AI-generated policy text must be clearly marked as AI-generated and requires human review
   
   f. AssessRiskNarrative(ctx, orgID, riskTitle, riskDescription, orgContext) → RiskNarrative:
      - Input: risk details
      - Output: threat analysis, potential impact scenarios, recommended treatment options, likelihood assessment rationale

2. internal/service/remediation_planner.go — Plan Generation & Tracking:
   
   - GeneratePlan(ctx, orgID, request) → creates a remediation plan:
     * If AI enabled: call AIService.GenerateRemediationPlan, then enhance with database cross-references
     * If AI disabled: use rule-based prioritisation (sort by risk_if_not_implemented, group by framework domain)
     * Either way: create remediation_plan + remediation_actions records
     * Link actions to existing control_implementations
     * Calculate dependency chains using topological sort
     * Generate a Gantt-chart-ready timeline
   
   - TrackProgress(ctx, orgID, planID) → progress metrics:
     * Total actions, completed, in-progress, blocked, not started
     * Estimated vs actual hours/cost
     * Critical path analysis: which incomplete actions are blocking the most downstream work
     * Projected completion date based on current velocity
   
   - RecalculateTimeline(ctx, orgID, planID) → after status changes, recalculate dates considering dependencies

3. internal/handler/remediation_handler.go — API Endpoints:
   - GET /remediation/plans — list remediation plans
   - POST /remediation/plans — create plan (manual)
   - POST /remediation/plans/generate — AI-generate a plan from current gaps
   - GET /remediation/plans/{id} — plan detail with actions
   - PUT /remediation/plans/{id} — update plan
   - POST /remediation/plans/{id}/approve — approve plan
   - GET /remediation/plans/{id}/timeline — Gantt timeline data
   - GET /remediation/plans/{id}/progress — progress metrics
   
   - PUT /remediation/actions/{id} — update action status
   - POST /remediation/actions/{id}/complete — complete with evidence
   
   - POST /ai/control-guidance — get AI guidance for a specific control
   - POST /ai/evidence-suggestion — get AI evidence suggestions
   - POST /ai/policy-draft — get AI policy draft
   - POST /ai/risk-narrative — get AI risk assessment narrative
   - GET /ai/usage — AI usage stats for the org (calls, tokens, cost)
   - POST /ai/feedback — rate an AI response

4. PROMPT ENGINEERING — Detailed prompt templates:

   a. Remediation Plan Prompt:
   ```
   You are a cybersecurity compliance expert advising a {industry} company with {employee_count} employees in {country}.

   The organisation has adopted the following frameworks: {frameworks_list}

   The following compliance gaps have been identified:
   {gap_list — control_code, title, framework, current_status, risk_if_not_implemented}

   Generate a prioritised remediation plan with the following structure for EACH gap:
   1. Priority rank (1 = most urgent) based on: risk level, regulatory penalty exposure, ease of implementation
   2. Recommended implementation approach (3–5 specific, actionable steps)
   3. Estimated effort in person-hours (calibrated for a {employee_count}-person company)
   4. Required skills/roles
   5. Dependencies (which other controls should be implemented first)
   6. Cross-framework benefit (what other frameworks are partially satisfied by implementing this)
   7. Evidence to collect after implementation
   8. Common pitfalls to avoid

   Group related controls that can share implementation effort.
   Consider industry-specific requirements for {industry}.
   
   Respond in JSON format: {"actions": [{...}]}
   ```

   b. Control Guidance Prompt:
   ```
   You are a cybersecurity implementation specialist. Provide practical implementation guidance for:

   Control: {control_code} — {control_title}
   Framework: {framework_name}
   Organisation: {industry}, {employee_count} employees, based in {country}
   Current status: {current_status}

   Provide:
   1. Step-by-step implementation guide (5–10 specific steps)
   2. Recommended tools/technologies
   3. Evidence documents to prepare for auditors
   4. Time estimate for implementation
   5. Common mistakes to avoid
   6. How this control maps to other frameworks the org uses: {other_frameworks}

   Be specific and actionable — avoid generic advice.
   ```

5. STATIC FALLBACK KNOWLEDGE BASE (when AI is unavailable):
   - Create a JSON knowledge base with basic implementation guidance for:
     * All 93 ISO 27001 Annex A controls
     * Top 50 NIST 800-53 controls
     * All 12 PCI DSS requirements
   - Each entry: {control_code, basic_guidance, typical_evidence, estimated_hours_range}
   - This ensures the feature works (at reduced quality) without AI credits

6. NEXT.JS FRONTEND:
   - /remediation page:
     * Plan list: ref, name, status badge, frameworks, progress bar, target date, owner
     * "Generate AI Plan" button → wizard:
       Step 1: Select frameworks to cover
       Step 2: Review detected gaps (from GET /compliance/gaps)
       Step 3: AI generates the plan (show loading animation with "AI is analysing {N} compliance gaps...")
       Step 4: Review AI-generated plan — user can edit priorities, estimates, assignees
       Step 5: Approve and activate the plan
     * Plan detail page:
       - Gantt chart / timeline view of actions with dependencies
       - Kanban board view (Not Started | In Progress | Blocked | Completed)
       - Progress dashboard: completion %, estimated vs actual, critical path
       - Action detail panel: AI guidance, evidence suggestions, status update form
   
   - Control implementation page enhancement:
     * "AI Assist" button on every control → opens a panel with:
       - AI-generated implementation guidance (streamed, appearing in real-time)
       - Evidence suggestions
       - Cross-framework impact analysis
       - "Was this helpful?" feedback (1-5 stars)
   
   - Policy draft page enhancement:
     * "AI Draft" button → generates a draft policy section using org context
     * AI output clearly marked: "This section was AI-generated and requires human review"
     * Accept / Edit / Regenerate buttons
   
   - AI Usage dashboard (admin):
     * Total AI calls this month
     * Token usage (input + output)
     * Estimated cost
     * Average response rating
     * Most requested features

CRITICAL REQUIREMENTS:
- AI responses MUST be clearly labelled as AI-generated throughout the UI
- AI-generated remediation plans MUST go through human approval workflow before activation
- NEVER send actual personal data (PII) to the AI — only org metadata, control codes, and industry context
- AI API key stored encrypted, separate from other secrets
- Rate limiting: Starter plan = 50 AI calls/month, Professional = 500, Enterprise = unlimited
- Fallback: every AI-powered feature works (with reduced quality) when AI is unavailable
- AI response caching: identical queries within 24 hours return cached response (save API costs)
- Token budget: max 4096 output tokens per request (prevent runaway costs)
- The prompt templates must be configurable per organisation (admin can adjust prompts)
- All AI interactions logged for audit and quality improvement

OUTPUT: Complete Golang code for AI service, remediation planner, handlers, migration, prompt templates, static fallback knowledge base (JSON for 93 ISO controls), and Next.js remediation pages + AI assist components. Include unit tests for the plan generation logic and prompt builder.
```

---

### PROMPT 22 OF 100 — Control Library Marketplace & Framework Template Exchange

```
You are a senior Golang backend engineer building the control library marketplace for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a marketplace where organisations can share, publish, and import custom control libraries, framework mappings, policy templates, and compliance playbooks. This creates a community-driven ecosystem where consulting firms can publish industry-specific control sets, auditors can share best-practice mappings, and organisations can import pre-built compliance packages to accelerate their implementation. Think "Terraform Registry" but for compliance controls.

DATABASE SCHEMA — Create migration 017:

TABLE marketplace_publishers:
  - id UUID PK
  - organization_id UUID FK — the publishing org
  - publisher_name VARCHAR(200) NOT NULL
  - publisher_slug VARCHAR(100) NOT NULL UNIQUE
  - description TEXT
  - website VARCHAR(500)
  - logo_url TEXT
  - is_verified BOOLEAN DEFAULT false — Digibit/ComplianceForge verified publisher
  - verification_date DATE
  - is_official BOOLEAN DEFAULT false — official ComplianceForge content
  - total_packages INT DEFAULT 0
  - total_downloads INT DEFAULT 0
  - rating_avg DECIMAL(3,2) DEFAULT 0
  - rating_count INT DEFAULT 0
  - contact_email VARCHAR(300)
  - created_at, updated_at

TABLE marketplace_packages:
  - id UUID PK
  - publisher_id UUID FK → marketplace_publishers
  - package_slug VARCHAR(200) NOT NULL — e.g., 'fintech-uk-gdpr-controls'
  - name VARCHAR(300) NOT NULL
  - description TEXT
  - long_description TEXT — markdown
  - package_type ENUM('control_library', 'framework_mapping', 'policy_template_pack', 'compliance_playbook', 'risk_library', 'evidence_template_pack', 'assessment_questionnaire', 'custom_framework')
  - category VARCHAR(100) — 'financial_services', 'healthcare', 'technology', 'government', 'manufacturing', 'energy', 'retail', 'general'
  - applicable_frameworks TEXT[] — which base frameworks this extends/customises
  - applicable_regions TEXT[] — ['EU', 'UK', 'DACH', 'Nordics', 'Global']
  - applicable_industries TEXT[] — target industries
  - tags TEXT[]
  
  -- Versioning
  - current_version VARCHAR(20) NOT NULL — semver: '1.2.0'
  - min_platform_version VARCHAR(20) — minimum ComplianceForge version required
  
  -- Pricing
  - pricing_model ENUM('free', 'one_time', 'subscription') DEFAULT 'free'
  - price_eur DECIMAL(10,2) DEFAULT 0
  
  -- Stats
  - download_count INT DEFAULT 0
  - install_count INT DEFAULT 0 — active installations
  - rating_avg DECIMAL(3,2) DEFAULT 0
  - rating_count INT DEFAULT 0
  - featured BOOLEAN DEFAULT false
  
  -- Content (what's included)
  - contents_summary JSONB — {"controls": 45, "mappings": 120, "policies": 8, "evidence_templates": 25}
  
  - status ENUM('draft', 'published', 'deprecated', 'removed')
  - published_at TIMESTAMPTZ
  - deprecated_at TIMESTAMPTZ
  - deprecation_message TEXT
  - license VARCHAR(100) DEFAULT 'CC-BY-4.0'
  
  - created_at, updated_at
  - UNIQUE(publisher_id, package_slug)

TABLE marketplace_package_versions:
  - id UUID PK
  - package_id UUID FK → marketplace_packages
  - version VARCHAR(20) NOT NULL — semver
  - release_notes TEXT — markdown changelog
  - package_data JSONB NOT NULL — the actual importable content:
    {
      "controls": [{"code": "...", "title": "...", "description": "...", ...}],
      "mappings": [{"source": "A.5.1", "target": "CUSTOM-01", "type": "equivalent", "strength": 0.9}],
      "policies": [{"title": "...", "content_html": "...", ...}],
      "evidence_templates": [{"name": "...", "description": "...", "fields": [...]}],
      "risk_categories": [{"name": "...", "description": "..."}]
    }
  - package_hash VARCHAR(128) — SHA-256 of package_data for integrity
  - file_size_bytes BIGINT
  - is_breaking_change BOOLEAN DEFAULT false
  - migration_notes TEXT — how to upgrade from previous version
  - published_at TIMESTAMPTZ
  - created_at

TABLE marketplace_installations:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - package_id UUID FK → marketplace_packages
  - version_id UUID FK → marketplace_package_versions
  - installed_version VARCHAR(20)
  - status ENUM('installed', 'update_available', 'updating', 'uninstalled')
  - installed_at TIMESTAMPTZ
  - installed_by UUID FK → users
  - updated_at TIMESTAMPTZ
  - uninstalled_at TIMESTAMPTZ
  - configuration JSONB — org-specific customisation applied during import
  - import_summary JSONB — {"controls_imported": 45, "mappings_imported": 120, "conflicts_resolved": 3}
  - created_at, updated_at
  - UNIQUE(organization_id, package_id)

TABLE marketplace_reviews:
  - id UUID PK
  - package_id UUID FK → marketplace_packages
  - organization_id UUID FK (RLS)
  - user_id UUID FK → users
  - rating INT NOT NULL CHECK (rating BETWEEN 1 AND 5)
  - title VARCHAR(200)
  - review_text TEXT
  - helpful_count INT DEFAULT 0
  - is_verified_install BOOLEAN DEFAULT false — reviewer actually installed the package
  - created_at, updated_at
  - UNIQUE(package_id, organization_id) — one review per org per package

GOLANG IMPLEMENTATION:

1. internal/service/marketplace_service.go:

   Publishing:
   - CreatePublisher(ctx, orgID, req) — register as a marketplace publisher
   - CreatePackage(ctx, publisherID, req) — create a new package
   - PublishVersion(ctx, packageID, version, packageData) — publish a version:
     * Validate package_data schema (controls have required fields, mappings reference valid codes)
     * Calculate SHA-256 hash
     * Store in marketplace_package_versions
     * Update marketplace_packages.current_version
     * Notify all installed orgs of the update
   - DeprecatePackage(ctx, packageID, message)
   
   Discovery:
   - SearchPackages(ctx, query, filters) — full-text search with faceted filtering:
     * Filter by: type, category, region, industry, framework, pricing, rating
     * Sort by: relevance, downloads, rating, newest, featured
     * Paginated results
   - GetFeaturedPackages(ctx) — curated featured packages
   - GetPackageDetail(ctx, publisherSlug, packageSlug) — full detail with versions, reviews, stats
   - GetPackagesByFramework(ctx, frameworkCode) — packages related to a specific framework
   
   Installation:
   - InstallPackage(ctx, orgID, packageID, versionID, config) → single transaction:
     * Download package_data from version
     * Validate compatibility with org's current setup
     * Import controls: create framework_controls with a custom framework reference
     * Import mappings: create framework_control_mappings linking to existing controls
     * Import policies: create policy records in draft status
     * Import evidence templates: store as templates for future use
     * Handle conflicts: if a control code already exists, offer merge/skip/override
     * Record in marketplace_installations
     * Increment download/install counts
   - UninstallPackage(ctx, orgID, packageID) — soft-remove imported content
   - UpdatePackage(ctx, orgID, packageID) — upgrade to latest version:
     * Diff old version vs new version
     * Apply additions, mark removals, handle modifications
     * Preserve org-specific customisations
   
   Reviews:
   - SubmitReview(ctx, orgID, userID, packageID, rating, title, text) — verify install before allowing review
   - GetReviews(ctx, packageID, page)

2. internal/service/package_builder.go — Export from Org:

   ExportAsPackage(ctx, orgID, config) → generates a marketplace-compatible package from an org's customisations:
   - Select which custom controls to include
   - Select which custom mappings to include
   - Select which policy templates to include
   - Strip org-specific data (user IDs, org references)
   - Generate package_data JSON
   - Calculate hash
   - Ready for publishing

3. internal/handler/marketplace_handler.go — API Endpoints:

   Public (no auth required):
   - GET /marketplace/packages — search/browse packages
   - GET /marketplace/packages/featured — featured packages
   - GET /marketplace/packages/{publisher}/{slug} — package detail
   - GET /marketplace/packages/{publisher}/{slug}/versions — version history
   - GET /marketplace/packages/{publisher}/{slug}/reviews — reviews

   Authenticated:
   - POST /marketplace/install — install a package
   - DELETE /marketplace/install/{installationId} — uninstall
   - POST /marketplace/install/{installationId}/update — update to latest
   - GET /marketplace/installed — list installed packages
   - POST /marketplace/reviews — submit review

   Publisher (publisher account required):
   - POST /marketplace/publishers — register as publisher
   - GET /marketplace/publishers/me — publisher profile
   - POST /marketplace/publishers/me/packages — create package
   - PUT /marketplace/publishers/me/packages/{id} — update package
   - POST /marketplace/publishers/me/packages/{id}/versions — publish version
   - GET /marketplace/publishers/me/stats — publisher analytics (downloads, ratings, revenue)
   
   Admin:
   - POST /marketplace/publishers/{id}/verify — verify publisher
   - POST /marketplace/packages/{id}/feature — feature a package
   - DELETE /marketplace/packages/{id} — remove package (policy violation)
   - POST /marketplace/export — export org controls as package

4. SEED — Official ComplianceForge Packages:

   a. "UK Financial Services Compliance Pack" (free):
      - 25 additional controls specific to FCA requirements
      - Mappings to ISO 27001, PCI DSS, UK GDPR
      - 5 policy templates (AML, KYC, fraud prevention, data handling, outsourcing)
      - Category: financial_services, Region: UK
   
   b. "Healthcare GDPR Data Protection Pack" (free):
      - 20 controls for health data processing per GDPR Article 9
      - Special category data handling procedures
      - DPIA template for health data
      - Category: healthcare, Region: EU
   
   c. "Cloud Security Controls Pack" (free):
      - 30 controls for cloud-native environments
      - Mappings to ISO 27001, NIST 800-53, CSA CCM
      - Evidence collection configs for AWS/Azure
      - Category: technology, Region: Global

5. NEXT.JS FRONTEND — /marketplace:
   - Marketplace browse page:
     * Search bar with instant results
     * Filter sidebar: type, category, region, industry, framework, pricing, rating
     * Package cards: icon, name, publisher, rating stars, download count, type badge, price
     * Featured carousel at top
   - Package detail page:
     * Header: name, publisher (with verified badge), rating, downloads, price
     * Tabs: Overview (long description markdown), Contents (what's included counts), Versions (changelog), Reviews
     * "Install" button → confirmation dialog showing what will be imported
     * Compatibility check: warns if package requires frameworks not adopted
   - Installed packages page:
     * List of installed packages with version, update availability
     * "Update Available" badge with diff summary
     * Uninstall button with confirmation
   - Publisher portal (for orgs that want to publish):
     * Package management
     * Analytics dashboard (downloads over time, ratings)
     * Export wizard: select org content → package it → publish

CRITICAL REQUIREMENTS:
- Package data validation: every control in a package must have required fields, mappings must reference valid codes
- Integrity: SHA-256 hash verified on install (tamper detection)
- Conflict resolution: when importing controls that conflict with existing ones, offer interactive resolution
- Sandboxing: installed packages cannot modify system frameworks (ISO 27001, NIST, etc.) — they can only add custom controls and mappings TO system frameworks
- Version compatibility: packages declare minimum platform version; install blocked if incompatible
- Review integrity: only verified installations can leave reviews
- Publisher trust: verified badge requires manual review by ComplianceForge team
- Uninstall must cleanly remove all imported content without affecting org-created customisations
- Package size limit: 10MB per version (prevent abuse)
- Rate limiting on marketplace API: 100 requests/minute for browse, 10 installs/hour per org

OUTPUT: Complete Golang code for marketplace service, package builder, handlers, migration, 3 seed packages with realistic content, and Next.js marketplace pages. Include unit tests for package validation, conflict detection, and install/uninstall logic.
```

---

### PROMPT 23 OF 100 — Regulatory Change Management & Horizon Scanning

```
You are a senior Golang backend engineer building the regulatory change management module for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a system that tracks regulatory changes (new laws, regulation updates, standard revisions, guidance publications) across all supported frameworks and jurisdictions, assesses their impact on the organisation's compliance posture, and generates response plans. European enterprises operate across multiple jurisdictions and must stay current with: GDPR enforcement decisions, NIS2 national transpositions, ICO/CNIL/BfDI guidance, ISO standard revisions, PCI SSC bulletins, NIST updates, and industry-specific regulations. This module automates regulatory horizon scanning and impact assessment.

DATABASE SCHEMA — Create migration 018:

TABLE regulatory_sources:
  - id UUID PK
  - name VARCHAR(300) NOT NULL — e.g., 'ICO (UK)', 'CNIL (France)', 'BfDI (Germany)', 'ENISA', 'PCI SSC'
  - source_type ENUM('supervisory_authority', 'standards_body', 'government', 'industry_body', 'legal_publisher', 'custom')
  - country_code VARCHAR(5) — NULL for EU-wide / international
  - region VARCHAR(50) — 'EU', 'UK', 'DACH', 'Nordics', 'Global'
  - url TEXT — official website
  - rss_feed_url TEXT — for automated scanning
  - api_url TEXT — for API-based scanning
  - relevance_frameworks TEXT[] — which frameworks updates from this source affect
  - scan_frequency ENUM('hourly', 'daily', 'weekly') DEFAULT 'daily'
  - last_scanned_at TIMESTAMPTZ
  - is_active BOOLEAN DEFAULT true
  - created_at, updated_at

TABLE regulatory_changes:
  - id UUID PK
  - source_id UUID FK → regulatory_sources
  - change_ref VARCHAR(30) NOT NULL — RC-2026-0001
  - title VARCHAR(500) NOT NULL
  - summary TEXT NOT NULL
  - full_text_url TEXT — link to the original regulation/guidance
  - published_date DATE NOT NULL
  - effective_date DATE — when it takes effect
  - change_type ENUM('new_regulation', 'amendment', 'guidance', 'enforcement_decision', 'standard_revision', 'standard_update', 'industry_bulletin', 'court_ruling', 'consultation')
  - severity ENUM('critical', 'high', 'medium', 'low', 'informational')
  - status ENUM('new', 'under_assessment', 'assessed', 'action_required', 'implemented', 'not_applicable', 'monitoring')
  
  -- Scope
  - affected_frameworks TEXT[] — ['ISO27001', 'UK_GDPR', 'NIS2']
  - affected_regions TEXT[] — ['UK', 'EU', 'DE']
  - affected_industries TEXT[] — ['financial_services', 'healthcare']
  - affected_control_codes TEXT[] — specific controls impacted, e.g., ['A.5.1', 'Art.33']
  
  -- Assessment
  - impact_assessment TEXT — what does this change mean for the organisation
  - impact_level ENUM('none', 'low', 'moderate', 'significant', 'critical')
  - compliance_gap_created BOOLEAN DEFAULT false
  - required_actions TEXT — what the org needs to do
  - deadline DATE — by when must the org comply
  - assessed_by UUID FK → users
  - assessed_at TIMESTAMPTZ
  
  -- Response
  - response_plan_id UUID FK → remediation_plans
  - assigned_to UUID FK → users
  - notes TEXT
  
  - tags TEXT[]
  - metadata JSONB
  - created_at, updated_at

TABLE regulatory_subscriptions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - source_id UUID FK → regulatory_sources
  - is_active BOOLEAN DEFAULT true
  - notification_on_new BOOLEAN DEFAULT true
  - notification_severity_filter TEXT[] — only notify for these severities
  - auto_assess BOOLEAN DEFAULT false — use AI to auto-assess impact
  - created_at

TABLE regulatory_impact_assessments:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - change_id UUID FK → regulatory_changes
  - status ENUM('pending', 'in_progress', 'completed')
  - impact_on_frameworks JSONB — [{"framework": "ISO27001", "impact": "moderate", "affected_controls": ["A.5.1", "A.5.2"]}]
  - gap_analysis JSONB — new gaps created by this change
  - existing_coverage DECIMAL(5,2) — what % of the new requirement the org already meets
  - estimated_effort_hours DECIMAL(8,1)
  - estimated_cost_eur DECIMAL(10,2)
  - ai_assessment TEXT — AI-generated impact summary
  - human_assessment TEXT — human reviewer's assessment
  - assessed_by UUID FK → users
  - assessed_at TIMESTAMPTZ
  - remediation_plan_id UUID FK → remediation_plans
  - created_at, updated_at

GOLANG IMPLEMENTATION:

1. internal/service/regulatory_scanner.go:
   
   - ScanRSSFeeds(ctx) → scan all active RSS feed sources:
     * Fetch RSS feed
     * Parse entries
     * De-duplicate against existing regulatory_changes (by URL or title similarity)
     * Create new regulatory_changes records with status='new'
     * AI-classify: use Claude to determine severity, affected frameworks, and change_type
     * Notify subscribed organisations
   
   - ScanAPIEndpoints(ctx) → for sources with API endpoints (e.g., NIST NVD, PCI SSC API)
   
   - ClassifyChange(ctx, change) → AI-assisted classification:
     * Determine which frameworks are affected
     * Determine severity
     * Generate initial summary if only title/URL available
   
   - AssessImpact(ctx, orgID, changeID) → per-org impact assessment:
     * Compare change requirements against org's current control implementations
     * Identify new gaps created
     * Calculate existing coverage percentage
     * AI-generate impact narrative
     * Estimate remediation effort
   
   - GenerateResponsePlan(ctx, orgID, changeID) → create remediation_plan from assessed impact

2. internal/worker/regulatory_scanner_worker.go:
   - Hourly: scan sources marked as 'hourly' frequency
   - Daily (06:00 UTC): scan all 'daily' sources
   - Weekly (Monday 06:00 UTC): scan all 'weekly' sources
   - For each new change found: classify, notify, auto-assess if configured

3. internal/handler/regulatory_handler.go — API Endpoints:
   - GET /regulatory/changes — browse regulatory changes (global, filterable)
   - GET /regulatory/changes/{id} — change detail
   - POST /regulatory/changes/{id}/assess — submit/trigger impact assessment for this org
   - GET /regulatory/changes/{id}/assessment — get org's impact assessment
   - POST /regulatory/changes/{id}/respond — create response plan
   - GET /regulatory/sources — list regulatory sources
   - POST /regulatory/sources — add custom source
   - GET /regulatory/subscriptions — org's subscriptions
   - POST /regulatory/subscriptions — subscribe to a source
   - GET /regulatory/dashboard — regulatory change dashboard (new changes, pending assessments, upcoming deadlines)
   - GET /regulatory/timeline — chronological timeline of changes affecting the org

4. SEED — Regulatory Sources (20+):
   - UK: ICO, NCSC, FCA, PRA, Bank of England
   - EU: ENISA, EDPB, European Commission
   - Germany: BSI, BfDI
   - France: ANSSI, CNIL
   - International: ISO, NIST, PCI SSC, ISACA, AXELOS
   - Each with: name, type, country, URL, RSS feed URL (where available), relevant frameworks

5. NEXT.JS FRONTEND — /regulatory:
   - Regulatory Dashboard:
     * New changes requiring attention (count + severity breakdown)
     * Upcoming deadlines timeline
     * Pending impact assessments
     * Changes by framework (which frameworks have the most regulatory activity)
   - Change feed (chronological, filterable):
     * Card per change: title, source, published date, severity badge, frameworks affected badges, status badge
     * Filter: source, severity, framework, region, status, date range
   - Change detail page:
     * Full summary, source link, effective date, affected frameworks
     * Impact assessment panel (if assessed): impact level, gaps created, coverage %, effort estimate
     * "Assess Impact" button → triggers assessment (AI + database analysis)
     * "Create Response Plan" button → generates remediation plan
   - Source management:
     * List of subscribed sources with scan status
     * "Add Custom Source" for org-specific regulatory feeds
   - Regulatory calendar: timeline/calendar view of all effective dates and deadlines

CRITICAL REQUIREMENTS:
- RSS scanning must be fault-tolerant: one failed feed doesn't block others
- AI classification must handle multilingual sources (German/French regulatory text)
- De-duplication must catch both exact URL matches and semantically similar changes
- Impact assessments are per-organisation (same regulatory change has different impact for different orgs)
- Auto-assessment (AI) requires human review before generating response plans
- Regulatory deadlines must feed into the notification engine (Prompt 11) for deadline alerts
- All regulatory changes and assessments are immutable audit trail records
- Dashboard must highlight "action required" items prominently
- Support for consultation responses (track org's submissions to regulatory consultations)
- Integration with notification engine: emit 'regulatory.new_change', 'regulatory.deadline_approaching' events

OUTPUT: Complete Golang code for scanner, classifier, impact assessor, handlers, worker, migration, 20+ seed sources, and Next.js regulatory management pages. Include unit tests for the RSS parser, de-duplication logic, and impact assessment calculation.
```

---

### PROMPT 24 OF 100 — Business Impact Analysis (BIA) & Business Continuity Module

```
You are a senior Golang backend engineer building the Business Impact Analysis and Business Continuity module for "ComplianceForge" — a GRC platform. This module addresses ISO 27001 controls A.5.29 (Information security during disruption) and A.5.30 (ICT readiness for business continuity), NIS2 Article 21(c) (business continuity and crisis management), and ITIL 4 Service Continuity Management practice.

OBJECTIVE:
Build a complete BIA module that: identifies critical business processes, analyses the impact of their disruption, determines Recovery Time Objectives (RTO) and Recovery Point Objectives (RPO), maps process dependencies (systems, people, vendors, data), generates business continuity plans, and tracks BC testing/exercises. This is essential for European enterprises under NIS2 which mandates business continuity capabilities.

DATABASE SCHEMA — Create migration 019:

TABLE business_processes:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - process_ref VARCHAR(20) NOT NULL — BP-001
  - name VARCHAR(300) NOT NULL
  - description TEXT
  - process_owner_user_id UUID FK → users
  - department VARCHAR(200)
  - category ENUM('core_revenue', 'customer_facing', 'regulatory_required', 'operational_support', 'strategic', 'administrative')
  - criticality ENUM('mission_critical', 'business_critical', 'important', 'minor', 'non_essential')
  - status ENUM('active', 'inactive', 'under_review')
  
  -- Impact Assessment
  - financial_impact_per_hour_eur DECIMAL(12,2) — cost of this process being down per hour
  - financial_impact_per_day_eur DECIMAL(12,2)
  - regulatory_impact TEXT — what regulations are violated if this process fails
  - reputational_impact ENUM('catastrophic', 'severe', 'moderate', 'minor', 'negligible')
  - legal_impact ENUM('catastrophic', 'severe', 'moderate', 'minor', 'negligible')
  - operational_impact ENUM('catastrophic', 'severe', 'moderate', 'minor', 'negligible')
  - safety_impact ENUM('catastrophic', 'severe', 'moderate', 'minor', 'negligible') — relevant for NIS2 essential services
  
  -- Recovery Objectives
  - rto_hours DECIMAL(8,1) — Recovery Time Objective: max tolerable downtime
  - rpo_hours DECIMAL(8,1) — Recovery Point Objective: max tolerable data loss
  - mtpd_hours DECIMAL(8,1) — Maximum Tolerable Period of Disruption
  - minimum_service_level TEXT — what's the bare minimum acceptable service during disruption
  
  -- Dependencies
  - dependent_asset_ids UUID[] — FK → assets
  - dependent_vendor_ids UUID[] — FK → vendors
  - dependent_process_ids UUID[] — other processes this depends on
  - key_personnel_user_ids UUID[] — people essential to this process
  - data_classification VARCHAR(50) — classification of data this process handles
  - peak_periods TEXT[] — e.g., ['month_end', 'quarter_end', 'tax_season']
  
  - last_bia_date DATE
  - next_bia_due DATE
  - bia_frequency_months INT DEFAULT 12
  - notes TEXT
  - metadata JSONB
  - created_at, updated_at

TABLE bia_scenarios:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - scenario_ref VARCHAR(20) — SCN-001
  - name VARCHAR(300) NOT NULL — e.g., 'Ransomware attack', 'Data centre failure', 'Key vendor bankruptcy'
  - description TEXT
  - scenario_type ENUM('cyber_attack', 'natural_disaster', 'infrastructure_failure', 'supply_chain', 'pandemic', 'regulatory_action', 'data_loss', 'key_person_loss', 'utility_failure', 'civil_unrest')
  - likelihood ENUM('almost_certain', 'likely', 'possible', 'unlikely', 'rare')
  - affected_process_ids UUID[] — which processes are impacted
  - affected_asset_ids UUID[]
  - impact_timeline JSONB — {"1_hour": "description of impact at 1h", "4_hours": "...", "24_hours": "...", "1_week": "..."}
  - estimated_financial_loss_eur DECIMAL(12,2)
  - mitigation_strategy TEXT
  - status ENUM('identified', 'analysed', 'mitigated', 'accepted')
  - created_at, updated_at

TABLE continuity_plans:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - plan_ref VARCHAR(20) — BCP-001
  - name VARCHAR(300) NOT NULL
  - plan_type ENUM('business_continuity_plan', 'disaster_recovery_plan', 'crisis_management_plan', 'incident_response_plan', 'pandemic_plan', 'it_service_continuity')
  - status ENUM('draft', 'approved', 'active', 'under_review', 'archived')
  - version INT DEFAULT 1
  - scope_description TEXT
  - covered_scenario_ids UUID[] — which scenarios this plan addresses
  - covered_process_ids UUID[] — which processes are covered
  
  -- Plan Content
  - activation_criteria TEXT — when to activate this plan
  - activation_authority TEXT — who can activate
  - command_structure JSONB — crisis team roles and contact info
  - communication_plan JSONB — who to notify, in what order, by what means
  - recovery_procedures JSONB — [{step_order, description, responsible, rto_alignment}]
  - resource_requirements JSONB — people, facilities, equipment, IT systems needed
  - alternate_site_details JSONB — backup locations, hot/warm/cold site info
  
  - owner_user_id UUID FK → users
  - approved_by UUID FK → users
  - approved_at TIMESTAMPTZ
  - next_review_date DATE
  - review_frequency_months INT DEFAULT 6
  - document_path TEXT — full BCP document stored in file storage
  - created_at, updated_at

TABLE bc_exercises:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - exercise_ref VARCHAR(20) — BCX-001
  - name VARCHAR(300) NOT NULL
  - exercise_type ENUM('tabletop', 'walkthrough', 'simulation', 'full_test', 'parallel_test', 'component_test')
  - plan_id UUID FK → continuity_plans
  - scenario_id UUID FK → bia_scenarios
  - status ENUM('planned', 'in_progress', 'completed', 'cancelled')
  - scheduled_date DATE
  - actual_date DATE
  - participants JSONB — [{user_id, role_in_exercise}]
  
  -- Results
  - rto_achieved_hours DECIMAL(8,1) — actual recovery time achieved
  - rpo_achieved_hours DECIMAL(8,1) — actual data loss
  - objectives_met BOOLEAN
  - lessons_learned TEXT
  - gaps_identified TEXT
  - improvement_actions JSONB — [{description, assigned_to, due_date, status}]
  - overall_rating ENUM('pass', 'pass_with_concerns', 'fail')
  - report_document_path TEXT
  
  - conducted_by UUID FK → users
  - created_at, updated_at

TABLE process_dependencies_map:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - process_id UUID FK → business_processes
  - dependency_type ENUM('system', 'application', 'data', 'vendor', 'person', 'process', 'facility', 'network')
  - dependency_entity_type VARCHAR(50) — 'asset', 'vendor', 'user', 'business_process'
  - dependency_entity_id UUID
  - dependency_name VARCHAR(300) — human-readable name
  - is_critical BOOLEAN DEFAULT false — process cannot function without this dependency
  - alternative_available BOOLEAN DEFAULT false
  - alternative_description TEXT
  - recovery_sequence INT — order in which dependencies should be recovered
  - notes TEXT
  - created_at, updated_at

GOLANG IMPLEMENTATION:

1. internal/service/bia_service.go:
   - CreateProcess(ctx, orgID, req) — with auto-ref generation
   - AssessProcess(ctx, orgID, processID, assessment) — update impact assessment, RTO/RPO
   - MapDependencies(ctx, orgID, processID, deps) — create dependency map
   - GenerateBIAReport(ctx, orgID) — comprehensive BIA report:
     * All processes ranked by criticality
     * Dependency chain analysis (single points of failure)
     * RTO/RPO summary with gap analysis (can we actually meet our objectives?)
     * Financial impact projections by scenario
   - IdentifySinglePointsOfFailure(ctx, orgID) → find assets/vendors that multiple critical processes depend on
   - GetDependencyGraph(ctx, orgID) → full dependency tree for visualisation

2. internal/service/continuity_service.go:
   - CreateScenario / AssessScenario
   - CreatePlan / ApprovePlan / ActivatePlan
   - ScheduleExercise / CompleteExercise
   - GenerateBCPDocument(ctx, orgID, planID) → PDF business continuity plan document
   - GetBCDashboard(ctx, orgID) → dashboard metrics:
     * Processes without BIA: count
     * Plans requiring review: count
     * Last exercise date and result
     * RTO/RPO coverage: % of critical processes with tested recovery capabilities
     * Single points of failure: count

3. internal/handler/bia_handler.go — API Endpoints:
   - GET /bia/processes — list business processes
   - POST /bia/processes — create process
   - GET /bia/processes/{id} — process detail with dependencies
   - PUT /bia/processes/{id} — update process / assessment
   - POST /bia/processes/{id}/dependencies — map dependencies
   - GET /bia/processes/{id}/dependency-graph — dependency tree
   - GET /bia/single-points-of-failure — SPoF analysis
   - GET /bia/report — BIA summary report
   
   - GET /bc/scenarios — list scenarios
   - POST /bc/scenarios — create scenario
   - GET /bc/plans — list continuity plans
   - POST /bc/plans — create plan
   - POST /bc/plans/{id}/approve — approve plan
   - GET /bc/exercises — list exercises
   - POST /bc/exercises — schedule exercise
   - PUT /bc/exercises/{id}/complete — record exercise results
   - GET /bc/dashboard — BC dashboard metrics

4. NEXT.JS FRONTEND:
   - /bia dashboard:
     * Summary: critical processes count, processes without BIA, SPoFs identified
     * Process list with criticality badges, RTO/RPO, last assessed date
     * Dependency graph visualisation (interactive force-directed graph or tree)
     * SPoF alerts: "Asset X supports 5 mission-critical processes"
   - Process detail page:
     * Impact assessment form (financial, regulatory, reputational, operational sliders)
     * RTO/RPO configuration with visual timeline
     * Dependency map (draggable nodes showing systems, vendors, people, other processes)
   - /bc section:
     * Scenario list with likelihood/impact matrix
     * Plan list with status, coverage, last test date
     * Exercise calendar and results history
     * BIA report generation and download

CRITICAL REQUIREMENTS:
- Dependency graph must detect circular dependencies and warn
- Single point of failure analysis must consider transitive dependencies (if Process A depends on Asset X, and Process B depends on Process A, then Asset X is a SPoF for both)
- RTO/RPO values feed into the continuous monitoring module (Prompt 15) — alert if backup frequency doesn't meet RPO
- BC exercises link to the audit module — exercise results are evidence for ISO 27001 A.5.29
- NIS2 Article 21(c) mapping: the BC module satisfies business continuity requirements
- Financial impact calculations aggregate up: org-level financial exposure = sum of per-process × probability
- Plans must have review dates that trigger the notification engine (Prompt 11)
- Export BIA report as professional PDF (using report engine from Prompt 12)

OUTPUT: Complete Golang code for BIA service, BC service, dependency analyser, handlers, migration, and Next.js BIA/BC pages with interactive dependency graph. Include unit tests for SPoF detection and financial impact aggregation.
```

---

### PROMPT 25 OF 100 — Advanced Analytics, Predictive Risk Scoring & BI Dashboard

```
You are a senior Golang backend engineer building the advanced analytics and business intelligence module for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a comprehensive analytics engine that provides: compliance trend analysis over time, predictive risk scoring using statistical models, KRI forecasting, peer benchmarking, executive BI dashboards with drill-down, and data export for external BI tools. This module transforms raw GRC data into strategic intelligence that justifies the platform's value to C-suite stakeholders.

DATABASE SCHEMA — Create migration 020:

TABLE analytics_snapshots:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - snapshot_type ENUM('daily', 'weekly', 'monthly', 'quarterly')
  - snapshot_date DATE NOT NULL
  - metrics JSONB NOT NULL — comprehensive snapshot of all metrics at this point in time:
    {
      "compliance": {
        "overall_score": 72.4,
        "by_framework": [{"code": "ISO27001", "score": 79.5, "controls_total": 93, "controls_implemented": 74}],
        "maturity_avg": 2.8,
        "gaps_total": 45,
        "gaps_critical": 8
      },
      "risks": {
        "total": 48, "critical": 3, "high": 8, "medium": 15, "low": 22,
        "avg_residual_score": 8.7,
        "treatment_completion_rate": 65.2,
        "new_this_period": 5, "closed_this_period": 3
      },
      "incidents": {
        "total_open": 4, "total_this_period": 7,
        "breaches": 2, "avg_resolution_hours": 18.5,
        "by_severity": {"critical": 1, "high": 2, "medium": 3, "low": 1}
      },
      "policies": {
        "total": 12, "published": 10, "overdue_review": 2,
        "attestation_rate": 87.5
      },
      "vendors": {
        "total": 25, "high_risk": 7, "missing_dpa": 3, "assessments_overdue": 2
      },
      "findings": {
        "total_open": 12, "overdue": 4, "critical_open": 2,
        "avg_remediation_days": 28.5
      }
    }
  - created_at

TABLE analytics_compliance_trends:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - framework_id UUID FK → compliance_frameworks
  - framework_code VARCHAR(20)
  - measurement_date DATE NOT NULL
  - compliance_score DECIMAL(5,2)
  - controls_implemented INT
  - controls_total INT
  - maturity_avg DECIMAL(3,2)
  - score_change_7d DECIMAL(5,2) — change from 7 days ago
  - score_change_30d DECIMAL(5,2) — change from 30 days ago
  - score_change_90d DECIMAL(5,2) — change from 90 days ago
  - trend_direction ENUM('improving', 'stable', 'declining')
  - created_at
  - UNIQUE(organization_id, framework_id, measurement_date)

TABLE analytics_risk_predictions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - risk_id UUID FK → risks
  - prediction_date DATE
  - prediction_type ENUM('score_forecast', 'breach_probability', 'treatment_effectiveness', 'escalation_likelihood')
  - predicted_value DECIMAL(10,4)
  - confidence_interval_low DECIMAL(10,4)
  - confidence_interval_high DECIMAL(10,4)
  - confidence_level DECIMAL(3,2) — 0.95 for 95% confidence
  - model_version VARCHAR(50)
  - input_features JSONB — what data points were used for the prediction
  - actual_value DECIMAL(10,4) — filled in later for model validation
  - created_at

TABLE analytics_benchmarks:
  - id UUID PK
  - benchmark_type ENUM('industry', 'size', 'region', 'framework', 'overall')
  - category VARCHAR(100) — industry name, size range, region, framework code
  - metric_name VARCHAR(200) — 'compliance_score', 'avg_resolution_hours', 'treatment_completion_rate'
  - period VARCHAR(20) — '2026-Q1'
  - percentile_25 DECIMAL(10,2)
  - percentile_50 DECIMAL(10,2) — median
  - percentile_75 DECIMAL(10,2)
  - percentile_90 DECIMAL(10,2)
  - sample_size INT
  - calculated_at TIMESTAMPTZ
  - created_at

TABLE analytics_custom_dashboards:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - name VARCHAR(200) NOT NULL
  - description TEXT
  - layout JSONB NOT NULL — grid layout of widgets:
    [
      {"widget_id": "uuid", "type": "line_chart", "title": "Compliance Score Trend", "position": {"x": 0, "y": 0, "w": 6, "h": 4}, "config": {"metric": "compliance_score", "period": "12m", "frameworks": ["ISO27001", "NIST_CSF_2"]}},
      {"widget_id": "uuid", "type": "kpi_card", "title": "Open Risks", "position": {"x": 6, "y": 0, "w": 3, "h": 2}, "config": {"metric": "risks_open_total", "comparison": "previous_month"}},
      ...
    ]
  - is_default BOOLEAN DEFAULT false
  - is_shared BOOLEAN DEFAULT false — visible to all org users
  - owner_user_id UUID FK → users
  - created_at, updated_at

TABLE analytics_widget_types:
  - id UUID PK
  - widget_type VARCHAR(50) — 'line_chart', 'bar_chart', 'donut_chart', 'kpi_card', 'heatmap', 'radar', 'table', 'gauge', 'sparkline', 'trend_arrow', 'map'
  - name VARCHAR(200)
  - description TEXT
  - available_metrics TEXT[] — which metrics this widget can display
  - default_config JSONB
  - min_width INT
  - min_height INT
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/analytics_engine.go — Core Analytics:

   a. SnapshotCollector:
      - TakeSnapshot(ctx, orgID, snapshotType) → captures all current metrics into analytics_snapshots
      - Scheduled: daily at 00:00 UTC, weekly on Mondays, monthly on 1st, quarterly on Jan/Apr/Jul/Oct 1st
      - Each snapshot is a complete point-in-time record — enables historical trend queries
   
   b. TrendCalculator:
      - CalculateComplianceTrends(ctx, orgID) → for each framework:
        * Current score, 7d/30d/90d deltas
        * Trend direction (linear regression on last 30 daily snapshots)
        * Projected score in 30/60/90 days (linear extrapolation)
      - Store in analytics_compliance_trends
   
   c. RiskPredictor (statistical model, not AI — runs without external API):
      - PredictRiskScoreTrajectory(ctx, orgID, riskID) → based on:
        * Historical risk score changes
        * Treatment completion rate
        * Similar risks in the same org
        * Time-series analysis (simple exponential smoothing)
        * Output: predicted score at +30d, +60d, +90d with confidence intervals
      
      - PredictBreachProbability(ctx, orgID) → based on:
        * Current risk posture (number and severity of open risks)
        * Incident history (frequency, severity, trends)
        * Control implementation coverage
        * Vendor risk exposure
        * Industry benchmark (if available)
        * Output: estimated probability of a material breach in next 30/90/365 days
        * Model: logistic regression on historical data
      
      - PredictTreatmentEffectiveness(ctx, orgID, treatmentID) → based on:
        * Treatment type
        * Historical effectiveness of similar treatments
        * Resource allocation (budget, personnel)
        * Output: probability of treatment success, expected risk reduction
   
   d. BenchmarkEngine:
      - CalculateBenchmarks(ctx) → aggregate anonymised metrics across all orgs:
        * Group by: industry, size, region, framework
        * Calculate percentiles: 25th, 50th, 75th, 90th
        * Metrics: compliance_score, avg_resolution_hours, treatment_completion_rate, attestation_rate, vendor_assessment_rate
      - CompareToPeris(ctx, orgID) → where does this org sit relative to peers:
        * "Your ISO 27001 compliance score of 79.5% is at the 68th percentile for technology companies in Europe"
        * "Your average incident resolution time of 18.5 hours is better than 75% of similar-sized organisations"
      
      IMPORTANT: Benchmarks use ONLY aggregated, anonymised data. No individual org's data is identifiable.

2. internal/service/analytics_query.go — Flexible Analytics Queries:
   
   - GetMetricTimeSeries(ctx, orgID, metric, period, granularity) → time series data for charts:
     * metric: 'compliance_score', 'risk_count_critical', 'incident_count', 'finding_count_overdue', etc.
     * period: '7d', '30d', '90d', '12m', '24m'
     * granularity: 'daily', 'weekly', 'monthly'
     * Returns: [{date, value}] array for chart rendering
   
   - GetMetricComparison(ctx, orgID, metric, current, previous) → compare two periods:
     * "Compliance score this month: 72.4% (up 3.2% from last month)"
   
   - GetTopMovers(ctx, orgID, metric, period, direction, limit) → biggest changes:
     * "Top 5 frameworks with most improved compliance scores this quarter"
     * "Top 5 risk categories with most new risks this month"
   
   - GetDistribution(ctx, orgID, entity, groupBy) → distribution analysis:
     * "Risks by category", "Controls by maturity level", "Findings by severity"
   
   - ExportAnalyticsData(ctx, orgID, config) → export raw data for external BI tools:
     * Formats: CSV, JSON, Excel
     * Configurable: which entities, which fields, date range, filters
     * Suitable for Power BI / Tableau / Looker import

3. internal/handler/analytics_handler.go — API Endpoints:
   - GET /analytics/snapshots — list snapshots (for historical comparison)
   - GET /analytics/trends/compliance — compliance score trends
   - GET /analytics/trends/risks — risk trends
   - GET /analytics/trends/incidents — incident trends
   - GET /analytics/predictions/risks/{riskId} — risk score prediction
   - GET /analytics/predictions/breach-probability — breach probability forecast
   - GET /analytics/benchmarks — peer comparison
   - GET /analytics/metrics/{metric} — time series for any metric
   - GET /analytics/metrics/{metric}/compare — period comparison
   - GET /analytics/top-movers — biggest positive/negative changes
   - GET /analytics/distribution/{entity} — distribution breakdown
   - POST /analytics/export — export analytics data
   
   - GET /analytics/dashboards — list custom dashboards
   - POST /analytics/dashboards — create custom dashboard
   - PUT /analytics/dashboards/{id} — update dashboard layout
   - DELETE /analytics/dashboards/{id} — delete dashboard
   - GET /analytics/widget-types — available widget types

4. NEXT.JS FRONTEND — /analytics:
   
   - Analytics Dashboard (customisable):
     * Default layout with key metrics:
       - Row 1: 6 KPI cards with sparkline trends (compliance score, risks, incidents, findings, policies, vendors) — each showing current value + trend arrow + period comparison
       - Row 2: Compliance score trend line chart (last 12 months, all frameworks overlaid)
       - Row 3: Risk heatmap + risk trend bar chart
       - Row 4: Incident volume over time + mean resolution time trend
       - Row 5: Peer benchmarking radar (your org vs industry median)
     * Custom dashboard builder:
       - Drag-and-drop grid layout editor
       - Widget palette: line chart, bar chart, donut, KPI card, heatmap, radar, table, gauge
       - Widget configuration: select metric, period, comparison, filters
       - Save, share, set as default
   
   - Trend Deep-Dive Pages:
     * /analytics/compliance — per-framework trend lines, maturity progression, gap closure rate
     * /analytics/risks — risk volume trends, category distribution changes, treatment velocity
     * /analytics/incidents — incident frequency, severity distribution, MTTR trend, breach count
     * /analytics/predictions — risk forecasts with confidence bands, breach probability gauge
   
   - Benchmarking Page:
     * Peer comparison cards: "You vs Industry" for each metric
     * Percentile position indicators (green if above 75th, amber if 25th-75th, red if below 25th)
     * "How to improve" suggestions based on where the org falls below median
   
   - Export Page:
     * Select entities to export (risks, controls, incidents, etc.)
     * Select fields, date range, filters
     * Choose format (CSV, JSON, XLSX)
     * Schedule recurring exports (daily/weekly to email or SFTP)

CRITICAL REQUIREMENTS:
- Snapshots are immutable — historical data never modified
- Trend calculations are idempotent — running twice for the same date produces the same result
- Predictions are clearly labelled as estimates with confidence intervals — never stated as facts
- Benchmarks use ONLY anonymised aggregates — no individual org data exposed
- Custom dashboards support real-time updates (polling every 30 seconds)
- Data export must respect ABAC permissions (Prompt 20) — users can only export data they can access
- Analytics queries must be performant: <500ms for time-series queries on 24 months of daily data
- The breach probability model must be validated: compare predictions against actual outcomes and publish accuracy metrics
- All analytics data respects data residency (EU storage)
- Support for custom KPIs: orgs can define their own metric calculations

OUTPUT: Complete Golang code for analytics engine, trend calculator, risk predictor, benchmark engine, query engine, handlers, migration, snapshot scheduler, and Next.js analytics dashboard with drag-and-drop builder. Include unit tests for trend calculation, prediction models, and benchmark aggregation.
```

---

## BATCH 5 SUMMARY

| Prompt | Focus Area | New Tables | New Endpoints | Key Capabilities |
|--------|-----------|------------|---------------|------------------|
| 21 | AI Remediation Planner | 3 (plans, actions, ai_logs) | ~14 | Claude API integration, AI-generated remediation plans with prioritisation/effort/cost, per-control AI guidance, policy draft assistant, evidence suggestions, static fallback knowledge base, prompt engineering templates |
| 22 | Control Marketplace | 5 (publishers, packages, versions, installations, reviews) | ~20 | Community-driven control library exchange, package publishing/versioning/installation, conflict resolution, integrity verification, 3 official seed packages, publisher verification |
| 23 | Regulatory Change Mgmt | 4 (sources, changes, subscriptions, assessments) | ~12 | RSS/API regulatory scanning, AI-assisted classification, per-org impact assessment, response plan generation, 20+ EU regulatory sources seeded, deadline tracking |
| 24 | Business Impact Analysis | 5 (processes, scenarios, plans, exercises, dependencies) | ~18 | Process criticality assessment, RTO/RPO/MTPD, dependency mapping with SPoF detection, BCP generation, exercise tracking, NIS2 Art.21(c) compliance |
| 25 | Advanced Analytics | 5 (snapshots, trends, predictions, benchmarks, dashboards) | ~20 | Daily metric snapshots, compliance trend analysis, predictive risk scoring (statistical), peer benchmarking, custom BI dashboards, data export for Power BI/Tableau |

**Running Total: 25/100 Prompts | ~97 Tables | ~245+ API Endpoints | AI-Powered Intelligence Layer**

---

> **NEXT BATCH (Prompts 26–30):** Exception Management & Compensating Controls, Evidence Template Library & Automated Testing, Third-Party Risk Assessment Questionnaires, Data Classification & ROPA (Records of Processing), and Executive Board Reporting Portal.
>
> Type **"next"** to continue.
