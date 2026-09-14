package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/database"
)

// RBACAuthorizer evaluates persisted user-role-permission assignments. It uses
// the request-scoped tenant querier when present, so the authorization lookup is
// subject to the same RLS boundary as the protected operation.
type RBACAuthorizer struct {
	pool *pgxpool.Pool
}

var _ authz.Authorizer = (*RBACAuthorizer)(nil)

func NewRBACAuthorizer(pool *pgxpool.Pool) (*RBACAuthorizer, error) {
	if pool == nil {
		return nil, errors.New("authorization database is required")
	}
	return &RBACAuthorizer{pool: pool}, nil
}

// Authorize defaults to deny. Active platform super-admins are allowed within
// the established tenant connection; every other principal needs an explicit
// persisted role permission for the requested resource and action.
func (a *RBACAuthorizer) Authorize(ctx context.Context, request authz.Request) (authz.Decision, error) {
	if a == nil {
		return authz.Decision{}, errors.New("authorization service is unavailable")
	}
	if _, err := uuid.Parse(request.SubjectID); err != nil {
		return authz.Decision{Reason: "invalid subject"}, nil
	}
	if _, err := uuid.Parse(request.OrganizationID); err != nil {
		return authz.Decision{Reason: "invalid organization"}, nil
	}
	resource := strings.ToLower(strings.TrimSpace(request.Resource))
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if !validPermissionComponent(resource) || !validPermissionComponent(action) {
		return authz.Decision{Reason: "invalid permission"}, nil
	}

	querier := database.QuerierFromContext(ctx, a.pool)
	if querier == nil {
		return authz.Decision{}, errors.New("authorization database is unavailable")
	}
	var allowed bool
	err := querier.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM users u
			WHERE u.id = $1
			  AND u.organization_id = $2
			  AND u.status = 'active'
			  AND u.deleted_at IS NULL
			  AND (
				u.is_super_admin
				OR EXISTS (
					SELECT 1
					FROM user_roles ur
					JOIN roles role
					  ON role.id = ur.role_id
					 AND role.deleted_at IS NULL
					 AND (role.organization_id IS NULL OR role.organization_id = u.organization_id)
					JOIN role_permissions rp ON rp.role_id = role.id
					JOIN permissions permission ON permission.id = rp.permission_id
					WHERE ur.user_id = u.id
					  AND ur.organization_id = u.organization_id
					  AND permission.resource = $3
					  AND permission.action = $4
				)
			  )
		)`, request.SubjectID, request.OrganizationID, resource, action).Scan(&allowed)
	if err != nil {
		return authz.Decision{}, fmt.Errorf("evaluate RBAC permission: %w", err)
	}
	if !allowed {
		return authz.Decision{Reason: "no matching role permission"}, nil
	}
	return authz.Decision{Allowed: true, Reason: "granted by role permission"}, nil
}

func validPermissionComponent(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}
