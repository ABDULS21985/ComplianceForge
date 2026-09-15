package scim

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrInvalidSyntax = errors.New("invalid SCIM syntax")
	ErrInvalidValue  = errors.New("invalid SCIM value")
)

const (
	ErrorTypeInvalidFilter = "invalidFilter"
	ErrorTypeInvalidSyntax = "invalidSyntax"
	ErrorTypeInvalidValue  = "invalidValue"
	ErrorTypeUniqueness    = "uniqueness"
	ErrorTypeMutability    = "mutability"
	ErrorTypeNoTarget      = "noTarget"
)

var languagePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)

var discoveryUpdatedAt = time.Date(2026, time.September, 14, 0, 0, 0, 0, time.UTC)

func VersionETag(version int64) string { return `W/"` + strconv.FormatInt(version, 10) + `"` }

func ParseVersionETag(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "W/") {
		value = strings.TrimSpace(strings.TrimPrefix(value, "W/"))
	}
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' || strings.Contains(value[1:len(value)-1], `"`) {
		return 0, fmt.Errorf("%w: If-Match must contain one SCIM version ETag", ErrInvalidSyntax)
	}
	version, err := strconv.ParseInt(value[1:len(value)-1], 10, 64)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("%w: If-Match version is invalid", ErrInvalidSyntax)
	}
	return version, nil
}

func NormalizeListRequest(startIndex, count int, filter string) (models.SCIMListRequest, error) {
	if startIndex == 0 {
		startIndex = 1
	}
	if startIndex < 1 || count < 0 {
		return models.SCIMListRequest{}, fmt.Errorf("%w: startIndex must be positive and count cannot be negative", ErrInvalidValue)
	}
	if count > models.SCIMMaximumResultsPerPage {
		count = models.SCIMMaximumResultsPerPage
	}
	filter = strings.TrimSpace(filter)
	if len(filter) > maximumFilterBytes {
		return models.SCIMListRequest{}, fmt.Errorf("%w: filter is too long", ErrInvalidFilter)
	}
	return models.SCIMListRequest{StartIndex: startIndex, Count: count, Filter: filter}, nil
}

func NormalizeUser(user *models.SCIMUser) error {
	if user == nil {
		return fmt.Errorf("%w: user document is required", ErrInvalidValue)
	}
	if err := validateSchemas(user.Schemas, models.SCIMUserSchema, user.Enterprise != nil); err != nil {
		return err
	}
	user.Schemas = []string{models.SCIMUserSchema}
	if user.Enterprise != nil {
		user.Schemas = append(user.Schemas, models.SCIMEnterpriseUserSchema)
	}
	user.ID = ""
	user.Meta = models.SCIMMeta{}
	user.Groups = nil
	user.UserName = strings.ToLower(strings.TrimSpace(user.UserName))
	user.ExternalID = strings.TrimSpace(user.ExternalID)
	user.Name.Formatted = strings.TrimSpace(user.Name.Formatted)
	user.Name.FamilyName = strings.TrimSpace(user.Name.FamilyName)
	user.Name.GivenName = strings.TrimSpace(user.Name.GivenName)
	user.Name.MiddleName = strings.TrimSpace(user.Name.MiddleName)
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	user.NickName = strings.TrimSpace(user.NickName)
	user.ProfileURL = strings.TrimSpace(user.ProfileURL)
	user.Title = strings.TrimSpace(user.Title)
	user.UserType = strings.TrimSpace(user.UserType)
	user.PreferredLanguage = strings.TrimSpace(user.PreferredLanguage)
	user.Locale = strings.TrimSpace(user.Locale)
	user.Timezone = strings.TrimSpace(user.Timezone)
	if !validEmail(user.UserName) || runeLength(user.UserName) > 320 || runeLength(user.ExternalID) > 255 ||
		!boundedStrings(100, user.Name.FamilyName, user.Name.GivenName) ||
		!boundedStrings(255, user.Name.Formatted, user.Name.MiddleName, user.DisplayName, user.NickName, user.UserType) ||
		runeLength(user.Title) > 200 {
		return fmt.Errorf("%w: userName must be an email and user attributes must be within their limits", ErrInvalidValue)
	}
	if user.ProfileURL != "" {
		parsed, err := url.ParseRequestURI(user.ProfileURL)
		if err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" {
			return fmt.Errorf("%w: profileUrl must be an HTTP(S) URI", ErrInvalidValue)
		}
	}
	if user.PreferredLanguage != "" && !languagePattern.MatchString(user.PreferredLanguage) ||
		user.Locale != "" && !languagePattern.MatchString(user.Locale) {
		return fmt.Errorf("%w: language and locale values are invalid", ErrInvalidValue)
	}
	if runeLength(user.Timezone) > 50 {
		return fmt.Errorf("%w: timezone is too long", ErrInvalidValue)
	}
	if user.Timezone != "" {
		if _, err := time.LoadLocation(user.Timezone); err != nil {
			return fmt.Errorf("%w: timezone must be an IANA location", ErrInvalidValue)
		}
	}
	if err := normalizeMultiValues(user); err != nil {
		return err
	}
	if user.Enterprise != nil {
		if err := normalizeEnterprise(user.Enterprise); err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(user)
	if err != nil || len(encoded) > 120*1024 {
		return fmt.Errorf("%w: user profile exceeds the supported size", ErrInvalidValue)
	}
	return nil
}

func NormalizeGroup(group *models.SCIMGroup) error {
	if group == nil {
		return fmt.Errorf("%w: group document is required", ErrInvalidValue)
	}
	if err := validateSchemas(group.Schemas, models.SCIMGroupSchema, false); err != nil {
		return err
	}
	group.Schemas = []string{models.SCIMGroupSchema}
	group.ID = ""
	group.Meta = models.SCIMMeta{}
	group.DisplayName = strings.TrimSpace(group.DisplayName)
	group.ExternalID = strings.TrimSpace(group.ExternalID)
	if runeLength(group.DisplayName) < 2 || runeLength(group.DisplayName) > 160 || runeLength(group.ExternalID) > 255 {
		return fmt.Errorf("%w: displayName or externalId is invalid", ErrInvalidValue)
	}
	if len(group.Members) > 1000 {
		return fmt.Errorf("%w: too many group members", ErrInvalidValue)
	}
	seen := make(map[string]struct{}, len(group.Members))
	for index := range group.Members {
		member := &group.Members[index]
		member.Value = strings.TrimSpace(member.Value)
		member.Display = ""
		member.Ref = ""
		if _, err := uuid.Parse(member.Value); err != nil || member.Type != "" && !strings.EqualFold(member.Type, "User") {
			return fmt.Errorf("%w: group members must reference User UUIDs", ErrInvalidValue)
		}
		member.Type = "User"
		if _, duplicate := seen[member.Value]; duplicate {
			return fmt.Errorf("%w: duplicate group member", ErrInvalidValue)
		}
		seen[member.Value] = struct{}{}
	}
	return nil
}

func ValidatePatchRequest(request models.SCIMPatchRequest) error {
	if len(request.Schemas) != 1 || request.Schemas[0] != models.SCIMPatchOperationSchema {
		return fmt.Errorf("%w: PatchOp schemas value is required", ErrInvalidSyntax)
	}
	if len(request.Operations) == 0 || len(request.Operations) > models.SCIMMaximumPatchOperations {
		return fmt.Errorf("%w: Operations must contain between 1 and %d entries", ErrInvalidValue, models.SCIMMaximumPatchOperations)
	}
	for index := range request.Operations {
		operation := &request.Operations[index]
		operation.Operation = strings.ToLower(strings.TrimSpace(operation.Operation))
		operation.Path = strings.TrimSpace(operation.Path)
		if operation.Operation != "add" && operation.Operation != "replace" && operation.Operation != "remove" {
			return fmt.Errorf("%w: operation %d has an unsupported op", ErrInvalidSyntax, index)
		}
		if operation.Operation != "remove" && len(operation.Value) == 0 {
			return fmt.Errorf("%w: operation %d requires a value", ErrInvalidSyntax, index)
		}
		if operation.Path == "" {
			if operation.Operation == "remove" {
				return fmt.Errorf("%w: pathless remove operation %d is ambiguous", ErrInvalidSyntax, index)
			}
			var attributes map[string]json.RawMessage
			if json.Unmarshal(operation.Value, &attributes) != nil || len(attributes) == 0 {
				return fmt.Errorf("%w: pathless operation %d requires an attribute object", ErrInvalidSyntax, index)
			}
		}
		if len(operation.Value) > 256*1024 || len(operation.Path) > 512 || !json.Valid(defaultJSON(operation.Value)) {
			return fmt.Errorf("%w: operation %d is malformed or too large", ErrInvalidSyntax, index)
		}
	}
	return nil
}

func ServiceProviderConfig() models.SCIMServiceProviderConfig {
	return models.SCIMServiceProviderConfig{
		Schemas:          []string{models.SCIMServiceProviderSchema},
		DocumentationURI: "https://www.rfc-editor.org/rfc/rfc7644",
		Patch:            models.SCIMSupported{Supported: true},
		Bulk:             models.SCIMBulk{Supported: false, MaxOperations: 0, MaxPayloadSize: 0},
		Filter:           models.SCIMFilter{Supported: true, MaxResults: models.SCIMMaximumResultsPerPage},
		ChangePassword:   models.SCIMSupported{Supported: false}, Sort: models.SCIMSupported{Supported: false},
		ETag: models.SCIMSupported{Supported: true},
		AuthenticationSchemes: []models.SCIMAuthenticationScheme{{
			Type: "oauthbearertoken", Name: "SCIM bearer token",
			Description: "Tenant-bound, hashed, expiring bearer token with resource-specific scopes.",
			SpecURI:     "https://www.rfc-editor.org/rfc/rfc6750", Primary: true,
		}},
		Meta: discoveryMeta("ServiceProviderConfig", "/api/scim/v2/ServiceProviderConfig"),
	}
}

func SchemaDefinitions() []models.SCIMSchemaDefinition {
	readWrite := func(name, kind string, required, multi, caseExact bool, uniqueness string) models.SCIMSchemaAttribute {
		return models.SCIMSchemaAttribute{Name: name, Type: kind, Required: required, MultiValued: multi,
			CaseExact: caseExact, Mutability: "readWrite", Returned: "default", Uniqueness: uniqueness}
	}
	return []models.SCIMSchemaDefinition{
		{Schemas: []string{models.SCIMSchemaResourceSchema}, ID: models.SCIMUserSchema, Name: "User",
			Description: "Core SCIM user account", Attributes: []models.SCIMSchemaAttribute{
				readWrite("userName", "string", true, false, false, "server"),
				readWrite("externalId", "string", false, false, true, "server"),
				readWrite("active", "boolean", false, false, false, "none"),
				readWrite("name", "complex", false, false, false, "none"),
				readWrite("displayName", "string", false, false, false, "none"),
				readWrite("emails", "complex", false, true, false, "none"),
				readWrite("phoneNumbers", "complex", false, true, false, "none"),
			}, Meta: discoveryMeta("Schema", "/api/scim/v2/Schemas/"+models.SCIMUserSchema)},
		{Schemas: []string{models.SCIMSchemaResourceSchema}, ID: models.SCIMEnterpriseUserSchema, Name: "EnterpriseUser",
			Description: "Enterprise user extension", Attributes: []models.SCIMSchemaAttribute{
				readWrite("employeeNumber", "string", false, false, true, "server"),
				readWrite("costCenter", "string", false, false, false, "none"),
				readWrite("organization", "string", false, false, false, "none"),
				readWrite("division", "string", false, false, false, "none"),
				readWrite("department", "string", false, false, false, "none"),
				readWrite("manager", "complex", false, false, false, "none"),
			}, Meta: discoveryMeta("Schema", "/api/scim/v2/Schemas/"+models.SCIMEnterpriseUserSchema)},
		{Schemas: []string{models.SCIMSchemaResourceSchema}, ID: models.SCIMGroupSchema, Name: "Group",
			Description: "Core SCIM group", Attributes: []models.SCIMSchemaAttribute{
				readWrite("displayName", "string", true, false, false, "server"),
				readWrite("externalId", "string", false, false, true, "server"),
				readWrite("members", "complex", false, true, false, "none"),
			}, Meta: discoveryMeta("Schema", "/api/scim/v2/Schemas/"+models.SCIMGroupSchema)},
	}
}

func ResourceTypes() []models.SCIMResourceType {
	return []models.SCIMResourceType{
		{Schemas: []string{models.SCIMResourceTypeSchema}, ID: "User", Name: "User", Endpoint: "/Users",
			Description: "Tenant user", Schema: models.SCIMUserSchema,
			SchemaExtensions: []models.SCIMSchemaExtension{{Schema: models.SCIMEnterpriseUserSchema}},
			Meta:             discoveryMeta("ResourceType", "/api/scim/v2/ResourceTypes/User")},
		{Schemas: []string{models.SCIMResourceTypeSchema}, ID: "Group", Name: "Group", Endpoint: "/Groups",
			Description: "Tenant static group", Schema: models.SCIMGroupSchema,
			Meta: discoveryMeta("ResourceType", "/api/scim/v2/ResourceTypes/Group")},
	}
}

func discoveryMeta(resourceType, location string) models.SCIMMeta {
	return models.SCIMMeta{ResourceType: resourceType, Created: discoveryUpdatedAt,
		LastModified: discoveryUpdatedAt, Location: location, Version: VersionETag(1)}
}

func validateSchemas(schemas []string, core string, enterprise bool) error {
	seen := make(map[string]struct{}, len(schemas))
	for _, schema := range schemas {
		if schema != core && schema != models.SCIMEnterpriseUserSchema {
			return fmt.Errorf("%w: unsupported schemas value %q", ErrInvalidValue, schema)
		}
		seen[schema] = struct{}{}
	}
	if _, ok := seen[core]; !ok {
		return fmt.Errorf("%w: core schema %q is required", ErrInvalidValue, core)
	}
	_, hasEnterprise := seen[models.SCIMEnterpriseUserSchema]
	if hasEnterprise != enterprise {
		return fmt.Errorf("%w: enterprise extension presence must match schemas", ErrInvalidValue)
	}
	return nil
}

func normalizeMultiValues(user *models.SCIMUser) error {
	if len(user.Emails) > 20 || len(user.PhoneNumbers) > 20 || len(user.Addresses) > 10 {
		return fmt.Errorf("%w: too many multi-valued attributes", ErrInvalidValue)
	}
	primaryEmails := 0
	for index := range user.Emails {
		value := &user.Emails[index]
		value.Value = strings.ToLower(strings.TrimSpace(value.Value))
		value.Type = strings.ToLower(strings.TrimSpace(value.Type))
		value.Display = strings.TrimSpace(value.Display)
		if !validEmail(value.Value) || !boundedStrings(255, value.Type, value.Display) {
			return fmt.Errorf("%w: email attribute is invalid", ErrInvalidValue)
		}
		if value.Primary {
			primaryEmails++
		}
	}
	if primaryEmails > 1 {
		return fmt.Errorf("%w: at most one email may be primary", ErrInvalidValue)
	}
	for index := range user.PhoneNumbers {
		value := &user.PhoneNumbers[index]
		value.Value = strings.TrimSpace(value.Value)
		value.Type = strings.ToLower(strings.TrimSpace(value.Type))
		value.Display = strings.TrimSpace(value.Display)
		if runeLength(value.Value) < 3 || runeLength(value.Value) > 50 || !boundedStrings(255, value.Type, value.Display) {
			return fmt.Errorf("%w: phone number attribute is invalid", ErrInvalidValue)
		}
	}
	for index := range user.Addresses {
		address := &user.Addresses[index]
		address.Formatted = strings.TrimSpace(address.Formatted)
		address.StreetAddress = strings.TrimSpace(address.StreetAddress)
		address.Locality = strings.TrimSpace(address.Locality)
		address.Region = strings.TrimSpace(address.Region)
		address.PostalCode = strings.TrimSpace(address.PostalCode)
		address.Country = strings.ToUpper(strings.TrimSpace(address.Country))
		address.Type = strings.ToLower(strings.TrimSpace(address.Type))
		if !boundedStrings(500, address.Formatted, address.StreetAddress, address.Locality, address.Region, address.PostalCode) ||
			address.Country != "" && len(address.Country) != 2 || runeLength(address.Type) > 50 {
			return fmt.Errorf("%w: address attribute is invalid", ErrInvalidValue)
		}
	}
	return nil
}

func normalizeEnterprise(extension *models.SCIMEnterpriseUser) error {
	extension.EmployeeNumber = strings.TrimSpace(extension.EmployeeNumber)
	extension.CostCenter = strings.TrimSpace(extension.CostCenter)
	extension.Organization = strings.TrimSpace(extension.Organization)
	extension.Division = strings.TrimSpace(extension.Division)
	extension.Department = strings.TrimSpace(extension.Department)
	if runeLength(extension.EmployeeNumber) > 100 ||
		!boundedStrings(200, extension.CostCenter, extension.Organization, extension.Division, extension.Department) {
		return fmt.Errorf("%w: enterprise attributes are too long", ErrInvalidValue)
	}
	if extension.Manager != nil {
		extension.Manager.Value = strings.TrimSpace(extension.Manager.Value)
		extension.Manager.Ref = ""
		extension.Manager.Display = ""
		if extension.Manager.Value != "" {
			if _, err := uuid.Parse(extension.Manager.Value); err != nil {
				return fmt.Errorf("%w: enterprise manager value must be a UUID", ErrInvalidValue)
			}
		}
	}
	return nil
}

func validEmail(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

func boundedStrings(maximum int, values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maximum {
			return false
		}
	}
	return true
}

func runeLength(value string) int { return utf8.RuneCountInString(value) }

func defaultJSON(value json.RawMessage) []byte {
	if len(value) == 0 {
		return []byte("null")
	}
	return value
}
