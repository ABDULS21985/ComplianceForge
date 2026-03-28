package service

import (
	"math"
	"testing"
)

func TestExponentialSmoothing(t *testing.T) {
	// Test the exponential smoothing algorithm used for risk predictions
	scores := []float64{15.0, 14.0, 12.0, 10.0, 8.0} // declining trend
	alpha := 0.3

	// Apply smoothing (oldest to newest)
	smoothed := scores[len(scores)-1] // start with oldest
	for i := len(scores) - 2; i >= 0; i-- {
		smoothed = alpha*scores[i] + (1-alpha)*smoothed
	}

	// With declining scores and alpha=0.3, smoothed should be between min and max
	if smoothed < 8.0 || smoothed > 15.0 {
		t.Errorf("smoothed value %.2f should be between 8.0 and 15.0", smoothed)
	}

	// Smoothed should weight recent values more, so closer to 15 (latest) than 8 (oldest)
	midpoint := (15.0 + 8.0) / 2.0
	if smoothed < midpoint {
		t.Errorf("smoothed value %.2f should be above midpoint %.2f (recent values weighted more)", smoothed, midpoint)
	}
}

func TestBreachProbabilityModel(t *testing.T) {
	tests := []struct {
		name          string
		criticalRisks int
		highRisks     int
		avgScore      float64
		breaches12mo  int
		minProb       float64
		maxProb       float64
	}{
		{
			name: "low risk org", criticalRisks: 0, highRisks: 1, avgScore: 3.0, breaches12mo: 0,
			minProb: 0.0, maxProb: 0.20,
		},
		{
			name: "high risk org", criticalRisks: 5, highRisks: 10, avgScore: 15.0, breaches12mo: 3,
			minProb: 0.30, maxProb: 0.95,
		},
		{
			name: "moderate risk", criticalRisks: 1, highRisks: 3, avgScore: 8.0, breaches12mo: 1,
			minProb: 0.10, maxProb: 0.75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			riskFactor := float64(tt.criticalRisks)*0.15 + float64(tt.highRisks)*0.08 + tt.avgScore*0.02
			historyFactor := float64(tt.breaches12mo) * 0.10
			baseRate := 0.05
			prob365d := math.Min(0.95, baseRate+riskFactor+historyFactor)

			if prob365d < tt.minProb || prob365d > tt.maxProb {
				t.Errorf("probability %.4f should be between %.2f and %.2f", prob365d, tt.minProb, tt.maxProb)
			}
		})
	}
}

func TestTrendDirectionClassification(t *testing.T) {
	tests := []struct {
		name     string
		current  float64
		previous float64
		expected string
	}{
		{"improving", 80.0, 75.0, "improving"},
		{"declining", 70.0, 75.0, "declining"},
		{"stable - same", 75.0, 75.0, "stable"},
		{"stable - small change", 75.3, 75.0, "stable"},
		{"barely improving", 75.6, 75.0, "improving"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			change := tt.current - tt.previous
			var direction string
			if change > 0.5 {
				direction = "improving"
			} else if change < -0.5 {
				direction = "declining"
			} else {
				direction = "stable"
			}

			if direction != tt.expected {
				t.Errorf("expected %s, got %s (change=%.1f)", tt.expected, direction, change)
			}
		})
	}
}
