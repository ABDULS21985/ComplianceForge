package router

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/models"
	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/service"
)

const (
	testUserID      = "10000000-0000-0000-0000-000000000010"
	testOrgID       = "20000000-0000-0000-0000-000000000010"
	testFrameworkID = "30000000-0000-0000-0000-000000000010"
	testControlID   = "40000000-0000-0000-0000-000000000010"
	testPolicyID    = "90000000-0000-0000-0000-000000000010"
	testAuditID     = "a0000000-0000-0000-0000-000000000010"
	testIncidentID  = "b0000000-0000-0000-0000-000000000010"
	testAssetID     = "c1000000-0000-0000-0000-000000000010"
	testVendorID    = "d1000000-0000-0000-0000-000000000010"
	testRoleID      = "e1000000-0000-0000-0000-000000000010"
	testGroupID     = "f1000000-0000-0000-0000-000000000010"
)

type routerAuthService struct {
	user *models.User
}

// routerLegacyAuthService intentionally exposes only the base authentication
// contract so dependency validation can prove identity completion fails closed.
type routerLegacyAuthService struct{ handler.AuthService }

type routerEmailSender struct{}

func (routerEmailSender) Send(context.Context, emailpkg.Message) error { return nil }

type routerAPIKeyLimiter struct{}

func (routerAPIKeyLimiter) Allow(context.Context, string, int) (bool, time.Duration, error) {
	return true, time.Minute, nil
}

type routerAPIKeyAuthenticator struct {
	permissions []string
}

type routerSCIMAuthenticator struct{ scopes []string }

func (a routerSCIMAuthenticator) AuthenticateSCIMToken(_ context.Context, token, _ string) (*authdomain.SCIMPrincipal, error) {
	if !strings.HasPrefix(token, "cfs_") {
		return nil, authdomain.ErrInvalidSCIMToken
	}
	return &authdomain.SCIMPrincipal{TokenID: "30000000-0000-0000-0000-000000000099",
		OrganizationID: testOrgID, CreatedByUserID: testUserID,
		Scopes: append([]string(nil), a.scopes...), RateLimitPerMinute: 120}, nil
}

type routerSCIMService struct{ handler.SCIMHandlerService }

func (routerSCIMService) ListTokens(context.Context, string, models.PaginationRequest) ([]models.SCIMToken, int, error) {
	return []models.SCIMToken{}, 0, nil
}

func (routerSCIMService) ListUsers(context.Context, string, models.SCIMListRequest) ([]models.SCIMUser, int, error) {
	return []models.SCIMUser{}, 0, nil
}

func (routerSCIMService) GetUser(context.Context, string, string) (*models.SCIMUser, error) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	return &models.SCIMUser{Schemas: []string{models.SCIMUserSchema}, ID: testUserID,
		UserName: "user@example.com", Active: true, Groups: []models.SCIMGroupReference{}, Version: 1,
		Meta: models.SCIMMeta{ResourceType: "User", Created: now, LastModified: now,
			Location: "/api/scim/v2/Users/" + testUserID, Version: `W/"1"`}}, nil
}

type routerEvidenceObjectService struct{}

func (routerEvidenceObjectService) Ready() bool { return true }
func (routerEvidenceObjectService) Upload(context.Context, string, string, string, string, string, io.Reader, service.EvidenceUploadMetadata) (*models.ControlEvidence, error) {
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: "70000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID}, nil
}
func (routerEvidenceObjectService) Supersede(context.Context, string, string, string, string, string, string, io.Reader, service.EvidenceUploadMetadata) (*models.ControlEvidence, error) {
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: "70000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID}, nil
}
func (routerEvidenceObjectService) Download(context.Context, string, string, string) (*service.EvidenceDownload, error) {
	return nil, service.ErrEvidenceObjectNotFound
}
func (routerEvidenceObjectService) Review(context.Context, string, string, string, string, models.ReviewControlEvidenceInput) (*models.ControlEvidence, error) {
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: "70000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID}, nil
}

type routerEvidenceLifecycleService struct{}

func (routerEvidenceLifecycleService) Ready() bool { return true }
func (routerEvidenceLifecycleService) History(context.Context, string, string, string) (*models.EvidenceLifecycleRecord, error) {
	return &models.EvidenceLifecycleRecord{}, nil
}
func (routerEvidenceLifecycleService) VerifyIntegrity(context.Context, string, string, string, string, string) (*models.EvidenceIntegrityResult, error) {
	return &models.EvidenceIntegrityResult{}, nil
}
func (routerEvidenceLifecycleService) RecordDownloadAuthorization(context.Context, string, string, string, string, string, string) error {
	return nil
}

func (a routerAPIKeyAuthenticator) AuthenticateAPIKey(context.Context, string, string) (*authdomain.APIKeyPrincipal, error) {
	return &authdomain.APIKeyPrincipal{
		KeyID: "api-key-1", OrganizationID: testOrgID,
		Permissions: a.permissions, RateLimitPerMinute: 60,
	}, nil
}

func (s *routerAuthService) Login(context.Context, authdomain.LoginRequest) (*authdomain.TokenPair, error) {
	return routerTokenPair(s.user), nil
}

func (s *routerAuthService) Register(context.Context, authdomain.RegisterRequest) (*authdomain.TokenPair, error) {
	return routerTokenPair(s.user), nil
}

func (s *routerAuthService) RefreshToken(context.Context, string) (*authdomain.TokenPair, error) {
	return routerTokenPair(s.user), nil
}

func (s *routerAuthService) CurrentUser(context.Context, string, string) (*models.User, error) {
	return s.user, nil
}

func (s *routerAuthService) Logout(context.Context, string, string, string) error {
	return nil
}

func (s *routerAuthService) CompleteLoginMFA(context.Context, models.IdentityMFAProofInput, models.IdentityRequestMetadata) (*authdomain.TokenPair, error) {
	return routerTokenPair(s.user), nil
}

func (s *routerAuthService) CompletePasskeyAuthentication(context.Context, models.IdentityPasskeyAuthenticationFinishInput, models.IdentityRequestMetadata) (*authdomain.TokenPair, error) {
	return routerTokenPair(s.user), nil
}

type routerIdentityService struct {
	handler.IdentityLifecycleService
}

func (routerIdentityService) GetPolicy(context.Context, string) (*models.IdentityPolicy, error) {
	now := time.Now().UTC()
	return &models.IdentityPolicy{
		OrganizationID: testOrgID, AllowedMethods: []string{"totp", "recovery_code", "passkey"},
		AuthenticationChallengeMins: 5, StepUpTTLMinutes: 5, Version: 1,
		UpdatedBy: testUserID, UpdateReason: "Initial test policy", CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (routerIdentityService) IssueInvitation(context.Context, string, string, string, models.IdentityInvitationIssueInput, models.IdentityRequestMetadata) (*models.IdentityInvitation, error) {
	now := time.Now().UTC()
	return &models.IdentityInvitation{
		ID: "11000000-0000-0000-0000-000000000010", OrganizationID: testOrgID,
		UserID: testUserID, Email: "user@example.com", ExpiresAt: now.Add(72 * time.Hour),
		CreatedBy: testUserID, CreatedAt: now, DeliveryQueued: true,
	}, nil
}

func (routerIdentityService) AcceptInvitation(context.Context, models.IdentityInvitationAcceptInput, models.IdentityRequestMetadata) (*models.IdentityAcceptanceResult, error) {
	return &models.IdentityAcceptanceResult{OrganizationID: testOrgID, UserID: testUserID, EmailVerifiedAt: time.Now().UTC()}, nil
}

func (routerIdentityService) RequestEmailVerification(context.Context, models.IdentityEmailRequest, models.IdentityRequestMetadata) error {
	return nil
}

func (routerIdentityService) RequestPasswordReset(context.Context, models.IdentityEmailRequest, models.IdentityRequestMetadata) error {
	return nil
}

func (routerIdentityService) BeginPasskeyAuthentication(context.Context, models.IdentityPasskeyAuthenticationBeginInput, models.IdentityRequestMetadata) (*models.IdentityPasskeyCeremony, error) {
	return &models.IdentityPasskeyCeremony{
		ChallengeToken: strings.Repeat("x", 32), Options: []byte(`{}`), ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	}, nil
}

func (routerIdentityService) ListSessions(context.Context, string, string, string) ([]models.IdentitySession, error) {
	return []models.IdentitySession{}, nil
}

type routerTokenValidator struct {
	claims *authdomain.Claims
}

type routerAuthorizer struct {
	allowed  bool
	err      error
	requests []authz.Request
}

func (a *routerAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	a.requests = append(a.requests, request)
	return authz.Decision{
		Allowed:            a.allowed,
		Reason:             "test authorization decision",
		ReasonCode:         "test_authorization_decision",
		ConstraintsApplied: false,
		Obligations:        []authz.Obligation{},
	}, a.err
}

func (v routerTokenValidator) ValidateAccessToken(context.Context, string) (*authdomain.Claims, error) {
	return v.claims, nil
}

type routerOrganizationService struct {
	organization *models.Organization
}

type routerComplianceService struct{}

type routerPermissionService struct{}

func (routerPermissionService) GetUserPermissions(context.Context, string, string) (map[string][]string, error) {
	return map[string][]string{
		"audits":   {"read"},
		"settings": {"read"},
	}, nil
}

type routerRiskService struct{ risk *models.Risk }

type routerPolicyService struct {
	handler.PolicyService
	policy *models.Policy
}

func (s routerPolicyService) Create(context.Context, string, string, models.PolicyCreateInput) (*models.Policy, error) {
	return s.policy, nil
}

func (s routerPolicyService) GetByID(context.Context, string, string) (*models.Policy, error) {
	return s.policy, nil
}

func (s routerPolicyService) List(context.Context, string, models.PolicyListFilter) ([]models.Policy, int, error) {
	return []models.Policy{*s.policy}, 1, nil
}

func (routerPolicyService) ListCategories(context.Context, string) ([]models.PolicyCategory, error) {
	return []models.PolicyCategory{}, nil
}

func (s routerPolicyService) SubmitForApproval(context.Context, string, string, string, models.PolicySubmitInput) (*models.PolicyApprovalWorkflow, error) {
	return &models.PolicyApprovalWorkflow{PolicyID: s.policy.ID, OrganizationID: s.policy.OrganizationID, Status: "pending", Steps: []models.PolicyApprovalStep{}}, nil
}

func (s routerPolicyService) DecideApproval(context.Context, string, string, string, string, models.PolicyApprovalDecisionInput) (*models.PolicyApprovalWorkflow, error) {
	return &models.PolicyApprovalWorkflow{PolicyID: s.policy.ID, OrganizationID: s.policy.OrganizationID, Status: "approved", Steps: []models.PolicyApprovalStep{}}, nil
}

type routerAuditService struct {
	handler.AuditService
	audit *models.Audit
}

type routerIncidentService struct {
	handler.IncidentService
	incident *models.Incident
}

type routerAssetService struct{ asset *models.Asset }

type routerVendorService struct {
	handler.VendorService
	vendor *models.Vendor
}

type routerAccessAdministrationService struct {
	role *models.ManagedRole
}

type routerUserAdministrationService struct {
	handler.UserAdministrationService
	user  *models.DirectoryUser
	group *models.DirectoryGroup
}

type routerFeatureFlagService struct{}

type routerDataGovernanceService struct {
	handler.DataGovernanceService
	policy *models.DataGovernancePolicy
}

type routerDiagnosticsService struct{}

func (routerDiagnosticsService) GetSnapshot(_ context.Context, organizationID string) (*models.DiagnosticsSnapshot, error) {
	return &models.DiagnosticsSnapshot{
		OrganizationID: organizationID,
		GeneratedAt:    time.Now().UTC(),
		OverallStatus:  models.DiagnosticStatusHealthy,
		Dependencies:   []models.DependencyDiagnostic{},
		Migration: models.MigrationDiagnostic{
			CurrentVersion: 53, SupportedVersion: 53, Status: models.DiagnosticStatusHealthy,
		},
		Queue:         models.QueueDiagnostic{Status: models.DiagnosticStatusHealthy},
		Notifications: models.NotificationDiagnostic{Status: models.DiagnosticStatusHealthy},
		Connectors:    models.ConnectorDiagnostic{Status: models.DiagnosticStatusHealthy},
		Configuration: []models.ConfigurationDiagnostic{},
	}, nil
}

func (s routerDataGovernanceService) GetPolicy(context.Context, string) (*models.DataGovernancePolicy, error) {
	return s.policy, nil
}

func (s routerDataGovernanceService) UpsertPolicy(
	_ context.Context, organizationID, actorID, _ string, input models.DataGovernancePolicyInput,
) (*models.DataGovernancePolicy, error) {
	item := *s.policy
	item.OrganizationID, item.PrimaryRegion, item.AllowedRegions = organizationID, input.PrimaryRegion, input.AllowedRegions
	item.UpdatedBy = actorID
	return &item, nil
}

type routerFeatureEvaluator struct {
	evaluation *models.FeatureFlagEvaluation
	err        error
	keys       []string
}

type routerEntitlementChecker struct {
	decision *models.EntitlementLimitDecision
	err      error
	metrics  []string
}

func (c *routerEntitlementChecker) CheckLimit(_ context.Context, _ string, metric string, _ int64) (*models.EntitlementLimitDecision, error) {
	c.metrics = append(c.metrics, metric)
	return c.decision, c.err
}

func (e *routerFeatureEvaluator) Evaluate(_ context.Context, _, key string) (*models.FeatureFlagEvaluation, error) {
	e.keys = append(e.keys, key)
	return e.evaluation, e.err
}

func (routerFeatureFlagService) ListEvaluations(context.Context, string) ([]models.FeatureFlagEvaluation, error) {
	return []models.FeatureFlagEvaluation{{
		Capability: models.ProductCapability{Key: "advanced_reporting", DisplayName: "Advanced reporting"},
		Enabled:    true, Entitled: true, InRollout: true, Variant: map[string]any{},
	}}, nil
}

func (routerFeatureFlagService) Evaluate(context.Context, string, string) (*models.FeatureFlagEvaluation, error) {
	return &models.FeatureFlagEvaluation{
		Capability: models.ProductCapability{Key: "advanced_reporting", DisplayName: "Advanced reporting"},
		Enabled:    true, Entitled: true, InRollout: true, Variant: map[string]any{},
	}, nil
}

func (routerFeatureFlagService) GetEntitlements(_ context.Context, organizationID string) (*models.EntitlementSnapshot, error) {
	return &models.EntitlementSnapshot{
		OrganizationID: organizationID, Source: "organization_tier", Tier: "enterprise",
		Features: map[string]bool{}, Limits: map[string]int64{"users": 100}, Usage: map[string]int64{"users": 1},
	}, nil
}

func (routerFeatureFlagService) CheckLimit(context.Context, string, string, int64) (*models.EntitlementLimitDecision, error) {
	return &models.EntitlementLimitDecision{Metric: "users", Allowed: true, Limit: 100, Usage: 1, Requested: 1}, nil
}

func (routerFeatureFlagService) UpsertOverride(_ context.Context, organizationID, capabilityKey, actorID, _ string, input models.FeatureFlagOverrideInput) (*models.TenantFeatureFlagOverride, error) {
	return &models.TenantFeatureFlagOverride{
		OrganizationID: organizationID, CapabilityKey: capabilityKey, Enabled: input.Enabled,
		Reason: input.Reason, Version: 1, CreatedBy: actorID, UpdatedBy: actorID, Variant: map[string]any{},
	}, nil
}

func (routerFeatureFlagService) ResetOverride(context.Context, string, string, string, string, models.FeatureFlagResetInput) error {
	return nil
}

func (routerFeatureFlagService) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.FeatureFlagChangeEvent, int, error) {
	return []models.FeatureFlagChangeEvent{}, 0, nil
}

func (routerAccessAdministrationService) ListPermissions(context.Context, string) ([]models.PermissionGrant, error) {
	return []models.PermissionGrant{{Resource: "settings", Action: "read"}}, nil
}

func (s routerAccessAdministrationService) CreateRole(_ context.Context, organizationID, _ string, input models.ManagedRoleCreateInput) (*models.ManagedRole, error) {
	item := *s.role
	item.OrganizationID, item.Name, item.Slug, item.Permissions = &organizationID, input.Name, input.Slug, input.Permissions
	return &item, nil
}

func (s routerAccessAdministrationService) GetRole(context.Context, string, string) (*models.ManagedRole, error) {
	return s.role, nil
}

func (s routerAccessAdministrationService) ListRoles(context.Context, string, models.ManagedRoleListFilter) ([]models.ManagedRole, int, error) {
	return []models.ManagedRole{*s.role}, 1, nil
}

func (s routerAccessAdministrationService) UpdateRole(context.Context, string, string, string, models.ManagedRolePatch) (*models.ManagedRole, error) {
	return s.role, nil
}

func (routerAccessAdministrationService) DeleteRole(context.Context, string, string, string, int64) error {
	return nil
}

func (s routerAccessAdministrationService) CloneRole(context.Context, string, string, string, models.ManagedRoleCloneInput) (*models.ManagedRole, error) {
	return s.role, nil
}

func (s routerAccessAdministrationService) PreviewImpact(context.Context, string, string, []models.PermissionGrant) (*models.ManagedRoleImpact, error) {
	return &models.ManagedRoleImpact{RoleID: s.role.ID}, nil
}

func (routerAccessAdministrationService) ListAssignments(context.Context, string, string) ([]models.ManagedRoleAssignment, error) {
	return []models.ManagedRoleAssignment{}, nil
}

func (routerAccessAdministrationService) AssignRole(context.Context, string, string, string, models.ManagedRoleAssignmentInput) error {
	return nil
}

func (routerAccessAdministrationService) UnassignRole(context.Context, string, string, string, string, models.ManagedRoleUnassignmentInput) error {
	return nil
}

func (routerAccessAdministrationService) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.RoleChangeEvent, int, error) {
	return []models.RoleChangeEvent{}, 0, nil
}

func (s routerUserAdministrationService) CreateUser(context.Context, string, string, models.DirectoryUserCreateInput) (*models.DirectoryUser, error) {
	return s.user, nil
}

func (s routerUserAdministrationService) GetUser(context.Context, string, string) (*models.DirectoryUser, error) {
	return s.user, nil
}

func (s routerUserAdministrationService) ListUsers(context.Context, string, models.DirectoryUserListFilter) ([]models.DirectoryUser, int, error) {
	return []models.DirectoryUser{*s.user}, 1, nil
}

func (s routerUserAdministrationService) UpdateUser(context.Context, string, string, string, models.DirectoryUserPatch) (*models.DirectoryUser, error) {
	return s.user, nil
}

func (s routerUserAdministrationService) SuspendUser(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error) {
	return s.user, nil
}

func (s routerUserAdministrationService) ReactivateUser(context.Context, string, string, string, models.DirectoryUserStateInput) (*models.DirectoryUser, error) {
	return s.user, nil
}

func (s routerUserAdministrationService) PreviewOwnership(context.Context, string, string) (*models.DirectoryOwnershipImpact, error) {
	return &models.DirectoryOwnershipImpact{UserID: s.user.ID}, nil
}

func (s routerUserAdministrationService) TransferOwnership(context.Context, string, string, string, models.DirectoryOwnershipTransferInput) (*models.DirectoryOwnershipImpact, *models.DirectoryUser, error) {
	return &models.DirectoryOwnershipImpact{UserID: s.user.ID}, s.user, nil
}

func (routerUserAdministrationService) DeprovisionUser(context.Context, string, string, string, models.DirectoryUserDeprovisionInput) error {
	return nil
}

func (routerUserAdministrationService) ListUserEvents(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error) {
	return []models.DirectoryChangeEvent{}, 0, nil
}

func (s routerUserAdministrationService) CreateGroup(context.Context, string, string, models.DirectoryGroupCreateInput) (*models.DirectoryGroup, error) {
	return s.group, nil
}

func (s routerUserAdministrationService) GetGroup(context.Context, string, string) (*models.DirectoryGroup, error) {
	return s.group, nil
}

func (s routerUserAdministrationService) ListGroups(context.Context, string, models.DirectoryGroupListFilter) ([]models.DirectoryGroup, int, error) {
	return []models.DirectoryGroup{*s.group}, 1, nil
}

func (s routerUserAdministrationService) UpdateGroup(context.Context, string, string, string, models.DirectoryGroupPatch) (*models.DirectoryGroup, error) {
	return s.group, nil
}

func (routerUserAdministrationService) DeleteGroup(context.Context, string, string, string, int64, string) error {
	return nil
}

func (s routerUserAdministrationService) ListGroupMembers(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryUser, int, error) {
	return []models.DirectoryUser{*s.user}, 1, nil
}

func (s routerUserAdministrationService) AddGroupMember(context.Context, string, string, string, models.DirectoryGroupMemberInput) (*models.DirectoryGroup, error) {
	return s.group, nil
}

func (s routerUserAdministrationService) RemoveGroupMember(context.Context, string, string, string, string, models.DirectoryGroupMemberRemoveInput) (*models.DirectoryGroup, error) {
	return s.group, nil
}

func (s routerUserAdministrationService) ChangeGroupMembers(context.Context, string, string, string, models.DirectoryGroupBulkMembersInput) (*models.DirectoryGroup, error) {
	return s.group, nil
}

func (routerUserAdministrationService) ListGroupEvents(context.Context, string, string, models.PaginationRequest) ([]models.DirectoryChangeEvent, int, error) {
	return []models.DirectoryChangeEvent{}, 0, nil
}

func (routerUserAdministrationService) PreviewImport(context.Context, string, []byte) (*models.DirectoryImportPreview, error) {
	return &models.DirectoryImportPreview{RowCount: 1, ValidCount: 1, CreateCount: 1}, nil
}

func (routerUserAdministrationService) ApplyImport(context.Context, string, string, string, string, []byte) (*models.DirectoryImportResult, error) {
	return &models.DirectoryImportResult{ID: testGroupID, RowCount: 1, CreatedCount: 1}, nil
}

func (s routerVendorService) Create(_ context.Context, organizationID, actorID string, input models.VendorCreateInput) (*models.Vendor, error) {
	item := *s.vendor
	item.OrganizationID, item.CreatedBy, item.Name = organizationID, actorID, input.Name
	return &item, nil
}

func (s routerVendorService) GetByID(context.Context, string, string) (*models.Vendor, error) {
	return s.vendor, nil
}

func (s routerVendorService) List(context.Context, string, models.VendorListFilter) ([]models.Vendor, int, error) {
	return []models.Vendor{*s.vendor}, 1, nil
}

func (routerVendorService) Statistics(context.Context, string) (*models.VendorStatistics, error) {
	return &models.VendorStatistics{Total: 1, Active: 1, ByStatus: map[models.VendorStatus]int{}, ByTier: map[models.VendorTier]int{}, ByCriticality: map[models.VendorCriticality]int{}}, nil
}

func (s routerVendorService) RecordAssessment(context.Context, string, string, string, models.VendorAssessmentInput) (*models.Vendor, error) {
	return s.vendor, nil
}

func (routerVendorService) ListDueForAssessment(context.Context, string, int, int) ([]models.Vendor, error) {
	return []models.Vendor{}, nil
}

func (routerVendorService) ListDueContracts(context.Context, string, int, int) ([]models.VendorDueContract, error) {
	return []models.VendorDueContract{}, nil
}

func (routerVendorService) ListExpiringCertifications(context.Context, string, int, int) ([]models.VendorExpiringCertification, error) {
	return []models.VendorExpiringCertification{}, nil
}

func (routerVendorService) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.VendorEvent, int, error) {
	return []models.VendorEvent{}, 0, nil
}

func (s routerAssetService) Create(_ context.Context, organizationID, actorID string, input models.AssetCreateInput) (*models.Asset, error) {
	item := *s.asset
	item.OrganizationID, item.CreatedBy, item.Name, item.AssetType = organizationID, actorID, input.Name, input.AssetType
	return &item, nil
}

func (s routerAssetService) GetByID(context.Context, string, string) (*models.Asset, error) {
	return s.asset, nil
}

func (s routerAssetService) Update(context.Context, string, string, string, models.AssetPatch) (*models.Asset, error) {
	return s.asset, nil
}

func (routerAssetService) Delete(context.Context, string, string, string, *int64) error { return nil }

func (s routerAssetService) List(context.Context, string, models.AssetListFilter) ([]models.Asset, int, error) {
	return []models.Asset{*s.asset}, 1, nil
}

func (routerAssetService) Stats(context.Context, string) (*models.AssetStats, error) {
	return &models.AssetStats{Total: 1, Active: 1, ByType: map[string]int{"data": 1}}, nil
}

func (routerAssetService) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.AssetLifecycleEvent, int, error) {
	return []models.AssetLifecycleEvent{}, 0, nil
}

func (s routerIncidentService) Create(context.Context, string, string, models.IncidentCreateInput) (*models.Incident, error) {
	return s.incident, nil
}

func (s routerIncidentService) GetByID(context.Context, string, string) (*models.Incident, error) {
	return s.incident, nil
}

func (s routerIncidentService) List(context.Context, string, models.IncidentListFilter) ([]models.Incident, int, error) {
	return []models.Incident{*s.incident}, 1, nil
}

func (routerIncidentService) ListBreachDue(context.Context, string, int, int) ([]models.Incident, error) {
	return []models.Incident{}, nil
}

func (routerIncidentService) Statistics(context.Context, string) (*models.IncidentStatistics, error) {
	return &models.IncidentStatistics{ByStatus: map[models.IncidentStatus]int{}, BySeverity: map[models.IncidentSeverity]int{}}, nil
}

func (routerIncidentService) ListEvents(context.Context, string, string, models.PaginationRequest) ([]models.IncidentEvent, int, error) {
	return []models.IncidentEvent{}, 0, nil
}

func (routerIncidentService) ListAssignments(context.Context, string, string, bool) ([]models.IncidentAssignment, error) {
	return []models.IncidentAssignment{}, nil
}

func (s routerAuditService) Create(context.Context, string, string, models.AuditCreateInput) (*models.Audit, error) {
	return s.audit, nil
}

func (s routerAuditService) GetByID(context.Context, string, string) (*models.Audit, error) {
	return s.audit, nil
}

func (s routerAuditService) List(context.Context, string, models.AuditListFilter) ([]models.Audit, int, error) {
	return []models.Audit{*s.audit}, 1, nil
}

func (s routerRiskService) Create(context.Context, string, models.RiskCreateInput) (*models.Risk, error) {
	return s.risk, nil
}
func (s routerRiskService) GetByID(context.Context, string, string) (*models.Risk, error) {
	return s.risk, nil
}
func (s routerRiskService) Update(context.Context, string, string, models.RiskPatch) (*models.Risk, error) {
	return s.risk, nil
}
func (s routerRiskService) Assign(context.Context, string, string, *string, *string) (*models.Risk, error) {
	return s.risk, nil
}
func (routerRiskService) Delete(context.Context, string, string) error { return nil }
func (s routerRiskService) List(context.Context, string, models.RiskListFilter) ([]models.Risk, int, error) {
	return []models.Risk{*s.risk}, 1, nil
}
func (routerRiskService) GetRiskMatrix(context.Context, string, string) (*models.RiskMatrixView, error) {
	return &models.RiskMatrixView{Dimension: "residual", Cells: []models.RiskMatrixCell{}}, nil
}
func (routerRiskService) ListCategories(context.Context, string) ([]models.RiskCategory, error) {
	return []models.RiskCategory{}, nil
}
func (routerRiskService) GetRiskHeatmap(context.Context, string) ([]models.RiskHeatmapEntry, error) {
	return []models.RiskHeatmapEntry{}, nil
}
func (routerRiskService) CreateAssessment(context.Context, string, string, string, models.RiskAssessmentInput) (*models.RiskAssessment, error) {
	return &models.RiskAssessment{}, nil
}
func (routerRiskService) ListAssessments(context.Context, string, string, models.PaginationRequest) ([]models.RiskAssessment, int, error) {
	return []models.RiskAssessment{}, 0, nil
}
func (routerRiskService) CreateTreatment(context.Context, string, string, string, models.RiskTreatmentInput) (*models.RiskTreatment, error) {
	return &models.RiskTreatment{}, nil
}
func (routerRiskService) GetTreatment(context.Context, string, string, string) (*models.RiskTreatment, error) {
	return &models.RiskTreatment{}, nil
}
func (routerRiskService) ListTreatments(context.Context, string, string, models.PaginationRequest) ([]models.RiskTreatment, int, error) {
	return []models.RiskTreatment{}, 0, nil
}
func (routerRiskService) UpdateTreatment(context.Context, string, string, string, models.RiskTreatmentPatch) (*models.RiskTreatment, error) {
	return &models.RiskTreatment{}, nil
}
func (routerRiskService) ListAppetite(context.Context, string) ([]models.RiskAppetiteStatement, error) {
	return []models.RiskAppetiteStatement{}, nil
}
func (routerRiskService) UpsertAppetite(context.Context, string, string, string, models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error) {
	return &models.RiskAppetiteStatement{}, nil
}
func (routerRiskService) ApproveAppetite(context.Context, string, string, string, models.RiskAppetiteInput) (*models.RiskAppetiteStatement, error) {
	return &models.RiskAppetiteStatement{}, nil
}
func (routerRiskService) CreateIndicator(context.Context, string, string, string, models.RiskIndicatorInput) (*models.RiskIndicator, error) {
	return &models.RiskIndicator{}, nil
}
func (routerRiskService) ListIndicators(context.Context, string, string) ([]models.RiskIndicator, error) {
	return []models.RiskIndicator{}, nil
}
func (routerRiskService) RecordIndicatorValue(context.Context, string, string, string, string, models.RiskIndicatorValueInput) (*models.RiskIndicatorValue, error) {
	return &models.RiskIndicatorValue{}, nil
}
func (routerRiskService) ListIndicatorValues(context.Context, string, string, string, models.PaginationRequest) ([]models.RiskIndicatorValue, int, error) {
	return []models.RiskIndicatorValue{}, 0, nil
}

func (routerComplianceService) ListFrameworks(context.Context, string, models.PaginationRequest) ([]models.ComplianceFramework, int, error) {
	return []models.ComplianceFramework{{BaseModel: models.BaseModel{ID: testFrameworkID}, Code: "TEST", Name: "Test", Version: "1"}}, 1, nil
}
func (routerComplianceService) GetFramework(context.Context, string, string) (*models.ComplianceFramework, error) {
	return &models.ComplianceFramework{BaseModel: models.BaseModel{ID: testFrameworkID}, Code: "TEST", Name: "Test", Version: "1"}, nil
}
func (routerComplianceService) AdoptFramework(context.Context, string, string, string) (*models.OrganizationFramework, error) {
	return &models.OrganizationFramework{BaseModel: models.BaseModel{ID: "50000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID, FrameworkID: testFrameworkID}, nil
}
func (routerComplianceService) ListFrameworkControls(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error) {
	return []models.Control{{BaseModel: models.BaseModel{ID: testControlID}, FrameworkID: testFrameworkID, Code: "A.1", Title: "Test"}}, 1, nil
}
func (routerComplianceService) ListControls(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error) {
	return []models.Control{{BaseModel: models.BaseModel{ID: testControlID}, FrameworkID: testFrameworkID, Code: "A.1", Title: "Test"}}, 1, nil
}
func (routerComplianceService) GetControl(context.Context, string, string) (*models.Control, error) {
	return &models.Control{BaseModel: models.BaseModel{ID: testControlID}, FrameworkID: testFrameworkID, Code: "A.1", Title: "Test"}, nil
}
func (routerComplianceService) UpdateControlImplementation(context.Context, string, string, models.ControlImplementationPatch) (*models.ControlImplementation, error) {
	return &models.ControlImplementation{BaseModel: models.BaseModel{ID: "60000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID, FrameworkControlID: testControlID}, nil
}
func (routerComplianceService) AttachControlEvidence(context.Context, string, string, string, models.AttachControlEvidenceInput) (*models.ControlEvidence, error) {
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: "70000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID}, nil
}
func (routerComplianceService) ListControlEvidence(context.Context, string, string, models.PaginationRequest) ([]models.ControlEvidence, int, error) {
	return []models.ControlEvidence{}, 0, nil
}

func (s *routerOrganizationService) Create(context.Context, *models.Organization) error {
	return nil
}

func (s *routerOrganizationService) GetByID(context.Context, string) (*models.Organization, error) {
	return s.organization, nil
}

func (s *routerOrganizationService) Update(context.Context, *models.Organization) error {
	return nil
}

func (s *routerOrganizationService) Delete(context.Context, string) error {
	return nil
}

func (s *routerOrganizationService) List(context.Context, models.PaginationRequest) ([]models.Organization, int, error) {
	return []models.Organization{*s.organization}, 1, nil
}

func TestNewRouterWithDependenciesFailsFast(t *testing.T) {
	base := testRouterDependencies()
	tests := []struct {
		name   string
		mutate func(*RouterDependencies)
		want   string
	}{
		{"auth handler", func(d *RouterDependencies) { d.Auth = nil }, "auth handler is required"},
		{"unconfigured auth handler", func(d *RouterDependencies) { d.Auth = handler.NewAuthHandler(nil) }, "auth handler is required"},
		{"auth identity completion", func(d *RouterDependencies) {
			d.Auth = handler.NewAuthHandler(routerLegacyAuthService{AuthService: &routerAuthService{user: &models.User{}}})
		}, "identity authentication completion is required"},
		{"identity handler", func(d *RouterDependencies) { d.Identity = nil }, "identity lifecycle handler is required"},
		{"unconfigured identity handler", func(d *RouterDependencies) { d.Identity = handler.NewIdentityLifecycleHandler(nil) }, "identity lifecycle handler is required"},
		{"SCIM handler", func(d *RouterDependencies) { d.SCIM = nil }, "SCIM handler is required"},
		{"unconfigured SCIM handler", func(d *RouterDependencies) { d.SCIM = handler.NewSCIMHandler(nil) }, "SCIM handler is required"},
		{"organization handler", func(d *RouterDependencies) { d.Organizations = nil }, "organization handler is required"},
		{"unconfigured organization handler", func(d *RouterDependencies) { d.Organizations = handler.NewOrganizationHandler(nil) }, "organization handler is required"},
		{"framework handler", func(d *RouterDependencies) { d.Frameworks = nil }, "framework handler is required"},
		{"unconfigured framework handler", func(d *RouterDependencies) { d.Frameworks = handler.NewFrameworkHandler(nil) }, "framework handler is required"},
		{"control handler", func(d *RouterDependencies) { d.Controls = nil }, "control handler is required"},
		{"unconfigured control handler", func(d *RouterDependencies) { d.Controls = handler.NewControlHandler(nil) }, "control handler is required"},
		{"control evidence service", func(d *RouterDependencies) { d.Controls = handler.NewControlHandler(routerComplianceService{}) }, "control handler is required"},
		{"risk handler", func(d *RouterDependencies) { d.Risks = nil }, "risk handler is required"},
		{"unconfigured risk handler", func(d *RouterDependencies) { d.Risks = handler.NewRiskHandler(nil) }, "risk handler is required"},
		{"policy handler", func(d *RouterDependencies) { d.Policies = nil }, "policy handler is required"},
		{"unconfigured policy handler", func(d *RouterDependencies) { d.Policies = handler.NewPolicyHandler(nil) }, "policy handler is required"},
		{"audit handler", func(d *RouterDependencies) { d.Audits = nil }, "audit handler is required"},
		{"unconfigured audit handler", func(d *RouterDependencies) { d.Audits = handler.NewAuditHandler(nil) }, "audit handler is required"},
		{"incident handler", func(d *RouterDependencies) { d.Incidents = nil }, "incident handler is required"},
		{"unconfigured incident handler", func(d *RouterDependencies) { d.Incidents = handler.NewIncidentHandler(nil) }, "incident handler is required"},
		{"asset handler", func(d *RouterDependencies) { d.Assets = nil }, "asset handler is required"},
		{"unconfigured asset handler", func(d *RouterDependencies) { d.Assets = handler.NewAssetHandler(nil) }, "asset handler is required"},
		{"vendor handler", func(d *RouterDependencies) { d.Vendors = nil }, "vendor handler is required"},
		{"unconfigured vendor handler", func(d *RouterDependencies) { d.Vendors = handler.NewVendorHandler(nil) }, "vendor handler is required"},
		{"access administration handler", func(d *RouterDependencies) { d.AccessAdministration = nil }, "access administration handler is required"},
		{"unconfigured access administration handler", func(d *RouterDependencies) { d.AccessAdministration = handler.NewAccessAdministrationHandler(nil) }, "access administration handler is required"},
		{"user administration handler", func(d *RouterDependencies) { d.UserAdministration = nil }, "user administration handler is required"},
		{"unconfigured user administration handler", func(d *RouterDependencies) { d.UserAdministration = handler.NewUserAdministrationHandler(nil) }, "user administration handler is required"},
		{"feature flag handler", func(d *RouterDependencies) { d.FeatureFlags = nil }, "feature flag handler is required"},
		{"unconfigured feature flag handler", func(d *RouterDependencies) { d.FeatureFlags = handler.NewFeatureFlagHandler(nil) }, "feature flag handler is required"},
		{"data governance handler", func(d *RouterDependencies) { d.DataGovernance = nil }, "data governance handler is required"},
		{"unconfigured data governance handler", func(d *RouterDependencies) { d.DataGovernance = handler.NewDataGovernanceHandler(nil) }, "data governance handler is required"},
		{"diagnostics handler", func(d *RouterDependencies) { d.Diagnostics = nil }, "diagnostics handler is required"},
		{"unconfigured diagnostics handler", func(d *RouterDependencies) { d.Diagnostics = handler.NewDiagnosticsHandler(nil) }, "diagnostics handler is required"},
		{"missing support bundle service", func(d *RouterDependencies) { d.Diagnostics = handler.NewDiagnosticsHandler(routerDiagnosticsService{}) }, "support bundle consent service is required"},
		{"nil support bundle service", func(d *RouterDependencies) {
			var nilProvider *routerSupportBundleService
			d.Diagnostics = handler.NewDiagnosticsHandler(routerDiagnosticsService{}, handler.WithSupportBundleService(nilProvider))
		}, "support bundle consent service is required"},
		{"permission handler", func(d *RouterDependencies) { d.Permissions = nil }, "permission handler is required"},
		{"unconfigured permission handler", func(d *RouterDependencies) { d.Permissions = handler.NewPermissionHandler(nil) }, "permission handler is required"},
		{"notification handler", func(d *RouterDependencies) { d.Notifications = nil }, "notification handler is required"},
		{"unconfigured notification handler", func(d *RouterDependencies) { d.Notifications = handler.NewNotificationHandler(nil, nil) }, "notification handler is required"},
		{"integration handler", func(d *RouterDependencies) { d.Integrations = nil }, "integration handler is required"},
		{"unconfigured integration handler", func(d *RouterDependencies) { d.Integrations = handler.NewIntegrationHandler(nil) }, "integration handler is required"},
		{"API-key authenticator", func(d *RouterDependencies) { d.APIKeyAuthenticator = nil }, "API-key authenticator is required"},
		{"SCIM authenticator", func(d *RouterDependencies) { d.SCIMAuthenticator = nil }, "SCIM authenticator is required"},
		{"API-key rate limiter", func(d *RouterDependencies) { d.APIKeyRateLimiter = nil }, "API-key rate limiter is required"},
		{"request rate limiter", func(d *RouterDependencies) { d.RequestRateLimiter = nil }, "request rate limiter is required"},
		{"token validator", func(d *RouterDependencies) { d.AccessTokenValidator = nil }, "access-token validator is required"},
		{"authorizer", func(d *RouterDependencies) { d.Authorizer = nil }, "authorizer is required"},
		{"typed nil authorizer", func(d *RouterDependencies) { var authorizer *routerAuthorizer; d.Authorizer = authorizer }, "authorizer is required"},
		{"feature evaluator", func(d *RouterDependencies) { d.FeatureEvaluator = nil }, "feature evaluator is required"},
		{"entitlement checker", func(d *RouterDependencies) { d.EntitlementChecker = nil }, "entitlement checker is required"},
		{"evidence scanner health check", func(d *RouterDependencies) { d.EvidenceScannerCheck = nil }, "evidence scanner health check is required"},
		{"health check", func(d *RouterDependencies) { d.HealthCheck = nil }, "health check is required"},
		{"tenant middleware", func(d *RouterDependencies) { d.TenantMiddleware = nil }, "tenant middleware is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dependencies := base
			tt.mutate(&dependencies)
			_, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewRouterWithDependencies() error = %v, want %q", err, tt.want)
			}
		})
	}

	if _, err := NewRouterWithDependencies(nil, base); err == nil {
		t.Fatal("NewRouterWithDependencies(nil, ...) returned nil error")
	}
}

func TestNewRouterRejectsMissingProductionDependencies(t *testing.T) {
	if _, err := NewRouter(nil, testRouterConfig()); err == nil || !strings.Contains(err.Error(), "database pool is required") {
		t.Fatalf("NewRouter(nil, cfg) error = %v, want database pool error", err)
	}
	if _, err := BuildDependencies(nil, nil); err == nil || !strings.Contains(err.Error(), "database pool is required") {
		t.Fatalf("BuildDependencies(nil, nil) error = %v, want database pool error", err)
	}
}

func TestRouterMountsRequiredCoreRoutes(t *testing.T) {
	router, err := NewRouterWithDependencies(testRouterConfig(), testRouterDependencies())
	if err != nil {
		t.Fatalf("NewRouterWithDependencies() error = %v", err)
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		headers    map[string]string
		authorized bool
		wantStatus int
	}{
		{
			name:       "login",
			method:     http.MethodPost,
			path:       "/api/v1/auth/login",
			body:       `{"email":"user@example.com","password":"correct-password"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "register",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       `{"email":"user@example.com","password":"correct-password","first_name":"A","last_name":"User","organization_id":"` + testOrgID + `"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "refresh",
			method:     http.MethodPost,
			path:       "/api/v1/auth/refresh",
			body:       `{"refresh_token":"refresh-token"}`,
			wantStatus: http.StatusOK,
		},
		{name: "accept invitation", method: http.MethodPost, path: "/api/v1/auth/invitations/accept", body: `{"token":"opaque-token","password":"correct-password"}`, wantStatus: http.StatusOK},
		{name: "request password reset", method: http.MethodPost, path: "/api/v1/auth/password/forgot", body: `{"organization_id":"` + testOrgID + `","email":"user@example.com"}`, wantStatus: http.StatusAccepted},
		{name: "begin passkey authentication", method: http.MethodPost, path: "/api/v1/auth/passkeys/authentication/options", body: `{"organization_id":"` + testOrgID + `","email":"user@example.com"}`, wantStatus: http.StatusOK},
		{name: "complete login MFA", method: http.MethodPost, path: "/api/v1/auth/mfa/verify", body: `{"challenge_token":"opaque-token","method":"totp","code":"123456"}`, wantStatus: http.StatusOK},
		{
			name:       "me",
			method:     http.MethodGet,
			path:       "/api/v1/auth/me",
			authorized: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "me requires authentication",
			method:     http.MethodGet,
			path:       "/api/v1/auth/me",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "logout",
			method:     http.MethodPost,
			path:       "/api/v1/auth/logout",
			authorized: true,
			wantStatus: http.StatusNoContent,
		},
		{name: "identity policy", method: http.MethodGet, path: "/api/v1/identity/policy", authorized: true, wantStatus: http.StatusOK},
		{name: "identity sessions", method: http.MethodGet, path: "/api/v1/identity/sessions", authorized: true, wantStatus: http.StatusOK},
		{
			name:       "organization",
			method:     http.MethodGet,
			path:       "/api/v1/organizations/" + testOrgID,
			authorized: true,
			wantStatus: http.StatusOK,
		},
		{name: "list frameworks", method: http.MethodGet, path: "/api/v1/frameworks", authorized: true, wantStatus: http.StatusOK},
		{name: "adopt framework", method: http.MethodPost, path: "/api/v1/frameworks/" + testFrameworkID + "/adopt", authorized: true, wantStatus: http.StatusOK},
		{name: "list framework controls", method: http.MethodGet, path: "/api/v1/frameworks/" + testFrameworkID + "/controls", authorized: true, wantStatus: http.StatusOK},
		{name: "update implementation", method: http.MethodPatch, path: "/api/v1/controls/" + testControlID + "/implementation", body: `{"maturity_level":2}`, authorized: true, wantStatus: http.StatusOK},
		{name: "attach evidence", method: http.MethodPost, path: "/api/v1/controls/" + testControlID + "/evidence", body: `{"title":"Policy","evidence_type":"policy"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "list evidence", method: http.MethodGet, path: "/api/v1/controls/" + testControlID + "/evidence", authorized: true, wantStatus: http.StatusOK},
		{name: "list risks", method: http.MethodGet, path: "/api/v1/risks", authorized: true, wantStatus: http.StatusOK},
		{name: "create risk", method: http.MethodPost, path: "/api/v1/risks", body: `{"title":"Availability","inherent_likelihood":4,"inherent_impact":5}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "risk matrix", method: http.MethodGet, path: "/api/v1/risks/matrix", authorized: true, wantStatus: http.StatusOK},
		{name: "risk categories", method: http.MethodGet, path: "/api/v1/risks/categories", authorized: true, wantStatus: http.StatusOK},
		{name: "create assessment", method: http.MethodPost, path: "/api/v1/risks/80000000-0000-0000-0000-000000000010/assessments", body: `{"assessment_type":"periodic","likelihood_after":3,"impact_after":4}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "list policies", method: http.MethodGet, path: "/api/v1/policies", authorized: true, wantStatus: http.StatusOK},
		{name: "create policy", method: http.MethodPost, path: "/api/v1/policies", body: `{"title":"Security policy","initial_version":{"content_text":"Policy content"}}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "policy categories", method: http.MethodGet, path: "/api/v1/policies/categories", authorized: true, wantStatus: http.StatusOK},
		{name: "effective permissions", method: http.MethodGet, path: "/api/v1/access/my-permissions", authorized: true, wantStatus: http.StatusOK},
		{name: "permission catalogue", method: http.MethodGet, path: "/api/v1/access/permissions", authorized: true, wantStatus: http.StatusOK},
		{name: "list managed roles", method: http.MethodGet, path: "/api/v1/access/roles", authorized: true, wantStatus: http.StatusOK},
		{name: "create managed role", method: http.MethodPost, path: "/api/v1/access/roles", body: `{"name":"Control reviewer","permissions":[]}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "get managed role", method: http.MethodGet, path: "/api/v1/access/roles/" + testRoleID, authorized: true, wantStatus: http.StatusOK},
		{name: "patch managed role", method: http.MethodPatch, path: "/api/v1/access/roles/" + testRoleID, body: `{"expected_version":1}`, authorized: true, wantStatus: http.StatusOK},
		{name: "replace managed role", method: http.MethodPut, path: "/api/v1/access/roles/" + testRoleID, body: `{"expected_version":1}`, authorized: true, wantStatus: http.StatusOK},
		{name: "clone managed role", method: http.MethodPost, path: "/api/v1/access/roles/" + testRoleID + "/clone", body: `{"name":"Cloned reviewer"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "preview role impact", method: http.MethodPost, path: "/api/v1/access/roles/" + testRoleID + "/impact-preview", body: `{"permissions":[]}`, authorized: true, wantStatus: http.StatusOK},
		{name: "list role assignments", method: http.MethodGet, path: "/api/v1/access/roles/" + testRoleID + "/assignments", authorized: true, wantStatus: http.StatusOK},
		{name: "assign managed role", method: http.MethodPost, path: "/api/v1/access/roles/" + testRoleID + "/assignments", body: `{"user_id":"` + testUserID + `","reason":"Operational assignment"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "unassign managed role", method: http.MethodDelete, path: "/api/v1/access/roles/" + testRoleID + "/assignments/" + testUserID, body: `{"reason":"Role no longer needed"}`, authorized: true, wantStatus: http.StatusNoContent},
		{name: "managed role history", method: http.MethodGet, path: "/api/v1/access/roles/" + testRoleID + "/events", authorized: true, wantStatus: http.StatusOK},
		{name: "delete managed role", method: http.MethodDelete, path: "/api/v1/access/roles/" + testRoleID + "?expected_version=1", authorized: true, wantStatus: http.StatusNoContent},
		{name: "capability catalogue", method: http.MethodGet, path: "/api/v1/settings/capabilities", authorized: true, wantStatus: http.StatusOK},
		{name: "capability evaluation", method: http.MethodGet, path: "/api/v1/settings/capabilities/advanced_reporting/evaluation", authorized: true, wantStatus: http.StatusOK},
		{name: "subscription entitlements", method: http.MethodGet, path: "/api/v1/settings/entitlements", authorized: true, wantStatus: http.StatusOK},
		{name: "subscription limit check", method: http.MethodGet, path: "/api/v1/settings/entitlements/limits/users/check?requested=1", authorized: true, wantStatus: http.StatusOK},
		{name: "feature flag evaluations", method: http.MethodGet, path: "/api/v1/settings/feature-flags", authorized: true, wantStatus: http.StatusOK},
		{name: "create feature flag override", method: http.MethodPut, path: "/api/v1/settings/feature-flags/advanced_reporting", body: `{"enabled":true,"reason":"Controlled rollout"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "reset feature flag override", method: http.MethodPost, path: "/api/v1/settings/feature-flags/advanced_reporting/reset", body: `{"expected_version":1,"reason":"Return to plan default"}`, authorized: true, wantStatus: http.StatusNoContent},
		{name: "feature flag history", method: http.MethodGet, path: "/api/v1/settings/feature-flags/advanced_reporting/history", authorized: true, wantStatus: http.StatusOK},
		{name: "data governance policy", method: http.MethodGet, path: "/api/v1/settings/data-governance/policy", authorized: true, wantStatus: http.StatusOK},
		{name: "save data governance policy", method: http.MethodPut, path: "/api/v1/settings/data-governance/policy", body: `{"primary_region":"eu-west-1","allowed_regions":["eu-west-1"],"cross_border_transfer_mode":"approved_regions","default_retention_days":365,"deletion_grace_days":30,"disposition_approval_mode":"single","legal_hold_enabled":true,"reason":"Approve data lifecycle baseline"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "administrator diagnostics", method: http.MethodGet, path: "/api/v1/settings/diagnostics", authorized: true, wantStatus: http.StatusOK},
		{name: "list SCIM credentials", method: http.MethodGet, path: "/api/v1/settings/scim/tokens", authorized: true, wantStatus: http.StatusOK},
		{name: "list incidents", method: http.MethodGet, path: "/api/v1/incidents", authorized: true, wantStatus: http.StatusOK},
		{name: "create incident", method: http.MethodPost, path: "/api/v1/incidents", body: `{"title":"Database exposure","description":"A production snapshot was exposed","category":"privacy","severity":"high"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "incident statistics", method: http.MethodGet, path: "/api/v1/incidents/statistics", authorized: true, wantStatus: http.StatusOK},
		{name: "list assets", method: http.MethodGet, path: "/api/v1/assets", authorized: true, wantStatus: http.StatusOK},
		{name: "create asset", method: http.MethodPost, path: "/api/v1/assets", body: `{"name":"Customer database","asset_type":"data"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "asset statistics", method: http.MethodGet, path: "/api/v1/assets/stats", authorized: true, wantStatus: http.StatusOK},
		{name: "asset history", method: http.MethodGet, path: "/api/v1/assets/" + testAssetID + "/events", authorized: true, wantStatus: http.StatusOK},
		{name: "list vendors", method: http.MethodGet, path: "/api/v1/vendors", authorized: true, wantStatus: http.StatusOK},
		{name: "create vendor", method: http.MethodPost, path: "/api/v1/vendors", body: `{"name":"Nimbus Hosting","service_description":"Managed hosting","contact_name":"Ada Vendor","contact_email":"ada@example.test"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "vendor statistics", method: http.MethodGet, path: "/api/v1/vendors/statistics", authorized: true, wantStatus: http.StatusOK},
		{name: "vendor history", method: http.MethodGet, path: "/api/v1/vendors/" + testVendorID + "/timeline", authorized: true, wantStatus: http.StatusOK},
		{name: "create directory user", method: http.MethodPost, path: "/api/v1/directory/users", body: `{"email":"new.user@example.test","first_name":"New","last_name":"User","reason":"Provision approved user"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "list directory users", method: http.MethodGet, path: "/api/v1/directory/users?search=user&status=active&sort_by=email&sort_dir=asc", authorized: true, wantStatus: http.StatusOK},
		{name: "get directory user", method: http.MethodGet, path: "/api/v1/directory/users/" + testUserID, authorized: true, wantStatus: http.StatusOK},
		{name: "update directory user", method: http.MethodPatch, path: "/api/v1/directory/users/" + testUserID, body: `{"expected_version":1,"department":"Security","reason":"Department transfer approved"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "suspend directory user", method: http.MethodPost, path: "/api/v1/directory/users/" + testUserID + "/suspend", body: `{"expected_version":1,"reason":"Security investigation started"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "reactivate directory user", method: http.MethodPost, path: "/api/v1/directory/users/" + testUserID + "/reactivate", body: `{"expected_version":1,"reason":"Security investigation completed"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "preview ownership impact", method: http.MethodGet, path: "/api/v1/directory/users/" + testUserID + "/ownership-impact", authorized: true, wantStatus: http.StatusOK},
		{name: "transfer directory ownership", method: http.MethodPost, path: "/api/v1/directory/users/" + testUserID + "/transfer-ownership", body: `{"expected_version":1,"replacement_user_id":"` + testUserID + `","reason":"Transfer responsibilities before departure"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "deprovision directory user", method: http.MethodPost, path: "/api/v1/directory/users/" + testUserID + "/deprovision", body: `{"expected_version":1,"reason":"Employment ended after ownership transfer"}`, authorized: true, wantStatus: http.StatusNoContent},
		{name: "directory user history", method: http.MethodGet, path: "/api/v1/directory/users/" + testUserID + "/history", authorized: true, wantStatus: http.StatusOK},
		{name: "issue directory invitation", method: http.MethodPost, path: "/api/v1/directory/users/" + testUserID + "/invitation", body: `{"reason":"Approved onboarding invitation"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "preview directory import", method: http.MethodPost, path: "/api/v1/directory/users/import/preview", body: "email,first_name,last_name,role_slug\nnew.user@example.test,New,User,viewer\n", headers: map[string]string{"Content-Type": "text/csv"}, authorized: true, wantStatus: http.StatusOK},
		{name: "apply directory import", method: http.MethodPost, path: "/api/v1/directory/users/import", body: "email,first_name,last_name,role_slug\nnew.user@example.test,New,User,viewer\n", headers: map[string]string{"Content-Type": "text/csv", "Idempotency-Key": "directory-import-1", "X-Change-Reason": "Approved HR roster"}, authorized: true, wantStatus: http.StatusCreated},
		{name: "create directory group", method: http.MethodPost, path: "/api/v1/directory/groups", body: `{"name":"Reviewers","group_type":"static","reason":"Create reviewer cohort"}`, authorized: true, wantStatus: http.StatusCreated},
		{name: "list directory groups", method: http.MethodGet, path: "/api/v1/directory/groups", authorized: true, wantStatus: http.StatusOK},
		{name: "get directory group", method: http.MethodGet, path: "/api/v1/directory/groups/" + testGroupID, authorized: true, wantStatus: http.StatusOK},
		{name: "update directory group", method: http.MethodPatch, path: "/api/v1/directory/groups/" + testGroupID, body: `{"expected_version":1,"description":"Control reviewers","reason":"Clarify group purpose"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "list directory group members", method: http.MethodGet, path: "/api/v1/directory/groups/" + testGroupID + "/members", authorized: true, wantStatus: http.StatusOK},
		{name: "add directory group member", method: http.MethodPost, path: "/api/v1/directory/groups/" + testGroupID + "/members", body: `{"expected_version":1,"user_id":"` + testUserID + `","reason":"Reviewer assignment approved"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "remove directory group member", method: http.MethodDelete, path: "/api/v1/directory/groups/" + testGroupID + "/members/" + testUserID, body: `{"expected_version":1,"reason":"Reviewer assignment ended"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "bulk change directory group members", method: http.MethodPost, path: "/api/v1/directory/groups/" + testGroupID + "/members/bulk", body: `{"expected_version":1,"add_user_ids":["` + testUserID + `"],"reason":"Quarterly membership review"}`, authorized: true, wantStatus: http.StatusOK},
		{name: "directory group history", method: http.MethodGet, path: "/api/v1/directory/groups/" + testGroupID + "/history", authorized: true, wantStatus: http.StatusOK},
		{name: "delete directory group", method: http.MethodDelete, path: "/api/v1/directory/groups/" + testGroupID + "?expected_version=1", headers: map[string]string{"X-Change-Reason": "Group retired after review"}, authorized: true, wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			if tt.authorized {
				req.Header.Set("Authorization", "Bearer access-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
		})
	}
}

func TestRouterMountsSCIMProtocolAndKeepsAuthenticationIsolated(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.SCIMAuthenticator = routerSCIMAuthenticator{scopes: []string{models.SCIMTokenScopeUsersRead}}
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, path, authorization string
		want                      int
	}{
		{name: "SCIM bearer required", path: "/api/scim/v2/ServiceProviderConfig", want: http.StatusUnauthorized},
		{name: "browser JWT rejected", path: "/api/scim/v2/ServiceProviderConfig", authorization: "Bearer access-token", want: http.StatusUnauthorized},
		{name: "discovery", path: "/api/scim/v2/ServiceProviderConfig", authorization: "Bearer cfs_protocol-token", want: http.StatusOK},
		{name: "scoped users list", path: "/api/scim/v2/Users", authorization: "Bearer cfs_protocol-token", want: http.StatusOK},
		{name: "groups scope denied", path: "/api/scim/v2/Groups", authorization: "Bearer cfs_protocol-token", want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
			if response.Header().Get("Content-Type") != "application/scim+json" {
				t.Fatalf("content-type=%q", response.Header().Get("Content-Type"))
			}
		})
	}
}

func TestRouterFeatureGatesFailClosedWithUpgradeSafeResponses(t *testing.T) {
	for _, test := range []struct {
		name       string
		evaluation *models.FeatureFlagEvaluation
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "subscription", evaluation: &models.FeatureFlagEvaluation{Reason: "subscription_denied"}, wantStatus: http.StatusPaymentRequired, wantCode: "ENTITLEMENT_REQUIRED"},
		{name: "tenant disabled", evaluation: &models.FeatureFlagEvaluation{Reason: "tenant_disabled"}, wantStatus: http.StatusForbidden, wantCode: "FEATURE_DISABLED"},
		{name: "evaluation unavailable", err: errors.New("database password should remain private"), wantStatus: http.StatusServiceUnavailable, wantCode: "FEATURE_EVALUATION_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependencies := testRouterDependencies()
			evaluator := &routerFeatureEvaluator{evaluation: test.evaluation, err: test.err}
			dependencies.FeatureEvaluator = evaluator
			router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/vendors", nil)
			request.Header.Set("Authorization", "Bearer access-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), `"error_code":"`+test.wantCode+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if len(evaluator.keys) != 1 || evaluator.keys[0] != "vendor_management" {
				t.Fatalf("evaluated keys=%v", evaluator.keys)
			}
			if strings.Contains(response.Body.String(), "database password") {
				t.Fatalf("internal evaluation error leaked: %s", response.Body.String())
			}
		})
	}
}

func TestRouterQuotaPreflightUsesStableUpgradeResponse(t *testing.T) {
	dependencies := testRouterDependencies()
	checker := &routerEntitlementChecker{decision: &models.EntitlementLimitDecision{
		Metric: "risks", Allowed: false, Limit: 50, Usage: 50, Requested: 1, Reason: "limit_exceeded",
	}}
	dependencies.EntitlementChecker = checker
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/risks", strings.NewReader(`{
		"title":"Capacity should block","inherent_likelihood":4,"inherent_impact":5}`))
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusPaymentRequired || !strings.Contains(response.Body.String(), `"error_code":"ENTITLEMENT_LIMIT_EXCEEDED"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(checker.metrics) != 1 || checker.metrics[0] != "risks" {
		t.Fatalf("checked metrics=%v", checker.metrics)
	}
}

func TestAutomationRoutesEnforceAPIKeyScopes(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.APIKeyAuthenticator = routerAPIKeyAuthenticator{permissions: []string{"read:controls"}}
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		path       string
		withKey    bool
		wantStatus int
	}{
		{name: "granted resource", path: "/api/v1/automation/controls", withKey: true, wantStatus: http.StatusOK},
		{name: "audit requires its exact scope", path: "/api/v1/automation/audits", withKey: true, wantStatus: http.StatusForbidden},
		{name: "missing resource scope", path: "/api/v1/automation/risks", withKey: true, wantStatus: http.StatusForbidden},
		{name: "missing credential", path: "/api/v1/automation/controls", wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.withKey {
				request.Header.Set("X-API-Key", "cf_live_test")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s, want %d", response.Code, response.Body.String(), test.wantStatus)
			}
		})
	}
}

func TestIncidentAutomationRoutesAreReadOnly(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.APIKeyAuthenticator = routerAPIKeyAuthenticator{permissions: []string{"read:incidents"}}
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/automation/incidents", "/api/v1/automation/incidents/statistics", "/api/v1/automation/incidents/" + testIncidentID + "/timeline"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-API-Key", "cf_live_test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/automation/incidents", strings.NewReader(`{}`))
	request.Header.Set("X-API-Key", "cf_live_test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("automation incident mutation status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAssetAutomationRoutesAreReadOnly(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.APIKeyAuthenticator = routerAPIKeyAuthenticator{permissions: []string{"read:assets"}}
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/automation/assets",
		"/api/v1/automation/assets/stats",
		"/api/v1/automation/assets/" + testAssetID,
		"/api/v1/automation/assets/" + testAssetID + "/events",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-API-Key", "cf_live_test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/automation/assets", strings.NewReader(`{}`))
	request.Header.Set("X-API-Key", "cf_live_test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("automation asset mutation status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestVendorAutomationRoutesAreReadOnly(t *testing.T) {
	dependencies := testRouterDependencies()
	dependencies.APIKeyAuthenticator = routerAPIKeyAuthenticator{permissions: []string{"read:vendors"}}
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/automation/vendors",
		"/api/v1/automation/vendors/statistics",
		"/api/v1/automation/vendors/due-for-assessment",
		"/api/v1/automation/vendors/" + testVendorID,
		"/api/v1/automation/vendors/" + testVendorID + "/timeline",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-API-Key", "cf_live_test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/automation/vendors", strings.NewReader(`{}`))
	request.Header.Set("X-API-Key", "cf_live_test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("automation vendor mutation status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRouterHealthEndpoints(t *testing.T) {
	dependencies := testRouterDependencies()
	router, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatalf("NewRouterWithDependencies() error = %v", err)
	}

	for _, path := range []string{"/health", "/health/live", "/health/ready"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}

	dependencies.HealthCheck = func(context.Context) error { return errors.New("database unavailable") }
	unreadyRouter, err := NewRouterWithDependencies(testRouterConfig(), dependencies)
	if err != nil {
		t.Fatalf("NewRouterWithDependencies() error = %v", err)
	}
	response := httptest.NewRecorder()
	unreadyRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func testRouterDependencies() RouterDependencies {
	dummyPool := new(pgxpool.Pool)
	notificationProtector, err := secretbox.NewHex(strings.Repeat("ab", 32))
	if err != nil {
		panic(err)
	}
	notificationEngine := service.NewNotificationEngineWithProtector(
		dummyPool, service.NewEventBus(), routerEmailSender{}, notificationProtector,
	)
	integrationService, err := service.NewIntegrationService(dummyPool, strings.Repeat("cd", 32))
	if err != nil {
		panic(err)
	}
	user := &models.User{
		TenantModel: models.TenantModel{
			BaseModel:      models.BaseModel{ID: testUserID},
			OrganizationID: testOrgID,
		},
		Email:     "user@example.com",
		FirstName: "A",
		LastName:  "User",
		Status:    models.UserStatusActive,
		Role:      models.UserRoleViewer,
		Language:  "en",
	}
	organization := &models.Organization{
		BaseModel: models.BaseModel{ID: testOrgID},
		Name:      "Example",
		Slug:      "example",
		Status:    "active",
		Tier:      "starter",
	}
	risk := &models.Risk{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: "80000000-0000-0000-0000-000000000010"}, OrganizationID: testOrgID},
		RiskRef:     "RSK-0001", Title: "Availability", Status: models.RiskStatusIdentified,
	}
	policy := &models.Policy{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testPolicyID}, OrganizationID: testOrgID},
		PolicyRef:   "POL-0001", Title: "Security policy", Status: models.PolicyStateDraft,
	}
	audit := &models.Audit{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testAuditID}, OrganizationID: testOrgID},
		AuditRef:    "AUD-0001", Title: "Annual audit", Type: models.AuditTypeInternal, Status: models.AuditStatusPlanned,
	}
	incident := &models.Incident{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testIncidentID}, OrganizationID: testOrgID},
		IncidentRef: "INC-000001", Title: "Database exposure", Description: "Production snapshot exposed",
		Category: "privacy", Severity: models.IncidentSeverityHigh, Status: models.IncidentStatusReported,
		ReporterID: testUserID, Version: 1,
	}
	asset := &models.Asset{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testAssetID}, OrganizationID: testOrgID},
		AssetRef:    "AST-000001", Name: "Customer database", AssetType: models.AssetTypeData,
		Criticality: models.AssetCriticalityCritical, Classification: models.AssetClassificationRestricted,
		Status: models.AssetStatusActive, Version: 1, CreatedBy: testUserID, Tags: []string{},
	}
	vendor := &models.Vendor{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testVendorID}, OrganizationID: testOrgID},
		VendorRef:   "VND-000001", Name: "Nimbus Hosting", Status: models.VendorStatusActive,
		Criticality: models.VendorCriticalityHigh, VendorTier: models.VendorTierOne,
		RiskTier: models.VendorRiskHigh, Version: 1, CreatedBy: testUserID,
		Services: []string{"hosting"}, DataCategories: []string{}, ProcessingLocations: []string{}, Certifications: []string{},
	}
	roleOrganizationID := testOrgID
	managedRole := &models.ManagedRole{
		ID: testRoleID, OrganizationID: &roleOrganizationID, Name: "Control reviewer", Slug: "control-reviewer",
		IsCustom: true, Version: 1, Permissions: []models.PermissionGrant{},
	}
	directoryUser := &models.DirectoryUser{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testUserID}, OrganizationID: testOrgID},
		Email:       "user@example.com", FirstName: "A", LastName: "User", Status: models.UserStatusActive,
		InvitationStatus: models.DirectoryInvitationNotRequired, Language: "en", Version: 1, RoleSlugs: []string{"viewer"},
	}
	directoryGroup := &models.DirectoryGroup{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: testGroupID}, OrganizationID: testOrgID},
		Name:        "Reviewers", Slug: "reviewers", GroupType: models.DirectoryGroupStatic, Version: 1,
		CreatedBy: testUserID, UpdatedBy: testUserID,
	}
	dataGovernancePolicy := &models.DataGovernancePolicy{
		OrganizationID: testOrgID, PrimaryRegion: "eu-west-1", AllowedRegions: []string{"eu-west-1"},
		CrossBorderTransferMode: "approved_regions", DefaultRetentionDays: 365,
		DeletionGraceDays: 30, DispositionApprovalMode: "single", LegalHoldEnabled: true,
		Version: 1, CreatedBy: testUserID, UpdatedBy: testUserID,
	}
	return RouterDependencies{
		Auth:          handler.NewAuthHandler(&routerAuthService{user: user}),
		Identity:      handler.NewIdentityLifecycleHandler(routerIdentityService{}),
		SCIM:          handler.NewSCIMHandler(routerSCIMService{}),
		Organizations: handler.NewOrganizationHandler(&routerOrganizationService{organization: organization}),
		Frameworks:    handler.NewFrameworkHandler(routerComplianceService{}),
		Controls: handler.NewControlHandler(
			routerComplianceService{},
			handler.WithEvidenceObjectService(routerEvidenceObjectService{}, 1<<20),
			handler.WithEvidenceLifecycleService(routerEvidenceLifecycleService{}),
		),
		Risks:                handler.NewRiskHandler(routerRiskService{risk: risk}),
		Policies:             handler.NewPolicyHandler(routerPolicyService{policy: policy}),
		Audits:               handler.NewAuditHandler(routerAuditService{audit: audit}),
		Incidents:            handler.NewIncidentHandler(routerIncidentService{incident: incident}),
		Assets:               handler.NewAssetHandler(routerAssetService{asset: asset}),
		Vendors:              handler.NewVendorHandler(routerVendorService{vendor: vendor}),
		AccessAdministration: handler.NewAccessAdministrationHandler(routerAccessAdministrationService{role: managedRole}),
		AccessGovernance:     handler.NewAccessGovernanceHandler(routerAccessGovernanceService{}),
		UserAdministration:   handler.NewUserAdministrationHandler(routerUserAdministrationService{user: directoryUser, group: directoryGroup}),
		FeatureFlags:         handler.NewFeatureFlagHandler(routerFeatureFlagService{}),
		DataGovernance:       handler.NewDataGovernanceHandler(routerDataGovernanceService{policy: dataGovernancePolicy}),
		Diagnostics:          handler.NewDiagnosticsHandler(routerDiagnosticsService{}, handler.WithSupportBundleService(&routerSupportBundleService{})),
		DataQuality:          handler.NewDataQualityHandler(routerDataQualityService{}),
		OrganizationProfile:  handler.NewOrganizationProfileHandler(routerOrganizationProfileService{}),
		CalendarRead:         handler.NewCalendarReadHandler(routerCalendarReadService{}),
		Permissions:          handler.NewPermissionHandler(routerPermissionService{}),
		Notifications:        handler.NewNotificationHandler(dummyPool, notificationEngine, notificationProtector),
		Integrations:         handler.NewIntegrationHandler(integrationService),
		APIKeyAuthenticator:  routerAPIKeyAuthenticator{permissions: []string{"read:controls"}},
		SCIMAuthenticator:    routerSCIMAuthenticator{scopes: []string{models.SCIMTokenScopeUsersRead}},
		APIKeyRateLimiter:    routerAPIKeyLimiter{},
		RequestRateLimiter:   routerAPIKeyLimiter{},
		AccessTokenValidator: routerTokenValidator{claims: &authdomain.Claims{
			UserID:         testUserID,
			OrganizationID: testOrgID,
			Role:           string(models.UserRoleViewer),
			Email:          user.Email,
			TokenType:      authdomain.TokenTypeAccess,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: testUserID,
			},
		}},
		Authorizer:           &routerAuthorizer{allowed: true},
		FeatureEvaluator:     routerFeatureFlagService{},
		EntitlementChecker:   routerFeatureFlagService{},
		EvidenceScannerCheck: func(context.Context) error { return nil },
		HealthCheck:          func(context.Context) error { return nil },
		TenantMiddleware: func(next http.Handler) http.Handler {
			return next
		},
		Domains: DomainHandlers{
			Access: handler.NewAccessHandler(routerPolicyAccessService{}, &routerAuthorizer{allowed: true}),
		},
	}
}

func testRouterConfig() *config.Config {
	return &config.Config{
		CORS:      config.CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}},
		RateLimit: config.RateLimitConfig{RPS: 10_000},
	}
}

func routerTokenPair(user *models.User) *authdomain.TokenPair {
	return &authdomain.TokenPair{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		User:         user,
	}
}
