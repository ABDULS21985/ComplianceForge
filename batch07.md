# GRC Compliance Management Solution — 100 Master Prompts

## BATCH 7 — Compliance Calendar, Advanced Search & Knowledge Base, Collaboration & Comments, Mobile API & Push Notifications, Tenant White-Labelling & Custom Branding

**Stack:** Golang 1.22+ | PostgreSQL 16 | Redis 7 | Next.js 14 | Elasticsearch (optional) | Firebase Cloud Messaging
**Prerequisite:** All previous batches (Prompts 1–30) completed
**Deliverable:** Unified compliance calendar, enterprise search, collaboration layer, mobile-ready API, and multi-tenant branding

---

### PROMPT 31 OF 100 — Compliance Calendar & Deadline Management Engine

```
You are a senior Golang backend engineer building the compliance calendar and deadline management engine for "ComplianceForge" — a GRC platform targeting European enterprises.

OBJECTIVE:
Build a unified compliance calendar that aggregates ALL deadlines, review dates, assessment schedules, and recurring obligations from every module into a single view. European enterprises juggle dozens of concurrent compliance deadlines: GDPR breach notification (72h), NIS2 reporting (24h/72h/1mo), policy reviews (annual), vendor assessments (quarterly), risk reviews (quarterly), audit schedules, DSR response deadlines (30/90 days), exception expiry dates, evidence collection due dates, board meeting dates, and regulatory effective dates. Missing any one of these creates regulatory exposure. This calendar is the operational nerve centre.

DATABASE SCHEMA — Create migration 026:

TABLE calendar_events:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - event_ref VARCHAR(30) — auto-generated from source: 'POL-REV-0001', 'VND-ASS-0003'
  - title VARCHAR(500) NOT NULL
  - description TEXT
  - event_type ENUM('policy_review', 'policy_attestation', 'risk_review', 'risk_treatment_due', 'vendor_assessment', 'vendor_contract_renewal', 'audit_planned', 'audit_fieldwork', 'finding_remediation_due', 'evidence_collection', 'exception_expiry', 'exception_review', 'dsr_deadline', 'dsr_extended_deadline', 'breach_notification_deadline', 'nis2_early_warning', 'nis2_notification', 'nis2_final_report', 'regulatory_effective_date', 'regulatory_response_deadline', 'training_due', 'training_expiry', 'certification_expiry', 'board_meeting', 'bc_exercise', 'bc_plan_review', 'bia_review', 'custom')
  - category ENUM('compliance', 'risk', 'audit', 'privacy', 'security', 'governance', 'operational', 'regulatory')
  - priority ENUM('critical', 'high', 'medium', 'low')
  
  -- Source Entity
  - source_entity_type VARCHAR(100) NOT NULL — 'policy', 'risk', 'vendor', 'audit', 'finding', 'exception', 'dsr_request', 'incident', 'regulatory_change', 'bc_exercise', etc.
  - source_entity_id UUID NOT NULL
  - source_entity_ref VARCHAR(50) — human-readable ref from source
  
  -- Timing
  - start_date DATE NOT NULL
  - start_time TIME — NULL for all-day events
  - end_date DATE — NULL for single-day events
  - end_time TIME
  - is_all_day BOOLEAN DEFAULT true
  - timezone VARCHAR(50) DEFAULT 'Europe/London'
  
  -- Recurrence
  - is_recurring BOOLEAN DEFAULT false
  - recurrence_rule VARCHAR(200) — iCal RRULE: 'FREQ=MONTHLY;INTERVAL=3;BYMONTHDAY=1' (quarterly on 1st)
  - recurrence_end_date DATE
  - parent_event_id UUID FK → calendar_events — links recurring instances to parent
  
  -- Status
  - status ENUM('upcoming', 'due_today', 'overdue', 'completed', 'cancelled', 'rescheduled')
  - completed_at TIMESTAMPTZ
  - completed_by UUID FK → users
  - completion_notes TEXT
  
  -- Assignment
  - assigned_to UUID FK → users
  - assigned_role VARCHAR(100) — if assigned to a role rather than specific user
  - watchers UUID[] — users who want notifications about this event
  
  -- Notification
  - reminder_days_before INT[] DEFAULT ARRAY[7, 3, 1, 0] — send reminders at these intervals
  - reminders_sent JSONB DEFAULT '{}' — {"7": "2026-03-21T09:00:00Z", "3": "2026-03-25T09:00:00Z"}
  - escalation_days_overdue INT DEFAULT 3 — escalate after this many days overdue
  - escalation_sent BOOLEAN DEFAULT false
  - escalation_user_ids UUID[] — who to escalate to
  
  - metadata JSONB
  - created_at, updated_at

TABLE calendar_subscriptions:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - user_id UUID FK → users
  - event_types TEXT[] — which event types to include, NULL = all
  - categories TEXT[] — which categories, NULL = all
  - assigned_to_me_only BOOLEAN DEFAULT false — only show events assigned to this user
  - ical_export_enabled BOOLEAN DEFAULT false — allow iCal feed export
  - ical_token_hash VARCHAR(128) — for external calendar integration
  - notification_preferences JSONB — override per-event-type notification settings
  - created_at, updated_at

TABLE calendar_sync_configs:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - sync_source VARCHAR(100) NOT NULL — which module generates these events
  - event_type calendar_event_type NOT NULL
  - is_enabled BOOLEAN DEFAULT true
  - default_priority calendar_priority DEFAULT 'medium'
  - default_reminder_days INT[] DEFAULT ARRAY[7, 3, 1]
  - default_escalation_days INT DEFAULT 3
  - created_at, updated_at

GOLANG IMPLEMENTATION:

1. internal/service/calendar_service.go — Core Calendar Engine:

   CalendarSyncEngine — continuously synchronises events from all source modules:
   
   - SyncPolicyEvents(ctx, orgID) → for each published policy:
     * Create 'policy_review' event at next_review_date
     * If requires_attestation: create 'policy_attestation' events per attestation campaign
     * Priority: 'high' if overdue, 'medium' otherwise
   
   - SyncRiskEvents(ctx, orgID) → for each active risk:
     * Create 'risk_review' event at next_review_date
     * For each open risk_treatment: create 'risk_treatment_due' at target_date
     * Priority based on risk level
   
   - SyncVendorEvents(ctx, orgID) → for each active vendor:
     * Create 'vendor_assessment' event at next_assessment_date
     * Create 'vendor_contract_renewal' at contract_end_date - 90 days
     * Priority: 'critical' if vendor is critical tier, 'high' for high tier
   
   - SyncAuditEvents(ctx, orgID) → for each planned/in-progress audit:
     * Create 'audit_planned' at planned_start_date
     * Create 'audit_fieldwork' spanning start-end
     * For each open finding: create 'finding_remediation_due' at due_date
     * Priority based on finding severity
   
   - SyncEvidenceEvents(ctx, orgID) → for each evidence requirement:
     * Create 'evidence_collection' at next_collection_due date
     * Priority based on auditor_priority
   
   - SyncExceptionEvents(ctx, orgID) → for each active exception:
     * Create 'exception_expiry' at expiry_date
     * Create 'exception_review' at next_review_date
     * Priority: 'critical' for expiring within 30 days
   
   - SyncDSREvents(ctx, orgID) → for each active DSR request:
     * Create 'dsr_deadline' at response_deadline
     * If extended: create 'dsr_extended_deadline' at extended_deadline
     * Priority: 'critical' when < 7 days remaining
   
   - SyncIncidentEvents(ctx, orgID) → for data breaches and NIS2 incidents:
     * Create 'breach_notification_deadline' at notification_deadline
     * Create NIS2 events: 'nis2_early_warning', 'nis2_notification', 'nis2_final_report'
     * Priority: always 'critical'
   
   - SyncRegulatoryEvents(ctx, orgID) → from regulatory changes:
     * Create 'regulatory_effective_date' at effective_date
     * Create 'regulatory_response_deadline' at deadline
     * Priority based on change severity
   
   - SyncBCEvents(ctx, orgID) → from BC module:
     * Create 'bc_exercise' at scheduled_date
     * Create 'bc_plan_review' at next_review_date
     * Create 'bia_review' at next_bia_due
   
   - SyncBoardEvents(ctx, orgID) → from board module:
     * Create 'board_meeting' at meeting date
   
   The sync engine runs:
   - Full sync: nightly at 02:00 UTC (rebuild all events from source data)
   - Incremental sync: triggered by webhooks when source entities change
   - Event deduplication: match by (source_entity_type, source_entity_id, event_type) — update, don't duplicate

2. internal/service/calendar_query.go — Calendar Queries:

   - GetCalendarView(ctx, orgID, userID, startDate, endDate, filters) → events for display:
     * Filter by: event_type, category, priority, status, assigned_to
     * Support views: month, week, day, list (agenda)
     * Apply user's subscription filters
     * Sort by: date (default), priority, category
     * Include computed fields: days_until, is_overdue, is_today
   
   - GetUpcomingDeadlines(ctx, orgID, userID, withinDays, limit) → critical upcoming items:
     * Ordered by date, filtered by priority
     * Used for the dashboard "Upcoming Deadlines" widget
   
   - GetOverdueItems(ctx, orgID) → all overdue events:
     * Grouped by category
     * With escalation status
     * Used for the "Overdue" alert panel
   
   - GetComplianceCalendarSummary(ctx, orgID, month) → month overview:
     * Total events per day (heat map data)
     * Events by category distribution
     * Critical events highlighted
     * Overdue count
   
   - ExportICalFeed(ctx, orgID, userID, token) → generate iCal (.ics) feed:
     * Standard iCalendar format (RFC 5545)
     * Importable into Google Calendar, Outlook, Apple Calendar
     * Filtered by user's subscription preferences
     * Token-based access for external calendar apps

3. internal/worker/calendar_worker.go — Background Processing:
   
   - ReminderScheduler (runs every 15 minutes):
     * For each event where reminder_days_before includes today's offset:
       - Check if reminder already sent (reminders_sent JSONB)
       - If not sent: emit notification event 'calendar.reminder'
       - Record sent timestamp in reminders_sent
     * Example: event on March 28, reminder at [7,3,1,0]:
       - March 21: "7 days until Policy Review (POL-0003)"
       - March 25: "3 days until Policy Review (POL-0003)"
       - March 27: "Tomorrow: Policy Review (POL-0003)"
       - March 28: "Today: Policy Review (POL-0003) is due"
   
   - OverdueEscalator (runs hourly):
     * For events where status='overdue' AND days_overdue >= escalation_days_overdue:
       - If not already escalated: emit 'calendar.escalation' to escalation_user_ids
       - Mark escalation_sent = true
   
   - StatusUpdater (runs every 30 minutes):
     * Update event statuses:
       - 'upcoming' → 'due_today' when start_date = today
       - 'due_today' → 'overdue' when start_date < today AND not completed
     * Generate recurring event instances:
       - Parse RRULE for recurring events
       - Create next instance when current instance is completed or date passes
       - Respect recurrence_end_date

4. internal/handler/calendar_handler.go — API Endpoints:
   
   - GET /calendar/events?start=&end=&types=&categories=&priority=&assigned_to=&status= — calendar view
   - GET /calendar/events/{id} — event detail
   - PUT /calendar/events/{id}/complete — mark event completed
   - PUT /calendar/events/{id}/reschedule — change date (with reason)
   - PUT /calendar/events/{id}/assign — reassign event
   - POST /calendar/events — create custom event
   
   - GET /calendar/deadlines?within_days=30&limit=20 — upcoming deadlines
   - GET /calendar/overdue — all overdue items
   - GET /calendar/summary?month=2026-03 — month heat map data
   
   - GET /calendar/subscriptions — user's calendar preferences
   - PUT /calendar/subscriptions — update preferences
   - GET /calendar/ical/{token} — iCal feed export (public, token-authenticated)
   
   - GET /calendar/sync/status — sync status per module
   - POST /calendar/sync/trigger — trigger manual full sync (admin)
   - PUT /calendar/sync/configs — update sync configuration per module

5. NEXT.JS FRONTEND — /calendar:

   - Month View:
     * Full calendar grid showing all days in the month
     * Each day cell: colored dots indicating events (red=critical, orange=high, blue=medium, green=low)
     * Click a day → shows all events for that day in a side panel
     * Click an event → navigates to the source entity (e.g., click policy review → /policies/{id})
     * Header: month/year selector, view toggle (Month|Week|Agenda), filter dropdown
     * "Today" button to jump to current date
   
   - Week View:
     * 7-column layout with hour rows
     * Events positioned by time (or full-day bar at top)
     * Drag to reschedule (calls PUT /calendar/events/{id}/reschedule)
   
   - Agenda View:
     * Chronological list grouped by day
     * Each event: title, type badge, category badge, priority indicator, assigned user, days until/overdue
     * "Complete" checkbox inline
     * Filter bar: type, category, priority, assigned to me
   
   - Deadline Dashboard (sidebar widget or dedicated page):
     * "Next 7 Days" section with countdown timers
     * "Overdue" section highlighted in red with escalation status
     * "This Month" section grouped by week
     * Quick actions: complete, reschedule, reassign
   
   - Calendar Settings:
     * iCal export URL (copy to clipboard for adding to external calendar)
     * Event type toggles (which types to show)
     * Default reminder preferences
     * Working hours configuration
   
   - Dashboard Integration:
     * "Upcoming Deadlines" widget on the main dashboard showing next 5 critical items
     * "Overdue" count in the sidebar navigation badge on "Calendar" item
   
   - Email/Notification Integration:
     * Weekly digest email (Monday morning): summary of the week's compliance events
     * Daily digest option: today's events + overdue items
     * Templates for each reminder level (7d, 3d, 1d, due today, overdue)

CRITICAL REQUIREMENTS:
- Calendar sync must be IDEMPOTENT: running sync twice produces the same events (no duplicates)
- Source module changes auto-trigger incremental sync (e.g., policy review date changed → calendar event updated)
- Overdue events persist until explicitly completed or the source entity is resolved
- Recurring events use iCal RRULE standard for maximum compatibility with external calendars
- iCal feed export uses a per-user token (not JWT) that can be revoked without logging the user out
- The calendar must handle timezone differences: org in multiple EU timezones (CET, GMT, EET)
- Events from GDPR breach notifications are ALWAYS priority 'critical' — cannot be downgraded
- Performance: calendar view query must return <200ms for a month with 500+ events
- The weekly digest email must aggregate events intelligently: "5 evidence collections due, 2 policy reviews, 1 vendor assessment"
- Completed events show a strikethrough in the UI but remain visible for audit trail purposes
- Calendar events link bidirectionally: the source entity page also shows its calendar events

OUTPUT: Complete Golang code for calendar service, sync engine, query engine, worker, handlers, migration, and Next.js calendar pages (month/week/agenda views + deadline dashboard). Include unit tests for recurrence rule parsing, reminder scheduling, and sync idempotency.
```

---

### PROMPT 32 OF 100 — Advanced Search, Knowledge Base & Compliance Guidance Engine

```
You are a senior Golang backend engineer building the advanced search and compliance knowledge base for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a unified search engine that searches across ALL entities (controls, risks, policies, incidents, vendors, assets, findings, exceptions, DSRs, regulatory changes) with full-text search, faceted filtering, and relevance ranking. Additionally, build a compliance knowledge base that provides contextual guidance: when a user is looking at a control, they see relevant implementation guidance, related policies, mapped controls in other frameworks, applicable risks, available evidence templates, and relevant regulatory context. This makes ComplianceForge a compliance encyclopedia, not just a tracking tool.

DATABASE SCHEMA — Create migration 027:

TABLE search_index:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - entity_type VARCHAR(50) NOT NULL — 'control', 'risk', 'policy', 'incident', 'vendor', 'asset', 'finding', 'exception', 'dsr_request', 'regulatory_change', 'processing_activity', 'remediation_action'
  - entity_id UUID NOT NULL
  - entity_ref VARCHAR(50) — human-readable ref (A.5.1, RSK-0001, POL-0003)
  - title TEXT NOT NULL
  - body TEXT — searchable content (description, notes, content)
  - tags TEXT[]
  - framework_codes TEXT[] — relevant frameworks
  - status VARCHAR(50) — current status
  - severity VARCHAR(20) — risk level, incident severity, finding severity
  - category VARCHAR(100) — risk category, incident type, policy category
  - owner_name VARCHAR(200) — owner's full name for search
  - department VARCHAR(200)
  - classification VARCHAR(50) — data classification level
  - created_date DATE
  - updated_date DATE
  - search_vector TSVECTOR — PostgreSQL full-text search vector
  - metadata JSONB — additional searchable attributes
  - UNIQUE(organization_id, entity_type, entity_id)

CREATE INDEX idx_search_vector ON search_index USING GIN(search_vector);
CREATE INDEX idx_search_entity_type ON search_index(organization_id, entity_type);
CREATE INDEX idx_search_tags ON search_index USING GIN(tags);

TABLE knowledge_articles:
  - id UUID PK
  - organization_id UUID FK (RLS, NULL for system articles)
  - article_type ENUM('implementation_guide', 'best_practice', 'faq', 'regulatory_guide', 'tool_recommendation', 'template', 'glossary', 'how_to', 'case_study')
  - title VARCHAR(500) NOT NULL
  - slug VARCHAR(200) NOT NULL — URL-friendly
  - content_markdown TEXT NOT NULL — full article in markdown
  - summary TEXT — 2-3 sentence summary
  - applicable_frameworks TEXT[] — which frameworks this relates to
  - applicable_control_codes TEXT[] — specific controls
  - applicable_industries TEXT[]
  - tags TEXT[]
  - difficulty ENUM('beginner', 'intermediate', 'advanced')
  - reading_time_minutes INT
  - author_name VARCHAR(200)
  - is_system BOOLEAN DEFAULT false — official ComplianceForge content
  - is_published BOOLEAN DEFAULT true
  - view_count INT DEFAULT 0
  - helpful_count INT DEFAULT 0
  - not_helpful_count INT DEFAULT 0
  - search_vector TSVECTOR
  - created_at, updated_at

TABLE knowledge_bookmarks:
  - id UUID PK
  - user_id UUID FK → users
  - organization_id UUID FK (RLS)
  - article_id UUID FK → knowledge_articles
  - created_at
  - UNIQUE(user_id, article_id)

TABLE recent_searches:
  - id UUID PK
  - user_id UUID FK → users
  - organization_id UUID FK (RLS)
  - query TEXT NOT NULL
  - result_count INT
  - clicked_entity_type VARCHAR(50)
  - clicked_entity_id UUID
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/search_service.go — Unified Search Engine:

   SearchEngine struct:

   a. IndexEntity(ctx, orgID, entityType, entityID) → index/re-index a single entity:
      - Fetch the entity's current data from the appropriate repository
      - Build search_index record with: title, body, tags, status, severity, etc.
      - Generate tsvector using PostgreSQL's to_tsvector('english', title || ' ' || body)
      - Upsert into search_index
   
   b. IndexAllEntities(ctx, orgID) → full re-index:
      - Called during onboarding and nightly maintenance
      - Index all: controls (from control_implementations), risks, policies, incidents, vendors, assets, findings, exceptions, DSRs, regulatory changes, processing activities
      - Batch processing: 500 entities per batch, parallel by entity type
      - Report: total indexed, errors
   
   c. Search(ctx, orgID, request) → unified search:
      SearchRequest:
        - query: string (the search text)
        - entity_types: []string — filter to specific types (empty = all)
        - frameworks: []string — filter by framework
        - statuses: []string — filter by status
        - severities: []string — filter by severity
        - categories: []string — filter by category
        - tags: []string — filter by tags
        - date_from / date_to: filter by date range
        - sort_by: 'relevance' (default), 'date', 'title', 'severity'
        - page, page_size: pagination
      
      SearchResponse:
        - results: []SearchResult — {entity_type, entity_id, entity_ref, title, snippet (highlighted), score, status, severity, framework_codes, updated_date}
        - facets: {entity_types: [{value, count}], frameworks: [{value, count}], statuses: [{value, count}], severities: [{value, count}]}
        - total_results: int
        - query_time_ms: int
      
      Implementation:
        - Use PostgreSQL ts_query for full-text search
        - ts_rank_cd for relevance scoring
        - ts_headline for highlighted snippets
        - Facets via GROUP BY with COUNT
        - ABAC filtering: apply user's access policies to filter results (from Prompt 20)
        - Performance target: <100ms for typical queries
   
   d. Autocomplete(ctx, orgID, prefix, limit) → search suggestions:
      - Returns top matches as the user types (min 2 characters)
      - Sources: entity titles, entity refs, tags, control codes
      - Debounced: frontend calls this after 300ms of no typing
      - Performance target: <50ms
   
   e. GetRelatedEntities(ctx, orgID, entityType, entityID) → contextual related items:
      - For a control: related policies, risks, evidence templates, mapped controls, exceptions, findings
      - For a risk: related controls, treatments, KRIs, incidents, vendors
      - For a policy: related controls it covers, exceptions, attestation status
      - For an incident: related risks, controls, vendor if third-party
      - Uses: shared tags, direct FK relationships, cross-framework mappings, text similarity
      - Ranked by relevance and relationship strength

2. internal/service/knowledge_service.go — Knowledge Base:

   - GetArticlesForControl(ctx, controlCode, frameworkCode) → guidance articles for a specific control
   - GetArticlesForTopic(ctx, topic) → search knowledge base
   - GetRecommendedArticles(ctx, orgID, userID) → personalized recommendations based on:
     * User's role (DPO sees privacy articles, auditor sees audit articles)
     * Org's adopted frameworks
     * Org's industry
     * Recent user activity (looked at access controls → recommend access control guides)
   - TrackArticleEngagement(ctx, articleID, userID, action) → view, helpful, not_helpful
   - ManageArticles(ctx, orgID) → CRUD for org-specific knowledge articles

3. internal/worker/search_indexer.go — Background Indexing:
   
   - Listen for entity change events (from the event bus in Prompt 11):
     * risk.created → IndexEntity('risk', riskID)
     * policy.updated → IndexEntity('policy', policyID)
     * incident.created → IndexEntity('incident', incidentID)
     * control.status_changed → IndexEntity('control', implID)
   - Nightly full re-index at 03:00 UTC (catch anything missed)
   - Index health check: compare counts per entity type between source tables and search_index

4. internal/handler/search_handler.go — API Endpoints:
   
   - GET /search?q=&types=&frameworks=&statuses=&severities=&sort=&page= — unified search
   - GET /search/autocomplete?q=&limit=10 — autocomplete suggestions
   - GET /search/related/{entityType}/{entityId} — related entities
   - POST /search/reindex — trigger full re-index (admin)
   
   - GET /knowledge — browse knowledge base (paginated, filterable)
   - GET /knowledge/{slug} — article detail
   - GET /knowledge/for-control/{frameworkCode}/{controlCode} — guidance for a control
   - GET /knowledge/recommended — personalized recommendations
   - POST /knowledge/articles — create org article
   - PUT /knowledge/articles/{id} — update article
   - POST /knowledge/articles/{id}/feedback — helpful/not helpful
   - GET /knowledge/bookmarks — user's bookmarked articles
   - POST /knowledge/bookmarks/{articleId} — bookmark article

5. SEED — Knowledge Base Articles (50+ system articles):

   Implementation Guides (per framework domain):
   - "Implementing ISO 27001 Annex A.5: Organisational Controls — A Practical Guide"
   - "Access Control Implementation Checklist (A.5.15, A.8.2, A.8.3, A.8.5)"
   - "Setting Up Logging and Monitoring for ISO 27001 A.8.15 & A.8.16"
   - "Cryptography Controls: A.8.24 Implementation for European Enterprises"
   - "Vulnerability Management Programme: A.8.8 Step-by-Step"
   
   Regulatory Guides:
   - "GDPR Article 30 ROPA: What to Include and How to Maintain It"
   - "GDPR Breach Notification: The 72-Hour Workflow Explained"
   - "NIS2 Compliance Roadmap for Essential and Important Entities"
   - "Cyber Essentials Certification: Self-Assessment Guide"
   - "PCI DSS v4.0 Transition: Key Changes from v3.2.1"
   
   Best Practices:
   - "Building a 5×5 Risk Matrix: Industry Best Practices"
   - "Evidence Collection Best Practices for Compliance Audits"
   - "Vendor Risk Assessment: A DPO's Guide to GDPR Article 28"
   - "Security Awareness Training Programme Design"
   - "Incident Response Plan: Template and Testing Guide"
   
   Glossary:
   - Comprehensive GRC glossary: 200+ terms with definitions
   - Each term linked to relevant controls and frameworks
   
   Each article: 500-2000 words, practical, actionable, with links to relevant ComplianceForge features

6. NEXT.JS FRONTEND:

   - Global Search (Ctrl+K command palette):
     * Opens a modal overlay (like Spotlight/Alfred)
     * Search input with autocomplete dropdown
     * As user types: show instant results grouped by entity type
     * Each result: entity type icon, ref, title, status badge, highlighted snippet
     * Click result → navigate to entity
     * Recent searches shown when input is empty
     * Keyboard navigation: arrow keys to select, Enter to navigate
   
   - /search — Full Search Results Page:
     * Search bar at top
     * Left sidebar: faceted filters (entity type, framework, status, severity, date range)
     * Results list: entity icon, ref, title, snippet with query terms highlighted, status badge, date
     * Result count and query time
     * Pagination
   
   - /knowledge — Knowledge Base:
     * Browse by category: Implementation Guides, Regulatory Guides, Best Practices, Glossary
     * Search within knowledge base
     * Article cards: title, summary, frameworks badges, difficulty badge, reading time
     * Article page: rendered markdown with table of contents, related controls sidebar, helpful/not-helpful buttons, bookmark button
     * Recommended articles section on dashboard
   
   - Contextual Guidance Panel (on every entity detail page):
     * Collapsible right sidebar panel: "Related & Guidance"
     * Shows: related entities (linked controls, risks, policies), knowledge articles, evidence templates
     * "View Implementation Guide" link to relevant knowledge article
     * "Related Controls in Other Frameworks" from cross-mapping data

CRITICAL REQUIREMENTS:
- Search respects ABAC: users only see entities they have access to
- Full-text search supports: exact phrases ("access control"), OR logic (encryption OR cryptography), negation (-obsolete)
- Autocomplete returns results in <50ms (use Redis cache for frequent queries)
- Search index stays in sync: entity changes trigger re-indexing within 30 seconds
- Knowledge articles support markdown with: headers, code blocks, tables, links, images, callout boxes
- Glossary terms are linkable: hovering over a term in any page shows a tooltip with the definition
- Search analytics: track popular queries, zero-result queries (to identify content gaps)
- Knowledge base supports versioning: article updates create a new version, old version accessible
- System articles (seeded) are read-only for orgs; orgs can create their own articles
- The global search (Ctrl+K) must feel instant — pre-load recent searches and autocomplete cache

OUTPUT: Complete Golang code for search engine, knowledge service, indexer, handlers, migration, 50+ seed knowledge articles, and Next.js global search + knowledge base pages. Include unit tests for search query parsing, faceted filtering, and relevance ranking.
```

---

### PROMPT 33 OF 100 — Collaboration, Comments, Mentions & Activity Feed System

```
You are a senior Golang backend engineer building the collaboration and activity system for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Build a collaboration layer that enables team communication within the compliance context: comments on any entity (risks, controls, policies, incidents, findings, exceptions), @mentions that notify referenced users, threaded conversations, file attachments in comments, activity feeds showing team actions, and a notification digest. Enterprise GRC is a team sport — compliance officers, risk managers, control owners, DPOs, CISOs, and auditors all need to communicate in context without leaving the platform.

DATABASE SCHEMA — Create migration 028:

TABLE comments:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - entity_type VARCHAR(50) NOT NULL — any entity: 'risk', 'policy', 'control_implementation', 'incident', 'finding', 'exception', 'vendor', 'dsr_request', 'remediation_action', 'vendor_assessment'
  - entity_id UUID NOT NULL
  - parent_comment_id UUID FK → comments — NULL for top-level, set for replies (threads)
  - author_user_id UUID FK → users NOT NULL
  - content TEXT NOT NULL — supports markdown
  - content_html TEXT — pre-rendered HTML for display
  - is_internal BOOLEAN DEFAULT true — internal comments vs. comments shared with vendors/external
  - is_resolution_note BOOLEAN DEFAULT false — marks a comment as the resolution/outcome
  - is_pinned BOOLEAN DEFAULT false — pinned to top of comments section
  
  -- Mentions
  - mentioned_user_ids UUID[] — extracted from @mention syntax in content
  - mentioned_role_slugs TEXT[] — extracted from @role mentions
  
  -- Attachments
  - attachment_paths TEXT[] — file storage paths
  - attachment_names TEXT[] — original file names
  - attachment_sizes BIGINT[] — file sizes in bytes
  
  -- Reactions (lightweight feedback)
  - reactions JSONB DEFAULT '{}' — {"thumbs_up": ["user_id1", "user_id2"], "check": ["user_id3"]}
  
  -- Status
  - is_edited BOOLEAN DEFAULT false
  - edited_at TIMESTAMPTZ
  - is_deleted BOOLEAN DEFAULT false — soft delete, shows "[deleted]" placeholder
  - deleted_at TIMESTAMPTZ
  
  - created_at TIMESTAMPTZ DEFAULT NOW()
  - updated_at TIMESTAMPTZ

TABLE activity_feed:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - actor_user_id UUID FK → users NOT NULL
  - action VARCHAR(100) NOT NULL — 'created', 'updated', 'status_changed', 'assigned', 'commented', 'uploaded_evidence', 'approved', 'rejected', 'completed', 'escalated'
  - entity_type VARCHAR(50) NOT NULL
  - entity_id UUID NOT NULL
  - entity_ref VARCHAR(50) — human-readable ref
  - entity_title VARCHAR(500) — for display without fetching entity
  - description TEXT NOT NULL — human-readable: "John Smith changed risk RSK-0001 status from 'assessed' to 'treated'"
  - changes JSONB — structured diff: [{"field": "status", "old": "assessed", "new": "treated"}]
  - is_system BOOLEAN DEFAULT false — system-generated actions (e.g., auto-expiry)
  - visibility ENUM('all', 'team', 'admin_only') DEFAULT 'all'
  - created_at TIMESTAMPTZ DEFAULT NOW()

TABLE user_follows:
  - id UUID PK
  - user_id UUID FK → users
  - organization_id UUID FK (RLS)
  - entity_type VARCHAR(50) NOT NULL
  - entity_id UUID NOT NULL
  - follow_type ENUM('watching', 'participating', 'mentioned') — watching=all updates, participating=comments only, mentioned=one-time
  - created_at
  - UNIQUE(user_id, entity_type, entity_id)

TABLE user_read_markers:
  - id UUID PK
  - user_id UUID FK → users
  - organization_id UUID FK (RLS)
  - entity_type VARCHAR(50) NOT NULL
  - entity_id UUID NOT NULL
  - last_read_at TIMESTAMPTZ — timestamp of last viewed activity on this entity
  - unread_count INT DEFAULT 0
  - UNIQUE(user_id, entity_type, entity_id)

GOLANG IMPLEMENTATION:

1. internal/service/collaboration_service.go:

   Comments:
   - CreateComment(ctx, orgID, userID, entityType, entityID, content, parentID, attachments) →
     * Parse markdown content: extract @mentions (pattern: @[User Name](user_id) or @role:risk_manager)
     * Render content to HTML (using goldmark library)
     * Store comment
     * For each @mentioned user: create/update user_follows entry, emit 'comment.mention' notification
     * For entity followers (user_follows where follow_type='watching'): emit 'comment.new' notification
     * Auto-follow: comment author automatically starts 'participating' on the entity
     * Create activity_feed entry: "{author} commented on {entity_type} {entity_ref}"
   
   - EditComment(ctx, orgID, userID, commentID, newContent) →
     * Only the author can edit (within 24 hours)
     * Set is_edited=true, updated_at
     * Re-parse mentions (add new notifications if new mentions added)
   
   - DeleteComment(ctx, orgID, userID, commentID) →
     * Soft delete: set is_deleted=true, replace content with "[This comment was deleted]"
     * Only author or admin can delete
   
   - PinComment(ctx, orgID, commentID) → pin to top (admin/entity owner only)
   
   - ReactToComment(ctx, orgID, userID, commentID, reactionType) →
     * Toggle reaction (add if not present, remove if present)
     * Available reactions: 'thumbs_up', 'thumbs_down', 'check', 'eyes', 'rocket', 'warning'
   
   - GetComments(ctx, orgID, entityType, entityID, sortBy) →
     * Returns threaded comments (top-level with nested replies)
     * Sort: 'newest', 'oldest', 'most_reactions'
     * Include author details (name, avatar, role)
     * Mark which comments are unread for the requesting user

   Activity Feed:
   - RecordActivity(ctx, orgID, userID, action, entityType, entityID, description, changes) →
     * Create activity_feed entry
     * Notify followers of the entity
     * Update user_read_markers for all followers (increment unread_count)
   
   - GetActivityFeed(ctx, orgID, userID, filters) →
     * Personal feed: activities on entities the user follows or is assigned to
     * Org feed: all activities in the org (admin view)
     * Entity feed: all activities for a specific entity
     * Filters: entity_type, action, date range, actor
     * Paginated, most recent first
   
   - GetUnreadCounts(ctx, orgID, userID) → per-entity unread activity counts

   Following:
   - FollowEntity(ctx, userID, entityType, entityID, followType) → subscribe to updates
   - UnfollowEntity(ctx, userID, entityType, entityID) → unsubscribe
   - GetFollowedEntities(ctx, userID) → list of entities user is following
   - Auto-follow rules: user automatically follows entities they:
     * Own (risk owner, policy owner, control owner)
     * Are assigned to (finding assignee, action assignee)
     * Comment on
     * Are mentioned in

2. internal/handler/collaboration_handler.go — API Endpoints:

   Comments:
   - GET /comments/{entityType}/{entityId} — get threaded comments for an entity
   - POST /comments/{entityType}/{entityId} — create comment
   - PUT /comments/{id} — edit comment
   - DELETE /comments/{id} — soft delete comment
   - POST /comments/{id}/pin — pin/unpin comment
   - POST /comments/{id}/react — add/remove reaction
   - POST /comments/{entityType}/{entityId}/attachments — upload attachment for a comment

   Activity Feed:
   - GET /activity/feed — personal activity feed
   - GET /activity/org — organisation-wide activity feed (admin)
   - GET /activity/{entityType}/{entityId} — entity-specific activity feed
   - GET /activity/unread — unread counts per entity
   - POST /activity/{entityType}/{entityId}/mark-read — mark entity as read

   Following:
   - GET /following — list followed entities
   - POST /following/{entityType}/{entityId} — follow entity
   - DELETE /following/{entityType}/{entityId} — unfollow entity

3. NEXT.JS FRONTEND:

   - Comments Component (reusable, added to EVERY entity detail page):
     * Comments section at bottom of entity detail pages
     * Threaded display: top-level comments with indented replies
     * Markdown editor with preview, @mention autocomplete, file drag-and-drop
     * @mention: type '@' → dropdown of org users filtered by typed text
     * Reply button per comment → opens nested reply editor
     * Reaction buttons row under each comment
     * Pin button (for admin/owner)
     * Edit/Delete menu (for author)
     * "X new comments" banner when new comments arrive while viewing
     * Unread indicator: new comments since last visit highlighted with blue dot
   
   - Activity Feed (sidebar widget or /activity page):
     * Timeline display: avatar, user name, action description, entity link, relative time
     * Filter: by entity type, by action, by user
     * "Mark all read" button
     * Infinite scroll pagination
   
   - Following Management:
     * "Follow" / "Unfollow" button on every entity detail page header
     * /settings/following — manage all followed entities
   
   - Dashboard Widget:
     * "Recent Activity" feed on the dashboard (last 10 items)
     * Unread badge count on "Activity" navigation item in sidebar
   
   - Notification Integration:
     * @mentions trigger in-app notification (bell icon)
     * Comment on followed entity triggers notification
     * Digest option: daily summary of all activity on followed entities

CRITICAL REQUIREMENTS:
- Comments support Markdown: bold, italic, links, code blocks, bullet lists, numbered lists
- @mentions resolve to actual user IDs (not just display names) — prevent ambiguity
- @role mentions notify ALL users with that role in the org
- Comment attachments stored in org's file storage (not in the database)
- Activity feed entries are immutable — never deleted or modified
- Threaded replies max depth: 3 levels (prevent deeply nested discussions)
- Comment content is sanitised: no XSS, no script injection in rendered HTML
- Reactions are lightweight: no notification for reactions (would be too noisy)
- Performance: activity feed query <100ms for 1000+ activities
- Auto-follow is best-effort: failure doesn't block the main action
- Comments on vendor assessments can be marked 'external' (visible to vendor in portal)
- Search integration: comments are indexed in the search engine (Prompt 32)

OUTPUT: Complete Golang code for collaboration service, handlers, migration, and Next.js comments component + activity feed. Include unit tests for mention parsing, threading logic, and auto-follow rules.
```

---

### PROMPT 34 OF 100 — Mobile-Optimised API, Push Notifications & Responsive Design

```
You are a senior full-stack engineer building the mobile experience layer for "ComplianceForge" — a GRC platform.

OBJECTIVE:
Optimise the platform for mobile use: a dedicated mobile API surface with condensed payloads, push notifications via Firebase Cloud Messaging (FCM) and Apple Push Notification Service (APNs), a Progressive Web App (PWA) configuration, and ensure all Next.js pages are fully responsive. GRC professionals need to: approve workflow items on the go, be notified of security incidents instantly, review their upcoming deadlines, and check compliance status from their phone. The mobile experience must prioritise: approval inbox, incident alerts, deadline notifications, and dashboard metrics.

DATABASE SCHEMA — Create migration 029:

TABLE push_notification_tokens:
  - id UUID PK
  - user_id UUID FK → users
  - organization_id UUID FK (RLS)
  - platform ENUM('ios', 'android', 'web') NOT NULL
  - token TEXT NOT NULL — FCM/APNs token
  - token_hash VARCHAR(128) NOT NULL — SHA-256 for deduplication
  - device_name VARCHAR(200) — "iPhone 15 Pro", "Chrome on macOS"
  - device_model VARCHAR(100)
  - os_version VARCHAR(50)
  - app_version VARCHAR(20)
  - is_active BOOLEAN DEFAULT true
  - last_used_at TIMESTAMPTZ
  - created_at, updated_at
  - UNIQUE(token_hash)

TABLE push_notification_log:
  - id UUID PK
  - organization_id UUID FK (RLS)
  - user_id UUID FK → users
  - token_id UUID FK → push_notification_tokens
  - notification_type VARCHAR(100) — 'breach_alert', 'approval_request', 'incident_created', 'deadline_reminder', 'mention', 'comment'
  - title VARCHAR(300)
  - body TEXT
  - data JSONB — deep link data: {"entity_type": "incident", "entity_id": "...", "action": "view"}
  - status ENUM('sent', 'delivered', 'failed', 'invalid_token')
  - platform push_platform
  - sent_at TIMESTAMPTZ
  - error_message TEXT
  - created_at

TABLE user_mobile_preferences:
  - id UUID PK
  - user_id UUID FK → users UNIQUE
  - organization_id UUID FK (RLS)
  - push_enabled BOOLEAN DEFAULT true
  - push_breach_alerts BOOLEAN DEFAULT true — cannot be disabled for breach alerts
  - push_approval_requests BOOLEAN DEFAULT true
  - push_incident_alerts BOOLEAN DEFAULT true
  - push_deadline_reminders BOOLEAN DEFAULT true
  - push_mentions BOOLEAN DEFAULT true
  - push_comments BOOLEAN DEFAULT false — off by default (too noisy)
  - quiet_hours_enabled BOOLEAN DEFAULT false
  - quiet_hours_start TIME DEFAULT '22:00'
  - quiet_hours_end TIME DEFAULT '07:00'
  - quiet_hours_timezone VARCHAR(50)
  - quiet_hours_override_critical BOOLEAN DEFAULT true — critical alerts ignore quiet hours
  - created_at, updated_at

GOLANG IMPLEMENTATION:

1. internal/service/push_service.go — Push Notification Service:

   PushService struct:
   
   - RegisterToken(ctx, userID, platform, token, deviceInfo) → register device:
     * Hash token with SHA-256
     * Upsert: if token_hash exists, update device info and last_used_at
     * If user has >5 tokens, deactivate oldest
   
   - UnregisterToken(ctx, userID, tokenHash) → deactivate token
   
   - SendPush(ctx, userID, notification) → send push to all user's active devices:
     * Check user's mobile_preferences: is this notification type enabled?
     * Check quiet hours: if within quiet hours AND not critical, queue for later
     * For each active token:
       - FCM (Android + Web): POST to Firebase Cloud Messaging API
       - APNs (iOS): POST to Apple Push Notification Service
     * Log delivery in push_notification_log
     * Handle invalid tokens: if FCM returns "not registered", deactivate the token
   
   - SendBulkPush(ctx, userIDs, notification) → send to multiple users
   
   Notification Types with payloads:
   
   a. breach_alert: title="⚠️ GDPR Data Breach Alert", body="INC-0001: {title}. {hours} hours remaining.", data={entity_type: 'incident', entity_id, action: 'view', priority: 'critical'}
   b. approval_request: title="Approval Required", body="{entity_type} {entity_ref}: {title}", data={entity_type: 'workflow_execution', entity_id, action: 'approve'}
   c. incident_created: title="New {severity} Incident", body="INC-XXXX: {title}", data={entity_type: 'incident', entity_id}
   d. deadline_reminder: title="Deadline: {days} days", body="{event_title} due on {date}", data={entity_type: 'calendar_event', entity_id}
   e. mention: title="{user} mentioned you", body="on {entity_type} {entity_ref}: '{snippet}'", data={entity_type, entity_id, comment_id}
   f. comment: title="New comment on {entity_ref}", body="{user}: {snippet}", data={entity_type, entity_id, comment_id}

2. internal/handler/mobile_handler.go — Mobile-Optimised API:

   The mobile API provides condensed response payloads optimised for mobile bandwidth and rendering:
   
   - GET /mobile/dashboard → condensed dashboard:
     * compliance_score, risk_counts (critical/high only), open_incidents, pending_approvals, overdue_deadlines
     * No charts data — mobile dashboard shows numbers only
     * Cached aggressively: 2 minute TTL
   
   - GET /mobile/approvals → pending approvals for current user:
     * Condensed: workflow_execution_id, entity_type, entity_ref, title, step_name, sla_deadline, priority
     * Only fields needed to approve/reject
   
   - POST /mobile/approvals/{id}/approve → approve with optional comment
   - POST /mobile/approvals/{id}/reject → reject with required reason
   
   - GET /mobile/incidents/active → open incidents (condensed):
     * id, ref, title, severity, status, is_breach, hours_remaining (if breach)
     * Sorted by severity then recency
   
   - GET /mobile/deadlines?days=7 → upcoming deadlines:
     * event_type, title, date, days_until, priority, assigned_to_me
   
   - GET /mobile/activity?limit=20 → recent activity feed (condensed)
   
   - POST /mobile/push/register → register push token
   - DELETE /mobile/push/unregister → unregister push token
   - GET /mobile/push/preferences → get push preferences
   - PUT /mobile/push/preferences → update push preferences
   
   Mobile responses include:
   - Minimal JSONB: only essential fields, no nested objects where avoidable
   - Pagination: smaller default page_size (10 vs 20)
   - ETag headers for caching
   - Compressed with gzip (already via middleware)

3. internal/service/push_integration.go — Integration with Notification Engine:

   Connect push service to the notification engine (Prompt 11):
   - When notification_engine dispatches a notification:
     * If channel='in_app': also check if user has push tokens → send push
     * If event is 'breach.detected' or 'breach.deadline_approaching': ALWAYS send push (bypass preferences)
     * If event is workflow step assignment: send push if push_approval_requests=true
     * If event is @mention: send push if push_mentions=true
   - Quiet hours enforcement:
     * During quiet hours: queue non-critical push notifications
     * At quiet_hours_end: send all queued notifications as a batch
     * Critical alerts (breach, critical incident): send immediately regardless

4. NEXT.JS PWA Configuration:

   a. next.config.js: add PWA plugin (next-pwa):
      - Service worker registration
      - Offline fallback page
      - Cache strategies: NetworkFirst for API, CacheFirst for static assets
   
   b. public/manifest.json:
      - name: "ComplianceForge"
      - short_name: "CF GRC"
      - theme_color: Indigo-600
      - background_color: white
      - display: "standalone"
      - icons: 192x192, 512x512 (include maskable variants)
      - start_url: "/dashboard"
      - scope: "/"
   
   c. Service Worker:
      - Cache: app shell (layout, nav, CSS)
      - Cache: recent API responses (dashboard, approvals)
      - Offline page: "You're offline. Some features require internet."
      - Push notification handler: display notification, handle click → open deep link
   
   d. Web Push Registration:
      - On first dashboard load: request notification permission
      - If granted: register with FCM via VAPID
      - Send token to POST /mobile/push/register with platform='web'

5. RESPONSIVE DESIGN AUDIT:

   Ensure ALL existing pages are fully responsive. Provide specific Tailwind changes for:
   
   a. Sidebar: hidden on mobile, hamburger menu toggle, drawer overlay
   b. Dashboard: KPI cards stack to 2-column on tablet, 1-column on mobile; charts full-width
   c. Data Tables: horizontal scroll on mobile with sticky first column; or card view toggle
   d. Forms: full-width inputs on mobile, single-column layout
   e. Modals/Slide-overs: full-screen on mobile
   f. Calendar: month view → simplified list view on mobile
   g. Board portal: fully mobile-optimised (board members check on their phones)
   h. Vendor portal: fully mobile-optimised (vendors may fill questionnaires on tablets)

   Create a responsive design specification document covering breakpoints:
   - Mobile: <640px (sm)
   - Tablet: 640-1024px (md)
   - Desktop: 1024-1280px (lg)
   - Large Desktop: >1280px (xl)

CRITICAL REQUIREMENTS:
- Push notifications for GDPR breach alerts CANNOT be disabled by user preferences (regulatory)
- Push tokens must be validated regularly: send a silent push monthly, deactivate undeliverable tokens
- FCM requires a Firebase project setup — document the configuration steps
- APNs requires an Apple Developer account — document the certificate/key setup
- Quiet hours respect the user's timezone (not server timezone)
- Deep links in push notifications must route to the correct page in the app
- PWA must score >90 on Lighthouse PWA audit
- Offline mode: dashboard shows last-cached data with "Last updated X minutes ago" indicator
- Mobile approval flow must be completable in <30 seconds (tap notification → review → approve → done)
- Battery optimisation: push notifications, not polling — no background sync except when app is active
- Data usage optimisation: mobile API responses are 50-70% smaller than desktop API responses

OUTPUT: Complete Golang code for push service, mobile API handlers, push integration, migration, Next.js PWA configuration (manifest, service worker, web push registration), responsive design specifications, and mobile-specific components (mobile dashboard, approval card, deadline list). Include setup documentation for FCM and APNs.
```

---

### PROMPT 35 OF 100 — Multi-Tenant White-Labelling, Custom Branding & Theming Engine

```
You are a senior full-stack engineer building the white-labelling and custom branding system for "ComplianceForge" — a GRC SaaS platform.

OBJECTIVE:
Build a theming engine that allows each tenant (organisation) to fully customise the platform's visual identity: logo, colours, fonts, email templates, PDF reports, login page, and even the product name. This enables ComplianceForge to be resold by consulting firms and system integrators under their own brand (white-label partnerships), and allows enterprise customers to match their corporate identity. The theming must be dynamic (no code changes or redeployment needed) and must apply across: web frontend, PDF reports, email notifications, the vendor portal, and the board portal.

DATABASE SCHEMA — Create migration 030:

TABLE tenant_branding:
  - id UUID PK
  - organization_id UUID FK → organizations UNIQUE (RLS)
  
  -- Identity
  - product_name VARCHAR(200) DEFAULT 'ComplianceForge' — custom product name
  - tagline VARCHAR(300) DEFAULT 'Enterprise GRC Platform'
  - company_name VARCHAR(200) — shown in footer, reports
  - support_email VARCHAR(300)
  - support_url TEXT
  - privacy_policy_url TEXT
  - terms_url TEXT
  
  -- Logos
  - logo_url TEXT — full logo (sidebar, reports, emails) — recommended 180×40px SVG/PNG
  - logo_icon_url TEXT — square icon (favicon, mobile) — recommended 64×64px
  - logo_dark_url TEXT — logo variant for dark mode
  - login_background_url TEXT — custom login page background image
  - email_header_logo_url TEXT — logo for email headers
  - report_logo_url TEXT — high-res logo for PDF reports
  
  -- Colours
  - primary_color VARCHAR(7) DEFAULT '#4F46E5' — main brand colour (buttons, links, active states)
  - primary_hover_color VARCHAR(7) DEFAULT '#4338CA'
  - primary_light_color VARCHAR(7) DEFAULT '#EEF2FF' — light variant (backgrounds)
  - secondary_color VARCHAR(7) DEFAULT '#6366F1'
  - accent_color VARCHAR(7) DEFAULT '#8B5CF6'
  - success_color VARCHAR(7) DEFAULT '#22C55E'
  - warning_color VARCHAR(7) DEFAULT '#F59E0B'
  - danger_color VARCHAR(7) DEFAULT '#EF4444'
  - info_color VARCHAR(7) DEFAULT '#3B82F6'
  - sidebar_bg_color VARCHAR(7) DEFAULT '#FFFFFF'
  - sidebar_text_color VARCHAR(7) DEFAULT '#374151'
  - sidebar_active_bg VARCHAR(7) DEFAULT '#EEF2FF'
  - sidebar_active_text VARCHAR(7) DEFAULT '#4F46E5'
  - topbar_bg_color VARCHAR(7) DEFAULT '#FFFFFF'
  - topbar_text_color VARCHAR(7) DEFAULT '#111827'
  - login_bg_color VARCHAR(7) DEFAULT '#F3F4F6'
  
  -- Typography
  - font_family VARCHAR(200) DEFAULT 'Inter' — web font name
  - font_url TEXT — custom font CSS URL (Google Fonts or self-hosted)
  - heading_font_family VARCHAR(200) — separate heading font (optional)
  
  -- Layout
  - sidebar_style ENUM('light', 'dark', 'branded') DEFAULT 'light'
  - corner_radius ENUM('none', 'small', 'medium', 'large') DEFAULT 'medium'
  - density ENUM('compact', 'default', 'comfortable') DEFAULT 'default'
  
  -- Custom Domain
  - custom_domain VARCHAR(300) — e.g., 'compliance.acme-consulting.com'
  - custom_domain_verified BOOLEAN DEFAULT false
  - custom_domain_ssl_status ENUM('pending', 'active', 'failed')
  
  -- Custom CSS
  - custom_css TEXT — additional CSS overrides (advanced, validated)
  
  -- Feature Flags
  - show_powered_by BOOLEAN DEFAULT true — "Powered by ComplianceForge" in footer
  - show_help_widget BOOLEAN DEFAULT true
  - show_marketplace BOOLEAN DEFAULT true
  - show_knowledge_base BOOLEAN DEFAULT true
  
  - created_at, updated_at

TABLE white_label_partners:
  - id UUID PK
  - partner_name VARCHAR(200) NOT NULL
  - partner_slug VARCHAR(100) NOT NULL UNIQUE
  - contact_email VARCHAR(300)
  - default_branding_id UUID FK → tenant_branding — default branding for all partner's clients
  - revenue_share_percent DECIMAL(5,2) — partner gets X% of subscription revenue
  - max_tenants INT — how many clients the partner can onboard
  - is_active BOOLEAN DEFAULT true
  - created_at, updated_at

TABLE partner_tenant_mappings:
  - id UUID PK
  - partner_id UUID FK → white_label_partners
  - organization_id UUID FK → organizations
  - onboarded_at TIMESTAMPTZ
  - created_at

GOLANG IMPLEMENTATION:

1. internal/service/branding_service.go:

   - GetBranding(ctx, orgID) → returns tenant_branding or defaults:
     * If org has custom branding: return it
     * If org belongs to a white-label partner: return partner's default branding
     * Otherwise: return ComplianceForge default branding
     * Cache in Redis with 5-minute TTL (branding rarely changes)
   
   - UpdateBranding(ctx, orgID, req) → update branding settings:
     * Validate colour hex codes (must be valid #XXXXXX)
     * Validate URLs (must be https for logos in production)
     * Validate custom CSS: sanitise to prevent XSS (whitelist: color, background, font, border, margin, padding)
     * Invalidate Redis cache
   
   - UploadLogo(ctx, orgID, logoType, file) → upload and store logo:
     * Validate: SVG, PNG, or JPEG
     * Validate dimensions: min 64px, max 2000px
     * Store in file storage: orgs/{orgID}/branding/{logo_type}.{ext}
     * Generate thumbnail for email use
     * Return URL
   
   - VerifyCustomDomain(ctx, orgID, domain) → custom domain setup:
     * Require CNAME record pointing to {orgSlug}.complianceforge.io
     * Require TXT record for ownership verification
     * DNS lookup to verify records
     * Trigger SSL certificate provisioning (Let's Encrypt via cert-manager)
   
   - GetBrandingCSS(ctx, orgID) → generate CSS variables from branding config:
     ```css
     :root {
       --cf-primary: #4F46E5;
       --cf-primary-hover: #4338CA;
       --cf-primary-light: #EEF2FF;
       --cf-success: #22C55E;
       --cf-warning: #F59E0B;
       --cf-danger: #EF4444;
       --cf-sidebar-bg: #FFFFFF;
       --cf-font-family: 'Inter', sans-serif;
       --cf-radius: 8px;
     }
     ```
   
   - ApplyBrandingToEmail(ctx, orgID, emailHTML) → replace ComplianceForge branding in email templates:
     * Replace logo URL
     * Replace primary colour
     * Replace product name
     * Replace footer company name and links
   
   - ApplyBrandingToPDF(ctx, orgID, reportConfig) → inject branding into PDF reports:
     * Custom logo on cover page
     * Custom header colour
     * Custom footer text
     * Company name in headers

2. internal/middleware/branding.go — Branding Middleware:
   
   - Attach branding config to request context for every authenticated request
   - The frontend fetches branding via API and applies CSS variables

3. internal/handler/branding_handler.go — API Endpoints:
   
   - GET /branding — get current org's branding (public, cached)
   - GET /branding/css — get generated CSS variables (public, cached, Content-Type: text/css)
   - PUT /branding — update branding (admin only)
   - POST /branding/logo — upload logo (admin only)
   - DELETE /branding/logo/{type} — remove custom logo
   - POST /branding/domain/verify — verify custom domain
   - GET /branding/domain/status — custom domain SSL status
   - POST /branding/preview — preview branding without saving (returns CSS + config)
   
   White-Label Partner (super admin):
   - GET /admin/partners — list white-label partners
   - POST /admin/partners — create partner
   - PUT /admin/partners/{id} — update partner
   - GET /admin/partners/{id}/tenants — list partner's tenants

4. NEXT.JS FRONTEND:

   a. Branding Provider (src/components/providers/branding-provider.tsx):
      - Fetch GET /branding on app load
      - Inject CSS custom properties into document root
      - Load custom font if specified
      - Replace favicon with custom icon
      - Set document title to custom product name
      - Provide branding context to all child components
   
   b. Dynamic Component Rendering:
      - Sidebar: logo from branding, background color from config
      - Login page: custom background, custom logo, custom product name
      - Topbar: custom logo, custom colours
      - Emails: branding applied server-side before sending
      - PDFs: branding applied during generation
      - Footer: custom company name, "Powered by ComplianceForge" toggle
      - Vendor portal: org's branding applied
      - Board portal: org's branding applied
   
   c. Branding Settings Page (/settings/branding):
      - Live preview panel showing a miniature version of the dashboard with current settings
      - Logo upload areas (drag-and-drop) for each logo type
      - Colour picker for each colour setting (with hex input)
      - Font selector (dropdown of Google Fonts + custom URL input)
      - Layout options: sidebar style, corner radius, density
      - Custom domain setup wizard:
        Step 1: Enter desired domain
        Step 2: Show required DNS records (CNAME + TXT)
        Step 3: "Verify Domain" button → checks DNS → provisions SSL
        Step 4: Active ✓
      - "Powered by ComplianceForge" toggle
      - Custom CSS editor (code editor with syntax highlighting, validated on save)
      - "Preview" button → applies changes temporarily without saving
      - "Reset to Default" button
   
   d. Tailwind Integration:
      - All components use CSS custom properties instead of hardcoded Tailwind colours:
        * `bg-[var(--cf-primary)]` instead of `bg-indigo-600`
        * `text-[var(--cf-primary)]` instead of `text-indigo-600`
      - Utility classes: `cf-bg-primary`, `cf-text-primary`, `cf-border-primary` mapped to CSS variables
      - Sidebar: dynamically themed based on sidebar_style setting

5. WHITE-LABEL PARTNER PORTAL:
   - /admin/partners — partner management (ComplianceForge super admin only):
     * Partner list with tenant count, revenue share, status
     * Create partner with default branding template
     * Assign existing orgs to partners
     * Partner analytics: total tenants, MRR from partner's clients, churn
   - Partner onboarding: partner provides their branding → applied to all their clients by default

CRITICAL REQUIREMENTS:
- Branding changes apply IMMEDIATELY (no deployment needed): cache invalidation + CSS variable injection
- Custom CSS is SANITISED: only allow safe CSS properties, no JavaScript, no url() with data: URIs
- Custom domains require HTTPS: cert-manager handles Let's Encrypt certificate provisioning
- Logo uploads: max 2MB per file, validated dimensions, converted to WebP for performance
- Branding is applied EVERYWHERE: login, dashboard, emails, PDFs, vendor portal, board portal, error pages
- Default branding (ComplianceForge) is used when no custom branding is set
- White-label: partner's branding is the default for their clients; clients can further customise
- "Powered by ComplianceForge" is mandatory for Starter and Professional plans; optional for Enterprise
- Performance: branding CSS is cached at CDN edge (max-age: 300) with ETag for efficient invalidation
- Dark mode: custom colours must work in dark mode (auto-generate dark variants or require separate dark colours)
- Accessibility: branding colours must meet WCAG AA contrast ratios — validate during save and warn if insufficient

OUTPUT: Complete Golang code for branding service, middleware, handlers, migration, Next.js branding provider, branding settings page with live preview, and CSS variable system. Include documentation for white-label partner setup and custom domain configuration.
```

---

## BATCH 7 SUMMARY

| Prompt | Focus Area | New Tables | New Endpoints | Key Capabilities |
|--------|-----------|------------|---------------|------------------|
| 31 | Compliance Calendar | 3 (events, subscriptions, sync_configs) | ~14 | Unified calendar aggregating ALL deadlines from every module, iCal export for external calendars, configurable reminders (7d/3d/1d/due/overdue), automatic overdue escalation, month/week/agenda views, weekly digest email |
| 32 | Advanced Search & KB | 4 (search_index, knowledge_articles, bookmarks, recent_searches) | ~16 | Unified full-text search across all entities with faceted filtering, Ctrl+K command palette, 50+ knowledge base articles, contextual guidance per control, autocomplete, ABAC-aware search results |
| 33 | Collaboration & Comments | 4 (comments, activity_feed, user_follows, read_markers) | ~14 | Threaded comments on any entity, @mentions with notifications, markdown support, file attachments, emoji reactions, activity feed, auto-follow rules, unread tracking |
| 34 | Mobile API & Push | 3 (push_tokens, push_log, mobile_preferences) | ~14 | Condensed mobile API, FCM/APNs push notifications, PWA with offline support, quiet hours, breach alerts bypass user preferences, responsive design audit, mobile approval flow |
| 35 | White-Labelling | 3 (tenant_branding, white_label_partners, partner_mappings) | ~12 | Full visual customisation (logo, colours, fonts, layout), dynamic CSS variables, custom domain with SSL, white-label partner programme, branding applied to: web, email, PDF, vendor portal, board portal |

**Running Total: 35/100 Prompts | ~137 Tables | ~405+ API Endpoints | Complete Enterprise SaaS Platform**

---

> **NEXT BATCH (Prompts 36–40):** Compliance-as-Code Engine (policy-as-code, control-as-code with Git sync), Multi-Region Deployment & Data Residency Controls, Advanced Audit Management (audit programme, sampling, workpapers), Compliance Training & Certification Tracking Module, and API Versioning, Webhooks Marketplace & Developer Portal.
>
> Type **"next"** to continue.
