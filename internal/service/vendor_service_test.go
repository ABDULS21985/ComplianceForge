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
	vendorTestOrg  = "51000000-0000-0000-0000-000000000001"
	vendorTestUser = "52000000-0000-0000-0000-000000000001"
	vendorTestID   = "53000000-0000-0000-0000-000000000001"
)

type vendorServiceRepositoryStub struct {
	VendorManagementRepository
	item            *models.Vendor
	createInput     models.VendorCreateInput
	transitionInput models.VendorTransitionInput
	assessmentInput models.VendorAssessmentInput
	deleteCalled    bool
	err             error
}

func (r *vendorServiceRepositoryStub) Create(_ context.Context, orgID, actorID string, input models.VendorCreateInput) (*models.Vendor, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.createInput = input
	r.item = vendorTestItem(models.VendorStatusProspective)
	r.item.OrganizationID, r.item.CreatedBy = orgID, actorID
	r.item.Name, r.item.Criticality, r.item.VendorTier, r.item.RiskTier = input.Name, input.Criticality, input.VendorTier, input.RiskTier
	r.item.Services, r.item.ServiceDescription, r.item.DataProcessing = input.Services, input.ServiceDescription, input.DataProcessing
	r.item.DataCategories, r.item.DPARequired, r.item.DPAStatus = input.DataCategories, input.DPARequired, input.DPAStatus
	r.item.DPAInPlace = input.DPAStatus == models.VendorDPAExecuted
	return cloneVendor(r.item), nil
}

func (r *vendorServiceRepositoryStub) GetByID(_ context.Context, orgID, id string) (*models.Vendor, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.item == nil || r.item.OrganizationID != orgID || r.item.ID != id {
		return nil, pgx.ErrNoRows
	}
	return cloneVendor(r.item), nil
}

func (r *vendorServiceRepositoryStub) Transition(_ context.Context, _, _, _ string, input models.VendorTransitionInput) (*models.Vendor, error) {
	r.transitionInput = input
	r.item.Status, r.item.Version = input.Status, r.item.Version+1
	return cloneVendor(r.item), r.err
}

func (r *vendorServiceRepositoryStub) RecordAssessment(_ context.Context, _, _, _ string, input models.VendorAssessmentInput) (*models.Vendor, error) {
	r.assessmentInput = input
	r.item.RiskTier, r.item.RiskScore, r.item.AssessmentStatus = input.RiskTier, input.RiskScore, input.Status
	r.item.LastAssessmentDate, r.item.NextAssessmentDate, r.item.Version = &input.AssessedAt, input.NextAssessmentDate, r.item.Version+1
	return cloneVendor(r.item), r.err
}

func (r *vendorServiceRepositoryStub) Delete(context.Context, string, string, string, int64) error {
	r.deleteCalled = true
	return r.err
}

func vendorTestItem(status models.VendorStatus) *models.Vendor {
	return &models.Vendor{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: vendorTestID}, OrganizationID: vendorTestOrg},
		VendorRef:   "VND-000001", Name: "Cloud processor", Criticality: models.VendorCriticalityHigh,
		VendorTier: models.VendorTierOne, RiskTier: models.VendorRiskHigh, Status: status,
		AssessmentFrequency: "annual", AssessmentCadenceDays: 365, AssessmentStatus: models.VendorAssessmentNotDue,
		Services: []string{"hosting"}, DataCategories: []string{}, ProcessingLocations: []string{},
		Metadata: json.RawMessage(`{}`), Version: 1, CreatedBy: vendorTestUser,
	}
}

func cloneVendor(item *models.Vendor) *models.Vendor {
	if item == nil {
		return nil
	}
	clone := *item
	return &clone
}

func TestVendorLifecycleStateMachine(t *testing.T) {
	statuses := []models.VendorStatus{models.VendorStatusProspective, models.VendorStatusOnboarding, models.VendorStatusActive,
		models.VendorStatusSuspended, models.VendorStatusOffboarding, models.VendorStatusOffboarded, models.VendorStatusRejected}
	allowed := map[[2]models.VendorStatus]bool{
		{models.VendorStatusProspective, models.VendorStatusOnboarding}: true, {models.VendorStatusProspective, models.VendorStatusRejected}: true,
		{models.VendorStatusOnboarding, models.VendorStatusActive}: true, {models.VendorStatusOnboarding, models.VendorStatusRejected}: true,
		{models.VendorStatusActive, models.VendorStatusSuspended}: true, {models.VendorStatusActive, models.VendorStatusOffboarding}: true,
		{models.VendorStatusSuspended, models.VendorStatusActive}: true, {models.VendorStatusSuspended, models.VendorStatusOffboarding}: true,
		{models.VendorStatusOffboarding, models.VendorStatusOffboarded}: true, {models.VendorStatusOffboarding, models.VendorStatusActive}: true,
		{models.VendorStatusOffboarded, models.VendorStatusOnboarding}: true, {models.VendorStatusRejected, models.VendorStatusOnboarding}: true,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			if got, want := isValidVendorTransition(from, to), allowed[[2]models.VendorStatus{from, to}]; got != want {
				t.Errorf("%s -> %s = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestVendorServiceNormalizesFrontendCreateContract(t *testing.T) {
	repo := &vendorServiceRepositoryStub{}
	svc := NewVendorService(repo, zerolog.Nop())
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	item, err := svc.Create(context.Background(), vendorTestOrg, vendorTestUser, models.VendorCreateInput{
		Name: "  Nimbus Hosting ", CountryCode: "ng", RiskTier: "HIGH", ContactName: " Ada Vendor ",
		ContactEmail: "ADA@EXAMPLE.TEST", ServiceDescription: " Managed hosting ", Services: []string{"Hosting", "hosting"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != models.VendorStatusProspective || repo.createInput.Name != "Nimbus Hosting" || repo.createInput.CountryCode != "NG" ||
		repo.createInput.RiskTier != models.VendorRiskHigh || repo.createInput.ContactEmail != "ada@example.test" || len(repo.createInput.Services) != 1 {
		t.Fatalf("normalized create=%#v item=%#v", repo.createInput, item)
	}
	if repo.createInput.NextAssessmentDate == nil || repo.createInput.AssessmentCadenceDays != 365 {
		t.Fatalf("assessment defaults=%#v", repo.createInput)
	}
}

func TestVendorActivationRequiresOwnerContactServiceAndDPA(t *testing.T) {
	item := vendorTestItem(models.VendorStatusOnboarding)
	item.ServiceDescription = "Processes customer support tickets"
	repo := &vendorServiceRepositoryStub{item: item}
	svc := NewVendorService(repo, zerolog.Nop())
	if _, err := svc.Transition(context.Background(), vendorTestOrg, vendorTestUser, vendorTestID, models.VendorTransitionInput{Status: models.VendorStatusActive, Version: 1}); !errors.Is(err, ErrVendorConflict) {
		t.Fatalf("missing owner/contact activation error=%v", err)
	}
	item.OwnerUserID = pointerToVendorString(vendorTestUser)
	item.Contacts = []models.VendorContact{{Name: "Vendor contact", Email: "vendor@example.test"}}
	item.DPARequired, item.DPAInPlace = true, false
	if _, err := svc.Transition(context.Background(), vendorTestOrg, vendorTestUser, vendorTestID, models.VendorTransitionInput{Status: models.VendorStatusActive, Version: 1}); !errors.Is(err, ErrVendorConflict) {
		t.Fatalf("missing DPA activation error=%v", err)
	}
	item.DPAInPlace = true
	if _, err := svc.Transition(context.Background(), vendorTestOrg, vendorTestUser, vendorTestID, models.VendorTransitionInput{Status: models.VendorStatusActive, Version: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestVendorAssessmentCadenceAndTerminalRetention(t *testing.T) {
	item := vendorTestItem(models.VendorStatusActive)
	repo := &vendorServiceRepositoryStub{item: item}
	svc := NewVendorService(repo, zerolog.Nop())
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	assessed, err := svc.RecordAssessment(context.Background(), vendorTestOrg, vendorTestUser, vendorTestID, models.VendorAssessmentInput{Version: 1, Status: models.VendorAssessmentCompleted, RiskTier: models.VendorRiskMedium, AssessedAt: now, Notes: "Reviewed SOC 2 report and remediation evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if assessed.NextAssessmentDate == nil || !assessed.NextAssessmentDate.Equal(now.AddDate(0, 0, 365)) {
		t.Fatalf("assessment=%#v", assessed)
	}
	item.Status = models.VendorStatusOffboarded
	future := now.Add(24 * time.Hour)
	item.RetentionUntil = &future
	if err := svc.Delete(context.Background(), vendorTestOrg, vendorTestUser, vendorTestID, item.Version); !errors.Is(err, ErrVendorConflict) {
		t.Fatalf("retention error=%v", err)
	}
}

func TestVendorServiceBoundsFiltersAndMapsRepositoryErrors(t *testing.T) {
	svc := NewVendorService(&vendorServiceRepositoryStub{}, zerolog.Nop())
	if _, _, err := svc.List(context.Background(), vendorTestOrg, models.VendorListFilter{Search: strings.Repeat("x", 501)}); !errors.Is(err, ErrVendorInvalid) {
		t.Fatalf("oversized filter error=%v", err)
	}
	svc = NewVendorService(&vendorServiceRepositoryStub{err: repository.ErrVendorVersionConflict, item: vendorTestItem(models.VendorStatusActive)}, zerolog.Nop())
	if _, err := svc.GetByID(context.Background(), vendorTestOrg, vendorTestID); !errors.Is(err, ErrVendorVersionConflict) {
		t.Fatalf("repository mapping error=%v", err)
	}
}

func pointerToVendorString(value string) *string { return &value }
