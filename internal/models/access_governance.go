package models

import (
	"encoding/json"
	"time"
)

type AccessReviewCampaign struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	ReviewerID     string    `json:"reviewer_id"`
	CreatedBy      string    `json:"created_by"`
	Status         string    `json:"status"`
	DueAt          time.Time `json:"due_at"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
}

type AccessReviewCampaignInput struct {
	Name       string    `json:"name"`
	ReviewerID string    `json:"reviewer_id"`
	SubjectIDs []string  `json:"subject_ids"`
	DueAt      time.Time `json:"due_at"`
	Reason     string    `json:"reason"`
}

type AccessReviewItem struct {
	ID             string                `json:"id"`
	CampaignID     string                `json:"campaign_id"`
	SubjectID      string                `json:"subject_id"`
	Kind           string                `json:"kind"`
	ResourceID     string                `json:"resource_id"`
	Snapshot       json.RawMessage       `json:"snapshot"`
	SnapshotSHA256 string                `json:"snapshot_sha256"`
	Decision       *AccessReviewDecision `json:"decision,omitempty"`
}

type AccessReviewDecision struct {
	ID             string    `json:"id"`
	ItemID         string    `json:"item_id"`
	ReviewerID     string    `json:"reviewer_id"`
	Decision       string    `json:"decision"`
	Reason         string    `json:"reason"`
	RequestID      string    `json:"request_id"`
	SnapshotSHA256 string    `json:"snapshot_sha256"`
	DecidedAt      time.Time `json:"decided_at"`
}

type AccessReviewDecisionInput struct {
	Decision       string `json:"decision"`
	Reason         string `json:"reason"`
	RequestID      string `json:"request_id"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
}

type AccessGovernanceTransitionInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

type AccessSoDRule struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	RoleAID        string    `json:"role_a_id"`
	RoleBID        string    `json:"role_b_id"`
	Enabled        bool      `json:"enabled"`
	Version        int64     `json:"version"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

type AccessSoDRuleInput struct {
	Name    string `json:"name"`
	RoleAID string `json:"role_a_id"`
	RoleBID string `json:"role_b_id"`
	Reason  string `json:"reason"`
}

type AccessSoDException struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	RuleID         string    `json:"rule_id"`
	SubjectID      string    `json:"subject_id"`
	RequestedBy    string    `json:"requested_by"`
	ApprovedBy     *string   `json:"approved_by,omitempty"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason"`
	ValidFrom      time.Time `json:"valid_from"`
	ExpiresAt      time.Time `json:"expires_at"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
}

type AccessSoDExceptionInput struct {
	SubjectID string    `json:"subject_id"`
	ValidFrom time.Time `json:"valid_from"`
	ExpiresAt time.Time `json:"expires_at"`
	Reason    string    `json:"reason"`
}

type AccessGovernanceEvent struct {
	ID        string          `json:"id"`
	EntityID  string          `json:"entity_id"`
	EventType string          `json:"event_type"`
	ActorID   string          `json:"actor_id"`
	Reason    string          `json:"reason"`
	Details   json.RawMessage `json:"details"`
	CreatedAt time.Time       `json:"created_at"`
}

type AccessSoDViolation struct {
	ID         string    `json:"id"`
	RuleID     string    `json:"rule_id"`
	SubjectID  string    `json:"subject_id"`
	DetectedAt time.Time `json:"detected_at"`
}
