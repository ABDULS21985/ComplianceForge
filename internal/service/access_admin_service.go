package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrManagedRoleNotFound     = errors.New("managed role not found")
	ErrManagedRoleInvalid      = errors.New("managed role request is invalid")
	ErrManagedRoleConflict     = errors.New("managed role conflicts with current state")
	ErrManagedRoleImmutable    = errors.New("system role is immutable")
	ErrManagedRoleInUse        = errors.New("managed role is assigned to users")
	ErrManagedRolePermission   = errors.New("managed role permission is invalid")
	ErrRoleAssignmentInvalid   = errors.New("managed role assignment is invalid")
	ErrLastTenantAdministrator = errors.New("the tenant must retain at least one administrator")
)

type AccessAdministrationStore interface {
	ListPermissions(context.Context) ([]models.PermissionGrant, error)
	CreateRole(context.Context, string, string, models.ManagedRoleCreateInput) (*models.ManagedRole, error)
	GetRole(context.Context, string, string) (*models.ManagedRole, error)
	ListRoles(context.Context, string, models.ManagedRoleListFilter) ([]models.ManagedRole, int, error)
	UpdateRole(context.Context, string, string, string, models.ManagedRolePatch) (*models.ManagedRole, error)
	DeleteRole(context.Context, string, string, string, int64) error
	CloneRole(context.Context, string, string, string, models.ManagedRoleCloneInput) (*models.ManagedRole, error)
	PreviewImpact(context.Context, string, string, []models.PermissionGrant) (*models.ManagedRoleImpact, error)
	ListAssignments(context.Context, string, string) ([]models.ManagedRoleAssignment, error)
	AssignRole(context.Context, string, string, string, models.ManagedRoleAssignmentInput) error
	UnassignRole(context.Context, string, string, string, string, models.ManagedRoleUnassignmentInput) error
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.RoleChangeEvent, int, error)
}

type AccessAdministrationService struct {
	store  AccessAdministrationStore
	logger zerolog.Logger
}

func NewAccessAdministrationService(store AccessAdministrationStore, logger zerolog.Logger) *AccessAdministrationService {
	return &AccessAdministrationService{store: store, logger: logger.With().Str("service", "access_administration").Logger()}
}

func (s *AccessAdministrationService) ListPermissions(ctx context.Context, organizationID string) ([]models.PermissionGrant, error) {
	if err := validateManagedRoleOrganization(organizationID); err != nil {
		return nil, err
	}
	items, err := s.store.ListPermissions(ctx)
	if err != nil {
		return nil, mapManagedRoleError(err)
	}
	return items, nil
}

func (s *AccessAdministrationService) CreateRole(ctx context.Context, organizationID, actorID string, input models.ManagedRoleCreateInput) (*models.ManagedRole, error) {
	if err := validateManagedRoleIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	normalizeManagedRoleCreate(&input)
	if err := validateManagedRoleCreate(input); err != nil {
		return nil, err
	}
	role, err := s.store.CreateRole(ctx, organizationID, actorID, input)
	if err != nil {
		return nil, mapManagedRoleError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("role_id", role.ID).Msg("custom role created")
	return role, nil
}

func (s *AccessAdministrationService) GetRole(ctx context.Context, organizationID, roleID string) (*models.ManagedRole, error) {
	if err := validateManagedRoleObject(organizationID, roleID); err != nil {
		return nil, err
	}
	role, err := s.store.GetRole(ctx, organizationID, roleID)
	if err != nil {
		return nil, mapManagedRoleError(err)
	}
	return role, nil
}

func (s *AccessAdministrationService) ListRoles(ctx context.Context, organizationID string, filter models.ManagedRoleListFilter) ([]models.ManagedRole, int, error) {
	if err := validateManagedRoleOrganization(organizationID); err != nil {
		return nil, 0, err
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if utf8.RuneCountInString(filter.Search) > 200 {
		return nil, 0, fmt.Errorf("%w: search must not exceed 200 characters", ErrManagedRoleInvalid)
	}
	filter.PaginationRequest = normalizeManagedRolePagination(filter.PaginationRequest)
	items, total, err := s.store.ListRoles(ctx, organizationID, filter)
	if err != nil {
		return nil, 0, mapManagedRoleError(err)
	}
	return items, total, nil
}

func (s *AccessAdministrationService) UpdateRole(ctx context.Context, organizationID, roleID, actorID string, patch models.ManagedRolePatch) (*models.ManagedRole, error) {
	if err := validateManagedRoleIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(roleID); err != nil {
		return nil, fmt.Errorf("%w: role id must be a UUID", ErrManagedRoleInvalid)
	}
	normalizeManagedRolePatch(&patch)
	if err := validateManagedRolePatch(patch); err != nil {
		return nil, err
	}
	role, err := s.store.UpdateRole(ctx, organizationID, roleID, actorID, patch)
	if err != nil {
		return nil, mapManagedRoleError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("role_id", roleID).Int64("version", role.Version).Msg("custom role updated")
	return role, nil
}

func (s *AccessAdministrationService) DeleteRole(ctx context.Context, organizationID, roleID, actorID string, expectedVersion int64) error {
	if err := validateManagedRoleIdentity(organizationID, actorID); err != nil {
		return err
	}
	if _, err := uuid.Parse(roleID); err != nil || expectedVersion < 1 {
		return fmt.Errorf("%w: role id and positive expected_version are required", ErrManagedRoleInvalid)
	}
	if err := s.store.DeleteRole(ctx, organizationID, roleID, actorID, expectedVersion); err != nil {
		return mapManagedRoleError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("role_id", roleID).Msg("custom role deleted")
	return nil
}

func (s *AccessAdministrationService) CloneRole(ctx context.Context, organizationID, sourceRoleID, actorID string, input models.ManagedRoleCloneInput) (*models.ManagedRole, error) {
	if err := validateManagedRoleIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(sourceRoleID); err != nil {
		return nil, fmt.Errorf("%w: source role id must be a UUID", ErrManagedRoleInvalid)
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = normalizeManagedRoleSlug(input.Slug, input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if err := validateManagedRoleText(input.Name, input.Slug, input.Description); err != nil {
		return nil, err
	}
	role, err := s.store.CloneRole(ctx, organizationID, sourceRoleID, actorID, input)
	if err != nil {
		return nil, mapManagedRoleError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("source_role_id", sourceRoleID).Str("role_id", role.ID).Msg("role cloned")
	return role, nil
}

func (s *AccessAdministrationService) PreviewImpact(ctx context.Context, organizationID, roleID string, permissions []models.PermissionGrant) (*models.ManagedRoleImpact, error) {
	if err := validateManagedRoleObject(organizationID, roleID); err != nil {
		return nil, err
	}
	permissions = normalizeManagedPermissions(permissions)
	if err := validateManagedPermissions(permissions); err != nil {
		return nil, err
	}
	impact, err := s.store.PreviewImpact(ctx, organizationID, roleID, permissions)
	if err != nil {
		return nil, mapManagedRoleError(err)
	}
	return impact, nil
}

func (s *AccessAdministrationService) ListAssignments(ctx context.Context, organizationID, roleID string) ([]models.ManagedRoleAssignment, error) {
	if err := validateManagedRoleObject(organizationID, roleID); err != nil {
		return nil, err
	}
	items, err := s.store.ListAssignments(ctx, organizationID, roleID)
	if err != nil {
		return nil, mapManagedRoleError(err)
	}
	return items, nil
}

func (s *AccessAdministrationService) AssignRole(ctx context.Context, organizationID, roleID, actorID string, input models.ManagedRoleAssignmentInput) error {
	if err := validateManagedRoleIdentity(organizationID, actorID); err != nil {
		return err
	}
	if _, err := uuid.Parse(roleID); err != nil {
		return fmt.Errorf("%w: role id must be a UUID", ErrManagedRoleInvalid)
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.Reason = strings.TrimSpace(input.Reason)
	if _, err := uuid.Parse(input.UserID); err != nil || utf8.RuneCountInString(input.Reason) < 3 || utf8.RuneCountInString(input.Reason) > 1000 {
		return fmt.Errorf("%w: active user and a 3-1000 character reason are required", ErrRoleAssignmentInvalid)
	}
	if input.ExpiresAt != nil {
		start := time.Now().UTC()
		if input.ValidFrom != nil {
			start = *input.ValidFrom
		}
		if actorID == input.UserID || !governanceWindow(time.Now().UTC(), start, *input.ExpiresAt) {
			return ErrRoleAssignmentInvalid
		}
		input.ValidFrom = &start
	} else if input.ValidFrom != nil {
		return ErrRoleAssignmentInvalid
	}
	if err := s.store.AssignRole(ctx, organizationID, roleID, actorID, input); err != nil {
		return mapManagedRoleError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("role_id", roleID).Str("target_user_id", input.UserID).Msg("role assigned")
	return nil
}

func (s *AccessAdministrationService) UnassignRole(ctx context.Context, organizationID, roleID, userID, actorID string, input models.ManagedRoleUnassignmentInput) error {
	if err := validateManagedRoleIdentity(organizationID, actorID); err != nil {
		return err
	}
	if _, err := uuid.Parse(roleID); err != nil {
		return fmt.Errorf("%w: role id must be a UUID", ErrManagedRoleInvalid)
	}
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("%w: user id must be a UUID", ErrRoleAssignmentInvalid)
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if utf8.RuneCountInString(input.Reason) < 3 || utf8.RuneCountInString(input.Reason) > 1000 {
		return fmt.Errorf("%w: a 3-1000 character reason is required", ErrRoleAssignmentInvalid)
	}
	if err := s.store.UnassignRole(ctx, organizationID, roleID, userID, actorID, input); err != nil {
		return mapManagedRoleError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("role_id", roleID).Str("target_user_id", userID).Msg("role unassigned")
	return nil
}

func (s *AccessAdministrationService) ListEvents(ctx context.Context, organizationID, roleID string, pagination models.PaginationRequest) ([]models.RoleChangeEvent, int, error) {
	if err := validateManagedRoleObject(organizationID, roleID); err != nil {
		return nil, 0, err
	}
	pagination = normalizeManagedRolePagination(pagination)
	items, total, err := s.store.ListEvents(ctx, organizationID, roleID, pagination)
	if err != nil {
		return nil, 0, mapManagedRoleError(err)
	}
	return items, total, nil
}

func normalizeManagedRoleCreate(input *models.ManagedRoleCreateInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = normalizeManagedRoleSlug(input.Slug, input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Permissions = normalizeManagedPermissions(input.Permissions)
}

func normalizeManagedRolePatch(patch *models.ManagedRolePatch) {
	if patch.Name != nil {
		value := strings.TrimSpace(*patch.Name)
		patch.Name = &value
	}
	if patch.Slug != nil {
		value := normalizeManagedRoleSlug(*patch.Slug, "")
		patch.Slug = &value
	}
	if patch.Description != nil {
		value := strings.TrimSpace(*patch.Description)
		patch.Description = &value
	}
	if patch.Permissions != nil {
		values := normalizeManagedPermissions(*patch.Permissions)
		patch.Permissions = &values
	}
}

func normalizeManagedPermissions(values []models.PermissionGrant) []models.PermissionGrant {
	result := make([]models.PermissionGrant, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value.ID = ""
		value.Description = ""
		value.Resource = strings.ToLower(strings.TrimSpace(value.Resource))
		value.Action = strings.ToLower(strings.TrimSpace(value.Action))
		key := value.Resource + ":" + value.Action
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func validateManagedRoleCreate(input models.ManagedRoleCreateInput) error {
	if err := validateManagedRoleText(input.Name, input.Slug, input.Description); err != nil {
		return err
	}
	return validateManagedPermissions(input.Permissions)
}

func validateManagedRolePatch(patch models.ManagedRolePatch) error {
	if patch.ExpectedVersion < 1 {
		return fmt.Errorf("%w: expected_version must be positive", ErrManagedRoleInvalid)
	}
	if patch.Description != nil && patch.ClearDescription {
		return fmt.Errorf("%w: description cannot be set and cleared together", ErrManagedRoleInvalid)
	}
	if patch.Name == nil && patch.Slug == nil && patch.Description == nil && !patch.ClearDescription && patch.Permissions == nil {
		return fmt.Errorf("%w: at least one change is required", ErrManagedRoleInvalid)
	}
	if patch.Name != nil && (utf8.RuneCountInString(*patch.Name) < 2 || utf8.RuneCountInString(*patch.Name) > 100) {
		return fmt.Errorf("%w: name must contain 2-100 characters", ErrManagedRoleInvalid)
	}
	if patch.Slug != nil && !validManagedRoleSlug(*patch.Slug) {
		return fmt.Errorf("%w: slug must contain 2-100 lowercase letters, numbers, or hyphens", ErrManagedRoleInvalid)
	}
	if patch.Description != nil && utf8.RuneCountInString(*patch.Description) > 2000 {
		return fmt.Errorf("%w: description must not exceed 2000 characters", ErrManagedRoleInvalid)
	}
	if patch.Permissions != nil {
		return validateManagedPermissions(*patch.Permissions)
	}
	return nil
}

func validateManagedRoleText(name, slug, description string) error {
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 100 {
		return fmt.Errorf("%w: name must contain 2-100 characters", ErrManagedRoleInvalid)
	}
	if !validManagedRoleSlug(slug) {
		return fmt.Errorf("%w: slug must contain 2-100 lowercase letters, numbers, or hyphens", ErrManagedRoleInvalid)
	}
	if utf8.RuneCountInString(description) > 2000 {
		return fmt.Errorf("%w: description must not exceed 2000 characters", ErrManagedRoleInvalid)
	}
	return nil
}

func validateManagedPermissions(values []models.PermissionGrant) error {
	if len(values) > 128 {
		return fmt.Errorf("%w: no more than 128 permissions are allowed", ErrManagedRoleInvalid)
	}
	for _, value := range values {
		if !validPermissionComponent(value.Resource) || !validPermissionComponent(value.Action) {
			return fmt.Errorf("%w: permission resource and action are invalid", ErrManagedRolePermission)
		}
	}
	return nil
}

func normalizeManagedRoleSlug(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(fallback))
	}
	var builder strings.Builder
	previousHyphen := false
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
			previousHyphen = false
			continue
		}
		if !previousHyphen && builder.Len() > 0 {
			builder.WriteByte('-')
			previousHyphen = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func validManagedRoleSlug(value string) bool {
	if len(value) < 2 || len(value) > 100 || strings.HasPrefix(value, "-") || strings.HasSuffix(value, "-") || strings.Contains(value, "--") {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}

func normalizeManagedRolePagination(value models.PaginationRequest) models.PaginationRequest {
	if value.Page < 1 {
		value.Page = 1
	}
	if value.PageSize < 1 {
		value.PageSize = 20
	}
	if value.PageSize > 100 {
		value.PageSize = 100
	}
	return value
}

func validateManagedRoleOrganization(organizationID string) error {
	if _, err := uuid.Parse(organizationID); err != nil {
		return fmt.Errorf("%w: organization id must be a UUID", ErrManagedRoleInvalid)
	}
	return nil
}

func validateManagedRoleIdentity(organizationID, actorID string) error {
	if err := validateManagedRoleOrganization(organizationID); err != nil {
		return err
	}
	if _, err := uuid.Parse(actorID); err != nil {
		return fmt.Errorf("%w: actor id must be a UUID", ErrManagedRoleInvalid)
	}
	return nil
}

func validateManagedRoleObject(organizationID, roleID string) error {
	if err := validateManagedRoleOrganization(organizationID); err != nil {
		return err
	}
	if _, err := uuid.Parse(roleID); err != nil {
		return fmt.Errorf("%w: role id must be a UUID", ErrManagedRoleInvalid)
	}
	return nil
}

func mapManagedRoleError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrManagedRoleNotFound
	case errors.Is(err, repository.ErrManagedRoleVersionConflict), errors.Is(err, repository.ErrManagedRoleConflict):
		return fmt.Errorf("%w: %v", ErrManagedRoleConflict, err)
	case errors.Is(err, repository.ErrManagedRoleImmutable):
		return ErrManagedRoleImmutable
	case errors.Is(err, repository.ErrManagedRoleInUse):
		return ErrManagedRoleInUse
	case errors.Is(err, repository.ErrManagedRolePermission):
		return fmt.Errorf("%w: permission does not exist", ErrManagedRolePermission)
	case errors.Is(err, repository.ErrManagedRoleAssignment):
		return ErrRoleAssignmentInvalid
	case errors.Is(err, repository.ErrLastTenantAdministrator):
		return ErrLastTenantAdministrator
	default:
		return err
	}
}
