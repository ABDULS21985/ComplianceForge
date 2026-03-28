package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// PackageSummary is a brief listing entry for a marketplace package.
type PackageSummary struct {
	ID              string   `json:"id"`
	PublisherSlug   string   `json:"publisher_slug"`
	PublisherName   string   `json:"publisher_name"`
	Slug            string   `json:"slug"`
	Name            string   `json:"name"`
	ShortDesc       string   `json:"short_description"`
	Category        string   `json:"category"`
	Frameworks      []string `json:"frameworks"`
	LatestVersion   string   `json:"latest_version"`
	AvgRating       float64  `json:"avg_rating"`
	TotalReviews    int      `json:"total_reviews"`
	TotalInstalls   int      `json:"total_installs"`
	IsFree          bool     `json:"is_free"`
	PriceEUR        float64  `json:"price_eur"`
	IsVerified      bool     `json:"is_verified"`
	CreatedAt       string   `json:"created_at"`
}

// PackageDetail is the full detail view of a marketplace package.
type PackageDetail struct {
	ID              string                   `json:"id"`
	PublisherSlug   string                   `json:"publisher_slug"`
	PublisherName   string                   `json:"publisher_name"`
	Slug            string                   `json:"slug"`
	Name            string                   `json:"name"`
	ShortDesc       string                   `json:"short_description"`
	LongDesc        string                   `json:"long_description"`
	Category        string                   `json:"category"`
	Frameworks      []string                 `json:"frameworks"`
	Tags            []string                 `json:"tags"`
	LatestVersion   string                   `json:"latest_version"`
	AvgRating       float64                  `json:"avg_rating"`
	TotalReviews    int                      `json:"total_reviews"`
	TotalInstalls   int                      `json:"total_installs"`
	IsFree          bool                     `json:"is_free"`
	PriceEUR        float64                  `json:"price_eur"`
	IsVerified      bool                     `json:"is_verified"`
	Versions        []PackageVersion         `json:"versions"`
	Reviews         []PackageReview          `json:"reviews"`
	ControlsCount   int                      `json:"controls_count"`
	PoliciesCount   int                      `json:"policies_count"`
	MappingsCount   int                      `json:"mappings_count"`
	CreatedAt       string                   `json:"created_at"`
}

// PackageVersion represents a published version of a package.
type PackageVersion struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	ReleaseNotes string `json:"release_notes"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
}

// PackageReview is a user review of a package.
type PackageReview struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Rating    int    `json:"rating"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// InstallResult describes the outcome of installing a package.
type InstallResult struct {
	InstallationID  string `json:"installation_id"`
	PackageID       string `json:"package_id"`
	VersionID       string `json:"version_id"`
	ControlsCreated int    `json:"controls_created"`
	MappingsCreated int    `json:"mappings_created"`
	PoliciesCreated int    `json:"policies_created"`
	InstalledAt     string `json:"installed_at"`
}

// InstalledPackage represents a package installed in an organisation.
type InstalledPackage struct {
	InstallationID string `json:"installation_id"`
	PackageID      string `json:"package_id"`
	PackageName    string `json:"package_name"`
	PackageSlug    string `json:"package_slug"`
	VersionID      string `json:"version_id"`
	Version        string `json:"version"`
	InstalledAt    string `json:"installed_at"`
	Status         string `json:"status"`
}

// CreatePublisherRequest holds the input for creating a publisher.
type CreatePublisherRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Website     string `json:"website"`
	ContactEmail string `json:"contact_email"`
}

// Publisher represents a marketplace content publisher.
type Publisher struct {
	ID           string `json:"id"`
	OrgID        string `json:"organization_id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Description  string `json:"description"`
	Website      string `json:"website"`
	ContactEmail string `json:"contact_email"`
	IsVerified   bool   `json:"is_verified"`
	CreatedAt    string `json:"created_at"`
}

// PublisherStats aggregates metrics for a publisher.
type PublisherStats struct {
	PublisherID    string  `json:"publisher_id"`
	TotalPackages  int     `json:"total_packages"`
	TotalInstalls  int     `json:"total_installs"`
	TotalReviews   int     `json:"total_reviews"`
	AvgRating      float64 `json:"avg_rating"`
	TotalRevenue   float64 `json:"total_revenue_eur"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// MarketplaceService manages the control library marketplace.
type MarketplaceService struct {
	pool *pgxpool.Pool
}

// NewMarketplaceService creates a new MarketplaceService.
func NewMarketplaceService(pool *pgxpool.Pool) *MarketplaceService {
	return &MarketplaceService{pool: pool}
}

// SearchPackages performs full-text search with faceted filtering.
func (ms *MarketplaceService) SearchPackages(ctx context.Context, query string, filters map[string]interface{}, page, pageSize int) ([]PackageSummary, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Build dynamic WHERE clause.
	where := "WHERE mp.status = 'published'"
	args := []interface{}{}
	argIdx := 1

	if query != "" {
		where += fmt.Sprintf(` AND (
			mp.name ILIKE '%%' || $%d || '%%'
			OR mp.short_description ILIKE '%%' || $%d || '%%'
			OR mp.category ILIKE '%%' || $%d || '%%'
		)`, argIdx, argIdx, argIdx)
		args = append(args, query)
		argIdx++
	}

	if cat, ok := filters["category"].(string); ok && cat != "" {
		where += fmt.Sprintf(" AND mp.category = $%d", argIdx)
		args = append(args, cat)
		argIdx++
	}
	if fw, ok := filters["framework"].(string); ok && fw != "" {
		where += fmt.Sprintf(" AND mp.frameworks @> ARRAY[$%d]::text[]", argIdx)
		args = append(args, fw)
		argIdx++
	}
	if free, ok := filters["is_free"].(bool); ok {
		where += fmt.Sprintf(" AND mp.is_free = $%d", argIdx)
		args = append(args, free)
		argIdx++
	}
	if verified, ok := filters["is_verified"].(bool); ok {
		where += fmt.Sprintf(" AND mp.is_verified = $%d", argIdx)
		args = append(args, verified)
		argIdx++
	}

	// Count total.
	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM marketplace_packages mp %s`, where)
	var total int
	err := ms.pool.QueryRow(ctx, countQ, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting packages: %w", err)
	}

	// Fetch page.
	listArgs := append(args, pageSize, offset)
	listQ := fmt.Sprintf(`
		SELECT mp.id, pub.slug, pub.name, mp.slug, mp.name, mp.short_description,
			mp.category, mp.frameworks, mp.latest_version,
			COALESCE(mp.avg_rating, 0), COALESCE(mp.total_reviews, 0),
			COALESCE(mp.total_installs, 0), mp.is_free,
			COALESCE(mp.price_eur, 0), mp.is_verified,
			TO_CHAR(mp.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM marketplace_packages mp
		JOIN marketplace_publishers pub ON pub.id = mp.publisher_id
		%s
		ORDER BY mp.total_installs DESC, mp.avg_rating DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)

	rows, err := ms.pool.Query(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying packages: %w", err)
	}
	defer rows.Close()

	var packages []PackageSummary
	for rows.Next() {
		var p PackageSummary
		if err := rows.Scan(
			&p.ID, &p.PublisherSlug, &p.PublisherName, &p.Slug, &p.Name, &p.ShortDesc,
			&p.Category, &p.Frameworks, &p.LatestVersion,
			&p.AvgRating, &p.TotalReviews, &p.TotalInstalls,
			&p.IsFree, &p.PriceEUR, &p.IsVerified, &p.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning package: %w", err)
		}
		packages = append(packages, p)
	}

	return packages, total, nil
}

// GetPackageDetail returns the full detail for a package by publisher and package slug.
func (ms *MarketplaceService) GetPackageDetail(ctx context.Context, publisherSlug, packageSlug string) (*PackageDetail, error) {
	var pkg PackageDetail
	var longDescPtr *string
	var tagsJSON []byte

	err := ms.pool.QueryRow(ctx, `
		SELECT mp.id, pub.slug, pub.name, mp.slug, mp.name,
			mp.short_description, mp.long_description,
			mp.category, mp.frameworks, mp.tags,
			mp.latest_version, COALESCE(mp.avg_rating, 0),
			COALESCE(mp.total_reviews, 0), COALESCE(mp.total_installs, 0),
			mp.is_free, COALESCE(mp.price_eur, 0), mp.is_verified,
			COALESCE(mp.controls_count, 0), COALESCE(mp.policies_count, 0),
			COALESCE(mp.mappings_count, 0),
			TO_CHAR(mp.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM marketplace_packages mp
		JOIN marketplace_publishers pub ON pub.id = mp.publisher_id
		WHERE pub.slug = $1 AND mp.slug = $2 AND mp.status = 'published'
	`, publisherSlug, packageSlug).Scan(
		&pkg.ID, &pkg.PublisherSlug, &pkg.PublisherName, &pkg.Slug, &pkg.Name,
		&pkg.ShortDesc, &longDescPtr,
		&pkg.Category, &pkg.Frameworks, &tagsJSON,
		&pkg.LatestVersion, &pkg.AvgRating,
		&pkg.TotalReviews, &pkg.TotalInstalls,
		&pkg.IsFree, &pkg.PriceEUR, &pkg.IsVerified,
		&pkg.ControlsCount, &pkg.PoliciesCount, &pkg.MappingsCount,
		&pkg.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("package not found")
		}
		return nil, fmt.Errorf("querying package: %w", err)
	}
	if longDescPtr != nil {
		pkg.LongDesc = *longDescPtr
	}
	if tagsJSON != nil {
		_ = json.Unmarshal(tagsJSON, &pkg.Tags)
	}

	// Fetch versions.
	vRows, err := ms.pool.Query(ctx, `
		SELECT id, version, COALESCE(release_notes, ''), status,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM marketplace_package_versions
		WHERE package_id = $1
		ORDER BY created_at DESC
		LIMIT 10
	`, pkg.ID)
	if err != nil {
		return nil, fmt.Errorf("querying versions: %w", err)
	}
	defer vRows.Close()

	for vRows.Next() {
		var v PackageVersion
		if err := vRows.Scan(&v.ID, &v.Version, &v.ReleaseNotes, &v.Status, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning version: %w", err)
		}
		pkg.Versions = append(pkg.Versions, v)
	}

	// Fetch recent reviews.
	rRows, err := ms.pool.Query(ctx, `
		SELECT id, user_id, rating, COALESCE(title, ''), COALESCE(review_text, ''),
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM marketplace_reviews
		WHERE package_id = $1
		ORDER BY created_at DESC
		LIMIT 10
	`, pkg.ID)
	if err != nil {
		return nil, fmt.Errorf("querying reviews: %w", err)
	}
	defer rRows.Close()

	for rRows.Next() {
		var r PackageReview
		if err := rRows.Scan(&r.ID, &r.UserID, &r.Rating, &r.Title, &r.Text, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning review: %w", err)
		}
		pkg.Reviews = append(pkg.Reviews, r)
	}

	return &pkg, nil
}

// InstallPackage installs a package into an organisation, transactionally
// importing controls, mappings, and policies.
func (ms *MarketplaceService) InstallPackage(ctx context.Context, orgID, packageID, versionID string, config map[string]interface{}) (*InstallResult, error) {
	tx, err := ms.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check for duplicate installation.
	var existing *string
	_ = tx.QueryRow(ctx, `
		SELECT id FROM marketplace_installations
		WHERE organization_id = $1 AND package_id = $2 AND status = 'active'
	`, orgID, packageID).Scan(&existing)
	if existing != nil {
		return nil, fmt.Errorf("package already installed")
	}

	// Fetch version data.
	var versionData []byte
	err = tx.QueryRow(ctx, `
		SELECT package_data FROM marketplace_package_versions
		WHERE id = $1 AND package_id = $2 AND status = 'published'
	`, versionID, packageID).Scan(&versionData)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("package version not found or not published")
		}
		return nil, fmt.Errorf("fetching version data: %w", err)
	}

	var pkgData struct {
		Controls []struct {
			Code        string `json:"code"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Domain      string `json:"domain"`
		} `json:"controls"`
		Mappings []struct {
			SourceCode string `json:"source_code"`
			TargetCode string `json:"target_code"`
			Relation   string `json:"relation"`
		} `json:"mappings"`
		Policies []struct {
			Title   string `json:"title"`
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"policies"`
	}
	if versionData != nil {
		_ = json.Unmarshal(versionData, &pkgData)
	}

	configJSON, _ := json.Marshal(config)

	// Create installation record.
	var installID string
	err = tx.QueryRow(ctx, `
		INSERT INTO marketplace_installations (
			organization_id, package_id, version_id, status, config, installed_at
		) VALUES ($1, $2, $3, 'active', $4, NOW())
		RETURNING id
	`, orgID, packageID, versionID, configJSON).Scan(&installID)
	if err != nil {
		return nil, fmt.Errorf("creating installation: %w", err)
	}

	controlsCreated := 0
	mappingsCreated := 0
	policiesCreated := 0

	// Import controls.
	for _, ctrl := range pkgData.Controls {
		_, err := tx.Exec(ctx, `
			INSERT INTO marketplace_installed_controls (
				installation_id, organization_id, control_code, control_title,
				description, domain, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, NOW())
			ON CONFLICT DO NOTHING
		`, installID, orgID, ctrl.Code, ctrl.Title, ctrl.Description, ctrl.Domain)
		if err != nil {
			log.Warn().Err(err).Str("code", ctrl.Code).Msg("marketplace: failed to import control")
			continue
		}
		controlsCreated++
	}

	// Import mappings.
	for _, m := range pkgData.Mappings {
		_, err := tx.Exec(ctx, `
			INSERT INTO marketplace_installed_mappings (
				installation_id, organization_id, source_code, target_code,
				relation_type, created_at
			) VALUES ($1, $2, $3, $4, $5, NOW())
			ON CONFLICT DO NOTHING
		`, installID, orgID, m.SourceCode, m.TargetCode, m.Relation)
		if err != nil {
			log.Warn().Err(err).Msg("marketplace: failed to import mapping")
			continue
		}
		mappingsCreated++
	}

	// Import policies.
	for _, p := range pkgData.Policies {
		_, err := tx.Exec(ctx, `
			INSERT INTO marketplace_installed_policies (
				installation_id, organization_id, title, policy_type,
				content, status, created_at
			) VALUES ($1, $2, $3, $4, $5, 'draft', NOW())
			ON CONFLICT DO NOTHING
		`, installID, orgID, p.Title, p.Type, p.Content)
		if err != nil {
			log.Warn().Err(err).Msg("marketplace: failed to import policy")
			continue
		}
		policiesCreated++
	}

	// Increment install counter.
	_, _ = tx.Exec(ctx, `
		UPDATE marketplace_packages
		SET total_installs = COALESCE(total_installs, 0) + 1
		WHERE id = $1
	`, packageID)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	log.Info().
		Str("installation_id", installID).
		Int("controls", controlsCreated).
		Int("mappings", mappingsCreated).
		Int("policies", policiesCreated).
		Msg("marketplace: package installed")

	return &InstallResult{
		InstallationID:  installID,
		PackageID:       packageID,
		VersionID:       versionID,
		ControlsCreated: controlsCreated,
		MappingsCreated: mappingsCreated,
		PoliciesCreated: policiesCreated,
		InstalledAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// UninstallPackage removes a package installation and its imported artefacts.
func (ms *MarketplaceService) UninstallPackage(ctx context.Context, orgID, installationID string) error {
	tx, err := ms.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Verify ownership.
	var pkgID string
	err = tx.QueryRow(ctx, `
		SELECT package_id FROM marketplace_installations
		WHERE id = $1 AND organization_id = $2 AND status = 'active'
	`, installationID, orgID).Scan(&pkgID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("installation not found")
		}
		return fmt.Errorf("querying installation: %w", err)
	}

	// Remove imported artefacts.
	_, _ = tx.Exec(ctx, `DELETE FROM marketplace_installed_controls WHERE installation_id = $1`, installationID)
	_, _ = tx.Exec(ctx, `DELETE FROM marketplace_installed_mappings WHERE installation_id = $1`, installationID)
	_, _ = tx.Exec(ctx, `DELETE FROM marketplace_installed_policies WHERE installation_id = $1`, installationID)

	// Mark installation as uninstalled.
	_, err = tx.Exec(ctx, `
		UPDATE marketplace_installations
		SET status = 'uninstalled', uninstalled_at = NOW()
		WHERE id = $1
	`, installationID)
	if err != nil {
		return fmt.Errorf("updating installation: %w", err)
	}

	// Decrement install counter.
	_, _ = tx.Exec(ctx, `
		UPDATE marketplace_packages
		SET total_installs = GREATEST(COALESCE(total_installs, 0) - 1, 0)
		WHERE id = $1
	`, pkgID)

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	log.Info().Str("installation_id", installationID).Msg("marketplace: package uninstalled")
	return nil
}

// GetInstalledPackages returns all active installations for an organisation.
func (ms *MarketplaceService) GetInstalledPackages(ctx context.Context, orgID string) ([]InstalledPackage, error) {
	rows, err := ms.pool.Query(ctx, `
		SELECT mi.id, mi.package_id, mp.name, mp.slug, mi.version_id,
			mpv.version,
			TO_CHAR(mi.installed_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			mi.status
		FROM marketplace_installations mi
		JOIN marketplace_packages mp ON mp.id = mi.package_id
		JOIN marketplace_package_versions mpv ON mpv.id = mi.version_id
		WHERE mi.organization_id = $1 AND mi.status = 'active'
		ORDER BY mi.installed_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying installations: %w", err)
	}
	defer rows.Close()

	var installed []InstalledPackage
	for rows.Next() {
		var ip InstalledPackage
		if err := rows.Scan(&ip.InstallationID, &ip.PackageID, &ip.PackageName, &ip.PackageSlug,
			&ip.VersionID, &ip.Version, &ip.InstalledAt, &ip.Status); err != nil {
			return nil, fmt.Errorf("scanning installation: %w", err)
		}
		installed = append(installed, ip)
	}

	return installed, nil
}

// SubmitReview creates or updates a review for a package.
func (ms *MarketplaceService) SubmitReview(ctx context.Context, orgID, userID, packageID string, rating int, title, text string) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5")
	}

	// Upsert review.
	_, err := ms.pool.Exec(ctx, `
		INSERT INTO marketplace_reviews (
			package_id, organization_id, user_id, rating, title, review_text, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (package_id, user_id) DO UPDATE
		SET rating = EXCLUDED.rating, title = EXCLUDED.title,
		    review_text = EXCLUDED.review_text, updated_at = NOW()
	`, packageID, orgID, userID, rating, title, text)
	if err != nil {
		return fmt.Errorf("submitting review: %w", err)
	}

	// Update aggregate rating on the package.
	_, _ = ms.pool.Exec(ctx, `
		UPDATE marketplace_packages
		SET avg_rating = sub.avg, total_reviews = sub.cnt
		FROM (
			SELECT AVG(rating)::float8 AS avg, COUNT(*)::int AS cnt
			FROM marketplace_reviews WHERE package_id = $1
		) sub
		WHERE marketplace_packages.id = $1
	`, packageID)

	log.Info().Str("package_id", packageID).Int("rating", rating).Msg("marketplace: review submitted")
	return nil
}

// CreatePublisher registers a new marketplace publisher for an organisation.
func (ms *MarketplaceService) CreatePublisher(ctx context.Context, orgID string, req CreatePublisherRequest) (*Publisher, error) {
	var pub Publisher
	err := ms.pool.QueryRow(ctx, `
		INSERT INTO marketplace_publishers (
			organization_id, name, slug, description, website, contact_email,
			is_verified, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, false, NOW())
		RETURNING id, organization_id, name, slug, description, website, contact_email,
			is_verified, TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	`, orgID, req.Name, req.Slug, req.Description, req.Website, req.ContactEmail).Scan(
		&pub.ID, &pub.OrgID, &pub.Name, &pub.Slug, &pub.Description,
		&pub.Website, &pub.ContactEmail, &pub.IsVerified, &pub.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating publisher: %w", err)
	}

	log.Info().Str("publisher_id", pub.ID).Str("slug", pub.Slug).Msg("marketplace: publisher created")
	return &pub, nil
}

// PublishVersion creates a new version for a package.
func (ms *MarketplaceService) PublishVersion(ctx context.Context, packageID string, version string, data map[string]interface{}, releaseNotes string) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshalling package data: %w", err)
	}

	var versionID string
	err = ms.pool.QueryRow(ctx, `
		INSERT INTO marketplace_package_versions (
			package_id, version, package_data, release_notes, status, created_at
		) VALUES ($1, $2, $3, $4, 'published', NOW())
		RETURNING id
	`, packageID, version, dataJSON, releaseNotes).Scan(&versionID)
	if err != nil {
		return fmt.Errorf("creating version: %w", err)
	}

	// Update latest version on the package.
	_, _ = ms.pool.Exec(ctx, `
		UPDATE marketplace_packages SET latest_version = $2, updated_at = NOW() WHERE id = $1
	`, packageID, version)

	log.Info().Str("package_id", packageID).Str("version", version).Msg("marketplace: version published")
	return nil
}

// GetPublisherStats returns aggregate statistics for a publisher.
func (ms *MarketplaceService) GetPublisherStats(ctx context.Context, publisherID string) (*PublisherStats, error) {
	var stats PublisherStats
	stats.PublisherID = publisherID

	err := ms.pool.QueryRow(ctx, `
		SELECT
			COUNT(DISTINCT mp.id)::int AS total_packages,
			COALESCE(SUM(mp.total_installs), 0)::int AS total_installs,
			COALESCE(SUM(mp.total_reviews), 0)::int AS total_reviews,
			COALESCE(AVG(mp.avg_rating), 0)::float8 AS avg_rating
		FROM marketplace_packages mp
		WHERE mp.publisher_id = $1
	`, publisherID).Scan(&stats.TotalPackages, &stats.TotalInstalls, &stats.TotalReviews, &stats.AvgRating)
	if err != nil {
		return nil, fmt.Errorf("querying publisher stats: %w", err)
	}

	// Revenue from paid installs.
	err = ms.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(mp.price_eur), 0)::float8
		FROM marketplace_installations mi
		JOIN marketplace_packages mp ON mp.id = mi.package_id
		WHERE mp.publisher_id = $1 AND mi.status = 'active' AND mp.is_free = false
	`, publisherID).Scan(&stats.TotalRevenue)
	if err != nil {
		// Non-fatal; just log and continue.
		log.Warn().Err(err).Msg("marketplace: failed to query revenue")
	}

	return &stats, nil
}
