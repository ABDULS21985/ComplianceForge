package repository_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestFrameworkCatalogRLSWithNonSuperuser verifies migration 000042 using a
// genuine NOSUPERUSER/NOBYPASSRLS database role. TEST_DATABASE_URL must point
// to a migrated database and have authority to create and SET ROLE.
func TestFrameworkCatalogRLSWithNonSuperuser(t *testing.T) {
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

	orgA, orgB := uuid.NewString(), uuid.NewString()
	frameworkA, frameworkB, globalFramework := uuid.NewString(), uuid.NewString(), uuid.NewString()
	domainA, domainB, globalDomain := uuid.NewString(), uuid.NewString(), uuid.NewString()
	controlA, controlB, globalControl := uuid.NewString(), uuid.NewString(), uuid.NewString()
	databaseRole := "grc_catalog_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{databaseRole}.Sanitize()

	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("creating non-superuser role: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM compliance_frameworks WHERE id=$1`, globalFramework)
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}()

	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Catalog RLS A',$3,'active','starter'),($2,'Catalog RLS B',$4,'active','starter')`, orgA, orgB, "catalog-a-"+orgA, "catalog-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO compliance_frameworks (id,organization_id,code,name,version,is_system_framework) VALUES
		($1,$2,$3,'Tenant A','1',false),($4,$5,$6,'Tenant B','1',false),($7,NULL,$8,'Global','1',true)`, frameworkA, orgA, "A_"+domainA, frameworkB, orgB, "B_"+domainB, globalFramework, "G_"+globalDomain); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO framework_domains (id,framework_id,code,name) VALUES ($1,$4,'A','A'),($2,$5,'B','B'),($3,$6,'G','G')`, domainA, domainB, globalDomain, frameworkA, frameworkB, globalFramework); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO framework_controls (id,framework_id,domain_id,code,title) VALUES ($1,$4,$7,'A.1','A'),($2,$5,$8,'B.1','B'),($3,$6,$9,'G.1','G')`, controlA, controlB, globalControl, frameworkA, frameworkB, globalFramework, domainA, domainB, globalDomain); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quotedRole+"; GRANT SELECT ON compliance_frameworks TO "+quotedRole+"; GRANT SELECT,INSERT,UPDATE,DELETE ON framework_domains,framework_controls TO "+quotedRole); err != nil {
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
	if _, err := connection.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgA); err != nil {
		t.Fatal(err)
	}

	assertVisibleIDs(t, ctx, connection, "framework_domains", []string{domainA, domainB, globalDomain}, 2)
	assertVisibleIDs(t, ctx, connection, "framework_controls", []string{controlA, controlB, globalControl}, 2)
	var hidden string
	if err := connection.QueryRow(ctx, `SELECT id FROM framework_controls WHERE id=$1`, controlB).Scan(&hidden); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant control read error=%v", err)
	}
	if tag, err := connection.Exec(ctx, `UPDATE framework_controls SET title='intrusion' WHERE id=$1`, controlB); err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("cross-tenant update rows=%d err=%v", tag.RowsAffected(), err)
	}
	if tag, err := connection.Exec(ctx, `UPDATE framework_controls SET title='owned' WHERE id=$1`, controlA); err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("own update rows=%d err=%v", tag.RowsAffected(), err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO framework_controls (framework_id,code,title) VALUES ($1,'B.2','intrusion')`, frameworkB); !isRLSDenial(err) {
		t.Fatalf("cross-tenant insert error=%v, want RLS denial", err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO framework_domains (framework_id,code,name) VALUES ($1,'G2','global intrusion')`, globalFramework); !isRLSDenial(err) {
		t.Fatalf("global framework mutation error=%v, want RLS denial", err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO framework_controls (framework_id,code,title) VALUES ($1,'A.2','owned')`, frameworkA); err != nil {
		t.Fatalf("own insert denied: %v", err)
	}

	if _, err := connection.Exec(ctx, `SELECT set_config('app.current_tenant','',false)`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func assertVisibleIDs(t *testing.T, ctx context.Context, q queryRower, table string, ids []string, want int) {
	t.Helper()
	var count int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE id=ANY($1::uuid[])", ids).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("visible %s=%d, want %d", table, count, want)
	}
}

func isRLSDenial(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "42501"
}
