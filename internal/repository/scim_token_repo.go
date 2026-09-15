package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

func (r *scimRepo) CreateSCIMToken(ctx context.Context, organizationID, actorID string, token *models.SCIMToken, reason string) (*models.SCIMToken, error) {
	var result *models.SCIMToken
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := ensureDirectoryActiveUser(scoped, tx, organizationID, actorID); err != nil {
				return ErrSCIMNotFound
			}
			var err error
			result, err = scanSCIMToken(tx.QueryRow(scoped, `INSERT INTO scim_tokens AS token(
				id,organization_id,name,token_prefix,token_hash,scopes,rate_limit_per_minute,expires_at,
				is_active,created_by) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,true,$9::uuid)
				RETURNING `+scimTokenColumns, token.ID, organizationID, token.Name, token.Prefix,
				token.TokenHash, token.Scopes, token.RateLimitPerMinute, token.ExpiresAt, actorID))
			if err != nil {
				return classifySCIMWrite(err)
			}
			return r.recordTokenEvent(scoped, tx, result, actorID, "created", reason,
				map[string]any{"name": result.Name, "prefix": result.Prefix, "scopes": result.Scopes,
					"rate_limit_per_minute": result.RateLimitPerMinute, "expires_at": result.ExpiresAt})
		})
	})
	return result, err
}

func (r *scimRepo) RotateSCIMToken(ctx context.Context, organizationID, tokenID, actorID string,
	expectedVersion int64, prefix, digest, reason string, expiresAt *time.Time) (*models.SCIMToken, error) {
	var result *models.SCIMToken
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := ensureDirectoryActiveUser(scoped, tx, organizationID, actorID); err != nil {
				return ErrSCIMNotFound
			}
			current, err := lockSCIMToken(scoped, tx, organizationID, tokenID, expectedVersion)
			if err != nil {
				return err
			}
			if !current.Active || current.RevokedAt != nil {
				return ErrSCIMConflict
			}
			result, err = scanSCIMToken(tx.QueryRow(scoped, `UPDATE scim_tokens AS token SET
				token_prefix=$4,token_hash=$5,expires_at=COALESCE($6,expires_at),last_used_at=NULL,
				last_used_ip=NULL,version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid
				AND version=$3 AND is_active AND revoked_at IS NULL RETURNING `+scimTokenColumns,
				organizationID, tokenID, expectedVersion, prefix, digest, expiresAt))
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrSCIMVersion
			}
			if err != nil {
				return classifySCIMWrite(err)
			}
			return r.recordTokenEvent(scoped, tx, result, actorID, "rotated", reason,
				map[string]any{"old_prefix": current.Prefix, "new_prefix": result.Prefix,
					"scopes": result.Scopes, "expires_at": result.ExpiresAt})
		})
	})
	return result, err
}

func (r *scimRepo) RevokeSCIMToken(ctx context.Context, organizationID, tokenID, actorID string,
	expectedVersion int64, reason string) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		return withTransaction(scoped, querier, func(tx pgx.Tx) error {
			if err := ensureDirectoryActiveUser(scoped, tx, organizationID, actorID); err != nil {
				return ErrSCIMNotFound
			}
			current, err := lockSCIMToken(scoped, tx, organizationID, tokenID, expectedVersion)
			if err != nil {
				return err
			}
			if !current.Active || current.RevokedAt != nil {
				return ErrSCIMConflict
			}
			result, err := scanSCIMToken(tx.QueryRow(scoped, `UPDATE scim_tokens AS token SET is_active=false,
				revoked_by=$4::uuid,revoked_at=NOW(),revoke_reason=$5,version=version+1
				WHERE organization_id=$1::uuid AND id=$2::uuid AND version=$3 AND is_active
				RETURNING `+scimTokenColumns, organizationID, tokenID, expectedVersion, actorID, reason))
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrSCIMVersion
			}
			if err != nil {
				return classifySCIMWrite(err)
			}
			return r.recordTokenEvent(scoped, tx, result, actorID, "revoked", reason,
				map[string]any{"prefix": current.Prefix, "scopes": current.Scopes})
		})
	})
}

func (r *scimRepo) ListSCIMTokens(ctx context.Context, organizationID string, pagination models.PaginationRequest) ([]models.SCIMToken, int, error) {
	items := []models.SCIMToken{}
	total := 0
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		if err := querier.QueryRow(scoped, `SELECT COUNT(*) FROM scim_tokens
			WHERE organization_id=$1::uuid`, organizationID).Scan(&total); err != nil {
			return fmt.Errorf("count SCIM tokens: %w", err)
		}
		rows, err := querier.Query(scoped, `SELECT `+scimTokenColumns+` FROM scim_tokens token
			WHERE token.organization_id=$1::uuid ORDER BY token.created_at DESC,token.id DESC
			LIMIT $2 OFFSET $3`, organizationID, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
		if err != nil {
			return fmt.Errorf("list SCIM tokens: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanSCIMToken(rows)
			if err != nil {
				return fmt.Errorf("scan SCIM token: %w", err)
			}
			item.TokenHash = ""
			items = append(items, *item)
		}
		return rows.Err()
	})
	return items, total, err
}

// ResolveSCIMTokenTenant is the only intentionally pre-tenant lookup. The
// SECURITY DEFINER function exposes only row/tenant IDs for one exact opaque
// prefix; credential material and authorization state remain behind FORCE RLS.
func (r *scimRepo) ResolveSCIMTokenTenant(ctx context.Context, prefix string) (string, string, error) {
	var tokenID, organizationID string
	if err := r.pool.QueryRow(ctx, `SELECT token_id,organization_id
		FROM resolve_scim_token_tenant($1)`, prefix).Scan(&tokenID, &organizationID); err != nil {
		return "", "", classifySCIMRead(err)
	}
	return tokenID, organizationID, nil
}

func (r *scimRepo) GetSCIMTokenForAuthentication(ctx context.Context, organizationID, tokenID, prefix string) (*models.SCIMToken, error) {
	var result *models.SCIMToken
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		var err error
		result, err = scanSCIMToken(querier.QueryRow(scoped, `SELECT `+scimTokenColumns+`
			FROM scim_tokens token WHERE token.organization_id=$1::uuid AND token.id=$2::uuid
			AND token.token_prefix=$3 AND EXISTS(SELECT 1 FROM users creator
				WHERE creator.organization_id=token.organization_id AND creator.id=token.created_by
				AND creator.status='active' AND creator.deleted_at IS NULL)`, organizationID, tokenID, prefix))
		return classifySCIMRead(err)
	})
	return result, err
}

func (r *scimRepo) RecordSCIMTokenUse(ctx context.Context, organizationID, tokenID, tokenHash string,
	expectedVersion int64, clientIP string, usedAt time.Time) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, querier database.Querier) error {
		tag, err := querier.Exec(scoped, `UPDATE scim_tokens SET last_used_at=$5,
			last_used_ip=NULLIF($6,'')::inet WHERE organization_id=$1::uuid AND id=$2::uuid
			AND token_hash=$3 AND version=$4 AND is_active AND revoked_at IS NULL
			AND (expires_at IS NULL OR expires_at>$5) AND EXISTS(SELECT 1 FROM users creator
				WHERE creator.organization_id=scim_tokens.organization_id AND creator.id=scim_tokens.created_by
				AND creator.status='active' AND creator.deleted_at IS NULL)`, organizationID, tokenID, tokenHash,
			expectedVersion, usedAt, clientIP)
		if err != nil {
			return fmt.Errorf("record SCIM token use: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrSCIMNotFound
		}
		return nil
	})
}

func lockSCIMToken(ctx context.Context, querier database.Querier, organizationID, tokenID string,
	expectedVersion int64) (*models.SCIMToken, error) {
	item, err := scanSCIMToken(querier.QueryRow(ctx, `SELECT `+scimTokenColumns+`
		FROM scim_tokens token WHERE token.organization_id=$1::uuid AND token.id=$2::uuid FOR UPDATE`,
		organizationID, tokenID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSCIMNotFound
	}
	if err != nil {
		return nil, err
	}
	if item.Version != expectedVersion {
		return nil, ErrSCIMVersion
	}
	return item, nil
}
