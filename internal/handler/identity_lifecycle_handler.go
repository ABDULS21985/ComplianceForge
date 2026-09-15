package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type IdentityLifecycleService interface {
	GetPolicy(context.Context, string) (*models.IdentityPolicy, error)
	UpdatePolicy(context.Context, string, string, models.IdentityPolicyPatch) (*models.IdentityPolicy, error)
	IssueInvitation(context.Context, string, string, string, models.IdentityInvitationIssueInput, models.IdentityRequestMetadata) (*models.IdentityInvitation, error)
	AcceptInvitation(context.Context, models.IdentityInvitationAcceptInput, models.IdentityRequestMetadata) (*models.IdentityAcceptanceResult, error)
	RequestEmailVerification(context.Context, models.IdentityEmailRequest, models.IdentityRequestMetadata) error
	VerifyEmail(context.Context, models.IdentityTokenInput, models.IdentityRequestMetadata) error
	RequestPasswordReset(context.Context, models.IdentityEmailRequest, models.IdentityRequestMetadata) error
	ResetPassword(context.Context, models.IdentityPasswordResetInput, models.IdentityRequestMetadata) error
	ListSessions(context.Context, string, string, string) ([]models.IdentitySession, error)
	RevokeSession(context.Context, string, string, string, string, string, models.IdentitySessionRevokeInput, models.IdentityRequestMetadata) error
	GlobalSignOut(context.Context, string, string, string, string, models.IdentityGlobalSignOutInput, models.IdentityRequestMetadata) (int, error)
	ListMFAFactors(context.Context, string, string) ([]models.IdentityMFAFactor, error)
	BeginTOTPEnrollment(context.Context, string, string, string, models.IdentityTOTPEnrollmentInput, models.IdentityRequestMetadata) (*models.IdentityTOTPEnrollment, error)
	VerifyTOTPEnrollment(context.Context, string, string, string, models.IdentityTOTPVerifyInput, models.IdentityRequestMetadata) (*models.IdentityTOTPVerifyResult, error)
	DisableMFAFactor(context.Context, string, string, string, string, string, models.IdentityMFADisableInput, models.IdentityRequestMetadata) error
	RegenerateRecoveryCodes(context.Context, string, string, string, string, string, models.IdentityRecoveryRegenerateInput, models.IdentityRequestMetadata) ([]string, error)
	BeginStepUp(context.Context, string, string, string, models.IdentityStepUpBeginInput, models.IdentityRequestMetadata) (*models.IdentityMFAChallengeResponse, error)
	VerifyStepUp(context.Context, string, string, string, models.IdentityMFAProofInput, models.IdentityRequestMetadata) (*models.IdentityStepUpGrant, error)
	BeginPasskeyRegistration(context.Context, string, string, string, models.IdentityPasskeyRegistrationBeginInput, models.IdentityRequestMetadata) (*models.IdentityPasskeyCeremony, error)
	FinishPasskeyRegistration(context.Context, string, string, string, string, models.IdentityPasskeyRegistrationFinishInput, models.IdentityRequestMetadata) (*models.IdentityPasskey, error)
	BeginPasskeyAuthentication(context.Context, models.IdentityPasskeyAuthenticationBeginInput, models.IdentityRequestMetadata) (*models.IdentityPasskeyCeremony, error)
	ListPasskeys(context.Context, string, string) ([]models.IdentityPasskey, error)
	RemovePasskey(context.Context, string, string, string, string, string, models.IdentityPasskeyRemoveInput, models.IdentityRequestMetadata) error
	AdminResetMFA(context.Context, string, string, string, string, models.IdentityAdminMFAResetInput, models.IdentityRequestMetadata) error
	ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.IdentitySecurityEvent, int, error)
}

type IdentityLifecycleHandler struct{ service IdentityLifecycleService }

func NewIdentityLifecycleHandler(identityService IdentityLifecycleService) *IdentityLifecycleHandler {
	return &IdentityLifecycleHandler{service: identityService}
}

func (h *IdentityLifecycleHandler) Ready() bool { return h != nil && h.service != nil }

func (h *IdentityLifecycleHandler) IssueInvitation(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityInvitationIssueInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	item, err := h.service.IssueInvitation(r.Context(), identityOrgID(r), chi.URLParam(r, "id"), identityUserID(r), input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to issue invitation")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *IdentityLifecycleHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityInvitationAcceptInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	result, err := h.service.AcceptInvitation(r.Context(), input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Invitation could not be accepted")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *IdentityLifecycleHandler) RequestEmailVerification(w http.ResponseWriter, r *http.Request) {
	h.requestGenericDelivery(w, r, h.service.RequestEmailVerification)
}

func (h *IdentityLifecycleHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	h.requestGenericDelivery(w, r, h.service.RequestPasswordReset)
}

type identityDeliveryRequester func(context.Context, models.IdentityEmailRequest, models.IdentityRequestMetadata) error

func (h *IdentityLifecycleHandler) requestGenericDelivery(w http.ResponseWriter, r *http.Request, request identityDeliveryRequester) {
	var input models.IdentityEmailRequest
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	if err := request(r.Context(), input, identityMetadata(r)); err != nil {
		writeIdentityError(w, r, err, "Identity delivery request failed")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "If the account is eligible, delivery has been queued."})
}

func (h *IdentityLifecycleHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityTokenInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	if err := h.service.VerifyEmail(r.Context(), input, identityMetadata(r)); err != nil {
		writeIdentityError(w, r, err, "Email verification failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityLifecycleHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityPasswordResetInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	if err := h.service.ResetPassword(r.Context(), input, identityMetadata(r)); err != nil {
		writeIdentityError(w, r, err, "Password reset failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityLifecycleHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetPolicy(r.Context(), identityOrgID(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to load identity policy")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *IdentityLifecycleHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityPolicyPatch
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	item, err := h.service.UpdatePolicy(r.Context(), identityOrgID(r), identityUserID(r), input)
	if err != nil {
		writeIdentityError(w, r, err, "Failed to update identity policy")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *IdentityLifecycleHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	items, err := h.service.ListSessions(r.Context(), identityOrgID(r), identityUserID(r), accessToken)
	if err != nil {
		writeIdentityError(w, r, err, "Failed to list sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *IdentityLifecycleHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	var input models.IdentitySessionRevokeInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	if err := h.service.RevokeSession(r.Context(), identityOrgID(r), identityUserID(r), identityUserID(r),
		chi.URLParam(r, "sessionID"), accessToken, input, identityMetadata(r)); err != nil {
		writeIdentityError(w, r, err, "Failed to revoke session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityLifecycleHandler) GlobalSignOut(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityGlobalSignOutInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	count, err := h.service.GlobalSignOut(r.Context(), identityOrgID(r), identityUserID(r), identityUserID(r),
		accessToken, input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to sign out sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"revoked_sessions": count})
}

func (h *IdentityLifecycleHandler) ListMFAFactors(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListMFAFactors(r.Context(), identityOrgID(r), identityUserID(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to list MFA factors")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *IdentityLifecycleHandler) BeginTOTPEnrollment(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityTOTPEnrollmentInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	result, err := h.service.BeginTOTPEnrollment(r.Context(), identityOrgID(r), identityUserID(r), identityUserID(r), input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to begin TOTP enrollment")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *IdentityLifecycleHandler) VerifyTOTPEnrollment(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityTOTPVerifyInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	result, err := h.service.VerifyTOTPEnrollment(r.Context(), identityOrgID(r), identityUserID(r), identityUserID(r), input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to verify TOTP enrollment")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *IdentityLifecycleHandler) DisableMFAFactor(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityMFADisableInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	input.StepUpToken = r.Header.Get("X-Step-Up-Token")
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	if err := h.service.DisableMFAFactor(r.Context(), identityOrgID(r), identityUserID(r), identityUserID(r),
		chi.URLParam(r, "factorID"), accessToken, input, identityMetadata(r)); err != nil {
		writeIdentityError(w, r, err, "Failed to disable MFA factor")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityLifecycleHandler) RegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityRecoveryRegenerateInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	input.StepUpToken = r.Header.Get("X-Step-Up-Token")
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	codes, err := h.service.RegenerateRecoveryCodes(r.Context(), identityOrgID(r), identityUserID(r), identityUserID(r),
		chi.URLParam(r, "factorID"), accessToken, input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to regenerate recovery codes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (h *IdentityLifecycleHandler) BeginStepUp(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityStepUpBeginInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	result, err := h.service.BeginStepUp(r.Context(), identityOrgID(r), identityUserID(r), accessToken, input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to begin step-up authentication")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *IdentityLifecycleHandler) VerifyStepUp(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityMFAProofInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	result, err := h.service.VerifyStepUp(r.Context(), identityOrgID(r), identityUserID(r), accessToken, input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Step-up authentication failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *IdentityLifecycleHandler) BeginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityPasskeyRegistrationBeginInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	result, err := h.service.BeginPasskeyRegistration(r.Context(), identityOrgID(r), identityUserID(r), accessToken, input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to begin passkey registration")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *IdentityLifecycleHandler) FinishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityPasskeyRegistrationFinishInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	result, err := h.service.FinishPasskeyRegistration(r.Context(), identityOrgID(r), identityUserID(r), identityUserID(r),
		accessToken, input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Passkey registration failed")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *IdentityLifecycleHandler) BeginPasskeyAuthentication(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityPasskeyAuthenticationBeginInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	result, err := h.service.BeginPasskeyAuthentication(r.Context(), input, identityMetadata(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to begin passkey authentication")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *IdentityLifecycleHandler) ListPasskeys(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListPasskeys(r.Context(), identityOrgID(r), identityUserID(r))
	if err != nil {
		writeIdentityError(w, r, err, "Failed to list passkeys")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *IdentityLifecycleHandler) RemovePasskey(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityPasskeyRemoveInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	input.StepUpToken = r.Header.Get("X-Step-Up-Token")
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	if err := h.service.RemovePasskey(r.Context(), identityOrgID(r), identityUserID(r), identityUserID(r),
		chi.URLParam(r, "passkeyID"), accessToken, input, identityMetadata(r)); err != nil {
		writeIdentityError(w, r, err, "Failed to remove passkey")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityLifecycleHandler) AdminResetMFA(w http.ResponseWriter, r *http.Request) {
	var input models.IdentityAdminMFAResetInput
	if !decodeIdentityJSON(w, r, &input) {
		return
	}
	input.StepUpToken = r.Header.Get("X-Step-Up-Token")
	accessToken, ok := identityBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	if err := h.service.AdminResetMFA(r.Context(), identityOrgID(r), chi.URLParam(r, "id"), identityUserID(r),
		accessToken, input, identityMetadata(r)); err != nil {
		writeIdentityError(w, r, err, "Failed to reset user MFA")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityLifecycleHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	pagination := parsePagination(r)
	items, total, err := h.service.ListEvents(r.Context(), identityOrgID(r), userID, pagination)
	if err != nil {
		writeIdentityError(w, r, err, "Failed to list identity history")
		return
	}
	writePaginated(w, items, total, normalizedHandlerPagination(pagination))
}

func decodeIdentityJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	if err := decodeAuthRequest(w, r, destination); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid identity request", err.Error())
		return false
	}
	return true
}

func identityMetadata(r *http.Request) models.IdentityRequestMetadata {
	ip := middleware.GetClientIPFromContext(r.Context())
	if ip == "" {
		ip = strings.TrimSpace(r.RemoteAddr)
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		if net.ParseIP(ip) == nil {
			ip = ""
		}
	}
	return models.IdentityRequestMetadata{RequestID: middleware.GetRequestIDFromContext(r.Context()),
		IPAddress: ip, UserAgent: r.UserAgent(), DeviceName: strings.TrimSpace(r.Header.Get("X-Device-Name"))}
}

func identityBearer(r *http.Request) (string, bool) {
	return bearerToken(r.Header.Get("Authorization"))
}
func identityOrgID(r *http.Request) string  { return middleware.GetOrgIDFromContext(r.Context()) }
func identityUserID(r *http.Request) string { return middleware.GetUserIDFromContext(r.Context()) }

func writeIdentityError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrIdentityInvalid):
		writeError(w, http.StatusBadRequest, "Invalid identity request", "")
	case errors.Is(err, service.ErrIdentityCredential):
		writeError(w, http.StatusUnauthorized, "Identity verification failed", "")
	case errors.Is(err, service.ErrIdentityRateLimited):
		writeError(w, http.StatusTooManyRequests, "Too many identity attempts", "Try again later.")
	case errors.Is(err, service.ErrIdentityVersionConflict):
		writeError(w, http.StatusConflict, "Identity record has changed", "Refresh and retry with the current version.")
	case errors.Is(err, service.ErrIdentityState):
		writeError(w, http.StatusConflict, "Identity state conflict", "")
	case errors.Is(err, service.ErrIdentityEnrollment):
		writeError(w, http.StatusForbidden, "MFA enrollment required", "Enroll an allowed factor before continuing.")
	case errors.Is(err, service.ErrIdentityLastFactor):
		writeError(w, http.StatusConflict, "Required MFA factor is protected", "Add another allowed factor before removing this one.")
	case errors.Is(err, service.ErrIdentityAdminRecovery):
		writeError(w, http.StatusConflict, "MFA recovery safeguard blocked the reset", "Another MFA-enabled administrator is required.")
	case errors.Is(err, service.ErrIdentityUnavailable):
		writeError(w, http.StatusServiceUnavailable, "Identity security service unavailable", "")
	default:
		log.Error().Err(err).Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Str("organization_id", identityOrgID(r)).Msg("identity lifecycle request failed")
		writeError(w, http.StatusInternalServerError, fallback, "")
	}
}
