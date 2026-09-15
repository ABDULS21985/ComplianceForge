package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const profileRepoOrg = "20000000-aaaa-4000-8000-000000000001"
const profileRepoActor = "30000000-aaaa-4000-8000-000000000001"

type profileRepositoryQuerierSpy struct {
	calls    int
	rowError error
}

func (q *profileRepositoryQuerierSpy) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	q.calls++
	return pgconn.CommandTag{}, q.rowError
}
func (q *profileRepositoryQuerierSpy) Query(context.Context, string, ...any) (pgx.Rows, error) {
	q.calls++
	return nil, q.rowError
}
func (q *profileRepositoryQuerierSpy) QueryRow(context.Context, string, ...any) pgx.Row {
	q.calls++
	return profileRepositoryErrorRow{q.rowError}
}

type profileRepositoryErrorRow struct{ err error }

func (r profileRepositoryErrorRow) Scan(...any) error { return r.err }

func TestOrganizationProfileRepositoryRequiresScopeAndDatabase(t *testing.T) {
	if _, err := NewOrganizationProfileRepository(nil); err == nil {
		t.Fatal("missing database accepted")
	}
	repo, err := NewOrganizationProfileRepository(new(pgxpool.Pool))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetOrganizationProfile(context.Background(), profileRepoOrg, profileRepoActor); !errors.Is(err, models.ErrOrganizationProfileScope) {
		t.Fatal("unscoped pool fallback accepted")
	}
	for _, test := range []struct{ org, actor string }{
		{"", profileRepoActor}, {profileRepoOrg, ""}, {strings.ToUpper(profileRepoOrg), profileRepoActor}, {profileRepoOrg, "urn:uuid:" + profileRepoActor},
	} {
		spy := &profileRepositoryQuerierSpy{}
		ctx := database.WithQuerier(context.Background(), spy)
		_, err := repo.GetOrganizationProfile(ctx, test.org, test.actor)
		if !errors.Is(err, models.ErrOrganizationProfileScope) || spy.calls != 0 {
			t.Fatal("invalid scope reached SQL")
		}
	}
}

func TestOrganizationProfileRepositoryMasksDatabaseErrors(t *testing.T) {
	repo, _ := NewOrganizationProfileRepository(new(pgxpool.Pool))
	for _, test := range []struct{ source, want error }{
		{pgx.ErrNoRows, models.ErrOrganizationProfileNotFound}, {errors.New("password=secret; host=internal"), models.ErrOrganizationProfileUnavailable},
	} {
		spy := &profileRepositoryQuerierSpy{rowError: test.source}
		result, err := repo.GetOrganizationProfile(database.WithQuerier(context.Background(), spy), profileRepoOrg, profileRepoActor)
		if result != nil || !errors.Is(err, test.want) || spy.calls != 1 || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe repository result: %v", err)
		}
	}
}

func TestOrganizationProfileRepositoryRejectsUnsafeMutationsBeforeSQL(t *testing.T) {
	repo, _ := NewOrganizationProfileRepository(new(pgxpool.Pool))
	for _, test := range []struct {
		name            string
		update          func(*models.OrganizationProfile)
		request, reason string
	}{
		{"wrong organization", func(p *models.OrganizationProfile) { p.ID = "other" }, "", "Reviewed"},
		{"missing version", func(p *models.OrganizationProfile) { p.Version = 0 }, "", "Reviewed"},
		{"invalid request", func(*models.OrganizationProfile) {}, "internal-provider-secret", "Reviewed"},
		{"missing reason", func(*models.OrganizationProfile) {}, "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			spy := &profileRepositoryQuerierSpy{}
			candidate := models.OrganizationProfile{ID: profileRepoOrg, Version: 1}
			test.update(&candidate)
			_, err := repo.UpdateOrganizationProfile(database.WithQuerier(context.Background(), spy), profileRepoOrg, profileRepoActor, test.request, candidate, test.reason)
			if !errors.Is(err, models.ErrOrganizationProfileScope) || spy.calls != 0 {
				t.Fatalf("unsafe mutation reached SQL: %v", err)
			}
		})
	}
	spy := &profileRepositoryQuerierSpy{}
	_, err := repo.UpdateOrganizationProfile(database.WithQuerier(context.Background(), spy), profileRepoOrg, profileRepoActor, "", models.OrganizationProfile{ID: profileRepoOrg, Version: 1}, "Reviewed")
	if !errors.Is(err, models.ErrOrganizationProfileUnavailable) || spy.calls != 0 {
		t.Fatal("nontransactional executor accepted")
	}
}

func TestOrganizationProfileAuditContainsFieldLabelsNotValues(t *testing.T) {
	previous := models.OrganizationProfile{Name: "Before", Timezone: "UTC", Contacts: []models.OrganizationContact{}, SupportedLanguages: []string{"en"}, FiscalYearStartMonth: 1, FiscalYearStartDay: 1}
	current := previous
	current.Name = "Confidential business name"
	current.Timezone = "Africa/Lagos"
	current.Contacts = []models.OrganizationContact{{Purpose: "security", Email: "private@example.com"}}
	fields := changedOrganizationProfileFields(previous, current)
	if strings.Join(fields, ",") != "name,timezone,contacts" {
		t.Fatalf("unexpected audit labels: %v", fields)
	}
	for _, forbidden := range []string{"Confidential", "private@example.com", "Africa/Lagos"} {
		if strings.Contains(strings.Join(fields, ","), forbidden) {
			t.Fatal("audit labels duplicated profile values")
		}
	}
	for _, forbidden := range []string{"settings", "branding", "metadata", "parent_organization_id", "tax_id"} {
		if strings.Contains(organizationProfileColumns, forbidden) {
			t.Fatalf("unreviewed read projection: %s", forbidden)
		}
	}
}
