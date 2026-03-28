package handler

import (
	"encoding/json"
	"net/http"

	"github.com/complianceforge/platform/internal/models"
)

// AuthService defines the methods required by AuthHandler.
type AuthService interface {
	Login(email, password string) (*TokenResponse, error)
	Register(req *RegisterRequest) (*models.User, error)
	RefreshToken(refreshToken string) (*TokenResponse, error)
}

// TokenResponse holds the JWT token pair returned on login or refresh.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// LoginRequest is the payload for POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

// RegisterRequest is the payload for POST /auth/register.
type RegisterRequest struct {
	Email          string `json:"email" validate:"required,email"`
	Password       string `json:"password" validate:"required,min=8"`
	FirstName      string `json:"first_name" validate:"required"`
	LastName       string `json:"last_name" validate:"required"`
	OrganizationID string `json:"organization_id" validate:"required,uuid"`
}

// RefreshRequest is the payload for POST /auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	service AuthService
}

// NewAuthHandler creates a new AuthHandler with the given service.
func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	tokens, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Authentication failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

// Register handles POST /auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	user, err := h.service.Register(&req)
	if err != nil {
		writeError(w, http.StatusConflict, "Registration failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

// Refresh handles POST /auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	tokens, err := h.service.RefreshToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Token refresh failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

// writeJSON marshals v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a models.ErrorResponse as JSON with the given status code.
func writeError(w http.ResponseWriter, code int, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(models.ErrorResponse{
		Code:    code,
		Message: message,
		Details: details,
	})
}
