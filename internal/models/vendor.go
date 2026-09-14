package models

import (
	"encoding/json"
	"time"
)

type VendorStatus string

const (
	VendorStatusProspective VendorStatus = "prospective"
	VendorStatusOnboarding  VendorStatus = "onboarding"
	VendorStatusActive      VendorStatus = "active"
	VendorStatusSuspended   VendorStatus = "suspended"
	VendorStatusOffboarding VendorStatus = "offboarding"
	VendorStatusOffboarded  VendorStatus = "offboarded"
	VendorStatusRejected    VendorStatus = "rejected"

	// Source-compatible names retained for callers of the former stub.
	VendorStatusUnderReview = VendorStatusOnboarding
	VendorStatusApproved    = VendorStatusActive
	VendorStatusTerminated  = VendorStatusOffboarded
)

type VendorCriticality string

const (
	VendorCriticalityCritical VendorCriticality = "critical"
	VendorCriticalityHigh     VendorCriticality = "high"
	VendorCriticalityMedium   VendorCriticality = "medium"
	VendorCriticalityLow      VendorCriticality = "low"
)

type VendorRiskTier string

const (
	VendorRiskCritical VendorRiskTier = "critical"
	VendorRiskHigh     VendorRiskTier = "high"
	VendorRiskMedium   VendorRiskTier = "medium"
	VendorRiskLow      VendorRiskTier = "low"
)

type VendorTier string

const (
	VendorTierOne   VendorTier = "tier_1"
	VendorTierTwo   VendorTier = "tier_2"
	VendorTierThree VendorTier = "tier_3"
	VendorTierFour  VendorTier = "tier_4"
)

type VendorDPAStatus string

const (
	VendorDPANotRequired VendorDPAStatus = "not_required"
	VendorDPAPending     VendorDPAStatus = "pending"
	VendorDPAExecuted    VendorDPAStatus = "executed"
	VendorDPAExpired     VendorDPAStatus = "expired"
	VendorDPATerminated  VendorDPAStatus = "terminated"
)

type VendorAssessmentStatus string

const (
	VendorAssessmentNotDue     VendorAssessmentStatus = "not_due"
	VendorAssessmentDue        VendorAssessmentStatus = "due"
	VendorAssessmentInProgress VendorAssessmentStatus = "in_progress"
	VendorAssessmentCompleted  VendorAssessmentStatus = "completed"
	VendorAssessmentOverdue    VendorAssessmentStatus = "overdue"
	VendorAssessmentWaived     VendorAssessmentStatus = "waived"
)

type VendorPerson struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// Vendor is the canonical third-party register projection. Related records
// are populated on detail reads but omitted from paginated list rows.
type Vendor struct {
	TenantModel
	VendorRef             string                 `json:"vendor_ref"`
	Name                  string                 `json:"name"`
	LegalName             string                 `json:"legal_name,omitempty"`
	Description           string                 `json:"description,omitempty"`
	Website               string                 `json:"website,omitempty"`
	Industry              string                 `json:"industry,omitempty"`
	Category              string                 `json:"category,omitempty"`
	CountryCode           string                 `json:"country_code,omitempty"`
	OwnerUserID           *string                `json:"owner_user_id,omitempty"`
	Owner                 *VendorPerson          `json:"owner,omitempty"`
	Criticality           VendorCriticality      `json:"criticality"`
	VendorTier            VendorTier             `json:"vendor_tier"`
	RiskTier              VendorRiskTier         `json:"risk_tier"`
	RiskScore             *float64               `json:"risk_score,omitempty"`
	Status                VendorStatus           `json:"status"`
	ServiceDescription    string                 `json:"service_description,omitempty"`
	Services              []string               `json:"services"`
	DataProcessing        bool                   `json:"data_processing"`
	DataCategories        []string               `json:"data_categories"`
	ProcessingLocations   []string               `json:"processing_locations"`
	DPARequired           bool                   `json:"dpa_required"`
	DPAStatus             VendorDPAStatus        `json:"dpa_status"`
	DPAInPlace            bool                   `json:"dpa_in_place"`
	DPAReference          string                 `json:"dpa_reference,omitempty"`
	DPASignedDate         *time.Time             `json:"dpa_signed_date,omitempty"`
	DPAExpiryDate         *time.Time             `json:"dpa_expiry_date,omitempty"`
	AssessmentFrequency   string                 `json:"assessment_frequency"`
	AssessmentCadenceDays int                    `json:"assessment_cadence_days"`
	AssessmentStatus      VendorAssessmentStatus `json:"assessment_status"`
	LastAssessmentDate    *time.Time             `json:"last_assessment_date,omitempty"`
	NextAssessmentDate    *time.Time             `json:"next_assessment_date,omitempty"`
	NextReviewDate        *time.Time             `json:"next_review_date,omitempty"`
	OnboardingStartedAt   *time.Time             `json:"onboarding_started_at,omitempty"`
	OnboardedAt           *time.Time             `json:"onboarded_at,omitempty"`
	SuspendedAt           *time.Time             `json:"suspended_at,omitempty"`
	OffboardingStartedAt  *time.Time             `json:"offboarding_started_at,omitempty"`
	OffboardedAt          *time.Time             `json:"offboarded_at,omitempty"`
	RejectedAt            *time.Time             `json:"rejected_at,omitempty"`
	RetentionUntil        *time.Time             `json:"retention_until,omitempty"`
	LegalHold             bool                   `json:"legal_hold"`
	Version               int64                  `json:"version"`
	Metadata              json.RawMessage        `json:"metadata"`
	CreatedBy             string                 `json:"created_by"`
	ContactName           string                 `json:"contact_name,omitempty"`
	ContactEmail          string                 `json:"contact_email,omitempty"`
	ContactPhone          string                 `json:"contact_phone,omitempty"`
	ContractStartDate     *time.Time             `json:"contract_start_date,omitempty"`
	ContractEndDate       *time.Time             `json:"contract_end_date,omitempty"`
	ContractValueEUR      *float64               `json:"contract_value_eur,omitempty"`
	Certifications        []string               `json:"certifications"`
	Contacts              []VendorContact        `json:"contacts,omitempty"`
	Contracts             []VendorContract       `json:"contracts,omitempty"`
	CertificationDetails  []VendorCertification  `json:"certification_details,omitempty"`
	SubProcessors         []VendorSubprocessor   `json:"sub_processors,omitempty"`
}

type VendorCreateInput struct {
	Name                  string                    `json:"name"`
	LegalName             string                    `json:"legal_name,omitempty"`
	Description           string                    `json:"description,omitempty"`
	Website               string                    `json:"website,omitempty"`
	Industry              string                    `json:"industry,omitempty"`
	Category              string                    `json:"category,omitempty"`
	CountryCode           string                    `json:"country_code,omitempty"`
	OwnerUserID           *string                   `json:"owner_user_id,omitempty"`
	Criticality           VendorCriticality         `json:"criticality,omitempty"`
	VendorTier            VendorTier                `json:"vendor_tier,omitempty"`
	RiskTier              VendorRiskTier            `json:"risk_tier,omitempty"`
	RiskScore             *float64                  `json:"risk_score,omitempty"`
	ServiceDescription    string                    `json:"service_description,omitempty"`
	Services              []string                  `json:"services,omitempty"`
	DataProcessing        bool                      `json:"data_processing"`
	DataCategories        []string                  `json:"data_categories,omitempty"`
	ProcessingLocations   []string                  `json:"processing_locations,omitempty"`
	DPARequired           bool                      `json:"dpa_required"`
	DPAStatus             VendorDPAStatus           `json:"dpa_status,omitempty"`
	DPAReference          string                    `json:"dpa_reference,omitempty"`
	DPASignedDate         *time.Time                `json:"dpa_signed_date,omitempty"`
	DPAExpiryDate         *time.Time                `json:"dpa_expiry_date,omitempty"`
	AssessmentFrequency   string                    `json:"assessment_frequency,omitempty"`
	AssessmentCadenceDays int                       `json:"assessment_cadence_days,omitempty"`
	NextAssessmentDate    *time.Time                `json:"next_assessment_date,omitempty"`
	NextReviewDate        *time.Time                `json:"next_review_date,omitempty"`
	RetentionUntil        *time.Time                `json:"retention_until,omitempty"`
	Metadata              json.RawMessage           `json:"metadata,omitempty"`
	ContactName           string                    `json:"contact_name,omitempty"`
	ContactEmail          string                    `json:"contact_email,omitempty"`
	ContactPhone          string                    `json:"contact_phone,omitempty"`
	Certifications        []string                  `json:"certifications,omitempty"`
	InitialContracts      []VendorContractInput     `json:"contracts,omitempty"`
	InitialSubProcessors  []VendorSubprocessorInput `json:"sub_processors,omitempty"`
}

type VendorPatch struct {
	Version               int64              `json:"version"`
	Name                  *string            `json:"name,omitempty"`
	LegalName             *string            `json:"legal_name,omitempty"`
	Description           *string            `json:"description,omitempty"`
	Website               *string            `json:"website,omitempty"`
	Industry              *string            `json:"industry,omitempty"`
	Category              *string            `json:"category,omitempty"`
	CountryCode           *string            `json:"country_code,omitempty"`
	OwnerUserID           *string            `json:"owner_user_id,omitempty"`
	ClearOwner            bool               `json:"clear_owner,omitempty"`
	Criticality           *VendorCriticality `json:"criticality,omitempty"`
	VendorTier            *VendorTier        `json:"vendor_tier,omitempty"`
	RiskTier              *VendorRiskTier    `json:"risk_tier,omitempty"`
	RiskScore             *float64           `json:"risk_score,omitempty"`
	ClearRiskScore        bool               `json:"clear_risk_score,omitempty"`
	ServiceDescription    *string            `json:"service_description,omitempty"`
	Services              *[]string          `json:"services,omitempty"`
	DataProcessing        *bool              `json:"data_processing,omitempty"`
	DataCategories        *[]string          `json:"data_categories,omitempty"`
	ProcessingLocations   *[]string          `json:"processing_locations,omitempty"`
	DPARequired           *bool              `json:"dpa_required,omitempty"`
	DPAStatus             *VendorDPAStatus   `json:"dpa_status,omitempty"`
	DPAReference          *string            `json:"dpa_reference,omitempty"`
	DPASignedDate         *time.Time         `json:"dpa_signed_date,omitempty"`
	DPAExpiryDate         *time.Time         `json:"dpa_expiry_date,omitempty"`
	ClearDPA              bool               `json:"clear_dpa,omitempty"`
	AssessmentFrequency   *string            `json:"assessment_frequency,omitempty"`
	AssessmentCadenceDays *int               `json:"assessment_cadence_days,omitempty"`
	NextAssessmentDate    *time.Time         `json:"next_assessment_date,omitempty"`
	ClearNextAssessment   bool               `json:"clear_next_assessment_date,omitempty"`
	NextReviewDate        *time.Time         `json:"next_review_date,omitempty"`
	ClearNextReview       bool               `json:"clear_next_review_date,omitempty"`
	RetentionUntil        *time.Time         `json:"retention_until,omitempty"`
	LegalHold             *bool              `json:"legal_hold,omitempty"`
	Metadata              json.RawMessage    `json:"metadata,omitempty"`
}

type VendorListFilter struct {
	PaginationRequest
	Status         string
	Criticality    string
	VendorTier     string
	RiskTier       string
	OwnerUserID    string
	CountryCode    string
	DataProcessing *bool
	DueAssessment  *bool
	Search         string
	SortBy         string
	SortDirection  string
}

type VendorTransitionInput struct {
	Status  VendorStatus `json:"status"`
	Reason  string       `json:"reason,omitempty"`
	Version int64        `json:"version"`
}

type VendorAssessmentInput struct {
	Version            int64                  `json:"version"`
	Status             VendorAssessmentStatus `json:"status"`
	RiskTier           VendorRiskTier         `json:"risk_tier"`
	RiskScore          *float64               `json:"risk_score,omitempty"`
	AssessedAt         time.Time              `json:"assessed_at"`
	NextAssessmentDate *time.Time             `json:"next_assessment_date,omitempty"`
	Notes              string                 `json:"notes"`
}

type VendorContact struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	VendorID       string     `json:"vendor_id"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	Phone          string     `json:"phone,omitempty"`
	Title          string     `json:"title,omitempty"`
	ContactType    string     `json:"contact_type"`
	IsPrimary      bool       `json:"is_primary"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

type VendorContactInput struct {
	Version     int64  `json:"version"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Phone       string `json:"phone,omitempty"`
	Title       string `json:"title,omitempty"`
	ContactType string `json:"contact_type"`
	IsPrimary   bool   `json:"is_primary"`
}

type VendorContract struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	VendorID       string          `json:"vendor_id"`
	ContractRef    string          `json:"contract_ref"`
	Name           string          `json:"name"`
	Status         string          `json:"status"`
	StartDate      *time.Time      `json:"start_date,omitempty"`
	EndDate        *time.Time      `json:"end_date,omitempty"`
	NoticeDays     int             `json:"notice_days"`
	RenewalDate    *time.Time      `json:"renewal_date,omitempty"`
	AutoRenew      bool            `json:"auto_renew"`
	ValueAmount    *float64        `json:"value_amount,omitempty"`
	Currency       string          `json:"currency"`
	IncludesDPA    bool            `json:"includes_dpa"`
	SignedAt       *time.Time      `json:"signed_at,omitempty"`
	OwnerUserID    *string         `json:"owner_user_id,omitempty"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty"`
}

type VendorContractInput struct {
	Version     int64           `json:"version"`
	ContractRef string          `json:"contract_ref"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	StartDate   *time.Time      `json:"start_date,omitempty"`
	EndDate     *time.Time      `json:"end_date,omitempty"`
	NoticeDays  int             `json:"notice_days"`
	RenewalDate *time.Time      `json:"renewal_date,omitempty"`
	AutoRenew   bool            `json:"auto_renew"`
	ValueAmount *float64        `json:"value_amount,omitempty"`
	Currency    string          `json:"currency"`
	IncludesDPA bool            `json:"includes_dpa"`
	SignedAt    *time.Time      `json:"signed_at,omitempty"`
	OwnerUserID *string         `json:"owner_user_id,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type VendorCertification struct {
	ID                string          `json:"id"`
	OrganizationID    string          `json:"organization_id"`
	VendorID          string          `json:"vendor_id"`
	Name              string          `json:"name"`
	Issuer            string          `json:"issuer,omitempty"`
	CertificateNumber string          `json:"certificate_number,omitempty"`
	Status            string          `json:"status"`
	IssuedOn          *time.Time      `json:"issued_on,omitempty"`
	ExpiresOn         *time.Time      `json:"expires_on,omitempty"`
	EvidenceReference string          `json:"evidence_reference,omitempty"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	DeletedAt         *time.Time      `json:"deleted_at,omitempty"`
}

type VendorCertificationInput struct {
	Version           int64           `json:"version"`
	Name              string          `json:"name"`
	Issuer            string          `json:"issuer,omitempty"`
	CertificateNumber string          `json:"certificate_number,omitempty"`
	Status            string          `json:"status"`
	IssuedOn          *time.Time      `json:"issued_on,omitempty"`
	ExpiresOn         *time.Time      `json:"expires_on,omitempty"`
	EvidenceReference string          `json:"evidence_reference,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
}

type VendorSubprocessor struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	VendorID       string          `json:"vendor_id"`
	Name           string          `json:"name"`
	Purpose        string          `json:"purpose"`
	CountryCode    string          `json:"country_code,omitempty"`
	DataCategories []string        `json:"data_categories"`
	Status         string          `json:"status"`
	ApprovedAt     *time.Time      `json:"approved_at,omitempty"`
	ApprovedBy     *string         `json:"approved_by,omitempty"`
	RemovedAt      *time.Time      `json:"removed_at,omitempty"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty"`
}

type VendorSubprocessorInput struct {
	Version        int64           `json:"version"`
	Name           string          `json:"name"`
	Purpose        string          `json:"purpose"`
	CountryCode    string          `json:"country_code,omitempty"`
	DataCategories []string        `json:"data_categories,omitempty"`
	Status         string          `json:"status"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type VendorEvent struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	VendorID       string          `json:"vendor_id"`
	EventType      string          `json:"event_type"`
	ActorUserID    string          `json:"actor_user_id"`
	VendorVersion  int64           `json:"vendor_version"`
	Summary        string          `json:"summary"`
	Details        json.RawMessage `json:"details"`
	CreatedAt      time.Time       `json:"created_at"`
}

type VendorStatistics struct {
	Total                  int                       `json:"total"`
	Active                 int                       `json:"active"`
	CriticalRisk           int                       `json:"critical_risk"`
	HighRisk               int                       `json:"high_risk"`
	MissingDPA             int                       `json:"missing_dpa"`
	AssessmentsDue         int                       `json:"assessments_due"`
	ContractsExpiring      int                       `json:"contracts_expiring"`
	CertificationsExpiring int                       `json:"certifications_expiring"`
	TotalContractValueEUR  float64                   `json:"total_contract_value_eur"`
	ByStatus               map[VendorStatus]int      `json:"by_status"`
	ByTier                 map[VendorTier]int        `json:"by_tier"`
	ByCriticality          map[VendorCriticality]int `json:"by_criticality"`
}

type VendorDueContract struct {
	VendorID   string         `json:"vendor_id"`
	VendorRef  string         `json:"vendor_ref"`
	VendorName string         `json:"vendor_name"`
	Contract   VendorContract `json:"contract"`
	DueDate    time.Time      `json:"due_date"`
}

type VendorExpiringCertification struct {
	VendorID      string              `json:"vendor_id"`
	VendorRef     string              `json:"vendor_ref"`
	VendorName    string              `json:"vendor_name"`
	Certification VendorCertification `json:"certification"`
}
