package models

import (
	"encoding/json"
	"time"
)

const (
	PolicyStateDraft           = "draft"
	PolicyStateUnderReview     = "under_review"
	PolicyStatePendingApproval = "pending_approval"
	PolicyStateApproved        = "approved"
	PolicyStatePublished       = "published"
	PolicyStateArchived        = "archived"
	PolicyStateRetired         = "retired"
	PolicyStateSuperseded      = "superseded"
)

// Policy is the canonical policy register record from migration 000013.
// Content is held in immutable PolicyVersion records.
type Policy struct {
	TenantModel
	PolicyRef                  string          `json:"policy_ref"`
	Title                      string          `json:"title"`
	CategoryID                 *string         `json:"category_id,omitempty"`
	Status                     string          `json:"status"`
	Classification             string          `json:"classification"`
	OwnerUserID                *string         `json:"owner_user_id,omitempty"`
	AuthorUserID               *string         `json:"author_user_id,omitempty"`
	ApproverUserID             *string         `json:"approver_user_id,omitempty"`
	DepartmentID               *string         `json:"department_id,omitempty"`
	CurrentVersion             int             `json:"current_version"`
	CurrentVersionID           *string         `json:"current_version_id,omitempty"`
	ReviewFrequencyMonths      int             `json:"review_frequency_months"`
	LastReviewDate             *time.Time      `json:"last_review_date,omitempty"`
	NextReviewDate             *time.Time      `json:"next_review_date,omitempty"`
	ReviewStatus               string          `json:"review_status"`
	AppliesToAll               bool            `json:"applies_to_all"`
	ApplicableDepartments      []string        `json:"applicable_departments"`
	ApplicableRoles            []string        `json:"applicable_roles"`
	ApplicableLocations        []string        `json:"applicable_locations"`
	LinkedFrameworkIDs         []string        `json:"linked_framework_ids"`
	LinkedControlIDs           []string        `json:"linked_control_ids"`
	LinkedRiskIDs              []string        `json:"linked_risk_ids"`
	ParentPolicyID             *string         `json:"parent_policy_id,omitempty"`
	SupersedesPolicyID         *string         `json:"supersedes_policy_id,omitempty"`
	EffectiveDate              *time.Time      `json:"effective_date,omitempty"`
	ExpiryDate                 *time.Time      `json:"expiry_date,omitempty"`
	Tags                       []string        `json:"tags"`
	Priority                   *string         `json:"priority,omitempty"`
	IsMandatory                bool            `json:"is_mandatory"`
	RequiresAttestation        bool            `json:"requires_attestation"`
	AttestationFrequencyMonths int             `json:"attestation_frequency_months"`
	Metadata                   json.RawMessage `json:"metadata"`
	CurrentVersionRecord       *PolicyVersion  `json:"current_version_record,omitempty"`
}

type PolicyCreateInput struct {
	PolicyRef                  string             `json:"policy_ref,omitempty"`
	Title                      string             `json:"title"`
	CategoryID                 *string            `json:"category_id,omitempty"`
	Classification             string             `json:"classification,omitempty"`
	OwnerUserID                *string            `json:"owner_user_id,omitempty"`
	ApproverUserID             *string            `json:"approver_user_id,omitempty"`
	DepartmentID               *string            `json:"department_id,omitempty"`
	ReviewFrequencyMonths      int                `json:"review_frequency_months,omitempty"`
	AppliesToAll               *bool              `json:"applies_to_all,omitempty"`
	ApplicableDepartments      []string           `json:"applicable_departments,omitempty"`
	ApplicableRoles            []string           `json:"applicable_roles,omitempty"`
	ApplicableLocations        []string           `json:"applicable_locations,omitempty"`
	LinkedFrameworkIDs         []string           `json:"linked_framework_ids,omitempty"`
	LinkedControlIDs           []string           `json:"linked_control_ids,omitempty"`
	LinkedRiskIDs              []string           `json:"linked_risk_ids,omitempty"`
	ParentPolicyID             *string            `json:"parent_policy_id,omitempty"`
	SupersedesPolicyID         *string            `json:"supersedes_policy_id,omitempty"`
	ExpiryDate                 *time.Time         `json:"expiry_date,omitempty"`
	Tags                       []string           `json:"tags,omitempty"`
	Priority                   *string            `json:"priority,omitempty"`
	IsMandatory                *bool              `json:"is_mandatory,omitempty"`
	RequiresAttestation        *bool              `json:"requires_attestation,omitempty"`
	AttestationFrequencyMonths int                `json:"attestation_frequency_months,omitempty"`
	Metadata                   json.RawMessage    `json:"metadata,omitempty"`
	InitialVersion             PolicyVersionInput `json:"initial_version"`
}

type PolicyPatch struct {
	Title                      *string         `json:"title,omitempty"`
	CategoryID                 *string         `json:"category_id,omitempty"`
	Classification             *string         `json:"classification,omitempty"`
	OwnerUserID                *string         `json:"owner_user_id,omitempty"`
	ApproverUserID             *string         `json:"approver_user_id,omitempty"`
	DepartmentID               *string         `json:"department_id,omitempty"`
	ReviewFrequencyMonths      *int            `json:"review_frequency_months,omitempty"`
	AppliesToAll               *bool           `json:"applies_to_all,omitempty"`
	ApplicableDepartments      []string        `json:"applicable_departments,omitempty"`
	ApplicableRoles            []string        `json:"applicable_roles,omitempty"`
	ApplicableLocations        []string        `json:"applicable_locations,omitempty"`
	LinkedFrameworkIDs         []string        `json:"linked_framework_ids,omitempty"`
	LinkedControlIDs           []string        `json:"linked_control_ids,omitempty"`
	LinkedRiskIDs              []string        `json:"linked_risk_ids,omitempty"`
	ParentPolicyID             *string         `json:"parent_policy_id,omitempty"`
	SupersedesPolicyID         *string         `json:"supersedes_policy_id,omitempty"`
	ExpiryDate                 *time.Time      `json:"expiry_date,omitempty"`
	Tags                       []string        `json:"tags,omitempty"`
	Priority                   *string         `json:"priority,omitempty"`
	IsMandatory                *bool           `json:"is_mandatory,omitempty"`
	RequiresAttestation        *bool           `json:"requires_attestation,omitempty"`
	AttestationFrequencyMonths *int            `json:"attestation_frequency_months,omitempty"`
	Metadata                   json.RawMessage `json:"metadata,omitempty"`
}

type PolicyListFilter struct {
	PaginationRequest
	Status, Classification, CategoryID, OwnerUserID, Search string
	DueBefore                                               *time.Time
}

type PolicyCategory struct {
	BaseModel
	OrganizationID   *string `json:"organization_id,omitempty"`
	Name             string  `json:"name"`
	Code             string  `json:"code"`
	Description      *string `json:"description,omitempty"`
	ParentCategoryID *string `json:"parent_category_id,omitempty"`
	SortOrder        int     `json:"sort_order"`
	IsSystemDefault  bool    `json:"is_system_default"`
}

type PolicyVersion struct {
	BaseModel
	PolicyID          string          `json:"policy_id"`
	OrganizationID    string          `json:"organization_id"`
	VersionNumber     int             `json:"version_number"`
	VersionLabel      string          `json:"version_label"`
	Title             string          `json:"title"`
	ContentHTML       *string         `json:"content_html,omitempty"`
	ContentText       *string         `json:"content_text,omitempty"`
	Summary           *string         `json:"summary,omitempty"`
	ChangeDescription *string         `json:"change_description,omitempty"`
	ChangeType        *string         `json:"change_type,omitempty"`
	Language          string          `json:"language"`
	WordCount         *int            `json:"word_count,omitempty"`
	Status            string          `json:"status"`
	CreatedBy         *string         `json:"created_by,omitempty"`
	PublishedAt       *time.Time      `json:"published_at,omitempty"`
	PublishedBy       *string         `json:"published_by,omitempty"`
	FilePath          *string         `json:"file_path,omitempty"`
	FileHash          *string         `json:"file_hash,omitempty"`
	Metadata          json.RawMessage `json:"metadata"`
}

type PolicyVersionInput struct {
	VersionLabel      string          `json:"version_label,omitempty"`
	Title             string          `json:"title,omitempty"`
	ContentHTML       *string         `json:"content_html,omitempty"`
	ContentText       *string         `json:"content_text,omitempty"`
	Summary           *string         `json:"summary,omitempty"`
	ChangeDescription *string         `json:"change_description,omitempty"`
	ChangeType        *string         `json:"change_type,omitempty"`
	Language          string          `json:"language,omitempty"`
	FilePath          *string         `json:"file_path,omitempty"`
	FileHash          *string         `json:"file_hash,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
}

type PolicyApprovalWorkflow struct {
	BaseModel
	PolicyID        string               `json:"policy_id"`
	PolicyVersionID *string              `json:"policy_version_id,omitempty"`
	OrganizationID  string               `json:"organization_id"`
	WorkflowType    string               `json:"workflow_type"`
	Status          string               `json:"status"`
	InitiatedBy     *string              `json:"initiated_by,omitempty"`
	InitiatedAt     time.Time            `json:"initiated_at"`
	CompletedAt     *time.Time           `json:"completed_at,omitempty"`
	DueDate         *time.Time           `json:"due_date,omitempty"`
	CurrentStep     int                  `json:"current_step"`
	TotalSteps      int                  `json:"total_steps"`
	Comments        *string              `json:"comments,omitempty"`
	Metadata        json.RawMessage      `json:"metadata"`
	Steps           []PolicyApprovalStep `json:"steps"`
}

type PolicyApprovalStep struct {
	BaseModel
	WorkflowID       string     `json:"workflow_id"`
	OrganizationID   string     `json:"organization_id"`
	StepNumber       int        `json:"step_number"`
	ApproverUserID   *string    `json:"approver_user_id,omitempty"`
	ApproverRole     *string    `json:"approver_role,omitempty"`
	Status           string     `json:"status"`
	DecisionDate     *time.Time `json:"decision_date,omitempty"`
	Comments         *string    `json:"comments,omitempty"`
	DelegationUserID *string    `json:"delegation_user_id,omitempty"`
	DueDate          *time.Time `json:"due_date,omitempty"`
	ReminderSent     bool       `json:"reminder_sent"`
}

type PolicyApprovalStepInput struct {
	ApproverUserID *string    `json:"approver_user_id,omitempty"`
	ApproverRole   *string    `json:"approver_role,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
}

type PolicySubmitInput struct {
	WorkflowType string                    `json:"workflow_type,omitempty"`
	DueDate      *time.Time                `json:"due_date,omitempty"`
	Comments     *string                   `json:"comments,omitempty"`
	Approvers    []PolicyApprovalStepInput `json:"approvers"`
}

type PolicyApprovalDecisionInput struct {
	Decision         string  `json:"decision"`
	Comments         *string `json:"comments,omitempty"`
	DigitalSignature *string `json:"digital_signature,omitempty"`
}

type PolicyReview struct {
	BaseModel
	OrganizationID  string     `json:"organization_id"`
	PolicyID        string     `json:"policy_id"`
	ReviewType      string     `json:"review_type"`
	Status          string     `json:"status"`
	ReviewerUserID  *string    `json:"reviewer_user_id,omitempty"`
	ReviewDate      *time.Time `json:"review_date,omitempty"`
	DueDate         *time.Time `json:"due_date,omitempty"`
	CompletedDate   *time.Time `json:"completed_date,omitempty"`
	Outcome         *string    `json:"outcome,omitempty"`
	Findings        *string    `json:"findings,omitempty"`
	Recommendations *string    `json:"recommendations,omitempty"`
	TriggeredBy     *string    `json:"triggered_by,omitempty"`
	NewVersionID    *string    `json:"new_version_id,omitempty"`
}

type PolicyReviewInput struct {
	ReviewType     string     `json:"review_type"`
	ReviewerUserID *string    `json:"reviewer_user_id,omitempty"`
	ReviewDate     *time.Time `json:"review_date,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	TriggeredBy    *string    `json:"triggered_by,omitempty"`
}

type PolicyReviewPatch struct {
	Status          *string `json:"status,omitempty"`
	Outcome         *string `json:"outcome,omitempty"`
	Findings        *string `json:"findings,omitempty"`
	Recommendations *string `json:"recommendations,omitempty"`
	NewVersionID    *string `json:"new_version_id,omitempty"`
}

type PolicyAttestation struct {
	ID                string          `json:"id"`
	PolicyID          string          `json:"policy_id"`
	PolicyVersionID   *string         `json:"policy_version_id,omitempty"`
	OrganizationID    string          `json:"organization_id"`
	UserID            string          `json:"user_id"`
	CampaignID        *string         `json:"campaign_id,omitempty"`
	Status            string          `json:"status"`
	AttestedAt        *time.Time      `json:"attested_at,omitempty"`
	AttestedFromIP    *string         `json:"attested_from_ip,omitempty"`
	AttestationMethod *string         `json:"attestation_method,omitempty"`
	AttestationText   *string         `json:"attestation_text,omitempty"`
	DeclinedReason    *string         `json:"declined_reason,omitempty"`
	DueDate           *time.Time      `json:"due_date,omitempty"`
	ExpiresAt         *time.Time      `json:"expires_at,omitempty"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         time.Time       `json:"created_at"`
}

type PolicyAttestationInput struct {
	Decision          string          `json:"decision"`
	AttestationMethod string          `json:"attestation_method,omitempty"`
	AttestationText   *string         `json:"attestation_text,omitempty"`
	DeclinedReason    *string         `json:"declined_reason,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
}

type PolicyException struct {
	BaseModel
	OrganizationID       string     `json:"organization_id"`
	PolicyID             string     `json:"policy_id"`
	ExceptionRef         string     `json:"exception_ref"`
	Title                string     `json:"title"`
	Description          *string    `json:"description,omitempty"`
	Justification        string     `json:"justification"`
	RiskAssessment       *string    `json:"risk_assessment,omitempty"`
	CompensatingControls *string    `json:"compensating_controls,omitempty"`
	Status               string     `json:"status"`
	RequestedBy          *string    `json:"requested_by,omitempty"`
	ApprovedBy           *string    `json:"approved_by,omitempty"`
	ApprovedAt           *time.Time `json:"approved_at,omitempty"`
	EffectiveDate        *time.Time `json:"effective_date,omitempty"`
	ExpiryDate           *time.Time `json:"expiry_date,omitempty"`
	ReviewDate           *time.Time `json:"review_date,omitempty"`
	RiskLevel            *string    `json:"risk_level,omitempty"`
	LinkedRiskID         *string    `json:"linked_risk_id,omitempty"`
	Conditions           *string    `json:"conditions,omitempty"`
}

type PolicyExceptionInput struct {
	Title                string     `json:"title"`
	Description          *string    `json:"description,omitempty"`
	Justification        string     `json:"justification"`
	RiskAssessment       *string    `json:"risk_assessment,omitempty"`
	CompensatingControls *string    `json:"compensating_controls,omitempty"`
	EffectiveDate        *time.Time `json:"effective_date,omitempty"`
	ExpiryDate           *time.Time `json:"expiry_date,omitempty"`
	ReviewDate           *time.Time `json:"review_date,omitempty"`
	RiskLevel            *string    `json:"risk_level,omitempty"`
	LinkedRiskID         *string    `json:"linked_risk_id,omitempty"`
}

type PolicyExceptionDecisionInput struct {
	Decision   string     `json:"decision"`
	Conditions *string    `json:"conditions,omitempty"`
	ExpiryDate *time.Time `json:"expiry_date,omitempty"`
}
