package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrAccessDecisionUnavailable = errors.New("policy access decision is unavailable")
	ErrAccessEvidenceUnavailable = errors.New("policy access decision evidence is unavailable")
	ErrInvalidAccessPolicy       = errors.New("access policy is invalid")
)

// AccessPolicyDecisionStore is the persistence boundary used by the composite
// authorizer. Implementations must use the request-scoped tenant querier and
// must durably insert decision evidence before returning from RecordDecision.
type AccessPolicyDecisionStore interface {
	LoadEvaluationBundle(context.Context, string, string, string, string, string, time.Time) (*models.AccessEvaluationBundle, error)
	RecordDecision(context.Context, models.AccessDecisionEvidence) error
}

// AccessEvidenceFailureReporter receives bounded operational signals when a
// denial could not be persisted. An allow never takes this path: it fails
// closed when evidence cannot be recorded.
type AccessEvidenceFailureReporter func(context.Context, models.AccessDecisionEvidence, error)

// PolicyBasedAuthorizer composes persisted RBAC with policy constraints. RBAC
// is always evaluated first and is an absolute upper bound.
type PolicyBasedAuthorizer struct {
	rbac                  authz.Authorizer
	store                 AccessPolicyDecisionStore
	now                   func() time.Time
	reportEvidenceFailure AccessEvidenceFailureReporter
}

var _ authz.Authorizer = (*PolicyBasedAuthorizer)(nil)

func NewPolicyBasedAuthorizer(
	rbac authz.Authorizer,
	store AccessPolicyDecisionStore,
	reporter AccessEvidenceFailureReporter,
) (*PolicyBasedAuthorizer, error) {
	if rbac == nil {
		return nil, errors.New("RBAC authorizer is required")
	}
	if store == nil {
		return nil, errors.New("policy decision store is required")
	}
	if reporter == nil {
		reporter = func(_ context.Context, evidence models.AccessDecisionEvidence, err error) {
			log.Error().
				Err(err).
				Str("decision_id", evidence.ID).
				Str("request_id", evidence.RequestID).
				Str("resource", evidence.ResourceType).
				Str("action", evidence.Action).
				Msg("failed to persist denied authorization decision evidence")
		}
	}
	return &PolicyBasedAuthorizer{
		rbac: rbac, store: store, now: time.Now, reportEvidenceFailure: reporter,
	}, nil
}

func (a *PolicyBasedAuthorizer) Authorize(ctx context.Context, request authz.Request) (authz.Decision, error) {
	if a == nil || a.rbac == nil || a.store == nil {
		return authz.Decision{}, ErrAccessDecisionUnavailable
	}
	rbacDecision, err := a.rbac.Authorize(ctx, request)
	if err != nil {
		return authz.Decision{}, fmt.Errorf("evaluate RBAC upper bound: %w", err)
	}
	now := a.now().UTC()
	if !rbacDecision.Allowed {
		if strings.TrimSpace(rbacDecision.ReasonCode) == "" {
			rbacDecision.ReasonCode = "rbac_denied"
		}
		evidence := buildDecisionEvidence(request, rbacDecision, models.AccessConstraintNotApplicable, nil, now, 0)
		if err := a.store.RecordDecision(ctx, evidence); err != nil {
			a.reportEvidenceFailure(ctx, evidence, err)
		} else {
			rbacDecision.DecisionID = evidence.ID
		}
		return rbacDecision, nil
	}

	bundle, err := a.store.LoadEvaluationBundle(
		ctx, request.OrganizationID, request.SubjectID, request.Resource, request.ResourceID, request.Action, now,
	)
	if err != nil {
		return authz.Decision{}, fmt.Errorf("%w: load policy context: %v", ErrAccessDecisionUnavailable, err)
	}
	constraint, err := EvaluateAccessConstraints(*bundle, request, now)
	if err != nil {
		return authz.Decision{}, fmt.Errorf("%w: evaluate policy constraints: %v", ErrAccessDecisionUnavailable, err)
	}

	decision := authz.Decision{
		Allowed:            constraint.Allowed,
		Reason:             constraint.Reason,
		ReasonCode:         constraint.ReasonCode,
		ConstraintsApplied: constraint.Outcome != models.AccessConstraintNotApplicable,
		Obligations:        []authz.Obligation{},
	}
	if constraint.WinningPolicyID != nil {
		decision.PolicyID = *constraint.WinningPolicyID
	}
	if constraint.WinningPolicyName != nil {
		decision.PolicyName = *constraint.WinningPolicyName
	}
	for _, obligation := range constraint.Obligations {
		decision.Obligations = append(decision.Obligations, authz.Obligation{
			Kind: obligation.Kind, Parameters: cloneStringMap(obligation.Parameters),
		})
	}
	decision.Obligations = append(decision.Obligations, AccessFieldObligations(constraint.FieldPermissions)...)

	evidence := buildDecisionEvidence(request, decision, constraint.Outcome, bundle, now, constraint.EvaluationTimeUS)
	evidence.MatchedPolicyIDs = append([]string(nil), constraint.MatchedPolicyIDs...)
	evidence.WinningPolicyID = cloneStringPointer(constraint.WinningPolicyID)
	evidence.MatchedObjectGrantID = cloneStringPointer(constraint.MatchedObjectGrant)
	if err := a.store.RecordDecision(ctx, evidence); err != nil {
		if !decision.Allowed {
			a.reportEvidenceFailure(ctx, evidence, err)
			return decision, nil
		}
		return authz.Decision{}, fmt.Errorf("%w: %v", ErrAccessEvidenceUnavailable, err)
	}
	decision.DecisionID = evidence.ID
	return decision, nil
}

// EvaluateAccessConstraints is the deterministic, persistence-free policy
// combining algorithm. The caller invokes it only after RBAC has allowed.
func EvaluateAccessConstraints(
	bundle models.AccessEvaluationBundle,
	request authz.Request,
	now time.Time,
) (models.AccessConstraintDecision, error) {
	started := time.Now()
	decision := models.AccessConstraintDecision{
		Allowed:          true,
		Outcome:          models.AccessConstraintNotApplicable,
		ReasonCode:       "rbac_allowed_no_relevant_policy",
		Reason:           "allowed by role permission; no relevant policy constraint",
		FieldPermissions: []models.AccessFieldPermission{},
		Obligations:      []models.AccessObligation{},
	}

	subjectAttributes, err := subjectAttributeValues(bundle.Subject)
	if err != nil {
		return models.AccessConstraintDecision{}, err
	}
	resourceAttributes, err := resourceAttributeValues(bundle.Resource)
	if err != nil {
		return models.AccessConstraintDecision{}, err
	}
	environmentAttributes := environmentAttributeValues(request, now)

	grant := matchingObjectGrant(bundle.ObjectGrants, request, now)
	isExternalAuditor := stringSliceContainsFold(bundle.Subject.Roles, "external_auditor")
	if isExternalAuditor {
		switch {
		case strings.TrimSpace(request.ResourceID) == "":
			decision.Outcome = models.AccessConstraintDeny
			decision.Allowed = false
			decision.ReasonCode = "external_auditor_collection_scope_required"
			decision.Reason = "external auditor collection access requires a grant-filtered resource listing"
			decision.EvaluationTimeUS = elapsedMicroseconds(started)
			return decision, nil
		case grant == nil:
			decision.Outcome = models.AccessConstraintDeny
			decision.Allowed = false
			decision.ReasonCode = "external_auditor_grant_required"
			decision.Reason = "external auditor access requires an active sponsor-approved object grant"
			decision.EvaluationTimeUS = elapsedMicroseconds(started)
			return decision, nil
		}
	}

	policies := append([]models.AccessPolicy(nil), bundle.Policies...)
	sort.SliceStable(policies, func(i, j int) bool {
		if policies[i].Priority != policies[j].Priority {
			return policies[i].Priority < policies[j].Priority
		}
		return policies[i].ID < policies[j].ID
	})

	var matchedAllows []models.AccessPolicy
	var matchedDenies []models.AccessPolicy
	for _, policy := range policies {
		if !policyRelevantForRequest(policy, request, now) {
			continue
		}
		subjectMatches, err := evaluateConditionSet(policy.SubjectConditions, subjectAttributes, subjectAttributes, false)
		if err != nil {
			return models.AccessConstraintDecision{}, fmt.Errorf("policy %s subject conditions: %w", policy.ID, err)
		}
		if !subjectMatches {
			continue
		}
		decision.RelevantPolicyIDs = append(decision.RelevantPolicyIDs, policy.ID)

		resourceMatches, err := evaluateConditionSet(
			policy.ResourceConditions, resourceAttributes, subjectAttributes,
			policy.Effect == models.AccessPolicyEffectAllow && grant != nil,
		)
		if err != nil {
			return models.AccessConstraintDecision{}, fmt.Errorf("policy %s resource conditions: %w", policy.ID, err)
		}
		environmentMatches, err := evaluateConditionSet(
			policy.EnvironmentConditions, environmentAttributes, subjectAttributes, false,
		)
		if err != nil {
			return models.AccessConstraintDecision{}, fmt.Errorf("policy %s environment conditions: %w", policy.ID, err)
		}
		if !resourceMatches || !environmentMatches {
			continue
		}
		decision.MatchedPolicyIDs = append(decision.MatchedPolicyIDs, policy.ID)
		switch policy.Effect {
		case models.AccessPolicyEffectDeny:
			matchedDenies = append(matchedDenies, policy)
		case models.AccessPolicyEffectAllow:
			matchedAllows = append(matchedAllows, policy)
		default:
			return models.AccessConstraintDecision{}, fmt.Errorf("policy %s has unsupported effect %q", policy.ID, policy.Effect)
		}
	}

	if len(matchedDenies) > 0 {
		winner := matchedDenies[0]
		decision.Allowed = false
		decision.Outcome = models.AccessConstraintDeny
		decision.ReasonCode = "abac_explicit_deny"
		decision.Reason = "denied by an applicable policy constraint"
		decision.WinningPolicyID = stringPointer(winner.ID)
		decision.WinningPolicyName = stringPointer(winner.Name)
		decision.EvaluationTimeUS = elapsedMicroseconds(started)
		return decision, nil
	}

	relevantAllows := make(map[string]struct{})
	for _, policy := range policies {
		if policy.Effect != models.AccessPolicyEffectAllow || !accessContainsString(decision.RelevantPolicyIDs, policy.ID) {
			continue
		}
		relevantAllows[policy.ID] = struct{}{}
	}
	if len(relevantAllows) > 0 && len(matchedAllows) == 0 {
		decision.Allowed = false
		decision.Outcome = models.AccessConstraintDeny
		decision.ReasonCode = "abac_allow_constraint_unsatisfied"
		decision.Reason = "role permission is constrained by an unsatisfied access policy"
		decision.EvaluationTimeUS = elapsedMicroseconds(started)
		return decision, nil
	}

	if len(matchedAllows) > 0 {
		winner := matchedAllows[0]
		decision.Outcome = models.AccessConstraintAllow
		decision.ReasonCode = "abac_allow_constraint_satisfied"
		decision.Reason = "allowed by role permission and satisfied policy constraints"
		decision.WinningPolicyID = stringPointer(winner.ID)
		decision.WinningPolicyName = stringPointer(winner.Name)
		decision.FieldPermissions = resolveFieldPermissions(bundle.FieldPermissions, matchedAllows)
	}
	if grant != nil {
		decision.MatchedObjectGrant = stringPointer(grant.ID)
		if (strings.EqualFold(request.Action, "export") || strings.EqualFold(request.Action, "download")) && grant.RequireWatermark {
			decision.Obligations = append(decision.Obligations, models.AccessObligation{
				Kind: "watermark", Parameters: map[string]string{"text": grant.WatermarkText},
			})
		}
	}
	decision.EvaluationTimeUS = elapsedMicroseconds(started)
	return decision, nil
}

func policyRelevantForRequest(policy models.AccessPolicy, request authz.Request, now time.Time) bool {
	if !policy.IsActive || policy.DeletedAt != nil {
		return false
	}
	if policy.ValidFrom != nil && now.Before(policy.ValidFrom.UTC()) {
		return false
	}
	if policy.ValidUntil != nil && !now.Before(policy.ValidUntil.UTC()) {
		return false
	}
	if policy.ResourceType != "*" && canonicalAccessResource(policy.ResourceType) != canonicalAccessResource(request.Resource) {
		return false
	}
	for _, action := range policy.Actions {
		if action == "*" || strings.EqualFold(action, request.Action) {
			return true
		}
	}
	return false
}

func matchingObjectGrant(grants []models.AccessObjectGrant, request authz.Request, now time.Time) *models.AccessObjectGrant {
	ordered := append([]models.AccessObjectGrant(nil), grants...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].ValidUntil.Equal(ordered[j].ValidUntil) {
			return ordered[i].ValidUntil.Before(ordered[j].ValidUntil)
		}
		return ordered[i].ID < ordered[j].ID
	})
	for index := range ordered {
		grant := &ordered[index]
		if grant.Status != models.AccessObjectGrantApproved || grant.ApprovedBy == nil || grant.ApprovedAt == nil || strings.TrimSpace(grant.SponsorID) == "" {
			continue
		}
		if now.Before(grant.ValidFrom.UTC()) || !now.Before(grant.ValidUntil.UTC()) {
			continue
		}
		if grant.SubjectID != request.SubjectID || grant.ResourceID != request.ResourceID || canonicalAccessResource(grant.ResourceType) != canonicalAccessResource(request.Resource) {
			continue
		}
		if !actionInList(grant.Actions, request.Action) {
			continue
		}
		if (strings.EqualFold(request.Action, "export") || strings.EqualFold(request.Action, "download")) && !grant.AllowDownload {
			continue
		}
		return grant
	}
	return nil
}

func resolveFieldPermissions(rules []models.AccessFieldPermission, matchedAllows []models.AccessPolicy) []models.AccessFieldPermission {
	matched := make(map[string]struct{}, len(matchedAllows))
	for _, policy := range matchedAllows {
		matched[policy.ID] = struct{}{}
	}
	filtered := make([]models.AccessFieldPermission, 0, len(rules))
	for _, rule := range rules {
		if _, ok := matched[rule.PolicyID]; ok {
			filtered = append(filtered, rule)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].FieldPath != filtered[j].FieldPath {
			return filtered[i].FieldPath < filtered[j].FieldPath
		}
		if visibilityRank(filtered[i].Visibility) != visibilityRank(filtered[j].Visibility) {
			return visibilityRank(filtered[i].Visibility) > visibilityRank(filtered[j].Visibility)
		}
		if filtered[i].PolicyPriority != filtered[j].PolicyPriority {
			return filtered[i].PolicyPriority < filtered[j].PolicyPriority
		}
		if filtered[i].PolicyID != filtered[j].PolicyID {
			return filtered[i].PolicyID < filtered[j].PolicyID
		}
		return filtered[i].ID < filtered[j].ID
	})
	resolved := make([]models.AccessFieldPermission, 0, len(filtered))
	for _, rule := range filtered {
		if len(resolved) == 0 || resolved[len(resolved)-1].FieldPath != rule.FieldPath {
			resolved = append(resolved, rule)
		}
	}
	return resolved
}

func visibilityRank(visibility models.AccessFieldVisibility) int {
	switch visibility {
	case models.AccessFieldHidden:
		return 3
	case models.AccessFieldMasked:
		return 2
	case models.AccessFieldVisible:
		return 1
	default:
		return 0
	}
}

func subjectAttributeValues(subject models.AccessSubject) (map[string]any, error) {
	values, err := decodeAttributeBag(subject.Attributes)
	if err != nil {
		return nil, fmt.Errorf("decode subject attributes: %w", err)
	}
	values["id"] = subject.ID
	values["roles"] = append([]string(nil), subject.Roles...)
	values["department"] = subject.Department
	values["location"] = subject.Location
	values["status"] = subject.Status
	values["super_admin"] = subject.SuperAdmin
	return values, nil
}

func resourceAttributeValues(resource models.AccessResource) (map[string]any, error) {
	values, err := decodeAttributeBag(resource.Attributes)
	if err != nil {
		return nil, fmt.Errorf("decode resource attributes: %w", err)
	}
	values["id"] = resource.ID
	values["type"] = canonicalAccessResource(resource.Type)
	if resource.OwnerID != "" {
		values["owner_id"] = resource.OwnerID
	}
	if resource.Department != "" {
		values["department"] = resource.Department
	}
	if resource.Location != "" {
		values["location"] = resource.Location
	}
	if resource.Classification != "" {
		values["classification"] = resource.Classification
	}
	return values, nil
}

func environmentAttributeValues(request authz.Request, now time.Time) map[string]any {
	return map[string]any{
		"mfa_verified": request.MFAVerified,
		"ip_address":   normalizedIPAddress(request.IPAddress),
		"hour":         float64(now.Hour()),
		"day_of_week":  strings.ToLower(now.Weekday().String()),
	}
}

func decodeAttributeBag(raw map[string]json.RawMessage) (map[string]any, error) {
	values := make(map[string]any, len(raw)+6)
	for key, encoded := range raw {
		value, err := decodeJSONValue(encoded)
		if err != nil {
			return nil, fmt.Errorf("attribute %q: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}

func evaluateConditionSet(conditions []models.AccessCondition, attributes, subject map[string]any, grantSatisfiesScope bool) (bool, error) {
	for _, condition := range conditions {
		actual, exists := attributes[condition.Attribute]
		if !exists {
			return false, nil
		}
		expected, err := decodeJSONValue(condition.Value)
		if err != nil {
			return false, fmt.Errorf("condition %q value: %w", condition.Attribute, err)
		}
		matches, err := evaluateAccessOperator(condition.Operator, actual, expected, subject)
		if err != nil {
			return false, fmt.Errorf("condition %q: %w", condition.Attribute, err)
		}
		if !matches && grantSatisfiesScope && isGrantScopeAttribute(condition.Attribute) {
			continue
		}
		if !matches {
			return false, nil
		}
	}
	return true, nil
}

func evaluateAccessOperator(operator models.AccessConditionOperator, actual, expected any, subject map[string]any) (bool, error) {
	switch operator {
	case models.AccessOperatorEquals:
		return typedEqual(actual, expected), nil
	case models.AccessOperatorNotEquals:
		return !typedEqual(actual, expected), nil
	case models.AccessOperatorIn:
		return valueIn(actual, expected)
	case models.AccessOperatorNotIn:
		matched, err := valueIn(actual, expected)
		return !matched, err
	case models.AccessOperatorContains:
		actualString, ok := actual.(string)
		if !ok {
			return false, errors.New("contains requires a string attribute")
		}
		expectedString, ok := expected.(string)
		if !ok {
			return false, errors.New("contains requires a string value")
		}
		return strings.Contains(actualString, expectedString), nil
	case models.AccessOperatorContainsAny:
		return slicesIntersect(actual, expected)
	case models.AccessOperatorGreaterThan, models.AccessOperatorGreaterThanOrEqual,
		models.AccessOperatorLessThan, models.AccessOperatorLessThanOrEqual:
		actualNumber, err := numericValue(actual)
		if err != nil {
			return false, err
		}
		expectedNumber, err := numericValue(expected)
		if err != nil {
			return false, err
		}
		switch operator {
		case models.AccessOperatorGreaterThan:
			return actualNumber > expectedNumber, nil
		case models.AccessOperatorGreaterThanOrEqual:
			return actualNumber >= expectedNumber, nil
		case models.AccessOperatorLessThan:
			return actualNumber < expectedNumber, nil
		default:
			return actualNumber <= expectedNumber, nil
		}
	case models.AccessOperatorBetween:
		bounds, ok := expected.([]any)
		if !ok || len(bounds) != 2 {
			return false, errors.New("between requires exactly two numeric values")
		}
		actualNumber, err := numericValue(actual)
		if err != nil {
			return false, err
		}
		lower, err := numericValue(bounds[0])
		if err != nil {
			return false, err
		}
		upper, err := numericValue(bounds[1])
		if err != nil || lower > upper {
			return false, errors.New("between bounds are invalid")
		}
		return actualNumber >= lower && actualNumber <= upper, nil
	case models.AccessOperatorInCIDR:
		actualIP, ok := actual.(string)
		if !ok {
			return false, errors.New("in_cidr requires a string IP attribute")
		}
		return ipInCIDRs(actualIP, expected)
	case models.AccessOperatorEqualsSubject:
		subjectAttribute, ok := expected.(string)
		if !ok || subjectAttribute == "" {
			return false, errors.New("equals_subject requires a subject attribute name")
		}
		subjectValue, exists := subject[subjectAttribute]
		if !exists {
			return false, nil
		}
		return typedEqual(actual, subjectValue), nil
	default:
		return false, fmt.Errorf("unsupported operator %q", operator)
	}
}

func decodeJSONValue(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("value is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values are not permitted")
		}
		return nil, err
	}
	return value, nil
}

func typedEqual(left, right any) bool {
	leftNumber, leftNumeric := optionalNumericValue(left)
	rightNumber, rightNumeric := optionalNumericValue(right)
	if leftNumeric || rightNumeric {
		return leftNumeric && rightNumeric && leftNumber == rightNumber
	}
	switch leftValue := left.(type) {
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	case bool:
		rightValue, ok := right.(bool)
		return ok && leftValue == rightValue
	case nil:
		return right == nil
	default:
		leftSlice, leftOK := valueSlice(left)
		rightSlice, rightOK := valueSlice(right)
		if !leftOK || !rightOK || len(leftSlice) != len(rightSlice) {
			return false
		}
		for index := range leftSlice {
			if !typedEqual(leftSlice[index], rightSlice[index]) {
				return false
			}
		}
		return true
	}
}

func valueIn(actual, expected any) (bool, error) {
	values, ok := valueSlice(expected)
	if !ok {
		return false, errors.New("in/not_in requires an array value")
	}
	for _, value := range values {
		if typedEqual(actual, value) {
			return true, nil
		}
	}
	return false, nil
}

func slicesIntersect(actual, expected any) (bool, error) {
	actualValues, actualOK := valueSlice(actual)
	expectedValues, expectedOK := valueSlice(expected)
	if !actualOK || !expectedOK {
		return false, errors.New("contains_any requires array attribute and value")
	}
	for _, actualValue := range actualValues {
		for _, expectedValue := range expectedValues {
			if typedEqual(actualValue, expectedValue) {
				return true, nil
			}
		}
	}
	return false, nil
}

func valueSlice(value any) ([]any, bool) {
	switch values := value.(type) {
	case []any:
		return values, true
	case []string:
		result := make([]any, len(values))
		for index, item := range values {
			result[index] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func numericValue(value any) (float64, error) {
	number, ok := optionalNumericValue(value)
	if !ok {
		return 0, errors.New("numeric comparison requires numeric values")
	}
	return number, nil
}

func optionalNumericValue(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func ipInCIDRs(ipValue string, expected any) (bool, error) {
	ip := net.ParseIP(normalizedIPAddress(ipValue))
	if ip == nil {
		return false, errors.New("invalid IP address")
	}
	var cidrs []any
	if scalar, ok := expected.(string); ok {
		cidrs = []any{scalar}
	} else {
		var ok bool
		cidrs, ok = valueSlice(expected)
		if !ok {
			return false, errors.New("in_cidr requires a CIDR string or array")
		}
	}
	for _, value := range cidrs {
		cidrString, ok := value.(string)
		if !ok {
			return false, errors.New("CIDR values must be strings")
		}
		_, network, err := net.ParseCIDR(cidrString)
		if err != nil {
			return false, fmt.Errorf("invalid CIDR %q", cidrString)
		}
		if network.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}

func isGrantScopeAttribute(attribute string) bool {
	switch attribute {
	case "id", "owner_id", "department", "location":
		return true
	default:
		return false
	}
}

func actionInList(actions []string, requested string) bool {
	for _, action := range actions {
		if action == "*" || strings.EqualFold(action, requested) {
			return true
		}
	}
	return false
}

func canonicalAccessResource(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "risk":
		return "risks"
	case "policy":
		return "policies"
	case "control", "control_implementation":
		return "controls"
	case "framework":
		return "frameworks"
	case "audit":
		return "audits"
	case "finding":
		return "findings"
	case "incident":
		return "incidents"
	case "asset":
		return "assets"
	case "vendor":
		return "vendors"
	case "report":
		return "reports"
	case "user":
		return "users"
	default:
		return value
	}
}

func buildDecisionEvidence(
	request authz.Request,
	decision authz.Decision,
	outcome models.AccessConstraintOutcome,
	bundle *models.AccessEvaluationBundle,
	now time.Time,
	evaluationTimeUS int,
) models.AccessDecisionEvidence {
	evidence := models.AccessDecisionEvidence{
		ID:                uuid.NewString(),
		OrganizationID:    request.OrganizationID,
		SubjectID:         request.SubjectID,
		Action:            strings.ToLower(strings.TrimSpace(request.Action)),
		ResourceType:      canonicalAccessResource(request.Resource),
		RBACAllowed:       outcome != models.AccessConstraintNotApplicable || decision.Allowed,
		ConstraintOutcome: outcome,
		Decision:          "deny",
		ReasonCode:        decision.ReasonCode,
		RequestID:         requestIDFromAttributes(request.Attributes),
		EvaluationTimeUS:  evaluationTimeUS,
		CreatedAt:         now,
	}
	if decision.Allowed {
		evidence.Decision = "allow"
	}
	if request.ResourceID != "" {
		evidence.ResourceID = stringPointer(request.ResourceID)
	}
	subjectSnapshot := map[string]any{"id": request.SubjectID, "role": request.Role}
	resourceSnapshot := map[string]any{"id": request.ResourceID, "type": canonicalAccessResource(request.Resource)}
	if bundle != nil {
		subjectSnapshot = map[string]any{
			"id": bundle.Subject.ID, "roles": bundle.Subject.Roles, "department": bundle.Subject.Department,
			"location": bundle.Subject.Location, "status": bundle.Subject.Status, "super_admin": bundle.Subject.SuperAdmin,
		}
		resourceSnapshot = map[string]any{
			"id": bundle.Resource.ID, "type": canonicalAccessResource(bundle.Resource.Type), "owner_id": bundle.Resource.OwnerID,
			"department": bundle.Resource.Department, "location": bundle.Resource.Location,
			"classification": bundle.Resource.Classification,
		}
	}
	evidence.SubjectSnapshot = mustMarshalBoundedSnapshot(subjectSnapshot)
	evidence.ResourceSnapshot = mustMarshalBoundedSnapshot(resourceSnapshot)
	evidence.EnvironmentSnapshot = mustMarshalBoundedSnapshot(map[string]any{
		"mfa_verified": request.MFAVerified, "ip_address": normalizedIPAddress(request.IPAddress),
		"hour": now.Hour(), "day_of_week": strings.ToLower(now.Weekday().String()),
	})
	return evidence
}

func mustMarshalBoundedSnapshot(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > 16*1024 {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func requestIDFromAttributes(attributes map[string]any) string {
	requestID, _ := attributes["request_id"].(string)
	requestID = strings.TrimSpace(requestID)
	if len(requestID) > 64 {
		return ""
	}
	for _, char := range requestID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("-_.:", char) {
			continue
		}
		return ""
	}
	return requestID
}

func normalizedIPAddress(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return strings.Trim(value, "[]")
}

func elapsedMicroseconds(started time.Time) int {
	elapsed := time.Since(started).Microseconds()
	if elapsed < 0 {
		return 0
	}
	if elapsed > int64(math.MaxInt) {
		return math.MaxInt
	}
	return int(elapsed)
}

func stringSliceContainsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(value, expected) {
			return true
		}
	}
	return false
}

func accessContainsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string {
	return &value
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	return stringPointer(*value)
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
