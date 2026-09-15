package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
)

func (r *identityLifecycleRepo) IssueInvitation(ctx context.Context, invitation models.IdentityInvitation, metadata models.IdentityRequestMetadata, reason string) (*models.IdentityInvitation, error) {
	var result *models.IdentityInvitation
	err := r.withTenant(ctx, invitation.OrganizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			account, err := scanIdentityAccount(tx.QueryRow(scoped, identityAccountSelect+`
				WHERE u.organization_id=$1::uuid AND u.id=$2::uuid AND u.deleted_at IS NULL FOR UPDATE`,
				invitation.OrganizationID, invitation.UserID))
			if err != nil {
				return classifyIdentityRead(err)
			}
			if account.User.Status != models.UserStatusPendingVerification ||
				account.InvitationStatus == models.DirectoryInvitationAccepted || account.InvitationStatus == models.DirectoryInvitationNotRequired {
				return ErrIdentityConflict
			}
			if _, err := tx.Exec(scoped, `UPDATE identity_invitations SET revoked_at=$3
				WHERE organization_id=$1::uuid AND user_id=$2::uuid AND accepted_at IS NULL AND revoked_at IS NULL`,
				invitation.OrganizationID, invitation.UserID, invitation.CreatedAt); err != nil {
				return fmt.Errorf("revoke prior invitation: %w", err)
			}
			if invitation.ID == "" {
				invitation.ID = uuid.NewString()
			}
			err = tx.QueryRow(scoped, `INSERT INTO identity_invitations(id,organization_id,user_id,email,token_hash,
				delivery_secret_ciphertext,expires_at,created_by,request_id,created_at)
				VALUES($1::uuid,$2::uuid,$3::uuid,lower($4),$5,$6,$7,$8::uuid,NULLIF($9,''),$10)
				RETURNING created_at`, invitation.ID, invitation.OrganizationID, invitation.UserID, account.User.Email,
				invitation.TokenHash, invitation.DeliveryCipher, invitation.ExpiresAt, invitation.CreatedBy,
				metadata.RequestID, invitation.CreatedAt).Scan(&invitation.CreatedAt)
			if err != nil {
				return classifyIdentityWrite(err)
			}
			invitation.Email = account.User.Email
			invitation.DeliveryQueued = true
			if _, err := tx.Exec(scoped, `UPDATE users SET invitation_status='sent',invited_at=$3,
				invitation_expires_at=$4,identity_version=identity_version+1,version=version+1,updated_by=$5::uuid
				WHERE organization_id=$1::uuid AND id=$2::uuid`, invitation.OrganizationID, invitation.UserID,
				invitation.CreatedAt, invitation.ExpiresAt, invitation.CreatedBy); err != nil {
				return fmt.Errorf("mark invitation sent: %w", err)
			}
			result = &invitation
			return r.recordEvent(scoped, tx, invitation.OrganizationID, &invitation.UserID, &invitation.CreatedBy, nil,
				"invitation.issued", reason, metadata, map[string]any{"invitation_id": invitation.ID,
					"delivery_kind": "identity_invitation", "delivery_id": invitation.ID, "expires_at": invitation.ExpiresAt})
		})
	})
	return result, err
}

func (r *identityLifecycleRepo) AcceptInvitation(ctx context.Context, organizationID, tokenHash, passwordHash, firstName, lastName string, now time.Time, metadata models.IdentityRequestMetadata) (*models.IdentityAcceptanceResult, error) {
	var result *models.IdentityAcceptanceResult
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			var invitationID, userID string
			var expiresAt time.Time
			var acceptedAt, revokedAt *time.Time
			err := tx.QueryRow(scoped, `SELECT id,user_id,expires_at,accepted_at,revoked_at FROM identity_invitations
				WHERE organization_id=$1::uuid AND token_hash=$2 FOR UPDATE`, organizationID, tokenHash).
				Scan(&invitationID, &userID, &expiresAt, &acceptedAt, &revokedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrIdentityNotFound
			}
			if err != nil {
				return fmt.Errorf("load invitation: %w", err)
			}
			if acceptedAt != nil || revokedAt != nil {
				return ErrIdentityConsumed
			}
			if !now.Before(expiresAt) {
				if _, updateErr := tx.Exec(scoped, `UPDATE identity_invitations SET revoked_at=$3
					WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, invitationID, now); updateErr != nil {
					return updateErr
				}
				if _, updateErr := tx.Exec(scoped, `UPDATE users SET invitation_status='expired',identity_version=identity_version+1,
					version=version+1 WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, userID); updateErr != nil {
					return updateErr
				}
				return ErrIdentityExpired
			}
			var email string
			err = tx.QueryRow(scoped, `UPDATE users SET password_hash=$3,status='active',first_name=COALESCE(NULLIF($4,''),first_name),
				last_name=COALESCE(NULLIF($5,''),last_name),email_verified_at=$6,password_changed_at=$6,
				invitation_status='accepted',invitation_expires_at=NULL,identity_version=identity_version+1,version=version+1
				WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL AND status='pending_verification'
				RETURNING email`, organizationID, userID, passwordHash, firstName, lastName, now).Scan(&email)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrIdentityConflict
			}
			if err != nil {
				return classifyIdentityWrite(err)
			}
			if _, err := tx.Exec(scoped, `UPDATE identity_invitations SET accepted_at=$3,delivery_secret_ciphertext=''
				WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, invitationID, now); err != nil {
				return fmt.Errorf("consume invitation: %w", err)
			}
			if _, err := tx.Exec(scoped, `UPDATE identity_email_verification_tokens SET invalidated_at=$3,
				delivery_secret_ciphertext='' WHERE organization_id=$1::uuid AND user_id=$2::uuid
				AND verified_at IS NULL AND invalidated_at IS NULL`, organizationID, userID, now); err != nil {
				return fmt.Errorf("invalidate email tokens: %w", err)
			}
			result = &models.IdentityAcceptanceResult{UserID: userID, OrganizationID: organizationID, EmailVerifiedAt: now}
			return r.recordEvent(scoped, tx, organizationID, &userID, &userID, nil, "invitation.accepted",
				"Invitation accepted and email verified", metadata, map[string]any{"invitation_id": invitationID})
		})
	})
	return result, err
}

func (r *identityLifecycleRepo) IssueEmailVerification(ctx context.Context, token models.IdentityCredentialToken) (bool, error) {
	return r.issueAccountToken(ctx, token, true)
}

func (r *identityLifecycleRepo) IssuePasswordReset(ctx context.Context, token models.IdentityCredentialToken) (bool, error) {
	return r.issueAccountToken(ctx, token, false)
}

func (r *identityLifecycleRepo) issueAccountToken(ctx context.Context, token models.IdentityCredentialToken, emailVerification bool) (bool, error) {
	issued := false
	err := r.withTenant(ctx, token.OrganizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			account, err := scanIdentityAccount(tx.QueryRow(scoped, identityAccountSelect+`
				WHERE u.organization_id=$1::uuid AND lower(u.email)=lower($2) AND u.deleted_at IS NULL FOR UPDATE`,
				token.OrganizationID, token.Email))
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("find token account: %w", err)
			}
			if account.User.Status == models.UserStatusInactive {
				return nil
			}
			token.UserID = account.User.ID
			if emailVerification {
				if account.EmailVerifiedAt != nil {
					return nil
				}
				if _, err := tx.Exec(scoped, `UPDATE identity_email_verification_tokens SET invalidated_at=$3,
					delivery_secret_ciphertext='' WHERE organization_id=$1::uuid AND user_id=$2::uuid
					AND verified_at IS NULL AND invalidated_at IS NULL`, token.OrganizationID, token.UserID, token.IssuedAt); err != nil {
					return err
				}
				_, err = tx.Exec(scoped, `INSERT INTO identity_email_verification_tokens(id,organization_id,user_id,token_hash,
					delivery_secret_ciphertext,expires_at,request_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,NULLIF($7,''))`,
					token.ID, token.OrganizationID, token.UserID, token.TokenHash, token.DeliveryCipher, token.ExpiresAt, token.Metadata.RequestID)
			} else {
				if account.User.PasswordHash == "" || account.User.Status != models.UserStatusActive {
					return nil
				}
				if _, err := tx.Exec(scoped, `UPDATE password_reset_tokens SET invalidated_at=$3,
					delivery_secret_ciphertext='' WHERE organization_id=$1::uuid AND user_id=$2::uuid
					AND used_at IS NULL AND invalidated_at IS NULL`, token.OrganizationID, token.UserID, token.IssuedAt); err != nil {
					return err
				}
				_, err = tx.Exec(scoped, `INSERT INTO password_reset_tokens(id,user_id,organization_id,token_hash,
					delivery_secret_ciphertext,expires_at,requested_ip,requested_user_agent,request_id)
					VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,NULLIF($7,'')::inet,NULLIF($8,''),NULLIF($9,''))`,
					token.ID, token.UserID, token.OrganizationID, token.TokenHash, token.DeliveryCipher, token.ExpiresAt,
					token.Metadata.IPAddress, token.Metadata.UserAgent, token.Metadata.RequestID)
			}
			if err != nil {
				return classifyIdentityWrite(err)
			}
			issued = true
			kind, eventType, reason := "password_reset", "password_reset.requested", "Password reset requested"
			if emailVerification {
				kind, eventType, reason = "email_verification", "email_verification.requested", "Email verification requested"
			}
			return r.recordEvent(scoped, tx, token.OrganizationID, &token.UserID, nil, nil, eventType, reason,
				token.Metadata, map[string]any{"delivery_kind": kind, "delivery_id": token.ID, "expires_at": token.ExpiresAt})
		})
	})
	return issued, err
}

func (r *identityLifecycleRepo) VerifyEmail(ctx context.Context, organizationID, tokenHash string, now time.Time, metadata models.IdentityRequestMetadata) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			var id, userID string
			var expires time.Time
			var verified, invalidated *time.Time
			err := tx.QueryRow(scoped, `SELECT id,user_id,expires_at,verified_at,invalidated_at
				FROM identity_email_verification_tokens WHERE organization_id=$1::uuid AND token_hash=$2 FOR UPDATE`,
				organizationID, tokenHash).Scan(&id, &userID, &expires, &verified, &invalidated)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrIdentityNotFound
			}
			if err != nil {
				return err
			}
			if verified != nil || invalidated != nil {
				return ErrIdentityConsumed
			}
			if !now.Before(expires) {
				return ErrIdentityExpired
			}
			if _, err := tx.Exec(scoped, `UPDATE identity_email_verification_tokens SET verified_at=$3,
				delivery_secret_ciphertext='' WHERE organization_id=$1::uuid AND id=$2::uuid`, organizationID, id, now); err != nil {
				return err
			}
			if _, err := tx.Exec(scoped, `UPDATE users SET email_verified_at=COALESCE(email_verified_at,$3),
				status=CASE WHEN status='pending_verification' THEN 'active'::user_status ELSE status END,
				identity_version=identity_version+1 WHERE organization_id=$1::uuid AND id=$2::uuid AND deleted_at IS NULL`,
				organizationID, userID, now); err != nil {
				return err
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &userID, nil, "email_verification.completed",
				"Email address verified", metadata, map[string]any{"verification_id": id})
		})
	})
}

func (r *identityLifecycleRepo) ResetPassword(ctx context.Context, organizationID, tokenHash, passwordHash string, now time.Time, metadata models.IdentityRequestMetadata) error {
	return r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			var id, userID string
			var expires time.Time
			var used, invalidated *time.Time
			err := tx.QueryRow(scoped, `SELECT id,user_id,expires_at,used_at,invalidated_at FROM password_reset_tokens
				WHERE organization_id=$1::uuid AND token_hash=$2 FOR UPDATE`, organizationID, tokenHash).
				Scan(&id, &userID, &expires, &used, &invalidated)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrIdentityNotFound
			}
			if err != nil {
				return err
			}
			if used != nil || invalidated != nil {
				return ErrIdentityConsumed
			}
			if !now.Before(expires) {
				return ErrIdentityExpired
			}
			if _, err := tx.Exec(scoped, `UPDATE password_reset_tokens SET used_at=$3,used_ip=NULLIF($4,'')::inet,
				delivery_secret_ciphertext='' WHERE organization_id=$1::uuid AND id=$2::uuid`,
				organizationID, id, now, metadata.IPAddress); err != nil {
				return err
			}
			tag, err := tx.Exec(scoped, `UPDATE users SET password_hash=$3,password_changed_at=$4,failed_login_attempts=0,
				locked_until=NULL,identity_version=identity_version+1 WHERE organization_id=$1::uuid AND id=$2::uuid
				AND status<>'inactive' AND deleted_at IS NULL`, organizationID, userID, passwordHash, now)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrIdentityConflict
			}
			if _, err := tx.Exec(scoped, `UPDATE user_sessions SET revoked_at=COALESCE(revoked_at,$3),
				revoke_reason=COALESCE(revoke_reason,'Password reset'),version=version+1
				WHERE organization_id=$1::uuid AND user_id=$2::uuid AND revoked_at IS NULL`, organizationID, userID, now); err != nil {
				return err
			}
			return r.recordEvent(scoped, tx, organizationID, &userID, &userID, nil, "password.reset",
				"Password reset completed; all sessions revoked", metadata, map[string]any{"reset_id": id})
		})
	})
}

func normalizeIdentityEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
