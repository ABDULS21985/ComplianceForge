package models

import "context"

// GenerateReportRequest is the payload for POST /reports/generate.
type GenerateReportRequest struct {
	ReportType string                 `json:"report_type" validate:"required"`
	Title      string                 `json:"title"`
	Format     string                 `json:"format"` // pdf, xlsx, csv, json
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// ReportRun represents the status of a report generation run.
type ReportRun struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	DefinitionID   string `json:"definition_id,omitempty"`
	ReportType     string `json:"report_type"`
	Title          string `json:"title"`
	Format         string `json:"format"`
	Status         string `json:"status"` // pending, generating, completed, failed
	FileURL        string `json:"file_url,omitempty"`
	Error          string `json:"error,omitempty"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
}

// ReportFile holds the downloadable report content.
type ReportFile struct {
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"-"`
}

// ReportDefinition is a saved, reusable report template.
type ReportDefinition struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	Name           string                 `json:"name" validate:"required"`
	ReportType     string                 `json:"report_type" validate:"required"`
	Format         string                 `json:"format"`
	Parameters     map[string]interface{} `json:"parameters,omitempty"`
	CreatedBy      string                 `json:"created_by"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

// ReportSchedule defines a recurring report generation schedule.
type ReportSchedule struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	DefinitionID   string   `json:"definition_id" validate:"required"`
	CronExpr       string   `json:"cron_expr" validate:"required"`
	Enabled        bool     `json:"enabled"`
	Recipients     []string `json:"recipients,omitempty"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	NextRunAt      string   `json:"next_run_at,omitempty"`
}

// ReportEngine defines the methods required for report generation.
type ReportEngine interface {
	GenerateReport(ctx context.Context, orgID, userID string, req *GenerateReportRequest) (*ReportRun, error)
	GetRunStatus(ctx context.Context, orgID, runID string) (*ReportRun, error)
	DownloadReport(ctx context.Context, orgID, runID string) (*ReportFile, error)

	ListDefinitions(ctx context.Context, orgID string, pagination PaginationRequest) ([]ReportDefinition, int, error)
	CreateDefinition(ctx context.Context, orgID, userID string, def *ReportDefinition) error
	GetDefinition(ctx context.Context, orgID, defID string) (*ReportDefinition, error)
	UpdateDefinition(ctx context.Context, orgID string, def *ReportDefinition) error
	DeleteDefinition(ctx context.Context, orgID, defID string) error
	GenerateFromDefinition(ctx context.Context, orgID, userID, defID string) (*ReportRun, error)

	ListSchedules(ctx context.Context, orgID string, pagination PaginationRequest) ([]ReportSchedule, int, error)
	CreateSchedule(ctx context.Context, orgID, userID string, sched *ReportSchedule) error
	UpdateSchedule(ctx context.Context, orgID string, sched *ReportSchedule) error
	DeleteSchedule(ctx context.Context, orgID, schedID string) error

	ListHistory(ctx context.Context, orgID string, pagination PaginationRequest) ([]ReportRun, int, error)
}
