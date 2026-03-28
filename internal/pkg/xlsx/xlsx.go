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
func (g *ExcelizeGenerator) headerStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
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
	return style
}

// dataStyle returns a standard data cell style.
func (g *ExcelizeGenerator) dataStyle(f *excelize.File, even bool) int {
	bg := "#FFFFFF"
	if even {
		bg = "#F8FAFC"
	}
	style, _ := f.NewStyle(&excelize.Style{
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
	return style
}

// titleStyle returns a large bold title style.
func (g *ExcelizeGenerator) titleStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 16, Color: "#1E3A8A", Family: "Calibri"},
	})
	return style
}

// kpiValueStyle returns a style for KPI values.
func (g *ExcelizeGenerator) kpiValueStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Color: "#1E3A8A", Family: "Calibri"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	return style
}

func (g *ExcelizeGenerator) kpiLabelStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: "#6B7280", Family: "Calibri"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	return style
}

// redStyle for critical/overdue values.
func (g *ExcelizeGenerator) redStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 10, Color: "#DC2626", Family: "Calibri"},
	})
	return style
}

// greenStyle for good values.
func (g *ExcelizeGenerator) greenStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 10, Color: "#16A34A", Family: "Calibri"},
	})
	return style
}

func (g *ExcelizeGenerator) addSummarySheet(f *excelize.File, sheet string, title string, kpis []struct{ Label, Value string }) {
	f.NewSheet(sheet)
	titleSty := g.titleStyle(f)
	kpiVal := g.kpiValueStyle(f)
	kpiLbl := g.kpiLabelStyle(f)

	f.SetCellValue(sheet, "A1", title)
	f.SetCellStyle(sheet, "A1", "A1", titleSty)
	f.MergeCell(sheet, "A1", "D1")

	f.SetCellValue(sheet, "A2", fmt.Sprintf("Generated: %s | %s | %s",
		time.Now().UTC().Format("02 Jan 2006 15:04 UTC"), g.companyName, g.classification))
	f.MergeCell(sheet, "A2", "D2")

	// KPI row
	row := 4
	for i, kpi := range kpis {
		col, _ := excelize.ColumnNumberToName(i + 1)
		cell1 := fmt.Sprintf("%s%d", col, row)
		cell2 := fmt.Sprintf("%s%d", col, row+1)
		f.SetCellValue(sheet, cell1, kpi.Value)
		f.SetCellStyle(sheet, cell1, cell1, kpiVal)
		f.SetCellValue(sheet, cell2, kpi.Label)
		f.SetCellStyle(sheet, cell2, cell2, kpiLbl)
		f.SetColWidth(sheet, col, col, 25)
	}
}

func (g *ExcelizeGenerator) addDataSheet(f *excelize.File, sheet string, headers []string, widths []float64, rows [][]interface{}) {
	f.NewSheet(sheet)
	hdrSty := g.headerStyle(f)
	evenSty := g.dataStyle(f, true)
	oddSty := g.dataStyle(f, false)

	// Set column widths
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, w)
	}

	// Frozen header row
	f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	// Headers
	for i, h := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		cell := fmt.Sprintf("%s1", col)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, hdrSty)
	}

	// Auto filter
	if len(headers) > 0 {
		lastCol, _ := excelize.ColumnNumberToName(len(headers))
		lastRow := len(rows) + 1
		f.AutoFilter(sheet, fmt.Sprintf("A1:%s%d", lastCol, lastRow), nil)
	}

	// Data rows
	for rowIdx, row := range rows {
		excelRow := rowIdx + 2
		sty := evenSty
		if rowIdx%2 != 0 {
			sty = oddSty
		}
		for colIdx, val := range row {
			col, _ := excelize.ColumnNumberToName(colIdx + 1)
			cell := fmt.Sprintf("%s%d", col, excelRow)
			f.SetCellValue(sheet, cell, val)
			f.SetCellStyle(sheet, cell, cell, sty)
		}
	}
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
	g.addSummarySheet(f, "Summary", "Compliance Status Report", []struct{ Label, Value string }{
		{"Overall Score", fmt.Sprintf("%.1f%%", reportData["overall_score"])},
		{"Frameworks", fmt.Sprintf("%v", reportData["frameworks_count"])},
		{"Total Controls", fmt.Sprintf("%v", reportData["total_controls"])},
		{"Overdue Remediations", fmt.Sprintf("%v", reportData["overdue_remediations"])},
	})

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
		g.addDataSheet(f, "Framework Scores", headers, widths, rows)
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
		g.addDataSheet(f, "Gap Analysis", headers, widths, rows)
	}

	// Remove default "Sheet1"
	f.DeleteSheet("Sheet1")

	return g.toBytes(f)
}

func (g *ExcelizeGenerator) GenerateRiskReport(data interface{}) ([]byte, error) {
	reportData, _ := data.(map[string]interface{})
	f := excelize.NewFile()
	defer f.Close()

	g.addSummarySheet(f, "Summary", "Risk Register Report", []struct{ Label, Value string }{
		{"Total Risks", fmt.Sprintf("%v", reportData["total_risks"])},
		{"Critical", fmt.Sprintf("%v", reportData["critical_count"])},
		{"Avg Residual Score", fmt.Sprintf("%.1f", reportData["avg_residual_score"])},
		{"Treatment Rate", fmt.Sprintf("%.0f%%", reportData["treatment_completion_rate"])},
	})

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
		g.addDataSheet(f, "Risk Register", headers, widths, rows)
	}

	f.DeleteSheet("Sheet1")
	return g.toBytes(f)
}

func (g *ExcelizeGenerator) GenerateAuditReport(data interface{}) ([]byte, error) {
	reportData, _ := data.(map[string]interface{})
	f := excelize.NewFile()
	defer f.Close()

	g.addSummarySheet(f, "Summary", "Audit Findings Report", []struct{ Label, Value string }{
		{"Total Findings", fmt.Sprintf("%v", reportData["total_findings"])},
		{"Critical", fmt.Sprintf("%v", reportData["critical_findings"])},
		{"Open", fmt.Sprintf("%v", reportData["open_findings"])},
		{"Resolved", fmt.Sprintf("%v", reportData["resolved_findings"])},
	})

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
		g.addDataSheet(f, "Findings", headers, widths, rows)
	}

	f.DeleteSheet("Sheet1")
	return g.toBytes(f)
}

func (g *ExcelizeGenerator) GenerateCustomReport(data interface{}, sections []string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	g.addSummarySheet(f, "Summary", "Custom Report", []struct{ Label, Value string }{
		{"Sections", fmt.Sprintf("%d", len(sections))},
		{"Generated", time.Now().UTC().Format("02 Jan 2006")},
	})

	f.DeleteSheet("Sheet1")
	return g.toBytes(f)
}
