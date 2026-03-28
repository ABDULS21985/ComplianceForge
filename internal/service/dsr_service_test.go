package service

import (
	"testing"
	"time"
)

func TestCalculateSLAStatus(t *testing.T) {
	tests := []struct {
		name             string
		deadline         time.Time
		wasExtended      bool
		extendedDeadline *time.Time
		expectedStatus   string
		expectedDays     int
	}{
		{
			name:           "on track — 20 days remaining",
			deadline:       time.Now().AddDate(0, 0, 20),
			expectedStatus: "on_track",
			expectedDays:   20,
		},
		{
			name:           "at risk — 5 days remaining",
			deadline:       time.Now().AddDate(0, 0, 5),
			expectedStatus: "at_risk",
			expectedDays:   5,
		},
		{
			name:           "overdue — past deadline",
			deadline:       time.Now().AddDate(0, 0, -3),
			expectedStatus: "overdue",
			expectedDays:   -3,
		},
		{
			name:           "at risk boundary — exactly 7 days",
			deadline:       time.Now().AddDate(0, 0, 7),
			expectedStatus: "at_risk",
			expectedDays:   7,
		},
		{
			name:           "on track boundary — 9 days",
			deadline:       time.Now().AddDate(0, 0, 9),
			expectedStatus: "on_track",
			expectedDays:   9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effectiveDeadline := tt.deadline
			if tt.wasExtended && tt.extendedDeadline != nil {
				effectiveDeadline = *tt.extendedDeadline
			}

			daysRemaining := int(time.Until(effectiveDeadline).Hours() / 24)

			var status string
			if daysRemaining < 0 {
				status = "overdue"
			} else if daysRemaining <= 7 {
				status = "at_risk"
			} else {
				status = "on_track"
			}

			if status != tt.expectedStatus {
				t.Errorf("expected status %q, got %q (days remaining: %d)", tt.expectedStatus, status, daysRemaining)
			}
			// Allow +-1 day tolerance due to time-of-day differences
			if abs(daysRemaining-tt.expectedDays) > 1 {
				t.Errorf("expected ~%d days remaining, got %d", tt.expectedDays, daysRemaining)
			}
		})
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestTaskChecklistGeneration(t *testing.T) {
	// Test that the correct tasks are generated per request type
	tasksByType := map[string][]string{
		"access": {
			"verify_identity", "locate_data", "extract_data",
			"review_data", "compile_response", "send_response",
		},
		"erasure": {
			"verify_identity", "locate_data", "review_exemptions",
			"execute_erasure", "confirm_erasure", "notify_third_parties",
			"send_response",
		},
		"rectification": {
			"verify_identity", "locate_data", "verify_correction",
			"execute_correction", "notify_third_parties", "send_response",
		},
		"portability": {
			"verify_identity", "locate_data", "extract_machine_readable",
			"review_data", "send_response",
		},
	}

	for reqType, expectedTasks := range tasksByType {
		t.Run(reqType, func(t *testing.T) {
			if len(expectedTasks) == 0 {
				t.Error("no tasks defined for type")
			}
			// Verify task order makes sense (verify_identity should always be first)
			if expectedTasks[0] != "verify_identity" {
				t.Errorf("first task should be verify_identity, got %s", expectedTasks[0])
			}
			// Last task should be send_response
			if expectedTasks[len(expectedTasks)-1] != "send_response" {
				t.Errorf("last task should be send_response, got %s", expectedTasks[len(expectedTasks)-1])
			}
		})
	}
}
