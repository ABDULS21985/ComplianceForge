package models

import (
	"encoding/json"
	"time"
)

// AccessPolicyEffect describes whether a fully matched policy constrains or
// preserves an RBAC permission. An allow policy is a constraint, never a grant:
// RBAC remains the upper authorization bound.
type AccessPolicyEffect string

const (
	AccessPolicyEffectAllow AccessPolicyEffect = "allow"
	AccessPolicyEffectDeny  AccessPolicyEffect = "deny"
)

type AccessConditionOperator string

const (
	AccessOperatorEquals             AccessConditionOperator = "equals"
	AccessOperatorNotEquals          AccessConditionOperator = "not_equals"
	AccessOperatorIn                 AccessConditionOperator = "in"
	AccessOperatorNotIn              AccessConditionOperator = "not_in"
	AccessOperatorContains           AccessConditionOperator = "contains"
	AccessOperatorContainsAny        AccessConditionOperator = "contains_any"
	AccessOperatorGreaterThan        AccessConditionOperator = "greater_than"
	AccessOperatorGreaterThanOrEqual AccessConditionOperator = "greater_than_or_equal"
	AccessOperatorLessThan           AccessConditionOperator = "less_than"
	AccessOperatorLessThanOrEqual    AccessConditionOperator = "less_than_or_equal"
	AccessOperatorBetween            AccessConditionOperator = "between"
	AccessOperatorInCIDR             AccessConditionOperator = "in_cidr"
	AccessOperatorEqualsSubject      AccessConditionOperator = "equals_subject"
)

// AccessCondition uses RawMessage rather than interface{} so boundary code can
// strictly validate the JSON value for the selected operator and attribute.
type AccessCondition struct {
	Attribute string                  `json:"attribute"`
	Operator  AccessConditionOperator `json:"operator"`
	Value     json.RawMessage         `json:"value"`
}

type AccessPolicy struct {
	ID                    string             `json:"id"`
	OrganizationID        string             `json:"organization_id"`
	Name                  string             `json:"name"`
	Description           string             `json:"description,omitempty"`
	Priority              int                `json:"priority"`
	Effect                AccessPolicyEffect `json:"effect"`
	IsActive              bool               `json:"is_active"`
	SubjectConditions     []AccessCondition  `json:"subject_conditions"`
	ResourceType          string             `json:"resource_type"`
	ResourceConditions    []AccessCondition  `json:"resource_conditions"`
	Actions               []string           `json:"actions"`
	EnvironmentConditions []AccessCondition  `json:"environment_conditions"`
	ValidFrom             *time.Time         `json:"valid_from,omitempty"`
	ValidUntil            *time.Time         `json:"valid_until,omitempty"`
	Version               int64              `json:"version"`
	CreatedBy             *string            `json:"created_by,omitempty"`
	UpdatedBy             *string            `json:"updated_by,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
	DeletedAt             *time.Time         `json:"deleted_at,omitempty"`
}

type AccessPolicyInput struct {
	Name                  string             `json:"name"`
	Description           string             `json:"description,omitempty"`
	Priority              int                `json:"priority"`
	Effect                AccessPolicyEffect `json:"effect"`
	IsActive              bool               `json:"is_active"`
	SubjectConditions     []AccessCondition  `json:"subject_conditions"`
	ResourceType          string             `json:"resource_type"`
	ResourceConditions    []AccessCondition  `json:"resource_conditions"`
	Actions               []string           `json:"actions"`
	EnvironmentConditions []AccessCondition  `json:"environment_conditions"`
	ValidFrom             *time.Time         `json:"valid_from,omitempty"`
	ValidUntil            *time.Time         `json:"valid_until,omitempty"`
	ExpectedVersion       *int64             `json:"expected_version,omitempty"`
	Reason                string             `json:"reason"`
}

type AccessPolicyListFilter struct {
	PaginationRequest
	Search       string
	ResourceType string
	Effect       AccessPolicyEffect
	Active       *bool
}

type AccessAssigneeType string

const (
	AccessAssigneeUser     AccessAssigneeType = "user"
	AccessAssigneeRole     AccessAssigneeType = "role"
	AccessAssigneeGroup    AccessAssigneeType = "group"
	AccessAssigneeAllUsers AccessAssigneeType = "all_users"
)

type AccessPolicyAssignment struct {
	ID             string             `json:"id"`
	OrganizationID string             `json:"organization_id"`
	PolicyID       string             `json:"policy_id"`
	AssigneeType   AccessAssigneeType `json:"assignee_type"`
	AssigneeID     *string            `json:"assignee_id,omitempty"`
	ValidFrom      *time.Time         `json:"valid_from,omitempty"`
	ValidUntil     *time.Time         `json:"valid_until,omitempty"`
	CreatedBy      string             `json:"created_by"`
	CreatedAt      time.Time          `json:"created_at"`
}

type AccessPolicyAssignmentInput struct {
	AssigneeType AccessAssigneeType `json:"assignee_type"`
	AssigneeID   *string            `json:"assignee_id,omitempty"`
	ValidFrom    *time.Time         `json:"valid_from,omitempty"`
	ValidUntil   *time.Time         `json:"valid_until,omitempty"`
	Reason       string             `json:"reason"`
}

type AccessFieldClassification string

const (
	AccessFieldPublic            AccessFieldClassification = "public"
	AccessFieldInternal          AccessFieldClassification = "internal"
	AccessFieldConfidential      AccessFieldClassification = "confidential"
	AccessFieldRestricted        AccessFieldClassification = "restricted"
	AccessFieldPersonal          AccessFieldClassification = "personal"
	AccessFieldFinancial         AccessFieldClassification = "financial"
	AccessFieldLegal             AccessFieldClassification = "legal"
	AccessFieldSecuritySensitive AccessFieldClassification = "security_sensitive"
)

type AccessFieldVisibility string

const (
	AccessFieldVisible AccessFieldVisibility = "visible"
	AccessFieldMasked  AccessFieldVisibility = "masked"
	AccessFieldHidden  AccessFieldVisibility = "hidden"
)

type AccessMaskStrategy string

const (
	AccessMaskRedact AccessMaskStrategy = "redact"
	AccessMaskLast4  AccessMaskStrategy = "last4"
	AccessMaskEmail  AccessMaskStrategy = "email"
	AccessMaskCustom AccessMaskStrategy = "custom"
)

type AccessFieldPermission struct {
	ID             string                    `json:"id"`
	OrganizationID string                    `json:"organization_id"`
	PolicyID       string                    `json:"policy_id"`
	PolicyPriority int                       `json:"policy_priority"`
	ResourceType   string                    `json:"resource_type"`
	FieldPath      string                    `json:"field_path"`
	Classification AccessFieldClassification `json:"classification"`
	Visibility     AccessFieldVisibility     `json:"visibility"`
	MaskStrategy   AccessMaskStrategy        `json:"mask_strategy,omitempty"`
	MaskPattern    string                    `json:"mask_pattern,omitempty"`
	Version        int64                     `json:"version"`
	CreatedAt      time.Time                 `json:"created_at"`
}

type AccessFieldPermissionInput struct {
	ResourceType    string                    `json:"resource_type"`
	FieldPath       string                    `json:"field_path"`
	Classification  AccessFieldClassification `json:"classification"`
	Visibility      AccessFieldVisibility     `json:"visibility"`
	MaskStrategy    AccessMaskStrategy        `json:"mask_strategy,omitempty"`
	MaskPattern     string                    `json:"mask_pattern,omitempty"`
	ExpectedVersion *int64                    `json:"expected_version,omitempty"`
	Reason          string                    `json:"reason"`
}

type AccessObjectGrantStatus string

const (
	AccessObjectGrantPending  AccessObjectGrantStatus = "pending"
	AccessObjectGrantApproved AccessObjectGrantStatus = "approved"
	AccessObjectGrantRejected AccessObjectGrantStatus = "rejected"
	AccessObjectGrantRevoked  AccessObjectGrantStatus = "revoked"
	AccessObjectGrantExpired  AccessObjectGrantStatus = "expired"
)

// AccessObjectGrant is the typed view of user_entity_permissions. It is always
// subordinate to RBAC and can only satisfy a relevant ABAC scope constraint.
type AccessObjectGrant struct {
	ID               string                  `json:"id"`
	OrganizationID   string                  `json:"organization_id"`
	SubjectID        string                  `json:"subject_id"`
	ResourceType     string                  `json:"resource_type"`
	ResourceID       string                  `json:"resource_id"`
	Actions          []string                `json:"actions"`
	Status           AccessObjectGrantStatus `json:"status"`
	SponsorID        string                  `json:"sponsor_id"`
	ApprovedBy       *string                 `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time              `json:"approved_at,omitempty"`
	ValidFrom        time.Time               `json:"valid_from"`
	ValidUntil       time.Time               `json:"valid_until"`
	AllowDownload    bool                    `json:"allow_download"`
	RequireWatermark bool                    `json:"require_watermark"`
	WatermarkText    string                  `json:"watermark_text,omitempty"`
	Reason           string                  `json:"reason"`
	Version          int64                   `json:"version"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
	RevokedAt        *time.Time              `json:"revoked_at,omitempty"`
	RevokedBy        *string                 `json:"revoked_by,omitempty"`
}

type AccessObjectGrantInput struct {
	SubjectID        string     `json:"subject_id"`
	ResourceType     string     `json:"resource_type"`
	ResourceID       string     `json:"resource_id"`
	Actions          []string   `json:"actions"`
	SponsorID        string     `json:"-"`
	ValidFrom        time.Time  `json:"valid_from"`
	ValidUntil       time.Time  `json:"valid_until"`
	AllowDownload    bool       `json:"allow_download"`
	RequireWatermark bool       `json:"require_watermark"`
	WatermarkText    string     `json:"watermark_text,omitempty"`
	Reason           string     `json:"reason"`
	ExpectedVersion  *int64     `json:"expected_version,omitempty"`
	ApprovedBy       *string    `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
}

type AccessObjectGrantFilter struct {
	PaginationRequest
	SubjectID    string
	ResourceType string
	ResourceID   string
	Status       AccessObjectGrantStatus
	ActiveAt     *time.Time
}

type AccessObjectGrantDecisionInput struct {
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expected_version"`
}

type AccessObjectGrantRevocationInput struct {
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expected_version"`
}

type AccessSubject struct {
	ID         string                     `json:"id"`
	Roles      []string                   `json:"roles"`
	Department string                     `json:"department,omitempty"`
	Location   string                     `json:"location,omitempty"`
	Status     string                     `json:"status"`
	SuperAdmin bool                       `json:"super_admin"`
	Attributes map[string]json.RawMessage `json:"attributes,omitempty"`
}

type AccessResource struct {
	ID             string                     `json:"id,omitempty"`
	Type           string                     `json:"type"`
	OwnerID        string                     `json:"owner_id,omitempty"`
	Department     string                     `json:"department,omitempty"`
	Location       string                     `json:"location,omitempty"`
	Classification string                     `json:"classification,omitempty"`
	Attributes     map[string]json.RawMessage `json:"attributes,omitempty"`
}

type AccessEvaluationBundle struct {
	Subject          AccessSubject           `json:"subject"`
	Resource         AccessResource          `json:"resource"`
	Policies         []AccessPolicy          `json:"policies"`
	ObjectGrants     []AccessObjectGrant     `json:"object_grants"`
	FieldPermissions []AccessFieldPermission `json:"field_permissions"`
}

type AccessConstraintOutcome string

const (
	AccessConstraintNotApplicable AccessConstraintOutcome = "not_applicable"
	AccessConstraintAllow         AccessConstraintOutcome = "allow"
	AccessConstraintDeny          AccessConstraintOutcome = "deny"
)

type AccessObligation struct {
	Kind       string            `json:"kind"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

type AccessConstraintDecision struct {
	Allowed            bool                    `json:"allowed"`
	Outcome            AccessConstraintOutcome `json:"outcome"`
	ReasonCode         string                  `json:"reason_code"`
	Reason             string                  `json:"reason"`
	RelevantPolicyIDs  []string                `json:"relevant_policy_ids"`
	MatchedPolicyIDs   []string                `json:"matched_policy_ids"`
	WinningPolicyID    *string                 `json:"winning_policy_id,omitempty"`
	WinningPolicyName  *string                 `json:"winning_policy_name,omitempty"`
	MatchedObjectGrant *string                 `json:"matched_object_grant_id,omitempty"`
	FieldPermissions   []AccessFieldPermission `json:"field_permissions"`
	Obligations        []AccessObligation      `json:"obligations"`
	EvaluationTimeUS   int                     `json:"evaluation_time_us"`
}

type AccessDecisionEvidence struct {
	ID                   string                  `json:"id"`
	OrganizationID       string                  `json:"organization_id"`
	SubjectID            string                  `json:"subject_id"`
	Action               string                  `json:"action"`
	ResourceType         string                  `json:"resource_type"`
	ResourceID           *string                 `json:"resource_id,omitempty"`
	RBACAllowed          bool                    `json:"rbac_allowed"`
	ConstraintOutcome    AccessConstraintOutcome `json:"constraint_outcome"`
	Decision             string                  `json:"decision"`
	ReasonCode           string                  `json:"reason_code"`
	MatchedPolicyIDs     []string                `json:"matched_policy_ids"`
	WinningPolicyID      *string                 `json:"winning_policy_id,omitempty"`
	MatchedObjectGrantID *string                 `json:"matched_object_grant_id,omitempty"`
	RequestID            string                  `json:"request_id,omitempty"`
	SubjectSnapshot      json.RawMessage         `json:"subject_snapshot"`
	ResourceSnapshot     json.RawMessage         `json:"resource_snapshot"`
	EnvironmentSnapshot  json.RawMessage         `json:"environment_snapshot"`
	EvaluationTimeUS     int                     `json:"evaluation_time_us"`
	CreatedAt            time.Time               `json:"created_at"`
}

type AccessDecisionEvidenceFilter struct {
	PaginationRequest
	SubjectID    string
	ResourceType string
	ResourceID   string
	Action       string
	Decision     string
	From         *time.Time
	Until        *time.Time
}

type AccessPolicyCertification struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	PolicyID       string          `json:"policy_id"`
	PolicyVersion  int64           `json:"policy_version"`
	CertifiedBy    string          `json:"certified_by"`
	Decision       string          `json:"decision"`
	Reason         string          `json:"reason"`
	PolicySnapshot json.RawMessage `json:"policy_snapshot"`
	SnapshotSHA256 string          `json:"snapshot_sha256"`
	CertifiedAt    time.Time       `json:"certified_at"`
}

type AccessPolicyCertificationInput struct {
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expected_version"`
}
