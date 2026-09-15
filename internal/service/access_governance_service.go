package service

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrGovernanceInvalid    = errors.New("invalid access governance request")
	ErrGovernanceNotFound   = errors.New("access governance resource not found")
	ErrGovernanceConflict   = errors.New("access governance snapshot or version is stale")
	ErrGovernanceSeparation = errors.New("independent reviewer required")
)

type AccessGovernanceService struct {
	store repository.AccessGovernanceRepository
	now   func() time.Time
}

func NewAccessGovernanceService(store repository.AccessGovernanceRepository, now func() time.Time) (*AccessGovernanceService, error) {
	if store == nil {
		return nil, errors.New("access governance store is required")
	}
	if now == nil {
		now = time.Now
	}
	return &AccessGovernanceService{store: store, now: now}, nil
}
func governanceIDs(ids ...string) bool {
	for _, id := range ids {
		if parsed, err := uuid.Parse(id); err != nil || parsed == uuid.Nil || parsed.String() != id {
			return false
		}
	}
	return true
}
func governanceReason(reason string) bool { return len(reason) >= 3 && len(reason) <= 1000 }
func governancePagination(p models.PaginationRequest) models.PaginationRequest {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	if p.Page > 100000 {
		p.Page = 100000
	}
	return p
}
func governanceError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrGovernanceNotFound
	case errors.Is(err, repository.ErrAccessGovernanceConflict):
		return ErrGovernanceConflict
	case errors.Is(err, repository.ErrAccessGovernanceSeparation):
		return ErrGovernanceSeparation
	case errors.Is(err, repository.ErrLastTenantAdministrator):
		return ErrLastTenantAdministrator
	case errors.Is(err, repository.ErrAccessGovernanceInvalid), errors.Is(err, repository.ErrAccessGovernanceBound):
		return ErrGovernanceInvalid
	}
	return err
}
func (s *AccessGovernanceService) CreateCampaign(ctx context.Context, org, actor string, in models.AccessReviewCampaignInput) (*models.AccessReviewCampaign, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Reason = strings.TrimSpace(in.Reason)
	if !governanceIDs(org, actor, in.ReviewerID) || in.ReviewerID == actor || len(in.Name) < 3 || len(in.Name) > 200 || !governanceReason(in.Reason) || len(in.SubjectIDs) < 1 || len(in.SubjectIDs) > 200 || !in.DueAt.After(s.now()) || in.DueAt.After(s.now().Add(90*24*time.Hour)) {
		return nil, ErrGovernanceInvalid
	}
	seen := map[string]bool{}
	for _, id := range in.SubjectIDs {
		if !governanceIDs(id) || seen[id] {
			return nil, ErrGovernanceInvalid
		}
		if id == in.ReviewerID {
			return nil, ErrGovernanceSeparation
		}
		seen[id] = true
	}
	sort.Strings(in.SubjectIDs)
	v, err := s.store.CreateCampaign(ctx, org, actor, in)
	return v, governanceError(err)
}
func (s *AccessGovernanceService) GetCampaign(ctx context.Context, org, id string) (*models.AccessReviewCampaign, error) {
	if !governanceIDs(org, id) {
		return nil, ErrGovernanceInvalid
	}
	v, err := s.store.GetCampaign(ctx, org, id)
	return v, governanceError(err)
}
func (s *AccessGovernanceService) ListCampaigns(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessReviewCampaign, int, error) {
	if !governanceIDs(org) {
		return nil, 0, ErrGovernanceInvalid
	}
	v, n, err := s.store.ListCampaigns(ctx, org, governancePagination(p))
	return v, n, governanceError(err)
}
func (s *AccessGovernanceService) ListItems(ctx context.Context, org, id string, p models.PaginationRequest) ([]models.AccessReviewItem, int, error) {
	if !governanceIDs(org, id) {
		return nil, 0, ErrGovernanceInvalid
	}
	v, n, err := s.store.ListItems(ctx, org, id, governancePagination(p))
	return v, n, governanceError(err)
}

var governanceSHA = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *AccessGovernanceService) DecideItem(ctx context.Context, org, campaign, item, actor string, in models.AccessReviewDecisionInput) (*models.AccessReviewDecision, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	in.RequestID = strings.TrimSpace(in.RequestID)
	if !governanceIDs(org, campaign, item, actor) || !governanceReason(in.Reason) || len(in.RequestID) < 1 || len(in.RequestID) > 64 || !governanceSHA.MatchString(in.SnapshotSHA256) || (in.Decision != "retain" && in.Decision != "revoke") {
		return nil, ErrGovernanceInvalid
	}
	v, err := s.store.DecideItem(ctx, org, campaign, item, actor, in)
	return v, governanceError(err)
}
func (s *AccessGovernanceService) TransitionCampaign(ctx context.Context, org, id, actor, status string, in models.AccessGovernanceTransitionInput) (*models.AccessReviewCampaign, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !governanceIDs(org, id, actor) || in.ExpectedVersion < 1 || !governanceReason(in.Reason) || (status != "completed" && status != "cancelled") {
		return nil, ErrGovernanceInvalid
	}
	v, err := s.store.TransitionCampaign(ctx, org, id, actor, status, in)
	return v, governanceError(err)
}
func (s *AccessGovernanceService) CreateSoDRule(ctx context.Context, org, actor string, in models.AccessSoDRuleInput) (*models.AccessSoDRule, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Reason = strings.TrimSpace(in.Reason)
	if !governanceIDs(org, actor, in.RoleAID, in.RoleBID) || in.RoleAID == in.RoleBID || len(in.Name) < 3 || len(in.Name) > 200 || !governanceReason(in.Reason) {
		return nil, ErrGovernanceInvalid
	}
	if in.RoleAID > in.RoleBID {
		in.RoleAID, in.RoleBID = in.RoleBID, in.RoleAID
	}
	v, err := s.store.CreateSoDRule(ctx, org, actor, in)
	return v, governanceError(err)
}
func (s *AccessGovernanceService) ListSoDRules(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessSoDRule, int, error) {
	if !governanceIDs(org) {
		return nil, 0, ErrGovernanceInvalid
	}
	v, n, err := s.store.ListSoDRules(ctx, org, governancePagination(p))
	return v, n, governanceError(err)
}
func (s *AccessGovernanceService) DisableSoDRule(ctx context.Context, org, id, actor string, in models.AccessGovernanceTransitionInput) (*models.AccessSoDRule, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !governanceIDs(org, id, actor) || in.ExpectedVersion < 1 || !governanceReason(in.Reason) {
		return nil, ErrGovernanceInvalid
	}
	v, err := s.store.DisableSoDRule(ctx, org, id, actor, in)
	return v, governanceError(err)
}
func governanceWindow(now, start, end time.Time) bool {
	return !start.Before(now.Add(-5*time.Minute)) && start.Before(now.Add(90*24*time.Hour)) && end.After(start) && end.After(now) && !end.After(start.Add(90*24*time.Hour))
}
func (s *AccessGovernanceService) RequestException(ctx context.Context, org, rule, actor string, in models.AccessSoDExceptionInput) (*models.AccessSoDException, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !governanceIDs(org, rule, actor, in.SubjectID) || !governanceReason(in.Reason) || !governanceWindow(s.now(), in.ValidFrom, in.ExpiresAt) {
		return nil, ErrGovernanceInvalid
	}
	v, err := s.store.RequestException(ctx, org, rule, actor, in)
	return v, governanceError(err)
}
func (s *AccessGovernanceService) DecideException(ctx context.Context, org, id, actor, status string, in models.AccessGovernanceTransitionInput) (*models.AccessSoDException, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !governanceIDs(org, id, actor) || in.ExpectedVersion < 1 || !governanceReason(in.Reason) || (status != "approved" && status != "rejected" && status != "revoked") {
		return nil, ErrGovernanceInvalid
	}
	v, err := s.store.DecideException(ctx, org, id, actor, status, in)
	return v, governanceError(err)
}
func (s *AccessGovernanceService) ListExceptions(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessSoDException, int, error) {
	if !governanceIDs(org) {
		return nil, 0, ErrGovernanceInvalid
	}
	v, n, err := s.store.ListExceptions(ctx, org, governancePagination(p))
	return v, n, governanceError(err)
}
func (s *AccessGovernanceService) ListViolations(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessSoDViolation, int, error) {
	if !governanceIDs(org) {
		return nil, 0, ErrGovernanceInvalid
	}
	v, n, err := s.store.ListViolations(ctx, org, governancePagination(p))
	return v, n, governanceError(err)
}
func (s *AccessGovernanceService) ListEvents(ctx context.Context, org string, p models.PaginationRequest) ([]models.AccessGovernanceEvent, int, error) {
	if !governanceIDs(org) {
		return nil, 0, ErrGovernanceInvalid
	}
	v, n, err := s.store.ListEvents(ctx, org, governancePagination(p))
	return v, n, governanceError(err)
}
func (s *AccessGovernanceService) SetAssignmentWindow(ctx context.Context, org, role, user, actor string, in models.ManagedRoleWindowInput) (*models.ManagedRoleAssignment, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !governanceIDs(org, role, user, actor, in.AssignmentID) || !governanceReason(in.Reason) || in.ExpectedVersion < 1 || !governanceWindow(s.now(), in.ValidFrom, in.ExpiresAt) {
		return nil, ErrGovernanceInvalid
	}
	if user == actor {
		return nil, ErrGovernanceSeparation
	}
	v, err := s.store.SetAssignmentWindow(ctx, org, role, user, actor, in)
	return v, governanceError(err)
}
