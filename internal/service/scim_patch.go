package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/models"
)

var scimMemberFilterPath = regexp.MustCompile(`(?i)^members\[\s*value\s+eq\s+"([0-9a-f-]{36})"\s*\]$`)

func applySCIMUserPatch(user *models.SCIMUser, patch models.SCIMPatchRequest) error {
	for _, operation := range patch.Operations {
		if strings.TrimSpace(operation.Path) == "" {
			expanded, err := expandSCIMAttributeObject(operation)
			if err != nil {
				return err
			}
			if err := applySCIMUserPatch(user, models.SCIMPatchRequest{Operations: expanded}); err != nil {
				return err
			}
			continue
		}
		path := strings.ToLower(strings.TrimSpace(operation.Path))
		remove := strings.EqualFold(operation.Operation, "remove")
		switch path {
		case "username":
			if remove {
				return fmt.Errorf("%w: userName is required", ErrSCIMInvalid)
			}
			if err := decodeSCIMValue(operation.Value, &user.UserName); err != nil {
				return err
			}
		case "externalid":
			if remove {
				user.ExternalID = ""
			} else if err := decodeSCIMValue(operation.Value, &user.ExternalID); err != nil {
				return err
			}
		case "active":
			if remove {
				user.Active = true
			} else if err := decodeSCIMValue(operation.Value, &user.Active); err != nil {
				return err
			}
		case "name.givenname":
			if err := patchSCIMString(operation.Value, remove, &user.Name.GivenName); err != nil {
				return err
			}
		case "name.familyname":
			if err := patchSCIMString(operation.Value, remove, &user.Name.FamilyName); err != nil {
				return err
			}
		case "name.formatted":
			if err := patchSCIMString(operation.Value, remove, &user.Name.Formatted); err != nil {
				return err
			}
		case "name":
			if remove {
				user.Name = models.SCIMName{}
			} else if err := decodeSCIMValue(operation.Value, &user.Name); err != nil {
				return err
			}
		case "displayname":
			if err := patchSCIMString(operation.Value, remove, &user.DisplayName); err != nil {
				return err
			}
		case "nickname":
			if err := patchSCIMString(operation.Value, remove, &user.NickName); err != nil {
				return err
			}
		case "profileurl":
			if err := patchSCIMString(operation.Value, remove, &user.ProfileURL); err != nil {
				return err
			}
		case "title":
			if err := patchSCIMString(operation.Value, remove, &user.Title); err != nil {
				return err
			}
		case "usertype":
			if err := patchSCIMString(operation.Value, remove, &user.UserType); err != nil {
				return err
			}
		case "preferredlanguage":
			if err := patchSCIMString(operation.Value, remove, &user.PreferredLanguage); err != nil {
				return err
			}
		case "locale":
			if err := patchSCIMString(operation.Value, remove, &user.Locale); err != nil {
				return err
			}
		case "timezone":
			if err := patchSCIMString(operation.Value, remove, &user.Timezone); err != nil {
				return err
			}
		case "emails":
			if remove {
				user.Emails = nil
			} else if err := json.Unmarshal(operation.Value, &user.Emails); err != nil {
				return invalidSCIMPatchValue(path)
			}
		case "phonenumbers":
			if remove {
				user.PhoneNumbers = nil
			} else if err := json.Unmarshal(operation.Value, &user.PhoneNumbers); err != nil {
				return invalidSCIMPatchValue(path)
			}
		case "addresses":
			if remove {
				user.Addresses = nil
			} else if err := json.Unmarshal(operation.Value, &user.Addresses); err != nil {
				return invalidSCIMPatchValue(path)
			}
		case "urn:ietf:params:scim:schemas:extension:enterprise:2.0:user:employeenumber",
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:user:costcenter",
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:user:organization",
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:user:division",
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:user:department",
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:user:manager.value":
			if err := patchSCIMEnterpriseString(user, path, operation.Value, remove); err != nil {
				return err
			}
		case strings.ToLower(models.SCIMEnterpriseUserSchema):
			if remove {
				user.Enterprise = nil
				user.Schemas = withoutSCIMSchema(user.Schemas, models.SCIMEnterpriseUserSchema)
			} else {
				var enterprise models.SCIMEnterpriseUser
				if err := decodeSCIMValue(operation.Value, &enterprise); err != nil {
					return err
				}
				user.Enterprise = &enterprise
				if !containsSCIMSchema(user.Schemas, models.SCIMEnterpriseUserSchema) {
					user.Schemas = append(user.Schemas, models.SCIMEnterpriseUserSchema)
				}
			}
		default:
			return fmt.Errorf("%w: unsupported user PATCH path %q", ErrSCIMInvalid, operation.Path)
		}
	}
	return nil
}

func applySCIMGroupPatch(group *models.SCIMGroup, patch models.SCIMPatchRequest) error {
	for _, operation := range patch.Operations {
		if strings.TrimSpace(operation.Path) == "" {
			expanded, err := expandSCIMAttributeObject(operation)
			if err != nil {
				return err
			}
			if err := applySCIMGroupPatch(group, models.SCIMPatchRequest{Operations: expanded}); err != nil {
				return err
			}
			continue
		}
		path := strings.ToLower(strings.TrimSpace(operation.Path))
		remove := strings.EqualFold(operation.Operation, "remove")
		switch path {
		case "displayname":
			if remove {
				return fmt.Errorf("%w: displayName is required", ErrSCIMInvalid)
			}
			if err := decodeSCIMValue(operation.Value, &group.DisplayName); err != nil {
				return err
			}
		case "externalid":
			if remove {
				group.ExternalID = ""
			} else if err := decodeSCIMValue(operation.Value, &group.ExternalID); err != nil {
				return err
			}
		case "members":
			if remove {
				group.Members = nil
				continue
			}
			members, err := decodeSCIMMembers(operation.Value)
			if err != nil {
				return err
			}
			if strings.EqualFold(operation.Operation, "add") {
				group.Members = appendUniqueSCIMMembers(group.Members, members)
			} else {
				group.Members = members
			}
		default:
			match := scimMemberFilterPath.FindStringSubmatch(operation.Path)
			if !remove || len(match) != 2 {
				return fmt.Errorf("%w: unsupported group PATCH path %q", ErrSCIMInvalid, operation.Path)
			}
			if _, err := uuid.Parse(match[1]); err != nil {
				return invalidSCIMPatchValue(operation.Path)
			}
			members := group.Members[:0]
			for _, member := range group.Members {
				if !strings.EqualFold(member.Value, match[1]) {
					members = append(members, member)
				}
			}
			group.Members = members
		}
	}
	return nil
}

// RFC 7644 permits add/replace with an omitted path when value is an object
// whose keys are resource attributes. Expansion keeps the supported surface
// explicit and makes provider payload ordering irrelevant.
func expandSCIMAttributeObject(operation models.SCIMPatchOperation) ([]models.SCIMPatchOperation, error) {
	var attributes map[string]json.RawMessage
	if err := json.Unmarshal(operation.Value, &attributes); err != nil || len(attributes) == 0 {
		return nil, invalidSCIMPatchValue("")
	}
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]models.SCIMPatchOperation, 0, len(keys))
	for _, key := range keys {
		result = append(result, models.SCIMPatchOperation{Operation: operation.Operation,
			Path: key, Value: attributes[key]})
	}
	return result, nil
}

func patchSCIMString(raw json.RawMessage, remove bool, destination *string) error {
	if remove {
		*destination = ""
		return nil
	}
	return decodeSCIMValue(raw, destination)
}

func decodeSCIMValue(raw json.RawMessage, destination any) error {
	if len(raw) == 0 || json.Unmarshal(raw, destination) != nil {
		return invalidSCIMPatchValue("")
	}
	return nil
}

func invalidSCIMPatchValue(path string) error {
	return fmt.Errorf("%w: PATCH value for %q has the wrong shape", ErrSCIMInvalid, path)
}

func patchSCIMEnterpriseString(user *models.SCIMUser, path string, raw json.RawMessage, remove bool) error {
	if user.Enterprise == nil {
		user.Enterprise = &models.SCIMEnterpriseUser{}
		if !containsSCIMSchema(user.Schemas, models.SCIMEnterpriseUserSchema) {
			user.Schemas = append(user.Schemas, models.SCIMEnterpriseUserSchema)
		}
	}
	var destination *string
	switch {
	case strings.HasSuffix(path, ":employeenumber"):
		destination = &user.Enterprise.EmployeeNumber
	case strings.HasSuffix(path, ":costcenter"):
		destination = &user.Enterprise.CostCenter
	case strings.HasSuffix(path, ":organization"):
		destination = &user.Enterprise.Organization
	case strings.HasSuffix(path, ":division"):
		destination = &user.Enterprise.Division
	case strings.HasSuffix(path, ":department"):
		destination = &user.Enterprise.Department
	case strings.HasSuffix(path, ":manager.value"):
		if user.Enterprise.Manager == nil {
			user.Enterprise.Manager = &models.SCIMManager{}
		}
		destination = &user.Enterprise.Manager.Value
	default:
		return fmt.Errorf("%w: unsupported enterprise PATCH path", ErrSCIMInvalid)
	}
	return patchSCIMString(raw, remove, destination)
}

func decodeSCIMMembers(raw json.RawMessage) ([]models.SCIMMember, error) {
	var members []models.SCIMMember
	if err := json.Unmarshal(raw, &members); err == nil {
		return members, nil
	}
	var member models.SCIMMember
	if err := json.Unmarshal(raw, &member); err != nil {
		return nil, invalidSCIMPatchValue("members")
	}
	return []models.SCIMMember{member}, nil
}

func appendUniqueSCIMMembers(existing, additions []models.SCIMMember) []models.SCIMMember {
	seen := make(map[string]struct{}, len(existing)+len(additions))
	result := make([]models.SCIMMember, 0, len(existing)+len(additions))
	for _, member := range append(existing, additions...) {
		key := strings.ToLower(strings.TrimSpace(member.Value))
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, member)
	}
	return result
}

func containsSCIMSchema(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func withoutSCIMSchema(values []string, excluded string) []string {
	result := values[:0]
	for _, value := range values {
		if value != excluded {
			result = append(result, value)
		}
	}
	return result
}
