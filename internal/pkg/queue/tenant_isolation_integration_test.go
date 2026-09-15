//go:build integration

package queue_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/repository"
)

func TestQueueStorageSplitRoleTenantIsolation(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL (or DATABASE_URL) is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatal(err)
	}

	var schemaVersion int64
	var dirty bool
	if err := adminPool.QueryRow(ctx, `SELECT version,dirty FROM schema_migrations`).Scan(&schemaVersion, &dirty); err != nil {
		adminPool.Close()
		t.Fatal(err)
	}
	if schemaVersion < 57 || dirty {
		adminPool.Close()
		t.Skipf("queue isolation integration requires clean schema >=57; got version=%d dirty=%t", schemaVersion, dirty)
	}

	for _, group := range []string{"complianceforge_api", "complianceforge_scheduler"} {
		var exists bool
		if err := adminPool.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=$1
		)`, group).Scan(&exists); err != nil {
			adminPool.Close()
			t.Fatal(err)
		}
		if !exists {
			adminPool.Close()
			t.Skipf("split-role proof requires runtime group %s", group)
		}
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	apiRole := "queue_api_" + suffix
	workerRole := "queue_worker_" + suffix
	apiPassword := "api-" + uuid.NewString()
	workerPassword := "worker-" + uuid.NewString()
	queueName := "queue.rls." + suffix
	consumerName := "queue-rls-" + suffix
	organizationA := uuid.NewString()
	organizationB := uuid.NewString()

	quotedAPI := pgx.Identifier{apiRole}.Sanitize()
	quotedWorker := pgx.Identifier{workerRole}.Sanitize()
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(
		"CREATE ROLE %s LOGIN PASSWORD %s NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION INHERIT",
		quotedAPI, quotePostgresLiteral(apiPassword),
	)); err != nil {
		adminPool.Close()
		t.Fatal(err)
	}
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(
		"CREATE ROLE %s LOGIN PASSWORD %s NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION INHERIT",
		quotedWorker, quotePostgresLiteral(workerPassword),
	)); err != nil {
		_, _ = adminPool.Exec(context.Background(), "DROP ROLE "+quotedAPI)
		adminPool.Close()
		t.Fatal(err)
	}

	var apiPool, workerPool, ownerMemberPool *pgxpool.Pool
	testRoles := []string{quotedAPI, quotedWorker}
	t.Cleanup(func() {
		if apiPool != nil {
			apiPool.Close()
		}
		if workerPool != nil {
			workerPool.Close()
		}
		if ownerMemberPool != nil {
			ownerMemberPool.Close()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, cleanupErr := adminPool.Exec(cleanupCtx,
			`DELETE FROM queue_inbox WHERE consumer_name=$1`, consumerName); cleanupErr != nil {
			t.Errorf("clean queue inbox fixtures: %v", cleanupErr)
		}
		if _, cleanupErr := adminPool.Exec(cleanupCtx,
			`DELETE FROM queue_outbox WHERE queue_name=$1`, queueName); cleanupErr != nil {
			t.Errorf("clean queue outbox fixtures: %v", cleanupErr)
		}
		for _, role := range testRoles {
			if _, cleanupErr := adminPool.Exec(cleanupCtx, "DROP OWNED BY "+role); cleanupErr != nil {
				t.Errorf("drop queue test role privileges for %s: %v", role, cleanupErr)
				continue
			}
			if _, cleanupErr := adminPool.Exec(cleanupCtx, "DROP ROLE "+role); cleanupErr != nil {
				t.Errorf("drop queue test role %s: %v", role, cleanupErr)
			}
		}
		adminPool.Close()
	})

	if _, err := adminPool.Exec(ctx, "GRANT complianceforge_api TO "+quotedAPI+
		"; GRANT complianceforge_scheduler TO "+quotedWorker); err != nil {
		t.Fatal(err)
	}
	// Diagnostics needs these non-queue reads. Queue privileges continue to be
	// inherited solely from the reviewed complianceforge_api group.
	if _, err := adminPool.Exec(ctx, "GRANT SELECT ON schema_migrations,notifications,integrations,integration_sync_logs TO "+quotedAPI); err != nil {
		t.Fatal(err)
	}

	apiPool = queueRolePool(t, ctx, databaseURL, apiRole, apiPassword)
	workerPool = queueRolePool(t, ctx, databaseURL, workerRole, workerPassword)

	assertQueueRolePosture(t, ctx, adminPool, apiRole, workerRole)
	assertQueueGroupPrivileges(t, ctx, adminPool)

	outboxConfig := queuepkg.DefaultOutboxConfig()
	outboxConfig.BatchSize = 10
	outboxConfig.Lease = 30 * time.Second
	outboxConfig.Retention = time.Hour
	apiOutbox, err := queuepkg.NewPostgresOutbox(apiPool, uuid.NewString(), outboxConfig)
	if err != nil {
		t.Fatal(err)
	}
	workerOutbox, err := queuepkg.NewPostgresOutbox(workerPool, uuid.NewString(), outboxConfig)
	if err != nil {
		t.Fatal(err)
	}

	tenantEnvelope, err := queuepkg.NewEnvelope("queue.rls.tenant", organizationA, map[string]string{"scope": "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithTenantConnection(ctx, apiPool, organizationA, func(scopedContext context.Context) error {
		executor := database.QuerierFromContext(scopedContext, nil)
		if err := apiOutbox.Enqueue(scopedContext, executor, queueName, tenantEnvelope); err != nil {
			return err
		}
		return apiOutbox.Enqueue(scopedContext, executor, queueName, tenantEnvelope)
	})
	if err != nil {
		t.Fatalf("same-tenant idempotent API enqueue: %v", err)
	}

	crossTenantEnvelope, err := queuepkg.NewEnvelope("queue.rls.cross", organizationB, map[string]string{"scope": "tenant-b"})
	if err != nil {
		t.Fatal(err)
	}
	systemEnvelope, err := queuepkg.NewEnvelope("queue.rls.system", "", map[string]string{"scope": "system"})
	if err != nil {
		t.Fatal(err)
	}
	for name, envelope := range map[string]queuepkg.Envelope{
		"cross-tenant": crossTenantEnvelope,
		"system":       systemEnvelope,
	} {
		t.Run("api-rejects-"+name+"-enqueue", func(t *testing.T) {
			err := database.WithTenantConnection(ctx, apiPool, organizationA, func(scopedContext context.Context) error {
				return apiOutbox.Enqueue(scopedContext, database.QuerierFromContext(scopedContext, nil), queueName, envelope)
			})
			assertPostgresErrorCode(t, err, "42501")
		})
	}

	// RLS and the existing envelope constraint are independent defenses: even
	// a same-tenant row cannot carry a differently scoped serialized envelope.
	mismatchedMessageID := uuid.NewString()
	err = database.WithTenantConnection(ctx, apiPool, organizationA, func(scopedContext context.Context) error {
		_, insertErr := database.QuerierFromContext(scopedContext, nil).Exec(scopedContext, `
			INSERT INTO queue_outbox(
				message_id,queue_name,tenant_id,message_type,schema_version,
				correlation_id,envelope
			) VALUES(
				$1::uuid,$2,$3::uuid,'queue.rls.mismatch',1,$1,
				jsonb_build_object(
					'id',$1::text,'type','queue.rls.mismatch','schema_version',1,
					'tenant_id',$4::text,'correlation_id',$1::text,'attempt',1,
					'created_at',statement_timestamp(),'payload','{}'::jsonb
				)
			)`, mismatchedMessageID, queueName, organizationA, organizationB)
		return insertErr
	})
	assertPostgresErrorCode(t, err, "23514")

	if err := workerOutbox.Enqueue(ctx, workerPool, queueName, crossTenantEnvelope); err != nil {
		t.Fatalf("worker enqueue tenant B: %v", err)
	}
	if err := workerOutbox.Enqueue(ctx, workerPool, queueName, systemEnvelope); err != nil {
		t.Fatalf("worker enqueue system envelope: %v", err)
	}

	// A role membership policy would accidentally admit roles that can assume
	// the table owner. The owner fallback is stricter: both current_user and
	// session_user must be the exact owner, so SET ROLE cannot activate it.
	var queueOwner string
	var safeQueueOwner, ownerIsScheduler bool
	if err := adminPool.QueryRow(ctx, `SELECT owner.rolname,
		NOT owner.rolsuper AND NOT owner.rolbypassrls,
		pg_catalog.pg_has_role(owner.oid,scheduler.oid,'MEMBER')
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_roles AS owner ON owner.oid=relation.relowner
		JOIN pg_catalog.pg_roles AS scheduler ON scheduler.rolname='complianceforge_scheduler'
		WHERE relation.oid='queue_outbox'::regclass`).Scan(
		&queueOwner, &safeQueueOwner, &ownerIsScheduler,
	); err != nil {
		t.Fatal(err)
	}
	if !safeQueueOwner || ownerIsScheduler {
		t.Fatalf("queue owner %s is unsuitable for owner-membership proof: safe=%t scheduler=%t",
			queueOwner, safeQueueOwner, ownerIsScheduler)
	}
	ownerMemberRole := "queue_owner_member_" + suffix
	quotedOwnerMember := pgx.Identifier{ownerMemberRole}.Sanitize()
	ownerMemberPassword := "owner-member-" + uuid.NewString()
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(
		"CREATE ROLE %s LOGIN PASSWORD %s NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION INHERIT",
		quotedOwnerMember, quotePostgresLiteral(ownerMemberPassword),
	)); err != nil {
		t.Fatal(err)
	}
	testRoles = append(testRoles, quotedOwnerMember)
	if _, err := adminPool.Exec(ctx, "GRANT "+pgx.Identifier{queueOwner}.Sanitize()+" TO "+quotedOwnerMember); err != nil {
		t.Fatal(err)
	}
	ownerMemberPool = queueRolePool(t, ctx, databaseURL, ownerMemberRole, ownerMemberPassword)
	ownerMemberConnection, err := ownerMemberPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerMemberConnection.Release()
	for _, assumeOwner := range []bool{false, true} {
		if assumeOwner {
			if _, err := ownerMemberConnection.Exec(ctx, "SET ROLE "+pgx.Identifier{queueOwner}.Sanitize()); err != nil {
				t.Fatal(err)
			}
		}
		var visible int
		if err := ownerMemberConnection.QueryRow(ctx,
			`SELECT count(*) FROM queue_outbox WHERE queue_name=$1`, queueName).Scan(&visible); err != nil {
			t.Fatal(err)
		}
		if visible != 0 {
			t.Fatalf("owner member visibility after SET ROLE=%t is %d, want 0", assumeOwner, visible)
		}
	}
	if _, err := ownerMemberConnection.Exec(ctx, `RESET ROLE`); err != nil {
		t.Fatal(err)
	}
	visibleConflict := tenantEnvelope
	visibleConflict.Payload = []byte(`{"scope":"changed-visible"}`)
	var visibleConflictErr error
	visibleConflictErr = database.WithTenantConnection(ctx, apiPool, organizationA, func(scopedContext context.Context) error {
		return apiOutbox.Enqueue(scopedContext, database.QuerierFromContext(scopedContext, nil), queueName, visibleConflict)
	})
	if !errors.Is(visibleConflictErr, queuepkg.ErrOutboxConflict) {
		t.Fatalf("same-tenant conflicting enqueue error=%v, want opaque conflict", visibleConflictErr)
	}
	crossTenantCollision := crossTenantEnvelope
	crossTenantCollision.TenantID = organizationA
	crossTenantCollision.Payload = []byte(`{"scope":"changed-hidden"}`)
	var hiddenConflictErr error
	hiddenConflictErr = database.WithTenantConnection(ctx, apiPool, organizationA, func(scopedContext context.Context) error {
		return apiOutbox.Enqueue(scopedContext, database.QuerierFromContext(scopedContext, nil), queueName, crossTenantCollision)
	})
	if !errors.Is(hiddenConflictErr, queuepkg.ErrOutboxConflict) {
		t.Fatalf("cross-tenant duplicate enqueue error=%v, want opaque conflict", hiddenConflictErr)
	}

	deduplicator, err := queuepkg.NewPostgresDeduplicator(workerPool, queuepkg.PostgresDeduplicatorConfig{
		ConsumerName: consumerName,
		OwnerID:      uuid.NewString(),
		Lease:        30 * time.Second,
		Retention:    time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	inboxTenantA, err := queuepkg.NewEnvelope("queue.rls.inbox", organizationA, map[string]string{"scope": "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	inboxTenantB, err := queuepkg.NewEnvelope("queue.rls.inbox", organizationB, map[string]string{"scope": "tenant-b"})
	if err != nil {
		t.Fatal(err)
	}
	inboxSystem, err := queuepkg.NewEnvelope("queue.rls.inbox", "", map[string]string{"scope": "system"})
	if err != nil {
		t.Fatal(err)
	}
	for _, envelope := range []queuepkg.Envelope{inboxTenantA, inboxTenantB, inboxSystem} {
		if status, beginErr := deduplicator.Begin(ctx, envelope); beginErr != nil || status != queuepkg.DeduplicationNew {
			t.Fatalf("worker inbox Begin(%q) status=%v error=%v", envelope.TenantID, status, beginErr)
		}
	}

	err = database.WithTenantConnection(ctx, apiPool, organizationA, func(scopedContext context.Context) error {
		executor := database.QuerierFromContext(scopedContext, nil)
		var outboxRows, inboxRows int
		if err := executor.QueryRow(scopedContext,
			`SELECT count(*) FROM queue_outbox WHERE queue_name=$1`, queueName).Scan(&outboxRows); err != nil {
			return err
		}
		if err := executor.QueryRow(scopedContext,
			`SELECT count(*) FROM queue_inbox WHERE consumer_name=$1`, consumerName).Scan(&inboxRows); err != nil {
			return err
		}
		if outboxRows != 1 || inboxRows != 1 {
			return fmt.Errorf("API visible outbox=%d inbox=%d, want 1/1", outboxRows, inboxRows)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	diagnostics, err := repository.NewDiagnosticsRepository(apiPool)
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithTenantConnection(ctx, apiPool, organizationA, func(scopedContext context.Context) error {
		result, loadErr := diagnostics.LoadOperationalDiagnostics(scopedContext, organizationA)
		if loadErr != nil {
			return loadErr
		}
		if result.Queue.Pending != 1 || result.Queue.InboxProcessing != 1 {
			return fmt.Errorf("tenant diagnostics pending=%d inbox_processing=%d, want 1/1",
				result.Queue.Pending, result.Queue.InboxProcessing)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant queue diagnostics: %v", err)
	}

	// Prove RLS remains fail-closed even if an operator accidentally grants API
	// DML outside the reviewed matrix.
	if _, err := adminPool.Exec(ctx, "GRANT UPDATE,DELETE ON queue_outbox TO "+quotedAPI+
		"; GRANT INSERT,UPDATE,DELETE ON queue_inbox TO "+quotedAPI); err != nil {
		t.Fatal(err)
	}
	err = database.WithTenantConnection(ctx, apiPool, organizationA, func(scopedContext context.Context) error {
		executor := database.QuerierFromContext(scopedContext, nil)
		updateTag, err := executor.Exec(scopedContext,
			`UPDATE queue_outbox SET last_error='forged' WHERE queue_name=$1`, queueName)
		if err != nil {
			return err
		}
		deleteTag, err := executor.Exec(scopedContext,
			`DELETE FROM queue_outbox WHERE queue_name=$1`, queueName)
		if err != nil {
			return err
		}
		if updateTag.RowsAffected() != 0 || deleteTag.RowsAffected() != 0 {
			return fmt.Errorf("accidental outbox DML affected update=%d delete=%d",
				updateTag.RowsAffected(), deleteTag.RowsAffected())
		}
		_, err = executor.Exec(scopedContext, `INSERT INTO queue_inbox(
			consumer_name,message_id,delivery_attempt,tenant_id,message_type,envelope_hash,
			status,lease_owner,leased_until)
			VALUES('forged-api',gen_random_uuid(),1,$1::uuid,'forged',decode(repeat('aa',32),'hex'),
			'processing',gen_random_uuid(),statement_timestamp()+interval '1 minute')`, organizationA)
		return err
	})
	assertPostgresErrorCode(t, err, "42501")

	claimed, err := workerOutbox.ClaimBatch(ctx)
	if err != nil {
		t.Fatalf("worker claim outbox: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("worker claimed %d rows, want tenant A, tenant B, and system rows", len(claimed))
	}
	claimedTenants := make(map[string]bool, 3)
	for _, message := range claimed {
		claimedTenants[message.Envelope.TenantID] = true
		if err := workerOutbox.MarkPublished(ctx, message); err != nil {
			t.Fatalf("worker publish %s: %v", message.MessageID, err)
		}
	}
	if !claimedTenants[organizationA] || !claimedTenants[organizationB] || !claimedTenants[""] {
		t.Fatalf("worker claimed tenant set=%v", claimedTenants)
	}
	if _, err := workerPool.Exec(ctx,
		`UPDATE queue_outbox SET published_at=statement_timestamp()-interval '1 day' WHERE queue_name=$1`, queueName); err != nil {
		t.Fatal(err)
	}
	if purged, err := workerOutbox.PurgePublishedBefore(ctx, time.Now().Add(-time.Hour), 10); err != nil || purged != 3 {
		t.Fatalf("worker purge outbox=%d error=%v, want 3", purged, err)
	}

	for _, envelope := range []queuepkg.Envelope{inboxTenantA, inboxTenantB, inboxSystem} {
		if err := deduplicator.Complete(ctx, envelope); err != nil {
			t.Fatalf("worker inbox Complete(%q): %v", envelope.TenantID, err)
		}
	}
	if _, err := workerPool.Exec(ctx,
		`UPDATE queue_inbox SET expires_at=statement_timestamp()-interval '1 day' WHERE consumer_name=$1`, consumerName); err != nil {
		t.Fatal(err)
	}
	if purged, err := deduplicator.PurgeExpired(ctx, 10); err != nil || purged != 3 {
		t.Fatalf("worker purge inbox=%d error=%v, want 3", purged, err)
	}
}

func queueRolePool(t *testing.T, ctx context.Context, databaseURL, role, password string) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.User = role
	config.ConnConfig.Password = password
	config.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return pool
}

func assertQueueRolePosture(
	t *testing.T,
	ctx context.Context,
	adminPool *pgxpool.Pool,
	apiRole, workerRole string,
) {
	t.Helper()
	for role, expectedGroup := range map[string]string{
		apiRole:    "complianceforge_api",
		workerRole: "complianceforge_scheduler",
	} {
		var safe, member, ownsDatabase, ownsSchema, ownsQueue bool
		err := adminPool.QueryRow(ctx, `SELECT
			NOT runtime.rolsuper AND NOT runtime.rolbypassrls
			AND NOT runtime.rolcreatedb AND NOT runtime.rolcreaterole
			AND NOT runtime.rolreplication,
			pg_catalog.pg_has_role(runtime.oid, queue_group.oid, 'MEMBER'),
			pg_catalog.pg_has_role(runtime.oid, database.datdba, 'MEMBER'),
			pg_catalog.pg_has_role(runtime.oid, namespace.nspowner, 'MEMBER'),
			EXISTS (
				SELECT 1 FROM pg_catalog.pg_class AS relation
				WHERE relation.relname IN ('queue_outbox','queue_inbox')
				  AND pg_catalog.pg_has_role(runtime.oid, relation.relowner, 'MEMBER')
			)
		FROM pg_catalog.pg_roles AS runtime
		JOIN pg_catalog.pg_roles AS queue_group ON queue_group.rolname=$2
		JOIN pg_catalog.pg_database AS database ON database.datname=current_database()
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.nspname='public'
		WHERE runtime.rolname=$1`, role, expectedGroup).Scan(
			&safe, &member, &ownsDatabase, &ownsSchema, &ownsQueue,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !safe || !member || ownsDatabase || ownsSchema || ownsQueue {
			t.Fatalf("unsafe queue role %s: safe=%t member=%t db_owner=%t schema_owner=%t queue_owner=%t",
				role, safe, member, ownsDatabase, ownsSchema, ownsQueue)
		}
	}
}

func assertQueueGroupPrivileges(t *testing.T, ctx context.Context, adminPool *pgxpool.Pool) {
	t.Helper()
	var apiOutboxSelect, apiOutboxInsert, apiInboxSelect bool
	var apiOutboxUpdate, apiOutboxDelete, apiInboxInsert, apiInboxUpdate, apiInboxDelete bool
	var workerOutboxAll, workerInboxAll, publicAny bool
	var bothForced, policySetComplete, envelopeConstraintValidated bool
	err := adminPool.QueryRow(ctx, `SELECT
		has_table_privilege('complianceforge_api','queue_outbox','SELECT'),
		has_table_privilege('complianceforge_api','queue_outbox','INSERT'),
		has_table_privilege('complianceforge_api','queue_inbox','SELECT'),
		has_table_privilege('complianceforge_api','queue_outbox','UPDATE'),
		has_table_privilege('complianceforge_api','queue_outbox','DELETE'),
		has_table_privilege('complianceforge_api','queue_inbox','INSERT'),
		has_table_privilege('complianceforge_api','queue_inbox','UPDATE'),
		has_table_privilege('complianceforge_api','queue_inbox','DELETE'),
		has_table_privilege('complianceforge_scheduler','queue_outbox','SELECT,INSERT,UPDATE,DELETE'),
		has_table_privilege('complianceforge_scheduler','queue_inbox','SELECT,INSERT,UPDATE,DELETE'),
		has_table_privilege('public','queue_outbox','SELECT,INSERT,UPDATE,DELETE,TRUNCATE')
		OR has_table_privilege('public','queue_inbox','SELECT,INSERT,UPDATE,DELETE,TRUNCATE'),
		(SELECT bool_and(relation.relrowsecurity AND relation.relforcerowsecurity)
		 FROM pg_catalog.pg_class AS relation
		 WHERE relation.relname IN ('queue_outbox','queue_inbox')),
		(SELECT count(*)=7 FROM pg_catalog.pg_policy AS policy
		 WHERE policy.polrelid IN ('queue_outbox'::regclass,'queue_inbox'::regclass)),
		(SELECT constraint_record.convalidated
		 FROM pg_catalog.pg_constraint AS constraint_record
		 WHERE constraint_record.conrelid='queue_outbox'::regclass
		   AND constraint_record.conname='chk_queue_outbox_envelope')`).Scan(
		&apiOutboxSelect, &apiOutboxInsert, &apiInboxSelect,
		&apiOutboxUpdate, &apiOutboxDelete, &apiInboxInsert, &apiInboxUpdate, &apiInboxDelete,
		&workerOutboxAll, &workerInboxAll, &publicAny,
		&bothForced, &policySetComplete, &envelopeConstraintValidated,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !apiOutboxSelect || !apiOutboxInsert || !apiInboxSelect ||
		apiOutboxUpdate || apiOutboxDelete || apiInboxInsert || apiInboxUpdate || apiInboxDelete ||
		!workerOutboxAll || !workerInboxAll || publicAny ||
		!bothForced || !policySetComplete || !envelopeConstraintValidated {
		t.Fatalf("unexpected queue posture api outbox=%t/%t/%t/%t inbox=%t/%t/%t/%t worker=%t/%t public=%t forced=%t policies=%t envelope_constraint=%t",
			apiOutboxSelect, apiOutboxInsert, apiOutboxUpdate, apiOutboxDelete,
			apiInboxSelect, apiInboxInsert, apiInboxUpdate, apiInboxDelete,
			workerOutboxAll, workerInboxAll, publicAny,
			bothForced, policySetComplete, envelopeConstraintValidated)
	}
}

func assertPostgresErrorCode(t *testing.T, err error, expected string) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != expected {
		t.Fatalf("PostgreSQL error=%v, want SQLSTATE %s", err, expected)
	}
}

func quotePostgresLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
