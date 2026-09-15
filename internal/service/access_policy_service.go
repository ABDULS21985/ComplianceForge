package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/accesscontrol"
	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrAccessPolicyNotFound = errors.New("access control record was not found")
	ErrAccessPolicyConflict = errors.New("access control record changed or conflicts")
	ErrAccessPolicyState    = errors.New("access control record state prevents the operation")
)

// PolicyAccessStore is the typed administration and evidence contract. Its
// concrete PostgreSQL implementation also satisfies AccessPolicyDecisionStore.
type PolicyAccessStore interface {
	AccessPolicyDecisionStore
	ListPolicies(context.Context, string, models.AccessPolicyListFilter) ([]models.AccessPolicy, int, error)
	GetPolicy(context.Context, string, string) (*models.AccessPolicy, error)
	CreatePolicy(context.Context, string, string, string, models.AccessPolicyInput) (*models.AccessPolicy, error)
	UpdatePolicy(context.Context, string, string, string, string, models.AccessPolicyInput) (*models.AccessPolicy, error)
	DeletePolicy(context.Context, string, string, string, string, int64, string) error
	ListAssignments(context.Context, string, string) ([]models.AccessPolicyAssignment, error)
	CreateAssignment(context.Context, string, string, string, string, models.AccessPolicyAssignmentInput) (*models.AccessPolicyAssignment, error)
	RemoveAssignment(context.Context, string, string, string, string, string, string) error
	ListFieldPermissions(context.Context, string, string, string) ([]models.AccessFieldPermission, error)
	UpsertFieldPermission(context.Context, string, string, string, string, models.AccessFieldPermissionInput) (*models.AccessFieldPermission, error)
	DeleteFieldPermission(context.Context, string, string, string, string, string, int64, string) error
	ListObjectGrants(context.Context, string, models.AccessObjectGrantFilter) ([]models.AccessObjectGrant, int, error)
	CreateObjectGrant(context.Context, string, string, string, models.AccessObjectGrantInput) (*models.AccessObjectGrant, error)
	DecideObjectGrant(context.Context, string, string, string, string, models.AccessObjectGrantDecisionInput) (*models.AccessObjectGrant, error)
	RevokeObjectGrant(context.Context, string, string, string, string, models.AccessObjectGrantRevocationInput) (*models.AccessObjectGrant, error)
	ListDecisionEvidence(context.Context, string, models.AccessDecisionEvidenceFilter) ([]models.AccessDecisionEvidence, int, error)
	CertifyPolicy(context.Context, string, string, string, string, models.AccessPolicyCertificationInput) (*models.AccessPolicyCertification, error)
	ListPolicyCertifications(context.Context, string, string, models.PaginationRequest) ([]models.AccessPolicyCertification, int, error)
}

type PolicyAccessService struct {
	store  PolicyAccessStore
	logger zerolog.Logger
	now    func() time.Time
}

func NewPolicyAccessService(store PolicyAccessStore, logger zerolog.Logger) (*PolicyAccessService, error) {
	if store == nil {
		return nil, errors.New("policy access store is required")
	}
	return &PolicyAccessService{
		store: store, logger: logger.With().Str("service", "policy_access").Logger(), now: time.Now,
	}, nil
}

func (s *PolicyAccessService) ListPolicies(
	ctx context.Context, organizationID string, filter models.AccessPolicyListFilter,
) ([]models.AccessPolicy, int, error) {
	if err := validateAccessUUID("organization_id", organizationID); err != nil {
		return nil, 0, err
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if utf8.RuneCountInString(filter.Search) > 200 {
		return nil, 0, invalidAccessPolicy(errors.New("search cannot exceed 200 characters"))
	}
	if filter.ResourceType != "" {
		filter.ResourceType = canonicalAccessResource(filter.ResourceType)
		if _, ok := supportedAccessResources[filter.ResourceType]; !ok {
			return nil, 0, invalidAccessPolicy(errors.New("resource_type is unsupported"))
		}
	}
	if filter.Effect != "" && filter.Effect != models.AccessPolicyEffectAllow && filter.Effect != models.AccessPolicyEffectDeny {
		return nil, 0, invalidAccessPolicy(errors.New("effect must be allow or deny"))
	}
	filter.PaginationRequest = normalizeAccessPagination(filter.PaginationRequest)
	items, total, err := s.store.ListPolicies(ctx, organizationID, filter)
	return items, total, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) GetPolicy(ctx context.Context, organizationID, policyID string) (*models.AccessPolicy, error) {
	if err := validateAccessObject(organizationID, policyID); err != nil {
		return nil, err
	}
	item, err := s.store.GetPolicy(ctx, organizationID, policyID)
	return item, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) CreatePolicy(
	ctx context.Context, organizationID, actorID, requestID string, input models.AccessPolicyInput,
) (*models.AccessPolicy, error) {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	normalizeAccessPolicyInput(&input)
	input.ExpectedVersion = nil
	if err := ValidateAccessPolicyInput(input); err != nil {
		return nil, err
	}
	item, err := s.store.CreatePolicy(ctx, organizationID, actorID, requestID, input)
	if err != nil {
		return nil, mapAccessPolicyError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("policy_id", item.ID).Msg("access policy created")
	return item, nil
}

func (s *PolicyAccessService) UpdatePolicy(
	ctx context.Context, organizationID, policyID, actorID, requestID string, input models.AccessPolicyInput,
) (*models.AccessPolicy, error) {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	if err := validateAccessUUID("policy_id", policyID); err != nil {
		return nil, err
	}
	normalizeAccessPolicyInput(&input)
	if input.ExpectedVersion == nil {
		return nil, invalidAccessPolicy(errors.New("expected_version is required"))
	}
	if err := ValidateAccessPolicyInput(input); err != nil {
		return nil, err
	}
	item, err := s.store.UpdatePolicy(ctx, organizationID, policyID, actorID, requestID, input)
	if err != nil {
		return nil, mapAccessPolicyError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("policy_id", policyID).
		Int64("version", item.Version).Msg("access policy updated")
	return item, nil
}

func (s *PolicyAccessService) DeletePolicy(
	ctx context.Context, organizationID, policyID, actorID, requestID string, expectedVersion int64, reason string,
) error {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return err
	}
	if err := validateAccessUUID("policy_id", policyID); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if expectedVersion < 1 {
		return invalidAccessPolicy(errors.New("expected_version must be positive"))
	}
	if err := validateChangeReason(reason); err != nil {
		return invalidAccessPolicy(err)
	}
	if err := s.store.DeletePolicy(ctx, organizationID, policyID, actorID, requestID, expectedVersion, reason); err != nil {
		return mapAccessPolicyError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("policy_id", policyID).Msg("access policy retired")
	return nil
}

func (s *PolicyAccessService) ListAssignments(
	ctx context.Context, organizationID, policyID string,
) ([]models.AccessPolicyAssignment, error) {
	if err := validateAccessObject(organizationID, policyID); err != nil {
		return nil, err
	}
	items, err := s.store.ListAssignments(ctx, organizationID, policyID)
	return items, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) CreateAssignment(
	ctx context.Context, organizationID, policyID, actorID, requestID string, input models.AccessPolicyAssignmentInput,
) (*models.AccessPolicyAssignment, error) {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	if err := validateAccessUUID("policy_id", policyID); err != nil {
		return nil, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.AssigneeID != nil {
		trimmed := strings.TrimSpace(*input.AssigneeID)
		input.AssigneeID = &trimmed
	}
	if err := ValidateAccessPolicyAssignmentInput(input); err != nil {
		return nil, err
	}
	item, err := s.store.CreateAssignment(ctx, organizationID, policyID, actorID, requestID, input)
	return item, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) RemoveAssignment(
	ctx context.Context, organizationID, policyID, assignmentID, actorID, requestID, reason string,
) error {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return err
	}
	if err := validateAccessUUID("policy_id", policyID); err != nil {
		return err
	}
	if err := validateAccessUUID("assignment_id", assignmentID); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if err := validateChangeReason(reason); err != nil {
		return invalidAccessPolicy(err)
	}
	return mapAccessPolicyError(s.store.RemoveAssignment(ctx, organizationID, policyID, assignmentID, actorID, requestID, reason))
}

func (s *PolicyAccessService) ListFieldPermissions(
	ctx context.Context, organizationID, policyID, resourceType string,
) ([]models.AccessFieldPermission, error) {
	if err := validateAccessUUID("organization_id", organizationID); err != nil {
		return nil, err
	}
	if policyID != "" {
		if err := validateAccessUUID("policy_id", policyID); err != nil {
			return nil, err
		}
	}
	resourceType = canonicalAccessResource(resourceType)
	if resourceType != "" {
		if _, ok := supportedAccessResources[resourceType]; !ok || resourceType == "*" {
			return nil, invalidAccessPolicy(errors.New("resource_type is unsupported"))
		}
	}
	items, err := s.store.ListFieldPermissions(ctx, organizationID, policyID, resourceType)
	return items, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) UpsertFieldPermission(
	ctx context.Context, organizationID, policyID, actorID, requestID string, input models.AccessFieldPermissionInput,
) (*models.AccessFieldPermission, error) {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	if err := validateAccessUUID("policy_id", policyID); err != nil {
		return nil, err
	}
	normalizeAccessFieldPermissionInput(&input)
	if err := validateChangeReason(input.Reason); err != nil {
		return nil, invalidAccessPolicy(err)
	}
	rule := models.AccessFieldPermission{
		ResourceType: input.ResourceType, FieldPath: input.FieldPath, Classification: input.Classification,
		Visibility: input.Visibility, MaskStrategy: input.MaskStrategy, MaskPattern: input.MaskPattern,
	}
	if err := ValidateAccessFieldPermission(rule); err != nil {
		return nil, err
	}
	item, err := s.store.UpsertFieldPermission(ctx, organizationID, policyID, actorID, requestID, input)
	return item, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) DeleteFieldPermission(
	ctx context.Context, organizationID, policyID, fieldPermissionID, actorID, requestID string, expectedVersion int64, reason string,
) error {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return err
	}
	for name, value := range map[string]string{"policy_id": policyID, "field_permission_id": fieldPermissionID} {
		if err := validateAccessUUID(name, value); err != nil {
			return err
		}
	}
	reason = strings.TrimSpace(reason)
	if expectedVersion < 1 {
		return invalidAccessPolicy(errors.New("expected_version must be positive"))
	}
	if err := validateChangeReason(reason); err != nil {
		return invalidAccessPolicy(err)
	}
	return mapAccessPolicyError(s.store.DeleteFieldPermission(
		ctx, organizationID, policyID, fieldPermissionID, actorID, requestID, expectedVersion, reason,
	))
}

func (s *PolicyAccessService) ListObjectGrants(
	ctx context.Context, organizationID string, filter models.AccessObjectGrantFilter,
) ([]models.AccessObjectGrant, int, error) {
	if err := validateAccessUUID("organization_id", organizationID); err != nil {
		return nil, 0, err
	}
	for name, value := range map[string]string{"subject_id": filter.SubjectID, "resource_id": filter.ResourceID} {
		if value != "" {
			if err := validateAccessUUID(name, value); err != nil {
				return nil, 0, err
			}
		}
	}
	if filter.ResourceType != "" {
		filter.ResourceType = canonicalAccessResource(filter.ResourceType)
		if _, ok := supportedAccessResources[filter.ResourceType]; !ok || filter.ResourceType == "*" {
			return nil, 0, invalidAccessPolicy(errors.New("resource_type is unsupported"))
		}
	}
	switch filter.Status {
	case "", models.AccessObjectGrantPending, models.AccessObjectGrantApproved,
		models.AccessObjectGrantRejected, models.AccessObjectGrantRevoked, models.AccessObjectGrantExpired:
	default:
		return nil, 0, invalidAccessPolicy(errors.New("grant status is unsupported"))
	}
	filter.PaginationRequest = normalizeAccessPagination(filter.PaginationRequest)
	items, total, err := s.store.ListObjectGrants(ctx, organizationID, filter)
	return items, total, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) CreateObjectGrant(
	ctx context.Context, organizationID, actorID, requestID string, input models.AccessObjectGrantInput,
) (*models.AccessObjectGrant, error) {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	normalizeAccessObjectGrantInput(&input)
	input.SponsorID = actorID
	input.ApprovedAt, input.ApprovedBy, input.ExpectedVersion = nil, nil, nil
	if err := ValidateAccessObjectGrantInput(input); err != nil {
		return nil, err
	}
	item, err := s.store.CreateObjectGrant(ctx, organizationID, actorID, requestID, input)
	return item, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) DecideObjectGrant(
	ctx context.Context, organizationID, grantID, actorID, requestID string, input models.AccessObjectGrantDecisionInput,
) (*models.AccessObjectGrant, error) {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	if err := validateAccessUUID("grant_id", grantID); err != nil {
		return nil, err
	}
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Decision != "approve" && input.Decision != "reject" {
		return nil, invalidAccessPolicy(errors.New("decision must be approve or reject"))
	}
	if input.ExpectedVersion < 1 {
		return nil, invalidAccessPolicy(errors.New("expected_version must be positive"))
	}
	if err := validateChangeReason(input.Reason); err != nil {
		return nil, invalidAccessPolicy(err)
	}
	item, err := s.store.DecideObjectGrant(ctx, organizationID, grantID, actorID, requestID, input)
	return item, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) RevokeObjectGrant(
	ctx context.Context, organizationID, grantID, actorID, requestID string, input models.AccessObjectGrantRevocationInput,
) (*models.AccessObjectGrant, error) {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	if err := validateAccessUUID("grant_id", grantID); err != nil {
		return nil, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ExpectedVersion < 1 {
		return nil, invalidAccessPolicy(errors.New("expected_version must be positive"))
	}
	if err := validateChangeReason(input.Reason); err != nil {
		return nil, invalidAccessPolicy(err)
	}
	item, err := s.store.RevokeObjectGrant(ctx, organizationID, grantID, actorID, requestID, input)
	return item, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) ListDecisionEvidence(
	ctx context.Context, organizationID string, filter models.AccessDecisionEvidenceFilter,
) ([]models.AccessDecisionEvidence, int, error) {
	if err := validateAccessUUID("organization_id", organizationID); err != nil {
		return nil, 0, err
	}
	for name, value := range map[string]string{"subject_id": filter.SubjectID, "resource_id": filter.ResourceID} {
		if value != "" {
			if err := validateAccessUUID(name, value); err != nil {
				return nil, 0, err
			}
		}
	}
	if filter.ResourceType != "" {
		filter.ResourceType = canonicalAccessResource(filter.ResourceType)
		if _, ok := supportedAccessResources[filter.ResourceType]; !ok {
			return nil, 0, invalidAccessPolicy(errors.New("resource_type is unsupported"))
		}
	}
	if filter.Action != "" {
		filter.Action = strings.ToLower(strings.TrimSpace(filter.Action))
		if _, ok := supportedAccessActions[filter.Action]; !ok || filter.Action == "*" {
			return nil, 0, invalidAccessPolicy(errors.New("action is unsupported"))
		}
	}
	if filter.Decision != "" && filter.Decision != "allow" && filter.Decision != "deny" {
		return nil, 0, invalidAccessPolicy(errors.New("decision must be allow or deny"))
	}
	if filter.From != nil && filter.Until != nil && !filter.Until.After(*filter.From) {
		return nil, 0, invalidAccessPolicy(errors.New("until must be after from"))
	}
	filter.PaginationRequest = normalizeAccessPagination(filter.PaginationRequest)
	items, total, err := s.store.ListDecisionEvidence(ctx, organizationID, filter)
	return items, total, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) CertifyPolicy(
	ctx context.Context, organizationID, policyID, actorID, requestID string, input models.AccessPolicyCertificationInput,
) (*models.AccessPolicyCertification, error) {
	if err := validateAccessActor(organizationID, actorID, requestID); err != nil {
		return nil, err
	}
	if err := validateAccessUUID("policy_id", policyID); err != nil {
		return nil, err
	}
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Decision != "certified" && input.Decision != "changes_required" {
		return nil, invalidAccessPolicy(errors.New("decision must be certified or changes_required"))
	}
	if input.ExpectedVersion < 1 {
		return nil, invalidAccessPolicy(errors.New("expected_version must be positive"))
	}
	if err := validateChangeReason(input.Reason); err != nil {
		return nil, invalidAccessPolicy(err)
	}
	item, err := s.store.CertifyPolicy(ctx, organizationID, policyID, actorID, requestID, input)
	return item, mapAccessPolicyError(err)
}

func (s *PolicyAccessService) ListPolicyCertifications(
	ctx context.Context, organizationID, policyID string, pagination models.PaginationRequest,
) ([]models.AccessPolicyCertification, int, error) {
	if err := validateAccessObject(organizationID, policyID); err != nil {
		return nil, 0, err
	}
	pagination = normalizeAccessPagination(pagination)
	items, total, err := s.store.ListPolicyCertifications(ctx, organizationID, policyID, pagination)
	return items, total, mapAccessPolicyError(err)
}

func normalizeAccessPolicyInput(input *models.AccessPolicyInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.ResourceType = canonicalAccessResource(input.ResourceType)
	input.Reason = strings.TrimSpace(input.Reason)
	for index := range input.Actions {
		input.Actions[index] = strings.ToLower(strings.TrimSpace(input.Actions[index]))
	}
	sort.Strings(input.Actions)
	for _, conditions := range [][]models.AccessCondition{
		input.SubjectConditions, input.ResourceConditions, input.EnvironmentConditions,
	} {
		for index := range conditions {
			conditions[index].Attribute = strings.ToLower(strings.TrimSpace(conditions[index].Attribute))
			conditions[index].Operator = models.AccessConditionOperator(strings.ToLower(strings.TrimSpace(string(conditions[index].Operator))))
		}
	}
}

func normalizeAccessFieldPermissionInput(input *models.AccessFieldPermissionInput) {
	input.ResourceType = canonicalAccessResource(input.ResourceType)
	input.FieldPath = strings.TrimSpace(input.FieldPath)
	input.MaskPattern = strings.TrimSpace(input.MaskPattern)
	input.Reason = strings.TrimSpace(input.Reason)
}

func normalizeAccessObjectGrantInput(input *models.AccessObjectGrantInput) {
	input.SubjectID = strings.TrimSpace(input.SubjectID)
	input.ResourceType = canonicalAccessResource(input.ResourceType)
	input.ResourceID = strings.TrimSpace(input.ResourceID)
	input.SponsorID = strings.TrimSpace(input.SponsorID)
	input.WatermarkText = strings.TrimSpace(input.WatermarkText)
	input.Reason = strings.TrimSpace(input.Reason)
	for index := range input.Actions {
		input.Actions[index] = strings.ToLower(strings.TrimSpace(input.Actions[index]))
	}
	sort.Strings(input.Actions)
}

func normalizeAccessPagination(input models.PaginationRequest) models.PaginationRequest {
	if input.Page < 1 {
		input.Page = 1
	}
	if input.PageSize < 1 {
		input.PageSize = 20
	}
	if input.PageSize > 100 {
		input.PageSize = 100
	}
	return input
}

func validateAccessActor(organizationID, actorID, requestID string) error {
	if err := validateAccessUUID("organization_id", organizationID); err != nil {
		return err
	}
	if err := validateAccessUUID("actor_id", actorID); err != nil {
		return err
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > 64 || requestIDFromAttributes(map[string]any{"request_id": requestID}) == "" {
		return invalidAccessPolicy(errors.New("request_id is invalid"))
	}
	return nil
}

func validateAccessObject(organizationID, objectID string) error {
	if err := validateAccessUUID("organization_id", organizationID); err != nil {
		return err
	}
	return validateAccessUUID("id", objectID)
}

func validateAccessUUID(name, value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return invalidAccessPolicy(fmt.Errorf("%s must be a UUID", name))
	}
	return nil
}

func mapAccessPolicyError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, accesscontrol.ErrNotFound):
		return fmt.Errorf("%w: %v", ErrAccessPolicyNotFound, err)
	case errors.Is(err, accesscontrol.ErrConflict):
		return fmt.Errorf("%w: %v", ErrAccessPolicyConflict, err)
	case errors.Is(err, accesscontrol.ErrInvalidRelation):
		return fmt.Errorf("%w: %v", ErrInvalidAccessPolicy, err)
	case errors.Is(err, accesscontrol.ErrState), errors.Is(err, accesscontrol.ErrImmutable):
		return fmt.Errorf("%w: %v", ErrAccessPolicyState, err)
	default:
		return err
	}
}
