package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
)

// ReportScheduler checks for due report schedules and enqueues generation jobs.
// It runs every minute via the background worker process.
type ReportScheduler struct {
	pool *pgxpool.Pool
}

func NewReportScheduler(pool *pgxpool.Pool) *ReportScheduler {
	return &ReportScheduler{pool: pool}
}

// Run checks for report schedules that are due and enqueues generation jobs.
func (rs *ReportScheduler) Run(ctx context.Context) error {
	log.Info().Msg("report_scheduler: checking for due report schedules")
	return runForScheduledTenants(ctx, rs.pool, "report schedules", rs.runForTenant)
}

func (rs *ReportScheduler) runForTenant(ctx context.Context, organizationID string) error {
	querier := database.QuerierFromContext(ctx, rs.pool)
	// Find all active schedules where next_run_at <= NOW()
	rows, err := querier.Query(ctx, `
		SELECT rs.id, rs.organization_id, rs.report_definition_id, rs.frequency,
		       rs.next_run_at, rs.timezone
		FROM report_schedules rs
		WHERE rs.organization_id = $1::uuid
		  AND rs.is_active = true
		  AND rs.next_run_at <= NOW()
		ORDER BY rs.next_run_at ASC
		LIMIT 50
	`, organizationID)
	if err != nil {
		return fmt.Errorf("querying due schedules: %w", err)
	}
	type dueSchedule struct {
		ID           string
		OrgID        string
		DefinitionID string
		Frequency    string
		NextRunAt    time.Time
		Timezone     string
	}

	var schedules []dueSchedule
	for rows.Next() {
		var s dueSchedule
		if err := rows.Scan(&s.ID, &s.OrgID, &s.DefinitionID, &s.Frequency, &s.NextRunAt, &s.Timezone); err != nil {
			rows.Close()
			return fmt.Errorf("scan due report schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return fmt.Errorf("iterating due report schedules: %w", rowErr)
	}

	if len(schedules) == 0 {
		log.Debug().Msg("report_scheduler: no schedules due")
		return nil
	}

	log.Info().Int("count", len(schedules)).Msg("report_scheduler: processing due schedules")

	var scheduleErrors []error
	for _, s := range schedules {
		if err := rs.processSchedule(ctx, s.ID, s.OrgID, s.NextRunAt); err != nil {
			log.Error().Err(err).Str("schedule_id", s.ID).Msg("report_scheduler: failed to process schedule")
			scheduleErrors = append(scheduleErrors, err)
		}
	}

	return errors.Join(scheduleErrors...)
}

func (rs *ReportScheduler) processSchedule(ctx context.Context, scheduleID, orgID string, expectedRunAt time.Time) error {
	querier := database.QuerierFromContext(ctx, rs.pool)
	tx, err := beginWorkerTransaction(ctx, querier)
	if err != nil {
		return fmt.Errorf("begin report schedule transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	// Lock and re-check the exact due occurrence. This makes claim, run creation,
	// and schedule advancement one atomic operation and prevents another worker
	// from generating the same occurrence after a crash or stale read.
	var definitionID, frequency, timezone, timeOfDay string
	var scheduledAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT report_definition_id, frequency::text, timezone, next_run_at, time_of_day::text
		FROM report_schedules
		WHERE id = $1::uuid
		  AND organization_id = $2::uuid
		  AND is_active
		  AND next_run_at = $3
		  AND next_run_at <= statement_timestamp()
		FOR UPDATE SKIP LOCKED
	`, scheduleID, orgID, expectedRunAt).Scan(&definitionID, &frequency, &timezone, &scheduledAt, &timeOfDay)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("claim due report schedule: %w", err)
	}

	configuredOccurrence, err := reportOccurrenceAtConfiguredTime(scheduledAt, timezone, timeOfDay)
	if err != nil {
		return fmt.Errorf("read configured report time: %w", err)
	}
	nextRun, err := calculateNextRunFrom(configuredOccurrence, frequency, timezone, time.Now())
	if err != nil {
		return fmt.Errorf("calculate next report run: %w", err)
	}

	// Create the run in the same transaction as advancing the claimed schedule.
	var runID string
	err = tx.QueryRow(ctx, `
		INSERT INTO report_runs (organization_id, report_definition_id, schedule_id, status, format, parameters)
		SELECT $1, $2, $3, 'pending', rd.format, rd.filters
		FROM report_definitions rd
		WHERE rd.id = $2 AND rd.organization_id = $1
		RETURNING id
	`, orgID, definitionID, scheduleID).Scan(&runID)
	if err != nil {
		return fmt.Errorf("creating report run: %w", err)
	}

	log.Info().
		Str("schedule_id", scheduleID).
		Str("run_id", runID).
		Str("definition_id", definitionID).
		Msg("report_scheduler: enqueued report generation")

	// Update schedule: set last_run_at and next_run_at
	_, err = tx.Exec(ctx, `
		UPDATE report_schedules
		SET last_run_at = $4, next_run_at = $2, updated_at = NOW()
		WHERE id = $1 AND organization_id = $3::uuid
	`, scheduleID, nextRun, orgID, scheduledAt)
	if err != nil {
		return fmt.Errorf("updating schedule: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit report schedule: %w", err)
	}
	return nil
}

// The persisted claim timestamp is the idempotency boundary, but operators
// may change the configured wall clock before the next claim. Advance from
// that configuration, not the current polling time or a stale UTC offset.
func reportOccurrenceAtConfiguredTime(scheduledAt time.Time, timezone, timeOfDay string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	clock, err := time.Parse("15:04:05", timeOfDay)
	if err != nil {
		return time.Time{}, err
	}
	local := scheduledAt.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), clock.Second(), clock.Nanosecond(), location), nil
}

// calculateNextRunFrom advances a persisted occurrence in calendar units in
// its configured location. AddDate preserves the local wall clock across DST;
// adding 24-hour durations does not. Missed occurrences are coalesced to the
// first future occurrence rather than generating an unbounded catch-up burst.
func calculateNextRunFrom(scheduledAt time.Time, frequency, timezone string, now time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("load timezone %q: %w", timezone, err)
	}
	next := scheduledAt.In(loc)
	advance := func(value time.Time) (time.Time, error) {
		switch frequency {
		case "daily":
			return value.AddDate(0, 0, 1), nil
		case "weekly":
			return value.AddDate(0, 0, 7), nil
		case "monthly":
			return value.AddDate(0, 1, 0), nil
		case "quarterly":
			return value.AddDate(0, 3, 0), nil
		case "annually":
			return value.AddDate(1, 0, 0), nil
		default:
			return time.Time{}, fmt.Errorf("unsupported report schedule frequency %q", frequency)
		}
	}
	for attempts := 0; attempts < 10000; attempts++ {
		next, err = advance(next)
		if err != nil {
			return time.Time{}, err
		}
		if next.After(now.In(loc)) {
			return next, nil
		}
	}
	return time.Time{}, fmt.Errorf("report schedule is more than 10000 occurrences behind")
}
