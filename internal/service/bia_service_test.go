package service

import (
	"testing"
)

func TestSinglePointOfFailureDetection(t *testing.T) {
	// Simulate dependency graph: multiple processes depend on same asset
	type dependency struct {
		processName string
		assetName   string
		isCritical  bool
	}

	deps := []dependency{
		{"Payment Processing", "Database Server", true},
		{"Customer Portal", "Database Server", true},
		{"Reporting", "Database Server", false},
		{"Payment Processing", "Load Balancer", true},
		{"Customer Portal", "Load Balancer", true},
		{"Email Service", "SMTP Server", true},
	}

	// Count how many critical processes depend on each asset
	assetCriticalCount := make(map[string]int)
	for _, d := range deps {
		if d.isCritical {
			assetCriticalCount[d.assetName]++
		}
	}

	// SPoF = asset with 2+ critical process dependencies
	var spofs []string
	for asset, count := range assetCriticalCount {
		if count >= 2 {
			spofs = append(spofs, asset)
		}
	}

	if len(spofs) != 2 {
		t.Errorf("expected 2 SPoFs (Database Server, Load Balancer), got %d: %v", len(spofs), spofs)
	}

	// Verify Database Server is detected
	found := false
	for _, s := range spofs {
		if s == "Database Server" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Database Server should be detected as SPoF")
	}
}

func TestFinancialImpactAggregation(t *testing.T) {
	type process struct {
		name         string
		hourlyImpact float64
		rtoHours     float64
	}

	processes := []process{
		{"Payment Processing", 50000, 4},
		{"Customer Portal", 10000, 8},
		{"Reporting", 2000, 24},
	}

	// Total financial exposure = sum(hourlyImpact × rtoHours) for each process
	var totalExposure float64
	for _, p := range processes {
		totalExposure += p.hourlyImpact * p.rtoHours
	}

	expected := 50000*4 + 10000*8 + 2000*24 // 200,000 + 80,000 + 48,000 = 328,000
	if totalExposure != float64(expected) {
		t.Errorf("expected total exposure €%d, got €%.0f", expected, totalExposure)
	}
}

func TestRTOPrioritization(t *testing.T) {
	// Processes should be recovered in order of RTO (shortest first)
	type process struct {
		name     string
		rtoHours float64
	}

	processes := []process{
		{"Payment Processing", 4},
		{"Email", 24},
		{"Customer Portal", 8},
		{"DNS", 1},
	}

	// Sort by RTO
	for i := 0; i < len(processes)-1; i++ {
		for j := i + 1; j < len(processes); j++ {
			if processes[j].rtoHours < processes[i].rtoHours {
				processes[i], processes[j] = processes[j], processes[i]
			}
		}
	}

	if processes[0].name != "DNS" {
		t.Errorf("DNS (RTO 1h) should be recovered first, got %s", processes[0].name)
	}
	if processes[1].name != "Payment Processing" {
		t.Errorf("Payment Processing (RTO 4h) should be second, got %s", processes[1].name)
	}
}
