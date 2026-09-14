package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

const (
	riskTestOrgID       = "10000000-0000-0000-0000-000000000099"
	riskTestUserID      = "20000000-0000-0000-0000-000000000099"
	riskTestID          = "30000000-0000-0000-0000-000000000099"
	riskTestTreatmentID = "40000000-0000-0000-0000-000000000099"
)

type riskManagementRepoStub struct {
	RiskManagementRepository
	createdOrg       string
	createdInput     models.RiskCreateInput
	createCalls      int
	risk             *models.Risk
	getErr           error
	updateCalls      int
	assessmentScore  *float64
	assessmentLevel  *string
	treatment        *models.RiskTreatment
	treatmentUpdates int
}

func (r *riskManagementRepoStub) Create(_ context.Context, orgID string, input models.RiskCreateInput) (*models.Risk, error) {
	r.createCalls++
	r.createdOrg, r.createdInput = orgID, input
	return &models.Risk{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: riskTestID}, OrganizationID: orgID}, RiskRef: "RSK-0001", Title: input.Title, Status: models.RiskStatusIdentified}, nil
}

func (r *riskManagementRepoStub) GetByID(context.Context, string, string) (*models.Risk, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.risk, nil
}

func (r *riskManagementRepoStub) Update(_ context.Context, _ string, risk *models.Risk) (*models.Risk, error) {
	r.updateCalls++
	return risk, nil
}

func (r *riskManagementRepoStub) CreateAssessment(_ context.Context, _, _, _ string, _ models.RiskAssessmentInput, _ *float64, _ *string, scoreAfter *float64, levelAfter *string) (*models.RiskAssessment, error) {
	r.assessmentScore, r.assessmentLevel = scoreAfter, levelAfter
	return &models.RiskAssessment{}, nil
}

func (r *riskManagementRepoStub) GetTreatment(context.Context, string, string, string) (*models.RiskTreatment, error) {
	return r.treatment, nil
}

func (r *riskManagementRepoStub) UpdateTreatment(_ context.Context, _, _ string, treatment *models.RiskTreatment) (*models.RiskTreatment, error) {
	r.treatmentUpdates++
	return treatment, nil
}

func TestRiskServiceCreateValidatesAndNormalizesCanonicalRisk(t *testing.T) {
	repo := &riskManagementRepoStub{}
	svc := NewRiskService(repo, zerolog.Nop())
	likelihood := 4
	if _, err := svc.Create(context.Background(), riskTestOrgID, models.RiskCreateInput{Title: "incomplete", InherentLikelihood: &likelihood}); !errors.Is(err, ErrInvalidRisk) {
		t.Fatalf("incomplete scoring pair error=%v", err)
	}
	if repo.createCalls != 0 {
		t.Fatal("repository called for invalid risk")
	}
	impact := 5
	risk, err := svc.Create(context.Background(), riskTestOrgID, models.RiskCreateInput{Title: "  Availability  ", InherentLikelihood: &likelihood, InherentImpact: &impact})
	if err != nil {
		t.Fatal(err)
	}
	if risk.ID != riskTestID || repo.createdOrg != riskTestOrgID || repo.createdInput.Title != "Availability" {
		t.Fatalf("risk/repository propagation = %#v / %q / %#v", risk, repo.createdOrg, repo.createdInput)
	}
	if string(repo.createdInput.ImpactCategories) != "{}" || string(repo.createdInput.Attachments) != "[]" || string(repo.createdInput.Metadata) != "{}" {
		t.Fatalf("JSON defaults were not normalized: %#v", repo.createdInput)
	}
}

func TestRiskServiceRejectsIllegalLifecycleTransition(t *testing.T) {
	repo := &riskManagementRepoStub{risk: &models.Risk{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: riskTestID}, OrganizationID: riskTestOrgID},
		RiskRef:     "RSK-0001", Title: "Closed", Status: models.RiskStatusClosed,
		ImpactCategories: []byte(`{}`), Attachments: []byte(`[]`), Metadata: []byte(`{}`),
	}}
	svc := NewRiskService(repo, zerolog.Nop())
	status := models.RiskStatusAssessed
	if _, err := svc.Update(context.Background(), riskTestOrgID, riskTestID, models.RiskPatch{Status: &status}); !errors.Is(err, ErrInvalidRiskTransition) {
		t.Fatalf("transition error=%v", err)
	}
	if repo.updateCalls != 0 {
		t.Fatal("repository updated a closed risk")
	}
}

func TestRiskServiceAssessmentUsesMigrationScoreThresholds(t *testing.T) {
	repo := &riskManagementRepoStub{}
	svc := NewRiskService(repo, zerolog.Nop())
	likelihood, impact := 3, 4
	_, err := svc.CreateAssessment(context.Background(), riskTestOrgID, riskTestID, riskTestUserID, models.RiskAssessmentInput{
		AssessmentType: "periodic", LikelihoodAfter: &likelihood, ImpactAfter: &impact,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.assessmentScore == nil || *repo.assessmentScore != 12 || repo.assessmentLevel == nil || *repo.assessmentLevel != "high" {
		t.Fatalf("assessment score/level = %v/%v", repo.assessmentScore, repo.assessmentLevel)
	}
	if score, level := svc.CalculateRiskScore(2, 3); score != 6 || level != "medium" {
		t.Fatalf("CalculateRiskScore(2,3)=%v/%s", score, level)
	}
}

func TestRiskServiceTreatmentTransitionsAreFailClosed(t *testing.T) {
	repo := &riskManagementRepoStub{treatment: &models.RiskTreatment{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: riskTestTreatmentID}, OrganizationID: riskTestOrgID},
		RiskID:      riskTestID, TreatmentType: "mitigate", Title: "Patch", Status: "planned",
	}}
	svc := NewRiskService(repo, zerolog.Nop())
	completed, progress := "completed", 100
	_, err := svc.UpdateTreatment(context.Background(), riskTestOrgID, riskTestID, riskTestTreatmentID, models.RiskTreatmentPatch{Status: &completed, ProgressPercentage: &progress})
	if !errors.Is(err, ErrInvalidTreatmentState) || repo.treatmentUpdates != 0 {
		t.Fatalf("planned->completed error=%v updates=%d", err, repo.treatmentUpdates)
	}
	inProgress := "in_progress"
	repo.treatment.Status = inProgress
	updated, err := svc.UpdateTreatment(context.Background(), riskTestOrgID, riskTestID, riskTestTreatmentID, models.RiskTreatmentPatch{Status: &completed, ProgressPercentage: &progress})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CompletedDate == nil || repo.treatmentUpdates != 1 {
		t.Fatalf("completion metadata=%#v updates=%d", updated, repo.treatmentUpdates)
	}
}

func TestRiskServiceMapsTenantMissAndRejectsBadKRIThresholds(t *testing.T) {
	repo := &riskManagementRepoStub{getErr: pgx.ErrNoRows}
	svc := NewRiskService(repo, zerolog.Nop())
	if _, err := svc.GetByID(context.Background(), riskTestOrgID, riskTestID); !errors.Is(err, ErrRiskNotFound) {
		t.Fatalf("tenant miss error=%v", err)
	}
	green, amber := 10.0, 5.0
	_, err := svc.CreateIndicator(context.Background(), riskTestOrgID, riskTestID, riskTestUserID, models.RiskIndicatorInput{
		Name: "Failed logins", MetricType: "count", ThresholdGreen: &green, ThresholdAmber: &amber,
	})
	if !errors.Is(err, ErrInvalidRiskIndicator) {
		t.Fatalf("threshold validation error=%v", err)
	}
}
