package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

var (
	ErrAssetNotFound      = errors.New("asset not found")
	ErrInvalidAsset       = errors.New("invalid asset")
	ErrAssetConflict      = errors.New("asset request conflicts with current state")
	ErrAssetOwnerNotFound = errors.New("asset owner is not an active tenant user")
)

type AssetManagementRepository interface {
	Create(context.Context, string, string, models.AssetCreateInput) (*models.Asset, error)
	GetByID(context.Context, string, string) (*models.Asset, error)
	Update(context.Context, string, string, string, models.AssetPatch) (*models.Asset, error)
	Delete(context.Context, string, string, string, *int64) error
	List(context.Context, string, models.AssetListFilter) ([]models.Asset, int, error)
	Stats(context.Context, string) (*models.AssetStats, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error)
}

type AssetService struct {
	repository AssetManagementRepository
	logger     zerolog.Logger
}

func NewAssetService(repository AssetManagementRepository, logger zerolog.Logger) *AssetService {
	return &AssetService{repository: repository, logger: logger.With().Str("service", "asset").Logger()}
}

func (s *AssetService) Create(ctx context.Context, organizationID, actorID string, input models.AssetCreateInput) (*models.Asset, error) {
	if err := validateAssetIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	normalizeAssetCreate(&input)
	if err := validateAssetCreate(input); err != nil {
		return nil, err
	}
	item, err := s.repository.Create(ctx, organizationID, actorID, input)
	if err != nil {
		return nil, mapAssetRepositoryError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("asset_id", item.ID).Msg("asset created")
	return item, nil
}

func (s *AssetService) GetByID(ctx context.Context, organizationID, id string) (*models.Asset, error) {
	if err := validateAssetObjectIdentity(organizationID, id); err != nil {
		return nil, err
	}
	item, err := s.repository.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, mapAssetRepositoryError(err)
	}
	return item, nil
}

func (s *AssetService) Update(ctx context.Context, organizationID, id, actorID string, patch models.AssetPatch) (*models.Asset, error) {
	if err := validateAssetIdentity(organizationID, actorID); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: asset id must be a UUID", ErrInvalidAsset)
	}
	current, err := s.repository.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, mapAssetRepositoryError(err)
	}
	normalizeAssetPatch(&patch)
	if err := validateAssetPatch(current, patch); err != nil {
		return nil, err
	}
	if patch.ExpectedVersion == nil {
		version := current.Version
		patch.ExpectedVersion = &version
	}
	item, err := s.repository.Update(ctx, organizationID, id, actorID, patch)
	if err != nil {
		return nil, mapAssetRepositoryError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("asset_id", id).Int64("version", item.Version).Msg("asset updated")
	return item, nil
}

func (s *AssetService) Delete(ctx context.Context, organizationID, id, actorID string, expectedVersion *int64) error {
	if err := validateAssetIdentity(organizationID, actorID); err != nil {
		return err
	}
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: asset id must be a UUID", ErrInvalidAsset)
	}
	current, err := s.repository.GetByID(ctx, organizationID, id)
	if err != nil {
		return mapAssetRepositoryError(err)
	}
	if expectedVersion == nil {
		version := current.Version
		expectedVersion = &version
	} else if *expectedVersion < 1 {
		return fmt.Errorf("%w: expected_version must be positive", ErrInvalidAsset)
	}
	if err := s.repository.Delete(ctx, organizationID, id, actorID, expectedVersion); err != nil {
		return mapAssetRepositoryError(err)
	}
	s.logger.Info().Str("organization_id", organizationID).Str("asset_id", id).Msg("asset deleted")
	return nil
}

func (s *AssetService) List(ctx context.Context, organizationID string, filter models.AssetListFilter) ([]models.Asset, int, error) {
	if _, err := uuid.Parse(organizationID); err != nil {
		return nil, 0, fmt.Errorf("%w: organization id must be a UUID", ErrInvalidAsset)
	}
	normalizeAssetFilter(&filter)
	if err := validateAssetFilter(filter); err != nil {
		return nil, 0, err
	}
	items, total, err := s.repository.List(ctx, organizationID, filter)
	if err != nil {
		return nil, 0, mapAssetRepositoryError(err)
	}
	return items, total, nil
}

func (s *AssetService) Stats(ctx context.Context, organizationID string) (*models.AssetStats, error) {
	if _, err := uuid.Parse(organizationID); err != nil {
		return nil, fmt.Errorf("%w: organization id must be a UUID", ErrInvalidAsset)
	}
	stats, err := s.repository.Stats(ctx, organizationID)
	if err != nil {
		return nil, mapAssetRepositoryError(err)
	}
	return stats, nil
}

func (s *AssetService) ListEvents(ctx context.Context, organizationID, assetID string, pagination models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error) {
	if err := validateAssetObjectIdentity(organizationID, assetID); err != nil {
		return nil, 0, err
	}
	if _, err := s.repository.GetByID(ctx, organizationID, assetID); err != nil {
		return nil, 0, mapAssetRepositoryError(err)
	}
	pagination = normalizeAssetPagination(pagination)
	events, total, err := s.repository.ListEvents(ctx, organizationID, assetID, pagination)
	if err != nil {
		return nil, 0, mapAssetRepositoryError(err)
	}
	return events, total, nil
}

func normalizeAssetCreate(input *models.AssetCreateInput) {
	input.Name = strings.TrimSpace(input.Name)
	input.Category = strings.TrimSpace(input.Category)
	input.Description = strings.TrimSpace(input.Description)
	input.Location = strings.TrimSpace(input.Location)
	if input.Criticality == "" {
		input.Criticality = models.AssetCriticalityMedium
	}
	if input.Classification == "" {
		input.Classification = models.AssetClassificationInternal
	}
	input.OwnerUserID = normalizedOptionalString(input.OwnerUserID)
	input.IPAddress = normalizedOptionalString(input.IPAddress)
	input.LinkedVendorID = normalizedOptionalString(input.LinkedVendorID)
	input.Tags = normalizeAssetTags(input.Tags)
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage("{}")
	}
}

func normalizeAssetPatch(patch *models.AssetPatch) {
	trimAssetPointer(patch.Name)
	trimAssetPointer(patch.Category)
	trimAssetPointer(patch.Description)
	trimAssetPointer(patch.Location)
	patch.OwnerUserID = normalizedOptionalString(patch.OwnerUserID)
	patch.IPAddress = normalizedOptionalString(patch.IPAddress)
	patch.LinkedVendorID = normalizedOptionalString(patch.LinkedVendorID)
	if patch.Tags != nil {
		normalized := normalizeAssetTags(*patch.Tags)
		patch.Tags = &normalized
	}
}

func validateAssetCreate(input models.AssetCreateInput) error {
	if err := validateAssetText(input.Name, "name", 1, 200); err != nil {
		return err
	}
	if !validAssetType(input.AssetType) {
		return fmt.Errorf("%w: unsupported asset_type", ErrInvalidAsset)
	}
	for _, field := range []struct {
		value string
		name  string
		max   int
	}{{input.Category, "category", 100}, {input.Description, "description", 10000}, {input.Location, "location", 200}} {
		if err := validateAssetText(field.value, field.name, 0, field.max); err != nil {
			return err
		}
	}
	if !validAssetCriticality(input.Criticality) || !validAssetClassification(input.Classification) {
		return fmt.Errorf("%w: unsupported criticality or classification", ErrInvalidAsset)
	}
	if err := validateAssetOptionalIDs(input.OwnerUserID, input.LinkedVendorID); err != nil {
		return err
	}
	if err := validateAssetIP(input.IPAddress); err != nil {
		return err
	}
	if err := validateAssetTags(input.Tags); err != nil {
		return err
	}
	return validateAssetMetadata(input.Metadata)
}

func validateAssetPatch(current *models.Asset, patch models.AssetPatch) error {
	if current == nil {
		return ErrAssetNotFound
	}
	if current.Status == models.AssetStatusDecommissioned {
		return fmt.Errorf("%w: decommissioned assets are immutable", ErrAssetConflict)
	}
	if patch.ExpectedVersion != nil && *patch.ExpectedVersion != current.Version {
		return fmt.Errorf("%w: expected version does not match", ErrAssetConflict)
	}
	if patch.Name != nil {
		if err := validateAssetText(*patch.Name, "name", 1, 200); err != nil {
			return err
		}
	}
	if patch.AssetType != nil && !validAssetType(*patch.AssetType) {
		return fmt.Errorf("%w: unsupported asset_type", ErrInvalidAsset)
	}
	for _, field := range []struct {
		value *string
		name  string
		max   int
	}{{patch.Category, "category", 100}, {patch.Description, "description", 10000}, {patch.Location, "location", 200}} {
		if field.value != nil {
			if err := validateAssetText(*field.value, field.name, 0, field.max); err != nil {
				return err
			}
		}
	}
	if patch.Criticality != nil && !validAssetCriticality(*patch.Criticality) {
		return fmt.Errorf("%w: unsupported criticality", ErrInvalidAsset)
	}
	if patch.Classification != nil && !validAssetClassification(*patch.Classification) {
		return fmt.Errorf("%w: unsupported classification", ErrInvalidAsset)
	}
	if patch.Status != nil && !validAssetStatusTransition(current.Status, *patch.Status) {
		return fmt.Errorf("%w: status transition %s to %s is not allowed", ErrAssetConflict, current.Status, *patch.Status)
	}
	if patch.ClearOwner && patch.OwnerUserID != nil {
		return fmt.Errorf("%w: owner cannot be set and cleared together", ErrInvalidAsset)
	}
	if patch.ClearIPAddress && patch.IPAddress != nil {
		return fmt.Errorf("%w: ip_address cannot be set and cleared together", ErrInvalidAsset)
	}
	if patch.ClearLinkedVendor && patch.LinkedVendorID != nil {
		return fmt.Errorf("%w: linked_vendor_id cannot be set and cleared together", ErrInvalidAsset)
	}
	if err := validateAssetOptionalIDs(patch.OwnerUserID, patch.LinkedVendorID); err != nil {
		return err
	}
	if err := validateAssetIP(patch.IPAddress); err != nil {
		return err
	}
	if patch.Tags != nil {
		if err := validateAssetTags(*patch.Tags); err != nil {
			return err
		}
	}
	if len(patch.Metadata) > 0 {
		return validateAssetMetadata(patch.Metadata)
	}
	return nil
}

func normalizeAssetFilter(filter *models.AssetListFilter) {
	filter.PaginationRequest = normalizeAssetPagination(filter.PaginationRequest)
	filter.AssetType = strings.ToLower(strings.TrimSpace(filter.AssetType))
	filter.Criticality = strings.ToLower(strings.TrimSpace(filter.Criticality))
	filter.Classification = strings.ToLower(strings.TrimSpace(filter.Classification))
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.OwnerUserID = strings.TrimSpace(filter.OwnerUserID)
	filter.Tag = strings.ToLower(strings.TrimSpace(filter.Tag))
	filter.Search = strings.TrimSpace(filter.Search)
	filter.SortBy = strings.ToLower(strings.TrimSpace(filter.SortBy))
	filter.SortDirection = strings.ToLower(strings.TrimSpace(filter.SortDirection))
	if filter.SortBy == "" {
		filter.SortBy = "updated_at"
	}
	if filter.SortDirection == "" {
		filter.SortDirection = "desc"
	}
}

func validateAssetFilter(filter models.AssetListFilter) error {
	if filter.AssetType != "" && !validAssetType(models.AssetType(filter.AssetType)) {
		return fmt.Errorf("%w: unsupported asset_type filter", ErrInvalidAsset)
	}
	if filter.Criticality != "" && !validAssetCriticality(models.AssetCriticality(filter.Criticality)) {
		return fmt.Errorf("%w: unsupported criticality filter", ErrInvalidAsset)
	}
	if filter.Classification != "" && !validAssetClassification(models.AssetClassification(filter.Classification)) {
		return fmt.Errorf("%w: unsupported classification filter", ErrInvalidAsset)
	}
	if filter.Status != "" && !validAssetStatus(models.AssetStatus(filter.Status)) {
		return fmt.Errorf("%w: unsupported status filter", ErrInvalidAsset)
	}
	if filter.OwnerUserID != "" {
		if _, err := uuid.Parse(filter.OwnerUserID); err != nil {
			return fmt.Errorf("%w: owner_user_id filter must be a UUID", ErrInvalidAsset)
		}
	}
	if utf8.RuneCountInString(filter.Search) > 200 || utf8.RuneCountInString(filter.Tag) > 64 {
		return fmt.Errorf("%w: search or tag filter is too long", ErrInvalidAsset)
	}
	validSorts := map[string]bool{"updated_at": true, "created_at": true, "name": true, "asset_ref": true, "asset_type": true, "criticality": true}
	if !validSorts[filter.SortBy] || (filter.SortDirection != "asc" && filter.SortDirection != "desc") {
		return fmt.Errorf("%w: unsupported sort", ErrInvalidAsset)
	}
	return nil
}

func validateAssetIdentity(organizationID, actorID string) error {
	if _, err := uuid.Parse(organizationID); err != nil {
		return fmt.Errorf("%w: organization id must be a UUID", ErrInvalidAsset)
	}
	if _, err := uuid.Parse(actorID); err != nil {
		return fmt.Errorf("%w: actor id must be a UUID", ErrInvalidAsset)
	}
	return nil
}

func validateAssetObjectIdentity(organizationID, id string) error {
	if _, err := uuid.Parse(organizationID); err != nil {
		return fmt.Errorf("%w: organization id must be a UUID", ErrInvalidAsset)
	}
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: asset id must be a UUID", ErrInvalidAsset)
	}
	return nil
}

func validateAssetOptionalIDs(values ...*string) error {
	for _, value := range values {
		if value != nil {
			if _, err := uuid.Parse(*value); err != nil {
				return fmt.Errorf("%w: related identifiers must be UUIDs", ErrInvalidAsset)
			}
		}
	}
	return nil
}

func validateAssetIP(value *string) error {
	if value == nil {
		return nil
	}
	if _, err := netip.ParseAddr(*value); err != nil {
		return fmt.Errorf("%w: ip_address must be an IPv4 or IPv6 address", ErrInvalidAsset)
	}
	return nil
}

func validateAssetMetadata(value json.RawMessage) error {
	if len(value) > 16<<10 || !json.Valid(value) {
		return fmt.Errorf("%w: metadata must be valid JSON no larger than 16 KiB", ErrInvalidAsset)
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return fmt.Errorf("%w: metadata must be a JSON object", ErrInvalidAsset)
	}
	return nil
}

func validateAssetTags(tags []string) error {
	if len(tags) > 50 {
		return fmt.Errorf("%w: no more than 50 tags are allowed", ErrInvalidAsset)
	}
	for _, tag := range tags {
		if count := utf8.RuneCountInString(tag); count < 1 || count > 64 {
			return fmt.Errorf("%w: tags must contain between 1 and 64 characters", ErrInvalidAsset)
		}
	}
	return nil
}

func normalizeAssetTags(tags []string) []string {
	unique := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			unique[tag] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for tag := range unique {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func normalizedOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func trimAssetPointer(value *string) {
	if value != nil {
		*value = strings.TrimSpace(*value)
	}
}

func validateAssetText(value, field string, minimum, maximum int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: %s must be valid UTF-8", ErrInvalidAsset, field)
	}
	length := utf8.RuneCountInString(value)
	if length < minimum || length > maximum {
		return fmt.Errorf("%w: %s must contain between %d and %d characters", ErrInvalidAsset, field, minimum, maximum)
	}
	return nil
}

func validAssetType(value models.AssetType) bool {
	switch value {
	case models.AssetTypeHardware, models.AssetTypeSoftware, models.AssetTypeData, models.AssetTypeService,
		models.AssetTypeNetwork, models.AssetTypePeople, models.AssetTypeFacility:
		return true
	default:
		return false
	}
}

func validAssetCriticality(value models.AssetCriticality) bool {
	switch value {
	case models.AssetCriticalityCritical, models.AssetCriticalityHigh, models.AssetCriticalityMedium, models.AssetCriticalityLow:
		return true
	default:
		return false
	}
}

func validAssetClassification(value models.AssetClassification) bool {
	switch value {
	case models.AssetClassificationPublic, models.AssetClassificationInternal,
		models.AssetClassificationConfidential, models.AssetClassificationRestricted:
		return true
	default:
		return false
	}
}

func validAssetStatus(value models.AssetStatus) bool {
	switch value {
	case models.AssetStatusActive, models.AssetStatusInactive, models.AssetStatusDecommissioned:
		return true
	default:
		return false
	}
}

func validAssetStatusTransition(from, to models.AssetStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case models.AssetStatusActive:
		return to == models.AssetStatusInactive || to == models.AssetStatusDecommissioned
	case models.AssetStatusInactive:
		return to == models.AssetStatusActive || to == models.AssetStatusDecommissioned
	default:
		return false
	}
}

func normalizeAssetPagination(value models.PaginationRequest) models.PaginationRequest {
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

func mapAssetRepositoryError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrAssetNotFound
	case errors.Is(err, repository.ErrAssetVersionConflict):
		return ErrAssetConflict
	case errors.Is(err, repository.ErrAssetOwnerInvalid):
		return ErrAssetOwnerNotFound
	default:
		return err
	}
}
