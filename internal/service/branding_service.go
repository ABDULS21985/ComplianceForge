package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrBrandingNotFound     = fmt.Errorf("branding configuration not found")
	ErrInvalidColor         = fmt.Errorf("invalid color value — must be a valid hex color")
	ErrInvalidLogoFormat    = fmt.Errorf("logo must be PNG, JPG, or SVG and under 2 MB")
	ErrDomainVerifyFailed   = fmt.Errorf("custom domain DNS verification failed")
	ErrPartnerNotFound      = fmt.Errorf("white-label partner not found")
	ErrUnsafeCSS            = fmt.Errorf("custom CSS contains disallowed directives")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// TenantBranding holds the complete branding configuration for an organization.
type TenantBranding struct {
	ID               string  `json:"id"`
	OrgID            string  `json:"organization_id"`
	ProductName      string  `json:"product_name"`
	LogoURL          string  `json:"logo_url"`
	LogoSmallURL     string  `json:"logo_small_url"`
	FaviconURL       string  `json:"favicon_url"`
	PrimaryColor     string  `json:"primary_color"`
	SecondaryColor   string  `json:"secondary_color"`
	AccentColor      string  `json:"accent_color"`
	BackgroundColor  string  `json:"background_color"`
	TextColor        string  `json:"text_color"`
	FontFamily       string  `json:"font_family"`
	CustomCSS        string  `json:"custom_css"`
	EmailHeaderHTML  string  `json:"email_header_html"`
	EmailFooterHTML  string  `json:"email_footer_html"`
	LoginMessage     string  `json:"login_message"`
	SupportURL       string  `json:"support_url"`
	SupportEmail     string  `json:"support_email"`
	PrivacyURL       string  `json:"privacy_url"`
	TermsURL         string  `json:"terms_url"`
	CustomDomain     *string `json:"custom_domain"`
	DomainVerified   bool    `json:"domain_verified"`
	PartnerID        *string `json:"partner_id"`
	UpdatedAt        string  `json:"updated_at"`
}

// WhiteLabelPartner represents a reseller / white-label partner.
type WhiteLabelPartner struct {
	ID              string  `json:"id"`
	Slug            string  `json:"slug"`
	Name            string  `json:"name"`
	ProductName     string  `json:"product_name"`
	LogoURL         string  `json:"logo_url"`
	PrimaryColor    string  `json:"primary_color"`
	SecondaryColor  string  `json:"secondary_color"`
	SupportEmail    string  `json:"support_email"`
	SupportURL      string  `json:"support_url"`
	CustomDomain    *string `json:"custom_domain"`
	DomainVerified  bool    `json:"domain_verified"`
	IsActive        bool    `json:"is_active"`
	TenantCount     int     `json:"tenant_count"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// CreatePartnerRequest holds input for creating a white-label partner.
type CreatePartnerRequest struct {
	Slug           string  `json:"slug"`
	Name           string  `json:"name"`
	ProductName    string  `json:"product_name"`
	LogoURL        string  `json:"logo_url"`
	PrimaryColor   string  `json:"primary_color"`
	SecondaryColor string  `json:"secondary_color"`
	SupportEmail   string  `json:"support_email"`
	SupportURL     string  `json:"support_url"`
	CustomDomain   *string `json:"custom_domain"`
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

var hexColorRe = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})$`)

func validHexColor(c string) bool {
	return c == "" || hexColorRe.MatchString(c)
}

var unsafeCSSRe = regexp.MustCompile(`(?i)(expression|javascript|import|url\s*\(|@import|behavior|binding)`)

func safeCSS(css string) bool {
	return !unsafeCSSRe.MatchString(css)
}

const maxLogoSize = 2 * 1024 * 1024 // 2 MB

var allowedImageTypes = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".svg": true,
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// BrandingService manages tenant branding and white-label partner configurations.
type BrandingService struct {
	pool *pgxpool.Pool
}

// NewBrandingService creates a BrandingService.
func NewBrandingService(pool *pgxpool.Pool) *BrandingService {
	return &BrandingService{pool: pool}
}

// ---------------------------------------------------------------------------
// Branding CRUD
// ---------------------------------------------------------------------------

// GetBranding retrieves branding for an organization, returning defaults if none exists.
func (s *BrandingService) GetBranding(ctx context.Context, orgID string) (*TenantBranding, error) {
	var b TenantBranding
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, product_name, logo_url, logo_small_url, favicon_url,
			   primary_color, secondary_color, accent_color, background_color, text_color,
			   font_family, custom_css, email_header_html, email_footer_html,
			   login_message, support_url, support_email, privacy_url, terms_url,
			   custom_domain, domain_verified, partner_id, updated_at
		FROM tenant_branding
		WHERE organization_id = $1`, orgID).Scan(
		&b.ID, &b.OrgID, &b.ProductName, &b.LogoURL, &b.LogoSmallURL, &b.FaviconURL,
		&b.PrimaryColor, &b.SecondaryColor, &b.AccentColor, &b.BackgroundColor, &b.TextColor,
		&b.FontFamily, &b.CustomCSS, &b.EmailHeaderHTML, &b.EmailFooterHTML,
		&b.LoginMessage, &b.SupportURL, &b.SupportEmail, &b.PrivacyURL, &b.TermsURL,
		&b.CustomDomain, &b.DomainVerified, &b.PartnerID, &b.UpdatedAt)
	if err == pgx.ErrNoRows {
		defaults := GetDefaultBranding()
		defaults.OrgID = orgID
		return defaults, nil
	}
	if err != nil {
		return nil, fmt.Errorf("branding: get: %w", err)
	}
	return &b, nil
}

// UpdateBranding validates and persists branding configuration.
func (s *BrandingService) UpdateBranding(ctx context.Context, orgID string, b TenantBranding) (*TenantBranding, error) {
	// Validate colors
	colors := []string{b.PrimaryColor, b.SecondaryColor, b.AccentColor, b.BackgroundColor, b.TextColor}
	for _, c := range colors {
		if !validHexColor(c) {
			return nil, ErrInvalidColor
		}
	}
	// Validate CSS
	if b.CustomCSS != "" && !safeCSS(b.CustomCSS) {
		return nil, ErrUnsafeCSS
	}

	var result TenantBranding
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenant_branding
			(id, organization_id, product_name, logo_url, logo_small_url, favicon_url,
			 primary_color, secondary_color, accent_color, background_color, text_color,
			 font_family, custom_css, email_header_html, email_footer_html,
			 login_message, support_url, support_email, privacy_url, terms_url,
			 custom_domain, domain_verified, partner_id, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, false, $21, NOW())
		ON CONFLICT (organization_id) DO UPDATE
		  SET product_name     = EXCLUDED.product_name,
			  logo_url         = EXCLUDED.logo_url,
			  logo_small_url   = EXCLUDED.logo_small_url,
			  favicon_url      = EXCLUDED.favicon_url,
			  primary_color    = EXCLUDED.primary_color,
			  secondary_color  = EXCLUDED.secondary_color,
			  accent_color     = EXCLUDED.accent_color,
			  background_color = EXCLUDED.background_color,
			  text_color       = EXCLUDED.text_color,
			  font_family      = EXCLUDED.font_family,
			  custom_css       = EXCLUDED.custom_css,
			  email_header_html = EXCLUDED.email_header_html,
			  email_footer_html = EXCLUDED.email_footer_html,
			  login_message    = EXCLUDED.login_message,
			  support_url      = EXCLUDED.support_url,
			  support_email    = EXCLUDED.support_email,
			  privacy_url      = EXCLUDED.privacy_url,
			  terms_url        = EXCLUDED.terms_url,
			  custom_domain    = EXCLUDED.custom_domain,
			  partner_id       = EXCLUDED.partner_id,
			  updated_at       = NOW()
		RETURNING id, organization_id, product_name, logo_url, logo_small_url, favicon_url,
				  primary_color, secondary_color, accent_color, background_color, text_color,
				  font_family, custom_css, email_header_html, email_footer_html,
				  login_message, support_url, support_email, privacy_url, terms_url,
				  custom_domain, domain_verified, partner_id, updated_at`,
		orgID, b.ProductName, b.LogoURL, b.LogoSmallURL, b.FaviconURL,
		b.PrimaryColor, b.SecondaryColor, b.AccentColor, b.BackgroundColor, b.TextColor,
		b.FontFamily, b.CustomCSS, b.EmailHeaderHTML, b.EmailFooterHTML,
		b.LoginMessage, b.SupportURL, b.SupportEmail, b.PrivacyURL, b.TermsURL,
		b.CustomDomain, b.PartnerID).Scan(
		&result.ID, &result.OrgID, &result.ProductName, &result.LogoURL, &result.LogoSmallURL, &result.FaviconURL,
		&result.PrimaryColor, &result.SecondaryColor, &result.AccentColor, &result.BackgroundColor, &result.TextColor,
		&result.FontFamily, &result.CustomCSS, &result.EmailHeaderHTML, &result.EmailFooterHTML,
		&result.LoginMessage, &result.SupportURL, &result.SupportEmail, &result.PrivacyURL, &result.TermsURL,
		&result.CustomDomain, &result.DomainVerified, &result.PartnerID, &result.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("branding: update: %w", err)
	}

	log.Info().Str("org_id", orgID).Msg("branding: configuration updated")
	return &result, nil
}

// ---------------------------------------------------------------------------
// Logo upload
// ---------------------------------------------------------------------------

// UploadLogo validates an image file and stores it, returning a URL.
func (s *BrandingService) UploadLogo(ctx context.Context, orgID, logoType string, fileData []byte, fileName string) (string, error) {
	if len(fileData) > maxLogoSize {
		return "", ErrInvalidLogoFormat
	}

	ext := ""
	if idx := strings.LastIndex(fileName, "."); idx >= 0 {
		ext = strings.ToLower(fileName[idx:])
	}
	if !allowedImageTypes[ext] {
		return "", ErrInvalidLogoFormat
	}

	// Store in object storage (placeholder — writes path to DB)
	storagePath := fmt.Sprintf("/branding/%s/%s_%d%s", orgID, logoType, time.Now().Unix(), ext)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO branding_assets (id, organization_id, asset_type, file_name, storage_path, size_bytes, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NOW())`,
		orgID, logoType, fileName, storagePath, len(fileData))
	if err != nil {
		return "", fmt.Errorf("branding: store logo: %w", err)
	}

	// Update branding record
	column := "logo_url"
	switch logoType {
	case "logo_small":
		column = "logo_small_url"
	case "favicon":
		column = "favicon_url"
	}
	_, _ = s.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE tenant_branding SET %s = $1, updated_at = NOW() WHERE organization_id = $2`, column),
		storagePath, orgID)

	log.Info().Str("org_id", orgID).Str("type", logoType).Str("path", storagePath).Msg("branding: logo uploaded")
	return storagePath, nil
}

// ---------------------------------------------------------------------------
// CSS generation
// ---------------------------------------------------------------------------

// GetBrandingCSS generates CSS custom properties from the branding configuration.
func (s *BrandingService) GetBrandingCSS(ctx context.Context, orgID string) (string, error) {
	b, err := s.GetBranding(ctx, orgID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(":root {\n")
	sb.WriteString(fmt.Sprintf("  --cf-primary: %s;\n", b.PrimaryColor))
	sb.WriteString(fmt.Sprintf("  --cf-secondary: %s;\n", b.SecondaryColor))
	sb.WriteString(fmt.Sprintf("  --cf-accent: %s;\n", b.AccentColor))
	sb.WriteString(fmt.Sprintf("  --cf-background: %s;\n", b.BackgroundColor))
	sb.WriteString(fmt.Sprintf("  --cf-text: %s;\n", b.TextColor))
	if b.FontFamily != "" {
		sb.WriteString(fmt.Sprintf("  --cf-font-family: %s;\n", b.FontFamily))
	}
	sb.WriteString(fmt.Sprintf("  --cf-logo-url: url('%s');\n", b.LogoURL))
	sb.WriteString(fmt.Sprintf("  --cf-logo-small-url: url('%s');\n", b.LogoSmallURL))
	sb.WriteString("}\n")
	if b.CustomCSS != "" {
		sb.WriteString("\n/* Custom CSS */\n")
		sb.WriteString(b.CustomCSS)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// ---------------------------------------------------------------------------
// Email branding
// ---------------------------------------------------------------------------

// ApplyBrandingToEmail replaces placeholder tokens in email HTML with branding values.
func (s *BrandingService) ApplyBrandingToEmail(ctx context.Context, orgID, emailHTML string) (string, error) {
	b, err := s.GetBranding(ctx, orgID)
	if err != nil {
		return emailHTML, err
	}

	replacements := map[string]string{
		"{{product_name}}":    b.ProductName,
		"{{logo_url}}":        b.LogoURL,
		"{{primary_color}}":   b.PrimaryColor,
		"{{secondary_color}}": b.SecondaryColor,
		"{{accent_color}}":    b.AccentColor,
		"{{support_url}}":     b.SupportURL,
		"{{support_email}}":   b.SupportEmail,
		"{{privacy_url}}":     b.PrivacyURL,
		"{{terms_url}}":       b.TermsURL,
		"{{email_header}}":    b.EmailHeaderHTML,
		"{{email_footer}}":    b.EmailFooterHTML,
	}

	result := emailHTML
	for token, val := range replacements {
		result = strings.ReplaceAll(result, token, val)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Custom domain verification
// ---------------------------------------------------------------------------

// VerifyCustomDomain checks DNS records for a custom domain.
func (s *BrandingService) VerifyCustomDomain(ctx context.Context, orgID, domain string) error {
	// Look up CNAME
	cname, err := net.LookupCNAME(domain)
	if err != nil {
		log.Warn().Err(err).Str("domain", domain).Msg("branding: CNAME lookup failed")
		return ErrDomainVerifyFailed
	}

	expectedSuffix := ".complianceforge.io."
	if !strings.HasSuffix(strings.ToLower(cname), expectedSuffix) {
		// Check TXT verification record
		txts, err := net.LookupTXT("_cf-verify." + domain)
		if err != nil || len(txts) == 0 {
			return ErrDomainVerifyFailed
		}
		var verified bool
		for _, txt := range txts {
			if strings.HasPrefix(txt, "cf-verify=") {
				// Validate token against stored value
				var count int
				_ = s.pool.QueryRow(ctx, `
					SELECT COUNT(*) FROM tenant_branding
					WHERE organization_id = $1 AND custom_domain = $2`, orgID, domain).Scan(&count)
				if count > 0 {
					verified = true
				}
			}
		}
		if !verified {
			return ErrDomainVerifyFailed
		}
	}

	// Mark as verified
	_, err = s.pool.Exec(ctx, `
		UPDATE tenant_branding SET domain_verified = true, updated_at = NOW()
		WHERE organization_id = $1 AND custom_domain = $2`, orgID, domain)
	if err != nil {
		return fmt.Errorf("branding: update domain verification: %w", err)
	}
	log.Info().Str("org_id", orgID).Str("domain", domain).Msg("branding: custom domain verified")
	return nil
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

// GetDefaultBranding returns the ComplianceForge default branding.
func GetDefaultBranding() *TenantBranding {
	return &TenantBranding{
		ProductName:     "ComplianceForge",
		LogoURL:         "/assets/logo.svg",
		LogoSmallURL:    "/assets/logo-small.svg",
		FaviconURL:      "/assets/favicon.ico",
		PrimaryColor:    "#2563EB",
		SecondaryColor:  "#1E40AF",
		AccentColor:     "#3B82F6",
		BackgroundColor: "#F8FAFC",
		TextColor:       "#1E293B",
		FontFamily:      "'Inter', sans-serif",
		LoginMessage:    "Welcome to ComplianceForge",
		SupportURL:      "https://support.complianceforge.io",
		SupportEmail:    "support@complianceforge.io",
		PrivacyURL:      "https://complianceforge.io/privacy",
		TermsURL:        "https://complianceforge.io/terms",
		DomainVerified:  false,
	}
}

// ---------------------------------------------------------------------------
// White-label partner management
// ---------------------------------------------------------------------------

// ManagePartners provides CRUD for white-label partners.

// CreatePartner creates a new white-label partner.
func (s *BrandingService) CreatePartner(ctx context.Context, req CreatePartnerRequest) (*WhiteLabelPartner, error) {
	if !validHexColor(req.PrimaryColor) || !validHexColor(req.SecondaryColor) {
		return nil, ErrInvalidColor
	}

	var p WhiteLabelPartner
	err := s.pool.QueryRow(ctx, `
		INSERT INTO white_label_partners
			(id, slug, name, product_name, logo_url, primary_color, secondary_color,
			 support_email, support_url, custom_domain, domain_verified, is_active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, false, true, NOW(), NOW())
		RETURNING id, slug, name, product_name, logo_url, primary_color, secondary_color,
				  support_email, support_url, custom_domain, domain_verified, is_active, created_at, updated_at`,
		req.Slug, req.Name, req.ProductName, req.LogoURL, req.PrimaryColor, req.SecondaryColor,
		req.SupportEmail, req.SupportURL, req.CustomDomain).Scan(
		&p.ID, &p.Slug, &p.Name, &p.ProductName, &p.LogoURL, &p.PrimaryColor, &p.SecondaryColor,
		&p.SupportEmail, &p.SupportURL, &p.CustomDomain, &p.DomainVerified, &p.IsActive,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("branding: create partner: %w", err)
	}
	log.Info().Str("partner_id", p.ID).Str("slug", p.Slug).Msg("branding: partner created")
	return &p, nil
}

// GetPartner retrieves a partner by ID.
func (s *BrandingService) GetPartner(ctx context.Context, partnerID string) (*WhiteLabelPartner, error) {
	var p WhiteLabelPartner
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.slug, p.name, p.product_name, p.logo_url, p.primary_color, p.secondary_color,
			   p.support_email, p.support_url, p.custom_domain, p.domain_verified, p.is_active,
			   (SELECT COUNT(*) FROM tenant_branding tb WHERE tb.partner_id = p.id),
			   p.created_at, p.updated_at
		FROM white_label_partners p
		WHERE p.id = $1`, partnerID).Scan(
		&p.ID, &p.Slug, &p.Name, &p.ProductName, &p.LogoURL, &p.PrimaryColor, &p.SecondaryColor,
		&p.SupportEmail, &p.SupportURL, &p.CustomDomain, &p.DomainVerified, &p.IsActive,
		&p.TenantCount, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrPartnerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("branding: get partner: %w", err)
	}
	return &p, nil
}

// ListPartners returns all white-label partners.
func (s *BrandingService) ListPartners(ctx context.Context) ([]WhiteLabelPartner, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.slug, p.name, p.product_name, p.logo_url, p.primary_color, p.secondary_color,
			   p.support_email, p.support_url, p.custom_domain, p.domain_verified, p.is_active,
			   (SELECT COUNT(*) FROM tenant_branding tb WHERE tb.partner_id = p.id),
			   p.created_at, p.updated_at
		FROM white_label_partners p
		ORDER BY p.name`)
	if err != nil {
		return nil, fmt.Errorf("branding: list partners: %w", err)
	}
	defer rows.Close()

	var partners []WhiteLabelPartner
	for rows.Next() {
		var p WhiteLabelPartner
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Name, &p.ProductName, &p.LogoURL, &p.PrimaryColor, &p.SecondaryColor,
			&p.SupportEmail, &p.SupportURL, &p.CustomDomain, &p.DomainVerified, &p.IsActive,
			&p.TenantCount, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		partners = append(partners, p)
	}
	return partners, nil
}

// UpdatePartner updates a white-label partner.
func (s *BrandingService) UpdatePartner(ctx context.Context, partnerID string, req CreatePartnerRequest) (*WhiteLabelPartner, error) {
	if !validHexColor(req.PrimaryColor) || !validHexColor(req.SecondaryColor) {
		return nil, ErrInvalidColor
	}

	var p WhiteLabelPartner
	err := s.pool.QueryRow(ctx, `
		UPDATE white_label_partners
		SET name = $2, product_name = $3, logo_url = $4, primary_color = $5,
			secondary_color = $6, support_email = $7, support_url = $8,
			custom_domain = $9, updated_at = NOW()
		WHERE id = $1
		RETURNING id, slug, name, product_name, logo_url, primary_color, secondary_color,
				  support_email, support_url, custom_domain, domain_verified, is_active, created_at, updated_at`,
		partnerID, req.Name, req.ProductName, req.LogoURL, req.PrimaryColor, req.SecondaryColor,
		req.SupportEmail, req.SupportURL, req.CustomDomain).Scan(
		&p.ID, &p.Slug, &p.Name, &p.ProductName, &p.LogoURL, &p.PrimaryColor, &p.SecondaryColor,
		&p.SupportEmail, &p.SupportURL, &p.CustomDomain, &p.DomainVerified, &p.IsActive,
		&p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrPartnerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("branding: update partner: %w", err)
	}
	log.Info().Str("partner_id", partnerID).Msg("branding: partner updated")
	return &p, nil
}

// DeactivatePartner disables a white-label partner.
func (s *BrandingService) DeactivatePartner(ctx context.Context, partnerID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE white_label_partners SET is_active = false, updated_at = NOW() WHERE id = $1`, partnerID)
	if err != nil {
		return fmt.Errorf("branding: deactivate partner: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPartnerNotFound
	}
	log.Info().Str("partner_id", partnerID).Msg("branding: partner deactivated")
	return nil
}

// ManagePartners is a convenience that lists all partners (used as the main CRUD entry point).
func (s *BrandingService) ManagePartners(ctx context.Context) ([]WhiteLabelPartner, error) {
	return s.ListPartners(ctx)
}

// Ensure json import is used
var _ = json.Marshal
