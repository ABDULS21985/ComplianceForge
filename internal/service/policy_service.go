package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrPolicyNotFound          = errors.New("policy not found")
	ErrPolicyVersionNotFound   = errors.New("policy version not found")
	ErrPolicyWorkflowNotFound  = errors.New("active policy approval workflow not found")
	ErrPolicyReviewNotFound    = errors.New("policy review not found")
	ErrPolicyExceptionNotFound = errors.New("policy exception not found")
	ErrPolicyConflict          = errors.New("policy reference or version already exists")
	ErrPolicyInvalid           = errors.New("invalid policy")
	ErrPolicyInvalidID         = errors.New("invalid policy identifier")
	ErrPolicyInvalidReference  = errors.New("policy references a resource outside this organization")
	ErrPolicyInvalidTransition = errors.New("invalid policy lifecycle transition")
	ErrPolicyApprovalForbidden = errors.New("current approval step is not assigned to this principal")
)

// PolicyManagementRepository is the persistence contract for the canonical
// policy tables introduced by migrations 000013 and 000014.
type PolicyManagementRepository interface {
	Create(context.Context, string, string, models.PolicyCreateInput, int) (*models.Policy, error)
	GetByID(context.Context, string, string) (*models.Policy, error)
	Update(context.Context, string, *models.Policy) (*models.Policy, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.PolicyListFilter) ([]models.Policy, int, error)
	ListCategories(context.Context, string) ([]models.PolicyCategory, error)
	CreateVersion(context.Context, string, string, string, models.PolicyVersionInput, int) (*models.PolicyVersion, error)
	GetVersion(context.Context, string, string, string) (*models.PolicyVersion, error)
	ListVersions(context.Context, string, string, models.PaginationRequest) ([]models.PolicyVersion, int, error)
	CreateApprovalWorkflow(context.Context, string, string, string, models.PolicySubmitInput) (*models.PolicyApprovalWorkflow, error)
	GetActiveApprovalWorkflow(context.Context, string, string) (*models.PolicyApprovalWorkflow, error)
	DecideApproval(context.Context, string, string, string, string, models.PolicyApprovalDecisionInput) (*models.PolicyApprovalWorkflow, error)
	Publish(context.Context, string, string, string) (*models.Policy, error)
	CreateReview(context.Context, string, string, models.PolicyReviewInput) (*models.PolicyReview, error)
	GetReview(context.Context, string, string, string) (*models.PolicyReview, error)
	UpdateReview(context.Context, string, string, *models.PolicyReview) (*models.PolicyReview, error)
	ListReviews(context.Context, string, string, models.PaginationRequest) ([]models.PolicyReview, int, error)
	Acknowledge(context.Context, string, string, string, string, models.PolicyAttestationInput) (*models.PolicyAttestation, error)
	ListAttestations(context.Context, string, string, models.PaginationRequest) ([]models.PolicyAttestation, int, error)
	CreateException(context.Context, string, string, string, models.PolicyExceptionInput) (*models.PolicyException, error)
	GetException(context.Context, string, string, string) (*models.PolicyException, error)
	UpdateExceptionDecision(context.Context, string, string, string, string, models.PolicyExceptionDecisionInput) (*models.PolicyException, error)
	ListExceptions(context.Context, string, string, models.PaginationRequest) ([]models.PolicyException, int, error)
}

type PolicyService struct {
	repository PolicyManagementRepository
	logger     zerolog.Logger
}

func NewPolicyService(repository PolicyManagementRepository, logger zerolog.Logger) *PolicyService {
	return &PolicyService{repository: repository, logger: logger.With().Str("service", "policy").Logger()}
}

func (s *PolicyService) Create(ctx context.Context, orgID, userID string, input models.PolicyCreateInput) (*models.Policy, error) {
	if !validUUID(orgID) || !validUUID(userID) {
		return nil, ErrPolicyInvalidID
	}
	normalizePolicyCreate(&input)
	if err := validatePolicyCreate(input); err != nil {
		return nil, err
	}
	item, err := s.repository.Create(ctx, orgID, userID, input, policyWordCount(input.InitialVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyInvalidReference
	}
	if err != nil {
		return nil, mapPolicyWriteError(err)
	}
	s.logger.Info().Str("organization_id", orgID).Str("policy_id", item.ID).Str("policy_ref", item.PolicyRef).Msg("policy created")
	return item, nil
}

func (s *PolicyService) GetByID(ctx context.Context, orgID, id string) (*models.Policy, error) {
	if !validUUID(orgID) || !validUUID(id) {
		return nil, ErrPolicyInvalidID
	}
	item, err := s.repository.GetByID(ctx, orgID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyNotFound
	}
	return item, err
}

func (s *PolicyService) Update(ctx context.Context, orgID, id string, patch models.PolicyPatch) (*models.Policy, error) {
	if !validUUID(orgID) || !validUUID(id) {
		return nil, ErrPolicyInvalidID
	}
	if policyPatchEmpty(patch) {
		return nil, fmt.Errorf("%w: no fields supplied", ErrPolicyInvalid)
	}
	item, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if item.Status != models.PolicyStateDraft {
		return nil, fmt.Errorf("%w: policy content and metadata are editable only in draft", ErrPolicyInvalidTransition)
	}
	applyPolicyPatch(item, patch)
	if err := validatePolicy(item); err != nil {
		return nil, err
	}
	updated, err := s.repository.Update(ctx, orgID, item)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyInvalidReference
	}
	return updated, mapPolicyWriteError(err)
}

// AssignOwner is separate from general editing so ownership can be maintained
// after publication without making published content mutable.
func (s *PolicyService) AssignOwner(ctx context.Context, orgID, id string, ownerID, approverID *string) (*models.Policy, error) {
	if !validUUID(orgID) || !validUUID(id) {
		return nil, ErrPolicyInvalidID
	}
	if ownerID == nil && approverID == nil {
		return nil, fmt.Errorf("%w: owner_user_id or approver_user_id is required", ErrPolicyInvalid)
	}
	for _, candidate := range []*string{ownerID, approverID} {
		if candidate != nil && strings.TrimSpace(*candidate) != "" && !validUUID(strings.TrimSpace(*candidate)) {
			return nil, ErrPolicyInvalidID
		}
	}
	item, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if ownerID != nil {
		item.OwnerUserID = nullableID(strings.TrimSpace(*ownerID))
	}
	if approverID != nil {
		item.ApproverUserID = nullableID(strings.TrimSpace(*approverID))
	}
	updated, err := s.repository.Update(ctx, orgID, item)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyInvalidReference
	}
	return updated, mapPolicyWriteError(err)
}

func (s *PolicyService) Delete(ctx context.Context, orgID, id string) error {
	item, err := s.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if !allowed(item.Status, models.PolicyStateDraft, models.PolicyStateArchived, models.PolicyStateRetired) {
		return fmt.Errorf("%w: only draft, archived, or retired policies may be deleted", ErrPolicyInvalidTransition)
	}
	err = s.repository.Delete(ctx, orgID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPolicyNotFound
	}
	return err
}

func (s *PolicyService) List(ctx context.Context, orgID string, filter models.PolicyListFilter) ([]models.Policy, int, error) {
	if !validUUID(orgID) {
		return nil, 0, ErrPolicyInvalidID
	}
	filter.PaginationRequest = normalizePolicyPagination(filter.PaginationRequest)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Classification = strings.TrimSpace(filter.Classification)
	filter.CategoryID = strings.TrimSpace(filter.CategoryID)
	filter.OwnerUserID = strings.TrimSpace(filter.OwnerUserID)
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && !allowed(filter.Status, policyStatuses()...) {
		return nil, 0, fmt.Errorf("%w: unsupported status filter", ErrPolicyInvalid)
	}
	if filter.Classification != "" && !allowed(filter.Classification, "public", "internal", "confidential", "restricted") {
		return nil, 0, fmt.Errorf("%w: unsupported classification filter", ErrPolicyInvalid)
	}
	for _, id := range []string{filter.CategoryID, filter.OwnerUserID} {
		if id != "" && !validUUID(id) {
			return nil, 0, ErrPolicyInvalidID
		}
	}
	return s.repository.List(ctx, orgID, filter)
}

func (s *PolicyService) ListCategories(ctx context.Context, orgID string) ([]models.PolicyCategory, error) {
	if !validUUID(orgID) {
		return nil, ErrPolicyInvalidID
	}
	return s.repository.ListCategories(ctx, orgID)
}

func (s *PolicyService) CreateVersion(ctx context.Context, orgID, policyID, userID string, input models.PolicyVersionInput) (*models.PolicyVersion, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(userID) {
		return nil, ErrPolicyInvalidID
	}
	policy, err := s.GetByID(ctx, orgID, policyID)
	if err != nil {
		return nil, err
	}
	if !allowed(policy.Status, models.PolicyStateDraft, models.PolicyStateApproved, models.PolicyStatePublished, models.PolicyStateArchived) {
		return nil, fmt.Errorf("%w: a version cannot be created while approval is active or after retirement", ErrPolicyInvalidTransition)
	}
	normalizeVersion(&input, policy.Title)
	if err := validateVersion(input); err != nil {
		return nil, err
	}
	item, err := s.repository.CreateVersion(ctx, orgID, policyID, userID, input, policyWordCount(input))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyNotFound
	}
	return item, mapPolicyWriteError(err)
}

func (s *PolicyService) GetVersion(ctx context.Context, orgID, policyID, versionID string) (*models.PolicyVersion, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(versionID) {
		return nil, ErrPolicyInvalidID
	}
	item, err := s.repository.GetVersion(ctx, orgID, policyID, versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyVersionNotFound
	}
	return item, err
}

func (s *PolicyService) ListVersions(ctx context.Context, orgID, policyID string, pagination models.PaginationRequest) ([]models.PolicyVersion, int, error) {
	if _, err := s.GetByID(ctx, orgID, policyID); err != nil {
		return nil, 0, err
	}
	return s.repository.ListVersions(ctx, orgID, policyID, normalizePolicyPagination(pagination))
}

func (s *PolicyService) SubmitForApproval(ctx context.Context, orgID, policyID, userID string, input models.PolicySubmitInput) (*models.PolicyApprovalWorkflow, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(userID) {
		return nil, ErrPolicyInvalidID
	}
	policy, err := s.GetByID(ctx, orgID, policyID)
	if err != nil {
		return nil, err
	}
	if policy.Status != models.PolicyStateDraft {
		return nil, fmt.Errorf("%w: only draft policies may enter approval", ErrPolicyInvalidTransition)
	}
	normalizeSubmit(&input)
	if err := validateSubmit(input); err != nil {
		return nil, err
	}
	item, err := s.repository.CreateApprovalWorkflow(ctx, orgID, policyID, userID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyInvalidReference
	}
	return item, mapPolicyWriteError(err)
}

func (s *PolicyService) GetActiveApproval(ctx context.Context, orgID, policyID string) (*models.PolicyApprovalWorkflow, error) {
	if !validUUID(orgID) || !validUUID(policyID) {
		return nil, ErrPolicyInvalidID
	}
	item, err := s.repository.GetActiveApprovalWorkflow(ctx, orgID, policyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyWorkflowNotFound
	}
	return item, err
}

func (s *PolicyService) DecideApproval(ctx context.Context, orgID, policyID, userID, role string, input models.PolicyApprovalDecisionInput) (*models.PolicyApprovalWorkflow, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(userID) {
		return nil, ErrPolicyInvalidID
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	if !allowed(decision, "approve", "reject") {
		return nil, fmt.Errorf("%w: decision must be approve or reject", ErrPolicyInvalid)
	}
	workflow, err := s.GetActiveApproval(ctx, orgID, policyID)
	if err != nil {
		return nil, err
	}
	if workflow.CurrentStep < 1 || workflow.CurrentStep > len(workflow.Steps) {
		return nil, ErrPolicyWorkflowNotFound
	}
	step := workflow.Steps[workflow.CurrentStep-1]
	assigned := (step.ApproverUserID != nil && *step.ApproverUserID == userID) ||
		(step.DelegationUserID != nil && *step.DelegationUserID == userID) ||
		(step.ApproverUserID == nil && step.ApproverRole != nil && *step.ApproverRole == role)
	if !assigned {
		return nil, ErrPolicyApprovalForbidden
	}
	if step.Status != "pending" {
		return nil, fmt.Errorf("%w: approval step is already decided", ErrPolicyInvalidTransition)
	}
	if input.Comments != nil && len(strings.TrimSpace(*input.Comments)) > 4000 {
		return nil, fmt.Errorf("%w: comments exceed 4000 characters", ErrPolicyInvalid)
	}
	if decision == "reject" && (input.Comments == nil || strings.TrimSpace(*input.Comments) == "") {
		return nil, fmt.Errorf("%w: rejection comments are required", ErrPolicyInvalid)
	}
	input.Decision = map[string]string{"approve": "approved", "reject": "rejected"}[decision]
	item, err := s.repository.DecideApproval(ctx, orgID, policyID, userID, role, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyApprovalForbidden
	}
	return item, err
}

func (s *PolicyService) Publish(ctx context.Context, orgID, policyID, userID string) (*models.Policy, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(userID) {
		return nil, ErrPolicyInvalidID
	}
	policy, err := s.GetByID(ctx, orgID, policyID)
	if err != nil {
		return nil, err
	}
	if policy.Status != models.PolicyStateApproved {
		return nil, fmt.Errorf("%w: only an approved policy can be published", ErrPolicyInvalidTransition)
	}
	item, err := s.repository.Publish(ctx, orgID, policyID, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyInvalidTransition
	}
	return item, err
}

func (s *PolicyService) CreateReview(ctx context.Context, orgID, policyID string, input models.PolicyReviewInput) (*models.PolicyReview, error) {
	if !validUUID(orgID) || !validUUID(policyID) {
		return nil, ErrPolicyInvalidID
	}
	policy, err := s.GetByID(ctx, orgID, policyID)
	if err != nil {
		return nil, err
	}
	if policy.Status != models.PolicyStatePublished {
		return nil, fmt.Errorf("%w: reviews may be scheduled only for published policies", ErrPolicyInvalidTransition)
	}
	input.ReviewType = strings.TrimSpace(input.ReviewType)
	if !allowed(input.ReviewType, "scheduled", "triggered", "regulatory_change", "incident_driven", "ad_hoc") {
		return nil, fmt.Errorf("%w: unsupported review_type", ErrPolicyInvalid)
	}
	if input.ReviewerUserID != nil && !validUUID(*input.ReviewerUserID) {
		return nil, ErrPolicyInvalidID
	}
	if input.ReviewDate != nil && input.DueDate != nil && input.DueDate.Before(*input.ReviewDate) {
		return nil, fmt.Errorf("%w: due_date precedes review_date", ErrPolicyInvalid)
	}
	item, err := s.repository.CreateReview(ctx, orgID, policyID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyInvalidReference
	}
	return item, err
}

func (s *PolicyService) GetReview(ctx context.Context, orgID, policyID, reviewID string) (*models.PolicyReview, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(reviewID) {
		return nil, ErrPolicyInvalidID
	}
	item, err := s.repository.GetReview(ctx, orgID, policyID, reviewID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyReviewNotFound
	}
	return item, err
}

func (s *PolicyService) UpdateReview(ctx context.Context, orgID, policyID, reviewID string, patch models.PolicyReviewPatch) (*models.PolicyReview, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(reviewID) {
		return nil, ErrPolicyInvalidID
	}
	if reviewPatchEmpty(patch) {
		return nil, fmt.Errorf("%w: no review fields supplied", ErrPolicyInvalid)
	}
	review, err := s.repository.GetReview(ctx, orgID, policyID, reviewID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyReviewNotFound
	}
	if err != nil {
		return nil, err
	}
	previous := review.Status
	applyReviewPatch(review, patch)
	if !validReviewTransition(previous, review.Status) {
		return nil, fmt.Errorf("%w: review %s to %s", ErrPolicyInvalidTransition, previous, review.Status)
	}
	if review.Status == "completed" && (review.Outcome == nil || !allowed(*review.Outcome, "no_change", "minor_update", "major_revision", "retirement")) {
		return nil, fmt.Errorf("%w: completed review requires a valid outcome", ErrPolicyInvalid)
	}
	if review.NewVersionID != nil && !validUUID(*review.NewVersionID) {
		return nil, ErrPolicyInvalidID
	}
	item, err := s.repository.UpdateReview(ctx, orgID, policyID, review)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyInvalidReference
	}
	return item, err
}

func (s *PolicyService) ListReviews(ctx context.Context, orgID, policyID string, pagination models.PaginationRequest) ([]models.PolicyReview, int, error) {
	if _, err := s.GetByID(ctx, orgID, policyID); err != nil {
		return nil, 0, err
	}
	return s.repository.ListReviews(ctx, orgID, policyID, normalizePolicyPagination(pagination))
}

func (s *PolicyService) Acknowledge(ctx context.Context, orgID, policyID, userID, ip string, input models.PolicyAttestationInput) (*models.PolicyAttestation, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(userID) {
		return nil, ErrPolicyInvalidID
	}
	policy, err := s.GetByID(ctx, orgID, policyID)
	if err != nil {
		return nil, err
	}
	if policy.Status != models.PolicyStatePublished || !policy.RequiresAttestation {
		return nil, fmt.Errorf("%w: policy is not published for attestation", ErrPolicyInvalidTransition)
	}
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	if !allowed(input.Decision, "attest", "decline") {
		return nil, fmt.Errorf("%w: decision must be attest or decline", ErrPolicyInvalid)
	}
	if input.Decision == "decline" && (input.DeclinedReason == nil || strings.TrimSpace(*input.DeclinedReason) == "") {
		return nil, fmt.Errorf("%w: declined_reason is required", ErrPolicyInvalid)
	}
	if input.AttestationMethod == "" {
		input.AttestationMethod = "digital_click"
	}
	if !allowed(input.AttestationMethod, "digital_click", "digital_signature", "email_reply", "sso_confirmation") {
		return nil, fmt.Errorf("%w: unsupported attestation_method", ErrPolicyInvalid)
	}
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(input.Metadata) {
		return nil, fmt.Errorf("%w: malformed attestation metadata", ErrPolicyInvalid)
	}
	item, err := s.repository.Acknowledge(ctx, orgID, policyID, userID, ip, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyNotFound
	}
	return item, err
}

func (s *PolicyService) ListAttestations(ctx context.Context, orgID, policyID string, pagination models.PaginationRequest) ([]models.PolicyAttestation, int, error) {
	if _, err := s.GetByID(ctx, orgID, policyID); err != nil {
		return nil, 0, err
	}
	return s.repository.ListAttestations(ctx, orgID, policyID, normalizePolicyPagination(pagination))
}

func (s *PolicyService) CreateException(ctx context.Context, orgID, policyID, userID string, input models.PolicyExceptionInput) (*models.PolicyException, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(userID) {
		return nil, ErrPolicyInvalidID
	}
	policy, err := s.GetByID(ctx, orgID, policyID)
	if err != nil {
		return nil, err
	}
	if policy.Status != models.PolicyStatePublished {
		return nil, fmt.Errorf("%w: exceptions may be requested only for published policies", ErrPolicyInvalidTransition)
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Justification = strings.TrimSpace(input.Justification)
	if input.Title == "" || len(input.Title) > 500 || input.Justification == "" {
		return nil, fmt.Errorf("%w: title and justification are required", ErrPolicyInvalid)
	}
	if input.RiskLevel != nil && !allowed(*input.RiskLevel, "critical", "high", "medium", "low") {
		return nil, fmt.Errorf("%w: unsupported risk_level", ErrPolicyInvalid)
	}
	if input.LinkedRiskID != nil && !validUUID(*input.LinkedRiskID) {
		return nil, ErrPolicyInvalidID
	}
	if input.EffectiveDate != nil && input.ExpiryDate != nil && input.ExpiryDate.Before(*input.EffectiveDate) {
		return nil, fmt.Errorf("%w: expiry_date precedes effective_date", ErrPolicyInvalid)
	}
	item, err := s.repository.CreateException(ctx, orgID, policyID, userID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyInvalidReference
	}
	return item, mapPolicyWriteError(err)
}

func (s *PolicyService) GetException(ctx context.Context, orgID, policyID, exceptionID string) (*models.PolicyException, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(exceptionID) {
		return nil, ErrPolicyInvalidID
	}
	item, err := s.repository.GetException(ctx, orgID, policyID, exceptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyExceptionNotFound
	}
	return item, err
}

func (s *PolicyService) DecideException(ctx context.Context, orgID, policyID, exceptionID, userID string, input models.PolicyExceptionDecisionInput) (*models.PolicyException, error) {
	if !validUUID(orgID) || !validUUID(policyID) || !validUUID(exceptionID) || !validUUID(userID) {
		return nil, ErrPolicyInvalidID
	}
	item, err := s.repository.GetException(ctx, orgID, policyID, exceptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyExceptionNotFound
	}
	if err != nil {
		return nil, err
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	switch decision {
	case "revoke":
		decision = "revoked"
	case "approve":
		decision = "approved"
	case "reject":
		decision = "rejected"
	}
	if (item.Status == "requested" || item.Status == "under_review") && !allowed(decision, "approved", "rejected") {
		return nil, fmt.Errorf("%w: requested exception may only be approved or rejected", ErrPolicyInvalidTransition)
	}
	if item.Status == "approved" && decision != "revoked" {
		return nil, fmt.Errorf("%w: approved exception may only be revoked", ErrPolicyInvalidTransition)
	}
	if !allowed(item.Status, "requested", "under_review", "approved") {
		return nil, fmt.Errorf("%w: exception is terminal", ErrPolicyInvalidTransition)
	}
	if decision == "approved" && input.ExpiryDate == nil && item.ExpiryDate == nil {
		return nil, fmt.Errorf("%w: approved exception requires an expiry_date", ErrPolicyInvalid)
	}
	if decision == "approved" && input.ExpiryDate != nil && item.EffectiveDate != nil && input.ExpiryDate.Before(*item.EffectiveDate) {
		return nil, fmt.Errorf("%w: expiry_date precedes effective_date", ErrPolicyInvalid)
	}
	input.Decision = decision
	updated, err := s.repository.UpdateExceptionDecision(ctx, orgID, policyID, exceptionID, userID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPolicyExceptionNotFound
	}
	return updated, err
}

func (s *PolicyService) ListExceptions(ctx context.Context, orgID, policyID string, pagination models.PaginationRequest) ([]models.PolicyException, int, error) {
	if _, err := s.GetByID(ctx, orgID, policyID); err != nil {
		return nil, 0, err
	}
	return s.repository.ListExceptions(ctx, orgID, policyID, normalizePolicyPagination(pagination))
}

func normalizePolicyCreate(input *models.PolicyCreateInput) {
	input.PolicyRef = strings.TrimSpace(input.PolicyRef)
	input.Title = strings.TrimSpace(input.Title)
	input.Classification = strings.ToLower(strings.TrimSpace(input.Classification))
	if input.Classification == "" {
		input.Classification = "internal"
	}
	if input.ReviewFrequencyMonths == 0 {
		input.ReviewFrequencyMonths = 12
	}
	if input.AttestationFrequencyMonths == 0 {
		input.AttestationFrequencyMonths = 12
	}
	if input.AppliesToAll == nil {
		value := true
		input.AppliesToAll = &value
	}
	if input.IsMandatory == nil {
		value := true
		input.IsMandatory = &value
	}
	if input.RequiresAttestation == nil {
		value := true
		input.RequiresAttestation = &value
	}
	for _, ids := range []*[]string{&input.ApplicableDepartments, &input.LinkedFrameworkIDs, &input.LinkedControlIDs, &input.LinkedRiskIDs, &input.Tags} {
		if *ids == nil {
			*ids = []string{}
		}
	}
	for _, values := range []*[]string{&input.ApplicableRoles, &input.ApplicableLocations} {
		if *values == nil {
			*values = []string{}
		}
	}
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
	for _, id := range []**string{&input.CategoryID, &input.OwnerUserID, &input.ApproverUserID, &input.DepartmentID, &input.ParentPolicyID, &input.SupersedesPolicyID} {
		normalizeOptionalID(id)
	}
	normalizeVersion(&input.InitialVersion, input.Title)
}

func normalizeVersion(input *models.PolicyVersionInput, fallbackTitle string) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		input.Title = fallbackTitle
	}
	input.Language = strings.ToLower(strings.TrimSpace(input.Language))
	if input.Language == "" {
		input.Language = "en"
	}
	if input.ChangeType == nil {
		value := "major"
		input.ChangeType = &value
	}
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
}

func validatePolicyCreate(input models.PolicyCreateInput) error {
	if len(input.PolicyRef) > 30 || input.Title == "" || len(input.Title) > 500 {
		return fmt.Errorf("%w: title is required and policy_ref cannot exceed 30 characters", ErrPolicyInvalid)
	}
	if input.ReviewFrequencyMonths < 1 || input.ReviewFrequencyMonths > 60 || input.AttestationFrequencyMonths < 1 || input.AttestationFrequencyMonths > 60 {
		return fmt.Errorf("%w: review and attestation frequencies must be between 1 and 60 months", ErrPolicyInvalid)
	}
	if err := validatePolicyValues(input.Classification, input.Priority, input.Metadata); err != nil {
		return err
	}
	if err := validateVersion(input.InitialVersion); err != nil {
		return err
	}
	for _, id := range []*string{input.CategoryID, input.OwnerUserID, input.ApproverUserID, input.DepartmentID, input.ParentPolicyID, input.SupersedesPolicyID} {
		if id != nil && !validUUID(*id) {
			return ErrPolicyInvalidID
		}
	}
	for _, ids := range [][]string{input.ApplicableDepartments, input.LinkedFrameworkIDs, input.LinkedControlIDs, input.LinkedRiskIDs} {
		for _, id := range ids {
			if !validUUID(id) {
				return ErrPolicyInvalidID
			}
		}
	}
	return nil
}

func validatePolicy(item *models.Policy) error {
	if strings.TrimSpace(item.Title) == "" || len(item.Title) > 500 {
		return fmt.Errorf("%w: title is required and cannot exceed 500 characters", ErrPolicyInvalid)
	}
	if item.ReviewFrequencyMonths < 1 || item.ReviewFrequencyMonths > 60 || item.AttestationFrequencyMonths < 1 || item.AttestationFrequencyMonths > 60 {
		return fmt.Errorf("%w: invalid review or attestation frequency", ErrPolicyInvalid)
	}
	for _, id := range []*string{item.CategoryID, item.OwnerUserID, item.ApproverUserID, item.DepartmentID, item.ParentPolicyID, item.SupersedesPolicyID} {
		if id != nil && !validUUID(*id) {
			return ErrPolicyInvalidID
		}
	}
	if (item.ParentPolicyID != nil && *item.ParentPolicyID == item.ID) ||
		(item.SupersedesPolicyID != nil && *item.SupersedesPolicyID == item.ID) {
		return fmt.Errorf("%w: a policy cannot parent or supersede itself", ErrPolicyInvalid)
	}
	for _, ids := range [][]string{item.ApplicableDepartments, item.LinkedFrameworkIDs, item.LinkedControlIDs, item.LinkedRiskIDs} {
		for _, id := range ids {
			if !validUUID(id) {
				return ErrPolicyInvalidID
			}
		}
	}
	if item.EffectiveDate != nil && item.ExpiryDate != nil && item.ExpiryDate.Before(*item.EffectiveDate) {
		return fmt.Errorf("%w: expiry_date precedes effective_date", ErrPolicyInvalid)
	}
	return validatePolicyValues(item.Classification, item.Priority, item.Metadata)
}

func validatePolicyValues(classification string, priority *string, metadata json.RawMessage) error {
	if !allowed(classification, "public", "internal", "confidential", "restricted") {
		return fmt.Errorf("%w: unsupported classification", ErrPolicyInvalid)
	}
	if priority != nil && !allowed(*priority, "critical", "high", "medium", "low") {
		return fmt.Errorf("%w: unsupported priority", ErrPolicyInvalid)
	}
	if !json.Valid(metadata) {
		return fmt.Errorf("%w: malformed metadata", ErrPolicyInvalid)
	}
	return nil
}

func validateVersion(input models.PolicyVersionInput) error {
	if input.Title == "" || len(input.Title) > 500 {
		return fmt.Errorf("%w: version title is required", ErrPolicyInvalid)
	}
	hasHTML := input.ContentHTML != nil && strings.TrimSpace(*input.ContentHTML) != ""
	hasText := input.ContentText != nil && strings.TrimSpace(*input.ContentText) != ""
	hasFile := input.FilePath != nil && strings.TrimSpace(*input.FilePath) != ""
	if !hasHTML && !hasText && !hasFile {
		return fmt.Errorf("%w: version content or file_path is required", ErrPolicyInvalid)
	}
	if input.ChangeType != nil && !allowed(*input.ChangeType, "major", "minor", "editorial") {
		return fmt.Errorf("%w: unsupported change_type", ErrPolicyInvalid)
	}
	if len(input.Language) < 2 || len(input.Language) > 10 {
		return fmt.Errorf("%w: language must be a BCP-47-like code", ErrPolicyInvalid)
	}
	if !json.Valid(input.Metadata) {
		return fmt.Errorf("%w: malformed version metadata", ErrPolicyInvalid)
	}
	return nil
}

func normalizeSubmit(input *models.PolicySubmitInput) {
	input.WorkflowType = strings.ToLower(strings.TrimSpace(input.WorkflowType))
	if input.WorkflowType == "" {
		input.WorkflowType = "new_policy"
	}
	for i := range input.Approvers {
		if input.Approvers[i].ApproverUserID != nil {
			value := strings.TrimSpace(*input.Approvers[i].ApproverUserID)
			input.Approvers[i].ApproverUserID = nullableID(value)
		}
		if input.Approvers[i].ApproverRole != nil {
			value := strings.TrimSpace(*input.Approvers[i].ApproverRole)
			input.Approvers[i].ApproverRole = nullableString(value)
		}
	}
}

func validateSubmit(input models.PolicySubmitInput) error {
	if !allowed(input.WorkflowType, "new_policy", "review", "amendment", "retirement") {
		return fmt.Errorf("%w: unsupported workflow_type", ErrPolicyInvalid)
	}
	if len(input.Approvers) == 0 || len(input.Approvers) > 20 {
		return fmt.Errorf("%w: between 1 and 20 approval steps are required", ErrPolicyInvalid)
	}
	for index, step := range input.Approvers {
		if (step.ApproverUserID == nil) == (step.ApproverRole == nil) {
			return fmt.Errorf("%w: approval step %d requires exactly one user or role", ErrPolicyInvalid, index+1)
		}
		if step.ApproverUserID != nil && !validUUID(*step.ApproverUserID) {
			return ErrPolicyInvalidID
		}
		if step.ApproverRole != nil && len(*step.ApproverRole) > 50 {
			return fmt.Errorf("%w: approver role exceeds 50 characters", ErrPolicyInvalid)
		}
	}
	return nil
}

func applyPolicyPatch(item *models.Policy, patch models.PolicyPatch) {
	if patch.Title != nil {
		item.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.CategoryID != nil {
		item.CategoryID = nullableID(strings.TrimSpace(*patch.CategoryID))
	}
	if patch.Classification != nil {
		item.Classification = strings.ToLower(strings.TrimSpace(*patch.Classification))
	}
	if patch.OwnerUserID != nil {
		item.OwnerUserID = nullableID(strings.TrimSpace(*patch.OwnerUserID))
	}
	if patch.ApproverUserID != nil {
		item.ApproverUserID = nullableID(strings.TrimSpace(*patch.ApproverUserID))
	}
	if patch.DepartmentID != nil {
		item.DepartmentID = nullableID(strings.TrimSpace(*patch.DepartmentID))
	}
	if patch.ReviewFrequencyMonths != nil {
		item.ReviewFrequencyMonths = *patch.ReviewFrequencyMonths
	}
	if patch.AppliesToAll != nil {
		item.AppliesToAll = *patch.AppliesToAll
	}
	if patch.ApplicableDepartments != nil {
		item.ApplicableDepartments = patch.ApplicableDepartments
	}
	if patch.ApplicableRoles != nil {
		item.ApplicableRoles = patch.ApplicableRoles
	}
	if patch.ApplicableLocations != nil {
		item.ApplicableLocations = patch.ApplicableLocations
	}
	if patch.LinkedFrameworkIDs != nil {
		item.LinkedFrameworkIDs = patch.LinkedFrameworkIDs
	}
	if patch.LinkedControlIDs != nil {
		item.LinkedControlIDs = patch.LinkedControlIDs
	}
	if patch.LinkedRiskIDs != nil {
		item.LinkedRiskIDs = patch.LinkedRiskIDs
	}
	if patch.ParentPolicyID != nil {
		item.ParentPolicyID = nullableID(strings.TrimSpace(*patch.ParentPolicyID))
	}
	if patch.SupersedesPolicyID != nil {
		item.SupersedesPolicyID = nullableID(strings.TrimSpace(*patch.SupersedesPolicyID))
	}
	if patch.ExpiryDate != nil {
		item.ExpiryDate = patch.ExpiryDate
	}
	if patch.Tags != nil {
		item.Tags = patch.Tags
	}
	if patch.Priority != nil {
		item.Priority = nullableString(strings.ToLower(strings.TrimSpace(*patch.Priority)))
	}
	if patch.IsMandatory != nil {
		item.IsMandatory = *patch.IsMandatory
	}
	if patch.RequiresAttestation != nil {
		item.RequiresAttestation = *patch.RequiresAttestation
	}
	if patch.AttestationFrequencyMonths != nil {
		item.AttestationFrequencyMonths = *patch.AttestationFrequencyMonths
	}
	if patch.Metadata != nil {
		item.Metadata = patch.Metadata
	}
}

func policyPatchEmpty(p models.PolicyPatch) bool {
	return p.Title == nil && p.CategoryID == nil && p.Classification == nil && p.OwnerUserID == nil &&
		p.ApproverUserID == nil && p.DepartmentID == nil && p.ReviewFrequencyMonths == nil &&
		p.AppliesToAll == nil && p.ApplicableDepartments == nil && p.ApplicableRoles == nil &&
		p.ApplicableLocations == nil && p.LinkedFrameworkIDs == nil && p.LinkedControlIDs == nil &&
		p.LinkedRiskIDs == nil && p.ParentPolicyID == nil && p.SupersedesPolicyID == nil &&
		p.ExpiryDate == nil && p.Tags == nil && p.Priority == nil && p.IsMandatory == nil &&
		p.RequiresAttestation == nil && p.AttestationFrequencyMonths == nil && p.Metadata == nil
}

func reviewPatchEmpty(p models.PolicyReviewPatch) bool {
	return p.Status == nil && p.Outcome == nil && p.Findings == nil && p.Recommendations == nil && p.NewVersionID == nil
}

func applyReviewPatch(review *models.PolicyReview, patch models.PolicyReviewPatch) {
	if patch.Status != nil {
		review.Status = strings.ToLower(strings.TrimSpace(*patch.Status))
	}
	if patch.Outcome != nil {
		review.Outcome = nullableString(strings.ToLower(strings.TrimSpace(*patch.Outcome)))
	}
	if patch.Findings != nil {
		review.Findings = nullableString(strings.TrimSpace(*patch.Findings))
	}
	if patch.Recommendations != nil {
		review.Recommendations = nullableString(strings.TrimSpace(*patch.Recommendations))
	}
	if patch.NewVersionID != nil {
		review.NewVersionID = nullableID(strings.TrimSpace(*patch.NewVersionID))
	}
}

func validReviewTransition(from, to string) bool {
	if from == to {
		return true
	}
	transitions := map[string][]string{
		"scheduled":   {"in_progress", "overdue", "cancelled"},
		"in_progress": {"completed", "overdue", "cancelled"},
		"overdue":     {"in_progress", "completed", "cancelled"},
	}
	return allowed(to, transitions[from]...)
}

func policyStatuses() []string {
	return []string{models.PolicyStateDraft, models.PolicyStateUnderReview, models.PolicyStatePendingApproval,
		models.PolicyStateApproved, models.PolicyStatePublished, models.PolicyStateArchived,
		models.PolicyStateRetired, models.PolicyStateSuperseded}
}

func policyWordCount(input models.PolicyVersionInput) int {
	if input.ContentText != nil {
		return len(strings.Fields(*input.ContentText))
	}
	if input.ContentHTML != nil {
		return len(strings.Fields(*input.ContentHTML))
	}
	return 0
}

func normalizePolicyPagination(p models.PaginationRequest) models.PaginationRequest {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return p
}

func mapPolicyWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrPolicyConflict, pgError.ConstraintName)
	}
	return err
}
