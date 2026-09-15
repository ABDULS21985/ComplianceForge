package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrUserAdministrationInvalid       = errors.New("user administration request is invalid")
	ErrUserAdministrationNotFound      = errors.New("user or group was not found")
	ErrUserAdministrationConflict      = errors.New("user administration request conflicts with current state")
	ErrUserAdministrationVersion       = errors.New("user or group was modified by another request")
	ErrUserAdministrationLastAdmin     = errors.New("tenant must retain an active administrator")
	ErrUserAdministrationOwnership     = errors.New("ownership must be transferred before deprovisioning")
	ErrUserAdministrationDynamicGroup  = errors.New("dynamic group membership is rule-managed")
	ErrUserAdministrationImportInvalid = errors.New("CSV import contains invalid rows")
	ErrUserAdministrationIdempotency   = errors.New("idempotency key conflicts with a different import")
)

const (
	maximumDirectoryImportBytes = 2 << 20
	maximumDirectoryImportRows  = 500
)

type UserAdministrationStore interface {
	CreateUser(context.Context, string, string, models.DirectoryUserCreateInput) (*models.DirectoryUser, error)
	GetUser(context.Context, string, string) (*models.DirectoryUser, error)
	ListUsers(context.Context, string, models.DirectoryUserListFilter) ([]models.DirectoryUser, int, error)
	UpdateUser(context.Context, string, string, *models.DirectoryUser, int64, string) (*models.DirectoryUser, error)
	SuspendUser(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error)
	ReactivateUser(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error)
	PreviewOwnership(context.Context, string, string) (*models.DirectoryOwnershipImpact, error)
	TransferOwnership(context.Context, string, string, string, models.DirectoryOwnershipTransferInput) (*models.DirectoryOwnershipImpact, *models.DirectoryUser, error)
	DeprovisionUser(context.Context, string, string, string, models.DirectoryUserDeprovisionInput) error
	ListEvents(context.Context, string, string, string, models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error)
	CreateGroup(context.Context, string, string, models.DirectoryGroupCreateInput) (*models.DirectoryGroup, error)
	GetGroup(context.Context, string, string) (*models.DirectoryGroup, error)
	ListGroups(context.Context, string, models.DirectoryGroupListFilter) ([]models.DirectoryGroup, int, error)
	UpdateGroup(context.Context, string, string, string, models.DirectoryGroupPatch) (*models.DirectoryGroup, error)
	DeleteGroup(context.Context, string, string, string, int64, string) error
	ListGroupMembers(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryUser, int, error)
	ChangeGroupMembers(context.Context, string, string, string, models.DirectoryGroupBulkMembersInput) (*models.DirectoryGroup, error)
	PreviewImport(context.Context, string, []models.DirectoryImportRow, string) (*models.DirectoryImportPreview, error)
	ApplyImport(context.Context, string, string, string, string, string, []models.DirectoryImportRow) (*models.DirectoryImportResult, error)
}

type UserAdministrationService struct {
	store  UserAdministrationStore
	logger zerolog.Logger
	now    func() time.Time
}

var _ UserAdministrationStore = repository.UserAdministrationRepository(nil)

func NewUserAdministrationService(store UserAdministrationStore, logger zerolog.Logger) *UserAdministrationService {
	return &UserAdministrationService{store: store, logger: logger.With().Str("service", "user_administration").Logger(), now: func() time.Time { return time.Now().UTC() }}
}

func (s *UserAdministrationService) CreateUser(ctx context.Context, organizationID, actorID string, input models.DirectoryUserCreateInput) (*models.DirectoryUser, error) {
	if err := validateDirectoryIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	normalizeDirectoryUserCreate(&input)
	if input.InitialStatus == "" {
		input.InitialStatus = models.UserStatusPendingVerification
	}
	if input.InitialRoleSlug == "" {
		input.InitialRoleSlug = string(models.UserRoleViewer)
	}
	if input.Language == "" {
		input.Language = "en"
	}
	if input.InitialStatus == models.UserStatusPendingVerification && input.InvitationExpiresAt == nil {
		expires := s.now().Add(7 * 24 * time.Hour)
		input.InvitationExpiresAt = &expires
	}
	if err := validateDirectoryUserCreate(input, s.now()); err != nil {
		return nil, err
	}
	item, err := s.store.CreateUser(ctx, organizationID, actorID, input)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("target_user_id", item.ID).Msg("directory user created")
	return item, nil
}

func (s *UserAdministrationService) GetUser(ctx context.Context, organizationID, userID string) (*models.DirectoryUser, error) {
	if err := validateDirectoryObject(organizationID, userID); err != nil {
		return nil, err
	}
	item, err := s.store.GetUser(ctx, organizationID, userID)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return item, nil
}

func (s *UserAdministrationService) ListUsers(ctx context.Context, organizationID string, filter models.DirectoryUserListFilter) ([]models.DirectoryUser, int, error) {
	if !directoryUUID(organizationID) {
		return nil, 0, fmt.Errorf("%w: organization id must be a UUID", ErrUserAdministrationInvalid)
	}
	normalizeDirectoryUserFilter(&filter)
	if filter.Status != "" && !validDirectoryUserStatus(models.UserStatus(filter.Status)) ||
		filter.ManagerID != "" && !directoryUUID(filter.ManagerID) || filter.GroupID != "" && !directoryUUID(filter.GroupID) ||
		filter.RoleSlug != "" && !directorySlugPattern.MatchString(filter.RoleSlug) || utf8.RuneCountInString(filter.Search) > 300 ||
		utf8.RuneCountInString(filter.Department) > 200 || utf8.RuneCountInString(filter.Location) > 200 {
		return nil, 0, fmt.Errorf("%w: unsupported directory filter", ErrUserAdministrationInvalid)
	}
	validSort := map[string]bool{"updated_at": true, "created_at": true, "email": true, "name": true, "department": true, "last_login_at": true}
	if !validSort[filter.SortBy] || filter.SortDirection != "asc" && filter.SortDirection != "desc" {
		return nil, 0, fmt.Errorf("%w: unsupported directory sort", ErrUserAdministrationInvalid)
	}
	items, total, err := s.store.ListUsers(ctx, organizationID, filter)
	if err != nil {
		return nil, 0, mapUserAdministrationError(err)
	}
	return items, total, nil
}

func (s *UserAdministrationService) UpdateUser(ctx context.Context, organizationID, userID, actorID string, patch models.DirectoryUserPatch) (*models.DirectoryUser, error) {
	if err := validateDirectoryMutation(organizationID, userID, actorID, patch.ExpectedVersion, patch.Reason); err != nil {
		return nil, err
	}
	normalizeDirectoryUserPatch(&patch)
	if err := validateDirectoryUserPatchShape(patch); err != nil {
		return nil, err
	}
	current, err := s.GetUser(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	next := *current
	applyDirectoryUserPatch(&next, patch)
	if err := validateDirectoryUserRecord(&next); err != nil {
		return nil, err
	}
	item, err := s.store.UpdateUser(ctx, organizationID, actorID, &next, patch.ExpectedVersion, patch.Reason)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return item, nil
}

func (s *UserAdministrationService) SuspendUser(ctx context.Context, organizationID, userID, actorID string, input models.DirectoryUserStateInput) (*models.DirectoryUser, error) {
	if userID == actorID {
		return nil, fmt.Errorf("%w: administrators cannot suspend their own current account", ErrUserAdministrationConflict)
	}
	if err := validateDirectoryMutation(organizationID, userID, actorID, input.ExpectedVersion, input.Reason); err != nil {
		return nil, err
	}
	item, err := s.store.SuspendUser(ctx, organizationID, userID, actorID, normalizeDirectoryState(input))
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return item, nil
}

func (s *UserAdministrationService) ReactivateUser(ctx context.Context, organizationID, userID, actorID string, input models.DirectoryUserStateInput) (*models.DirectoryUser, error) {
	if err := validateDirectoryMutation(organizationID, userID, actorID, input.ExpectedVersion, input.Reason); err != nil {
		return nil, err
	}
	item, err := s.store.ReactivateUser(ctx, organizationID, userID, actorID, normalizeDirectoryState(input))
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return item, nil
}

func (s *UserAdministrationService) PreviewOwnership(ctx context.Context, organizationID, userID string) (*models.DirectoryOwnershipImpact, error) {
	if err := validateDirectoryObject(organizationID, userID); err != nil {
		return nil, err
	}
	impact, err := s.store.PreviewOwnership(ctx, organizationID, userID)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return impact, nil
}

func (s *UserAdministrationService) TransferOwnership(ctx context.Context, organizationID, userID, actorID string, input models.DirectoryOwnershipTransferInput) (*models.DirectoryOwnershipImpact, *models.DirectoryUser, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	input.ReplacementUserID = strings.TrimSpace(input.ReplacementUserID)
	if err := validateDirectoryMutation(organizationID, userID, actorID, input.ExpectedVersion, input.Reason); err != nil {
		return nil, nil, err
	}
	if !directoryUUID(input.ReplacementUserID) || input.ReplacementUserID == userID {
		return nil, nil, fmt.Errorf("%w: a different active replacement user is required", ErrUserAdministrationInvalid)
	}
	impact, item, err := s.store.TransferOwnership(ctx, organizationID, userID, actorID, input)
	if err != nil {
		return nil, nil, mapUserAdministrationError(err)
	}
	return impact, item, nil
}

func (s *UserAdministrationService) DeprovisionUser(ctx context.Context, organizationID, userID, actorID string, input models.DirectoryUserDeprovisionInput) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if userID == actorID {
		return fmt.Errorf("%w: administrators cannot deprovision their own current account", ErrUserAdministrationConflict)
	}
	if err := validateDirectoryMutation(organizationID, userID, actorID, input.ExpectedVersion, input.Reason); err != nil {
		return err
	}
	if input.ReplacementUserID != nil {
		value := strings.TrimSpace(*input.ReplacementUserID)
		input.ReplacementUserID = &value
		if !directoryUUID(value) || value == userID {
			return fmt.Errorf("%w: replacement user is invalid", ErrUserAdministrationInvalid)
		}
	}
	if err := s.store.DeprovisionUser(ctx, organizationID, userID, actorID, input); err != nil {
		return mapUserAdministrationError(err)
	}
	return nil
}

func (s *UserAdministrationService) ListUserEvents(ctx context.Context, organizationID, userID string, pagination models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error) {
	return s.listDirectoryEvents(ctx, organizationID, "user", userID, pagination)
}

func (s *UserAdministrationService) CreateGroup(ctx context.Context, organizationID, actorID string, input models.DirectoryGroupCreateInput) (*models.DirectoryGroup, error) {
	if err := validateDirectoryIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	normalizeDirectoryGroupCreate(&input)
	if input.GroupType == models.DirectoryGroupDynamic {
		rule, err := validateDynamicGroupRule(input.MembershipRule)
		if err != nil {
			return nil, err
		}
		input.MembershipRule, err = json.Marshal(rule)
		if err != nil {
			return nil, fmt.Errorf("%w: encode dynamic membership rule", ErrUserAdministrationInvalid)
		}
	}
	if err := validateDirectoryGroupCreate(input); err != nil {
		return nil, err
	}
	item, err := s.store.CreateGroup(ctx, organizationID, actorID, input)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return item, nil
}

func (s *UserAdministrationService) GetGroup(ctx context.Context, organizationID, groupID string) (*models.DirectoryGroup, error) {
	if err := validateDirectoryObject(organizationID, groupID); err != nil {
		return nil, err
	}
	item, err := s.store.GetGroup(ctx, organizationID, groupID)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return item, nil
}

func (s *UserAdministrationService) ListGroups(ctx context.Context, organizationID string, filter models.DirectoryGroupListFilter) ([]models.DirectoryGroup, int, error) {
	if !directoryUUID(organizationID) {
		return nil, 0, fmt.Errorf("%w: organization id must be a UUID", ErrUserAdministrationInvalid)
	}
	filter.Search = strings.TrimSpace(filter.Search)
	filter.GroupType = strings.ToLower(strings.TrimSpace(filter.GroupType))
	filter.SortBy = strings.ToLower(strings.TrimSpace(filter.SortBy))
	filter.SortDirection = strings.ToLower(strings.TrimSpace(filter.SortDirection))
	filter.PaginationRequest = normalizeDirectoryPagination(filter.PaginationRequest)
	if filter.SortBy == "" {
		filter.SortBy = "updated_at"
	}
	if filter.SortDirection == "" {
		filter.SortDirection = "desc"
	}
	if utf8.RuneCountInString(filter.Search) > 300 || filter.GroupType != "" && filter.GroupType != "static" && filter.GroupType != "dynamic" {
		return nil, 0, fmt.Errorf("%w: unsupported group filter", ErrUserAdministrationInvalid)
	}
	if (map[string]bool{"updated_at": true, "created_at": true, "name": true, "slug": true})[filter.SortBy] == false || filter.SortDirection != "asc" && filter.SortDirection != "desc" {
		return nil, 0, fmt.Errorf("%w: unsupported group sort", ErrUserAdministrationInvalid)
	}
	items, total, err := s.store.ListGroups(ctx, organizationID, filter)
	if err != nil {
		return nil, 0, mapUserAdministrationError(err)
	}
	return items, total, nil
}

func (s *UserAdministrationService) UpdateGroup(ctx context.Context, organizationID, groupID, actorID string, patch models.DirectoryGroupPatch) (*models.DirectoryGroup, error) {
	patch.Reason = strings.TrimSpace(patch.Reason)
	if err := validateDirectoryMutation(organizationID, groupID, actorID, patch.ExpectedVersion, patch.Reason); err != nil {
		return nil, err
	}
	normalizeDirectoryGroupPatch(&patch)
	if patch.Name == nil && patch.Slug == nil && patch.Description == nil && !patch.ClearDescription && len(patch.MembershipRule) == 0 {
		return nil, fmt.Errorf("%w: at least one group field is required", ErrUserAdministrationInvalid)
	}
	if patch.Description != nil && patch.ClearDescription {
		return nil, fmt.Errorf("%w: description cannot be set and cleared", ErrUserAdministrationInvalid)
	}
	current, err := s.GetGroup(ctx, organizationID, groupID)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil && !directoryText(*patch.Name, 2, 160) || patch.Slug != nil && !directoryGroupSlugPattern.MatchString(*patch.Slug) ||
		patch.Description != nil && !directoryText(*patch.Description, 3, 2000) {
		return nil, fmt.Errorf("%w: invalid group profile", ErrUserAdministrationInvalid)
	}
	if len(patch.MembershipRule) > 0 {
		if current.GroupType != models.DirectoryGroupDynamic {
			return nil, fmt.Errorf("%w: only dynamic groups accept membership rules", ErrUserAdministrationInvalid)
		}
		rule, err := validateDynamicGroupRule(patch.MembershipRule)
		if err != nil {
			return nil, err
		}
		patch.MembershipRule, err = json.Marshal(rule)
		if err != nil {
			return nil, fmt.Errorf("%w: encode dynamic membership rule", ErrUserAdministrationInvalid)
		}
	}
	item, err := s.store.UpdateGroup(ctx, organizationID, groupID, actorID, patch)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return item, nil
}

func (s *UserAdministrationService) DeleteGroup(ctx context.Context, organizationID, groupID, actorID string, expectedVersion int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if err := validateDirectoryMutation(organizationID, groupID, actorID, expectedVersion, reason); err != nil {
		return err
	}
	if err := s.store.DeleteGroup(ctx, organizationID, groupID, actorID, expectedVersion, reason); err != nil {
		return mapUserAdministrationError(err)
	}
	return nil
}

func (s *UserAdministrationService) ListGroupMembers(ctx context.Context, organizationID, groupID string, pagination models.PaginationRequest) ([]models.DirectoryUser, int, error) {
	if err := validateDirectoryObject(organizationID, groupID); err != nil {
		return nil, 0, err
	}
	pagination = normalizeDirectoryPagination(pagination)
	items, total, err := s.store.ListGroupMembers(ctx, organizationID, groupID, pagination)
	if err != nil {
		return nil, 0, mapUserAdministrationError(err)
	}
	return items, total, nil
}

func (s *UserAdministrationService) AddGroupMember(ctx context.Context, organizationID, groupID, actorID string, input models.DirectoryGroupMemberInput) (*models.DirectoryGroup, error) {
	bulk := models.DirectoryGroupBulkMembersInput{ExpectedVersion: input.ExpectedVersion, AddUserIDs: []string{input.UserID}, Reason: input.Reason}
	return s.ChangeGroupMembers(ctx, organizationID, groupID, actorID, bulk)
}

func (s *UserAdministrationService) RemoveGroupMember(ctx context.Context, organizationID, groupID, userID, actorID string, input models.DirectoryGroupMemberRemoveInput) (*models.DirectoryGroup, error) {
	bulk := models.DirectoryGroupBulkMembersInput{ExpectedVersion: input.ExpectedVersion, RemoveUserIDs: []string{userID}, Reason: input.Reason}
	return s.ChangeGroupMembers(ctx, organizationID, groupID, actorID, bulk)
}

func (s *UserAdministrationService) ChangeGroupMembers(ctx context.Context, organizationID, groupID, actorID string, input models.DirectoryGroupBulkMembersInput) (*models.DirectoryGroup, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if err := validateDirectoryMutation(organizationID, groupID, actorID, input.ExpectedVersion, input.Reason); err != nil {
		return nil, err
	}
	input.AddUserIDs = normalizeDirectoryUUIDs(input.AddUserIDs)
	input.RemoveUserIDs = normalizeDirectoryUUIDs(input.RemoveUserIDs)
	if len(input.AddUserIDs)+len(input.RemoveUserIDs) == 0 || len(input.AddUserIDs)+len(input.RemoveUserIDs) > 100 {
		return nil, fmt.Errorf("%w: membership change must contain 1-100 users", ErrUserAdministrationInvalid)
	}
	seen := map[string]bool{}
	for _, userID := range input.AddUserIDs {
		if !directoryUUID(userID) {
			return nil, fmt.Errorf("%w: membership user id must be a UUID", ErrUserAdministrationInvalid)
		}
		seen[userID] = true
	}
	for _, userID := range input.RemoveUserIDs {
		if !directoryUUID(userID) || seen[userID] {
			return nil, fmt.Errorf("%w: users cannot be added and removed together", ErrUserAdministrationInvalid)
		}
	}
	item, err := s.store.ChangeGroupMembers(ctx, organizationID, groupID, actorID, input)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return item, nil
}

func (s *UserAdministrationService) ListGroupEvents(ctx context.Context, organizationID, groupID string, pagination models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error) {
	return s.listDirectoryEvents(ctx, organizationID, "group", groupID, pagination)
}

func (s *UserAdministrationService) listDirectoryEvents(ctx context.Context, organizationID, entityType, entityID string, pagination models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error) {
	if err := validateDirectoryObject(organizationID, entityID); err != nil {
		return nil, 0, err
	}
	pagination = normalizeDirectoryPagination(pagination)
	items, total, err := s.store.ListEvents(ctx, organizationID, entityType, entityID, pagination)
	if err != nil {
		return nil, 0, mapUserAdministrationError(err)
	}
	return items, total, nil
}

func (s *UserAdministrationService) PreviewImport(ctx context.Context, organizationID string, content []byte) (*models.DirectoryImportPreview, error) {
	if !directoryUUID(organizationID) {
		return nil, fmt.Errorf("%w: organization id must be a UUID", ErrUserAdministrationInvalid)
	}
	rows, hash, err := parseDirectoryCSV(content)
	if err != nil {
		return nil, err
	}
	preview, err := s.store.PreviewImport(ctx, organizationID, rows, hash)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return preview, nil
}

func (s *UserAdministrationService) ApplyImport(ctx context.Context, organizationID, actorID, idempotencyKey, reason string, content []byte) (*models.DirectoryImportResult, error) {
	if err := validateDirectoryIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	reason = strings.TrimSpace(reason)
	if !directoryIdempotencyPattern.MatchString(idempotencyKey) || !directoryText(reason, 3, 1000) {
		return nil, fmt.Errorf("%w: idempotency key and 3-1000 character reason are required", ErrUserAdministrationInvalid)
	}
	rows, hash, err := parseDirectoryCSV(content)
	if err != nil {
		return nil, err
	}
	result, err := s.store.ApplyImport(ctx, organizationID, actorID, idempotencyKey, hash, reason, rows)
	if err != nil {
		return nil, mapUserAdministrationError(err)
	}
	return result, nil
}

var (
	directorySlugPattern        = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,99}$`)
	directoryGroupSlugPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,119}$`)
	directoryLanguagePattern    = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)
	directoryIdempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,159}$`)
)

func normalizeDirectoryUserCreate(input *models.DirectoryUserCreateInput) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.JobTitle = strings.TrimSpace(input.JobTitle)
	input.Department = strings.TrimSpace(input.Department)
	input.Phone = strings.TrimSpace(input.Phone)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
	input.Timezone = strings.TrimSpace(input.Timezone)
	input.Language = strings.TrimSpace(input.Language)
	input.EmployeeID = strings.TrimSpace(input.EmployeeID)
	input.Location = strings.TrimSpace(input.Location)
	input.InitialRoleSlug = strings.ToLower(strings.TrimSpace(input.InitialRoleSlug))
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ManagerUserID != nil {
		value := strings.TrimSpace(*input.ManagerUserID)
		input.ManagerUserID = &value
	}
}

func validateDirectoryUserCreate(input models.DirectoryUserCreateInput, now time.Time) error {
	if !validDirectoryEmail(input.Email) || input.FirstName == "" && input.LastName == "" ||
		!directoryText(input.FirstName, 0, 100) || !directoryText(input.LastName, 0, 100) ||
		!directoryText(input.JobTitle, 0, 200) || !directoryText(input.Department, 0, 200) ||
		!directoryText(input.Phone, 0, 50) || !directoryText(input.EmployeeID, 0, 100) ||
		!directoryText(input.Location, 0, 200) || !validDirectoryURL(input.AvatarURL) ||
		!validDirectoryTimezone(input.Timezone) || !directoryLanguagePattern.MatchString(input.Language) ||
		!directorySlugPattern.MatchString(input.InitialRoleSlug) || !directoryText(input.Reason, 3, 1000) {
		return fmt.Errorf("%w: invalid user profile, role, or reason", ErrUserAdministrationInvalid)
	}
	if input.InitialStatus != models.UserStatusActive && input.InitialStatus != models.UserStatusPendingVerification {
		return fmt.Errorf("%w: initial_status must be active or pending_verification", ErrUserAdministrationInvalid)
	}
	if input.ManagerUserID != nil && !directoryUUID(*input.ManagerUserID) {
		return fmt.Errorf("%w: manager_user_id must be a UUID", ErrUserAdministrationInvalid)
	}
	if input.InitialStatus == models.UserStatusActive && input.InvitationExpiresAt != nil {
		return fmt.Errorf("%w: active users do not have an invitation expiry", ErrUserAdministrationInvalid)
	}
	if input.InvitationExpiresAt != nil && (input.InvitationExpiresAt.Before(now.Add(time.Minute)) || input.InvitationExpiresAt.After(now.Add(30*24*time.Hour))) {
		return fmt.Errorf("%w: invitation expiry must be 1 minute to 30 days in the future", ErrUserAdministrationInvalid)
	}
	return nil
}

func normalizeDirectoryUserPatch(patch *models.DirectoryUserPatch) {
	patch.Reason = strings.TrimSpace(patch.Reason)
	for _, pointer := range []*string{patch.Email, patch.FirstName, patch.LastName, patch.JobTitle, patch.Department,
		patch.Phone, patch.AvatarURL, patch.Timezone, patch.Language, patch.ManagerUserID, patch.EmployeeID, patch.Location} {
		if pointer != nil {
			*pointer = strings.TrimSpace(*pointer)
		}
	}
	if patch.Email != nil {
		*patch.Email = strings.ToLower(*patch.Email)
	}
}

func validateDirectoryUserPatchShape(patch models.DirectoryUserPatch) error {
	setAndClear := patch.JobTitle != nil && patch.ClearJobTitle || patch.Department != nil && patch.ClearDepartment ||
		patch.Phone != nil && patch.ClearPhone || patch.AvatarURL != nil && patch.ClearAvatarURL ||
		patch.Timezone != nil && patch.ClearTimezone || patch.ManagerUserID != nil && patch.ClearManager ||
		patch.EmployeeID != nil && patch.ClearEmployeeID || patch.Location != nil && patch.ClearLocation
	if setAndClear {
		return fmt.Errorf("%w: a user field cannot be set and cleared together", ErrUserAdministrationInvalid)
	}
	if patch.Email == nil && patch.FirstName == nil && patch.LastName == nil && patch.JobTitle == nil && !patch.ClearJobTitle &&
		patch.Department == nil && !patch.ClearDepartment && patch.Phone == nil && !patch.ClearPhone &&
		patch.AvatarURL == nil && !patch.ClearAvatarURL && patch.Timezone == nil && !patch.ClearTimezone &&
		patch.Language == nil && patch.ManagerUserID == nil && !patch.ClearManager && patch.EmployeeID == nil &&
		!patch.ClearEmployeeID && patch.Location == nil && !patch.ClearLocation {
		return fmt.Errorf("%w: at least one profile change is required", ErrUserAdministrationInvalid)
	}
	return nil
}

func applyDirectoryUserPatch(item *models.DirectoryUser, patch models.DirectoryUserPatch) {
	if patch.Email != nil {
		item.Email = *patch.Email
	}
	if patch.FirstName != nil {
		item.FirstName = *patch.FirstName
	}
	if patch.LastName != nil {
		item.LastName = *patch.LastName
	}
	applyDirectoryOptional(&item.JobTitle, patch.JobTitle, patch.ClearJobTitle)
	applyDirectoryOptional(&item.Department, patch.Department, patch.ClearDepartment)
	applyDirectoryOptional(&item.Phone, patch.Phone, patch.ClearPhone)
	applyDirectoryOptional(&item.AvatarURL, patch.AvatarURL, patch.ClearAvatarURL)
	applyDirectoryOptional(&item.Timezone, patch.Timezone, patch.ClearTimezone)
	if patch.Language != nil {
		item.Language = *patch.Language
	}
	if patch.ClearManager {
		item.ManagerUserID = nil
	} else if patch.ManagerUserID != nil {
		value := *patch.ManagerUserID
		item.ManagerUserID = &value
	}
	applyDirectoryOptional(&item.EmployeeID, patch.EmployeeID, patch.ClearEmployeeID)
	applyDirectoryOptional(&item.Location, patch.Location, patch.ClearLocation)
}

func applyDirectoryOptional(target *string, value *string, clear bool) {
	if clear {
		*target = ""
	} else if value != nil {
		*target = *value
	}
}

func validateDirectoryUserRecord(item *models.DirectoryUser) error {
	if !validDirectoryEmail(item.Email) || item.FirstName == "" && item.LastName == "" ||
		!directoryText(item.FirstName, 0, 100) || !directoryText(item.LastName, 0, 100) ||
		!directoryText(item.JobTitle, 0, 200) || !directoryText(item.Department, 0, 200) ||
		!directoryText(item.Phone, 0, 50) || !directoryText(item.EmployeeID, 0, 100) ||
		!directoryText(item.Location, 0, 200) || !validDirectoryURL(item.AvatarURL) ||
		!validDirectoryTimezone(item.Timezone) || !directoryLanguagePattern.MatchString(item.Language) {
		return fmt.Errorf("%w: invalid user profile", ErrUserAdministrationInvalid)
	}
	if item.ManagerUserID != nil && (!directoryUUID(*item.ManagerUserID) || *item.ManagerUserID == item.ID) {
		return fmt.Errorf("%w: manager is invalid", ErrUserAdministrationInvalid)
	}
	return nil
}

func normalizeDirectoryUserFilter(filter *models.DirectoryUserListFilter) {
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.Department = strings.TrimSpace(filter.Department)
	filter.Location = strings.TrimSpace(filter.Location)
	filter.ManagerID = strings.TrimSpace(filter.ManagerID)
	filter.RoleSlug = strings.ToLower(strings.TrimSpace(filter.RoleSlug))
	filter.GroupID = strings.TrimSpace(filter.GroupID)
	filter.SortBy = strings.ToLower(strings.TrimSpace(filter.SortBy))
	filter.SortDirection = strings.ToLower(strings.TrimSpace(filter.SortDirection))
	filter.PaginationRequest = normalizeDirectoryPagination(filter.PaginationRequest)
	if filter.SortBy == "" {
		filter.SortBy = "updated_at"
	}
	if filter.SortDirection == "" {
		filter.SortDirection = "desc"
	}
}

func normalizeDirectoryGroupCreate(input *models.DirectoryGroupCreateInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = normalizeDirectoryGroupSlug(input.Slug, input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.GroupType == "" {
		input.GroupType = models.DirectoryGroupStatic
	}
	if input.GroupType == models.DirectoryGroupStatic {
		input.MembershipRule = json.RawMessage(`{}`)
	}
}

func validateDirectoryGroupCreate(input models.DirectoryGroupCreateInput) error {
	if !directoryText(input.Name, 2, 160) || !directoryGroupSlugPattern.MatchString(input.Slug) ||
		!directoryText(input.Description, 0, 2000) || !directoryText(input.Reason, 3, 1000) {
		return fmt.Errorf("%w: invalid group profile", ErrUserAdministrationInvalid)
	}
	switch input.GroupType {
	case models.DirectoryGroupStatic:
		if string(input.MembershipRule) != "{}" {
			return fmt.Errorf("%w: static groups cannot define membership rules", ErrUserAdministrationInvalid)
		}
	case models.DirectoryGroupDynamic:
		if _, err := validateDynamicGroupRule(input.MembershipRule); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: group_type must be static or dynamic", ErrUserAdministrationInvalid)
	}
	return nil
}

func normalizeDirectoryGroupPatch(patch *models.DirectoryGroupPatch) {
	patch.Reason = strings.TrimSpace(patch.Reason)
	if patch.Name != nil {
		value := strings.TrimSpace(*patch.Name)
		patch.Name = &value
	}
	if patch.Slug != nil {
		value := normalizeDirectoryGroupSlug(*patch.Slug, "")
		patch.Slug = &value
	}
	if patch.Description != nil {
		value := strings.TrimSpace(*patch.Description)
		patch.Description = &value
	}
}

func validateDynamicGroupRule(raw json.RawMessage) (models.DirectoryDynamicGroupRule, error) {
	if len(raw) == 0 || len(raw) > 16<<10 {
		return models.DirectoryDynamicGroupRule{}, fmt.Errorf("%w: dynamic membership rule is required and limited to 16 KiB", ErrUserAdministrationInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var rule models.DirectoryDynamicGroupRule
	if err := decoder.Decode(&rule); err != nil {
		return rule, fmt.Errorf("%w: invalid dynamic membership rule", ErrUserAdministrationInvalid)
	}
	if err := ensureDirectoryJSONEOF(decoder); err != nil {
		return rule, fmt.Errorf("%w: invalid dynamic membership rule", ErrUserAdministrationInvalid)
	}
	if len(rule.Departments)+len(rule.Locations)+len(rule.Statuses)+len(rule.RoleSlugs) == 0 {
		return rule, fmt.Errorf("%w: dynamic rule must contain at least one supported criterion", ErrUserAdministrationInvalid)
	}
	if len(rule.Departments) > 50 || len(rule.Locations) > 50 || len(rule.Statuses) > 4 || len(rule.RoleSlugs) > 50 {
		return rule, fmt.Errorf("%w: dynamic rule contains too many values", ErrUserAdministrationInvalid)
	}
	rule.Departments = normalizeDirectoryStrings(rule.Departments)
	rule.Locations = normalizeDirectoryStrings(rule.Locations)
	rule.RoleSlugs = normalizeDirectoryStrings(rule.RoleSlugs)
	for _, value := range append(append([]string{}, rule.Departments...), rule.Locations...) {
		if !directoryText(value, 1, 200) {
			return rule, fmt.Errorf("%w: dynamic rule value is invalid", ErrUserAdministrationInvalid)
		}
	}
	for _, slug := range rule.RoleSlugs {
		if !directorySlugPattern.MatchString(slug) {
			return rule, fmt.Errorf("%w: dynamic role slug is invalid", ErrUserAdministrationInvalid)
		}
	}
	seenStatuses := map[models.UserStatus]bool{}
	for _, status := range rule.Statuses {
		if !validDirectoryUserStatus(status) || seenStatuses[status] {
			return rule, fmt.Errorf("%w: dynamic user status is invalid", ErrUserAdministrationInvalid)
		}
		seenStatuses[status] = true
	}
	return rule, nil
}

func parseDirectoryCSV(content []byte) ([]models.DirectoryImportRow, string, error) {
	if len(content) == 0 || len(content) > maximumDirectoryImportBytes || !utf8.Valid(content) {
		return nil, "", fmt.Errorf("%w: CSV must be valid UTF-8 and no larger than 2 MiB", ErrUserAdministrationInvalid)
	}
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	reader.TrimLeadingSpace = true
	headers, err := reader.Read()
	if err != nil {
		return nil, "", fmt.Errorf("%w: CSV header is required", ErrUserAdministrationInvalid)
	}
	positions := map[string]int{}
	allowed := map[string]bool{"email": true, "first_name": true, "last_name": true, "job_title": true, "department": true, "location": true, "employee_id": true, "manager_email": true, "role_slug": true}
	for index, header := range headers {
		header = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(header, "\ufeff")))
		if !allowed[header] || positions[header] != 0 || header == "" {
			return nil, "", fmt.Errorf("%w: unknown or duplicate CSV header %q", ErrUserAdministrationInvalid, header)
		}
		positions[header] = index + 1
	}
	if positions["email"] == 0 {
		return nil, "", fmt.Errorf("%w: CSV requires an email header", ErrUserAdministrationInvalid)
	}
	rows := make([]models.DirectoryImportRow, 0, 64)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("%w: parse CSV row: %v", ErrUserAdministrationInvalid, err)
		}
		if len(record) != len(headers) {
			return nil, "", fmt.Errorf("%w: CSV row %d has %d fields; expected %d", ErrUserAdministrationInvalid, len(rows)+2, len(record), len(headers))
		}
		if len(rows) >= maximumDirectoryImportRows {
			return nil, "", fmt.Errorf("%w: CSV imports are limited to 500 rows", ErrUserAdministrationInvalid)
		}
		value := func(name string) string {
			index := positions[name]
			if index == 0 {
				return ""
			}
			return strings.TrimSpace(record[index-1])
		}
		row := models.DirectoryImportRow{RowNumber: len(rows) + 2, Email: strings.ToLower(value("email")), FirstName: value("first_name"),
			LastName: value("last_name"), JobTitle: value("job_title"), Department: value("department"), Location: value("location"),
			EmployeeID: value("employee_id"), ManagerEmail: strings.ToLower(value("manager_email")), RoleSlug: strings.ToLower(value("role_slug"))}
		if row.RoleSlug == "" {
			row.RoleSlug = string(models.UserRoleViewer)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, "", fmt.Errorf("%w: CSV requires at least one data row", ErrUserAdministrationInvalid)
	}
	return rows, hash, nil
}

func ensureDirectoryJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}
func normalizeDirectoryState(input models.DirectoryUserStateInput) models.DirectoryUserStateInput {
	input.Reason = strings.TrimSpace(input.Reason)
	return input
}
func normalizeDirectoryPagination(value models.PaginationRequest) models.PaginationRequest {
	if value.Page < 1 {
		value.Page = 1
	}
	if value.PageSize < 1 || value.PageSize > 100 {
		value.PageSize = 20
	}
	return value
}
func normalizeDirectoryUUIDs(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
func normalizeDirectoryStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
func normalizeDirectoryGroupSlug(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(fallback))
	}
	value = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-_")
}
func validateDirectoryIdentity(orgID, actorID string) error {
	if !directoryUUID(orgID) || !directoryUUID(actorID) {
		return fmt.Errorf("%w: organization and actor ids must be UUIDs", ErrUserAdministrationInvalid)
	}
	return nil
}
func validateDirectoryObject(orgID, objectID string) error {
	if !directoryUUID(orgID) || !directoryUUID(objectID) {
		return fmt.Errorf("%w: organization and object ids must be UUIDs", ErrUserAdministrationInvalid)
	}
	return nil
}
func validateDirectoryMutation(orgID, objectID, actorID string, version int64, reason string) error {
	if err := validateDirectoryIdentity(orgID, actorID); err != nil {
		return err
	}
	if !directoryUUID(objectID) || version < 1 || !directoryText(strings.TrimSpace(reason), 3, 1000) {
		return fmt.Errorf("%w: object id, positive version, and 3-1000 character reason are required", ErrUserAdministrationInvalid)
	}
	return nil
}
func directoryUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}
func directoryText(value string, minimum, maximum int) bool {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	return length >= minimum && length <= maximum && !strings.ContainsRune(value, '\x00')
}
func validDirectoryEmail(value string) bool {
	if value == "" || len(value) > 320 {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}
func validDirectoryURL(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && len(value) <= 2000
}
func validDirectoryTimezone(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 50 {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}
func validDirectoryUserStatus(value models.UserStatus) bool {
	switch value {
	case models.UserStatusActive, models.UserStatusInactive, models.UserStatusLocked, models.UserStatusPendingVerification:
		return true
	}
	return false
}

func mapUserAdministrationError(err error) error {
	switch {
	case errors.Is(err, repository.ErrDirectoryUserNotFound), errors.Is(err, repository.ErrDirectoryGroupNotFound):
		return ErrUserAdministrationNotFound
	case errors.Is(err, repository.ErrDirectoryVersionConflict):
		return ErrUserAdministrationVersion
	case errors.Is(err, repository.ErrDirectoryLastAdmin):
		return ErrUserAdministrationLastAdmin
	case errors.Is(err, repository.ErrDirectoryOwnershipRequired):
		return ErrUserAdministrationOwnership
	case errors.Is(err, repository.ErrDirectoryDynamicGroup):
		return ErrUserAdministrationDynamicGroup
	case errors.Is(err, repository.ErrDirectoryImportConflict):
		return ErrUserAdministrationIdempotency
	case errors.Is(err, repository.ErrDirectoryImportInvalid):
		return ErrUserAdministrationImportInvalid
	case errors.Is(err, repository.ErrEntitlementLimitExceeded):
		return fmt.Errorf("%w: user capacity is exhausted", ErrSubscriptionLimitExceeded)
	case errors.Is(err, repository.ErrDirectoryConflict), errors.Is(err, repository.ErrDirectoryInvalidUser):
		return fmt.Errorf("%w: %v", ErrUserAdministrationConflict, err)
	default:
		return err
	}
}
