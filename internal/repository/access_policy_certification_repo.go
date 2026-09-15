package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/accesscontrol"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

func (r *policyAccessRepo) CertifyPolicy(
	ctx context.Context, organizationID, policyID, actorID, requestID string,
	input models.AccessPolicyCertificationInput,
) (*models.AccessPolicyCertification, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var certification *models.AccessPolicyCertification
	err := withTransaction(ctx, querier, func(tx pgx.Tx) error {
		policy, err := lockAccessPolicy(ctx, tx, organizationID, policyID)
		if err != nil {
			return err
		}
		if policy.Version != input.ExpectedVersion {
			return accesscontrol.ErrConflict
		}
		snapshot, err := json.Marshal(policy)
		if err != nil {
			return fmt.Errorf("encode access policy certification snapshot: %w", err)
		}
		if len(snapshot) > 64*1024 {
			return errors.New("access policy certification snapshot exceeds 65536 bytes")
		}
		digest := sha256.Sum256(snapshot)
		certification, err = scanAccessPolicyCertification(tx.QueryRow(ctx, `INSERT INTO access_policy_certifications (
			organization_id,access_policy_id,policy_version,certified_by,decision,reason,
			policy_snapshot,snapshot_sha256,request_id
		) VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5,$6,$7::jsonb,$8,$9)
		RETURNING id,organization_id,access_policy_id,policy_version,certified_by,
			decision,reason,policy_snapshot,snapshot_sha256,certified_at`,
			organizationID, policyID, policy.Version, actorID, input.Decision, input.Reason,
			snapshot, hex.EncodeToString(digest[:]), requestID))
		if err != nil {
			return classifyPolicyAccessWrite("certify access policy", err)
		}
		return recordAccessPolicyChange(ctx, tx, organizationID, policyID, "certification", certification.ID,
			"policy_certified", actorID, requestID, input.Reason, nil, certification)
	})
	if err != nil {
		return nil, err
	}
	return certification, nil
}

func (r *policyAccessRepo) ListPolicyCertifications(
	ctx context.Context, organizationID, policyID string, pagination models.PaginationRequest,
) ([]models.AccessPolicyCertification, int, error) {
	querier := database.QuerierFromContext(ctx, r.pool)
	var total int
	if err := querier.QueryRow(ctx, `SELECT COUNT(*) FROM access_policy_certifications
		WHERE organization_id=$1::uuid AND access_policy_id=$2::uuid`, organizationID, policyID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count access policy certifications: %w", err)
	}
	rows, err := querier.Query(ctx, `SELECT id,organization_id,access_policy_id,policy_version,
		certified_by,decision,reason,policy_snapshot,snapshot_sha256,certified_at
		FROM access_policy_certifications
		WHERE organization_id=$1::uuid AND access_policy_id=$2::uuid
		ORDER BY certified_at DESC,id LIMIT $3 OFFSET $4`, organizationID, policyID,
		pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list access policy certifications: %w", err)
	}
	defer rows.Close()
	items := make([]models.AccessPolicyCertification, 0, pagination.PageSize)
	for rows.Next() {
		item, err := scanAccessPolicyCertification(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan access policy certification: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate access policy certifications: %w", err)
	}
	return items, total, nil
}

func scanAccessPolicyCertification(row policyAccessRowScanner) (*models.AccessPolicyCertification, error) {
	item := new(models.AccessPolicyCertification)
	if err := row.Scan(
		&item.ID, &item.OrganizationID, &item.PolicyID, &item.PolicyVersion,
		&item.CertifiedBy, &item.Decision, &item.Reason, &item.PolicySnapshot,
		&item.SnapshotSHA256, &item.CertifiedAt,
	); err != nil {
		return nil, err
	}
	return item, nil
}
