package models

import "time"

// AssetType classifies the category of an organizational asset.
type AssetType string

const (
	AssetTypeHardware AssetType = "Hardware"
	AssetTypeSoftware AssetType = "Software"
	AssetTypeData     AssetType = "Data"
	AssetTypeNetwork  AssetType = "Network"
	AssetTypePeople   AssetType = "People"
	AssetTypeService  AssetType = "Service"
)

// AssetClassification defines the data sensitivity level of an asset.
type AssetClassification string

const (
	AssetClassificationPublic       AssetClassification = "Public"
	AssetClassificationInternal     AssetClassification = "Internal"
	AssetClassificationConfidential AssetClassification = "Confidential"
	AssetClassificationRestricted   AssetClassification = "Restricted"
)

// AssetCriticality indicates how critical an asset is to business operations.
type AssetCriticality string

const (
	AssetCriticalityCritical AssetCriticality = "Critical"
	AssetCriticalityHigh     AssetCriticality = "High"
	AssetCriticalityMedium   AssetCriticality = "Medium"
	AssetCriticalityLow      AssetCriticality = "Low"
)

// AssetStatus tracks the operational state of an asset.
type AssetStatus string

const (
	AssetStatusActive          AssetStatus = "Active"
	AssetStatusInactive        AssetStatus = "Inactive"
	AssetStatusDecommissioned  AssetStatus = "Decommissioned"
)

// Asset represents a physical, digital, or human asset within the organization's inventory.
type Asset struct {
	TenantModel
	Name           string              `json:"name" gorm:"not null"`
	Type           AssetType           `json:"type" gorm:"type:varchar(50);not null"`
	Description    string              `json:"description" gorm:"type:text"`
	Owner          string              `json:"owner"`
	Location       string              `json:"location"`
	Classification AssetClassification `json:"classification" gorm:"type:varchar(50)"`
	Criticality    AssetCriticality    `json:"criticality" gorm:"type:varchar(50)"`
	Status         AssetStatus         `json:"status" gorm:"type:varchar(50);default:'Active'"`
	IPAddress      *string             `json:"ip_address,omitempty"`
	MACAddress     *string             `json:"mac_address,omitempty"`
	PurchaseDate   *time.Time          `json:"purchase_date,omitempty"`
	EndOfLifeDate  *time.Time          `json:"end_of_life_date,omitempty"`
	Value          *float64            `json:"value,omitempty"`
}
