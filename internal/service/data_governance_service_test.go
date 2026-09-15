package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const (
	governanceTestOrg      = "73000000-0000-0000-0000-000000000001"
	governanceTestActor    = "73000000-0000-0000-0000-000000000002"
	governanceTestSchedule = "73000000-0000-0000-0000-000000000003"
	governanceTestRecord   = "73000000-0000-0000-0000-000000000004"
	governanceTestHold     = "73000000-0000-0000-0000-000000000005"
	governanceTestOther    = "73000000-0000-0000-0000-000000000006"
)

type dataGovernanceStoreStub struct {
	DataGovernanceStore
	policyInput     models.DataGovernancePolicyInput
	policyErr       error
	scheduleInput   models.RetentionScheduleInput
	scheduleFilter  models.RetentionScheduleFilter
	assignment      models.RetentionAssignmentInput
	holdInput       models.LegalHoldInput
	holdStatus      string
	holdPagination  models.PaginationRequest
	eventFilter     models.DataGovernanceEventFilter
	currentSchedule *models.RetentionSchedule
}

func (s *dataGovernanceStoreStub) GetPolicy(context.Context, string) (*models.DataGovernancePolicy, error) {
	if s.policyErr != nil {
		return nil, s.policyErr
	}
	return &models.DataGovernancePolicy{OrganizationID: governanceTestOrg, Version: 1}, nil
}

func (s *dataGovernanceStoreStub) UpsertPolicy(_ context.Context, organizationID, _, _ string, input models.DataGovernancePolicyInput) (*models.DataGovernancePolicy, error) {
	s.policyInput = input
	return &models.DataGovernancePolicy{OrganizationID: organizationID, AllowedRegions: input.AllowedRegions, Version: 1}, nil
}

func (s *dataGovernanceStoreStub) CreateSchedule(_ context.Context, organizationID, _, _ string, input models.RetentionScheduleInput) (*models.RetentionSchedule, error) {
	s.scheduleInput = input
	return &models.RetentionSchedule{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: governanceTestSchedule}, OrganizationID: organizationID}, Name: input.Name, RecordType: input.RecordType, TriggerEvent: input.TriggerEvent, LegalBasis: input.LegalBasis, RetentionDays: input.RetentionDays, DispositionAction: input.DispositionAction, Priority: input.Priority, Status: input.Status, EffectiveFrom: input.EffectiveFrom, Version: 1}, nil
}

func (s *dataGovernanceStoreStub) GetSchedule(context.Context, string, string) (*models.RetentionSchedule, error) {
	if s.currentSchedule == nil {
		return nil, repository.ErrDataGovernanceNotFound
	}
	copy := *s.currentSchedule
	return &copy, nil
}

func (s *dataGovernanceStoreStub) ListSchedules(_ context.Context, _ string, filter models.RetentionScheduleFilter) ([]models.RetentionSchedule, int, error) {
	s.scheduleFilter = filter
	return []models.RetentionSchedule{}, 0, nil
}

func (s *dataGovernanceStoreStub) CreateAssignment(_ context.Context, organizationID, _, _ string, input models.RetentionAssignmentInput) (*models.RecordRetentionAssignment, error) {
	s.assignment = input
	return &models.RecordRetentionAssignment{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: governanceTestRecord}, OrganizationID: organizationID}, ScheduleID: input.ScheduleID, RecordType: input.RecordType, RecordID: input.RecordID, Source: input.Source, Version: 1}, nil
}

func (s *dataGovernanceStoreStub) CreateLegalHold(_ context.Context, organizationID, _, _ string, input models.LegalHoldInput) (*models.LegalHold, error) {
	s.holdInput = input
	return &models.LegalHold{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: governanceTestHold}, OrganizationID: organizationID}, HoldRef: "LH-000001", Name: input.Name, OwnerUserID: input.OwnerUserID, CustodianIDs: input.CustodianIDs, Version: 1}, nil
}

func (s *dataGovernanceStoreStub) ListLegalHolds(_ context.Context, _ string, status string, pagination models.PaginationRequest) ([]models.LegalHold, int, error) {
	s.holdStatus, s.holdPagination = status, pagination
	return []models.LegalHold{}, 0, nil
}

func (s *dataGovernanceStoreStub) ListEvents(_ context.Context, _ string, filter models.DataGovernanceEventFilter) ([]models.DataGovernanceEvent, int, error) {
	s.eventFilter = filter
	return []models.DataGovernanceEvent{}, 0, nil
}

func newDataGovernanceTestService(store DataGovernanceStore, now time.Time) *DataGovernanceService {
	return NewDataGovernanceServiceWithClock(store, zerolog.Nop(), func() time.Time { return now })
}

func TestDataGovernancePolicyNormalizesResidencyAndMetadata(t *testing.T) {
	store := &dataGovernanceStoreStub{}
	svc := newDataGovernanceTestService(store, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
	item, err := svc.UpsertPolicy(context.Background(), governanceTestOrg, governanceTestActor, "request-1", models.DataGovernancePolicyInput{
		PrimaryRegion: " EU-WEST-1 ", AllowedRegions: []string{" US-EAST-1 ", "eu-west-1", "EU-WEST-1", ""},
		CrossBorderTransferMode: " APPROVED_REGIONS ", DefaultRetentionDays: 365,
		DeletionGraceDays: 14, DispositionApprovalMode: " DUAL ", PolicyStatement: "  Regional storage policy  ",
		Reason: "  Establish data residency  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Version != 1 || store.policyInput.PrimaryRegion != "eu-west-1" ||
		!reflect.DeepEqual(store.policyInput.AllowedRegions, []string{"eu-west-1", "us-east-1"}) ||
		store.policyInput.CrossBorderTransferMode != "approved_regions" ||
		store.policyInput.DispositionApprovalMode != "dual" || store.policyInput.PolicyStatement != "Regional storage policy" ||
		store.policyInput.Reason != "Establish data residency" || string(store.policyInput.Metadata) != "{}" {
		t.Fatalf("item=%#v normalized=%#v", item, store.policyInput)
	}
	_, err = svc.UpsertPolicy(context.Background(), governanceTestOrg, governanceTestActor, "request-2", models.DataGovernancePolicyInput{
		PrimaryRegion: "eu-west-1", AllowedRegions: []string{"us-east-1"}, CrossBorderTransferMode: "prohibited",
		DefaultRetentionDays: 365, DispositionApprovalMode: "single", Reason: "Invalid primary region",
	})
	if !errors.Is(err, ErrDataGovernanceInvalid) {
		t.Fatalf("missing-primary error=%v", err)
	}
}

func TestDataGovernanceScheduleDefaultsAndBoundsFilters(t *testing.T) {
	fixed := time.Date(2026, 9, 14, 18, 30, 0, 0, time.UTC)
	store := &dataGovernanceStoreStub{}
	svc := newDataGovernanceTestService(store, fixed)
	item, err := svc.CreateSchedule(context.Background(), governanceTestOrg, governanceTestActor, "request-3", models.RetentionScheduleInput{
		Name: "  Incident records  ", RecordType: " Incident ", LegalBasis: "Regulatory obligation",
		RetentionDays: 2555, Reason: "  Establish incident retention  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedDate := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	if item.TriggerEvent != "record_created" || item.DispositionAction != "review" || item.Priority != 100 ||
		item.Status != "draft" || !item.EffectiveFrom.Equal(expectedDate) || store.scheduleInput.Name != "Incident records" {
		t.Fatalf("item=%#v input=%#v", item, store.scheduleInput)
	}
	_, _, err = svc.ListSchedules(context.Background(), governanceTestOrg, models.RetentionScheduleFilter{
		PaginationRequest: models.PaginationRequest{Page: -1, PageSize: 500}, RecordType: "Audit Finding",
		Status: " ACTIVE ", DataClassification: " CONFIDENTIAL ", Jurisdiction: " ng ", Search: "  regulator  ",
		SortBy: " UPDATED_AT ", SortDirection: " DESC ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.scheduleFilter.Page != 1 || store.scheduleFilter.PageSize != 100 || store.scheduleFilter.RecordType != "audit_finding" ||
		store.scheduleFilter.Status != "active" || store.scheduleFilter.DataClassification != "confidential" ||
		store.scheduleFilter.Jurisdiction != "NG" || store.scheduleFilter.Search != "regulator" ||
		store.scheduleFilter.SortBy != "updated_at" || store.scheduleFilter.SortDirection != "desc" {
		t.Fatalf("filter=%#v", store.scheduleFilter)
	}
}

func TestDataGovernanceAssignmentsNormalizeAndRejectFutureStarts(t *testing.T) {
	fixed := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	store := &dataGovernanceStoreStub{}
	svc := newDataGovernanceTestService(store, fixed)
	_, err := svc.CreateAssignment(context.Background(), governanceTestOrg, governanceTestActor, "request-4", models.RetentionAssignmentInput{
		ScheduleID: governanceTestSchedule, RecordType: "Audit Finding", RecordID: governanceTestRecord,
		DataClassification: " CONFIDENTIAL ", Jurisdiction: " ng ", RetentionStartedAt: fixed, Reason: " Assign retention schedule ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.assignment.RecordType != "audit_finding" || store.assignment.DataClassification != "confidential" ||
		store.assignment.Jurisdiction != "NG" || store.assignment.Source != "manual" || store.assignment.Reason != "Assign retention schedule" {
		t.Fatalf("assignment=%#v", store.assignment)
	}
	_, err = svc.CreateAssignment(context.Background(), governanceTestOrg, governanceTestActor, "request-5", models.RetentionAssignmentInput{
		ScheduleID: governanceTestSchedule, RecordType: "risk", RecordID: governanceTestRecord,
		RetentionStartedAt: fixed.Add(6 * time.Minute), Reason: "Future assignment",
	})
	if !errors.Is(err, ErrDataGovernanceInvalid) {
		t.Fatalf("future-start error=%v", err)
	}
}

func TestDataGovernanceLegalHoldNormalizesCustodiansAndPagination(t *testing.T) {
	fixed := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	store := &dataGovernanceStoreStub{}
	svc := newDataGovernanceTestService(store, fixed)
	item, err := svc.CreateLegalHold(context.Background(), governanceTestOrg, governanceTestActor, "request-6", models.LegalHoldInput{
		Name: "  Regulator preservation  ", Description: "Preserve records for inquiry", LegalAuthority: "Regulator notice",
		OwnerUserID: governanceTestActor, CustodianIDs: []string{governanceTestOther, governanceTestActor, governanceTestOther},
		Scope: json.RawMessage(`{"record_types":["risk"]}`), Reason: " Preserve responsive records ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.HoldRef == "" || store.holdInput.Name != "Regulator preservation" ||
		!reflect.DeepEqual(store.holdInput.CustodianIDs, []string{governanceTestActor, governanceTestOther}) ||
		store.holdInput.Reason != "Preserve responsive records" {
		t.Fatalf("hold=%#v input=%#v", item, store.holdInput)
	}
	_, _, err = svc.ListLegalHolds(context.Background(), governanceTestOrg, " ACTIVE ", models.PaginationRequest{Page: 0, PageSize: 1000})
	if err != nil || store.holdStatus != "active" || store.holdPagination.Page != 1 || store.holdPagination.PageSize != 100 {
		t.Fatalf("status=%q pagination=%#v err=%v", store.holdStatus, store.holdPagination, err)
	}
	_, _, err = svc.ListLegalHolds(context.Background(), governanceTestOrg, "paused", models.PaginationRequest{})
	if !errors.Is(err, ErrDataGovernanceInvalid) {
		t.Fatalf("invalid-status error=%v", err)
	}
}

func TestDataGovernanceErrorsAndEventFiltersAreSafe(t *testing.T) {
	store := &dataGovernanceStoreStub{policyErr: repository.ErrDataGovernanceConflict}
	svc := newDataGovernanceTestService(store, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
	_, err := svc.GetPolicy(context.Background(), governanceTestOrg)
	if !errors.Is(err, ErrDataGovernanceConflict) {
		t.Fatalf("mapped conflict=%v", err)
	}
	_, _, err = svc.ListEvents(context.Background(), governanceTestOrg, models.DataGovernanceEventFilter{
		PaginationRequest: models.PaginationRequest{Page: 0, PageSize: 101}, EntityType: " LEGAL_HOLD ",
		EntityID: governanceTestHold, EventType: " HOLD_RELEASED ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.eventFilter.Page != 1 || store.eventFilter.PageSize != 100 || store.eventFilter.EntityType != "legal_hold" ||
		store.eventFilter.EntityID != governanceTestHold || store.eventFilter.EventType != "hold_released" {
		t.Fatalf("event filter=%#v", store.eventFilter)
	}
	_, _, err = svc.ListEvents(context.Background(), governanceTestOrg, models.DataGovernanceEventFilter{EntityType: "unknown"})
	if !errors.Is(err, ErrDataGovernanceInvalid) {
		t.Fatalf("invalid-entity error=%v", err)
	}
}
