package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

const (
	auditTestOrg     = "10000000-0000-0000-0000-000000000001"
	auditTestUser    = "20000000-0000-0000-0000-000000000001"
	auditTestID      = "30000000-0000-0000-0000-000000000001"
	auditTestFinding = "40000000-0000-0000-0000-000000000001"
)

type auditServiceRepository struct {
	AuditManagementRepository
	audit   *models.Audit
	finding *models.AuditFinding
	stats   models.AuditFindingStats
}

func cloneAudit(item *models.Audit) *models.Audit {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func cloneFinding(item *models.AuditFinding) *models.AuditFinding {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func (r *auditServiceRepository) Create(_ context.Context, item *models.Audit) (*models.Audit, error) {
	item.ID = auditTestID
	item.AuditRef = "AUD-0001"
	r.audit = cloneAudit(item)
	return cloneAudit(item), nil
}

func (r *auditServiceRepository) GetByID(_ context.Context, orgID, id string) (*models.Audit, error) {
	if r.audit == nil || r.audit.OrganizationID != orgID || r.audit.ID != id {
		return nil, pgx.ErrNoRows
	}
	return cloneAudit(r.audit), nil
}

func (r *auditServiceRepository) Update(_ context.Context, orgID string, item *models.Audit) (*models.Audit, error) {
	if r.audit == nil || r.audit.OrganizationID != orgID || r.audit.ID != item.ID {
		return nil, pgx.ErrNoRows
	}
	r.audit = cloneAudit(item)
	return cloneAudit(item), nil
}

func (r *auditServiceRepository) Delete(_ context.Context, orgID, id string) error {
	if r.audit == nil || r.audit.OrganizationID != orgID || r.audit.ID != id {
		return pgx.ErrNoRows
	}
	r.audit = nil
	return nil
}

func (r *auditServiceRepository) CreateFinding(_ context.Context, item *models.AuditFinding) (*models.AuditFinding, error) {
	item.ID = auditTestFinding
	item.FindingRef = "FND-0001"
	r.finding = cloneFinding(item)
	return cloneFinding(item), nil
}

func (r *auditServiceRepository) GetFindingByID(_ context.Context, orgID, auditID, id string) (*models.AuditFinding, error) {
	if r.finding == nil || r.finding.OrganizationID != orgID || r.finding.AuditID != auditID || r.finding.ID != id {
		return nil, pgx.ErrNoRows
	}
	return cloneFinding(r.finding), nil
}

func (r *auditServiceRepository) UpdateFinding(_ context.Context, orgID, auditID string, item *models.AuditFinding) (*models.AuditFinding, error) {
	if r.finding == nil || r.finding.OrganizationID != orgID || r.finding.AuditID != auditID || r.finding.ID != item.ID {
		return nil, pgx.ErrNoRows
	}
	r.finding = cloneFinding(item)
	return cloneFinding(item), nil
}

func (r *auditServiceRepository) FindingStats(context.Context, string, string) (*models.AuditFindingStats, error) {
	stats := r.stats
	return &stats, nil
}

func TestAuditServiceCreateAcceptsBrowserDatesAndNormalizesState(t *testing.T) {
	repository := &auditServiceRepository{}
	service := NewAuditService(repository, zerolog.Nop())
	item, err := service.Create(context.Background(), auditTestOrg, auditTestUser, models.AuditCreateInput{
		Title: "  Annual audit  ", Description: " Evidence review ", AuditType: "INTERNAL",
		LeadAuditorID: auditTestUser, Scope: " Production ",
		ScheduledStartDate: "2026-10-01", ScheduledEndDate: "2026-10-05",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != auditTestID || item.Status != models.AuditStatusPlanned || item.Type != models.AuditTypeInternal || item.Title != "Annual audit" {
		t.Fatalf("created audit=%#v", item)
	}
	if string(item.Metadata) != `{}` || item.ScheduledStartDate == nil || item.ScheduledStartDate.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("created audit dates/metadata=%#v", item)
	}

	_, err = service.Create(context.Background(), auditTestOrg, auditTestUser, models.AuditCreateInput{
		Title: "Invalid dates", Description: "Invalid", AuditType: models.AuditTypeInternal,
		LeadAuditorID: auditTestUser, Scope: "Scope", ScheduledStartDate: "2026-10-05", ScheduledEndDate: "2026-10-01",
	})
	if !errors.Is(err, ErrAuditInvalid) {
		t.Fatalf("reversed dates error=%v", err)
	}
}

func TestAuditServiceEnforcesLifecycleAndRetention(t *testing.T) {
	start := time.Now().UTC().AddDate(0, 0, 1)
	end := start.AddDate(0, 0, 2)
	repository := &auditServiceRepository{audit: &models.Audit{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: auditTestID}, OrganizationID: auditTestOrg},
		AuditRef:    "AUD-0001", Title: "Lifecycle", Description: "Lifecycle test",
		Type: models.AuditTypeInternal, Status: models.AuditStatusPlanned,
		LeadAuditorID: auditTestUser, Scope: "Scope", ScheduledStartDate: &start,
		ScheduledEndDate: &end, CreatedBy: auditTestUser, Metadata: json.RawMessage(`{}`),
	}}
	service := NewAuditService(repository, zerolog.Nop())

	started, err := service.Start(context.Background(), auditTestOrg, auditTestID)
	if err != nil || started.Status != models.AuditStatusInProgress || started.ActualStartDate == nil {
		t.Fatalf("started=%#v err=%v", started, err)
	}
	if err := service.Delete(context.Background(), auditTestOrg, auditTestID); !errors.Is(err, ErrAuditConflict) {
		t.Fatalf("executed audit delete error=%v", err)
	}
	completed, err := service.Complete(context.Background(), auditTestOrg, auditTestID)
	if err != nil || completed.Status != models.AuditStatusCompleted || completed.ActualEndDate == nil {
		t.Fatalf("completed=%#v err=%v", completed, err)
	}
	repository.stats.Open = 1
	if _, err := service.Close(context.Background(), auditTestOrg, auditTestID); !errors.Is(err, ErrAuditConflict) {
		t.Fatalf("close with open finding error=%v", err)
	}
	repository.stats.Open = 0
	closed, err := service.Close(context.Background(), auditTestOrg, auditTestID)
	if err != nil || closed.Status != models.AuditStatusClosed {
		t.Fatalf("closed=%#v err=%v", closed, err)
	}
	if _, err := service.Update(context.Background(), auditTestOrg, auditTestID, models.AuditPatch{Title: stringPointerForAuditTest("Changed")}); !errors.Is(err, ErrAuditConflict) {
		t.Fatalf("closed audit update error=%v", err)
	}
}

func TestAuditServiceFindingAcceptanceRequiresReason(t *testing.T) {
	start := time.Now().UTC()
	end := start.AddDate(0, 0, 1)
	due := end.AddDate(0, 0, 10)
	repository := &auditServiceRepository{
		audit: &models.Audit{
			TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: auditTestID}, OrganizationID: auditTestOrg},
			Status:      models.AuditStatusInProgress, ScheduledStartDate: &start, ScheduledEndDate: &end,
		},
		finding: &models.AuditFinding{
			TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: auditTestFinding}, OrganizationID: auditTestOrg},
			AuditID:     auditTestID, Title: "Finding", Description: "Description", Severity: "high",
			Status: models.FindingStatusOpen, FindingType: "observation", Recommendation: "Fix it",
			ResponsibleUserID: auditTestUser, DueDate: &due, Metadata: json.RawMessage(`{}`),
		},
	}
	service := NewAuditService(repository, zerolog.Nop())
	accepted := models.FindingStatusAccepted
	if _, err := service.UpdateFinding(context.Background(), auditTestOrg, auditTestID, auditTestFinding, models.AuditFindingPatch{Status: &accepted}); !errors.Is(err, ErrFindingInvalid) {
		t.Fatalf("acceptance without reason error=%v", err)
	}
	reason := "Compensating control approved by the risk owner"
	item, err := service.UpdateFinding(context.Background(), auditTestOrg, auditTestID, auditTestFinding, models.AuditFindingPatch{Status: &accepted, AcceptedRiskReason: &reason})
	if err != nil || item.Status != models.FindingStatusAccepted || item.AcceptedRiskReason == nil || *item.AcceptedRiskReason != reason {
		t.Fatalf("accepted finding=%#v err=%v", item, err)
	}
	open := models.FindingStatusOpen
	if _, err := service.UpdateFinding(context.Background(), auditTestOrg, auditTestID, auditTestFinding, models.AuditFindingPatch{Status: &open}); !errors.Is(err, ErrFindingInvalidTransition) {
		t.Fatalf("accepted-to-open transition error=%v", err)
	}
}

func stringPointerForAuditTest(value string) *string { return &value }
