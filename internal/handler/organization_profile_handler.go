package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"time"

	"github.com/complianceforge/platform/internal/apiresponse"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const maximumOrganizationProfileBody = 8 << 10

type OrganizationProfileHandlerService interface {
	GetProfile(context.Context, string, string) (*models.OrganizationProfile, error)
	UpdateProfile(context.Context, string, string, string, models.OrganizationProfileUpdateInput) (*models.OrganizationProfile, error)
}

type OrganizationProfileHandler struct {
	service OrganizationProfileHandlerService
}

func NewOrganizationProfileHandler(service OrganizationProfileHandlerService) *OrganizationProfileHandler {
	return &OrganizationProfileHandler{service: service}
}

func (h *OrganizationProfileHandler) Ready() bool {
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

func (h *OrganizationProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !organizationProfileRequestAllowed(w, r, false) {
		return
	}
	if !h.Ready() {
		writeOrganizationProfileError(w, r, models.ErrOrganizationProfileUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	profile, err := h.service.GetProfile(ctx, middleware.GetOrgIDFromContext(ctx), middleware.GetUserIDFromContext(ctx))
	if err != nil {
		writeOrganizationProfileError(w, r, err)
		return
	}
	if profile == nil || profile.ID != middleware.GetOrgIDFromContext(ctx) {
		writeOrganizationProfileError(w, r, models.ErrOrganizationProfileUnavailable)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", organizationProfileResponse(r, profile))
}

// UpdateProfile preserves absent fields on the established PUT URL. A required
// expected version and reason prevent silent stale-write loss. No POST replay or
// unreviewed full Organization binding is used by this endpoint.
func (h *OrganizationProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !organizationProfileRequestAllowed(w, r, true) {
		return
	}
	if !h.Ready() {
		writeOrganizationProfileError(w, r, models.ErrOrganizationProfileUnavailable)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		apiresponse.WriteError(w, http.StatusUnsupportedMediaType, "organization_profile_json_required", "Use a JSON profile update", "", middleware.GetRequestIDFromContext(r.Context()))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumOrganizationProfileBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			apiresponse.WriteError(w, http.StatusRequestEntityTooLarge, "organization_profile_body_too_large", "The profile update exceeded its size limit", "", middleware.GetRequestIDFromContext(r.Context()))
		} else {
			writeOrganizationProfileBodyError(w, r)
		}
		return
	}
	var input models.OrganizationProfileUpdateInput
	if decodeOrganizationProfileInput(body, &input) != nil {
		writeOrganizationProfileBodyError(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	profile, err := h.service.UpdateProfile(ctx, middleware.GetOrgIDFromContext(ctx), middleware.GetUserIDFromContext(ctx), middleware.GetRequestIDFromContext(ctx), input)
	if err != nil {
		writeOrganizationProfileError(w, r, err)
		return
	}
	if profile == nil || profile.ID != middleware.GetOrgIDFromContext(ctx) {
		writeOrganizationProfileError(w, r, models.ErrOrganizationProfileUnavailable)
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "settings", organizationProfileResponse(r, profile))
}

func organizationProfileResponse(r *http.Request, profile *models.OrganizationProfile) models.OrganizationProfileResponse {
	permissions, err := classifiedResponsePermissions(r, "settings")
	editable := err == nil
	for _, permission := range permissions {
		if permission.Visibility != models.AccessFieldVisible {
			editable = false
		}
	}
	return models.OrganizationProfileResponse{Data: profile, Meta: models.OrganizationProfileMeta{
		SchemaVersion: 1, Scope: models.OrganizationProfileScope, Editable: editable,
	}}
}

func organizationProfileRequestAllowed(w http.ResponseWriter, r *http.Request, mutation bool) bool {
	if middleware.GetOrgIDFromContext(r.Context()) == "" || middleware.GetUserIDFromContext(r.Context()) == "" {
		apiresponse.WriteError(w, http.StatusUnauthorized, "authentication_required", "Authentication is required", "", middleware.GetRequestIDFromContext(r.Context()))
		return false
	}
	permissions, err := classifiedResponsePermissions(r, "settings")
	if err == nil && mutation {
		for _, permission := range permissions {
			if permission.Visibility != models.AccessFieldVisible {
				err = errors.New("restricted profile fields cannot be edited through a whole profile projection")
				break
			}
		}
	}
	if err == nil {
		decision, _ := middleware.GetAuthorizationDecision(r.Context())
		for _, obligation := range decision.Obligations {
			if obligation.Kind != service.AccessFieldVisibilityObligation {
				err = errors.New("profile request obligation is unsupported")
				break
			}
		}
	}
	if err != nil {
		writeClassifiedFailure(w, r, "settings", err)
		return false
	}
	return true
}

func decodeOrganizationProfileInput(body []byte, input *models.OrganizationProfileUpdateInput) error {
	if len(body) == 0 || len(body) > maximumOrganizationProfileBody {
		return errors.New("invalid profile body")
	}
	checker := json.NewDecoder(bytes.NewReader(body))
	checker.UseNumber()
	first, err := checker.Token()
	if err != nil || first != json.Delim('{') {
		return errors.New("profile object required")
	}
	if err := inspectProfileJSONObject(checker, 0, true); err != nil {
		return err
	}
	if _, err := checker.Token(); err != io.EOF {
		return errors.New("single profile object required")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(input)
}

func inspectProfileJSONObject(decoder *json.Decoder, depth int, top bool) error {
	if depth > 16 {
		return errors.New("profile nesting limit")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return errors.New("duplicate or invalid profile key")
		}
		seen[key] = true
		if err := inspectProfileJSONValue(decoder, depth+1, top); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return errors.New("invalid profile object")
	}
	return nil
}

func inspectProfileJSONValue(decoder *json.Decoder, depth int, rejectNull bool) error {
	if depth > 16 {
		return errors.New("profile nesting limit")
	}
	token, err := decoder.Token()
	if err != nil || (rejectNull && token == nil) {
		return errors.New("invalid or null profile field")
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		return inspectProfileJSONObject(decoder, depth, false)
	case '[':
		for decoder.More() {
			if err := inspectProfileJSONValue(decoder, depth+1, false); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("invalid profile array")
		}
		return nil
	default:
		return errors.New("invalid profile delimiter")
	}
}

func writeOrganizationProfileBodyError(w http.ResponseWriter, r *http.Request) {
	apiresponse.WriteError(w, http.StatusBadRequest, "organization_profile_body_invalid", "Provide one bounded profile object with unique reviewed fields and no explicit nulls", "", middleware.GetRequestIDFromContext(r.Context()))
}

func writeOrganizationProfileError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusServiceUnavailable, "organization_profile_unavailable", "Organisation profile settings are temporarily unavailable"
	var validation *models.OrganizationProfileValidationError
	switch {
	case errors.As(err, &validation):
		status, code, message = http.StatusUnprocessableEntity, "organization_profile_validation_failed", "Review the profile fields before saving"
	case errors.Is(err, models.ErrOrganizationProfileScope):
		status, code, message = http.StatusForbidden, "organization_profile_scope_denied", "Profile settings are not available in this authentication context"
	case errors.Is(err, models.ErrOrganizationProfileNotFound):
		status, code, message = http.StatusNotFound, "organization_profile_not_found", "Organisation profile settings are unavailable"
	case errors.Is(err, models.ErrOrganizationProfileConflict):
		status, code, message = http.StatusConflict, "organization_profile_conflict", "Another administrator changed this profile. Reload it before saving again"
	}
	// Even validation errors are not serialized verbatim; the profile form owns
	// its reviewed field messages and provider/database text never reaches here.
	apiresponse.WriteError(w, status, code, message, "", middleware.GetRequestIDFromContext(r.Context()))
}
