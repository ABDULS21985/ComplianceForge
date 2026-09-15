package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/models"
)

var policyTestNow = time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)

type policyTestRBAC struct {
	decision authz.Decision
	err      error
	calls    int
}

func (r *policyTestRBAC) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	r.calls++
	return r.decision, r.err
}

type policyTestDecisionStore struct {
	mu          sync.Mutex
	bundle      *models.AccessEvaluationBundle
	loadErr     error
	recordErr   error
	loadCalls   int
	recordCalls int
	recorded    []models.AccessDecisionEvidence
}

func (s *policyTestDecisionStore) LoadEvaluationBundle(
	context.Context, string, string, string, string, string, time.Time,
) (*models.AccessEvaluationBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadCalls++
	return s.bundle, s.loadErr
}

func (s *policyTestDecisionStore) RecordDecision(_ context.Context, evidence models.AccessDecisionEvidence) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordCalls++
	s.recorded = append(s.recorded, evidence)
	return s.recordErr
}

func policyTestRequest() authz.Request {
	return authz.Request{
		SubjectID:      "00000000-0000-0000-0000-000000000101",
		OrganizationID: "00000000-0000-0000-0000-000000000001",
		Role:           "risk_manager",
		Resource:       "risks",
		ResourceID:     "00000000-0000-0000-0000-000000000201",
		Action:         "read",
		IPAddress:      "192.0.2.10:4321",
		MFAVerified:    true,
		Attributes:     map[string]any{"request_id": "request-1"},
	}
}

func policyTestBundle() *models.AccessEvaluationBundle {
	return &models.AccessEvaluationBundle{
		Subject: models.AccessSubject{
			ID: "00000000-0000-0000-0000-000000000101", Roles: []string{"risk_manager"},
			Department: "Risk", Location: "Lagos", Status: "active",
		},
		Resource: models.AccessResource{
			ID: "00000000-0000-0000-0000-000000000201", Type: "risks",
			OwnerID: "00000000-0000-0000-0000-000000000999", Department: "Risk",
			Location: "Lagos", Classification: "confidential",
		},
		Policies:         []models.AccessPolicy{},
		ObjectGrants:     []models.AccessObjectGrant{},
		FieldPermissions: []models.AccessFieldPermission{},
	}
}

func policyTestCondition(attribute string, operator models.AccessConditionOperator, value string) models.AccessCondition {
	return models.AccessCondition{Attribute: attribute, Operator: operator, Value: json.RawMessage(value)}
}

func policyTestPolicy(id string, priority int, effect models.AccessPolicyEffect, subject, resource, environment []models.AccessCondition) models.AccessPolicy {
	return models.AccessPolicy{
		ID: id, OrganizationID: "00000000-0000-0000-0000-000000000001", Name: "Policy " + id,
		Priority: priority, Effect: effect, IsActive: true, SubjectConditions: subject,
		ResourceType: "risks", ResourceConditions: resource, Actions: []string{"read"},
		EnvironmentConditions: environment, Version: 1,
	}
}

func TestPolicyBasedAuthorizerKeepsRBACAsAbsoluteUpperBound(t *testing.T) {
	rbac := &policyTestRBAC{decision: authz.Decision{Allowed: false, Reason: "no role permission"}}
	store := &policyTestDecisionStore{bundle: policyTestBundle()}
	authorizer, err := NewPolicyBasedAuthorizer(rbac, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	authorizer.now = func() time.Time { return policyTestNow }

	decision, err := authorizer.Authorize(context.Background(), policyTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.ReasonCode != "rbac_denied" {
		t.Fatalf("decision=%+v, want RBAC denial", decision)
	}
	if store.loadCalls != 0 {
		t.Fatalf("ABAC store loaded %d times after RBAC denial", store.loadCalls)
	}
	if store.recordCalls != 1 || len(store.recorded) != 1 || store.recorded[0].RBACAllowed {
		t.Fatalf("recorded evidence=%+v", store.recorded)
	}
}

func TestPolicyBasedAuthorizerFailsClosedOnRBACOrPolicyStoreErrors(t *testing.T) {
	t.Run("RBAC", func(t *testing.T) {
		authorizer, err := NewPolicyBasedAuthorizer(
			&policyTestRBAC{err: errors.New("RBAC unavailable")}, &policyTestDecisionStore{}, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if decision, err := authorizer.Authorize(context.Background(), policyTestRequest()); err == nil || decision.Allowed {
			t.Fatalf("decision=%+v err=%v", decision, err)
		}
	})

	t.Run("policy context", func(t *testing.T) {
		authorizer, err := NewPolicyBasedAuthorizer(
			&policyTestRBAC{decision: authz.Decision{Allowed: true}},
			&policyTestDecisionStore{loadErr: errors.New("policy store unavailable")}, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if decision, err := authorizer.Authorize(context.Background(), policyTestRequest()); err == nil || decision.Allowed {
			t.Fatalf("decision=%+v err=%v", decision, err)
		}
	})
}

func TestPolicyBasedAuthorizerRequiresEvidenceBeforeAllow(t *testing.T) {
	store := &policyTestDecisionStore{bundle: policyTestBundle(), recordErr: errors.New("disk full")}
	authorizer, err := NewPolicyBasedAuthorizer(
		&policyTestRBAC{decision: authz.Decision{Allowed: true}}, store, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	authorizer.now = func() time.Time { return policyTestNow }
	decision, err := authorizer.Authorize(context.Background(), policyTestRequest())
	if !errors.Is(err, ErrAccessEvidenceUnavailable) || decision.Allowed {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
	if len(store.recorded) != 1 || store.recorded[0].Decision != "allow" || !store.recorded[0].RBACAllowed {
		t.Fatalf("evidence=%+v", store.recorded)
	}
}

func TestPolicyBasedAuthorizerEmitsValidatedFieldVisibilityObligations(t *testing.T) {
	bundle := policyTestBundle()
	bundle.Policies = []models.AccessPolicy{
		policyTestPolicy("00000000-0000-0000-0000-000000000401", 10, models.AccessPolicyEffectAllow, nil, nil, nil),
	}
	bundle.FieldPermissions = []models.AccessFieldPermission{{
		ID: "00000000-0000-0000-0000-000000000501", PolicyID: bundle.Policies[0].ID,
		PolicyPriority: 10, ResourceType: "risks", FieldPath: "financial_impact_eur",
		Classification: models.AccessFieldFinancial, Visibility: models.AccessFieldMasked,
		MaskStrategy: models.AccessMaskRedact,
	}}
	store := &policyTestDecisionStore{bundle: bundle}
	authorizer, err := NewPolicyBasedAuthorizer(
		&policyTestRBAC{decision: authz.Decision{Allowed: true}}, store, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	authorizer.now = func() time.Time { return policyTestNow }
	decision, err := authorizer.Authorize(context.Background(), policyTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.DecisionID == "" || len(decision.Obligations) != 1 {
		t.Fatalf("decision=%+v", decision)
	}
	rules, err := AccessFieldsFromObligations(decision.Obligations, "risks")
	if err != nil {
		t.Fatalf("parse decision obligations: %v", err)
	}
	if !reflect.DeepEqual(rules, bundle.FieldPermissions) {
		t.Fatalf("rules=%#v want=%#v", rules, bundle.FieldPermissions)
	}
}

func TestPolicyBasedAuthorizerKeepsDenialWhenEvidenceFails(t *testing.T) {
	bundle := policyTestBundle()
	bundle.Policies = []models.AccessPolicy{
		policyTestPolicy("deny", 100, models.AccessPolicyEffectDeny, nil, nil, nil),
	}
	store := &policyTestDecisionStore{bundle: bundle, recordErr: errors.New("disk full")}
	reported := 0
	authorizer, err := NewPolicyBasedAuthorizer(
		&policyTestRBAC{decision: authz.Decision{Allowed: true}}, store,
		func(_ context.Context, evidence models.AccessDecisionEvidence, err error) {
			reported++
			if evidence.Decision != "deny" || err == nil {
				t.Fatalf("evidence=%+v err=%v", evidence, err)
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	authorizer.now = func() time.Time { return policyTestNow }
	decision, err := authorizer.Authorize(context.Background(), policyTestRequest())
	if err != nil || decision.Allowed || reported != 1 {
		t.Fatalf("decision=%+v err=%v reported=%d", decision, err, reported)
	}
}

func TestEvaluateAccessConstraintsCombinesPoliciesDeterministically(t *testing.T) {
	roleCondition := []models.AccessCondition{
		policyTestCondition("roles", models.AccessOperatorContainsAny, `["risk_manager"]`),
	}
	ownerCondition := []models.AccessCondition{
		policyTestCondition("owner_id", models.AccessOperatorEqualsSubject, `"id"`),
	}
	classificationDeny := []models.AccessCondition{
		policyTestCondition("classification", models.AccessOperatorIn, `["confidential","restricted"]`),
	}
	bundle := *policyTestBundle()
	bundle.Policies = []models.AccessPolicy{
		policyTestPolicy("allow-z", 1, models.AccessPolicyEffectAllow, roleCondition, nil, nil),
		policyTestPolicy("deny-z", 900, models.AccessPolicyEffectDeny, roleCondition, classificationDeny, nil),
		policyTestPolicy("deny-a", 900, models.AccessPolicyEffectDeny, roleCondition, classificationDeny, nil),
		policyTestPolicy("allow-owner", 10, models.AccessPolicyEffectAllow, roleCondition, ownerCondition, nil),
	}

	decision, err := EvaluateAccessConstraints(bundle, policyTestRequest(), policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.ReasonCode != "abac_explicit_deny" {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.WinningPolicyID == nil || *decision.WinningPolicyID != "deny-a" {
		t.Fatalf("winner=%v, want deterministic deny-a", decision.WinningPolicyID)
	}
}

func TestEvaluateAccessConstraintsUsesRelevantAllowAsConstraint(t *testing.T) {
	roleCondition := []models.AccessCondition{
		policyTestCondition("roles", models.AccessOperatorContainsAny, `["risk_manager"]`),
	}
	bundle := *policyTestBundle()
	bundle.Policies = []models.AccessPolicy{
		policyTestPolicy("allow-owner", 10, models.AccessPolicyEffectAllow, roleCondition, []models.AccessCondition{
			policyTestCondition("owner_id", models.AccessOperatorEqualsSubject, `"id"`),
		}, nil),
	}

	decision, err := EvaluateAccessConstraints(bundle, policyTestRequest(), policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.ReasonCode != "abac_allow_constraint_unsatisfied" {
		t.Fatalf("decision=%+v", decision)
	}

	bundle.Subject.Roles = []string{"viewer"}
	decision, err = EvaluateAccessConstraints(bundle, policyTestRequest(), policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Outcome != models.AccessConstraintNotApplicable {
		t.Fatalf("nonmatching subject made policy relevant: %+v", decision)
	}
}

func TestEvaluateAccessConstraintsNonmatchingDenyPreservesRBAC(t *testing.T) {
	bundle := *policyTestBundle()
	bundle.Policies = []models.AccessPolicy{
		policyTestPolicy("deny-restricted", 1, models.AccessPolicyEffectDeny, nil, []models.AccessCondition{
			policyTestCondition("classification", models.AccessOperatorEquals, `"restricted"`),
		}, nil),
	}
	decision, err := EvaluateAccessConstraints(bundle, policyTestRequest(), policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Outcome != models.AccessConstraintNotApplicable {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestEvaluateAccessConstraintsGrantSatisfiesScopeButNotSecurityCondition(t *testing.T) {
	approvedBy := "00000000-0000-0000-0000-000000000301"
	approvedAt := policyTestNow.Add(-time.Hour)
	bundle := *policyTestBundle()
	bundle.ObjectGrants = []models.AccessObjectGrant{{
		ID: "grant-1", SubjectID: bundle.Subject.ID, ResourceType: "risks", ResourceID: bundle.Resource.ID,
		Actions: []string{"read"}, Status: models.AccessObjectGrantApproved,
		SponsorID: "00000000-0000-0000-0000-000000000302", ApprovedBy: &approvedBy, ApprovedAt: &approvedAt,
		ValidFrom: policyTestNow.Add(-time.Hour), ValidUntil: policyTestNow.Add(time.Hour),
	}}
	bundle.Policies = []models.AccessPolicy{
		policyTestPolicy("scoped", 10, models.AccessPolicyEffectAllow, nil, []models.AccessCondition{
			policyTestCondition("owner_id", models.AccessOperatorEqualsSubject, `"id"`),
			policyTestCondition("classification", models.AccessOperatorEquals, `"public"`),
		}, nil),
	}
	decision, err := EvaluateAccessConstraints(bundle, policyTestRequest(), policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatalf("grant bypassed classification constraint: %+v", decision)
	}

	bundle.Resource.Classification = "public"
	decision, err = EvaluateAccessConstraints(bundle, policyTestRequest(), policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.MatchedObjectGrant == nil || *decision.MatchedObjectGrant != "grant-1" {
		t.Fatalf("grant did not satisfy owner scope: %+v", decision)
	}
}

func TestEvaluateAccessConstraintsExternalAuditorRequiresApprovedObjectGrant(t *testing.T) {
	request := policyTestRequest()
	request.Role = "external_auditor"
	bundle := *policyTestBundle()
	bundle.Subject.Roles = []string{"external_auditor"}

	decision, err := EvaluateAccessConstraints(bundle, request, policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.ReasonCode != "external_auditor_grant_required" {
		t.Fatalf("decision=%+v", decision)
	}

	request.ResourceID = ""
	decision, err = EvaluateAccessConstraints(bundle, request, policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.ReasonCode != "external_auditor_collection_scope_required" {
		t.Fatalf("collection decision=%+v", decision)
	}

	request = policyTestRequest()
	request.Role = "external_auditor"
	request.Action = "export"
	approvedBy := "00000000-0000-0000-0000-000000000301"
	approvedAt := policyTestNow.Add(-time.Hour)
	bundle.ObjectGrants = []models.AccessObjectGrant{{
		ID: "grant-1", SubjectID: bundle.Subject.ID, ResourceType: "risk", ResourceID: bundle.Resource.ID,
		Actions: []string{"export"}, Status: models.AccessObjectGrantApproved,
		SponsorID: "00000000-0000-0000-0000-000000000302", ApprovedBy: &approvedBy, ApprovedAt: &approvedAt,
		ValidFrom: policyTestNow.Add(-time.Hour), ValidUntil: policyTestNow.Add(time.Hour),
		AllowDownload: true, RequireWatermark: true, WatermarkText: "Auditor copy",
	}}
	decision, err = EvaluateAccessConstraints(bundle, request, policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || len(decision.Obligations) != 1 || decision.Obligations[0].Kind != "watermark" {
		t.Fatalf("approved grant decision=%+v", decision)
	}
}

func TestEvaluateAccessConstraintsFieldVisibilityUsesMostRestrictiveRule(t *testing.T) {
	bundle := *policyTestBundle()
	bundle.Policies = []models.AccessPolicy{
		policyTestPolicy("allow-a", 20, models.AccessPolicyEffectAllow, nil, nil, nil),
		policyTestPolicy("allow-b", 10, models.AccessPolicyEffectAllow, nil, nil, nil),
	}
	bundle.FieldPermissions = []models.AccessFieldPermission{
		{ID: "visible", PolicyID: "allow-b", PolicyPriority: 10, FieldPath: "financial.amount", Visibility: models.AccessFieldVisible},
		{ID: "masked", PolicyID: "allow-a", PolicyPriority: 20, FieldPath: "financial.amount", Visibility: models.AccessFieldMasked},
		{ID: "hidden", PolicyID: "allow-b", PolicyPriority: 10, FieldPath: "financial.amount", Visibility: models.AccessFieldHidden},
	}
	decision, err := EvaluateAccessConstraints(bundle, policyTestRequest(), policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || len(decision.FieldPermissions) != 1 || decision.FieldPermissions[0].Visibility != models.AccessFieldHidden {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestEvaluateAccessConstraintsRejectsCorruptStoredConditions(t *testing.T) {
	bundle := *policyTestBundle()
	bundle.Policies = []models.AccessPolicy{
		policyTestPolicy("bad", 1, models.AccessPolicyEffectDeny, []models.AccessCondition{
			policyTestCondition("roles", models.AccessConditionOperator("execute_code"), `true`),
		}, nil, nil),
	}
	if decision, err := EvaluateAccessConstraints(bundle, policyTestRequest(), policyTestNow); err == nil || decision.Allowed {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestAccessConditionOperatorsAreTypeAware(t *testing.T) {
	tests := []struct {
		name     string
		operator models.AccessConditionOperator
		actual   any
		expected string
		want     bool
	}{
		{name: "numbers", operator: models.AccessOperatorGreaterThan, actual: 5, expected: `4.5`, want: true},
		{name: "number is not string", operator: models.AccessOperatorEquals, actual: 5, expected: `"5"`, want: false},
		{name: "roles", operator: models.AccessOperatorContainsAny, actual: []string{"viewer", "auditor"}, expected: `["auditor"]`, want: true},
		{name: "CIDR", operator: models.AccessOperatorInCIDR, actual: "192.0.2.10", expected: `["10.0.0.0/8","192.0.2.0/24"]`, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expected, err := decodeJSONValue(json.RawMessage(test.expected))
			if err != nil {
				t.Fatal(err)
			}
			got, err := evaluateAccessOperator(test.operator, test.actual, expected, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got=%t want=%t", got, test.want)
			}
		})
	}
}
