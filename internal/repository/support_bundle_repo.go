package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

var ErrSupportBundleAuditDenied = errors.New("support bundle consent cannot be recorded for this tenant and actor")

var supportAuditHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type SupportBundleRepository interface {
	RecordSupportBundleGeneration(context.Context, models.SupportBundleAudit) error
}

type supportBundleRepo struct{ pool *pgxpool.Pool }

func NewSupportBundleRepository(pool *pgxpool.Pool) (SupportBundleRepository, error) {
	if pool == nil {
		return nil, errors.New("support bundle audit database pool is required")
	}
	return &supportBundleRepo{pool: pool}, nil
}

func (r *supportBundleRepo) RecordSupportBundleGeneration(ctx context.Context, evidence models.SupportBundleAudit) error {
	if !validSupportAuditUUID(evidence.BundleID) || !validSupportAuditUUID(evidence.OrganizationID) || !validSupportAuditUUID(evidence.ActorID) ||
		(evidence.RequestID != "" && !validSupportAuditUUID(evidence.RequestID)) || evidence.GeneratedAt.IsZero() ||
		evidence.Scope != models.SupportBundleScope || evidence.RedactionProfile != models.SupportBundleRedactionProfile ||
		!supportAuditHashPattern.MatchString(evidence.ConfigurationFingerprint) || !supportAuditHashPattern.MatchString(evidence.ArchiveSHA256) ||
		evidence.ArchiveBytes < 4 || evidence.ArchiveBytes > models.MaximumSupportBundleBytes {
		return ErrSupportBundleAuditDenied
	}
	metadata, err := json.Marshal(map[string]any{
		"consent": true, "scope": evidence.Scope,
		"redaction_profile":         evidence.RedactionProfile,
		"configuration_fingerprint": evidence.ConfigurationFingerprint,
		"archive_sha256":            evidence.ArchiveSHA256, "archive_bytes": evidence.ArchiveBytes,
		"automatically_transmitted": false, "schema_version": 1,
	})
	if err != nil {
		return fmt.Errorf("encode support consent evidence: %w", err)
	}
	// One guarded INSERT binds tenant and active principal atomically. audit_logs
	// is append-only for the runtime role, and FORCE RLS independently guards the
	// destination even if the explicit predicates regress.
	tag, err := database.QuerierFromContext(ctx, r.pool).Exec(ctx, `
		INSERT INTO audit_logs (id,organization_id,user_id,action,entity_type,entity_id,request_id,metadata,created_at)
		SELECT $1::uuid,$2::uuid,actor.id,'SUPPORT_BUNDLE_GENERATED','support_bundles',$1::uuid,
		       NULLIF($4,'')::uuid,$5::jsonb,$6
		FROM users actor
		WHERE actor.id=$3::uuid AND actor.organization_id=$2::uuid AND actor.status='active'
		  AND actor.deleted_at IS NULL AND $2::uuid=get_current_tenant()`,
		evidence.BundleID, evidence.OrganizationID, evidence.ActorID,
		evidence.RequestID, metadata, evidence.GeneratedAt)
	if err != nil {
		return fmt.Errorf("record support consent evidence: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrSupportBundleAuditDenied
	}
	return nil
}

func validSupportAuditUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
