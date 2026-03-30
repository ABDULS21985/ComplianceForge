# ComplianceForge: A Comprehensive Governance, Risk & Compliance Platform for the Modern Enterprise

## Executive Summary

ComplianceForge is a production-grade, multi-tenant Governance, Risk, and Compliance (GRC) platform purpose-built for European enterprises navigating complex regulatory landscapes. It delivers end-to-end coverage across compliance management, risk assessment, policy governance, audit tracking, incident response, vendor oversight, and regulatory monitoring — all from a single, unified platform.

Built on a modern technology stack (Go, Next.js 14, PostgreSQL, Redis, RabbitMQ), ComplianceForge provides real-time compliance scoring, AI-powered remediation, multi-framework support, and a workflow engine that automates approvals and escalations across the entire compliance lifecycle.

---

## Table of Contents

1. [Platform Architecture](#1-platform-architecture)
2. [Multi-Framework Compliance Management](#2-multi-framework-compliance-management)
3. [Risk Management](#3-risk-management)
4. [Policy Lifecycle Management](#4-policy-lifecycle-management)
5. [Audit & Evidence Management](#5-audit--evidence-management)
6. [Incident Management & Breach Response](#6-incident-management--breach-response)
7. [Vendor Risk Management](#7-vendor-risk-management)
8. [Asset Management](#8-asset-management)
9. [GDPR Data Subject Requests (DSR)](#9-gdpr-data-subject-requests-dsr)
10. [NIS2 Directive Compliance](#10-nis2-directive-compliance)
11. [Continuous Monitoring & Drift Detection](#11-continuous-monitoring--drift-detection)
12. [Notification Engine](#12-notification-engine)
13. [Workflow & Approval Engine](#13-workflow--approval-engine)
14. [AI-Powered Remediation Planning](#14-ai-powered-remediation-planning)
15. [Business Impact Analysis & Continuity](#15-business-impact-analysis--continuity)
16. [Regulatory Change Management](#16-regulatory-change-management)
17. [Advanced Analytics & Reporting](#17-advanced-analytics--reporting)
18. [Marketplace](#18-marketplace)
19. [Authentication, Authorization & Security](#19-authentication-authorization--security)
20. [Integration Hub](#20-integration-hub)
21. [Guided Onboarding](#21-guided-onboarding)
22. [Deployment & Infrastructure](#22-deployment--infrastructure)
23. [Testing & Quality Assurance](#23-testing--quality-assurance)

---

## 1. Platform Architecture

### Technology Stack

| Layer | Technology |
|-------|-----------|
| **Backend API** | Go 1.24, Chi HTTP Router |
| **Frontend** | Next.js 14.2, React 18, TypeScript |
| **Database** | PostgreSQL 16 with Row-Level Security |
| **Cache** | Redis |
| **Message Queue** | RabbitMQ (AMQP 0-9-1) |
| **UI Components** | Radix UI, Tailwind CSS 3.4, Recharts |
| **State Management** | Zustand + TanStack React Query |
| **Form Handling** | React Hook Form + Zod Validation |
| **API Protocols** | REST (primary), gRPC (service-to-service) |
| **Authentication** | JWT + OAuth 2.0 / OIDC |

### Architectural Principles

- **Multi-Tenancy**: Complete data isolation enforced at the database level through PostgreSQL Row-Level Security (RLS) policies. Every query is automatically scoped to the requesting organization.
- **Layered Architecture**: Clean separation across handlers, services, repositories, and models following Go best practices.
- **Event-Driven Processing**: An in-process event bus with wildcard subscriptions routes domain events (e.g., `incident.created`, `control.updated`) to notification rules, analytics collectors, and workflow triggers.
- **Asynchronous Workers**: RabbitMQ-backed background job processors handle report generation, notification dispatch, compliance scoring, regulatory scanning, and analytics aggregation.
- **Graceful Lifecycle**: The API server supports graceful shutdown with a 30-second drain period, ensuring in-flight requests complete before termination.

### Backend Service Catalog

The backend comprises **24 HTTP handlers** and **42+ services** organized into domain-specific modules:

- **Core GRC**: Compliance engine, risk service, policy service, audit service, control service
- **Regulatory**: NIS2 service, DSR service, regulatory scanner
- **Automation**: Notification engine, workflow engine, event bus, drift detector
- **Intelligence**: AI service, analytics engine, remediation planner
- **Platform**: Auth service, ABAC engine, integration service, marketplace service, onboarding service

### Frontend Application

The frontend delivers **21 primary page groups** with detail views, organized under a responsive dashboard layout:

- Executive Dashboard, Frameworks, Risk Register, Policies, Audits, Incidents, Vendors, Assets, Controls, DSR, NIS2, Monitoring, Remediation, Marketplace, Regulatory, BIA, Analytics, Workflows, Settings, Onboarding Wizard, and Authentication pages.

---

## 2. Multi-Framework Compliance Management

### Supported Frameworks

ComplianceForge ships with native support for **9 major compliance frameworks**, with the ability to create and import custom frameworks:

| Framework | Category |
|-----------|----------|
| **ISO 27001** | Information Security Management |
| **UK GDPR** | Data Protection |
| **NCSC Cyber Assessment Framework (CAF)** | Cybersecurity |
| **Cyber Essentials / Cyber Essentials Plus** | Cybersecurity Baseline |
| **NIST SP 800-53** | Federal Information Security |
| **NIST Cybersecurity Framework 2.0** | Cybersecurity Risk Management |
| **PCI DSS** | Payment Card Industry |
| **ITIL 4** | IT Service Management |
| **COBIT 2019** | IT Governance |

### Key Capabilities

- **Framework Adoption**: Organizations select which frameworks apply to their business. The platform tracks adoption status and presents a grid view of framework cards showing name, version, issuing body, category, and total control counts.
- **Hierarchical Control Structure**: Frameworks are organized into domains (categories) containing individual controls with evidence requirements, implementation guidance, and maturity indicators.
- **Cross-Framework Mapping**: Controls are mapped across frameworks with confidence scoring. Implementing a control for ISO 27001 automatically shows coverage for related NIST or PCI DSS requirements, eliminating duplicate effort.
- **Compliance Scoring**: The compliance engine calculates real-time scores (0-100%) per framework based on control implementation status aggregation. Scores feed into dashboards, trend analytics, and benchmark comparisons.
- **Gap Analysis**: The platform identifies unimplemented or partially implemented controls, highlights gaps, and feeds them into the AI-powered remediation planner.
- **Full-Text Search**: PostgreSQL full-text search enables rapid discovery across the control library.

---

## 3. Risk Management

### Risk Register

The risk register provides a centralized repository for all organizational risks with rich metadata:

- **Risk Attributes**: Title, description, category, source, owner, tags, review frequency
- **Triple Scoring Model**: Each risk carries inherent, residual, and target risk scores across likelihood (1-5) and impact (1-5) dimensions
- **Auto-Calculated Risk Levels**: Scores automatically map to levels — Critical (20+), High (15-19), Medium (9-14), Low (4-8), Very Low (<4)
- **Financial Impact**: EUR-denominated financial impact estimation
- **Risk Velocity**: Categorized as immediate, fast, moderate, or slow
- **Treatment Tracking**: Mitigation status progression (Open, In Progress, Mitigated, Accepted, Transferred)

### Risk Visualization

- **5x5 Risk Heatmap**: An interactive matrix plotting likelihood against impact, with color-coded cells (red for 5+ risks, orange for 3+, yellow for 1+). Users can toggle between inherent and residual views.
- **Risk Distribution Donut Chart**: Dashboard-level pie chart showing risk counts by level (critical, high, medium, low, very low).
- **Trend Analysis**: Line charts tracking risk counts by level over configurable periods (3, 6, or 12 months).

### Risk Appetite & Tolerance

- **Risk Appetite Statements**: Defined per risk category (Cybersecurity, Operational, Compliance, Financial, Reputational, Strategic)
- **Tolerance Levels**: Very Low through Very High
- **Configurable Risk Matrices**: Support for 3x3, 4x4, and 5x5 matrix sizes to match organizational maturity

### Predictive Risk Analytics

The analytics engine generates risk predictions with confidence scores, enabling proactive risk management rather than reactive reporting.

---

## 4. Policy Lifecycle Management

### Full Policy Lifecycle

ComplianceForge manages policies through a complete lifecycle with version control:

**Draft** → **Under Review** → **Approved** → **Published** → **Archived** → **Retired**

### Policy Attributes

- **Metadata**: Title, category, classification (Public, Internal, Confidential, Restricted), summary
- **Content**: Rich HTML content editor for policy documents
- **Ownership**: Owner and approver assignment
- **Review Scheduling**: Configurable review frequency (in months) with automatic overdue tracking
- **Compliance Mapping**: Policies linked to controls and frameworks
- **Tags**: Comma-separated tags for categorization

### Governance Features

- **Version Control**: Full version history with audit trail for every policy change
- **Approval Workflows**: Multi-step approval processes through the workflow engine
- **User Attestations**: Mandatory attestation tracking when policies require user acknowledgment, with attestation rate percentage displayed
- **Exception Management**: Formal exception request process for policy deviations
- **Classification Badges**: Color-coded classification levels for visual identification

### Dashboard Metrics

- Published, Draft, and Under Review counts
- Overdue reviews count
- Average attestation rate (%)
- Review status indicators (On Track / Overdue)

---

## 5. Audit & Evidence Management

### Audit Engagement Management

ComplianceForge supports three audit types — **Internal**, **External**, and **Certification** — each progressing through four statuses: Planned, In Progress, Completed, and Closed.

### Audit Configuration

- **Audit Details**: Title, description, type, scope definition
- **Personnel**: Lead auditor selection
- **Scheduling**: Start and end date pickers
- **Framework Linkage**: Optional association with specific compliance frameworks
- **Findings Management**: Finding tracking with 5 status levels, remediation plan association, and due dates

### Evidence Management

Evidence is managed at the control level with comprehensive metadata:

- **Evidence Types**: Documents, screenshots, logs, interviews — each with color-coded badges
- **Collection Methods**: Manual upload, automated collection, or integration-sourced
- **Validity Tracking**: Collection date, validity period (from/until), ensuring evidence remains current
- **Review Workflow**: Review status tracking with reviewer notes
- **Automated Collection Rules**: Configurable rules for automatic evidence gathering via the continuous monitoring engine

### Dashboard Metrics

- Planned, In Progress, and Completed audit counts
- Total findings with critical findings highlighted
- Finding remediation progress

---

## 6. Incident Management & Breach Response

### Incident Reporting

- **Incident Types**: Configurable incident categories
- **Severity Levels**: Critical, High, Medium, Low
- **Data Breach Detection**: Explicit data breach checkbox that triggers regulatory workflows
- **GDPR Data Categories**: Names, Email, Phone, Financial, Health, Biometric, Location, National ID, Credentials
- **Affected Data Subjects**: Count tracking for breach impact assessment

### Incident Lifecycle

**Open** → **Investigating** → **Contained** → **Resolved** → **Closed**

- Root cause analysis documentation
- Lessons learned capture
- Timeline tracking with chronological progression

### GDPR Article 33 Compliance

When an incident is flagged as a data breach:

- **72-Hour Breach Notification**: Automatic deadline tracking for DPA (Data Protection Authority) notification
- **DPA Notification**: One-click DPA notification trigger with email/phone contact details
- **Breach Alert Banner**: Dashboard-level urgent breach notification with countdown timer
- **Severity Escalation**: Critical severity incidents auto-flag as potentially breach-notifiable

### NIS2 Incident Reporting

For organizations subject to the NIS2 Directive, the incident module integrates with the NIS2 three-phase reporting workflow (Early Warning → Notification → Final Report).

### Dashboard Metrics

- Total incidents, by status breakdown
- Data breaches count
- NIS2 reportable incidents count
- Urgent breaches requiring immediate attention

---

## 7. Vendor Risk Management

### Vendor Onboarding

- **Company Details**: Vendor name, legal name, website, country (EU/UK country list)
- **Contact Management**: Contact name and email
- **Risk Classification**: Risk tier assignment (Critical, High, Medium, Low)
- **Service Scope**: Service description and type
- **Data Processing**: Explicit data processing flag with data category multi-select (Personal, Special Category, Financial, Health, Employee, Customer, Children's)
- **Certifications**: ISO 27001, SOC 2, PCI DSS, Cyber Essentials tracking

### GDPR Article 28 Compliance

- **DPA Status Tracking**: Data Processing Agreement status monitoring
- **Sub-Processor Registry**: Full sub-processor tracking as required by GDPR Article 28
- **Contract Lifecycle**: Contract tracking with value and expiry management

### Risk Assessment

- **Assessment Scheduling**: Quarterly for critical/high-risk vendors, 6-monthly for others
- **Risk Assessment Reports**: Structured assessments with recommendations
- **Vendor Scoring**: Risk-based scoring feeding into overall third-party risk posture

### Dashboard Metrics

- Total vendors, by risk tier
- Missing DPA agreements count
- Total contract value (EUR)
- Last assessment dates

---

## 8. Asset Management

### Asset Registry

ComplianceForge maintains a comprehensive asset inventory spanning seven asset types:

| Type | Description |
|------|-------------|
| **Hardware** | Physical infrastructure and devices |
| **Software** | Applications and systems |
| **Data** | Data stores and repositories |
| **Service** | Cloud and managed services |
| **Network** | Network infrastructure |
| **People** | Key personnel and teams |
| **Facility** | Physical locations |

### Asset Attributes

- **Classification**: Public, Internal, Confidential, Restricted — with color-coded badges
- **Criticality**: Critical, High, Medium, Low
- **Ownership**: Assigned owner and location
- **Privacy Impact**: Personal data processing flag
- **Vendor Linkage**: Association with third-party vendors
- **Tags**: Flexible categorization

### Dashboard Metrics

- Total assets and critical asset count
- Assets processing personal data
- Distribution by asset type

---

## 9. GDPR Data Subject Requests (DSR)

### Request Types

ComplianceForge supports all GDPR data subject rights:

- **Right of Access** (Article 15)
- **Right to Rectification** (Article 16)
- **Right to Erasure** (Article 17)
- **Right to Restriction** (Article 18)
- **Right to Data Portability** (Article 20)
- **Right to Object** (Article 21)

### Security & Privacy

- **AES-256-GCM Encryption**: All personally identifiable information (PII) is encrypted at rest using AES-256-GCM encryption with keys managed via environment configuration
- **Immutable Audit Trail**: Every action on a DSR creates an immutable audit record for regulatory accountability

### Workflow Management

**Submitted** → **Verified** → **Under Review** → **Complete** / **Rejected**

- **Identity Verification**: Dedicated verification step before processing
- **Task-Based Workflow**: System-by-system data discovery tasks with individual assignment
- **Assignee Management**: DSR assignment to team members
- **SLA Tracking**: 30-day statutory deadline with extension capability
- **Progress Tracking**: Percentage completion based on task status

### Dashboard Metrics

- Total requests, open, overdue
- Status breakdown
- Days remaining per request
- Deadline compliance indicators

---

## 10. NIS2 Directive Compliance

### Entity Assessment

- **Entity Classification**: Essential or Important entity determination
- **Scope Determination**: Employee count and sector-based NIS2 applicability assessment
- **Cybersecurity Measures**: Article 21 security measures implementation tracking with status (Verified, Implemented, In Progress, Not Started)

### Three-Phase Incident Reporting

ComplianceForge automates the NIS2 incident reporting timeline:

| Phase | Deadline | Description |
|-------|----------|-------------|
| **Early Warning** | 24 hours | Initial notification to CSIRT |
| **Full Notification** | 72 hours | Detailed incident report |
| **Final Report** | 1 month | Root cause and remediation |

Each phase includes deadline tracking, status indicators, and days-remaining counters.

### Management Accountability (Article 20)

- **Board Training Records**: Tracking management body cybersecurity training as required by Article 20
- **Exercise & Drill Scheduling**: Scheduled cybersecurity exercises with tracking

### Dashboard

NIS2-specific compliance dashboard showing measures status, incident reporting timelines, and management accountability records.

---

## 11. Continuous Monitoring & Drift Detection

### Automated Monitoring

- **Monitoring Configurations**: Define automated compliance checks with scheduling
- **Evidence Collection**: Automated evidence gathering based on configurable collection rules
- **Health Status**: Per-monitor health indicators (Healthy, Degraded, Critical)
- **Collection History**: Full history of monitoring executions with success/failure tracking

### Configuration Drift Detection

The drift detection engine identifies unauthorized or unexpected changes across the compliance posture:

- **Drift Types**: Configuration, Policy, Compliance, Evidence
- **Drift Status**: Active → Acknowledged → Resolved
- **Consecutive Failure Tracking**: Monitors track consecutive failures for escalation
- **Color-Coded Badges**: Visual identification of drift type and severity

### Metrics & Visualization

- Collection success rates
- Time-series trend visualization
- Failure reason analysis

---

## 12. Notification Engine

### Multi-Channel Delivery

ComplianceForge delivers notifications across four channels:

| Channel | Implementation |
|---------|---------------|
| **Email** | SMTP dispatch with HTML templates using Go `text/template` |
| **Slack** | Webhook integration with Slack Block Kit formatting |
| **Webhook** | Generic HTTP webhooks with HMAC-SHA256 signature verification |
| **In-App** | Real-time in-application notification feed |

### Event-Driven Rules

- **Event Types**: `incident.created`, `control.updated`, `policy.approved`, `risk.escalated`, `dsr.deadline`, and dozens more
- **Severity Filtering**: Rules can match on event severity levels
- **JSON Condition Evaluation**: Complex conditional logic using JSON-based rule definitions
- **Cooldown Throttling**: Configurable cooldown periods to prevent alert fatigue

### Recipient Resolution

- **Role-Based**: Route to users by role (e.g., all Compliance Officers)
- **Owner-Based**: Route to the entity owner
- **Specific Users**: Route to named individuals
- **Specialized Roles**: Route to DPO or CISO for regulatory events

### Regulatory Bypass

Certain event types — data breaches, GDPR deadlines, NIS2 reporting deadlines, DSR SLA warnings — bypass user notification preferences to ensure regulatory obligations are met.

### User Preferences

- Per-event-type email notification toggles
- Digest frequency settings
- Mute periods for noise reduction

---

## 13. Workflow & Approval Engine

### Workflow Definitions

Reusable workflow templates with the following step types:

| Step Type | Description |
|-----------|-------------|
| **Approval** | Requires one or all approvers to approve |
| **Review** | Review step without formal approval |
| **Conditional** | Branches to true/false paths based on conditions |
| **Parallel Gate** | Synchronization point for parallel steps |
| **Auto-Action** | Automated step execution (e.g., auto-approve low-risk changes) |
| **Timer** | Time-based delay or deadline step |
| **Notification** | Sends notifications as a workflow step |

### Approval Features

- **Approval Modes**: Any approver (first wins) or All approvers (unanimous)
- **Approve/Reject**: Actions with optional comments and reasons
- **Delegation**: Delegate approval to another user with full audit trail
- **Temporary Delegation Rules**: Date-range-based delegation for vacations or absences

### SLA Tracking & Escalation

- **Deadline Monitoring**: Each workflow step tracks SLA compliance
- **SLA Status Indicators**: OK (green), At Risk (amber), Overdue (red)
- **Automatic Escalation**: Background scheduler escalates overdue approvals
- **Time Remaining Display**: Real-time countdown for pending approvals

### Entity Binding

Workflows bind to specific entity types: policies, controls, incidents, audit findings, DSRs — ensuring the right approval process applies to the right domain.

### My Approvals Queue

Users see a consolidated view of all pending approvals with entity context, requestor information, and SLA status.

---

## 14. AI-Powered Remediation Planning

### Intelligent Gap Analysis

ComplianceForge integrates AI (OpenAI) to transform compliance gaps into actionable remediation plans:

1. **Framework Selection**: Choose target frameworks for remediation
2. **Gap Identification**: System identifies unimplemented or partially implemented controls
3. **Plan Generation**: AI generates prioritized remediation actions with effort estimates

### Remediation Plans

- **Plan Attributes**: Framework scope, target completion date, status (Draft, Active, Completed, Archived)
- **Progress Tracking**: Visual progress bars showing % completion (completed vs. total actions)

### Remediation Actions

- **Action Details**: Title, description, priority (Critical, High, Medium, Low)
- **Status Workflow**: Todo → In Progress → Review → Done
- **AI Guidance**: Each action includes AI-generated implementation suggestions
- **Effort Estimation**: Time and resource estimates per action
- **Control Reference**: Direct linkage to the control being addressed
- **Assignment & Deadlines**: Assignee and due date tracking

### Additional AI Capabilities

- **Control Implementation Guidance**: Contextual recommendations for implementing specific controls
- **Evidence Suggestions**: AI-recommended evidence types for controls
- **Policy Draft Generation**: Automated policy content drafting
- **Risk Narrative Creation**: AI-generated risk descriptions and analysis
- **Regulatory Impact Assessment**: AI-assisted impact analysis for regulatory changes
- **Predictive Risk Scoring**: Machine learning-based risk trajectory forecasting

---

## 15. Business Impact Analysis & Continuity

### Business Process Registry

Register and assess critical business processes with:

- **Recovery Objectives**: Recovery Time Objective (RTO), Recovery Point Objective (RPO), Maximum Tolerable Period of Disruption (MTPD) — all in hours
- **Criticality Assessment**: Critical, High, Medium, Low
- **Organizational Context**: Department and owner assignment
- **Dependency Mapping**: System, vendor, personnel, and data dependencies

### Single Points of Failure

- **SPOF Identification**: Components that represent single points of failure
- **Risk Assessment**: Risk level per SPOF (Critical, High, Medium, Low)
- **Mitigation Status**: Unmitigated, Partial, Mitigated
- **Process Linkage**: Association to affected business processes

### Disaster Scenarios

- **Scenario Types**: Data center failure, ransomware, natural disaster, etc.
- **Risk Assessment**: Likelihood and impact scoring
- **Process Impact**: Count of affected business processes
- **Continuity Plans**: Link to response plans

### Business Continuity Plans

- **Plan Lifecycle**: Draft → Approved → Active → Archived
- **Scenario Association**: Each plan linked to specific disaster scenarios
- **Testing & Exercises**: Plan validation through structured exercises
- **Owner Assignment**: Clear plan ownership

### Exercises & Drills

- **Exercise Types**: Tabletop, Walkthrough, Simulation, Full-Scale
- **Scheduling**: Date-based exercise planning
- **Participation Tracking**: Participant records
- **Results**: Pass, Partial, Fail outcomes
- **Testing History**: Last tested date per continuity plan

---

## 16. Regulatory Change Management

### Regulatory Source Monitoring

ComplianceForge monitors regulatory changes through multiple channels:

- **RSS Feeds**: Automated monitoring of regulatory body publications
- **API Integration**: Direct API connections to regulatory databases
- **Manual Entry**: Manual regulatory change registration
- **Source Subscription Management**: Subscribe/unsubscribe from regional regulatory bodies

### Change Detection & Classification

- **Change Attributes**: Title, summary, source (e.g., UK ICO, EU EDPB), published date, effective date
- **Severity Classification**: Critical, High, Medium, Low
- **Status Workflow**: New → Under Review → Assessed → Action Required → Resolved → Dismissed
- **Region & Framework Tagging**: Applicable frameworks and geographic regions
- **Impact Scoring**: Quantified impact assessment

### Impact Assessment

- **Per-Organization Assessment**: Evaluate how each regulatory change affects your specific organization
- **Affected Framework Identification**: Automatic mapping to adopted compliance frameworks
- **AI-Assisted Analysis**: AI-powered impact assessment recommendations
- **Action Tracking**: Response plan creation and monitoring

### Dashboard Metrics

- New changes count
- Pending assessments
- Upcoming deadlines
- Actions required

---

## 17. Advanced Analytics & Reporting

### Executive KPI Dashboard

Real-time key performance indicators with trend visualization:

| KPI | Description |
|-----|-------------|
| **Compliance Score** | Aggregate compliance percentage with trend arrow |
| **Open Risks** | Total open risks with critical count |
| **Incident MTTR** | Mean Time to Resolution for incidents |
| **Policy Attestation Rate** | Percentage of required attestations completed |
| **Vendor Risk Score** | Aggregate third-party risk indicator |
| **Control Implementation %** | Percentage of controls fully implemented |

Each KPI includes trend indicators (up/down/stable) and sparkline mini-charts.

### Compliance Trends

- **Time-Series Charts**: Line charts showing compliance percentage over 12 months
- **Framework Breakdown**: Per-framework trend lines
- **Period Selection**: 3-month, 6-month, 12-month views
- **Point-in-Time Snapshots**: Daily analytics snapshots for historical analysis

### Risk Analytics

- **Risk Count by Level Over Time**: Trend visualization by severity
- **Heatmap Trends**: Evolution of risk distribution
- **Predictive Scoring**: ML-based risk trajectory forecasting with confidence intervals

### Incident Analytics

- **Volume Trends**: Incident count over time
- **MTTR Trends**: Resolution time analysis
- **Severity Breakdown**: Distribution by severity level

### Benchmarking

- **Peer Comparison**: Your metrics vs. peer average, P75, P90
- **Percentile Positioning**: Where you stand relative to industry benchmarks
- **Top Movers**: Fastest improving and fastest degrading metrics

### Custom Dashboards

- **Dashboard Builder**: Create personalized analytics views
- **Widget Types**: Configurable chart and metric widgets
- **Saved Views**: Persist and share custom dashboards

### Report Generation

- **Custom Report Definitions**: Template-based report composition with sections
- **Scheduled Generation**: Cron-based automatic report generation
- **Export Formats**: PDF, Excel, HTML, CSV
- **Report History**: Full execution history with downloadable outputs

---

## 18. Marketplace

### Package Discovery

The marketplace offers a curated ecosystem of compliance add-ons:

- **Package Types**: Control Library, Policy Template, Risk Template, Framework Pack, Integration
- **Categories**: Security, Privacy, Governance, Risk, Compliance, Audit
- **Regions**: Global, EU, UK, US, APAC
- **Search & Filter**: Full-text search with multi-faceted filtering
- **Sort Options**: Popular, Highest Rated, Newest, Price (ascending/descending)

### Package Details

- **Publisher Information**: Verified publishers with profiles
- **Ratings & Reviews**: Star ratings and user reviews
- **Download Statistics**: Popularity metrics
- **Pricing**: Free and paid packages with clear price labels
- **Version Control**: Package versioning with update tracking

### Installation & Management

- **One-Click Install**: Seamless package installation
- **Installed Packages Tab**: View and manage installed packages
- **Version Tracking**: Installation date and version information
- **Uninstall**: Clean package removal

---

## 19. Authentication, Authorization & Security

### Authentication

- **JWT Tokens**: Stateless authentication with configurable expiry (default: 24 hours)
- **Refresh Token Rotation**: Secure token renewal without re-authentication
- **OAuth 2.0 / OIDC SSO**: Enterprise SSO integration (Active Directory, Okta, etc.)
- **Multi-Factor Authentication**: TOTP, SMS, Email, Hardware Key support
- **Session Management**: Active session tracking with revocation capability
- **Password Security**: bcrypt hashing with secure reset token flow (SHA-256 hashed, single-use)

### Authorization — Attribute-Based Access Control (ABAC)

ComplianceForge implements a sophisticated ABAC engine with a deny-overrides combining algorithm:

- **Policy Model**: Allow/Deny policies with complex condition rules
- **Attribute Types**: Subject, Resource, Action, Environment conditions
- **Operators**: equals, not_equals, in, not_in, contains, starts_with, ends_with, between, in_cidr, regex, and more
- **Time-Based Rules**: Access restrictions based on time-of-day or date ranges
- **IP-Based Rules**: CIDR-based network access controls
- **Field-Level Permissions**: Data masking and visibility controls (Visible, Masked, Hidden) per role
- **Policy Assignment**: Assign to individual users, roles, or all users
- **Decision Audit Trail**: Every access decision logged for compliance and forensics
- **Test Evaluation**: Administrators can test policies before deployment

### Data Protection

| Data Type | Encryption Method |
|-----------|------------------|
| **Passwords** | bcrypt |
| **JWT Tokens** | HMAC-SHA256 signing |
| **Session Tokens** | SHA-256 hashing |
| **PII (DSR data)** | AES-256-GCM |
| **Integration Secrets** | AES-256 |
| **Password Reset Tokens** | SHA-256, single-use |

### Platform Security

- **Row-Level Security**: PostgreSQL RLS policies enforce tenant isolation at the database level
- **CORS Configuration**: Configurable allowed origins
- **Rate Limiting**: Configurable RPS limits (default: 100)
- **Audit Logging**: Partitioned audit trail capturing all user actions with IP addresses and change details
- **API Key Management**: Create, revoke, and scope API keys with rate limiting
- **Webhook Signing**: HMAC-SHA256 signatures for outbound webhooks

---

## 20. Integration Hub

### Integration Types

- **SSO Providers**: OIDC and SAML configuration for enterprise identity providers
- **SIEM Systems**: Security Information and Event Management integration
- **ITSM Platforms**: IT Service Management system connections
- **Cloud Platforms**: Cloud infrastructure integrations
- **Custom Integrations**: Configurable third-party system connections

### Integration Features

- **Encrypted Credential Storage**: AES-256 encrypted configuration and secrets
- **Connection Testing**: One-click health checks to validate integration connectivity
- **Sync Operations**: Manual and scheduled data synchronization
- **Sync Logging**: Complete audit trail of synchronization events with error handling
- **Capability Declaration**: Each integration declares its capabilities for the platform to consume
- **API Key Lifecycle**: Generate, scope, and revoke API keys for programmatic access

---

## 21. Guided Onboarding

ComplianceForge features a **7-step onboarding wizard** that takes organizations from signup to operational in minutes:

### Step 1: Organization Profile
Configure organization name, legal name, industry, country, and employee count.

### Step 2: Industry Assessment
Answer targeted questions to determine regulatory applicability:
- Do you process personal data? → Recommends GDPR
- Do you handle payment cards? → Recommends PCI DSS
- Are you critical national infrastructure? → Recommends NIS2, CAF
- Are you public sector? → Recommends relevant frameworks
- Do you provide supply chain services? → Recommends Cyber Essentials

### Step 3: Framework Selection
Multi-select from available frameworks with "Recommended" badges based on assessment answers. Framework count limited by subscription tier.

### Step 4: Team Setup
Invite team members by email with role assignment (Viewer, Editor, Admin, Auditor).

### Step 5: Risk Appetite Configuration
Define tolerance levels (Very Low → Very High) per risk category:
- Cybersecurity, Operational, Compliance, Financial, Reputational, Strategic
- Select risk matrix size (3x3, 4x4, or 5x5)

### Step 6: Quick Assessment
Auto-populated controls from selected frameworks. Set initial implementation status (Not Implemented, Partial, Implemented, N/A) to establish a baseline compliance score.

### Step 7: Summary & Launch
Review all setup details and launch the platform with a pre-configured compliance posture.

**Progress Tracking**: Visual step counter with connected progress bar, back/next/skip navigation.

---

## 22. Deployment & Infrastructure

### Docker Compose Deployment

```
Services:
  api         → Go REST/gRPC server      [Port 8080, 9090]
  frontend    → Next.js application      [Port 3000]
  postgres    → PostgreSQL 16            [Port 5432]
  redis       → Redis cache             [Port 6379]
  rabbitmq    → Message queue            [Port 5672, 15672 mgmt]
  mailhog     → Dev email testing        [Port 1025, 8025]
```

### Configuration Management

Viper-based configuration supporting:
- YAML config files
- Environment variable overrides (CF_ prefix)
- Multi-environment support (development, staging, production)
- Sensible defaults for all settings

### Database Management

- **29 Database Migrations**: Managed via golang-migrate
- **Connection Pooling**: pgx driver with 25 max connections, 5 minimum idle
- **Partitioned Tables**: Audit logs partitioned by date for performance
- **JSONB Support**: Flexible metadata storage for extensible attributes

### Background Workers

Five specialized schedulers running as background processes:

| Scheduler | Responsibility |
|-----------|---------------|
| **Regulatory Scheduler** | Regulatory deadline monitoring |
| **DSR Scheduler** | GDPR DSR SLA tracking and notifications |
| **Workflow Scheduler** | Approval SLA escalation and timeout handling |
| **Report Scheduler** | Scheduled report generation |
| **Analytics Scheduler** | Periodic analytics snapshot aggregation |

### Build & Deployment

- Makefile with 15+ targets for build, test, lint, and deploy
- Docker multi-stage builds for optimized images
- GHCR (GitHub Container Registry) image publishing
- Kubernetes-ready architecture (k8s directory prepared)

### Health & Monitoring

- `/health` endpoint (unauthenticated) checking database, Redis, and RabbitMQ connectivity
- Structured JSON logging via zerolog with request ID correlation
- Per-service logging for targeted debugging

---

## 23. Testing & Quality Assurance

### Backend Testing

- **Unit Tests**: Service-level tests with race condition detection enabled
- **Integration Tests**: Docker Compose-based integration test suite
- **Security Scanning**: Gosec for Go security analysis, Trivy for container scanning
- **Code Coverage**: Coverage reporting for test adequacy

### Frontend Testing

- **Unit Tests**: Vitest for component and utility testing
- **E2E Tests**: Playwright for full end-to-end browser testing
- **Type Safety**: TypeScript strict mode across the entire frontend
- **Code Quality**: ESLint + Prettier for consistent code style
- **Dependency Audit**: npm audit for vulnerability scanning

### Quality Patterns

- Zod runtime validation at system boundaries
- React Hook Form with schema-driven validation
- Comprehensive error states (loading, error, empty) across all pages
- Toast notifications for user feedback on all operations

---

## Subscription Tiers

ComplianceForge supports tiered access with feature gating:

| Tier | Description |
|------|-------------|
| **Starter** | Core GRC features with limited frameworks |
| **Professional** | Extended framework library and advanced features |
| **Enterprise** | Full platform with SSO, ABAC, advanced analytics |
| **Unlimited** | No restrictions, priority support |

Feature flags enforce tier-appropriate access, and the subscription management UI supports plan viewing, upgrades, downgrades, and cancellation.

---

## Summary

ComplianceForge delivers a **comprehensive, enterprise-grade GRC platform** that addresses the full spectrum of governance, risk, and compliance requirements for organizations operating in complex regulatory environments. Its key differentiators include:

- **Multi-Framework Coverage**: Nine frameworks out of the box with cross-mapping to eliminate duplicate effort
- **Regulatory Automation**: Purpose-built modules for GDPR (DSR) and NIS2 compliance with statutory deadline tracking
- **AI-Powered Intelligence**: Automated remediation planning, risk prediction, and regulatory impact assessment
- **Zero-Trust Security**: ABAC with deny-overrides, field-level permissions, AES-256 encryption, and complete audit trails
- **Workflow Automation**: Configurable multi-step approval workflows with SLA tracking and escalation
- **Real-Time Analytics**: Trend analysis, benchmarking, predictive scoring, and custom dashboards
- **Extensible Ecosystem**: Marketplace for community-contributed frameworks, templates, and integrations
- **Deployment Flexibility**: Containerized architecture ready for Docker Compose or Kubernetes deployment

From initial onboarding through continuous compliance monitoring, ComplianceForge provides the tools, automation, and intelligence that modern enterprises need to manage risk, maintain compliance, and demonstrate accountability to regulators and stakeholders.
