package repository

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

type OrganizationProfileRepository interface {
	GetOrganizationProfile(context.Context, string, string) (*models.OrganizationProfile, error)
	UpdateOrganizationProfile(context.Context, string, string, string, models.OrganizationProfile, string) (*models.OrganizationProfile, error)
}

type organizationProfileRepo struct{}

func NewOrganizationProfileRepository(pool *pgxpool.Pool) (OrganizationProfileRepository, error) {
	if pool == nil {
		return nil, errors.New("organization profile database pool is required")
	}
	// Queries deliberately have no unscoped pool fallback.
	return &organizationProfileRepo{}, nil
}

var _ OrganizationProfileRepository = (*organizationProfileRepo)(nil)

const organizationProfileColumns = `
	org.id, org.name, org.slug, COALESCE(org.legal_name,''),
	COALESCE(org.industry,''), COALESCE(org.country_code,''),
	COALESCE(org.timezone,''), COALESCE(org.default_language,''),
	COALESCE(org.supported_languages,ARRAY[]::text[]), COALESCE(org.employee_count_range,''),
	org.status::text, org.tier::text, org.fiscal_year_start_month,
	org.fiscal_year_start_day, org.enterprise_contacts, org.profile_version, org.updated_at`

const organizationProfileScope = `
	org.id=$1::uuid AND org.id=public.get_current_tenant()
	AND org.deleted_at IS NULL AND org.status IN ('active','trial')
	AND EXISTS (SELECT 1 FROM public.users actor
		WHERE actor.id=$2::uuid AND actor.organization_id=org.id
		AND actor.status='active' AND actor.deleted_at IS NULL)`

func scanOrganizationProfile(row interface{ Scan(...any) error }) (*models.OrganizationProfile, error) {
	profile := &models.OrganizationProfile{}
	var contacts []byte
	if err := row.Scan(&profile.ID, &profile.Name, &profile.Slug, &profile.LegalName,
		&profile.Industry, &profile.CountryCode, &profile.Timezone, &profile.DefaultLanguage,
		&profile.SupportedLanguages, &profile.EmployeeCountRange, &profile.Status, &profile.Tier,
		&profile.FiscalYearStartMonth, &profile.FiscalYearStartDay, &contacts, &profile.Version,
		&profile.UpdatedAt); err != nil {
		return nil, err
	}
	if len(contacts) > 2048 || json.Unmarshal(contacts, &profile.Contacts) != nil || profile.Contacts == nil {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	return profile, nil
}

func (r *organizationProfileRepo) GetOrganizationProfile(ctx context.Context, orgID, actorID string) (*models.OrganizationProfile, error) {
	q := database.QuerierFromContext(ctx, nil)
	if q == nil || !validProfileRepositoryID(orgID) || !validProfileRepositoryID(actorID) {
		return nil, models.ErrOrganizationProfileScope
	}
	profile, err := scanOrganizationProfile(q.QueryRow(ctx, `SELECT `+organizationProfileColumns+
		` FROM public.organizations org WHERE `+organizationProfileScope, orgID, actorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, models.ErrOrganizationProfileNotFound
	}
	if err != nil {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	return profile, nil
}

func (r *organizationProfileRepo) UpdateOrganizationProfile(
	ctx context.Context, orgID, actorID, requestID string, updated models.OrganizationProfile, reason string,
) (*models.OrganizationProfile, error) {
	q := database.QuerierFromContext(ctx, nil)
	if q == nil || !validProfileRepositoryID(orgID) || !validProfileRepositoryID(actorID) || updated.ID != orgID ||
		updated.Version < 1 || updated.Version >= models.OrganizationProfileMaximumVersion || len(reason) < 1 || len(reason) > 500 ||
		(requestID != "" && !validProfileRepositoryID(requestID)) {
		return nil, models.ErrOrganizationProfileScope
	}
	starter, ok := q.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	if !ok {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	tx, err := starter.Begin(ctx)
	if err != nil {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	defer func() {
		rollbackContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackContext)
	}()
	previous, err := scanOrganizationProfile(tx.QueryRow(ctx, `SELECT `+organizationProfileColumns+
		` FROM public.organizations org WHERE `+organizationProfileScope+` FOR UPDATE OF org`, orgID, actorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, models.ErrOrganizationProfileNotFound
	}
	if err != nil {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	if previous.Version != updated.Version {
		return nil, models.ErrOrganizationProfileConflict
	}
	contacts, err := json.Marshal(updated.Contacts)
	if err != nil || len(contacts) > 2048 || updated.Contacts == nil {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	// Identifier, status, tier, slug, raw settings, branding and metadata are never
	// SET here. Every organization UPDATE is versioned by the v59 invoker trigger.
	result, err := scanOrganizationProfile(tx.QueryRow(ctx, `
		UPDATE public.organizations org SET
			name=$3, legal_name=NULLIF($4,''), industry=NULLIF($5,''),
			country_code=NULLIF($6,''), timezone=$7, default_language=$8,
			supported_languages=$9::text[], employee_count_range=NULLIF($10,''),
			fiscal_year_start_month=$11, fiscal_year_start_day=$12,
			enterprise_contacts=$13::jsonb
		WHERE `+organizationProfileScope+` AND org.profile_version=$14::bigint
		RETURNING `+organizationProfileColumns,
		orgID, actorID, updated.Name, updated.LegalName, updated.Industry, updated.CountryCode,
		updated.Timezone, updated.DefaultLanguage, updated.SupportedLanguages, updated.EmployeeCountRange,
		updated.FiscalYearStartMonth, updated.FiscalYearStartDay, contacts, previous.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, models.ErrOrganizationProfileScope
	}
	if err != nil || result == nil || result.ID != orgID || result.Version != previous.Version+1 {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	metadata, err := json.Marshal(map[string]any{
		"schema_version": 1, "previous_version": previous.Version, "version": result.Version,
		"reason": reason, "changed_fields": changedOrganizationProfileFields(*previous, *result),
	})
	if err != nil {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	// Recheck active actor at the audit statement. A concurrent deprovisioning
	// cannot yield an unaudited committed profile change. Contact values and raw
	// configuration are not duplicated into the audit metadata.
	tag, err := tx.Exec(ctx, `
		INSERT INTO public.audit_logs (id,organization_id,user_id,action,entity_type,entity_id,request_id,metadata)
		SELECT $1::uuid,org.id,actor.id,'ORGANIZATION_PROFILE_UPDATED','organizations',org.id,
		       NULLIF($4,'')::uuid,$5::jsonb
		FROM public.organizations org JOIN public.users actor ON actor.organization_id=org.id
		WHERE org.id=$2::uuid AND org.id=public.get_current_tenant()
		  AND org.deleted_at IS NULL AND org.status IN ('active','trial')
		  AND actor.id=$3::uuid AND actor.status='active' AND actor.deleted_at IS NULL`,
		uuid.NewString(), orgID, actorID, requestID, metadata)
	if err != nil || tag.RowsAffected() != 1 {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, models.ErrOrganizationProfileUnavailable
	}
	return result, nil
}

func validProfileRepositoryID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func changedOrganizationProfileFields(previous, current models.OrganizationProfile) []string {
	fields := []string{}
	for _, field := range []struct{ name, previous, current string }{
		{"name", previous.Name, current.Name}, {"legal_name", previous.LegalName, current.LegalName},
		{"industry", previous.Industry, current.Industry}, {"country_code", previous.CountryCode, current.CountryCode},
		{"timezone", previous.Timezone, current.Timezone}, {"default_language", previous.DefaultLanguage, current.DefaultLanguage},
		{"employee_count_range", previous.EmployeeCountRange, current.EmployeeCountRange},
	} {
		if field.previous != field.current {
			fields = append(fields, field.name)
		}
	}
	if !slices.Equal(previous.SupportedLanguages, current.SupportedLanguages) {
		fields = append(fields, "supported_languages")
	}
	if previous.FiscalYearStartMonth != current.FiscalYearStartMonth {
		fields = append(fields, "fiscal_year_start_month")
	}
	if previous.FiscalYearStartDay != current.FiscalYearStartDay {
		fields = append(fields, "fiscal_year_start_day")
	}
	if !slices.Equal(previous.Contacts, current.Contacts) {
		fields = append(fields, "contacts")
	}
	return fields
}
