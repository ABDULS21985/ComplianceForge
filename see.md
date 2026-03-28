# GRC Compliance Management Solution — 100 Master Prompts

## BATCH 2 — Next.js Frontend (Real API Integration), CI/CD & E2E Testing

**Stack:** Next.js 14 (App Router) | TypeScript | Tailwind CSS | shadcn/ui | Zustand | Recharts
**Backend API:** Golang REST API at `http://localhost:8080/api/v1` (JWT Bearer auth)
**Prerequisite:** Backend from Prompts 1–5 must be running with seeded database

---

### PROMPT 6 OF 100 — Next.js Frontend: Core Architecture, Authentication & Layout System

```
You are a senior frontend architect building the Next.js 14 frontend for "ComplianceForge" — an enterprise GRC platform. The Golang backend API is already built and running at http://localhost:8080/api/v1 with JWT authentication.

OBJECTIVE:
Build the complete frontend foundation: project setup, authentication flow, API client, state management, reusable component library, and the main application shell/layout. ALL data must come from the real backend API — NO mock data, NO placeholder responses, NO hardcoded arrays.

TECH STACK:
- Next.js 14 (App Router with Server Components where appropriate)
- TypeScript 5.4+ (strict mode)
- Tailwind CSS 3.4+ with custom design tokens
- shadcn/ui component library (Radix UI primitives)
- Zustand 4+ for client state management
- React Query (TanStack Query v5) for server state & caching
- date-fns for date formatting
- lucide-react for icons
- recharts for data visualization
- next-themes for dark mode support

PROJECT STRUCTURE:
complianceforge/frontend/
├── src/
│   ├── app/                          # Next.js App Router
│   │   ├── (auth)/                   # Auth route group (no sidebar)
│   │   │   ├── login/page.tsx
│   │   │   ├── forgot-password/page.tsx
│   │   │   └── layout.tsx            # Auth layout (centered card, no sidebar)
│   │   ├── (dashboard)/              # Main app route group (with sidebar)
│   │   │   ├── dashboard/page.tsx
│   │   │   ├── frameworks/
│   │   │   ├── risks/
│   │   │   ├── policies/
│   │   │   ├── audits/
│   │   │   ├── incidents/
│   │   │   ├── vendors/
│   │   │   ├── assets/
│   │   │   ├── settings/
│   │   │   └── layout.tsx            # Dashboard layout (sidebar + topbar + content)
│   │   ├── layout.tsx                # Root layout (providers, fonts, metadata)
│   │   ├── page.tsx                  # Redirect to /dashboard
│   │   └── globals.css
│   ├── components/
│   │   ├── ui/                       # shadcn/ui components (Button, Input, Card, etc.)
│   │   ├── layout/
│   │   │   ├── sidebar.tsx           # Collapsible sidebar navigation
│   │   │   ├── topbar.tsx            # Top bar with breadcrumbs, search, user menu
│   │   │   ├── breadcrumbs.tsx       # Dynamic breadcrumbs from route segments
│   │   │   └── user-menu.tsx         # User dropdown (profile, settings, logout)
│   │   ├── data/
│   │   │   ├── data-table.tsx        # Reusable table with sorting, filtering, pagination
│   │   │   ├── stat-card.tsx         # KPI card (value, label, trend, color)
│   │   │   ├── badge-status.tsx      # Color-coded status/severity badges
│   │   │   ├── empty-state.tsx       # Empty state illustration with action
│   │   │   └── loading-skeleton.tsx  # Skeleton loaders matching each page layout
│   │   ├── charts/
│   │   │   ├── compliance-radar.tsx  # Radar chart for multi-framework scores
│   │   │   ├── risk-heatmap.tsx      # 5×5 risk heatmap (interactive)
│   │   │   ├── trend-chart.tsx       # Line chart for score trends
│   │   │   └── donut-chart.tsx       # Donut chart for distributions
│   │   └── forms/
│   │       ├── form-field.tsx        # Label + input + error wrapper
│   │       ├── select-field.tsx      # Dropdown with search
│   │       └── file-upload.tsx       # Drag-and-drop file upload (evidence)
│   ├── lib/
│   │   ├── api.ts                    # Typed API client class
│   │   ├── api-hooks.ts             # React Query hooks wrapping every API endpoint
│   │   ├── auth.ts                   # Auth utilities (token storage, refresh, redirect)
│   │   ├── store.ts                  # Zustand stores (auth, notifications, sidebar)
│   │   ├── utils.ts                  # cn(), formatDate(), formatCurrency(), truncate()
│   │   ├── constants.ts             # Status colors, severity mappings, framework metadata
│   │   └── validators.ts            # Zod schemas for form validation
│   └── types/
│       └── index.ts                  # TypeScript interfaces matching ALL Go backend models
├── public/
├── package.json
├── tsconfig.json
├── tailwind.config.ts
├── postcss.config.js
├── next.config.js
└── .env.local.example

DETAILED REQUIREMENTS:

1. API CLIENT (src/lib/api.ts):
   - Singleton class with typed methods for EVERY backend endpoint (60+ methods)
   - Automatic JWT token attachment from localStorage
   - 401 response interceptor → clear token → redirect to /login
   - Automatic retry with exponential backoff for 5xx errors (max 3 retries)
   - Request/response logging in development mode
   - AbortController integration for request cancellation
   - Generic request<T> method with full TypeScript inference
   - Methods grouped by domain: auth, frameworks, compliance, risks, policies, audits, incidents, vendors, assets, settings, reports

2. REACT QUERY HOOKS (src/lib/api-hooks.ts):
   - Create a custom hook for every API call using useQuery / useMutation
   - Proper cache keys scoped by entity (e.g., ['risks', page, sortBy])
   - Stale time: 30 seconds for dashboards, 5 minutes for framework lists
   - Automatic refetch on window focus for critical data (incidents, breaches)
   - Optimistic updates for status changes (risk status, control status)
   - Invalidation patterns: e.g., creating a risk invalidates ['risks'], ['dashboard']
   - Error handling with toast notifications
   - Example hooks:
     * useFrameworks() → GET /frameworks
     * useComplianceScores() → GET /compliance/scores
     * useRisks(page, sort, filter) → GET /risks?page=&sort_by=&status=
     * useCreateRisk() → POST /risks (mutation)
     * useDashboard() → GET /dashboard/summary
     * useIncidentStats() → GET /incidents/stats (refetchOnWindowFocus: true)
     * useUrgentBreaches() → GET /incidents/breaches/urgent (refetchInterval: 60000)

3. AUTHENTICATION FLOW:
   - Login page with email + password form
   - On success: store JWT in localStorage, redirect to /dashboard
   - Auth middleware using Next.js middleware.ts: redirect unauthenticated users to /login
   - Protected route wrapper component that checks token validity
   - Auto-refresh token before expiry (if refresh_token endpoint exists)
   - Logout: clear token, clear React Query cache, redirect to /login

4. LAYOUT SYSTEM:
   - Collapsible sidebar (expanded/collapsed state persisted in localStorage)
   - Navigation items with active state detection from current route
   - Navigation items: Dashboard, Frameworks, Risk Register, Policies, Audits, Incidents, Vendors, Assets, Settings
   - Badge counters on navigation items (e.g., Incidents shows count of open incidents from API)
   - Top bar with: Breadcrumbs (auto-generated from route), Global search (Ctrl+K), Notification bell, User avatar + dropdown
   - User dropdown: Profile, Organisation Settings, Audit Log, Logout
   - Responsive: sidebar collapses to icon-only on tablet, becomes a drawer on mobile
   - Footer with: ComplianceForge version, data residency region, last sync time

5. DATA TABLE COMPONENT (src/components/data/data-table.tsx):
   - Server-side pagination (page, page_size passed to API)
   - Column sorting (sort_by, sort_dir passed to API)
   - Search/filter bar with debounced input
   - Status filter dropdown
   - Column visibility toggle
   - Row click navigation to detail page
   - Bulk selection with checkboxes
   - Export button (CSV)
   - Loading skeleton matching the table layout
   - Empty state when no results
   - "Showing X–Y of Z results" footer with page navigation

6. TYPE DEFINITIONS (src/types/index.ts):
   Create TypeScript interfaces for EVERY entity returned by the backend API:
   - APIResponse<T>, PaginatedResponse<T>, APIError, Pagination
   - User, Role, LoginResponse
   - ComplianceFramework, FrameworkDomain, FrameworkControl
   - ComplianceScore, ComplianceScoreSummary, GapAnalysisEntry
   - CrossFrameworkMapping
   - Risk, RiskHeatmapEntry, RiskTreatment, RiskCategory
   - Policy, PolicyVersion, PolicyAttestation
   - Audit, AuditFinding
   - Incident (with GDPR breach fields: is_data_breach, notification_deadline, dpa_notified_at, data_subjects_affected)
   - Vendor (with DPA fields: data_processing, dpa_in_place, certifications)
   - Asset
   - ControlImplementation, ControlEvidence, ControlTestResult
   - DashboardSummary
   - All enum types matching Go backend: RiskLevel, ControlStatus, PolicyStatus, IncidentSeverity, AuditStatus, FindingSeverity, etc.

7. DESIGN SYSTEM:
   - Professional enterprise aesthetic (similar to Drata, Vanta, or ServiceNow)
   - Color palette: Indigo-600 primary, semantic colors for risk levels (red/orange/yellow/green)
   - Typography: Inter font, clear hierarchy (text-2xl for page titles, text-lg for section headings)
   - Consistent spacing: 6px grid
   - Dark mode support via next-themes (respect system preference)
   - Consistent card styling with subtle borders and shadows
   - Status badges with background colors matching severity
   - Loading states: skeleton loaders that match the actual layout

OUTPUT: Complete, production-ready TypeScript/React code for every file listed. Every component must fetch real data from the API — ZERO mock data, ZERO hardcoded arrays, ZERO placeholder responses. All files must compile with `next build` without errors.
```

---

### PROMPT 7 OF 100 — Dashboard, Compliance & Framework Pages (Real API Data)

```
You are a senior React/TypeScript developer building pages for "ComplianceForge" — an enterprise GRC platform built with Next.js 14. The API client, React Query hooks, layout system, and component library from Prompt 6 are available.

OBJECTIVE:
Build the Dashboard, Compliance Overview, and Framework Management pages. ALL data is fetched from the real Golang backend API using the React Query hooks — NO mock data whatsoever. Every loading state, error state, and empty state must be handled.

PREREQUISITE CODE AVAILABLE:
- src/lib/api.ts (typed API client)
- src/lib/api-hooks.ts (React Query hooks for every endpoint)
- src/components/data/data-table.tsx (reusable table)
- src/components/data/stat-card.tsx (KPI cards)
- src/components/charts/*.tsx (compliance-radar, risk-heatmap, trend-chart, donut-chart)
- src/types/index.ts (all TypeScript interfaces)
- src/lib/utils.ts (cn, formatDate, formatCurrency)
- src/lib/constants.ts (status colors, severity mappings)

BACKEND API ENDPOINTS AVAILABLE:
- GET /dashboard/summary → DashboardSummary
- GET /frameworks → ComplianceFramework[]
- GET /frameworks/{id} → Framework with domains
- GET /frameworks/{id}/controls?page=&page_size= → Paginated controls
- GET /frameworks/{id}/implementations?status= → Control implementations
- GET /frameworks/controls/search?q=&limit= → Control search
- GET /compliance/scores → ComplianceScore[]
- GET /compliance/gaps?framework_id= → GapAnalysisEntry[]
- GET /compliance/cross-mapping → CrossFrameworkMapping[]

BUILD THE FOLLOWING PAGES:

PAGE 1 — Executive Dashboard (src/app/(dashboard)/dashboard/page.tsx):

The dashboard is the CEO/CISO landing page. It must tell the compliance story at a glance.

Section A — GDPR Breach Alert Banner:
- Fetch GET /incidents/breaches/urgent
- If any breaches are approaching the 72-hour deadline, show a red alert banner at the top
- Each breach shows: incident ref, title, hours remaining, data subjects affected
- "Notify DPA" button per breach that calls POST /incidents/{id}/notify-dpa
- This section auto-refreshes every 60 seconds (React Query refetchInterval)

Section B — KPI Cards Row (6 cards):
- Overall Compliance Score (from GET /dashboard/summary → color: green ≥80%, amber ≥60%, red <60%)
- Total Open Risks (from GET /dashboard/summary → risk_summary, subtitle: "X critical")
- Open Incidents (show count, color by severity)
- Open Audit Findings (with overdue count)
- Policies Due for Review (count, amber if >3)
- High-Risk Vendors (count, red if >5)

Section C — Framework Compliance Chart:
- Horizontal bar chart showing each adopted framework's compliance score
- Data from GET /compliance/scores
- Bars color-coded by score (green/amber/red)
- Click on a bar navigates to /frameworks/{id}

Section D — Risk Distribution:
- Donut chart showing risks by residual_risk_level (critical/high/medium/low)
- Legend with counts
- "View Register" link to /risks

Section E — Recent Activity Feed:
- Timeline of recent actions from GET /settings/audit-log?page=1&page_size=10
- Each entry: user avatar, action description, entity link, timestamp (relative: "2 hours ago")

Section F — Compliance Maturity Radar:
- Radar chart with each framework as an axis
- Values are the maturity_avg from compliance scores (0–5 scale)
- Overlay: current vs target maturity

Section G — Quick Actions:
- "Register Risk", "Report Incident", "Draft Policy", "Plan Audit", "Search Controls"
- Each opens the respective create form or navigates to the list with ?action=new

PAGE 2 — Frameworks List (src/app/(dashboard)/frameworks/page.tsx):
- Fetch GET /frameworks
- Display as grid of cards (responsive: 3 columns desktop, 2 tablet, 1 mobile)
- Each card shows: framework icon/color, name, version, issuing body, category badge, total_controls count
- Click navigates to /frameworks/[id]
- "Adopt Framework" button opens a modal listing un-adopted frameworks
- Loading state: 9 skeleton cards
- Empty state: if no frameworks adopted yet

PAGE 3 — Framework Detail (src/app/(dashboard)/frameworks/[id]/page.tsx):
- Dynamic route: fetch GET /frameworks/{id} for framework metadata
- Tabs: "Controls", "Implementation Status", "Gap Analysis", "Cross-Mapping"
- Tab 1 — Controls:
  * Fetch GET /frameworks/{id}/controls with pagination
  * Use DataTable component with columns: Code, Title, Type, Implementation Type, Priority
  * Full-text search bar connected to GET /frameworks/controls/search?q=
  * Click on a control opens a slide-over panel with full details

- Tab 2 — Implementation Status:
  * Fetch GET /frameworks/{id}/implementations
  * Summary cards: Implemented, Partial, Not Implemented, Not Applicable, Total
  * Progress bar showing overall implementation percentage
  * DataTable with columns: Control Code, Title, Status (badge), Maturity Level, Owner, Last Tested
  * Status filter dropdown: All, Not Implemented, Partial, Implemented
  * Click on a row navigates to /controls/{implementation_id}

- Tab 3 — Gap Analysis:
  * Fetch GET /compliance/gaps?framework_id={id}
  * DataTable showing: Control Code, Title, Current Status, Risk If Not Implemented (badge), Owner, Remediation Due Date
  * Sort by risk_if_not_implemented (critical first)
  * Total gaps count at top
  * "Export Gaps to CSV" button

- Tab 4 — Cross-Framework Mapping:
  * Fetch GET /compliance/cross-mapping filtered to this framework
  * Show which controls in this framework map to controls in other frameworks
  * Display: Source Control, Target Framework, Target Control, Mapping Type (badge), Strength (progress bar)
  * Visual indicator: "Implementing A.5.1 also covers GV.PO-01 (95%), 12.1.1 (90%)"

PAGE 4 — Control Implementation Detail (src/app/(dashboard)/controls/[id]/page.tsx):
- Fetch GET /controls/{id} → returns implementation + evidence + control details
- Header: Control code, title, framework badge, status badge, maturity level
- Left column (60%): Implementation details
  * Status selector (dropdown that calls PUT /controls/{id})
  * Maturity level slider (0–5 with labels: Non-existent → Optimizing)
  * Owner selector (dropdown of org users)
  * Implementation description (rich text area)
  * Gap description (text area, shown if status != implemented)
  * Remediation plan (text area with due date picker)
  * Compensating control description (text area)
  * Save button → PUT /controls/{id}
- Right column (40%): Evidence & Testing
  * Evidence list (from GET response)
  * Each evidence: title, type badge, file name, upload date, review status badge
  * "Upload Evidence" button → file upload modal → POST /controls/{id}/evidence (multipart)
  * "Record Test" button → modal form → POST /controls/{id}/test
  * Test history section showing past test results

CRITICAL REQUIREMENTS:
1. EVERY page must use React Query hooks to fetch data from the REAL API
2. EVERY page must handle: loading state (skeleton), error state (retry button), empty state
3. EVERY form submission must use useMutation with proper loading/success/error handling
4. EVERY list must support pagination from the API (not client-side)
5. Toast notifications on success/error for all mutations
6. Breadcrumbs auto-update based on the current route and entity name
7. URL state: filters and pagination stored in URL search params (?page=2&status=not_implemented)
8. ZERO mock data — if the API returns an empty array, show the empty state component

OUTPUT: Complete, compilable TypeScript/React code for every page listed. Include proper error boundaries, Suspense boundaries, and loading.tsx files for each route segment.
```

---

### PROMPT 8 OF 100 — Risk, Policy, Audit & Incident Management Pages

```
You are a senior React/TypeScript developer continuing the "ComplianceForge" Next.js 14 frontend. The core architecture from Prompt 6 and the dashboard/framework pages from Prompt 7 are built.

OBJECTIVE:
Build the Risk Register, Policy Management, Audit Management, and Incident Management pages — all four core GRC modules. Every page fetches real data from the Golang backend API. Full CRUD (Create, Read, Update) operations with forms that submit to the real API. NO mock data.

BACKEND API ENDPOINTS AVAILABLE:
Risks:
  - GET /risks?page=&page_size=&sort_by=&sort_dir=&status= → PaginatedResponse<Risk>
  - GET /risks/{id} → Risk with treatments and KRIs
  - POST /risks → CreateRiskRequest
  - GET /risks/heatmap → RiskHeatmapEntry[]

Policies:
  - GET /policies?page=&page_size= → PaginatedResponse<Policy>
  - GET /policies/{id} → Policy with versions
  - POST /policies → CreatePolicyRequest (creates policy + first version)
  - POST /policies/{id}/publish → PublishPolicy
  - POST /policies/{id}/attest → SubmitAttestation
  - GET /policies/attestations/stats → AttestationStats

Audits:
  - GET /audits?page=&page_size= → PaginatedResponse<Audit>
  - GET /audits/{id} → Audit with findings
  - POST /audits → CreateAuditRequest
  - POST /audits/{id}/findings → CreateFindingRequest
  - GET /audits/{id}/findings → AuditFinding[]
  - GET /audits/findings/stats → FindingsStats

Incidents:
  - GET /incidents?page=&page_size= → PaginatedResponse<Incident>
  - GET /incidents/{id} → Incident with breach notification status
  - POST /incidents → ReportIncidentRequest
  - POST /incidents/{id}/notify-dpa → RecordDPANotification
  - POST /incidents/{id}/nis2-early-warning → RecordNIS2EarlyWarning
  - GET /incidents/stats → IncidentStats
  - GET /incidents/breaches/urgent → Urgent breaches nearing 72h GDPR deadline

BUILD THE FOLLOWING PAGES:

PAGE 1 — Risk Register (src/app/(dashboard)/risks/page.tsx):
- List View:
  * Fetch risks with useRisks(page, sortBy, sortDir, statusFilter)
  * DataTable columns: Ref (RSK-0001), Title (link to detail), Category, Residual Score (bold, color-coded), Level (badge), Owner, Status (badge), Treatments (count)
  * Filters: risk level dropdown (critical/high/medium/low/all), status dropdown, search bar
  * Sort by: residual_risk_score (default), created_at, title
  * "Register Risk" button → opens slide-over create form
  
- Risk Heatmap Tab:
  * Fetch GET /risks/heatmap
  * Interactive 5×5 grid: X-axis = Likelihood (1–5), Y-axis = Impact (1–5)
  * Each cell shows count of risks at that coordinate
  * Cell color: green (1–3), yellow (4–8), orange (9–14), red (15–25)
  * Click a cell to see the risks at that coordinate in a popover
  * Toggle between inherent and residual heatmap

- Create Risk Form (slide-over panel):
  * Fields: title (required, min 5 chars), description (textarea), risk_category_id (select from API), risk_source (select: internal/external/third_party/regulatory), owner_user_id (select users from API), inherent_likelihood (1–5 slider), inherent_impact (1–5 slider), residual_likelihood (1–5 slider), residual_impact (1–5 slider), financial_impact_eur (currency input), risk_velocity (select), review_frequency (select), tags (multi-select/combobox)
  * Auto-calculated score display: likelihood × impact with color-coded result
  * Submit: POST /risks → on success: close panel, invalidate risks query, show toast
  * Validation: Zod schema, show inline field errors

PAGE 2 — Risk Detail (src/app/(dashboard)/risks/[id]/page.tsx):
- Fetch GET /risks/{id}
- Header: risk_ref, title, status badge, residual_risk_level badge, residual_risk_score
- Summary cards: Inherent Score, Residual Score, Financial Impact (€), Review Date, Owner
- Tabs: "Overview", "Treatments", "Controls", "History"
- Overview tab: description, risk source, velocity, category, full inherent/residual matrix display
- Treatments tab: list of risk_treatments with status, responsible person, target date, cost
- Controls tab: linked controls from risk_control_mappings
- "Edit Risk" button opens the same form as create, pre-populated

PAGE 3 — Policy Management (src/app/(dashboard)/policies/page.tsx):
- List View:
  * Fetch policies with pagination
  * DataTable columns: Ref, Title (with classification badge inline), Status (color badge), Version, Review Status (with overdue highlighting in red), Next Review Date, Attestation Rate (progress bar + percentage), Actions
  * Summary cards row: Published count, Draft count, Under Review count, Reviews Overdue (red), Average Attestation Rate
  * "Draft Policy" button → opens create form

- Create Policy Form (full page or slide-over):
  * Fields: title (required), category_id (select), classification (select: public/internal/confidential/restricted), content_html (rich text editor or textarea with markdown preview), summary (textarea), owner_user_id (select users), approver_user_id (select users), review_frequency_months (number, 1–36), is_mandatory (checkbox), requires_attestation (checkbox), tags
  * Submit: POST /policies → on success: navigate to /policies/{id}

PAGE 4 — Policy Detail (src/app/(dashboard)/policies/[id]/page.tsx):
- Header: policy_ref, title, status badge, classification badge
- Action buttons based on status:
  * Draft → "Submit for Review"
  * Approved → "Publish" (calls POST /policies/{id}/publish)
  * Published → "Acknowledge" (calls POST /policies/{id}/attest)
- Tabs: "Content", "Versions", "Attestations", "Exceptions"
- Content tab: rendered HTML content of the current version
- Versions tab: version history with diff highlights
- Attestations tab: list of users who have/haven't attested, completion percentage

PAGE 5 — Audit Management (src/app/(dashboard)/audits/page.tsx):
- List View with DataTable: Ref, Title, Type (badge), Status (badge), Lead Auditor, Start Date, End Date, Findings (total/critical/high)
- Summary cards: Planned, In Progress, Completed, Total Findings, Critical Findings Open
- "Plan Audit" button → create form

PAGE 6 — Audit Detail (src/app/(dashboard)/audits/[id]/page.tsx):
- Fetch GET /audits/{id}
- Header: audit_ref, title, type badge, status badge
- Findings section:
  * Fetch GET /audits/{id}/findings
  * DataTable: Ref, Title, Severity (badge), Status (badge), Type, Responsible, Due Date (red if overdue)
  * "Add Finding" button → modal form → POST /audits/{id}/findings
  * Finding form: finding_ref, title, description, severity (select), finding_type (select), control_id (optional, search controls), root_cause, recommendation, responsible_user_id, due_date

PAGE 7 — Incident Management (src/app/(dashboard)/incidents/page.tsx):
- CRITICAL: This page has GDPR/NIS2 regulatory implications
- GDPR Breach Alert Section (always at top, fetched from GET /incidents/breaches/urgent):
  * If any data breaches are approaching 72-hour deadline: RED alert box
  * Each breach: ref, title, hours remaining (countdown), data subjects count, "Notify DPA" button
  * Auto-refreshes every 60 seconds
- Summary row: Open, Investigating, Contained, Resolved, Data Breaches Total, NIS2 Reportable
- DataTable: Ref, Title, Type (badge), Severity (badge), Status (badge), Breach? (icon + subject count), Reported Date, Actions
- Rows with is_data_breach=true and dpa_notified_at=null highlighted in red
- "Report Incident" button → create form

PAGE 8 — Incident Detail (src/app/(dashboard)/incidents/[id]/page.tsx):
- Fetch GET /incidents/{id}
- Header: incident_ref, title, severity badge, status badge
- If data breach:
  * Prominent breach notification panel:
    - Notification deadline (datetime, with countdown)
    - Hours remaining (updated live with setInterval)
    - DPA notification status: "Not notified" (red) or "Notified at [datetime]" (green)
    - "Record DPA Notification" button → POST /incidents/{id}/notify-dpa
    - Data subjects affected count
    - Data categories list
  * If NIS2 reportable:
    - NIS2 section: Early Warning status, "Submit Early Warning" button → POST /incidents/{id}/nis2-early-warning
    - Deadlines: 24h early warning, 72h notification, 1 month final report
- Timeline section: incident lifecycle events
- "Resolve Incident" and "Close Incident" action buttons

CRITICAL REQUIREMENTS:
1. ALL data from the real API — ZERO mock data, ZERO hardcoded arrays
2. All forms use Zod for validation, react-hook-form for state management
3. All mutations show loading spinner on submit button, disable form during submit
4. Success: toast + invalidate relevant queries + close modal/panel
5. Error: toast with error message from API, keep form open with values intact
6. All lists paginated server-side (API handles pagination, not client)
7. URL search params for all filters/pagination: ?page=2&status=critical&sort_by=residual_risk_score
8. Responsive: all pages work on desktop, tablet, mobile
9. Keyboard accessibility: all interactive elements focusable, forms submittable with Enter
10. Optimistic updates where appropriate (e.g., changing incident status)

OUTPUT: Complete TypeScript/React code for ALL 8 pages listed, plus any loading.tsx, error.tsx, and not-found.tsx files needed. Include the Zod validation schemas for every form.
```

---

### PROMPT 9 OF 100 — Vendor, Asset, Settings & Control Implementation Pages

```
You are a senior React/TypeScript developer completing the "ComplianceForge" Next.js 14 frontend. The core architecture (Prompt 6), dashboard/framework pages (Prompt 7), and risk/policy/audit/incident pages (Prompt 8) are built.

OBJECTIVE:
Build the remaining pages: Vendor Management, Asset Inventory, Organisation Settings (with User Management, Role Management, and Audit Log), and the Control Implementation detail page with evidence upload. ALL data from real API — NO mock data.

BACKEND API ENDPOINTS AVAILABLE:
Vendors:
  - GET /vendors?page=&page_size= → PaginatedResponse<Vendor>
  - GET /vendors/{id} → Vendor with compliance warnings
  - POST /vendors → OnboardVendorRequest (returns vendor + GDPR DPA requirements if data_processing=true)
  - GET /vendors/stats → VendorStats

Assets:
  - GET /assets?page=&page_size= → PaginatedResponse<Asset>
  - GET /assets/{id} → Asset detail
  - POST /assets → RegisterAssetRequest (returns asset + GDPR ROPA notice if processes_personal_data=true)
  - GET /assets/stats → AssetStats

Settings:
  - GET /settings → Organisation details
  - PUT /settings → UpdateOrganisationRequest
  - GET /settings/users?page=&search= → PaginatedResponse<User>
  - POST /settings/users → CreateUserRequest
  - GET /settings/users/{id} → User with roles
  - PUT /settings/users/{id} → UpdateUserRequest
  - DELETE /settings/users/{id} → DeactivateUser
  - POST /settings/users/{id}/roles → AssignRoleRequest
  - GET /settings/roles → Role[]
  - GET /settings/audit-log?page= → PaginatedResponse<AuditLog>

Reports:
  - GET /reports/compliance → ComplianceReport
  - GET /reports/risk → RiskReport

Controls (from Prompt 7 context):
  - GET /controls/{id} → ControlImplementation with evidence
  - PUT /controls/{id} → UpdateControlImplementationRequest
  - POST /controls/{id}/evidence → Multipart file upload
  - GET /controls/{id}/evidence → ControlEvidence[]
  - GET /evidence/{id}/download → File stream
  - PUT /evidence/{id}/review → ReviewEvidenceRequest
  - POST /controls/{id}/test → RecordTestResultRequest

BUILD THE FOLLOWING PAGES:

PAGE 1 — Vendor Management (src/app/(dashboard)/vendors/page.tsx):
- GDPR DPA Compliance Alert Banner:
  * Fetch vendors and filter where data_processing=true AND dpa_in_place=false
  * Red alert listing all vendors missing a Data Processing Agreement
  * "GDPR Article 28 requires a DPA before sharing personal data with processors."
- Summary cards: Total Vendors, Critical Risk, High Risk, Missing DPA (red), Total Contract Value (€)
- DataTable: Name (with ref), Country (flag emoji), Risk Tier (badge), Risk Score (color-coded number), Data Processing (Yes/No), DPA Status (✓ In Place / ✗ Missing, color-coded), Certifications (badge list), Next Assessment Date, Actions
- Rows with missing DPA: red background highlight
- "Onboard Vendor" button → modal/slide-over form:
  * name, legal_name, website, industry, country_code (select with all EU/UK countries), contact_name, contact_email, risk_tier (select), service_description, data_processing (toggle — when ON, show data_categories multi-select), certifications (multi-select: ISO27001, SOC2, PCI DSS, etc.), assessment_frequency (select), owner_user_id
  * If data_processing is toggled ON: show a warning box: "A Data Processing Agreement (DPA) will be required per GDPR Article 28"
  * Submit: POST /vendors → show GDPR requirements in the success response

PAGE 2 — Vendor Detail (src/app/(dashboard)/vendors/[id]/page.tsx):
- Fetch GET /vendors/{id}
- Header: vendor name, risk tier badge, status badge, country
- If data_processing=true AND dpa_in_place=false: red DPA warning banner
- Sections: Overview, Risk Assessment, Compliance Certifications, Data Processing Details, Contract Information
- "Start Assessment" button, "Edit Vendor" button

PAGE 3 — Asset Inventory (src/app/(dashboard)/assets/page.tsx):
- Summary cards from GET /assets/stats: Total Assets, Critical Assets, Personal Data Assets, By Type distribution
- DataTable: Ref (AST-0001), Name, Type (badge), Category, Criticality (badge), Classification (badge), Personal Data (⚠ flag if true), Owner, Location, Actions
- "Register Asset" button → form:
  * name, asset_type (select: hardware, software, data, service, network, people, facility), category, description, criticality (select: critical/high/medium/low), owner_user_id, location, ip_address, classification (select: public/internal/confidential/restricted), processes_personal_data (toggle), linked_vendor_id (optional vendor search), tags
  * If processes_personal_data toggled ON: show GDPR notice: "This asset will be flagged for inclusion in your ROPA (Record of Processing Activities) per GDPR Article 30."

PAGE 4 — Settings: Organisation Tab (src/app/(dashboard)/settings/page.tsx):
- Tabbed interface: Organisation | Users | Roles | Audit Log
- Organisation tab:
  * Fetch GET /settings
  * Display: Name, Legal Name, Industry, Country, Timezone, Subscription Tier, Data Residency Region, Frameworks Adopted (count/max), Users (count/max)
  * "Edit Settings" button → inline edit form → PUT /settings

PAGE 5 — Settings: Users Tab:
- Search bar with debounced input → GET /settings/users?search=
- DataTable: User (avatar + name + email), Role (badge), Department, Status (badge), Last Login (relative time), Actions (Edit, Deactivate)
- "Add User" button → modal form: email, password (min 12 chars), first_name, last_name, job_title, department, role_ids (multi-select from GET /settings/roles)
- Submit: POST /settings/users → on success: invalidate users query, close modal, show toast
- Deactivate: confirmation dialog → DELETE /settings/users/{id}
- Prevent deactivating yourself: grey out the deactivate button on your own row

PAGE 6 — Settings: Roles Tab:
- Fetch GET /settings/roles
- Cards layout: each role shows name, description, system/custom badge, user count
- "View Permissions" button per role → modal/accordion showing permission matrix

PAGE 7 — Settings: Audit Log Tab:
- Fetch GET /settings/audit-log with pagination
- DataTable: Timestamp (formatted datetime), User, Action (color-coded badge: create=green, update=amber, delete=red, login=blue), Entity (with link), Details, IP Address
- Date range filter
- Action type filter dropdown
- "Immutable audit trail — ISO 27001 A.8.15 compliant" subtitle

PAGE 8 — Reports Page (src/app/(dashboard)/reports/page.tsx):
- Two report cards: Compliance Report, Risk Report
- "Generate Compliance Report" → fetches GET /reports/compliance → displays:
  * Overall score, framework breakdown table, maturity distribution chart, top 10 gaps, controls by status
  * "Export as PDF" button (client-side generation using the report data)
- "Generate Risk Report" → fetches GET /reports/risk → displays:
  * Risk distribution chart, top 10 risks table, treatment progress, average residual score
  * "Export as PDF" button

ADDITIONAL REQUIREMENTS:
1. ALL data from the real API — absolutely ZERO mock/hardcoded data
2. All forms validated with Zod + react-hook-form
3. Evidence upload uses native FormData with progress indicator
4. File download: clicking evidence opens download via GET /evidence/{id}/download
5. GDPR compliance warnings are prominently displayed (red banners, not subtle text)
6. All tables support server-side pagination and sorting via URL params
7. Toast notifications for all create/update/delete operations
8. Confirmation dialogs for destructive actions (deactivate user, deactivate vendor)
9. Responsive design: all pages work on desktop, tablet, mobile

OUTPUT: Complete TypeScript/React code for all 8 pages plus supporting components (modals, forms, detail panels). Include Zod schemas for every form.
```

---

### PROMPT 10 OF 100 — CI/CD Pipeline, E2E Testing, Production Docker & Deployment

```
You are a senior DevOps/Platform engineer setting up the complete CI/CD pipeline, E2E testing, and production deployment for "ComplianceForge" — a full-stack GRC platform with a Golang backend and Next.js frontend.

OBJECTIVE:
Build the complete continuous integration, E2E testing suite, production Docker images, and deployment pipeline. This must be production-grade, supporting automated testing, security scanning, and zero-downtime deployments.

TECH STACK:
- CI/CD: GitHub Actions
- Container: Docker (multi-stage builds)
- Orchestration: Kubernetes (manifests already exist in deployments/k8s/)
- E2E Testing: Playwright
- Backend Testing: Go test + testcontainers-go (real PostgreSQL + Redis in CI)
- Security: Trivy container scanning, gosec for Go code, npm audit for frontend
- Linting: golangci-lint (backend), eslint + prettier (frontend)

CREATE THE FOLLOWING FILES:

FILE 1 — GitHub Actions CI Pipeline (.github/workflows/ci.yml):
```yaml
Triggers: push to main/develop, pull requests to main/develop

Jobs:

1. backend-lint:
   - golangci-lint with config (.golangci.yml)
   - gosec security scanner
   - go vet

2. backend-test:
   - Start PostgreSQL 16 service container
   - Start Redis 7 service container
   - Run migrations on test database
   - Seed test data
   - Run go test ./... -race -coverprofile=coverage.out -covermode=atomic
   - Upload coverage to Codecov
   - Minimum coverage threshold: 60%

3. backend-build:
   - go build -o /bin/api ./cmd/api
   - go build -o /bin/worker ./cmd/worker
   - go build -o /bin/migrate ./cmd/migrate
   - Verify binaries exist and are executable

4. frontend-lint:
   - npm ci
   - eslint + prettier check
   - TypeScript type check (tsc --noEmit)

5. frontend-test:
   - npm ci
   - vitest run (unit tests)
   - Coverage report

6. frontend-build:
   - npm run build
   - Verify .next/standalone output exists

7. e2e-tests (depends on: backend-build, frontend-build):
   - Start backend services (PostgreSQL, Redis, API server, worker)
   - Start frontend (next start)
   - Run Playwright tests
   - Upload test artifacts (screenshots, traces) on failure
   - Upload Playwright HTML report

8. security-scan:
   - Trivy scan on backend Docker image
   - Trivy scan on frontend Docker image
   - npm audit --production
   - Fail on critical/high vulnerabilities

9. docker-build (depends on all previous):
   - Build backend Docker image
   - Build frontend Docker image
   - Tag with git SHA and 'latest'
   - Push to container registry (GitHub Container Registry)
   - Only on push to main branch
```

FILE 2 — Backend Dockerfile (deployments/docker/Dockerfile.api):
```
Multi-stage build:
Stage 1 (Builder):
  - golang:1.22-alpine
  - Install build dependencies
  - Copy go.mod, go.sum → go mod download (layer caching)
  - Copy source → go build with CGO_ENABLED=0, GOOS=linux
  - Build api, worker, migrate binaries
  - Include sql/migrations/ and sql/seeds/ in output

Stage 2 (Runtime):
  - gcr.io/distroless/static-debian12 OR alpine:3.19 (minimal)
  - Non-root user (UID 1000)
  - Copy binaries from builder
  - Copy migrations and seeds
  - Expose port 8080
  - Health check: wget or curl to /api/v1/health
  - Entry point: /app/api
```

FILE 3 — Frontend Dockerfile (deployments/docker/Dockerfile.frontend):
```
Multi-stage build:
Stage 1 (Dependencies):
  - node:20-alpine
  - Copy package.json, package-lock.json → npm ci (layer caching)

Stage 2 (Builder):
  - Copy source
  - ARG NEXT_PUBLIC_API_URL
  - npm run build
  - Output: .next/standalone

Stage 3 (Runtime):
  - node:20-alpine (slim)
  - Non-root user
  - Copy standalone output
  - Copy public/ and .next/static
  - Expose port 3000
  - Entry point: node server.js
```

FILE 4 — Docker Compose Production (deployments/docker/docker-compose.prod.yml):
```yaml
Services:
  api:
    - Build from Dockerfile.api
    - Env from .env
    - Depends on: postgres, redis
    - Health check: /api/v1/health
    - Restart: unless-stopped
    - Replicas: 2 (deploy mode)

  worker:
    - Same image as api, different command: /app/worker
    - Depends on: postgres, redis
    - Restart: unless-stopped

  frontend:
    - Build from Dockerfile.frontend
    - NEXT_PUBLIC_API_URL=http://api:8080/api/v1
    - Depends on: api
    - Port: 3000

  postgres:
    - postgres:16-alpine
    - Persistent volume
    - initdb scripts for extensions
    - Backup cron

  redis:
    - redis:7-alpine
    - Persistent volume (AOF)
    - maxmemory-policy: allkeys-lru

  nginx:
    - nginx:alpine as reverse proxy
    - TLS termination
    - Rate limiting
    - Security headers (HSTS, CSP, X-Frame-Options)
    - Proxy /api → api:8080, / → frontend:3000
```

FILE 5 — Playwright E2E Test Suite (frontend/e2e/):
```
Tests covering:

1. auth.spec.ts:
   - Login with valid credentials → redirect to dashboard
   - Login with invalid credentials → show error
   - Logout → redirect to login
   - Unauthenticated access → redirect to login

2. dashboard.spec.ts:
   - Dashboard loads with KPI cards (verify numbers are present, not loading)
   - Framework compliance chart renders
   - Risk distribution chart renders
   - Quick action buttons navigate correctly
   - GDPR breach alert appears when breaches exist

3. frameworks.spec.ts:
   - Framework list loads with 9 cards
   - Click framework → navigates to detail
   - Framework detail shows controls table
   - Control search works
   - Implementation tab shows status breakdown

4. risks.spec.ts:
   - Risk register loads with data
   - Create risk form → fill all fields → submit → new risk appears in list
   - Risk heatmap renders
   - Filter by risk level works
   - Sort by score works
   - Risk detail page loads

5. incidents.spec.ts:
   - Incident list loads
   - Report incident → fill form → submit → appears in list
   - Data breach incident shows notification deadline
   - "Notify DPA" button works

6. policies.spec.ts:
   - Policy list loads
   - Draft policy → fill form → submit → appears as draft
   - Publish workflow

7. settings.spec.ts:
   - Organisation details load
   - Users tab shows user list
   - Create user → appears in list
   - Audit log shows entries

Configuration:
  - playwright.config.ts with:
    * baseURL from env or localhost:3000
    * Parallel test execution
    * Screenshot on failure
    * Trace on first retry
    * HTML reporter
    * 30 second timeout
    * webServer config to start backend + frontend
```

FILE 6 — golangci-lint Configuration (.golangci.yml):
```yaml
Linters enabled:
  - errcheck, govet, staticcheck, unused
  - gosec (security)
  - gocyclo (complexity max 15)
  - goconst (detect repeated strings)
  - misspell
  - nilerr, noctx
  - prealloc
  - revive
  - unconvert

Exclude:
  - test files from some linters
  - generated code (sqlc, proto)
```

FILE 7 — ESLint & Prettier Config (frontend/.eslintrc.js, frontend/.prettierrc):
```
ESLint:
  - extends: next/core-web-vitals, typescript-eslint/recommended
  - Rules: no-unused-vars (error), no-explicit-any (warn), prefer-const (error)
  - Import ordering

Prettier:
  - Semi: true, single quotes, trailing commas, 100 print width
  - Tailwind CSS plugin for class sorting
```

FILE 8 — Makefile Updates (root Makefile):
```
Add targets:
  - test-integration: docker compose up test DB, run go test with build tag integration
  - test-e2e: start services, run playwright
  - lint-all: golangci-lint + eslint + prettier
  - docker-build-all: build both images
  - docker-push: tag and push to registry
  - security-scan: trivy + gosec + npm audit
  - coverage-report: generate and open HTML coverage report
```

REQUIREMENTS:
1. CI pipeline must complete in under 15 minutes
2. All test databases use tmpfs for speed (RAM-backed storage)
3. Docker images must be under 50MB (backend) and 150MB (frontend)
4. Security scan blocks deployment on critical/high CVEs
5. E2E tests run against a real backend with seeded data (not mock API)
6. Coverage reports uploaded as artifacts
7. Branch protection: require CI pass before merge to main
8. Deploy to staging on push to develop, production on push to main

OUTPUT: Complete, copy-pasteable content for every file listed. Include all YAML, Dockerfiles, test files, and configuration. Every GitHub Actions workflow must be valid YAML.
```

---

## BATCH 2 SUMMARY

| Prompt | Focus Area | Pages/Files Created | Key Capabilities |
|--------|-----------|---------------------|------------------|
| 6 | Frontend Architecture, Auth, Layout, API Client | ~25 files (components, lib, types, layout) | Auth flow, API client, React Query hooks, DataTable, Chart components, Sidebar, Design system |
| 7 | Dashboard, Framework, Compliance Pages | 4 pages + sub-routes | Executive dashboard, Framework list/detail, Control implementation, Gap analysis, Cross-mapping |
| 8 | Risk, Policy, Audit, Incident Pages | 8 pages + forms | Full CRUD for all 4 core GRC modules, GDPR breach alerts, Zod validation, Server-side pagination |
| 9 | Vendor, Asset, Settings, Reports Pages | 8 pages + forms | Vendor DPA tracking, Asset ROPA flags, User management, Role management, Audit log, Report generation |
| 10 | CI/CD, E2E Testing, Docker, Deployment | ~15 files (workflows, Dockerfiles, tests, config) | GitHub Actions, Playwright E2E, Multi-stage Docker, Security scanning, Production deployment |

**Running Total: 10/100 Prompts | Full-Stack Platform | ~35 Pages | 60 API Routes | 593 Controls | 121 Cross-Mappings**

---

> **NEXT BATCH (Prompts 11–15):** Notification System (email + in-app + webhooks), Advanced Reporting Engine (PDF/XLSX generation), GDPR Data Subject Request Module, NIS2 Compliance Automation, and Continuous Monitoring / Evidence Collection Scheduler.
>
> Type **"next"** to continue.
