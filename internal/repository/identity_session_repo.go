package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

func (r *identityLifecycleRepo) ListSessions(ctx context.Context, organizationID, userID, currentTokenHash string) ([]models.IdentitySession, error) {
	items := []models.IdentitySession{}
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		rows, err := q.Query(scoped, `SELECT id,user_id,organization_id,COALESCE(ip_address::text,''),COALESCE(user_agent,''),
			COALESCE(device_name,''),authentication_method,mfa_verified_at,expires_at,last_seen_at,revoked_at,
			COALESCE(revoke_reason,''),version,created_at,token_hash=$3
			FROM user_sessions WHERE organization_id=$1::uuid AND user_id=$2::uuid
			ORDER BY (revoked_at IS NULL AND expires_at>NOW()) DESC,last_seen_at DESC,id DESC`, organizationID, userID, currentTokenHash)
		if err != nil {
			return fmt.Errorf("list identity sessions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanIdentitySession(rows)
			if err != nil {
				return fmt.Errorf("scan identity session: %w", err)
			}
			items = append(items, *item)
		}
		return rows.Err()
	})
	return items, err
}

func (r *identityLifecycleRepo) ResolveSession(ctx context.Context, organizationID, userID, accessTokenHash string) (*models.IdentitySession, error) {
	var result *models.IdentitySession
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		var err error
		result, err = scanIdentitySession(q.QueryRow(scoped, `SELECT id,user_id,organization_id,
			COALESCE(ip_address::text,''),COALESCE(user_agent,''),COALESCE(device_name,''),authentication_method,
			mfa_verified_at,expires_at,last_seen_at,revoked_at,COALESCE(revoke_reason,''),version,created_at,true
			FROM user_sessions WHERE organization_id=$1::uuid AND user_id=$2::uuid AND token_hash=$3
			AND revoked_at IS NULL AND expires_at>NOW()`, organizationID, userID, accessTokenHash))
		return classifyIdentityRead(err)
	})
	return result, err
}

func (r *identityLifecycleRepo) RevokeSession(ctx context.Context, organizationID, userID, sessionID, actorID string, expectedVersion int64, reason string, metadata models.IdentityRequestMetadata) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			var currentVersion int64
			if err := tx.QueryRow(scoped, `SELECT version FROM user_sessions WHERE organization_id=$1::uuid
				AND user_id=$2::uuid AND id=$3::uuid FOR UPDATE`, organizationID, userID, sessionID).Scan(&currentVersion); err != nil {
				return classifyIdentityRead(err)
			}
			if currentVersion != expectedVersion {
				return ErrIdentityVersion
			}
			tag, err := tx.Exec(scoped, `UPDATE user_sessions SET revoked_at=NOW(),revoked_by=$4::uuid,
				revoke_reason=$6,version=version+1 WHERE organization_id=$1::uuid AND user_id=$2::uuid
				AND id=$3::uuid AND version=$5 AND revoked_at IS NULL`, organizationID, userID, sessionID,
				actorID, expectedVersion, reason)
			if err != nil {
				return classifyIdentityWrite(err)
			}
			if tag.RowsAffected() != 1 {
				return ErrIdentityConflict
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &actorID, &sessionID, "session.revoked",
				reason, metadata, map[string]any{"session_id": sessionID})
		})
	})
}

func (r *identityLifecycleRepo) RevokeAllSessions(ctx context.Context, organizationID, userID, actorID, currentTokenHash string, exceptCurrent bool, reason string, metadata models.IdentityRequestMetadata) (int, error) {
	count := 0
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			query := `UPDATE user_sessions SET revoked_at=NOW(),revoked_by=$3::uuid,revoke_reason=$5,version=version+1
				WHERE organization_id=$1::uuid AND user_id=$2::uuid AND revoked_at IS NULL AND expires_at>NOW()`
			if exceptCurrent {
				query += ` AND token_hash<>$4`
			}
			tag, err := tx.Exec(scoped, query, organizationID, userID, actorID, currentTokenHash, reason)
			if err != nil {
				return classifyIdentityWrite(err)
			}
			count = int(tag.RowsAffected())
			return r.recordEvent(scoped, tx, organizationID, &userID, &actorID, nil, "session.global_sign_out",
				reason, metadata, map[string]any{"revoked_sessions": count, "except_current": exceptCurrent})
		})
	})
	return count, err
}

func (r *identityLifecycleRepo) CreateChallenge(ctx context.Context, challenge *models.IdentityAuthenticationChallenge) error {
	return r.withTenant(ctx, challenge.OrganizationID, func(scoped context.Context, q database.Querier) error {
		challenge.ID = uuid.NewString()
		_, err := q.Exec(scoped, `INSERT INTO identity_authentication_challenges(id,organization_id,user_id,session_id,
			token_hash,purpose,allowed_methods,webauthn_session,step_up_purpose,expires_at,max_attempts)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8::jsonb,NULLIF($9,''),$10,$11)`,
			challenge.ID, challenge.OrganizationID, challenge.UserID, challenge.SessionID, challenge.TokenHash,
			challenge.Purpose, challenge.AllowedMethods, nullableIdentityJSON(challenge.WebAuthnSession), challenge.StepUpPurpose,
			challenge.ExpiresAt, challenge.MaxAttempts)
		if err != nil {
			return classifyIdentityWrite(err)
		}
		return nil
	})
}

func (r *identityLifecycleRepo) GetChallenge(ctx context.Context, organizationID, tokenHash, purpose string) (*models.IdentityAuthenticationChallenge, error) {
	var result *models.IdentityAuthenticationChallenge
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		item := &models.IdentityAuthenticationChallenge{}
		var webauthnSession []byte
		err := q.QueryRow(scoped, `SELECT id,organization_id,user_id,session_id,purpose,allowed_methods,
			COALESCE(webauthn_session,'null'::jsonb),COALESCE(step_up_purpose,''),expires_at,consumed_at,
			failed_attempts,max_attempts FROM identity_authentication_challenges
			WHERE organization_id=$1::uuid AND token_hash=$2 AND purpose=$3`, organizationID, tokenHash, purpose).
			Scan(&item.ID, &item.OrganizationID, &item.UserID, &item.SessionID, &item.Purpose, &item.AllowedMethods,
				&webauthnSession, &item.StepUpPurpose, &item.ExpiresAt, &item.ConsumedAt, &item.FailedAttempts, &item.MaxAttempts)
		if err != nil {
			return classifyIdentityRead(err)
		}
		if string(webauthnSession) != "null" {
			item.WebAuthnSession = webauthnSession
		}
		result = item
		return nil
	})
	return result, err
}

func (r *identityLifecycleRepo) FailChallenge(ctx context.Context, organizationID, tokenHash, purpose string, now time.Time) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		tag, err := q.Exec(scoped, `UPDATE identity_authentication_challenges
			SET failed_attempts=failed_attempts+1,
			consumed_at=CASE WHEN failed_attempts+1>=max_attempts THEN $4 ELSE consumed_at END
			WHERE organization_id=$1::uuid AND token_hash=$2 AND purpose=$3 AND consumed_at IS NULL
			AND expires_at>$4 AND failed_attempts<max_attempts`, organizationID, tokenHash, purpose, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrIdentityAttempts
		}
		return nil
	})
}

func (r *identityLifecycleRepo) ConsumeChallenge(ctx context.Context, organizationID, tokenHash, purpose string, now time.Time) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		tag, err := q.Exec(scoped, `UPDATE identity_authentication_challenges SET consumed_at=$4
			WHERE organization_id=$1::uuid AND token_hash=$2 AND purpose=$3 AND consumed_at IS NULL
			AND expires_at>$4 AND failed_attempts<max_attempts`, organizationID, tokenHash, purpose, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrIdentityConsumed
		}
		return nil
	})
}

func (r *identityLifecycleRepo) CreateStepUpGrant(ctx context.Context, grant *models.IdentityStepUpGrant, tokenHash string) error {
	return r.withTenant(ctx, grant.OrganizationID, func(scoped context.Context, q database.Querier) error {
		grant.ID = uuid.NewString()
		_, err := q.Exec(scoped, `INSERT INTO identity_step_up_grants(id,organization_id,user_id,session_id,token_hash,
			purpose,authentication_method,expires_at) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8)`,
			grant.ID, grant.OrganizationID, grant.UserID, grant.SessionID, tokenHash, grant.Purpose,
			grant.AuthenticationMethod, grant.ExpiresAt)
		return classifyIdentityWrite(err)
	})
}

func (r *identityLifecycleRepo) ConsumeStepUpGrant(ctx context.Context, organizationID, userID, sessionID, purpose, tokenHash string, now time.Time) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		tag, err := q.Exec(scoped, `UPDATE identity_step_up_grants SET consumed_at=$6 WHERE organization_id=$1::uuid
			AND user_id=$2::uuid AND session_id=$3::uuid AND purpose=$4 AND token_hash=$5
			AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>$6`, organizationID, userID, sessionID,
			purpose, tokenHash, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrIdentityNotFound
		}
		return nil
	})
}

func scanIdentitySession(row rowScanner) (*models.IdentitySession, error) {
	item := &models.IdentitySession{}
	var method string
	err := row.Scan(&item.ID, &item.UserID, &item.OrganizationID, &item.IPAddress, &item.UserAgent,
		&item.DeviceName, &method, &item.MFAVerifiedAt, &item.ExpiresAt, &item.LastSeenAt, &item.RevokedAt,
		&item.RevokeReason, &item.Version, &item.CreatedAt, &item.Current)
	item.AuthenticationMethod = models.IdentityMethod(method)
	return item, err
}

func nullableIdentityJSON(value []byte) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return value
}
