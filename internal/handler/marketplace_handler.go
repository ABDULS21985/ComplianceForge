package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

// ---------- service interface ----------

// MarketplaceService defines the methods required by MarketplaceHandler.
type MarketplaceService interface {
	// Public
	SearchPackages(ctx context.Context, pagination models.PaginationRequest, filters PackageFilters) ([]MarketplacePackage, int, error)
	GetFeaturedPackages(ctx context.Context) ([]MarketplacePackage, error)
	GetPackageDetail(ctx context.Context, publisher, slug string) (*MarketplacePackageDetail, error)
	GetPackageReviews(ctx context.Context, publisher, slug string, pagination models.PaginationRequest) ([]PackageReview, int, error)

	// Authenticated
	InstallPackage(ctx context.Context, orgID, userID string, req *InstallPackageRequest) (*InstalledPackage, error)
	UninstallPackage(ctx context.Context, orgID, installID string) error
	ListInstalled(ctx context.Context, orgID string, pagination models.PaginationRequest) ([]InstalledPackage, int, error)
	SubmitReview(ctx context.Context, orgID, userID string, review *PackageReview) error

	// Publisher
	RegisterPublisher(ctx context.Context, orgID, userID string, pub *Publisher) error
	GetPublisherStats(ctx context.Context, orgID string) (*PublisherStats, error)
	CreatePackage(ctx context.Context, orgID string, pkg *MarketplacePackage) error
	PublishVersion(ctx context.Context, orgID, packageID string, version *PackageVersion) error
}

// ---------- request / response types ----------

// PackageFilters holds filter parameters for searching marketplace packages.
type PackageFilters struct {
	Category string `json:"category"`
	Search   string `json:"search"`
	Sort     string `json:"sort"` // popular, newest, rating
	Tag      string `json:"tag"`
}

// MarketplacePackage represents a marketplace package listing.
type MarketplacePackage struct {
	ID             string   `json:"id"`
	PublisherID    string   `json:"publisher_id"`
	PublisherName  string   `json:"publisher_name"`
	Slug           string   `json:"slug" validate:"required"`
	Name           string   `json:"name" validate:"required"`
	Description    string   `json:"description"`
	Category       string   `json:"category" validate:"required"` // framework, policy_template, report_template, integration, control_pack
	Tags           []string `json:"tags,omitempty"`
	IconURL        string   `json:"icon_url,omitempty"`
	LatestVersion  string   `json:"latest_version"`
	TotalInstalls  int      `json:"total_installs"`
	AverageRating  float64  `json:"average_rating"`
	ReviewCount    int      `json:"review_count"`
	IsFeatured     bool     `json:"is_featured"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// MarketplacePackageDetail extends MarketplacePackage with versions and full description.
type MarketplacePackageDetail struct {
	MarketplacePackage
	LongDescription string           `json:"long_description"`
	Versions        []PackageVersion `json:"versions"`
	Screenshots     []string         `json:"screenshots,omitempty"`
	Requirements    []string         `json:"requirements,omitempty"`
}

// PackageVersion represents a specific version of a marketplace package.
type PackageVersion struct {
	ID          string `json:"id"`
	PackageID   string `json:"package_id"`
	Version     string `json:"version" validate:"required"`
	Changelog   string `json:"changelog,omitempty"`
	MinPlatform string `json:"min_platform_version,omitempty"`
	FileURL     string `json:"file_url,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
	PublishedAt string `json:"published_at"`
}

// PackageReview represents a user review of a marketplace package.
type PackageReview struct {
	ID            string `json:"id"`
	PackageID     string `json:"package_id"`
	PublisherSlug string `json:"publisher_slug,omitempty"`
	PackageSlug   string `json:"package_slug,omitempty"`
	UserID        string `json:"user_id"`
	UserName      string `json:"user_name,omitempty"`
	Rating        int    `json:"rating" validate:"required"` // 1-5
	Title         string `json:"title,omitempty"`
	Body          string `json:"body,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// InstallPackageRequest is the payload for POST /marketplace/install.
type InstallPackageRequest struct {
	PackageID string `json:"package_id" validate:"required"`
	VersionID string `json:"version_id,omitempty"` // defaults to latest
}

// InstalledPackage represents a package installed in an organization.
type InstalledPackage struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	PackageID       string `json:"package_id"`
	PackageName     string `json:"package_name"`
	VersionID       string `json:"version_id"`
	InstalledVersion string `json:"installed_version"`
	Status          string `json:"status"` // active, disabled, update_available
	InstalledBy     string `json:"installed_by"`
	InstalledAt     string `json:"installed_at"`
}

// Publisher represents a marketplace publisher.
type Publisher struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name" validate:"required"`
	Slug           string `json:"slug" validate:"required"`
	Description    string `json:"description,omitempty"`
	Website        string `json:"website,omitempty"`
	Verified       bool   `json:"verified"`
	CreatedAt      string `json:"created_at"`
}

// PublisherStats provides metrics for a publisher.
type PublisherStats struct {
	TotalPackages   int     `json:"total_packages"`
	TotalInstalls   int     `json:"total_installs"`
	AverageRating   float64 `json:"average_rating"`
	TotalReviews    int     `json:"total_reviews"`
	MonthlyInstalls int     `json:"monthly_installs"`
}

// ---------- handler ----------

// MarketplaceHandler handles marketplace endpoints.
type MarketplaceHandler struct {
	svc MarketplaceService
}

// NewMarketplaceHandler creates a new MarketplaceHandler with the given service.
func NewMarketplaceHandler(svc MarketplaceService) *MarketplaceHandler {
	return &MarketplaceHandler{svc: svc}
}

// SearchPackages handles GET /marketplace/packages.
func (h *MarketplaceHandler) SearchPackages(w http.ResponseWriter, r *http.Request) {
	pagination := parsePagination(r)

	filters := PackageFilters{
		Category: r.URL.Query().Get("category"),
		Search:   r.URL.Query().Get("search"),
		Sort:     r.URL.Query().Get("sort"),
		Tag:      r.URL.Query().Get("tag"),
	}

	packages, total, err := h.svc.SearchPackages(r.Context(), pagination, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to search packages", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": packages,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// GetFeaturedPackages handles GET /marketplace/packages/featured.
func (h *MarketplaceHandler) GetFeaturedPackages(w http.ResponseWriter, r *http.Request) {
	packages, err := h.svc.GetFeaturedPackages(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get featured packages", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": packages})
}

// GetPackageDetail handles GET /marketplace/packages/{publisher}/{slug}.
func (h *MarketplaceHandler) GetPackageDetail(w http.ResponseWriter, r *http.Request) {
	publisher := chi.URLParam(r, "publisher")
	slug := chi.URLParam(r, "slug")
	if publisher == "" || slug == "" {
		writeError(w, http.StatusBadRequest, "Missing publisher or slug", "")
		return
	}

	detail, err := h.svc.GetPackageDetail(r.Context(), publisher, slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "Package not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// GetPackageReviews handles GET /marketplace/packages/{publisher}/{slug}/reviews.
func (h *MarketplaceHandler) GetPackageReviews(w http.ResponseWriter, r *http.Request) {
	publisher := chi.URLParam(r, "publisher")
	slug := chi.URLParam(r, "slug")
	if publisher == "" || slug == "" {
		writeError(w, http.StatusBadRequest, "Missing publisher or slug", "")
		return
	}

	pagination := parsePagination(r)

	reviews, total, err := h.svc.GetPackageReviews(r.Context(), publisher, slug, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get package reviews", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": reviews,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// InstallPackage handles POST /marketplace/install.
func (h *MarketplaceHandler) InstallPackage(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var req InstallPackageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.PackageID == "" {
		writeError(w, http.StatusBadRequest, "package_id is required", "")
		return
	}

	installed, err := h.svc.InstallPackage(r.Context(), orgID, userID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to install package", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, installed)
}

// UninstallPackage handles DELETE /marketplace/install/{id}.
func (h *MarketplaceHandler) UninstallPackage(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	installID := chi.URLParam(r, "id")
	if installID == "" {
		writeError(w, http.StatusBadRequest, "Missing install ID", "")
		return
	}

	if err := h.svc.UninstallPackage(r.Context(), orgID, installID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to uninstall package", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListInstalled handles GET /marketplace/installed.
func (h *MarketplaceHandler) ListInstalled(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	pagination := parsePagination(r)

	packages, total, err := h.svc.ListInstalled(r.Context(), orgID, pagination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list installed packages", err.Error())
		return
	}

	totalPages := 0
	if pagination.PageSize > 0 {
		totalPages = (total + pagination.PageSize - 1) / pagination.PageSize
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": packages,
		"pagination": models.PaginationResponse{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: total,
			TotalPages: totalPages,
		},
	})
}

// SubmitReview handles POST /marketplace/reviews.
func (h *MarketplaceHandler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var review PackageReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if review.PackageID == "" || review.Rating == 0 {
		writeError(w, http.StatusBadRequest, "package_id and rating are required", "")
		return
	}

	if review.Rating < 1 || review.Rating > 5 {
		writeError(w, http.StatusBadRequest, "rating must be between 1 and 5", "")
		return
	}

	if err := h.svc.SubmitReview(r.Context(), orgID, userID, &review); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to submit review", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, review)
}

// RegisterPublisher handles POST /marketplace/publishers.
func (h *MarketplaceHandler) RegisterPublisher(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	userID := middleware.GetUserIDFromContext(r.Context())

	var pub Publisher
	if err := json.NewDecoder(r.Body).Decode(&pub); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if pub.Name == "" || pub.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug are required", "")
		return
	}

	if err := h.svc.RegisterPublisher(r.Context(), orgID, userID, &pub); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to register publisher", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, pub)
}

// GetPublisherStats handles GET /marketplace/publishers/me/stats.
func (h *MarketplaceHandler) GetPublisherStats(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	stats, err := h.svc.GetPublisherStats(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get publisher stats", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// CreatePackageEntry handles POST /marketplace/publishers/me/packages.
func (h *MarketplaceHandler) CreatePackageEntry(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())

	var pkg MarketplacePackage
	if err := json.NewDecoder(r.Body).Decode(&pkg); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if pkg.Name == "" || pkg.Slug == "" || pkg.Category == "" {
		writeError(w, http.StatusBadRequest, "name, slug, and category are required", "")
		return
	}

	if err := h.svc.CreatePackage(r.Context(), orgID, &pkg); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create package", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, pkg)
}

// PublishVersion handles POST /marketplace/publishers/me/packages/{id}/versions.
func (h *MarketplaceHandler) PublishVersion(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgIDFromContext(r.Context())
	packageID := chi.URLParam(r, "id")
	if packageID == "" {
		writeError(w, http.StatusBadRequest, "Missing package ID", "")
		return
	}

	var version PackageVersion
	if err := json.NewDecoder(r.Body).Decode(&version); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if version.Version == "" {
		writeError(w, http.StatusBadRequest, "version is required", "")
		return
	}

	if err := h.svc.PublishVersion(r.Context(), orgID, packageID, &version); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to publish version", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, version)
}
