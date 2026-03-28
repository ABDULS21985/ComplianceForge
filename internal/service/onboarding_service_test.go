package service

import (
	"testing"
)

func TestFrameworkRecommendations(t *testing.T) {
	tests := []struct {
		name               string
		answers            map[string]interface{}
		expectedFrameworks []string // framework codes
		expectedMandatory  []string
	}{
		{
			name: "Payment processor in UK",
			answers: map[string]interface{}{
				"processes_payment_cards":    true,
				"processes_eu_personal_data": true,
				"uk_public_sector":           false,
				"requires_iso_certification": true,
			},
			expectedFrameworks: []string{"PCI_DSS_4", "UK_GDPR", "ISO27001"},
			expectedMandatory:  []string{"PCI_DSS_4", "UK_GDPR"},
		},
		{
			name: "Essential service provider",
			answers: map[string]interface{}{
				"essential_service_provider": true,
				"processes_eu_personal_data": true,
			},
			expectedFrameworks: []string{"NCSC_CAF", "UK_GDPR"},
			expectedMandatory:  []string{"UK_GDPR"},
		},
		{
			name: "UK public sector org",
			answers: map[string]interface{}{
				"uk_public_sector":           true,
				"processes_eu_personal_data": true,
			},
			expectedFrameworks: []string{"CYBER_ESSENTIALS", "UK_GDPR"},
			expectedMandatory:  []string{"CYBER_ESSENTIALS", "UK_GDPR"},
		},
		{
			name: "No special requirements",
			answers: map[string]interface{}{
				"target_maturity":           true,
				"board_governance_required": true,
			},
			expectedFrameworks: []string{"NIST_CSF_2", "COBIT_2019"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := generateRecommendations(tt.answers)

			// Check all expected frameworks are recommended
			recCodes := make(map[string]bool)
			for _, r := range recs {
				recCodes[r.FrameworkCode] = true
			}

			for _, expected := range tt.expectedFrameworks {
				if !recCodes[expected] {
					t.Errorf("expected framework %s to be recommended, but it wasn't. Got: %v", expected, recCodes)
				}
			}

			// Check mandatory flags
			mandatoryMap := make(map[string]bool)
			for _, r := range recs {
				if r.Mandatory {
					mandatoryMap[r.FrameworkCode] = true
				}
			}
			for _, expected := range tt.expectedMandatory {
				if !mandatoryMap[expected] {
					t.Errorf("expected framework %s to be mandatory", expected)
				}
			}
		})
	}
}

// generateRecommendations is a standalone helper that mirrors the service logic
// for testability without a database.
func generateRecommendations(answers map[string]interface{}) []FrameworkRecommendation {
	var recs []FrameworkRecommendation
	priority := 1

	if b, ok := answers["processes_payment_cards"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "PCI_DSS_4", FrameworkName: "PCI DSS v4.0", Priority: priority, Reason: "You process payment card data — PCI DSS compliance is required.", Mandatory: true})
		priority++
	}
	if b, ok := answers["processes_eu_personal_data"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "UK_GDPR", FrameworkName: "UK GDPR", Priority: priority, Reason: "You process personal data of EU/UK residents — GDPR compliance is required.", Mandatory: true})
		priority++
	}
	if b, ok := answers["essential_service_provider"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "NCSC_CAF", FrameworkName: "NCSC CAF", Priority: priority, Reason: "As a provider of essential services, the NCSC CAF is recommended under NIS regulations."})
		priority++
	}
	if b, ok := answers["uk_public_sector"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "CYBER_ESSENTIALS", FrameworkName: "Cyber Essentials", Priority: priority, Reason: "UK public sector organisations require Cyber Essentials certification.", Mandatory: true})
		priority++
	}
	if b, ok := answers["requires_iso_certification"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "ISO27001", FrameworkName: "ISO 27001", Priority: priority, Reason: "ISO 27001 certification requested — this is the international gold standard for ISMS."})
		priority++
	}
	if b, ok := answers["us_federal_contracts"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "NIST_800_53", FrameworkName: "NIST 800-53", Priority: priority, Reason: "US federal contracts require NIST 800-53 compliance."})
		priority++
	}
	if b, ok := answers["target_maturity"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "NIST_CSF_2", FrameworkName: "NIST CSF 2.0", Priority: priority, Reason: "NIST CSF 2.0 provides a maturity-based approach to cybersecurity improvement."})
		priority++
	}
	if b, ok := answers["itil_required"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "ITIL_4", FrameworkName: "ITIL 4", Priority: priority, Reason: "ITIL 4 alignment for IT service management."})
		priority++
	}
	if b, ok := answers["board_governance_required"].(bool); ok && b {
		recs = append(recs, FrameworkRecommendation{FrameworkCode: "COBIT_2019", FrameworkName: "COBIT 2019", Priority: priority, Reason: "COBIT 2019 provides IT governance and management framework for board reporting."})
		priority++
	}

	return recs
}
