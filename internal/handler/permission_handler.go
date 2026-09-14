package handler

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
)

// PermissionService exposes the authenticated user's effective RBAC grants.
// It is intentionally narrower than the optional ABAC administration surface.
type PermissionService interface {
	GetUserPermissions(context.Context, string, string) (map[string][]string, error)
}

type PermissionHandler struct {
	service PermissionService
}

func NewPermissionHandler(service PermissionService) *PermissionHandler {
	return &PermissionHandler{service: service}
}

func (h *PermissionHandler) Ready() bool { return h != nil && h.service != nil }

// GetMyPermissions handles GET /access/my-permissions. Both identifiers come
// exclusively from verified authentication and tenant middleware context.
func (h *PermissionHandler) GetMyPermissions(w http.ResponseWriter, r *http.Request) {
	organizationID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if organizationID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	permissions, err := h.service.GetUserPermissions(r.Context(), organizationID, userID)
	if err != nil {
		log.Error().Err(err).
			Str("organization_id", organizationID).
			Str("user_id", userID).
			Msg("failed to read effective permissions")
		writeError(w, http.StatusInternalServerError, "Failed to get permissions", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": permissions})
}
