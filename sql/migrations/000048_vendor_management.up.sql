-- Migration 048: tenant-isolated third-party and vendor risk management.

CREATE FUNCTION vendor_text_array_is_valid(values_to_check TEXT[], maximum_length INTEGER)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT COALESCE(bool_and(length(btrim(value)) BETWEEN 1 AND maximum_length), true)
    FROM unnest(values_to_check) AS value
$$;

CREATE TABLE vendor_reference_sequences (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    next_value BIGINT NOT NULL DEFAULT 1 CHECK (next_value > 0)
);

CREATE TABLE vendors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendor_ref VARCHAR(32) NOT NULL,
    name VARCHAR(200) NOT NULL CHECK (name = btrim(name) AND length(name) BETWEEN 1 AND 200),
    legal_name VARCHAR(250),
    description TEXT,
    website VARCHAR(500),
    industry VARCHAR(100),
    category VARCHAR(100),
    country_code CHAR(2),
    owner_user_id UUID,
    criticality VARCHAR(16) NOT NULL DEFAULT 'medium'
        CHECK (criticality IN ('critical','high','medium','low')),
    vendor_tier VARCHAR(16) NOT NULL DEFAULT 'tier_3'
        CHECK (vendor_tier IN ('tier_1','tier_2','tier_3','tier_4')),
    risk_tier VARCHAR(16) NOT NULL DEFAULT 'medium'
        CHECK (risk_tier IN ('critical','high','medium','low')),
    risk_score NUMERIC(5,2) CHECK (risk_score IS NULL OR risk_score BETWEEN 0 AND 100),
    status VARCHAR(24) NOT NULL DEFAULT 'prospective'
        CHECK (status IN ('prospective','onboarding','active','suspended','offboarding','offboarded','rejected')),
    service_description TEXT,
    services TEXT[] NOT NULL DEFAULT '{}',
    data_processing BOOLEAN NOT NULL DEFAULT false,
    data_categories TEXT[] NOT NULL DEFAULT '{}',
    processing_locations TEXT[] NOT NULL DEFAULT '{}',
    dpa_required BOOLEAN NOT NULL DEFAULT false,
    dpa_status VARCHAR(20) NOT NULL DEFAULT 'not_required'
        CHECK (dpa_status IN ('not_required','pending','executed','expired','terminated')),
    dpa_in_place BOOLEAN NOT NULL DEFAULT false,
    dpa_reference VARCHAR(200),
    dpa_signed_date DATE,
    dpa_expiry_date DATE,
    assessment_frequency VARCHAR(24) NOT NULL DEFAULT 'annual'
        CHECK (assessment_frequency IN ('monthly','quarterly','semi_annual','annual','biennial','custom')),
    assessment_cadence_days INTEGER NOT NULL DEFAULT 365
        CHECK (assessment_cadence_days BETWEEN 30 AND 1095),
    assessment_status VARCHAR(20) NOT NULL DEFAULT 'not_due'
        CHECK (assessment_status IN ('not_due','due','in_progress','completed','overdue','waived')),
    last_assessment_date DATE,
    next_assessment_date DATE,
    next_review_date DATE,
    onboarding_started_at TIMESTAMPTZ,
    onboarded_at TIMESTAMPTZ,
    suspended_at TIMESTAMPTZ,
    offboarding_started_at TIMESTAMPTZ,
    offboarded_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    retention_until TIMESTAMPTZ,
    legal_hold BOOLEAN NOT NULL DEFAULT false,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    search_vector TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(vendor_ref, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(legal_name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(service_description, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(industry, '')), 'C') ||
        setweight(to_tsvector('simple', coalesce(category, '')), 'C')
    ) STORED,
    CONSTRAINT uq_vendors_org_ref UNIQUE (organization_id, vendor_ref),
    CONSTRAINT uq_vendors_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vendors_owner_tenant FOREIGN KEY (organization_id, owner_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_vendors_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_vendors_optional_text CHECK (
        (legal_name IS NULL OR length(btrim(legal_name)) BETWEEN 1 AND 250) AND
        (description IS NULL OR length(description) <= 20000) AND
        (website IS NULL OR length(btrim(website)) BETWEEN 1 AND 500) AND
        (industry IS NULL OR length(btrim(industry)) BETWEEN 1 AND 100) AND
        (category IS NULL OR length(btrim(category)) BETWEEN 1 AND 100) AND
        (service_description IS NULL OR length(service_description) <= 10000)
    ),
    CONSTRAINT chk_vendors_country CHECK (country_code IS NULL OR country_code ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_vendors_services CHECK (
        cardinality(services) <= 50 AND vendor_text_array_is_valid(services, 200)
    ),
    CONSTRAINT chk_vendors_data_categories CHECK (
        cardinality(data_categories) <= 50 AND vendor_text_array_is_valid(data_categories, 100)
    ),
    CONSTRAINT chk_vendors_processing_locations CHECK (
        cardinality(processing_locations) <= 50 AND vendor_text_array_is_valid(processing_locations, 100)
    ),
    CONSTRAINT chk_vendors_dpa_consistency CHECK (
        (NOT dpa_in_place OR (data_processing AND dpa_status = 'executed'
            AND dpa_signed_date IS NOT NULL AND length(btrim(coalesce(dpa_reference, ''))) > 0))
        AND (NOT dpa_required OR data_processing)
        AND (dpa_expiry_date IS NULL OR dpa_signed_date IS NULL OR dpa_expiry_date >= dpa_signed_date)
    ),
    CONSTRAINT chk_vendors_assessment_dates CHECK (
        next_assessment_date IS NULL OR last_assessment_date IS NULL OR next_assessment_date >= last_assessment_date
    ),
    CONSTRAINT chk_vendors_retention CHECK (retention_until IS NULL OR retention_until >= created_at)
);

CREATE UNIQUE INDEX uq_vendors_org_name_active
    ON vendors (organization_id, lower(name)) WHERE deleted_at IS NULL;

CREATE TABLE vendor_contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendor_id UUID NOT NULL,
    name VARCHAR(200) NOT NULL CHECK (name = btrim(name) AND length(name) BETWEEN 1 AND 200),
    email VARCHAR(320) NOT NULL CHECK (email = btrim(email) AND length(email) BETWEEN 3 AND 320),
    phone VARCHAR(50),
    title VARCHAR(150),
    contact_type VARCHAR(20) NOT NULL DEFAULT 'business'
        CHECK (contact_type IN ('business','security','privacy','legal','billing','technical')),
    is_primary BOOLEAN NOT NULL DEFAULT false,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_vendor_contacts_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vendor_contacts_vendor_tenant FOREIGN KEY (organization_id, vendor_id)
        REFERENCES vendors(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_vendor_contacts_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_vendor_contacts_optional CHECK (
        (phone IS NULL OR length(btrim(phone)) BETWEEN 3 AND 50) AND
        (title IS NULL OR length(btrim(title)) BETWEEN 1 AND 150)
    )
);

CREATE UNIQUE INDEX uq_vendor_contacts_email_active
    ON vendor_contacts (organization_id, vendor_id, lower(email)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_vendor_primary_contact
    ON vendor_contacts (organization_id, vendor_id) WHERE is_primary AND deleted_at IS NULL;

CREATE TABLE vendor_contracts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendor_id UUID NOT NULL,
    contract_ref VARCHAR(100) NOT NULL,
    name VARCHAR(200) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft','active','renewal_due','expired','terminated')),
    start_date DATE,
    end_date DATE,
    notice_days INTEGER NOT NULL DEFAULT 30 CHECK (notice_days BETWEEN 0 AND 1095),
    renewal_date DATE,
    auto_renew BOOLEAN NOT NULL DEFAULT false,
    value_amount NUMERIC(18,2) CHECK (value_amount IS NULL OR value_amount >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'EUR' CHECK (currency ~ '^[A-Z]{3}$'),
    includes_dpa BOOLEAN NOT NULL DEFAULT false,
    signed_at TIMESTAMPTZ,
    owner_user_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_vendor_contracts_org_id UNIQUE (organization_id, id),
    CONSTRAINT uq_vendor_contract_ref UNIQUE (organization_id, vendor_id, contract_ref),
    CONSTRAINT fk_vendor_contracts_vendor_tenant FOREIGN KEY (organization_id, vendor_id)
        REFERENCES vendors(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_vendor_contracts_owner_tenant FOREIGN KEY (organization_id, owner_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_vendor_contracts_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_vendor_contract_text CHECK (
        contract_ref = btrim(contract_ref) AND length(contract_ref) BETWEEN 1 AND 100 AND
        name = btrim(name) AND length(name) BETWEEN 1 AND 200
    ),
    CONSTRAINT chk_vendor_contract_dates CHECK (
        end_date IS NULL OR start_date IS NULL OR end_date >= start_date
    )
);

CREATE TABLE vendor_certifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendor_id UUID NOT NULL,
    name VARCHAR(150) NOT NULL,
    issuer VARCHAR(200),
    certificate_number VARCHAR(150),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('pending','active','expired','revoked')),
    issued_on DATE,
    expires_on DATE,
    evidence_reference VARCHAR(500),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_vendor_certifications_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vendor_certifications_vendor_tenant FOREIGN KEY (organization_id, vendor_id)
        REFERENCES vendors(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_vendor_certifications_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_vendor_certification_name CHECK (name = btrim(name) AND length(name) BETWEEN 1 AND 150),
    CONSTRAINT chk_vendor_certification_dates CHECK (expires_on IS NULL OR issued_on IS NULL OR expires_on >= issued_on)
);

CREATE UNIQUE INDEX uq_vendor_certification_active
    ON vendor_certifications (organization_id, vendor_id, lower(name), coalesce(certificate_number, ''))
    WHERE deleted_at IS NULL;

CREATE TABLE vendor_subprocessors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendor_id UUID NOT NULL,
    name VARCHAR(200) NOT NULL,
    purpose VARCHAR(1000) NOT NULL,
    country_code CHAR(2),
    data_categories TEXT[] NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'proposed'
        CHECK (status IN ('proposed','approved','rejected','removed')),
    approved_at TIMESTAMPTZ,
    approved_by UUID,
    removed_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_vendor_subprocessors_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_vendor_subprocessors_vendor_tenant FOREIGN KEY (organization_id, vendor_id)
        REFERENCES vendors(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_vendor_subprocessors_approver_tenant FOREIGN KEY (organization_id, approved_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_vendor_subprocessors_creator_tenant FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_vendor_subprocessor_text CHECK (
        name = btrim(name) AND length(name) BETWEEN 1 AND 200 AND
        length(btrim(purpose)) BETWEEN 3 AND 1000
    ),
    CONSTRAINT chk_vendor_subprocessor_country CHECK (country_code IS NULL OR country_code ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_vendor_subprocessor_categories CHECK (
        cardinality(data_categories) <= 50 AND vendor_text_array_is_valid(data_categories, 100)
    ),
    CONSTRAINT chk_vendor_subprocessor_state CHECK (
        (status = 'approved' AND approved_at IS NOT NULL AND approved_by IS NOT NULL AND removed_at IS NULL)
        OR (status = 'removed' AND removed_at IS NOT NULL)
        OR (status IN ('proposed','rejected') AND removed_at IS NULL)
    )
);

CREATE UNIQUE INDEX uq_vendor_subprocessor_active
    ON vendor_subprocessors (organization_id, vendor_id, lower(name)) WHERE deleted_at IS NULL;

CREATE TABLE vendor_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendor_id UUID NOT NULL,
    event_type VARCHAR(40) NOT NULL CHECK (event_type IN (
        'created','updated','status_changed','assessment_recorded','contact_added','contact_updated','contact_removed',
        'contract_added','contract_updated','contract_removed','certification_added','certification_updated',
        'certification_removed','subprocessor_added','subprocessor_updated','subprocessor_removed','deleted'
    )),
    actor_user_id UUID NOT NULL,
    vendor_version BIGINT NOT NULL CHECK (vendor_version > 0),
    summary VARCHAR(500) NOT NULL CHECK (length(btrim(summary)) BETWEEN 1 AND 500),
    details JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_vendor_events_vendor_tenant FOREIGN KEY (organization_id, vendor_id)
        REFERENCES vendors(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_vendor_events_actor_tenant FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT
);

-- Earlier modules intentionally left these relationships unbound until the
-- canonical vendor table existed. NOT VALID preserves any legacy orphan rows
-- while enforcing tenant-safe references for all new and changed records.
ALTER TABLE vendor_assessments
    ADD CONSTRAINT fk_vendor_assessments_vendor_tenant
    FOREIGN KEY (organization_id, vendor_id)
    REFERENCES vendors(organization_id, id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE assets
    ADD CONSTRAINT fk_assets_vendor_tenant
    FOREIGN KEY (organization_id, linked_vendor_id)
    REFERENCES vendors(organization_id, id) ON DELETE RESTRICT NOT VALID;

CREATE INDEX idx_vendors_org_updated ON vendors (organization_id, updated_at DESC, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_vendors_org_status ON vendors (organization_id, status, criticality, risk_tier) WHERE deleted_at IS NULL;
CREATE INDEX idx_vendors_org_owner ON vendors (organization_id, owner_user_id) WHERE deleted_at IS NULL AND owner_user_id IS NOT NULL;
CREATE INDEX idx_vendors_org_assessment ON vendors (organization_id, next_assessment_date)
    WHERE deleted_at IS NULL AND status IN ('active','suspended') AND next_assessment_date IS NOT NULL;
CREATE INDEX idx_vendors_org_review ON vendors (organization_id, next_review_date)
    WHERE deleted_at IS NULL AND status IN ('active','suspended') AND next_review_date IS NOT NULL;
CREATE INDEX idx_vendors_search ON vendors USING GIN (search_vector);
CREATE INDEX idx_vendor_contacts_vendor ON vendor_contacts (organization_id, vendor_id, is_primary DESC, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_vendor_contracts_due ON vendor_contracts (organization_id, COALESCE(renewal_date, end_date))
    WHERE deleted_at IS NULL AND status IN ('active','renewal_due');
CREATE INDEX idx_vendor_certifications_expiry ON vendor_certifications (organization_id, expires_on)
    WHERE deleted_at IS NULL AND status IN ('active','pending') AND expires_on IS NOT NULL;
CREATE INDEX idx_vendor_subprocessors_vendor ON vendor_subprocessors (organization_id, vendor_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_vendor_events_timeline ON vendor_events (organization_id, vendor_id, created_at DESC, id DESC);

CREATE TRIGGER trg_vendors_updated_at BEFORE UPDATE ON vendors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_vendor_contacts_updated_at BEFORE UPDATE ON vendor_contacts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_vendor_contracts_updated_at BEFORE UPDATE ON vendor_contracts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_vendor_certifications_updated_at BEFORE UPDATE ON vendor_certifications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_vendor_subprocessors_updated_at BEFORE UPDATE ON vendor_subprocessors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE vendor_reference_sequences ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor_reference_sequences FORCE ROW LEVEL SECURITY;
ALTER TABLE vendors ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendors FORCE ROW LEVEL SECURITY;
ALTER TABLE vendor_contacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor_contacts FORCE ROW LEVEL SECURITY;
ALTER TABLE vendor_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor_contracts FORCE ROW LEVEL SECURITY;
ALTER TABLE vendor_certifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor_certifications FORCE ROW LEVEL SECURITY;
ALTER TABLE vendor_subprocessors ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor_subprocessors FORCE ROW LEVEL SECURITY;
ALTER TABLE vendor_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendor_events FORCE ROW LEVEL SECURITY;

CREATE POLICY vendor_reference_sequences_tenant_all ON vendor_reference_sequences
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendors_tenant_select ON vendors FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY vendors_tenant_insert ON vendors FOR INSERT WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendors_tenant_update ON vendors FOR UPDATE USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendors_tenant_delete ON vendors FOR DELETE USING (organization_id = get_current_tenant());
CREATE POLICY vendor_contacts_tenant_select ON vendor_contacts FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY vendor_contacts_tenant_insert ON vendor_contacts FOR INSERT WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_contacts_tenant_update ON vendor_contacts FOR UPDATE USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_contacts_tenant_delete ON vendor_contacts FOR DELETE USING (organization_id = get_current_tenant());
CREATE POLICY vendor_contracts_tenant_select ON vendor_contracts FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY vendor_contracts_tenant_insert ON vendor_contracts FOR INSERT WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_contracts_tenant_update ON vendor_contracts FOR UPDATE USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_contracts_tenant_delete ON vendor_contracts FOR DELETE USING (organization_id = get_current_tenant());
CREATE POLICY vendor_certifications_tenant_select ON vendor_certifications FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY vendor_certifications_tenant_insert ON vendor_certifications FOR INSERT WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_certifications_tenant_update ON vendor_certifications FOR UPDATE USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_certifications_tenant_delete ON vendor_certifications FOR DELETE USING (organization_id = get_current_tenant());
CREATE POLICY vendor_subprocessors_tenant_select ON vendor_subprocessors FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY vendor_subprocessors_tenant_insert ON vendor_subprocessors FOR INSERT WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_subprocessors_tenant_update ON vendor_subprocessors FOR UPDATE USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY vendor_subprocessors_tenant_delete ON vendor_subprocessors FOR DELETE USING (organization_id = get_current_tenant());
CREATE POLICY vendor_events_tenant_select ON vendor_events FOR SELECT USING (organization_id = get_current_tenant());
CREATE POLICY vendor_events_tenant_insert ON vendor_events FOR INSERT WITH CHECK (organization_id = get_current_tenant());

-- As with notification and incident scheduling, only tenant identifiers are
-- exposed outside RLS. Every vendor row is subsequently read under its tenant.
CREATE FUNCTION vendor_due_tenants(requested_limit INTEGER DEFAULT 100)
RETURNS TABLE (organization_id UUID)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
    SELECT v.organization_id
    FROM public.vendors AS v
    WHERE v.deleted_at IS NULL
      AND v.status IN ('active','suspended')
      AND v.next_assessment_date IS NOT NULL
      AND v.next_assessment_date <= statement_timestamp()::date + 30
    GROUP BY v.organization_id
    ORDER BY MIN(v.next_assessment_date), v.organization_id
    LIMIT LEAST(GREATEST(COALESCE(requested_limit, 100), 1), 1000)
$$;

REVOKE ALL ON FUNCTION vendor_due_tenants(INTEGER) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION vendor_due_tenants(INTEGER) TO PUBLIC;

COMMENT ON TABLE vendors IS 'Tenant-isolated third-party inventory and risk lifecycle register.';
COMMENT ON TABLE vendor_events IS 'Append-only vendor and related-record lifecycle history.';
COMMENT ON FUNCTION vendor_due_tenants(INTEGER) IS 'Returns tenant UUIDs with vendor assessments due within 30 days; vendor content remains protected by FORCE RLS.';
