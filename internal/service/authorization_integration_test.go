package service_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/service"
)

// TestRBACAuthorizerWithNonSuperuserTenantContext is opt-in because it creates
// a temporary PostgreSQL NOLOGIN role. TEST_DATABASE_URL must identify a
// migrated database principal with CREATEROLE for this test.
func TestRBACAuthorizerWithNonSuperuserTenantContext(t *testing.T) {
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
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, userB, superUser := uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleA, roleB := uuid.NewString(), uuid.NewString()
	databaseRole := "grc_authz_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{databaseRole}.Sanitize()

	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("creating non-superuser role (TEST_DATABASE_URL needs CREATEROLE): %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}()

	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Authz A',$3,'active','starter'),($2,'Authz B',$4,'active','starter')`, orgA, orgB, "authz-a-"+orgA, "authz-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,organization_id,email,status,is_super_admin) VALUES
		($1,$2,$3,'active',false),($4,$5,$6,'active',false),($7,$2,$8,'active',true)`,
		userA, orgA, userA+"@test.invalid", userB, orgB, userB+"@test.invalid", superUser, superUser+"@test.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO roles (id,organization_id,name,slug,is_custom) VALUES
		($1,$3,'Tenant A reader',$5,true),($2,$4,'Tenant B reader',$6,true)`, roleA, roleB, orgA, orgB, "reader-"+roleA, "reader-"+roleB); err != nil {
		t.Fatal(err)
	}
	var permissionID string
	if err := pool.QueryRow(ctx, `INSERT INTO permissions (resource,action,description)
		VALUES ('controls','read','Authorization integration test')
		ON CONFLICT (resource,action) DO UPDATE SET description=permissions.description
		RETURNING id`).Scan(&permissionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions (role_id,permission_id) VALUES ($1,$3),($2,$3)`, roleA, roleB, permissionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id,role_id,organization_id) VALUES ($1,$2,$3),($4,$5,$6)`, userA, roleA, orgA, userB, roleB, orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quotedRole+"; GRANT SELECT ON users,user_roles,roles,role_permissions,permissions TO "+quotedRole); err != nil {
		t.Fatal(err)
	}

	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = connection.Exec(context.Background(), "RESET ROLE") }()
	authorizer, err := service.NewRBACAuthorizer(pool)
	if err != nil {
		t.Fatal(err)
	}

	authorize := func(tenantID, subjectID, requestOrg, resource, action string) authz.Decision {
		t.Helper()
		if _, err := connection.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, tenantID); err != nil {
			t.Fatal(err)
		}
		decision, err := authorizer.Authorize(database.WithQuerier(ctx, connection), authz.Request{SubjectID: subjectID, OrganizationID: requestOrg, Resource: resource, Action: action})
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}

	if decision := authorize(orgA, userA, orgA, "controls", "read"); !decision.Allowed {
		t.Fatalf("reader denied: %#v", decision)
	}
	if decision := authorize(orgA, userA, orgA, "controls", "update"); decision.Allowed {
		t.Fatalf("ungranted update allowed: %#v", decision)
	}
	if decision := authorize(orgA, userB, orgB, "controls", "read"); decision.Allowed {
		t.Fatalf("request organization escaped connection tenant: %#v", decision)
	}
	if decision := authorize(orgB, userB, orgB, "controls", "read"); !decision.Allowed {
		t.Fatalf("tenant B reader denied: %#v", decision)
	}
	if decision := authorize(orgA, superUser, orgA, "controls", "delete"); !decision.Allowed {
		t.Fatalf("super-admin override denied: %#v", decision)
	}

	if _, err := connection.Exec(ctx, `SELECT set_config('app.current_tenant','',false)`); err != nil {
		t.Fatal(fmt.Errorf("clearing tenant: %w", err))
	}
}
