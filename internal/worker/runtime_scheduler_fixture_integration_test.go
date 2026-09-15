//go:build integration

package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/service"
)

type schedulerRoleFixture struct {
	userID, policyID, calendarReminderID, calendarEscalationID, calendarPastID, calendarTodayID string
	dsrDueID, dsrExtendedID, scheduleID                                                         string
	expiringExceptionID, expiredExceptionID                                                     string
	scheduledAt                                                                                 time.Time
	dueEntities                                                                                 map[string]string
}

// This populated contract deliberately seeds both tenants as the schema owner
// but executes every scheduler query and write as the non-owner worker. It
// catches dynamic SQL/schema drift, not merely role-catalog drift.
func testPopulatedSchedulerRoles(t *testing.T, ctx context.Context, adminPool, workerPool *pgxpool.Pool, orgA, orgB string) {
	t.Helper()
	fixtureA := seedSchedulerRoleFixture(t, ctx, adminPool, orgA)
	fixtureB := seedSchedulerRoleFixture(t, ctx, adminPool, orgB)
	t.Run("GDPR and vendor discovery under ordinary migrator ownership", func(t *testing.T) {
		discoveryBus := service.NewEventBus()
		discoveryEvents := discoveryBus.Subscribe("*")
		defer discoveryBus.Close()
		discovery := NewRegulatoryScheduler(workerPool, discoveryBus)
		if err := discovery.CheckGDPRBreachDeadlines(ctx); err != nil {
			t.Fatalf("global GDPR active-registry discovery: %v", err)
		}
		if err := discovery.CheckVendorAssessments(ctx); err != nil {
			t.Fatalf("global vendor active-registry discovery: %v", err)
		}
		found := make(map[string]bool)
		for len(discoveryEvents) > 0 {
			event := <-discoveryEvents
			var fixture schedulerRoleFixture
			switch event.OrgID {
			case orgA:
				fixture = fixtureA
			case orgB:
				fixture = fixtureB
			default:
				// Other fixtures may remain in this deliberately disposable DB.
				continue
			}
			if fixture.dueEntities[event.Type] != event.EntityID {
				t.Fatalf("global discovery emitted a future or wrong-tenant entity: %+v", event)
			}
			found[event.OrgID+":"+event.Type] = true
		}
		if len(found) != 4 {
			t.Fatalf("global GDPR/vendor discovery found %d tenant/type pairs, want both types for both tenants", len(found))
		}
	})
	bus := service.NewEventBus()
	events := bus.Subscribe("*")
	t.Cleanup(bus.Close)
	scheduler := NewRegulatoryScheduler(workerPool, bus)
	var previous map[string]service.Event
	for run := 0; run < 2; run++ {
		err := database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error {
			for _, check := range []func(context.Context, string) error{
				scheduler.checkGDPRBreachDeadlinesForTenant, scheduler.checkNIS2DeadlinesForTenant,
				scheduler.checkPolicyReviewsForTenant, scheduler.checkFindingRemediationsForTenant,
				scheduler.checkVendorAssessmentsForTenant, scheduler.checkRiskReviewsForTenant,
				scheduler.checkDSRDeadlinesForTenant,
			} {
				if err := check(tenantCtx, orgA); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("populated regulatory queries: %v", err)
		}
		current := make(map[string]service.Event)
		for {
			select {
			case event := <-events:
				if event.OrgID != orgA || fixtureA.dueEntities[event.Type] != event.EntityID {
					t.Fatalf("unexpected due/future/cross-tenant event: %+v", event)
				}
				encoded, err := json.Marshal(event)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), "PII-CIPHERTEXT-SENTINEL") {
					t.Fatal("DSR ciphertext leaked to scheduled event")
				}
				if event.ID == "" {
					t.Fatal("scheduled event has no durable identity")
				}
				current[event.Type] = event
			default:
				goto drained
			}
		}
	drained:
		if len(current) != 7 {
			t.Fatalf("got %d due event types, want seven: %+v", len(current), current)
		}
		if previous != nil && !reflect.DeepEqual(previous, current) {
			t.Fatal("repeat poll changed scheduled event identity or immutable payload")
		}
		previous = current
		outbox, err := queuepkg.NewPostgresOutbox(workerPool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range current {
			envelope, err := queuepkg.NewEnvelope("domain.event", event.OrgID, event)
			if err != nil {
				t.Fatal(err)
			}
			envelope.ID, envelope.CorrelationID, envelope.CreatedAt = event.ID, event.ID, event.Timestamp
			if err := outbox.Enqueue(ctx, workerPool, "runtime.scheduler", envelope); err != nil {
				t.Fatalf("repeat scheduled outbox enqueue: %v", err)
			}
		}
	}
	var messages int
	if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM queue_outbox WHERE tenant_id=$1::uuid AND queue_name='runtime.scheduler'`, orgA).Scan(&messages); err != nil || messages != 7 {
		t.Fatalf("repeat polls created %d durable messages, want seven: %v", messages, err)
	}

	t.Run("calendar reminders escalation status and no duplicate rows", func(t *testing.T) {
		calendar := NewCalendarWorker(workerPool)
		for run := 0; run < 2; run++ {
			if err := database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error {
				if err := calendar.remindersForTenant(tenantCtx, orgA); err != nil {
					return err
				}
				if err := calendar.escalationsForTenant(tenantCtx, orgA); err != nil {
					return err
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		var own, cross, valid int
		if err := adminPool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE organization_id=$1::uuid),
			count(*) FILTER (WHERE organization_id=$2::uuid),
			count(*) FILTER (WHERE organization_id=$1::uuid AND channel_type='in_app' AND length(delivery_key)=64 AND body_text IS NOT NULL)
			FROM notifications WHERE organization_id IN ($1::uuid,$2::uuid)`, orgA, orgB).Scan(&own, &cross, &valid); err != nil {
			t.Fatal(err)
		}
		if own != 2 || cross != 0 || valid != 2 {
			t.Fatalf("calendar notifications own=%d cross=%d valid=%d, want 2/0/2", own, cross, valid)
		}
		if err := database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error { return calendar.updateStatusForTenant(tenantCtx, orgA) }); err != nil {
			t.Fatal(err)
		}
		var past, today string
		if err := adminPool.QueryRow(ctx, `SELECT status FROM calendar_events WHERE id=$1::uuid`, fixtureA.calendarPastID).Scan(&past); err != nil {
			t.Fatal(err)
		}
		if err := adminPool.QueryRow(ctx, `SELECT status FROM calendar_events WHERE id=$1::uuid`, fixtureA.calendarTodayID).Scan(&today); err != nil {
			t.Fatal(err)
		}
		if past != "overdue" || today != "upcoming" {
			t.Fatalf("calendar statuses past=%s today=%s", past, today)
		}
	})

	t.Run("DSR effective deadline and ciphertext safe lifecycle", func(t *testing.T) {
		if err := database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error {
			return NewDSRScheduler(workerPool).runForTenant(tenantCtx, orgA)
		}); err != nil {
			t.Fatal(err)
		}
		var due, extended string
		if err := adminPool.QueryRow(ctx, `SELECT sla_status FROM dsr_requests WHERE id=$1::uuid`, fixtureA.dsrDueID).Scan(&due); err != nil {
			t.Fatal(err)
		}
		if err := adminPool.QueryRow(ctx, `SELECT sla_status FROM dsr_requests WHERE id=$1::uuid`, fixtureA.dsrExtendedID).Scan(&extended); err != nil {
			t.Fatal(err)
		}
		if due != "overdue" || extended != "on_track" {
			t.Fatalf("effective deadline statuses due=%s extended=%s", due, extended)
		}
	})

	t.Run("exception reminders and guarded expiry audit", func(t *testing.T) {
		exceptionBus := service.NewEventBus()
		exceptionEvents := exceptionBus.Subscribe("*")
		defer exceptionBus.Close()
		exceptions := NewExceptionScheduler(workerPool, exceptionBus)
		var previous map[string]service.Event
		for run := 0; run < 2; run++ {
			if err := database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error {
				if err := exceptions.checkExpiringExceptionsForTenant(tenantCtx, orgA); err != nil {
					return err
				}
				return exceptions.checkOverdueReviewsForTenant(tenantCtx, orgA)
			}); err != nil {
				t.Fatal(err)
			}
			current := make(map[string]service.Event)
			for len(exceptionEvents) > 0 {
				event := <-exceptionEvents
				if event.OrgID != orgA || event.EntityID != fixtureA.expiringExceptionID {
					t.Fatalf("exception reminder leaked or selected not-due row: %+v", event)
				}
				current[event.Type] = event
			}
			if len(current) != 2 {
				t.Fatalf("exception reminder types=%d, want expiry+review", len(current))
			}
			if previous != nil && !reflect.DeepEqual(previous, current) {
				t.Fatal("repeat exception reminders changed identity/payload")
			}
			previous = current
		}
		for run := 0; run < 2; run++ {
			if err := database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error {
				return exceptions.autoExpireExceptionsForTenant(tenantCtx, orgA)
			}); err != nil {
				t.Fatal(err)
			}
		}
		var audits, cross int
		if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM exception_audit_trail WHERE exception_id=$1::uuid AND action='auto_expired'`, fixtureA.expiredExceptionID).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("exception expiry audits=%d: %v", audits, err)
		}
		if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM compliance_exceptions WHERE organization_id=$1::uuid AND status='expired'`, orgB).Scan(&cross); err != nil || cross != 0 {
			t.Fatalf("cross-tenant exceptions expired=%d: %v", cross, err)
		}
	})

	t.Run("policy current-version full and incremental index", func(t *testing.T) {
		search := NewSearchIndexer(workerPool)
		if err := database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error {
			if _, err := search.indexAllEntities(tenantCtx, orgA); err != nil {
				return err
			}
			return search.incrementalIndexForTenant(tenantCtx, "policy", fixtureA.policyID, orgA, "upsert")
		}); err != nil {
			t.Fatal(err)
		}
		var body string
		if err := adminPool.QueryRow(ctx, `SELECT body FROM search_index WHERE organization_id=$1::uuid AND entity_type='policy' AND entity_id=$2::uuid`, orgA, fixtureA.policyID).Scan(&body); err != nil {
			t.Fatal(err)
		}
		if body != "Canonical version content" {
			t.Fatalf("indexed policy body=%q", body)
		}
		var cross int
		if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM search_index WHERE organization_id=$1::uuid`, orgB).Scan(&cross); err != nil || cross != 0 {
			t.Fatalf("cross-tenant indexed rows=%d: %v", cross, err)
		}
	})

	t.Run("atomic report occurrence claim across replicas", func(t *testing.T) {
		reports := NewReportScheduler(workerPool)
		var wait sync.WaitGroup
		failures := make(chan error, 2)
		for replica := 0; replica < 2; replica++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				failures <- database.WithTenantConnection(ctx, workerPool, orgA, func(tenantCtx context.Context) error {
					return reports.processSchedule(tenantCtx, fixtureA.scheduleID, orgA, fixtureA.scheduledAt)
				})
			}()
		}
		wait.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
		var runs int
		if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM report_runs WHERE schedule_id=$1::uuid`, fixtureA.scheduleID).Scan(&runs); err != nil || runs != 1 {
			t.Fatalf("concurrent schedule runs=%d: %v", runs, err)
		}
		var next, last time.Time
		if err := adminPool.QueryRow(ctx, `SELECT next_run_at,last_run_at FROM report_schedules WHERE id=$1::uuid`, fixtureA.scheduleID).Scan(&next, &last); err != nil {
			t.Fatal(err)
		}
		location, _ := time.LoadLocation("Europe/London")
		if !last.Equal(fixtureA.scheduledAt) || !next.After(time.Now()) || next.In(location).Format("15:04") != "08:15" {
			t.Fatalf("schedule advance last=%s next=%s", last, next)
		}
	})
}

func seedSchedulerRoleFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID string) schedulerRoleFixture {
	t.Helper()
	fixture := schedulerRoleFixture{userID: uuid.NewString(), dueEntities: make(map[string]string)}
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, statement, args...); err != nil {
			t.Fatalf("scheduler fixture: %v\n%s", err, statement)
		}
	}
	exec(`INSERT INTO users(id,organization_id,email,first_name,last_name,status) VALUES($1::uuid,$2::uuid,$3,'Runtime','Owner','active')`, fixture.userID, organizationID, "runtime-"+fixture.userID+"@example.invalid")
	var today time.Time
	if err := pool.QueryRow(ctx, `SELECT CURRENT_DATE`).Scan(&today); err != nil {
		t.Fatal(err)
	}
	for _, due := range []bool{true, false} {
		deadline := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Microsecond)
		date := today.AddDate(0, 0, 45)
		label := "future"
		if due {
			deadline = time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
			date = today.AddDate(0, 0, -1)
			label = "due"
		}
		incidentID, reportID, policyID, auditID, findingID, vendorID, riskID, dsrID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		exec(`INSERT INTO incidents(id,organization_id,incident_ref,title,description,category,severity,reporter_id,detected_at,is_data_breach,is_breach_notifiable,breach_assessment_status,breach_assessed_at,breach_assessed_by,breach_assessment_reason,breach_awareness_at,notification_deadline)
		VALUES($1::uuid,$2::uuid,$3,'Runtime incident','Fixture description','security','high',$4::uuid,NOW(),true,true,'notifiable',NOW(),$4::uuid,'Live fixture',$5::timestamptz-INTERVAL '72 hours',$5)`, incidentID, organizationID, "INC-"+label, fixture.userID, deadline)
		phaseStatus := "pending"
		var submittedAt *time.Time
		if due {
			phaseStatus = "submitted"
			submittedAt = &deadline
		}
		exec(`INSERT INTO nis2_incident_reports(id,organization_id,incident_id,report_ref,early_warning_deadline,notification_deadline,notification_status,notification_submitted_at,final_report_deadline,final_report_status)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$5,$6::nis2_report_phase_status,$7,$5,'not_required')`, reportID, organizationID, incidentID, "NIS2-"+label, deadline, phaseStatus, submittedAt)
		exec(`INSERT INTO policies(id,organization_id,policy_ref,title,status,owner_user_id,next_review_date) VALUES($1::uuid,$2::uuid,$3,'Runtime policy','approved',$4::uuid,$5::date)`, policyID, organizationID, "POL-"+label, fixture.userID, date)
		exec(`INSERT INTO audits(id,organization_id,audit_ref,title,audit_type,lead_auditor_id,scope,scheduled_start_date,scheduled_end_date,created_by)
		VALUES($1::uuid,$2::uuid,$3,'Runtime audit','internal',$4::uuid,'Fixture scope',CURRENT_DATE,CURRENT_DATE,$4::uuid)`, auditID, organizationID, "AUD-"+label, fixture.userID)
		exec(`INSERT INTO audit_findings(id,organization_id,audit_id,finding_ref,title,description,severity,finding_type,recommendation,responsible_user_id,due_date,created_by)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,'Runtime finding','Fixture description','high','nonconformity','Fixture recommendation',$5::uuid,$6::date,$5::uuid)`, findingID, organizationID, auditID, "FIND-"+label, fixture.userID, date)
		exec(`INSERT INTO vendors(id,organization_id,vendor_ref,name,status,owner_user_id,created_by,next_assessment_date)
		VALUES($1::uuid,$2::uuid,$3,$6,'active',$4::uuid,$4::uuid,$5::date)`, vendorID, organizationID, "VEND-"+label, fixture.userID, date, "Runtime vendor "+label)
		exec(`INSERT INTO risks(id,organization_id,risk_ref,title,owner_user_id,residual_risk_level,next_review_date)
		VALUES($1::uuid,$2::uuid,$3,'Runtime risk',$4::uuid,'high',$5::date)`, riskID, organizationID, "RISK-"+label, fixture.userID, date)
		exec(`INSERT INTO dsr_requests(id,organization_id,request_ref,request_type,status,data_subject_name_encrypted,data_subject_email_encrypted,request_description,received_date,response_deadline,assigned_to)
		VALUES($1::uuid,$2::uuid,$3,'access','in_progress','PII-CIPHERTEXT-SENTINEL','PII-CIPHERTEXT-SENTINEL','Fixture',$4::date-30,$4::date,$5::uuid)`, dsrID, organizationID, "DSR-"+label, date, fixture.userID)
		if due {
			fixture.policyID, fixture.dsrDueID = policyID, dsrID
			fixture.dueEntities = map[string]string{"gdpr.breach_deadline_exceeded": incidentID, "nis2.deadline_exceeded": reportID, "policy.review_overdue": policyID, "finding.remediation_overdue": findingID, "vendor.assessment_overdue": vendorID, "risk.review_overdue": riskID, "dsr.deadline_exceeded": dsrID}
			versionID := uuid.NewString()
			exec(`INSERT INTO policy_versions(id,policy_id,organization_id,version_number,version_label,title,content_text) VALUES($1::uuid,$2::uuid,$3::uuid,1,'1.0','Runtime version','Canonical version content')`, versionID, policyID, organizationID)
			exec(`UPDATE policies SET current_version_id=$1::uuid WHERE id=$2::uuid`, versionID, policyID)
		}
	}
	fixture.dsrExtendedID = uuid.NewString()
	exec(`INSERT INTO dsr_requests(id,organization_id,request_ref,request_type,status,data_subject_name_encrypted,data_subject_email_encrypted,request_description,received_date,response_deadline,extended_deadline,assigned_to,sla_status)
	VALUES($1::uuid,$2::uuid,'DSR-extended','access','extended','PII-CIPHERTEXT-SENTINEL','PII-CIPHERTEXT-SENTINEL','Fixture',CURRENT_DATE-31,CURRENT_DATE-1,CURRENT_DATE+45,$3::uuid,'overdue')`, fixture.dsrExtendedID, organizationID, fixture.userID)
	for _, exception := range []struct {
		id                     *string
		label                  string
		expiryDays, reviewDays int
	}{
		{&fixture.expiringExceptionID, "reminder", 5, -1},
		{&fixture.expiredExceptionID, "expired", -1, 45},
		{new(string), "future", 45, 45},
	} {
		*exception.id = uuid.NewString()
		exec(`INSERT INTO compliance_exceptions(id,organization_id,exception_ref,title,exception_type,status,scope_type,risk_justification,requested_by,effective_date,expiry_date,next_review_date)
		VALUES($1::uuid,$2::uuid,$3,'Runtime exception','temporary','approved','policy_requirement','Live fixture',$4::uuid,CURRENT_DATE-30,$5::date,$6::date)`, *exception.id, organizationID, "EXC-"+exception.label, fixture.userID, today.AddDate(0, 0, exception.expiryDays), today.AddDate(0, 0, exception.reviewDays))
	}
	for _, event := range []struct {
		id        *string
		offset    int
		status    string
		reminders []int32
	}{
		{&fixture.calendarReminderID, 3, "upcoming", []int32{3}},
		{&fixture.calendarEscalationID, -5, "overdue", []int32{}},
		{&fixture.calendarPastID, -1, "upcoming", []int32{}},
		{&fixture.calendarTodayID, 0, "upcoming", []int32{}},
	} {
		*event.id = uuid.NewString()
		exec(`INSERT INTO calendar_events(id,organization_id,title,event_type,source_entity_type,source_entity_id,start_date,status,assigned_to,reminder_days_before)
		VALUES($1::uuid,$2::uuid,'Runtime calendar','custom','custom',$1::uuid,$3::date,$4,$5::uuid,$6::integer[])`, *event.id, organizationID, today.AddDate(0, 0, event.offset), event.status, fixture.userID, event.reminders)
	}
	definitionID := uuid.NewString()
	fixture.scheduleID = uuid.NewString()
	location, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().In(location).AddDate(0, 0, -1)
	fixture.scheduledAt = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 8, 15, 0, 0, location)
	exec(`INSERT INTO report_definitions(id,organization_id,name,report_type,format) VALUES($1::uuid,$2::uuid,'Runtime report','custom','json')`, definitionID, organizationID)
	exec(`INSERT INTO report_schedules(id,organization_id,report_definition_id,frequency,time_of_day,timezone,next_run_at) VALUES($1::uuid,$2::uuid,$3::uuid,'daily','08:15','Europe/London',$4)`, fixture.scheduleID, organizationID, definitionID, fixture.scheduledAt)
	return fixture
}
