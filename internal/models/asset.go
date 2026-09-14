package models

import (
	"encoding/json"
	"time"
)

// AssetType classifies an inventory record using the public API vocabulary.
type AssetType string

const (
	AssetTypeHardware AssetType = "hardware"
	AssetTypeSoftware AssetType = "software"
	AssetTypeData     AssetType = "data"
	AssetTypeService  AssetType = "service"
	AssetTypeNetwork  AssetType = "network"
	AssetTypePeople   AssetType = "people"
	AssetTypeFacility AssetType = "facility"
)

type AssetClassification string

const (
	AssetClassificationPublic       AssetClassification = "public"
	AssetClassificationInternal     AssetClassification = "internal"
	AssetClassificationConfidential AssetClassification = "confidential"
	AssetClassificationRestricted   AssetClassification = "restricted"
)

type AssetCriticality string

const (
	AssetCriticalityCritical AssetCriticality = "critical"
	AssetCriticalityHigh     AssetCriticality = "high"
	AssetCriticalityMedium   AssetCriticality = "medium"
	AssetCriticalityLow      AssetCriticality = "low"
)

type AssetStatus string

const (
	AssetStatusActive         AssetStatus = "active"
	AssetStatusInactive       AssetStatus = "inactive"
	AssetStatusDecommissioned AssetStatus = "decommissioned"
)

// AssetPerson is the minimal non-sensitive owner projection returned by asset
// APIs. Authentication and account-security fields are never embedded.
type AssetPerson struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// Asset is a tenant-owned physical, digital, service, data, facility, or
// people asset used for risk, privacy, and control scoping.
type Asset struct {
	TenantModel
	AssetRef              string              `json:"asset_ref"`
	Name                  string              `json:"name"`
	AssetType             AssetType           `json:"asset_type"`
	Category              string              `json:"category,omitempty"`
	Description           string              `json:"description,omitempty"`
	Criticality           AssetCriticality    `json:"criticality"`
	OwnerUserID           *string             `json:"owner_user_id,omitempty"`
	Owner                 *AssetPerson        `json:"owner,omitempty"`
	Location              string              `json:"location,omitempty"`
	IPAddress             *string             `json:"ip_address,omitempty"`
	Classification        AssetClassification `json:"classification"`
	ProcessesPersonalData bool                `json:"processes_personal_data"`
	LinkedVendorID        *string             `json:"linked_vendor_id,omitempty"`
	Status                AssetStatus         `json:"status"`
	Tags                  []string            `json:"tags"`
	Metadata              json.RawMessage     `json:"metadata"`
	Version               int64               `json:"version"`
	CreatedBy             string              `json:"created_by"`
}

type AssetCreateInput struct {
	Name                  string              `json:"name"`
	AssetType             AssetType           `json:"asset_type"`
	Category              string              `json:"category,omitempty"`
	Description           string              `json:"description,omitempty"`
	Criticality           AssetCriticality    `json:"criticality"`
	OwnerUserID           *string             `json:"owner_user_id,omitempty"`
	Location              string              `json:"location,omitempty"`
	IPAddress             *string             `json:"ip_address,omitempty"`
	Classification        AssetClassification `json:"classification"`
	ProcessesPersonalData bool                `json:"processes_personal_data"`
	LinkedVendorID        *string             `json:"linked_vendor_id,omitempty"`
	Tags                  []string            `json:"tags,omitempty"`
	Metadata              json.RawMessage     `json:"metadata,omitempty"`
}

type AssetPatch struct {
	Name                  *string              `json:"name,omitempty"`
	AssetType             *AssetType           `json:"asset_type,omitempty"`
	Category              *string              `json:"category,omitempty"`
	Description           *string              `json:"description,omitempty"`
	Criticality           *AssetCriticality    `json:"criticality,omitempty"`
	OwnerUserID           *string              `json:"owner_user_id,omitempty"`
	ClearOwner            bool                 `json:"clear_owner,omitempty"`
	Location              *string              `json:"location,omitempty"`
	IPAddress             *string              `json:"ip_address,omitempty"`
	ClearIPAddress        bool                 `json:"clear_ip_address,omitempty"`
	Classification        *AssetClassification `json:"classification,omitempty"`
	ProcessesPersonalData *bool                `json:"processes_personal_data,omitempty"`
	LinkedVendorID        *string              `json:"linked_vendor_id,omitempty"`
	ClearLinkedVendor     bool                 `json:"clear_linked_vendor,omitempty"`
	Status                *AssetStatus         `json:"status,omitempty"`
	Tags                  *[]string            `json:"tags,omitempty"`
	Metadata              json.RawMessage      `json:"metadata,omitempty"`
	ExpectedVersion       *int64               `json:"expected_version,omitempty"`
}

type AssetListFilter struct {
	PaginationRequest
	AssetType             string
	Criticality           string
	Classification        string
	Status                string
	OwnerUserID           string
	Tag                   string
	Search                string
	ProcessesPersonalData *bool
	SortBy                string
	SortDirection         string
}

type AssetStats struct {
	Total        int            `json:"total"`
	Critical     int            `json:"critical"`
	PersonalData int            `json:"personal_data"`
	Active       int            `json:"active"`
	ByType       map[string]int `json:"by_type"`
}

// AssetLifecycleEvent is reserved for an immutable asset history endpoint.
// Keeping the timestamp type here avoids future wire-contract ambiguity.
type AssetLifecycleEvent struct {
	ID        string          `json:"id"`
	AssetID   string          `json:"asset_id"`
	EventType string          `json:"event_type"`
	ActorID   string          `json:"actor_user_id"`
	Version   int64           `json:"asset_version"`
	Details   json.RawMessage `json:"details"`
	CreatedAt time.Time       `json:"created_at"`
}
