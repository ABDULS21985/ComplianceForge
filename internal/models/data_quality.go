package models

import (
	"errors"
	"time"
)

var (
	ErrDataQualityScope       = errors.New("data quality requires an active tenant-bound principal")
	ErrDataQualityUnavailable = errors.New("data quality snapshot is unavailable")
)

const (
	DataQualityRulesetVersion = 1
	DataQualityScope          = "tenant_structure_and_delivery_v1"
	MaximumDataQualityCount   = int64(9007199254740991) // Exact JSON/JavaScript integer domain.
)

type DataQualityStatus string

const (
	DataQualityHealthy  DataQualityStatus = "healthy"
	DataQualityWarning  DataQualityStatus = "warning"
	DataQualityCritical DataQualityStatus = "critical"
)

type DataQualityCategory string

const (
	DataQualityStructural DataQualityCategory = "structural"
	DataQualityLifecycle  DataQualityCategory = "lifecycle_precondition"
)

type DataQualityCheckKey string

const (
	DQControlAdoptionScope  DataQualityCheckKey = "control_adoption_tenant_parent"
	DQControlFramework      DataQualityCheckKey = "control_adoption_framework_alignment"
	DQPolicyVersionScope    DataQualityCheckKey = "policy_version_tenant_parent"
	DQPolicyCurrentVersion  DataQualityCheckKey = "policy_current_version_alignment"
	DQPolicyWorkflow        DataQualityCheckKey = "policy_workflow_parent_alignment"
	DQPolicyStepScope       DataQualityCheckKey = "policy_step_tenant_workflow"
	DQRiskAssessmentScope   DataQualityCheckKey = "risk_assessment_tenant_parent"
	DQRiskTreatmentScope    DataQualityCheckKey = "risk_treatment_tenant_parent"
	DQRiskIndicatorScope    DataQualityCheckKey = "risk_indicator_tenant_parent"
	DQRiskValueScope        DataQualityCheckKey = "risk_value_tenant_indicator"
	DQFindingAuditScope     DataQualityCheckKey = "finding_tenant_audit"
	DQAssetVendorScope      DataQualityCheckKey = "asset_tenant_vendor"
	DQNotificationRecipient DataQualityCheckKey = "notification_tenant_recipient"
	DQNotificationChannel   DataQualityCheckKey = "notification_channel_alignment"
	DQNotificationParent    DataQualityCheckKey = "notification_tenant_parent"
	DQUserRoleScope         DataQualityCheckKey = "user_role_scope_alignment"
	DQDueEmailRecipient     DataQualityCheckKey = "due_email_recipient_unavailable"
	DQUnremovedMembership   DataQualityCheckKey = "unremoved_tombstoned_group_membership"
)

type DataQualityCheckDefinition struct {
	Key        DataQualityCheckKey `json:"key"`
	Category   DataQualityCategory `json:"category"`
	Definition string              `json:"definition"`
}

// DataQualityCheckDefinitions returns an owned, fixed catalog. Definitions are
// curated invariant descriptions, never business messages or raw metadata.
func DataQualityCheckDefinitions() []DataQualityCheckDefinition {
	return []DataQualityCheckDefinition{
		{DQControlAdoptionScope, DataQualityStructural, "Control implementations must reference a retained framework adoption in the same tenant."},
		{DQControlFramework, DataQualityStructural, "An implementation's framework control must belong to its adopted framework."},
		{DQPolicyVersionScope, DataQualityStructural, "Policy versions must reference a retained policy in the same tenant."},
		{DQPolicyCurrentVersion, DataQualityStructural, "A nonempty current-version pointer must match the policy, tenant, and current version number."},
		{DQPolicyWorkflow, DataQualityStructural, "Policy workflows must reference the same-tenant policy and, when present, a version of that policy."},
		{DQPolicyStepScope, DataQualityStructural, "Approval steps must reference a retained workflow in the same tenant."},
		{DQRiskAssessmentScope, DataQualityStructural, "Risk assessments must reference a retained risk in the same tenant."},
		{DQRiskTreatmentScope, DataQualityStructural, "Risk treatments must reference a retained risk in the same tenant."},
		{DQRiskIndicatorScope, DataQualityStructural, "Nonempty indicator risk pointers must reference a retained risk in the same tenant."},
		{DQRiskValueScope, DataQualityStructural, "Indicator values must reference a retained indicator in the same tenant."},
		{DQFindingAuditScope, DataQualityStructural, "Findings must reference a retained audit in the same tenant."},
		{DQAssetVendorScope, DataQualityStructural, "Nonempty asset vendor pointers must reference a retained vendor in the same tenant."},
		{DQNotificationRecipient, DataQualityStructural, "Notifications must reference a retained recipient in the same tenant."},
		{DQNotificationChannel, DataQualityStructural, "Nonempty notification channel pointers must match the tenant and channel type."},
		{DQNotificationParent, DataQualityStructural, "Nonempty escalation parent pointers must reference a retained notification in the same tenant."},
		{DQUserRoleScope, DataQualityStructural, "Role assignments must reference same-tenant custom roles or genuine global system roles."},
		{DQDueEmailRecipient, DataQualityLifecycle, "Unleased, due, retry-eligible email deliveries require an active, nondeleted same-tenant recipient."},
		{DQUnremovedMembership, DataQualityLifecycle, "Deleted groups and deprovisioned users must have their persisted group memberships closed."},
	}
}

type DataQualityCheck struct {
	DataQualityCheckDefinition
	Count  int64             `json:"count"`
	Status DataQualityStatus `json:"status"`
}

// DataQualitySnapshot intentionally has no organization, actor, record,
// endpoint, message, or arbitrary metadata field.
type DataQualitySnapshot struct {
	RulesetVersion int                `json:"ruleset_version"`
	Scope          string             `json:"scope"`
	SchemaVersion  int64              `json:"schema_version"`
	AsOf           time.Time          `json:"as_of"`
	Status         DataQualityStatus  `json:"status"`
	Checks         []DataQualityCheck `json:"checks"`
}

type DataQualityCount struct {
	Key   DataQualityCheckKey
	Count int64
}

// DataQualityCounts is an internal scope-bound store result, never a response.
// Explicit JSON exclusions prevent an accidental internal-result marshal
// from exposing principals or unchecked count metadata.
type DataQualityCounts struct {
	OrganizationID string             `json:"-"`
	ActorID        string             `json:"-"`
	SchemaVersion  int64              `json:"-"`
	AsOf           time.Time          `json:"-"`
	Counts         []DataQualityCount `json:"-"`
}
