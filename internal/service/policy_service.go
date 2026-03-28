package service

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrPolicyNotFound       = errors.New("policy not found")
	ErrPolicyInvalidTransition = errors.New("invalid policy status transition")
)

// PolicyRepository defines the data access interface for policies.
type PolicyRepository interface {
	Create(ctx context.Context, policy *models.Policy) error
	GetByID(ctx context.Context, id string) (*models.Policy, error)
	Update(ctx context.Context, policy *models.Policy) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, orgID string, page, pageSize int) ([]models.Policy, int, error)
	ListByStatus(ctx context.Context, orgID string, status models.PolicyStatus) ([]models.Policy, error)
	ListDueForReview(ctx context.Context, orgID string, before time.Time) ([]models.Policy, error)
}

// PolicyService handles business logic for policy lifecycle management.
type PolicyService struct {
	policyRepo PolicyRepository
	logger     zerolog.Logger
}

// NewPolicyService constructs a new PolicyService.
func NewPolicyService(policyRepo PolicyRepository, logger zerolog.Logger) *PolicyService {
	return &PolicyService{
		policyRepo: policyRepo,
		logger:     logger.With().Str("service", "policy").Logger(),
	}
}

// Create persists a new policy in draft status.
func (s *PolicyService) Create(ctx context.Context, policy *models.Policy) error {
	policy.Status = models.PolicyStatusDraft

	if policy.Version == "" {
		policy.Version = "1.0"
	}

	if err := s.policyRepo.Create(ctx, policy); err != nil {
		s.logger.Error().Err(err).Str("title", policy.Title).Msg("failed to create policy")
		return err
	}

	s.logger.Info().Str("policy_id", policy.ID).Str("title", policy.Title).Msg("policy created")
	return nil
}

// GetByID retrieves a policy by its unique identifier.
func (s *PolicyService) GetByID(ctx context.Context, id string) (*models.Policy, error) {
	policy, err := s.policyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrPolicyNotFound
	}
	return policy, nil
}

// Update modifies an existing policy. Only draft or under-review policies can be edited.
func (s *PolicyService) Update(ctx context.Context, policy *models.Policy) error {
	existing, err := s.policyRepo.GetByID(ctx, policy.ID)
	if err != nil {
		return ErrPolicyNotFound
	}

	if existing.Status != models.PolicyStatusDraft && existing.Status != models.PolicyStatusUnderReview {
		return errors.New("only draft or under-review policies can be edited")
	}

	if err := s.policyRepo.Update(ctx, policy); err != nil {
		s.logger.Error().Err(err).Str("policy_id", policy.ID).Msg("failed to update policy")
		return err
	}

	s.logger.Info().Str("policy_id", policy.ID).Msg("policy updated")
	return nil
}

// Delete soft-deletes a policy by ID.
func (s *PolicyService) Delete(ctx context.Context, id string) error {
	if _, err := s.policyRepo.GetByID(ctx, id); err != nil {
		return ErrPolicyNotFound
	}

	if err := s.policyRepo.Delete(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("policy_id", id).Msg("failed to delete policy")
		return err
	}

	s.logger.Info().Str("policy_id", id).Msg("policy deleted")
	return nil
}

// List returns a paginated list of policies for an organization.
func (s *PolicyService) List(ctx context.Context, orgID string, page, pageSize int) ([]models.Policy, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	policies, total, err := s.policyRepo.List(ctx, orgID, page, pageSize)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to list policies")
		return nil, 0, err
	}
	return policies, total, nil
}

// SubmitForReview transitions a policy from Draft to UnderReview status.
func (s *PolicyService) SubmitForReview(ctx context.Context, id string) error {
	policy, err := s.policyRepo.GetByID(ctx, id)
	if err != nil {
		return ErrPolicyNotFound
	}

	if policy.Status != models.PolicyStatusDraft {
		return ErrPolicyInvalidTransition
	}

	policy.Status = models.PolicyStatusUnderReview
	if err := s.policyRepo.Update(ctx, policy); err != nil {
		s.logger.Error().Err(err).Str("policy_id", id).Msg("failed to submit policy for review")
		return err
	}

	s.logger.Info().Str("policy_id", id).Msg("policy submitted for review")
	return nil
}

// Approve transitions a policy from UnderReview to Approved status.
func (s *PolicyService) Approve(ctx context.Context, id, approverID string) error {
	policy, err := s.policyRepo.GetByID(ctx, id)
	if err != nil {
		return ErrPolicyNotFound
	}

	if policy.Status != models.PolicyStatusUnderReview {
		return ErrPolicyInvalidTransition
	}

	now := time.Now()
	policy.Status = models.PolicyStatusApproved
	policy.ApproverID = &approverID
	policy.ApprovedAt = &now

	// Set review date to one year from approval if not already set.
	if policy.ReviewDate == nil {
		reviewDate := now.AddDate(1, 0, 0)
		policy.ReviewDate = &reviewDate
	}

	if err := s.policyRepo.Update(ctx, policy); err != nil {
		s.logger.Error().Err(err).Str("policy_id", id).Msg("failed to approve policy")
		return err
	}

	s.logger.Info().Str("policy_id", id).Str("approver_id", approverID).Msg("policy approved")
	return nil
}

// Publish transitions a policy from Approved to Published status and sets the effective date.
func (s *PolicyService) Publish(ctx context.Context, id string) error {
	policy, err := s.policyRepo.GetByID(ctx, id)
	if err != nil {
		return ErrPolicyNotFound
	}

	if policy.Status != models.PolicyStatusApproved {
		return ErrPolicyInvalidTransition
	}

	now := time.Now()
	policy.Status = models.PolicyStatusPublished
	policy.EffectiveDate = &now

	if err := s.policyRepo.Update(ctx, policy); err != nil {
		s.logger.Error().Err(err).Str("policy_id", id).Msg("failed to publish policy")
		return err
	}

	s.logger.Info().Str("policy_id", id).Msg("policy published")
	return nil
}

// GetPoliciesDueForReview returns policies whose review date is on or before the given date.
func (s *PolicyService) GetPoliciesDueForReview(ctx context.Context, orgID string) ([]models.Policy, error) {
	cutoff := time.Now().AddDate(0, 1, 0) // Due within the next 30 days.
	policies, err := s.policyRepo.ListDueForReview(ctx, orgID, cutoff)
	if err != nil {
		s.logger.Error().Err(err).Str("org_id", orgID).Msg("failed to get policies due for review")
		return nil, err
	}

	s.logger.Info().Str("org_id", orgID).Int("count", len(policies)).Msg("policies due for review retrieved")
	return policies, nil
}
