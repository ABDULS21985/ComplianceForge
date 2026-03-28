-- Migration 024: Attribute-Based Access Control (ABAC)
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - ABAC complements the existing RBAC system (migration 004) by enabling
--     fine-grained, context-aware access decisions based on subject attributes
--     (role, department, clearance), resource attributes (classification, owner),
--     and environment attributes (time, IP, MFA status).
--   - access_policies define rules with subject_conditions (who), resource_type +
--     resource_conditions (what), actions (operations), and environment_conditions
--     (context). Priority + effect (allow/deny) enable layered policy evaluation.
--   - access_policy_assignments bind policies to users, roles, groups, or all_users.
--     This decouples "what the policy says" from "who it applies to".
--   - access_audit_log is append-only (immutable) for compliance auditability.
--     Every access decision is logged with full attribute context and the policy
--     that determined the outcome, plus evaluation timing for performance monitoring.
--   - field_level_permissions extend ABAC to column-level visibility: fields can be
--     visible, masked (e.g., "***42.00"), or hidden entirely — critical for GDPR
--     and financial data segregation.
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: access_policies
-- ============================================================================

CREATE TABLE access_policies (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                    VARCHAR(200) NOT NULL,
    description             TEXT,
    priority                INT NOT NULL DEFAULT 100,
    effect                  VARCHAR(10) NOT NULL
                            CHECK (effect IN ('allow', 'deny')),
    is_active               BOOLEAN NOT NULL DEFAULT true,

    -- Subject: who does this policy apply to?
    -- e.g., {"roles": ["org_admin", "ciso"], "departments": ["IT Security"]}
    subject_conditions      JSONB NOT NULL,

    -- Resource: what entity type and conditions?
    resource_type           VARCHAR(100) NOT NULL,
    -- e.g., {"classification": {"$nin": ["confidential", "restricted"]}, "owner_id": "$subject.id"}
    resource_conditions     JSONB,

    -- Actions: what operations are permitted/denied?
    actions                 TEXT[] NOT NULL,

    -- Environment: contextual conditions
    -- e.g., {"mfa_verified": true, "time_range": {"start": "09:00", "end": "18:00"}, "ip_ranges": ["10.0.0.0/8"]}
    environment_conditions  JSONB,

    -- Validity window (optional)
    valid_from              TIMESTAMPTZ,
    valid_until             TIMESTAMPTZ,

    created_by              UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_access_policies_org ON access_policies(organization_id);
CREATE INDEX idx_access_policies_org_active ON access_policies(organization_id, is_active)
    WHERE is_active = true;
CREATE INDEX idx_access_policies_resource_type ON access_policies(organization_id, resource_type);
CREATE INDEX idx_access_policies_effect ON access_policies(organization_id, effect);
CREATE INDEX idx_access_policies_priority ON access_policies(organization_id, priority);
CREATE INDEX idx_access_policies_created_by ON access_policies(created_by)
    WHERE created_by IS NOT NULL;

-- Trigger
CREATE TRIGGER trg_access_policies_updated_at
    BEFORE UPDATE ON access_policies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE access_policies IS 'ABAC policy definitions. Each policy specifies subject conditions (who), resource type and conditions (what), actions (operations), environment conditions (context), and an effect (allow/deny). Policies are evaluated by priority — lower numbers take precedence.';
COMMENT ON COLUMN access_policies.subject_conditions IS 'JSONB conditions on subject attributes: {"roles": ["org_admin"], "departments": ["Security"], "clearance_level": {"$gte": 3}}';
COMMENT ON COLUMN access_policies.resource_conditions IS 'JSONB conditions on resource attributes: {"classification": {"$nin": ["confidential"]}, "owner_id": "$subject.id"}. Supports $eq, $ne, $in, $nin, $gt, $gte, $lt, $lte operators.';
COMMENT ON COLUMN access_policies.environment_conditions IS 'JSONB conditions on environment: {"mfa_verified": true, "time_range": {"start": "09:00", "end": "18:00"}, "ip_ranges": ["10.0.0.0/8"]}';
COMMENT ON COLUMN access_policies.priority IS 'Lower numbers = higher priority. Deny policies typically use lower priority numbers to override allow policies.';

-- ============================================================================
-- TABLE: access_policy_assignments
-- ============================================================================

CREATE TABLE access_policy_assignments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    access_policy_id    UUID NOT NULL REFERENCES access_policies(id) ON DELETE CASCADE,
    assignee_type       VARCHAR(20) NOT NULL
                        CHECK (assignee_type IN ('user', 'role', 'group', 'all_users')),
    assignee_id         UUID,               -- NULL when assignee_type = 'all_users'
    valid_from          TIMESTAMPTZ,
    valid_until         TIMESTAMPTZ,
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_policy_assignments_org ON access_policy_assignments(organization_id);
CREATE INDEX idx_policy_assignments_policy ON access_policy_assignments(access_policy_id);
CREATE INDEX idx_policy_assignments_assignee ON access_policy_assignments(assignee_type, assignee_id);
CREATE INDEX idx_policy_assignments_org_type ON access_policy_assignments(organization_id, assignee_type);
CREATE INDEX idx_policy_assignments_created_by ON access_policy_assignments(created_by)
    WHERE created_by IS NOT NULL;

COMMENT ON TABLE access_policy_assignments IS 'Binds ABAC policies to assignees (users, roles, groups, or all users). Decouples policy definition from policy application, enabling reuse and time-bounded assignments.';
COMMENT ON COLUMN access_policy_assignments.assignee_id IS 'UUID of the user, role, or group. NULL when assignee_type is all_users.';
COMMENT ON COLUMN access_policy_assignments.valid_from IS 'Optional start time for this assignment. Supports time-limited access for auditors and contractors.';

-- ============================================================================
-- TABLE: access_audit_log (immutable — no UPDATE or DELETE)
-- ============================================================================

CREATE TABLE access_audit_log (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id                 UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action                  VARCHAR(50) NOT NULL,
    resource_type           VARCHAR(100) NOT NULL,
    resource_id             UUID,
    decision                VARCHAR(10) NOT NULL
                            CHECK (decision IN ('allow', 'deny')),
    matched_policy_id       UUID REFERENCES access_policies(id) ON DELETE SET NULL,
    evaluation_time_us      INT,                    -- microseconds
    subject_attributes      JSONB,
    resource_attributes     JSONB,
    environment_attributes  JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_access_audit_org_user_time ON access_audit_log(organization_id, user_id, created_at DESC);
CREATE INDEX idx_access_audit_org_resource ON access_audit_log(organization_id, resource_type, resource_id);
CREATE INDEX idx_access_audit_decision ON access_audit_log(organization_id, decision);
CREATE INDEX idx_access_audit_policy ON access_audit_log(matched_policy_id)
    WHERE matched_policy_id IS NOT NULL;
CREATE INDEX idx_access_audit_created ON access_audit_log(created_at DESC);

-- Immutability: prevent UPDATE and DELETE via a trigger.
CREATE OR REPLACE FUNCTION prevent_access_audit_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'access_audit_log is immutable. UPDATE and DELETE operations are not permitted.';
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_access_audit_no_update
    BEFORE UPDATE ON access_audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_access_audit_modification();

CREATE TRIGGER trg_access_audit_no_delete
    BEFORE DELETE ON access_audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_access_audit_modification();

COMMENT ON TABLE access_audit_log IS 'Immutable audit trail of every ABAC access decision. Records the full attribute context (subject, resource, environment) and the policy that determined the outcome. Cannot be updated or deleted — enforced by trigger.';
COMMENT ON COLUMN access_audit_log.evaluation_time_us IS 'Time in microseconds to evaluate the ABAC policy chain. Used for performance monitoring and optimization.';
COMMENT ON COLUMN access_audit_log.matched_policy_id IS 'The policy that determined the final allow/deny decision. NULL if no policy matched (implicit deny).';

-- ============================================================================
-- TABLE: field_level_permissions
-- ============================================================================

CREATE TABLE field_level_permissions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    access_policy_id    UUID NOT NULL REFERENCES access_policies(id) ON DELETE CASCADE,
    resource_type       VARCHAR(100) NOT NULL,
    field_name          VARCHAR(100) NOT NULL,
    permission          VARCHAR(10) NOT NULL DEFAULT 'visible'
                        CHECK (permission IN ('visible', 'masked', 'hidden')),
    mask_pattern        VARCHAR(50),            -- e.g., '***{last4}' or '€ ***.**'
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_field_perms_org ON field_level_permissions(organization_id);
CREATE INDEX idx_field_perms_policy ON field_level_permissions(access_policy_id);
CREATE INDEX idx_field_perms_resource_field ON field_level_permissions(organization_id, resource_type, field_name);

COMMENT ON TABLE field_level_permissions IS 'Extends ABAC to column-level visibility. Fields can be fully visible, masked (partial redaction with a pattern), or completely hidden. Linked to an access_policy to inherit subject/environment conditions.';
COMMENT ON COLUMN field_level_permissions.mask_pattern IS 'Pattern for masked fields: "***{last4}" shows last 4 chars, "EUR ***.**" for financial values. NULL when permission is visible or hidden.';
COMMENT ON COLUMN field_level_permissions.field_name IS 'Dot-notation field path, e.g., "financial_impact_eur" or "contact.email". Maps to API response fields.';

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- access_policies
ALTER TABLE access_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE access_policies FORCE ROW LEVEL SECURITY;

CREATE POLICY access_policies_tenant_select ON access_policies FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY access_policies_tenant_insert ON access_policies FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY access_policies_tenant_update ON access_policies FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY access_policies_tenant_delete ON access_policies FOR DELETE
    USING (organization_id = get_current_tenant());

-- access_policy_assignments
ALTER TABLE access_policy_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE access_policy_assignments FORCE ROW LEVEL SECURITY;

CREATE POLICY policy_assignments_tenant_select ON access_policy_assignments FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY policy_assignments_tenant_insert ON access_policy_assignments FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY policy_assignments_tenant_update ON access_policy_assignments FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY policy_assignments_tenant_delete ON access_policy_assignments FOR DELETE
    USING (organization_id = get_current_tenant());

-- access_audit_log
ALTER TABLE access_audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE access_audit_log FORCE ROW LEVEL SECURITY;

CREATE POLICY access_audit_tenant_select ON access_audit_log FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY access_audit_tenant_insert ON access_audit_log FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
-- No UPDATE/DELETE RLS policies needed — immutability trigger prevents those operations.
-- However, we still define them for defense-in-depth (RLS denies before trigger fires).
CREATE POLICY access_audit_tenant_update ON access_audit_log FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY access_audit_tenant_delete ON access_audit_log FOR DELETE
    USING (organization_id = get_current_tenant());

-- field_level_permissions
ALTER TABLE field_level_permissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE field_level_permissions FORCE ROW LEVEL SECURITY;

CREATE POLICY field_perms_tenant_select ON field_level_permissions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY field_perms_tenant_insert ON field_level_permissions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY field_perms_tenant_update ON field_level_permissions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY field_perms_tenant_delete ON field_level_permissions FOR DELETE
    USING (organization_id = get_current_tenant());
