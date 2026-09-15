package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/complianceforge/platform/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCalendarReadRepositoryNeverFallsBackToUnscopedPool(t *testing.T) {
	repo := &calendarReadRepo{pool: &pgxpool.Pool{}}
	if _, err := repo.LoadCandidates(context.Background(), "tenant", "subject", models.CalendarEventQuery{}); !errors.Is(err, ErrCalendarReadScope) {
		t.Fatalf("unscoped pool fallback accepted: %v", err)
	}
	if _, err := NewCalendarReadRepository(nil); err == nil {
		t.Fatal("nil database accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repo.LoadCandidates(ctx, "tenant", "subject", models.CalendarEventQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}
