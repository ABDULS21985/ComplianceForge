package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

const (
	complianceOrgID       = "10000000-0000-0000-0000-000000000001"
	complianceFrameworkID = "20000000-0000-0000-0000-000000000001"
	complianceControlID   = "30000000-0000-0000-0000-000000000001"
	complianceUserID      = "40000000-0000-0000-0000-000000000001"
)

type frameworkRepoStub struct {
	getOrg string
}

func (*frameworkRepoStub) List(context.Context, string, models.PaginationRequest) ([]models.ComplianceFramework, int, error) {
	return nil, 0, nil
}
func (r *frameworkRepoStub) GetByID(_ context.Context, orgID, id string) (*models.ComplianceFramework, error) {
	r.getOrg = orgID
	return &models.ComplianceFramework{BaseModel: models.BaseModel{ID: id}}, nil
}
func (*frameworkRepoStub) Adopt(context.Context, string, string, string) (*models.OrganizationFramework, error) {
	return &models.OrganizationFramework{}, nil
}

type controlRepoStub struct {
	listOrg, listFramework string
	updateOrg, updateID    string
	updateCalls            int
	getErr                 error
}

func (r *controlRepoStub) ListByFramework(_ context.Context, orgID, frameworkID string, p models.PaginationRequest) ([]models.Control, int, error) {
	r.listOrg, r.listFramework = orgID, frameworkID
	if p.Page != 1 || p.PageSize != 20 {
		return nil, 0, errors.New("pagination was not normalized")
	}
	return []models.Control{}, 0, nil
}
func (*controlRepoStub) ListAdopted(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error) {
	return nil, 0, nil
}
func (r *controlRepoStub) GetAdoptedByID(context.Context, string, string) (*models.Control, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return &models.Control{}, nil
}
func (r *controlRepoStub) UpdateImplementation(_ context.Context, orgID, controlID string, _ models.ControlImplementationPatch) (*models.ControlImplementation, error) {
	r.updateCalls++
	r.updateOrg, r.updateID = orgID, controlID
	return &models.ControlImplementation{}, nil
}
func (*controlRepoStub) AttachEvidence(context.Context, string, string, string, models.AttachControlEvidenceInput) (*models.ControlEvidence, error) {
	return &models.ControlEvidence{}, nil
}
func (*controlRepoStub) ListEvidence(context.Context, string, string, models.PaginationRequest) ([]models.ControlEvidence, int, error) {
	return nil, 0, nil
}

func TestFrameworkServiceScopesCatalogControlsToTenant(t *testing.T) {
	frameworks, controls := &frameworkRepoStub{}, &controlRepoStub{}
	svc := NewFrameworkService(frameworks, controls, zerolog.Nop())
	if _, _, err := svc.ListFrameworkControls(context.Background(), complianceOrgID, complianceFrameworkID, models.PaginationRequest{}); err != nil {
		t.Fatal(err)
	}
	if frameworks.getOrg != complianceOrgID || controls.listOrg != complianceOrgID || controls.listFramework != complianceFrameworkID {
		t.Fatalf("tenant/framework propagation = %q/%q/%q", frameworks.getOrg, controls.listOrg, controls.listFramework)
	}
}

func TestFrameworkServiceValidatesImplementationBeforeRepository(t *testing.T) {
	controls := &controlRepoStub{}
	svc := NewFrameworkService(&frameworkRepoStub{}, controls, zerolog.Nop())
	invalid := models.ControlImplementationState("Compliant")
	if _, err := svc.UpdateControlImplementation(context.Background(), complianceOrgID, complianceControlID, models.ControlImplementationPatch{Status: &invalid}); !errors.Is(err, ErrInvalidControlPatch) {
		t.Fatalf("error=%v", err)
	}
	if controls.updateCalls != 0 {
		t.Fatal("repository called for invalid patch")
	}
	maturity := 3
	if _, err := svc.UpdateControlImplementation(context.Background(), complianceOrgID, complianceControlID, models.ControlImplementationPatch{MaturityLevel: &maturity}); err != nil {
		t.Fatal(err)
	}
	if controls.updateOrg != complianceOrgID || controls.updateID != complianceControlID {
		t.Fatal("tenant/control identity was not propagated")
	}
}

func TestFrameworkServiceMapsTenantMissAndValidatesEvidenceDates(t *testing.T) {
	controls := &controlRepoStub{getErr: pgx.ErrNoRows}
	svc := NewFrameworkService(&frameworkRepoStub{}, controls, zerolog.Nop())
	if _, err := svc.GetControl(context.Background(), complianceOrgID, complianceControlID); !errors.Is(err, ErrControlNotFound) {
		t.Fatalf("error=%v", err)
	}
	from, until := time.Now(), time.Now().Add(-time.Hour)
	_, err := svc.AttachControlEvidence(context.Background(), complianceOrgID, complianceUserID, complianceControlID, models.AttachControlEvidenceInput{Title: "Evidence", EvidenceType: "document", ValidFrom: &from, ValidUntil: &until})
	if !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("error=%v", err)
	}
}
