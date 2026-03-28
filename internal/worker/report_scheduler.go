package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
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

	// Find all active schedules where next_run_at <= NOW()
	rows, err := rs.pool.Query(ctx, `
		SELECT rs.id, rs.organization_id, rs.report_definition_id, rs.frequency,
		       rs.next_run_at, rs.timezone
		FROM report_schedules rs
		WHERE rs.is_active = true
		  AND rs.next_run_at <= NOW()
		ORDER BY rs.next_run_at ASC
		LIMIT 50
	`)
	if err != nil {
		return fmt.Errorf("querying due schedules: %w", err)
	}
	defer rows.Close()

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
			log.Error().Err(err).Msg("report_scheduler: scanning schedule row")
			continue
		}
		schedules = append(schedules, s)
	}

	if len(schedules) == 0 {
		log.Debug().Msg("report_scheduler: no schedules due")
		return nil
	}

	log.Info().Int("count", len(schedules)).Msg("report_scheduler: processing due schedules")

	for _, s := range schedules {
		if err := rs.processSchedule(ctx, s.ID, s.OrgID, s.DefinitionID, s.Frequency, s.Timezone); err != nil {
			log.Error().Err(err).Str("schedule_id", s.ID).Msg("report_scheduler: failed to process schedule")
			continue
		}
	}

	return nil
}

func (rs *ReportScheduler) processSchedule(ctx context.Context, scheduleID, orgID, definitionID, frequency, timezone string) error {
	// Create a report_runs record
	var runID string
	err := rs.pool.QueryRow(ctx, `
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

	// Calculate next_run_at based on frequency
	nextRun := calculateNextRun(frequency, timezone)

	// Update schedule: set last_run_at and next_run_at
	_, err = rs.pool.Exec(ctx, `
		UPDATE report_schedules
		SET last_run_at = NOW(), next_run_at = $2, updated_at = NOW()
		WHERE id = $1
	`, scheduleID, nextRun)
	if err != nil {
		return fmt.Errorf("updating schedule: %w", err)
	}

	return nil
}

// calculateNextRun determines the next run time based on frequency.
func calculateNextRun(frequency, timezone string) time.Time {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)

	switch frequency {
	case "daily":
		return now.Add(24 * time.Hour)
	case "weekly":
		return now.Add(7 * 24 * time.Hour)
	case "monthly":
		return now.AddDate(0, 1, 0)
	case "quarterly":
		return now.AddDate(0, 3, 0)
	case "annually":
		return now.AddDate(1, 0, 0)
	default:
		return now.Add(24 * time.Hour)
	}
}
