package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
)

var (
	ErrIdentityNotFound      = errors.New("identity record not found")
	ErrIdentityConflict      = errors.New("identity state conflict")
	ErrIdentityVersion       = errors.New("identity version conflict")
	ErrIdentityExpired       = errors.New("identity credential expired")
	ErrIdentityConsumed      = errors.New("identity credential already consumed")
	ErrIdentityAttempts      = errors.New("identity challenge attempts exhausted")
	ErrIdentityLastFactor    = errors.New("last required MFA factor is protected")
	ErrIdentityAdminRecovery = errors.New("another MFA-enabled administrator is required")
)

// IdentityLifecycleRepository is the tenant-safe persistence contract for
// credential lifecycles. Raw invitation/reset secrets are never accepted by
// this boundary: callers provide only hashes and tenant-bound ciphertext.
type IdentityLifecycleRepository interface {
	GetAccount(context.Context, string, string) (*models.IdentityAccountState, error)
	GetAccountByEmail(context.Context, string, string) (*models.IdentityAccountState, error)
	GetPolicy(context.Context, string) (*models.IdentityPolicy, error)
	UpdatePolicy(context.Context, string, string, models.IdentityPolicyPatch) (*models.IdentityPolicy, error)

	IssueInvitation(context.Context, models.IdentityInvitation, models.IdentityRequestMetadata, string) (*models.IdentityInvitation, error)
	AcceptInvitation(context.Context, string, string, string, string, string, time.Time, models.IdentityRequestMetadata) (*models.IdentityAcceptanceResult, error)
	IssueEmailVerification(context.Context, models.IdentityCredentialToken) (bool, error)
	VerifyEmail(context.Context, string, string, time.Time, models.IdentityRequestMetadata) error
	IssuePasswordReset(context.Context, models.IdentityCredentialToken) (bool, error)
	ResetPassword(context.Context, string, string, string, time.Time, models.IdentityRequestMetadata) error

	ListSessions(context.Context, string, string, string) ([]models.IdentitySession, error)
	ResolveSession(context.Context, string, string, string) (*models.IdentitySession, error)
	RevokeSession(context.Context, string, string, string, string, int64, string, models.IdentityRequestMetadata) error
	RevokeAllSessions(context.Context, string, string, string, string, bool, string, models.IdentityRequestMetadata) (int, error)

	CreateTOTPFactor(context.Context, string, string, string, string, []byte, string, models.IdentityRequestMetadata) (*models.IdentityMFAFactor, error)
	GetMFAFactor(context.Context, string, string, string) (*models.IdentityMFAFactor, error)
	ListMFAFactors(context.Context, string, string) ([]models.IdentityMFAFactor, error)
	VerifyTOTPFactor(context.Context, string, string, string, int64, int64, []string, string, models.IdentityRequestMetadata) (*models.IdentityMFAFactor, error)
	UseTOTPFactor(context.Context, string, string, string, int64, time.Time) error
	ListRecoveryCodeHashes(context.Context, string, string) (map[string]string, error)
	ConsumeRecoveryCode(context.Context, string, string, string, string, time.Time) error
	ReplaceRecoveryCodes(context.Context, string, string, string, []string, string, models.IdentityRequestMetadata) error
	DisableMFAFactor(context.Context, string, string, string, string, int64, string, models.IdentityRequestMetadata) error
	AdminResetMFA(context.Context, string, string, string, string, time.Time, models.IdentityRequestMetadata) error

	CreatePasskey(context.Context, string, string, string, *models.IdentityPasskey, string, time.Time, string, models.IdentityRequestMetadata) (*models.IdentityPasskey, error)
	ListPasskeys(context.Context, string, string) ([]models.IdentityPasskey, error)
	GetPasskeyByCredentialID(context.Context, string, string, []byte) (*models.IdentityPasskey, error)
	UpdatePasskeyAuthentication(context.Context, string, string, string, string, uint32, bool, bool, string, string, time.Time, models.IdentityRequestMetadata) error
	RemovePasskey(context.Context, string, string, string, string, int64, string, models.IdentityRequestMetadata) error

	CreateChallenge(context.Context, *models.IdentityAuthenticationChallenge) error
	GetChallenge(context.Context, string, string, string) (*models.IdentityAuthenticationChallenge, error)
	FailChallenge(context.Context, string, string, string, time.Time) error
	ConsumeChallenge(context.Context, string, string, string, time.Time) error
	CreateStepUpGrant(context.Context, *models.IdentityStepUpGrant, string) error
	ConsumeStepUpGrant(context.Context, string, string, string, string, string, time.Time) error

	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.IdentitySecurityEvent, int, error)
}

type identityLifecycleRepo struct {
	pool        *pgxpool.Pool
	outbox      queuepkg.OutboxEnqueuer
	outboxQueue string
}

var _ IdentityLifecycleRepository = (*identityLifecycleRepo)(nil)

func NewIdentityLifecycleRepository(pool *pgxpool.Pool, outbox queuepkg.OutboxEnqueuer, outboxQueue string) (IdentityLifecycleRepository, error) {
	if pool == nil {
		return nil, errors.New("identity lifecycle database pool is required")
	}
	if outbox == nil {
		return nil, errors.New("identity lifecycle outbox is required")
	}
	outboxQueue = strings.TrimSpace(outboxQueue)
	if outboxQueue == "" {
		return nil, errors.New("identity lifecycle outbox queue is required")
	}
	return &identityLifecycleRepo{pool: pool, outbox: outbox, outboxQueue: outboxQueue}, nil
}

func (r *identityLifecycleRepo) GetAccount(ctx context.Context, organizationID, userID string) (*models.IdentityAccountState, error) {
	var result *models.IdentityAccountState
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		var err error
		result, err = scanIdentityAccount(q.QueryRow(scoped, identityAccountSelect+`
			WHERE u.organization_id=$1::uuid AND u.id=$2::uuid AND u.deleted_at IS NULL`, organizationID, userID))
		return classifyIdentityRead(err)
	})
	return result, err
}

func (r *identityLifecycleRepo) GetAccountByEmail(ctx context.Context, organizationID, email string) (*models.IdentityAccountState, error) {
	var result *models.IdentityAccountState
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		var err error
		result, err = scanIdentityAccount(q.QueryRow(scoped, identityAccountSelect+`
			WHERE u.organization_id=$1::uuid AND lower(u.email)=lower($2) AND u.deleted_at IS NULL`, organizationID, email))
		return classifyIdentityRead(err)
	})
	return result, err
}

const identityAccountSelect = `SELECT u.id,u.organization_id,u.email,COALESCE(u.password_hash,''),
	COALESCE(u.first_name,''),COALESCE(u.last_name,''),COALESCE(u.job_title,''),COALESCE(u.department,''),
	COALESCE(u.phone,''),COALESCE(u.avatar_url,''),u.status::text,u.is_super_admin,COALESCE(u.timezone,''),
	COALESCE(u.language,'en'),u.last_login_at,
	COALESCE((SELECT role.slug FROM effective_user_roles ur JOIN roles role ON role.id=ur.role_id AND role.deleted_at IS NULL
		WHERE ur.user_id=u.id AND ur.organization_id=u.organization_id ORDER BY ur.assigned_at,role.slug LIMIT 1),
		CASE WHEN u.is_super_admin THEN 'super_admin' ELSE 'viewer' END),
	EXISTS(SELECT 1 FROM user_mfa factor WHERE factor.organization_id=u.organization_id AND factor.user_id=u.id
		AND factor.is_verified AND factor.disabled_at IS NULL),
	u.created_at,u.updated_at,u.deleted_at,u.email_verified_at,u.mfa_exempt_until,
	COALESCE(u.mfa_exempt_reason,''),u.identity_version,u.invitation_status::text
	FROM users u `

// scanIdentityAccount cannot append columns to userSelect directly because the
// shared scanner deliberately owns its fixed projection. Keep a standalone
// projection so identity-specific state remains explicit.
func scanIdentityAccount(row rowScanner) (*models.IdentityAccountState, error) {
	state := &models.IdentityAccountState{}
	user := &state.User
	var status, role, invitation string
	if err := row.Scan(
		&user.ID, &user.OrganizationID, &user.Email, &user.PasswordHash,
		&user.FirstName, &user.LastName, &user.JobTitle, &user.Department,
		&user.Phone, &user.AvatarURL, &status, &user.IsSuperAdmin, &user.Timezone,
		&user.Language, &user.LastLoginAt, &role, &user.MFAEnabled,
		&user.CreatedAt, &user.UpdatedAt, &user.DeletedAt,
		&state.EmailVerifiedAt, &state.MFAExemptUntil, &state.MFAExemptReason, &state.IdentityVersion, &invitation,
	); err != nil {
		return nil, err
	}
	user.Status = models.UserStatus(status)
	user.Role = models.UserRole(role)
	state.InvitationStatus = models.DirectoryInvitationStatus(invitation)
	return state, nil
}

func (r *identityLifecycleRepo) GetPolicy(ctx context.Context, organizationID string) (*models.IdentityPolicy, error) {
	var result *models.IdentityPolicy
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		policy, err := scanIdentityPolicy(q.QueryRow(scoped, `SELECT organization_id,require_mfa,require_mfa_for_admins,
			allowed_methods,enrollment_grace_hours,authentication_challenge_minutes,step_up_ttl_minutes,
			version,updated_by,update_reason,created_at,updated_at FROM tenant_identity_policies WHERE organization_id=$1::uuid`, organizationID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIdentityNotFound
		}
		result = policy
		return err
	})
	return result, err
}

func (r *identityLifecycleRepo) UpdatePolicy(ctx context.Context, organizationID, actorID string, patch models.IdentityPolicyPatch) (*models.IdentityPolicy, error) {
	var result *models.IdentityPolicy
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		return withTransaction(scoped, q, func(tx pgx.Tx) error {
			current, err := scanIdentityPolicy(tx.QueryRow(scoped, `SELECT organization_id,require_mfa,require_mfa_for_admins,
				allowed_methods,enrollment_grace_hours,authentication_challenge_minutes,step_up_ttl_minutes,
				version,updated_by,update_reason,created_at,updated_at FROM tenant_identity_policies
				WHERE organization_id=$1::uuid FOR UPDATE`, organizationID))
			creating := errors.Is(err, pgx.ErrNoRows)
			if creating {
				if patch.ExpectedVersion != 0 {
					return ErrIdentityVersion
				}
				current = &models.IdentityPolicy{OrganizationID: organizationID, RequireMFAForAdmins: true,
					AllowedMethods: []string{"totp", "passkey"}, EnrollmentGraceHours: 24,
					AuthenticationChallengeMins: 5, StepUpTTLMinutes: 10}
			} else if err != nil {
				return err
			} else if current.Version != patch.ExpectedVersion {
				return ErrIdentityVersion
			}
			applyIdentityPolicyPatch(current, patch)
			if creating {
				err = tx.QueryRow(scoped, `INSERT INTO tenant_identity_policies(
					organization_id,require_mfa,require_mfa_for_admins,allowed_methods,enrollment_grace_hours,
					authentication_challenge_minutes,step_up_ttl_minutes,updated_by,update_reason)
					VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8::uuid,$9)
					RETURNING organization_id,require_mfa,require_mfa_for_admins,allowed_methods,enrollment_grace_hours,
					authentication_challenge_minutes,step_up_ttl_minutes,version,updated_by,update_reason,created_at,updated_at`,
					organizationID, current.RequireMFA, current.RequireMFAForAdmins, current.AllowedMethods,
					current.EnrollmentGraceHours, current.AuthenticationChallengeMins, current.StepUpTTLMinutes,
					actorID, patch.Reason).Scan(&current.OrganizationID, &current.RequireMFA,
					&current.RequireMFAForAdmins, &current.AllowedMethods, &current.EnrollmentGraceHours,
					&current.AuthenticationChallengeMins, &current.StepUpTTLMinutes, &current.Version,
					&current.UpdatedBy, &current.UpdateReason, &current.CreatedAt, &current.UpdatedAt)
			} else {
				err = tx.QueryRow(scoped, `UPDATE tenant_identity_policies SET require_mfa=$2,
					require_mfa_for_admins=$3,allowed_methods=$4,enrollment_grace_hours=$5,
					authentication_challenge_minutes=$6,step_up_ttl_minutes=$7,updated_by=$8::uuid,
					update_reason=$9,version=version+1 WHERE organization_id=$1::uuid AND version=$10
					RETURNING organization_id,require_mfa,require_mfa_for_admins,allowed_methods,enrollment_grace_hours,
					authentication_challenge_minutes,step_up_ttl_minutes,version,updated_by,update_reason,created_at,updated_at`,
					organizationID, current.RequireMFA, current.RequireMFAForAdmins, current.AllowedMethods,
					current.EnrollmentGraceHours, current.AuthenticationChallengeMins, current.StepUpTTLMinutes,
					actorID, patch.Reason, patch.ExpectedVersion).Scan(&current.OrganizationID, &current.RequireMFA,
					&current.RequireMFAForAdmins, &current.AllowedMethods, &current.EnrollmentGraceHours,
					&current.AuthenticationChallengeMins, &current.StepUpTTLMinutes, &current.Version,
					&current.UpdatedBy, &current.UpdateReason, &current.CreatedAt, &current.UpdatedAt)
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrIdentityVersion
			}
			if err != nil {
				return classifyIdentityWrite(err)
			}
			result = current
			return r.recordEvent(scoped, tx, organizationID, nil, stringPtr(actorID), nil, "identity_policy.updated",
				patch.Reason, models.IdentityRequestMetadata{}, map[string]any{"version": current.Version})
		})
	})
	return result, err
}

func (r *identityLifecycleRepo) ListEvents(ctx context.Context, organizationID, userID string, pagination models.PaginationRequest) ([]models.IdentitySecurityEvent, int, error) {
	items := []models.IdentitySecurityEvent{}
	var total int
	err := r.withTenant(ctx, organizationID, func(scoped context.Context, q database.Querier) error {
		where := `organization_id=$1::uuid`
		args := []any{organizationID}
		if userID != "" {
			where += ` AND user_id=$2::uuid`
			args = append(args, userID)
		}
		if err := q.QueryRow(scoped, `SELECT COUNT(*) FROM identity_security_events WHERE `+where, args...).Scan(&total); err != nil {
			return fmt.Errorf("count identity events: %w", err)
		}
		args = append(args, pagination.PageSize, (pagination.Page-1)*pagination.PageSize)
		rows, err := q.Query(scoped, `SELECT id,organization_id,user_id,actor_user_id,session_id,event_type,reason,
			COALESCE(request_id,''),COALESCE(ip_address::text,''),COALESCE(user_agent,''),details,created_at
			FROM identity_security_events WHERE `+where+` ORDER BY created_at DESC,id DESC`+
			fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
		if err != nil {
			return fmt.Errorf("list identity events: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var item models.IdentitySecurityEvent
			if err := rows.Scan(&item.ID, &item.OrganizationID, &item.UserID, &item.ActorUserID, &item.SessionID,
				&item.EventType, &item.Reason, &item.RequestID, &item.IPAddress, &item.UserAgent, &item.Details, &item.CreatedAt); err != nil {
				return fmt.Errorf("scan identity event: %w", err)
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, total, err
}

func (r *identityLifecycleRepo) recordEvent(ctx context.Context, tx pgx.Tx, organizationID string, userID, actorID, sessionID *string, eventType, reason string, metadata models.IdentityRequestMetadata, details map[string]any) error {
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal identity event: %w", err)
	}
	eventID := uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO identity_security_events(id,organization_id,user_id,actor_user_id,session_id,
		event_type,reason,request_id,ip_address,user_agent,details) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,
		$6,$7,NULLIF($8,''),NULLIF($9,'')::inet,NULLIF($10,''),$11::jsonb)`, eventID, organizationID, userID, actorID,
		sessionID, eventType, reason, metadata.RequestID, metadata.IPAddress, metadata.UserAgent, encoded); err != nil {
		return fmt.Errorf("append identity event: %w", err)
	}
	payload := map[string]any{"type": "identity." + eventType, "severity": identityEventSeverity(eventType),
		"org_id": organizationID, "entity_type": "identity", "entity_id": eventID, "entity_ref": eventID,
		"data":      map[string]any{"event_id": eventID, "user_id": userID, "actor_user_id": actorID, "details": details},
		"timestamp": time.Now().UTC()}
	envelope, err := queuepkg.NewEnvelope("notification.event", organizationID, payload)
	if err != nil {
		return fmt.Errorf("create identity event envelope: %w", err)
	}
	envelope.CausationID = eventID
	envelope.Metadata = map[string]string{"entity_type": "identity", "event_type": eventType}
	if err := r.outbox.Enqueue(ctx, tx, r.outboxQueue, envelope); err != nil {
		return fmt.Errorf("enqueue identity event: %w", err)
	}
	return nil
}

func (r *identityLifecycleRepo) withTenant(ctx context.Context, organizationID string, fn func(context.Context, database.Querier) error) error {
	return database.WithTenantConnection(ctx, r.pool, organizationID, func(scoped context.Context) error {
		return fn(scoped, database.QuerierFromContext(scoped, r.pool))
	})
}

func scanIdentityPolicy(row rowScanner) (*models.IdentityPolicy, error) {
	item := &models.IdentityPolicy{}
	err := row.Scan(&item.OrganizationID, &item.RequireMFA, &item.RequireMFAForAdmins, &item.AllowedMethods,
		&item.EnrollmentGraceHours, &item.AuthenticationChallengeMins, &item.StepUpTTLMinutes, &item.Version,
		&item.UpdatedBy, &item.UpdateReason, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func applyIdentityPolicyPatch(policy *models.IdentityPolicy, patch models.IdentityPolicyPatch) {
	if patch.RequireMFA != nil {
		policy.RequireMFA = *patch.RequireMFA
	}
	if patch.RequireMFAForAdmins != nil {
		policy.RequireMFAForAdmins = *patch.RequireMFAForAdmins
	}
	if patch.AllowedMethods != nil {
		policy.AllowedMethods = patch.AllowedMethods
	}
	if patch.EnrollmentGraceHours != nil {
		policy.EnrollmentGraceHours = *patch.EnrollmentGraceHours
	}
	if patch.AuthenticationChallengeMins != nil {
		policy.AuthenticationChallengeMins = *patch.AuthenticationChallengeMins
	}
	if patch.StepUpTTLMinutes != nil {
		policy.StepUpTTLMinutes = *patch.StepUpTTLMinutes
	}
}

func classifyIdentityRead(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrIdentityNotFound
	}
	return err
}

func classifyIdentityWrite(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23503", "23505", "23514", "23P01":
			return fmt.Errorf("%w: %s", ErrIdentityConflict, postgresError.ConstraintName)
		}
	}
	return err
}

func identityEventSeverity(eventType string) string {
	switch eventType {
	case "password.reset", "mfa.admin_reset", "session.global_sign_out", "passkey.clone_detected":
		return "high"
	default:
		return "medium"
	}
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
