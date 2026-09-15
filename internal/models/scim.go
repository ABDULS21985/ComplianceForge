package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	SCIMUserSchema             = "urn:ietf:params:scim:schemas:core:2.0:User"
	SCIMEnterpriseUserSchema   = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	SCIMGroupSchema            = "urn:ietf:params:scim:schemas:core:2.0:Group"
	SCIMListResponseSchema     = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SCIMErrorSchema            = "urn:ietf:params:scim:api:messages:2.0:Error"
	SCIMPatchOperationSchema   = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SCIMServiceProviderSchema  = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	SCIMSchemaResourceSchema   = "urn:ietf:params:scim:schemas:core:2.0:Schema"
	SCIMResourceTypeSchema     = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	SCIMTokenScopeUsersRead    = "scim:users:read"   // #nosec G101 -- Public SCIM permission identifier, not a bearer credential.
	SCIMTokenScopeUsersWrite   = "scim:users:write"  // #nosec G101 -- Public SCIM permission identifier, not a bearer credential.
	SCIMTokenScopeGroupsRead   = "scim:groups:read"  // #nosec G101 -- Public SCIM permission identifier, not a bearer credential.
	SCIMTokenScopeGroupsWrite  = "scim:groups:write" // #nosec G101 -- Public SCIM permission identifier, not a bearer credential.
	SCIMDefaultResultsPerPage  = 100
	SCIMMaximumResultsPerPage  = 200
	SCIMMaximumPatchOperations = 100
)

var SCIMAllowedTokenScopes = map[string]bool{
	SCIMTokenScopeUsersRead: true, SCIMTokenScopeUsersWrite: true,
	SCIMTokenScopeGroupsRead: true, SCIMTokenScopeGroupsWrite: true,
}

type SCIMMeta struct {
	ResourceType string    `json:"resourceType"`
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	Location     string    `json:"location"`
	Version      string    `json:"version"`
}

type SCIMName struct {
	Formatted       string `json:"formatted,omitempty"`
	FamilyName      string `json:"familyName,omitempty"`
	GivenName       string `json:"givenName,omitempty"`
	MiddleName      string `json:"middleName,omitempty"`
	HonorificPrefix string `json:"honorificPrefix,omitempty"`
	HonorificSuffix string `json:"honorificSuffix,omitempty"`
}

type SCIMMultiValue struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type SCIMAddress struct {
	Formatted     string `json:"formatted,omitempty"`
	StreetAddress string `json:"streetAddress,omitempty"`
	Locality      string `json:"locality,omitempty"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postalCode,omitempty"`
	Country       string `json:"country,omitempty"`
	Type          string `json:"type,omitempty"`
	Primary       bool   `json:"primary,omitempty"`
}

type SCIMManager struct {
	Value   string `json:"value,omitempty"`
	Ref     string `json:"$ref,omitempty"`
	Display string `json:"displayName,omitempty"`
}

type SCIMEnterpriseUser struct {
	EmployeeNumber string       `json:"employeeNumber,omitempty"`
	CostCenter     string       `json:"costCenter,omitempty"`
	Organization   string       `json:"organization,omitempty"`
	Division       string       `json:"division,omitempty"`
	Department     string       `json:"department,omitempty"`
	Manager        *SCIMManager `json:"manager,omitempty"`
}

type SCIMUser struct {
	Schemas           []string             `json:"schemas"`
	ID                string               `json:"id,omitempty"`
	ExternalID        string               `json:"externalId,omitempty"`
	UserName          string               `json:"userName"`
	Name              SCIMName             `json:"name,omitempty"`
	DisplayName       string               `json:"displayName,omitempty"`
	NickName          string               `json:"nickName,omitempty"`
	ProfileURL        string               `json:"profileUrl,omitempty"`
	Title             string               `json:"title,omitempty"`
	UserType          string               `json:"userType,omitempty"`
	PreferredLanguage string               `json:"preferredLanguage,omitempty"`
	Locale            string               `json:"locale,omitempty"`
	Timezone          string               `json:"timezone,omitempty"`
	Active            bool                 `json:"active"`
	Emails            []SCIMMultiValue     `json:"emails,omitempty"`
	PhoneNumbers      []SCIMMultiValue     `json:"phoneNumbers,omitempty"`
	Addresses         []SCIMAddress        `json:"addresses,omitempty"`
	Groups            []SCIMGroupReference `json:"groups,omitempty"`
	Enterprise        *SCIMEnterpriseUser  `json:"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User,omitempty"`
	Meta              SCIMMeta             `json:"meta,omitempty"`
	Version           int64                `json:"-"`
}

// UnmarshalJSON applies SCIM's default-active semantics while preserving an
// explicit false value. Responses always serialize the resulting boolean.
func (u *SCIMUser) UnmarshalJSON(data []byte) error {
	type alias SCIMUser
	value := alias(*u)
	value.Active = true
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("SCIM user must contain one JSON object")
		}
		return err
	}
	*u = SCIMUser(value)
	return nil
}

type SCIMGroupReference struct {
	Value   string `json:"value"`
	Ref     string `json:"$ref,omitempty"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
}

type SCIMMember struct {
	Value   string `json:"value"`
	Ref     string `json:"$ref,omitempty"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
}

type SCIMGroup struct {
	Schemas     []string     `json:"schemas"`
	ID          string       `json:"id,omitempty"`
	ExternalID  string       `json:"externalId,omitempty"`
	DisplayName string       `json:"displayName"`
	Members     []SCIMMember `json:"members,omitempty"`
	Meta        SCIMMeta     `json:"meta,omitempty"`
	Version     int64        `json:"-"`
}

type SCIMListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
	Resources    any      `json:"Resources"`
}

type SCIMError struct {
	Schemas  []string `json:"schemas"`
	Status   string   `json:"status"`
	SCIMType string   `json:"scimType,omitempty"`
	Detail   string   `json:"detail"`
}

type SCIMPatchRequest struct {
	Schemas    []string             `json:"schemas"`
	Operations []SCIMPatchOperation `json:"Operations"`
}

type SCIMPatchOperation struct {
	Operation string          `json:"op"`
	Path      string          `json:"path,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
}

type SCIMListRequest struct {
	StartIndex int
	Count      int
	Filter     string
}

type SCIMToken struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organization_id"`
	Name               string     `json:"name"`
	Prefix             string     `json:"prefix"`
	Scopes             []string   `json:"scopes"`
	RateLimitPerMinute int        `json:"rate_limit_per_minute"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP         string     `json:"last_used_ip,omitempty"`
	Active             bool       `json:"active"`
	CreatedBy          string     `json:"created_by"`
	RevokedBy          *string    `json:"revoked_by,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	RevokeReason       string     `json:"revoke_reason,omitempty"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	TokenHash          string     `json:"-"`
	PlainToken         string     `json:"-"`
}

type SCIMTokenCreateInput struct {
	Name               string     `json:"name"`
	Scopes             []string   `json:"scopes"`
	RateLimitPerMinute int        `json:"rate_limit_per_minute,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	Reason             string     `json:"reason"`
}

type SCIMTokenRotateInput struct {
	ExpectedVersion int64      `json:"expected_version"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Reason          string     `json:"reason"`
}

type SCIMTokenRevokeInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

type SCIMTokenIssueResult struct {
	Token      SCIMToken `json:"token"`
	Credential string    `json:"credential"`
}

type SCIMTokenEvent struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	TokenID        string          `json:"token_id"`
	EventType      string          `json:"event_type"`
	ActorUserID    string          `json:"actor_user_id"`
	Reason         string          `json:"reason"`
	Version        int64           `json:"version"`
	Details        json.RawMessage `json:"details"`
	CreatedAt      time.Time       `json:"created_at"`
}

type SCIMChangeEvent struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organization_id"`
	TokenID         string          `json:"token_id"`
	ResourceType    string          `json:"resource_type"`
	ResourceID      string          `json:"resource_id"`
	EventType       string          `json:"event_type"`
	ResourceVersion int64           `json:"resource_version"`
	BeforeState     json.RawMessage `json:"before_state,omitempty"`
	AfterState      json.RawMessage `json:"after_state,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

type SCIMServiceProviderConfig struct {
	Schemas               []string                   `json:"schemas"`
	DocumentationURI      string                     `json:"documentationUri,omitempty"`
	Patch                 SCIMSupported              `json:"patch"`
	Bulk                  SCIMBulk                   `json:"bulk"`
	Filter                SCIMFilter                 `json:"filter"`
	ChangePassword        SCIMSupported              `json:"changePassword"`
	Sort                  SCIMSupported              `json:"sort"`
	ETag                  SCIMSupported              `json:"etag"`
	AuthenticationSchemes []SCIMAuthenticationScheme `json:"authenticationSchemes"`
	Meta                  SCIMMeta                   `json:"meta"`
}

type SCIMSupported struct {
	Supported bool `json:"supported"`
}
type SCIMBulk struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
}
type SCIMFilter struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}
type SCIMAuthenticationScheme struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SpecURI     string `json:"specUri,omitempty"`
	Primary     bool   `json:"primary"`
}

type SCIMSchemaDefinition struct {
	Schemas     []string              `json:"schemas"`
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Attributes  []SCIMSchemaAttribute `json:"attributes"`
	Meta        SCIMMeta              `json:"meta,omitempty"`
}

type SCIMSchemaAttribute struct {
	Name            string                `json:"name"`
	Type            string                `json:"type"`
	MultiValued     bool                  `json:"multiValued"`
	Description     string                `json:"description,omitempty"`
	Required        bool                  `json:"required"`
	CaseExact       bool                  `json:"caseExact"`
	Mutability      string                `json:"mutability"`
	Returned        string                `json:"returned"`
	Uniqueness      string                `json:"uniqueness"`
	CanonicalValues []string              `json:"canonicalValues,omitempty"`
	SubAttributes   []SCIMSchemaAttribute `json:"subAttributes,omitempty"`
}

type SCIMResourceType struct {
	Schemas          []string              `json:"schemas"`
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	Endpoint         string                `json:"endpoint"`
	Description      string                `json:"description"`
	Schema           string                `json:"schema"`
	SchemaExtensions []SCIMSchemaExtension `json:"schemaExtensions,omitempty"`
	Meta             SCIMMeta              `json:"meta,omitempty"`
}

type SCIMSchemaExtension struct {
	Schema   string `json:"schema"`
	Required bool   `json:"required"`
}
