package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	protocol "github.com/complianceforge/platform/internal/scim"
	"github.com/complianceforge/platform/internal/service"
)

const (
	scimMediaType       = "application/scim+json"
	maximumSCIMBodySize = 1 << 20
)

// SCIMHandlerService is deliberately independent from browser authentication.
// The protocol methods receive only the tenant, token, and actor identities
// established by SCIMAuth; callers cannot supply tenant identity in payloads.
type SCIMHandlerService interface {
	CreateToken(context.Context, string, string, models.SCIMTokenCreateInput) (*models.SCIMTokenIssueResult, error)
	RotateToken(context.Context, string, string, string, models.SCIMTokenRotateInput) (*models.SCIMTokenIssueResult, error)
	RevokeToken(context.Context, string, string, string, models.SCIMTokenRevokeInput) error
	ListTokens(context.Context, string, models.PaginationRequest) ([]models.SCIMToken, int, error)

	ListUsers(context.Context, string, models.SCIMListRequest) ([]models.SCIMUser, int, error)
	GetUser(context.Context, string, string) (*models.SCIMUser, error)
	CreateUser(context.Context, string, string, string, *models.SCIMUser) (*models.SCIMUser, error)
	ReplaceUser(context.Context, string, string, string, string, int64, *models.SCIMUser) (*models.SCIMUser, error)
	PatchUser(context.Context, string, string, string, string, int64, models.SCIMPatchRequest) (*models.SCIMUser, error)
	DeleteUser(context.Context, string, string, string, string, int64) error
	ListGroups(context.Context, string, models.SCIMListRequest) ([]models.SCIMGroup, int, error)
	GetGroup(context.Context, string, string) (*models.SCIMGroup, error)
	CreateGroup(context.Context, string, string, string, *models.SCIMGroup) (*models.SCIMGroup, error)
	ReplaceGroup(context.Context, string, string, string, string, int64, *models.SCIMGroup) (*models.SCIMGroup, error)
	PatchGroup(context.Context, string, string, string, string, int64, models.SCIMPatchRequest) (*models.SCIMGroup, error)
	DeleteGroup(context.Context, string, string, string, string, int64) error
}

var _ SCIMHandlerService = (*service.SCIMService)(nil)

type SCIMHandler struct{ service SCIMHandlerService }

func NewSCIMHandler(scimService SCIMHandlerService) *SCIMHandler {
	return &SCIMHandler{service: scimService}
}

func (h *SCIMHandler) Ready() bool {
	if h == nil || h.service == nil {
		return false
	}
	value := reflect.ValueOf(h.service)
	return !((value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface || value.Kind() == reflect.Map ||
		value.Kind() == reflect.Slice || value.Kind() == reflect.Func) && value.IsNil())
}

func (h *SCIMHandler) ServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	writeSCIMResource(w, r, http.StatusOK, protocol.ServiceProviderConfig(), protocol.VersionETag(1), "")
}

func (h *SCIMHandler) ListSchemas(w http.ResponseWriter, r *http.Request) {
	items := protocol.SchemaDefinitions()
	writeSCIMJSON(w, http.StatusOK, models.SCIMListResponse{Schemas: []string{models.SCIMListResponseSchema},
		TotalResults: len(items), StartIndex: 1, ItemsPerPage: len(items), Resources: items})
}

func (h *SCIMHandler) GetSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "schema")
	for _, item := range protocol.SchemaDefinitions() {
		if item.ID == id {
			writeSCIMResource(w, r, http.StatusOK, item, protocol.VersionETag(1), "")
			return
		}
	}
	writeSCIMError(w, r, http.StatusNotFound, "", "Schema resource not found")
}

func (h *SCIMHandler) ListResourceTypes(w http.ResponseWriter, r *http.Request) {
	items := protocol.ResourceTypes()
	writeSCIMJSON(w, http.StatusOK, models.SCIMListResponse{Schemas: []string{models.SCIMListResponseSchema},
		TotalResults: len(items), StartIndex: 1, ItemsPerPage: len(items), Resources: items})
}

func (h *SCIMHandler) GetResourceType(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "resourceType")
	for _, item := range protocol.ResourceTypes() {
		if strings.EqualFold(item.ID, id) {
			writeSCIMResource(w, r, http.StatusOK, item, protocol.VersionETag(1), "")
			return
		}
	}
	writeSCIMError(w, r, http.StatusNotFound, "", "Resource type not found")
}

func (h *SCIMHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	request, ok := parseSCIMListRequest(w, r)
	if !ok {
		return
	}
	items, total, err := h.service.ListUsers(r.Context(), scimOrganizationID(r), request)
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMJSON(w, http.StatusOK, models.SCIMListResponse{Schemas: []string{models.SCIMListResponseSchema},
		TotalResults: total, StartIndex: request.StartIndex, ItemsPerPage: len(items), Resources: items})
}

func (h *SCIMHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetUser(r.Context(), scimOrganizationID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMResource(w, r, http.StatusOK, item, item.Meta.Version, "")
}

func (h *SCIMHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var input models.SCIMUser
	if !decodeSCIMJSON(w, r, &input) {
		return
	}
	item, err := h.service.CreateUser(r.Context(), scimOrganizationID(r), scimTokenID(r), scimActorID(r), &input)
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMResource(w, r, http.StatusCreated, item, item.Meta.Version, item.Meta.Location)
}

func (h *SCIMHandler) ReplaceUser(w http.ResponseWriter, r *http.Request) {
	version, ok := parseSCIMIfMatch(w, r)
	if !ok {
		return
	}
	var input models.SCIMUser
	if !decodeSCIMJSON(w, r, &input) {
		return
	}
	item, err := h.service.ReplaceUser(r.Context(), scimOrganizationID(r), chi.URLParam(r, "id"),
		scimTokenID(r), scimActorID(r), version, &input)
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMResource(w, r, http.StatusOK, item, item.Meta.Version, item.Meta.Location)
}

func (h *SCIMHandler) PatchUser(w http.ResponseWriter, r *http.Request) {
	version, ok := parseSCIMIfMatch(w, r)
	if !ok {
		return
	}
	var input models.SCIMPatchRequest
	if !decodeSCIMJSON(w, r, &input) {
		return
	}
	item, err := h.service.PatchUser(r.Context(), scimOrganizationID(r), chi.URLParam(r, "id"),
		scimTokenID(r), scimActorID(r), version, input)
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMResource(w, r, http.StatusOK, item, item.Meta.Version, item.Meta.Location)
}

func (h *SCIMHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	version, ok := parseSCIMIfMatch(w, r)
	if !ok {
		return
	}
	if err := h.service.DeleteUser(r.Context(), scimOrganizationID(r), chi.URLParam(r, "id"),
		scimTokenID(r), scimActorID(r), version); err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SCIMHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	request, ok := parseSCIMListRequest(w, r)
	if !ok {
		return
	}
	items, total, err := h.service.ListGroups(r.Context(), scimOrganizationID(r), request)
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMJSON(w, http.StatusOK, models.SCIMListResponse{Schemas: []string{models.SCIMListResponseSchema},
		TotalResults: total, StartIndex: request.StartIndex, ItemsPerPage: len(items), Resources: items})
}

func (h *SCIMHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetGroup(r.Context(), scimOrganizationID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMResource(w, r, http.StatusOK, item, item.Meta.Version, "")
}

func (h *SCIMHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var input models.SCIMGroup
	if !decodeSCIMJSON(w, r, &input) {
		return
	}
	item, err := h.service.CreateGroup(r.Context(), scimOrganizationID(r), scimTokenID(r), scimActorID(r), &input)
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMResource(w, r, http.StatusCreated, item, item.Meta.Version, item.Meta.Location)
}

func (h *SCIMHandler) ReplaceGroup(w http.ResponseWriter, r *http.Request) {
	version, ok := parseSCIMIfMatch(w, r)
	if !ok {
		return
	}
	var input models.SCIMGroup
	if !decodeSCIMJSON(w, r, &input) {
		return
	}
	item, err := h.service.ReplaceGroup(r.Context(), scimOrganizationID(r), chi.URLParam(r, "id"),
		scimTokenID(r), scimActorID(r), version, &input)
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMResource(w, r, http.StatusOK, item, item.Meta.Version, item.Meta.Location)
}

func (h *SCIMHandler) PatchGroup(w http.ResponseWriter, r *http.Request) {
	version, ok := parseSCIMIfMatch(w, r)
	if !ok {
		return
	}
	var input models.SCIMPatchRequest
	if !decodeSCIMJSON(w, r, &input) {
		return
	}
	item, err := h.service.PatchGroup(r.Context(), scimOrganizationID(r), chi.URLParam(r, "id"),
		scimTokenID(r), scimActorID(r), version, input)
	if err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	writeSCIMResource(w, r, http.StatusOK, item, item.Meta.Version, item.Meta.Location)
}

func (h *SCIMHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	version, ok := parseSCIMIfMatch(w, r)
	if !ok {
		return
	}
	if err := h.service.DeleteGroup(r.Context(), scimOrganizationID(r), chi.URLParam(r, "id"),
		scimTokenID(r), scimActorID(r), version); err != nil {
		writeSCIMServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// The token administration methods are mounted on the normal authenticated
// API, not the SCIM protocol router. Credentials are returned only on create
// and rotate and are excluded from all list payloads.
func (h *SCIMHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)
	items, total, err := h.service.ListTokens(r.Context(), middleware.GetOrgIDFromContext(r.Context()), pagination)
	if err != nil {
		writeSCIMAdministrationError(w, r, err)
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(pagination))
}

func (h *SCIMHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	var input models.SCIMTokenCreateInput
	if !decodeAdministrationJSON(w, r, &input) {
		return
	}
	item, err := h.service.CreateToken(r.Context(), middleware.GetOrgIDFromContext(r.Context()),
		middleware.GetUserIDFromContext(r.Context()), input)
	if err != nil {
		writeSCIMAdministrationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *SCIMHandler) RotateToken(w http.ResponseWriter, r *http.Request) {
	var input models.SCIMTokenRotateInput
	if !decodeAdministrationJSON(w, r, &input) {
		return
	}
	item, err := h.service.RotateToken(r.Context(), middleware.GetOrgIDFromContext(r.Context()),
		chi.URLParam(r, "id"), middleware.GetUserIDFromContext(r.Context()), input)
	if err != nil {
		writeSCIMAdministrationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *SCIMHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	var input models.SCIMTokenRevokeInput
	if !decodeAdministrationJSON(w, r, &input) {
		return
	}
	if err := h.service.RevokeToken(r.Context(), middleware.GetOrgIDFromContext(r.Context()),
		chi.URLParam(r, "id"), middleware.GetUserIDFromContext(r.Context()), input); err != nil {
		writeSCIMAdministrationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseSCIMListRequest(w http.ResponseWriter, r *http.Request) (models.SCIMListRequest, bool) {
	startIndex, count := 1, models.SCIMDefaultResultsPerPage
	var err error
	if value, present := r.URL.Query()["startIndex"]; present {
		if len(value) != 1 {
			writeSCIMError(w, r, http.StatusBadRequest, protocol.ErrorTypeInvalidValue, "startIndex must appear once")
			return models.SCIMListRequest{}, false
		}
		startIndex, err = strconv.Atoi(value[0])
		if err != nil {
			writeSCIMError(w, r, http.StatusBadRequest, protocol.ErrorTypeInvalidValue, "startIndex must be an integer")
			return models.SCIMListRequest{}, false
		}
	}
	if value, present := r.URL.Query()["count"]; present {
		if len(value) != 1 {
			writeSCIMError(w, r, http.StatusBadRequest, protocol.ErrorTypeInvalidValue, "count must appear once")
			return models.SCIMListRequest{}, false
		}
		count, err = strconv.Atoi(value[0])
		if err != nil {
			writeSCIMError(w, r, http.StatusBadRequest, protocol.ErrorTypeInvalidValue, "count must be an integer")
			return models.SCIMListRequest{}, false
		}
	}
	request, err := protocol.NormalizeListRequest(startIndex, count, r.URL.Query().Get("filter"))
	if err != nil {
		typeName := protocol.ErrorTypeInvalidValue
		if errors.Is(err, protocol.ErrInvalidFilter) {
			typeName = protocol.ErrorTypeInvalidFilter
		}
		writeSCIMError(w, r, http.StatusBadRequest, typeName, err.Error())
		return models.SCIMListRequest{}, false
	}
	return request, true
}

func parseSCIMIfMatch(w http.ResponseWriter, r *http.Request) (int64, bool) {
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if value == "" {
		writeSCIMError(w, r, http.StatusPreconditionRequired, protocol.ErrorTypeMutability,
			"If-Match with the current resource version is required")
		return 0, false
	}
	version, err := protocol.ParseVersionETag(value)
	if err != nil {
		writeSCIMError(w, r, http.StatusBadRequest, protocol.ErrorTypeInvalidSyntax, err.Error())
		return 0, false
	}
	return version, true
}

func decodeSCIMJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, scimMediaType) {
		writeSCIMError(w, r, http.StatusUnsupportedMediaType, protocol.ErrorTypeInvalidSyntax,
			"Content-Type must be application/scim+json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximumSCIMBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeSCIMError(w, r, http.StatusRequestEntityTooLarge, protocol.ErrorTypeInvalidValue,
				"SCIM document exceeds the one MiB limit")
			return false
		}
		writeSCIMError(w, r, http.StatusBadRequest, protocol.ErrorTypeInvalidSyntax, "Malformed SCIM JSON document")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeSCIMError(w, r, http.StatusBadRequest, protocol.ErrorTypeInvalidSyntax, "Request must contain one JSON document")
		return false
	}
	return true
}

func decodeAdministrationJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	if err := decodeAuthRequest(w, r, destination); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid SCIM administration request", "")
		return false
	}
	return true
}

func writeSCIMResource(w http.ResponseWriter, r *http.Request, status int, value any, etag, location string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
		if status == http.StatusOK && r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	if location != "" {
		w.Header().Set("Location", location)
	}
	writeSCIMJSON(w, status, value)
}

func writeSCIMJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", scimMediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeSCIMError(w http.ResponseWriter, r *http.Request, status int, scimType, detail string) {
	if detail == "" {
		detail = http.StatusText(status)
	}
	writeSCIMJSON(w, status, models.SCIMError{Schemas: []string{models.SCIMErrorSchema},
		Status: strconv.Itoa(status), SCIMType: scimType, Detail: detail})
}

func writeSCIMServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrSCIMInvalid):
		typeName := protocol.ErrorTypeInvalidValue
		if errors.Is(err, protocol.ErrInvalidFilter) || errors.Is(err, protocol.ErrUnsupportedFilter) {
			typeName = protocol.ErrorTypeInvalidFilter
		}
		writeSCIMError(w, r, http.StatusBadRequest, typeName, "SCIM request is invalid")
	case errors.Is(err, service.ErrSCIMNotFound):
		writeSCIMError(w, r, http.StatusNotFound, "", "SCIM resource not found")
	case errors.Is(err, service.ErrSCIMConflict):
		writeSCIMError(w, r, http.StatusConflict, protocol.ErrorTypeUniqueness, "SCIM resource conflicts with existing tenant data")
	case errors.Is(err, service.ErrSCIMVersion):
		writeSCIMError(w, r, http.StatusPreconditionFailed, protocol.ErrorTypeMutability, "SCIM resource version has changed")
	case errors.Is(err, service.ErrSCIMLastAdmin):
		writeSCIMError(w, r, http.StatusConflict, protocol.ErrorTypeMutability, "The final active tenant administrator cannot be deprovisioned")
	case errors.Is(err, service.ErrSCIMDynamicGroup):
		writeSCIMError(w, r, http.StatusConflict, protocol.ErrorTypeMutability, "Rule-managed group membership cannot be changed through SCIM")
	case errors.Is(err, service.ErrSubscriptionLimitExceeded):
		writeSCIMError(w, r, http.StatusPaymentRequired, protocol.ErrorTypeMutability, "User capacity is exhausted; upgrade or remove a seat")
	default:
		log.Error().Err(err).Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Str("organization_id", scimOrganizationID(r)).Str("scim_token_id", scimTokenID(r)).
			Msg("SCIM protocol request failed")
		writeSCIMError(w, r, http.StatusInternalServerError, "", "SCIM service is temporarily unavailable")
	}
}

func writeSCIMAdministrationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrSCIMTokenInvalid):
		writeError(w, http.StatusBadRequest, "Invalid SCIM token request", "")
	case errors.Is(err, service.ErrSCIMTokenNotFound):
		writeError(w, http.StatusNotFound, "SCIM token not found", "")
	case errors.Is(err, service.ErrSCIMTokenConflict), errors.Is(err, service.ErrSCIMVersion):
		writeError(w, http.StatusConflict, "SCIM token has changed", "Refresh and retry with its current version.")
	default:
		log.Error().Err(err).Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Str("organization_id", middleware.GetOrgIDFromContext(r.Context())).Msg("SCIM administration request failed")
		writeError(w, http.StatusInternalServerError, "SCIM administration service unavailable", "")
	}
}

func scimOrganizationID(r *http.Request) string { return middleware.GetOrgIDFromContext(r.Context()) }
func scimTokenID(r *http.Request) string        { return middleware.GetSCIMTokenIDFromContext(r.Context()) }
func scimActorID(r *http.Request) string {
	return middleware.GetSCIMActorUserIDFromContext(r.Context())
}
