// Package main starts the durable background worker process.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/observability"
	"github.com/complianceforge/platform/internal/pkg/coordination"
	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
	workerpkg "github.com/complianceforge/platform/internal/worker"
)

const (
	defaultWorkerQueue    = "complianceforge.worker"
	defaultShutdownPeriod = 30 * time.Second
	defaultSchedulerLease = 2 * time.Minute
)

type workerComponents struct {
	notifications        *service.NotificationEngine
	notificationDelivery service.NotificationDeliveryConfig
	analytics            *workerpkg.AnalyticsScheduler
	calendar             *workerpkg.CalendarWorker
	dsr                  *workerpkg.DSRScheduler
	evidence             *workerpkg.EvidenceScheduler
	exceptions           *workerpkg.ExceptionScheduler
	regulatory           *workerpkg.RegulatoryScheduler
	reports              *workerpkg.ReportScheduler
	search               *workerpkg.SearchIndexer
	workflows            *workerpkg.WorkflowScheduler
}

type scheduledTask struct {
	key       string
	name      string
	interval  time.Duration
	runOnBoot bool
	run       func(context.Context) error
}

type searchIndexJob struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Action     string `json:"action"`
}

type eventOutbox interface {
	Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error
}

type taskRunner interface {
	Run(context.Context, string, func(context.Context) error) (bool, error)
}

type observedTaskRunner struct {
	next    taskRunner
	metrics *observability.Metrics
}

func (r observedTaskRunner) Run(ctx context.Context, taskName string, work func(context.Context) error) (bool, error) {
	started := time.Now()
	observedWork := func(workCtx context.Context) error {
		r.metrics.WorkerStarted()
		defer r.metrics.WorkerFinished()
		return work(workCtx)
	}
	acquired, err := r.next.Run(ctx, taskName, observedWork)
	r.metrics.ObserveWorkerTask(taskName, acquired, time.Since(started), err)
	return acquired, err
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}
	level, err := zerolog.ParseLevel(cfg.Log.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	if cfg.App.Env == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	if err := run(ctx, cfg); err != nil {
		log.Fatal().Err(err).Msg("worker stopped with an error")
	}
}

func run(parentCtx context.Context, cfg *config.Config) error {
	shutdownPeriod, err := workerShutdownPeriod()
	if err != nil {
		return err
	}
	pool, err := database.NewPostgresPool(cfg)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()
	if cfg.App.Env == "production" {
		checkCtx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
		err := database.ValidateRuntimeDatabaseLoginIdentity(checkCtx, pool, pool.Config().ConnConfig.User)
		cancel()
		if err != nil {
			return fmt.Errorf("verify worker database login identity: %w", err)
		}
	}
	if err := verifyWorkerDatabasePosture(parentCtx, pool, cfg.App.Env == "production"); err != nil {
		return err
	}

	queueName := strings.TrimSpace(os.Getenv("WORKER_QUEUE_NAME"))
	if queueName == "" {
		queueName = defaultWorkerQueue
	}
	instanceID, err := workerInstanceID()
	if err != nil {
		return err
	}
	telemetry, err := observability.New(parentCtx, cfg.Observability, "complianceforge-worker", cfg.App.Env, instanceID)
	if err != nil {
		return fmt.Errorf("initialize observability: %w", err)
	}
	telemetry.Metrics().RegisterPostgresPool(pool)
	telemetry.Metrics().RegisterQueueStorage(pool)
	defer shutdownWorkerTelemetry(telemetry, cfg.Observability.ShutdownTimeoutSeconds)
	notificationDelivery, err := service.NotificationDeliveryConfigFromEnvironment(instanceID)
	if err != nil {
		return fmt.Errorf("load notification delivery configuration: %w", err)
	}
	queueConfig, err := queuepkg.ConfigFromEnvironment(cfg.RabbitMQ.URL)
	if err != nil {
		return fmt.Errorf("load queue configuration: %w", err)
	}
	if _, err := queuepkg.TopologyFor(queueName, queueConfig); err != nil {
		return err
	}
	deduplicator, err := queuepkg.NewPostgresDeduplicator(pool, queuepkg.PostgresDeduplicatorConfig{
		ConsumerName: queueName,
		OwnerID:      instanceID,
		Lease:        queueConfig.IdempotencyLease,
		Retention:    queueConfig.IdempotencyTTL,
	})
	if err != nil {
		return fmt.Errorf("create durable inbox: %w", err)
	}
	broker, err := queuepkg.NewRabbitMQServiceWithConfig(queueConfig, deduplicator)
	if err != nil {
		return fmt.Errorf("create queue service: %w", err)
	}
	broker.SetObserver(telemetry.Metrics())
	defer broker.Close()

	outboxConfig, err := queuepkg.OutboxConfigFromEnvironment()
	if err != nil {
		return fmt.Errorf("load outbox configuration: %w", err)
	}
	if outboxConfig.MaxMessageBytes > queueConfig.MaxMessageBytes {
		outboxConfig.MaxMessageBytes = queueConfig.MaxMessageBytes
	}
	outbox, err := queuepkg.NewPostgresOutbox(pool, instanceID, outboxConfig)
	if err != nil {
		return fmt.Errorf("create transactional outbox: %w", err)
	}
	outboxDispatcher, err := queuepkg.NewOutboxDispatcher(outbox, broker, outboxConfig)
	if err != nil {
		return fmt.Errorf("create outbox dispatcher: %w", err)
	}
	outboxDispatcher.SetObserver(telemetry.Metrics())
	schedulerLease, err := workerSchedulerLease()
	if err != nil {
		return err
	}
	leaseStore, err := coordination.NewPostgresLeaseStore(pool, instanceID)
	if err != nil {
		return fmt.Errorf("create scheduler lease store: %w", err)
	}
	coordinator, err := coordination.NewCoordinator(leaseStore, coordination.CoordinatorConfig{
		Lease: schedulerLease, ReleaseTimeout: 10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create scheduler coordinator: %w", err)
	}
	runner := observedTaskRunner{next: coordinator, metrics: telemetry.Metrics()}
	emailSender, err := workerEmailSender(cfg.SMTP)
	if err != nil {
		return fmt.Errorf("create SMTP email sender: %w", err)
	}
	notificationProtector, err := secretbox.NewHex(cfg.Encryption.NotificationKey)
	if err != nil {
		return fmt.Errorf("create notification secret protector: %w", err)
	}

	eventBus := service.NewEventBus()
	defer eventBus.Close()
	eventStream := eventBus.Subscribe("*")
	evidenceLifecycle, err := repository.NewEvidenceLifecycleRepository(
		pool,
		repository.WithEvidenceLifecycleOutbox(outbox, queueName),
	)
	if err != nil {
		return fmt.Errorf("create evidence lifecycle repository: %w", err)
	}
	components := workerComponents{
		notifications:        service.NewNotificationEngineWithProtector(pool, eventBus, emailSender, notificationProtector),
		notificationDelivery: notificationDelivery,
		analytics:            workerpkg.NewAnalyticsScheduler(pool),
		calendar:             workerpkg.NewCalendarWorker(pool),
		dsr:                  workerpkg.NewDSRScheduler(pool),
		evidence:             workerpkg.NewEvidenceScheduler(pool, eventBus, evidenceLifecycle),
		exceptions:           workerpkg.NewExceptionScheduler(pool, eventBus),
		regulatory:           workerpkg.NewRegulatoryScheduler(pool, eventBus),
		reports:              workerpkg.NewReportScheduler(pool),
		search:               workerpkg.NewSearchIndexer(pool),
		workflows:            workerpkg.NewWorkflowScheduler(pool),
	}
	tasks := scheduledTasks(components)
	tasks = append(tasks,
		scheduledTask{key: "maintenance.queue.inbox-retention", name: "queue inbox retention", interval: time.Hour, runOnBoot: false, run: func(ctx context.Context) error {
			_, err := deduplicator.PurgeExpired(ctx, 1000)
			return err
		}},
		scheduledTask{key: "maintenance.queue.outbox-retention", name: "queue outbox retention", interval: time.Hour, runOnBoot: false, run: func(ctx context.Context) error {
			_, err := outbox.PurgePublishedBefore(ctx, time.Now().UTC().Add(-outboxConfig.Retention), 1000)
			return err
		}},
	)
	taskNames := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskNames = append(taskNames, task.key)
	}
	telemetry.Metrics().RegisterWorkerTasks(taskNames)
	dispatcher := queuepkg.NewDispatcher()
	if err := registerJobHandlers(dispatcher, components, runner); err != nil {
		return err
	}
	postgresHealth := telemetry.Metrics().WrapDependencyCheck("postgres", func(ctx context.Context) error {
		return database.HealthCheck(ctx, pool)
	})
	rabbitHealth := telemetry.Metrics().WrapDependencyCheck("rabbitmq", broker.HealthCheck)
	readiness := func(ctx context.Context) error {
		return errors.Join(postgresHealth(ctx), rabbitHealth(ctx))
	}
	if err := telemetry.StartMetricsServer(readiness); err != nil {
		return err
	}

	workerCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	var workers sync.WaitGroup
	workerErrors := make(chan error, 3)

	workers.Add(1)
	go func() {
		defer workers.Done()
		if err := outboxDispatcher.Run(workerCtx); err != nil && workerCtx.Err() == nil {
			select {
			case workerErrors <- fmt.Errorf("dispatch transactional outbox: %w", err):
			default:
			}
		}
	}()

	workers.Add(1)
	go func() {
		defer workers.Done()
		if err := broker.SubscribeEnvelope(workerCtx, queueName, dispatcher.Handle); err != nil && workerCtx.Err() == nil {
			select {
			case workerErrors <- fmt.Errorf("consume worker queue: %w", err):
			default:
			}
		}
	}()

	workers.Add(1)
	go func() {
		defer workers.Done()
		if err := bridgeEvents(workerCtx, eventStream, outbox, pool, queueName); err != nil && workerCtx.Err() == nil {
			select {
			case workerErrors <- fmt.Errorf("bridge domain events: %w", err):
			default:
			}
		}
	}()

	for _, task := range tasks {
		task := task
		workers.Add(1)
		go func() {
			defer workers.Done()
			runScheduledTask(workerCtx, task, runner)
		}()
	}

	log.Info().
		Str("instance_id", instanceID).
		Str("queue", queueName).
		Int("prefetch", queueConfig.Prefetch).
		Int("max_attempts", queueConfig.MaxAttempts).
		Int("scheduled_tasks", len(tasks)).
		Strs("message_types", dispatcher.Types()).
		Msg("background worker started")

	var runErr error
	select {
	case <-parentCtx.Done():
		log.Info().Msg("worker shutdown requested")
	case runErr = <-workerErrors:
		log.Error().Err(runErr).Msg("worker component failed")
	case runErr = <-telemetry.Errors():
		log.Error().Err(runErr).Msg("worker observability server failed")
	}
	telemetry.SetDraining()
	cancel()

	drained := make(chan struct{})
	go func() {
		workers.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		log.Info().Msg("worker drained gracefully")
	case <-time.After(shutdownPeriod):
		_ = broker.Close()
		return fmt.Errorf("worker drain exceeded %s", shutdownPeriod)
	}
	return runErr
}

func verifyWorkerDatabasePosture(ctx context.Context, pool database.Querier, production bool) error {
	if pool == nil {
		return fmt.Errorf("verify worker database posture: database executor is required")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := database.ValidateWorkerDatabasePosture(checkCtx, pool, production); err != nil {
		return fmt.Errorf("verify worker database posture: %w", err)
	}
	return nil
}

func shutdownWorkerTelemetry(runtime *observability.Runtime, timeoutSeconds int) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("worker observability shutdown failed")
	}
}

func registerJobHandlers(dispatcher *queuepkg.Dispatcher, components workerComponents, runner taskRunner) error {
	if runner == nil {
		return fmt.Errorf("distributed task runner is required")
	}
	registrations := map[string]queuepkg.EnvelopeHandler{
		"notification.event": func(ctx context.Context, envelope queuepkg.Envelope) error {
			var event service.Event
			if err := decodePayload(envelope.Payload, &event); err != nil {
				return queuepkg.Permanent(err)
			}
			if envelope.TenantID == "" || event.OrgID != envelope.TenantID {
				return queuepkg.Permanent(fmt.Errorf("notification tenant does not match envelope"))
			}
			if strings.TrimSpace(event.Type) == "" {
				return queuepkg.Permanent(fmt.Errorf("notification event type is required"))
			}
			if event.ID != "" && event.ID != envelope.ID {
				return queuepkg.Permanent(fmt.Errorf("notification event ID does not match envelope"))
			}
			event.ID = envelope.ID
			return components.notifications.ProcessEvent(ctx, event)
		},
		"scheduler.notifications.delivery": systemJob("scheduler.notifications.delivery", runner, func(ctx context.Context) error {
			return components.notifications.RunDeliveryCycle(ctx, components.notificationDelivery)
		}),
		"search.index": func(ctx context.Context, envelope queuepkg.Envelope) error {
			var job searchIndexJob
			if err := decodePayload(envelope.Payload, &job); err != nil {
				return queuepkg.Permanent(err)
			}
			action, err := validateSearchIndexJob(envelope, job)
			if err != nil {
				return queuepkg.Permanent(err)
			}
			return components.search.IncrementalIndex(ctx, job.EntityType, job.EntityID, envelope.TenantID, action)
		},
		"scheduler.analytics": systemJob("scheduler.analytics", runner, func(ctx context.Context) error {
			return runAnalytics(ctx, components.analytics)
		}),
		"scheduler.calendar.overdue":   systemJob("scheduler.calendar.overdue", runner, components.calendar.OverdueEscalator),
		"scheduler.calendar.reminders": systemJob("scheduler.calendar.reminders", runner, components.calendar.ReminderScheduler),
		"scheduler.calendar.status":    systemJob("scheduler.calendar.status", runner, components.calendar.StatusUpdater),
		"scheduler.dsr":                systemJob("scheduler.dsr", runner, components.dsr.Run),
		"scheduler.evidence":           systemJob("scheduler.evidence", runner, components.evidence.Run),
		"scheduler.exceptions":         systemJob("scheduler.exceptions", runner, components.exceptions.Run),
		"scheduler.regulatory":         systemJob("scheduler.regulatory", runner, components.regulatory.Run),
		"scheduler.reports":            systemJob("scheduler.reports", runner, components.reports.Run),
		"scheduler.search.health":      systemJob("scheduler.search.health", runner, components.search.HealthCheck),
		"scheduler.search.reindex":     systemJob("scheduler.search.reindex", runner, components.search.NightlyReindex),
		"scheduler.workflows":          systemJob("scheduler.workflows", runner, components.workflows.Run),
	}
	for messageType, handler := range registrations {
		if err := dispatcher.Register(messageType, handler); err != nil {
			return err
		}
	}
	return nil
}

func systemJob(taskName string, runner taskRunner, handler func(context.Context) error) queuepkg.EnvelopeHandler {
	return func(ctx context.Context, envelope queuepkg.Envelope) error {
		if envelope.TenantID != "" {
			return queuepkg.Permanent(fmt.Errorf("system scheduler jobs must not specify a tenant"))
		}
		acquired, err := runner.Run(ctx, taskName, handler)
		if err != nil {
			return err
		}
		if !acquired {
			return fmt.Errorf("task %s is already running on another worker", taskName)
		}
		return nil
	}
}

func validateSearchIndexJob(envelope queuepkg.Envelope, job searchIndexJob) (string, error) {
	if envelope.TenantID == "" || job.EntityType == "" || job.EntityID == "" || job.Action == "" {
		return "", fmt.Errorf("search job requires tenant, entity_type, entity_id, and action")
	}
	if _, err := uuid.Parse(job.EntityID); err != nil {
		return "", fmt.Errorf("search entity_id must be a UUID")
	}
	switch job.EntityType {
	case "risk", "control", "policy", "incident", "finding", "evidence", "asset", "vendor":
	default:
		return "", fmt.Errorf("unsupported search entity_type %q", job.EntityType)
	}
	switch job.Action {
	case "create", "created", "update", "updated", "upsert":
		return "upsert", nil
	case "delete", "deleted":
		return "delete", nil
	default:
		return "", fmt.Errorf("unsupported search action %q", job.Action)
	}
}

func scheduledTasks(components workerComponents) []scheduledTask {
	return []scheduledTask{
		{key: "scheduler.notifications.delivery", name: "notification delivery", interval: components.notificationDelivery.PollInterval, runOnBoot: true, run: func(ctx context.Context) error {
			return components.notifications.RunDeliveryCycle(ctx, components.notificationDelivery)
		}},
		{key: "scheduler.reports", name: "report schedules", interval: time.Minute, runOnBoot: true, run: components.reports.Run},
		{key: "scheduler.workflows", name: "workflow SLAs", interval: 5 * time.Minute, runOnBoot: true, run: components.workflows.Run},
		{key: "scheduler.regulatory", name: "regulatory deadlines", interval: 15 * time.Minute, runOnBoot: true, run: components.regulatory.Run},
		{key: "scheduler.calendar.reminders", name: "calendar reminders", interval: 15 * time.Minute, runOnBoot: true, run: components.calendar.ReminderScheduler},
		{key: "scheduler.calendar.status", name: "calendar status", interval: 30 * time.Minute, runOnBoot: true, run: components.calendar.StatusUpdater},
		{key: "scheduler.calendar.overdue", name: "calendar escalation", interval: time.Hour, runOnBoot: true, run: components.calendar.OverdueEscalator},
		{key: "scheduler.search.reindex", name: "search reindex", interval: time.Hour, runOnBoot: true, run: components.search.NightlyReindex},
		{key: "scheduler.search.health", name: "search health", interval: 6 * time.Hour, runOnBoot: true, run: components.search.HealthCheck},
		{key: "scheduler.dsr", name: "DSR lifecycle", interval: 24 * time.Hour, runOnBoot: true, run: components.dsr.Run},
		{key: "scheduler.evidence", name: "evidence lifecycle", interval: 24 * time.Hour, runOnBoot: true, run: components.evidence.Run},
		{key: "scheduler.exceptions", name: "exception lifecycle", interval: 24 * time.Hour, runOnBoot: true, run: components.exceptions.Run},
		{key: "scheduler.analytics", name: "analytics snapshots", interval: 24 * time.Hour, runOnBoot: true, run: func(ctx context.Context) error {
			return runAnalytics(ctx, components.analytics)
		}},
	}
}

func runScheduledTask(ctx context.Context, task scheduledTask, runner taskRunner) {
	if task.interval <= 0 || task.run == nil || task.key == "" || runner == nil {
		log.Error().Str("task", task.name).Msg("scheduled task has invalid configuration")
		return
	}
	if task.runOnBoot {
		executeScheduledTask(ctx, task, runner)
	}
	ticker := time.NewTicker(task.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			executeScheduledTask(ctx, task, runner)
		}
	}
}

func executeScheduledTask(ctx context.Context, task scheduledTask, runner taskRunner) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Error().Interface("panic", recovered).Str("task", task.name).Msg("scheduled task panicked")
		}
	}()
	startedAt := time.Now()
	acquired, err := runner.Run(ctx, task.key, task.run)
	if err != nil && ctx.Err() == nil {
		log.Error().Err(err).Str("task", task.name).Dur("duration", time.Since(startedAt)).Msg("scheduled task failed")
		return
	}
	if !acquired {
		log.Debug().Str("task", task.name).Msg("scheduled task is owned by another worker")
		return
	}
	log.Debug().Str("task", task.name).Dur("duration", time.Since(startedAt)).Msg("scheduled task completed")
}

func runAnalytics(ctx context.Context, scheduler *workerpkg.AnalyticsScheduler) error {
	return errors.Join(
		scheduler.RunDaily(ctx),
		scheduler.RunWeekly(ctx),
		scheduler.RunMonthly(ctx),
	)
}

func bridgeEvents(ctx context.Context, events <-chan service.Event, outbox eventOutbox, executor database.Querier, queueName string) error {
	if outbox == nil || executor == nil {
		return fmt.Errorf("event outbox and database executor are required")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}
			envelope, err := queuepkg.NewEnvelope("notification.event", event.OrgID, event)
			if err != nil {
				log.Error().Err(err).Str("event_type", event.Type).Msg("could not encode domain event")
				continue
			}
			if event.ID != "" {
				// Scheduler events use a deterministic UUID derived from the
				// entity, source deadline and threshold. Preserve it through the
				// transactional outbox so repeated polling is idempotent.
				envelope.ID = event.ID
				envelope.CorrelationID = event.ID
				if !event.Timestamp.IsZero() {
					envelope.CreatedAt = event.Timestamp.UTC()
				}
			}
			envelope.CausationID = event.EntityID
			envelope.Metadata = map[string]string{"entity_type": event.EntityType, "entity_id": event.EntityID}
			if err := envelope.Validate(); err != nil {
				log.Error().Err(err).Str("event_type", event.Type).Msg("invalid domain event was not queued")
				continue
			}
			if err := outbox.Enqueue(ctx, executor, queueName, envelope); err != nil {
				if event.ID != "" && errors.Is(err, queuepkg.ErrOutboxConflict) {
					// The first event snapshot for a deterministic occurrence wins.
					// Later edits to display/owner metadata must not restart the
					// bridge or resend the same deadline threshold every poll.
					log.Warn().Str("event_type", event.Type).Str("event_id", event.ID).
						Msg("scheduled occurrence already queued; later snapshot suppressed")
					continue
				}
				return err
			}
		}
	}
}

func workerInstanceID() (string, error) {
	value := strings.TrimSpace(os.Getenv("WORKER_INSTANCE_ID"))
	if value == "" {
		return uuid.NewString(), nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", fmt.Errorf("WORKER_INSTANCE_ID must be a UUID: %w", err)
	}
	return parsed.String(), nil
}

func workerSchedulerLease() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv("WORKER_SCHEDULER_LEASE"))
	if value == "" {
		return defaultSchedulerLease, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 3*time.Second {
		return 0, fmt.Errorf("WORKER_SCHEDULER_LEASE must be a duration of at least three seconds")
	}
	return duration, nil
}

func workerEmailSender(config config.SMTPConfig) (emailpkg.Sender, error) {
	return emailpkg.NewSMTPEmailService(emailpkg.Config{
		Host:       config.Host,
		Port:       config.Port,
		Username:   config.User,
		Password:   config.Password,
		From:       config.From,
		TLSMode:    config.TLSMode,
		Timeout:    time.Duration(config.TimeoutSeconds) * time.Second,
		HelloName:  config.HelloName,
		ServerName: config.ServerName,
	})
}

func decodePayload(payload json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode job payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode job payload: multiple JSON values")
		}
		return fmt.Errorf("decode job payload: %w", err)
	}
	return nil
}

func workerShutdownPeriod() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv("WORKER_SHUTDOWN_TIMEOUT"))
	if value == "" {
		return defaultShutdownPeriod, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("WORKER_SHUTDOWN_TIMEOUT must be a positive duration")
	}
	return duration, nil
}
