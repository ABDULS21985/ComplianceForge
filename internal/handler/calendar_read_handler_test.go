package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type calendarHandlerServiceStub struct {
	CalendarReadService
	err   error
	query models.CalendarEventQuery
	calls int
}

func (stub *calendarHandlerServiceStub) ListEvents(_ context.Context, _, _ string, query models.CalendarEventQuery, _ models.CalendarAccessContext) (*models.CalendarEventPage, error) {
	stub.calls++
	stub.query = query
	return &models.CalendarEventPage{Data: []models.CalendarEventView{}}, stub.err
}
func calendarHandlerRequest(target string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	ctx := context.WithValue(r.Context(), middleware.ContextKeyOrgID, "c1000000-0000-4000-8000-000000000001")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "c1000000-0000-4000-8000-000000000002")
	ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true})
	return r.WithContext(ctx)
}
func TestCalendarReadHandlerStrictQueriesAndSafeErrors(t *testing.T) {
	for _, query := range []string{"?page=1&page=2", "?page=0", "?unknown=secret", "?event_type=", "?start_date=2026-09-01&from_date=2026-09-02", "?days_ahead=20", "?search=bad;unknown=secret"} {
		stub := &calendarHandlerServiceStub{}
		w := httptest.NewRecorder()
		NewCalendarReadHandler(stub).ListEvents(w, calendarHandlerRequest("/calendar/events"+query))
		if w.Code != 400 || stub.calls != 0 {
			t.Fatalf("ambiguous query accepted: %s status=%d", query, w.Code)
		}
	}
	stub := &calendarHandlerServiceStub{}
	w := httptest.NewRecorder()
	NewCalendarReadHandler(stub).ListEvents(w, calendarHandlerRequest("/calendar/events?from_date=2026-09-01&to_date=2026-09-30"))
	if w.Code != 200 || stub.query.StartDate != "2026-09-01" || stub.query.EndDate != "2026-09-30" {
		t.Fatalf("typed aliases: status=%d body=%s", w.Code, w.Body.String())
	}
	for _, test := range []struct {
		err    error
		status int
	}{{service.ErrComplianceCalendarInvalid, 400}, {service.ErrComplianceCalendarScope, 403}, {service.ErrComplianceCalendarUnavailable, 503}} {
		stub.err = test.err
		w = httptest.NewRecorder()
		NewCalendarReadHandler(stub).ListEvents(w, calendarHandlerRequest("/calendar/events"))
		if w.Code != test.status || strings.Contains(w.Body.String(), "SQL") {
			t.Fatal("unsafe error mapping")
		}
	}
}
func TestCalendarReadHandlerFailsClosedWithoutVerifiedIdentityAndDecision(t *testing.T) {
	h := NewCalendarReadHandler(&calendarHandlerServiceStub{})
	w := httptest.NewRecorder()
	h.ListEvents(w, httptest.NewRequest("GET", "/calendar/events", nil))
	if w.Code != 401 {
		t.Fatal("unauthenticated calendar admitted")
	}
	r := calendarHandlerRequest("/calendar/events")
	r = r.WithContext(middleware.ContextWithAuthorizationDecision(r.Context(), authz.Decision{Allowed: true, Obligations: []authz.Obligation{{Kind: "unknown"}}}))
	w = httptest.NewRecorder()
	h.ListEvents(w, r)
	if w.Code != 503 {
		t.Fatal("unsupported obligations admitted")
	}
	if NewCalendarReadHandler(nil).Ready() {
		t.Fatal("nil service ready")
	}
}

func TestCalendarReadHandlerRejectsTypedNilServices(t *testing.T) {
	var nilStub *calendarHandlerServiceStub
	var nilProductionService *service.ComplianceCalendarService
	for _, dependency := range []CalendarReadService{nilStub, nilProductionService} {
		h := NewCalendarReadHandler(dependency)
		if h.Ready() {
			t.Fatal("typed nil calendar service reported ready")
		}
		w := httptest.NewRecorder()
		h.ListEvents(w, calendarHandlerRequest("/calendar/events"))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("typed nil service did not fail closed: status=%d", w.Code)
		}
	}
}
