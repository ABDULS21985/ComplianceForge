-- Migration 028: Business Impact Analysis & Business Continuity
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - business_processes are the core entity: each represents a business function
--     with multi-dimensional impact ratings (financial, regulatory, reputational,
--     legal, operational, safety) and recovery objectives (RTO, RPO, MTPD).
--   - bia_scenarios model disruptive events (cyber attack, natural disaster, supply
--     chain failure, etc.) with affected processes/assets and timeline-based impact
--     progression via JSONB impact_timeline.
--   - continuity_plans are structured response documents with activation criteria,
--     command structure, communication plan, and recovery procedures — all stored
--     as JSONB to accommodate varying plan structures.
--   - bc_exercises track plan testing (tabletop, walkthrough, simulation, full)
--     with measured RTO/RPO achievement, lessons learned, and improvement actions.
--   - process_dependencies_map provides a flexible dependency graph: processes can
--     depend on other processes, assets, vendors, systems, or personnel — with
--     criticality flags and alternative availability tracking.
--   - Refs auto-generated: BPR-YYYY-NNNN, BIA-YYYY-NNNN, BCP-YYYY-NNNN, BCE-YYYY-NNNN.
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: business_processes
-- ============================================================================

CREATE TABLE business_processes (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    process_ref                     VARCHAR(20) NOT NULL,
    name                            VARCHAR(300) NOT NULL,
    description                     TEXT,
    process_owner_user_id           UUID REFERENCES users(id) ON DELETE SET NULL,
    department                      VARCHAR(200),
    category                        VARCHAR(30) NOT NULL DEFAULT 'operational'
                                    CHECK (category IN ('core', 'support', 'management', 'operational', 'strategic', 'compliance')),
    criticality                     VARCHAR(20) NOT NULL DEFAULT 'medium'
                                    CHECK (criticality IN ('critical', 'high', 'medium', 'low')),
    status                          VARCHAR(20) NOT NULL DEFAULT 'active'
                                    CHECK (status IN ('active', 'inactive', 'under_review', 'deprecated')),

    -- Financial impact
    financial_impact_per_hour_eur   DECIMAL(12,2),
    financial_impact_per_day_eur    DECIMAL(12,2),

    -- Multi-dimensional impact ratings
    regulatory_impact               VARCHAR(20)
                                    CHECK (regulatory_impact IS NULL OR regulatory_impact IN ('critical', 'high', 'medium', 'low', 'none')),
    reputational_impact             VARCHAR(20)
                                    CHECK (reputational_impact IS NULL OR reputational_impact IN ('critical', 'high', 'medium', 'low', 'none')),
    legal_impact                    VARCHAR(20)
                                    CHECK (legal_impact IS NULL OR legal_impact IN ('critical', 'high', 'medium', 'low', 'none')),
    operational_impact              VARCHAR(20)
                                    CHECK (operational_impact IS NULL OR operational_impact IN ('critical', 'high', 'medium', 'low', 'none')),
    safety_impact                   VARCHAR(20)
                                    CHECK (safety_impact IS NULL OR safety_impact IN ('critical', 'high', 'medium', 'low', 'none')),

    -- Recovery objectives
    rto_hours                       DECIMAL(8,2),
    rpo_hours                       DECIMAL(8,2),
    mtpd_hours                      DECIMAL(8,2),
    minimum_service_level           VARCHAR(300),

    -- Dependencies (arrays of UUIDs for flexible linking)
    dependent_asset_ids             UUID[],
    dependent_vendor_ids            UUID[],
    dependent_process_ids           UUID[],
    key_personnel_user_ids          UUID[],

    -- Classification & scheduling
    data_classification             VARCHAR(50),
    peak_periods                    TEXT[],
    last_bia_date                   DATE,
    next_bia_due                    DATE,
    bia_frequency_months            INT,

    notes                           TEXT,
    metadata                        JSONB,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_business_processes_org_ref UNIQUE (organization_id, process_ref)
);

-- Indexes
CREATE INDEX idx_biz_processes_org ON business_processes(organization_id);
CREATE INDEX idx_biz_processes_org_criticality ON business_processes(organization_id, criticality);
CREATE INDEX idx_biz_processes_org_status ON business_processes(organization_id, status);
CREATE INDEX idx_biz_processes_org_category ON business_processes(organization_id, category);
CREATE INDEX idx_biz_processes_owner ON business_processes(process_owner_user_id) WHERE process_owner_user_id IS NOT NULL;
CREATE INDEX idx_biz_processes_department ON business_processes(organization_id, department) WHERE department IS NOT NULL;
CREATE INDEX idx_biz_processes_bia_due ON business_processes(next_bia_due) WHERE next_bia_due IS NOT NULL;
CREATE INDEX idx_biz_processes_rto ON business_processes(organization_id, rto_hours) WHERE rto_hours IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_biz_processes_updated_at
    BEFORE UPDATE ON business_processes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE business_processes IS 'Business process register for BIA. Each process has multi-dimensional impact ratings, recovery objectives (RTO/RPO/MTPD), dependency tracking, and BIA scheduling.';
COMMENT ON COLUMN business_processes.process_ref IS 'Auto-generated reference per org per year: BPR-YYYY-NNNN.';
COMMENT ON COLUMN business_processes.rto_hours IS 'Recovery Time Objective: maximum tolerable downtime in hours before unacceptable business impact.';
COMMENT ON COLUMN business_processes.rpo_hours IS 'Recovery Point Objective: maximum tolerable data loss in hours.';
COMMENT ON COLUMN business_processes.mtpd_hours IS 'Maximum Tolerable Period of Disruption: absolute limit before business survival is threatened.';
COMMENT ON COLUMN business_processes.peak_periods IS 'Periods of peak activity when disruption impact is amplified: ["Q4", "month-end", "tax-season"].';

-- ============================================================================
-- TABLE: bia_scenarios
-- ============================================================================

CREATE TABLE bia_scenarios (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    scenario_ref                VARCHAR(20) NOT NULL,
    name                        VARCHAR(300) NOT NULL,
    description                 TEXT,
    scenario_type               VARCHAR(30) NOT NULL
                                CHECK (scenario_type IN ('cyber_attack', 'natural_disaster', 'pandemic', 'supply_chain', 'infrastructure_failure', 'personnel_loss', 'regulatory', 'reputational', 'technology_failure', 'other')),
    likelihood                  VARCHAR(20) NOT NULL DEFAULT 'possible'
                                CHECK (likelihood IN ('rare', 'unlikely', 'possible', 'likely', 'almost_certain')),
    affected_process_ids        UUID[],
    affected_asset_ids          UUID[],
    impact_timeline             JSONB,
    estimated_financial_loss_eur DECIMAL(14,2),
    mitigation_strategy         TEXT,
    status                      VARCHAR(20) NOT NULL DEFAULT 'draft'
                                CHECK (status IN ('draft', 'assessed', 'approved', 'archived')),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_bia_scenarios_org_ref UNIQUE (organization_id, scenario_ref)
);

-- Indexes
CREATE INDEX idx_bia_scenarios_org ON bia_scenarios(organization_id);
CREATE INDEX idx_bia_scenarios_org_type ON bia_scenarios(organization_id, scenario_type);
CREATE INDEX idx_bia_scenarios_org_status ON bia_scenarios(organization_id, status);
CREATE INDEX idx_bia_scenarios_likelihood ON bia_scenarios(organization_id, likelihood);

-- Trigger
CREATE TRIGGER trg_bia_scenarios_updated_at
    BEFORE UPDATE ON bia_scenarios
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE bia_scenarios IS 'Disruptive event scenarios for BIA analysis. Each scenario defines affected processes/assets, impact timeline, financial loss estimate, and mitigation strategy.';
COMMENT ON COLUMN bia_scenarios.scenario_ref IS 'Auto-generated reference per org per year: BIA-YYYY-NNNN.';
COMMENT ON COLUMN bia_scenarios.impact_timeline IS 'JSONB timeline of impact progression: {"1h": {"operational": "high", "financial_eur": 5000}, "4h": {...}, "24h": {...}, "72h": {...}}';

-- ============================================================================
-- TABLE: continuity_plans
-- ============================================================================

CREATE TABLE continuity_plans (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    plan_ref                VARCHAR(20) NOT NULL,
    name                    VARCHAR(300) NOT NULL,
    plan_type               VARCHAR(30) NOT NULL DEFAULT 'bcp'
                            CHECK (plan_type IN ('bcp', 'drp', 'crisis_management', 'pandemic', 'it_recovery', 'communication', 'site_recovery')),
    status                  VARCHAR(20) NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft', 'in_review', 'approved', 'active', 'under_revision', 'retired')),
    version                 VARCHAR(20),
    scope_description       TEXT,
    covered_scenario_ids    UUID[],
    covered_process_ids     UUID[],

    -- Plan content (structured JSONB for flexible plan formats)
    activation_criteria     TEXT,
    activation_authority    TEXT,
    command_structure       JSONB,
    communication_plan      JSONB,
    recovery_procedures     JSONB,
    resource_requirements   JSONB,
    alternate_site_details  JSONB,

    -- Ownership & approval
    owner_user_id           UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_at             TIMESTAMPTZ,
    next_review_date        DATE,
    review_frequency_months INT,

    document_path           VARCHAR(500),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_continuity_plans_org_ref UNIQUE (organization_id, plan_ref)
);

-- Indexes
CREATE INDEX idx_cont_plans_org ON continuity_plans(organization_id);
CREATE INDEX idx_cont_plans_org_type ON continuity_plans(organization_id, plan_type);
CREATE INDEX idx_cont_plans_org_status ON continuity_plans(organization_id, status);
CREATE INDEX idx_cont_plans_owner ON continuity_plans(owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX idx_cont_plans_approved_by ON continuity_plans(approved_by) WHERE approved_by IS NOT NULL;
CREATE INDEX idx_cont_plans_review_date ON continuity_plans(next_review_date) WHERE next_review_date IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_cont_plans_updated_at
    BEFORE UPDATE ON continuity_plans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE continuity_plans IS 'Business continuity and disaster recovery plans. Each plan covers specific scenarios and processes with structured activation criteria, command structure, communication plan, and recovery procedures.';
COMMENT ON COLUMN continuity_plans.plan_ref IS 'Auto-generated reference per org per year: BCP-YYYY-NNNN.';
COMMENT ON COLUMN continuity_plans.command_structure IS 'JSONB incident command structure: {"incident_commander": "...", "teams": [{"name": "IT Recovery", "lead": "...", "members": [...]}]}';
COMMENT ON COLUMN continuity_plans.communication_plan IS 'JSONB communication plan: {"internal": [...], "external": [...], "regulatory": [...], "media": [...]}';
COMMENT ON COLUMN continuity_plans.recovery_procedures IS 'JSONB ordered recovery steps: [{"phase": "Immediate", "steps": [...], "rto_target_hours": 4}, ...]';

-- ============================================================================
-- TABLE: bc_exercises
-- ============================================================================

CREATE TABLE bc_exercises (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    exercise_ref            VARCHAR(20) NOT NULL,
    name                    VARCHAR(300) NOT NULL,
    exercise_type           VARCHAR(20) NOT NULL
                            CHECK (exercise_type IN ('tabletop', 'walkthrough', 'simulation', 'full_exercise', 'call_tree')),
    plan_id                 UUID REFERENCES continuity_plans(id) ON DELETE SET NULL,
    scenario_id             UUID REFERENCES bia_scenarios(id) ON DELETE SET NULL,
    status                  VARCHAR(20) NOT NULL DEFAULT 'planned'
                            CHECK (status IN ('planned', 'in_progress', 'completed', 'cancelled')),
    scheduled_date          DATE,
    actual_date             DATE,
    participants            JSONB,

    -- Results
    rto_achieved_hours      DECIMAL(8,2),
    rpo_achieved_hours      DECIMAL(8,2),
    objectives_met          BOOLEAN,
    lessons_learned         TEXT,
    gaps_identified         TEXT,
    improvement_actions     JSONB,
    overall_rating          VARCHAR(20)
                            CHECK (overall_rating IS NULL OR overall_rating IN ('excellent', 'good', 'satisfactory', 'needs_improvement', 'failed')),

    report_document_path    VARCHAR(500),
    conducted_by            UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_bc_exercises_org_ref UNIQUE (organization_id, exercise_ref)
);

-- Indexes
CREATE INDEX idx_bc_exercises_org ON bc_exercises(organization_id);
CREATE INDEX idx_bc_exercises_org_type ON bc_exercises(organization_id, exercise_type);
CREATE INDEX idx_bc_exercises_org_status ON bc_exercises(organization_id, status);
CREATE INDEX idx_bc_exercises_plan ON bc_exercises(plan_id) WHERE plan_id IS NOT NULL;
CREATE INDEX idx_bc_exercises_scenario ON bc_exercises(scenario_id) WHERE scenario_id IS NOT NULL;
CREATE INDEX idx_bc_exercises_scheduled ON bc_exercises(scheduled_date) WHERE scheduled_date IS NOT NULL;
CREATE INDEX idx_bc_exercises_conducted_by ON bc_exercises(conducted_by) WHERE conducted_by IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_bc_exercises_updated_at
    BEFORE UPDATE ON bc_exercises
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE bc_exercises IS 'Business continuity exercise/drill records. Tracks plan testing with measured RTO/RPO achievement, participant data, lessons learned, and improvement actions.';
COMMENT ON COLUMN bc_exercises.exercise_ref IS 'Auto-generated reference per org per year: BCE-YYYY-NNNN.';
COMMENT ON COLUMN bc_exercises.participants IS 'JSONB participant list: [{"user_id": "...", "name": "...", "role": "Incident Commander", "attended": true}, ...]';
COMMENT ON COLUMN bc_exercises.improvement_actions IS 'JSONB improvement actions: [{"description": "...", "assigned_to": "...", "due_date": "...", "status": "open"}, ...]';

-- ============================================================================
-- TABLE: process_dependencies_map
-- ============================================================================

CREATE TABLE process_dependencies_map (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    process_id                  UUID NOT NULL REFERENCES business_processes(id) ON DELETE CASCADE,
    dependency_type             VARCHAR(30) NOT NULL
                                CHECK (dependency_type IN ('upstream', 'downstream', 'bidirectional', 'supporting')),
    dependency_entity_type      VARCHAR(30) NOT NULL
                                CHECK (dependency_entity_type IN ('process', 'asset', 'vendor', 'system', 'personnel', 'facility', 'service')),
    dependency_entity_id        UUID,
    dependency_name             VARCHAR(300) NOT NULL,
    is_critical                 BOOLEAN NOT NULL DEFAULT false,
    alternative_available       BOOLEAN NOT NULL DEFAULT false,
    alternative_description     TEXT,
    recovery_sequence           INT,
    notes                       TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_proc_deps_org ON process_dependencies_map(organization_id);
CREATE INDEX idx_proc_deps_process ON process_dependencies_map(process_id);
CREATE INDEX idx_proc_deps_entity ON process_dependencies_map(dependency_entity_type, dependency_entity_id);
CREATE INDEX idx_proc_deps_critical ON process_dependencies_map(organization_id, is_critical) WHERE is_critical = true;
CREATE INDEX idx_proc_deps_sequence ON process_dependencies_map(process_id, recovery_sequence) WHERE recovery_sequence IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_proc_deps_updated_at
    BEFORE UPDATE ON process_dependencies_map
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE process_dependencies_map IS 'Flexible dependency graph for business processes. Maps dependencies on other processes, assets, vendors, systems, personnel, and facilities with criticality flags and recovery sequencing.';
COMMENT ON COLUMN process_dependencies_map.dependency_type IS 'Relationship direction: upstream (this process depends on), downstream (depends on this process), bidirectional, or supporting.';
COMMENT ON COLUMN process_dependencies_map.recovery_sequence IS 'Order in which this dependency should be recovered. Lower numbers = recover first.';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate business process reference: BPR-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_business_process_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.process_ref IS NULL OR NEW.process_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN process_ref ~ ('^BPR-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(process_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM business_processes
        WHERE organization_id = NEW.organization_id;

        NEW.process_ref := 'BPR-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_biz_processes_generate_ref
    BEFORE INSERT ON business_processes
    FOR EACH ROW EXECUTE FUNCTION generate_business_process_ref();

-- Auto-generate BIA scenario reference: BIA-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_bia_scenario_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.scenario_ref IS NULL OR NEW.scenario_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN scenario_ref ~ ('^BIA-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(scenario_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM bia_scenarios
        WHERE organization_id = NEW.organization_id;

        NEW.scenario_ref := 'BIA-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_bia_scenarios_generate_ref
    BEFORE INSERT ON bia_scenarios
    FOR EACH ROW EXECUTE FUNCTION generate_bia_scenario_ref();

-- Auto-generate continuity plan reference: BCP-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_continuity_plan_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.plan_ref IS NULL OR NEW.plan_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN plan_ref ~ ('^BCP-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(plan_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM continuity_plans
        WHERE organization_id = NEW.organization_id;

        NEW.plan_ref := 'BCP-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cont_plans_generate_ref
    BEFORE INSERT ON continuity_plans
    FOR EACH ROW EXECUTE FUNCTION generate_continuity_plan_ref();

-- Auto-generate BC exercise reference: BCE-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_bc_exercise_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.exercise_ref IS NULL OR NEW.exercise_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN exercise_ref ~ ('^BCE-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(exercise_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM bc_exercises
        WHERE organization_id = NEW.organization_id;

        NEW.exercise_ref := 'BCE-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_bc_exercises_generate_ref
    BEFORE INSERT ON bc_exercises
    FOR EACH ROW EXECUTE FUNCTION generate_bc_exercise_ref();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- business_processes
ALTER TABLE business_processes ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_processes FORCE ROW LEVEL SECURITY;

CREATE POLICY biz_processes_tenant_select ON business_processes FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY biz_processes_tenant_insert ON business_processes FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY biz_processes_tenant_update ON business_processes FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY biz_processes_tenant_delete ON business_processes FOR DELETE
    USING (organization_id = get_current_tenant());

-- bia_scenarios
ALTER TABLE bia_scenarios ENABLE ROW LEVEL SECURITY;
ALTER TABLE bia_scenarios FORCE ROW LEVEL SECURITY;

CREATE POLICY bia_scenarios_tenant_select ON bia_scenarios FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY bia_scenarios_tenant_insert ON bia_scenarios FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY bia_scenarios_tenant_update ON bia_scenarios FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY bia_scenarios_tenant_delete ON bia_scenarios FOR DELETE
    USING (organization_id = get_current_tenant());

-- continuity_plans
ALTER TABLE continuity_plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE continuity_plans FORCE ROW LEVEL SECURITY;

CREATE POLICY cont_plans_tenant_select ON continuity_plans FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY cont_plans_tenant_insert ON continuity_plans FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY cont_plans_tenant_update ON continuity_plans FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY cont_plans_tenant_delete ON continuity_plans FOR DELETE
    USING (organization_id = get_current_tenant());

-- bc_exercises
ALTER TABLE bc_exercises ENABLE ROW LEVEL SECURITY;
ALTER TABLE bc_exercises FORCE ROW LEVEL SECURITY;

CREATE POLICY bc_exercises_tenant_select ON bc_exercises FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY bc_exercises_tenant_insert ON bc_exercises FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY bc_exercises_tenant_update ON bc_exercises FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY bc_exercises_tenant_delete ON bc_exercises FOR DELETE
    USING (organization_id = get_current_tenant());

-- process_dependencies_map
ALTER TABLE process_dependencies_map ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_dependencies_map FORCE ROW LEVEL SECURITY;

CREATE POLICY proc_deps_tenant_select ON process_dependencies_map FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY proc_deps_tenant_insert ON process_dependencies_map FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY proc_deps_tenant_update ON process_dependencies_map FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY proc_deps_tenant_delete ON process_dependencies_map FOR DELETE
    USING (organization_id = get_current_tenant());
