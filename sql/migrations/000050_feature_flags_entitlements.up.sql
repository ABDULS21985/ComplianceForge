-- Migration 050: versioned capability catalogue, tenant feature flags, and
-- audited entitlement administration.

CREATE TABLE product_capabilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    capability_key VARCHAR(100) NOT NULL UNIQUE
        CHECK (capability_key ~ '^[a-z][a-z0-9_.-]{1,99}$'),
    display_name VARCHAR(160) NOT NULL
        CHECK (display_name = btrim(display_name) AND length(display_name) BETWEEN 2 AND 160),
    description TEXT NOT NULL
        CHECK (description = btrim(description) AND length(description) BETWEEN 10 AND 2000),
    owner_team VARCHAR(100) NOT NULL
        CHECK (owner_team = btrim(owner_team) AND length(owner_team) BETWEEN 2 AND 100),
    maturity VARCHAR(32) NOT NULL DEFAULT 'general_availability'
        CHECK (maturity IN ('experimental','beta','general_availability','deprecated')),
    minimum_tier org_tier NOT NULL DEFAULT 'starter',
    required_plan_feature VARCHAR(100)
        CHECK (required_plan_feature IS NULL OR required_plan_feature ~ '^[a-z][a-z0-9_.-]{1,99}$'),
    prerequisites TEXT[] NOT NULL DEFAULT '{}',
    default_enabled BOOLEAN NOT NULL DEFAULT true,
    kill_switch BOOLEAN NOT NULL DEFAULT false,
    rollout_basis_points INTEGER NOT NULL DEFAULT 10000
        CHECK (rollout_basis_points BETWEEN 0 AND 10000),
    is_active BOOLEAN NOT NULL DEFAULT true,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_capability_not_own_prerequisite
        CHECK (NOT capability_key = ANY(prerequisites))
);

CREATE INDEX idx_product_capabilities_active
    ON product_capabilities (is_active, capability_key) WHERE is_active;

CREATE TRIGGER trg_product_capabilities_updated_at
    BEFORE UPDATE ON product_capabilities
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE tenant_feature_flag_overrides (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    capability_key VARCHAR(100) NOT NULL
        REFERENCES product_capabilities(capability_key) ON UPDATE CASCADE ON DELETE RESTRICT,
    enabled BOOLEAN NOT NULL,
    rollout_basis_points INTEGER
        CHECK (rollout_basis_points IS NULL OR rollout_basis_points BETWEEN 0 AND 10000),
    variant JSONB NOT NULL DEFAULT '{}'
        CHECK (jsonb_typeof(variant) = 'object'),
    reason VARCHAR(1000) NOT NULL
        CHECK (reason = btrim(reason) AND length(reason) BETWEEN 3 AND 1000),
    starts_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by UUID NOT NULL,
    updated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, capability_key),
    CONSTRAINT fk_tenant_flag_creator FOREIGN KEY (organization_id, created_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_tenant_flag_updater FOREIGN KEY (organization_id, updated_by)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_tenant_flag_window CHECK (
        expires_at IS NULL OR starts_at IS NULL OR expires_at > starts_at
    )
);

CREATE INDEX idx_tenant_feature_flags_expiry
    ON tenant_feature_flag_overrides (organization_id, expires_at)
    WHERE expires_at IS NOT NULL;

CREATE TRIGGER trg_tenant_feature_flags_updated_at
    BEFORE UPDATE ON tenant_feature_flag_overrides
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE feature_flag_change_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    capability_key VARCHAR(100) NOT NULL
        REFERENCES product_capabilities(capability_key) ON UPDATE CASCADE ON DELETE RESTRICT,
    event_type VARCHAR(20) NOT NULL CHECK (event_type IN ('created','updated','reset')),
    actor_user_id UUID NOT NULL,
    override_version BIGINT NOT NULL CHECK (override_version > 0),
    reason VARCHAR(1000) NOT NULL
        CHECK (reason = btrim(reason) AND length(reason) BETWEEN 3 AND 1000),
    before_state JSONB CHECK (before_state IS NULL OR jsonb_typeof(before_state) = 'object'),
    after_state JSONB CHECK (after_state IS NULL OR jsonb_typeof(after_state) = 'object'),
    request_id VARCHAR(160),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_feature_flag_event_actor FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES users(organization_id, id) ON DELETE RESTRICT
);

CREATE INDEX idx_feature_flag_events_tenant_key
    ON feature_flag_change_events (organization_id, capability_key, created_at DESC, id DESC);

CREATE FUNCTION prevent_feature_flag_event_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'feature flag change events are append-only: % is not permitted', TG_OP
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER trg_feature_flag_events_immutable
    BEFORE UPDATE OR DELETE ON feature_flag_change_events
    FOR EACH ROW EXECUTE FUNCTION prevent_feature_flag_event_mutation();

ALTER TABLE tenant_feature_flag_overrides ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_feature_flag_overrides FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_feature_flags_select ON tenant_feature_flag_overrides FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY tenant_feature_flags_insert ON tenant_feature_flag_overrides FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY tenant_feature_flags_update ON tenant_feature_flag_overrides FOR UPDATE
    USING (organization_id = get_current_tenant())
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY tenant_feature_flags_delete ON tenant_feature_flag_overrides FOR DELETE
    USING (organization_id = get_current_tenant());

ALTER TABLE feature_flag_change_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE feature_flag_change_events FORCE ROW LEVEL SECURITY;
CREATE POLICY feature_flag_events_select ON feature_flag_change_events FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY feature_flag_events_insert ON feature_flag_change_events FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());

INSERT INTO product_capabilities
    (capability_key,display_name,description,owner_team,maturity,minimum_tier,required_plan_feature,prerequisites)
VALUES
    ('control_tracking','Control tracking','Track control implementation, ownership, maturity, and evidence across adopted frameworks.','Compliance','general_availability','starter','control_tracking','{}'),
    ('risk_register','Risk register','Manage inherent and residual enterprise risks, treatments, indicators, and appetite.','Risk','general_availability','starter','risk_register','{}'),
    ('policy_management','Policy management','Draft, review, approve, publish, assign, and acknowledge governed policies.','Policy','general_availability','starter','policy_management','{}'),
    ('basic_reporting','Basic reporting','Generate standard operational compliance and risk reports for tenant users.','Reporting','general_availability','starter','basic_reporting','{}'),
    ('asset_inventory','Asset inventory','Track governed information, technology, service, facility, and people assets across their lifecycle.','Asset Management','general_availability','starter',NULL,'{}'),
    ('audit_workspace','Audit workspace','Plan and execute audit engagements, findings, evidence requests, and reviews.','Audit','general_availability','starter','audit_workspace','{control_tracking}'),
    ('vendor_management','Vendor management','Manage third parties, assessments, contracts, subprocessors, and lifecycle risk.','Third Party Risk','general_availability','professional','vendor_management','{control_tracking}'),
    ('incident_management','Incident management','Manage security and privacy incidents, breach assessments, assignments, and timelines.','Incident Response','general_availability','professional','incident_management','{}'),
    ('dsr_management','Data subject requests','Coordinate verified data-subject requests, fulfilment, evidence, and statutory deadlines.','Privacy','beta','professional','dsr_management','{incident_management}'),
    ('sso','Enterprise single sign-on','Configure tenant-bound OIDC and SAML single sign-on with governed identity routing.','Identity','beta','professional','sso','{}'),
    ('api_access','Automation API access','Use scoped API credentials and supported automation endpoints for integrations.','Platform','general_availability','professional','api_access','{}'),
    ('advanced_reporting','Advanced reporting','Use advanced report templates, dashboards, scheduled delivery, and analytics.','Reporting','beta','professional','advanced_reporting','{basic_reporting}'),
    ('continuous_monitoring','Continuous monitoring','Run automated evidence collection and control drift monitoring on a schedule.','Compliance Automation','beta','professional','continuous_monitoring','{control_tracking}'),
    ('ai_scoring','AI-assisted scoring','Use governed AI assistance for risk scoring and remediation recommendations.','Applied AI','beta','professional','ai_scoring','{risk_register}'),
    ('priority_support','Priority support','Receive priority support routing and enhanced response targets.','Customer Operations','general_availability','professional','priority_support','{}'),
    ('custom_branding','Custom branding','Apply tenant logos, colours, domains, and templates to supported experiences.','Experience','beta','enterprise','custom_branding','{}'),
    ('multi_language','Multi-language experience','Use supported locale-aware product translations and tenant language preferences.','Experience','beta','enterprise','multi_language','{}'),
    ('dedicated_csm','Dedicated customer success','Receive a named customer-success owner and enterprise service reviews.','Customer Operations','general_availability','enterprise','dedicated_csm','{priority_support}'),
    ('abac','Attribute-based access control','Define contextual access policies in addition to persisted role permissions.','Identity','beta','enterprise','abac','{}'),
    ('field_level_security','Field-level security','Mask or restrict sensitive fields according to governed access policies.','Identity','beta','enterprise','field_level_security','{abac}'),
    ('custom_integrations','Custom integrations','Configure tenant-specific enterprise connectors and custom integration mappings.','Platform','beta','enterprise','custom_integrations','{api_access}'),
    ('sla_guarantee','Enterprise SLA','Apply contracted enterprise availability and support service-level commitments.','Reliability','general_availability','enterprise','sla_guarantee','{priority_support}'),
    ('on_premise_option','Private deployment option','Support a separately governed private or customer-managed deployment model.','Platform','experimental','enterprise','on_premise_option','{}');

COMMENT ON TABLE product_capabilities IS
    'Versioned global product capability catalogue and operational feature defaults. Definitions are deployment-managed; tenant APIs are read-only.';
COMMENT ON COLUMN product_capabilities.rollout_basis_points IS
    'Deterministic tenant rollout from 0 to 10000 basis points; 10000 is 100 percent.';
COMMENT ON COLUMN product_capabilities.kill_switch IS
    'Global fail-closed operational switch that no tenant override or subscription may bypass.';
COMMENT ON TABLE tenant_feature_flag_overrides IS
    'Audited, time-bounded tenant overrides. Subscription entitlement, global kill switch, prerequisites, and rollout still fail closed.';
COMMENT ON TABLE feature_flag_change_events IS
    'Immutable tenant-visible history for every feature-flag override mutation.';
