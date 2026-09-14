package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/database"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrIntegrationNotFound = errors.New("integration not found")
	ErrAPIKeyNotFound      = errors.New("api key not found")
	ErrAPIKeyInvalid       = errors.New("api key is invalid or expired")
	ErrAPIKeyRevoked       = errors.New("api key has been revoked")
	ErrIntegrationInactive = errors.New("integration must be active before it can sync")
	ErrInvalidSSOConfig    = errors.New("invalid SSO configuration")
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// IntegrationService manages third-party integrations, sync operations, and
// API key lifecycle.
type IntegrationService struct {
	pool   *pgxpool.Pool
	encKey []byte // AES-256 key (32 bytes) for config encryption
}

// Integration represents a configured third-party integration.
type Integration struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"organization_id"`
	IntegrationType string     `json:"integration_type"`
	Name            string     `json:"name"`
	Description     *string    `json:"description"`
	Status          string     `json:"status"`
	HealthStatus    string     `json:"health_status"`
	LastHealthCheck *time.Time `json:"last_health_check_at"`
	LastSyncAt      *time.Time `json:"last_sync_at"`
	SyncFreqMinutes int        `json:"sync_frequency_minutes"`
	ErrorCount      int        `json:"error_count"`
	LastError       *string    `json:"last_error_message"`
	Capabilities    []string   `json:"capabilities"`
	CreatedAt       time.Time  `json:"created_at"`
}

// SyncLog records the result of a synchronisation run.
type SyncLog struct {
	ID               string    `json:"id"`
	IntegrationID    string    `json:"integration_id"`
	SyncType         string    `json:"sync_type"`
	Status           string    `json:"status"`
	RecordsProcessed int       `json:"records_processed"`
	RecordsCreated   int       `json:"records_created"`
	RecordsUpdated   int       `json:"records_updated"`
	RecordsFailed    int       `json:"records_failed"`
	DurationMs       *int      `json:"duration_ms"`
	ErrorMessage     *string   `json:"error_message"`
	CreatedAt        time.Time `json:"created_at"`
}

// APIKey represents an issued API key for programmatic access.
type APIKey struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"organization_id"`
	Name        string     `json:"name"`
	KeyPrefix   string     `json:"key_prefix"`
	Permissions []string   `json:"permissions"`
	RateLimit   int        `json:"rate_limit_per_minute"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
}

// SSOConfiguration is the non-secret SAML/OIDC configuration returned to an
// administrator. OIDC client secrets are write-only and represented only by a
// configured flag.
type SSOConfiguration struct {
	ID                         string         `json:"id,omitempty"`
	OrgID                      string         `json:"organization_id"`
	Protocol                   string         `json:"protocol"`
	Enabled                    bool           `json:"is_enabled"`
	Enforced                   bool           `json:"is_enforced"`
	SAMLEntityID               *string        `json:"saml_entity_id,omitempty"`
	SAMLSSOURL                 *string        `json:"saml_sso_url,omitempty"`
	SAMLSLOURL                 *string        `json:"saml_slo_url,omitempty"`
	SAMLCertificate            *string        `json:"saml_certificate,omitempty"`
	SAMLNameIDFormat           *string        `json:"saml_name_id_format,omitempty"`
	SAMLAttributeMapping       map[string]any `json:"saml_attribute_mapping"`
	OIDCIssuerURL              *string        `json:"oidc_issuer_url,omitempty"`
	OIDCClientID               *string        `json:"oidc_client_id,omitempty"`
	OIDCClientSecretConfigured bool           `json:"oidc_client_secret_configured"`
	OIDCScopes                 []string       `json:"oidc_scopes"`
	OIDCClaimMapping           map[string]any `json:"oidc_claim_mapping"`
	AutoProvisionUsers         bool           `json:"auto_provision_users"`
	DefaultRoleID              *string        `json:"default_role_id,omitempty"`
	AllowedDomains             []string       `json:"allowed_domains"`
	GroupToRoleMapping         map[string]any `json:"group_to_role_mapping"`
	JITProvisioning            bool           `json:"jit_provisioning"`
	CreatedAt                  *time.Time     `json:"created_at,omitempty"`
	UpdatedAt                  *time.Time     `json:"updated_at,omitempty"`
}

// UpdateSSOConfigurationInput contains mutable SSO settings. ClientSecret is
// write-only; nil preserves the existing secret while an explicit value rotates
// it. Switching away from OIDC removes the stored OIDC secret.
type UpdateSSOConfigurationInput struct {
	Protocol             string         `json:"protocol"`
	Enabled              *bool          `json:"is_enabled,omitempty"`
	Enforced             *bool          `json:"is_enforced,omitempty"`
	SAMLEntityID         *string        `json:"saml_entity_id,omitempty"`
	SAMLSSOURL           *string        `json:"saml_sso_url,omitempty"`
	SAMLSLOURL           *string        `json:"saml_slo_url,omitempty"`
	SAMLCertificate      *string        `json:"saml_certificate,omitempty"`
	SAMLNameIDFormat     *string        `json:"saml_name_id_format,omitempty"`
	SAMLAttributeMapping map[string]any `json:"saml_attribute_mapping,omitempty"`
	OIDCIssuerURL        *string        `json:"oidc_issuer_url,omitempty"`
	OIDCClientID         *string        `json:"oidc_client_id,omitempty"`
	OIDCClientSecret     *string        `json:"oidc_client_secret,omitempty"`
	OIDCScopes           []string       `json:"oidc_scopes,omitempty"`
	OIDCClaimMapping     map[string]any `json:"oidc_claim_mapping,omitempty"`
	AutoProvisionUsers   *bool          `json:"auto_provision_users,omitempty"`
	DefaultRoleID        *string        `json:"default_role_id,omitempty"`
	AllowedDomains       []string       `json:"allowed_domains,omitempty"`
	GroupToRoleMapping   map[string]any `json:"group_to_role_mapping,omitempty"`
	JITProvisioning      *bool          `json:"jit_provisioning,omitempty"`
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewIntegrationService creates a service from explicitly supplied key
// material. keyHex must contain a hex-encoded 32-byte AES key. Constructors
// return configuration errors instead of terminating the process so callers
// can fail startup cleanly and tests do not depend on process-global state.
func NewIntegrationService(pool *pgxpool.Pool, keyHex string) (*IntegrationService, error) {
	if pool == nil {
		return nil, errors.New("integration database is required")
	}
	if strings.TrimSpace(keyHex) == "" {
		return nil, errors.New("INTEGRATION_ENCRYPTION_KEY is required")
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("decode INTEGRATION_ENCRYPTION_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("INTEGRATION_ENCRYPTION_KEY must decode to exactly 32 bytes")
	}

	return &IntegrationService{pool: pool, encKey: append([]byte(nil), key...)}, nil
}

// ---------------------------------------------------------------------------
// Integrations CRUD
// ---------------------------------------------------------------------------

// ListIntegrations returns all integrations for an organisation.
func (s *IntegrationService) ListIntegrations(ctx context.Context, orgID string) ([]Integration, error) {
	rows, err := database.QuerierFromContext(ctx, s.pool).Query(ctx, `
		SELECT id, organization_id, integration_type, name, description,
			   status, health_status, last_health_check_at, last_sync_at,
			   sync_frequency_minutes, error_count, last_error_message,
			   capabilities, created_at
		FROM integrations
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	defer rows.Close()

	var results []Integration
	for rows.Next() {
		i, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate integrations: %w", err)
	}
	return results, nil
}

// CreateIntegration creates a new integration with its config encrypted at rest.
func (s *IntegrationService) CreateIntegration(
	ctx context.Context,
	orgID, userID string,
	integ Integration,
	configJSON string,
) (*Integration, error) {
	encConfig, err := s.encryptConfig(orgID, configJSON)
	if err != nil {
		return nil, fmt.Errorf("encrypt config: %w", err)
	}

	err = database.QuerierFromContext(ctx, s.pool).QueryRow(ctx, `
		INSERT INTO integrations
			(organization_id, integration_type, name, description,
			 status, health_status, sync_frequency_minutes,
			 capabilities, configuration_encrypted, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at`,
		orgID, integ.IntegrationType, integ.Name, integ.Description,
		"pending_setup", "unknown", integ.SyncFreqMinutes,
		integ.Capabilities, encConfig, userID,
	).Scan(&integ.ID, &integ.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create integration: %w", err)
	}

	integ.OrgID = orgID
	integ.Status = "pending_setup"
	integ.HealthStatus = "unknown"

	log.Info().
		Str("integration_id", integ.ID).
		Str("type", integ.IntegrationType).
		Msg("integration created")

	return &integ, nil
}

// GetIntegration returns a single integration by ID.
func (s *IntegrationService) GetIntegration(ctx context.Context, orgID, integID string) (*Integration, error) {
	row := database.QuerierFromContext(ctx, s.pool).QueryRow(ctx, `
		SELECT id, organization_id, integration_type, name, description,
			   status, health_status, last_health_check_at, last_sync_at,
			   sync_frequency_minutes, error_count, last_error_message,
			   capabilities, created_at
		FROM integrations
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
		integID, orgID,
	)

	var i Integration
	err := row.Scan(
		&i.ID, &i.OrgID, &i.IntegrationType, &i.Name, &i.Description,
		&i.Status, &i.HealthStatus, &i.LastHealthCheck, &i.LastSyncAt,
		&i.SyncFreqMinutes, &i.ErrorCount, &i.LastError,
		&i.Capabilities, &i.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIntegrationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration: %w", err)
	}
	return &i, nil
}

// UpdateIntegration updates mutable fields of an integration. If configJSON is
// non-nil the encrypted config is replaced.
func (s *IntegrationService) UpdateIntegration(
	ctx context.Context,
	orgID, integID string,
	integ Integration,
	configJSON *string,
) error {
	var encConfig *string
	if configJSON != nil {
		enc, err := s.encryptConfig(orgID, *configJSON)
		if err != nil {
			return fmt.Errorf("encrypt config: %w", err)
		}
		encConfig = &enc
	}

	tag, err := database.QuerierFromContext(ctx, s.pool).Exec(ctx, `
		UPDATE integrations
		SET name                  = COALESCE(NULLIF($1,''), name),
			description           = COALESCE($2, description),
			sync_frequency_minutes= CASE WHEN $3 > 0 THEN $3 ELSE sync_frequency_minutes END,
			capabilities          = COALESCE($4, capabilities),
			configuration_encrypted = COALESCE($5, configuration_encrypted),
			updated_at            = NOW()
		WHERE id = $6 AND organization_id = $7 AND deleted_at IS NULL`,
		integ.Name, integ.Description, integ.SyncFreqMinutes,
		integ.Capabilities, encConfig,
		integID, orgID,
	)
	if err != nil {
		return fmt.Errorf("update integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrIntegrationNotFound
	}
	return nil
}

// DeleteIntegration soft-deletes an integration while retaining its audit and
// sync history. The status enum deliberately remains valid.
func (s *IntegrationService) DeleteIntegration(ctx context.Context, orgID, integID string) error {
	tag, err := database.QuerierFromContext(ctx, s.pool).Exec(ctx, `
		UPDATE integrations
		SET status = 'inactive', deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
		integID, orgID,
	)
	if err != nil {
		return fmt.Errorf("delete integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrIntegrationNotFound
	}
	log.Info().Str("integration_id", integID).Msg("integration deleted")
	return nil
}

// ---------------------------------------------------------------------------
// Connection testing & health
// ---------------------------------------------------------------------------

// TestConnection performs a health check on the integration by decrypting the
// config and calling the appropriate connector. Returns the new health status.
func (s *IntegrationService) TestConnection(ctx context.Context, orgID, integID string) (string, error) {
	// Fetch encrypted config.
	var encConfig string
	var integrationType string
	err := database.QuerierFromContext(ctx, s.pool).QueryRow(ctx, `
		SELECT integration_type, configuration_encrypted
		FROM integrations
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
		integID, orgID,
	).Scan(&integrationType, &encConfig)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrIntegrationNotFound
	}
	if err != nil {
		return "", fmt.Errorf("fetch integration config: %w", err)
	}

	// Decrypt config (we validate we can decrypt; actual connector calls are
	// type-specific and would be dispatched here in a full implementation).
	_, err = s.decryptConfig(orgID, encConfig)
	healthStatus := "healthy"
	var errMsg *string
	if err != nil {
		healthStatus = "unhealthy"
		e := err.Error()
		errMsg = &e
	}

	// Update health status.
	if updateErr := s.UpdateHealthStatus(ctx, orgID, integID, healthStatus, ptrToString(errMsg)); updateErr != nil {
		return healthStatus, updateErr
	}

	log.Info().
		Str("integration_id", integID).
		Str("health", healthStatus).
		Msg("connection test completed")

	return healthStatus, nil
}

// UpdateHealthStatus records a new health status for an integration.
func (s *IntegrationService) UpdateHealthStatus(ctx context.Context, orgID, integID, healthStatus, errorMsg string) error {
	var errPtr *string
	if errorMsg != "" {
		errPtr = &errorMsg
	}

	tag, err := database.QuerierFromContext(ctx, s.pool).Exec(ctx, `
		UPDATE integrations
		SET health_status        = $1,
			last_health_check_at = NOW(),
			last_error_message   = $2,
			error_count          = CASE WHEN $1 = 'healthy' THEN 0 ELSE error_count + 1 END,
			updated_at           = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL`,
		healthStatus, errPtr, integID, orgID,
	)
	if err != nil {
		return fmt.Errorf("update health status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrIntegrationNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sync operations
// ---------------------------------------------------------------------------

// TriggerSync initiates a synchronisation run for the given integration. It
// creates a sync_log record in 'started' state and returns it. The actual sync
// work would be performed asynchronously by a worker.
func (s *IntegrationService) TriggerSync(ctx context.Context, orgID, integID, syncType string) (*SyncLog, error) {
	syncType = strings.TrimSpace(syncType)
	if syncType == "" || len(syncType) > 100 {
		return nil, fmt.Errorf("sync type must contain between 1 and 100 characters")
	}

	querier := database.QuerierFromContext(ctx, s.pool)

	// Verify integration exists and is active.
	var status string
	err := querier.QueryRow(ctx, `
		SELECT status FROM integrations
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
		integID, orgID,
	).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIntegrationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("check integration status: %w", err)
	}
	if status != "active" {
		return nil, ErrIntegrationInactive
	}

	sl := SyncLog{
		IntegrationID: integID,
		SyncType:      syncType,
		Status:        "started",
	}

	err = querier.QueryRow(ctx, `
		INSERT INTO integration_sync_logs
			(organization_id, integration_id, sync_type, status)
		VALUES ($1,$2,$3,$4)
		RETURNING id, created_at`,
		orgID, sl.IntegrationID, sl.SyncType, sl.Status,
	).Scan(&sl.ID, &sl.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create sync log: %w", err)
	}

	log.Info().
		Str("sync_id", sl.ID).
		Str("integration_id", integID).
		Str("sync_type", syncType).
		Msg("sync triggered")

	return &sl, nil
}

// GetSyncLogs returns paginated sync logs for an integration.
func (s *IntegrationService) GetSyncLogs(
	ctx context.Context,
	orgID, integID string,
	page, pageSize int,
) ([]SyncLog, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Verify integration belongs to org.
	var exists bool
	querier := database.QuerierFromContext(ctx, s.pool)
	err := querier.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM integrations WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL)`,
		integID, orgID,
	).Scan(&exists)
	if err != nil {
		return nil, 0, fmt.Errorf("verify integration: %w", err)
	}
	if !exists {
		return nil, 0, ErrIntegrationNotFound
	}

	var total int
	err = querier.QueryRow(ctx, `
		SELECT COUNT(*) FROM integration_sync_logs WHERE integration_id = $1 AND organization_id = $2`,
		integID, orgID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count sync logs: %w", err)
	}

	rows, err := querier.Query(ctx, `
		SELECT id, integration_id, sync_type, status,
			   records_processed, records_created, records_updated, records_failed,
			   duration_ms, error_message, created_at
		FROM integration_sync_logs
		WHERE integration_id = $1 AND organization_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		integID, orgID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query sync logs: %w", err)
	}
	defer rows.Close()

	var logs []SyncLog
	for rows.Next() {
		var sl SyncLog
		if err := rows.Scan(
			&sl.ID, &sl.IntegrationID, &sl.SyncType, &sl.Status,
			&sl.RecordsProcessed, &sl.RecordsCreated, &sl.RecordsUpdated, &sl.RecordsFailed,
			&sl.DurationMs, &sl.ErrorMessage, &sl.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan sync log: %w", err)
		}
		logs = append(logs, sl)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate sync logs: %w", err)
	}

	return logs, total, nil
}

// ---------------------------------------------------------------------------
// SSO configuration
// ---------------------------------------------------------------------------

// GetSSOConfig returns a tenant's non-secret SSO settings. An organization
// without a configuration receives secure defaults rather than a 404.
func (s *IntegrationService) GetSSOConfig(ctx context.Context, orgID string) (*SSOConfiguration, error) {
	row := database.QuerierFromContext(ctx, s.pool).QueryRow(ctx, `
		SELECT id, organization_id, protocol, is_enabled, is_enforced,
		       saml_entity_id, saml_sso_url, saml_slo_url, saml_certificate,
		       saml_name_id_format, COALESCE(saml_attribute_mapping, '{}'::jsonb),
		       oidc_issuer_url, oidc_client_id,
		       oidc_client_secret_encrypted IS NOT NULL,
		       COALESCE(oidc_scopes, '{}'::text[]),
		       COALESCE(oidc_claim_mapping, '{}'::jsonb),
		       auto_provision_users, default_role_id,
		       COALESCE(allowed_domains, '{}'::text[]),
		       COALESCE(group_to_role_mapping, '{}'::jsonb),
		       jit_provisioning, created_at, updated_at
		FROM sso_configurations
		WHERE organization_id = $1`, orgID)

	var config SSOConfiguration
	err := row.Scan(
		&config.ID, &config.OrgID, &config.Protocol, &config.Enabled, &config.Enforced,
		&config.SAMLEntityID, &config.SAMLSSOURL, &config.SAMLSLOURL, &config.SAMLCertificate,
		&config.SAMLNameIDFormat, &config.SAMLAttributeMapping,
		&config.OIDCIssuerURL, &config.OIDCClientID, &config.OIDCClientSecretConfigured,
		&config.OIDCScopes, &config.OIDCClaimMapping,
		&config.AutoProvisionUsers, &config.DefaultRoleID, &config.AllowedDomains,
		&config.GroupToRoleMapping, &config.JITProvisioning, &config.CreatedAt, &config.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return &SSOConfiguration{
			OrgID:                orgID,
			Protocol:             "oidc",
			OIDCScopes:           []string{"openid", "profile", "email"},
			AutoProvisionUsers:   true,
			JITProvisioning:      true,
			SAMLAttributeMapping: map[string]any{},
			OIDCClaimMapping:     map[string]any{},
			GroupToRoleMapping:   map[string]any{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get SSO configuration: %w", err)
	}
	return &config, nil
}

// UpdateSSOConfig validates and atomically upserts tenant SSO settings. Secret
// values are encrypted with tenant- and purpose-bound authenticated encryption
// and are never returned by GetSSOConfig.
func (s *IntegrationService) UpdateSSOConfig(ctx context.Context, orgID string, input UpdateSSOConfigurationInput) error {
	current, err := s.GetSSOConfig(ctx, orgID)
	if err != nil {
		return err
	}

	input.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	if input.Protocol == "saml" {
		input.Protocol = "saml2"
	}
	if input.Protocol == "" {
		input.Protocol = current.Protocol
	}
	if input.Protocol != "saml2" && input.Protocol != "oidc" {
		return fmt.Errorf("%w: protocol must be saml2 or oidc", ErrInvalidSSOConfig)
	}

	input.SAMLEntityID = cleanOptionalString(input.SAMLEntityID)
	input.SAMLSSOURL = cleanOptionalString(input.SAMLSSOURL)
	input.SAMLSLOURL = cleanOptionalString(input.SAMLSLOURL)
	input.SAMLCertificate = cleanOptionalString(input.SAMLCertificate)
	input.SAMLNameIDFormat = cleanOptionalString(input.SAMLNameIDFormat)
	input.OIDCIssuerURL = cleanOptionalString(input.OIDCIssuerURL)
	input.OIDCClientID = cleanOptionalString(input.OIDCClientID)
	input.OIDCClientSecret = cleanOptionalString(input.OIDCClientSecret)
	input.DefaultRoleID = cleanOptionalString(input.DefaultRoleID)

	effectiveEnabled := current.Enabled
	if input.Enabled != nil {
		effectiveEnabled = *input.Enabled
	}
	effectiveEnforced := current.Enforced
	if input.Enforced != nil {
		effectiveEnforced = *input.Enforced
	}
	if effectiveEnforced && !effectiveEnabled {
		return fmt.Errorf("%w: enforced SSO must also be enabled", ErrInvalidSSOConfig)
	}

	if input.DefaultRoleID != nil {
		if _, err := uuid.Parse(*input.DefaultRoleID); err != nil {
			return fmt.Errorf("%w: default_role_id must be a UUID", ErrInvalidSSOConfig)
		}
	}
	if err := normalizeSSODomains(&input); err != nil {
		return err
	}
	if err := validateSSOProtocolSettings(input, current, effectiveEnabled); err != nil {
		return err
	}

	if len(input.OIDCScopes) == 0 {
		input.OIDCScopes = current.OIDCScopes
		if len(input.OIDCScopes) == 0 {
			input.OIDCScopes = []string{"openid", "profile", "email"}
		}
	}
	if !containsString(input.OIDCScopes, "openid") {
		return fmt.Errorf("%w: OIDC scopes must include openid", ErrInvalidSSOConfig)
	}

	var encryptedClientSecret *string
	if input.OIDCClientSecret != nil {
		encrypted, err := s.encryptSecret(orgID, "oidc-client-secret", *input.OIDCClientSecret)
		if err != nil {
			return fmt.Errorf("encrypt OIDC client secret: %w", err)
		}
		encryptedClientSecret = &encrypted
	}

	_, err = database.QuerierFromContext(ctx, s.pool).Exec(ctx, `
		INSERT INTO sso_configurations (
			organization_id, protocol, is_enabled, is_enforced,
			saml_entity_id, saml_sso_url, saml_slo_url, saml_certificate,
			saml_name_id_format, saml_attribute_mapping,
			oidc_issuer_url, oidc_client_id, oidc_client_secret_encrypted,
			oidc_scopes, oidc_claim_mapping, auto_provision_users,
			default_role_id, allowed_domains, group_to_role_mapping, jit_provisioning
		) VALUES (
			$1, $2, COALESCE($3::boolean, false), COALESCE($4::boolean, false),
			$5, $6, $7, $8, $9, COALESCE($10::jsonb, '{}'::jsonb),
			$11, $12, $13, $14, COALESCE($15, '{}'::jsonb),
			COALESCE($16::boolean, true), $17, $18,
			COALESCE($19, '{}'::jsonb), COALESCE($20::boolean, true)
		)
		ON CONFLICT (organization_id) DO UPDATE SET
			protocol = EXCLUDED.protocol,
			is_enabled = COALESCE($3::boolean, sso_configurations.is_enabled),
			is_enforced = COALESCE($4::boolean, sso_configurations.is_enforced),
			saml_entity_id = CASE WHEN EXCLUDED.protocol = 'saml2' THEN COALESCE(EXCLUDED.saml_entity_id, sso_configurations.saml_entity_id) ELSE NULL END,
			saml_sso_url = CASE WHEN EXCLUDED.protocol = 'saml2' THEN COALESCE(EXCLUDED.saml_sso_url, sso_configurations.saml_sso_url) ELSE NULL END,
			saml_slo_url = CASE WHEN EXCLUDED.protocol = 'saml2' THEN COALESCE(EXCLUDED.saml_slo_url, sso_configurations.saml_slo_url) ELSE NULL END,
			saml_certificate = CASE WHEN EXCLUDED.protocol = 'saml2' THEN COALESCE(EXCLUDED.saml_certificate, sso_configurations.saml_certificate) ELSE NULL END,
			saml_name_id_format = CASE WHEN EXCLUDED.protocol = 'saml2' THEN COALESCE(EXCLUDED.saml_name_id_format, sso_configurations.saml_name_id_format) ELSE NULL END,
			saml_attribute_mapping = CASE WHEN EXCLUDED.protocol = 'saml2' THEN COALESCE($10::jsonb, sso_configurations.saml_attribute_mapping) ELSE NULL END,
			oidc_issuer_url = CASE WHEN EXCLUDED.protocol = 'oidc' THEN COALESCE(EXCLUDED.oidc_issuer_url, sso_configurations.oidc_issuer_url) ELSE NULL END,
			oidc_client_id = CASE WHEN EXCLUDED.protocol = 'oidc' THEN COALESCE(EXCLUDED.oidc_client_id, sso_configurations.oidc_client_id) ELSE NULL END,
			oidc_client_secret_encrypted = CASE
				WHEN EXCLUDED.protocol != 'oidc' THEN NULL
				ELSE COALESCE(EXCLUDED.oidc_client_secret_encrypted, sso_configurations.oidc_client_secret_encrypted)
			END,
			oidc_scopes = CASE WHEN EXCLUDED.protocol = 'oidc' THEN COALESCE($14::text[], sso_configurations.oidc_scopes) ELSE '{}'::text[] END,
			oidc_claim_mapping = CASE WHEN EXCLUDED.protocol = 'oidc' THEN COALESCE($15::jsonb, sso_configurations.oidc_claim_mapping) ELSE NULL END,
			auto_provision_users = COALESCE($16::boolean, sso_configurations.auto_provision_users),
			default_role_id = COALESCE(EXCLUDED.default_role_id, sso_configurations.default_role_id),
			allowed_domains = COALESCE($18::text[], sso_configurations.allowed_domains),
			group_to_role_mapping = COALESCE($19::jsonb, sso_configurations.group_to_role_mapping),
			jit_provisioning = COALESCE($20::boolean, sso_configurations.jit_provisioning),
			updated_at = NOW()`,
		orgID, input.Protocol, input.Enabled, input.Enforced,
		input.SAMLEntityID, input.SAMLSSOURL, input.SAMLSLOURL, input.SAMLCertificate,
		input.SAMLNameIDFormat, input.SAMLAttributeMapping,
		input.OIDCIssuerURL, input.OIDCClientID, encryptedClientSecret,
		input.OIDCScopes, input.OIDCClaimMapping, input.AutoProvisionUsers,
		input.DefaultRoleID, input.AllowedDomains, input.GroupToRoleMapping, input.JITProvisioning,
	)
	if err != nil {
		return fmt.Errorf("update SSO configuration: %w", err)
	}
	return nil
}

func validateSSOProtocolSettings(input UpdateSSOConfigurationInput, current *SSOConfiguration, enabled bool) error {
	if !enabled {
		return nil
	}
	if input.Protocol == "saml2" {
		entityID := firstNonEmpty(input.SAMLEntityID, current.SAMLEntityID)
		ssoURL := firstNonEmpty(input.SAMLSSOURL, current.SAMLSSOURL)
		certificate := firstNonEmpty(input.SAMLCertificate, current.SAMLCertificate)
		if entityID == "" || ssoURL == "" || certificate == "" {
			return fmt.Errorf("%w: enabled SAML requires entity ID, SSO URL, and certificate", ErrInvalidSSOConfig)
		}
		if err := validateSecureEndpoint(ssoURL); err != nil {
			return fmt.Errorf("%w: SAML SSO URL: %v", ErrInvalidSSOConfig, err)
		}
		if input.SAMLSLOURL != nil {
			if err := validateSecureEndpoint(*input.SAMLSLOURL); err != nil {
				return fmt.Errorf("%w: SAML SLO URL: %v", ErrInvalidSSOConfig, err)
			}
		}
		if !strings.Contains(certificate, "-----BEGIN CERTIFICATE-----") {
			return fmt.Errorf("%w: SAML certificate must be PEM encoded", ErrInvalidSSOConfig)
		}
		return nil
	}

	issuerURL := firstNonEmpty(input.OIDCIssuerURL, current.OIDCIssuerURL)
	clientID := firstNonEmpty(input.OIDCClientID, current.OIDCClientID)
	secretConfigured := current.Protocol == "oidc" && current.OIDCClientSecretConfigured
	if input.OIDCClientSecret != nil {
		secretConfigured = true
	}
	if issuerURL == "" || clientID == "" || !secretConfigured {
		return fmt.Errorf("%w: enabled OIDC requires issuer URL, client ID, and client secret", ErrInvalidSSOConfig)
	}
	if err := validateSecureEndpoint(issuerURL); err != nil {
		return fmt.Errorf("%w: OIDC issuer URL: %v", ErrInvalidSSOConfig, err)
	}
	return nil
}

func validateSecureEndpoint(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return errors.New("must be an absolute URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return errors.New("must not contain credentials or a fragment")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	hostname := strings.ToLower(parsed.Hostname())
	if parsed.Scheme == "http" && (hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1") {
		return nil
	}
	return errors.New("must use HTTPS")
}

func normalizeSSODomains(input *UpdateSSOConfigurationInput) error {
	if input.AllowedDomains == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(input.AllowedDomains))
	result := make([]string, 0, len(input.AllowedDomains))
	for _, configuredDomain := range input.AllowedDomains {
		domain := strings.ToLower(strings.TrimSpace(configuredDomain))
		if domain == "" || strings.ContainsAny(domain, "@/ :") || !strings.Contains(domain, ".") {
			return fmt.Errorf("%w: invalid allowed domain %q", ErrInvalidSSOConfig, configuredDomain)
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	input.AllowedDomains = result
	return nil
}

func cleanOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func firstNonEmpty(primary, fallback *string) string {
	if primary != nil && *primary != "" {
		return *primary
	}
	if fallback != nil {
		return *fallback
	}
	return ""
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

// ListAPIKeys returns all API keys for an organisation (without the hash).
func (s *IntegrationService) ListAPIKeys(ctx context.Context, orgID string) ([]APIKey, error) {
	rows, err := database.QuerierFromContext(ctx, s.pool).Query(ctx, `
		SELECT id, organization_id, name, key_prefix, permissions,
			   rate_limit_per_minute, expires_at, last_used_at, is_active, created_at
		FROM api_keys
		WHERE organization_id = $1
		ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}
	return keys, nil
}

// CreateAPIKey generates a new API key, stores its SHA-256 hash, and returns
// the full key exactly once.
func (s *IntegrationService) CreateAPIKey(
	ctx context.Context,
	orgID, userID, name string,
	permissions []string,
	rateLimit int,
	expiresAt *time.Time,
) (*APIKey, string, error) {
	// Generate 48 random bytes and encode as base64url. A product-specific
	// prefix makes accidental disclosure easier for secret scanners to detect.
	rawKey := make([]byte, 48)
	if _, err := io.ReadFull(rand.Reader, rawKey); err != nil {
		return nil, "", fmt.Errorf("generate key: %w", err)
	}
	fullKey := "cf_live_" + base64.RawURLEncoding.EncodeToString(rawKey)

	// Prefix is first 10 characters for lookup.
	prefix := fullKey[:10]

	// Store SHA-256 hash.
	hash := sha256.Sum256([]byte(fullKey))
	hashHex := hex.EncodeToString(hash[:])

	if rateLimit <= 0 {
		rateLimit = 60
	}

	k := APIKey{
		OrgID:       orgID,
		Name:        name,
		KeyPrefix:   prefix,
		Permissions: permissions,
		RateLimit:   rateLimit,
		ExpiresAt:   expiresAt,
		IsActive:    true,
	}

	err := database.QuerierFromContext(ctx, s.pool).QueryRow(ctx, `
		INSERT INTO api_keys
			(organization_id, name, key_prefix, key_hash,
			 permissions, rate_limit_per_minute, expires_at, is_active, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,true,$8)
		RETURNING id, created_at`,
		orgID, name, prefix, hashHex,
		permissions, rateLimit, expiresAt, userID,
	).Scan(&k.ID, &k.CreatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("insert api key: %w", err)
	}

	log.Info().
		Str("key_id", k.ID).
		Str("prefix", prefix).
		Msg("api key created")

	return &k, fullKey, nil
}

// RevokeAPIKey deactivates an API key.
func (s *IntegrationService) RevokeAPIKey(ctx context.Context, orgID, keyID string) error {
	tag, err := database.QuerierFromContext(ctx, s.pool).Exec(ctx, `
		UPDATE api_keys
		SET is_active = false, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2`,
		keyID, orgID,
	)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	log.Info().Str("key_id", keyID).Msg("api key revoked")
	return nil
}

// ValidateAPIKey validates a raw API key string. It extracts the prefix, looks
// up the key by prefix, verifies the SHA-256 hash, and checks active/expiry
// status. Returns the APIKey record and the organisation ID.
func (s *IntegrationService) ValidateAPIKey(ctx context.Context, keyString string) (*APIKey, string, error) {
	return s.validateAPIKey(ctx, keyString, "")
}

// AuthenticateAPIKey implements auth.APIKeyAuthenticator. Authentication
// failures intentionally collapse to one public error while operational
// failures remain distinguishable for a fail-closed 503 response.
func (s *IntegrationService) AuthenticateAPIKey(ctx context.Context, keyString, clientIP string) (*authdomain.APIKeyPrincipal, error) {
	key, orgID, err := s.validateAPIKey(ctx, keyString, clientIP)
	if err != nil {
		if errors.Is(err, ErrAPIKeyNotFound) || errors.Is(err, ErrAPIKeyInvalid) || errors.Is(err, ErrAPIKeyRevoked) {
			return nil, authdomain.ErrInvalidAPIKey
		}
		return nil, err
	}
	return &authdomain.APIKeyPrincipal{
		KeyID:              key.ID,
		OrganizationID:     orgID,
		Permissions:        append([]string(nil), key.Permissions...),
		RateLimitPerMinute: key.RateLimit,
	}, nil
}

func (s *IntegrationService) validateAPIKey(ctx context.Context, keyString, clientIP string) (*APIKey, string, error) {
	if len(keyString) < 10 {
		return nil, "", ErrAPIKeyInvalid
	}

	prefix := keyString[:10]

	var k APIKey
	var keyHash string

	// This deliberately queries only the minimal non-RLS tenant index. Once the
	// organization is known, all credential reads and writes happen through a
	// tenant-scoped connection and remain protected by row-level security.
	var keyID, orgID string
	err := s.pool.QueryRow(ctx, `
		SELECT key_id, organization_id
		FROM resolve_api_key_tenant($1)`,
		prefix,
	).Scan(&keyID, &orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("resolve api key tenant: %w", err)
	}

	err = database.WithTenantConnection(ctx, s.pool, orgID, func(tenantCtx context.Context) error {
		querier := database.QuerierFromContext(tenantCtx, s.pool)
		return querier.QueryRow(tenantCtx, `
		SELECT id, organization_id, name, key_prefix, key_hash,
			   permissions, rate_limit_per_minute, expires_at,
			   last_used_at, is_active, created_at
		FROM api_keys
		WHERE id = $1 AND organization_id = $2 AND key_prefix = $3`,
			keyID, orgID, prefix,
		).Scan(
			&k.ID, &k.OrgID, &k.Name, &k.KeyPrefix, &keyHash,
			&k.Permissions, &k.RateLimit, &k.ExpiresAt,
			&k.LastUsedAt, &k.IsActive, &k.CreatedAt,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("lookup api key: %w", err)
	}

	// Verify hash.
	hash := sha256.Sum256([]byte(keyString))
	expectedHash, err := hex.DecodeString(keyHash)
	if err != nil || len(expectedHash) != sha256.Size || subtle.ConstantTimeCompare(hash[:], expectedHash) != 1 {
		return nil, "", ErrAPIKeyInvalid
	}

	// Check active.
	if !k.IsActive {
		return nil, "", ErrAPIKeyRevoked
	}

	// Check expiry.
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return nil, "", ErrAPIKeyInvalid
	}

	// Update last_used_at.
	if err := database.WithTenantConnection(ctx, s.pool, k.OrgID, func(tenantCtx context.Context) error {
		_, err := database.QuerierFromContext(tenantCtx, s.pool).Exec(tenantCtx, `
			UPDATE api_keys
			SET last_used_at = NOW(),
				last_used_ip = COALESCE(NULLIF($1, ''), last_used_ip)
			WHERE id = $2 AND organization_id = $3`,
			clientIP, k.ID, k.OrgID,
		)
		return err
	}); err != nil {
		return nil, "", fmt.Errorf("record api key usage: %w", err)
	}

	return &k, k.OrgID, nil
}

// ---------------------------------------------------------------------------
// Encryption helpers
// ---------------------------------------------------------------------------

// encryptConfig encrypts a plaintext config string using AES-256-GCM. The
// organization is authenticated as additional data, preventing a ciphertext
// from being copied to another tenant and decrypted successfully.
func (s *IntegrationService) encryptConfig(orgID, plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), integrationAdditionalData(orgID, "configuration"))
	return "v1:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptConfig decrypts a base64-encoded AES-256-GCM ciphertext.
func (s *IntegrationService) decryptConfig(orgID, encoded string) (string, error) {
	versioned := strings.HasPrefix(encoded, "v1:")
	if versioned {
		encoded = strings.TrimPrefix(encoded, "v1:")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	var additionalData []byte
	if versioned {
		additionalData = integrationAdditionalData(orgID, "configuration")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], additionalData)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

func (s *IntegrationService) encryptSecret(orgID, purpose, plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), integrationAdditionalData(orgID, purpose))
	return "v1:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *IntegrationService) decryptSecret(orgID, purpose, encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "v1:") {
		return "", errors.New("unsupported secret ciphertext version")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "v1:"))
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], integrationAdditionalData(orgID, purpose))
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

func integrationAdditionalData(orgID, purpose string) []byte {
	return []byte("complianceforge:integration-secret:v1:" + orgID + ":" + purpose)
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

// scannable is satisfied by both pgx.Row and pgx.Rows.
type scannable interface {
	Scan(dest ...interface{}) error
}

// scanIntegration scans a row into an Integration struct.
func scanIntegration(row scannable) (*Integration, error) {
	var i Integration
	err := row.Scan(
		&i.ID, &i.OrgID, &i.IntegrationType, &i.Name, &i.Description,
		&i.Status, &i.HealthStatus, &i.LastHealthCheck, &i.LastSyncAt,
		&i.SyncFreqMinutes, &i.ErrorCount, &i.LastError,
		&i.Capabilities, &i.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan integration: %w", err)
	}
	return &i, nil
}

// scanAPIKey scans a row into an APIKey struct.
func scanAPIKey(row scannable) (*APIKey, error) {
	var k APIKey
	err := row.Scan(
		&k.ID, &k.OrgID, &k.Name, &k.KeyPrefix, &k.Permissions,
		&k.RateLimit, &k.ExpiresAt, &k.LastUsedAt, &k.IsActive, &k.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}
	return &k, nil
}

// ptrToString dereferences a *string returning "" if nil.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
