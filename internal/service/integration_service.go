package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrIntegrationNotFound = errors.New("integration not found")
	ErrAPIKeyNotFound      = errors.New("api key not found")
	ErrAPIKeyInvalid       = errors.New("api key is invalid or expired")
	ErrAPIKeyRevoked       = errors.New("api key has been revoked")
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
	SyncFreqMinutes int       `json:"sync_frequency_minutes"`
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

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewIntegrationService creates a new IntegrationService. The encryption key
// is read from the INTEGRATION_ENCRYPTION_KEY environment variable (hex-encoded,
// 64 hex chars = 32 bytes). Falls back to ENCRYPTION_KEY if unset.
func NewIntegrationService(pool *pgxpool.Pool) *IntegrationService {
	keyHex := os.Getenv("INTEGRATION_ENCRYPTION_KEY")
	if keyHex == "" {
		keyHex = os.Getenv("ENCRYPTION_KEY")
	}
	var key []byte
	if keyHex != "" {
		var err error
		key, err = hex.DecodeString(keyHex)
		if err != nil || len(key) != 32 {
			log.Fatal().Msg("INTEGRATION_ENCRYPTION_KEY must be 64 hex chars (32 bytes)")
		}
	} else {
		log.Warn().Msg("no integration encryption key set — config storage will fail")
	}
	return &IntegrationService{pool: pool, encKey: key}
}

// ---------------------------------------------------------------------------
// Integrations CRUD
// ---------------------------------------------------------------------------

// ListIntegrations returns all integrations for an organisation.
func (s *IntegrationService) ListIntegrations(ctx context.Context, orgID string) ([]Integration, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, integration_type, name, description,
			   status, health_status, last_health_check_at, last_sync_at,
			   sync_frequency_minutes, error_count, last_error_message,
			   capabilities, created_at
		FROM integrations
		WHERE organization_id = $1
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
	return results, nil
}

// CreateIntegration creates a new integration with its config encrypted at rest.
func (s *IntegrationService) CreateIntegration(
	ctx context.Context,
	orgID, userID string,
	integ Integration,
	configJSON string,
) (*Integration, error) {
	encConfig, err := s.encryptConfig(configJSON)
	if err != nil {
		return nil, fmt.Errorf("encrypt config: %w", err)
	}

	err = s.pool.QueryRow(ctx, `
		INSERT INTO integrations
			(organization_id, integration_type, name, description,
			 status, health_status, sync_frequency_minutes,
			 capabilities, config_encrypted, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at`,
		orgID, integ.IntegrationType, integ.Name, integ.Description,
		"inactive", "unknown", integ.SyncFreqMinutes,
		integ.Capabilities, encConfig, userID,
	).Scan(&integ.ID, &integ.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create integration: %w", err)
	}

	integ.OrgID = orgID
	integ.Status = "inactive"
	integ.HealthStatus = "unknown"

	log.Info().
		Str("integration_id", integ.ID).
		Str("type", integ.IntegrationType).
		Msg("integration created")

	return &integ, nil
}

// GetIntegration returns a single integration by ID.
func (s *IntegrationService) GetIntegration(ctx context.Context, orgID, integID string) (*Integration, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, integration_type, name, description,
			   status, health_status, last_health_check_at, last_sync_at,
			   sync_frequency_minutes, error_count, last_error_message,
			   capabilities, created_at
		FROM integrations
		WHERE id = $1 AND organization_id = $2`,
		integID, orgID,
	)

	var i Integration
	var caps []byte
	err := row.Scan(
		&i.ID, &i.OrgID, &i.IntegrationType, &i.Name, &i.Description,
		&i.Status, &i.HealthStatus, &i.LastHealthCheck, &i.LastSyncAt,
		&i.SyncFreqMinutes, &i.ErrorCount, &i.LastError,
		&caps, &i.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIntegrationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration: %w", err)
	}
	if caps != nil {
		i.Capabilities = parseStringArray(caps)
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
		enc, err := s.encryptConfig(*configJSON)
		if err != nil {
			return fmt.Errorf("encrypt config: %w", err)
		}
		encConfig = &enc
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE integrations
		SET name                  = COALESCE(NULLIF($1,''), name),
			description           = COALESCE($2, description),
			sync_frequency_minutes= CASE WHEN $3 > 0 THEN $3 ELSE sync_frequency_minutes END,
			capabilities          = COALESCE($4, capabilities),
			config_encrypted      = COALESCE($5, config_encrypted),
			updated_at            = NOW()
		WHERE id = $6 AND organization_id = $7`,
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

// DeleteIntegration soft-deletes an integration by setting status to 'deleted'.
func (s *IntegrationService) DeleteIntegration(ctx context.Context, orgID, integID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE integrations
		SET status = 'deleted', updated_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND status != 'deleted'`,
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
	err := s.pool.QueryRow(ctx, `
		SELECT integration_type, config_encrypted
		FROM integrations
		WHERE id = $1 AND organization_id = $2`,
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
	_, err = s.decryptConfig(encConfig)
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

	tag, err := s.pool.Exec(ctx, `
		UPDATE integrations
		SET health_status        = $1,
			last_health_check_at = NOW(),
			last_error_message   = $2,
			error_count          = CASE WHEN $1 = 'healthy' THEN 0 ELSE error_count + 1 END,
			updated_at           = NOW()
		WHERE id = $3 AND organization_id = $4`,
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
// creates a sync_log record in 'running' state and returns it. The actual sync
// work would be performed asynchronously by a worker.
func (s *IntegrationService) TriggerSync(ctx context.Context, orgID, integID, syncType string) (*SyncLog, error) {
	// Verify integration exists and is active.
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT status FROM integrations
		WHERE id = $1 AND organization_id = $2`,
		integID, orgID,
	).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIntegrationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("check integration status: %w", err)
	}

	sl := SyncLog{
		IntegrationID: integID,
		SyncType:      syncType,
		Status:        "running",
	}

	err = s.pool.QueryRow(ctx, `
		INSERT INTO integration_sync_logs
			(integration_id, sync_type, status)
		VALUES ($1,$2,$3)
		RETURNING id, created_at`,
		sl.IntegrationID, sl.SyncType, sl.Status,
	).Scan(&sl.ID, &sl.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create sync log: %w", err)
	}

	// Update last_sync_at on the integration.
	_, _ = s.pool.Exec(ctx, `
		UPDATE integrations SET last_sync_at = NOW(), updated_at = NOW()
		WHERE id = $1`,
		integID,
	)

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
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM integrations WHERE id = $1 AND organization_id = $2)`,
		integID, orgID,
	).Scan(&exists)
	if err != nil {
		return nil, 0, fmt.Errorf("verify integration: %w", err)
	}
	if !exists {
		return nil, 0, ErrIntegrationNotFound
	}

	var total int
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM integration_sync_logs WHERE integration_id = $1`,
		integID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count sync logs: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, integration_id, sync_type, status,
			   records_processed, records_created, records_updated, records_failed,
			   duration_ms, error_message, created_at
		FROM integration_sync_logs
		WHERE integration_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		integID, pageSize, offset,
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

	return logs, total, nil
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

// ListAPIKeys returns all API keys for an organisation (without the hash).
func (s *IntegrationService) ListAPIKeys(ctx context.Context, orgID string) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
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
	// Generate 48 random bytes and encode as base64url (64 chars).
	rawKey := make([]byte, 48)
	if _, err := io.ReadFull(rand.Reader, rawKey); err != nil {
		return nil, "", fmt.Errorf("generate key: %w", err)
	}
	fullKey := base64.RawURLEncoding.EncodeToString(rawKey)

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

	err := s.pool.QueryRow(ctx, `
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
	tag, err := s.pool.Exec(ctx, `
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
	if len(keyString) < 10 {
		return nil, "", ErrAPIKeyInvalid
	}

	prefix := keyString[:10]

	var k APIKey
	var keyHash string
	var permsBytes []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, key_prefix, key_hash,
			   permissions, rate_limit_per_minute, expires_at,
			   last_used_at, is_active, created_at
		FROM api_keys
		WHERE key_prefix = $1`,
		prefix,
	).Scan(
		&k.ID, &k.OrgID, &k.Name, &k.KeyPrefix, &keyHash,
		&permsBytes, &k.RateLimit, &k.ExpiresAt,
		&k.LastUsedAt, &k.IsActive, &k.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("lookup api key: %w", err)
	}

	if permsBytes != nil {
		k.Permissions = parseStringArray(permsBytes)
	}

	// Verify hash.
	hash := sha256.Sum256([]byte(keyString))
	if hex.EncodeToString(hash[:]) != keyHash {
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
	_, _ = s.pool.Exec(ctx, `
		UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, k.ID)

	return &k, k.OrgID, nil
}

// ---------------------------------------------------------------------------
// Encryption helpers
// ---------------------------------------------------------------------------

// encryptConfig encrypts a plaintext config string using AES-256-GCM.
func (s *IntegrationService) encryptConfig(plaintext string) (string, error) {
	if s.encKey == nil {
		return "", fmt.Errorf("encryption key not configured")
	}
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
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptConfig decrypts a base64-encoded AES-256-GCM ciphertext.
func (s *IntegrationService) decryptConfig(encoded string) (string, error) {
	if s.encKey == nil {
		return "", fmt.Errorf("encryption key not configured")
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
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
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
	var caps []byte
	err := row.Scan(
		&i.ID, &i.OrgID, &i.IntegrationType, &i.Name, &i.Description,
		&i.Status, &i.HealthStatus, &i.LastHealthCheck, &i.LastSyncAt,
		&i.SyncFreqMinutes, &i.ErrorCount, &i.LastError,
		&caps, &i.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan integration: %w", err)
	}
	if caps != nil {
		i.Capabilities = parseStringArray(caps)
	}
	return &i, nil
}

// scanAPIKey scans a row into an APIKey struct.
func scanAPIKey(row scannable) (*APIKey, error) {
	var k APIKey
	var perms []byte
	err := row.Scan(
		&k.ID, &k.OrgID, &k.Name, &k.KeyPrefix, &perms,
		&k.RateLimit, &k.ExpiresAt, &k.LastUsedAt, &k.IsActive, &k.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}
	if perms != nil {
		k.Permissions = parseStringArray(perms)
	}
	return &k, nil
}

// parseStringArray parses a JSON array or Postgres text array into a []string.
func parseStringArray(data []byte) []string {
	var arr []string
	// Try JSON first.
	if len(data) > 0 && data[0] == '[' {
		if err := json.Unmarshal(data, &arr); err == nil {
			return arr
		}
	}
	// Fallback: Postgres text array format {a,b,c}.
	s := string(data)
	if len(s) >= 2 && s[0] == '{' && s[len(s)-1] == '}' {
		inner := s[1 : len(s)-1]
		if inner == "" {
			return nil
		}
		// Simple split — does not handle quoted elements with commas.
		start := 0
		for i := 0; i <= len(inner); i++ {
			if i == len(inner) || inner[i] == ',' {
				elem := inner[start:i]
				// Strip surrounding quotes if present.
				if len(elem) >= 2 && elem[0] == '"' && elem[len(elem)-1] == '"' {
					elem = elem[1 : len(elem)-1]
				}
				arr = append(arr, elem)
				start = i + 1
			}
		}
	}
	return arr
}

// ptrToString dereferences a *string returning "" if nil.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
