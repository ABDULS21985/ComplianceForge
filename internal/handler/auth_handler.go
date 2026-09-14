package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	requestvalidator "github.com/complianceforge/platform/internal/validator"
)

const maxAuthRequestBytes = 1 << 20

// AuthService defines the methods required by AuthHandler.
type AuthService interface {
	Login(ctx context.Context, req authdomain.LoginRequest) (*authdomain.TokenPair, error)
	Register(ctx context.Context, req authdomain.RegisterRequest) (*authdomain.TokenPair, error)
	RefreshToken(ctx context.Context, refreshToken string) (*authdomain.TokenPair, error)
	CurrentUser(ctx context.Context, orgID, userID string) (*models.User, error)
	Logout(ctx context.Context, orgID, userID, accessToken string) error
}

// Backwards-compatible aliases retain the handler package's former DTO names.
type TokenResponse = authdomain.TokenPair
type LoginRequest = authdomain.LoginRequest
type RegisterRequest = authdomain.RegisterRequest
type RefreshRequest = authdomain.RefreshRequest

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	service   AuthService
	validator *requestvalidator.Validator
}

// NewAuthHandler creates a new AuthHandler with the given service.
func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{
		service:   service,
		validator: requestvalidator.New(),
	}
}

// Ready reports whether the handler has its required service dependency. The
// router uses it to reject partially constructed dependency graphs at startup.
func (h *AuthHandler) Ready() bool {
	return h != nil && h.service != nil && h.validator != nil
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := decodeAuthRequest(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.validator.ValidateStruct(req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid login request", err.Error())
		return
	}

	tokens, err := h.service.Login(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Authentication failed", "")
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

// Register handles POST /auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := decodeAuthRequest(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.validator.ValidateStruct(req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid registration request", err.Error())
		return
	}

	tokens, err := h.service.Register(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusConflict, "Registration failed", "")
		return
	}

	writeJSON(w, http.StatusCreated, tokens)
}

// Refresh handles POST /auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := decodeAuthRequest(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if err := h.validator.ValidateStruct(req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid refresh request", err.Error())
		return
	}

	tokens, err := h.service.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Token refresh failed", "")
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

// Me handles GET /auth/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	orgID := middleware.GetOrgIDFromContext(r.Context())
	if userID == "" || orgID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}

	user, err := h.service.CurrentUser(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// Logout handles POST /auth/logout and revokes the current token pair.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	accessToken, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	if err := h.service.Logout(
		r.Context(),
		middleware.GetOrgIDFromContext(r.Context()),
		middleware.GetUserIDFromContext(r.Context()),
		accessToken,
	); err != nil {
		writeError(w, http.StatusUnauthorized, "Logout failed", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeAuthRequest(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// writeJSON marshals v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a models.ErrorResponse as JSON with the given status code.
func writeError(w http.ResponseWriter, code int, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(models.ErrorResponse{
		Code:    code,
		Message: message,
		Details: details,
	})
}
