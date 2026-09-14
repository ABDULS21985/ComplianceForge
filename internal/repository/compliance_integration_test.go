package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

// TestComplianceRepositoriesRejectCrossTenantAccess runs against a fully
// migrated PostgreSQL database when TEST_DATABASE_URL is set. It verifies both
// catalog visibility and tenant-owned implementation/evidence boundaries.
func TestComplianceRepositoriesRejectCrossTenantAccess(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgA, orgB, userA := uuid.NewString(), uuid.NewString(), uuid.NewString()
	seedTenant := func(orgID, userID string) {
		t.Helper()
		err := database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
			q := database.QuerierFromContext(tenantCtx, pool)
			if _, err := q.Exec(tenantCtx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES ($1,$2,$3,'active','starter')`, orgID, "Compliance Test", "compliance-"+orgID); err != nil {
				return err
			}
			if userID != "" {
				_, err := q.Exec(tenantCtx, `INSERT INTO users (id,organization_id,email,status) VALUES ($1,$2,$3,'active')`, userID, orgID, userID+"@example.test")
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("seeding tenant %s: %v", orgID, err)
		}
	}
	seedTenant(orgA, userA)
	seedTenant(orgB, "")
	defer func() {
		for _, orgID := range []string{orgA, orgB} {
			_ = database.WithTenantConnection(context.Background(), pool, orgID, func(cleanupCtx context.Context) error {
				_, err := database.QuerierFromContext(cleanupCtx, pool).Exec(cleanupCtx, `DELETE FROM organizations WHERE id=$1`, orgID)
				return err
			})
		}
	}()

	frameworkID, controlID := uuid.NewString(), uuid.NewString()
	frameworkRepo := repository.NewFrameworkRepository(pool)
	controlRepo := repository.NewControlRepository(pool)
	withTenant := func(orgID string, fn func(context.Context) error) {
		t.Helper()
		if err := database.WithTenantConnection(ctx, pool, orgID, fn); err != nil {
			t.Fatal(err)
		}
	}
	withTenant(orgA, func(tenantCtx context.Context) error {
		q := database.QuerierFromContext(tenantCtx, pool)
		if _, err := q.Exec(tenantCtx, `INSERT INTO compliance_frameworks (id,organization_id,code,name,version,category) VALUES ($1,$2,'TENANT_TEST','Tenant Test','1','security')`, frameworkID, orgA); err != nil {
			return err
		}
		if _, err := q.Exec(tenantCtx, `INSERT INTO framework_controls (id,framework_id,code,title) VALUES ($1,$2,'T.1','Tenant-only control')`, controlID, frameworkID); err != nil {
			return err
		}
		if _, err := frameworkRepo.Adopt(tenantCtx, orgA, userA, frameworkID); err != nil {
			return err
		}
		framework, err := frameworkRepo.GetByID(tenantCtx, orgA, frameworkID)
		if err != nil || framework.Adoption == nil {
			return fmt.Errorf("getting adopted framework: framework=%v err=%v", framework, err)
		}
		if controls, total, err := controlRepo.ListByFramework(tenantCtx, orgA, frameworkID, models.PaginationRequest{Page: 1, PageSize: 20}); err != nil || total != 1 || len(controls) != 1 || controls[0].Implementation == nil {
			return fmt.Errorf("listing framework controls: total=%d items=%d err=%v", total, len(controls), err)
		}
		maturity := 3
		implementation, err := controlRepo.UpdateImplementation(tenantCtx, orgA, controlID, models.ControlImplementationPatch{MaturityLevel: &maturity})
		if err != nil || implementation.MaturityLevel != maturity {
			return fmt.Errorf("updating own implementation: maturity=%v err=%v", implementation, err)
		}
		evidence, err := controlRepo.AttachEvidence(tenantCtx, orgA, userA, controlID, models.AttachControlEvidenceInput{Title: "Tenant A policy", EvidenceType: "policy"})
		if err != nil || evidence.OrganizationID != orgA {
			return fmt.Errorf("attaching own evidence: evidence=%v err=%v", evidence, err)
		}
		items, total, err := controlRepo.ListEvidence(tenantCtx, orgA, controlID, models.PaginationRequest{Page: 1, PageSize: 20})
		if err != nil || total != 1 || len(items) != 1 {
			return fmt.Errorf("listing own evidence: total=%d items=%d err=%v", total, len(items), err)
		}
		return nil
	})

	withTenant(orgB, func(tenantCtx context.Context) error {
		if _, err := frameworkRepo.GetByID(tenantCtx, orgB, frameworkID); !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cross-tenant framework read error=%v, want no rows", err)
		}
		if _, err := controlRepo.GetAdoptedByID(tenantCtx, orgB, controlID); !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cross-tenant control read error=%v, want no rows", err)
		}
		maturity := 5
		if _, err := controlRepo.UpdateImplementation(tenantCtx, orgB, controlID, models.ControlImplementationPatch{MaturityLevel: &maturity}); !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cross-tenant update error=%v, want no rows", err)
		}
		if _, err := controlRepo.AttachEvidence(tenantCtx, orgB, uuid.NewString(), controlID, models.AttachControlEvidenceInput{Title: "Intrusion", EvidenceType: "document"}); !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cross-tenant evidence error=%v, want no rows", err)
		}
		return nil
	})
}
