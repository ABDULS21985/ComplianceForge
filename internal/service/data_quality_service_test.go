package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const (
	dataQualityTestOrganization = "a1000000-0000-0000-0000-000000000001"
	dataQualityTestActor        = "a1000000-0000-0000-0000-000000000002"
)

type dataQualityStoreFunc func(context.Context, string, string) (*models.DataQualityCounts, error)

func (f dataQualityStoreFunc) LoadDataQualityCounts(ctx context.Context, organizationID, actorID string) (*models.DataQualityCounts, error) {
	return f(ctx, organizationID, actorID)
}

func dataQualityServiceCounts() *models.DataQualityCounts {
	result := &models.DataQualityCounts{
		OrganizationID: dataQualityTestOrganization, ActorID: dataQualityTestActor,
		SchemaVersion: database.SupportedSchemaVersion,
		AsOf:          time.Date(2026, 9, 15, 12, 34, 56, 123456000, time.FixedZone("fixture-local", 3600)),
	}
	for _, definition := range models.DataQualityCheckDefinitions() {
		result.Counts = append(result.Counts, models.DataQualityCount{Key: definition.Key})
	}
	return result
}

func TestDataQualitySnapshotUsesFixedCountsAndSingleClockWithoutMutation(t *testing.T) {
	counts := dataQualityServiceCounts()
	counts.Counts[0].Count = 3
	counts.Counts[16].Count = 5
	counts.Counts[17].Count = models.MaximumDataQualityCount
	before := *counts
	before.Counts = append([]models.DataQualityCount(nil), counts.Counts...)
	// Store order is not trusted; output order is always the curated catalog.
	for left, right := 0, len(counts.Counts)-1; left < right; left, right = left+1, right-1 {
		counts.Counts[left], counts.Counts[right] = counts.Counts[right], counts.Counts[left]
	}
	storeOrder := append([]models.DataQualityCount(nil), counts.Counts...)
	provider, err := NewDataQualityService(dataQualityStoreFunc(func(ctx context.Context, organizationID, actorID string) (*models.DataQualityCounts, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second || organizationID != dataQualityTestOrganization || actorID != dataQualityTestActor {
			t.Fatalf("missing bounded scope: deadline=%v org=%s actor=%s", deadline, organizationID, actorID)
		}
		return counts, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := provider.GetSnapshot(context.Background(), dataQualityTestOrganization, dataQualityTestActor)
	if err != nil || !ValidateDataQualitySnapshot(snapshot) {
		t.Fatalf("invalid snapshot %#v, error=%v", snapshot, err)
	}
	if snapshot.Status != models.DataQualityCritical || snapshot.Checks[0].Count != 3 || snapshot.Checks[16].Status != models.DataQualityWarning || snapshot.Checks[17].Count != models.MaximumDataQualityCount {
		t.Fatalf("counts/status drift: %#v", snapshot)
	}
	if !snapshot.AsOf.Equal(before.AsOf) || snapshot.AsOf.Location() != time.UTC || !reflect.DeepEqual(counts.Counts, storeOrder) {
		t.Fatalf("store clock or count order mutated: %#v", counts)
	}
	snapshot.Checks[0].Count = 999
	if !reflect.DeepEqual(counts.Counts, storeOrder) {
		t.Fatal("response mutated shared store counts")
	}
}

func TestDataQualitySnapshotRejectsIncompleteUnsafeOrForeignResults(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.DataQualityCounts)
	}{
		{"foreign organization", func(value *models.DataQualityCounts) { value.OrganizationID = dataQualityTestActor }},
		{"foreign actor", func(value *models.DataQualityCounts) { value.ActorID = dataQualityTestOrganization }},
		{"old schema", func(value *models.DataQualityCounts) { value.SchemaVersion-- }},
		{"new schema", func(value *models.DataQualityCounts) { value.SchemaVersion++ }},
		{"missing clock", func(value *models.DataQualityCounts) { value.AsOf = time.Time{} }},
		{"ancient clock", func(value *models.DataQualityCounts) { value.AsOf = time.Date(1969, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{"unserializable clock", func(value *models.DataQualityCounts) { value.AsOf = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{"missing check", func(value *models.DataQualityCounts) { value.Counts = value.Counts[:17] }},
		{"extra check", func(value *models.DataQualityCounts) {
			value.Counts = append(value.Counts, models.DataQualityCount{Key: "unknown"})
		}},
		{"unknown check", func(value *models.DataQualityCounts) { value.Counts[0].Key = "unknown" }},
		{"duplicate check", func(value *models.DataQualityCounts) { value.Counts[0].Key = value.Counts[1].Key }},
		{"negative count", func(value *models.DataQualityCounts) { value.Counts[0].Count = -1 }},
		{"unsafe JavaScript count", func(value *models.DataQualityCounts) { value.Counts[0].Count = models.MaximumDataQualityCount + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			counts := dataQualityServiceCounts()
			test.mutate(counts)
			provider, err := NewDataQualityService(dataQualityStoreFunc(func(context.Context, string, string) (*models.DataQualityCounts, error) { return counts, nil }))
			if err != nil {
				t.Fatal(err)
			}
			if snapshot, err := provider.GetSnapshot(context.Background(), dataQualityTestOrganization, dataQualityTestActor); snapshot != nil || !errors.Is(err, ErrDataQualityUnavailable) {
				t.Fatalf("unsafe/partial result survived: %#v error=%v", snapshot, err)
			}
		})
	}
}

func TestDataQualityFailsClosedOnStoreFailureCancellationOrScope(t *testing.T) {
	for _, test := range []struct {
		name  string
		err   error
		want  error
		value *models.DataQualityCounts
	}{
		{"query failure", errors.New("postgres credentials business record secret"), ErrDataQualityUnavailable, nil},
		{"scope denial", ErrDataQualityScope, ErrDataQualityScope, nil},
		{"nil result", nil, ErrDataQualityUnavailable, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider, _ := NewDataQualityService(dataQualityStoreFunc(func(context.Context, string, string) (*models.DataQualityCounts, error) { return test.value, test.err }))
			value, err := provider.GetSnapshot(context.Background(), dataQualityTestOrganization, dataQualityTestActor)
			if value != nil || !errors.Is(err, test.want) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("safe failure drift: %#v error=%v", value, err)
			}
		})
	}
	called := false
	provider, _ := NewDataQualityService(dataQualityStoreFunc(func(context.Context, string, string) (*models.DataQualityCounts, error) {
		called = true
		return dataQualityServiceCounts(), nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if value, err := provider.GetSnapshot(ctx, dataQualityTestOrganization, dataQualityTestActor); value != nil || !errors.Is(err, ErrDataQualityUnavailable) || called {
		t.Fatalf("canceled request queried store: value=%#v error=%v called=%v", value, err, called)
	}
	for _, invalid := range []string{"", "a1000000000000000000000000000001", strings.ToUpper(dataQualityTestOrganization), "00000000-0000-0000-0000-000000000000"} {
		if value, err := provider.GetSnapshot(context.Background(), invalid, dataQualityTestActor); value != nil || !errors.Is(err, ErrDataQualityScope) || called {
			t.Fatalf("noncanonical principal reached store: %q value=%#v error=%v", invalid, value, err)
		}
	}
}

func TestDataQualityQueryBudgetIsBoundedAndDoesNotReturnPartialCounts(t *testing.T) {
	var typedNil dataQualityStoreFunc
	for _, test := range []struct {
		store   DataQualityStore
		timeout time.Duration
	}{
		{nil, time.Second}, {typedNil, time.Second}, {dataQualityStoreFunc(func(context.Context, string, string) (*models.DataQualityCounts, error) { return nil, nil }), 0},
		{dataQualityStoreFunc(func(context.Context, string, string) (*models.DataQualityCounts, error) { return nil, nil }), 2*time.Second + time.Nanosecond},
	} {
		if value, err := NewDataQualityServiceWithTimeout(test.store, test.timeout); value != nil || err == nil {
			t.Fatalf("invalid budget/store accepted: %#v error=%v", value, err)
		}
	}
	provider, err := NewDataQualityServiceWithTimeout(dataQualityStoreFunc(func(ctx context.Context, _, _ string) (*models.DataQualityCounts, error) {
		<-ctx.Done()
		return dataQualityServiceCounts(), nil // A late full-looking result is still unavailable.
	}), 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := provider.GetSnapshot(context.Background(), dataQualityTestOrganization, dataQualityTestActor); value != nil || !errors.Is(err, ErrDataQualityUnavailable) {
		t.Fatalf("deadline produced healthy/partial data: %#v error=%v", value, err)
	}
}

func TestValidateDataQualitySnapshotRejectsInjectedDefinitionsAndDerivedStatus(t *testing.T) {
	provider, _ := NewDataQualityService(dataQualityStoreFunc(func(context.Context, string, string) (*models.DataQualityCounts, error) {
		return dataQualityServiceCounts(), nil
	}))
	for _, test := range []struct {
		name   string
		mutate func(*models.DataQualitySnapshot)
	}{
		{"injected definition", func(value *models.DataQualitySnapshot) { value.Checks[0].Definition = "private record message" }},
		{"reordered check", func(value *models.DataQualitySnapshot) {
			value.Checks[0], value.Checks[1] = value.Checks[1], value.Checks[0]
		}},
		{"false healthy", func(value *models.DataQualitySnapshot) {
			value.Checks[0].Count = 1
			value.Checks[0].Status = models.DataQualityCritical
		}},
		{"false check status", func(value *models.DataQualitySnapshot) { value.Checks[0].Status = models.DataQualityWarning }},
		{"unknown overall status", func(value *models.DataQualitySnapshot) { value.Status = "unknown" }},
		{"unknown scope", func(value *models.DataQualitySnapshot) { value.Scope = "global" }},
		{"wrong ruleset", func(value *models.DataQualitySnapshot) { value.RulesetVersion++ }},
		{"non-UTC clock", func(value *models.DataQualitySnapshot) { value.AsOf = value.AsOf.In(time.FixedZone("offset", 3600)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := provider.GetSnapshot(context.Background(), dataQualityTestOrganization, dataQualityTestActor)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(value)
			if ValidateDataQualitySnapshot(value) {
				t.Fatalf("unsafe typed provider result validated: %#v", value)
			}
		})
	}
	if ValidateDataQualitySnapshot(nil) {
		t.Fatal("nil snapshot validated")
	}
}
