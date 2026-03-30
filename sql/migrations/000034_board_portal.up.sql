-- Migration 034: Board Reporting Portal
-- ComplianceForge GRC Platform
--
-- Design decisions:
--   - board_members represent individuals with governance oversight roles. They may
--     or may not have a user account (user_id nullable for external board members).
--     Portal access is token-based with expiry for secure external access.
--   - board_meetings track scheduled and completed governance meetings with agenda,
--     board pack documents, minutes, and attendance tracking.
--   - board_decisions record formal decisions made during meetings with voting
--     tallies, conditions, linked entities, and action tracking.
--   - board_reports are generated compliance/risk/governance reports for board
--     consumption, optionally linked to a specific meeting.
--   - Refs auto-generated: BMT-YYYY-NNNN (meetings), BDC-YYYY-NNNN (decisions).
--   - All tables are tenant-isolated via RLS on organization_id.

-- ============================================================================
-- TABLE: board_members
-- ============================================================================

CREATE TABLE board_members (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id                     UUID REFERENCES users(id) ON DELETE SET NULL,
    name                        VARCHAR(300) NOT NULL,
    title                       VARCHAR(200),
    email                       VARCHAR(300) NOT NULL,
    member_type                 VARCHAR(30) NOT NULL
                                CHECK (member_type IN (
                                    'chairperson', 'vice_chairperson', 'executive_director',
                                    'non_executive_director', 'independent_director', 'observer'
                                )),
    committees                  TEXT[],
    is_active                   BOOLEAN NOT NULL DEFAULT true,
    portal_access_enabled       BOOLEAN NOT NULL DEFAULT false,
    portal_access_token_hash    VARCHAR(128),
    portal_access_expires_at    TIMESTAMPTZ,
    last_portal_access_at       TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_board_members_org ON board_members(organization_id);
CREATE INDEX idx_board_members_user ON board_members(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_board_members_org_active ON board_members(organization_id, is_active) WHERE is_active = true;
CREATE INDEX idx_board_members_org_type ON board_members(organization_id, member_type);
CREATE INDEX idx_board_members_email ON board_members(organization_id, email);
CREATE INDEX idx_board_members_portal ON board_members(portal_access_enabled) WHERE portal_access_enabled = true;
CREATE INDEX idx_board_members_token ON board_members(portal_access_token_hash) WHERE portal_access_token_hash IS NOT NULL;
CREATE INDEX idx_board_members_committees ON board_members USING GIN (committees);

-- Trigger
CREATE TRIGGER trg_board_members_updated_at
    BEFORE UPDATE ON board_members
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE board_members IS 'Board and governance committee members. May or may not have platform user accounts (user_id nullable for external directors). Supports token-based portal access with expiry.';
COMMENT ON COLUMN board_members.portal_access_token_hash IS 'SHA-256 hash of the board portal access token. Token itself is never stored.';
COMMENT ON COLUMN board_members.committees IS 'Committees the member belongs to: ["audit", "risk", "compensation", "governance"].';

-- ============================================================================
-- TABLE: board_meetings
-- ============================================================================

CREATE TABLE board_meetings (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    meeting_ref                 VARCHAR(20) NOT NULL,
    title                       VARCHAR(300) NOT NULL,
    meeting_type                VARCHAR(30) NOT NULL
                                CHECK (meeting_type IN (
                                    'regular_board', 'special_board', 'audit_committee',
                                    'risk_committee', 'governance_committee', 'annual_general',
                                    'extraordinary_general'
                                )),
    date                        DATE NOT NULL,
    time                        TIME,
    location                    VARCHAR(500),
    status                      VARCHAR(20) NOT NULL DEFAULT 'scheduled'
                                CHECK (status IN ('scheduled', 'in_progress', 'completed', 'cancelled', 'postponed')),
    agenda_items                JSONB,
    board_pack_document_path    TEXT,
    board_pack_generated_at     TIMESTAMPTZ,
    minutes_document_path       TEXT,
    minutes_approved_at         TIMESTAMPTZ,
    minutes_approved_by         UUID REFERENCES users(id) ON DELETE SET NULL,
    attendees                   UUID[],
    apologies                   UUID[],
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_board_meetings_org_ref UNIQUE (organization_id, meeting_ref)
);

-- Indexes
CREATE INDEX idx_board_meetings_org ON board_meetings(organization_id);
CREATE INDEX idx_board_meetings_org_type ON board_meetings(organization_id, meeting_type);
CREATE INDEX idx_board_meetings_org_status ON board_meetings(organization_id, status);
CREATE INDEX idx_board_meetings_date ON board_meetings(date DESC);
CREATE INDEX idx_board_meetings_org_date ON board_meetings(organization_id, date DESC);
CREATE INDEX idx_board_meetings_minutes_by ON board_meetings(minutes_approved_by) WHERE minutes_approved_by IS NOT NULL;
CREATE INDEX idx_board_meetings_attendees ON board_meetings USING GIN (attendees);
CREATE INDEX idx_board_meetings_agenda ON board_meetings USING GIN (agenda_items);

-- Trigger
CREATE TRIGGER trg_board_meetings_updated_at
    BEFORE UPDATE ON board_meetings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE board_meetings IS 'Board and committee meeting records. Tracks scheduling, agenda, board pack documents, minutes, attendance, and approval status.';
COMMENT ON COLUMN board_meetings.meeting_ref IS 'Auto-generated reference per org per year: BMT-YYYY-NNNN.';
COMMENT ON COLUMN board_meetings.agenda_items IS 'JSONB agenda: [{"item_number": 1, "title": "Opening", "presenter": "...", "duration_minutes": 5, "type": "procedural"}, ...]';
COMMENT ON COLUMN board_meetings.attendees IS 'Array of board_member UUIDs who attended the meeting.';
COMMENT ON COLUMN board_meetings.apologies IS 'Array of board_member UUIDs who sent apologies/regrets.';

-- ============================================================================
-- TABLE: board_decisions
-- ============================================================================

CREATE TABLE board_decisions (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id             UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    meeting_id                  UUID NOT NULL REFERENCES board_meetings(id) ON DELETE CASCADE,
    decision_ref                VARCHAR(20) NOT NULL,
    title                       VARCHAR(300) NOT NULL,
    description                 TEXT,
    decision_type               VARCHAR(30) NOT NULL
                                CHECK (decision_type IN (
                                    'risk_acceptance', 'policy_approval', 'budget_allocation',
                                    'strategy_direction', 'compliance_action', 'incident_response',
                                    'vendor_approval', 'exception_approval', 'audit_response',
                                    'general'
                                )),
    decision                    VARCHAR(25) NOT NULL
                                CHECK (decision IN ('approved', 'rejected', 'deferred', 'conditional_approval')),
    conditions                  TEXT,
    vote_for                    INT NOT NULL DEFAULT 0,
    vote_against                INT NOT NULL DEFAULT 0,
    vote_abstain                INT NOT NULL DEFAULT 0,
    rationale                   TEXT,
    linked_entity_type          VARCHAR(50),
    linked_entity_id            UUID,
    action_required             BOOLEAN NOT NULL DEFAULT false,
    action_description          TEXT,
    action_owner_user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    action_due_date             DATE,
    action_status               VARCHAR(20)
                                CHECK (action_status IS NULL OR action_status IN ('not_started', 'in_progress', 'completed', 'overdue')),
    action_completed_at         TIMESTAMPTZ,
    decided_at                  TIMESTAMPTZ,
    tags                        TEXT[],
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_board_decisions_org_ref UNIQUE (organization_id, decision_ref)
);

-- Indexes
CREATE INDEX idx_board_decisions_org ON board_decisions(organization_id);
CREATE INDEX idx_board_decisions_meeting ON board_decisions(meeting_id);
CREATE INDEX idx_board_decisions_org_type ON board_decisions(organization_id, decision_type);
CREATE INDEX idx_board_decisions_org_decision ON board_decisions(organization_id, decision);
CREATE INDEX idx_board_decisions_action_owner ON board_decisions(action_owner_user_id) WHERE action_owner_user_id IS NOT NULL;
CREATE INDEX idx_board_decisions_action_status ON board_decisions(organization_id, action_status) WHERE action_status IS NOT NULL;
CREATE INDEX idx_board_decisions_action_due ON board_decisions(action_due_date) WHERE action_due_date IS NOT NULL;
CREATE INDEX idx_board_decisions_linked ON board_decisions(linked_entity_type, linked_entity_id) WHERE linked_entity_id IS NOT NULL;
CREATE INDEX idx_board_decisions_decided ON board_decisions(decided_at DESC) WHERE decided_at IS NOT NULL;
CREATE INDEX idx_board_decisions_tags ON board_decisions USING GIN (tags);

-- Trigger
CREATE TRIGGER trg_board_decisions_updated_at
    BEFORE UPDATE ON board_decisions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE board_decisions IS 'Formal decisions made during board/committee meetings. Tracks voting, conditions, linked entities (risks, policies, exceptions), and follow-up action items with ownership and due dates.';
COMMENT ON COLUMN board_decisions.decision_ref IS 'Auto-generated reference per org per year: BDC-YYYY-NNNN.';
COMMENT ON COLUMN board_decisions.linked_entity_type IS 'Type of linked entity: "risk", "policy", "exception", "vendor", "incident", etc.';
COMMENT ON COLUMN board_decisions.linked_entity_id IS 'UUID of the linked entity for cross-referencing decisions to specific GRC records.';

-- ============================================================================
-- TABLE: board_reports
-- ============================================================================

CREATE TABLE board_reports (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    meeting_id              UUID REFERENCES board_meetings(id) ON DELETE SET NULL,
    report_type             VARCHAR(30) NOT NULL
                            CHECK (report_type IN (
                                'compliance_summary', 'risk_dashboard', 'incident_summary',
                                'vendor_risk', 'audit_status', 'policy_status',
                                'kpi_scorecard', 'regulatory_update', 'executive_summary'
                            )),
    title                   VARCHAR(300) NOT NULL,
    period_start            DATE NOT NULL,
    period_end              DATE NOT NULL,
    file_path               TEXT NOT NULL,
    file_format             VARCHAR(10) NOT NULL
                            CHECK (file_format IN ('pdf', 'xlsx')),
    generated_by            UUID REFERENCES users(id) ON DELETE SET NULL,
    generated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    classification          VARCHAR(50) NOT NULL DEFAULT 'board_confidential',
    page_count              INT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_board_reports_org ON board_reports(organization_id);
CREATE INDEX idx_board_reports_meeting ON board_reports(meeting_id) WHERE meeting_id IS NOT NULL;
CREATE INDEX idx_board_reports_org_type ON board_reports(organization_id, report_type);
CREATE INDEX idx_board_reports_period ON board_reports(organization_id, period_start, period_end);
CREATE INDEX idx_board_reports_generated ON board_reports(generated_at DESC);
CREATE INDEX idx_board_reports_generated_by ON board_reports(generated_by) WHERE generated_by IS NOT NULL;
CREATE INDEX idx_board_reports_classification ON board_reports(organization_id, classification);
CREATE INDEX idx_board_reports_format ON board_reports(file_format);

COMMENT ON TABLE board_reports IS 'Generated compliance/risk/governance reports for board consumption. Optionally linked to a specific meeting. Classified as board_confidential by default.';
COMMENT ON COLUMN board_reports.classification IS 'Document classification level: board_confidential, restricted, internal, etc.';
COMMENT ON COLUMN board_reports.report_type IS 'Report category: compliance_summary, risk_dashboard, incident_summary, vendor_risk, audit_status, policy_status, kpi_scorecard, regulatory_update, executive_summary.';

-- ============================================================================
-- TRIGGER FUNCTIONS
-- ============================================================================

-- Auto-generate board meeting reference: BMT-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_board_meeting_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.meeting_ref IS NULL OR NEW.meeting_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN meeting_ref ~ ('^BMT-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(meeting_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM board_meetings
        WHERE organization_id = NEW.organization_id;

        NEW.meeting_ref := 'BMT-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_board_meetings_generate_ref
    BEFORE INSERT ON board_meetings
    FOR EACH ROW EXECUTE FUNCTION generate_board_meeting_ref();

-- Auto-generate board decision reference: BDC-YYYY-NNNN
CREATE OR REPLACE FUNCTION generate_board_decision_ref()
RETURNS TRIGGER AS $$
DECLARE
    current_year TEXT;
    next_num INT;
BEGIN
    IF NEW.decision_ref IS NULL OR NEW.decision_ref = '' THEN
        current_year := TO_CHAR(NOW(), 'YYYY');

        SELECT COALESCE(MAX(
            CASE
                WHEN decision_ref ~ ('^BDC-' || current_year || '-[0-9]{4}$')
                THEN SUBSTRING(decision_ref FROM '[0-9]{4}$')::INT
                ELSE 0
            END
        ), 0) + 1
        INTO next_num
        FROM board_decisions
        WHERE organization_id = NEW.organization_id;

        NEW.decision_ref := 'BDC-' || current_year || '-' || LPAD(next_num::TEXT, 4, '0');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_board_decisions_generate_ref
    BEFORE INSERT ON board_decisions
    FOR EACH ROW EXECUTE FUNCTION generate_board_decision_ref();

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- board_members
ALTER TABLE board_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE board_members FORCE ROW LEVEL SECURITY;

CREATE POLICY board_members_tenant_select ON board_members FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY board_members_tenant_insert ON board_members FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY board_members_tenant_update ON board_members FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY board_members_tenant_delete ON board_members FOR DELETE
    USING (organization_id = get_current_tenant());

-- board_meetings
ALTER TABLE board_meetings ENABLE ROW LEVEL SECURITY;
ALTER TABLE board_meetings FORCE ROW LEVEL SECURITY;

CREATE POLICY board_meetings_tenant_select ON board_meetings FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY board_meetings_tenant_insert ON board_meetings FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY board_meetings_tenant_update ON board_meetings FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY board_meetings_tenant_delete ON board_meetings FOR DELETE
    USING (organization_id = get_current_tenant());

-- board_decisions
ALTER TABLE board_decisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE board_decisions FORCE ROW LEVEL SECURITY;

CREATE POLICY board_decisions_tenant_select ON board_decisions FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY board_decisions_tenant_insert ON board_decisions FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY board_decisions_tenant_update ON board_decisions FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY board_decisions_tenant_delete ON board_decisions FOR DELETE
    USING (organization_id = get_current_tenant());

-- board_reports
ALTER TABLE board_reports ENABLE ROW LEVEL SECURITY;
ALTER TABLE board_reports FORCE ROW LEVEL SECURITY;

CREATE POLICY board_reports_tenant_select ON board_reports FOR SELECT
    USING (organization_id = get_current_tenant());
CREATE POLICY board_reports_tenant_insert ON board_reports FOR INSERT
    WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY board_reports_tenant_update ON board_reports FOR UPDATE
    USING (organization_id = get_current_tenant()) WITH CHECK (organization_id = get_current_tenant());
CREATE POLICY board_reports_tenant_delete ON board_reports FOR DELETE
    USING (organization_id = get_current_tenant());
