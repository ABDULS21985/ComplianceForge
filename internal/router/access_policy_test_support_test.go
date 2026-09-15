package router

import (
	"context"
	"time"

	"github.com/complianceforge/platform/internal/models"
)

const testAccessAssignmentID = "83000000-0000-0000-0000-000000000001"

// routerPolicyAccessService keeps route-walk and OpenAPI mount tests focused on
// transport composition without requiring PostgreSQL.
type routerPolicyAccessService struct{}

func (routerPolicyAccessService) ListPolicies(context.Context, string, models.AccessPolicyListFilter) ([]models.AccessPolicy, int, error) {
	return []models.AccessPolicy{}, 0, nil
}
func (routerPolicyAccessService) GetPolicy(_ context.Context, organizationID, policyID string) (*models.AccessPolicy, error) {
	return &models.AccessPolicy{ID: policyID, OrganizationID: organizationID, Name: "Scoped review", Effect: models.AccessPolicyEffectAllow,
		ResourceType: "risks", Actions: []string{"read"}, SubjectConditions: []models.AccessCondition{}, ResourceConditions: []models.AccessCondition{},
		EnvironmentConditions: []models.AccessCondition{}, IsActive: true, Version: 1}, nil
}
func (service routerPolicyAccessService) CreatePolicy(ctx context.Context, organizationID, _, _ string, _ models.AccessPolicyInput) (*models.AccessPolicy, error) {
	return service.GetPolicy(ctx, organizationID, testPolicyID)
}
func (service routerPolicyAccessService) UpdatePolicy(ctx context.Context, organizationID, policyID, _, _ string, _ models.AccessPolicyInput) (*models.AccessPolicy, error) {
	return service.GetPolicy(ctx, organizationID, policyID)
}
func (routerPolicyAccessService) DeletePolicy(context.Context, string, string, string, string, int64, string) error {
	return nil
}
func (routerPolicyAccessService) ListAssignments(context.Context, string, string) ([]models.AccessPolicyAssignment, error) {
	return []models.AccessPolicyAssignment{}, nil
}
func (routerPolicyAccessService) CreateAssignment(_ context.Context, organizationID, policyID, actorID, _ string, input models.AccessPolicyAssignmentInput) (*models.AccessPolicyAssignment, error) {
	return &models.AccessPolicyAssignment{ID: testAccessAssignmentID, OrganizationID: organizationID, PolicyID: policyID,
		AssigneeType: input.AssigneeType, AssigneeID: input.AssigneeID, CreatedBy: actorID}, nil
}
func (routerPolicyAccessService) RemoveAssignment(context.Context, string, string, string, string, string, string) error {
	return nil
}
func (routerPolicyAccessService) ListFieldPermissions(context.Context, string, string, string) ([]models.AccessFieldPermission, error) {
	return []models.AccessFieldPermission{}, nil
}
func (routerPolicyAccessService) UpsertFieldPermission(_ context.Context, organizationID, policyID, _, _ string, input models.AccessFieldPermissionInput) (*models.AccessFieldPermission, error) {
	return &models.AccessFieldPermission{ID: testAccessAssignmentID, OrganizationID: organizationID, PolicyID: policyID,
		ResourceType: input.ResourceType, FieldPath: input.FieldPath, Classification: input.Classification,
		Visibility: input.Visibility, MaskStrategy: input.MaskStrategy, Version: 1}, nil
}
func (routerPolicyAccessService) DeleteFieldPermission(context.Context, string, string, string, string, string, int64, string) error {
	return nil
}
func (routerPolicyAccessService) ListObjectGrants(context.Context, string, models.AccessObjectGrantFilter) ([]models.AccessObjectGrant, int, error) {
	return []models.AccessObjectGrant{}, 0, nil
}
func (routerPolicyAccessService) CreateObjectGrant(_ context.Context, organizationID, actorID, _ string, input models.AccessObjectGrantInput) (*models.AccessObjectGrant, error) {
	return &models.AccessObjectGrant{ID: testAccessAssignmentID, OrganizationID: organizationID, SubjectID: input.SubjectID,
		ResourceType: input.ResourceType, ResourceID: input.ResourceID, Actions: input.Actions, Status: models.AccessObjectGrantPending,
		SponsorID: actorID, ValidFrom: input.ValidFrom, ValidUntil: input.ValidUntil, Reason: input.Reason, Version: 1}, nil
}
func (routerPolicyAccessService) DecideObjectGrant(_ context.Context, organizationID, grantID, actorID, _ string, _ models.AccessObjectGrantDecisionInput) (*models.AccessObjectGrant, error) {
	now := time.Now().UTC()
	return &models.AccessObjectGrant{ID: grantID, OrganizationID: organizationID, Status: models.AccessObjectGrantApproved,
		ApprovedBy: &actorID, ApprovedAt: &now, Version: 2}, nil
}
func (routerPolicyAccessService) RevokeObjectGrant(_ context.Context, organizationID, grantID, actorID, _ string, _ models.AccessObjectGrantRevocationInput) (*models.AccessObjectGrant, error) {
	now := time.Now().UTC()
	return &models.AccessObjectGrant{ID: grantID, OrganizationID: organizationID, Status: models.AccessObjectGrantRevoked,
		RevokedBy: &actorID, RevokedAt: &now, Version: 2}, nil
}
func (routerPolicyAccessService) LoadEvaluationBundle(context.Context, string, string, string, string, string, time.Time) (*models.AccessEvaluationBundle, error) {
	return &models.AccessEvaluationBundle{}, nil
}
func (routerPolicyAccessService) RecordDecision(context.Context, models.AccessDecisionEvidence) error {
	return nil
}
func (routerPolicyAccessService) ListDecisionEvidence(context.Context, string, models.AccessDecisionEvidenceFilter) ([]models.AccessDecisionEvidence, int, error) {
	return []models.AccessDecisionEvidence{}, 0, nil
}
func (routerPolicyAccessService) CertifyPolicy(_ context.Context, organizationID, policyID, actorID, _ string, input models.AccessPolicyCertificationInput) (*models.AccessPolicyCertification, error) {
	return &models.AccessPolicyCertification{ID: testAccessAssignmentID, OrganizationID: organizationID, PolicyID: policyID,
		PolicyVersion: input.ExpectedVersion, CertifiedBy: actorID, Decision: input.Decision, Reason: input.Reason}, nil
}
func (routerPolicyAccessService) ListPolicyCertifications(context.Context, string, string, models.PaginationRequest) ([]models.AccessPolicyCertification, int, error) {
	return []models.AccessPolicyCertification{}, 0, nil
}
