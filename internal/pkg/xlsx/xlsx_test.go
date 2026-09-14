package xlsx

import (
	"bytes"
	"slices"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestReportGeneratorsProduceReadableWorkbooks(t *testing.T) {
	t.Parallel()
	generator := NewExcelizeGenerator("ComplianceForge", "confidential")
	tests := []struct {
		name           string
		generate       func() ([]byte, error)
		expectedSheets []string
	}{
		{
			name: "compliance",
			generate: func() ([]byte, error) {
				return generator.GenerateComplianceReport(map[string]interface{}{
					"overall_score": 92.5, "frameworks_count": 2, "total_controls": 20,
					"overdue_remediations": 1,
					"framework_scores":     []interface{}{map[string]interface{}{"framework_name": "ISO 27001"}},
					"gaps":                 []interface{}{map[string]interface{}{"control_code": "A.5.1"}},
				})
			},
			expectedSheets: []string{"Summary", "Framework Scores", "Gap Analysis"},
		},
		{
			name: "risk",
			generate: func() ([]byte, error) {
				return generator.GenerateRiskReport(map[string]interface{}{
					"total_risks": 3, "critical_count": 1, "avg_residual_score": 4.5,
					"treatment_completion_rate": 50.0,
					"top_risks":                 []interface{}{map[string]interface{}{"risk_ref": "R-1"}},
				})
			},
			expectedSheets: []string{"Summary", "Risk Register"},
		},
		{
			name: "audit",
			generate: func() ([]byte, error) {
				return generator.GenerateAuditReport(map[string]interface{}{
					"total_findings": 2, "critical_findings": 1, "open_findings": 1, "resolved_findings": 1,
					"findings": []interface{}{map[string]interface{}{"finding_ref": "F-1"}},
				})
			},
			expectedSheets: []string{"Summary", "Findings"},
		},
		{
			name: "custom",
			generate: func() ([]byte, error) {
				return generator.GenerateCustomReport(nil, []string{"risks", "controls"})
			},
			expectedSheets: []string{"Summary"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			payload, err := test.generate()
			if err != nil {
				t.Fatalf("generate workbook: %v", err)
			}
			workbook, err := excelize.OpenReader(bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("open generated workbook: %v", err)
			}
			defer func() {
				if err := workbook.Close(); err != nil {
					t.Errorf("close generated workbook: %v", err)
				}
			}()

			sheets := workbook.GetSheetList()
			if slices.Contains(sheets, "Sheet1") {
				t.Fatalf("default sheet was not removed: %v", sheets)
			}
			for _, expected := range test.expectedSheets {
				if !slices.Contains(sheets, expected) {
					t.Errorf("missing sheet %q in %v", expected, sheets)
				}
			}
		})
	}
}
