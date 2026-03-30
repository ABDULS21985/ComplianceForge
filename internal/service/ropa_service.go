package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrProcessingActivityNotFound = fmt.Errorf("processing activity not found")
	ErrClassificationNotFound     = fmt.Errorf("data classification not found")
	ErrDataCategoryNotFound       = fmt.Errorf("data category not found")
	ErrDataFlowNotFound           = fmt.Errorf("data flow not found")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// DataClassification represents a data sensitivity classification level.
type DataClassification struct {
	ID          string `json:"id"`
	OrgID       string `json:"organization_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Level       int    `json:"level"` // 1=public, 2=internal, 3=confidential, 4=restricted, 5=highly_restricted
	Color       string `json:"color"`
	CreatedAt   string `json:"created_at"`
}

// DataCategory represents a GDPR data category (e.g., personal, special category).
type DataCategory struct {
	ID                string `json:"id"`
	OrgID             string `json:"organization_id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	CategoryType      string `json:"category_type"` // personal, special_category, non_personal
	IsSpecialCategory bool   `json:"is_special_category"`
	GDPRArticle       string `json:"gdpr_article"` // e.g., Art. 9(1)
	CreatedAt         string `json:"created_at"`
}

// ProcessingActivity represents a GDPR Article 30 processing activity record.
type ProcessingActivity struct {
	ID                   string                 `json:"id"`
	OrgID                string                 `json:"organization_id"`
	ActivityRef          string                 `json:"activity_ref"`
	Name                 string                 `json:"name"`
	Description          string                 `json:"description"`
	Purpose              string                 `json:"purpose"`
	LegalBasis           string                 `json:"legal_basis"` // consent, contract, legal_obligation, vital_interest, public_interest, legitimate_interest
	LegalBasisDetail     string                 `json:"legal_basis_detail"`
	DataControllerName   string                 `json:"data_controller_name"`
	DataControllerContact string                `json:"data_controller_contact"`
	DPOContact           string                 `json:"dpo_contact"`
	JointControllers     []string               `json:"joint_controllers"`
	DataSubjectCategories []string              `json:"data_subject_categories"` // employees, customers, prospects, minors, patients
	DataCategories       []string               `json:"data_categories"`
	SpecialCategories    []string               `json:"special_categories"`
	Recipients           []string               `json:"recipients"`
	ThirdCountryTransfers []ThirdCountryTransfer `json:"third_country_transfers"`
	RetentionPeriod      string                 `json:"retention_period"`
	RetentionJustification string              `json:"retention_justification"`
	SecurityMeasures     []string               `json:"security_measures"`
	DPIARequired         bool                   `json:"dpia_required"`
	DPIAStatus           string                 `json:"dpia_status"` // not_required, pending, in_progress, completed
	DPIADate             *string                `json:"dpia_date"`
	AutomatedDecision    bool                   `json:"automated_decision_making"`
	ProfilingUsed        bool                   `json:"profiling_used"`
	Status               string                 `json:"status"` // draft, active, under_review, retired
	Owner                *string                `json:"owner"`
	Department           string                 `json:"department"`
	NextReviewDate       *string                `json:"next_review_date"`
	Metadata             map[string]interface{} `json:"metadata"`
	CreatedAt            string                 `json:"created_at"`
	UpdatedAt            string                 `json:"updated_at"`
}

// ThirdCountryTransfer represents a data transfer outside the EEA.
type ThirdCountryTransfer struct {
	Country       string `json:"country"`
	Recipient     string `json:"recipient"`
	Safeguard     string `json:"safeguard"` // adequacy_decision, sccs, bcrs, derogation, none
	SafeguardRef  string `json:"safeguard_ref"`
}

// DataFlow represents data movement for a processing activity.
type DataFlow struct {
	ID           string `json:"id"`
	OrgID        string `json:"organization_id"`
	ActivityID   string `json:"activity_id"`
	SourceSystem string `json:"source_system"`
	SourceType   string `json:"source_type"` // internal, external, third_party
	DestSystem   string `json:"destination_system"`
	DestType     string `json:"destination_type"`
	DataElements []string `json:"data_elements"`
	TransferMethod string `json:"transfer_method"` // api, file_transfer, manual, streaming, email
	Encrypted    bool   `json:"encrypted"`
	Frequency    string `json:"frequency"`
	Volume       string `json:"volume"`
	CreatedAt    string `json:"created_at"`
}

// CreateProcessingActivityRequest holds input for creating an activity.
type CreateProcessingActivityRequest struct {
	Name                  string                 `json:"name"`
	Description           string                 `json:"description"`
	Purpose               string                 `json:"purpose"`
	LegalBasis            string                 `json:"legal_basis"`
	LegalBasisDetail      string                 `json:"legal_basis_detail"`
	DataControllerName    string                 `json:"data_controller_name"`
	DataControllerContact string                 `json:"data_controller_contact"`
	DPOContact            string                 `json:"dpo_contact"`
	JointControllers      []string               `json:"joint_controllers"`
	DataSubjectCategories []string               `json:"data_subject_categories"`
	DataCategories        []string               `json:"data_categories"`
	SpecialCategories     []string               `json:"special_categories"`
	Recipients            []string               `json:"recipients"`
	ThirdCountryTransfers []ThirdCountryTransfer `json:"third_country_transfers"`
	RetentionPeriod       string                 `json:"retention_period"`
	RetentionJustification string               `json:"retention_justification"`
	SecurityMeasures      []string               `json:"security_measures"`
	AutomatedDecision     bool                   `json:"automated_decision_making"`
	ProfilingUsed         bool                   `json:"profiling_used"`
	Owner                 *string                `json:"owner"`
	Department            string                 `json:"department"`
	Metadata              map[string]interface{} `json:"metadata"`
}

// UpdateProcessingActivityRequest holds partial update fields.
type UpdateProcessingActivityRequest struct {
	Name                  *string                 `json:"name"`
	Description           *string                 `json:"description"`
	Purpose               *string                 `json:"purpose"`
	LegalBasis            *string                 `json:"legal_basis"`
	Status                *string                 `json:"status"`
	RetentionPeriod       *string                 `json:"retention_period"`
	SecurityMeasures      []string                `json:"security_measures"`
	Owner                 *string                 `json:"owner"`
	Department            *string                 `json:"department"`
	NextReviewDate        *string                 `json:"next_review_date"`
}

// ROPADocument represents a generated ROPA export.
type ROPADocument struct {
	GeneratedAt  string               `json:"generated_at"`
	Organization string               `json:"organization"`
	Format       string               `json:"format"` // json, csv, pdf
	Activities   []ProcessingActivity `json:"activities"`
	TotalCount   int                  `json:"total_count"`
}

// ROPADashboard provides aggregate ROPA statistics.
type ROPADashboard struct {
	TotalActivities       int            `json:"total_activities"`
	ByLegalBasis          map[string]int `json:"by_legal_basis"`
	SpecialCategoryCount  int            `json:"special_category_count"`
	TransferCount         int            `json:"third_country_transfers"`
	DPIAPending           int            `json:"dpia_pending"`
	DPIACompleted         int            `json:"dpia_completed"`
	OverdueReviews        int            `json:"overdue_reviews"`
	ByStatus              map[string]int `json:"by_status"`
	ByDepartment          map[string]int `json:"by_department"`
}

// HighRiskIndicator flags a processing activity with DPIA triggers.
type HighRiskIndicator struct {
	ActivityID   string   `json:"activity_id"`
	ActivityRef  string   `json:"activity_ref"`
	Name         string   `json:"name"`
	RiskTriggers []string `json:"risk_triggers"`
	DPIARequired bool     `json:"dpia_required"`
}

// DataSubjectImpactEntry describes what data is collected for a subject category.
type DataSubjectImpactEntry struct {
	ActivityRef   string   `json:"activity_ref"`
	ActivityName  string   `json:"activity_name"`
	Purpose       string   `json:"purpose"`
	DataElements  []string `json:"data_elements"`
	LegalBasis    string   `json:"legal_basis"`
	Retention     string   `json:"retention_period"`
	Recipients    []string `json:"recipients"`
	Transfers     []string `json:"third_country_transfers"`
}

// CreateDataFlowRequest holds input for mapping a data flow.
type CreateDataFlowRequest struct {
	SourceSystem   string   `json:"source_system"`
	SourceType     string   `json:"source_type"`
	DestSystem     string   `json:"destination_system"`
	DestType       string   `json:"destination_type"`
	DataElements   []string `json:"data_elements"`
	TransferMethod string   `json:"transfer_method"`
	Encrypted      bool     `json:"encrypted"`
	Frequency      string   `json:"frequency"`
	Volume         string   `json:"volume"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// ROPAService manages GDPR Records of Processing Activities.
type ROPAService struct {
	pool *pgxpool.Pool
}

// NewROPAService creates a new ROPAService.
func NewROPAService(pool *pgxpool.Pool) *ROPAService {
	return &ROPAService{pool: pool}
}

// ---------------------------------------------------------------------------
// Classification CRUD
// ---------------------------------------------------------------------------

// CreateClassification creates a new data classification.
func (s *ROPAService) CreateClassification(ctx context.Context, orgID string, c DataClassification) (*DataClassification, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO data_classifications (organization_id, name, description, level, color)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, name, description, level, color, created_at`,
		orgID, c.Name, c.Description, c.Level, c.Color,
	).Scan(&c.ID, &c.OrgID, &c.Name, &c.Description, &c.Level, &c.Color, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create classification: %w", err)
	}
	log.Info().Str("classification_id", c.ID).Str("name", c.Name).Msg("classification created")
	return &c, nil
}

// UpdateClassification updates an existing classification.
func (s *ROPAService) UpdateClassification(ctx context.Context, orgID, id string, c DataClassification) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE data_classifications SET name = $1, description = $2, level = $3, color = $4, updated_at = NOW()
		WHERE id = $5 AND organization_id = $6`,
		c.Name, c.Description, c.Level, c.Color, id, orgID)
	if err != nil {
		return fmt.Errorf("update classification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrClassificationNotFound
	}
	return nil
}

// DeleteClassification removes a classification.
func (s *ROPAService) DeleteClassification(ctx context.Context, orgID, id string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM data_classifications WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("delete classification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrClassificationNotFound
	}
	return nil
}

// ListClassifications returns all classifications for an organization.
func (s *ROPAService) ListClassifications(ctx context.Context, orgID string) ([]DataClassification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, description, level, color, created_at
		FROM data_classifications WHERE organization_id = $1
		ORDER BY level`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list classifications: %w", err)
	}
	defer rows.Close()

	var result []DataClassification
	for rows.Next() {
		var c DataClassification
		if err := rows.Scan(&c.ID, &c.OrgID, &c.Name, &c.Description, &c.Level, &c.Color, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan classification: %w", err)
		}
		result = append(result, c)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Data Category CRUD
// ---------------------------------------------------------------------------

// CreateDataCategory creates a new data category.
func (s *ROPAService) CreateDataCategory(ctx context.Context, orgID string, c DataCategory) (*DataCategory, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO data_categories (organization_id, name, description, category_type, is_special_category, gdpr_article)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, organization_id, name, description, category_type, is_special_category, gdpr_article, created_at`,
		orgID, c.Name, c.Description, c.CategoryType, c.IsSpecialCategory, c.GDPRArticle,
	).Scan(&c.ID, &c.OrgID, &c.Name, &c.Description, &c.CategoryType, &c.IsSpecialCategory, &c.GDPRArticle, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create data category: %w", err)
	}
	log.Info().Str("category_id", c.ID).Str("name", c.Name).Msg("data category created")
	return &c, nil
}

// UpdateDataCategory updates an existing data category.
func (s *ROPAService) UpdateDataCategory(ctx context.Context, orgID, id string, c DataCategory) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE data_categories
		SET name = $1, description = $2, category_type = $3, is_special_category = $4, gdpr_article = $5, updated_at = NOW()
		WHERE id = $6 AND organization_id = $7`,
		c.Name, c.Description, c.CategoryType, c.IsSpecialCategory, c.GDPRArticle, id, orgID)
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDataCategoryNotFound
	}
	return nil
}

// DeleteDataCategory removes a data category.
func (s *ROPAService) DeleteDataCategory(ctx context.Context, orgID, id string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM data_categories WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDataCategoryNotFound
	}
	return nil
}

// ListCategories returns all data categories for an organization.
func (s *ROPAService) ListCategories(ctx context.Context, orgID string) ([]DataCategory, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, description, category_type, is_special_category, gdpr_article, created_at
		FROM data_categories WHERE organization_id = $1
		ORDER BY category_type, name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var result []DataCategory
	for rows.Next() {
		var c DataCategory
		if err := rows.Scan(&c.ID, &c.OrgID, &c.Name, &c.Description, &c.CategoryType,
			&c.IsSpecialCategory, &c.GDPRArticle, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		result = append(result, c)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Processing Activity operations
// ---------------------------------------------------------------------------

// CreateProcessingActivity creates a new GDPR processing activity with auto-generated PA-NNN reference.
// Flags DPIA required if high-risk criteria are met.
func (s *ROPAService) CreateProcessingActivity(ctx context.Context, orgID string, req CreateProcessingActivityRequest) (*ProcessingActivity, error) {
	jointJSON, _ := json.Marshal(req.JointControllers)
	subjectJSON, _ := json.Marshal(req.DataSubjectCategories)
	catJSON, _ := json.Marshal(req.DataCategories)
	specialJSON, _ := json.Marshal(req.SpecialCategories)
	recipientJSON, _ := json.Marshal(req.Recipients)
	transferJSON, _ := json.Marshal(req.ThirdCountryTransfers)
	securityJSON, _ := json.Marshal(req.SecurityMeasures)
	metadataJSON, _ := json.Marshal(req.Metadata)
	if metadataJSON == nil {
		metadataJSON = []byte("{}")
	}

	// Determine if DPIA is required based on high-risk indicators.
	dpiaRequired := s.assessDPIARequired(req)
	dpiaStatus := "not_required"
	if dpiaRequired {
		dpiaStatus = "pending"
	}

	var pa ProcessingActivity
	var jc, sc, dc, spc, rec, sec []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO processing_activities (
			organization_id, name, description, purpose, legal_basis, legal_basis_detail,
			data_controller_name, data_controller_contact, dpo_contact,
			joint_controllers, data_subject_categories, data_categories,
			special_categories, recipients, third_country_transfers,
			retention_period, retention_justification, security_measures,
			dpia_required, dpia_status, automated_decision_making, profiling_used,
			owner, department, status, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, $23, $24, 'draft', $25
		)
		RETURNING id, organization_id, activity_ref, name, description, purpose,
			legal_basis, legal_basis_detail, data_controller_name, data_controller_contact,
			dpo_contact, joint_controllers, data_subject_categories, data_categories,
			special_categories, recipients, third_country_transfers,
			retention_period, retention_justification, security_measures,
			dpia_required, dpia_status, automated_decision_making, profiling_used,
			status, owner, department, created_at, updated_at`,
		orgID, req.Name, req.Description, req.Purpose, req.LegalBasis, req.LegalBasisDetail,
		req.DataControllerName, req.DataControllerContact, req.DPOContact,
		jointJSON, subjectJSON, catJSON, specialJSON, recipientJSON, transferJSON,
		req.RetentionPeriod, req.RetentionJustification, securityJSON,
		dpiaRequired, dpiaStatus, req.AutomatedDecision, req.ProfilingUsed,
		req.Owner, req.Department, metadataJSON,
	).Scan(
		&pa.ID, &pa.OrgID, &pa.ActivityRef, &pa.Name, &pa.Description, &pa.Purpose,
		&pa.LegalBasis, &pa.LegalBasisDetail, &pa.DataControllerName, &pa.DataControllerContact,
		&pa.DPOContact, &jc, &sc, &dc, &spc, &rec, &transferJSON,
		&pa.RetentionPeriod, &pa.RetentionJustification, &sec,
		&pa.DPIARequired, &pa.DPIAStatus, &pa.AutomatedDecision, &pa.ProfilingUsed,
		&pa.Status, &pa.Owner, &pa.Department, &pa.CreatedAt, &pa.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create processing activity: %w", err)
	}

	_ = json.Unmarshal(jc, &pa.JointControllers)
	_ = json.Unmarshal(sc, &pa.DataSubjectCategories)
	_ = json.Unmarshal(dc, &pa.DataCategories)
	_ = json.Unmarshal(spc, &pa.SpecialCategories)
	_ = json.Unmarshal(rec, &pa.Recipients)
	_ = json.Unmarshal(sec, &pa.SecurityMeasures)
	_ = json.Unmarshal(transferJSON, &pa.ThirdCountryTransfers)

	log.Info().
		Str("activity_id", pa.ID).
		Str("ref", pa.ActivityRef).
		Bool("dpia_required", dpiaRequired).
		Msg("processing activity created")

	return &pa, nil
}

// assessDPIARequired checks EDPB guidelines for DPIA triggers.
func (s *ROPAService) assessDPIARequired(req CreateProcessingActivityRequest) bool {
	triggers := 0

	// Systematic monitoring or profiling.
	if req.ProfilingUsed || req.AutomatedDecision {
		triggers++
	}
	// Special categories of data.
	if len(req.SpecialCategories) > 0 {
		triggers++
	}
	// Large-scale processing (heuristic: multiple subject categories).
	if len(req.DataSubjectCategories) >= 3 {
		triggers++
	}
	// Cross-border transfers.
	if len(req.ThirdCountryTransfers) > 0 {
		triggers++
	}
	// Vulnerable data subjects (minors, patients, employees).
	vulnerable := []string{"minors", "patients", "children"}
	for _, subj := range req.DataSubjectCategories {
		for _, v := range vulnerable {
			if strings.EqualFold(subj, v) {
				triggers++
				break
			}
		}
	}

	// EDPB guideline: 2+ triggers means DPIA required.
	return triggers >= 2
}

// UpdateProcessingActivity updates an existing processing activity.
func (s *ROPAService) UpdateProcessingActivity(ctx context.Context, orgID, activityID string, req UpdateProcessingActivityRequest) error {
	// Build dynamic update.
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	addArg := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", clause, argIdx))
		args = append(args, val)
		argIdx++
	}

	if req.Name != nil {
		addArg("name", *req.Name)
	}
	if req.Description != nil {
		addArg("description", *req.Description)
	}
	if req.Purpose != nil {
		addArg("purpose", *req.Purpose)
	}
	if req.LegalBasis != nil {
		addArg("legal_basis", *req.LegalBasis)
	}
	if req.Status != nil {
		addArg("status", *req.Status)
	}
	if req.RetentionPeriod != nil {
		addArg("retention_period", *req.RetentionPeriod)
	}
	if req.SecurityMeasures != nil {
		secJSON, _ := json.Marshal(req.SecurityMeasures)
		addArg("security_measures", secJSON)
	}
	if req.Owner != nil {
		addArg("owner", *req.Owner)
	}
	if req.Department != nil {
		addArg("department", *req.Department)
	}
	if req.NextReviewDate != nil {
		addArg("next_review_date", *req.NextReviewDate)
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf("UPDATE processing_activities SET %s WHERE id = $%d AND organization_id = $%d",
		strings.Join(setClauses, ", "), argIdx, argIdx+1)
	args = append(args, activityID, orgID)

	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update processing activity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProcessingActivityNotFound
	}
	return nil
}

// MapDataFlow maps a data flow for a processing activity.
func (s *ROPAService) MapDataFlow(ctx context.Context, orgID, activityID string, req CreateDataFlowRequest) (*DataFlow, error) {
	// Verify activity exists.
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM processing_activities WHERE id = $1 AND organization_id = $2)`,
		activityID, orgID).Scan(&exists)
	if err != nil || !exists {
		return nil, ErrProcessingActivityNotFound
	}

	elemJSON, _ := json.Marshal(req.DataElements)

	var f DataFlow
	var elems []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO data_flows (
			organization_id, activity_id, source_system, source_type,
			destination_system, destination_type, data_elements,
			transfer_method, encrypted, frequency, volume
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, organization_id, activity_id, source_system, source_type,
			destination_system, destination_type, data_elements,
			transfer_method, encrypted, frequency, volume, created_at`,
		orgID, activityID, req.SourceSystem, req.SourceType,
		req.DestSystem, req.DestType, elemJSON,
		req.TransferMethod, req.Encrypted, req.Frequency, req.Volume,
	).Scan(
		&f.ID, &f.OrgID, &f.ActivityID, &f.SourceSystem, &f.SourceType,
		&f.DestSystem, &f.DestType, &elems,
		&f.TransferMethod, &f.Encrypted, &f.Frequency, &f.Volume, &f.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create data flow: %w", err)
	}
	_ = json.Unmarshal(elems, &f.DataElements)

	log.Info().Str("flow_id", f.ID).Str("activity_id", activityID).Msg("data flow mapped")
	return &f, nil
}

// GenerateROPA exports a complete ROPA document with all Article 30(1) required fields.
func (s *ROPAService) GenerateROPA(ctx context.Context, orgID, format string) (*ROPADocument, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, activity_ref, name, description, purpose,
			legal_basis, legal_basis_detail, data_controller_name, data_controller_contact,
			dpo_contact, joint_controllers, data_subject_categories, data_categories,
			special_categories, recipients, third_country_transfers,
			retention_period, retention_justification, security_measures,
			dpia_required, dpia_status, dpia_date, automated_decision_making, profiling_used,
			status, owner, department, next_review_date, created_at, updated_at
		FROM processing_activities
		WHERE organization_id = $1 AND status IN ('active', 'draft', 'under_review')
		ORDER BY activity_ref`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query activities: %w", err)
	}
	defer rows.Close()

	doc := &ROPADocument{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Format:      format,
	}

	// Get org name.
	_ = s.pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&doc.Organization)

	for rows.Next() {
		pa, err := s.scanProcessingActivity(rows)
		if err != nil {
			return nil, err
		}
		doc.Activities = append(doc.Activities, *pa)
	}
	doc.TotalCount = len(doc.Activities)

	log.Info().Str("org_id", orgID).Int("count", doc.TotalCount).Msg("ROPA document generated")
	return doc, nil
}

// GetROPADashboard returns aggregate ROPA statistics.
func (s *ROPAService) GetROPADashboard(ctx context.Context, orgID string) (*ROPADashboard, error) {
	dash := &ROPADashboard{
		ByLegalBasis: make(map[string]int),
		ByStatus:     make(map[string]int),
		ByDepartment: make(map[string]int),
	}

	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM processing_activities WHERE organization_id = $1`, orgID).Scan(&dash.TotalActivities)

	// By legal basis.
	rows, _ := s.pool.Query(ctx, `
		SELECT legal_basis, COUNT(*) FROM processing_activities
		WHERE organization_id = $1 GROUP BY legal_basis`, orgID)
	if rows != nil {
		for rows.Next() {
			var k string
			var v int
			rows.Scan(&k, &v)
			dash.ByLegalBasis[k] = v
		}
		rows.Close()
	}

	// By status.
	rows, _ = s.pool.Query(ctx, `
		SELECT status, COUNT(*) FROM processing_activities
		WHERE organization_id = $1 GROUP BY status`, orgID)
	if rows != nil {
		for rows.Next() {
			var k string
			var v int
			rows.Scan(&k, &v)
			dash.ByStatus[k] = v
		}
		rows.Close()
	}

	// By department.
	rows, _ = s.pool.Query(ctx, `
		SELECT department, COUNT(*) FROM processing_activities
		WHERE organization_id = $1 AND department != '' GROUP BY department`, orgID)
	if rows != nil {
		for rows.Next() {
			var k string
			var v int
			rows.Scan(&k, &v)
			dash.ByDepartment[k] = v
		}
		rows.Close()
	}

	// Special categories.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM processing_activities
		WHERE organization_id = $1 AND jsonb_array_length(special_categories) > 0`, orgID).Scan(&dash.SpecialCategoryCount)

	// Third country transfers.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM processing_activities
		WHERE organization_id = $1 AND jsonb_array_length(third_country_transfers) > 0`, orgID).Scan(&dash.TransferCount)

	// DPIA status.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM processing_activities
		WHERE organization_id = $1 AND dpia_status = 'pending'`, orgID).Scan(&dash.DPIAPending)
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM processing_activities
		WHERE organization_id = $1 AND dpia_status = 'completed'`, orgID).Scan(&dash.DPIACompleted)

	// Overdue reviews.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM processing_activities
		WHERE organization_id = $1 AND next_review_date IS NOT NULL AND next_review_date < CURRENT_DATE`,
		orgID).Scan(&dash.OverdueReviews)

	return dash, nil
}

// IdentifyHighRiskProcessing flags processing activities that require a DPIA per EDPB guidelines.
func (s *ROPAService) IdentifyHighRiskProcessing(ctx context.Context, orgID string) ([]HighRiskIndicator, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, activity_ref, name,
			profiling_used, automated_decision_making,
			jsonb_array_length(special_categories) AS special_count,
			jsonb_array_length(third_country_transfers) AS transfer_count,
			jsonb_array_length(data_subject_categories) AS subject_count,
			data_subject_categories
		FROM processing_activities
		WHERE organization_id = $1 AND status != 'retired'`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query activities: %w", err)
	}
	defer rows.Close()

	var result []HighRiskIndicator
	for rows.Next() {
		var id, ref, name string
		var profiling, automated bool
		var specialCount, transferCount, subjectCount int
		var subjectsJSON []byte
		if err := rows.Scan(&id, &ref, &name, &profiling, &automated,
			&specialCount, &transferCount, &subjectCount, &subjectsJSON); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}

		var triggers []string
		if profiling {
			triggers = append(triggers, "profiling_used")
		}
		if automated {
			triggers = append(triggers, "automated_decision_making")
		}
		if specialCount > 0 {
			triggers = append(triggers, "special_category_data")
		}
		if transferCount > 0 {
			triggers = append(triggers, "third_country_transfers")
		}
		if subjectCount >= 3 {
			triggers = append(triggers, "large_scale_processing")
		}

		var subjects []string
		_ = json.Unmarshal(subjectsJSON, &subjects)
		for _, subj := range subjects {
			lower := strings.ToLower(subj)
			if lower == "minors" || lower == "children" || lower == "patients" {
				triggers = append(triggers, "vulnerable_data_subjects")
				break
			}
		}

		if len(triggers) >= 2 {
			result = append(result, HighRiskIndicator{
				ActivityID:   id,
				ActivityRef:  ref,
				Name:         name,
				RiskTriggers: triggers,
				DPIARequired: true,
			})
		}
	}
	return result, nil
}

// DataSubjectImpactMap returns what data is collected for a given subject category.
func (s *ROPAService) DataSubjectImpactMap(ctx context.Context, orgID, subjectCategory string) ([]DataSubjectImpactEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pa.activity_ref, pa.name, pa.purpose, pa.data_categories,
			pa.legal_basis, pa.retention_period, pa.recipients, pa.third_country_transfers
		FROM processing_activities pa
		WHERE pa.organization_id = $1
			AND pa.status != 'retired'
			AND pa.data_subject_categories @> $2::jsonb
		ORDER BY pa.activity_ref`,
		orgID, fmt.Sprintf(`["%s"]`, subjectCategory))
	if err != nil {
		return nil, fmt.Errorf("query impact map: %w", err)
	}
	defer rows.Close()

	var result []DataSubjectImpactEntry
	for rows.Next() {
		var entry DataSubjectImpactEntry
		var dataElems, recipients, transfers []byte
		if err := rows.Scan(&entry.ActivityRef, &entry.ActivityName, &entry.Purpose,
			&dataElems, &entry.LegalBasis, &entry.Retention, &recipients, &transfers); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		_ = json.Unmarshal(dataElems, &entry.DataElements)
		_ = json.Unmarshal(recipients, &entry.Recipients)
		var tct []ThirdCountryTransfer
		_ = json.Unmarshal(transfers, &tct)
		for _, t := range tct {
			entry.Transfers = append(entry.Transfers, fmt.Sprintf("%s (%s)", t.Country, t.Safeguard))
		}
		result = append(result, entry)
	}
	return result, nil
}

// ListProcessingActivities returns paginated processing activities.
func (s *ROPAService) ListProcessingActivities(ctx context.Context, orgID string, page, pageSize int) ([]ProcessingActivity, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM processing_activities WHERE organization_id = $1`, orgID).Scan(&total)

	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, activity_ref, name, description, purpose,
			legal_basis, legal_basis_detail, data_controller_name, data_controller_contact,
			dpo_contact, joint_controllers, data_subject_categories, data_categories,
			special_categories, recipients, third_country_transfers,
			retention_period, retention_justification, security_measures,
			dpia_required, dpia_status, dpia_date, automated_decision_making, profiling_used,
			status, owner, department, next_review_date, created_at, updated_at
		FROM processing_activities WHERE organization_id = $1
		ORDER BY activity_ref
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	var result []ProcessingActivity
	for rows.Next() {
		pa, err := s.scanProcessingActivity(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, *pa)
	}
	return result, total, nil
}

// GetProcessingActivity retrieves a single processing activity by ID.
func (s *ROPAService) GetProcessingActivity(ctx context.Context, orgID, activityID string) (*ProcessingActivity, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, activity_ref, name, description, purpose,
			legal_basis, legal_basis_detail, data_controller_name, data_controller_contact,
			dpo_contact, joint_controllers, data_subject_categories, data_categories,
			special_categories, recipients, third_country_transfers,
			retention_period, retention_justification, security_measures,
			dpia_required, dpia_status, dpia_date, automated_decision_making, profiling_used,
			status, owner, department, next_review_date, created_at, updated_at
		FROM processing_activities
		WHERE id = $1 AND organization_id = $2`, activityID, orgID)

	pa, err := s.scanProcessingActivityRow(row)
	if err != nil {
		return nil, ErrProcessingActivityNotFound
	}
	return pa, nil
}

// ListDataFlows returns all data flows for a processing activity.
func (s *ROPAService) ListDataFlows(ctx context.Context, orgID, activityID string) ([]DataFlow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, activity_id, source_system, source_type,
			destination_system, destination_type, data_elements,
			transfer_method, encrypted, frequency, volume, created_at
		FROM data_flows
		WHERE organization_id = $1 AND activity_id = $2
		ORDER BY created_at`, orgID, activityID)
	if err != nil {
		return nil, fmt.Errorf("list data flows: %w", err)
	}
	defer rows.Close()

	var result []DataFlow
	for rows.Next() {
		var f DataFlow
		var elems []byte
		if err := rows.Scan(&f.ID, &f.OrgID, &f.ActivityID, &f.SourceSystem, &f.SourceType,
			&f.DestSystem, &f.DestType, &elems,
			&f.TransferMethod, &f.Encrypted, &f.Frequency, &f.Volume, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan flow: %w", err)
		}
		_ = json.Unmarshal(elems, &f.DataElements)
		result = append(result, f)
	}
	return result, nil
}

// scanProcessingActivity scans a Rows cursor into a ProcessingActivity.
func (s *ROPAService) scanProcessingActivity(rows pgx.Rows) (*ProcessingActivity, error) {
	var pa ProcessingActivity
	var jc, sc, dc, spc, rec, transfers, sec, meta []byte
	err := rows.Scan(
		&pa.ID, &pa.OrgID, &pa.ActivityRef, &pa.Name, &pa.Description, &pa.Purpose,
		&pa.LegalBasis, &pa.LegalBasisDetail, &pa.DataControllerName, &pa.DataControllerContact,
		&pa.DPOContact, &jc, &sc, &dc, &spc, &rec, &transfers,
		&pa.RetentionPeriod, &pa.RetentionJustification, &sec,
		&pa.DPIARequired, &pa.DPIAStatus, &pa.DPIADate, &pa.AutomatedDecision, &pa.ProfilingUsed,
		&pa.Status, &pa.Owner, &pa.Department, &pa.NextReviewDate, &pa.CreatedAt, &pa.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan processing activity: %w", err)
	}
	_ = json.Unmarshal(jc, &pa.JointControllers)
	_ = json.Unmarshal(sc, &pa.DataSubjectCategories)
	_ = json.Unmarshal(dc, &pa.DataCategories)
	_ = json.Unmarshal(spc, &pa.SpecialCategories)
	_ = json.Unmarshal(rec, &pa.Recipients)
	_ = json.Unmarshal(transfers, &pa.ThirdCountryTransfers)
	_ = json.Unmarshal(sec, &pa.SecurityMeasures)
	_ = json.Unmarshal(meta, &pa.Metadata)
	return &pa, nil
}

// scanProcessingActivityRow scans a single Row into a ProcessingActivity.
func (s *ROPAService) scanProcessingActivityRow(row pgx.Row) (*ProcessingActivity, error) {
	var pa ProcessingActivity
	var jc, sc, dc, spc, rec, transfers, sec []byte
	err := row.Scan(
		&pa.ID, &pa.OrgID, &pa.ActivityRef, &pa.Name, &pa.Description, &pa.Purpose,
		&pa.LegalBasis, &pa.LegalBasisDetail, &pa.DataControllerName, &pa.DataControllerContact,
		&pa.DPOContact, &jc, &sc, &dc, &spc, &rec, &transfers,
		&pa.RetentionPeriod, &pa.RetentionJustification, &sec,
		&pa.DPIARequired, &pa.DPIAStatus, &pa.DPIADate, &pa.AutomatedDecision, &pa.ProfilingUsed,
		&pa.Status, &pa.Owner, &pa.Department, &pa.NextReviewDate, &pa.CreatedAt, &pa.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(jc, &pa.JointControllers)
	_ = json.Unmarshal(sc, &pa.DataSubjectCategories)
	_ = json.Unmarshal(dc, &pa.DataCategories)
	_ = json.Unmarshal(spc, &pa.SpecialCategories)
	_ = json.Unmarshal(rec, &pa.Recipients)
	_ = json.Unmarshal(transfers, &pa.ThirdCountryTransfers)
	_ = json.Unmarshal(sec, &pa.SecurityMeasures)
	return &pa, nil
}
