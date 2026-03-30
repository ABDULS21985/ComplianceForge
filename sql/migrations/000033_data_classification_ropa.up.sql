-- Migration 033: Data Classification & Records of Processing Activities (ROPA)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - data_classifications define a hierarchical sensitivity classification scheme
--     (e.g., Public, Internal, Confidential, Restricted) with handling requirements,
--     encryption, access restriction, masking, retention, and disposal policies.
--   - data_categories organize personal/non-personal data types with GDPR special
--     category flags and Article 9 legal basis tracking. Each category links to a
--     classification level.
--   - processing_activities form the ROPA register (GDPR Art. 30). Each activity
--     records purpose, legal basis, data subjects, recipients, transfers, retention,
--     DPIA status, and linked controls. Full lifecycle with review scheduling.
--   - data_flow_maps visualize data movement between systems, vendors, and
--     geographies. Linked to processing activities for ROPA completeness.
--   - ropa_exports track generated ROPA export files for audit trail.
--   - Refs auto-generated: RPA-YYYY-NNNN (processing activities), RPX-YYYY-NNNN (exports).
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: data_classifications
-- ============================================================================

CREATE TABLE data_classifications (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                        VARCHAR(200) NOT NULL,
    level                       INT NOT NULL,
    description                 TEXT,
    handling_requirements       TEXT,
    encryption_required         BOOLEAN NOT NULL DEFAULT false,
    access_restriction_required BOOLEAN NOT NULL DEFAULT false,
    data_masking_required       BOOLEAN NOT NULL DEFAULT false,
    retention_policy            TEXT,
    disposal_method             TEXT,
    color_hex                   VARCHAR(7),
    is_system                   BOOLEAN NOT NULL DEFAULT false,
    sort_order                  INT NOT NULL DEFAULT 0,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_data_class_org ON data_classifications(organization_id);
CREATE INDEX idx_data_class_org_level ON data_classifications(organization_id, level);
CREATE INDEX idx_data_class_org_sort ON data_classifications(organization_id, sort_order);
CREATE INDEX idx_data_class_system ON data_classifications(is_system) WHERE is_system = true;

-- Trigger
CREATE TRIGGER trg_data_class_updated_at
    BEFORE UPDATE ON data_classifications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE data_classifications IS 'Hierarchical data sensitivity classification scheme. Each level defines handling requirements, encryption, access restriction, masking, retention, and disposal policies. Higher level = more sensitive.';
COMMENT ON COLUMN data_classifications.level IS 'Numeric sensitivity level (higher = more sensitive). E.g., 1 = Public, 2 = Internal, 3 = Confidential, 4 = Restricted.';
COMMENT ON COLUMN data_classifications.color_hex IS 'Hex color code for UI display: "#FF0000" for restricted, "#00FF00" for public.';

-- ============================================================================
-- TABLE: data_categories
-- ============================================================================

CREATE TABLE data_categories (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                    VARCHAR(200) NOT NULL,
    category_type           VARCHAR(30) NOT NULL
                            CHECK (category_type IN (
                                'personal', 'sensitive_personal', 'financial',
                                'health', 'biometric', 'genetic', 'behavioral'
                            )),
    gdpr_special_category   BOOLEAN NOT NULL DEFAULT false,
    gdpr_article_9_basis    TEXT,
    description             TEXT,
    examples                TEXT[],
    classification_id       UUID REFERENCES data_classifications(id) ON DELETE SET NULL,
    retention_period_months INT,
    is_system               BOOLEAN NOT NULL DEFAULT false,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_data_cat_org ON data_categories(organization_id);
CREATE INDEX idx_data_cat_org_type ON data_categories(organization_id, category_type);
CREATE INDEX idx_data_cat_classification ON data_categories(classification_id) WHERE classification_id IS NOT NULL;
CREATE INDEX idx_data_cat_gdpr_special ON data_categories(organization_id, gdpr_special_category) WHERE gdpr_special_category = true;
CREATE INDEX idx_data_cat_system ON data_categories(is_system) WHERE is_system = true;
CREATE INDEX idx_data_cat_examples ON data_categories USING GIN (examples);

-- Trigger
CREATE TRIGGER trg_data_cat_updated_at
    BEFORE UPDATE ON data_categories
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE data_categories IS 'Data category register for privacy management. Categories are typed (personal, sensitive_personal, financial, health, biometric, genetic, behavioral) with GDPR special category flags and Article 9 legal basis tracking.';
COMMENT ON COLUMN data_categories.gdpr_article_9_basis IS 'Legal basis under GDPR Article 9 for processing special category data: explicit consent, employment law, vital interests, etc.';
COMMENT ON COLUMN data_categories.examples IS 'Example data elements in this category: ["email address", "phone number", "IP address"].';

-- ============================================================================
-- TABLE: processing_activities
-- ============================================================================

CREATE TABLE processing_activities (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id                 UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    activity_ref                    VARCHAR(20) NOT NULL,
    name                            VARCHAR(300) NOT NULL,
    description                     TEXT,
    purpose                         TEXT,
    legal_basis                     VARCHAR(30) NOT NULL
                                    CHECK (legal_basis IN (
                                        'consent', 'contract', 'legal_obligation',
                                        'vital_interests', 'public_task', 'legitimate_interest'
                                    )),
    legal_basis_detail              TEXT,
    status                          VARCHAR(20) NOT NULL DEFAULT 'draft'
                                    CHECK (status IN ('draft', 'active', 'under_review', 'suspended', 'archived')),
    role                            VARCHAR(20) NOT NULL DEFAULT 'controller'
                                    CHECK (role IN ('controller', 'joint_controller', 'processor')),

    -- Data subjects & categories
    data_subject_categories         TEXT[],
    estimated_data_subjects_count   INT,
    data_category_ids               UUID[],
    special_categories_processed    BOOLEAN NOT NULL DEFAULT false,

    -- Recipients & transfers
    recipient_categories            TEXT[],
    recipient_vendor_ids            UUID[],
    involves_international_transfer BOOLEAN NOT NULL DEFAULT false,
    transfer_countries              TEXT[],
    transfer_safeguards             VARCHAR(30)
                                    CHECK (transfer_safeguards IS NULL OR transfer_safeguards IN (
                                        'adequacy_decision', 'standard_contractual_clauses', 'binding_corporate_rules',
                                        'certification', 'code_of_conduct', 'derogation', 'none'
                                    )),
    tia_conducted                   BOOLEAN NOT NULL DEFAULT false,

    -- Retention
    retention_period_months         INT,
    retention_justification         TEXT,

    -- Systems & storage
    system_ids                      UUID[],
    storage_locations               TEXT[],

    -- DPIA
    dpia_required                   BOOLEAN NOT NULL DEFAULT false,
    dpia_status                     VARCHAR(20)
                                    CHECK (dpia_status IS NULL OR dpia_status IN ('not_started', 'in_progress', 'completed', 'not_required')),
    dpia_document_path              TEXT,

    -- Security & controls
    security_measures               TEXT,
    linked_control_codes            TEXT[],
    risk_level                      VARCHAR(20)
                                    CHECK (risk_level IS NULL OR risk_level IN ('critical', 'high', 'medium', 'low')),

    -- Ownership & review
    data_steward_user_id            UUID REFERENCES users(id) ON DELETE SET NULL,
    department                      VARCHAR(200),
    process_owner_user_id           UUID REFERENCES users(id) ON DELETE SET NULL,
    last_review_date                DATE,
    next_review_date                DATE,
    review_frequency_months         INT DEFAULT 12,

    metadata                        JSONB,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                      TIMESTAMPTZ,

    CONSTRAINT uq_processing_activities_org_ref UNIQUE (organization_id, activity_ref)
);

-- Indexes
CREATE INDEX idx_proc_activities_org ON processing_activities(organization_id);
CREATE INDEX idx_proc_activities_org_status ON processing_activities(organization_id, status);
CREATE INDEX idx_proc_activities_org_basis ON processing_activities(organization_id, legal_basis);
CREATE INDEX idx_proc_activities_org_role ON processing_activities(organization_id, role);
CREATE INDEX idx_proc_activities_steward ON processing_activities(data_steward_user_id) WHERE data_steward_user_id IS NOT NULL;
CREATE INDEX idx_proc_activities_owner ON processing_activities(process_owner_user_id) WHERE process_owner_user_id IS NOT NULL;
CREATE INDEX idx_proc_activities_department ON processing_activities(organization_id, department) WHERE department IS NOT NULL;
CREATE INDEX idx_proc_activities_dpia ON processing_activities(organization_id, dpia_required, dpia_status) WHERE dpia_required = true;
CREATE INDEX idx_proc_activities_transfer ON processing_activities(organization_id, involves_international_transfer) WHERE involves_international_transfer = true;
CREATE INDEX idx_proc_activities_special ON processing_activities(organization_id, special_categories_processed) WHERE special_categories_processed = true;
CREATE INDEX idx_proc_activities_next_review ON processing_activities(next_review_date) WHERE next_review_date IS NOT NULL;
CREATE INDEX idx_proc_activities_risk ON processing_activities(organization_id, risk_level) WHERE risk_level IS NOT NULL;
CREATE INDEX idx_proc_activities_deleted ON processing_activities(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_proc_activities_data_cats ON processing_activities USING GIN (data_category_ids);
CREATE INDEX idx_proc_activities_controls ON processing_activities USING GIN (linked_control_codes);
CREATE INDEX idx_proc_activities_countries ON processing_activities USING GIN (transfer_countries);
CREATE INDEX idx_proc_activities_subjects ON processing_activities USING GIN (data_subject_categories);
CREATE INDEX idx_proc_activities_metadata ON processing_activities USING GIN (metadata);

-- Trigger
CREATE TRIGGER trg_proc_activities_updated_at
    BEFORE UPDATE ON processing_activities
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE processing_activities IS 'ROPA register (GDPR Art. 30). Each record describes a data processing activity with purpose, legal basis, data subjects, recipients, international transfers, retention, DPIA status, and linked security controls.';
COMMENT ON COLUMN processing_activities.activity_ref IS 'Auto-generated reference per org per year: RPA-YYYY-NNNN.';
COMMENT ON COLUMN processing_activities.legal_basis IS 'GDPR Art. 6 legal basis for processing: consent, contract, legal_obligation, vital_interests, public_task, legitimate_interest.';
COMMENT ON COLUMN processing_activities.transfer_safeguards IS 'GDPR Chapter V safeguard mechanism for international transfers.';
COMMENT ON COLUMN processing_activities.data_category_ids IS 'Array of data_categories UUIDs processed in this activity.';

-- ============================================================================
-- TABLE: data_flow_maps
-- ============================================================================

CREATE TABLE data_flow_maps (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    processing_activity_id      UUID NOT NULL REFERENCES processing_activities(id) ON DELETE CASCADE,
    name                        VARCHAR(300) NOT NULL,
    flow_type                   VARCHAR(20) NOT NULL
                                CHECK (flow_type IN ('collection', 'processing', 'storage', 'transfer', 'deletion', 'sharing')),
    source_type                 VARCHAR(20) NOT NULL
                                CHECK (source_type IN ('system', 'vendor', 'user', 'api', 'manual', 'external')),
    source_name                 VARCHAR(300) NOT NULL,
    source_entity_id            UUID,
    destination_type            VARCHAR(20) NOT NULL
                                CHECK (destination_type IN ('system', 'vendor', 'user', 'api', 'manual', 'external')),
    destination_name            VARCHAR(300) NOT NULL,
    destination_entity_id       UUID,
    destination_country         VARCHAR(100),
    data_category_ids           UUID[],
    transfer_method             VARCHAR(200),
    encryption_in_transit       BOOLEAN NOT NULL DEFAULT false,
    encryption_at_rest          BOOLEAN NOT NULL DEFAULT false,
    volume_description          VARCHAR(300),
    frequency                   VARCHAR(100),
    legal_basis                 VARCHAR(200),
    notes                       TEXT,
    sort_order                  INT NOT NULL DEFAULT 0,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_data_flows_org ON data_flow_maps(organization_id);
CREATE INDEX idx_data_flows_activity ON data_flow_maps(processing_activity_id);
CREATE INDEX idx_data_flows_type ON data_flow_maps(flow_type);
CREATE INDEX idx_data_flows_source ON data_flow_maps(source_type, source_entity_id);
CREATE INDEX idx_data_flows_dest ON data_flow_maps(destination_type, destination_entity_id);
CREATE INDEX idx_data_flows_country ON data_flow_maps(destination_country) WHERE destination_country IS NOT NULL;
CREATE INDEX idx_data_flows_sort ON data_flow_maps(processing_activity_id, sort_order);
CREATE INDEX idx_data_flows_data_cats ON data_flow_maps USING GIN (data_category_ids);

-- Trigger
CREATE TRIGGER trg_data_flows_updated_at
    BEFORE UPDATE ON data_flow_maps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE data_flow_maps IS 'Data flow visualization records linked to processing activities. Maps data movement between systems, vendors, and geographies with encryption and legal basis details.';
COMMENT ON COLUMN data_flow_maps.flow_type IS 'Type of data flow: collection, processing, storage, transfer, deletion, or sharing.';
COMMENT ON COLUMN data_flow_maps.source_entity_id IS 'Optional UUID reference to the source entity (system, vendor, etc.) for linking.';

-- ============================================================================
-- TABLE: ropa_exports
-- ============================================================================

CREATE TABLE ropa_exports (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    export_ref              VARCHAR(20) NOT NULL,
    export_date             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    format                  VARCHAR(10) NOT NULL
                            CHECK (format IN ('pdf', 'xlsx', 'csv', 'json')),
    file_path               TEXT NOT NULL,
    activities_included     INT NOT NULL DEFAULT 0,
    exported_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    export_reason           TEXT,
    notes                   TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_ropa_exports_org_ref UNIQUE (organization_id, export_ref)
);

-- Indexes
CREATE INDEX idx_ropa_exports_org ON ropa_exports(organization_id);
CREATE INDEX idx_ropa_exports_date ON ropa_exports(export_date DESC);
CREATE INDEX idx_ropa_exports_format ON ropa_exports(organization_id, format);
CREATE INDEX idx_ropa_exports_exported_by ON ropa_exports(exported_by) WHERE exported_by IS NOT NULL;

COMMENT ON TABLE ropa_exports IS 'Audit trail of ROPA export operations. Tracks when, by whom, in what format, and how many activities were included in each export.';
COMMENT ON COLUMN ropa_exports.export_ref IS 'Auto-generated reference per org per year: RPX-YYYY-NNNN.';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate processing activity reference: RPA-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_processing_activity_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.activity_ref IS NULL OR NEW.activity_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN activity_ref ~ ('^RPA-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(activity_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM processing_activities
        WHERE organization_id = NEW.organization_id;

        NEW.activity_ref := 'RPA-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_proc_activities_generate_ref
    BEFORE INSERT ON processing_activities
    FOR EACH ROW EXECUTE FUNCTION generate_processing_activity_ref();

-- Auto-generate ROPA export reference: RPX-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_ropa_export_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.export_ref IS NULL OR NEW.export_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN export_ref ~ ('^RPX-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(export_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM ropa_exports
        WHERE organization_id = NEW.organization_id;

        NEW.export_ref := 'RPX-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ropa_exports_generate_ref
    BEFORE INSERT ON ropa_exports
    FOR EACH ROW EXECUTE FUNCTION generate_ropa_export_ref();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- data_classifications
ALTER TABLE data_classifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_classifications FORCE ROW LEVEL SECURITY;

CREATE POLICY data_class_tenant_select ON data_classifications FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY data_class_tenant_insert ON data_classifications FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY data_class_tenant_update ON data_classifications FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY data_class_tenant_delete ON data_classifications FOR DELETE
    USING (organization_id = get_current_tenant());

-- data_categories
ALTER TABLE data_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_categories FORCE ROW LEVEL SECURITY;

CREATE POLICY data_cat_tenant_select ON data_categories FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY data_cat_tenant_insert ON data_categories FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY data_cat_tenant_update ON data_categories FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY data_cat_tenant_delete ON data_categories FOR DELETE
    USING (organization_id = get_current_tenant());

-- processing_activities
ALTER TABLE processing_activities ENABLE ROW LEVEL SECURITY;
ALTER TABLE processing_activities FORCE ROW LEVEL SECURITY;

CREATE POLICY proc_activities_tenant_select ON processing_activities FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY proc_activities_tenant_insert ON processing_activities FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY proc_activities_tenant_update ON processing_activities FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY proc_activities_tenant_delete ON processing_activities FOR DELETE
    USING (organization_id = get_current_tenant());

-- data_flow_maps
ALTER TABLE data_flow_maps ENABLE ROW LEVEL SECURITY;
ALTER TABLE data_flow_maps FORCE ROW LEVEL SECURITY;

CREATE POLICY data_flows_tenant_select ON data_flow_maps FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY data_flows_tenant_insert ON data_flow_maps FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY data_flows_tenant_update ON data_flow_maps FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY data_flows_tenant_delete ON data_flow_maps FOR DELETE
    USING (organization_id = get_current_tenant());

-- ropa_exports
ALTER TABLE ropa_exports ENABLE ROW LEVEL SECURITY;
ALTER TABLE ropa_exports FORCE ROW LEVEL SECURITY;

CREATE POLICY ropa_exports_tenant_select ON ropa_exports FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY ropa_exports_tenant_insert ON ropa_exports FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ropa_exports_tenant_update ON ropa_exports FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY ropa_exports_tenant_delete ON ropa_exports FOR DELETE
    USING (organization_id = get_current_tenant());
