package worker

import (
	"testing"
	"time"
)

func TestReportOccurrenceUsesConfiguredLocalTime(t *testing.T) {
	claimed := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	configured, err := reportOccurrenceAtConfiguredTime(claimed, "Europe/London", "08:15:00")
	if err != nil || configured.Format("15:04") != "08:15" {
		t.Fatalf("configured occurrence=%s: %v", configured, err)
	}
	if _, err := reportOccurrenceAtConfiguredTime(claimed, "Europe/London", "invalid"); err == nil {
		t.Fatal("invalid configured wall clock accepted")
	}
}

func TestCalculateNextRunFromPreservesWallClockAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	scheduled := time.Date(2026, time.March, 28, 8, 15, 0, 0, location)
	now := scheduled.Add(time.Hour)

	next, err := calculateNextRunFrom(scheduled, "daily", "Europe/London", now)
	if err != nil {
		t.Fatal(err)
	}
	if hour, minute := next.In(location).Hour(), next.In(location).Minute(); hour != 8 || minute != 15 {
		t.Fatalf("next local time = %02d:%02d, want 08:15", hour, minute)
	}
	if elapsed := next.Sub(scheduled); elapsed != 23*time.Hour {
		t.Fatalf("DST transition elapsed time = %s, want 23h", elapsed)
	}
}

func TestCalculateNextRunFromCoalescesMissedOccurrences(t *testing.T) {
	scheduled := time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.January, 5, 9, 0, 0, 0, time.UTC)

	next, err := calculateNextRunFrom(scheduled, "daily", "UTC", now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.January, 6, 8, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestCalculateNextRunFromRejectsInvalidConfiguration(t *testing.T) {
	scheduled := time.Now().UTC()
	if _, err := calculateNextRunFrom(scheduled, "daily", "Not/AZone", scheduled); err == nil {
		t.Fatal("invalid timezone was accepted")
	}
	if _, err := calculateNextRunFrom(scheduled, "hourly", "UTC", scheduled); err == nil {
		t.Fatal("invalid frequency was accepted")
	}
}
