package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

var (
	ErrDataQualityScope       = models.ErrDataQualityScope
	ErrDataQualityUnavailable = models.ErrDataQualityUnavailable
)

const defaultDataQualityTimeout = 2 * time.Second

type DataQualityStore interface {
	LoadDataQualityCounts(context.Context, string, string) (*models.DataQualityCounts, error)
}

type DataQualityService struct {
	store   DataQualityStore
	timeout time.Duration
}

func NewDataQualityService(store DataQualityStore) (*DataQualityService, error) {
	return NewDataQualityServiceWithTimeout(store, defaultDataQualityTimeout)
}

// NewDataQualityServiceWithTimeout permits a shorter deployment/test budget,
// never an unbounded scan. Timeout returns no partial or sampled counts.
func NewDataQualityServiceWithTimeout(store DataQualityStore, timeout time.Duration) (*DataQualityService, error) {
	if interfaceValueIsNil(store) || timeout <= 0 || timeout > defaultDataQualityTimeout {
		return nil, errors.New("data quality requires a store and a query budget no greater than two seconds")
	}
	return &DataQualityService{store: store, timeout: timeout}, nil
}

func (s *DataQualityService) GetSnapshot(ctx context.Context, organizationID, actorID string) (*models.DataQualitySnapshot, error) {
	if !validDataQualityPrincipal(organizationID) || !validDataQualityPrincipal(actorID) {
		return nil, ErrDataQualityScope
	}
	if s == nil || interfaceValueIsNil(s.store) || s.timeout <= 0 || s.timeout > defaultDataQualityTimeout || ctx.Err() != nil {
		return nil, ErrDataQualityUnavailable
	}
	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	counts, err := s.store.LoadDataQualityCounts(queryCtx, organizationID, actorID)
	if errors.Is(err, ErrDataQualityScope) {
		return nil, ErrDataQualityScope
	}
	if err != nil || queryCtx.Err() != nil || counts == nil {
		return nil, ErrDataQualityUnavailable
	}
	if counts.OrganizationID != organizationID || counts.ActorID != actorID ||
		counts.SchemaVersion != database.SupportedSchemaVersion || !validDataQualityAsOf(counts.AsOf) {
		return nil, ErrDataQualityUnavailable
	}
	definitions := models.DataQualityCheckDefinitions()
	if len(counts.Counts) != len(definitions) {
		return nil, ErrDataQualityUnavailable
	}
	byKey := make(map[models.DataQualityCheckKey]int64, len(definitions))
	for _, value := range counts.Counts {
		if _, duplicate := byKey[value.Key]; duplicate || !validDataQualityCount(value.Count) {
			return nil, ErrDataQualityUnavailable
		}
		byKey[value.Key] = value.Count
	}
	snapshot := &models.DataQualitySnapshot{
		RulesetVersion: models.DataQualityRulesetVersion, Scope: models.DataQualityScope,
		SchemaVersion: counts.SchemaVersion, AsOf: counts.AsOf.UTC(),
		Status: models.DataQualityHealthy, Checks: make([]models.DataQualityCheck, 0, len(definitions)),
	}
	for _, definition := range definitions {
		count, exists := byKey[definition.Key]
		if !exists {
			return nil, ErrDataQualityUnavailable // Missing and unknown keys both fail closed.
		}
		status := dataQualityCheckStatus(definition.Category, count)
		snapshot.Checks = append(snapshot.Checks, models.DataQualityCheck{
			DataQualityCheckDefinition: definition, Count: count, Status: status,
		})
		snapshot.Status = worseDataQualityStatus(snapshot.Status, status)
	}
	return snapshot, nil
}

// ValidateDataQualitySnapshot is the final typed-provider integrity boundary.
// It rejects injected definitions, hidden partial check sets, invalid counts,
// and derived statuses inconsistent with the exact fixed ruleset.
func ValidateDataQualitySnapshot(snapshot *models.DataQualitySnapshot) bool {
	if snapshot == nil || snapshot.RulesetVersion != models.DataQualityRulesetVersion ||
		snapshot.Scope != models.DataQualityScope || snapshot.SchemaVersion != database.SupportedSchemaVersion ||
		!validDataQualityAsOf(snapshot.AsOf) || snapshot.AsOf.Location() != time.UTC {
		return false
	}
	definitions := models.DataQualityCheckDefinitions()
	if len(snapshot.Checks) != len(definitions) {
		return false
	}
	overall := models.DataQualityHealthy
	for index, definition := range definitions {
		check := snapshot.Checks[index]
		if check.DataQualityCheckDefinition != definition || !validDataQualityCount(check.Count) ||
			check.Status != dataQualityCheckStatus(definition.Category, check.Count) {
			return false
		}
		overall = worseDataQualityStatus(overall, check.Status)
	}
	return snapshot.Status == overall
}

func validDataQualityPrincipal(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func validDataQualityCount(value int64) bool {
	return value >= 0 && value <= models.MaximumDataQualityCount
}

func validDataQualityAsOf(value time.Time) bool {
	return !value.IsZero() && value.Year() >= 1970 && value.Year() <= 9999
}

func dataQualityCheckStatus(category models.DataQualityCategory, count int64) models.DataQualityStatus {
	if count == 0 {
		return models.DataQualityHealthy
	}
	if category == models.DataQualityLifecycle {
		return models.DataQualityWarning
	}
	return models.DataQualityCritical
}

func worseDataQualityStatus(left, right models.DataQualityStatus) models.DataQualityStatus {
	if left == models.DataQualityCritical || right == models.DataQualityCritical {
		return models.DataQualityCritical
	}
	if left == models.DataQualityWarning || right == models.DataQualityWarning {
		return models.DataQualityWarning
	}
	return models.DataQualityHealthy
}
