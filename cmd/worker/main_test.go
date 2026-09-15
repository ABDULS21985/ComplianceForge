package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/service"
)

type outboxRecorder struct {
	queue     string
	envelopes []queuepkg.Envelope
	err       error
}

func (r *outboxRecorder) Enqueue(_ context.Context, _ database.Querier, queueName string, envelope queuepkg.Envelope) error {
	r.queue = queueName
	r.envelopes = append(r.envelopes, envelope)
	return r.err
}

type taskRunnerStub struct {
	acquired bool
	err      error
}

func (r taskRunnerStub) Run(ctx context.Context, _ string, handler func(context.Context) error) (bool, error) {
	if r.err != nil || !r.acquired {
		return r.acquired, r.err
	}
	return true, handler(ctx)
}

func TestRegisterJobHandlersRegistersCompleteContract(t *testing.T) {
	dispatcher := queuepkg.NewDispatcher()
	if err := registerJobHandlers(dispatcher, workerComponents{}, taskRunnerStub{acquired: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"notification.event",
		"scheduler.analytics",
		"scheduler.calendar.overdue",
		"scheduler.calendar.reminders",
		"scheduler.calendar.status",
		"scheduler.dsr",
		"scheduler.evidence",
		"scheduler.exceptions",
		"scheduler.notifications.delivery",
		"scheduler.regulatory",
		"scheduler.reports",
		"scheduler.search.health",
		"scheduler.search.reindex",
		"scheduler.workflows",
		"search.index",
	}
	if got := dispatcher.Types(); !reflect.DeepEqual(got, want) {
		t.Fatalf("registered types = %v, want %v", got, want)
	}
}

func TestDecodePayloadIsStrict(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	var decoded payload
	if err := decodePayload(json.RawMessage(`{"name":"job"}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != "job" {
		t.Fatalf("decoded payload = %+v", decoded)
	}
	if err := decodePayload(json.RawMessage(`{"name":"job","unknown":true}`), &decoded); err == nil {
		t.Fatal("expected unknown-field error")
	}
	if err := decodePayload(json.RawMessage(`{"name":"job"} {"name":"second"}`), &decoded); err == nil {
		t.Fatal("expected multiple-value error")
	}
}

func TestValidateSearchIndexJob(t *testing.T) {
	envelope, err := queuepkg.NewEnvelope("search.index", uuid.NewString(), map[string]bool{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	valid := searchIndexJob{EntityType: "risk", EntityID: uuid.NewString(), Action: "updated"}
	action, err := validateSearchIndexJob(envelope, valid)
	if err != nil || action != "upsert" {
		t.Fatalf("valid job action=%q err=%v", action, err)
	}

	tests := []struct {
		name     string
		envelope queuepkg.Envelope
		job      searchIndexJob
	}{
		{name: "tenant", envelope: queuepkg.Envelope{}, job: valid},
		{name: "entity id", envelope: envelope, job: searchIndexJob{EntityType: "risk", EntityID: "not-a-uuid", Action: "updated"}},
		{name: "entity type", envelope: envelope, job: searchIndexJob{EntityType: "user", EntityID: uuid.NewString(), Action: "updated"}},
		{name: "action", envelope: envelope, job: searchIndexJob{EntityType: "risk", EntityID: uuid.NewString(), Action: "execute"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := validateSearchIndexJob(tt.envelope, tt.job); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSystemJobRejectsTenantScopedTrigger(t *testing.T) {
	called := false
	handler := systemJob("scheduler.test", taskRunnerStub{acquired: true}, func(context.Context) error {
		called = true
		return nil
	})
	err := handler(context.Background(), queuepkg.Envelope{TenantID: uuid.NewString()})
	var permanent *queuepkg.PermanentError
	if !errors.As(err, &permanent) || called {
		t.Fatalf("tenant system job result err=%v called=%t", err, called)
	}
	if err := handler(context.Background(), queuepkg.Envelope{}); err != nil || !called {
		t.Fatalf("global system job result err=%v called=%t", err, called)
	}
}

func TestSystemJobRetriesWhenAnotherReplicaOwnsTask(t *testing.T) {
	handler := systemJob("scheduler.test", taskRunnerStub{}, func(context.Context) error {
		t.Fatal("busy task handler must not run")
		return nil
	})
	if err := handler(context.Background(), queuepkg.Envelope{}); err == nil {
		t.Fatal("expected retryable busy-task error")
	}
}

func TestWorkerShutdownPeriod(t *testing.T) {
	t.Setenv("WORKER_SHUTDOWN_TIMEOUT", "45s")
	period, err := workerShutdownPeriod()
	if err != nil || period != 45*time.Second {
		t.Fatalf("workerShutdownPeriod() = %s, %v", period, err)
	}
	t.Setenv("WORKER_SHUTDOWN_TIMEOUT", "never")
	if _, err := workerShutdownPeriod(); err == nil {
		t.Fatal("expected invalid shutdown period error")
	}
}

func TestBridgeEventsBuildsTenantAwareNotificationEnvelope(t *testing.T) {
	orgID := uuid.NewString()
	entityID := uuid.NewString()
	eventID := uuid.NewString()
	thresholdTime := time.Date(2026, time.September, 14, 8, 0, 0, 0, time.UTC)
	events := make(chan service.Event, 1)
	events <- service.Event{
		ID: eventID, Type: "risk.created", OrgID: orgID, EntityType: "risk", EntityID: entityID,
		Data: map[string]interface{}{"title": "Concentration risk"}, Timestamp: thresholdTime,
	}
	close(events)
	recorder := &outboxRecorder{}
	if err := bridgeEvents(context.Background(), events, recorder, new(pgxpool.Pool), "worker.test"); err != nil {
		t.Fatal(err)
	}
	if recorder.queue != "worker.test" || len(recorder.envelopes) != 1 {
		t.Fatalf("recorded queue=%q envelopes=%d", recorder.queue, len(recorder.envelopes))
	}
	envelope := recorder.envelopes[0]
	if envelope.Type != "notification.event" || envelope.TenantID != orgID || envelope.CausationID != entityID {
		t.Fatalf("notification envelope = %+v", envelope)
	}
	if envelope.ID != eventID || envelope.CorrelationID != eventID || !envelope.CreatedAt.Equal(thresholdTime) {
		t.Fatalf("stable event identity was not preserved: %+v", envelope)
	}
	if envelope.Metadata["entity_type"] != "risk" || envelope.Metadata["entity_id"] != entityID {
		t.Fatalf("notification metadata = %+v", envelope.Metadata)
	}
}

func TestBridgeEventsKeepsFirstDeterministicOccurrenceSnapshot(t *testing.T) {
	events := make(chan service.Event, 1)
	events <- service.Event{ID: uuid.NewString(), Type: "risk.review_due_soon", OrgID: uuid.NewString()}
	close(events)
	recorder := &outboxRecorder{err: queuepkg.ErrOutboxConflict}
	if err := bridgeEvents(context.Background(), events, recorder, new(pgxpool.Pool), "worker.test"); err != nil {
		t.Fatalf("duplicate scheduled occurrence stopped bridge: %v", err)
	}
}

func TestBridgeEventsPropagatesPublisherFailure(t *testing.T) {
	events := make(chan service.Event, 1)
	events <- service.Event{Type: "risk.created", OrgID: uuid.NewString(), EntityType: "risk", EntityID: uuid.NewString()}
	recorder := &outboxRecorder{err: errors.New("database unavailable")}
	if err := bridgeEvents(context.Background(), events, recorder, new(pgxpool.Pool), "worker.test"); err == nil {
		t.Fatal("expected publisher error")
	}
}

func TestWorkerInstanceIDAndSchedulerLeaseValidation(t *testing.T) {
	instanceID := uuid.NewString()
	t.Setenv("WORKER_INSTANCE_ID", instanceID)
	if got, err := workerInstanceID(); err != nil || got != instanceID {
		t.Fatalf("workerInstanceID() = %q, %v", got, err)
	}
	t.Setenv("WORKER_INSTANCE_ID", "not-a-uuid")
	if _, err := workerInstanceID(); err == nil {
		t.Fatal("expected invalid worker instance ID")
	}

	t.Setenv("WORKER_SCHEDULER_LEASE", "45s")
	if got, err := workerSchedulerLease(); err != nil || got != 45*time.Second {
		t.Fatalf("workerSchedulerLease() = %s, %v", got, err)
	}
	t.Setenv("WORKER_SCHEDULER_LEASE", "1s")
	if _, err := workerSchedulerLease(); err == nil {
		t.Fatal("expected unsafe scheduler lease error")
	}
}

func TestWorkerEmailSenderValidatesTransportAtStartup(t *testing.T) {
	sender, err := workerEmailSender(config.SMTPConfig{
		Host: "mail.example.com", Port: 587, User: "worker", Password: "secret",
		From: "ComplianceForge <notifications@example.com>", TLSMode: "starttls",
		TimeoutSeconds: 15, HelloName: "worker.example.com", ServerName: "mail.example.com",
	})
	if err != nil || sender == nil {
		t.Fatalf("workerEmailSender() = %T, %v", sender, err)
	}
	if _, err := workerEmailSender(config.SMTPConfig{
		Host: "mail.example.com", Port: 587, From: "not-an-address", TLSMode: "starttls", TimeoutSeconds: 15,
	}); err == nil {
		t.Fatal("expected invalid SMTP transport to fail worker startup")
	}
}
