package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

// TestAuthAndOrganizationRepositoriesAgainstMigratedPostgres is opt-in so the
// normal unit suite remains dependency-free. CI can set TEST_DATABASE_URL
// after applying migrations to exercise the real PostgreSQL enums, JSONB,
// arrays, role joins, and session rotation queries.
func TestAuthAndOrganizationRepositoriesAgainstMigratedPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("database ping error = %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO roles (id, organization_id, name, slug, description, is_system_role)
		VALUES ($1, NULL, 'Viewer', 'viewer', 'Integration-test viewer', true)
		ON CONFLICT ON CONSTRAINT uq_roles_org_slug DO NOTHING`, uuid.NewString()); err != nil {
		t.Fatalf("seeding viewer role: %v", err)
	}

	organizationRepo := repository.NewOrganizationRepository(pool)
	organization := &models.Organization{
		BaseModel:          models.BaseModel{ID: uuid.NewString()},
		Name:               "Repository Integration " + uuid.NewString(),
		Slug:               "repository-integration-" + uuid.NewString(),
		CountryCode:        "GB",
		Status:             "active",
		Tier:               "starter",
		Settings:           map[string]any{"test": true},
		Branding:           map[string]any{},
		SupportedLanguages: []string{"en"},
	}
	if err := organizationRepo.Create(ctx, organization); err != nil {
		t.Fatalf("OrganizationRepository.Create() error = %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, organization.ID)
	}()

	loadedOrganization, err := organizationRepo.GetBySlug(ctx, organization.Slug)
	if err != nil || loadedOrganization.ID != organization.ID {
		t.Fatalf("OrganizationRepository.GetBySlug() org = %#v, error = %v", loadedOrganization, err)
	}
	organization.LegalName = "Repository Integration Limited"
	if err := organizationRepo.Update(ctx, organization); err != nil {
		t.Fatalf("OrganizationRepository.Update() error = %v", err)
	}
	organizations, total, err := organizationRepo.List(ctx, models.PaginationRequest{Page: 1, PageSize: 20})
	if err != nil || total < 1 || len(organizations) < 1 {
		t.Fatalf("OrganizationRepository.List() count = %d/%d, error = %v", len(organizations), total, err)
	}

	userRepo := repository.NewUserRepository(pool)
	user := &models.User{
		TenantModel: models.TenantModel{
			BaseModel:      models.BaseModel{ID: uuid.NewString()},
			OrganizationID: organization.ID,
		},
		Email:        "repository-" + uuid.NewString() + "@example.com",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNO1234567890",
		FirstName:    "Repo",
		LastName:     "Test",
		Status:       models.UserStatusActive,
		Role:         models.UserRoleViewer,
		Language:     "en",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepository.Create() error = %v", err)
	}
	loadedUser, err := userRepo.GetByEmail(ctx, organization.ID, user.Email)
	if err != nil || loadedUser.ID != user.ID || loadedUser.Role != models.UserRoleViewer {
		t.Fatalf("UserRepository.GetByEmail() user = %#v, error = %v", loadedUser, err)
	}
	if _, err := userRepo.GetByID(ctx, organization.ID, user.ID); err != nil {
		t.Fatalf("UserRepository.GetByID() error = %v", err)
	}
	if err := userRepo.UpdateLastLogin(ctx, organization.ID, user.ID, time.Now()); err != nil {
		t.Fatalf("UserRepository.UpdateLastLogin() error = %v", err)
	}

	session := &models.UserSession{
		ID:               uuid.NewString(),
		UserID:           user.ID,
		OrganizationID:   organization.ID,
		TokenHash:        "access-hash-" + uuid.NewString(),
		RefreshTokenHash: "refresh-hash-" + uuid.NewString(),
		ExpiresAt:        time.Now().Add(time.Hour),
	}
	if err := userRepo.CreateSession(ctx, session); err != nil {
		t.Fatalf("UserRepository.CreateSession() error = %v", err)
	}
	active, err := userRepo.IsSessionActive(ctx, organization.ID, user.ID, session.TokenHash)
	if err != nil || !active {
		t.Fatalf("UserRepository.IsSessionActive() active = %v, error = %v", active, err)
	}

	nextSession := &models.UserSession{
		ID:               session.ID,
		UserID:           user.ID,
		OrganizationID:   organization.ID,
		TokenHash:        "next-access-hash-" + uuid.NewString(),
		RefreshTokenHash: "next-refresh-hash-" + uuid.NewString(),
		ExpiresAt:        time.Now().Add(2 * time.Hour),
	}
	rotated, err := userRepo.RotateSession(
		ctx,
		organization.ID,
		user.ID,
		session.RefreshTokenHash,
		nextSession,
	)
	if err != nil || !rotated {
		t.Fatalf("UserRepository.RotateSession() rotated = %v, error = %v", rotated, err)
	}
	if err := userRepo.RevokeSession(ctx, organization.ID, user.ID, nextSession.TokenHash); err != nil {
		t.Fatalf("UserRepository.RevokeSession() error = %v", err)
	}
	active, err = userRepo.IsSessionActive(ctx, organization.ID, user.ID, nextSession.TokenHash)
	if err != nil || active {
		t.Fatalf("revoked IsSessionActive() active = %v, error = %v", active, err)
	}
}
