package service

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

	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
)

type concurrentEmailRecorder struct {
	mu       sync.Mutex
	messages []emailpkg.Message
	err      error
}

func (sender *concurrentEmailRecorder) Send(_ context.Context, message emailpkg.Message) error {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.messages = append(sender.messages, message)
	return sender.err
}

func (sender *concurrentEmailRecorder) snapshot() []emailpkg.Message {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return append([]emailpkg.Message(nil), sender.messages...)
}

// TestNotificationDeliveryWithNonSuperuserRLS is opt-in because it creates a
// temporary NOSUPERUSER/NOBYPASSRLS role. It covers global tenant discovery,
// tenant-scoped SKIP LOCKED claims, concurrent workers, fencing, acknowledgement
// isolation, bounded retries/dead state, and redacted persisted failures.
func TestNotificationDeliveryWithNonSuperuserRLS(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	var migrationReady bool
	if err := admin.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.columns
		WHERE table_schema='public' AND table_name='notifications' AND column_name='lease_token')`).Scan(&migrationReady); err != nil {
		t.Fatal(err)
	}
	if !migrationReady {
		t.Fatal("notification reliability migration 000044 is not applied")
	}

	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, userB := uuid.NewString(), uuid.NewString()
	channelA, channelB := uuid.NewString(), uuid.NewString()
	roleName := "grc_notification_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Notification RLS A',$3,'active','starter'),($2,'Notification RLS B',$4,'active','starter')`,
		orgA, orgB, "notification-rls-a-"+suffix, "notification-rls-b-"+suffix); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
		_, _ = admin.Exec(context.Background(), "DROP OWNED BY "+quotedRole)
		_, _ = admin.Exec(context.Background(), "DROP ROLE "+quotedRole)
	})
	if _, err := admin.Exec(ctx, `INSERT INTO users (id,organization_id,email,status) VALUES
		($1,$2,$3,'active'),($4,$5,$6,'active')`, userA, orgA, userA+"@example.test", userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO notification_channels
		(id,organization_id,channel_type,name,configuration) VALUES
		($1,$2,'email','Tenant A email','{}'),($3,$4,'email','Tenant B email','{}')`,
		channelA, orgA, channelB, orgB); err != nil {
		t.Fatal(err)
	}

	const perTenant = 6
	notificationIDs := map[string][]string{orgA: {}, orgB: {}}
	for _, tenant := range []struct {
		orgID, userID, channelID string
	}{{orgA, userA, channelA}, {orgB, userB, channelB}} {
		for index := 0; index < perTenant; index++ {
			notificationID, eventID := uuid.NewString(), uuid.NewString()
			notificationIDs[tenant.orgID] = append(notificationIDs[tenant.orgID], notificationID)
			if _, err := admin.Exec(ctx, `INSERT INTO notifications
				(id,organization_id,event_id,event_type,event_payload,recipient_user_id,channel_type,channel_id,
				 subject,body,body_text,status,delivery_key,scheduled_for,created_at)
				VALUES ($1,$2,$3,'risk.changed','{}',$4,'email',$5,$6,$7,$7,'pending',$8,NOW()-INTERVAL '1 minute',NOW())`,
				notificationID, tenant.orgID, eventID, tenant.userID, tenant.channelID,
				fmt.Sprintf("Notification %d", index), fmt.Sprintf("Body %d", index),
				notificationDeliveryKey(eventID, "rule", tenant.userID, tenant.channelID)); err != nil {
				t.Fatal(err)
			}
		}
	}

	if _, err := admin.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatalf("create non-superuser role: %v", err)
	}
	grants := "GRANT USAGE ON SCHEMA public TO " + quotedRole + ";" +
		"GRANT SELECT ON users,notification_channels,notification_rules,notifications TO " + quotedRole + ";" +
		"GRANT INSERT,UPDATE ON notifications TO " + quotedRole + ";" +
		"GRANT EXECUTE ON FUNCTION notification_due_tenants(INTEGER) TO " + quotedRole
	if _, err := admin.Exec(ctx, grants); err != nil {
		t.Fatal(err)
	}

	appConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	appConfig.MaxConns = 8
	appConfig.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		_, err := connection.Exec(ctx, "SET ROLE "+quotedRole)
		return err
	}
	appPool, err := pgxpool.NewWithConfig(ctx, appConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(appPool.Close)

	sender := &concurrentEmailRecorder{}
	engineA := NewNotificationEngine(appPool, NewEventBus(), sender)
	engineB := NewNotificationEngine(appPool, NewEventBus(), sender)
	configFor := func(owner string) NotificationDeliveryConfig {
		return NotificationDeliveryConfig{
			OwnerID: owner, TenantBatch: 10, ClaimBatch: 1, LeaseDuration: 3 * time.Minute,
			RetryBaseDelay: time.Second, RetryMaxDelay: time.Minute, PollInterval: time.Second,
		}
	}

	var workers sync.WaitGroup
	workerErrors := make(chan error, 2)
	for _, worker := range []struct {
		engine *NotificationEngine
		config NotificationDeliveryConfig
	}{{engineA, configFor(uuid.NewString())}, {engineB, configFor(uuid.NewString())}} {
		worker := worker
		workers.Add(1)
		go func() {
			defer workers.Done()
			for cycle := 0; cycle < perTenant; cycle++ {
				if err := worker.engine.RunDeliveryCycle(ctx, worker.config); err != nil {
					workerErrors <- err
					return
				}
			}
		}()
	}
	workers.Wait()
	close(workerErrors)
	for err := range workerErrors {
		t.Fatalf("concurrent delivery: %v", err)
	}

	messages := sender.snapshot()
	if len(messages) != perTenant*2 {
		t.Fatalf("delivered messages=%d want=%d", len(messages), perTenant*2)
	}
	seenDeliveryIDs := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		deliveryID := message.Headers["X-ComplianceForge-Notification-ID"]
		if _, duplicate := seenDeliveryIDs[deliveryID]; duplicate {
			t.Fatalf("notification %s was delivered more than once", deliveryID)
		}
		seenDeliveryIDs[deliveryID] = struct{}{}
	}

	connection, err := appPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SELECT set_config('app.current_tenant',$1,false)`, orgA); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err := connection.QueryRow(ctx, `SELECT COUNT(*) FROM notifications`).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != perTenant {
		t.Fatalf("tenant A visible notifications=%d want=%d", visible, perTenant)
	}
	result, err := connection.Exec(ctx, `UPDATE notifications SET subject='cross-tenant' WHERE id=$1`, notificationIDs[orgB][0])
	if err != nil || result.RowsAffected() != 0 {
		t.Fatalf("cross-tenant update rows=%d err=%v", result.RowsAffected(), err)
	}

	acknowledgedAt, err := engineA.AcknowledgeNotification(ctx, orgA, userA, notificationIDs[orgA][0])
	if err != nil {
		t.Fatalf("acknowledge own notification: %v", err)
	}
	repeatedAt, err := engineA.AcknowledgeNotification(ctx, orgA, userA, notificationIDs[orgA][0])
	if err != nil || !repeatedAt.Equal(acknowledgedAt) {
		t.Fatalf("idempotent acknowledgement=%s first=%s err=%v", repeatedAt, acknowledgedAt, err)
	}
	if _, err := engineA.AcknowledgeNotification(ctx, orgA, userA, notificationIDs[orgB][0]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant acknowledgement error=%v", err)
	}

	if err := engineA.RecordNotificationBounce(ctx, orgA, notificationIDs[orgA][1], "smtp.550"); err != nil {
		t.Fatalf("record bounce: %v", err)
	}
	var bounceStatus, bounceError string
	var bounceDeadAt *time.Time
	if err := connection.QueryRow(ctx, `SELECT status,error_message,dead_at FROM notifications WHERE id=$1`, notificationIDs[orgA][1]).
		Scan(&bounceStatus, &bounceError, &bounceDeadAt); err != nil {
		t.Fatal(err)
	}
	if bounceStatus != "bounced" || bounceDeadAt == nil || bounceError != "notification bounced (smtp.550)" {
		t.Fatalf("bounce status=%s error=%q dead_at=%v", bounceStatus, bounceError, bounceDeadAt)
	}

	// A transient provider failure is retried once, then reaches a terminal dead
	// state without persisting the provider's potentially secret error text.
	retryID, retryEventID := uuid.NewString(), uuid.NewString()
	if _, err := admin.Exec(ctx, `INSERT INTO notifications
		(id,organization_id,event_id,event_type,event_payload,recipient_user_id,channel_type,channel_id,
		 subject,body,body_text,status,max_retries,delivery_key,scheduled_for,created_at)
		VALUES ($1,$2,$3,'risk.retry','{}',$4,'email',$5,'Retry','Retry body','Retry body','pending',2,$6,NOW()-INTERVAL '1 minute',NOW())`,
		retryID, orgA, retryEventID, userA, channelA,
		notificationDeliveryKey(retryEventID, "rule", userA, channelA)); err != nil {
		t.Fatal(err)
	}
	failingSender := &concurrentEmailRecorder{err: errors.New("smtp://user:secret@example.test private token")}
	failingEngine := NewNotificationEngine(appPool, NewEventBus(), failingSender)
	failureConfig := configFor(uuid.NewString())
	failureConfig.ClaimBatch = 10
	if err := failingEngine.RunDeliveryCycle(ctx, failureConfig); err != nil {
		t.Fatalf("first retry cycle state update: %v", err)
	}
	if _, err := admin.Exec(ctx, `UPDATE notifications SET next_retry_at=NOW()-INTERVAL '1 second' WHERE id=$1`, retryID); err != nil {
		t.Fatal(err)
	}
	if err := failingEngine.RunDeliveryCycle(ctx, failureConfig); err != nil {
		t.Fatalf("terminal retry cycle state update: %v", err)
	}
	var retryCount int
	var deadAt, nextRetryAt *time.Time
	var storedError, failureCode string
	if err := admin.QueryRow(ctx, `SELECT retry_count,dead_at,next_retry_at,error_message,failure_code FROM notifications WHERE id=$1`, retryID).
		Scan(&retryCount, &deadAt, &nextRetryAt, &storedError, &failureCode); err != nil {
		t.Fatal(err)
	}
	if retryCount != 2 || deadAt == nil || nextRetryAt != nil || failureCode != "transport_failure" {
		t.Fatalf("retry_count=%d dead=%v next=%v code=%s", retryCount, deadAt, nextRetryAt, failureCode)
	}
	if strings.Contains(storedError, "secret") || strings.Contains(storedError, "example.test") {
		t.Fatalf("stored failure leaked provider detail: %q", storedError)
	}

	// Two notifications in one digest bucket are claimed independently but sent
	// as one provider message with a stable digest idempotency key.
	digestScheduledFor := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	for index := 0; index < 2; index++ {
		digestID, digestEventID := uuid.NewString(), uuid.NewString()
		if _, err := admin.Exec(ctx, `INSERT INTO notifications
			(id,organization_id,event_id,event_type,event_payload,recipient_user_id,channel_type,channel_id,
			 subject,body,body_text,status,delivery_key,digest_frequency,scheduled_for,created_at)
			VALUES ($1,$2,$3,'risk.digest','{}',$4,'email',$5,$6,$7,$7,'pending',$8,'hourly',$9,NOW())`,
			digestID, orgA, digestEventID, userA, channelA, fmt.Sprintf("Digest %d", index),
			fmt.Sprintf("Digest body %d", index), notificationDeliveryKey(digestEventID, "digest-rule", userA, channelA),
			digestScheduledFor); err != nil {
			t.Fatal(err)
		}
	}
	messageCountBeforeDigest := len(sender.snapshot())
	digestConfig := configFor(uuid.NewString())
	digestConfig.ClaimBatch = 10
	if err := engineA.RunDeliveryCycle(ctx, digestConfig); err != nil {
		t.Fatalf("digest delivery cycle: %v", err)
	}
	digestMessages := sender.snapshot()
	if len(digestMessages) != messageCountBeforeDigest+1 ||
		!strings.Contains(digestMessages[len(digestMessages)-1].Subject, "2 notifications") {
		t.Fatalf("digest messages before=%d after=%d final=%#v", messageCountBeforeDigest, len(digestMessages), digestMessages[len(digestMessages)-1])
	}

	// A delivered notification whose acknowledgement SLA expires creates one
	// idempotent child per escalation channel and delivers it in the same cycle.
	escalationRuleID, escalationParentID, escalationEventID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := admin.Exec(ctx, `INSERT INTO notification_rules
		(id,organization_id,name,event_type,channel_ids,recipient_type,recipient_ids,
		 escalation_after_minutes,escalation_channel_ids)
		VALUES ($1,$2,'Acknowledgement SLA','risk.ack',ARRAY[$3::uuid],'user',ARRAY[$4::uuid],1,ARRAY[$3::uuid])`,
		escalationRuleID, orgA, channelA, userA); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO notifications
		(id,organization_id,rule_id,event_id,event_type,event_payload,recipient_user_id,channel_type,channel_id,
		 subject,body,body_text,status,sent_at,delivery_key,scheduled_for,acknowledgement_due_at,created_at)
		VALUES ($1,$2,$3,$4,'risk.ack','{}',$5,'email',$6,'Acknowledge risk','Please acknowledge',
		        'Please acknowledge','sent',NOW()-INTERVAL '2 minutes',$7,NOW()-INTERVAL '3 minutes',
		        NOW()-INTERVAL '1 minute',NOW()-INTERVAL '3 minutes')`,
		escalationParentID, orgA, escalationRuleID, escalationEventID, userA, channelA,
		notificationDeliveryKey(escalationEventID, escalationRuleID, userA, channelA)); err != nil {
		t.Fatal(err)
	}
	messageCountBeforeEscalation := len(sender.snapshot())
	if err := engineA.RunDeliveryCycle(ctx, configFor(uuid.NewString())); err != nil {
		t.Fatalf("escalation delivery cycle: %v", err)
	}
	if got := len(sender.snapshot()); got != messageCountBeforeEscalation+1 {
		t.Fatalf("escalation provider messages=%d want=%d", got, messageCountBeforeEscalation+1)
	}
	var escalatedAt *time.Time
	var escalationChildren int
	if err := admin.QueryRow(ctx, `SELECT escalated_at FROM notifications WHERE id=$1`, escalationParentID).Scan(&escalatedAt); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE parent_notification_id=$1 AND status='sent'`, escalationParentID).Scan(&escalationChildren); err != nil {
		t.Fatal(err)
	}
	if escalatedAt == nil || escalationChildren != 1 {
		t.Fatalf("escalated_at=%v delivered_children=%d", escalatedAt, escalationChildren)
	}
}
