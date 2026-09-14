package xlsx

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// XLSXGenerator defines the interface for Excel report generation.
type XLSXGenerator interface {
	GenerateComplianceReport(data interface{}) ([]byte, error)
	GenerateRiskReport(data interface{}) ([]byte, error)
	GenerateAuditReport(data interface{}) ([]byte, error)
	GenerateCustomReport(data interface{}, sections []string) ([]byte, error)
}

// ExcelizeGenerator produces professional .xlsx reports using excelize.
type ExcelizeGenerator struct {
	companyName    string
	classification string
}

func NewExcelizeGenerator(companyName, classification string) *ExcelizeGenerator {
	return &ExcelizeGenerator{companyName: companyName, classification: classification}
}

// --- Internal helpers ---

// headerStyle returns a bold white-on-indigo header style.
func (g *ExcelizeGenerator) headerStyle(f *excelize.File) (int, error) {
	style, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 10, Family: "Calibri"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#1E3A8A"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "#D1D5DB", Style: 1},
			{Type: "right", Color: "#D1D5DB", Style: 1},
			{Type: "top", Color: "#D1D5DB", Style: 1},
			{Type: "bottom", Color: "#D1D5DB", Style: 1},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("create header style: %w", err)
	}
	return style, nil
}

// dataStyle returns a standard data cell style.
func (g *ExcelizeGenerator) dataStyle(f *excelize.File, even bool) (int, error) {
	bg := "#FFFFFF"
	if even {
		bg = "#F8FAFC"
	}
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 10, Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{bg}},
		Border: []excelize.Border{
			{Type: "left", Color: "#E5E7EB", Style: 1},
			{Type: "right", Color: "#E5E7EB", Style: 1},
			{Type: "top", Color: "#E5E7EB", Style: 1},
			{Type: "bottom", Color: "#E5E7EB", Style: 1},
		},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	if err != nil {
		return 0, fmt.Errorf("create data style: %w", err)
	}
	return style, nil
}

// titleStyle returns a large bold title style.
func (g *ExcelizeGenerator) titleStyle(f *excelize.File) (int, error) {
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 16, Color: "#1E3A8A", Family: "Calibri"},
	})
	if err != nil {
		return 0, fmt.Errorf("create title style: %w", err)
	}
	return style, nil
}

// kpiValueStyle returns a style for KPI values.
func (g *ExcelizeGenerator) kpiValueStyle(f *excelize.File) (int, error) {
	style, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Color: "#1E3A8A", Family: "Calibri"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return 0, fmt.Errorf("create KPI value style: %w", err)
	}
	return style, nil
}

func (g *ExcelizeGenerator) kpiLabelStyle(f *excelize.File) (int, error) {
	style, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: "#6B7280", Family: "Calibri"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return 0, fmt.Errorf("create KPI label style: %w", err)
	}
	return style, nil
}

// redStyle for critical/overdue values.
func (g *ExcelizeGenerator) redStyle(f *excelize.File) (int, error) {
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 10, Color: "#DC2626", Family: "Calibri"},
	})
	if err != nil {
		return 0, fmt.Errorf("create critical-value style: %w", err)
	}
	return style, nil
}

// greenStyle for good values.
func (g *ExcelizeGenerator) greenStyle(f *excelize.File) (int, error) {
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 10, Color: "#16A34A", Family: "Calibri"},
	})
	if err != nil {
		return 0, fmt.Errorf("create success-value style: %w", err)
	}
	return style, nil
}

func (g *ExcelizeGenerator) addSummarySheet(f *excelize.File, sheet string, title string, kpis []struct{ Label, Value string }) error {
	if _, err := f.NewSheet(sheet); err != nil {
		return fmt.Errorf("create summary sheet %q: %w", sheet, err)
	}
	titleSty, err := g.titleStyle(f)
	if err != nil {
		return err
	}
	kpiVal, err := g.kpiValueStyle(f)
	if err != nil {
		return err
	}
	kpiLbl, err := g.kpiLabelStyle(f)
	if err != nil {
		return err
	}

	if err := f.SetCellValue(sheet, "A1", title); err != nil {
		return fmt.Errorf("set summary title: %w", err)
	}
	if err := f.SetCellStyle(sheet, "A1", "A1", titleSty); err != nil {
		return fmt.Errorf("style summary title: %w", err)
	}
	if err := f.MergeCell(sheet, "A1", "D1"); err != nil {
		return fmt.Errorf("merge summary title cells: %w", err)
	}

	if err := f.SetCellValue(sheet, "A2", fmt.Sprintf("Generated: %s | %s | %s",
		time.Now().UTC().Format("02 Jan 2006 15:04 UTC"), g.companyName, g.classification)); err != nil {
		return fmt.Errorf("set summary metadata: %w", err)
	}
	if err := f.MergeCell(sheet, "A2", "D2"); err != nil {
		return fmt.Errorf("merge summary metadata cells: %w", err)
	}

	// KPI row
	row := 4
	for i, kpi := range kpis {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return fmt.Errorf("resolve KPI column: %w", err)
		}
		cell1 := fmt.Sprintf("%s%d", col, row)
		cell2 := fmt.Sprintf("%s%d", col, row+1)
		if err := f.SetCellValue(sheet, cell1, kpi.Value); err != nil {
			return fmt.Errorf("set KPI value: %w", err)
		}
		if err := f.SetCellStyle(sheet, cell1, cell1, kpiVal); err != nil {
			return fmt.Errorf("style KPI value: %w", err)
		}
		if err := f.SetCellValue(sheet, cell2, kpi.Label); err != nil {
			return fmt.Errorf("set KPI label: %w", err)
		}
		if err := f.SetCellStyle(sheet, cell2, cell2, kpiLbl); err != nil {
			return fmt.Errorf("style KPI label: %w", err)
		}
		if err := f.SetColWidth(sheet, col, col, 25); err != nil {
			return fmt.Errorf("set KPI column width: %w", err)
		}
	}
	return nil
}

func (g *ExcelizeGenerator) addDataSheet(f *excelize.File, sheet string, headers []string, widths []float64, rows [][]interface{}) error {
	if _, err := f.NewSheet(sheet); err != nil {
		return fmt.Errorf("create data sheet %q: %w", sheet, err)
	}
	hdrSty, err := g.headerStyle(f)
	if err != nil {
		return err
	}
	evenSty, err := g.dataStyle(f, true)
	if err != nil {
		return err
	}
	oddSty, err := g.dataStyle(f, false)
	if err != nil {
		return err
	}

	// Set column widths
	for i, w := range widths {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return fmt.Errorf("resolve data column: %w", err)
		}
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return fmt.Errorf("set data column width: %w", err)
		}
	}

	// Frozen header row
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze data header: %w", err)
	}

	// Headers
	for i, h := range headers {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return fmt.Errorf("resolve header column: %w", err)
		}
		cell := fmt.Sprintf("%s1", col)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return fmt.Errorf("set data header: %w", err)
		}
		if err := f.SetCellStyle(sheet, cell, cell, hdrSty); err != nil {
			return fmt.Errorf("style data header: %w", err)
		}
	}

	// Auto filter
	if len(headers) > 0 {
		lastCol, err := excelize.ColumnNumberToName(len(headers))
		if err != nil {
			return fmt.Errorf("resolve final filter column: %w", err)
		}
		lastRow := len(rows) + 1
		if err := f.AutoFilter(sheet, fmt.Sprintf("A1:%s%d", lastCol, lastRow), nil); err != nil {
			return fmt.Errorf("set data filter: %w", err)
		}
	}

	// Data rows
	for rowIdx, row := range rows {
		excelRow := rowIdx + 2
		sty := evenSty
		if rowIdx%2 != 0 {
			sty = oddSty
		}
		for colIdx, val := range row {
			col, err := excelize.ColumnNumberToName(colIdx + 1)
			if err != nil {
				return fmt.Errorf("resolve value column: %w", err)
			}
			cell := fmt.Sprintf("%s%d", col, excelRow)
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				return fmt.Errorf("set data value: %w", err)
			}
			if err := f.SetCellStyle(sheet, cell, cell, sty); err != nil {
				return fmt.Errorf("style data value: %w", err)
			}
		}
	}
	return nil
}

func (g *ExcelizeGenerator) toBytes(f *excelize.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("writing xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// --- Report generators ---

func (g *ExcelizeGenerator) GenerateComplianceReport(data interface{}) ([]byte, error) {
	reportData, _ := data.(map[string]interface{})
	f := excelize.NewFile()
	defer f.Close()

	// Summary sheet
	if err := g.addSummarySheet(f, "Summary", "Compliance Status Report", []struct{ Label, Value string }{
		{"Overall Score", fmt.Sprintf("%.1f%%", reportData["overall_score"])},
		{"Frameworks", fmt.Sprintf("%v", reportData["frameworks_count"])},
		{"Total Controls", fmt.Sprintf("%v", reportData["total_controls"])},
		{"Overdue Remediations", fmt.Sprintf("%v", reportData["overdue_remediations"])},
	}); err != nil {
		return nil, fmt.Errorf("build compliance summary: %w", err)
	}

	// Framework Scores sheet
	if scores, ok := reportData["framework_scores"].([]interface{}); ok {
		headers := []string{"Framework", "Version", "Score (%)", "Total Controls", "Implemented", "Partial", "Not Implemented", "N/A", "Maturity Avg"}
		widths := []float64{30, 12, 14, 16, 14, 12, 18, 10, 14}
		var rows [][]interface{}
		for _, s := range scores {
			if score, ok := s.(map[string]interface{}); ok {
				rows = append(rows, []interface{}{
					score["framework_name"], score["framework_version"],
					score["compliance_score"], score["total_controls"],
					score["implemented_count"], score["partial_count"],
					score["not_implemented_count"], score["not_applicable_count"],
					score["avg_maturity_level"],
				})
			}
		}
		if err := g.addDataSheet(f, "Framework Scores", headers, widths, rows); err != nil {
			return nil, fmt.Errorf("build framework-scores sheet: %w", err)
		}
	}

	// Gap Analysis sheet
	if gaps, ok := reportData["gaps"].([]interface{}); ok && len(gaps) > 0 {
		headers := []string{"Control Code", "Control Title", "Framework", "Domain", "Status", "Risk If Not Implemented", "Owner", "Remediation Due"}
		widths := []float64{14, 40, 20, 20, 16, 22, 20, 16}
		var rows [][]interface{}
		for _, g := range gaps {
			if gap, ok := g.(map[string]interface{}); ok {
				rows = append(rows, []interface{}{
					gap["control_code"], gap["control_title"], gap["framework_name"],
					gap["domain_name"], gap["status"], gap["risk_if_not_implemented"],
					gap["owner_name"], gap["remediation_due_date"],
				})
			}
		}
		if err := g.addDataSheet(f, "Gap Analysis", headers, widths, rows); err != nil {
			return nil, fmt.Errorf("build gap-analysis sheet: %w", err)
		}
	}

	// Remove default "Sheet1"
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, fmt.Errorf("remove default compliance sheet: %w", err)
	}

	return g.toBytes(f)
}

func (g *ExcelizeGenerator) GenerateRiskReport(data interface{}) ([]byte, error) {
	reportData, _ := data.(map[string]interface{})
	f := excelize.NewFile()
	defer f.Close()

	if err := g.addSummarySheet(f, "Summary", "Risk Register Report", []struct{ Label, Value string }{
		{"Total Risks", fmt.Sprintf("%v", reportData["total_risks"])},
		{"Critical", fmt.Sprintf("%v", reportData["critical_count"])},
		{"Avg Residual Score", fmt.Sprintf("%.1f", reportData["avg_residual_score"])},
		{"Treatment Rate", fmt.Sprintf("%.0f%%", reportData["treatment_completion_rate"])},
	}); err != nil {
		return nil, fmt.Errorf("build risk summary: %w", err)
	}

	if risks, ok := reportData["top_risks"].([]interface{}); ok {
		headers := []string{"Ref", "Title", "Category", "Source", "Inherent Score", "Residual Score", "Residual Level", "Financial Impact (€)", "Status", "Owner"}
		widths := []float64{12, 40, 18, 14, 16, 16, 16, 20, 14, 20}
		var rows [][]interface{}
		for _, r := range risks {
			if risk, ok := r.(map[string]interface{}); ok {
				rows = append(rows, []interface{}{
					risk["risk_ref"], risk["title"], risk["category_name"], risk["risk_source"],
					risk["inherent_risk_score"], risk["residual_risk_score"], risk["residual_risk_level"],
					risk["financial_impact_eur"], risk["status"], risk["owner_name"],
				})
			}
		}
		if err := g.addDataSheet(f, "Risk Register", headers, widths, rows); err != nil {
			return nil, fmt.Errorf("build risk-register sheet: %w", err)
		}
	}

	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, fmt.Errorf("remove default risk sheet: %w", err)
	}
	return g.toBytes(f)
}

func (g *ExcelizeGenerator) GenerateAuditReport(data interface{}) ([]byte, error) {
	reportData, _ := data.(map[string]interface{})
	f := excelize.NewFile()
	defer f.Close()

	if err := g.addSummarySheet(f, "Summary", "Audit Findings Report", []struct{ Label, Value string }{
		{"Total Findings", fmt.Sprintf("%v", reportData["total_findings"])},
		{"Critical", fmt.Sprintf("%v", reportData["critical_findings"])},
		{"Open", fmt.Sprintf("%v", reportData["open_findings"])},
		{"Resolved", fmt.Sprintf("%v", reportData["resolved_findings"])},
	}); err != nil {
		return nil, fmt.Errorf("build audit summary: %w", err)
	}

	if findings, ok := reportData["findings"].([]interface{}); ok {
		headers := []string{"Ref", "Title", "Audit", "Severity", "Status", "Type", "Due Date", "Responsible", "Root Cause"}
		widths := []float64{12, 35, 25, 12, 12, 16, 14, 20, 30}
		var rows [][]interface{}
		for _, f2 := range findings {
			if finding, ok := f2.(map[string]interface{}); ok {
				rows = append(rows, []interface{}{
					finding["finding_ref"], finding["title"], finding["audit_title"],
					finding["severity"], finding["status"], finding["finding_type"],
					finding["due_date"], finding["responsible_name"], finding["root_cause"],
				})
			}
		}
		if err := g.addDataSheet(f, "Findings", headers, widths, rows); err != nil {
			return nil, fmt.Errorf("build findings sheet: %w", err)
		}
	}

	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, fmt.Errorf("remove default audit sheet: %w", err)
	}
	return g.toBytes(f)
}

func (g *ExcelizeGenerator) GenerateCustomReport(data interface{}, sections []string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	if err := g.addSummarySheet(f, "Summary", "Custom Report", []struct{ Label, Value string }{
		{"Sections", fmt.Sprintf("%d", len(sections))},
		{"Generated", time.Now().UTC().Format("02 Jan 2006")},
	}); err != nil {
		return nil, fmt.Errorf("build custom-report summary: %w", err)
	}

	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, fmt.Errorf("remove default custom-report sheet: %w", err)
	}
	return g.toBytes(f)
}
