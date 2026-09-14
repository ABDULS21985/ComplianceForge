package service

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
)

// TestIntegrationServiceAgainstMigratedPostgres exercises the schema contract,
// RLS-scoped queries, encrypted configuration, API-key tenant resolution, and
// sync lifecycle. It is opt-in for local runs and enabled by CI after migration.
func TestIntegrationServiceAgainstMigratedPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	orgID := uuid.NewString()
	userID := uuid.NewString()
	suffix := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, status, tier)
		VALUES ($1, $2, $3, 'active', 'starter')`,
		orgID, "Integration Service Test "+suffix, "integration-service-"+suffix,
	); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
	}()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, organization_id, email, first_name, last_name, status)
		VALUES ($1, $2, $3, 'Integration', 'Test', 'active')`,
		userID, orgID, "integration-"+suffix+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	keyHex := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	svc, err := NewIntegrationService(pool, keyHex)
	if err != nil {
		t.Fatalf("NewIntegrationService() error = %v", err)
	}

	enabled := true
	issuerURL := "https://idp.example.com"
	clientID := "complianceforge-test"
	clientSecret := "oidc-client-secret"
	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		return svc.UpdateSSOConfig(tenantCtx, orgID, UpdateSSOConfigurationInput{
			Protocol:           "oidc",
			Enabled:            &enabled,
			OIDCIssuerURL:      &issuerURL,
			OIDCClientID:       &clientID,
			OIDCClientSecret:   &clientSecret,
			OIDCScopes:         []string{"openid", "profile", "email"},
			AllowedDomains:     []string{"Example.COM", "example.com"},
			JITProvisioning:    &enabled,
			AutoProvisionUsers: &enabled,
		})
	})
	if err != nil {
		t.Fatalf("UpdateSSOConfig() error = %v", err)
	}
	var ssoConfig *SSOConfiguration
	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		var getErr error
		ssoConfig, getErr = svc.GetSSOConfig(tenantCtx, orgID)
		return getErr
	})
	if err != nil {
		t.Fatalf("GetSSOConfig() error = %v", err)
	}
	if !ssoConfig.OIDCClientSecretConfigured || len(ssoConfig.AllowedDomains) != 1 || ssoConfig.AllowedDomains[0] != "example.com" {
		t.Fatalf("unexpected SSO configuration: %#v", ssoConfig)
	}
	var storedOIDCSecret string
	if err := pool.QueryRow(ctx, `SELECT oidc_client_secret_encrypted FROM sso_configurations WHERE organization_id = $1`, orgID).Scan(&storedOIDCSecret); err != nil {
		t.Fatalf("read stored OIDC secret: %v", err)
	}
	if storedOIDCSecret == clientSecret || len(storedOIDCSecret) < 4 || storedOIDCSecret[:3] != "v1:" {
		t.Fatalf("OIDC secret was not stored as a versioned ciphertext: %q", storedOIDCSecret)
	}

	var created *Integration
	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		var createErr error
		created, createErr = svc.CreateIntegration(tenantCtx, orgID, userID, Integration{
			IntegrationType: "custom_api",
			Name:            "Contract Test",
			SyncFreqMinutes: 15,
			Capabilities:    []string{"read_assets", "write_tickets"},
		}, `{"token":"never-store-plaintext"}`)
		return createErr
	})
	if err != nil {
		t.Fatalf("CreateIntegration() error = %v", err)
	}
	if created.Status != "pending_setup" {
		t.Fatalf("created status = %q, want pending_setup", created.Status)
	}

	var storedConfig string
	if err := pool.QueryRow(ctx, `SELECT configuration_encrypted FROM integrations WHERE id = $1`, created.ID).Scan(&storedConfig); err != nil {
		t.Fatalf("read stored configuration: %v", err)
	}
	if storedConfig == `{"token":"never-store-plaintext"}` || len(storedConfig) < 4 || storedConfig[:3] != "v1:" {
		t.Fatalf("configuration was not stored as a versioned ciphertext: %q", storedConfig)
	}

	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		_, triggerErr := svc.TriggerSync(tenantCtx, orgID, created.ID, "full")
		if !errors.Is(triggerErr, ErrIntegrationInactive) {
			return errors.New("inactive integration was allowed to sync")
		}
		_, updateErr := database.QuerierFromContext(tenantCtx, pool).Exec(tenantCtx, `
			UPDATE integrations SET status = 'active' WHERE id = $1 AND organization_id = $2`,
			created.ID, orgID,
		)
		return updateErr
	})
	if err != nil {
		t.Fatalf("activate integration: %v", err)
	}

	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		syncLog, syncErr := svc.TriggerSync(tenantCtx, orgID, created.ID, "full")
		if syncErr != nil {
			return syncErr
		}
		if syncLog.Status != "started" {
			return errors.New("sync did not use the canonical started status")
		}
		logs, total, logsErr := svc.GetSyncLogs(tenantCtx, orgID, created.ID, 1, 20)
		if logsErr != nil {
			return logsErr
		}
		if total != 1 || len(logs) != 1 {
			return errors.New("created sync log was not returned")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("integration sync contract: %v", err)
	}

	var rawAPIKey string
	var apiKey *APIKey
	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		var createErr error
		apiKey, rawAPIKey, createErr = svc.CreateAPIKey(
			tenantCtx, orgID, userID, "Automation", []string{"read:controls"}, 60, nil,
		)
		return createErr
	})
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}
	validated, validatedOrgID, err := svc.ValidateAPIKey(ctx, rawAPIKey)
	if err != nil || validated.ID != apiKey.ID || validatedOrgID != orgID {
		t.Fatalf("ValidateAPIKey() key = %#v, org = %q, error = %v", validated, validatedOrgID, err)
	}

	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		return svc.RevokeAPIKey(tenantCtx, orgID, apiKey.ID)
	})
	if err != nil {
		t.Fatalf("RevokeAPIKey() error = %v", err)
	}
	if _, _, err := svc.ValidateAPIKey(ctx, rawAPIKey); !errors.Is(err, ErrAPIKeyRevoked) {
		t.Fatalf("ValidateAPIKey() after revoke error = %v, want ErrAPIKeyRevoked", err)
	}

	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		return svc.DeleteIntegration(tenantCtx, orgID, created.ID)
	})
	if err != nil {
		t.Fatalf("DeleteIntegration() error = %v", err)
	}
	err = database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
		_, getErr := svc.GetIntegration(tenantCtx, orgID, created.ID)
		if !errors.Is(getErr, ErrIntegrationNotFound) {
			return errors.New("soft-deleted integration remains visible")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("soft-delete visibility: %v", err)
	}
}
