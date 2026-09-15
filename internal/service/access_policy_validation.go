package service

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/models"
)

var accessComponentPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

var supportedAccessResources = map[string]struct{}{
	"*": {}, "organizations": {}, "frameworks": {}, "controls": {}, "risks": {},
	"policies": {}, "audits": {}, "findings": {}, "incidents": {}, "assets": {},
	"vendors": {}, "reports": {}, "users": {}, "settings": {}, "evidence": {},
}

var supportedAccessActions = map[string]struct{}{
	"*": {}, "create": {}, "read": {}, "update": {}, "delete": {}, "approve": {},
	"assign": {}, "export": {}, "configure": {},
}

var subjectConditionAttributes = map[string]struct{}{
	"id": {}, "roles": {}, "department": {}, "location": {}, "status": {}, "super_admin": {},
}

var resourceConditionAttributes = map[string]struct{}{
	"id": {}, "type": {}, "owner_id": {}, "department": {}, "location": {}, "classification": {},
}

var environmentConditionAttributes = map[string]struct{}{
	"mfa_verified": {}, "ip_address": {}, "hour": {}, "day_of_week": {},
}

func ValidateAccessPolicyInput(input models.AccessPolicyInput) error {
	if err := validateBoundedTrimmed("name", input.Name, 2, 200); err != nil {
		return invalidAccessPolicy(err)
	}
	if input.Description != "" {
		if err := validateBoundedTrimmed("description", input.Description, 3, 4000); err != nil {
			return invalidAccessPolicy(err)
		}
	}
	if input.Priority < 0 || input.Priority > 10000 {
		return invalidAccessPolicy(errors.New("priority must be between 0 and 10000"))
	}
	if input.Effect != models.AccessPolicyEffectAllow && input.Effect != models.AccessPolicyEffectDeny {
		return invalidAccessPolicy(errors.New("effect must be allow or deny"))
	}
	resource := canonicalAccessResource(input.ResourceType)
	if _, ok := supportedAccessResources[resource]; !ok {
		return invalidAccessPolicy(fmt.Errorf("unsupported resource_type %q", input.ResourceType))
	}
	if len(input.Actions) == 0 || len(input.Actions) > 16 {
		return invalidAccessPolicy(errors.New("actions must contain between 1 and 16 values"))
	}
	seenActions := make(map[string]struct{}, len(input.Actions))
	for _, action := range input.Actions {
		action = strings.ToLower(strings.TrimSpace(action))
		if _, ok := supportedAccessActions[action]; !ok {
			return invalidAccessPolicy(fmt.Errorf("unsupported action %q", action))
		}
		if _, exists := seenActions[action]; exists {
			return invalidAccessPolicy(fmt.Errorf("duplicate action %q", action))
		}
		seenActions[action] = struct{}{}
	}
	if _, wildcard := seenActions["*"]; wildcard && len(seenActions) != 1 {
		return invalidAccessPolicy(errors.New("wildcard action must be the only action"))
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && !input.ValidUntil.After(*input.ValidFrom) {
		return invalidAccessPolicy(errors.New("valid_until must be after valid_from"))
	}
	if err := validateAccessConditions("subject_conditions", input.SubjectConditions, subjectConditionAttributes, "subject"); err != nil {
		return invalidAccessPolicy(err)
	}
	if err := validateAccessConditions("resource_conditions", input.ResourceConditions, resourceConditionAttributes, "resource"); err != nil {
		return invalidAccessPolicy(err)
	}
	if err := validateAccessConditions("environment_conditions", input.EnvironmentConditions, environmentConditionAttributes, "environment"); err != nil {
		return invalidAccessPolicy(err)
	}
	if err := validateChangeReason(input.Reason); err != nil {
		return invalidAccessPolicy(err)
	}
	if input.ExpectedVersion != nil && *input.ExpectedVersion < 1 {
		return invalidAccessPolicy(errors.New("expected_version must be positive"))
	}
	return nil
}

func ValidateAccessPolicyAssignmentInput(input models.AccessPolicyAssignmentInput) error {
	switch input.AssigneeType {
	case models.AccessAssigneeAllUsers:
		if input.AssigneeID != nil && strings.TrimSpace(*input.AssigneeID) != "" {
			return invalidAccessPolicy(errors.New("assignee_id must be omitted for all_users"))
		}
	case models.AccessAssigneeUser, models.AccessAssigneeRole, models.AccessAssigneeGroup:
		if input.AssigneeID == nil {
			return invalidAccessPolicy(errors.New("assignee_id is required"))
		}
		if _, err := uuid.Parse(strings.TrimSpace(*input.AssigneeID)); err != nil {
			return invalidAccessPolicy(errors.New("assignee_id must be a UUID"))
		}
	default:
		return invalidAccessPolicy(errors.New("assignee_type must be user, role, group, or all_users"))
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && !input.ValidUntil.After(*input.ValidFrom) {
		return invalidAccessPolicy(errors.New("valid_until must be after valid_from"))
	}
	if err := validateChangeReason(input.Reason); err != nil {
		return invalidAccessPolicy(err)
	}
	return nil
}

func ValidateAccessObjectGrantInput(input models.AccessObjectGrantInput) error {
	for name, value := range map[string]string{
		"subject_id": input.SubjectID, "resource_id": input.ResourceID, "sponsor_id": input.SponsorID,
	} {
		if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
			return invalidAccessPolicy(fmt.Errorf("%s must be a UUID", name))
		}
	}
	if input.SubjectID == input.SponsorID {
		return invalidAccessPolicy(errors.New("sponsor_id must differ from subject_id"))
	}
	resource := canonicalAccessResource(input.ResourceType)
	if resource == "*" {
		return invalidAccessPolicy(errors.New("object grants require a concrete resource_type"))
	}
	if _, ok := supportedAccessResources[resource]; !ok {
		return invalidAccessPolicy(fmt.Errorf("unsupported resource_type %q", input.ResourceType))
	}
	if len(input.Actions) == 0 || len(input.Actions) > 16 {
		return invalidAccessPolicy(errors.New("actions must contain between 1 and 16 values"))
	}
	seenActions := make(map[string]struct{}, len(input.Actions))
	for _, action := range input.Actions {
		action = strings.ToLower(strings.TrimSpace(action))
		if action == "*" {
			return invalidAccessPolicy(errors.New("object grants do not support wildcard actions"))
		}
		if _, ok := supportedAccessActions[action]; !ok {
			return invalidAccessPolicy(fmt.Errorf("unsupported action %q", action))
		}
		if _, exists := seenActions[action]; exists {
			return invalidAccessPolicy(fmt.Errorf("duplicate action %q", action))
		}
		seenActions[action] = struct{}{}
	}
	if input.ValidFrom.IsZero() || input.ValidUntil.IsZero() || !input.ValidUntil.After(input.ValidFrom) {
		return invalidAccessPolicy(errors.New("valid_until must be after a non-zero valid_from"))
	}
	if input.RequireWatermark && !input.AllowDownload {
		return invalidAccessPolicy(errors.New("watermarking requires downloads to be enabled"))
	}
	if input.RequireWatermark {
		if err := validateBoundedTrimmed("watermark_text", input.WatermarkText, 3, 200); err != nil {
			return invalidAccessPolicy(err)
		}
	} else if input.WatermarkText != "" {
		return invalidAccessPolicy(errors.New("watermark_text requires require_watermark"))
	}
	if err := validateChangeReason(input.Reason); err != nil {
		return invalidAccessPolicy(err)
	}
	if input.ExpectedVersion != nil && *input.ExpectedVersion < 1 {
		return invalidAccessPolicy(errors.New("expected_version must be positive"))
	}
	if input.ApprovedAt != nil && input.ApprovedBy == nil {
		return invalidAccessPolicy(errors.New("approved_at requires approved_by"))
	}
	if input.ApprovedBy != nil {
		if _, err := uuid.Parse(strings.TrimSpace(*input.ApprovedBy)); err != nil {
			return invalidAccessPolicy(errors.New("approved_by must be a UUID"))
		}
	}
	return nil
}

func ValidateAccessFieldPermission(rule models.AccessFieldPermission) error {
	resource := canonicalAccessResource(rule.ResourceType)
	if _, ok := supportedAccessResources[resource]; !ok || resource == "*" {
		return invalidAccessPolicy(fmt.Errorf("unsupported concrete resource_type %q", rule.ResourceType))
	}
	if len(splitFieldPath(rule.FieldPath)) == 0 || len(rule.FieldPath) > 500 {
		return invalidAccessPolicy(errors.New("field_path is invalid"))
	}
	switch rule.Classification {
	case models.AccessFieldPublic, models.AccessFieldInternal, models.AccessFieldConfidential,
		models.AccessFieldRestricted, models.AccessFieldPersonal, models.AccessFieldFinancial,
		models.AccessFieldLegal, models.AccessFieldSecuritySensitive:
	default:
		return invalidAccessPolicy(errors.New("classification is unsupported"))
	}
	switch rule.Visibility {
	case models.AccessFieldVisible, models.AccessFieldHidden:
		if rule.MaskStrategy != "" || rule.MaskPattern != "" {
			return invalidAccessPolicy(errors.New("visible/hidden fields cannot define masking"))
		}
	case models.AccessFieldMasked:
		switch rule.MaskStrategy {
		case models.AccessMaskRedact, models.AccessMaskLast4, models.AccessMaskEmail:
			if rule.MaskPattern != "" {
				return invalidAccessPolicy(errors.New("mask_pattern is allowed only for custom masking"))
			}
		case models.AccessMaskCustom:
			if err := validateBoundedTrimmed("mask_pattern", rule.MaskPattern, 1, 100); err != nil {
				return invalidAccessPolicy(err)
			}
			remaining := strings.NewReplacer("{first1}", "", "{last4}", "").Replace(rule.MaskPattern)
			if strings.ContainsAny(remaining, "{}\r\n") {
				return invalidAccessPolicy(errors.New("mask_pattern contains unsupported placeholders"))
			}
		default:
			return invalidAccessPolicy(errors.New("masked fields require a supported mask_strategy"))
		}
	default:
		return invalidAccessPolicy(errors.New("visibility must be visible, masked, or hidden"))
	}
	return nil
}

func validateAccessConditions(name string, conditions []models.AccessCondition, attributes map[string]struct{}, scope string) error {
	if len(conditions) > 32 {
		return fmt.Errorf("%s cannot contain more than 32 conditions", name)
	}
	totalBytes := 0
	for index, condition := range conditions {
		if _, ok := attributes[condition.Attribute]; !ok {
			return fmt.Errorf("%s[%d] uses unsupported %s attribute %q", name, index, scope, condition.Attribute)
		}
		if len(condition.Value) == 0 || len(condition.Value) > 4096 {
			return fmt.Errorf("%s[%d] value must contain at most 4096 bytes", name, index)
		}
		totalBytes += len(condition.Attribute) + len(condition.Operator) + len(condition.Value)
		if totalBytes > 16*1024 {
			return fmt.Errorf("%s exceeds 16384 bytes", name)
		}
		value, err := decodeJSONValue(condition.Value)
		if err != nil {
			return fmt.Errorf("%s[%d] value is invalid JSON", name, index)
		}
		if err := validateConditionOperand(condition, value, scope); err != nil {
			return fmt.Errorf("%s[%d]: %w", name, index, err)
		}
	}
	return nil
}

func validateConditionOperand(condition models.AccessCondition, value any, scope string) error {
	switch condition.Operator {
	case models.AccessOperatorEquals, models.AccessOperatorNotEquals:
		if !isScalarAccessValue(value) {
			return errors.New("equals/not_equals requires a string, number, or boolean")
		}
	case models.AccessOperatorIn, models.AccessOperatorNotIn, models.AccessOperatorContainsAny:
		values, ok := valueSlice(value)
		if !ok || len(values) == 0 || len(values) > 50 {
			return errors.New("set operator requires between 1 and 50 values")
		}
		for _, item := range values {
			if !isScalarAccessValue(item) {
				return errors.New("set values must be strings, numbers, or booleans")
			}
		}
	case models.AccessOperatorContains:
		if _, ok := value.(string); !ok {
			return errors.New("contains requires a string value")
		}
	case models.AccessOperatorGreaterThan, models.AccessOperatorGreaterThanOrEqual,
		models.AccessOperatorLessThan, models.AccessOperatorLessThanOrEqual:
		if _, err := numericValue(value); err != nil {
			return errors.New("numeric comparison requires a numeric value")
		}
	case models.AccessOperatorBetween:
		values, ok := valueSlice(value)
		if !ok || len(values) != 2 {
			return errors.New("between requires exactly two numeric values")
		}
		lower, lowerErr := numericValue(values[0])
		upper, upperErr := numericValue(values[1])
		if lowerErr != nil || upperErr != nil || lower > upper {
			return errors.New("between requires ordered numeric bounds")
		}
	case models.AccessOperatorInCIDR:
		if condition.Attribute != "ip_address" || scope != "environment" {
			return errors.New("in_cidr is permitted only for environment ip_address")
		}
		values, ok := valueSlice(value)
		if !ok {
			values = []any{value}
		}
		if len(values) == 0 || len(values) > 20 {
			return errors.New("in_cidr requires between 1 and 20 CIDRs")
		}
		for _, item := range values {
			cidr, ok := item.(string)
			if !ok {
				return errors.New("CIDR values must be strings")
			}
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("invalid CIDR %q", cidr)
			}
		}
	case models.AccessOperatorEqualsSubject:
		if scope != "resource" || !isGrantScopeAttribute(condition.Attribute) {
			return errors.New("equals_subject is permitted only for resource scope attributes")
		}
		subjectAttribute, ok := value.(string)
		if !ok || (subjectAttribute != "id" && subjectAttribute != "department" && subjectAttribute != "location") {
			return errors.New("equals_subject must reference id, department, or location")
		}
	default:
		return fmt.Errorf("unsupported operator %q", condition.Operator)
	}
	return nil
}

func isScalarAccessValue(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	if _, ok := value.(bool); ok {
		return true
	}
	_, ok := optionalNumericValue(value)
	return ok
}

func validateChangeReason(value string) error {
	return validateBoundedTrimmed("reason", value, 3, 1000)
}

func validateBoundedTrimmed(name, value string, minimum, maximum int) error {
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	if len(value) < minimum || len(value) > maximum {
		return fmt.Errorf("%s must contain between %d and %d characters", name, minimum, maximum)
	}
	if strings.ContainsAny(value, "\x00\r") {
		return fmt.Errorf("%s contains unsupported control characters", name)
	}
	return nil
}

func invalidAccessPolicy(err error) error {
	return fmt.Errorf("%w: %v", ErrInvalidAccessPolicy, err)
}

func accessPolicyWindowActive(from, until *time.Time, at time.Time) bool {
	return (from == nil || !at.Before(from.UTC())) && (until == nil || at.Before(until.UTC()))
}

func validAccessComponent(value string) bool {
	return accessComponentPattern.MatchString(value)
}
