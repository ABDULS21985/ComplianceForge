package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const (
	incidentTestOrg  = "10000000-0000-0000-0000-000000000001"
	incidentTestUser = "20000000-0000-0000-0000-000000000001"
	incidentTestID   = "30000000-0000-0000-0000-000000000001"
)

type incidentServiceRepository struct {
	IncidentManagementRepository
	incident      *models.Incident
	createInput   models.IncidentCreateInput
	transition    models.IncidentTransitionInput
	assessment    models.IncidentBreachAssessmentInput
	notification  models.IncidentDPANotificationInput
	deleteCalled  bool
	transitionErr error
}

func cloneIncident(item *models.Incident) *models.Incident {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func (r *incidentServiceRepository) Create(_ context.Context, orgID, actorID string, input models.IncidentCreateInput) (*models.Incident, error) {
	r.createInput = input
	r.incident = &models.Incident{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: incidentTestID}, OrganizationID: orgID},
		IncidentRef: "INC-000001", Title: input.Title, Description: input.Description,
		Category: input.Category, Severity: input.Severity, Status: models.IncidentStatusReported,
		ReporterID: actorID, DetectedAt: input.DetectedAt, Version: 1, Metadata: input.Metadata,
	}
	return cloneIncident(r.incident), nil
}

func (r *incidentServiceRepository) GetByID(_ context.Context, orgID, id string) (*models.Incident, error) {
	if r.incident == nil || r.incident.OrganizationID != orgID || r.incident.ID != id {
		return nil, pgx.ErrNoRows
	}
	return cloneIncident(r.incident), nil
}

func (r *incidentServiceRepository) Transition(_ context.Context, _, _, _ string, input models.IncidentTransitionInput) (*models.Incident, error) {
	if r.transitionErr != nil {
		return nil, r.transitionErr
	}
	r.transition = input
	r.incident.Status = input.Status
	r.incident.Version++
	return cloneIncident(r.incident), nil
}

func (r *incidentServiceRepository) Escalate(_ context.Context, _, _, _ string, input models.IncidentEscalationInput) (*models.Incident, error) {
	r.incident.Severity = input.Severity
	r.incident.Version++
	return cloneIncident(r.incident), nil
}

func (r *incidentServiceRepository) AssessBreach(_ context.Context, _, actorID, _ string, input models.IncidentBreachAssessmentInput) (*models.Incident, error) {
	r.assessment = input
	r.incident.IsDataBreach = input.IsDataBreach
	r.incident.BreachAssessmentStatus = input.Status
	r.incident.IsBreachNotifiable = input.Status == models.BreachAssessmentNotifiable
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	r.incident.BreachAssessedAt, r.incident.BreachAssessedBy = &now, &actorID
	r.incident.BreachAwarenessAt = input.AwarenessAt
	if input.AwarenessAt != nil && r.incident.IsBreachNotifiable {
		deadline := input.AwarenessAt.Add(gdprBreachNotificationHours * time.Hour)
		r.incident.NotificationDeadline = &deadline
	}
	r.incident.Version++
	return cloneIncident(r.incident), nil
}

func (r *incidentServiceRepository) NotifyDPA(_ context.Context, _, _, _ string, input models.IncidentDPANotificationInput) (*models.Incident, error) {
	r.notification = input
	r.incident.DPANotifiedAt = &input.NotifiedAt
	reference, reason := input.Reference, input.Reason
	r.incident.DPANotificationReference, r.incident.DPANotificationReason = &reference, &reason
	r.incident.Version++
	return cloneIncident(r.incident), nil
}

func (r *incidentServiceRepository) Delete(context.Context, string, string, string, int64) error {
	r.deleteCalled = true
	return nil
}

func testIncident(status models.IncidentStatus) *models.Incident {
	detected := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	return &models.Incident{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: incidentTestID}, OrganizationID: incidentTestOrg},
		IncidentRef: "INC-000001", Title: "Database snapshot exposure",
		Description: "A production snapshot was accessible outside the approved boundary",
		Category:    "privacy", Severity: models.IncidentSeverityLow, Status: status,
		ReporterID: incidentTestUser, DetectedAt: &detected, Version: 1,
		Metadata: json.RawMessage(`{}`), DataCategories: []string{},
	}
}

func TestIncidentLifecycleStateMachine(t *testing.T) {
	statuses := []models.IncidentStatus{
		models.IncidentStatusReported, models.IncidentStatusTriaged,
		models.IncidentStatusInvestigating, models.IncidentStatusContained,
		models.IncidentStatusResolved, models.IncidentStatusClosed,
		models.IncidentStatusCancelled,
	}
	allowed := map[[2]models.IncidentStatus]bool{
		{models.IncidentStatusReported, models.IncidentStatusTriaged}:        true,
		{models.IncidentStatusReported, models.IncidentStatusCancelled}:      true,
		{models.IncidentStatusTriaged, models.IncidentStatusInvestigating}:   true,
		{models.IncidentStatusTriaged, models.IncidentStatusCancelled}:       true,
		{models.IncidentStatusInvestigating, models.IncidentStatusContained}: true,
		{models.IncidentStatusInvestigating, models.IncidentStatusResolved}:  true,
		{models.IncidentStatusInvestigating, models.IncidentStatusCancelled}: true,
		{models.IncidentStatusContained, models.IncidentStatusInvestigating}: true,
		{models.IncidentStatusContained, models.IncidentStatusResolved}:      true,
		{models.IncidentStatusContained, models.IncidentStatusCancelled}:     true,
		{models.IncidentStatusResolved, models.IncidentStatusClosed}:         true,
		{models.IncidentStatusResolved, models.IncidentStatusInvestigating}:  true,
		{models.IncidentStatusClosed, models.IncidentStatusInvestigating}:    true,
		{models.IncidentStatusCancelled, models.IncidentStatusReported}:      true,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			if got, want := isValidIncidentTransition(from, to), allowed[[2]models.IncidentStatus{from, to}]; got != want {
				t.Errorf("transition %s -> %s = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestIncidentServiceCreatesReportedIncidentFromTrustedIdentity(t *testing.T) {
	repository := &incidentServiceRepository{}
	service := NewIncidentService(repository, zerolog.Nop())
	service.now = func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) }
	item, err := service.Create(context.Background(), incidentTestOrg, incidentTestUser, models.IncidentCreateInput{
		Title: "  Snapshot exposure  ", Description: "  Production data was exposed  ",
		Category: " privacy ", Severity: "HIGH",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != models.IncidentStatusReported || item.ReporterID != incidentTestUser || item.Severity != models.IncidentSeverityHigh || item.Title != "Snapshot exposure" {
		t.Fatalf("created incident=%#v", item)
	}
	if repository.createInput.DetectedAt == nil || string(repository.createInput.Metadata) != `{}` {
		t.Fatalf("normalized create input=%#v", repository.createInput)
	}
}

func TestIncidentServiceRequiresJustifiedTerminalTransitions(t *testing.T) {
	repository := &incidentServiceRepository{incident: testIncident(models.IncidentStatusReported)}
	service := NewIncidentService(repository, zerolog.Nop())
	if _, err := service.Cancel(context.Background(), incidentTestOrg, incidentTestUser, incidentTestID, 1, ""); !errors.Is(err, ErrIncidentInvalid) {
		t.Fatalf("cancel without reason error=%v", err)
	}
	item, err := service.Cancel(context.Background(), incidentTestOrg, incidentTestUser, incidentTestID, 1, "Duplicate of INC-000002")
	if err != nil || item.Status != models.IncidentStatusCancelled {
		t.Fatalf("cancelled=%#v err=%v", item, err)
	}
	if _, err := service.Reopen(context.Background(), incidentTestOrg, incidentTestUser, incidentTestID, 2, "no"); !errors.Is(err, ErrIncidentInvalid) {
		t.Fatalf("reopen without adequate reason error=%v", err)
	}
}

func TestIncidentServiceCalculatesGDPRDeadlineAndValidatesDPAAction(t *testing.T) {
	repository := &incidentServiceRepository{incident: testIncident(models.IncidentStatusInvestigating)}
	service := NewIncidentService(repository, zerolog.Nop())
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	awareness := now.Add(-2 * time.Hour)
	item, err := service.AssessBreach(context.Background(), incidentTestOrg, incidentTestUser, incidentTestID, models.IncidentBreachAssessmentInput{
		Version: 1, Status: models.BreachAssessmentNotifiable, Reason: "Risk to affected data subjects is likely",
		IsDataBreach: true, AwarenessAt: &awareness, DataCategories: []string{"identity", "identity"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDeadline := awareness.Add(72 * time.Hour)
	if item.NotificationDeadline == nil || !item.NotificationDeadline.Equal(wantDeadline) || item.DeadlineState != "upcoming" {
		t.Fatalf("breach assessment=%#v", item)
	}
	if len(repository.assessment.DataCategories) != 1 {
		t.Fatalf("data categories were not normalized: %#v", repository.assessment.DataCategories)
	}
	_, err = service.NotifyDPA(context.Background(), incidentTestOrg, incidentTestUser, incidentTestID, models.IncidentDPANotificationInput{
		Version: 2, IdempotencyKey: "40000000-0000-0000-0000-000000000001",
		NotifiedAt: now, Reference: "DPA-2026-981", Reason: "Article 33 notification",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.notification.Reference != "DPA-2026-981" {
		t.Fatalf("notification=%#v", repository.notification)
	}
}

func TestIncidentServiceBlocksClosingUnnotifiedNotifiableBreach(t *testing.T) {
	item := testIncident(models.IncidentStatusResolved)
	item.LessonsLearned = "Validate snapshot access before restoring production data"
	item.IsDataBreach = true
	item.IsBreachNotifiable = true
	item.BreachAssessmentStatus = models.BreachAssessmentNotifiable
	repository := &incidentServiceRepository{incident: item}
	service := NewIncidentService(repository, zerolog.Nop())

	if _, err := service.Close(context.Background(), incidentTestOrg, incidentTestUser, incidentTestID, 1, "Response and notification duties completed"); !errors.Is(err, ErrIncidentConflict) {
		t.Fatalf("close before DPA notification error=%v", err)
	}
	if repository.transition.Status != "" {
		t.Fatalf("repository transition called despite missing DPA notification: %#v", repository.transition)
	}
}

func TestIncidentServiceBoundsListFilters(t *testing.T) {
	service := NewIncidentService(&incidentServiceRepository{}, zerolog.Nop())
	if _, _, err := service.List(context.Background(), incidentTestOrg, models.IncidentListFilter{Search: strings.Repeat("x", 501)}); !errors.Is(err, ErrIncidentInvalid) {
		t.Fatalf("oversized search error=%v", err)
	}
	if _, _, err := service.List(context.Background(), incidentTestOrg, models.IncidentListFilter{Category: strings.Repeat("x", 101)}); !errors.Is(err, ErrIncidentInvalid) {
		t.Fatalf("oversized category error=%v", err)
	}
}

func TestIncidentServiceEnforcesRetentionAndOptimisticConflicts(t *testing.T) {
	item := testIncident(models.IncidentStatusClosed)
	lessons := "Preserve snapshots only in approved storage"
	item.LessonsLearned = lessons
	future := time.Now().UTC().Add(24 * time.Hour)
	item.RetentionUntil = &future
	repository := &incidentServiceRepository{incident: item}
	service := NewIncidentService(repository, zerolog.Nop())
	if err := service.Delete(context.Background(), incidentTestOrg, incidentTestUser, incidentTestID, 1); !errors.Is(err, ErrIncidentConflict) {
		t.Fatalf("retention delete error=%v", err)
	}
	item.RetentionUntil = nil
	repository.transitionErr = repositorypkgVersionConflict()
	if _, err := service.Reopen(context.Background(), incidentTestOrg, incidentTestUser, incidentTestID, 1, "Additional evidence requires investigation"); !errors.Is(err, ErrIncidentVersionConflict) {
		t.Fatalf("optimistic conflict error=%v", err)
	}
}

func repositorypkgVersionConflict() error { return repository.ErrIncidentVersionConflict }
