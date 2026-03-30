package service

import (
	"testing"
	"time"
)

func TestReminderScheduling(t *testing.T) {
	// Test that reminders fire at the correct offsets
	eventDate := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)
	reminderDays := []int{7, 3, 1, 0}

	tests := []struct {
		today    time.Time
		expected []int // which reminders should fire
	}{
		{time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC), []int{7}},
		{time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC), []int{3}},
		{time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC), []int{1}},
		{time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC), []int{0}},
		{time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC), nil},
	}

	for _, tt := range tests {
		daysUntil := int(eventDate.Sub(tt.today).Hours() / 24)
		var firing []int
		for _, rd := range reminderDays {
			if daysUntil == rd {
				firing = append(firing, rd)
			}
		}
		if len(firing) != len(tt.expected) {
			t.Errorf("on %s (days until=%d): expected reminders %v, got %v",
				tt.today.Format("2006-01-02"), daysUntil, tt.expected, firing)
		}
	}
}

func TestEventStatusTransitions(t *testing.T) {
	tests := []struct {
		name      string
		startDate time.Time
		today     time.Time
		completed bool
		expected  string
	}{
		{"future event", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC), false, "upcoming"},
		{"due today", time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC), false, "due_today"},
		{"overdue", time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC), false, "overdue"},
		{"completed", time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC), true, "completed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := determineEventStatus(tt.startDate, tt.today, tt.completed)
			if status != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, status)
			}
		})
	}
}

func determineEventStatus(startDate, today time.Time, completed bool) string {
	if completed {
		return "completed"
	}
	start := startDate.Truncate(24 * time.Hour)
	now := today.Truncate(24 * time.Hour)
	if start.After(now) {
		return "upcoming"
	}
	if start.Equal(now) {
		return "due_today"
	}
	return "overdue"
}
