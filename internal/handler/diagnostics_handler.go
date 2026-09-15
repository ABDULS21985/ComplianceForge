package handler

import (
	"context"
	"net/http"
	"reflect"

	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

type DiagnosticsService interface {
	GetSnapshot(context.Context, string) (*models.DiagnosticsSnapshot, error)
}

type DiagnosticsHandler struct {
	service        DiagnosticsService
	supportBundles SupportBundleService
}

type DiagnosticsHandlerOption func(*DiagnosticsHandler)

func NewDiagnosticsHandler(service DiagnosticsService, options ...DiagnosticsHandlerOption) *DiagnosticsHandler {
	handler := &DiagnosticsHandler{service: service}
	for _, option := range options {
		if option != nil {
			option(handler)
		}
	}
	return handler
}

func (h *DiagnosticsHandler) Ready() bool {
	if h == nil || h.service == nil {
		return false
	}
	value := reflect.ValueOf(h.service)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

// GetSnapshot handles GET /api/v1/settings/diagnostics. The endpoint is
// protected by settings:read and returns no raw dependency/configuration
// values. Responses are never cacheable because they contain live health data.
func (h *DiagnosticsHandler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	organizationID := middleware.GetOrgIDFromContext(r.Context())
	if organizationID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}
	snapshot, err := h.service.GetSnapshot(r.Context(), organizationID)
	if err != nil {
		// Dependency errors can contain endpoints or driver detail. The caller
		// receives a stable response and logs retain only safe correlation data.
		log.Error().
			Str("organization_id", organizationID).
			Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Msg("administrator diagnostics snapshot failed")
		writeError(w, http.StatusServiceUnavailable, "Diagnostics are temporarily unavailable", "")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeClassifiedJSON(w, r, http.StatusOK, "settings", map[string]any{"data": snapshot})
}
