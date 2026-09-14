package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const (
	managedRoleTestOrg   = "51000000-0000-0000-0000-000000000001"
	managedRoleTestActor = "52000000-0000-0000-0000-000000000001"
	managedRoleTestID    = "53000000-0000-0000-0000-000000000001"
	managedRoleTestUser  = "54000000-0000-0000-0000-000000000001"
)

type managedRoleStoreStub struct {
	AccessAdministrationStore
	createInput models.ManagedRoleCreateInput
	patch       models.ManagedRolePatch
	listFilter  models.ManagedRoleListFilter
	assignment  models.ManagedRoleAssignmentInput
	createErr   error
	updateErr   error
	assignErr   error
}

func (s *managedRoleStoreStub) CreateRole(_ context.Context, _, _ string, input models.ManagedRoleCreateInput) (*models.ManagedRole, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &models.ManagedRole{ID: managedRoleTestID, Name: input.Name, Slug: input.Slug, Permissions: input.Permissions, Version: 1}, nil
}

func (s *managedRoleStoreStub) UpdateRole(_ context.Context, _, _, _ string, patch models.ManagedRolePatch) (*models.ManagedRole, error) {
	s.patch = patch
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return &models.ManagedRole{ID: managedRoleTestID, Name: "Reviewer", Slug: "reviewer", Version: patch.ExpectedVersion + 1}, nil
}

func (s *managedRoleStoreStub) ListRoles(_ context.Context, _ string, filter models.ManagedRoleListFilter) ([]models.ManagedRole, int, error) {
	s.listFilter = filter
	return []models.ManagedRole{}, 0, nil
}

func (s *managedRoleStoreStub) AssignRole(_ context.Context, _, _, _ string, input models.ManagedRoleAssignmentInput) error {
	s.assignment = input
	return s.assignErr
}

func TestAccessAdministrationServiceNormalizesRoleCreate(t *testing.T) {
	store := &managedRoleStoreStub{}
	svc := NewAccessAdministrationService(store, zerolog.Nop())
	role, err := svc.CreateRole(context.Background(), managedRoleTestOrg, managedRoleTestActor, models.ManagedRoleCreateInput{
		Name: "  Security Reviewer  ", Description: "  Reviews security changes  ",
		Permissions: []models.PermissionGrant{
			{Resource: " Risks ", Action: " READ "},
			{Resource: "risks", Action: "read"},
			{Resource: "Policies", Action: "APPROVE"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if role.Slug != "security-reviewer" || store.createInput.Description != "Reviews security changes" {
		t.Fatalf("normalized role=%#v input=%#v", role, store.createInput)
	}
	if len(store.createInput.Permissions) != 2 || store.createInput.Permissions[0].Resource != "risks" || store.createInput.Permissions[1].Action != "approve" {
		t.Fatalf("normalized permissions=%#v", store.createInput.Permissions)
	}
}

func TestAccessAdministrationServiceRejectsInvalidDefinitions(t *testing.T) {
	svc := NewAccessAdministrationService(&managedRoleStoreStub{}, zerolog.Nop())
	for _, input := range []models.ManagedRoleCreateInput{
		{Name: "x"},
		{Name: "Valid role", Slug: "###"},
		{Name: "Valid role", Permissions: []models.PermissionGrant{{Resource: "risks", Action: "read:*"}}},
	} {
		if _, err := svc.CreateRole(context.Background(), managedRoleTestOrg, managedRoleTestActor, input); err == nil {
			t.Fatalf("invalid role accepted: %#v", input)
		}
	}
	if _, err := svc.CreateRole(context.Background(), "not-a-uuid", managedRoleTestActor, models.ManagedRoleCreateInput{Name: "Valid role"}); !errors.Is(err, ErrManagedRoleInvalid) {
		t.Fatalf("invalid tenant error=%v", err)
	}
}

func TestAccessAdministrationServiceEnforcesOptimisticRoleUpdates(t *testing.T) {
	name := "Updated reviewer"
	store := &managedRoleStoreStub{}
	svc := NewAccessAdministrationService(store, zerolog.Nop())
	if _, err := svc.UpdateRole(context.Background(), managedRoleTestOrg, managedRoleTestID, managedRoleTestActor, models.ManagedRolePatch{Name: &name}); !errors.Is(err, ErrManagedRoleInvalid) {
		t.Fatalf("missing version error=%v", err)
	}
	role, err := svc.UpdateRole(context.Background(), managedRoleTestOrg, managedRoleTestID, managedRoleTestActor, models.ManagedRolePatch{Name: &name, ExpectedVersion: 3})
	if err != nil || role.Version != 4 || store.patch.Name == nil || *store.patch.Name != name {
		t.Fatalf("role=%#v patch=%#v err=%v", role, store.patch, err)
	}
	store.updateErr = repository.ErrManagedRoleVersionConflict
	if _, err := svc.UpdateRole(context.Background(), managedRoleTestOrg, managedRoleTestID, managedRoleTestActor, models.ManagedRolePatch{Name: &name, ExpectedVersion: 3}); !errors.Is(err, ErrManagedRoleConflict) {
		t.Fatalf("version conflict error=%v", err)
	}
	store.updateErr = repository.ErrManagedRoleImmutable
	if _, err := svc.UpdateRole(context.Background(), managedRoleTestOrg, managedRoleTestID, managedRoleTestActor, models.ManagedRolePatch{Name: &name, ExpectedVersion: 3}); !errors.Is(err, ErrManagedRoleImmutable) {
		t.Fatalf("immutable error=%v", err)
	}
}

func TestAccessAdministrationServiceBoundsListAndAssignment(t *testing.T) {
	store := &managedRoleStoreStub{}
	svc := NewAccessAdministrationService(store, zerolog.Nop())
	if _, _, err := svc.ListRoles(context.Background(), managedRoleTestOrg, models.ManagedRoleListFilter{
		PaginationRequest: models.PaginationRequest{Page: -1, PageSize: 1000}, Search: " reviewer ",
	}); err != nil {
		t.Fatal(err)
	}
	if store.listFilter.Page != 1 || store.listFilter.PageSize != 100 || store.listFilter.Search != "reviewer" {
		t.Fatalf("normalized list filter=%#v", store.listFilter)
	}
	if err := svc.AssignRole(context.Background(), managedRoleTestOrg, managedRoleTestID, managedRoleTestActor, models.ManagedRoleAssignmentInput{UserID: managedRoleTestUser, Reason: "x"}); !errors.Is(err, ErrRoleAssignmentInvalid) {
		t.Fatalf("short reason error=%v", err)
	}
	if err := svc.AssignRole(context.Background(), managedRoleTestOrg, managedRoleTestID, managedRoleTestActor, models.ManagedRoleAssignmentInput{UserID: managedRoleTestUser, Reason: "  Quarterly access review  "}); err != nil {
		t.Fatal(err)
	}
	if store.assignment.Reason != "Quarterly access review" {
		t.Fatalf("normalized assignment=%#v", store.assignment)
	}
	store.assignErr = repository.ErrManagedRoleAssignment
	if err := svc.AssignRole(context.Background(), managedRoleTestOrg, managedRoleTestID, managedRoleTestActor, models.ManagedRoleAssignmentInput{UserID: managedRoleTestUser, Reason: "Quarterly access review"}); !errors.Is(err, ErrRoleAssignmentInvalid) {
		t.Fatalf("assignment mapping error=%v", err)
	}
}

func TestManagedRoleSlugNormalization(t *testing.T) {
	for input, want := range map[string]string{
		"Security & Privacy Reviewer": "security-privacy-reviewer",
		"  --Risk___Owner--  ":        "risk-owner",
		"CISO Level 2":                "ciso-level-2",
	} {
		if got := normalizeManagedRoleSlug(input, ""); got != want {
			t.Errorf("normalizeManagedRoleSlug(%q)=%q want=%q", input, got, want)
		}
	}
}
