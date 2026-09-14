package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

const (
	directoryTestOrg   = "71000000-0000-0000-0000-000000000001"
	directoryTestActor = "71000000-0000-0000-0000-000000000002"
	directoryTestUser  = "71000000-0000-0000-0000-000000000003"
	directoryTestOther = "71000000-0000-0000-0000-000000000004"
	directoryTestGroup = "71000000-0000-0000-0000-000000000005"
)

type directoryStoreStub struct {
	UserAdministrationStore
	createInput  models.DirectoryUserCreateInput
	createErr    error
	currentUser  *models.DirectoryUser
	updatedUser  *models.DirectoryUser
	group        *models.DirectoryGroup
	groupInput   models.DirectoryGroupCreateInput
	membersInput models.DirectoryGroupBulkMembersInput
	previewRows  []models.DirectoryImportRow
	applyKey     string
	applyHash    string
	applyRows    []models.DirectoryImportRow
}

func (s *directoryStoreStub) CreateUser(_ context.Context, orgID, _ string, input models.DirectoryUserCreateInput) (*models.DirectoryUser, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &models.DirectoryUser{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: directoryTestUser}, OrganizationID: orgID}, Email: input.Email, Status: input.InitialStatus, Version: 1}, nil
}

func (s *directoryStoreStub) GetUser(context.Context, string, string) (*models.DirectoryUser, error) {
	if s.currentUser == nil {
		return nil, repository.ErrDirectoryUserNotFound
	}
	copy := *s.currentUser
	return &copy, nil
}

func (s *directoryStoreStub) UpdateUser(_ context.Context, _, _ string, item *models.DirectoryUser, _ int64, _ string) (*models.DirectoryUser, error) {
	copy := *item
	copy.Version++
	s.updatedUser = &copy
	return &copy, nil
}

func (s *directoryStoreStub) SuspendUser(_ context.Context, _, _, _ string, input models.DirectoryUserStateInput) (*models.DirectoryUser, error) {
	return &models.DirectoryUser{Version: input.ExpectedVersion + 1, Status: models.UserStatusInactive}, nil
}

func (s *directoryStoreStub) ReactivateUser(_ context.Context, _, _, _ string, input models.DirectoryUserStateInput) (*models.DirectoryUser, error) {
	return &models.DirectoryUser{Version: input.ExpectedVersion + 1, Status: models.UserStatusActive}, nil
}

func (s *directoryStoreStub) CreateGroup(_ context.Context, orgID, _ string, input models.DirectoryGroupCreateInput) (*models.DirectoryGroup, error) {
	s.groupInput = input
	return &models.DirectoryGroup{TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: directoryTestGroup}, OrganizationID: orgID}, Name: input.Name, Slug: input.Slug, GroupType: input.GroupType, MembershipRule: input.MembershipRule, Version: 1}, nil
}

func (s *directoryStoreStub) GetGroup(context.Context, string, string) (*models.DirectoryGroup, error) {
	if s.group == nil {
		return nil, repository.ErrDirectoryGroupNotFound
	}
	copy := *s.group
	return &copy, nil
}

func (s *directoryStoreStub) ChangeGroupMembers(_ context.Context, _, _, _ string, input models.DirectoryGroupBulkMembersInput) (*models.DirectoryGroup, error) {
	s.membersInput = input
	return &models.DirectoryGroup{Version: input.ExpectedVersion + 1}, nil
}

func (s *directoryStoreStub) PreviewImport(_ context.Context, _ string, rows []models.DirectoryImportRow, hash string) (*models.DirectoryImportPreview, error) {
	s.previewRows = rows
	return &models.DirectoryImportPreview{ContentSHA256: hash, RowCount: len(rows), ValidCount: len(rows), CreateCount: len(rows)}, nil
}

func (s *directoryStoreStub) ApplyImport(_ context.Context, _, _, key, hash, _ string, rows []models.DirectoryImportRow) (*models.DirectoryImportResult, error) {
	s.applyKey, s.applyHash, s.applyRows = key, hash, rows
	return &models.DirectoryImportResult{ID: directoryTestGroup, IdempotencyKey: key, ContentSHA256: hash, RowCount: len(rows)}, nil
}

func TestUserAdministrationCreateNormalizesInviteReadyUser(t *testing.T) {
	store := &directoryStoreStub{}
	service := NewUserAdministrationService(store, zerolog.Nop())
	fixed := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	item, err := service.CreateUser(context.Background(), directoryTestOrg, directoryTestActor, models.DirectoryUserCreateInput{
		Email: "  Ada.Vendor@Example.TEST ", FirstName: " Ada ", Department: " Security ", Reason: " New team member ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Email != "ada.vendor@example.test" || store.createInput.InitialRoleSlug != "viewer" || store.createInput.InitialStatus != models.UserStatusPendingVerification {
		t.Fatalf("created=%#v input=%#v", item, store.createInput)
	}
	if store.createInput.InvitationExpiresAt == nil || !store.createInput.InvitationExpiresAt.Equal(fixed.Add(7*24*time.Hour)) {
		t.Fatalf("invitation expiry=%v", store.createInput.InvitationExpiresAt)
	}
}

func TestUserAdministrationRejectsUnsafeCreateAndMapsCapacity(t *testing.T) {
	store := &directoryStoreStub{}
	service := NewUserAdministrationService(store, zerolog.Nop())
	_, err := service.CreateUser(context.Background(), directoryTestOrg, directoryTestActor, models.DirectoryUserCreateInput{Email: "not-an-email", Reason: "create user"})
	if !errors.Is(err, ErrUserAdministrationInvalid) {
		t.Fatalf("invalid create error=%v", err)
	}
	store.createErr = repository.ErrEntitlementLimitExceeded
	_, err = service.CreateUser(context.Background(), directoryTestOrg, directoryTestActor, models.DirectoryUserCreateInput{Email: "valid@example.test", FirstName: "Valid", Reason: "create user"})
	if !errors.Is(err, ErrSubscriptionLimitExceeded) {
		t.Fatalf("capacity error=%v", err)
	}
}

func TestUserAdministrationUpdateAndSelfProtection(t *testing.T) {
	store := &directoryStoreStub{currentUser: &models.DirectoryUser{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: directoryTestUser}, OrganizationID: directoryTestOrg},
		Email:       "old@example.test", FirstName: "Old", Status: models.UserStatusActive, Language: "en", Version: 3,
	}}
	service := NewUserAdministrationService(store, zerolog.Nop())
	department := "Engineering"
	item, err := service.UpdateUser(context.Background(), directoryTestOrg, directoryTestUser, directoryTestActor, models.DirectoryUserPatch{ExpectedVersion: 3, Department: &department, Reason: "Move into engineering"})
	if err != nil || item.Department != department || item.Version != 4 {
		t.Fatalf("updated=%#v err=%v", item, err)
	}
	_, err = service.UpdateUser(context.Background(), directoryTestOrg, directoryTestUser, directoryTestActor, models.DirectoryUserPatch{ExpectedVersion: 3, Department: &department, ClearDepartment: true, Reason: "Invalid collision"})
	if !errors.Is(err, ErrUserAdministrationInvalid) {
		t.Fatalf("set/clear error=%v", err)
	}
	_, err = service.SuspendUser(context.Background(), directoryTestOrg, directoryTestActor, directoryTestActor, models.DirectoryUserStateInput{ExpectedVersion: 1, Reason: "self suspend attempt"})
	if !errors.Is(err, ErrUserAdministrationConflict) {
		t.Fatalf("self suspension error=%v", err)
	}
}

func TestUserAdministrationValidatesStaticAndDynamicGroups(t *testing.T) {
	store := &directoryStoreStub{}
	service := NewUserAdministrationService(store, zerolog.Nop())
	static, err := service.CreateGroup(context.Background(), directoryTestOrg, directoryTestActor, models.DirectoryGroupCreateInput{Name: "Control Owners", Reason: "Create owner group"})
	if err != nil || static.Slug != "control-owners" || string(static.MembershipRule) != "{}" {
		t.Fatalf("static=%#v err=%v", static, err)
	}
	_, err = service.CreateGroup(context.Background(), directoryTestOrg, directoryTestActor, models.DirectoryGroupCreateInput{Name: "Dynamic", GroupType: models.DirectoryGroupDynamic, MembershipRule: []byte(`{"sql":"DROP TABLE users"}`), Reason: "Create dynamic group"})
	if !errors.Is(err, ErrUserAdministrationInvalid) {
		t.Fatalf("unknown dynamic rule error=%v", err)
	}
	dynamic, err := service.CreateGroup(context.Background(), directoryTestOrg, directoryTestActor, models.DirectoryGroupCreateInput{Name: "Engineering", GroupType: models.DirectoryGroupDynamic, MembershipRule: []byte(`{"departments":["Engineering"],"statuses":["active"]}`), Reason: "Create engineering group"})
	if err != nil || dynamic.GroupType != models.DirectoryGroupDynamic {
		t.Fatalf("dynamic=%#v err=%v", dynamic, err)
	}
}

func TestUserAdministrationBoundsAndNormalizesMembershipChanges(t *testing.T) {
	store := &directoryStoreStub{}
	service := NewUserAdministrationService(store, zerolog.Nop())
	item, err := service.ChangeGroupMembers(context.Background(), directoryTestOrg, directoryTestGroup, directoryTestActor, models.DirectoryGroupBulkMembersInput{
		ExpectedVersion: 2, AddUserIDs: []string{directoryTestOther, directoryTestUser, directoryTestUser}, Reason: "Add project members",
	})
	if err != nil || item.Version != 3 || len(store.membersInput.AddUserIDs) != 2 {
		t.Fatalf("group=%#v input=%#v err=%v", item, store.membersInput, err)
	}
	_, err = service.ChangeGroupMembers(context.Background(), directoryTestOrg, directoryTestGroup, directoryTestActor, models.DirectoryGroupBulkMembersInput{
		ExpectedVersion: 2, AddUserIDs: []string{directoryTestUser}, RemoveUserIDs: []string{directoryTestUser}, Reason: "Conflicting membership",
	})
	if !errors.Is(err, ErrUserAdministrationInvalid) {
		t.Fatalf("overlap error=%v", err)
	}
}

func TestUserAdministrationCSVPreviewAndIdempotentApplyContracts(t *testing.T) {
	store := &directoryStoreStub{}
	service := NewUserAdministrationService(store, zerolog.Nop())
	csv := []byte("email,first_name,last_name,department,role_slug\nADA@example.test,Ada,Lovelace,Engineering,viewer\n")
	preview, err := service.PreviewImport(context.Background(), directoryTestOrg, csv)
	if err != nil || preview.RowCount != 1 || len(store.previewRows) != 1 || store.previewRows[0].Email != "ada@example.test" {
		t.Fatalf("preview=%#v rows=%#v err=%v", preview, store.previewRows, err)
	}
	result, err := service.ApplyImport(context.Background(), directoryTestOrg, directoryTestActor, "directory-import-001", "Quarterly directory sync", csv)
	if err != nil || result.RowCount != 1 || store.applyKey != "directory-import-001" || len(store.applyHash) != 64 {
		t.Fatalf("result=%#v key=%q hash=%q err=%v", result, store.applyKey, store.applyHash, err)
	}
	_, err = service.PreviewImport(context.Background(), directoryTestOrg, []byte("email,unknown\na@example.test,value\n"))
	if !errors.Is(err, ErrUserAdministrationInvalid) {
		t.Fatalf("unknown header error=%v", err)
	}
	tooMany := "email,first_name\n" + strings.Repeat("user@example.test,User\n", maximumDirectoryImportRows+1)
	_, err = service.PreviewImport(context.Background(), directoryTestOrg, []byte(tooMany))
	if !errors.Is(err, ErrUserAdministrationInvalid) {
		t.Fatalf("oversized rows error=%v", err)
	}
}
