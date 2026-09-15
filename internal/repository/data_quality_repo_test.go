package repository

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const (
	dataQualityRepoOrganization = "a2000000-0000-0000-0000-000000000001"
	dataQualityRepoActor        = "a2000000-0000-0000-0000-000000000002"
)

type dataQualityRowFunc func(...any) error

func (f dataQualityRowFunc) Scan(destinations ...any) error { return f(destinations...) }

type dataQualityQuerierStub struct {
	testing *testing.T
	row     pgx.Row
	queries int
	args    []any
}

func (q *dataQualityQuerierStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	q.testing.Fatal("read-only repository attempted Exec")
	return pgconn.CommandTag{}, nil
}

func (q *dataQualityQuerierStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	q.testing.Fatal("fixed-count repository attempted an unbounded row query")
	return nil, nil
}

func (q *dataQualityQuerierStub) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	q.queries++
	q.args = append([]any(nil), args...)
	if query != dataQualityCountsQuery {
		q.testing.Fatal("repository did not execute its reviewed fixed query")
	}
	return q.row
}

func dataQualityRepoRow(allowed, ready bool, counts []int64) pgx.Row {
	return dataQualityRowFunc(func(destinations ...any) error {
		if len(destinations) != 5 {
			return errors.New("unexpected projection")
		}
		*destinations[0].(*time.Time) = time.Date(2026, 9, 15, 1, 2, 3, 0, time.UTC)
		*destinations[1].(*bool) = allowed
		*destinations[2].(*bool) = ready
		*destinations[3].(*bool) = true
		*destinations[4].(*[]int64) = append([]int64(nil), counts...)
		return nil
	})
}

func TestDataQualityRepositoryUsesOnlyBoundRequestQuerierAndExactProjection(t *testing.T) {
	counts := make([]int64, 18)
	counts[0] = 2
	counts[17] = models.MaximumDataQualityCount
	querier := &dataQualityQuerierStub{testing: t, row: dataQualityRepoRow(true, true, counts)}
	ctx := database.WithQuerier(context.Background(), querier)
	value, err := NewDataQualityRepository().LoadDataQualityCounts(ctx, dataQualityRepoOrganization, dataQualityRepoActor)
	if err != nil || value == nil || len(value.Counts) != 18 || querier.queries != 1 {
		t.Fatalf("fixed read failed: %#v error=%v queries=%d", value, err, querier.queries)
	}
	if !reflect.DeepEqual(querier.args, []any{dataQualityRepoOrganization, dataQualityRepoActor, database.SupportedSchemaVersion}) {
		t.Fatalf("tenant/actor/schema were not bound parameters: %#v", querier.args)
	}
	for index, definition := range models.DataQualityCheckDefinitions() {
		if value.Counts[index].Key != definition.Key || value.Counts[index].Count != counts[index] {
			t.Fatalf("fixed projection drift at %d: %#v", index, value.Counts[index])
		}
	}
	if value.OrganizationID != dataQualityRepoOrganization || value.ActorID != dataQualityRepoActor || value.SchemaVersion != database.SupportedSchemaVersion {
		t.Fatalf("store result lost scope binding: %#v", value)
	}
}

func TestDataQualityRepositoryFailsClosedWithoutTenantConnectionOrScope(t *testing.T) {
	repo := NewDataQualityRepository()
	if value, err := repo.LoadDataQualityCounts(context.Background(), dataQualityRepoOrganization, dataQualityRepoActor); value != nil || !errors.Is(err, models.ErrDataQualityUnavailable) {
		t.Fatalf("unbound query succeeded: %#v error=%v", value, err)
	}
	querier := &dataQualityQuerierStub{testing: t, row: dataQualityRepoRow(true, true, make([]int64, 18))}
	ctx := database.WithQuerier(context.Background(), querier)
	for _, invalid := range []string{"", "00000000-0000-0000-0000-000000000000", strings.ToUpper(dataQualityRepoOrganization), "not-a-uuid"} {
		if value, err := repo.LoadDataQualityCounts(ctx, invalid, dataQualityRepoActor); value != nil || !errors.Is(err, models.ErrDataQualityScope) || querier.queries != 0 {
			t.Fatalf("invalid scope queried: %q value=%#v error=%v", invalid, value, err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if value, err := repo.LoadDataQualityCounts(canceled, dataQualityRepoOrganization, dataQualityRepoActor); value != nil || !errors.Is(err, models.ErrDataQualityUnavailable) || querier.queries != 0 {
		t.Fatalf("canceled repository request queried: %#v error=%v", value, err)
	}
}

func TestDataQualityRepositoryRejectsDeniedDirtyOrIncompleteProjection(t *testing.T) {
	for _, test := range []struct {
		name   string
		row    pgx.Row
		wanted error
	}{
		{"inactive or wrong actor", dataQualityRepoRow(false, true, nil), models.ErrDataQualityScope},
		{"wrong or dirty schema", dataQualityRepoRow(true, false, nil), models.ErrDataQualityUnavailable},
		{"retention-hidden framework controls", dataQualityRowFunc(func(destinations ...any) error {
			if err := dataQualityRepoRow(true, true, nil).Scan(destinations...); err != nil {
				return err
			}
			*destinations[3].(*bool) = false
			return nil
		}), models.ErrDataQualityUnavailable},
		{"query error", dataQualityRowFunc(func(...any) error { return errors.New("private SQL credential") }), models.ErrDataQualityUnavailable},
		{"missing projection", dataQualityRepoRow(true, true, make([]int64, 17)), nil},
		{"extra projection", dataQualityRepoRow(true, true, make([]int64, 19)), nil},
		{"negative count", dataQualityRepoRow(true, true, append([]int64{-1}, make([]int64, 17)...)), models.ErrDataQualityUnavailable},
		{"unsafe JSON count", dataQualityRepoRow(true, true, append([]int64{models.MaximumDataQualityCount + 1}, make([]int64, 17)...)), models.ErrDataQualityUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			querier := &dataQualityQuerierStub{testing: t, row: test.row}
			value, err := NewDataQualityRepository().LoadDataQualityCounts(database.WithQuerier(context.Background(), querier), dataQualityRepoOrganization, dataQualityRepoActor)
			if value != nil || err == nil || (test.wanted != nil && !errors.Is(err, test.wanted)) || strings.Contains(err.Error(), "credential") {
				t.Fatalf("unsafe repository result survived: %#v error=%v", value, err)
			}
		})
	}
}

func TestDataQualityFixedSQLHasNoRepairSamplingOrHistoricalLifecycleFilters(t *testing.T) {
	if regexp.MustCompile(`(?i)\b(insert|update|delete|truncate|create|alter|drop|limit|offset)\b`).MatchString(dataQualityCountsQuery) || strings.Contains(dataQualityCountsQuery, ";") {
		t.Fatal("fixed counts query contains a write, sampling clause, or multiple statements")
	}
	if strings.Count(dataQualityCountsQuery, "statement_timestamp()") != 1 || strings.Count(dataQualityCountsQuery, "SELECT COUNT(*)") != 19 {
		t.Fatal("fixed projection no longer shares one clock or eighteen complete counts")
	}
	for _, required := range []string{
		"get_current_tenant()=$1::uuid", "actor.id=$2::uuid", "organization.status='active'", "actor.status='active'",
		"version=$3::bigint AND NOT dirty", "parent.organization_id=child.organization_id",
		"child.risk_id IS NOT NULL", "child.linked_vendor_id IS NOT NULL", "child.parent_notification_id IS NOT NULL",
		"parent.organization_id IS NULL AND parent.is_system_role", "child.retry_count < child.max_retries",
		"COALESCE(child.next_retry_at,child.scheduled_for) <= scope.as_of", "subject.deprovisioned_at IS NOT NULL",
	} {
		if !strings.Contains(dataQualityCountsQuery, required) {
			t.Fatalf("reviewed fixed query lost %q", required)
		}
	}
	structural := strings.Split(dataQualityCountsQuery, "(SELECT COUNT(*) FROM notifications child\n            WHERE child.organization_id=$1::uuid AND child.channel_type='email'")[0]
	for _, forbidden := range []string{"parent.deleted_at", "parent.status", "child.deleted_at", "child.status"} {
		if strings.Contains(structural, forbidden) {
			t.Fatalf("retained historical records would be mislabelled by %q", forbidden)
		}
	}
}
