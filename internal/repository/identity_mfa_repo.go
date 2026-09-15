package repository

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

const identityMFASelect = `SELECT factor.id,factor.organization_id,factor.user_id,factor.method::text,
	COALESCE(factor.display_name,''),factor.is_primary,factor.is_verified,factor.verified_at,factor.last_used_at,
	factor.disabled_at,COALESCE((SELECT COUNT(*) FROM identity_mfa_recovery_codes code
		WHERE code.organization_id=factor.organization_id AND code.mfa_id=factor.id AND code.consumed_at IS NULL),0),
	factor.version,factor.created_at,factor.updated_at,factor.secret_encrypted,factor.last_totp_step
	FROM user_mfa factor `

func (r *identityLifecycleRepo) CreateTOTPFactor(ctx context.Context, organizationID, userID, actorID, displayName string, secretCipher []byte, reason string, metadata models.IdentityRequestMetadata) (*models.IdentityMFAFactor, error) {
	var result *models.IdentityMFAFactor
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			if err := ensureIdentityActiveUser(scoped, tx, organizationID, userID); err != nil {
				return err
			}
			// An abandoned, unverified enrollment is replaced atomically.
			if _, err := tx.Exec(scoped, `UPDATE user_mfa SET disabled_at=NOW(),disabled_by=$3::uuid,
				disable_reason='Superseded enrollment',version=version+1 WHERE organization_id=$1::uuid
				AND user_id=$2::uuid AND method='totp' AND is_verified=false AND disabled_at IS NULL`,
				organizationID, userID, actorID); err != nil {
				return err
			}
			var primary bool
			if err := tx.QueryRow(scoped, `SELECT NOT EXISTS(SELECT 1 FROM user_mfa WHERE organization_id=$1::uuid
				AND user_id=$2::uuid AND is_primary AND disabled_at IS NULL)`, organizationID, userID).Scan(&primary); err != nil {
				return err
			}
			id := uuid.NewString()
			_, err := tx.Exec(scoped, `INSERT INTO user_mfa(id,organization_id,user_id,method,secret_encrypted,
				display_name,is_primary,is_verified) VALUES($1::uuid,$2::uuid,$3::uuid,'totp',$4,NULLIF($5,''),$6,false)`,
				id, organizationID, userID, secretCipher, displayName, primary)
			if err != nil {
				return classifyIdentityWrite(err)
			}
			result, err = scanIdentityMFA(tx.QueryRow(scoped, identityMFASelect+`
				WHERE factor.organization_id=$1::uuid AND factor.id=$2::uuid`, organizationID, id))
			if err != nil {
				return err
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &actorID, nil, "mfa.totp_enrollment_started",
				reason, metadata, map[string]any{"factor_id": id})
		})
	})
	return result, err
}

func (r *identityLifecycleRepo) GetMFAFactor(ctx context.Context, organizationID, userID, factorID string) (*models.IdentityMFAFactor, error) {
	var result *models.IdentityMFAFactor
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		var err error
		result, err = scanIdentityMFA(q.QueryRow(scoped, identityMFASelect+`
			WHERE factor.organization_id=$1::uuid AND factor.user_id=$2::uuid AND factor.id=$3::uuid`,
			organizationID, userID, factorID))
		return classifyIdentityRead(err)
	})
	return result, err
}

func (r *identityLifecycleRepo) ListMFAFactors(ctx context.Context, organizationID, userID string) ([]models.IdentityMFAFactor, error) {
	items := []models.IdentityMFAFactor{}
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		rows, err := q.Query(scoped, identityMFASelect+` WHERE factor.organization_id=$1::uuid AND factor.user_id=$2::uuid
			AND factor.disabled_at IS NULL ORDER BY factor.is_primary DESC,factor.created_at,factor.id`, organizationID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanIdentityMFA(rows)
			if err != nil {
				return err
			}
			items = append(items, *item)
		}
		return rows.Err()
	})
	return items, err
}

func (r *identityLifecycleRepo) VerifyTOTPFactor(ctx context.Context, organizationID, userID, factorID string, expectedVersion, totpStep int64, recoveryHashes []string, reason string, metadata models.IdentityRequestMetadata) (*models.IdentityMFAFactor, error) {
	var result *models.IdentityMFAFactor
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			tag, err := tx.Exec(scoped, `UPDATE user_mfa SET is_verified=true,verified_at=NOW(),last_used_at=NOW(),
				last_totp_step=$5,version=version+1 WHERE organization_id=$1::uuid AND user_id=$2::uuid AND id=$3::uuid
				AND method='totp' AND version=$4 AND disabled_at IS NULL AND is_verified=false
				AND (last_totp_step IS NULL OR last_totp_step<$5)`, organizationID, userID, factorID, expectedVersion, totpStep)
			if err != nil {
				return classifyIdentityWrite(err)
			}
			if tag.RowsAffected() != 1 {
				return ErrIdentityVersion
			}
			if err := insertRecoveryCodes(scoped, tx, organizationID, userID, factorID, recoveryHashes); err != nil {
				return err
			}
			if _, err := tx.Exec(scoped, `UPDATE users SET mfa_exempt_until=NULL,mfa_exempt_reason=NULL,
				identity_version=identity_version+1 WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, userID); err != nil {
				return err
			}
			result, err = scanIdentityMFA(tx.QueryRow(scoped, identityMFASelect+`
				WHERE factor.organization_id=$1::uuid AND factor.id=$2::uuid`, organizationID, factorID))
			if err != nil {
				return err
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &userID, nil, "mfa.totp_verified",
				reason, metadata, map[string]any{"factor_id": factorID, "recovery_codes": len(recoveryHashes)})
		})
	})
	return result, err
}

func (r *identityLifecycleRepo) ListRecoveryCodeHashes(ctx context.Context, organizationID, userID string) (map[string]string, error) {
	result := map[string]string{}
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		rows, err := q.Query(scoped, `SELECT id,code_hash FROM identity_mfa_recovery_codes
			WHERE organization_id=$1::uuid AND user_id=$2::uuid AND consumed_at IS NULL ORDER BY position`, organizationID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, hash string
			if err := rows.Scan(&id, &hash); err != nil {
				return err
			}
			result[id] = hash
		}
		return rows.Err()
	})
	return result, err
}

func (r *identityLifecycleRepo) UseTOTPFactor(ctx context.Context, organizationID, userID, factorID string, totpStep int64, now time.Time) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		tag, err := q.Exec(scoped, `UPDATE user_mfa SET last_totp_step=$4,last_used_at=$5,version=version+1
			WHERE organization_id=$1::uuid AND user_id=$2::uuid AND id=$3::uuid AND method='totp'
			AND is_verified AND disabled_at IS NULL AND (last_totp_step IS NULL OR last_totp_step<$4)`,
			organizationID, userID, factorID, totpStep, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrIdentityConsumed
		}
		return nil
	})
}

func (r *identityLifecycleRepo) ConsumeRecoveryCode(ctx context.Context, organizationID, userID, codeID, expectedHash string, now time.Time) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		tag, err := q.Exec(scoped, `UPDATE identity_mfa_recovery_codes SET consumed_at=$5
			WHERE organization_id=$1::uuid AND user_id=$2::uuid AND id=$3::uuid AND code_hash=$4 AND consumed_at IS NULL`,
			organizationID, userID, codeID, expectedHash, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrIdentityConsumed
		}
		return nil
	})
}

func (r *identityLifecycleRepo) ReplaceRecoveryCodes(ctx context.Context, organizationID, userID, factorID string, hashes []string, reason string, metadata models.IdentityRequestMetadata) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			if err := tx.QueryRow(scoped, `SELECT id FROM user_mfa WHERE organization_id=$1::uuid AND user_id=$2::uuid
				AND id=$3::uuid AND is_verified AND disabled_at IS NULL FOR UPDATE`, organizationID, userID, factorID).Scan(&factorID); err != nil {
				return classifyIdentityRead(err)
			}
			if _, err := tx.Exec(scoped, `DELETE FROM identity_mfa_recovery_codes
				WHERE organization_id=$1::uuid AND user_id=$2::uuid AND mfa_id=$3::uuid`, organizationID, userID, factorID); err != nil {
				return err
			}
			if err := insertRecoveryCodes(scoped, tx, organizationID, userID, factorID, hashes); err != nil {
				return err
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &userID, nil, "mfa.recovery_codes_regenerated",
				reason, metadata, map[string]any{"factor_id": factorID, "count": len(hashes)})
		})
	})
}

func (r *identityLifecycleRepo) DisableMFAFactor(ctx context.Context, organizationID, userID, factorID, actorID string, expectedVersion int64, reason string, metadata models.IdentityRequestMetadata) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			if err := ensureCanRemoveIdentityFactor(scoped, tx, organizationID, userID, "totp", factorID); err != nil {
				return err
			}
			tag, err := tx.Exec(scoped, `UPDATE user_mfa SET disabled_at=NOW(),disabled_by=$4::uuid,disable_reason=$6,
				is_primary=false,version=version+1 WHERE organization_id=$1::uuid AND user_id=$2::uuid AND id=$3::uuid
				AND version=$5 AND disabled_at IS NULL`, organizationID, userID, factorID, actorID, expectedVersion, reason)
			if err != nil {
				return classifyIdentityWrite(err)
			}
			if tag.RowsAffected() != 1 {
				return ErrIdentityVersion
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &actorID, nil, "mfa.factor_disabled",
				reason, metadata, map[string]any{"factor_id": factorID})
		})
	})
}

func (r *identityLifecycleRepo) AdminResetMFA(ctx context.Context, organizationID, targetUserID, actorID, reason string, exemptUntil time.Time, metadata models.IdentityRequestMetadata) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			if actorID == targetUserID {
				return ErrIdentityConflict
			}
			var actorReady bool
			if err := tx.QueryRow(scoped, `SELECT EXISTS(SELECT 1 FROM users actor
				WHERE actor.organization_id=$1::uuid AND actor.id=$2::uuid AND actor.status='active' AND actor.deleted_at IS NULL
				AND (actor.is_super_admin OR EXISTS(SELECT 1 FROM effective_user_roles ur JOIN roles role ON role.id=ur.role_id
					WHERE ur.organization_id=actor.organization_id AND ur.user_id=actor.id
					AND role.slug IN ('org_admin','super_admin')))
				AND (EXISTS(SELECT 1 FROM user_mfa factor WHERE factor.organization_id=actor.organization_id
					AND factor.user_id=actor.id AND factor.is_verified AND factor.disabled_at IS NULL)
					OR EXISTS(SELECT 1 FROM identity_passkeys passkey WHERE passkey.organization_id=actor.organization_id
					AND passkey.user_id=actor.id AND passkey.removed_at IS NULL)))`, organizationID, actorID).Scan(&actorReady); err != nil {
				return err
			}
			if !actorReady {
				return ErrIdentityAdminRecovery
			}
			if err := ensureIdentityActiveUser(scoped, tx, organizationID, targetUserID); err != nil {
				return err
			}
			if _, err := tx.Exec(scoped, `UPDATE users SET mfa_exempt_until=$3,mfa_exempt_reason=$4,
				identity_version=identity_version+1 WHERE organization_id=$1::uuid AND id=$2::uuid`,
				organizationID, targetUserID, exemptUntil, reason); err != nil {
				return err
			}
			if _, err := tx.Exec(scoped, `UPDATE user_mfa SET disabled_at=COALESCE(disabled_at,NOW()),disabled_by=$3::uuid,
				disable_reason=COALESCE(disable_reason,$4),is_primary=false,version=version+1
				WHERE organization_id=$1::uuid AND user_id=$2::uuid AND disabled_at IS NULL`,
				organizationID, targetUserID, actorID, reason); err != nil {
				return err
			}
			if _, err := tx.Exec(scoped, `UPDATE identity_passkeys SET removed_at=COALESCE(removed_at,NOW()),removed_by=$3::uuid,
				removal_reason=COALESCE(removal_reason,$4),version=version+1
				WHERE organization_id=$1::uuid AND user_id=$2::uuid AND removed_at IS NULL`,
				organizationID, targetUserID, actorID, reason); err != nil {
				return err
			}
			if _, err := tx.Exec(scoped, `UPDATE user_sessions SET revoked_at=COALESCE(revoked_at,NOW()),revoked_by=$3::uuid,
				revoke_reason=COALESCE(revoke_reason,$4),version=version+1
				WHERE organization_id=$1::uuid AND user_id=$2::uuid AND revoked_at IS NULL`,
				organizationID, targetUserID, actorID, reason); err != nil {
				return err
			}
			return r.recordEvent(scoped, tx, organizationID, &targetUserID, &actorID, nil, "mfa.admin_reset",
				reason, metadata, map[string]any{"target_user_id": targetUserID})
		})
	})
}

func (r *identityLifecycleRepo) CreatePasskey(ctx context.Context, organizationID, userID, actorID string, passkey *models.IdentityPasskey, challengeHash string, now time.Time, reason string, metadata models.IdentityRequestMetadata) (*models.IdentityPasskey, error) {
	var result *models.IdentityPasskey
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			if err := ensureIdentityActiveUser(scoped, tx, organizationID, userID); err != nil {
				return err
			}
			if err := consumeIdentityChallengeTx(scoped, tx, organizationID, userID, challengeHash, "passkey_registration", now); err != nil {
				return err
			}
			if passkey.ID == "" {
				passkey.ID = uuid.NewString()
			}
			_, err := tx.Exec(scoped, `INSERT INTO identity_passkeys(id,organization_id,user_id,credential_id,
				credential_ciphertext,attestation_type,transport,sign_count,clone_warning,backup_eligible,backup_state,
				device_name,aaguid) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::uuid)`,
				passkey.ID, organizationID, userID, passkey.CredentialID, passkey.CredentialCipher,
				passkey.AttestationType, passkey.Transports, passkey.SignCount, passkey.CloneWarning,
				passkey.BackupEligible, passkey.BackupState, passkey.DeviceName, passkey.AAGUID)
			if err != nil {
				return classifyIdentityWrite(err)
			}
			if _, err := tx.Exec(scoped, `UPDATE users SET mfa_exempt_until=NULL,mfa_exempt_reason=NULL,
				identity_version=identity_version+1 WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, userID); err != nil {
				return err
			}
			result, err = scanIdentityPasskey(tx.QueryRow(scoped, identityPasskeySelect+`
				WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, passkey.ID))
			if err != nil {
				return err
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &actorID, nil, "passkey.registered",
				reason, metadata, map[string]any{"passkey_id": passkey.ID, "device_name": passkey.DeviceName})
		})
	})
	return result, err
}

const identityPasskeySelect = `SELECT id,organization_id,user_id,credential_id,credential_ciphertext,
	attestation_type,transport,sign_count,clone_warning,backup_eligible,backup_state,device_name,aaguid,
	last_used_at,removed_at,version,created_at,updated_at FROM identity_passkeys `

func (r *identityLifecycleRepo) ListPasskeys(ctx context.Context, organizationID, userID string) ([]models.IdentityPasskey, error) {
	items := []models.IdentityPasskey{}
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		rows, err := q.Query(scoped, identityPasskeySelect+` WHERE organization_id=$1::uuid AND user_id=$2::uuid
			AND removed_at IS NULL ORDER BY created_at,id`, organizationID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanIdentityPasskey(rows)
			if err != nil {
				return err
			}
			items = append(items, *item)
		}
		return rows.Err()
	})
	return items, err
}

func (r *identityLifecycleRepo) GetPasskeyByCredentialID(ctx context.Context, organizationID, userID string, credentialID []byte) (*models.IdentityPasskey, error) {
	var result *models.IdentityPasskey
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		var err error
		result, err = scanIdentityPasskey(q.QueryRow(scoped, identityPasskeySelect+`
			WHERE organization_id=$1::uuid AND user_id=$2::uuid AND credential_id=$3 AND removed_at IS NULL`,
			organizationID, userID, credentialID))
		return classifyIdentityRead(err)
	})
	return result, err
}

func (r *identityLifecycleRepo) UpdatePasskeyAuthentication(ctx context.Context, organizationID, userID, passkeyID, credentialCipher string, signCount uint32, cloneWarning, backupState bool, challengeHash, purpose string, now time.Time, metadata models.IdentityRequestMetadata) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			if err := consumeIdentityChallengeTx(scoped, tx, organizationID, userID, challengeHash, purpose, now); err != nil {
				return err
			}
			tag, err := tx.Exec(scoped, `UPDATE identity_passkeys SET credential_ciphertext=$4,sign_count=GREATEST(sign_count,$5),
				clone_warning=clone_warning OR $6,backup_state=$7,last_used_at=$8,version=version+1
				WHERE organization_id=$1::uuid AND user_id=$2::uuid AND id=$3::uuid AND removed_at IS NULL
				AND sign_count BETWEEN 0 AND 4294967295
				AND ($5=0 OR sign_count=0 OR $5>sign_count OR $6)`, organizationID, userID, passkeyID,
				credentialCipher, signCount, cloneWarning, backupState, now)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrIdentityConflict
			}
			eventType := "passkey.authenticated"
			if cloneWarning {
				eventType = "passkey.clone_detected"
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &userID, nil, eventType,
				"Passkey authentication completed", metadata, map[string]any{"passkey_id": passkeyID, "clone_warning": cloneWarning})
		})
	})
}

func consumeIdentityChallengeTx(ctx context.Context, tx pgx.Tx, organizationID, userID, challengeHash, purpose string, now time.Time) error {
	tag, err := tx.Exec(ctx, `UPDATE identity_authentication_challenges SET consumed_at=$5
		WHERE organization_id=$1::uuid AND user_id=$2::uuid AND token_hash=$3 AND purpose=$4
		AND consumed_at IS NULL AND expires_at>$5 AND failed_attempts<max_attempts`,
		organizationID, userID, challengeHash, purpose, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrIdentityConsumed
	}
	return nil
}

func (r *identityLifecycleRepo) RemovePasskey(ctx context.Context, organizationID, userID, passkeyID, actorID string, expectedVersion int64, reason string, metadata models.IdentityRequestMetadata) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			if err := ensureCanRemoveIdentityFactor(scoped, tx, organizationID, userID, "passkey", passkeyID); err != nil {
				return err
			}
			tag, err := tx.Exec(scoped, `UPDATE identity_passkeys SET removed_at=NOW(),removed_by=$4::uuid,
				removal_reason=$6,version=version+1 WHERE organization_id=$1::uuid AND user_id=$2::uuid
				AND id=$3::uuid AND version=$5 AND removed_at IS NULL`, organizationID, userID, passkeyID,
				actorID, expectedVersion, reason)
			if err != nil {
				return classifyIdentityWrite(err)
			}
			if tag.RowsAffected() != 1 {
				return ErrIdentityVersion
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &actorID, nil, "passkey.removed",
				reason, metadata, map[string]any{"passkey_id": passkeyID})
		})
	})
}

func insertRecoveryCodes(ctx context.Context, tx pgx.Tx, organizationID, userID, factorID string, hashes []string) error {
	for index, hash := range hashes {
		if _, err := tx.Exec(ctx, `INSERT INTO identity_mfa_recovery_codes(organization_id,user_id,mfa_id,code_hash,position)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5)`, organizationID, userID, factorID, hash, index+1); err != nil {
			return classifyIdentityWrite(err)
		}
	}
	return nil
}

func ensureIdentityActiveUser(ctx context.Context, q database.Querier, organizationID, userID string) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE organization_id=$1::uuid AND id=$2::uuid
		AND status='active' AND deleted_at IS NULL)`, organizationID, userID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrIdentityNotFound
	}
	return nil
}

func ensureCanRemoveIdentityFactor(ctx context.Context, q database.Querier, organizationID, userID, removingMethod, factorID string) error {
	var policyRequires, another bool
	if err := q.QueryRow(ctx, `SELECT COALESCE(policy.require_mfa,false)
		OR COALESCE(policy.require_mfa_for_admins,true) AND (account.is_super_admin OR EXISTS(
			SELECT 1 FROM effective_user_roles assignment JOIN roles role ON role.id=assignment.role_id
			WHERE assignment.organization_id=account.organization_id AND assignment.user_id=account.id
			AND role.deleted_at IS NULL AND role.slug IN ('org_admin','super_admin')))
		FROM users account LEFT JOIN tenant_identity_policies policy ON policy.organization_id=account.organization_id
		WHERE account.organization_id=$1::uuid AND account.id=$2::uuid AND account.deleted_at IS NULL`,
		organizationID, userID).Scan(&policyRequires); err != nil {
		return err
	}
	if !policyRequires {
		return nil
	}
	if err := q.QueryRow(ctx, `SELECT (
		EXISTS(SELECT 1 FROM user_mfa WHERE organization_id=$1::uuid AND user_id=$2::uuid
			AND is_verified AND disabled_at IS NULL AND ($3<>'totp' OR id<>$4::uuid))
		OR EXISTS(SELECT 1 FROM identity_passkeys WHERE organization_id=$1::uuid AND user_id=$2::uuid
			AND removed_at IS NULL AND ($3<>'passkey' OR id<>$4::uuid)))`,
		organizationID, userID, removingMethod, factorID).Scan(&another); err != nil {
		return err
	}
	if !another {
		return ErrIdentityLastFactor
	}
	return nil
}

func scanIdentityMFA(row rowScanner) (*models.IdentityMFAFactor, error) {
	item := &models.IdentityMFAFactor{}
	var method string
	err := row.Scan(&item.ID, &item.OrganizationID, &item.UserID, &method, &item.DisplayName,
		&item.Primary, &item.Verified, &item.VerifiedAt, &item.LastUsedAt, &item.DisabledAt,
		&item.RecoveryCodes, &item.Version, &item.CreatedAt, &item.UpdatedAt, &item.SecretCipher, &item.LastTOTPStep)
	item.Method = models.IdentityMethod(method)
	return item, err
}

func scanIdentityPasskey(row rowScanner) (*models.IdentityPasskey, error) {
	item := &models.IdentityPasskey{}
	var signCount int64
	err := row.Scan(&item.ID, &item.OrganizationID, &item.UserID, &item.CredentialID, &item.CredentialCipher,
		&item.AttestationType, &item.Transports, &signCount, &item.CloneWarning, &item.BackupEligible,
		&item.BackupState, &item.DeviceName, &item.AAGUID, &item.LastUsedAt, &item.RemovedAt,
		&item.Version, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	// PostgreSQL BIGINT can contain values outside WebAuthn's uint32 domain.
	// Validate before narrowing so corrupt/legacy rows cannot reset replay
	// protection by wrapping to zero (or another apparently valid counter).
	if signCount < 0 || signCount > math.MaxUint32 {
		return nil, errors.New("passkey signature counter is outside the WebAuthn range")
	}
	item.SignCount = uint32(signCount)
	return item, nil
}
