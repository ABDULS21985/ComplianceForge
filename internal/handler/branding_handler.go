package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- service interface ----------

// BrandingService defines the methods required by BrandingHandler.
type BrandingService interface {
	// Branding (tenant)
	GetBranding(ctx context.Context, orgID string) (*BrandingConfig, error)
	GetBrandingCSS(ctx context.Context, orgID string) (string, error)
	UpdateBranding(ctx context.Context, orgID, userID string, config *BrandingConfig) error
	UploadLogo(ctx context.Context, orgID, userID, logoType string, data []byte, contentType string) (*LogoUploadResult, error)
	DeleteLogo(ctx context.Context, orgID, userID, logoType string) error

	// Custom domain
	VerifyDomain(ctx context.Context, orgID, userID string, req *DomainVerifyRequest) (*DomainVerifyResult, error)
	GetDomainStatus(ctx context.Context, orgID string) (*DomainStatus, error)

	// Preview
	PreviewBranding(ctx context.Context, orgID string, config *BrandingConfig) (*BrandingPreview, error)

	// Partner / white-label admin
	ListPartners(ctx context.Context, pagination models.PaginationRequest) ([]Partner, int, error)
	CreatePartner(ctx context.Context, userID string, partner *Partner) error
	UpdatePartner(ctx context.Context, userID, partnerID string, partner *Partner) error
	GetPartnerTenants(ctx context.Context, partnerID string, pagination models.PaginationRequest) ([]PartnerTenant, int, error)
}

// ---------- request / response types ----------

// BrandingConfig holds the full branding configuration for a tenant.
type BrandingConfig struct {
	ID               string            `json:"id"`
	OrganizationID   string            `json:"organization_id"`
	CompanyName      string            `json:"company_name,omitempty"`
	PrimaryColor     string            `json:"primary_color,omitempty"`
	SecondaryColor   string            `json:"secondary_color,omitempty"`
	AccentColor      string            `json:"accent_color,omitempty"`
	BackgroundColor  string            `json:"background_color,omitempty"`
	TextColor        string            `json:"text_color,omitempty"`
	FontFamily       string            `json:"font_family,omitempty"`
	LogoURL          string            `json:"logo_url,omitempty"`
	LogoDarkURL      string            `json:"logo_dark_url,omitempty"`
	FaviconURL       string            `json:"favicon_url,omitempty"`
	CustomCSS        string            `json:"custom_css,omitempty"`
	CustomDomain     string            `json:"custom_domain,omitempty"`
	EmailFromName    string            `json:"email_from_name,omitempty"`
	EmailFromAddress string            `json:"email_from_address,omitempty"`
	FooterText       string            `json:"footer_text,omitempty"`
	LoginMessage     string            `json:"login_message,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	UpdatedBy        string            `json:"updated_by,omitempty"`
	UpdatedAt        string            `json:"updated_at,omitempty"`
}

// LogoUploadResult holds the result of a logo upload.
type LogoUploadResult struct {
	LogoType string `json:"logo_type"` // logo, logo_dark, favicon
	URL      string `json:"url"`
}

// DomainVerifyRequest is the payload for POST /branding/domain/verify.
type DomainVerifyRequest struct {
	Domain string `json:"domain" validate:"required"`
}

// DomainVerifyResult holds the result of a domain verification request.
type DomainVerifyResult struct {
	Domain           string `json:"domain"`
	Status           string `json:"status"` // pending, verified, failed
	VerificationRecord string `json:"verification_record,omitempty"`
	CNAMETarget      string `json:"cname_target,omitempty"`
	Instructions     string `json:"instructions,omitempty"`
}

// DomainStatus holds the current status of a custom domain.
type DomainStatus struct {
	Domain       string `json:"domain"`
	Status       string `json:"status"` // pending, verified, active, error
	SSLStatus    string `json:"ssl_status,omitempty"` // pending, active, error
	DNSVerified  bool   `json:"dns_verified"`
	SSLActive    bool   `json:"ssl_active"`
	VerifiedAt   string `json:"verified_at,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// BrandingPreview holds preview data for branding changes.
type BrandingPreview struct {
	PreviewURL string `json:"preview_url"`
	ExpiresAt  string `json:"expires_at"`
	CSS        string `json:"css,omitempty"`
}

// Partner represents a white-label partner.
type Partner struct {
	ID               string `json:"id"`
	Name             string `json:"name" validate:"required"`
	Slug             string `json:"slug"`
	ContactEmail     string `json:"contact_email" validate:"required"`
	ContactName      string `json:"contact_name,omitempty"`
	LogoURL          string `json:"logo_url,omitempty"`
	PrimaryColor     string `json:"primary_color,omitempty"`
	CustomDomain     string `json:"custom_domain,omitempty"`
	MaxTenants       int    `json:"max_tenants"`
	ActiveTenants    int    `json:"active_tenants"`
	IsActive         bool   `json:"is_active"`
	CommissionRate   float64 `json:"commission_rate,omitempty"`
	CreatedBy        string `json:"created_by,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// PartnerTenant represents a tenant under a partner.
type PartnerTenant struct {
	TenantID       string `json:"tenant_id"`
	OrganizationName string `json:"organization_name"`
	Plan           string `json:"plan"`
	Status         string `json:"status"` // active, suspended, cancelled
	UserCount      int    `json:"user_count"`
	CreatedAt      string `json:"created_at"`
}

// ---------- handler ----------

// BrandingHandler handles branding, white-label, and partner management endpoints.
type BrandingHandler struct {
	svc BrandingService
}

// NewBrandingHandler creates a new BrandingHandler with the given service.
func NewBrandingHandler(svc BrandingService) *BrandingHandler {
	return &BrandingHandler{svc: svc}
}

// GetBranding handles GET /branding (public).
func (h *BrandingHandler) GetBranding(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "Query parameter 'org_id' is required", "")
		return
	}

	config, err := h.svc.GetBranding(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Branding not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, config)
}

// GetBrandingCSS handles GET /branding/css (public).
func (h *BrandingHandler) GetBrandingCSS(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "Query parameter 'org_id' is required", "")
		return
	}

	css, err := h.svc.GetBrandingCSS(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Branding CSS not found", err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(css))
}

// UpdateBranding handles PUT /branding.
func (h *BrandingHandler) UpdateBranding(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var config BrandingConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	config.OrganizationID = orgID
	config.UpdatedBy = userID

	if err := h.svc.UpdateBranding(r.Context(), orgID, userID, &config); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update branding", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, config)
}

// UploadLogo handles POST /branding/logo.
func (h *BrandingHandler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	// Max 5MB
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse multipart form", err.Error())
		return
	}

	logoType := r.FormValue("type")
	if logoType == "" {
		logoType = "logo"
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing file in request", err.Error())
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to read file", err.Error())
		return
	}

	result, err := h.svc.UploadLogo(r.Context(), orgID, userID, logoType, data, header.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to upload logo", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// DeleteLogo handles DELETE /branding/logo/{type}.
func (h *BrandingHandler) DeleteLogo(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())
	logoType := chi.URLParam(r, "type")
	if logoType == "" {
		writeError(w, http.StatusBadRequest, "Missing logo type", "")
		return
	}

	if err := h.svc.DeleteLogo(r.Context(), orgID, userID, logoType); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete logo", err.Error())
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// VerifyDomain handles POST /branding/domain/verify.
func (h *BrandingHandler) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req DomainVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required", "")
		return
	}

	result, err := h.svc.VerifyDomain(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to verify domain", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetDomainStatus handles GET /branding/domain/status.
func (h *BrandingHandler) GetDomainStatus(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	status, err := h.svc.GetDomainStatus(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Domain status not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// PreviewBranding handles POST /branding/preview.
func (h *BrandingHandler) PreviewBranding(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	var config BrandingConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	preview, err := h.svc.PreviewBranding(r.Context(), orgID, &config)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate preview", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

// ListPartners handles GET /admin/partners.
func (h *BrandingHandler) ListPartners(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	partners, total, err := h.svc.ListPartners(r.Context(), pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list partners", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": partners,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// CreatePartner handles POST /admin/partners.
func (h *BrandingHandler) CreatePartner(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())

	var partner Partner
	if err := json.NewDecoder(r.Body).Decode(&partner); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if partner.Name == "" || partner.ContactEmail == "" {
		writeError(w, http.StatusBadRequest, "name and contact_email are required", "")
		return
	}

	if err := h.svc.CreatePartner(r.Context(), userID, &partner); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create partner", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, partner)
}

// UpdatePartner handles PUT /admin/partners/{id}.
func (h *BrandingHandler) UpdatePartner(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	partnerID := chi.URLParam(r, "id")
	if partnerID == "" {
		writeError(w, http.StatusBadRequest, "Missing partner ID", "")
		return
	}

	var partner Partner
	if err := json.NewDecoder(r.Body).Decode(&partner); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	partner.ID = partnerID

	if err := h.svc.UpdatePartner(r.Context(), userID, partnerID, &partner); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update partner", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, partner)
}

// GetPartnerTenants handles GET /admin/partners/{id}/tenants.
func (h *BrandingHandler) GetPartnerTenants(w http.ResponseWriter, r *http.Request) {
	partnerID := chi.URLParam(r, "id")
	if partnerID == "" {
		writeError(w, http.StatusBadRequest, "Missing partner ID", "")
		return
	}

	pagination := parsePagination(r)

	tenants, total, err := h.svc.GetPartnerTenants(r.Context(), partnerID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list partner tenants", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": tenants,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}
