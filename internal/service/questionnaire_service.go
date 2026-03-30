package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrQuestionnaireNotFound = fmt.Errorf("questionnaire not found")
	ErrAssessmentNotFound    = fmt.Errorf("assessment not found")
	ErrInvalidPortalToken    = fmt.Errorf("invalid or expired portal token")
	ErrAssessmentNotPending  = fmt.Errorf("assessment is not in a submittable state")
	ErrMissingRequiredAnswer = fmt.Errorf("one or more required questions are unanswered")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// Questionnaire is a vendor assessment questionnaire template.
type Questionnaire struct {
	ID            string              `json:"id"`
	OrgID         string              `json:"organization_id"`
	Title         string              `json:"title"`
	Description   string              `json:"description"`
	Category      string              `json:"category"` // security, privacy, compliance, risk, general
	Version       int                 `json:"version"`
	Status        string              `json:"status"` // draft, published, archived
	ScoringMethod string              `json:"scoring_method"` // weighted_average, pass_fail, risk_rated
	Sections      []QuestionSection   `json:"sections"`
	IsTemplate    bool                `json:"is_template"`
	CreatedBy     string              `json:"created_by"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// QuestionSection groups related questions.
type QuestionSection struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Weight       float64    `json:"weight"`
	SortOrder    int        `json:"sort_order"`
	Questions    []Question `json:"questions"`
}

// Question is a single question within a section.
type Question struct {
	ID            string   `json:"id"`
	SectionID     string   `json:"section_id"`
	QuestionText  string   `json:"question_text"`
	QuestionType  string   `json:"question_type"` // yes_no, multiple_choice, text, rating, file_upload
	IsRequired    bool     `json:"is_required"`
	Weight        float64  `json:"weight"`
	Options       []string `json:"options"`
	SortOrder     int      `json:"sort_order"`
	GuidanceText  string   `json:"guidance_text"`
}

// VendorAssessment is a sent questionnaire instance for a specific vendor.
type VendorAssessment struct {
	ID                string                 `json:"id"`
	OrgID             string                 `json:"organization_id"`
	QuestionnaireID   string                 `json:"questionnaire_id"`
	VendorID          string                 `json:"vendor_id"`
	VendorName        string                 `json:"vendor_name"`
	ContactEmail      string                 `json:"contact_email"`
	Status            string                 `json:"status"` // sent, in_progress, submitted, reviewed, expired
	DueDate           string                 `json:"due_date"`
	SubmittedAt       *string                `json:"submitted_at"`
	ReviewedAt        *string                `json:"reviewed_at"`
	ReviewedBy        *string                `json:"reviewed_by"`
	ReviewComments    *string                `json:"review_comments"`
	ReviewOutcome     *string                `json:"review_outcome"` // approved, conditionally_approved, rejected, needs_followup
	OverallScore      *float64               `json:"overall_score"`
	SectionScores     map[string]float64     `json:"section_scores"`
	RiskLevel         *string                `json:"risk_level"`
	TokenHash         string                 `json:"-"`
	ReminderCount     int                    `json:"reminder_count"`
	LastReminderAt    *string                `json:"last_reminder_at"`
	Answers           []AssessmentAnswer     `json:"answers"`
	CreatedAt         string                 `json:"created_at"`
}

// AssessmentAnswer is a vendor's response to a single question.
type AssessmentAnswer struct {
	QuestionID   string  `json:"question_id"`
	QuestionText string  `json:"question_text"`
	AnswerValue  string  `json:"answer_value"`
	AnswerScore  float64 `json:"answer_score"`
	Comments     string  `json:"comments"`
	FileURL      *string `json:"file_url"`
}

// AssessmentReview holds reviewer input.
type AssessmentReview struct {
	ReviewerID string  `json:"reviewer_id"`
	Outcome    string  `json:"outcome"` // approved, conditionally_approved, rejected, needs_followup
	Comments   string  `json:"comments"`
	RiskLevel  *string `json:"risk_level"`
}

// VendorComparison provides side-by-side assessment scores.
type VendorComparison struct {
	Assessments []VendorComparisonEntry `json:"assessments"`
}

// VendorComparisonEntry holds a single vendor's comparison data.
type VendorComparisonEntry struct {
	AssessmentID  string             `json:"assessment_id"`
	VendorID      string             `json:"vendor_id"`
	VendorName    string             `json:"vendor_name"`
	OverallScore  float64            `json:"overall_score"`
	RiskLevel     string             `json:"risk_level"`
	SectionScores map[string]float64 `json:"section_scores"`
	SubmittedAt   string             `json:"submitted_at"`
}

// AssessmentDashboard provides aggregate assessment statistics.
type AssessmentDashboard struct {
	Total           int            `json:"total"`
	ByStatus        map[string]int `json:"by_status"`
	AvgScore        float64        `json:"avg_score"`
	OverdueCount    int            `json:"overdue_count"`
	HighRiskVendors int            `json:"high_risk_vendors"`
	PendingReview   int            `json:"pending_review"`
	CompletionRate  float64        `json:"completion_rate"`
}

// CreateQuestionnaireRequest holds input for creating a questionnaire.
type CreateQuestionnaireRequest struct {
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Category      string            `json:"category"`
	ScoringMethod string            `json:"scoring_method"`
	Sections      []QuestionSection `json:"sections"`
	CreatedBy     string            `json:"created_by"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// QuestionnaireService manages vendor assessment questionnaires.
type QuestionnaireService struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// NewQuestionnaireService creates a new QuestionnaireService.
func NewQuestionnaireService(pool *pgxpool.Pool, bus *EventBus) *QuestionnaireService {
	return &QuestionnaireService{pool: pool, bus: bus}
}

// CreateQuestionnaire creates a new questionnaire with sections and questions.
func (s *QuestionnaireService) CreateQuestionnaire(ctx context.Context, orgID string, req CreateQuestionnaireRequest) (*Questionnaire, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var q Questionnaire
	err = tx.QueryRow(ctx, `
		INSERT INTO questionnaires (
			organization_id, title, description, category, scoring_method, status, created_by
		) VALUES ($1, $2, $3, $4, $5, 'draft', $6)
		RETURNING id, organization_id, title, description, category, version, status,
			scoring_method, is_template, created_by, created_at, updated_at`,
		orgID, req.Title, req.Description, req.Category, req.ScoringMethod, req.CreatedBy,
	).Scan(
		&q.ID, &q.OrgID, &q.Title, &q.Description, &q.Category, &q.Version, &q.Status,
		&q.ScoringMethod, &q.IsTemplate, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create questionnaire: %w", err)
	}

	for si, sec := range req.Sections {
		var secID string
		err = tx.QueryRow(ctx, `
			INSERT INTO questionnaire_sections (
				questionnaire_id, title, description, weight, sort_order
			) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			q.ID, sec.Title, sec.Description, sec.Weight, si+1,
		).Scan(&secID)
		if err != nil {
			return nil, fmt.Errorf("create section: %w", err)
		}

		sec.ID = secID
		sec.SortOrder = si + 1
		var questions []Question
		for qi, question := range sec.Questions {
			optJSON, _ := json.Marshal(question.Options)
			var qID string
			err = tx.QueryRow(ctx, `
				INSERT INTO questionnaire_questions (
					section_id, question_text, question_type, is_required, weight,
					options, sort_order, guidance_text
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
				secID, question.QuestionText, question.QuestionType, question.IsRequired,
				question.Weight, optJSON, qi+1, question.GuidanceText,
			).Scan(&qID)
			if err != nil {
				return nil, fmt.Errorf("create question: %w", err)
			}
			question.ID = qID
			question.SectionID = secID
			question.SortOrder = qi + 1
			questions = append(questions, question)
		}
		sec.Questions = questions
		q.Sections = append(q.Sections, sec)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	log.Info().Str("questionnaire_id", q.ID).Str("title", q.Title).Msg("questionnaire created")
	return &q, nil
}

// CloneTemplate creates a new questionnaire by cloning a template.
func (s *QuestionnaireService) CloneTemplate(ctx context.Context, orgID, templateID string) (*Questionnaire, error) {
	// Get the source questionnaire.
	src, err := s.GetQuestionnaire(ctx, templateID)
	if err != nil {
		return nil, err
	}

	req := CreateQuestionnaireRequest{
		Title:         src.Title + " (Copy)",
		Description:   src.Description,
		Category:      src.Category,
		ScoringMethod: src.ScoringMethod,
		Sections:      src.Sections,
		CreatedBy:     "system",
	}
	return s.CreateQuestionnaire(ctx, orgID, req)
}

// GetQuestionnaire retrieves a questionnaire with all sections and questions.
func (s *QuestionnaireService) GetQuestionnaire(ctx context.Context, id string) (*Questionnaire, error) {
	var q Questionnaire
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, title, description, category, version, status,
			scoring_method, is_template, created_by, created_at, updated_at
		FROM questionnaires WHERE id = $1`, id,
	).Scan(
		&q.ID, &q.OrgID, &q.Title, &q.Description, &q.Category, &q.Version, &q.Status,
		&q.ScoringMethod, &q.IsTemplate, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, ErrQuestionnaireNotFound
	}

	// Load sections.
	secRows, err := s.pool.Query(ctx, `
		SELECT id, title, description, weight, sort_order
		FROM questionnaire_sections WHERE questionnaire_id = $1 ORDER BY sort_order`, q.ID)
	if err != nil {
		return nil, fmt.Errorf("load sections: %w", err)
	}
	defer secRows.Close()

	for secRows.Next() {
		var sec QuestionSection
		if err := secRows.Scan(&sec.ID, &sec.Title, &sec.Description, &sec.Weight, &sec.SortOrder); err != nil {
			return nil, fmt.Errorf("scan section: %w", err)
		}

		qRows, err := s.pool.Query(ctx, `
			SELECT id, section_id, question_text, question_type, is_required, weight,
				options, sort_order, guidance_text
			FROM questionnaire_questions WHERE section_id = $1 ORDER BY sort_order`, sec.ID)
		if err != nil {
			return nil, fmt.Errorf("load questions: %w", err)
		}

		for qRows.Next() {
			var question Question
			var optJSON []byte
			if err := qRows.Scan(&question.ID, &question.SectionID, &question.QuestionText,
				&question.QuestionType, &question.IsRequired, &question.Weight,
				&optJSON, &question.SortOrder, &question.GuidanceText); err != nil {
				qRows.Close()
				return nil, fmt.Errorf("scan question: %w", err)
			}
			_ = json.Unmarshal(optJSON, &question.Options)
			sec.Questions = append(sec.Questions, question)
		}
		qRows.Close()

		q.Sections = append(q.Sections, sec)
	}

	return &q, nil
}

// ListQuestionnaires returns paginated questionnaires for an organization.
func (s *QuestionnaireService) ListQuestionnaires(ctx context.Context, orgID string, page, pageSize int) ([]Questionnaire, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM questionnaires WHERE organization_id = $1`, orgID).Scan(&total)

	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, title, description, category, version, status,
			scoring_method, is_template, created_by, created_at, updated_at
		FROM questionnaires WHERE organization_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list questionnaires: %w", err)
	}
	defer rows.Close()

	var result []Questionnaire
	for rows.Next() {
		var q Questionnaire
		if err := rows.Scan(&q.ID, &q.OrgID, &q.Title, &q.Description, &q.Category, &q.Version,
			&q.Status, &q.ScoringMethod, &q.IsTemplate, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan questionnaire: %w", err)
		}
		result = append(result, q)
	}
	return result, total, nil
}

// generateToken creates a cryptographically secure random token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken returns the SHA-256 hash of a token.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// SendAssessment sends a questionnaire to a vendor, generating a secure access token.
func (s *QuestionnaireService) SendAssessment(ctx context.Context, orgID, vendorID, questionnaireID string, dueDate, contactEmail string) (*VendorAssessment, error) {
	// Generate secure access token.
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	tokenHash := hashToken(token)

	// Get vendor name.
	var vendorName string
	_ = s.pool.QueryRow(ctx, `SELECT name FROM vendors WHERE id = $1`, vendorID).Scan(&vendorName)

	var a VendorAssessment
	err = s.pool.QueryRow(ctx, `
		INSERT INTO vendor_assessments (
			organization_id, questionnaire_id, vendor_id, vendor_name,
			contact_email, status, due_date, token_hash
		) VALUES ($1, $2, $3, $4, $5, 'sent', $6, $7)
		RETURNING id, organization_id, questionnaire_id, vendor_id, vendor_name,
			contact_email, status, due_date, reminder_count, created_at`,
		orgID, questionnaireID, vendorID, vendorName,
		contactEmail, dueDate, tokenHash,
	).Scan(
		&a.ID, &a.OrgID, &a.QuestionnaireID, &a.VendorID, &a.VendorName,
		&a.ContactEmail, &a.Status, &a.DueDate, &a.ReminderCount, &a.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create assessment: %w", err)
	}

	s.bus.Publish(Event{
		Type:       "assessment.sent",
		Severity:   "low",
		OrgID:      orgID,
		EntityType: "vendor_assessment",
		EntityID:   a.ID,
		Data: map[string]interface{}{
			"vendor_id":    vendorID,
			"vendor_name":  vendorName,
			"due_date":     dueDate,
			"access_token": token, // Included for email delivery; not stored in plaintext.
		},
		Timestamp: time.Now(),
	})

	log.Info().
		Str("assessment_id", a.ID).
		Str("vendor", vendorName).
		Str("due_date", dueDate).
		Msg("assessment sent to vendor")

	return &a, nil
}

// SubmitAssessment processes a vendor's assessment submission via portal token.
func (s *QuestionnaireService) SubmitAssessment(ctx context.Context, token string, answers []AssessmentAnswer) error {
	// Validate token.
	assessment, err := s.ValidatePortalToken(ctx, token)
	if err != nil {
		return err
	}
	if assessment.Status != "sent" && assessment.Status != "in_progress" {
		return ErrAssessmentNotPending
	}

	// Validate all required questions are answered.
	var requiredIDs []string
	rows, err := s.pool.Query(ctx, `
		SELECT qq.id FROM questionnaire_questions qq
		JOIN questionnaire_sections qs ON qq.section_id = qs.id
		WHERE qs.questionnaire_id = $1 AND qq.is_required = true`,
		assessment.QuestionnaireID)
	if err != nil {
		return fmt.Errorf("get required questions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		rows.Scan(&id)
		requiredIDs = append(requiredIDs, id)
	}

	answeredMap := make(map[string]bool)
	for _, a := range answers {
		answeredMap[a.QuestionID] = true
	}
	for _, reqID := range requiredIDs {
		if !answeredMap[reqID] {
			return ErrMissingRequiredAnswer
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Store answers.
	for _, a := range answers {
		_, err = tx.Exec(ctx, `
			INSERT INTO assessment_answers (
				assessment_id, question_id, answer_value, comments, file_url
			) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (assessment_id, question_id) DO UPDATE
			SET answer_value = $3, comments = $4, file_url = $5, updated_at = NOW()`,
			assessment.ID, a.QuestionID, a.AnswerValue, a.Comments, a.FileURL)
		if err != nil {
			return fmt.Errorf("store answer: %w", err)
		}
	}

	// Update assessment status.
	_, err = tx.Exec(ctx, `
		UPDATE vendor_assessments SET status = 'submitted', submitted_at = NOW(), updated_at = NOW()
		WHERE id = $1`, assessment.ID)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Calculate score asynchronously (but we do it here synchronously for simplicity).
	_, _ = s.CalculateScore(ctx, assessment.ID)

	s.bus.Publish(Event{
		Type:       "assessment.submitted",
		Severity:   "medium",
		OrgID:      assessment.OrgID,
		EntityType: "vendor_assessment",
		EntityID:   assessment.ID,
		Data:       map[string]interface{}{"vendor_id": assessment.VendorID, "vendor_name": assessment.VendorName},
		Timestamp:  time.Now(),
	})

	log.Info().Str("assessment_id", assessment.ID).Msg("assessment submitted")
	return nil
}

// ReviewAssessment records a reviewer's decision on a submitted assessment.
func (s *QuestionnaireService) ReviewAssessment(ctx context.Context, orgID, assessmentID string, review AssessmentReview) error {
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT status FROM vendor_assessments WHERE id = $1 AND organization_id = $2`,
		assessmentID, orgID).Scan(&status)
	if err != nil {
		return ErrAssessmentNotFound
	}
	if status != "submitted" {
		return fmt.Errorf("assessment must be in 'submitted' status for review")
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE vendor_assessments
		SET status = 'reviewed', reviewed_at = NOW(), reviewed_by = $1,
			review_comments = $2, review_outcome = $3,
			risk_level = COALESCE($4, risk_level), updated_at = NOW()
		WHERE id = $5`,
		review.ReviewerID, review.Comments, review.Outcome, review.RiskLevel, assessmentID)
	if err != nil {
		return fmt.Errorf("review assessment: %w", err)
	}

	log.Info().
		Str("assessment_id", assessmentID).
		Str("outcome", review.Outcome).
		Str("reviewer", review.ReviewerID).
		Msg("assessment reviewed")
	return nil
}

// CalculateScore computes the assessment score based on the questionnaire's scoring method.
func (s *QuestionnaireService) CalculateScore(ctx context.Context, assessmentID string) (*float64, error) {
	var qID, scoringMethod string
	err := s.pool.QueryRow(ctx, `
		SELECT va.questionnaire_id, q.scoring_method
		FROM vendor_assessments va
		JOIN questionnaires q ON va.questionnaire_id = q.id
		WHERE va.id = $1`, assessmentID).Scan(&qID, &scoringMethod)
	if err != nil {
		return nil, ErrAssessmentNotFound
	}

	var overallScore float64
	sectionScores := make(map[string]float64)

	// Get sections and their weights.
	secRows, err := s.pool.Query(ctx, `
		SELECT id, title, weight FROM questionnaire_sections
		WHERE questionnaire_id = $1 ORDER BY sort_order`, qID)
	if err != nil {
		return nil, fmt.Errorf("get sections: %w", err)
	}
	defer secRows.Close()

	type sectionInfo struct {
		ID     string
		Title  string
		Weight float64
	}
	var sections []sectionInfo
	for secRows.Next() {
		var si sectionInfo
		secRows.Scan(&si.ID, &si.Title, &si.Weight)
		sections = append(sections, si)
	}

	totalWeight := 0.0
	weightedSum := 0.0

	for _, sec := range sections {
		// Score each section.
		var sectionScore float64

		switch scoringMethod {
		case "weighted_average":
			err = s.pool.QueryRow(ctx, `
				SELECT COALESCE(
					SUM(aa.answer_score * qq.weight) / NULLIF(SUM(qq.weight), 0), 0)
				FROM assessment_answers aa
				JOIN questionnaire_questions qq ON aa.question_id = qq.id
				WHERE aa.assessment_id = $1 AND qq.section_id = $2`,
				assessmentID, sec.ID).Scan(&sectionScore)

		case "pass_fail":
			var totalQ, passedQ int
			err = s.pool.QueryRow(ctx, `
				SELECT COUNT(*), COUNT(*) FILTER (WHERE aa.answer_score >= 1)
				FROM assessment_answers aa
				JOIN questionnaire_questions qq ON aa.question_id = qq.id
				WHERE aa.assessment_id = $1 AND qq.section_id = $2`,
				assessmentID, sec.ID).Scan(&totalQ, &passedQ)
			if totalQ > 0 {
				sectionScore = float64(passedQ) * 100.0 / float64(totalQ)
			}

		case "risk_rated":
			err = s.pool.QueryRow(ctx, `
				SELECT COALESCE(AVG(aa.answer_score), 0)
				FROM assessment_answers aa
				JOIN questionnaire_questions qq ON aa.question_id = qq.id
				WHERE aa.assessment_id = $1 AND qq.section_id = $2`,
				assessmentID, sec.ID).Scan(&sectionScore)
		}

		if err != nil {
			return nil, fmt.Errorf("calculate section score: %w", err)
		}

		sectionScores[sec.Title] = sectionScore
		weightedSum += sectionScore * sec.Weight
		totalWeight += sec.Weight
	}

	if totalWeight > 0 {
		overallScore = weightedSum / totalWeight
	}

	// Determine risk level from score.
	riskLevel := "low"
	switch {
	case overallScore < 40:
		riskLevel = "critical"
	case overallScore < 60:
		riskLevel = "high"
	case overallScore < 80:
		riskLevel = "medium"
	}

	scoresJSON, _ := json.Marshal(sectionScores)
	_, _ = s.pool.Exec(ctx, `
		UPDATE vendor_assessments
		SET overall_score = $1, section_scores = $2, risk_level = $3, updated_at = NOW()
		WHERE id = $4`, overallScore, scoresJSON, riskLevel, assessmentID)

	log.Info().
		Str("assessment_id", assessmentID).
		Float64("score", overallScore).
		Str("risk_level", riskLevel).
		Msg("assessment score calculated")

	return &overallScore, nil
}

// CompareVendors provides side-by-side comparison of vendor assessment scores.
func (s *QuestionnaireService) CompareVendors(ctx context.Context, orgID string, assessmentIDs []string) (*VendorComparison, error) {
	comparison := &VendorComparison{}

	for _, aID := range assessmentIDs {
		var entry VendorComparisonEntry
		var scoresJSON []byte
		var riskLevel, submittedAt *string
		var score *float64
		err := s.pool.QueryRow(ctx, `
			SELECT id, vendor_id, vendor_name, overall_score, risk_level,
				section_scores, submitted_at
			FROM vendor_assessments
			WHERE id = $1 AND organization_id = $2`,
			aID, orgID).Scan(
			&entry.AssessmentID, &entry.VendorID, &entry.VendorName,
			&score, &riskLevel, &scoresJSON, &submittedAt)
		if err != nil {
			continue
		}

		if score != nil {
			entry.OverallScore = *score
		}
		if riskLevel != nil {
			entry.RiskLevel = *riskLevel
		}
		if submittedAt != nil {
			entry.SubmittedAt = *submittedAt
		}
		entry.SectionScores = make(map[string]float64)
		_ = json.Unmarshal(scoresJSON, &entry.SectionScores)

		comparison.Assessments = append(comparison.Assessments, entry)
	}

	return comparison, nil
}

// SendReminder sends a reminder for an overdue or pending assessment.
func (s *QuestionnaireService) SendReminder(ctx context.Context, orgID, assessmentID string) error {
	var status, vendorName, contactEmail string
	err := s.pool.QueryRow(ctx, `
		SELECT status, vendor_name, contact_email
		FROM vendor_assessments WHERE id = $1 AND organization_id = $2`,
		assessmentID, orgID).Scan(&status, &vendorName, &contactEmail)
	if err != nil {
		return ErrAssessmentNotFound
	}
	if status != "sent" && status != "in_progress" {
		return fmt.Errorf("cannot send reminder for assessment in '%s' status", status)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE vendor_assessments
		SET reminder_count = reminder_count + 1, last_reminder_at = NOW(), updated_at = NOW()
		WHERE id = $1`, assessmentID)
	if err != nil {
		return fmt.Errorf("update reminder count: %w", err)
	}

	s.bus.Publish(Event{
		Type:       "assessment.reminder",
		Severity:   "low",
		OrgID:      orgID,
		EntityType: "vendor_assessment",
		EntityID:   assessmentID,
		Data: map[string]interface{}{
			"vendor_name":   vendorName,
			"contact_email": contactEmail,
		},
		Timestamp: time.Now(),
	})

	log.Info().Str("assessment_id", assessmentID).Str("vendor", vendorName).Msg("reminder sent")
	return nil
}

// GetAssessmentDashboard returns aggregate assessment statistics.
func (s *QuestionnaireService) GetAssessmentDashboard(ctx context.Context, orgID string) (*AssessmentDashboard, error) {
	dash := &AssessmentDashboard{ByStatus: make(map[string]int)}

	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM vendor_assessments WHERE organization_id = $1`, orgID).Scan(&dash.Total)

	rows, err := s.pool.Query(ctx, `
		SELECT status, COUNT(*) FROM vendor_assessments
		WHERE organization_id = $1 GROUP BY status`, orgID)
	if err != nil {
		return nil, fmt.Errorf("by status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var cnt int
		rows.Scan(&st, &cnt)
		dash.ByStatus[st] = cnt
	}

	_ = s.pool.QueryRow(ctx, `
		SELECT COALESCE(AVG(overall_score), 0) FROM vendor_assessments
		WHERE organization_id = $1 AND overall_score IS NOT NULL`, orgID).Scan(&dash.AvgScore)

	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM vendor_assessments
		WHERE organization_id = $1 AND status IN ('sent', 'in_progress')
			AND due_date < CURRENT_DATE`, orgID).Scan(&dash.OverdueCount)

	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM vendor_assessments
		WHERE organization_id = $1 AND risk_level IN ('critical', 'high')`, orgID).Scan(&dash.HighRiskVendors)

	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM vendor_assessments
		WHERE organization_id = $1 AND status = 'submitted'`, orgID).Scan(&dash.PendingReview)

	var completed, total int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status IN ('submitted', 'reviewed')), COUNT(*)
		FROM vendor_assessments WHERE organization_id = $1`, orgID).Scan(&completed, &total)
	if total > 0 {
		dash.CompletionRate = float64(completed) * 100.0 / float64(total)
	}

	return dash, nil
}

// ValidatePortalToken validates a portal access token by hashing and looking up.
func (s *QuestionnaireService) ValidatePortalToken(ctx context.Context, token string) (*VendorAssessment, error) {
	tokenHash := hashToken(token)

	var a VendorAssessment
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, questionnaire_id, vendor_id, vendor_name,
			contact_email, status, due_date, created_at
		FROM vendor_assessments
		WHERE token_hash = $1 AND status IN ('sent', 'in_progress')`, tokenHash,
	).Scan(
		&a.ID, &a.OrgID, &a.QuestionnaireID, &a.VendorID, &a.VendorName,
		&a.ContactEmail, &a.Status, &a.DueDate, &a.CreatedAt,
	)
	if err != nil {
		return nil, ErrInvalidPortalToken
	}
	return &a, nil
}

// ListAssessments returns paginated assessments for an organization.
func (s *QuestionnaireService) ListAssessments(ctx context.Context, orgID string, page, pageSize int) ([]VendorAssessment, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM vendor_assessments WHERE organization_id = $1`, orgID).Scan(&total)

	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, questionnaire_id, vendor_id, vendor_name,
			contact_email, status, due_date, submitted_at, reviewed_at, reviewed_by,
			review_comments, review_outcome, overall_score, risk_level,
			reminder_count, last_reminder_at, created_at
		FROM vendor_assessments WHERE organization_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list assessments: %w", err)
	}
	defer rows.Close()

	var result []VendorAssessment
	for rows.Next() {
		var a VendorAssessment
		if err := rows.Scan(&a.ID, &a.OrgID, &a.QuestionnaireID, &a.VendorID, &a.VendorName,
			&a.ContactEmail, &a.Status, &a.DueDate, &a.SubmittedAt, &a.ReviewedAt, &a.ReviewedBy,
			&a.ReviewComments, &a.ReviewOutcome, &a.OverallScore, &a.RiskLevel,
			&a.ReminderCount, &a.LastReminderAt, &a.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan assessment: %w", err)
		}
		result = append(result, a)
	}
	return result, total, nil
}

// GetAssessment retrieves a single assessment with answers.
func (s *QuestionnaireService) GetAssessment(ctx context.Context, orgID, assessmentID string) (*VendorAssessment, error) {
	var a VendorAssessment
	var scoresJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, questionnaire_id, vendor_id, vendor_name,
			contact_email, status, due_date, submitted_at, reviewed_at, reviewed_by,
			review_comments, review_outcome, overall_score, section_scores, risk_level,
			reminder_count, last_reminder_at, created_at
		FROM vendor_assessments WHERE id = $1 AND organization_id = $2`,
		assessmentID, orgID,
	).Scan(
		&a.ID, &a.OrgID, &a.QuestionnaireID, &a.VendorID, &a.VendorName,
		&a.ContactEmail, &a.Status, &a.DueDate, &a.SubmittedAt, &a.ReviewedAt, &a.ReviewedBy,
		&a.ReviewComments, &a.ReviewOutcome, &a.OverallScore, &scoresJSON, &a.RiskLevel,
		&a.ReminderCount, &a.LastReminderAt, &a.CreatedAt,
	)
	if err != nil {
		return nil, ErrAssessmentNotFound
	}

	a.SectionScores = make(map[string]float64)
	_ = json.Unmarshal(scoresJSON, &a.SectionScores)

	// Load answers.
	ansRows, err := s.pool.Query(ctx, `
		SELECT aa.question_id, qq.question_text, aa.answer_value, aa.answer_score,
			COALESCE(aa.comments, ''), aa.file_url
		FROM assessment_answers aa
		JOIN questionnaire_questions qq ON aa.question_id = qq.id
		WHERE aa.assessment_id = $1
		ORDER BY qq.sort_order`, assessmentID)
	if err == nil {
		defer ansRows.Close()
		for ansRows.Next() {
			var ans AssessmentAnswer
			ansRows.Scan(&ans.QuestionID, &ans.QuestionText, &ans.AnswerValue,
				&ans.AnswerScore, &ans.Comments, &ans.FileURL)
			a.Answers = append(a.Answers, ans)
		}
	}

	return &a, nil
}
