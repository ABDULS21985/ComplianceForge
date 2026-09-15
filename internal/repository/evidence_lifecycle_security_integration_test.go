//go:build integration

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

// TestEvidenceLifecycleCapabilitiesAndLegalHoldSerialization exercises the
// migration-056 privilege boundary through a genuine NOSUPERUSER/NOBYPASSRLS
// role. It intentionally grants that role direct ledger DML to prove the exact
// owner RLS policies still reject it, while the narrow API capabilities work.
func TestEvidenceLifecycleCapabilitiesAndLegalHoldSerialization(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)

	var schemaVersion uint
	var dirty bool
	if err := adminPool.QueryRow(ctx, `SELECT version,dirty FROM schema_migrations`).Scan(&schemaVersion, &dirty); err != nil {
		t.Fatal(err)
	}
	if schemaVersion < 56 || dirty {
		t.Fatalf("migration 056 is required and must be clean: version=%d dirty=%t", schemaVersion, dirty)
	}

	orgID, userID := uuid.NewString(), uuid.NewString()
	frameworkID, controlID := uuid.NewString(), uuid.NewString()
	orgFrameworkID, implementationID := uuid.NewString(), uuid.NewString()
	evidenceID, holdID := uuid.NewString(), uuid.NewString()
	custodianHoldID, placementHoldID := uuid.NewString(), uuid.NewString()
	roleName := "grc_evidence_lifecycle_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()

	if _, err := adminPool.Exec(ctx, "CREATE ROLE "+quotedRole+
		" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		connection, acquireErr := adminPool.Acquire(cleanupCtx)
		if acquireErr != nil {
			t.Errorf("acquire fixture cleanup connection: %v", acquireErr)
			return
		}
		defer connection.Release()
		if _, cleanupErr := connection.Exec(cleanupCtx, `RESET ROLE`); cleanupErr != nil {
			t.Errorf("reset role before fixture cleanup: %v", cleanupErr)
			return
		}
		if _, cleanupErr := connection.Exec(cleanupCtx, `SELECT set_config('app.current_tenant',$1,false)`, orgID); cleanupErr != nil {
			t.Errorf("set fixture cleanup tenant: %v", cleanupErr)
			return
		}
		if _, cleanupErr := connection.Exec(cleanupCtx, `UPDATE legal_hold_records SET
			released_at=statement_timestamp(),released_by=$2,release_reason='Integration fixture cleanup'
			WHERE organization_id=$1 AND released_at IS NULL`, orgID, userID); cleanupErr != nil {
			t.Errorf("release fixture legal-hold records: %v", cleanupErr)
			return
		}
		if _, cleanupErr := connection.Exec(cleanupCtx, `UPDATE legal_hold_custodians SET
			released_at=statement_timestamp(),released_by=$2,release_reason='Integration fixture cleanup'
			WHERE organization_id=$1 AND released_at IS NULL`, orgID, userID); cleanupErr != nil {
			t.Errorf("release fixture legal-hold custodians: %v", cleanupErr)
			return
		}
		if _, cleanupErr := connection.Exec(cleanupCtx, `UPDATE legal_holds SET
			status='released',released_at=statement_timestamp(),released_by=$2,
			release_reason='Integration fixture cleanup',version=version+1
			WHERE organization_id=$1 AND status='active'`, orgID, userID); cleanupErr != nil {
			t.Errorf("release fixture legal holds: %v", cleanupErr)
			return
		}
		deleteTag, cleanupErr := connection.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=$1`, orgID)
		if cleanupErr != nil {
			t.Errorf("delete evidence lifecycle fixture organization: %v", cleanupErr)
			return
		}
		if deleteTag.RowsAffected() != 1 {
			t.Errorf("deleted fixture organizations=%d, want 1", deleteTag.RowsAffected())
			return
		}
		if _, cleanupErr = connection.Exec(cleanupCtx, `SELECT set_config('app.current_tenant','',false)`); cleanupErr != nil {
			t.Errorf("clear fixture cleanup tenant: %v", cleanupErr)
			return
		}
		if _, cleanupErr = connection.Exec(cleanupCtx, "DROP OWNED BY "+quotedRole); cleanupErr != nil {
			t.Errorf("drop evidence lifecycle test role privileges: %v", cleanupErr)
			return
		}
		if _, cleanupErr = connection.Exec(cleanupCtx, "DROP ROLE "+quotedRole); cleanupErr != nil {
			t.Errorf("drop evidence lifecycle test role: %v", cleanupErr)
		}
	})

	if _, err := adminPool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier)
		VALUES($1,'Evidence lifecycle security',$2,'active','enterprise')`, orgID, "evidence-lifecycle-"+orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminPool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status)
		VALUES($1,$2,$3,'Evidence','Reviewer','active')`, userID, orgID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := adminPool.Exec(ctx, `INSERT INTO compliance_frameworks(id,organization_id,code,name,version,is_active)
		VALUES($1,$2,$3,'Evidence lifecycle framework','1',true)`, frameworkID, orgID, "EL-"+frameworkID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := adminPool.Exec(ctx, `INSERT INTO framework_controls(id,framework_id,code,title)
		VALUES($1,$2,'EL.1','Evidence lifecycle control')`, controlID, frameworkID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminPool.Exec(ctx, `INSERT INTO organization_frameworks(id,organization_id,framework_id,status)
		VALUES($1,$2,$3,'not_started')`, orgFrameworkID, orgID, frameworkID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminPool.Exec(ctx, `INSERT INTO control_implementations(
		id,organization_id,framework_control_id,org_framework_id,status)
		VALUES($1,$2,$3,$4,'not_implemented')`, implementationID, orgID, controlID, orgFrameworkID); err != nil {
		t.Fatal(err)
	}

	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON control_evidence,legal_holds,legal_hold_records,legal_hold_custodians TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON evidence_reviews,evidence_custody_events,evidence_custody_chain_heads TO " + quotedRole +
		"; GRANT EXECUTE ON FUNCTION submit_evidence_review(UUID,UUID,UUID,UUID,TEXT,TEXT,TEXT) TO " + quotedRole +
		"; GRANT EXECUTE ON FUNCTION append_evidence_access_custody_event(UUID,UUID,UUID,UUID,TEXT,TEXT,TEXT) TO " + quotedRole +
		"; GRANT EXECUTE ON FUNCTION verify_evidence_custody_chain(UUID,UUID) TO " + quotedRole
	if _, err := adminPool.Exec(ctx, grant); err != nil {
		t.Fatal(err)
	}

	connection, err := adminPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), `SELECT set_config('app.current_tenant','',false)`)
		_, _ = connection.Exec(context.Background(), `RESET ROLE`)
	}()
	if _, err := connection.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgID); err != nil {
		t.Fatal(err)
	}

	var chainOwnerSafe, registryOwnerSafe, callerCanAssumeOwner bool
	if err := connection.QueryRow(ctx, `SELECT
		(SELECT NOT rolcanlogin AND NOT rolsuper AND NOT rolbypassrls
		        AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolreplication AND NOT rolinherit
		 FROM pg_roles WHERE rolname='complianceforge_evidence_chain_owner'),
		(SELECT NOT rolcanlogin AND NOT rolsuper AND NOT rolbypassrls
		        AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolreplication AND NOT rolinherit
		 FROM pg_roles WHERE rolname='complianceforge_evidence_registry_owner'),
		pg_has_role(current_user,'complianceforge_evidence_chain_owner','SET')`).Scan(
		&chainOwnerSafe, &registryOwnerSafe, &callerCanAssumeOwner,
	); err != nil {
		t.Fatal(err)
	}
	if !chainOwnerSafe || !registryOwnerSafe || callerCanAssumeOwner {
		t.Fatalf("capability role safety chain=%t registry=%t caller_member=%t",
			chainOwnerSafe, registryOwnerSafe, callerCanAssumeOwner)
	}
	var trustedFunctionCount int
	if err := connection.QueryRow(ctx, `SELECT count(*) FROM pg_proc function
		JOIN pg_roles owner ON owner.oid=function.proowner
		WHERE function.proname=ANY($1::text[]) AND function.prosecdef
		  AND owner.rolname='complianceforge_evidence_chain_owner'`, []string{
		"append_evidence_custody_event_chain", "record_control_evidence_upload",
		"record_control_evidence_transition", "validate_evidence_review", "project_evidence_review",
		"submit_evidence_review", "append_evidence_access_custody_event", "expire_due_evidence",
		"record_evidence_legal_hold_event",
	}).Scan(&trustedFunctionCount); err != nil {
		t.Fatal(err)
	}
	if trustedFunctionCount != 9 {
		t.Fatalf("trusted chain-owner function count=%d, want 9", trustedFunctionCount)
	}

	if _, err := connection.Exec(ctx, `INSERT INTO control_evidence(
		id,organization_id,control_implementation_id,title,evidence_type,file_name,
		file_size_bytes,mime_type,file_hash,collection_method,collected_by,valid_until)
		VALUES($1,$2,$3,'Capability evidence','document','capability.pdf',128,
		'application/pdf',$4,'manual_upload',$5,CURRENT_DATE+30)`,
		evidenceID, orgID, implementationID, strings.Repeat("a", 64), userID); err != nil {
		t.Fatalf("trusted upload trigger: %v", err)
	}
	var uploadedEvents int
	if err := connection.QueryRow(ctx, `SELECT count(*) FROM evidence_custody_events
		WHERE organization_id=$1 AND evidence_id=$2 AND event_type='uploaded'`, orgID, evidenceID).Scan(&uploadedEvents); err != nil || uploadedEvents != 1 {
		t.Fatalf("uploaded event count=%d error=%v", uploadedEvents, err)
	}

	// User triggers normally canonicalize this projection before the constraint
	// runs. Disable only triggers on an isolated superuser connection to prove
	// the database CHECK itself rejects both possible NULL-half states.
	constraintConnection, err := adminPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := constraintConnection.Exec(ctx, `SET session_replication_role='replica'`); err != nil {
		constraintConnection.Release()
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE control_evidence SET expires_at=NULL WHERE organization_id=$1 AND id=$2`,
		`UPDATE control_evidence SET valid_until=NULL WHERE organization_id=$1 AND id=$2`,
	} {
		_, projectionErr := constraintConnection.Exec(ctx, statement, orgID, evidenceID)
		var postgresError *pgconn.PgError
		if !errors.As(projectionErr, &postgresError) || postgresError.Code != "23514" ||
			postgresError.ConstraintName != "chk_control_evidence_expiry_projection" {
			_, _ = constraintConnection.Exec(context.Background(), `SET session_replication_role='origin'`)
			constraintConnection.Release()
			t.Fatalf("expiry projection error=%v, want chk_control_evidence_expiry_projection SQLSTATE 23514", projectionErr)
		}
	}
	if _, err := constraintConnection.Exec(ctx, `SET session_replication_role='origin'`); err != nil {
		constraintConnection.Release()
		t.Fatal(err)
	}
	constraintConnection.Release()

	var reviewID string
	if err := connection.QueryRow(ctx, `SELECT review_id FROM submit_evidence_review(
		$1,$2,$3,$4,'accepted','Fingerprint and provenance verified','review-capability-1')`,
		orgID, controlID, evidenceID, userID).Scan(&reviewID); err != nil {
		t.Fatalf("submit review capability: %v", err)
	}
	var accessEventID string
	if err := connection.QueryRow(ctx, `SELECT event_id FROM append_evidence_access_custody_event(
		$1,$2,$3,$4,'download_authorized','download-capability-1','private_stream')`,
		orgID, controlID, evidenceID, userID).Scan(&accessEventID); err != nil {
		t.Fatalf("append access capability: %v", err)
	}
	if reviewID == "" || accessEventID == "" {
		t.Fatalf("empty capability identifiers review=%q access=%q", reviewID, accessEventID)
	}
	var valid bool
	var eventCount, headSequence int64
	if err := connection.QueryRow(ctx, `SELECT valid,event_count,head_sequence
		FROM verify_evidence_custody_chain($1,$2)`, orgID, evidenceID).Scan(
		&valid, &eventCount, &headSequence,
	); err != nil || !valid || eventCount != 3 || headSequence != eventCount {
		t.Fatalf("custody verification valid=%t events=%d head=%d error=%v", valid, eventCount, headSequence, err)
	}

	assertRLSViolation := func(statement string, arguments ...any) {
		t.Helper()
		_, execErr := connection.Exec(ctx, statement, arguments...)
		var postgresError *pgconn.PgError
		if !errors.As(execErr, &postgresError) || postgresError.Code != "42501" {
			t.Fatalf("direct ledger DML error=%v, want SQLSTATE 42501", execErr)
		}
	}
	assertRLSViolation(`INSERT INTO evidence_reviews(
		organization_id,evidence_id,decision,reviewer_id)
		VALUES($1,$2,'accepted',$3)`, orgID, evidenceID, userID)
	assertRLSViolation(`INSERT INTO evidence_custody_events(
		organization_id,evidence_id,series_id,event_type,actor_user_id,actor_type,reason,details)
		VALUES($1,$2,$2,'legal_hold_released',$3,'user','Forged direct event','{}')`,
		orgID, evidenceID, userID)
	tag, err := connection.Exec(ctx, `UPDATE evidence_custody_chain_heads
		SET last_sequence=999 WHERE organization_id=$1 AND evidence_id=$2`, orgID, evidenceID)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("direct head update affected=%d error=%v", tag.RowsAffected(), err)
	}
	if _, err := connection.Exec(ctx, `SELECT event_id FROM append_evidence_access_custody_event(
		$1,$2,$3,$4,'download_authorized','missing-mode',NULL)`, orgID, controlID, evidenceID, userID); err == nil {
		t.Fatal("download capability accepted a null delivery mode")
	}

	assertSQLState := func(expectedCode, statement string, arguments ...any) {
		t.Helper()
		_, execErr := connection.Exec(ctx, statement, arguments...)
		var postgresError *pgconn.PgError
		if !errors.As(execErr, &postgresError) || postgresError.Code != expectedCode {
			t.Fatalf("statement error=%v, want SQLSTATE %s", execErr, expectedCode)
		}
	}

	if _, err := connection.Exec(ctx, `INSERT INTO legal_holds(
		id,organization_id,hold_ref,name,description,legal_authority,owner_user_id,placed_by)
		VALUES($1,$2,$3,'Custodian immutability hold','Protect custodian assignment history',
		'Litigation preservation order',$4,$4)`, custodianHoldID, orgID, "LH-"+custodianHoldID[:8], userID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO legal_hold_custodians(
		organization_id,hold_id,user_id,added_by) VALUES($1,$2,$3,$3)`,
		orgID, custodianHoldID, userID); err != nil {
		t.Fatalf("add legal-hold custodian: %v", err)
	}
	assertSQLState("55000", `UPDATE legal_hold_custodians SET added_at=added_at-interval '1 second'
		WHERE organization_id=$1 AND hold_id=$2 AND user_id=$3`, orgID, custodianHoldID, userID)
	assertSQLState("55000", `DELETE FROM legal_hold_custodians
		WHERE organization_id=$1 AND hold_id=$2 AND user_id=$3`, orgID, custodianHoldID, userID)
	if _, err := connection.Exec(ctx, `UPDATE legal_hold_custodians SET
		released_at=statement_timestamp(),released_by=$3,release_reason='Custodian preservation duty completed'
		WHERE organization_id=$1 AND hold_id=$2 AND user_id=$3`, orgID, custodianHoldID, userID); err != nil {
		t.Fatalf("release legal-hold custodian: %v", err)
	}
	assertSQLState("55000", `UPDATE legal_hold_custodians SET release_reason='Rewritten release rationale'
		WHERE organization_id=$1 AND hold_id=$2 AND user_id=$3`, orgID, custodianHoldID, userID)
	assertSQLState("55000", `UPDATE legal_hold_custodians SET
		released_at=NULL,released_by=NULL,release_reason=NULL
		WHERE organization_id=$1 AND hold_id=$2 AND user_id=$3`, orgID, custodianHoldID, userID)

	if _, err := connection.Exec(ctx, `INSERT INTO legal_holds(
		id,organization_id,hold_ref,name,description,legal_authority,owner_user_id,placed_by)
		VALUES($1,$2,$3,'Concurrent evidence hold','Serialize release and placement',
		'Litigation preservation order',$4,$4)`, holdID, orgID, "LH-"+holdID[:8], userID); err != nil {
		t.Fatal(err)
	}

	// The parent transition obtains its row lock first. A concurrent child
	// placement must block, then re-check the committed terminal status and fail.
	releaseConnection, err := adminPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseConnection.Release()
	placeConnection, err := adminPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer placeConnection.Release()
	for _, candidate := range []*pgxpool.Conn{releaseConnection, placeConnection} {
		if _, err := candidate.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
			t.Fatal(err)
		}
		defer func(candidate *pgxpool.Conn) {
			_, _ = candidate.Exec(context.Background(), `SELECT set_config('app.current_tenant','',false)`)
			_, _ = candidate.Exec(context.Background(), `RESET ROLE`)
		}(candidate)
	}
	releaseTransaction, err := releaseConnection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseTransaction.Rollback(context.Background()) }()
	if _, err := releaseTransaction.Exec(ctx, `SELECT set_config('app.current_tenant',$1,true)`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTransaction.Exec(ctx, `UPDATE legal_holds SET
		status='released',released_by=$3,released_at=statement_timestamp(),
		release_reason='Evidence matter completed',version=version+1
		WHERE organization_id=$1 AND id=$2`, orgID, holdID, userID); err != nil {
		t.Fatal(err)
	}

	placementResult := make(chan error, 1)
	go func() {
		if _, setErr := placeConnection.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgID); setErr != nil {
			placementResult <- setErr
			return
		}
		_, insertErr := placeConnection.Exec(ctx, `INSERT INTO legal_hold_records(
			organization_id,hold_id,record_type,record_id,reason,placed_by)
			VALUES($1,$2,'evidence',$3,'Concurrent preservation request',$4)`,
			orgID, holdID, evidenceID, userID)
		placementResult <- insertErr
	}()
	select {
	case placementErr := <-placementResult:
		t.Fatalf("child placement did not serialize behind parent transition: %v", placementErr)
	case <-time.After(150 * time.Millisecond):
	}
	if err := releaseTransaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case placementErr := <-placementResult:
		if placementErr == nil || !strings.Contains(placementErr.Error(), "only on an active hold") {
			t.Fatalf("concurrent child placement error=%v", placementErr)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for serialized legal-hold placement")
	}
	var activeChildren int
	if err := connection.QueryRow(ctx, `SELECT count(*) FROM legal_hold_records
		WHERE organization_id=$1 AND hold_id=$2 AND released_at IS NULL`, orgID, holdID).Scan(&activeChildren); err != nil || activeChildren != 0 {
		t.Fatalf("active child records=%d error=%v", activeChildren, err)
	}

	if _, err := connection.Exec(ctx, `INSERT INTO legal_holds(
		id,organization_id,hold_ref,name,description,legal_authority,owner_user_id,placed_by)
		VALUES($1,$2,$3,'Evidence disposition race hold','Serialize placement and evidence disposition',
		'Litigation preservation order',$4,$4)`, placementHoldID, orgID, "LH-"+placementHoldID[:8], userID); err != nil {
		t.Fatal(err)
	}

	// Placement locks the evidence target before creating the preservation
	// record. A concurrent soft-delete must wait and then observe that active
	// record through the disposition guard rather than deleting the evidence.
	placementTransaction, err := placeConnection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = placementTransaction.Rollback(context.Background()) }()
	if _, err := placementTransaction.Exec(ctx, `SELECT set_config('app.current_tenant',$1,true)`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := placementTransaction.Exec(ctx, `INSERT INTO legal_hold_records(
		organization_id,hold_id,record_type,record_id,reason,placed_by)
		VALUES($1,$2,'evidence',$3,'Concurrent evidence preservation request',$4)`,
		orgID, placementHoldID, evidenceID, userID); err != nil {
		t.Fatalf("place evidence hold record: %v", err)
	}

	deletionResult := make(chan error, 1)
	go func() {
		if _, setErr := releaseConnection.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgID); setErr != nil {
			deletionResult <- setErr
			return
		}
		_, updateErr := releaseConnection.Exec(ctx, `UPDATE control_evidence
			SET deleted_at=statement_timestamp() WHERE organization_id=$1 AND id=$2`, orgID, evidenceID)
		deletionResult <- updateErr
	}()
	select {
	case deletionErr := <-deletionResult:
		t.Fatalf("evidence soft-delete did not serialize behind hold placement: %v", deletionErr)
	case <-time.After(150 * time.Millisecond):
	}
	if err := placementTransaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case deletionErr := <-deletionResult:
		var postgresError *pgconn.PgError
		if !errors.As(deletionErr, &postgresError) || postgresError.Code != "55000" ||
			!strings.Contains(postgresError.Message, "protected by an active legal hold") {
			t.Fatalf("serialized evidence soft-delete error=%v, want active-hold SQLSTATE 55000", deletionErr)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for serialized evidence soft-delete")
	}
	var evidenceDeletedAt *time.Time
	if err := connection.QueryRow(ctx, `SELECT deleted_at FROM control_evidence
		WHERE organization_id=$1 AND id=$2`, orgID, evidenceID).Scan(&evidenceDeletedAt); err != nil || evidenceDeletedAt != nil {
		t.Fatalf("evidence deleted_at=%v error=%v", evidenceDeletedAt, err)
	}
	if err := connection.QueryRow(ctx, `SELECT count(*) FROM legal_hold_records
		WHERE organization_id=$1 AND hold_id=$2 AND record_type='evidence'
		  AND record_id=$3 AND released_at IS NULL`, orgID, placementHoldID, evidenceID).Scan(&activeChildren); err != nil || activeChildren != 1 {
		t.Fatalf("active evidence hold records=%d error=%v", activeChildren, err)
	}
}
