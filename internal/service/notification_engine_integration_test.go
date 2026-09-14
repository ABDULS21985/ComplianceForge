package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNotificationEngineAgainstMigratedPostgres(t *testing.T) {
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
	userA := uuid.NewString()
	channelA, channelB := uuid.NewString(), uuid.NewString()
	templateID, ruleID := uuid.NewString(), uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Notification A',$3,'active','starter'),
		($2,'Notification B',$4,'active','starter')`,
		orgA, orgB, "notification-a-"+suffix, "notification-b-"+suffix); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = ANY($1::uuid[])`, []string{orgA, orgB})
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id,organization_id,email,status)
		VALUES ($1,$2,$3,'active')`, userA, orgA, "notification-"+suffix+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO notification_templates
			(id,organization_id,name,event_type,subject_template,body_text_template,body_html_template)
		VALUES ($1,$2,'Risk escalated','risk.escalated','Risk {{.risk_title}}',
			'Risk {{.risk_title}} is {{.severity}}','<p>Risk <strong>{{.risk_title}}</strong> is {{.severity}}</p>')`,
		templateID, orgA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO notification_channels (id,organization_id,channel_type,name,configuration)
		VALUES ($1,$3,'email','Tenant A email','{}'),($2,$4,'email','Tenant B email','{}')`,
		channelA, channelB, orgA, orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO notification_rules
			(id,organization_id,name,event_type,severity_filter,conditions,channel_ids,
			 recipient_type,recipient_ids,template_id,cooldown_minutes)
		VALUES ($1,$2,'Critical risks','risk.escalated',ARRAY['critical'],
			'{"requires_review":true}',ARRAY[$3::uuid],'user',ARRAY[$4::uuid],$5,60)`,
		ruleID, orgA, channelA, userA, templateID); err != nil {
		t.Fatal(err)
	}

	sender := &recordingEmailSender{}
	engine := NewNotificationEngine(pool, NewEventBus(), sender)
	event := Event{
		ID:         uuid.NewString(),
		Type:       "risk.escalated",
		Severity:   "critical",
		OrgID:      orgA,
		EntityType: "risk",
		EntityID:   uuid.NewString(),
		EntityRef:  "RISK-001",
		Data: map[string]any{
			"risk_title":      "<script>alert(1)</script>",
			"requires_review": true,
		},
		Timestamp: time.Now().UTC(),
	}
	if err := engine.ProcessEvent(ctx, event); err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("ProcessEvent synchronously delivered %d messages, want durable enqueue only", len(sender.messages))
	}
	deliveryConfig := NotificationDeliveryConfig{
		OwnerID: uuid.NewString(), TenantBatch: 10, ClaimBatch: 10,
		LeaseDuration: time.Minute, RetryBaseDelay: time.Second,
		RetryMaxDelay: time.Minute, PollInterval: time.Second,
	}
	if err := engine.RunDeliveryCycle(ctx, deliveryConfig); err != nil {
		t.Fatalf("RunDeliveryCycle() error = %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("delivered messages = %d, want 1", len(sender.messages))
	}
	if strings.Contains(sender.messages[0].HTMLBody, "<script>") || !strings.Contains(sender.messages[0].HTMLBody, "&lt;script&gt;") {
		t.Fatalf("HTML template did not escape event data: %s", sender.messages[0].HTMLBody)
	}

	var status, storedEntityID string
	var sentAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT status, event_payload->>'entity_id', sent_at
		FROM notifications
		WHERE organization_id=$1 AND rule_id=$2`, orgA, ruleID).
		Scan(&status, &storedEntityID, &sentAt); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || sentAt == nil || storedEntityID != event.EntityID {
		t.Fatalf("stored notification status=%q sent_at=%v entity_id=%q", status, sentAt, storedEntityID)
	}

	if err := engine.ProcessEvent(ctx, event); err != nil {
		t.Fatalf("cooldown ProcessEvent() error = %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("cooldown delivered %d messages, want 1", len(sender.messages))
	}

	if _, err := pool.Exec(ctx, `UPDATE notification_rules SET channel_ids=ARRAY[$1::uuid] WHERE id=$2`, channelB, ruleID); err != nil {
		t.Fatal(err)
	}
	event.EntityID = uuid.NewString()
	if err := engine.ProcessEvent(ctx, event); err == nil {
		t.Fatal("ProcessEvent() accepted a channel owned by another tenant")
	}
	if len(sender.messages) != 1 {
		t.Fatal("cross-tenant channel caused a delivery")
	}
}
