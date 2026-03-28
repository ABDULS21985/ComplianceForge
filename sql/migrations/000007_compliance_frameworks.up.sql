-- Migration 007: Compliance Frameworks, Controls, Domains & Cross-Mappings
-- ComplianceForge GRC Platform
--
-- This is the intellectual property core of the platform. The schema supports:
--   - 9+ standard frameworks (ISO 27001, NIST CSF, PCI DSS, etc.)
--   - Custom/private frameworks created by organizations
--   - Versioned frameworks for standard updates (e.g., PCI DSS 3.2.1 → 4.0)
--   - Hierarchical control structures (domain → family → control → sub-control)
--   - Cross-framework mappings with confidence scoring
--   - Full-text search across control libraries
--
-- Design decisions:
--   - compliance_frameworks.organization_id is NULLABLE: NULL = system/global
--     framework shipped with the platform (read-only); NOT NULL = custom framework
--     created by a customer org
--   - framework_domains supports self-referencing parent_domain_id for arbitrary
--     nesting depth (NIST 800-53 has families, COBIT has domains → processes)
--   - framework_controls supports parent_control_id for sub-controls (PCI DSS
--     has requirements → sub-requirements → test procedures)
--   - search_vector is a GENERATED ALWAYS column combining code + title + description
--     for PostgreSQL full-text search — no application-level indexing needed
--   - framework_control_mappings stores bidirectional relationships with a
--     mapping_strength score (0.00–1.00) enabling "implement ISO 27001 and get
--     X% coverage of PCI DSS" calculations

-- ============================================================================
-- TABLE: compliance_frameworks
-- ============================================================================

CREATE TABLE compliance_frameworks (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID REFERENCES organizations(id) ON DELETE CASCADE,
    code                    VARCHAR(50) NOT NULL,
    name                    VARCHAR(255) NOT NULL,
    full_name               TEXT,
    version                 VARCHAR(20) NOT NULL,
    description             TEXT,
    issuing_body            VARCHAR(255),
    category                VARCHAR(50),
    applicable_regions      TEXT[] DEFAULT '{}',
    applicable_industries   TEXT[] DEFAULT '{}',
    is_system_framework     BOOLEAN NOT NULL DEFAULT false,
    is_active               BOOLEAN NOT NULL DEFAULT true,
    effective_date          DATE,
    sunset_date             DATE,
    total_controls          INT NOT NULL DEFAULT 0,
    icon_url                TEXT,
    color_hex               VARCHAR(7),
    metadata                JSONB DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ,

    -- System frameworks have globally unique code+version; custom ones are unique per org.
    CONSTRAINT uq_frameworks_org_code_version UNIQUE NULLS NOT DISTINCT (organization_id, code, version),

    -- Category must be one of the defined values.
    CONSTRAINT chk_framework_category CHECK (
        category IS NULL OR category IN ('security', 'privacy', 'governance', 'risk', 'operational', 'it_service_management')
    )
);

-- Indexes
CREATE INDEX idx_frameworks_org ON compliance_frameworks(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_frameworks_code ON compliance_frameworks(code) WHERE deleted_at IS NULL;
CREATE INDEX idx_frameworks_category ON compliance_frameworks(category) WHERE deleted_at IS NULL;
CREATE INDEX idx_frameworks_system ON compliance_frameworks(is_system_framework) WHERE is_system_framework = true AND deleted_at IS NULL;
CREATE INDEX idx_frameworks_active ON compliance_frameworks(is_active) WHERE is_active = true AND deleted_at IS NULL;
CREATE INDEX idx_frameworks_regions ON compliance_frameworks USING gin (applicable_regions) WHERE deleted_at IS NULL;
CREATE INDEX idx_frameworks_industries ON compliance_frameworks USING gin (applicable_industries) WHERE deleted_at IS NULL;
CREATE INDEX idx_frameworks_deleted ON compliance_frameworks(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE TRIGGER trg_compliance_frameworks_updated_at
    BEFORE UPDATE ON compliance_frameworks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE compliance_frameworks IS 'Compliance standards and regulations. organization_id=NULL means system-provided (immutable); non-NULL means custom/private to that org.';
COMMENT ON COLUMN compliance_frameworks.code IS 'Short identifier: ISO27001, NIST_CSF_2, PCI_DSS_4, UK_GDPR, NCSC_CAF, CYBER_ESSENTIALS, NIST_800_53, COBIT_2019, ITIL_4';
COMMENT ON COLUMN compliance_frameworks.total_controls IS 'Denormalized count updated via trigger or application code for dashboard performance.';
COMMENT ON COLUMN compliance_frameworks.sunset_date IS 'Date when this framework version is retired. Used to prompt migrations to newer versions.';

-- ============================================================================
-- TABLE: framework_domains
-- ============================================================================

CREATE TABLE framework_domains (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    framework_id        UUID NOT NULL REFERENCES compliance_frameworks(id) ON DELETE CASCADE,
    code                VARCHAR(50) NOT NULL,
    name                VARCHAR(255) NOT NULL,
    description         TEXT,
    sort_order          INT NOT NULL DEFAULT 0,
    parent_domain_id    UUID REFERENCES framework_domains(id) ON DELETE CASCADE,
    depth_level         INT NOT NULL DEFAULT 0,
    total_controls      INT NOT NULL DEFAULT 0,
    metadata            JSONB DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_domain_framework_code UNIQUE (framework_id, code),
    CONSTRAINT chk_domain_depth CHECK (depth_level >= 0 AND depth_level <= 5)
);

CREATE INDEX idx_domains_framework ON framework_domains(framework_id);
CREATE INDEX idx_domains_parent ON framework_domains(parent_domain_id) WHERE parent_domain_id IS NOT NULL;
CREATE INDEX idx_domains_sort ON framework_domains(framework_id, sort_order);

CREATE TRIGGER trg_framework_domains_updated_at
    BEFORE UPDATE ON framework_domains
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE framework_domains IS 'Hierarchical groupings within a framework: ISO Annex A themes, NIST CSF functions, PCI DSS requirements, COBIT domains.';
COMMENT ON COLUMN framework_domains.depth_level IS '0=top-level domain, 1=sub-domain/family, 2+=deeper nesting. Max depth 5.';

-- ============================================================================
-- TABLE: framework_controls
-- ============================================================================

CREATE TABLE framework_controls (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    framework_id            UUID NOT NULL REFERENCES compliance_frameworks(id) ON DELETE CASCADE,
    domain_id               UUID REFERENCES framework_domains(id) ON DELETE SET NULL,
    code                    VARCHAR(100) NOT NULL,
    title                   VARCHAR(500) NOT NULL,
    description             TEXT,
    guidance                TEXT,
    objective               TEXT,
    control_type            VARCHAR(30),
    implementation_type     VARCHAR(30),
    is_mandatory            BOOLEAN NOT NULL DEFAULT true,
    priority                VARCHAR(10),
    sort_order              INT NOT NULL DEFAULT 0,
    parent_control_id       UUID REFERENCES framework_controls(id) ON DELETE CASCADE,
    depth_level             INT NOT NULL DEFAULT 0,
    evidence_requirements   JSONB DEFAULT '[]',
    test_procedures         JSONB DEFAULT '[]',
    "references"            JSONB DEFAULT '[]',
    keywords                TEXT[] DEFAULT '{}',
    metadata                JSONB DEFAULT '{}',

    -- Full-text search vector: auto-generated from code + title + description
    search_vector           tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(code, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(guidance, '')), 'C')
    ) STORED,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_control_framework_code UNIQUE (framework_id, code),

    CONSTRAINT chk_control_type CHECK (
        control_type IS NULL OR control_type IN ('preventive', 'detective', 'corrective', 'directive', 'compensating', 'recovery')
    ),
    CONSTRAINT chk_implementation_type CHECK (
        implementation_type IS NULL OR implementation_type IN ('technical', 'administrative', 'physical', 'management')
    ),
    CONSTRAINT chk_control_priority CHECK (
        priority IS NULL OR priority IN ('critical', 'high', 'medium', 'low')
    ),
    CONSTRAINT chk_control_depth CHECK (depth_level >= 0 AND depth_level <= 10)
);

-- Indexes
CREATE INDEX idx_controls_framework ON framework_controls(framework_id);
CREATE INDEX idx_controls_domain ON framework_controls(domain_id) WHERE domain_id IS NOT NULL;
CREATE INDEX idx_controls_parent ON framework_controls(parent_control_id) WHERE parent_control_id IS NOT NULL;
CREATE INDEX idx_controls_type ON framework_controls(control_type) WHERE control_type IS NOT NULL;
CREATE INDEX idx_controls_impl_type ON framework_controls(implementation_type) WHERE implementation_type IS NOT NULL;
CREATE INDEX idx_controls_priority ON framework_controls(priority) WHERE priority IS NOT NULL;
CREATE INDEX idx_controls_sort ON framework_controls(framework_id, sort_order);
CREATE INDEX idx_controls_keywords ON framework_controls USING gin (keywords);
-- Full-text search GIN index
CREATE INDEX idx_controls_search ON framework_controls USING gin (search_vector);

CREATE TRIGGER trg_framework_controls_updated_at
    BEFORE UPDATE ON framework_controls
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE framework_controls IS 'Individual requirements/controls within a framework. Supports hierarchical nesting via parent_control_id.';
COMMENT ON COLUMN framework_controls.search_vector IS 'Weighted tsvector: code+title=A, description=B, guidance=C. Enables ts_rank-based full-text search.';
COMMENT ON COLUMN framework_controls.evidence_requirements IS 'JSON array of expected evidence: [{"type":"document","description":"...","required":true}]';
COMMENT ON COLUMN framework_controls.test_procedures IS 'JSON array of test steps: [{"step":1,"description":"...","expected_result":"..."}]';

-- ============================================================================
-- TABLE: framework_control_mappings
-- ============================================================================

CREATE TABLE framework_control_mappings (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_control_id       UUID NOT NULL REFERENCES framework_controls(id) ON DELETE CASCADE,
    target_control_id       UUID NOT NULL REFERENCES framework_controls(id) ON DELETE CASCADE,
    mapping_type            VARCHAR(30) NOT NULL,
    mapping_strength        DECIMAL(3,2) NOT NULL DEFAULT 0.50,
    notes                   TEXT,
    is_verified             BOOLEAN NOT NULL DEFAULT false,
    verified_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    verified_at             TIMESTAMPTZ,
    metadata                JSONB DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_control_mapping UNIQUE (source_control_id, target_control_id),
    -- Cannot map a control to itself
    CONSTRAINT chk_mapping_not_self CHECK (source_control_id != target_control_id),
    CONSTRAINT chk_mapping_type CHECK (
        mapping_type IN ('equivalent', 'partial', 'related', 'superset', 'subset')
    ),
    CONSTRAINT chk_mapping_strength CHECK (
        mapping_strength >= 0.00 AND mapping_strength <= 1.00
    )
);

CREATE INDEX idx_mappings_source ON framework_control_mappings(source_control_id);
CREATE INDEX idx_mappings_target ON framework_control_mappings(target_control_id);
CREATE INDEX idx_mappings_type ON framework_control_mappings(mapping_type);
CREATE INDEX idx_mappings_verified ON framework_control_mappings(is_verified) WHERE is_verified = false;
-- Composite for the most common query: "what does control X map to?"
CREATE INDEX idx_mappings_source_target ON framework_control_mappings(source_control_id, target_control_id);

CREATE TRIGGER trg_control_mappings_updated_at
    BEFORE UPDATE ON framework_control_mappings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE framework_control_mappings IS 'Cross-framework control mappings with confidence scoring. Powers "implement ISO 27001, get X% of PCI DSS free" calculations.';
COMMENT ON COLUMN framework_control_mappings.mapping_strength IS '0.00=loosely related, 1.00=exact equivalent. Used to weight cross-framework compliance scores.';
COMMENT ON COLUMN framework_control_mappings.mapping_type IS 'equivalent=1:1, partial=partial overlap, related=thematic link, superset=source covers more, subset=target covers more.';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- compliance_frameworks: system frameworks (org_id NULL) visible to all; custom scoped to org
ALTER TABLE compliance_frameworks ENABLE ROW LEVEL SECURITY;
ALTER TABLE compliance_frameworks FORCE ROW LEVEL SECURITY;

CREATE POLICY frameworks_tenant_select ON compliance_frameworks FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY frameworks_tenant_insert ON compliance_frameworks FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY frameworks_tenant_update ON compliance_frameworks FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY frameworks_tenant_delete ON compliance_frameworks FOR DELETE
    USING (organization_id = get_current_tenant());

-- framework_domains: visible if the parent framework is visible
-- Since domains don't have organization_id directly, we join through framework.
-- For simplicity and performance, domains inherit visibility from their framework.
-- The application layer ensures domain operations go through framework access checks.
-- We do NOT enable RLS on framework_domains — they're accessed through framework_controls
-- which are checked at the control_implementations level.

-- framework_controls: same logic as domains — no direct org_id.
-- Access is controlled through control_implementations (which has org_id + RLS).

-- framework_control_mappings: no org_id — mappings are global knowledge.
-- Custom mappings could be added later with an optional org_id column.
