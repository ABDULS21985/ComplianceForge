package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

type failingSCIMOutbox struct{}

func (failingSCIMOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced SCIM outbox failure")
}

// TestSCIMRepositoryWithNonSuperuserTenants exercises the real schema through
// a NOSUPERUSER/NOBYPASSRLS role. It covers the pre-tenant credential boundary,
// token rotation/replay, tenant isolation, case-insensitive uniqueness,
// concurrent optimistic updates, membership tenant checks, offboarding, and
// transactional audit/outbox rollback.
func TestSCIMRepositoryWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer adminPool.Close()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	adminA, backupAdminA, adminB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleName := "grc_scim_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	if _, err = adminPool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'SCIM Tenant A',$3,'active','unlimited'),($2,'SCIM Tenant B',$4,'active','unlimited')`,
		orgA, orgB, "scim-a-"+orgA, "scim-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status,
		is_super_admin,invitation_status,email_verified_at) VALUES
		($1,$2,$3,'Admin','A','active',true,'accepted',NOW()),
		($4,$2,$5,'Backup','Admin','active',true,'accepted',NOW()),
		($6,$7,$8,'Admin','B','active',true,'accepted',NOW())`, adminA, orgA,
		adminA+"@example.test", backupAdminA, backupAdminA+"@example.test", adminB, orgB,
		adminB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO roles(id,organization_id,name,slug,is_custom,created_by,updated_by)
		VALUES(gen_random_uuid(),$1,'SCIM tenant A viewer','viewer',true,$2,$2),
		(gen_random_uuid(),$3,'SCIM tenant B viewer','viewer',true,$4,$4)`, orgA, adminA, orgB, adminB); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, "CREATE ROLE "+quotedRole+
		" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quotedRole+
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+quotedRole+
		"; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO "+quotedRole+
		"; REVOKE ALL ON scim_token_tenant_index FROM "+quotedRole); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		connection, acquireErr := adminPool.Acquire(cleanupCtx)
		if acquireErr == nil {
			_, _ = connection.Exec(cleanupCtx, `SET session_replication_role='replica'`)
			_, _ = connection.Exec(cleanupCtx, `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB})
			_, _ = connection.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
			_, _ = connection.Exec(cleanupCtx, `SET session_replication_role='origin'`)
			connection.Release()
		}
		_, _ = adminPool.Exec(cleanupCtx, "DROP OWNED BY "+quotedRole)
		_, _ = adminPool.Exec(cleanupCtx, "DROP ROLE "+quotedRole)
	}()

	roleConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	roleConfig.MaxConns = 12
	roleConfig.AfterConnect = func(connectCtx context.Context, connection *pgx.Conn) error {
		_, connectErr := connection.Exec(connectCtx, "SET ROLE "+quotedRole)
		return connectErr
	}
	rolePool, err := pgxpool.NewWithConfig(ctx, roleConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer rolePool.Close()
	outbox, err := queuepkg.NewPostgresOutbox(rolePool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	store, err := repository.NewSCIMRepository(rolePool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	scimService, err := service.NewSCIMService(store, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}

	issue, err := scimService.CreateToken(ctx, orgA, adminA, models.SCIMTokenCreateInput{
		Name: "Primary IdP", Scopes: []string{models.SCIMTokenScopeUsersRead, models.SCIMTokenScopeUsersWrite,
			models.SCIMTokenScopeGroupsRead, models.SCIMTokenScopeGroupsWrite}, Reason: "Connect enterprise identity provider",
	})
	if err != nil || issue.Credential == "" || issue.Token.TokenHash == "" {
		t.Fatalf("CreateToken()=%#v error=%v", issue, err)
	}
	principal, err := scimService.AuthenticateSCIMToken(ctx, issue.Credential, "192.0.2.10")
	if err != nil || principal.OrganizationID != orgA || principal.TokenID != issue.Token.ID {
		t.Fatalf("AuthenticateSCIMToken()=%#v error=%v", principal, err)
	}
	// The role cannot enumerate the global index directly; the exact-prefix
	// resolver remains callable through its SECURITY DEFINER boundary.
	if err := rolePool.QueryRow(ctx, `SELECT count(*) FROM scim_token_tenant_index`).Scan(new(int)); err == nil {
		t.Fatal("non-superuser directly enumerated SCIM tenant index")
	}
	var storedRaw bool
	if err := adminPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scim_tokens
		WHERE id=$1 AND token_hash=$2)`, issue.Token.ID, issue.Credential).Scan(&storedRaw); err != nil || storedRaw {
		t.Fatalf("raw credential storage check=%v error=%v", storedRaw, err)
	}

	userA, err := scimService.CreateUser(ctx, orgA, issue.Token.ID, adminA, scimTestUser("Ada@Example.Test", "ext-ada"))
	if err != nil || userA.UserName != "ada@example.test" || userA.Version != 1 || userA.Meta.Version != `W/"1"` {
		t.Fatalf("CreateUser()=%#v error=%v", userA, err)
	}
	if _, err := scimService.CreateUser(ctx, orgA, issue.Token.ID, adminA,
		scimTestUser("ADA@example.test", "ext-duplicate")); !errors.Is(err, service.ErrSCIMConflict) {
		t.Fatalf("case-insensitive duplicate error=%v", err)
	}
	var createWait sync.WaitGroup
	var createMutex sync.Mutex
	createSuccesses, createConflicts := 0, 0
	for index := range 2 {
		createWait.Add(1)
		go func(index int) {
			defer createWait.Done()
			_, createErr := scimService.CreateUser(ctx, orgA, issue.Token.ID, adminA,
				scimTestUser("concurrent@example.test", fmt.Sprintf("ext-concurrent-%d", index)))
			createMutex.Lock()
			defer createMutex.Unlock()
			switch {
			case createErr == nil:
				createSuccesses++
			case errors.Is(createErr, service.ErrSCIMConflict):
				createConflicts++
			default:
				t.Errorf("concurrent SCIM create error=%v", createErr)
			}
		}(index)
	}
	createWait.Wait()
	if createSuccesses != 1 || createConflicts != 1 {
		t.Fatalf("concurrent username results successes=%d conflicts=%d", createSuccesses, createConflicts)
	}
	userBToken, err := scimService.CreateToken(ctx, orgB, adminB, models.SCIMTokenCreateInput{
		Name: "Tenant B IdP", Scopes: []string{models.SCIMTokenScopeUsersRead, models.SCIMTokenScopeUsersWrite,
			models.SCIMTokenScopeGroupsRead, models.SCIMTokenScopeGroupsWrite}, Reason: "Connect second identity provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	userB, err := scimService.CreateUser(ctx, orgB, userBToken.Token.ID, adminB,
		scimTestUser("grace@example.test", "ext-grace"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scimService.GetUser(ctx, orgB, userA.ID); !errors.Is(err, service.ErrSCIMNotFound) {
		t.Fatalf("cross-tenant user read error=%v", err)
	}
	if _, err := scimService.CreateGroup(ctx, orgA, issue.Token.ID, adminA, &models.SCIMGroup{
		Schemas: []string{models.SCIMGroupSchema}, DisplayName: "Cross Tenant", Members: []models.SCIMMember{{Value: userB.ID}},
	}); !errors.Is(err, service.ErrSCIMConflict) {
		t.Fatalf("cross-tenant group member error=%v", err)
	}

	group, err := scimService.CreateGroup(ctx, orgA, issue.Token.ID, adminA, &models.SCIMGroup{
		Schemas: []string{models.SCIMGroupSchema}, ExternalID: "ext-auditors", DisplayName: "Auditors",
		Members: []models.SCIMMember{{Value: userA.ID}},
	})
	if err != nil || group.Version != 1 || len(group.Members) != 1 {
		t.Fatalf("CreateGroup()=%#v error=%v", group, err)
	}

	base := *userA
	base.DisplayName = "Ada Lovelace"
	var successes int
	var wait sync.WaitGroup
	var mutex sync.Mutex
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			copy := base
			if _, updateErr := scimService.ReplaceUser(ctx, orgA, userA.ID, issue.Token.ID, adminA,
				userA.Version, &copy); updateErr == nil {
				mutex.Lock()
				successes++
				mutex.Unlock()
			}
		}()
	}
	wait.Wait()
	if successes != 1 {
		t.Fatalf("concurrent optimistic user updates succeeded=%d, want 1", successes)
	}
	userA, err = scimService.GetUser(ctx, orgA, userA.ID)
	if err != nil || userA.Version != 2 {
		t.Fatalf("updated user=%#v error=%v", userA, err)
	}

	sessionHashSeed := strings.ReplaceAll(uuid.NewString(), "-", "")
	apiKeyHashSeed := strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = adminPool.Exec(ctx, `INSERT INTO user_sessions(id,user_id,organization_id,token_hash,
		refresh_token_hash,expires_at) VALUES(gen_random_uuid(),$1,$2,$3,$4,NOW()+INTERVAL '1 hour')`,
		userA.ID, orgA, sessionHashSeed+sessionHashSeed, apiKeyHashSeed+apiKeyHashSeed); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO api_keys(organization_id,name,key_prefix,key_hash,
		permissions,created_by) VALUES($1,'SCIM offboard key',$2,$3,ARRAY['read:risks'],$4)`, orgA,
		"scim"+userA.ID[:6], strings.Repeat(apiKeyHashSeed[:16], 4), userA.ID); err != nil {
		t.Fatal(err)
	}
	userA.Active = false
	userA, err = scimService.ReplaceUser(ctx, orgA, userA.ID, issue.Token.ID, adminA, userA.Version, userA)
	if err != nil || userA.Active || userA.Version != 3 {
		t.Fatalf("deprovisioned user=%#v error=%v", userA, err)
	}
	var activeSessions, activeKeys, activeRoles, activeMemberships int
	if err = adminPool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM user_sessions WHERE organization_id=$1 AND user_id=$2 AND revoked_at IS NULL),
		(SELECT count(*) FROM api_keys WHERE organization_id=$1 AND created_by=$2 AND is_active),
		(SELECT count(*) FROM user_roles WHERE organization_id=$1 AND user_id=$2),
		(SELECT count(*) FROM directory_group_memberships WHERE organization_id=$1 AND user_id=$2 AND removed_at IS NULL)`,
		orgA, userA.ID).Scan(&activeSessions, &activeKeys, &activeRoles, &activeMemberships); err != nil ||
		activeSessions+activeKeys+activeRoles+activeMemberships != 0 {
		t.Fatalf("offboarding residue sessions=%d keys=%d roles=%d memberships=%d error=%v",
			activeSessions, activeKeys, activeRoles, activeMemberships, err)
	}

	rotated, err := scimService.RotateToken(ctx, orgA, issue.Token.ID, adminA, models.SCIMTokenRotateInput{
		ExpectedVersion: issue.Token.Version, Reason: "Scheduled SCIM credential rotation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scimService.AuthenticateSCIMToken(ctx, issue.Credential, "192.0.2.11"); !errors.Is(err, authdomain.ErrInvalidSCIMToken) {
		t.Fatalf("rotated credential replay error=%v", err)
	}
	if _, err := scimService.AuthenticateSCIMToken(ctx, rotated.Credential, "192.0.2.12"); err != nil {
		t.Fatalf("rotated credential authentication error=%v", err)
	}

	failingStore, err := repository.NewSCIMRepository(rolePool, failingSCIMOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	failingService, err := service.NewSCIMService(failingStore, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	_, err = failingService.CreateUser(ctx, orgA, rotated.Token.ID, adminA,
		scimTestUser("rollback@example.test", "ext-rollback"))
	if err == nil || !strings.Contains(err.Error(), "forced SCIM outbox failure") {
		t.Fatalf("forced SCIM outbox error=%v", err)
	}
	var rolledBack int
	if err = adminPool.QueryRow(ctx, `SELECT count(*) FROM users WHERE organization_id=$1
		AND email='rollback@example.test'`, orgA).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("SCIM rollback user count=%d error=%v", rolledBack, err)
	}

	if _, err = adminPool.Exec(ctx, `UPDATE users SET status='inactive',suspended_at=NOW(),
		suspended_by=$3::uuid,suspension_reason='SCIM token owner suspended'
		WHERE organization_id=$1::uuid AND id=$2::uuid`, orgA, adminA, backupAdminA); err != nil {
		t.Fatal(err)
	}
	if _, err := scimService.AuthenticateSCIMToken(ctx, rotated.Credential, "192.0.2.13"); !errors.Is(err, authdomain.ErrInvalidSCIMToken) {
		t.Fatalf("suspended token owner authentication error=%v", err)
	}

	var visible int
	err = database.WithTenantConnection(ctx, rolePool, orgB, func(scoped context.Context) error {
		return database.QuerierFromContext(scoped, rolePool).QueryRow(scoped,
			`SELECT count(*) FROM scim_tokens WHERE organization_id=$1::uuid`, orgA).Scan(&visible)
	})
	if err != nil || visible != 0 {
		t.Fatalf("cross-tenant SCIM token visibility=%d error=%v", visible, err)
	}
}

func scimTestUser(userName, externalID string) *models.SCIMUser {
	return &models.SCIMUser{Schemas: []string{models.SCIMUserSchema, models.SCIMEnterpriseUserSchema},
		ExternalID: externalID, UserName: userName, Active: true,
		Name:   models.SCIMName{GivenName: "Ada", FamilyName: "Lovelace"},
		Emails: []models.SCIMMultiValue{{Value: userName, Type: "work", Primary: true}},
		Enterprise: &models.SCIMEnterpriseUser{EmployeeNumber: "EMP-" + externalID,
			Department: "Security"}}
}
