package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

var (
	ErrVendorVersionConflict = errors.New("vendor version conflict")
	ErrVendorUserInvalid     = errors.New("vendor user is not an active member of the tenant")
)

type VendorRepository interface {
	Create(context.Context, string, string, models.VendorCreateInput) (*models.Vendor, error)
	GetByID(context.Context, string, string) (*models.Vendor, error)
	Update(context.Context, string, string, string, models.VendorPatch) (*models.Vendor, error)
	Delete(context.Context, string, string, string, int64) error
	List(context.Context, string, models.VendorListFilter) ([]models.Vendor, int, error)
	Transition(context.Context, string, string, string, models.VendorTransitionInput) (*models.Vendor, error)
	RecordAssessment(context.Context, string, string, string, models.VendorAssessmentInput) (*models.Vendor, error)
	Statistics(context.Context, string) (*models.VendorStatistics, error)
	ListDueForAssessment(context.Context, string, time.Time, int) ([]models.Vendor, error)
	ListDueContracts(context.Context, string, time.Time, int) ([]models.VendorDueContract, error)
	ListExpiringCertifications(context.Context, string, time.Time, int) ([]models.VendorExpiringCertification, error)
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.VendorEvent, int, error)
	SaveContact(context.Context, string, string, string, string, models.VendorContactInput) (*models.VendorContact, *models.Vendor, error)
	DeleteContact(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveContract(context.Context, string, string, string, string, models.VendorContractInput) (*models.VendorContract, *models.Vendor, error)
	DeleteContract(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveCertification(context.Context, string, string, string, string, models.VendorCertificationInput) (*models.VendorCertification, *models.Vendor, error)
	DeleteCertification(context.Context, string, string, string, string, int64) (*models.Vendor, error)
	SaveSubprocessor(context.Context, string, string, string, string, models.VendorSubprocessorInput) (*models.VendorSubprocessor, *models.Vendor, error)
	DeleteSubprocessor(context.Context, string, string, string, string, int64) (*models.Vendor, error)
}

type vendorRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

func NewVendorRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string) (VendorRepository, error) {
	if pool == nil {
		return nil, errors.New("vendor repository database pool is required")
	}
	if outbox == nil {
		return nil, errors.New("vendor repository outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("vendor repository outbox queue is required")
	}
	return &vendorRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue}, nil
}

var _ VendorRepository = (*vendorRepo)(nil)

const vendorSelectColumns = `
	v.id,v.organization_id,v.vendor_ref,v.name,COALESCE(v.legal_name,''),
	COALESCE(v.description,''),COALESCE(v.website,''),COALESCE(v.industry,''),
	COALESCE(v.category,''),COALESCE(v.country_code::text,''),v.owner_user_id,
	v.criticality,v.vendor_tier,v.risk_tier,v.risk_score,v.status,
	COALESCE(v.service_description,''),v.services,v.data_processing,v.data_categories,
	v.processing_locations,v.dpa_required,v.dpa_status,v.dpa_in_place,
	COALESCE(v.dpa_reference,''),v.dpa_signed_date,v.dpa_expiry_date,
	v.assessment_frequency,v.assessment_cadence_days,v.assessment_status,
	v.last_assessment_date,v.next_assessment_date,v.next_review_date,
	v.onboarding_started_at,v.onboarded_at,v.suspended_at,v.offboarding_started_at,
	v.offboarded_at,v.rejected_at,v.retention_until,v.legal_hold,v.version,v.metadata,
	v.created_by,v.created_at,v.updated_at,v.deleted_at,
	owner.id,COALESCE(owner.first_name,''),COALESCE(owner.last_name,''),COALESCE(owner.email,''),
	COALESCE(primary_contact.name,''),COALESCE(primary_contact.email,''),COALESCE(primary_contact.phone,''),
	primary_contract.start_date,primary_contract.end_date,primary_contract.value_amount`

const vendorJoins = `
	LEFT JOIN users AS owner
	  ON owner.organization_id=v.organization_id AND owner.id=v.owner_user_id AND owner.deleted_at IS NULL
	LEFT JOIN LATERAL (
		SELECT c.name,c.email,c.phone FROM vendor_contacts AS c
		WHERE c.organization_id=v.organization_id AND c.vendor_id=v.id
		  AND c.deleted_at IS NULL
		ORDER BY c.is_primary DESC,c.created_at,c.id LIMIT 1
	) AS primary_contact ON true
	LEFT JOIN LATERAL (
		SELECT c.start_date,c.end_date,c.value_amount FROM vendor_contracts AS c
		WHERE c.organization_id=v.organization_id AND c.vendor_id=v.id
		  AND c.deleted_at IS NULL AND c.status IN ('active','renewal_due')
		ORDER BY c.end_date NULLS LAST,c.created_at,c.id LIMIT 1
	) AS primary_contract ON true`

func (r *vendorRepo) Create(ctx context.Context, organizationID, actorID string, input models.VendorCreateInput) (*models.Vendor, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var created *models.Vendor
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if input.OwnerUserID != nil {
			if err := ensureVendorUser(ctx, tx, organizationID, *input.OwnerUserID); err != nil {
				return err
			}
		}
		var sequence int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO vendor_reference_sequences (organization_id,next_value)
			VALUES ($1::uuid,2)
			ON CONFLICT (organization_id)
			DO UPDATE SET next_value=vendor_reference_sequences.next_value+1
			RETURNING next_value-1`, organizationID).Scan(&sequence); err != nil {
			return fmt.Errorf("allocate vendor reference: %w", err)
		}
		var vendorID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO vendors (
				organization_id,vendor_ref,name,legal_name,description,website,industry,category,country_code,
				owner_user_id,criticality,vendor_tier,risk_tier,risk_score,service_description,services,
				data_processing,data_categories,processing_locations,dpa_required,dpa_status,dpa_in_place,
				dpa_reference,dpa_signed_date,dpa_expiry_date,assessment_frequency,assessment_cadence_days,
				next_assessment_date,next_review_date,retention_until,metadata,created_by
			) VALUES (
				$1::uuid,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),
				$10::uuid,$11,$12,$13,$14,NULLIF($15,''),$16,$17,$18,$19,$20,$21,$22,
				NULLIF($23,''),$24,$25,$26,$27,$28,$29,$30,COALESCE($31::jsonb,'{}'::jsonb),$32::uuid
			) RETURNING id`,
			organizationID, fmt.Sprintf("VND-%06d", sequence), input.Name, input.LegalName,
			input.Description, input.Website, input.Industry, input.Category, input.CountryCode,
			input.OwnerUserID, input.Criticality, input.VendorTier, input.RiskTier, input.RiskScore,
			input.ServiceDescription, input.Services, input.DataProcessing, input.DataCategories,
			input.ProcessingLocations, input.DPARequired, input.DPAStatus,
			input.DPAStatus == models.VendorDPAExecuted, input.DPAReference, input.DPASignedDate,
			input.DPAExpiryDate, input.AssessmentFrequency, input.AssessmentCadenceDays,
			input.NextAssessmentDate, input.NextReviewDate, input.RetentionUntil,
			nullableJSON(input.Metadata), actorID,
		).Scan(&vendorID); err != nil {
			return fmt.Errorf("insert vendor: %w", err)
		}
		if input.ContactName != "" || input.ContactEmail != "" {
			if _, err := insertVendorContact(ctx, tx, organizationID, vendorID, actorID, models.VendorContactInput{
				Name: input.ContactName, Email: input.ContactEmail, Phone: input.ContactPhone,
				ContactType: "business", IsPrimary: true,
			}); err != nil {
				return err
			}
		}
		for _, name := range input.Certifications {
			if _, err := insertVendorCertification(ctx, tx, organizationID, vendorID, actorID, models.VendorCertificationInput{Name: name, Status: "active"}); err != nil {
				return err
			}
		}
		for _, contract := range input.InitialContracts {
			if _, err := insertVendorContract(ctx, tx, organizationID, vendorID, actorID, contract); err != nil {
				return err
			}
		}
		for _, subprocessor := range input.InitialSubProcessors {
			if _, err := insertVendorSubprocessor(ctx, tx, organizationID, vendorID, actorID, subprocessor); err != nil {
				return err
			}
		}
		var err error
		created, err = getVendorWithQuerier(ctx, tx, organizationID, vendorID, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, created, actorID, "created", "Vendor created", map[string]any{
			"status": created.Status, "criticality": created.Criticality, "risk_tier": created.RiskTier,
		})
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *vendorRepo) GetByID(ctx context.Context, organizationID, id string) (*models.Vendor, error) {
	return getVendorWithQuerier(ctx, database.QuerierFromContext(ctx, r.pool), organizationID, id, true)
}

func getVendorWithQuerier(ctx context.Context, querier database.Querier, organizationID, id string, related bool) (*models.Vendor, error) {
	item, err := scanVendor(querier.QueryRow(ctx, `
		SELECT `+vendorSelectColumns+` FROM vendors AS v `+vendorJoins+`
		WHERE v.organization_id=$1::uuid AND v.id=$2::uuid AND v.deleted_at IS NULL`, organizationID, id))
	if err != nil {
		return nil, err
	}
	if related {
		if err := loadVendorRelated(ctx, querier, item); err != nil {
			return nil, err
		}
	}
	return item, nil
}

func (r *vendorRepo) Update(ctx context.Context, organizationID, actorID, id string, patch models.VendorPatch) (*models.Vendor, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var updated *models.Vendor
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if patch.OwnerUserID != nil && !patch.ClearOwner {
			if err := ensureVendorUser(ctx, tx, organizationID, *patch.OwnerUserID); err != nil {
				return err
			}
		}
		if _, _, err := lockVendor(ctx, tx, organizationID, id, patch.Version); err != nil {
			return err
		}
		sets := []string{"version=version+1"}
		args := []any{organizationID, id, patch.Version}
		add := func(column, expression string, value any) {
			args = append(args, value)
			sets = append(sets, column+"="+fmt.Sprintf(expression, len(args)))
		}
		if patch.Name != nil {
			add("name", "$%d", *patch.Name)
		}
		if patch.LegalName != nil {
			add("legal_name", "NULLIF($%d,'')", *patch.LegalName)
		}
		if patch.Description != nil {
			add("description", "NULLIF($%d,'')", *patch.Description)
		}
		if patch.Website != nil {
			add("website", "NULLIF($%d,'')", *patch.Website)
		}
		if patch.Industry != nil {
			add("industry", "NULLIF($%d,'')", *patch.Industry)
		}
		if patch.Category != nil {
			add("category", "NULLIF($%d,'')", *patch.Category)
		}
		if patch.CountryCode != nil {
			add("country_code", "NULLIF($%d,'')", *patch.CountryCode)
		}
		if patch.ClearOwner {
			sets = append(sets, "owner_user_id=NULL")
		} else if patch.OwnerUserID != nil {
			add("owner_user_id", "$%d::uuid", *patch.OwnerUserID)
		}
		if patch.Criticality != nil {
			add("criticality", "$%d", *patch.Criticality)
		}
		if patch.VendorTier != nil {
			add("vendor_tier", "$%d", *patch.VendorTier)
		}
		if patch.RiskTier != nil {
			add("risk_tier", "$%d", *patch.RiskTier)
		}
		if patch.ClearRiskScore {
			sets = append(sets, "risk_score=NULL")
		} else if patch.RiskScore != nil {
			add("risk_score", "$%d", *patch.RiskScore)
		}
		if patch.ServiceDescription != nil {
			add("service_description", "NULLIF($%d,'')", *patch.ServiceDescription)
		}
		if patch.Services != nil {
			add("services", "$%d", *patch.Services)
		}
		if patch.DataProcessing != nil {
			add("data_processing", "$%d", *patch.DataProcessing)
		}
		if patch.DataCategories != nil {
			add("data_categories", "$%d", *patch.DataCategories)
		}
		if patch.ProcessingLocations != nil {
			add("processing_locations", "$%d", *patch.ProcessingLocations)
		}
		if patch.DPARequired != nil {
			add("dpa_required", "$%d", *patch.DPARequired)
		}
		if patch.ClearDPA {
			sets = append(sets, "dpa_status='not_required'", "dpa_in_place=false", "dpa_reference=NULL", "dpa_signed_date=NULL", "dpa_expiry_date=NULL")
		} else {
			if patch.DPAStatus != nil {
				add("dpa_status", "$%d", *patch.DPAStatus)
				add("dpa_in_place", "$%d", *patch.DPAStatus == models.VendorDPAExecuted)
			}
			if patch.DPAReference != nil {
				add("dpa_reference", "NULLIF($%d,'')", *patch.DPAReference)
			}
			if patch.DPASignedDate != nil {
				add("dpa_signed_date", "$%d", *patch.DPASignedDate)
			}
			if patch.DPAExpiryDate != nil {
				add("dpa_expiry_date", "$%d", *patch.DPAExpiryDate)
			}
		}
		if patch.AssessmentFrequency != nil {
			add("assessment_frequency", "$%d", *patch.AssessmentFrequency)
		}
		if patch.AssessmentCadenceDays != nil {
			add("assessment_cadence_days", "$%d", *patch.AssessmentCadenceDays)
		}
		if patch.ClearNextAssessment {
			sets = append(sets, "next_assessment_date=NULL")
		} else if patch.NextAssessmentDate != nil {
			add("next_assessment_date", "$%d", *patch.NextAssessmentDate)
		}
		if patch.ClearNextReview {
			sets = append(sets, "next_review_date=NULL")
		} else if patch.NextReviewDate != nil {
			add("next_review_date", "$%d", *patch.NextReviewDate)
		}
		if patch.RetentionUntil != nil {
			add("retention_until", "$%d", *patch.RetentionUntil)
		}
		if patch.LegalHold != nil {
			add("legal_hold", "$%d", *patch.LegalHold)
		}
		if len(patch.Metadata) > 0 {
			add("metadata", "$%d::jsonb", patch.Metadata)
		}
		query := `UPDATE vendors SET ` + strings.Join(sets, ",") + `
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL`
		if tag, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("update vendor: %w", err)
		} else if tag.RowsAffected() != 1 {
			return ErrVendorVersionConflict
		}
		var err error
		updated, err = getVendorWithQuerier(ctx, tx, organizationID, id, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, updated, actorID, "updated", "Vendor profile updated", map[string]any{"version": updated.Version})
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *vendorRepo) Delete(ctx context.Context, organizationID, actorID, id string, version int64) error {
	querier := database.QuerierFromContext(ctx, r.pool)
	return withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if _, _, err := lockVendor(ctx, tx, organizationID, id, version); err != nil {
			return err
		}
		var item models.Vendor
		item.ID, item.OrganizationID = id, organizationID
		if err := tx.QueryRow(ctx, `UPDATE vendors SET deleted_at=NOW(),version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL
			RETURNING vendor_ref,name,criticality,risk_tier,status,version`, organizationID, id, version).Scan(
			&item.VendorRef, &item.Name, &item.Criticality, &item.RiskTier, &item.Status, &item.Version,
		); err != nil {
			return classifyVendorMutation(ctx, tx, organizationID, id, version, err)
		}
		return r.recordVendorEvent(ctx, tx, &item, actorID, "deleted", "Vendor soft-deleted", map[string]any{"version": item.Version})
	})
}

func (r *vendorRepo) List(ctx context.Context, organizationID string, filter models.VendorListFilter) ([]models.Vendor, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	args := []any{organizationID, filter.Status, filter.Criticality, filter.VendorTier, filter.RiskTier,
		filter.OwnerUserID, filter.CountryCode, filter.DataProcessing, filter.DueAssessment, filter.Search}
	predicate := ` FROM vendors AS v WHERE v.organization_id=$1::uuid AND v.deleted_at IS NULL
		AND ($2='' OR v.status=$2) AND ($3='' OR v.criticality=$3)
		AND ($4='' OR v.vendor_tier=$4) AND ($5='' OR v.risk_tier=$5)
		AND ($6='' OR v.owner_user_id=$6::uuid) AND ($7='' OR v.country_code=$7)
		AND ($8::boolean IS NULL OR v.data_processing=$8)
		AND ($9::boolean IS NULL OR (v.next_assessment_date IS NOT NULL AND v.next_assessment_date<=CURRENT_DATE)= $9)
		AND ($10='' OR v.search_vector @@ websearch_to_tsquery('simple',$10))`
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*)`+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count vendors: %w", err)
	}
	direction := "DESC"
	if filter.SortDirection == "asc" {
		direction = "ASC"
	}
	query := `SELECT ` + vendorSelectColumns + ` FROM vendors AS v ` + vendorJoins +
		strings.TrimPrefix(predicate, " FROM vendors AS v") + ` ORDER BY ` + vendorSortColumn(filter.SortBy) + ` ` + direction + `,v.id ` + direction + ` LIMIT $11 OFFSET $12`
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list vendors: %w", err)
	}
	defer rows.Close()
	items := make([]models.Vendor, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanVendor(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan vendor list: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate vendors: %w", err)
	}
	return items, total, nil
}

func (r *vendorRepo) Transition(ctx context.Context, organizationID, actorID, id string, input models.VendorTransitionInput) (*models.Vendor, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var updated *models.Vendor
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		_, previous, err := lockVendor(ctx, tx, organizationID, id, input.Version)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE vendors SET status=$4,version=version+1,
			onboarding_started_at=CASE WHEN $4='onboarding' THEN NOW() ELSE onboarding_started_at END,
			onboarded_at=CASE WHEN $4='active' AND onboarded_at IS NULL THEN NOW() ELSE onboarded_at END,
			suspended_at=CASE WHEN $4='suspended' THEN NOW() ELSE suspended_at END,
			offboarding_started_at=CASE WHEN $4='offboarding' THEN NOW() ELSE offboarding_started_at END,
			offboarded_at=CASE WHEN $4='offboarded' THEN NOW() ELSE offboarded_at END,
			rejected_at=CASE WHEN $4='rejected' THEN NOW() ELSE rejected_at END
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL
			RETURNING version`, organizationID, id, input.Version, input.Status).Scan(new(int64)); err != nil {
			return classifyVendorMutation(ctx, tx, organizationID, id, input.Version, err)
		}
		updated, err = getVendorWithQuerier(ctx, tx, organizationID, id, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, updated, actorID, "status_changed", "Vendor lifecycle changed", map[string]any{
			"from_status": previous, "to_status": input.Status, "reason": input.Reason,
		})
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *vendorRepo) RecordAssessment(ctx context.Context, organizationID, actorID, id string, input models.VendorAssessmentInput) (*models.Vendor, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var updated *models.Vendor
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		if _, _, err := lockVendor(ctx, tx, organizationID, id, input.Version); err != nil {
			return err
		}
		var err error
		if err = tx.QueryRow(ctx, `UPDATE vendors SET assessment_status=$4,risk_tier=$5,risk_score=$6,
			last_assessment_date=$7::date,next_assessment_date=$8::date,version=version+1
			WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL RETURNING version`,
			organizationID, id, input.Version, input.Status, input.RiskTier, input.RiskScore,
			input.AssessedAt, input.NextAssessmentDate).Scan(new(int64)); err != nil {
			return classifyVendorMutation(ctx, tx, organizationID, id, input.Version, err)
		}
		updated, err = getVendorWithQuerier(ctx, tx, organizationID, id, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, updated, actorID, "assessment_recorded", "Vendor assessment recorded", map[string]any{
			"assessment_status": input.Status, "risk_tier": input.RiskTier, "risk_score": input.RiskScore,
			"assessed_at": input.AssessedAt, "next_assessment_date": input.NextAssessmentDate, "notes": input.Notes,
		})
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *vendorRepo) Statistics(ctx context.Context, organizationID string) (*models.VendorStatistics, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	stats := &models.VendorStatistics{ByStatus: map[models.VendorStatus]int{}, ByTier: map[models.VendorTier]int{}, ByCriticality: map[models.VendorCriticality]int{}}
	if err := querier.QueryRow(ctx, `SELECT COUNT(*)::int,COUNT(*) FILTER (WHERE status='active')::int,
		COUNT(*) FILTER (WHERE risk_tier='critical')::int,COUNT(*) FILTER (WHERE risk_tier='high')::int,
		COUNT(*) FILTER (WHERE data_processing AND NOT dpa_in_place)::int,
		COUNT(*) FILTER (WHERE next_assessment_date<=CURRENT_DATE)::int
		FROM vendors WHERE organization_id=$1::uuid AND deleted_at IS NULL`, organizationID).Scan(
		&stats.Total, &stats.Active, &stats.CriticalRisk, &stats.HighRisk, &stats.MissingDPA, &stats.AssessmentsDue,
	); err != nil {
		return nil, fmt.Errorf("aggregate vendor statistics: %w", err)
	}
	if err := querier.QueryRow(ctx, `SELECT
		COUNT(*) FILTER (WHERE COALESCE(renewal_date,end_date)<=CURRENT_DATE+30)::int,
		COALESCE(SUM(value_amount) FILTER (WHERE currency='EUR' AND status IN ('active','renewal_due')),0)::float8
		FROM vendor_contracts WHERE organization_id=$1::uuid AND deleted_at IS NULL`, organizationID).Scan(
		&stats.ContractsExpiring, &stats.TotalContractValueEUR,
	); err != nil {
		return nil, fmt.Errorf("aggregate vendor contract statistics: %w", err)
	}
	if err := querier.QueryRow(ctx, `SELECT COUNT(*)::int FROM vendor_certifications
		WHERE organization_id=$1::uuid AND deleted_at IS NULL AND status IN ('active','pending') AND expires_on<=CURRENT_DATE+30`, organizationID).Scan(&stats.CertificationsExpiring); err != nil {
		return nil, fmt.Errorf("aggregate vendor certification statistics: %w", err)
	}
	rows, err := querier.Query(ctx, `SELECT status,COUNT(*)::int,vendor_tier,criticality FROM vendors
		WHERE organization_id=$1::uuid AND deleted_at IS NULL GROUP BY GROUPING SETS ((status),(vendor_tier),(criticality))`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("aggregate vendor dimensions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status, tier, criticality *string
		var count int
		if err := rows.Scan(&status, &count, &tier, &criticality); err != nil {
			return nil, err
		}
		if status != nil {
			stats.ByStatus[models.VendorStatus(*status)] = count
		}
		if tier != nil {
			stats.ByTier[models.VendorTier(*tier)] = count
		}
		if criticality != nil {
			stats.ByCriticality[models.VendorCriticality(*criticality)] = count
		}
	}
	return stats, rows.Err()
}

func (r *vendorRepo) ListDueForAssessment(ctx context.Context, organizationID string, before time.Time, limit int) ([]models.Vendor, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	rows, err := querier.Query(ctx, `SELECT `+vendorSelectColumns+` FROM vendors AS v `+vendorJoins+`
		WHERE v.organization_id=$1::uuid AND v.deleted_at IS NULL AND v.status IN ('active','suspended')
		AND v.next_assessment_date IS NOT NULL AND v.next_assessment_date<=$2::date
		ORDER BY v.next_assessment_date,v.id LIMIT $3`, organizationID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list vendors due for assessment: %w", err)
	}
	defer rows.Close()
	items := make([]models.Vendor, 0)
	for rows.Next() {
		item, err := scanVendor(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *vendorRepo) ListDueContracts(ctx context.Context, organizationID string, before time.Time, limit int) ([]models.VendorDueContract, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT v.id,v.vendor_ref,v.name,
		c.id,c.organization_id,c.vendor_id,c.contract_ref,c.name,c.status,c.start_date,c.end_date,c.notice_days,
		c.renewal_date,c.auto_renew,c.value_amount,c.currency,c.includes_dpa,c.signed_at,c.owner_user_id,c.metadata,c.created_at,c.updated_at,c.deleted_at,
		COALESCE(c.renewal_date,c.end_date) FROM vendor_contracts c JOIN vendors v ON v.organization_id=c.organization_id AND v.id=c.vendor_id
		WHERE c.organization_id=$1::uuid AND c.deleted_at IS NULL AND v.deleted_at IS NULL
		AND c.status IN ('active','renewal_due') AND COALESCE(c.renewal_date,c.end_date)<=$2::date
		ORDER BY COALESCE(c.renewal_date,c.end_date),c.id LIMIT $3`, organizationID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list due vendor contracts: %w", err)
	}
	defer rows.Close()
	items := make([]models.VendorDueContract, 0)
	for rows.Next() {
		var item models.VendorDueContract
		if err := rows.Scan(&item.VendorID, &item.VendorRef, &item.VendorName,
			&item.Contract.ID, &item.Contract.OrganizationID, &item.Contract.VendorID, &item.Contract.ContractRef, &item.Contract.Name,
			&item.Contract.Status, &item.Contract.StartDate, &item.Contract.EndDate, &item.Contract.NoticeDays, &item.Contract.RenewalDate,
			&item.Contract.AutoRenew, &item.Contract.ValueAmount, &item.Contract.Currency, &item.Contract.IncludesDPA, &item.Contract.SignedAt,
			&item.Contract.OwnerUserID, &item.Contract.Metadata, &item.Contract.CreatedAt, &item.Contract.UpdatedAt, &item.Contract.DeletedAt, &item.DueDate); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *vendorRepo) ListExpiringCertifications(ctx context.Context, organizationID string, before time.Time, limit int) ([]models.VendorExpiringCertification, error) {
	rows, err := database.QuerierFromContext(ctx, r.pool).Query(ctx, `SELECT v.id,v.vendor_ref,v.name,
		c.id,c.organization_id,c.vendor_id,c.name,COALESCE(c.issuer,''),COALESCE(c.certificate_number,''),c.status,c.issued_on,c.expires_on,
		COALESCE(c.evidence_reference,''),c.metadata,c.created_at,c.updated_at,c.deleted_at
		FROM vendor_certifications c JOIN vendors v ON v.organization_id=c.organization_id AND v.id=c.vendor_id
		WHERE c.organization_id=$1::uuid AND c.deleted_at IS NULL AND v.deleted_at IS NULL
		AND c.status IN ('active','pending') AND c.expires_on IS NOT NULL AND c.expires_on<=$2::date
		ORDER BY c.expires_on,c.id LIMIT $3`, organizationID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list expiring vendor certifications: %w", err)
	}
	defer rows.Close()
	items := make([]models.VendorExpiringCertification, 0)
	for rows.Next() {
		var item models.VendorExpiringCertification
		if err := rows.Scan(&item.VendorID, &item.VendorRef, &item.VendorName,
			&item.Certification.ID, &item.Certification.OrganizationID, &item.Certification.VendorID, &item.Certification.Name,
			&item.Certification.Issuer, &item.Certification.CertificateNumber, &item.Certification.Status, &item.Certification.IssuedOn,
			&item.Certification.ExpiresOn, &item.Certification.EvidenceReference, &item.Certification.Metadata,
			&item.Certification.CreatedAt, &item.Certification.UpdatedAt, &item.Certification.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *vendorRepo) ListEvents(ctx context.Context, organizationID, vendorID string, pagination models.PaginationRequest) ([]models.VendorEvent, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*) FROM vendor_events WHERE organization_id=$1::uuid AND vendor_id=$2::uuid`, organizationID, vendorID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := querier.Query(ctx, `SELECT id,organization_id,vendor_id,event_type,actor_user_id,vendor_version,summary,details,created_at
		FROM vendor_events WHERE organization_id=$1::uuid AND vendor_id=$2::uuid ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`,
		organizationID, vendorID, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]models.VendorEvent, 0, pagination.PageSize)
	for rows.Next() {
		var item models.VendorEvent
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.VendorID, &item.EventType, &item.ActorUserID, &item.VendorVersion, &item.Summary, &item.Details, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *vendorRepo) SaveContact(ctx context.Context, orgID, actorID, vendorID, contactID string, input models.VendorContactInput) (*models.VendorContact, *models.Vendor, error) {
	var saved *models.VendorContact
	var vendor *models.Vendor
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if _, _, err := lockVendor(ctx, tx, orgID, vendorID, input.Version); err != nil {
			return err
		}
		if input.IsPrimary {
			if _, err := tx.Exec(ctx, `UPDATE vendor_contacts SET is_primary=false WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND deleted_at IS NULL`, orgID, vendorID); err != nil {
				return err
			}
		}
		var err error
		eventType := "contact_added"
		if contactID == "" {
			saved, err = insertVendorContact(ctx, tx, orgID, vendorID, actorID, input)
		} else {
			eventType = "contact_updated"
			saved, err = updateVendorContact(ctx, tx, orgID, vendorID, contactID, input)
		}
		if err != nil {
			return err
		}
		if err = bumpVendorVersion(ctx, tx, orgID, vendorID, input.Version); err != nil {
			return err
		}
		vendor, err = getVendorWithQuerier(ctx, tx, orgID, vendorID, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, vendor, actorID, eventType, "Vendor contact saved", map[string]any{"contact_id": saved.ID, "contact_type": saved.ContactType})
	})
	return saved, vendor, err
}

func (r *vendorRepo) DeleteContact(ctx context.Context, orgID, actorID, vendorID, contactID string, version int64) (*models.Vendor, error) {
	return r.deleteRelated(ctx, orgID, actorID, vendorID, contactID, version, "vendor_contacts", "contact_removed", "contact")
}

func (r *vendorRepo) SaveContract(ctx context.Context, orgID, actorID, vendorID, contractID string, input models.VendorContractInput) (*models.VendorContract, *models.Vendor, error) {
	var saved *models.VendorContract
	var vendor *models.Vendor
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if _, _, err := lockVendor(ctx, tx, orgID, vendorID, input.Version); err != nil {
			return err
		}
		if input.OwnerUserID != nil {
			if err := ensureVendorUser(ctx, tx, orgID, *input.OwnerUserID); err != nil {
				return err
			}
		}
		var err error
		eventType := "contract_added"
		if contractID == "" {
			saved, err = insertVendorContract(ctx, tx, orgID, vendorID, actorID, input)
		} else {
			eventType = "contract_updated"
			saved, err = updateVendorContract(ctx, tx, orgID, vendorID, contractID, input)
		}
		if err != nil {
			return err
		}
		if err = bumpVendorVersion(ctx, tx, orgID, vendorID, input.Version); err != nil {
			return err
		}
		vendor, err = getVendorWithQuerier(ctx, tx, orgID, vendorID, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, vendor, actorID, eventType, "Vendor contract saved", map[string]any{"contract_id": saved.ID, "contract_ref": saved.ContractRef, "status": saved.Status})
	})
	return saved, vendor, err
}

func (r *vendorRepo) DeleteContract(ctx context.Context, orgID, actorID, vendorID, contractID string, version int64) (*models.Vendor, error) {
	return r.deleteRelated(ctx, orgID, actorID, vendorID, contractID, version, "vendor_contracts", "contract_removed", "contract")
}

func (r *vendorRepo) SaveCertification(ctx context.Context, orgID, actorID, vendorID, certificationID string, input models.VendorCertificationInput) (*models.VendorCertification, *models.Vendor, error) {
	var saved *models.VendorCertification
	var vendor *models.Vendor
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if _, _, err := lockVendor(ctx, tx, orgID, vendorID, input.Version); err != nil {
			return err
		}
		var err error
		eventType := "certification_added"
		if certificationID == "" {
			saved, err = insertVendorCertification(ctx, tx, orgID, vendorID, actorID, input)
		} else {
			eventType = "certification_updated"
			saved, err = updateVendorCertification(ctx, tx, orgID, vendorID, certificationID, input)
		}
		if err != nil {
			return err
		}
		if err = bumpVendorVersion(ctx, tx, orgID, vendorID, input.Version); err != nil {
			return err
		}
		vendor, err = getVendorWithQuerier(ctx, tx, orgID, vendorID, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, vendor, actorID, eventType, "Vendor certification saved", map[string]any{"certification_id": saved.ID, "name": saved.Name, "status": saved.Status})
	})
	return saved, vendor, err
}

func (r *vendorRepo) DeleteCertification(ctx context.Context, orgID, actorID, vendorID, certificationID string, version int64) (*models.Vendor, error) {
	return r.deleteRelated(ctx, orgID, actorID, vendorID, certificationID, version, "vendor_certifications", "certification_removed", "certification")
}

func (r *vendorRepo) SaveSubprocessor(ctx context.Context, orgID, actorID, vendorID, subprocessorID string, input models.VendorSubprocessorInput) (*models.VendorSubprocessor, *models.Vendor, error) {
	var saved *models.VendorSubprocessor
	var vendor *models.Vendor
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if _, _, err := lockVendor(ctx, tx, orgID, vendorID, input.Version); err != nil {
			return err
		}
		var err error
		eventType := "subprocessor_added"
		if subprocessorID == "" {
			saved, err = insertVendorSubprocessor(ctx, tx, orgID, vendorID, actorID, input)
		} else {
			eventType = "subprocessor_updated"
			saved, err = updateVendorSubprocessor(ctx, tx, orgID, vendorID, subprocessorID, actorID, input)
		}
		if err != nil {
			return err
		}
		if err = bumpVendorVersion(ctx, tx, orgID, vendorID, input.Version); err != nil {
			return err
		}
		vendor, err = getVendorWithQuerier(ctx, tx, orgID, vendorID, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, vendor, actorID, eventType, "Vendor subprocessor saved", map[string]any{"subprocessor_id": saved.ID, "name": saved.Name, "status": saved.Status})
	})
	return saved, vendor, err
}

func (r *vendorRepo) DeleteSubprocessor(ctx context.Context, orgID, actorID, vendorID, subprocessorID string, version int64) (*models.Vendor, error) {
	return r.deleteRelated(ctx, orgID, actorID, vendorID, subprocessorID, version, "vendor_subprocessors", "subprocessor_removed", "subprocessor")
}

func (r *vendorRepo) deleteRelated(ctx context.Context, orgID, actorID, vendorID, resourceID string, version int64, table, eventType, kind string) (*models.Vendor, error) {
	var vendor *models.Vendor
	err := withTransaction(ctx, database.QuerierFromContext(ctx, r.pool), func(tx pgx.Tx) error {
		if _, _, err := lockVendor(ctx, tx, orgID, vendorID, version); err != nil {
			return err
		}
		query := `UPDATE ` + table + ` SET deleted_at=NOW() WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND id=$3::uuid AND deleted_at IS NULL`
		tag, err := tx.Exec(ctx, query, orgID, vendorID, resourceID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		if err = bumpVendorVersion(ctx, tx, orgID, vendorID, version); err != nil {
			return err
		}
		vendor, err = getVendorWithQuerier(ctx, tx, orgID, vendorID, true)
		if err != nil {
			return err
		}
		return r.recordVendorEvent(ctx, tx, vendor, actorID, eventType, "Vendor "+kind+" removed", map[string]any{kind + "_id": resourceID})
	})
	return vendor, err
}

type vendorScanner interface{ Scan(...any) error }

func scanVendor(row vendorScanner) (*models.Vendor, error) {
	var item models.Vendor
	var ownerID *string
	var ownerFirst, ownerLast, ownerEmail string
	if err := row.Scan(&item.ID, &item.OrganizationID, &item.VendorRef, &item.Name, &item.LegalName, &item.Description, &item.Website, &item.Industry, &item.Category, &item.CountryCode, &item.OwnerUserID, &item.Criticality, &item.VendorTier, &item.RiskTier, &item.RiskScore, &item.Status, &item.ServiceDescription, &item.Services, &item.DataProcessing, &item.DataCategories, &item.ProcessingLocations, &item.DPARequired, &item.DPAStatus, &item.DPAInPlace, &item.DPAReference, &item.DPASignedDate, &item.DPAExpiryDate, &item.AssessmentFrequency, &item.AssessmentCadenceDays, &item.AssessmentStatus, &item.LastAssessmentDate, &item.NextAssessmentDate, &item.NextReviewDate, &item.OnboardingStartedAt, &item.OnboardedAt, &item.SuspendedAt, &item.OffboardingStartedAt, &item.OffboardedAt, &item.RejectedAt, &item.RetentionUntil, &item.LegalHold, &item.Version, &item.Metadata, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &ownerID, &ownerFirst, &ownerLast, &ownerEmail, &item.ContactName, &item.ContactEmail, &item.ContactPhone, &item.ContractStartDate, &item.ContractEndDate, &item.ContractValueEUR); err != nil {
		return nil, err
	}
	if ownerID != nil {
		item.Owner = &models.VendorPerson{ID: *ownerID, FirstName: ownerFirst, LastName: ownerLast, Email: ownerEmail}
	}
	if item.Services == nil {
		item.Services = []string{}
	}
	if item.DataCategories == nil {
		item.DataCategories = []string{}
	}
	if item.ProcessingLocations == nil {
		item.ProcessingLocations = []string{}
	}
	item.Certifications = []string{}
	return &item, nil
}

func loadVendorRelated(ctx context.Context, q database.Querier, item *models.Vendor) error {
	contacts, err := listVendorContacts(ctx, q, item.OrganizationID, item.ID)
	if err != nil {
		return err
	}
	contracts, err := listVendorContracts(ctx, q, item.OrganizationID, item.ID)
	if err != nil {
		return err
	}
	certs, err := listVendorCertifications(ctx, q, item.OrganizationID, item.ID)
	if err != nil {
		return err
	}
	subs, err := listVendorSubprocessors(ctx, q, item.OrganizationID, item.ID)
	if err != nil {
		return err
	}
	item.Contacts, item.Contracts, item.CertificationDetails, item.SubProcessors = contacts, contracts, certs, subs
	item.Certifications = make([]string, 0, len(certs))
	for _, cert := range certs {
		item.Certifications = append(item.Certifications, cert.Name)
	}
	return nil
}

func listVendorContacts(ctx context.Context, q database.Querier, orgID, vendorID string) ([]models.VendorContact, error) {
	rows, err := q.Query(ctx, `SELECT id,organization_id,vendor_id,name,email,COALESCE(phone,''),COALESCE(title,''),contact_type,is_primary,created_at,updated_at,deleted_at FROM vendor_contacts WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND deleted_at IS NULL ORDER BY is_primary DESC,name,id LIMIT 500`, orgID, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.VendorContact{}
	for rows.Next() {
		var x models.VendorContact
		if err := rows.Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Email, &x.Phone, &x.Title, &x.ContactType, &x.IsPrimary, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func listVendorContracts(ctx context.Context, q database.Querier, orgID, vendorID string) ([]models.VendorContract, error) {
	rows, err := q.Query(ctx, `SELECT id,organization_id,vendor_id,contract_ref,name,status,start_date,end_date,notice_days,renewal_date,auto_renew,value_amount,currency,includes_dpa,signed_at,owner_user_id,metadata,created_at,updated_at,deleted_at FROM vendor_contracts WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND deleted_at IS NULL ORDER BY end_date NULLS LAST,name,id LIMIT 500`, orgID, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.VendorContract{}
	for rows.Next() {
		var x models.VendorContract
		if err := rows.Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.ContractRef, &x.Name, &x.Status, &x.StartDate, &x.EndDate, &x.NoticeDays, &x.RenewalDate, &x.AutoRenew, &x.ValueAmount, &x.Currency, &x.IncludesDPA, &x.SignedAt, &x.OwnerUserID, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func listVendorCertifications(ctx context.Context, q database.Querier, orgID, vendorID string) ([]models.VendorCertification, error) {
	rows, err := q.Query(ctx, `SELECT id,organization_id,vendor_id,name,COALESCE(issuer,''),COALESCE(certificate_number,''),status,issued_on,expires_on,COALESCE(evidence_reference,''),metadata,created_at,updated_at,deleted_at FROM vendor_certifications WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND deleted_at IS NULL ORDER BY expires_on NULLS LAST,name,id LIMIT 500`, orgID, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.VendorCertification{}
	for rows.Next() {
		var x models.VendorCertification
		if err := rows.Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Issuer, &x.CertificateNumber, &x.Status, &x.IssuedOn, &x.ExpiresOn, &x.EvidenceReference, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func listVendorSubprocessors(ctx context.Context, q database.Querier, orgID, vendorID string) ([]models.VendorSubprocessor, error) {
	rows, err := q.Query(ctx, `SELECT id,organization_id,vendor_id,name,purpose,COALESCE(country_code::text,''),data_categories,status,approved_at,approved_by,removed_at,metadata,created_at,updated_at,deleted_at FROM vendor_subprocessors WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND deleted_at IS NULL ORDER BY name,id LIMIT 500`, orgID, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.VendorSubprocessor{}
	for rows.Next() {
		var x models.VendorSubprocessor
		if err := rows.Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Purpose, &x.CountryCode, &x.DataCategories, &x.Status, &x.ApprovedAt, &x.ApprovedBy, &x.RemovedAt, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func insertVendorContact(ctx context.Context, q database.Querier, orgID, vendorID, actorID string, input models.VendorContactInput) (*models.VendorContact, error) {
	var x models.VendorContact
	err := q.QueryRow(ctx, `INSERT INTO vendor_contacts(organization_id,vendor_id,name,email,phone,title,contact_type,is_primary,created_by)VALUES($1::uuid,$2::uuid,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9::uuid)RETURNING id,organization_id,vendor_id,name,email,COALESCE(phone,''),COALESCE(title,''),contact_type,is_primary,created_at,updated_at,deleted_at`, orgID, vendorID, input.Name, input.Email, input.Phone, input.Title, input.ContactType, input.IsPrimary, actorID).Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Email, &x.Phone, &x.Title, &x.ContactType, &x.IsPrimary, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("insert vendor contact: %w", err)
	}
	return &x, nil
}
func updateVendorContact(ctx context.Context, q database.Querier, orgID, vendorID, id string, input models.VendorContactInput) (*models.VendorContact, error) {
	var x models.VendorContact
	err := q.QueryRow(ctx, `UPDATE vendor_contacts SET name=$4,email=$5,phone=NULLIF($6,''),title=NULLIF($7,''),contact_type=$8,is_primary=$9 WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND id=$3::uuid AND deleted_at IS NULL RETURNING id,organization_id,vendor_id,name,email,COALESCE(phone,''),COALESCE(title,''),contact_type,is_primary,created_at,updated_at,deleted_at`, orgID, vendorID, id, input.Name, input.Email, input.Phone, input.Title, input.ContactType, input.IsPrimary).Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Email, &x.Phone, &x.Title, &x.ContactType, &x.IsPrimary, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("update vendor contact: %w", err)
	}
	return &x, nil
}
func insertVendorContract(ctx context.Context, q database.Querier, orgID, vendorID, actorID string, input models.VendorContractInput) (*models.VendorContract, error) {
	var x models.VendorContract
	err := q.QueryRow(ctx, `INSERT INTO vendor_contracts(organization_id,vendor_id,contract_ref,name,status,start_date,end_date,notice_days,renewal_date,auto_renew,value_amount,currency,includes_dpa,signed_at,owner_user_id,metadata,created_by)VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::uuid,COALESCE($16::jsonb,'{}'::jsonb),$17::uuid)RETURNING id,organization_id,vendor_id,contract_ref,name,status,start_date,end_date,notice_days,renewal_date,auto_renew,value_amount,currency,includes_dpa,signed_at,owner_user_id,metadata,created_at,updated_at,deleted_at`, orgID, vendorID, input.ContractRef, input.Name, input.Status, input.StartDate, input.EndDate, input.NoticeDays, input.RenewalDate, input.AutoRenew, input.ValueAmount, input.Currency, input.IncludesDPA, input.SignedAt, input.OwnerUserID, nullableJSON(input.Metadata), actorID).Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.ContractRef, &x.Name, &x.Status, &x.StartDate, &x.EndDate, &x.NoticeDays, &x.RenewalDate, &x.AutoRenew, &x.ValueAmount, &x.Currency, &x.IncludesDPA, &x.SignedAt, &x.OwnerUserID, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("insert vendor contract: %w", err)
	}
	return &x, nil
}
func updateVendorContract(ctx context.Context, q database.Querier, orgID, vendorID, id string, input models.VendorContractInput) (*models.VendorContract, error) {
	var x models.VendorContract
	err := q.QueryRow(ctx, `UPDATE vendor_contracts SET contract_ref=$4,name=$5,status=$6,start_date=$7,end_date=$8,notice_days=$9,renewal_date=$10,auto_renew=$11,value_amount=$12,currency=$13,includes_dpa=$14,signed_at=$15,owner_user_id=$16::uuid,metadata=COALESCE($17::jsonb,metadata) WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND id=$3::uuid AND deleted_at IS NULL RETURNING id,organization_id,vendor_id,contract_ref,name,status,start_date,end_date,notice_days,renewal_date,auto_renew,value_amount,currency,includes_dpa,signed_at,owner_user_id,metadata,created_at,updated_at,deleted_at`, orgID, vendorID, id, input.ContractRef, input.Name, input.Status, input.StartDate, input.EndDate, input.NoticeDays, input.RenewalDate, input.AutoRenew, input.ValueAmount, input.Currency, input.IncludesDPA, input.SignedAt, input.OwnerUserID, nullableJSON(input.Metadata)).Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.ContractRef, &x.Name, &x.Status, &x.StartDate, &x.EndDate, &x.NoticeDays, &x.RenewalDate, &x.AutoRenew, &x.ValueAmount, &x.Currency, &x.IncludesDPA, &x.SignedAt, &x.OwnerUserID, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("update vendor contract: %w", err)
	}
	return &x, nil
}
func insertVendorCertification(ctx context.Context, q database.Querier, orgID, vendorID, actorID string, input models.VendorCertificationInput) (*models.VendorCertification, error) {
	var x models.VendorCertification
	err := q.QueryRow(ctx, `INSERT INTO vendor_certifications(organization_id,vendor_id,name,issuer,certificate_number,status,issued_on,expires_on,evidence_reference,metadata,created_by)VALUES($1::uuid,$2::uuid,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,NULLIF($9,''),COALESCE($10::jsonb,'{}'::jsonb),$11::uuid)RETURNING id,organization_id,vendor_id,name,COALESCE(issuer,''),COALESCE(certificate_number,''),status,issued_on,expires_on,COALESCE(evidence_reference,''),metadata,created_at,updated_at,deleted_at`, orgID, vendorID, input.Name, input.Issuer, input.CertificateNumber, input.Status, input.IssuedOn, input.ExpiresOn, input.EvidenceReference, nullableJSON(input.Metadata), actorID).Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Issuer, &x.CertificateNumber, &x.Status, &x.IssuedOn, &x.ExpiresOn, &x.EvidenceReference, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("insert vendor certification: %w", err)
	}
	return &x, nil
}
func updateVendorCertification(ctx context.Context, q database.Querier, orgID, vendorID, id string, input models.VendorCertificationInput) (*models.VendorCertification, error) {
	var x models.VendorCertification
	err := q.QueryRow(ctx, `UPDATE vendor_certifications SET name=$4,issuer=NULLIF($5,''),certificate_number=NULLIF($6,''),status=$7,issued_on=$8,expires_on=$9,evidence_reference=NULLIF($10,''),metadata=COALESCE($11::jsonb,metadata) WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND id=$3::uuid AND deleted_at IS NULL RETURNING id,organization_id,vendor_id,name,COALESCE(issuer,''),COALESCE(certificate_number,''),status,issued_on,expires_on,COALESCE(evidence_reference,''),metadata,created_at,updated_at,deleted_at`, orgID, vendorID, id, input.Name, input.Issuer, input.CertificateNumber, input.Status, input.IssuedOn, input.ExpiresOn, input.EvidenceReference, nullableJSON(input.Metadata)).Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Issuer, &x.CertificateNumber, &x.Status, &x.IssuedOn, &x.ExpiresOn, &x.EvidenceReference, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("update vendor certification: %w", err)
	}
	return &x, nil
}
func insertVendorSubprocessor(ctx context.Context, q database.Querier, orgID, vendorID, actorID string, input models.VendorSubprocessorInput) (*models.VendorSubprocessor, error) {
	var x models.VendorSubprocessor
	approved := input.Status == "approved"
	err := q.QueryRow(ctx, `INSERT INTO vendor_subprocessors(organization_id,vendor_id,name,purpose,country_code,data_categories,status,approved_at,approved_by,removed_at,metadata,created_by)VALUES($1::uuid,$2::uuid,$3,$4,NULLIF($5,''),$6,$7,CASE WHEN $8 THEN NOW() END,CASE WHEN $8 THEN $9::uuid END,CASE WHEN $7='removed' THEN NOW() END,COALESCE($10::jsonb,'{}'::jsonb),$9::uuid)RETURNING id,organization_id,vendor_id,name,purpose,COALESCE(country_code::text,''),data_categories,status,approved_at,approved_by,removed_at,metadata,created_at,updated_at,deleted_at`, orgID, vendorID, input.Name, input.Purpose, input.CountryCode, input.DataCategories, input.Status, approved, actorID, nullableJSON(input.Metadata)).Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Purpose, &x.CountryCode, &x.DataCategories, &x.Status, &x.ApprovedAt, &x.ApprovedBy, &x.RemovedAt, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("insert vendor subprocessor: %w", err)
	}
	return &x, nil
}
func updateVendorSubprocessor(ctx context.Context, q database.Querier, orgID, vendorID, id, actorID string, input models.VendorSubprocessorInput) (*models.VendorSubprocessor, error) {
	var x models.VendorSubprocessor
	err := q.QueryRow(ctx, `UPDATE vendor_subprocessors SET name=$4,purpose=$5,country_code=NULLIF($6,''),data_categories=$7,status=$8,approved_at=CASE WHEN $8='approved' THEN COALESCE(approved_at,NOW()) ELSE approved_at END,approved_by=CASE WHEN $8='approved' THEN COALESCE(approved_by,$9::uuid) ELSE approved_by END,removed_at=CASE WHEN $8='removed' THEN COALESCE(removed_at,NOW()) ELSE NULL END,metadata=COALESCE($10::jsonb,metadata) WHERE organization_id=$1::uuid AND vendor_id=$2::uuid AND id=$3::uuid AND deleted_at IS NULL RETURNING id,organization_id,vendor_id,name,purpose,COALESCE(country_code::text,''),data_categories,status,approved_at,approved_by,removed_at,metadata,created_at,updated_at,deleted_at`, orgID, vendorID, id, input.Name, input.Purpose, input.CountryCode, input.DataCategories, input.Status, actorID, nullableJSON(input.Metadata)).Scan(&x.ID, &x.OrganizationID, &x.VendorID, &x.Name, &x.Purpose, &x.CountryCode, &x.DataCategories, &x.Status, &x.ApprovedAt, &x.ApprovedBy, &x.RemovedAt, &x.Metadata, &x.CreatedAt, &x.UpdatedAt, &x.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("update vendor subprocessor: %w", err)
	}
	return &x, nil
}

func lockVendor(ctx context.Context, q database.Querier, orgID, id string, expected int64) (int64, models.VendorStatus, error) {
	var version int64
	var status models.VendorStatus
	err := q.QueryRow(ctx, `SELECT version,status FROM vendors WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL FOR UPDATE`, orgID, id).Scan(&version, &status)
	if err != nil {
		return 0, "", err
	}
	if version != expected {
		return version, status, ErrVendorVersionConflict
	}
	return version, status, nil
}
func bumpVendorVersion(ctx context.Context, q database.Querier, orgID, id string, expected int64) error {
	tag, err := q.Exec(ctx, `UPDATE vendors SET version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND deleted_at IS NULL`, orgID, id, expected)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrVendorVersionConflict
	}
	return nil
}
func ensureVendorUser(ctx context.Context, q database.Querier, orgID, userID string) error {
	var valid bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE organization_id=$1::uuid AND id=$2::uuid AND status='active' AND deleted_at IS NULL)`, orgID, userID).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrVendorUserInvalid
	}
	return nil
}

func (r *vendorRepo) recordVendorEvent(ctx context.Context, tx pgx.Tx, item *models.Vendor, actorID, eventType, summary string, details map[string]any) error {
	encoded, err := json.Marshal(details)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO vendor_events(organization_id,vendor_id,event_type,actor_user_id,vendor_version,summary,details)VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::jsonb)`, item.OrganizationID, item.ID, eventType, actorID, item.Version, summary, encoded); err != nil {
		return fmt.Errorf("append vendor event: %w", err)
	}
	payload := map[string]any{"type": "vendor." + eventType, "severity": item.Criticality, "org_id": item.OrganizationID, "entity_type": "vendor", "entity_id": item.ID, "entity_ref": item.VendorRef, "data": map[string]any{"vendor_id": item.ID, "vendor_ref": item.VendorRef, "name": item.Name, "status": item.Status, "criticality": item.Criticality, "risk_tier": item.RiskTier, "version": item.Version}, "timestamp": time.Now().UTC()}
	envelope, err := queuepkg.NewEnvelope("notification.event", item.OrganizationID, payload)
	if err != nil {
		return err
	}
	envelope.CausationID = item.ID
	envelope.Metadata = map[string]string{"entity_type": "vendor", "entity_id": item.ID, "event_type": eventType}
	if err = r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueue vendor event: %w", err)
	}
	return nil
}

func classifyVendorMutation(ctx context.Context, q database.Querier, orgID, id string, expected int64, cause error) error {
	if !errors.Is(cause, pgx.ErrNoRows) {
		return cause
	}
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendors WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL)`, orgID, id).Scan(&exists); err != nil {
		return err
	}
	if exists && expected > 0 {
		return ErrVendorVersionConflict
	}
	return pgx.ErrNoRows
}
func vendorSortColumn(value string) string {
	switch value {
	case "name":
		return "v.name"
	case "vendor_ref":
		return "v.vendor_ref"
	case "criticality":
		return "v.criticality"
	case "risk_tier":
		return "v.risk_tier"
	case "next_assessment_date":
		return "v.next_assessment_date"
	case "created_at":
		return "v.created_at"
	default:
		return "v.updated_at"
	}
}
