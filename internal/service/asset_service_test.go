package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const (
	assetTestOrg   = "20000000-0000-0000-0000-000000000001"
	assetTestUser  = "10000000-0000-0000-0000-000000000001"
	assetTestID    = "30000000-0000-0000-0000-000000000001"
	assetTestOwner = "40000000-0000-0000-0000-000000000001"
)

type assetRepositoryStub struct {
	current     *models.Asset
	createInput models.AssetCreateInput
	updatePatch models.AssetPatch
	listFilter  models.AssetListFilter
	err         error
}

func (r *assetRepositoryStub) Create(_ context.Context, orgID, actorID string, input models.AssetCreateInput) (*models.Asset, error) {
	r.createInput = input
	if r.err != nil {
		return nil, r.err
	}
	return &models.Asset{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: assetTestID}, OrganizationID: orgID},
		AssetRef:    "AST-000001", Name: input.Name, AssetType: input.AssetType,
		Criticality: input.Criticality, Classification: input.Classification,
		Status: models.AssetStatusActive, Tags: input.Tags, Metadata: input.Metadata,
		Version: 1, CreatedBy: actorID,
	}, nil
}

func (r *assetRepositoryStub) GetByID(context.Context, string, string) (*models.Asset, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.current == nil {
		return nil, pgx.ErrNoRows
	}
	copy := *r.current
	return &copy, nil
}

func (r *assetRepositoryStub) Update(_ context.Context, _, _, _ string, patch models.AssetPatch) (*models.Asset, error) {
	r.updatePatch = patch
	if r.err != nil {
		return nil, r.err
	}
	copy := *r.current
	copy.Version++
	if patch.Status != nil {
		copy.Status = *patch.Status
	}
	return &copy, nil
}

func (r *assetRepositoryStub) Delete(context.Context, string, string, string, *int64) error {
	return r.err
}

func (r *assetRepositoryStub) List(_ context.Context, _ string, filter models.AssetListFilter) ([]models.Asset, int, error) {
	r.listFilter = filter
	return nil, 0, r.err
}

func (r *assetRepositoryStub) Stats(context.Context, string) (*models.AssetStats, error) {
	return &models.AssetStats{ByType: map[string]int{}}, r.err
}

func (r *assetRepositoryStub) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error) {
	return nil, 0, r.err
}

func assetTestService(repository AssetManagementRepository) *AssetService {
	return NewAssetService(repository, zerolog.Nop())
}

func activeAsset() *models.Asset {
	return &models.Asset{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: assetTestID}, OrganizationID: assetTestOrg},
		AssetRef:    "AST-000001", Name: "Payments database", AssetType: models.AssetTypeData,
		Criticality: models.AssetCriticalityCritical, Classification: models.AssetClassificationRestricted,
		Status: models.AssetStatusActive, Tags: []string{"pci"}, Metadata: []byte("{}"),
		Version: 4, CreatedBy: assetTestUser,
	}
}

func TestAssetServiceCreateNormalizesEnterpriseContract(t *testing.T) {
	repository := &assetRepositoryStub{}
	ip := " 2001:db8::1 "
	owner := " " + assetTestOwner + " "
	item, err := assetTestService(repository).Create(context.Background(), assetTestOrg, assetTestUser, models.AssetCreateInput{
		Name: "  Payments database  ", AssetType: models.AssetTypeData,
		OwnerUserID: &owner, IPAddress: &ip, Tags: []string{" PCI ", "pci", "Critical"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != models.AssetStatusActive || repository.createInput.Criticality != models.AssetCriticalityMedium ||
		repository.createInput.Classification != models.AssetClassificationInternal {
		t.Fatalf("defaults were not applied: item=%+v input=%+v", item, repository.createInput)
	}
	if repository.createInput.Name != "Payments database" || *repository.createInput.IPAddress != "2001:db8::1" ||
		*repository.createInput.OwnerUserID != assetTestOwner {
		t.Fatalf("input was not normalized: %+v", repository.createInput)
	}
	if len(repository.createInput.Tags) != 2 || repository.createInput.Tags[0] != "critical" || repository.createInput.Tags[1] != "pci" {
		t.Fatalf("tags = %#v", repository.createInput.Tags)
	}
}

func TestAssetServiceRejectsInvalidCreateValues(t *testing.T) {
	service := assetTestService(&assetRepositoryStub{})
	for _, test := range []struct {
		name  string
		input models.AssetCreateInput
	}{
		{"missing type", models.AssetCreateInput{Name: "Asset"}},
		{"invalid IP", models.AssetCreateInput{Name: "Asset", AssetType: models.AssetTypeSoftware, IPAddress: pointerTo("not-an-ip")}},
		{"metadata array", models.AssetCreateInput{Name: "Asset", AssetType: models.AssetTypeSoftware, Metadata: []byte("[]")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Create(context.Background(), assetTestOrg, assetTestUser, test.input); !errors.Is(err, ErrInvalidAsset) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestAssetServiceUsesOptimisticVersionAndLifecycle(t *testing.T) {
	repository := &assetRepositoryStub{current: activeAsset()}
	status := models.AssetStatusDecommissioned
	item, err := assetTestService(repository).Update(context.Background(), assetTestOrg, assetTestID, assetTestUser, models.AssetPatch{Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != status || repository.updatePatch.ExpectedVersion == nil || *repository.updatePatch.ExpectedVersion != 4 {
		t.Fatalf("update item=%+v patch=%+v", item, repository.updatePatch)
	}

	repository.current.Status = models.AssetStatusDecommissioned
	name := "Changed after retirement"
	if _, err := assetTestService(repository).Update(context.Background(), assetTestOrg, assetTestID, assetTestUser, models.AssetPatch{Name: &name}); !errors.Is(err, ErrAssetConflict) {
		t.Fatalf("decommissioned update error = %v", err)
	}
}

func TestAssetServiceRejectsStaleVersionAndInvalidTransition(t *testing.T) {
	repository := &assetRepositoryStub{current: activeAsset()}
	stale := int64(3)
	if _, err := assetTestService(repository).Update(context.Background(), assetTestOrg, assetTestID, assetTestUser, models.AssetPatch{ExpectedVersion: &stale}); !errors.Is(err, ErrAssetConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	invalid := models.AssetStatus("lost")
	if _, err := assetTestService(repository).Update(context.Background(), assetTestOrg, assetTestID, assetTestUser, models.AssetPatch{Status: &invalid}); !errors.Is(err, ErrAssetConflict) {
		t.Fatalf("invalid transition error = %v", err)
	}
}

func TestAssetServiceNormalizesAndValidatesListFilter(t *testing.T) {
	repository := &assetRepositoryStub{}
	personalData := true
	_, _, err := assetTestService(repository).List(context.Background(), assetTestOrg, models.AssetListFilter{
		PaginationRequest: models.PaginationRequest{Page: 0, PageSize: 500},
		AssetType:         " DATA ", SortBy: " NAME ", SortDirection: " ASC ",
		ProcessesPersonalData: &personalData,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.listFilter.Page != 1 || repository.listFilter.PageSize != 100 ||
		repository.listFilter.AssetType != "data" || repository.listFilter.SortBy != "name" ||
		repository.listFilter.SortDirection != "asc" {
		t.Fatalf("filter = %+v", repository.listFilter)
	}
}

func TestAssetRepositoryErrorsAreClassified(t *testing.T) {
	service := assetTestService(&assetRepositoryStub{err: repository.ErrAssetOwnerInvalid})
	if _, err := service.Create(context.Background(), assetTestOrg, assetTestUser, models.AssetCreateInput{
		Name: "Asset", AssetType: models.AssetTypeSoftware,
	}); !errors.Is(err, ErrAssetOwnerNotFound) {
		t.Fatalf("owner error = %v", err)
	}
}

func pointerTo(value string) *string { return &value }
