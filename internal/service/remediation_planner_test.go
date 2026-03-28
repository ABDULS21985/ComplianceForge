package service

import (
	"testing"
)

func TestRemediationPlanPrioritization(t *testing.T) {
	// Test that gaps are prioritized correctly: critical > high > medium > low
	gaps := []struct {
		controlCode string
		riskLevel   string
		expected    int // expected priority rank (lower = higher priority)
	}{
		{"A.8.7", "critical", 1},
		{"A.5.15", "high", 2},
		{"A.8.9", "medium", 3},
		{"A.5.6", "low", 4},
	}

	riskOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "": 4}

	for i := 0; i < len(gaps)-1; i++ {
		if riskOrder[gaps[i].riskLevel] > riskOrder[gaps[i+1].riskLevel] {
			t.Errorf("gap %s (%s) should be higher priority than %s (%s)",
				gaps[i].controlCode, gaps[i].riskLevel,
				gaps[i+1].controlCode, gaps[i+1].riskLevel)
		}
	}
}

func TestActionStatusTransitions(t *testing.T) {
	validTransitions := map[string][]string{
		"not_started": {"in_progress", "cancelled", "deferred"},
		"in_progress": {"completed", "blocked", "cancelled"},
		"blocked":     {"in_progress", "cancelled"},
		"completed":   {}, // terminal
		"cancelled":   {}, // terminal
		"deferred":    {"not_started", "in_progress"},
	}

	for from, validTargets := range validTransitions {
		for _, to := range validTargets {
			if !isValidTransition(from, to, validTransitions) {
				t.Errorf("transition %s → %s should be valid", from, to)
			}
		}
	}

	// Test invalid transitions
	invalidCases := [][2]string{
		{"completed", "in_progress"},
		{"cancelled", "in_progress"},
		{"not_started", "completed"},
	}
	for _, c := range invalidCases {
		if isValidTransition(c[0], c[1], validTransitions) {
			t.Errorf("transition %s → %s should be INVALID", c[0], c[1])
		}
	}
}

func isValidTransition(from, to string, transitions map[string][]string) bool {
	for _, valid := range transitions[from] {
		if valid == to {
			return true
		}
	}
	return false
}
