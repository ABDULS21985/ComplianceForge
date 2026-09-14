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
	workerpkg "github.com/complianceforge/platform/internal/worker"
)

type failingIncidentOutbox struct{}

func (failingIncidentOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced outbox failure")
}

// TestIncidentRepositoryWithNonSuperuserTenants exercises the complete
// incident register through a genuine NOSUPERUSER/NOBYPASSRLS database role.
// It verifies defense-in-depth tenant predicates, FORCE RLS, optimistic writes,
// atomic references/outbox, assignment history, GDPR deadlines, and soft delete.
func TestIncidentRepositoryWithNonSuperuserTenants(t *testing.T) {
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
	userA, userB, investigatorA := uuid.NewString(), uuid.NewString(), uuid.NewString()
	roleName := "grc_incident_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Incident RLS A',$3,'active','starter'),($2,'Incident RLS B',$4,'active','starter')`,
		orgA, orgB, "incident-a-"+orgA, "incident-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,organization_id,email,first_name,last_name,status) VALUES
		($1,$2,$3,'Alice','Responder','active'),($4,$5,$6,'Bob','Responder','active'),
		($7,$2,$8,'Ivy','Investigator','active')`, userA, orgA, userA+"@example.test",
		userB, orgB, userB+"@example.test", investigatorA, investigatorA+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("creating non-superuser role: %v", err)
	}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE "+quotedRole)
	}
	defer cleanup()
	grant := "GRANT USAGE ON SCHEMA public TO " + quotedRole +
		"; GRANT SELECT ON organizations,users TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON incident_reference_sequences,incidents,incident_assignments TO " + quotedRole +
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON incident_events TO " + quotedRole +
		"; GRANT SELECT,INSERT ON queue_outbox TO " + quotedRole
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

	failingRepo, err := repository.NewIncidentRepository(pool, failingIncidentOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	tenantA := setTenant(orgA)
	detected := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	create := models.IncidentCreateInput{Title: "Exposed production snapshot", Description: "A snapshot was accessible outside the approved boundary", Category: "privacy", Severity: models.IncidentSeverityHigh, DetectedAt: &detected}
	if _, err := failingRepo.Create(tenantA, orgA, userA, create); err == nil || !strings.Contains(err.Error(), "forced outbox failure") {
		t.Fatalf("forced atomic create error=%v", err)
	}
	var rolledBack int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM incidents WHERE organization_id=$1`, orgA).Scan(&rolledBack); err != nil || rolledBack != 0 {
		t.Fatalf("mutation survived outbox failure count=%d err=%v", rolledBack, err)
	}

	outbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	dataRepo, err := repository.NewIncidentRepository(pool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	incidentService := service.NewIncidentService(dataRepo, zerolog.Nop())
	incidentA, err := incidentService.Create(tenantA, orgA, userA, create)
	if err != nil {
		t.Fatalf("creating tenant A incident: %v", err)
	}
	if incidentA.IncidentRef != "INC-000001" || incidentA.Status != models.IncidentStatusReported || incidentA.Version != 1 {
		t.Fatalf("tenant A incident=%#v", incidentA)
	}
	secondA, err := incidentService.Create(tenantA, orgA, userA, models.IncidentCreateInput{
		Title: "Duplicate alert", Description: "Alert was generated by a test", Category: "operations",
		Severity: models.IncidentSeverityLow, DetectedAt: &detected,
	})
	if err != nil || secondA.IncidentRef != "INC-000002" {
		t.Fatalf("second reference incident=%#v err=%v", secondA, err)
	}
	secondA, err = incidentService.Cancel(tenantA, orgA, userA, secondA.ID, secondA.Version, "Confirmed duplicate test alert")
	if err != nil {
		t.Fatal(err)
	}
	if err := incidentService.Delete(tenantA, orgA, userA, secondA.ID, secondA.Version); err != nil {
		t.Fatalf("soft deleting cancelled incident: %v", err)
	}
	if _, err := incidentService.GetByID(tenantA, orgA, secondA.ID); !errors.Is(err, service.ErrIncidentNotFound) {
		t.Fatalf("soft-deleted incident read error=%v", err)
	}

	const concurrentCreates = 8
	references := make(chan string, concurrentCreates)
	createErrors := make(chan error, concurrentCreates)
	var createGroup sync.WaitGroup
	for index := 0; index < concurrentCreates; index++ {
		index := index
		createGroup.Add(1)
		go func() {
			defer createGroup.Done()
			workerConn, acquireErr := pool.Acquire(ctx)
			if acquireErr != nil {
				createErrors <- acquireErr
				return
			}
			defer workerConn.Release()
			if _, roleErr := workerConn.Exec(ctx, "SET ROLE "+quotedRole); roleErr != nil {
				createErrors <- roleErr
				return
			}
			defer func() {
				_, _ = workerConn.Exec(context.Background(), `SELECT set_config('app.current_tenant','',false)`)
				_, _ = workerConn.Exec(context.Background(), "RESET ROLE")
			}()
			if _, tenantErr := workerConn.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgA); tenantErr != nil {
				createErrors <- tenantErr
				return
			}
			workerCtx := database.WithQuerier(ctx, workerConn)
			created, createErr := incidentService.Create(workerCtx, orgA, userA, models.IncidentCreateInput{
				Title: fmt.Sprintf("Concurrent incident %d", index), Description: "Concurrent reference allocation test",
				Category: "operations", Severity: models.IncidentSeverityLow, DetectedAt: &detected,
			})
			if createErr != nil {
				createErrors <- createErr
				return
			}
			references <- created.IncidentRef
		}()
	}
	createGroup.Wait()
	close(references)
	close(createErrors)
	for createErr := range createErrors {
		t.Fatalf("concurrent incident creation: %v", createErr)
	}
	seenReferences := make(map[string]bool)
	for reference := range references {
		if seenReferences[reference] {
			t.Fatalf("duplicate concurrent reference %s", reference)
		}
		seenReferences[reference] = true
	}
	if len(seenReferences) != concurrentCreates {
		t.Fatalf("concurrent references=%v", seenReferences)
	}

	incidentA, err = incidentService.Transition(tenantA, orgA, userA, incidentA.ID, models.IncidentTransitionInput{Status: models.IncidentStatusTriaged, Version: incidentA.Version})
	if err != nil {
		t.Fatal(err)
	}
	assignment, incidentA, err := incidentService.CreateAssignment(tenantA, orgA, userA, incidentA.ID, models.IncidentAssignmentInput{
		Version: incidentA.Version, AssigneeID: investigatorA, Role: models.IncidentAssignmentPrimary,
		Reason: "Lead the privacy incident investigation",
	})
	if err != nil || incidentA.AssigneeID == nil || *incidentA.AssigneeID != investigatorA {
		t.Fatalf("assignment=%#v incident=%#v err=%v", assignment, incidentA, err)
	}

	awareness := time.Now().UTC().Add(-30 * time.Hour).Truncate(time.Microsecond)
	incidentA, err = incidentService.AssessBreach(tenantA, orgA, userA, incidentA.ID, models.IncidentBreachAssessmentInput{
		Version: incidentA.Version, Status: models.BreachAssessmentNotifiable,
		Reason: "Unauthorized disclosure creates likely risk to data subjects", IsDataBreach: true,
		AwarenessAt: &awareness, DataSubjectsAffected: integerPointer(120), RecordsAffected: int64Pointer(340),
		DataCategories: []string{"identity", "contact"}, BreachNature: "Unauthorized disclosure",
		LikelyConsequences: "Identity fraud", MitigationMeasures: "Access revoked and credentials rotated",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDeadline := awareness.Add(72 * time.Hour)
	if incidentA.NotificationDeadline == nil || !incidentA.NotificationDeadline.Equal(wantDeadline) {
		t.Fatalf("notification deadline=%v want=%v", incidentA.NotificationDeadline, wantDeadline)
	}
	rolePoolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	rolePoolConfig.AfterConnect = func(connectCtx context.Context, roleConn *pgx.Conn) error {
		_, roleErr := roleConn.Exec(connectCtx, "SET ROLE "+quotedRole)
		return roleErr
	}
	rolePool, err := pgxpool.NewWithConfig(ctx, rolePoolConfig)
	if err != nil {
		t.Fatal(err)
	}
	bus := service.NewEventBus()
	deadlineEvents := bus.Subscribe("*")
	scheduler := workerpkg.NewRegulatoryScheduler(rolePool, bus)
	if err := scheduler.CheckGDPRBreachDeadlines(ctx); err != nil {
		rolePool.Close()
		t.Fatalf("tenant-scoped GDPR scheduler: %v", err)
	}
	select {
	case event := <-deadlineEvents:
		if event.OrgID != orgA || event.EntityID != incidentA.ID || event.EntityRef != incidentA.IncidentRef || event.Type != "gdpr.breach_deadline_48h" {
			t.Fatalf("GDPR scheduler event=%#v", event)
		}
	default:
		t.Fatal("GDPR scheduler did not publish the due incident")
	}
	bus.Close()
	rolePool.Close()
	idempotencyKey := uuid.NewString()
	notifiedAt := time.Now().UTC().Truncate(time.Microsecond)
	notification := models.IncidentDPANotificationInput{Version: incidentA.Version, IdempotencyKey: idempotencyKey, NotifiedAt: notifiedAt, Reference: "DPA-2026-00981", Reason: "GDPR Article 33 notification"}
	incidentA, err = incidentService.NotifyDPA(tenantA, orgA, userA, incidentA.ID, notification)
	if err != nil {
		t.Fatal(err)
	}
	versionAfterNotification := incidentA.Version
	retry, err := incidentService.NotifyDPA(tenantA, orgA, userA, incidentA.ID, notification)
	if err != nil || retry.Version != versionAfterNotification {
		t.Fatalf("idempotent DPA retry=%#v err=%v", retry, err)
	}
	notification.IdempotencyKey = uuid.NewString()
	if _, err := incidentService.NotifyDPA(tenantA, orgA, userA, incidentA.ID, notification); !errors.Is(err, service.ErrIncidentIdempotency) {
		t.Fatalf("conflicting DPA retry error=%v", err)
	}

	events, totalEvents, err := incidentService.ListEvents(tenantA, orgA, incidentA.ID, models.PaginationRequest{Page: 1, PageSize: 100})
	if err != nil || totalEvents < 4 || len(events) != totalEvents {
		t.Fatalf("timeline total=%d len=%d err=%v", totalEvents, len(events), err)
	}
	if tag, err := conn.Exec(ctx, `UPDATE incident_events SET summary='tampered' WHERE id=$1`, events[0].ID); err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("append-only timeline update affected=%d err=%v", tag.RowsAffected(), err)
	}
	if tag, err := conn.Exec(ctx, `DELETE FROM incident_events WHERE id=$1`, events[0].ID); err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("append-only timeline delete affected=%d err=%v", tag.RowsAffected(), err)
	}
	assignments, err := incidentService.ListAssignments(tenantA, orgA, incidentA.ID, true)
	if err != nil || len(assignments) != 1 || assignments[0].ID != assignment.ID {
		t.Fatalf("assignments=%#v err=%v", assignments, err)
	}
	stats, err := incidentService.Statistics(tenantA, orgA)
	if err != nil || stats.Total != concurrentCreates+1 || stats.NotifiedBreaches != 1 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
	var outboxEvents int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM queue_outbox WHERE tenant_id=$1`, orgA).Scan(&outboxEvents); err != nil || outboxEvents != totalEvents+3+concurrentCreates {
		// The deleted second incident contributes create, cancel, and delete; its
		// timeline cascades on tenant cleanup but all three outbox events persist.
		t.Fatalf("outbox events=%d timeline=%d err=%v", outboxEvents, totalEvents, err)
	}

	tenantB := setTenant(orgB)
	incidentB, err := incidentService.Create(tenantB, orgB, userB, models.IncidentCreateInput{
		Title: "Tenant B outage", Description: "Service interruption", Category: "availability",
		Severity: models.IncidentSeverityMedium, DetectedAt: &detected,
	})
	if err != nil || incidentB.IncidentRef != "INC-000001" {
		t.Fatalf("tenant B incident=%#v err=%v", incidentB, err)
	}
	if _, err := incidentService.GetByID(tenantB, orgB, incidentA.ID); !errors.Is(err, service.ErrIncidentNotFound) {
		t.Fatalf("cross-tenant read error=%v", err)
	}
	patchTitle := "Cross-tenant overwrite"
	if _, err := incidentService.Update(tenantB, orgB, userB, incidentA.ID, models.IncidentPatch{Version: incidentA.Version, Title: &patchTitle}); !errors.Is(err, service.ErrIncidentNotFound) {
		t.Fatalf("cross-tenant mutation error=%v", err)
	}
	if _, _, err := incidentService.CreateAssignment(tenantB, orgB, userB, incidentB.ID, models.IncidentAssignmentInput{
		Version: incidentB.Version, AssigneeID: investigatorA, Role: models.IncidentAssignmentInvestigator,
		Reason: "Cross-tenant assignment must fail",
	}); err == nil {
		t.Fatal("cross-tenant user assignment unexpectedly succeeded")
	}
	if _, err := dataRepo.Transition(tenantB, orgB, userB, incidentA.ID, models.IncidentTransitionInput{Status: models.IncidentStatusTriaged, Version: incidentA.Version}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("repository cross-tenant transition error=%v", err)
	}
	var visibleEvents int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM incident_events WHERE incident_id=$1`, incidentA.ID).Scan(&visibleEvents); err != nil || visibleEvents != 0 {
		t.Fatalf("cross-tenant direct timeline count=%d err=%v", visibleEvents, err)
	}

	tenantA = setTenant(orgA)
	staleVersion := incidentA.Version - 1
	if _, err := dataRepo.Transition(tenantA, orgA, userA, incidentA.ID, models.IncidentTransitionInput{Status: models.IncidentStatusInvestigating, Version: staleVersion}); !errors.Is(err, repository.ErrIncidentVersionConflict) {
		t.Fatalf("stale version transition error=%v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant','',false)`); err != nil {
		t.Fatal(fmt.Errorf("clearing tenant: %w", err))
	}
	if _, err := conn.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
}

func integerPointer(value int) *int   { return &value }
func int64Pointer(value int64) *int64 { return &value }
