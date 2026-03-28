-- Migration 010: Risk Management Core — Categories, Appetite, Matrices & Risk Register
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - risk_categories supports hierarchical categorization with system defaults
--     (organization_id NULL) and custom org-specific categories
--   - risk_appetite_statements links appetite to categories, enabling per-category
--     risk tolerance thresholds as required by ISO 31000 and COSO ERM
--   - risk_matrices are configurable per org (3×3 to 5×5) with JSONB scales —
--     this avoids a rigid schema and lets orgs define custom impact dimensions
--     (financial, operational, reputational, etc.)
--   - risks table is the core risk register with three risk dimensions:
--     inherent (before controls), residual (after controls), target (desired state)
--   - risk_velocity and risk_proximity are emerging best practices from IRM/COSO
--     for characterizing how fast a risk materializes and when
--   - search_vector is maintained by trigger (not GENERATED ALWAYS) because it
--     references columns that may be updated independently
--   - Financial impacts stored in EUR (platform targets European enterprises)

-- ============================================================================
-- TABLE: risk_categories
-- ============================================================================

CREATE TABLE risk_categories (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID REFERENCES organizations(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    code                VARCHAR(50) NOT NULL,
    description         TEXT,
    parent_category_id  UUID REFERENCES risk_categories(id) ON DELETE SET NULL,
    color_hex           VARCHAR(7),
    icon                VARCHAR(50),
    sort_order          INT NOT NULL DEFAULT 0,
    is_system_default   BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_risk_cat_org_code UNIQUE NULLS NOT DISTINCT (organization_id, code)
);

CREATE INDEX idx_risk_cat_org ON risk_categories(organization_id);
CREATE INDEX idx_risk_cat_parent ON risk_categories(parent_category_id) WHERE parent_category_id IS NOT NULL;
CREATE INDEX idx_risk_cat_system ON risk_categories(is_system_default) WHERE is_system_default = true;

CREATE TRIGGER trg_risk_categories_updated_at
    BEFORE UPDATE ON risk_categories
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS: system defaults visible to all, custom scoped to org
ALTER TABLE risk_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_categories FORCE ROW LEVEL SECURITY;

CREATE POLICY risk_cat_tenant_select ON risk_categories FOR SELECT
    USING (organization_id IS NULL OR organization_id = get_current_tenant());
CREATE POLICY risk_cat_tenant_insert ON risk_categories FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY risk_cat_tenant_update ON risk_categories FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY risk_cat_tenant_delete ON risk_categories FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_categories IS 'Hierarchical risk taxonomy. System defaults (org_id NULL) provide ISO 31000-aligned categories; orgs can add custom ones.';

-- ============================================================================
-- TABLE: risk_appetite_statements
-- ============================================================================

CREATE TABLE risk_appetite_statements (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    risk_category_id                UUID NOT NULL REFERENCES risk_categories(id) ON DELETE CASCADE,
    appetite_level                  VARCHAR(20) NOT NULL,
    appetite_description            TEXT,
    quantitative_threshold_low      DECIMAL(12,2),
    quantitative_threshold_high     DECIMAL(12,2),
    threshold_metric                VARCHAR(100),
    tolerance_level                 VARCHAR(20) NOT NULL DEFAULT 'moderate',
    approved_by                     UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_at                     TIMESTAMPTZ,
    review_date                     DATE,
    status                          VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_appetite_org_cat UNIQUE (organization_id, risk_category_id),
    CONSTRAINT chk_appetite_level CHECK (
        appetite_level IN ('averse', 'minimal', 'cautious', 'open', 'hungry')
    ),
    CONSTRAINT chk_appetite_tolerance CHECK (
        tolerance_level IN ('zero', 'low', 'moderate', 'high')
    ),
    CONSTRAINT chk_appetite_status CHECK (
        status IN ('draft', 'approved', 'under_review')
    ),
    CONSTRAINT chk_appetite_thresholds CHECK (
        quantitative_threshold_high IS NULL OR quantitative_threshold_low IS NULL
        OR quantitative_threshold_high >= quantitative_threshold_low
    )
);

CREATE INDEX idx_appetite_org ON risk_appetite_statements(organization_id);
CREATE INDEX idx_appetite_category ON risk_appetite_statements(risk_category_id);
CREATE INDEX idx_appetite_status ON risk_appetite_statements(organization_id, status);
CREATE INDEX idx_appetite_review ON risk_appetite_statements(review_date) WHERE review_date IS NOT NULL;

CREATE TRIGGER trg_risk_appetite_updated_at
    BEFORE UPDATE ON risk_appetite_statements
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE risk_appetite_statements ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_appetite_statements FORCE ROW LEVEL SECURITY;

CREATE POLICY appetite_tenant_select ON risk_appetite_statements FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY appetite_tenant_insert ON risk_appetite_statements FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY appetite_tenant_update ON risk_appetite_statements FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY appetite_tenant_delete ON risk_appetite_statements FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_appetite_statements IS 'Per-category risk appetite and tolerance thresholds. Supports both qualitative levels and quantitative thresholds.';
COMMENT ON COLUMN risk_appetite_statements.threshold_metric IS 'Unit of measurement: financial_loss_eur, downtime_hours, affected_records, incidents_per_year, etc.';

-- ============================================================================
-- TABLE: risk_matrices
-- ============================================================================

CREATE TABLE risk_matrices (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    description         TEXT,
    likelihood_scale    JSONB NOT NULL,
    impact_scale        JSONB NOT NULL,
    risk_levels         JSONB NOT NULL,
    matrix_size         INT NOT NULL DEFAULT 5,
    is_default          BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_matrix_size CHECK (matrix_size >= 3 AND matrix_size <= 10)
);

-- Ensure only one default matrix per org
CREATE UNIQUE INDEX idx_risk_matrix_default ON risk_matrices(organization_id) WHERE is_default = true;
CREATE INDEX idx_risk_matrix_org ON risk_matrices(organization_id);

CREATE TRIGGER trg_risk_matrices_updated_at
    BEFORE UPDATE ON risk_matrices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE risk_matrices ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_matrices FORCE ROW LEVEL SECURITY;

CREATE POLICY matrix_tenant_select ON risk_matrices FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY matrix_tenant_insert ON risk_matrices FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY matrix_tenant_update ON risk_matrices FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY matrix_tenant_delete ON risk_matrices FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risk_matrices IS 'Configurable risk matrices (3×3 to 10×10). JSONB scales allow custom impact dimensions (financial, operational, reputational).';
COMMENT ON COLUMN risk_matrices.likelihood_scale IS 'JSON array: [{"level":1,"label":"Rare","description":"...","probability_range":"0-5%"}, ...]';
COMMENT ON COLUMN risk_matrices.impact_scale IS 'JSON array: [{"level":1,"label":"Insignificant","financial":"<€10K","operational":"...","reputational":"..."}, ...]';
COMMENT ON COLUMN risk_matrices.risk_levels IS 'JSON array: [{"min_score":1,"max_score":4,"label":"Low","color":"#22C55E"}, ...]';

-- ============================================================================
-- TABLE: risks (Core Risk Register)
-- ============================================================================

CREATE TABLE risks (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    risk_ref                VARCHAR(20) NOT NULL,
    title                   VARCHAR(500) NOT NULL,
    description             TEXT,
    risk_category_id        UUID REFERENCES risk_categories(id) ON DELETE SET NULL,
    risk_source             VARCHAR(100),
    risk_type               VARCHAR(50),
    status                  VARCHAR(30) NOT NULL DEFAULT 'identified',
    owner_user_id           UUID REFERENCES users(id) ON DELETE SET NULL,
    delegate_user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    business_unit_id        UUID,
    risk_matrix_id          UUID REFERENCES risk_matrices(id) ON DELETE SET NULL,

    -- Inherent Risk (before any controls are applied)
    inherent_likelihood     INT,
    inherent_impact         INT,
    inherent_risk_score     DECIMAL(5,2),
    inherent_risk_level     VARCHAR(20),

    -- Residual Risk (after existing controls)
    residual_likelihood     INT,
    residual_impact         INT,
    residual_risk_score     DECIMAL(5,2),
    residual_risk_level     VARCHAR(20),

    -- Target Risk (desired end state after treatment)
    target_likelihood       INT,
    target_impact           INT,
    target_risk_score       DECIMAL(5,2),
    target_risk_level       VARCHAR(20),

    -- Impact dimensions
    financial_impact_eur    DECIMAL(15,2),
    impact_description      TEXT,
    impact_categories       JSONB DEFAULT '{}',

    -- Velocity and proximity (IRM/COSO best practices)
    risk_velocity           VARCHAR(20),
    risk_proximity          VARCHAR(20),

    -- Dates and review cycle
    identified_date         DATE NOT NULL DEFAULT CURRENT_DATE,
    last_assessed_date      DATE,
    next_review_date        DATE,
    review_frequency        VARCHAR(20) DEFAULT 'quarterly',

    -- Regulatory and control linkage
    linked_regulations      TEXT[] DEFAULT '{}',
    linked_control_ids      UUID[] DEFAULT '{}',

    -- Additional metadata
    tags                    TEXT[] DEFAULT '{}',
    attachments             JSONB DEFAULT '[]',
    is_emerging             BOOLEAN NOT NULL DEFAULT false,
    metadata                JSONB DEFAULT '{}',

    -- Full-text search (maintained by trigger)
    search_vector           tsvector,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ,

    CONSTRAINT uq_risk_org_ref UNIQUE (organization_id, risk_ref),
    CONSTRAINT chk_risk_status CHECK (
        status IN ('identified', 'assessed', 'treated', 'accepted', 'closed', 'monitoring')
    ),
    CONSTRAINT chk_risk_source CHECK (
        risk_source IS NULL OR risk_source IN ('internal', 'external', 'third_party', 'regulatory', 'environmental', 'emerging')
    ),
    CONSTRAINT chk_risk_type CHECK (
        risk_type IS NULL OR risk_type IN ('threat', 'vulnerability', 'event', 'consequence', 'opportunity')
    ),
    CONSTRAINT chk_risk_velocity CHECK (
        risk_velocity IS NULL OR risk_velocity IN ('immediate', 'fast', 'moderate', 'slow')
    ),
    CONSTRAINT chk_risk_proximity CHECK (
        risk_proximity IS NULL OR risk_proximity IN ('imminent', 'short_term', 'medium_term', 'long_term')
    ),
    CONSTRAINT chk_risk_review_freq CHECK (
        review_frequency IS NULL OR review_frequency IN ('monthly', 'quarterly', 'semi_annually', 'annually')
    ),
    CONSTRAINT chk_inherent_likelihood CHECK (inherent_likelihood IS NULL OR (inherent_likelihood >= 1 AND inherent_likelihood <= 10)),
    CONSTRAINT chk_inherent_impact CHECK (inherent_impact IS NULL OR (inherent_impact >= 1 AND inherent_impact <= 10)),
    CONSTRAINT chk_residual_likelihood CHECK (residual_likelihood IS NULL OR (residual_likelihood >= 1 AND residual_likelihood <= 10)),
    CONSTRAINT chk_residual_impact CHECK (residual_impact IS NULL OR (residual_impact >= 1 AND residual_impact <= 10)),
    CONSTRAINT chk_target_likelihood CHECK (target_likelihood IS NULL OR (target_likelihood >= 1 AND target_likelihood <= 10)),
    CONSTRAINT chk_target_impact CHECK (target_impact IS NULL OR (target_impact >= 1 AND target_impact <= 10))
);

-- Indexes
CREATE INDEX idx_risks_org ON risks(organization_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_risks_status ON risks(organization_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_risks_category ON risks(risk_category_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_risks_owner ON risks(owner_user_id) WHERE owner_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_risks_delegate ON risks(delegate_user_id) WHERE delegate_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_risks_inherent_level ON risks(organization_id, inherent_risk_level) WHERE deleted_at IS NULL;
CREATE INDEX idx_risks_residual_level ON risks(organization_id, residual_risk_level) WHERE deleted_at IS NULL;
CREATE INDEX idx_risks_residual_score ON risks(organization_id, residual_risk_score DESC NULLS LAST) WHERE deleted_at IS NULL;
CREATE INDEX idx_risks_next_review ON risks(next_review_date) WHERE next_review_date IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_risks_emerging ON risks(organization_id, is_emerging) WHERE is_emerging = true AND deleted_at IS NULL;
CREATE INDEX idx_risks_tags ON risks USING gin (tags) WHERE deleted_at IS NULL;
CREATE INDEX idx_risks_regulations ON risks USING gin (linked_regulations) WHERE deleted_at IS NULL;
CREATE INDEX idx_risks_search ON risks USING gin (search_vector);
CREATE INDEX idx_risks_deleted ON risks(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_risks_matrix ON risks(risk_matrix_id) WHERE risk_matrix_id IS NOT NULL AND deleted_at IS NULL;

CREATE TRIGGER trg_risks_updated_at
    BEFORE UPDATE ON risks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Trigger: auto-update search_vector on INSERT or UPDATE
CREATE OR REPLACE FUNCTION risks_search_vector_update()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', coalesce(NEW.risk_ref, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(NEW.description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(NEW.impact_description, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_risks_search_vector
    BEFORE INSERT OR UPDATE OF title, description, risk_ref, impact_description ON risks
    FOR EACH ROW EXECUTE FUNCTION risks_search_vector_update();

-- Trigger: auto-calculate risk scores on likelihood/impact changes
CREATE OR REPLACE FUNCTION risks_calculate_scores()
RETURNS TRIGGER AS $$
BEGIN
    -- Inherent risk score = likelihood × impact
    IF NEW.inherent_likelihood IS NOT NULL AND NEW.inherent_impact IS NOT NULL THEN
        NEW.inherent_risk_score := NEW.inherent_likelihood * NEW.inherent_impact;
        NEW.inherent_risk_level := CASE
            WHEN NEW.inherent_risk_score >= 20 THEN 'critical'
            WHEN NEW.inherent_risk_score >= 12 THEN 'high'
            WHEN NEW.inherent_risk_score >= 6 THEN 'medium'
            WHEN NEW.inherent_risk_score >= 3 THEN 'low'
            ELSE 'very_low'
        END;
    END IF;

    -- Residual risk score
    IF NEW.residual_likelihood IS NOT NULL AND NEW.residual_impact IS NOT NULL THEN
        NEW.residual_risk_score := NEW.residual_likelihood * NEW.residual_impact;
        NEW.residual_risk_level := CASE
            WHEN NEW.residual_risk_score >= 20 THEN 'critical'
            WHEN NEW.residual_risk_score >= 12 THEN 'high'
            WHEN NEW.residual_risk_score >= 6 THEN 'medium'
            WHEN NEW.residual_risk_score >= 3 THEN 'low'
            ELSE 'very_low'
        END;
    END IF;

    -- Target risk score
    IF NEW.target_likelihood IS NOT NULL AND NEW.target_impact IS NOT NULL THEN
        NEW.target_risk_score := NEW.target_likelihood * NEW.target_impact;
        NEW.target_risk_level := CASE
            WHEN NEW.target_risk_score >= 20 THEN 'critical'
            WHEN NEW.target_risk_score >= 12 THEN 'high'
            WHEN NEW.target_risk_score >= 6 THEN 'medium'
            WHEN NEW.target_risk_score >= 3 THEN 'low'
            ELSE 'very_low'
        END;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_risks_calculate_scores
    BEFORE INSERT OR UPDATE OF inherent_likelihood, inherent_impact,
                               residual_likelihood, residual_impact,
                               target_likelihood, target_impact ON risks
    FOR EACH ROW EXECUTE FUNCTION risks_calculate_scores();

-- Trigger: auto-generate risk_ref as RSK-NNNN
CREATE OR REPLACE FUNCTION risks_generate_ref()
RETURNS TRIGGER AS $$
DECLARE
    next_num INT;
BEGIN
    IF NEW.risk_ref IS NULL OR NEW.risk_ref = '' THEN
        SELECT COALESCE(MAX(
            CASE WHEN risk_ref ~ '^RSK-[0-9]+$'
                 THEN CAST(SUBSTRING(risk_ref FROM 5) AS INT)
                 ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM risks
        WHERE organization_id = NEW.organization_id;

        NEW.risk_ref := 'RSK-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_risks_generate_ref
    BEFORE INSERT ON risks
    FOR EACH ROW EXECUTE FUNCTION risks_generate_ref();

-- RLS
ALTER TABLE risks ENABLE ROW LEVEL SECURITY;
ALTER TABLE risks FORCE ROW LEVEL SECURITY;

CREATE POLICY risks_tenant_select ON risks FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY risks_tenant_insert ON risks FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY risks_tenant_update ON risks FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY risks_tenant_delete ON risks FOR DELETE
    USING (organization_id = get_current_tenant());

COMMENT ON TABLE risks IS 'Core risk register. Tracks inherent, residual, and target risk with velocity/proximity dimensions. Aligned with ISO 31000 and COSO ERM.';
COMMENT ON COLUMN risks.risk_ref IS 'Auto-generated sequential reference per org: RSK-0001, RSK-0002, etc.';
COMMENT ON COLUMN risks.inherent_risk_score IS 'Auto-calculated: likelihood × impact. Updated by trigger.';
COMMENT ON COLUMN risks.risk_velocity IS 'How quickly the risk would materialise (IRM best practice): immediate, fast, moderate, slow.';
COMMENT ON COLUMN risks.risk_proximity IS 'When the risk is expected to materialise: imminent (<1mo), short_term (<6mo), medium_term (<2yr), long_term (>2yr).';
COMMENT ON COLUMN risks.impact_categories IS 'Multi-dimensional impact: {"financial":4,"operational":3,"reputational":5,"compliance":4,"safety":1}';
