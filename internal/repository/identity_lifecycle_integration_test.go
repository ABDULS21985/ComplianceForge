package repository_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/models"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/repository"
)

type failingIdentityOutbox struct{}

func (failingIdentityOutbox) Enqueue(context.Context, database.Querier, string, queuepkg.Envelope) error {
	return errors.New("forced identity outbox failure")
}

func TestIdentityLifecycleWithNonSuperuserTenants(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer adminPool.Close()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	adminA, pendingA, concurrentA, rollbackA, verificationA, userB := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	adminRole := uuid.NewString()
	roleName := "grc_identity_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{roleName}.Sanitize()
	if _, err = adminPool.Exec(ctx, `INSERT INTO organizations(id,name,slug,status,tier) VALUES
		($1,'Identity Tenant A',$3,'active','unlimited'),($2,'Identity Tenant B',$4,'active','unlimited')`,
		orgA, orgB, "identity-a-"+orgA, "identity-b-"+orgB); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO users(id,organization_id,email,password_hash,first_name,last_name,status,invitation_status,email_verified_at)
		VALUES($1,$2,$3,'password-hash','Admin','A','active','accepted',NOW()),
		($4,$2,$5,NULL,'Pending','A','pending_verification','ready',NULL),
		($6,$2,$7,NULL,'Concurrent','A','pending_verification','ready',NULL),
			($8,$2,$9,NULL,'Rollback','A','pending_verification','ready',NULL),
			($10,$2,$11,'password-hash','Verify','A','pending_verification','not_required',NULL),
			($12,$13,$14,'password-hash','User','B','active','accepted',NOW())`,
		adminA, orgA, adminA+"@example.test", pendingA, pendingA+"@example.test", concurrentA,
		concurrentA+"@example.test", rollbackA, rollbackA+"@example.test", verificationA,
		verificationA+"@example.test", userB, orgB, userB+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO roles(id,organization_id,name,slug,is_custom)
		VALUES($1::uuid,$2::uuid,'Identity test administrator','org_admin',true)`, adminRole, orgA); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,organization_id)
		VALUES($1::uuid,$2::uuid,$3::uuid)`, adminA, adminRole, orgA); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, "CREATE ROLE "+quotedRole+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	if _, err = adminPool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quotedRole+
		"; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+quotedRole+
		"; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO "+quotedRole); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		conn, acquireErr := adminPool.Acquire(cleanupCtx)
		if acquireErr == nil {
			_, _ = conn.Exec(cleanupCtx, `SET session_replication_role='replica'`)
			_, _ = conn.Exec(cleanupCtx, `DELETE FROM queue_outbox WHERE tenant_id=ANY($1::uuid[])`, []string{orgA, orgB})
			for _, table := range []string{
				"identity_step_up_grants", "identity_authentication_challenges", "identity_passkeys",
				"identity_mfa_recovery_codes", "identity_email_verification_tokens", "identity_invitations",
				"identity_security_events", "tenant_identity_policies", "password_reset_tokens", "user_sessions", "user_mfa",
			} {
				_, _ = conn.Exec(cleanupCtx, `DELETE FROM `+pgx.Identifier{table}.Sanitize()+` WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			}
			_, _ = conn.Exec(cleanupCtx, `DELETE FROM user_roles WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			_, _ = conn.Exec(cleanupCtx, `DELETE FROM roles WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			_, _ = conn.Exec(cleanupCtx, `DELETE FROM users WHERE organization_id=ANY($1::uuid[])`, []string{orgA, orgB})
			_, _ = conn.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
			_, _ = conn.Exec(cleanupCtx, `SET session_replication_role='origin'`)
			conn.Release()
		}
		_, _ = adminPool.Exec(cleanupCtx, "DROP OWNED BY "+quotedRole)
		_, _ = adminPool.Exec(cleanupCtx, "DROP ROLE "+quotedRole)
	}()

	roleConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	roleConfig.MaxConns = 8
	roleConfig.AfterConnect = func(connectCtx context.Context, connection *pgx.Conn) error {
		_, connectErr := connection.Exec(connectCtx, "SET ROLE "+quotedRole)
		return connectErr
	}
	rolePool, err := pgxpool.NewWithConfig(ctx, roleConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer rolePool.Close()
	outbox, err := queuepkg.NewPostgresOutbox(rolePool, uuid.NewString(), queuepkg.DefaultOutboxConfig())
	if err != nil {
		t.Fatal(err)
	}
	store, err := repository.NewIdentityLifecycleRepository(rolePool, outbox, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	invitationToken, invitationHash := "opaque-invitation-a", identityIntegrationHash("opaque-invitation-a:"+orgA)
	invitation, err := store.IssueInvitation(ctx, models.IdentityInvitation{ID: uuid.NewString(), OrganizationID: orgA,
		UserID: pendingA, CreatedBy: adminA, TokenHash: invitationHash, DeliveryCipher: "v1:ciphertext",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, models.IdentityRequestMetadata{RequestID: uuid.NewString()}, "Invite pending account")
	if err != nil || invitation.UserID != pendingA || !invitation.DeliveryQueued {
		t.Fatalf("IssueInvitation()=%#v err=%v token=%s", invitation, err, invitationToken)
	}
	accepted, err := store.AcceptInvitation(ctx, orgA, invitationHash, "new-password-hash", "", "", now.Add(time.Minute), models.IdentityRequestMetadata{})
	if err != nil || accepted.UserID != pendingA {
		t.Fatalf("AcceptInvitation()=%#v err=%v", accepted, err)
	}
	if _, err = store.AcceptInvitation(ctx, orgA, invitationHash, "other", "", "", now.Add(2*time.Minute), models.IdentityRequestMetadata{}); !errors.Is(err, repository.ErrIdentityConsumed) {
		t.Fatalf("invitation replay error=%v", err)
	}
	if _, err = store.AcceptInvitation(ctx, orgB, invitationHash, "other", "", "", now.Add(2*time.Minute), models.IdentityRequestMetadata{}); !errors.Is(err, repository.ErrIdentityNotFound) {
		t.Fatalf("cross-tenant invitation error=%v", err)
	}

	concurrentHash := identityIntegrationHash("concurrent-invitation:" + orgA)
	_, err = store.IssueInvitation(ctx, models.IdentityInvitation{ID: uuid.NewString(), OrganizationID: orgA,
		UserID: concurrentA, CreatedBy: adminA, TokenHash: concurrentHash, DeliveryCipher: "v1:ciphertext",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, models.IdentityRequestMetadata{}, "Concurrent invitation")
	if err != nil {
		t.Fatal(err)
	}
	var successes int
	var mutex sync.Mutex
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, acceptErr := store.AcceptInvitation(ctx, orgA, concurrentHash, "new-password-hash", "", "", now.Add(time.Minute), models.IdentityRequestMetadata{})
			if acceptErr == nil {
				mutex.Lock()
				successes++
				mutex.Unlock()
			}
		}()
	}
	wait.Wait()
	if successes != 1 {
		t.Fatalf("concurrent invitation successes=%d, want 1", successes)
	}

	resetToken := models.IdentityCredentialToken{ID: uuid.NewString(), OrganizationID: orgA, Email: pendingA + "@example.test",
		TokenHash: identityIntegrationHash("reset-token:" + orgA), DeliveryCipher: "v1:ciphertext", IssuedAt: now,
		ExpiresAt: now.Add(time.Hour), Metadata: models.IdentityRequestMetadata{IPAddress: "192.0.2.1"}}
	issued, err := store.IssuePasswordReset(ctx, resetToken)
	if err != nil || !issued {
		t.Fatalf("IssuePasswordReset() issued=%v err=%v", issued, err)
	}
	if err = store.ResetPassword(ctx, orgA, resetToken.TokenHash, "replacement-hash", now.Add(time.Minute), models.IdentityRequestMetadata{IPAddress: "192.0.2.2"}); err != nil {
		t.Fatal(err)
	}
	if err = store.ResetPassword(ctx, orgA, resetToken.TokenHash, "replay-hash", now.Add(2*time.Minute), models.IdentityRequestMetadata{}); !errors.Is(err, repository.ErrIdentityConsumed) {
		t.Fatalf("password reset replay error=%v", err)
	}

	verificationToken := models.IdentityCredentialToken{ID: uuid.NewString(), OrganizationID: orgA,
		Email: verificationA + "@example.test", TokenHash: identityIntegrationHash("verify-token:" + orgA),
		DeliveryCipher: "v1:ciphertext", IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	issued, err = store.IssueEmailVerification(ctx, verificationToken)
	if err != nil || !issued {
		t.Fatalf("IssueEmailVerification() issued=%v err=%v", issued, err)
	}
	if err = store.VerifyEmail(ctx, orgA, verificationToken.TokenHash, now.Add(time.Minute), models.IdentityRequestMetadata{}); err != nil {
		t.Fatal(err)
	}
	if err = store.VerifyEmail(ctx, orgA, verificationToken.TokenHash, now.Add(2*time.Minute), models.IdentityRequestMetadata{}); !errors.Is(err, repository.ErrIdentityConsumed) {
		t.Fatalf("email verification replay error=%v", err)
	}
	verifiedAccount, err := store.GetAccount(ctx, orgA, verificationA)
	if err != nil || !verifiedAccount.User.IsActive() || verifiedAccount.EmailVerifiedAt == nil {
		t.Fatalf("verified account=%#v err=%v", verifiedAccount, err)
	}

	userStore := repository.NewUserRepository(rolePool)
	session := &models.UserSession{ID: uuid.NewString(), OrganizationID: orgA, UserID: pendingA,
		TokenHash: identityIntegrationHash("access:" + orgA), RefreshTokenHash: identityIntegrationHash("refresh:" + orgA),
		IPAddress: "192.0.2.40", UserAgent: "identity-integration", DeviceName: "Work laptop",
		AuthenticationMethod: models.IdentityMethodTOTP, ExpiresAt: now.Add(time.Hour)}
	if err = userStore.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession() error=%v", err)
	}
	sessions, err := store.ListSessions(ctx, orgA, pendingA, session.TokenHash)
	if err != nil || len(sessions) != 1 || !sessions[0].Current || sessions[0].DeviceName != session.DeviceName ||
		sessions[0].AuthenticationMethod != models.IdentityMethodTOTP {
		t.Fatalf("ListSessions()=%#v err=%v", sessions, err)
	}
	if err = store.RevokeSession(ctx, orgB, pendingA, session.ID, userB, 1, "Cross tenant revocation", models.IdentityRequestMetadata{}); !errors.Is(err, repository.ErrIdentityNotFound) {
		t.Fatalf("cross-tenant session revoke error=%v", err)
	}

	challengeHash := identityIntegrationHash("login-challenge:" + orgA)
	challenge := &models.IdentityAuthenticationChallenge{OrganizationID: orgA, UserID: pendingA,
		TokenHash: challengeHash, Purpose: "login", AllowedMethods: []string{"totp"},
		ExpiresAt: now.Add(5 * time.Minute), MaxAttempts: 5}
	if err = store.CreateChallenge(ctx, challenge); err != nil {
		t.Fatalf("CreateChallenge() error=%v", err)
	}
	successes = 0
	wait = sync.WaitGroup{}
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if consumeErr := store.ConsumeChallenge(ctx, orgA, challengeHash, "login", now.Add(time.Minute)); consumeErr == nil {
				mutex.Lock()
				successes++
				mutex.Unlock()
			}
		}()
	}
	wait.Wait()
	if successes != 1 {
		t.Fatalf("concurrent challenge consumptions=%d, want 1", successes)
	}

	grantHash := identityIntegrationHash("step-up:" + orgA)
	grant := &models.IdentityStepUpGrant{OrganizationID: orgA, UserID: pendingA, SessionID: session.ID,
		Purpose: "mfa_change", AuthenticationMethod: models.IdentityMethodTOTP, ExpiresAt: now.Add(10 * time.Minute)}
	if err = store.CreateStepUpGrant(ctx, grant, grantHash); err != nil {
		t.Fatalf("CreateStepUpGrant() error=%v", err)
	}
	if err = store.ConsumeStepUpGrant(ctx, orgB, pendingA, session.ID, "mfa_change", grantHash, now.Add(time.Minute)); !errors.Is(err, repository.ErrIdentityNotFound) {
		t.Fatalf("cross-tenant step-up consumption error=%v", err)
	}
	if err = store.ConsumeStepUpGrant(ctx, orgA, pendingA, session.ID, "mfa_change", grantHash, now.Add(time.Minute)); err != nil {
		t.Fatalf("ConsumeStepUpGrant() error=%v", err)
	}
	if err = store.ConsumeStepUpGrant(ctx, orgA, pendingA, session.ID, "mfa_change", grantHash, now.Add(2*time.Minute)); !errors.Is(err, repository.ErrIdentityNotFound) {
		t.Fatalf("step-up replay error=%v", err)
	}

	policy, err := store.UpdatePolicy(ctx, orgA, adminA, models.IdentityPolicyPatch{ExpectedVersion: 0,
		RequireMFA: boolIdentityPtr(true), AllowedMethods: []string{"totp", "passkey"}, Reason: "Require strong authentication"})
	if err != nil || policy.Version != 1 || !policy.RequireMFA {
		t.Fatalf("UpdatePolicy()=%#v err=%v", policy, err)
	}
	successes = 0
	wait = sync.WaitGroup{}
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, updateErr := store.UpdatePolicy(ctx, orgA, adminA, models.IdentityPolicyPatch{ExpectedVersion: 1,
				StepUpTTLMinutes: intIdentityPtr(12), Reason: "Concurrent policy update"})
			if updateErr == nil {
				mutex.Lock()
				successes++
				mutex.Unlock()
			}
		}()
	}
	wait.Wait()
	if successes != 1 {
		t.Fatalf("concurrent policy successes=%d, want 1", successes)
	}

	factor, err := store.CreateTOTPFactor(ctx, orgA, pendingA, pendingA, "Authenticator", []byte("v1:cipher"),
		"Enroll authenticator", models.IdentityRequestMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	factor, err = store.VerifyTOTPFactor(ctx, orgA, pendingA, factor.ID, factor.Version, now.Unix()/30,
		[]string{"bcrypt-hash-one", "bcrypt-hash-two"}, "Verify authenticator", models.IdentityRequestMetadata{})
	if err != nil || !factor.Verified || factor.RecoveryCodes != 2 {
		t.Fatalf("VerifyTOTPFactor()=%#v err=%v", factor, err)
	}
	if err = store.UseTOTPFactor(ctx, orgA, pendingA, factor.ID, now.Unix()/30+1, now); err != nil {
		t.Fatal(err)
	}
	if err = store.UseTOTPFactor(ctx, orgA, pendingA, factor.ID, now.Unix()/30+1, now); !errors.Is(err, repository.ErrIdentityConsumed) {
		t.Fatalf("TOTP replay error=%v", err)
	}
	if err = store.DisableMFAFactor(ctx, orgA, pendingA, factor.ID, pendingA, factor.Version,
		"Attempt last-factor removal", models.IdentityRequestMetadata{}); !errors.Is(err, repository.ErrIdentityLastFactor) {
		t.Fatalf("last-factor safeguard error=%v", err)
	}
	adminFactor, err := store.CreateTOTPFactor(ctx, orgA, adminA, adminA, "Admin authenticator", []byte("v1:admin-cipher"),
		"Enroll recovery administrator", models.IdentityRequestMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	adminFactor, err = store.VerifyTOTPFactor(ctx, orgA, adminA, adminFactor.ID, adminFactor.Version, now.Unix()/30,
		[]string{"admin-recovery-hash"}, "Verify recovery administrator", models.IdentityRequestMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	exemptUntil := now.Add(2 * time.Hour)
	if err = store.AdminResetMFA(ctx, orgA, pendingA, adminA, "Approved account recovery", exemptUntil,
		models.IdentityRequestMetadata{}); err != nil {
		t.Fatalf("AdminResetMFA() error=%v adminFactor=%#v", err, adminFactor)
	}
	recovered, err := store.GetAccount(ctx, orgA, pendingA)
	if err != nil || recovered.MFAExemptUntil == nil || !recovered.MFAExemptUntil.Equal(exemptUntil) {
		t.Fatalf("recovered account=%#v err=%v", recovered, err)
	}

	if _, err = store.GetAccount(ctx, orgB, pendingA); !errors.Is(err, repository.ErrIdentityNotFound) {
		t.Fatalf("cross-tenant account error=%v", err)
	}
	var visible int
	err = database.WithTenantConnection(ctx, rolePool, orgA, func(scoped context.Context) error {
		return database.QuerierFromContext(scoped, rolePool).QueryRow(scoped,
			`SELECT COUNT(*) FROM identity_security_events WHERE organization_id=$1::uuid`, orgB).Scan(&visible)
	})
	if err != nil || visible != 0 {
		t.Fatalf("cross-tenant RLS visible=%d err=%v", visible, err)
	}

	failingStore, err := repository.NewIdentityLifecycleRepository(rolePool, failingIdentityOutbox{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	rollbackID := uuid.NewString()
	_, err = failingStore.IssueInvitation(ctx, models.IdentityInvitation{ID: rollbackID, OrganizationID: orgA,
		UserID: rollbackA, CreatedBy: adminA, TokenHash: identityIntegrationHash("rollback-token:" + orgA),
		DeliveryCipher: "v1:ciphertext", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		models.IdentityRequestMetadata{}, "Prove outbox rollback")
	if err == nil || !strings.Contains(err.Error(), "forced identity outbox failure") {
		t.Fatalf("forced outbox error=%v", err)
	}
	var rolledBack int
	if err = database.WithTenantConnection(ctx, rolePool, orgA, func(scoped context.Context) error {
		return database.QuerierFromContext(scoped, rolePool).QueryRow(scoped,
			`SELECT COUNT(*) FROM identity_invitations WHERE id=$1::uuid`, rollbackID).Scan(&rolledBack)
	}); err != nil || rolledBack != 0 {
		t.Fatalf("rolled-back invitations=%d err=%v", rolledBack, err)
	}

	// The DB's legacy BIGINT counter has a wider domain than WebAuthn's
	// unsigned 32-bit wire value. Preserve valid bounds and fail closed on
	// malformed stored values before loading or updating a credential.
	registrationHash := identityIntegrationHash("counter-registration:" + orgA)
	if err = store.CreateChallenge(ctx, &models.IdentityAuthenticationChallenge{
		OrganizationID: orgA, UserID: pendingA, TokenHash: registrationHash,
		Purpose: "passkey_registration", AllowedMethods: []string{"passkey"},
		WebAuthnSession: json.RawMessage(`{}`), ExpiresAt: now.Add(5 * time.Minute), MaxAttempts: 5,
	}); err != nil {
		t.Fatal(err)
	}
	credentialID := []byte("counter-fixture:" + orgA)
	passkey, err := store.CreatePasskey(ctx, orgA, pendingA, pendingA, &models.IdentityPasskey{
		CredentialID: credentialID, CredentialCipher: "v1:counter-fixture", AttestationType: "none",
		Transports: []string{"internal"}, DeviceName: "Counter boundary fixture", SignCount: math.MaxUint32,
	}, registrationHash, now, "Verify signature counter boundary", models.IdentityRequestMetadata{})
	if err != nil || passkey == nil || passkey.SignCount != math.MaxUint32 {
		t.Fatalf("maximum WebAuthn counter was not persisted safely: err=%v", err)
	}
	if _, err = store.GetPasskeyByCredentialID(ctx, orgB, pendingA, credentialID); !errors.Is(err, repository.ErrIdentityNotFound) {
		t.Fatalf("cross-tenant counter credential read: %v", err)
	}
	badCounter := int64(math.MaxUint32) + 1
	if _, err = adminPool.Exec(ctx, `UPDATE identity_passkeys SET sign_count=$2 WHERE id=$1::uuid`, passkey.ID, badCounter); err != nil {
		t.Fatal(err)
	}
	if malformed, readErr := store.GetPasskeyByCredentialID(ctx, orgA, pendingA, credentialID); readErr == nil || malformed != nil {
		t.Fatal("out-of-range stored counter yielded a usable credential")
	}
	authenticationHash := identityIntegrationHash("counter-authentication:" + orgA)
	if err = store.CreateChallenge(ctx, &models.IdentityAuthenticationChallenge{
		OrganizationID: orgA, UserID: pendingA, TokenHash: authenticationHash,
		Purpose: "passkey_authentication", AllowedMethods: []string{"passkey"},
		WebAuthnSession: json.RawMessage(`{}`), ExpiresAt: now.Add(5 * time.Minute), MaxAttempts: 5,
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.UpdatePasskeyAuthentication(ctx, orgA, pendingA, passkey.ID, "v1:unexpected-mutation",
		0, true, false, authenticationHash, "passkey_authentication", now, models.IdentityRequestMetadata{}); !errors.Is(err, repository.ErrIdentityConflict) {
		t.Fatalf("out-of-range stored counter accepted an authentication update: %v", err)
	}
	var storedCounter int64
	var storedCipher string
	if err = adminPool.QueryRow(ctx, `SELECT sign_count,credential_ciphertext FROM identity_passkeys WHERE id=$1::uuid`,
		passkey.ID).Scan(&storedCounter, &storedCipher); err != nil || storedCounter != badCounter || storedCipher != "v1:counter-fixture" {
		t.Fatal("rejected counter authentication changed credential state")
	}
	if _, err = adminPool.Exec(ctx, `UPDATE identity_passkeys SET sign_count=0 WHERE id=$1::uuid`, passkey.ID); err != nil {
		t.Fatal(err)
	}
	// A failed authentication must not consume its challenge: the enclosing
	// transaction rolls back before the corrected credential can use it once.
	if err = store.UpdatePasskeyAuthentication(ctx, orgA, pendingA, passkey.ID, "v1:valid-counter-update",
		1, false, false, authenticationHash, "passkey_authentication", now, models.IdentityRequestMetadata{}); err != nil {
		t.Fatalf("valid counter update/challenge rollback: %v", err)
	}
	if updated, readErr := store.GetPasskeyByCredentialID(ctx, orgA, pendingA, credentialID); readErr != nil || updated.SignCount != 1 {
		t.Fatal("valid WebAuthn counter update was not retained")
	}

	var eventID string
	if err = database.WithTenantConnection(ctx, rolePool, orgA, func(scoped context.Context) error {
		return database.QuerierFromContext(scoped, rolePool).QueryRow(scoped,
			`SELECT id FROM identity_security_events ORDER BY created_at LIMIT 1`).Scan(&eventID)
	}); err != nil {
		t.Fatal(err)
	}
	err = database.WithTenantConnection(ctx, rolePool, orgA, func(scoped context.Context) error {
		_, updateErr := database.QuerierFromContext(scoped, rolePool).Exec(scoped,
			`UPDATE identity_security_events SET reason='tampered' WHERE id=$1::uuid`, eventID)
		return updateErr
	})
	if err == nil {
		t.Fatal("immutable identity event accepted an update")
	}
}

func identityIntegrationHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func boolIdentityPtr(value bool) *bool { return &value }
func intIdentityPtr(value int) *int    { return &value }
