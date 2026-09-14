package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

// RiskRepository is the canonical persistence contract for migrations 000010
// and 000011. Every operation carries an explicit organization ID in addition
// to running on the request-scoped RLS connection.
type RiskRepository interface {
	Create(context.Context, string, models.RiskCreateInput) (*models.Risk, error)
	GetByID(context.Context, string, string) (*models.Risk, error)
	Update(context.Context, string, *models.Risk) (*models.Risk, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, models.RiskListFilter) ([]models.Risk, int, error)
	ListCategories(context.Context, string) ([]models.RiskCategory, error)
	GetTopRisks(context.Context, string, int) ([]models.Risk, error)
	CountByRiskLevel(context.Context, string) (map[models.RiskLevel]int, error)
	GetRiskMatrix(context.Context, string, string) (*models.RiskMatrixView, error)
	GetRiskHeatmap(context.Context, string) ([]models.RiskHeatmapEntry, error)
	CreateAssessment(context.Context, string, string, string, models.RiskAssessmentInput, *float64, *string, *float64, *string) (*models.RiskAssessment, error)
	ListAssessments(context.Context, string, string, models.PaginationRequest) ([]models.RiskAssessment, int, error)
	CreateTreatment(context.Context, string, string, models.RiskTreatmentInput) (*models.RiskTreatment, error)
	GetTreatment(context.Context, string, string, string) (*models.RiskTreatment, error)
	UpdateTreatment(context.Context, string, string, *models.RiskTreatment) (*models.RiskTreatment, error)
	ListTreatments(context.Context, string, string, models.PaginationRequest) ([]models.RiskTreatment, int, error)
	ListAppetite(context.Context, string) ([]models.RiskAppetiteStatement, error)
	UpsertAppetite(context.Context, string, string, string, models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error)
	CreateIndicator(context.Context, string, string, models.RiskIndicatorInput) (*models.RiskIndicator, error)
	ListIndicators(context.Context, string, string) ([]models.RiskIndicator, error)
	RecordIndicatorValue(context.Context, string, string, string, string, models.RiskIndicatorValueInput) (*models.RiskIndicatorValue, error)
	ListIndicatorValues(context.Context, string, string, string, models.PaginationRequest) ([]models.RiskIndicatorValue, int, error)
}

type riskRepo struct{ pool *pgxpool.Pool }

func NewRiskRepository(pool *pgxpool.Pool) RiskRepository { return &riskRepo{pool: pool} }

var _ RiskRepository = (*riskRepo)(nil)

const riskColumns = `
	r.id, r.organization_id, r.risk_ref, r.title, r.description,
	r.risk_category_id, r.risk_source, r.risk_type, r.status,
	r.owner_user_id, r.delegate_user_id, r.business_unit_id, r.risk_matrix_id,
	r.inherent_likelihood, r.inherent_impact, r.inherent_risk_score, r.inherent_risk_level,
	r.residual_likelihood, r.residual_impact, r.residual_risk_score, r.residual_risk_level,
	r.target_likelihood, r.target_impact, r.target_risk_score, r.target_risk_level,
	r.financial_impact_eur, r.impact_description, r.impact_categories,
	r.risk_velocity, r.risk_proximity, r.identified_date, r.last_assessed_date,
	r.next_review_date, r.review_frequency, r.linked_regulations,
	r.linked_control_ids, r.tags, r.attachments, r.is_emerging, r.metadata,
	r.created_at, r.updated_at, r.deleted_at`

type riskRowScanner interface{ Scan(...any) error }

func scanRisk(row riskRowScanner) (*models.Risk, error) {
	risk := &models.Risk{}
	var impactCategories, attachments, metadata []byte
	if err := row.Scan(
		&risk.ID, &risk.OrganizationID, &risk.RiskRef, &risk.Title, &risk.Description,
		&risk.RiskCategoryID, &risk.RiskSource, &risk.RiskType, &risk.Status,
		&risk.OwnerUserID, &risk.DelegateUserID, &risk.BusinessUnitID, &risk.RiskMatrixID,
		&risk.InherentLikelihood, &risk.InherentImpact, &risk.InherentRiskScore, &risk.InherentRiskLevel,
		&risk.ResidualLikelihood, &risk.ResidualImpact, &risk.ResidualRiskScore, &risk.ResidualRiskLevel,
		&risk.TargetLikelihood, &risk.TargetImpact, &risk.TargetRiskScore, &risk.TargetRiskLevel,
		&risk.FinancialImpactEUR, &risk.ImpactDescription, &impactCategories,
		&risk.RiskVelocity, &risk.RiskProximity, &risk.IdentifiedDate, &risk.LastAssessedDate,
		&risk.NextReviewDate, &risk.ReviewFrequency, &risk.LinkedRegulations,
		&risk.LinkedControlIDs, &risk.Tags, &attachments, &risk.IsEmerging, &metadata,
		&risk.CreatedAt, &risk.UpdatedAt, &risk.DeletedAt,
	); err != nil {
		return nil, err
	}
	risk.ImpactCategories = impactCategories
	risk.Attachments = attachments
	risk.Metadata = metadata
	return risk, nil
}

type riskTransactionStarter interface {
	Begin(context.Context) (pgx.Tx, error)
}

func (r *riskRepo) Create(ctx context.Context, orgID string, input models.RiskCreateInput) (*models.Risk, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	starter, ok := q.(riskTransactionStarter)
	if !ok {
		return nil, errors.New("risk creation requires a transactional database executor")
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning risk creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The migration's RSK-NNNN trigger uses MAX()+1. Serialize reference
	// allocation per tenant so concurrent creates cannot collide.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, orgID); err != nil {
		return nil, fmt.Errorf("locking tenant risk reference allocation: %w", err)
	}

	risk, err := scanRisk(tx.QueryRow(ctx, `
		INSERT INTO risks AS r (
			organization_id, risk_ref, title, description, risk_category_id,
			risk_source, risk_type, owner_user_id, delegate_user_id, business_unit_id,
			risk_matrix_id, inherent_likelihood, inherent_impact,
			residual_likelihood, residual_impact, target_likelihood, target_impact,
			financial_impact_eur, impact_description, impact_categories,
			risk_velocity, risk_proximity, identified_date, next_review_date,
			review_frequency, linked_regulations, linked_control_ids, tags,
			attachments, is_emerging, metadata
		)
		SELECT $1::uuid, NULLIF($2, ''), $3, $4, $5::uuid, $6, $7,
			$8::uuid, $9::uuid, $10::uuid, $11::uuid, $12, $13, $14, $15,
			$16, $17, $18, $19, $20::jsonb, $21, $22,
			COALESCE($23::date, CURRENT_DATE), $24::date, COALESCE($25, 'quarterly'),
			$26::text[], $27::uuid[], $28::text[], $29::jsonb, $30, $31::jsonb
		WHERE ($5::uuid IS NULL OR EXISTS (
			SELECT 1 FROM risk_categories c WHERE c.id=$5::uuid
			AND (c.organization_id IS NULL OR c.organization_id=$1::uuid)))
		AND ($8::uuid IS NULL OR EXISTS (
			SELECT 1 FROM users u WHERE u.id=$8::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		AND ($9::uuid IS NULL OR EXISTS (
			SELECT 1 FROM users u WHERE u.id=$9::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		AND ($11::uuid IS NULL OR EXISTS (
			SELECT 1 FROM risk_matrices m WHERE m.id=$11::uuid AND m.organization_id=$1::uuid))
		RETURNING `+riskColumns, orgID, input.RiskRef, input.Title, input.Description,
		input.RiskCategoryID, input.RiskSource, input.RiskType, input.OwnerUserID,
		input.DelegateUserID, input.BusinessUnitID, input.RiskMatrixID,
		input.InherentLikelihood, input.InherentImpact, input.ResidualLikelihood,
		input.ResidualImpact, input.TargetLikelihood, input.TargetImpact,
		input.FinancialImpactEUR, input.ImpactDescription, input.ImpactCategories,
		input.RiskVelocity, input.RiskProximity, input.IdentifiedDate,
		input.NextReviewDate, input.ReviewFrequency, input.LinkedRegulations,
		input.LinkedControlIDs, input.Tags, input.Attachments, input.IsEmerging,
		input.Metadata))
	if err != nil {
		return nil, fmt.Errorf("creating risk: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing risk creation: %w", err)
	}
	return risk, nil
}

func (r *riskRepo) GetByID(ctx context.Context, orgID, id string) (*models.Risk, error) {
	return scanRisk(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `SELECT `+riskColumns+`
		FROM risks r WHERE r.id=$2::uuid AND r.organization_id=$1::uuid AND r.deleted_at IS NULL`, orgID, id))
}

func (r *riskRepo) Update(ctx context.Context, orgID string, risk *models.Risk) (*models.Risk, error) {
	return scanRisk(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		UPDATE risks r SET title=$3, description=$4, risk_category_id=$5::uuid,
			risk_source=$6, risk_type=$7, status=$8, owner_user_id=$9::uuid,
			delegate_user_id=$10::uuid, business_unit_id=$11::uuid, risk_matrix_id=$12::uuid,
			inherent_likelihood=$13, inherent_impact=$14,
			residual_likelihood=$15, residual_impact=$16,
			target_likelihood=$17, target_impact=$18, financial_impact_eur=$19,
			impact_description=$20, impact_categories=$21::jsonb,
			risk_velocity=$22, risk_proximity=$23, next_review_date=$24::date,
			review_frequency=$25, linked_regulations=$26::text[],
			linked_control_ids=$27::uuid[], tags=$28::text[], attachments=$29::jsonb,
			is_emerging=$30, metadata=$31::jsonb
		WHERE r.id=$2::uuid AND r.organization_id=$1::uuid AND r.deleted_at IS NULL
		AND ($5::uuid IS NULL OR EXISTS (SELECT 1 FROM risk_categories c WHERE c.id=$5::uuid AND (c.organization_id IS NULL OR c.organization_id=$1::uuid)))
		AND ($9::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$9::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		AND ($10::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$10::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		AND ($12::uuid IS NULL OR EXISTS (SELECT 1 FROM risk_matrices m WHERE m.id=$12::uuid AND m.organization_id=$1::uuid))
		RETURNING `+riskColumns, orgID, risk.ID, risk.Title, risk.Description,
		risk.RiskCategoryID, risk.RiskSource, risk.RiskType, risk.Status,
		risk.OwnerUserID, risk.DelegateUserID, risk.BusinessUnitID, risk.RiskMatrixID,
		risk.InherentLikelihood, risk.InherentImpact, risk.ResidualLikelihood,
		risk.ResidualImpact, risk.TargetLikelihood, risk.TargetImpact,
		risk.FinancialImpactEUR, risk.ImpactDescription, risk.ImpactCategories,
		risk.RiskVelocity, risk.RiskProximity, risk.NextReviewDate,
		risk.ReviewFrequency, risk.LinkedRegulations, risk.LinkedControlIDs,
		risk.Tags, risk.Attachments, risk.IsEmerging, risk.Metadata))
}

func (r *riskRepo) Delete(ctx context.Context, orgID, id string) error {
	tag, err := database.QuerierFromContext(ctx, r.pool).Exec(ctx, `
		UPDATE risks SET deleted_at=NOW() WHERE id=$2::uuid AND organization_id=$1::uuid AND deleted_at IS NULL`, orgID, id)
	if err != nil {
		return fmt.Errorf("soft-deleting risk: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *riskRepo) List(ctx context.Context, orgID string, filter models.RiskListFilter) ([]models.Risk, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` FROM risks r WHERE r.organization_id=$1::uuid AND r.deleted_at IS NULL
		AND ($2='' OR r.status=$2)
		AND ($3='' OR COALESCE(r.residual_risk_level, r.inherent_risk_level)=$3)
		AND ($4='' OR r.risk_category_id=$4::uuid)
		AND ($5='' OR r.owner_user_id=$5::uuid)
		AND ($6='' OR r.search_vector @@ websearch_to_tsquery('english', $6))
		AND ($7::boolean IS NULL OR r.is_emerging=$7::boolean)`
	args := []any{orgID, filter.Status, filter.Level, filter.CategoryID, filter.OwnerUserID, filter.Search, filter.Emerging}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting risks: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+riskColumns+where+`
		ORDER BY COALESCE(r.residual_risk_score, r.inherent_risk_score) DESC NULLS LAST, r.created_at DESC
		LIMIT $8 OFFSET $9`, append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing risks: %w", err)
	}
	defer rows.Close()
	risks := make([]models.Risk, 0)
	for rows.Next() {
		risk, err := scanRisk(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning risk: %w", err)
		}
		risks = append(risks, *risk)
	}
	return risks, total, rows.Err()
}

func (r *riskRepo) ListCategories(ctx context.Context, orgID string) ([]models.RiskCategory, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `
		SELECT id, organization_id, name, code, description, parent_category_id,
			color_hex, icon, sort_order, is_system_default, created_at, updated_at
		FROM risk_categories
		WHERE organization_id IS NULL OR organization_id=$1::uuid
		ORDER BY sort_order, name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing risk categories: %w", err)
	}
	defer rows.Close()
	items := make([]models.RiskCategory, 0)
	for rows.Next() {
		var item models.RiskCategory
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Code,
			&item.Description, &item.ParentCategoryID, &item.ColorHex, &item.Icon,
			&item.SortOrder, &item.IsSystemDefault, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning risk category: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *riskRepo) GetTopRisks(ctx context.Context, orgID string, limit int) ([]models.Risk, error) {
	if limit < 1 || limit > 100 {
		limit = 10
	}
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT `+riskColumns+`
		FROM risks r WHERE r.organization_id=$1::uuid AND r.deleted_at IS NULL
		ORDER BY COALESCE(r.residual_risk_score, r.inherent_risk_score) DESC NULLS LAST, r.created_at DESC
		LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing top risks: %w", err)
	}
	defer rows.Close()
	items := make([]models.Risk, 0)
	for rows.Next() {
		item, err := scanRisk(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning top risk: %w", err)
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *riskRepo) CountByRiskLevel(ctx context.Context, orgID string) (map[models.RiskLevel]int, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `
		SELECT COALESCE(residual_risk_level, inherent_risk_level), count(*)
		FROM risks WHERE organization_id=$1::uuid AND deleted_at IS NULL
		AND COALESCE(residual_risk_level, inherent_risk_level) IS NOT NULL
		GROUP BY COALESCE(residual_risk_level, inherent_risk_level)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("counting risks by level: %w", err)
	}
	defer rows.Close()
	counts := make(map[models.RiskLevel]int)
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return nil, fmt.Errorf("scanning risk-level count: %w", err)
		}
		counts[legacyRiskLevel(level)] = count
	}
	return counts, rows.Err()
}

func legacyRiskLevel(level string) models.RiskLevel {
	switch level {
	case "critical":
		return models.RiskLevelCritical
	case "high":
		return models.RiskLevelHigh
	case "medium":
		return models.RiskLevelMedium
	case "low":
		return models.RiskLevelLow
	case "very_low":
		return models.RiskLevelVeryLow
	default:
		return models.RiskLevel(level)
	}
}

func (r *riskRepo) GetRiskMatrix(ctx context.Context, orgID, dimension string) (*models.RiskMatrixView, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	matrix := &models.RiskMatrix{}
	var likelihood, impact, levels []byte
	err := q.QueryRow(ctx, `SELECT id, organization_id, name, description, likelihood_scale,
		impact_scale, risk_levels, matrix_size, is_default, created_at, updated_at
		FROM risk_matrices WHERE organization_id=$1::uuid ORDER BY is_default DESC, created_at LIMIT 1`, orgID).Scan(
		&matrix.ID, &matrix.OrganizationID, &matrix.Name, &matrix.Description, &likelihood,
		&impact, &levels, &matrix.MatrixSize, &matrix.IsDefault, &matrix.CreatedAt, &matrix.UpdatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("getting risk matrix configuration: %w", err)
	}
	view := &models.RiskMatrixView{Dimension: dimension, Cells: make([]models.RiskMatrixCell, 0)}
	if err == nil {
		matrix.LikelihoodScale, matrix.ImpactScale, matrix.RiskLevels = likelihood, impact, levels
		view.Configuration = matrix
	}
	likelihoodColumn, impactColumn := "residual_likelihood", "residual_impact"
	if dimension == "inherent" {
		likelihoodColumn, impactColumn = "inherent_likelihood", "inherent_impact"
	} else if dimension == "target" {
		likelihoodColumn, impactColumn = "target_likelihood", "target_impact"
	}
	rows, err := q.Query(ctx, `SELECT `+likelihoodColumn+`, `+impactColumn+`, count(*)
		FROM risks WHERE organization_id=$1::uuid AND deleted_at IS NULL
		AND `+likelihoodColumn+` IS NOT NULL AND `+impactColumn+` IS NOT NULL
		GROUP BY `+likelihoodColumn+`, `+impactColumn+` ORDER BY 1, 2`, orgID)
	if err != nil {
		return nil, fmt.Errorf("aggregating risk matrix: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cell models.RiskMatrixCell
		if err := rows.Scan(&cell.Likelihood, &cell.Impact, &cell.Count); err != nil {
			return nil, fmt.Errorf("scanning risk matrix cell: %w", err)
		}
		view.Cells = append(view.Cells, cell)
	}
	return view, rows.Err()
}

func (r *riskRepo) GetRiskHeatmap(ctx context.Context, orgID string) ([]models.RiskHeatmapEntry, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `
		SELECT id, risk_ref, title, status, inherent_likelihood, inherent_impact,
			inherent_risk_score, inherent_risk_level, residual_likelihood, residual_impact,
			residual_risk_score, residual_risk_level, target_likelihood, target_impact,
			target_risk_score, target_risk_level, owner_user_id, is_emerging
		FROM risks WHERE organization_id=$1::uuid AND deleted_at IS NULL
		ORDER BY COALESCE(residual_risk_score, inherent_risk_score) DESC NULLS LAST`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying risk heatmap: %w", err)
	}
	defer rows.Close()
	entries := make([]models.RiskHeatmapEntry, 0)
	for rows.Next() {
		var entry models.RiskHeatmapEntry
		if err := rows.Scan(&entry.RiskID, &entry.RiskRef, &entry.Title, &entry.Status,
			&entry.InherentLikelihood, &entry.InherentImpact, &entry.InherentRiskScore, &entry.InherentRiskLevel,
			&entry.ResidualLikelihood, &entry.ResidualImpact, &entry.ResidualRiskScore, &entry.ResidualRiskLevel,
			&entry.TargetLikelihood, &entry.TargetImpact, &entry.TargetRiskScore, &entry.TargetRiskLevel,
			&entry.OwnerUserID, &entry.IsEmerging); err != nil {
			return nil, fmt.Errorf("scanning risk heatmap: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (r *riskRepo) CreateAssessment(ctx context.Context, orgID, riskID, userID string, input models.RiskAssessmentInput, scoreBefore *float64, levelBefore *string, scoreAfter *float64, levelAfter *string) (*models.RiskAssessment, error) {
	date := input.AssessmentDate
	assessment := &models.RiskAssessment{}
	err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		WITH target AS MATERIALIZED (
			SELECT id FROM risks WHERE id=$2::uuid AND organization_id=$1::uuid AND deleted_at IS NULL
		), inserted AS (
			INSERT INTO risk_assessments (organization_id, risk_id, assessment_type,
				assessor_user_id, assessment_date, likelihood_before, impact_before,
				score_before, level_before, likelihood_after, impact_after, score_after,
				level_after, assessment_notes, methodology, confidence_level, data_sources)
			SELECT $1::uuid, target.id, $4, $3::uuid, COALESCE($5::date, CURRENT_DATE),
				$6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17::text[] FROM target
			RETURNING *
		), updated AS (
			UPDATE risks SET last_assessed_date=inserted.assessment_date,
				residual_likelihood=COALESCE(inserted.likelihood_after, residual_likelihood),
				residual_impact=COALESCE(inserted.impact_after, residual_impact),
				status=CASE WHEN status='identified' THEN 'assessed' ELSE status END
			FROM inserted WHERE risks.id=inserted.risk_id RETURNING risks.id
		)
		SELECT id, organization_id, risk_id, assessment_type, assessor_user_id,
			assessment_date, likelihood_before, impact_before, score_before, level_before,
			likelihood_after, impact_after, score_after, level_after, assessment_notes,
			methodology, confidence_level, data_sources, created_at FROM inserted`,
		orgID, riskID, userID, input.AssessmentType, date, input.LikelihoodBefore,
		input.ImpactBefore, scoreBefore, levelBefore, input.LikelihoodAfter,
		input.ImpactAfter, scoreAfter, levelAfter, input.AssessmentNotes,
		input.Methodology, input.ConfidenceLevel, input.DataSources).Scan(
		&assessment.ID, &assessment.OrganizationID, &assessment.RiskID,
		&assessment.AssessmentType, &assessment.AssessorUserID, &assessment.AssessmentDate,
		&assessment.LikelihoodBefore, &assessment.ImpactBefore, &assessment.ScoreBefore,
		&assessment.LevelBefore, &assessment.LikelihoodAfter, &assessment.ImpactAfter,
		&assessment.ScoreAfter, &assessment.LevelAfter, &assessment.AssessmentNotes,
		&assessment.Methodology, &assessment.ConfidenceLevel, &assessment.DataSources,
		&assessment.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating risk assessment: %w", err)
	}
	return assessment, nil
}

func (r *riskRepo) ListAssessments(ctx context.Context, orgID, riskID string, p models.PaginationRequest) ([]models.RiskAssessment, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM risk_assessments a JOIN risks r ON r.id=a.risk_id
		WHERE a.organization_id=$1::uuid AND a.risk_id=$2::uuid AND r.deleted_at IS NULL`, orgID, riskID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting risk assessments: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT a.id, a.organization_id, a.risk_id, a.assessment_type,
		a.assessor_user_id, a.assessment_date, a.likelihood_before, a.impact_before,
		a.score_before, a.level_before, a.likelihood_after, a.impact_after, a.score_after,
		a.level_after, a.assessment_notes, a.methodology, a.confidence_level,
		a.data_sources, a.created_at FROM risk_assessments a JOIN risks r ON r.id=a.risk_id
		WHERE a.organization_id=$1::uuid AND a.risk_id=$2::uuid AND r.deleted_at IS NULL
		ORDER BY a.assessment_date DESC, a.created_at DESC LIMIT $3 OFFSET $4`, orgID, riskID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing risk assessments: %w", err)
	}
	defer rows.Close()
	items := make([]models.RiskAssessment, 0)
	for rows.Next() {
		var a models.RiskAssessment
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.RiskID, &a.AssessmentType,
			&a.AssessorUserID, &a.AssessmentDate, &a.LikelihoodBefore, &a.ImpactBefore,
			&a.ScoreBefore, &a.LevelBefore, &a.LikelihoodAfter, &a.ImpactAfter,
			&a.ScoreAfter, &a.LevelAfter, &a.AssessmentNotes, &a.Methodology,
			&a.ConfidenceLevel, &a.DataSources, &a.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning risk assessment: %w", err)
		}
		items = append(items, a)
	}
	return items, total, rows.Err()
}

const treatmentColumns = `t.id, t.organization_id, t.risk_id, t.treatment_type, t.title,
	t.description, t.status, t.priority, t.owner_user_id, t.start_date, t.target_date,
	t.completed_date, t.estimated_cost_eur, t.actual_cost_eur, t.expected_risk_reduction,
	t.progress_percentage, t.linked_control_ids, t.notes, t.created_at, t.updated_at`

func scanTreatment(row riskRowScanner) (*models.RiskTreatment, error) {
	t := &models.RiskTreatment{}
	err := row.Scan(&t.ID, &t.OrganizationID, &t.RiskID, &t.TreatmentType, &t.Title,
		&t.Description, &t.Status, &t.Priority, &t.OwnerUserID, &t.StartDate, &t.TargetDate,
		&t.CompletedDate, &t.EstimatedCostEUR, &t.ActualCostEUR, &t.ExpectedRiskReduction,
		&t.ProgressPercentage, &t.LinkedControlIDs, &t.Notes, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *riskRepo) CreateTreatment(ctx context.Context, orgID, riskID string, input models.RiskTreatmentInput) (*models.RiskTreatment, error) {
	return scanTreatment(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO risk_treatments AS t (organization_id, risk_id, treatment_type, title,
			description, priority, owner_user_id, start_date, target_date,
			estimated_cost_eur, expected_risk_reduction, linked_control_ids, notes)
		SELECT $1::uuid, r.id, $3, $4, $5, $6, $7::uuid, $8::date, $9::date,
			$10, $11, $12::uuid[], $13 FROM risks r
		WHERE r.id=$2::uuid AND r.organization_id=$1::uuid AND r.deleted_at IS NULL
		AND ($7::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$7::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		RETURNING `+treatmentColumns, orgID, riskID, input.TreatmentType, input.Title,
		input.Description, input.Priority, input.OwnerUserID, input.StartDate,
		input.TargetDate, input.EstimatedCostEUR, input.ExpectedRiskReduction,
		input.LinkedControlIDs, input.Notes))
}

func (r *riskRepo) GetTreatment(ctx context.Context, orgID, riskID, treatmentID string) (*models.RiskTreatment, error) {
	return scanTreatment(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `SELECT `+treatmentColumns+`
		FROM risk_treatments t JOIN risks r ON r.id=t.risk_id
		WHERE t.id=$3::uuid AND t.risk_id=$2::uuid AND t.organization_id=$1::uuid AND r.deleted_at IS NULL`, orgID, riskID, treatmentID))
}

func (r *riskRepo) UpdateTreatment(ctx context.Context, orgID, riskID string, t *models.RiskTreatment) (*models.RiskTreatment, error) {
	return scanTreatment(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		UPDATE risk_treatments t SET status=$4, priority=$5, owner_user_id=$6::uuid,
			start_date=$7::date, target_date=$8::date, completed_date=$9::date,
			actual_cost_eur=$10, progress_percentage=$11, notes=$12
		FROM risks r WHERE t.id=$3::uuid AND t.risk_id=$2::uuid
		AND t.organization_id=$1::uuid AND r.id=t.risk_id AND r.deleted_at IS NULL
		AND ($6::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$6::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		RETURNING `+treatmentColumns, orgID, riskID, t.ID, t.Status, t.Priority,
		t.OwnerUserID, t.StartDate, t.TargetDate, t.CompletedDate, t.ActualCostEUR,
		t.ProgressPercentage, t.Notes))
}

func (r *riskRepo) ListTreatments(ctx context.Context, orgID, riskID string, p models.PaginationRequest) ([]models.RiskTreatment, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM risk_treatments t JOIN risks r ON r.id=t.risk_id
		WHERE t.organization_id=$1::uuid AND t.risk_id=$2::uuid AND r.deleted_at IS NULL`, orgID, riskID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting risk treatments: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+treatmentColumns+` FROM risk_treatments t JOIN risks r ON r.id=t.risk_id
		WHERE t.organization_id=$1::uuid AND t.risk_id=$2::uuid AND r.deleted_at IS NULL
		ORDER BY t.created_at DESC LIMIT $3 OFFSET $4`, orgID, riskID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing risk treatments: %w", err)
	}
	defer rows.Close()
	items := make([]models.RiskTreatment, 0)
	for rows.Next() {
		item, err := scanTreatment(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning risk treatment: %w", err)
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *riskRepo) ListAppetite(ctx context.Context, orgID string) ([]models.RiskAppetiteStatement, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT id, organization_id,
		risk_category_id, appetite_level, appetite_description, quantitative_threshold_low,
		quantitative_threshold_high, threshold_metric, tolerance_level, approved_by,
		approved_at, review_date, status, created_at, updated_at
		FROM risk_appetite_statements WHERE organization_id=$1::uuid ORDER BY risk_category_id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing risk appetite: %w", err)
	}
	defer rows.Close()
	items := make([]models.RiskAppetiteStatement, 0)
	for rows.Next() {
		var a models.RiskAppetiteStatement
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.RiskCategoryID, &a.AppetiteLevel,
			&a.AppetiteDescription, &a.QuantitativeThresholdLow, &a.QuantitativeThresholdHigh,
			&a.ThresholdMetric, &a.ToleranceLevel, &a.ApprovedBy, &a.ApprovedAt,
			&a.ReviewDate, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning risk appetite: %w", err)
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *riskRepo) UpsertAppetite(ctx context.Context, orgID, categoryID, userID string, input models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error) {
	a := &models.RiskAppetiteStatement{}
	err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO risk_appetite_statements (organization_id, risk_category_id,
			appetite_level, appetite_description, quantitative_threshold_low,
			quantitative_threshold_high, threshold_metric, tolerance_level,
			approved_by, approved_at, review_date, status)
		SELECT $1::uuid, c.id, $4, $5, $6, $7, $8, $9,
			CASE WHEN $11='approved' THEN $3::uuid ELSE NULL END,
			CASE WHEN $11='approved' THEN NOW() ELSE NULL END, $10::date, $11
		FROM risk_categories c WHERE c.id=$2::uuid
		AND (c.organization_id IS NULL OR c.organization_id=$1::uuid)
		ON CONFLICT (organization_id, risk_category_id) DO UPDATE SET
			appetite_level=EXCLUDED.appetite_level,
			appetite_description=EXCLUDED.appetite_description,
			quantitative_threshold_low=EXCLUDED.quantitative_threshold_low,
			quantitative_threshold_high=EXCLUDED.quantitative_threshold_high,
			threshold_metric=EXCLUDED.threshold_metric,
			tolerance_level=EXCLUDED.tolerance_level,
			approved_by=EXCLUDED.approved_by, approved_at=EXCLUDED.approved_at,
			review_date=EXCLUDED.review_date, status=EXCLUDED.status
		RETURNING id, organization_id, risk_category_id, appetite_level,
			appetite_description, quantitative_threshold_low, quantitative_threshold_high,
			threshold_metric, tolerance_level, approved_by, approved_at, review_date,
			status, created_at, updated_at`, orgID, categoryID, userID,
		input.AppetiteLevel, input.AppetiteDescription, input.QuantitativeThresholdLow,
		input.QuantitativeThresholdHigh, input.ThresholdMetric, input.ToleranceLevel,
		input.ReviewDate, input.Status).Scan(&a.ID, &a.OrganizationID, &a.RiskCategoryID,
		&a.AppetiteLevel, &a.AppetiteDescription, &a.QuantitativeThresholdLow,
		&a.QuantitativeThresholdHigh, &a.ThresholdMetric, &a.ToleranceLevel,
		&a.ApprovedBy, &a.ApprovedAt, &a.ReviewDate, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting risk appetite: %w", err)
	}
	return a, nil
}

const indicatorColumns = `i.id, i.organization_id, i.risk_id, i.name, i.description,
	i.metric_type, i.measurement_unit, i.collection_frequency, i.data_source,
	i.threshold_green, i.threshold_amber, i.threshold_red, i.current_value, i.trend,
	i.owner_user_id, i.last_updated_at, i.is_automated, i.automation_config,
	i.created_at, i.updated_at`

func scanIndicator(row riskRowScanner) (*models.RiskIndicator, error) {
	i := &models.RiskIndicator{}
	var config []byte
	err := row.Scan(&i.ID, &i.OrganizationID, &i.RiskID, &i.Name, &i.Description,
		&i.MetricType, &i.MeasurementUnit, &i.CollectionFrequency, &i.DataSource,
		&i.ThresholdGreen, &i.ThresholdAmber, &i.ThresholdRed, &i.CurrentValue,
		&i.Trend, &i.OwnerUserID, &i.LastUpdatedAt, &i.IsAutomated, &config,
		&i.CreatedAt, &i.UpdatedAt)
	i.AutomationConfig = config
	return i, err
}

func (r *riskRepo) CreateIndicator(ctx context.Context, orgID, riskID string, input models.RiskIndicatorInput) (*models.RiskIndicator, error) {
	return scanIndicator(database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO risk_indicators AS i (organization_id, risk_id, name, description,
			metric_type, measurement_unit, collection_frequency, data_source,
			threshold_green, threshold_amber, threshold_red, owner_user_id,
			is_automated, automation_config)
		SELECT $1::uuid, r.id, $3, $4, $5, $6, COALESCE(NULLIF($7,''),'monthly'),
			$8, $9, $10, $11, $12::uuid, $13, $14::jsonb FROM risks r
		WHERE r.id=$2::uuid AND r.organization_id=$1::uuid AND r.deleted_at IS NULL
		AND ($12::uuid IS NULL OR EXISTS (SELECT 1 FROM users u WHERE u.id=$12::uuid AND u.organization_id=$1::uuid AND u.deleted_at IS NULL))
		RETURNING `+indicatorColumns, orgID, riskID, input.Name, input.Description,
		input.MetricType, input.MeasurementUnit, input.CollectionFrequency,
		input.DataSource, input.ThresholdGreen, input.ThresholdAmber,
		input.ThresholdRed, input.OwnerUserID, input.IsAutomated,
		input.AutomationConfig))
}

func (r *riskRepo) ListIndicators(ctx context.Context, orgID, riskID string) ([]models.RiskIndicator, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT `+indicatorColumns+`
		FROM risk_indicators i JOIN risks r ON r.id=i.risk_id
		WHERE i.organization_id=$1::uuid AND i.risk_id=$2::uuid AND r.deleted_at IS NULL
		ORDER BY i.name`, orgID, riskID)
	if err != nil {
		return nil, fmt.Errorf("listing risk indicators: %w", err)
	}
	defer rows.Close()
	items := make([]models.RiskIndicator, 0)
	for rows.Next() {
		item, err := scanIndicator(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning risk indicator: %w", err)
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *riskRepo) RecordIndicatorValue(ctx context.Context, orgID, riskID, indicatorID, userID string, input models.RiskIndicatorValueInput) (*models.RiskIndicatorValue, error) {
	item := &models.RiskIndicatorValue{}
	err := database.QuerierFromContext(ctx, r.pool).QueryRow(ctx, `
		WITH indicator AS MATERIALIZED (
			SELECT i.* FROM risk_indicators i JOIN risks r ON r.id=i.risk_id
			WHERE i.id=$3::uuid AND i.risk_id=$2::uuid AND i.organization_id=$1::uuid AND r.deleted_at IS NULL
		), inserted AS (
			INSERT INTO risk_indicator_values (indicator_id, organization_id, value,
				status, measured_at, measured_by, notes)
			SELECT id, organization_id, $5,
				CASE WHEN threshold_red IS NOT NULL AND $5>=threshold_red THEN 'red'
					WHEN threshold_amber IS NOT NULL AND $5>=threshold_amber THEN 'amber'
					ELSE 'green' END,
				COALESCE($6::timestamptz, NOW()), $4::uuid, $7 FROM indicator RETURNING *
		), updated AS (
			UPDATE risk_indicators i SET current_value=inserted.value,
				last_updated_at=inserted.measured_at,
				trend=CASE WHEN i.current_value IS NULL OR i.current_value=inserted.value THEN 'stable'
					WHEN inserted.value<i.current_value THEN 'improving' ELSE 'deteriorating' END
			FROM inserted WHERE i.id=inserted.indicator_id RETURNING i.id
		)
		SELECT id, indicator_id, organization_id, value, status, measured_at,
			measured_by, notes, created_at FROM inserted`, orgID, riskID, indicatorID,
		userID, input.Value, input.MeasuredAt, input.Notes).Scan(&item.ID,
		&item.IndicatorID, &item.OrganizationID, &item.Value, &item.Status,
		&item.MeasuredAt, &item.MeasuredBy, &item.Notes, &item.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("recording risk indicator value: %w", err)
	}
	return item, nil
}

func (r *riskRepo) ListIndicatorValues(ctx context.Context, orgID, riskID, indicatorID string, p models.PaginationRequest) ([]models.RiskIndicatorValue, int, error) {
	q := database.QuerierFromContext(ctx, r.pool)
	where := ` FROM risk_indicator_values v JOIN risk_indicators i ON i.id=v.indicator_id JOIN risks r ON r.id=i.risk_id
		WHERE v.organization_id=$1::uuid AND i.risk_id=$2::uuid AND v.indicator_id=$3::uuid AND r.deleted_at IS NULL`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, orgID, riskID, indicatorID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting indicator values: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT v.id, v.indicator_id, v.organization_id, v.value,
		v.status, v.measured_at, v.measured_by, v.notes, v.created_at`+where+`
		ORDER BY v.measured_at DESC LIMIT $4 OFFSET $5`, orgID, riskID, indicatorID, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("listing indicator values: %w", err)
	}
	defer rows.Close()
	items := make([]models.RiskIndicatorValue, 0)
	for rows.Next() {
		var item models.RiskIndicatorValue
		if err := rows.Scan(&item.ID, &item.IndicatorID, &item.OrganizationID,
			&item.Value, &item.Status, &item.MeasuredAt, &item.MeasuredBy,
			&item.Notes, &item.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning indicator value: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
