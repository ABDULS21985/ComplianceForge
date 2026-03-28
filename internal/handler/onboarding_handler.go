package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
)

// OnboardingSvc defines the methods required by OnboardingHandler.
type OnboardingSvc interface {
	GetProgress(ctx context.Context, orgID string) (interface{}, error)
	SaveStepData(ctx context.Context, orgID string, step int, data map[string]interface{}) error
	SkipStep(ctx context.Context, orgID string, step int) error
	GetRecommendations(ctx context.Context, orgID string, data map[string]interface{}) ([]interface{}, error)
	CompleteOnboarding(ctx context.Context, orgID, userID string) error
	ListPlans(ctx context.Context) ([]interface{}, error)
	GetSubscription(ctx context.Context, orgID string) (interface{}, error)
	ChangePlan(ctx context.Context, orgID, planSlug, billingCycle string) error
	CancelSubscription(ctx context.Context, orgID, reason string) error
	GetUsageSummary(ctx context.Context, orgID string) (interface{}, error)
}

// OnboardingHandler handles onboarding and subscription endpoints.
type OnboardingHandler struct {
	svc OnboardingSvc
}

// NewOnboardingHandler creates a new OnboardingHandler with the given service.
func NewOnboardingHandler(svc OnboardingSvc) *OnboardingHandler {
	return &OnboardingHandler{svc: svc}
}

// GetProgress handles GET /onboard/progress.
func (h *OnboardingHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	progress, err := h.svc.GetProgress(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get onboarding progress", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": progress})
}

// SaveStep handles PUT /onboard/step/{n}.
func (h *OnboardingHandler) SaveStep(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	stepStr := chi.URLParam(r, "n")
	step, err := strconv.Atoi(stepStr)
	if err != nil || step < 1 {
		writeError(w, http.StatusBadRequest, "Invalid step number", "Step must be a positive integer")
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if err := h.svc.SaveStepData(r.Context(), orgID, step, data); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save step data", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Step data saved"})
}

// SkipStep handles POST /onboard/step/{n}/skip.
func (h *OnboardingHandler) SkipStep(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	stepStr := chi.URLParam(r, "n")
	step, err := strconv.Atoi(stepStr)
	if err != nil || step < 1 {
		writeError(w, http.StatusBadRequest, "Invalid step number", "Step must be a positive integer")
		return
	}

	if err := h.svc.SkipStep(r.Context(), orgID, step); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to skip step", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Step skipped"})
}

// Complete handles POST /onboard/complete.
func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	if orgID == "" || userID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}

	if err := h.svc.CompleteOnboarding(r.Context(), orgID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to complete onboarding", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Onboarding completed"})
}

// GetRecommendations handles GET /onboard/recommendations.
func (h *OnboardingHandler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	// Collect query parameters as recommendation context.
	data := make(map[string]interface{})
	for key, values := range r.URL.Query() {
		if len(values) == 1 {
			data[key] = values[0]
		} else {
			data[key] = values
		}
	}

	recommendations, err := h.svc.GetRecommendations(r.Context(), orgID, data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get recommendations", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": recommendations})
}

// GetSubscription handles GET /subscription.
func (h *OnboardingHandler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	subscription, err := h.svc.GetSubscription(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get subscription", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": subscription})
}

// ChangePlan handles PUT /subscription/plan.
func (h *OnboardingHandler) ChangePlan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	var body struct {
		PlanSlug     string `json:"plan_slug"`
		BillingCycle string `json:"billing_cycle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if body.PlanSlug == "" {
		writeError(w, http.StatusBadRequest, "plan_slug is required", "")
		return
	}

	if body.BillingCycle == "" {
		body.BillingCycle = "monthly"
	}

	if err := h.svc.ChangePlan(r.Context(), orgID, body.PlanSlug, body.BillingCycle); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to change plan", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Plan changed successfully"})
}

// Cancel handles POST /subscription/cancel.
func (h *OnboardingHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if err := h.svc.CancelSubscription(r.Context(), orgID, body.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to cancel subscription", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Subscription cancelled"})
}

// ListPlans handles GET /subscription/plans.
func (h *OnboardingHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.ListPlans(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list plans", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": plans})
}

// GetUsage handles GET /subscription/usage.
func (h *OnboardingHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusUnauthorized, "Missing organization context", "")
		return
	}

	usage, err := h.svc.GetUsageSummary(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get usage summary", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": usage})
}
