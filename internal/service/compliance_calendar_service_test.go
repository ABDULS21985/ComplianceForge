package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const calendarTestOrg = "c1000000-0000-4000-8000-000000000001"
const calendarTestUser = "c1000000-0000-4000-8000-000000000002"

type calendarStoreStub struct {
	items []models.CalendarEventView
	err   error
	query models.CalendarEventQuery
}

func (stub *calendarStoreStub) LoadCandidates(_ context.Context, _, _ string, query models.CalendarEventQuery) ([]models.CalendarEventView, error) {
	stub.query = query
	return append([]models.CalendarEventView(nil), stub.items...), stub.err
}

type calendarAuthorizerFunc func(context.Context, authz.Request) (authz.Decision, error)

func (fn calendarAuthorizerFunc) Authorize(ctx context.Context, request authz.Request) (authz.Decision, error) {
	return fn(ctx, request)
}

func calendarServiceFixture(t *testing.T, store *calendarStoreStub, authorizer authz.Authorizer) *ComplianceCalendarService {
	t.Helper()
	result, err := NewComplianceCalendarService(store, authorizer, WithComplianceCalendarClock(func() time.Time {
		return time.Date(2026, 9, 15, 12, 34, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func calendarEventFixture(id, date, resource string) models.CalendarEventView {
	return models.CalendarEventView{ID: id, OrganizationID: calendarTestOrg, SourceEntityID: id,
		AuthorizationResourceID: id, AuthorizationResource: resource, StartDate: date,
		EventType: "risk_review", Category: "risk", Priority: "high", Status: "upcoming", Title: "Reviewed source event"}
}

func TestComplianceCalendarFiltersBeforePaginationAndSummary(t *testing.T) {
	denied := calendarEventFixture("c2000000-0000-4000-8000-000000000001", "2026-09-01", "policies")
	allowed := calendarEventFixture("c2000000-0000-4000-8000-000000000002", "2026-09-16", "risks")
	second := allowed
	second.ID = "c2000000-0000-4000-8000-000000000003"
	second.StartDate = "2026-09-17"
	store := &calendarStoreStub{items: []models.CalendarEventView{second, denied, allowed}}
	requests := []authz.Request{}
	svc := calendarServiceFixture(t, store, calendarAuthorizerFunc(func(_ context.Context, request authz.Request) (authz.Decision, error) {
		requests = append(requests, request)
		return authz.Decision{Allowed: request.Resource == "risks"}, nil
	}))
	access := models.CalendarAccessContext{IPAddress: "192.0.2.1", MFAVerified: true, RequestID: "bounded-calendar-request"}
	page, err := svc.ListEvents(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{PageSize: 1}, access)
	if err != nil || page.Pagination.TotalItems != 2 || len(page.Data) != 1 || page.Data[0].ID != allowed.ID {
		t.Fatalf("filtering/pagination mismatch: page=%#v err=%v", page, err)
	}
	if len(requests) != 2 || requests[0].ResourceID == "" || !requests[0].MFAVerified || requests[0].IPAddress != access.IPAddress {
		t.Fatalf("source PDP identity or memoization mismatch: %#v", requests)
	}
	if store.query.StartDate != "2026-09-01" || store.query.EndDate != "2026-09-30" || page.Meta.SyncVerified || page.Meta.DateTimezone != "UTC" {
		t.Fatal("default month/provenance mismatch")
	}
	summary, err := svc.Summary(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, access)
	if err != nil || summary.Data.TotalEvents != 2 || summary.Data.UpcomingEvents != 2 || summary.Data.OverdueEvents != 0 || summary.Data.Days[0].Count != 0 {
		t.Fatalf("summary leaked excluded records: summary=%#v err=%v", summary, err)
	}
}

func TestComplianceCalendarSourceProjectionObligationsFailClosed(t *testing.T) {
	permission := models.AccessFieldPermission{ID: calendarTestUser, PolicyID: calendarTestOrg, ResourceType: "risks",
		FieldPath: "title", Classification: "confidential", Visibility: models.AccessFieldVisible}
	for _, kind := range []string{"visible", "hidden", "masked", "unknown", "malformed", "wrong-resource"} {
		t.Run(kind, func(t *testing.T) {
			field := permission
			if kind == "hidden" {
				field.Visibility = models.AccessFieldHidden
			}
			if kind == "masked" {
				field.Visibility = models.AccessFieldMasked
				field.MaskStrategy = "redact"
			}
			if kind == "wrong-resource" {
				field.ResourceType = "policies"
			}
			obligations := AccessFieldObligations([]models.AccessFieldPermission{field})
			if kind == "unknown" {
				obligations = []authz.Obligation{{Kind: "watermark"}}
			}
			if kind == "malformed" {
				obligations = []authz.Obligation{{Kind: AccessFieldVisibilityObligation}}
			}
			store := &calendarStoreStub{items: []models.CalendarEventView{calendarEventFixture(calendarTestUser, "2026-09-15", "risks")}}
			svc := calendarServiceFixture(t, store, calendarAuthorizerFunc(func(context.Context, authz.Request) (authz.Decision, error) {
				return authz.Decision{Allowed: true, Obligations: obligations}, nil
			}))
			page, err := svc.ListEvents(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if kind == "visible" {
				want = 1
			}
			if page.Pagination.TotalItems != want || len(page.Data) != want {
				t.Fatalf("%s source escaped projection filtering", kind)
			}
		})
	}
}

func TestComplianceCalendarValidatesCanonicalBounds(t *testing.T) {
	queries := []models.CalendarEventQuery{
		{StartDate: "2026-02-30", EndDate: "2026-03-01"}, {StartDate: "2026-09-01"},
		{StartDate: "2026-09-30", EndDate: "2026-09-01"}, {StartDate: "2025-01-01", EndDate: "2026-01-02"},
		{Status: "pending"}, {EventType: "audit_task"}, {Category: "raw_module"}, {Priority: "urgent"},
		{AssignedTo: strings.ToUpper(calendarTestUser)}, {AssignedTo: "urn:uuid:" + calendarTestUser},
		{SortBy: "title; DROP TABLE users"}, {Page: 100001}, {PageSize: 101}, {Page: -1},
		{Search: strings.Repeat("x", 201)}, {Search: " secret "}, {Search: "bad\x00query"},
	}
	store := &calendarStoreStub{}
	svc := calendarServiceFixture(t, store, calendarAuthorizerFunc(func(context.Context, authz.Request) (authz.Decision, error) {
		return authz.Decision{Allowed: true}, nil
	}))
	for _, query := range queries {
		if _, err := svc.ListEvents(context.Background(), calendarTestOrg, calendarTestUser, query, models.CalendarAccessContext{}); !errors.Is(err, ErrComplianceCalendarInvalid) {
			t.Fatalf("invalid query accepted: %#v err=%v", query, err)
		}
	}
	if _, err := svc.ListEvents(context.Background(), strings.ToUpper(calendarTestOrg), calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{}); !errors.Is(err, ErrComplianceCalendarInvalid) {
		t.Fatal("uppercase tenant accepted")
	}
}

func TestComplianceCalendarDerivedDateViewsAndSafeFailures(t *testing.T) {
	store := &calendarStoreStub{}
	svc := calendarServiceFixture(t, store, calendarAuthorizerFunc(func(context.Context, authz.Request) (authz.Decision, error) {
		return authz.Decision{Allowed: true}, nil
	}))
	if _, err := svc.Upcoming(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{}); err != nil || store.query.StartDate != "2026-09-15" || store.query.EndDate != "2026-10-15" {
		t.Fatal("upcoming window mismatch")
	}
	store.items = []models.CalendarEventView{calendarEventFixture(calendarTestUser, "2026-09-01", "risks")}
	page, err := svc.Overdue(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{})
	if err != nil || len(page.Data) != 1 || page.Data[0].Status != "overdue" {
		t.Fatal("overdue projection mismatch")
	}
	store.err = repository.ErrCalendarReadLimit
	if _, err = svc.ListEvents(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{}); !errors.Is(err, ErrComplianceCalendarUnavailable) {
		t.Fatal("candidate cap returned partial view")
	}
	store.err = errors.New("password=secret SQL details")
	if _, err = svc.Summary(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{}); !errors.Is(err, ErrComplianceCalendarUnavailable) || strings.Contains(err.Error(), "secret") {
		t.Fatal("store failure leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = svc.ListEvents(ctx, calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}

func TestComplianceCalendarConstructorAndAuthorizationCancellation(t *testing.T) {
	store := &calendarStoreStub{items: []models.CalendarEventView{calendarEventFixture(calendarTestUser, "2026-09-15", "risks")}}
	allow := calendarAuthorizerFunc(func(context.Context, authz.Request) (authz.Decision, error) {
		return authz.Decision{Allowed: true}, nil
	})
	if _, err := NewComplianceCalendarService(nil, allow); err == nil {
		t.Fatal("missing repository accepted")
	}
	if _, err := NewComplianceCalendarService(store, nil); err == nil {
		t.Fatal("missing composite authorizer accepted")
	}
	if _, err := NewComplianceCalendarService(store, allow, WithComplianceCalendarClock(nil)); err == nil {
		t.Fatal("missing clock accepted")
	}
	var typedNilStore *calendarStoreStub
	if _, err := NewComplianceCalendarService(typedNilStore, allow); err == nil {
		t.Fatal("typed nil repository accepted")
	}
	var typedNilAuthorizer calendarAuthorizerFunc
	if _, err := NewComplianceCalendarService(store, typedNilAuthorizer); err == nil {
		t.Fatal("typed nil authorizer accepted")
	}
	for _, incomplete := range []*ComplianceCalendarService{
		{store: typedNilStore, authorizer: allow, now: time.Now},
		{store: store, authorizer: typedNilAuthorizer, now: time.Now},
	} {
		if _, err := incomplete.ListEvents(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{}); !errors.Is(err, ErrComplianceCalendarUnavailable) {
			t.Fatalf("manually composed typed nil did not fail closed: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc := calendarServiceFixture(t, store, calendarAuthorizerFunc(func(context.Context, authz.Request) (authz.Decision, error) {
		cancel()
		return authz.Decision{}, context.Canceled
	}))
	if _, err := svc.ListEvents(ctx, calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("PDP cancellation was hidden: %v", err)
	}
	svc = calendarServiceFixture(t, store, allow)
	if _, err := svc.Upcoming(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{
		StartDate: "2026-09-15", EndDate: "2026-09-30", DaysAhead: 15,
	}, models.CalendarAccessContext{}); !errors.Is(err, ErrComplianceCalendarInvalid) {
		t.Fatal("ambiguous upcoming date window accepted")
	}
	store.items[0].Status = "pending"
	if _, err := svc.ListEvents(context.Background(), calendarTestOrg, calendarTestUser, models.CalendarEventQuery{}, models.CalendarAccessContext{}); !errors.Is(err, ErrComplianceCalendarUnavailable) {
		t.Fatal("noncanonical repository state accepted")
	}
}
