package service

import (
	"testing"
)

func TestExceptionComplianceImpactCalculation(t *testing.T) {
	// Test that exceptions affect compliance scores correctly
	tests := []struct {
		name                string
		compensatingEffect  string
		expectedScoreCredit float64
	}{
		{"full compensating controls", "full", 0.50},      // 50% credit
		{"partial compensating controls", "partial", 0.25}, // 25% credit
		{"minimal compensating controls", "minimal", 0.10}, // 10% credit
		{"no compensating controls", "none", 0.00},         // 0% credit
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credit := calculateExceptionCredit(tt.compensatingEffect)
			if credit != tt.expectedScoreCredit {
				t.Errorf("expected %.2f credit for %s, got %.2f", tt.expectedScoreCredit, tt.compensatingEffect, credit)
			}
		})
	}
}

func calculateExceptionCredit(compensatingEffectiveness string) float64 {
	switch compensatingEffectiveness {
	case "full":
		return 0.50
	case "partial":
		return 0.25
	case "minimal":
		return 0.10
	default:
		return 0.00
	}
}

func TestExceptionStatusTransitions(t *testing.T) {
	validTransitions := map[string][]string{
		"draft":                   {"pending_risk_assessment"},
		"pending_risk_assessment": {"pending_approval", "rejected"},
		"pending_approval":        {"approved", "rejected"},
		"approved":                {"expired", "revoked", "renewal_pending"},
		"rejected":                {"draft"},
		"expired":                 {"renewal_pending"},
		"revoked":                 {},
		"renewal_pending":         {"pending_risk_assessment", "rejected"},
	}

	// Valid transitions
	for from, targets := range validTransitions {
		for _, to := range targets {
			if !isValidExceptionTransition(from, to, validTransitions) {
				t.Errorf("transition %s → %s should be valid", from, to)
			}
		}
	}

	// Invalid transitions
	invalidCases := [][2]string{
		{"draft", "approved"},
		{"approved", "draft"},
		{"revoked", "approved"},
		{"expired", "approved"},
		{"pending_approval", "draft"},
	}
	for _, c := range invalidCases {
		if isValidExceptionTransition(c[0], c[1], validTransitions) {
			t.Errorf("transition %s → %s should be INVALID", c[0], c[1])
		}
	}
}

func isValidExceptionTransition(from, to string, transitions map[string][]string) bool {
	for _, valid := range transitions[from] {
		if valid == to {
			return true
		}
	}
	return false
}

func TestExceptionRenewalLimits(t *testing.T) {
	// Temporary exceptions can be renewed max 2 times
	tests := []struct {
		name          string
		exceptionType string
		renewalCount  int
		canRenew      bool
	}{
		{"temporary first renewal", "temporary", 0, true},
		{"temporary second renewal", "temporary", 1, true},
		{"temporary max reached", "temporary", 2, false},
		{"permanent always renewable", "permanent", 5, true},
		{"conditional first renewal", "conditional", 0, true},
		{"conditional max reached", "conditional", 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canRenew := canRenewException(tt.exceptionType, tt.renewalCount)
			if canRenew != tt.canRenew {
				t.Errorf("expected canRenew=%v for %s with %d renewals, got %v",
					tt.canRenew, tt.exceptionType, tt.renewalCount, canRenew)
			}
		})
	}
}

func canRenewException(exceptionType string, renewalCount int) bool {
	if exceptionType == "permanent" {
		return true // permanent exceptions always get annual review/renewal
	}
	return renewalCount < 2 // temporary and conditional: max 2 renewals
}

func TestExceptionExpiryDetermination(t *testing.T) {
	tests := []struct {
		name      string
		daysUntil int
		expected  string
	}{
		{"expired", -5, "expired"},
		{"expires today", 0, "expired"},
		{"expires in 7 days", 7, "expiring_soon"},
		{"expires in 14 days", 14, "expiring_soon"},
		{"expires in 30 days", 30, "expiring_soon"},
		{"expires in 31 days", 31, "active"},
		{"expires in 90 days", 90, "active"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := classifyExpiryStatus(tt.daysUntil)
			if status != tt.expected {
				t.Errorf("expected %s for %d days, got %s", tt.expected, tt.daysUntil, status)
			}
		})
	}
}

func classifyExpiryStatus(daysUntilExpiry int) string {
	if daysUntilExpiry <= 0 {
		return "expired"
	}
	if daysUntilExpiry <= 30 {
		return "expiring_soon"
	}
	return "active"
}
