//go:build integration

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

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

type failingDataGovernanceOutbox struct{}

func (failingDataGovernanceOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced data governance outbox failure")
}

func TestDataGovernanceWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	actorA, actorB := uuid.NewString(), uuid.NewString()
	assetA, unmanagedAssetA, assetB, legacyHoldVendorA := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleName := "grc_data_governance_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'Governance A',$3,'active','enterprise'),($2,'Governance B',$4,'active','enterprise')`,
		orgA, orgB, "governance-a-"+orgA, "governance-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Alice','Custodian','active'),($4,$5,$6,'Bob','Custodian','active')`,
		actorA, orgA, actorA+"@example.test", actorB, orgB, actorB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO assets
		(id,organization_id,asset_ref,name,asset_type,created_by) VALUES
		($1,$2,'AST-900001','Governed record','data',$3),
		($4,$2,'AST-900002','Unmanaged record','data',$3),
		($5,$6,'AST-900003','Other tenant record','data',$7)`,
		assetA, orgA, actorA, unmanagedAssetA, assetB, orgB, actorB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vendors
		(id,organization_id,vendor_ref,name,owner_user_id,created_by,legal_hold)
		VALUES ($1,$2,'VEN-900001','Legacy held processor',$3,$3,true)`,
		legacyHoldVendorA, orgA, actorA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		cleanupConn, acquireErr := pool.Acquire(context.Background())
		if acquireErr != nil {
			t.Errorf("acquire data governance cleanup connection: %v", acquireErr)
		} else {
			cleanupExec := func(statement string, arguments ...any) {
				if _, cleanupErr := cleanupConn.Exec(context.Background(), statement, arguments...); cleanupErr != nil {
					t.Errorf("data governance cleanup %q: %v", statement, cleanupErr)
				}
			}
			cleanupExec("RESET ROLE")
			cleanupExec(`SELECT set_config('app.current_tenant','',false)`)
			cleanupExec(`UPDATE vendors SET legal_hold=false WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			// Governance history is deliberately immutable, including to database
			// owners. This isolated test fixture uses a session-local replication
			// role only to remove its own generated event rows before deleting its
			// synthetic tenants; production tooling must never use this bypass.
			cleanupExec(`SET session_replication_role='replica'`)
			cleanupExec(`DELETE FROM data_governance_events WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM legal_hold_records WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM legal_hold_custodians WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM legal_holds WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`SET session_replication_role='origin'`)
			cleanupExec(`DELETE FROM legal_hold_reference_sequences WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM retention_exceptions WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM record_retention_assignments WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM retention_schedules WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM tenant_data_governance_policies WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM data_governance_event_chain_heads WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupExec(`DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
			cleanupConn.Release()
		}
		if _, cleanupErr := pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole); cleanupErr != nil {
			t.Errorf("drop data governance test role ownership: %v", cleanupErr)
		}
		if _, cleanupErr := pool.Exec(context.Background(), "DROP ROLE "+quotedRole); cleanupErr != nil {
			t.Errorf("drop data governance test role: %v", cleanupErr)
		}
	}
	defer cleanup()

	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole +
		"; GRANT SELECT ON organizations,users,assets,vendors TO " + quotedRole +
		"; GRANT SELECT,UPDATE,DELETE ON assets TO " + quotedRole +
		"; GRANT SELECT,UPDATE,DELETE ON vendors TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON tenant_data_governance_policies,retention_schedules," +
		"record_retention_assignments,retention_exceptions,legal_hold_reference_sequences,legal_holds," +
		"legal_hold_custodians,legal_hold_records TO " + quotedRole +
		"; GRANT SELECT ON data_governance_event_chain_heads TO " + quotedRole +
		"; GRANT SELECT,INSERT ON data_governance_events,queue_outbox TO " + quotedRole
	if _, err := pool.Exec(ctx, grant); err != nil {
		t.Fatal(err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET ROLE "+quotedRole); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), "RESET ROLE") }()
	setTenant := func(orgID string) context.Context {
		t.Helper()
		if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgID); err != nil {
			t.Fatal(err)
		}
		return database.WithQuerier(ctx, conn)
	}
	tenantA := setTenant(orgA)
	if _, err := conn.Exec(ctx, `UPDATE vendors SET deleted_at=NOW() WHERE organization_id=$1 AND id=$2`, orgA, legacyHoldVendorA); err == nil || !strings.Contains(err.Error(), "active legal hold") {
		t.Fatalf("legacy legal hold allowed soft deletion or returned the wrong guard: %v", err)
	}
	if _, err := conn.Exec(ctx, `DELETE FROM vendors WHERE organization_id=$1 AND id=$2`, orgA, legacyHoldVendorA); err == nil || !strings.Contains(err.Error(), "active legal hold") {
		t.Fatalf("legacy legal hold allowed hard deletion or returned the wrong guard: %v", err)
	}

	outbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.NewDataGovernanceRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	svc := service.NewDataGovernanceServiceWithClock(repo, zerolog.Nop(), func() time.Time { return now })

	if _, err := svc.GetPolicy(tenantA, orgA); !errors.Is(err, service.ErrDataGovernanceNotFound) {
		t.Fatalf("missing policy error=%v", err)
	}
	archiveDays := 30
	policy, err := svc.UpsertPolicy(tenantA, orgA, actorA, "governance-policy-create", models.DataGovernancePolicyInput{
		PrimaryRegion: "eu-west-1", AllowedRegions: []string{"eu-central-1", "eu-west-1", "eu-west-1"},
		CrossBorderTransferMode: "approved_regions", DefaultRetentionDays: 365,
		DefaultArchiveAfterDays: &archiveDays, DeletionGraceDays: 30,
		DispositionApprovalMode: "single", RequireProcessorConfirmation: true,
		LegalHoldEnabled: true, PolicyStatement: "Governed records remain in approved European regions.",
		Reason: "Approve enterprise data lifecycle baseline",
	})
	if err != nil || policy.Version != 1 || len(policy.AllowedRegions) != 2 {
		t.Fatalf("policy=%+v err=%v", policy, err)
	}
	stale := int64(9)
	if _, err := svc.UpsertPolicy(tenantA, orgA, actorA, "governance-policy-stale", models.DataGovernancePolicyInput{
		PrimaryRegion: "eu-west-1", AllowedRegions: []string{"eu-west-1"},
		CrossBorderTransferMode: "approved_regions", DefaultRetentionDays: 365,
		DeletionGraceDays: 30, DispositionApprovalMode: "single", LegalHoldEnabled: true,
		ExpectedVersion: &stale, Reason: "Reject stale policy update",
	}); !errors.Is(err, service.ErrDataGovernanceConflict) {
		t.Fatalf("stale policy error=%v", err)
	}

	// Competing writers for one tenant must serialize through the chain head
	// without duplicate sequences, gaps, or invalid hashes.
	const concurrentSchedules = 8
	concurrentErrors := make(chan error, concurrentSchedules)
	var concurrentWriters sync.WaitGroup
	for index := 0; index < concurrentSchedules; index++ {
		concurrentWriters.Add(1)
		go func(index int) {
			defer concurrentWriters.Done()
			writer, acquireErr := pool.Acquire(ctx)
			if acquireErr != nil {
				concurrentErrors <- acquireErr
				return
			}
			defer writer.Release()
			defer func() {
				_, _ = writer.Exec(context.Background(), `SELECT set_config('app.current_tenant','',false)`)
				_, _ = writer.Exec(context.Background(), "RESET ROLE")
			}()
			if _, roleErr := writer.Exec(ctx, "SET ROLE "+quotedRole); roleErr != nil {
				concurrentErrors <- roleErr
				return
			}
			if _, tenantErr := writer.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgA); tenantErr != nil {
				concurrentErrors <- tenantErr
				return
			}
			writerContext := database.WithQuerier(ctx, writer)
			_, createErr := svc.CreateSchedule(
				writerContext, orgA, actorA, fmt.Sprintf("governance-concurrent-%d", index),
				models.RetentionScheduleInput{
					Name: fmt.Sprintf("Concurrent schedule %02d", index), RecordType: "risk",
					LegalBasis: "Controlled concurrency verification", RetentionDays: 365,
					Status: "draft", Reason: "Verify serialized governance event allocation",
				},
			)
			concurrentErrors <- createErr
		}(index)
	}
	concurrentWriters.Wait()
	close(concurrentErrors)
	for concurrentErr := range concurrentErrors {
		if concurrentErr != nil {
			t.Fatalf("concurrent schedule creation: %v", concurrentErr)
		}
	}
	concurrentVerification, err := svc.VerifyEventChain(tenantA, orgA)
	if err != nil || !concurrentVerification.Valid || concurrentVerification.EventCount != concurrentSchedules+1 ||
		concurrentVerification.LastSequence != concurrentVerification.EventCount {
		t.Fatalf("concurrent chain verification=%+v err=%v", concurrentVerification, err)
	}

	failingRepo, err := repository.NewDataGovernanceRepository(pool, failingDataGovernanceOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	failingService := service.NewDataGovernanceServiceWithClock(failingRepo, zerolog.Nop(), func() time.Time { return now })
	_, err = failingService.CreateSchedule(tenantA, orgA, actorA, "governance-forced-rollback", models.RetentionScheduleInput{
		Name: "Rollback schedule", RecordType: "asset", LegalBasis: "Contractual obligation",
		RetentionDays: 90, ReviewRequired: true, Status: "active", Reason: "Must roll back with outbox",
	})
	if err == nil || !strings.Contains(err.Error(), "forced data governance outbox failure") {
		t.Fatalf("forced outbox error=%v", err)
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM retention_schedules WHERE name='Rollback schedule'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled back schedule count=%d err=%v", count, err)
	}

	schedule, err := svc.CreateSchedule(tenantA, orgA, actorA, "governance-schedule-create", models.RetentionScheduleInput{
		Name: "Business records", Description: "Default schedule for governed business records.",
		RecordType: "asset", DataClassification: "restricted", Jurisdiction: "EU",
		TriggerEvent: "record_created", LegalBasis: "GDPR accountability and contractual limitation periods",
		RetentionDays: 1, DispositionAction: "delete", ReviewRequired: true, Priority: 10,
		Status: "active", EffectiveFrom: now.AddDate(0, 0, -10), Reason: "Publish approved asset schedule",
	})
	if err != nil || schedule.Version != 1 {
		t.Fatalf("schedule=%+v err=%v", schedule, err)
	}
	assignment, err := svc.CreateAssignment(tenantA, orgA, actorA, "governance-assignment-create", models.RetentionAssignmentInput{
		ScheduleID: schedule.ID, RecordType: "asset", RecordID: assetA,
		DataClassification: "restricted", Jurisdiction: "EU", RetentionStartedAt: now.AddDate(0, 0, -2),
		Reason: "Classify asset under approved schedule",
	})
	if err != nil || assignment.ReviewStatus != "pending" {
		t.Fatalf("assignment=%+v err=%v", assignment, err)
	}
	decision, err := svc.GetRecordDisposition(tenantA, orgA, "asset", assetA)
	if err != nil || decision.Allowed || decision.Reason != "disposition_review_required" {
		t.Fatalf("pre-review decision=%+v err=%v", decision, err)
	}
	assignment, err = svc.ReviewDisposition(tenantA, orgA, assignment.ID, actorA, "governance-review", models.RetentionReviewInput{
		ExpectedVersion: assignment.Version, Decision: "approve", Reason: "Retention elapsed and destruction was reviewed",
	})
	if err != nil || assignment.ReviewStatus != "approved" {
		t.Fatalf("reviewed assignment=%+v err=%v", assignment, err)
	}
	decision, err = svc.GetRecordDisposition(tenantA, orgA, "asset", assetA)
	if err != nil || !decision.Allowed || decision.Reason != "eligible_for_disposition" {
		t.Fatalf("eligible decision=%+v err=%v", decision, err)
	}

	hold, err := svc.CreateLegalHold(tenantA, orgA, actorA, "governance-hold-create", models.LegalHoldInput{
		Name: "Regulatory inquiry", MatterReference: "DPA-2026-0042",
		Description:    "Preserve records relevant to the supervisory authority inquiry.",
		LegalAuthority: "Written preservation notice from supervisory authority", OwnerUserID: actorA,
		CustodianIDs: []string{actorA, actorA}, ReviewDueAt: ptrTime(now.AddDate(0, 1, 0)),
		Reason: "Legal approved preservation notice",
	})
	if err != nil || hold.HoldRef != "HOLD-000001" || len(hold.CustodianIDs) != 1 {
		t.Fatalf("hold=%+v err=%v", hold, err)
	}
	holdRecord, err := svc.AddLegalHoldRecord(tenantA, orgA, hold.ID, actorA, "governance-hold-record", models.LegalHoldRecordInput{
		RecordType: "asset", RecordID: assetA, Reason: "Asset falls within preservation scope",
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err = svc.GetRecordDisposition(tenantA, orgA, "asset", assetA)
	if err != nil || decision.Allowed || decision.Reason != "active_legal_hold" {
		t.Fatalf("held decision=%+v err=%v", decision, err)
	}
	if _, err := conn.Exec(ctx, `UPDATE assets SET deleted_at=NOW() WHERE organization_id=$1 AND id=$2`, orgA, assetA); err == nil {
		t.Fatal("database allowed soft deletion under legal hold")
	}
	if _, err := conn.Exec(ctx, `UPDATE assets SET deleted_at=NOW() WHERE organization_id=$1 AND id=$2`, orgA, unmanagedAssetA); err == nil {
		t.Fatal("configured policy allowed deletion without a retention assignment")
	}
	if err := svc.ReleaseLegalHoldRecord(tenantA, orgA, hold.ID, holdRecord.ID, actorA,
		"governance-hold-record-release", models.LegalHoldRecordReleaseInput{Reason: "Record released after scope review"}); err != nil {
		t.Fatal(err)
	}
	decision, err = svc.GetRecordDisposition(tenantA, orgA, "asset", assetA)
	if err != nil || !decision.Allowed {
		t.Fatalf("released decision=%+v err=%v", decision, err)
	}
	hold, err = svc.ReleaseLegalHold(tenantA, orgA, hold.ID, actorA, "governance-hold-release", models.LegalHoldReleaseInput{
		ExpectedVersion: hold.Version, Outcome: "release", Reason: "Authority closed the inquiry and counsel approved release",
	})
	if err != nil || hold.Status != "released" {
		t.Fatalf("released hold=%+v err=%v", hold, err)
	}

	retentionException, err := svc.RequestException(
		tenantA, orgA, assignment.ID, actorA, "governance-exception-request",
		models.RetentionExceptionInput{
			RequestedUntil: now.AddDate(0, 1, 0),
			Reason:         "Preserve the record while a regulator reviews the response",
		},
	)
	if err != nil || retentionException.Status != "pending" {
		t.Fatalf("pending exception=%+v err=%v", retentionException, err)
	}
	decision, err = svc.GetRecordDisposition(tenantA, orgA, "asset", assetA)
	if err != nil || decision.Allowed || decision.Reason != "retention_exception_pending" {
		t.Fatalf("pending-exception decision=%+v err=%v", decision, err)
	}
	if _, err := conn.Exec(ctx, `UPDATE assets SET deleted_at=NOW() WHERE organization_id=$1 AND id=$2`, orgA, assetA); err == nil {
		t.Fatal("database allowed soft deletion while a retention exception was pending")
	}
	retentionException, err = svc.DecideException(
		tenantA, orgA, retentionException.ID, actorA, "governance-exception-approve",
		models.RetentionExceptionDecisionInput{
			ExpectedVersion: retentionException.Version,
			Decision:        "approve",
			Reason:          "Counsel approved the documented retention extension",
		},
	)
	if err != nil || retentionException.Status != "approved" {
		t.Fatalf("approved exception=%+v err=%v", retentionException, err)
	}
	decision, err = svc.GetRecordDisposition(tenantA, orgA, "asset", assetA)
	if err != nil || decision.Allowed || decision.Reason != "retention_period_active" ||
		decision.Assignment == nil || !decision.Assignment.DispositionDueAt.Equal(retentionException.RequestedUntil) {
		t.Fatalf("extended-retention decision=%+v exception=%+v err=%v", decision, retentionException, err)
	}
	exceptions, err := svc.ListExceptions(tenantA, orgA, assignment.ID)
	if err != nil || len(exceptions) != 1 || exceptions[0].Status != "approved" {
		t.Fatalf("exceptions=%+v err=%v", exceptions, err)
	}

	verification, err := svc.VerifyEventChain(tenantA, orgA)
	if err != nil || !verification.Valid || verification.EventCount < concurrentSchedules+9 || verification.LastSequence != verification.EventCount {
		t.Fatalf("chain verification=%+v err=%v", verification, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE data_governance_events SET reason='tampered' WHERE organization_id=$1`, orgA); err == nil {
		t.Fatal("append-only governance history accepted privileged mutation")
	}
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM queue_outbox WHERE tenant_id=$1
		AND envelope->'payload'->>'type' LIKE 'data_governance.%'`, orgA).Scan(&count); err != nil || count < concurrentSchedules+9 {
		t.Fatalf("governance outbox count=%d err=%v", count, err)
	}

	// RLS must hide and reject tenant A governance state while tenant B is active.
	tenantB := setTenant(orgB)
	if _, err := svc.GetPolicy(tenantB, orgB); !errors.Is(err, service.ErrDataGovernanceNotFound) {
		t.Fatalf("tenant B policy error=%v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM retention_schedules`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cross-tenant schedule visibility count=%d err=%v", count, err)
	}
	if _, err := conn.Exec(ctx, `UPDATE retention_schedules SET name='Cross tenant' WHERE organization_id=$1`, orgA); err != nil {
		t.Fatalf("RLS-filtered update returned unexpected error: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM retention_schedules WHERE organization_id=$1`, orgA).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cross-tenant schedule read count=%d err=%v", count, err)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
