package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ABACEngine is the Attribute-Based Access Control policy decision point.
// It evaluates access requests against stored policies using a deny-overrides algorithm.
type ABACEngine struct {
	pool *pgxpool.Pool
}

// AccessRequest describes who is trying to do what to which resource.
type AccessRequest struct {
	SubjectID    string
	Action       string
	ResourceType string
	ResourceID   string
	OrgID        string
	IPAddress    string
	MFAVerified  bool
}

// AccessDecision is the result of a policy evaluation.
type AccessDecision struct {
	Effect     string  `json:"effect"` // "allow" or "deny"
	PolicyID   *string `json:"policy_id"`
	PolicyName *string `json:"policy_name"`
	Reason     string  `json:"reason"`
	EvalTimeUS int     `json:"evaluation_time_us"`
}

// AccessPolicy is a single ABAC policy stored in the database.
type AccessPolicy struct {
	ID                    string                   `json:"id"`
	OrgID                 string                   `json:"organization_id"`
	Name                  string                   `json:"name"`
	Priority              int                      `json:"priority"`
	Effect                string                   `json:"effect"`
	IsActive              bool                     `json:"is_active"`
	SubjectConditions     []map[string]interface{} `json:"subject_conditions"`
	ResourceType          string                   `json:"resource_type"`
	ResourceConditions    []map[string]interface{} `json:"resource_conditions"`
	Actions               []string                 `json:"actions"`
	EnvironmentConditions []map[string]interface{} `json:"environment_conditions"`
	ValidFrom             *time.Time               `json:"valid_from"`
	ValidUntil            *time.Time               `json:"valid_until"`
}

// FieldPermission describes visibility for a single field on a resource type.
type FieldPermission struct {
	ResourceType string `json:"resource_type"`
	FieldName    string `json:"field_name"`
	Permission   string `json:"permission"` // visible, masked, hidden
	MaskPattern  string `json:"mask_pattern"`
}

// NewABACEngine creates a new ABAC engine backed by the given connection pool.
func NewABACEngine(pool *pgxpool.Pool) *ABACEngine {
	return &ABACEngine{pool: pool}
}

// ---------------------------------------------------------------------------
// Core PDP — deny-overrides
// ---------------------------------------------------------------------------

// Evaluate is the core Policy Decision Point. It returns an allow/deny decision
// using the deny-overrides combining algorithm: if ANY matching policy denies,
// the overall result is deny.
func (e *ABACEngine) Evaluate(ctx context.Context, req AccessRequest) (*AccessDecision, error) {
	start := time.Now()

	// 1. Fetch subject (user) attributes.
	subjectAttrs, err := e.fetchSubjectAttributes(ctx, req.OrgID, req.SubjectID)
	if err != nil {
		return denyDecision(start, "failed to fetch subject attributes"), nil
	}

	// 2. Fetch all active policies assigned to this user (directly, via role, or via all_users).
	policies, err := e.fetchApplicablePolicies(ctx, req.OrgID, req.SubjectID)
	if err != nil {
		return denyDecision(start, "failed to fetch policies"), nil
	}

	now := time.Now().UTC()
	var matchedAllow *AccessPolicy
	var matchedDeny *AccessPolicy

	for i := range policies {
		p := &policies[i]

		// 3. Filter by resource type (exact or wildcard).
		if p.ResourceType != "*" && !strings.EqualFold(p.ResourceType, req.ResourceType) {
			continue
		}

		// 4. Filter by action.
		if !actionMatches(p.Actions, req.Action) {
			continue
		}

		// 5. Temporal constraints.
		if p.ValidFrom != nil && now.Before(*p.ValidFrom) {
			continue
		}
		if p.ValidUntil != nil && now.After(*p.ValidUntil) {
			continue
		}

		// 6a. Evaluate subject conditions.
		if !e.evaluateConditions(p.SubjectConditions, subjectAttrs) {
			continue
		}

		// 6b. Evaluate resource conditions.
		if len(p.ResourceConditions) > 0 && req.ResourceID != "" {
			resAttrs, err := e.fetchResourceAttributes(ctx, req.OrgID, req.ResourceType, req.ResourceID)
			if err != nil {
				continue
			}
			if !e.evaluateConditions(p.ResourceConditions, resAttrs) {
				continue
			}
		}

		// 6c. Evaluate environment conditions.
		envAttrs := map[string]interface{}{
			"ip_address":   req.IPAddress,
			"mfa_verified": req.MFAVerified,
			"hour":         float64(now.Hour()),
			"day_of_week":  now.Weekday().String(),
		}
		if !e.evaluateConditions(p.EnvironmentConditions, envAttrs) {
			continue
		}

		// 7. Collect matched policies.
		if p.Effect == "deny" {
			if matchedDeny == nil || p.Priority < matchedDeny.Priority {
				matchedDeny = p
			}
		} else if p.Effect == "allow" {
			if matchedAllow == nil || p.Priority < matchedAllow.Priority {
				matchedAllow = p
			}
		}
	}

	// 8. Deny-overrides: any deny wins.
	var decision *AccessDecision
	if matchedDeny != nil {
		decision = &AccessDecision{
			Effect:     "deny",
			PolicyID:   &matchedDeny.ID,
			PolicyName: &matchedDeny.Name,
			Reason:     fmt.Sprintf("denied by policy '%s'", matchedDeny.Name),
			EvalTimeUS: int(time.Since(start).Microseconds()),
		}
	} else if matchedAllow != nil {
		// 9. At least one allow and no deny → allow.
		decision = &AccessDecision{
			Effect:     "allow",
			PolicyID:   &matchedAllow.ID,
			PolicyName: &matchedAllow.Name,
			Reason:     fmt.Sprintf("allowed by policy '%s'", matchedAllow.Name),
			EvalTimeUS: int(time.Since(start).Microseconds()),
		}
	} else {
		// 10. Default deny.
		decision = denyDecision(start, "no matching policy found — default deny")
	}

	// 11. Audit log.
	e.logAccessDecision(ctx, req, decision)

	return decision, nil
}

// ---------------------------------------------------------------------------
// Condition evaluator
// ---------------------------------------------------------------------------

// evaluateConditions checks a list of conditions against a set of attributes.
// All conditions must match (AND logic). Each condition is a map with keys:
// attribute, operator, value.
func (e *ABACEngine) evaluateConditions(conditions []map[string]interface{}, attributes map[string]interface{}) bool {
	for _, cond := range conditions {
		attrName, _ := cond["attribute"].(string)
		operator, _ := cond["operator"].(string)
		expected := cond["value"]
		// "in"/"not_in" operators use "values" (plural) key
		if expected == nil {
			expected = cond["values"]
		}

		actual, exists := attributes[attrName]
		if !exists && operator != "not_equals" && operator != "not_in" {
			return false
		}

		if !evalOperator(operator, actual, expected) {
			return false
		}
	}
	return true
}

// evalOperator applies a single comparison operator.
func evalOperator(op string, actual, expected interface{}) bool {
	switch op {
	case "equals":
		return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)

	case "not_equals":
		return fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", expected)

	case "in":
		return inSlice(actual, expected)

	case "not_in":
		return !inSlice(actual, expected)

	case "contains":
		actualStr, _ := actual.(string)
		expectedStr, _ := expected.(string)
		return strings.Contains(actualStr, expectedStr)

	case "contains_any":
		return containsAny(actual, expected)

	case "greater_than":
		return toFloat(actual) > toFloat(expected)

	case "less_than":
		return toFloat(actual) < toFloat(expected)

	case "between":
		vals := toFloatSlice(expected)
		if len(vals) != 2 {
			return false
		}
		v := toFloat(actual)
		return v >= vals[0] && v <= vals[1]

	case "in_cidr":
		return matchCIDR(actual, expected)

	case "equals_subject":
		// The actual value should equal the subject attribute referenced by expected.
		// This is used for owner-based checks; the caller should have injected
		// the subject attribute value into the attributes map.
		return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)

	default:
		log.Warn().Str("operator", op).Msg("unknown ABAC operator")
		return false
	}
}

// ---------------------------------------------------------------------------
// Permission queries
// ---------------------------------------------------------------------------

// GetUserPermissions returns a map of resource type → allowed actions for the given user.
func (e *ABACEngine) GetUserPermissions(ctx context.Context, orgID, userID string) (map[string][]string, error) {
	resourceTypes := []string{
		"risk", "control", "policy", "framework", "evidence",
		"vendor", "incident", "audit", "report", "user", "organization",
	}
	allActions := []string{"create", "read", "update", "delete", "export", "approve"}

	perms := make(map[string][]string, len(resourceTypes))
	for _, rt := range resourceTypes {
		var allowed []string
		for _, action := range allActions {
			decision, err := e.Evaluate(ctx, AccessRequest{
				SubjectID:    userID,
				Action:       action,
				ResourceType: rt,
				OrgID:        orgID,
			})
			if err != nil {
				continue
			}
			if decision.Effect == "allow" {
				allowed = append(allowed, action)
			}
		}
		if len(allowed) > 0 {
			perms[rt] = allowed
		}
	}
	return perms, nil
}

// GetFieldPermissions returns field-level visibility rules for a user on a resource type.
func (e *ABACEngine) GetFieldPermissions(ctx context.Context, orgID, userID, resourceType string) ([]FieldPermission, error) {
	// Fetch user role IDs for lookup.
	roleRows, err := e.pool.Query(ctx, `
		SELECT r.slug
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 AND ur.organization_id = $2`, userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("fetch user roles: %w", err)
	}
	defer roleRows.Close()

	var roleSlugs []string
	for roleRows.Next() {
		var slug string
		if err := roleRows.Scan(&slug); err == nil {
			roleSlugs = append(roleSlugs, slug)
		}
	}

	rows, err := e.pool.Query(ctx, `
		SELECT resource_type, field_name, permission, mask_pattern
		FROM field_permissions
		WHERE organization_id = $1
		  AND resource_type = $2
		  AND (role_slug = ANY($3) OR role_slug = '*')
		ORDER BY field_name`, orgID, resourceType, roleSlugs)
	if err != nil {
		return nil, fmt.Errorf("query field permissions: %w", err)
	}
	defer rows.Close()

	var perms []FieldPermission
	for rows.Next() {
		var fp FieldPermission
		if err := rows.Scan(&fp.ResourceType, &fp.FieldName, &fp.Permission, &fp.MaskPattern); err != nil {
			continue
		}
		perms = append(perms, fp)
	}
	return perms, nil
}

// MaskFields applies field-level permissions to a data map, returning a new map
// with hidden fields removed and masked fields obfuscated.
func (e *ABACEngine) MaskFields(data map[string]interface{}, permissions []FieldPermission) map[string]interface{} {
	result := make(map[string]interface{}, len(data))
	for k, v := range data {
		result[k] = v
	}

	for _, fp := range permissions {
		switch fp.Permission {
		case "hidden":
			delete(result, fp.FieldName)
		case "masked":
			if _, exists := result[fp.FieldName]; exists {
				result[fp.FieldName] = applyMask(result[fp.FieldName], fp.MaskPattern)
			}
		// "visible" — no action needed.
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Policy CRUD
// ---------------------------------------------------------------------------

// ListPolicies returns all ABAC policies for an organization.
func (e *ABACEngine) ListPolicies(ctx context.Context, orgID string) ([]AccessPolicy, error) {
	rows, err := e.pool.Query(ctx, `
		SELECT id, organization_id, name, priority, effect, is_active,
		       subject_conditions, resource_type, resource_conditions,
		       actions, environment_conditions, valid_from, valid_until
		FROM access_policies
		WHERE organization_id = $1
		ORDER BY priority ASC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	defer rows.Close()
	return scanPolicies(rows)
}

// CreatePolicy inserts a new ABAC policy.
func (e *ABACEngine) CreatePolicy(ctx context.Context, orgID, userID string, policy AccessPolicy) (*AccessPolicy, error) {
	subjJSON, _ := json.Marshal(policy.SubjectConditions)
	resJSON, _ := json.Marshal(policy.ResourceConditions)
	actionsJSON, _ := json.Marshal(policy.Actions)
	envJSON, _ := json.Marshal(policy.EnvironmentConditions)

	err := e.pool.QueryRow(ctx, `
		INSERT INTO access_policies
			(organization_id, name, priority, effect, is_active,
			 subject_conditions, resource_type, resource_conditions,
			 actions, environment_conditions, valid_from, valid_until,
			 created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NOW())
		RETURNING id`,
		orgID, policy.Name, policy.Priority, policy.Effect, true,
		subjJSON, policy.ResourceType, resJSON,
		actionsJSON, envJSON, policy.ValidFrom, policy.ValidUntil,
		userID,
	).Scan(&policy.ID)
	if err != nil {
		return nil, fmt.Errorf("create policy: %w", err)
	}

	policy.OrgID = orgID
	policy.IsActive = true
	log.Info().Str("org_id", orgID).Str("policy_id", policy.ID).Msg("ABAC policy created")
	return &policy, nil
}

// UpdatePolicy modifies an existing ABAC policy.
func (e *ABACEngine) UpdatePolicy(ctx context.Context, orgID string, policy AccessPolicy) error {
	subjJSON, _ := json.Marshal(policy.SubjectConditions)
	resJSON, _ := json.Marshal(policy.ResourceConditions)
	actionsJSON, _ := json.Marshal(policy.Actions)
	envJSON, _ := json.Marshal(policy.EnvironmentConditions)

	tag, err := e.pool.Exec(ctx, `
		UPDATE access_policies
		SET name = $1, priority = $2, effect = $3, is_active = $4,
		    subject_conditions = $5, resource_type = $6, resource_conditions = $7,
		    actions = $8, environment_conditions = $9,
		    valid_from = $10, valid_until = $11, updated_at = NOW()
		WHERE id = $12 AND organization_id = $13`,
		policy.Name, policy.Priority, policy.Effect, policy.IsActive,
		subjJSON, policy.ResourceType, resJSON,
		actionsJSON, envJSON,
		policy.ValidFrom, policy.ValidUntil,
		policy.ID, orgID,
	)
	if err != nil {
		return fmt.Errorf("update policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("policy %s not found in org %s", policy.ID, orgID)
	}
	return nil
}

// DeletePolicy removes an ABAC policy.
func (e *ABACEngine) DeletePolicy(ctx context.Context, orgID, policyID string) error {
	tag, err := e.pool.Exec(ctx, `
		DELETE FROM access_policies WHERE id = $1 AND organization_id = $2`, policyID, orgID)
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("policy %s not found in org %s", policyID, orgID)
	}
	log.Info().Str("org_id", orgID).Str("policy_id", policyID).Msg("ABAC policy deleted")
	return nil
}

// AssignPolicy links a policy to a user, role, or all_users.
func (e *ABACEngine) AssignPolicy(ctx context.Context, orgID, policyID, assigneeType, assigneeID, createdBy string) error {
	if assigneeType != "user" && assigneeType != "role" && assigneeType != "all_users" {
		return fmt.Errorf("invalid assignee_type: %s (must be user, role, or all_users)", assigneeType)
	}
	_, err := e.pool.Exec(ctx, `
		INSERT INTO policy_assignments
			(organization_id, policy_id, assignee_type, assignee_id, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (organization_id, policy_id, assignee_type, assignee_id) DO NOTHING`,
		orgID, policyID, assigneeType, assigneeID, createdBy)
	if err != nil {
		return fmt.Errorf("assign policy: %w", err)
	}
	return nil
}

// RemoveAssignment removes a policy assignment by its ID.
func (e *ABACEngine) RemoveAssignment(ctx context.Context, orgID, assignmentID string) error {
	tag, err := e.pool.Exec(ctx, `
		DELETE FROM policy_assignments WHERE id = $1 AND organization_id = $2`, assignmentID, orgID)
	if err != nil {
		return fmt.Errorf("remove assignment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("assignment %s not found in org %s", assignmentID, orgID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (e *ABACEngine) fetchSubjectAttributes(ctx context.Context, orgID, userID string) (map[string]interface{}, error) {
	attrs := make(map[string]interface{})

	var email, department, firstName, lastName string
	var isActive bool
	err := e.pool.QueryRow(ctx, `
		SELECT email, COALESCE(department,''), COALESCE(first_name,''), COALESCE(last_name,''), is_active
		FROM users WHERE id = $1 AND organization_id = $2`, userID, orgID).
		Scan(&email, &department, &firstName, &lastName, &isActive)
	if err != nil {
		return nil, fmt.Errorf("fetch user: %w", err)
	}
	attrs["id"] = userID
	attrs["email"] = email
	attrs["department"] = department
	attrs["first_name"] = firstName
	attrs["last_name"] = lastName
	attrs["is_active"] = isActive

	// Roles.
	rows, err := e.pool.Query(ctx, `
		SELECT r.slug FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 AND ur.organization_id = $2`, userID, orgID)
	if err != nil {
		return attrs, nil
	}
	defer rows.Close()

	var roles []interface{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err == nil {
			roles = append(roles, slug)
		}
	}
	attrs["roles"] = roles
	return attrs, nil
}

func (e *ABACEngine) fetchApplicablePolicies(ctx context.Context, orgID, userID string) ([]AccessPolicy, error) {
	rows, err := e.pool.Query(ctx, `
		SELECT DISTINCT ap.id, ap.organization_id, ap.name, ap.priority, ap.effect, ap.is_active,
		       ap.subject_conditions, ap.resource_type, ap.resource_conditions,
		       ap.actions, ap.environment_conditions, ap.valid_from, ap.valid_until
		FROM access_policies ap
		JOIN policy_assignments pa ON pa.policy_id = ap.id AND pa.organization_id = ap.organization_id
		WHERE ap.organization_id = $1
		  AND ap.is_active = true
		  AND (
		      (pa.assignee_type = 'user' AND pa.assignee_id = $2)
		      OR (pa.assignee_type = 'role' AND pa.assignee_id IN (
		          SELECT ur.role_id::text FROM user_roles ur WHERE ur.user_id = $2 AND ur.organization_id = $1
		      ))
		      OR pa.assignee_type = 'all_users'
		  )
		ORDER BY ap.priority ASC`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch policies: %w", err)
	}
	defer rows.Close()
	return scanPolicies(rows)
}

func (e *ABACEngine) fetchResourceAttributes(ctx context.Context, orgID, resourceType, resourceID string) (map[string]interface{}, error) {
	// Dynamically fetch from the resource table.  Use a safe allowlist of tables.
	tableMap := map[string]string{
		"risk":         "risks",
		"control":      "controls",
		"policy":       "policies",
		"framework":    "frameworks",
		"evidence":     "evidence_files",
		"vendor":       "vendors",
		"incident":     "incidents",
		"audit":        "audits",
		"report":       "reports",
		"user":         "users",
		"organization": "organizations",
	}

	table, ok := tableMap[resourceType]
	if !ok {
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	query := fmt.Sprintf(`SELECT to_jsonb(t.*) FROM %s t WHERE t.id = $1 AND t.organization_id = $2`, table)
	var rawJSON []byte
	err := e.pool.QueryRow(ctx, query, resourceID, orgID).Scan(&rawJSON)
	if err != nil {
		return nil, fmt.Errorf("fetch resource %s/%s: %w", resourceType, resourceID, err)
	}

	attrs := make(map[string]interface{})
	_ = json.Unmarshal(rawJSON, &attrs)
	return attrs, nil
}

func (e *ABACEngine) logAccessDecision(ctx context.Context, req AccessRequest, dec *AccessDecision) {
	_, err := e.pool.Exec(ctx, `
		INSERT INTO access_audit_log
			(organization_id, subject_id, action, resource_type, resource_id,
			 decision, policy_id, reason, ip_address, evaluation_time_us, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW())`,
		req.OrgID, req.SubjectID, req.Action, req.ResourceType, req.ResourceID,
		dec.Effect, dec.PolicyID, dec.Reason, req.IPAddress, dec.EvalTimeUS)
	if err != nil {
		log.Warn().Err(err).Msg("failed to log access decision")
	}
}

func scanPolicies(rows pgx.Rows) ([]AccessPolicy, error) {
	var policies []AccessPolicy
	for rows.Next() {
		var p AccessPolicy
		var subjJSON, resJSON, actionsJSON, envJSON []byte
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.Priority, &p.Effect, &p.IsActive,
			&subjJSON, &p.ResourceType, &resJSON,
			&actionsJSON, &envJSON, &p.ValidFrom, &p.ValidUntil,
		); err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		_ = json.Unmarshal(subjJSON, &p.SubjectConditions)
		_ = json.Unmarshal(resJSON, &p.ResourceConditions)
		_ = json.Unmarshal(actionsJSON, &p.Actions)
		_ = json.Unmarshal(envJSON, &p.EnvironmentConditions)
		policies = append(policies, p)
	}
	return policies, nil
}

func denyDecision(start time.Time, reason string) *AccessDecision {
	return &AccessDecision{
		Effect:     "deny",
		Reason:     reason,
		EvalTimeUS: int(time.Since(start).Microseconds()),
	}
}

func actionMatches(policyActions []string, requested string) bool {
	for _, a := range policyActions {
		if a == "*" || strings.EqualFold(a, requested) {
			return true
		}
	}
	return false
}

func inSlice(actual, expected interface{}) bool {
	actualStr := fmt.Sprintf("%v", actual)

	switch v := expected.(type) {
	case []interface{}:
		for _, item := range v {
			if fmt.Sprintf("%v", item) == actualStr {
				return true
			}
		}
	case []string:
		for _, item := range v {
			if item == actualStr {
				return true
			}
		}
	case string:
		// Comma-separated fallback.
		for _, item := range strings.Split(v, ",") {
			if strings.TrimSpace(item) == actualStr {
				return true
			}
		}
	}
	return false
}

func containsAny(actual, expected interface{}) bool {
	// actual is a slice (e.g. roles), expected is a slice of values.
	actualSlice, ok := actual.([]interface{})
	if !ok {
		return false
	}
	expectedSlice, ok := expected.([]interface{})
	if !ok {
		return false
	}
	for _, a := range actualSlice {
		aStr := fmt.Sprintf("%v", a)
		for _, e := range expectedSlice {
			if fmt.Sprintf("%v", e) == aStr {
				return true
			}
		}
	}
	return false
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	}
	return 0
}

func toFloatSlice(v interface{}) []float64 {
	switch s := v.(type) {
	case []interface{}:
		out := make([]float64, len(s))
		for i, item := range s {
			out[i] = toFloat(item)
		}
		return out
	case []float64:
		return s
	}
	return nil
}

func matchCIDR(actual, expected interface{}) bool {
	ipStr, _ := actual.(string)
	cidrStr, _ := expected.(string)
	if ipStr == "" || cidrStr == "" {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	_, cidr, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false
	}
	return cidr.Contains(ip)
}

func applyMask(value interface{}, pattern string) string {
	s := fmt.Sprintf("%v", value)
	if pattern == "" {
		// Default mask: show last 4 characters.
		if len(s) <= 4 {
			return strings.Repeat("*", len(s))
		}
		return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
	}
	// Pattern-based mask: '*' in pattern means mask that position.
	if len(pattern) != len(s) {
		// Fallback: just replace middle with asterisks.
		if len(s) <= 2 {
			return strings.Repeat("*", len(s))
		}
		return string(s[0]) + strings.Repeat("*", len(s)-2) + string(s[len(s)-1])
	}
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '*' {
			out.WriteByte('*')
		} else {
			out.WriteByte(s[i])
		}
	}
	return out.String()
}
