package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/models"
)

// AccessFieldVisibilityObligation is emitted by the policy decision point for
// every effective field-level rule. Response serializers accept this exact
// versioned shape and fail closed when a rule is incomplete or inconsistent.
const AccessFieldVisibilityObligation = "field_visibility"

// MaskAccessFields applies already-authorized field obligations to a copy of
// JSON-shaped data. Dot paths traverse nested objects and every object inside
// arrays. The caller's map is never mutated.
func MaskAccessFields(input map[string]any, permissions []models.AccessFieldPermission) map[string]any {
	result := cloneAccessValue(input).(map[string]any)
	for _, permission := range permissions {
		parts := splitFieldPath(permission.FieldPath)
		if len(parts) == 0 {
			continue
		}
		applyFieldPermission(result, parts, permission)
	}
	return result
}

// MaskAccessResponse converts any JSON-serializable response into an owned
// JSON-shaped value and applies field rules at every resource object reachable
// through nested maps and arrays. It cannot mutate shared repository/model
// instances because the response is cloned through a serialization boundary
// before masking.
func MaskAccessResponse(input any, permissions []models.AccessFieldPermission) (any, error) {
	if len(permissions) == 0 {
		return input, nil
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("clone classified response: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var cloned any
	if err := decoder.Decode(&cloned); err != nil {
		return nil, fmt.Errorf("decode classified response: %w", err)
	}
	if decoder.More() {
		return nil, errors.New("decode classified response: unexpected trailing value")
	}
	applyAccessPermissionsRecursively(cloned, permissions)
	if _, err := json.Marshal(cloned); err != nil {
		return nil, fmt.Errorf("validate masked response: %w", err)
	}
	return cloned, nil
}

func applyAccessPermissionsRecursively(value any, permissions []models.AccessFieldPermission) {
	switch typed := value.(type) {
	case map[string]any:
		for _, permission := range permissions {
			parts := splitFieldPath(permission.FieldPath)
			if len(parts) > 0 {
				applyFieldPermission(typed, parts, permission)
			}
		}
		for _, nested := range typed {
			applyAccessPermissionsRecursively(nested, permissions)
		}
	case []any:
		for _, nested := range typed {
			applyAccessPermissionsRecursively(nested, permissions)
		}
	}
}

// AccessFieldObligations produces the transport-safe subset of an evaluated
// field policy. No classified values are included in decision metadata.
func AccessFieldObligations(permissions []models.AccessFieldPermission) []authz.Obligation {
	result := make([]authz.Obligation, 0, len(permissions))
	for _, permission := range permissions {
		result = append(result, authz.Obligation{Kind: AccessFieldVisibilityObligation, Parameters: map[string]string{
			"id": permission.ID, "policy_id": permission.PolicyID,
			"policy_priority": strconv.Itoa(permission.PolicyPriority),
			"resource_type":   canonicalAccessResource(permission.ResourceType),
			"field_path":      permission.FieldPath,
			"classification":  string(permission.Classification),
			"visibility":      string(permission.Visibility),
			"mask_strategy":   string(permission.MaskStrategy),
			"mask_pattern":    permission.MaskPattern,
		}})
	}
	return result
}

// AccessFieldsFromObligations validates decision metadata before a handler is
// allowed to serialize classified data. A malformed, cross-resource, or
// unsupported field rule is an authorization integrity failure.
func AccessFieldsFromObligations(obligations []authz.Obligation, resourceType string) ([]models.AccessFieldPermission, error) {
	resourceType = canonicalAccessResource(resourceType)
	result := make([]models.AccessFieldPermission, 0)
	for _, obligation := range obligations {
		if obligation.Kind != AccessFieldVisibilityObligation {
			continue
		}
		parameters := obligation.Parameters
		if parameters == nil {
			return nil, errors.New("field visibility obligation parameters are required")
		}
		priority, err := strconv.Atoi(parameters["policy_priority"])
		if err != nil || priority < 0 || priority > 10000 {
			return nil, errors.New("field visibility obligation priority is invalid")
		}
		permission := models.AccessFieldPermission{
			ID:             strings.TrimSpace(parameters["id"]),
			PolicyID:       strings.TrimSpace(parameters["policy_id"]),
			PolicyPriority: priority,
			ResourceType:   canonicalAccessResource(parameters["resource_type"]),
			FieldPath:      strings.TrimSpace(parameters["field_path"]),
			Classification: models.AccessFieldClassification(parameters["classification"]),
			Visibility:     models.AccessFieldVisibility(parameters["visibility"]),
			MaskStrategy:   models.AccessMaskStrategy(parameters["mask_strategy"]),
			MaskPattern:    parameters["mask_pattern"],
		}
		if permission.ID == "" || permission.PolicyID == "" || permission.ResourceType == "" || permission.ResourceType != resourceType {
			return nil, errors.New("field visibility obligation scope is invalid")
		}
		if err := ValidateAccessFieldPermission(permission); err != nil {
			return nil, fmt.Errorf("field visibility obligation is invalid: %w", err)
		}
		result = append(result, permission)
	}
	return result, nil
}

func applyFieldPermission(object map[string]any, parts []string, permission models.AccessFieldPermission) {
	value, exists := object[parts[0]]
	if !exists {
		return
	}
	if len(parts) == 1 {
		switch permission.Visibility {
		case models.AccessFieldHidden:
			delete(object, parts[0])
		case models.AccessFieldMasked:
			object[parts[0]] = maskAccessValue(value, permission.MaskStrategy, permission.MaskPattern)
		}
		return
	}
	applyNestedFieldPermission(value, parts[1:], permission)
}

func applyNestedFieldPermission(value any, parts []string, permission models.AccessFieldPermission) {
	switch typed := value.(type) {
	case map[string]any:
		applyFieldPermission(typed, parts, permission)
	case []any:
		for _, item := range typed {
			applyNestedFieldPermission(item, parts, permission)
		}
	case []map[string]any:
		for _, item := range typed {
			applyFieldPermission(item, parts, permission)
		}
	}
}

func splitFieldPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
		return nil
	}
	parts := strings.Split(path, ".")
	for _, part := range parts {
		if part == "" || len(part) > 100 {
			return nil
		}
		for _, character := range part {
			if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9') || character == '_' || character == '-' {
				continue
			}
			return nil
		}
	}
	return parts
}

func cloneAccessValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = cloneAccessValue(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = cloneAccessValue(item)
		}
		return result
	case []map[string]any:
		result := make([]map[string]any, len(typed))
		for index, item := range typed {
			result[index] = cloneAccessValue(item).(map[string]any)
		}
		return result
	default:
		return value
	}
}

func maskAccessValue(value any, strategy models.AccessMaskStrategy, pattern string) any {
	switch typed := value.(type) {
	case string:
		return maskAccessString(typed, strategy, pattern)
	case []any:
		masked := make([]any, len(typed))
		for index, item := range typed {
			masked[index] = maskAccessValue(item, strategy, pattern)
		}
		return masked
	case []string:
		masked := make([]string, len(typed))
		for index, item := range typed {
			masked[index] = maskAccessString(item, strategy, pattern)
		}
		return masked
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, item := range typed {
			masked[key] = maskAccessValue(item, strategy, pattern)
		}
		return masked
	case bool:
		return false
	case int:
		return int(0)
	case int8:
		return int8(0)
	case int16:
		return int16(0)
	case int32:
		return int32(0)
	case int64:
		return int64(0)
	case uint:
		return uint(0)
	case uint8:
		return uint8(0)
	case uint16:
		return uint16(0)
	case uint32:
		return uint32(0)
	case uint64:
		return uint64(0)
	case float32:
		return float32(0)
	case float64:
		return float64(0)
	case json.Number:
		return json.Number("0")
	case nil:
		return nil
	default:
		reflected := reflect.ValueOf(value)
		if !reflected.IsValid() {
			return nil
		}
		return reflect.Zero(reflected.Type()).Interface()
	}
}

func maskAccessString(value string, strategy models.AccessMaskStrategy, pattern string) string {
	switch strategy {
	case models.AccessMaskLast4:
		runes := []rune(value)
		if len(runes) <= 4 {
			return strings.Repeat("*", len(runes))
		}
		return strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-4:])
	case models.AccessMaskEmail:
		at := strings.LastIndex(value, "@")
		if at <= 0 || at == len(value)-1 {
			return "***"
		}
		local := []rune(value[:at])
		if len(local) == 0 {
			return "***@" + value[at+1:]
		}
		return string(local[0]) + "***@" + value[at+1:]
	case models.AccessMaskCustom:
		if strings.TrimSpace(pattern) == "" {
			return "***"
		}
		runes := []rune(value)
		first := ""
		last4 := ""
		if len(runes) > 0 {
			first = string(runes[0])
		}
		if len(runes) <= 4 {
			last4 = strings.Repeat("*", len(runes))
		} else {
			last4 = string(runes[len(runes)-4:])
		}
		return strings.NewReplacer("{first1}", first, "{last4}", last4).Replace(pattern)
	default:
		if utf8.RuneCountInString(value) == 0 {
			return ""
		}
		return "***"
	}
}
