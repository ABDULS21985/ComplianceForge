package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	authdomain "github.com/complianceforge/platform/internal/auth"
	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/middleware"
	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
	evidencepkg "github.com/complianceforge/platform/internal/pkg/evidence"
	queuepkg "github.com/complianceforge/platform/internal/pkg/queue"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	storagepkg "github.com/complianceforge/platform/internal/pkg/storage"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

// RouterDependencies contains the release-critical vertical slice. These
// dependencies are mandatory and validated before any HTTP server is built.
// DomainHandlers are explicitly optional until each remaining module has a
// schema-aligned composition path of its own.
type RouterDependencies struct {
	Auth                 *handler.AuthHandler
	Identity             *handler.IdentityLifecycleHandler
	SCIM                 *handler.SCIMHandler
	Organizations        *handler.OrganizationHandler
	Frameworks           *handler.FrameworkHandler
	Controls             *handler.ControlHandler
	Risks                *handler.RiskHandler
	Policies             *handler.PolicyHandler
	Audits               *handler.AuditHandler
	Incidents            *handler.IncidentHandler
	Assets               *handler.AssetHandler
	Vendors              *handler.VendorHandler
	AccessAdministration *handler.AccessAdministrationHandler
	AccessGovernance     *handler.AccessGovernanceHandler
	UserAdministration   *handler.UserAdministrationHandler
	FeatureFlags         *handler.FeatureFlagHandler
	DataGovernance       *handler.DataGovernanceHandler
	DataQuality          *handler.DataQualityHandler
	OrganizationProfile  *handler.OrganizationProfileHandler
	CalendarRead         *handler.CalendarReadHandler
	Diagnostics          *handler.DiagnosticsHandler
	Permissions          *handler.PermissionHandler
	Notifications        *handler.NotificationHandler
	Integrations         *handler.IntegrationHandler
	APIKeyAuthenticator  authdomain.APIKeyAuthenticator
	SCIMAuthenticator    authdomain.SCIMTokenAuthenticator
	APIKeyRateLimiter    middleware.APIKeyRateLimiter
	RequestRateLimiter   middleware.RequestRateLimiter
	AccessTokenValidator middleware.AccessTokenValidator
	Authorizer           authz.Authorizer
	FeatureEvaluator     middleware.FeatureEvaluator
	EntitlementChecker   middleware.EntitlementLimitChecker
	EvidenceScannerCheck func(context.Context) error
	HealthCheck          func(context.Context) error
	TenantMiddleware     func(http.Handler) http.Handler
	Domains              DomainHandlers
}

// DomainHandlers records modules that are not yet part of the required P0
// composition slice. Keeping them in the dependency object makes their
// disabled state explicit instead of creating hidden nil handlers in NewRouter.
type DomainHandlers struct {
	Dashboard        *handler.DashboardHandler
	Report           *handler.ReportHandler
	DSR              *handler.DSRHandler
	NIS2             *handler.NIS2Handler
	Monitoring       *handler.MonitoringHandler
	Workflow         *handler.WorkflowHandler
	Onboarding       *handler.OnboardingHandler
	Access           *handler.AccessHandler
	Remediation      *handler.RemediationHandler
	Marketplace      *handler.MarketplaceHandler
	Regulatory       *handler.RegulatoryHandler
	BIA              *handler.BIAHandler
	Analytics        *handler.AnalyticsHandler
	Exception        *handler.ExceptionHandler
	EvidenceTemplate *handler.EvidenceTemplateHandler
	Questionnaire    *handler.QuestionnaireHandler
	VendorPortal     *handler.VendorPortalHandler
	ROPA             *handler.ROPAHandler
	Board            *handler.BoardHandler
	BoardPortal      *handler.BoardPortalHandler
	Calendar         *handler.CalendarHandler
	Search           *handler.SearchHandler
	Collaboration    *handler.CollaborationHandler
	Mobile           *handler.MobileHandler
	Branding         *handler.BrandingHandler
}

// Compile-time assertions keep the concrete dependency chain honest.
var (
	_ service.UserRepository                    = repository.UserRepository(nil)
	_ handler.AuthService                       = (*service.AuthService)(nil)
	_ handler.IdentityAuthenticationCompleter   = (*service.AuthService)(nil)
	_ service.IdentityLifecycleStore            = repository.IdentityLifecycleRepository(nil)
	_ handler.IdentityLifecycleService          = (*service.IdentityLifecycleService)(nil)
	_ service.SCIMStore                         = repository.SCIMRepository(nil)
	_ handler.SCIMHandlerService                = (*service.SCIMService)(nil)
	_ authdomain.SCIMTokenAuthenticator         = (*service.SCIMService)(nil)
	_ middleware.AccessTokenValidator           = (*service.AuthService)(nil)
	_ authz.Authorizer                          = (*service.RBACAuthorizer)(nil)
	_ service.OrganizationRepository            = repository.OrganizationRepository(nil)
	_ handler.OrganizationService               = (*service.OrganizationService)(nil)
	_ service.FrameworkCatalogRepository        = repository.FrameworkRepository(nil)
	_ service.ControlImplementationRepository   = repository.ControlRepository(nil)
	_ handler.FrameworkService                  = (*service.FrameworkService)(nil)
	_ handler.ControlService                    = (*service.FrameworkService)(nil)
	_ service.EvidenceObjectRepository          = repository.ControlRepository(nil)
	_ handler.EvidenceObjectService             = (*service.EvidenceObjectService)(nil)
	_ service.EvidenceLifecycleStore            = repository.EvidenceLifecycleRepository(nil)
	_ handler.EvidenceLifecycleService          = (*service.EvidenceLifecycleService)(nil)
	_ service.RiskManagementRepository          = repository.RiskRepository(nil)
	_ service.RiskRepository                    = repository.RiskRepository(nil)
	_ handler.RiskService                       = (*service.RiskService)(nil)
	_ service.PolicyManagementRepository        = repository.PolicyRepository(nil)
	_ handler.PolicyService                     = (*service.PolicyService)(nil)
	_ service.AuditManagementRepository         = repository.AuditRepository(nil)
	_ handler.AuditService                      = (*service.AuditService)(nil)
	_ service.IncidentManagementRepository      = repository.IncidentRepository(nil)
	_ handler.IncidentService                   = (*service.IncidentService)(nil)
	_ service.AssetManagementRepository         = repository.AssetRepository(nil)
	_ handler.AssetManagementService            = (*service.AssetService)(nil)
	_ service.VendorManagementRepository        = repository.VendorRepository(nil)
	_ handler.VendorService                     = (*service.VendorService)(nil)
	_ service.AccessAdministrationStore         = repository.AccessAdministrationRepository(nil)
	_ handler.AccessAdministrationService       = (*service.AccessAdministrationService)(nil)
	_ handler.AccessGovernanceHandlerService    = (*service.AccessGovernanceService)(nil)
	_ service.UserAdministrationStore           = repository.UserAdministrationRepository(nil)
	_ handler.UserAdministrationService         = (*service.UserAdministrationService)(nil)
	_ service.FeatureFlagStore                  = repository.FeatureFlagRepository(nil)
	_ handler.FeatureFlagService                = (*service.FeatureFlagService)(nil)
	_ service.DataGovernanceStore               = repository.DataGovernanceRepository(nil)
	_ handler.DataGovernanceService             = (*service.DataGovernanceService)(nil)
	_ service.DiagnosticsStore                  = repository.DiagnosticsRepository(nil)
	_ handler.DiagnosticsService                = (*service.DiagnosticsService)(nil)
	_ service.OrganizationProfileStore          = repository.OrganizationProfileRepository(nil)
	_ handler.OrganizationProfileHandlerService = (*service.OrganizationProfileService)(nil)
	_ service.DataQualityStore                  = repository.DataQualityRepository(nil)
	_ handler.DataQualityService                = (*service.DataQualityService)(nil)
	_ handler.CalendarReadService               = (*service.ComplianceCalendarService)(nil)
	_ service.PolicyAccessStore                 = repository.PolicyAccessRepository(nil)
	_ handler.PolicyAccessHandlerService        = (*service.PolicyAccessService)(nil)
	_ handler.PermissionService                 = (*service.RBACAuthorizer)(nil)
	_ authz.Authorizer                          = (*service.PolicyBasedAuthorizer)(nil)
	_ handler.IntegrationSvc                    = (*service.IntegrationService)(nil)
)

// BuildDependencies composes repositories -> services -> handlers for the
// release-critical API slice.
func BuildDependencies(
	pool *pgxpool.Pool,
	cfg *config.Config,
	apiKeyLimiters ...middleware.APIKeyRateLimiter,
) (RouterDependencies, error) {
	if pool == nil {
		return RouterDependencies{}, errors.New("database pool is required")
	}
	if cfg == nil {
		return RouterDependencies{}, errors.New("configuration is required")
	}

	var userRepo service.UserRepository = repository.NewUserRepository(pool)

	var orgRepo service.OrganizationRepository = repository.NewOrganizationRepository(pool)
	organizationService := service.NewOrganizationService(orgRepo, log.Logger)
	var frameworkRepo service.FrameworkCatalogRepository = repository.NewFrameworkRepository(pool)
	controlRepository := repository.NewControlRepository(pool)
	var controlRepo service.ControlImplementationRepository = controlRepository
	complianceService := service.NewFrameworkService(frameworkRepo, controlRepo, log.Logger)
	evidenceStorage, err := storagepkg.NewConfiguredService(context.Background(), cfg.Storage)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building evidence object storage: %w", err)
	}
	evidenceScanner, err := evidencepkg.NewClamAVScanner(
		cfg.Evidence.ScannerNetwork,
		cfg.Evidence.ScannerAddress,
		time.Duration(cfg.Evidence.ScannerTimeoutSeconds)*time.Second,
	)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building evidence malware scanner: %w", err)
	}
	evidencePipeline, err := evidencepkg.NewPipeline(
		evidenceStorage,
		evidenceScanner,
		evidencepkg.WithMaximumSize(cfg.Evidence.MaximumUploadBytes),
	)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building evidence object pipeline: %w", err)
	}
	evidenceObjectService, err := service.NewEvidenceObjectService(
		controlRepository,
		evidencePipeline,
		evidenceStorage,
		time.Duration(cfg.Evidence.SignedDownloadSeconds)*time.Second,
		log.Logger,
	)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building evidence object service: %w", err)
	}
	evidenceLifecycleRepository, err := repository.NewEvidenceLifecycleRepository(pool)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building evidence lifecycle repository: %w", err)
	}
	evidenceLifecycleService, err := service.NewEvidenceLifecycleService(
		evidenceLifecycleRepository, evidenceStorage, log.Logger,
	)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building evidence lifecycle service: %w", err)
	}
	var riskRepo service.RiskManagementRepository = repository.NewRiskRepository(pool)
	riskService := service.NewRiskService(riskRepo, log.Logger)
	var policyRepo service.PolicyManagementRepository = repository.NewPolicyRepository(pool)
	policyService := service.NewPolicyService(policyRepo, log.Logger)
	var auditRepo service.AuditManagementRepository = repository.NewAuditRepository(pool)
	auditService := service.NewAuditService(auditRepo, log.Logger)
	outboxConfig, err := queuepkg.OutboxConfigFromEnvironment()
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("loading domain outbox configuration: %w", err)
	}
	domainOutbox, err := queuepkg.NewPostgresOutbox(pool, uuid.NewString(), outboxConfig)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building domain outbox: %w", err)
	}
	incidentQueue := strings.TrimSpace(os.Getenv("WORKER_QUEUE_NAME"))
	if incidentQueue == "" {
		incidentQueue = "complianceforge.worker"
	}
	var incidentRepo service.IncidentManagementRepository
	incidentRepo, err = repository.NewIncidentRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building incident repository: %w", err)
	}
	incidentService := service.NewIncidentService(incidentRepo, log.Logger)
	var assetRepo service.AssetManagementRepository
	assetRepo, err = repository.NewAssetRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building asset repository: %w", err)
	}
	assetService := service.NewAssetService(assetRepo, log.Logger)
	var vendorRepo service.VendorManagementRepository
	vendorRepo, err = repository.NewVendorRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building vendor repository: %w", err)
	}
	vendorService := service.NewVendorService(vendorRepo, log.Logger)
	var accessAdministrationRepo service.AccessAdministrationStore
	accessAdministrationRepo, err = repository.NewAccessAdministrationRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building access administration repository: %w", err)
	}
	accessAdministrationService := service.NewAccessAdministrationService(accessAdministrationRepo, log.Logger)
	accessGovernanceRepo, err := repository.NewAccessGovernanceRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building access governance repository: %w", err)
	}
	accessGovernanceService, err := service.NewAccessGovernanceService(accessGovernanceRepo, nil)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building access governance service: %w", err)
	}
	var userAdministrationRepo service.UserAdministrationStore
	userAdministrationRepo, err = repository.NewUserAdministrationRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building user administration repository: %w", err)
	}
	userAdministrationService := service.NewUserAdministrationService(userAdministrationRepo, log.Logger)
	var featureFlagRepo service.FeatureFlagStore
	featureFlagRepo, err = repository.NewFeatureFlagRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building feature flag repository: %w", err)
	}
	featureFlagService := service.NewFeatureFlagService(featureFlagRepo, log.Logger)
	var dataGovernanceRepo service.DataGovernanceStore
	dataGovernanceRepo, err = repository.NewDataGovernanceRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building data governance repository: %w", err)
	}
	dataGovernanceService := service.NewDataGovernanceService(dataGovernanceRepo, log.Logger)
	integrationService, err := service.NewIntegrationService(pool, cfg.Encryption.IntegrationKey)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building integration service: %w", err)
	}
	emailSender, err := emailpkg.NewSMTPEmailService(emailpkg.Config{
		Host:       cfg.SMTP.Host,
		Port:       cfg.SMTP.Port,
		Username:   cfg.SMTP.User,
		Password:   cfg.SMTP.Password,
		From:       cfg.SMTP.From,
		TLSMode:    cfg.SMTP.TLSMode,
		Timeout:    time.Duration(cfg.SMTP.TimeoutSeconds) * time.Second,
		HelloName:  cfg.SMTP.HelloName,
		ServerName: cfg.SMTP.ServerName,
	})
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building notification email transport: %w", err)
	}
	notificationProtector, err := secretbox.NewHex(cfg.Encryption.NotificationKey)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building notification secret protector: %w", err)
	}
	notificationEngine := service.NewNotificationEngineWithProtector(
		pool, service.NewEventBus(), emailSender, notificationProtector,
	)
	var apiKeyLimiter middleware.APIKeyRateLimiter = unavailableAPIKeyLimiter{}
	if len(apiKeyLimiters) > 0 && !interfaceIsNil(apiKeyLimiters[0]) {
		apiKeyLimiter = apiKeyLimiters[0]
	}
	var requestLimiter middleware.RequestRateLimiter = unavailableAPIKeyLimiter{}
	if len(apiKeyLimiters) > 1 && !interfaceIsNil(apiKeyLimiters[1]) {
		requestLimiter = apiKeyLimiters[1]
	}
	identityRepo, err := repository.NewIdentityLifecycleRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building identity lifecycle repository: %w", err)
	}
	identityProtector, err := secretbox.NewHex(cfg.Encryption.IdentityKey)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building identity lifecycle secret protector: %w", err)
	}
	identityVerifier, err := service.NewIdentityWebAuthnVerifier(cfg.Identity)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building identity WebAuthn verifier: %w", err)
	}
	identityService, err := service.NewIdentityLifecycleService(
		identityRepo, identityProtector, identityVerifier, requestLimiter, log.Logger,
	)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building identity lifecycle service: %w", err)
	}
	scimRepo, err := repository.NewSCIMRepository(pool, domainOutbox, incidentQueue)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building SCIM repository: %w", err)
	}
	scimService, err := service.NewSCIMService(scimRepo, log.Logger)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building SCIM service: %w", err)
	}
	authService := service.NewAuthService(userRepo, cfg.JWT, log.Logger, identityService)
	rbacAuthorizer, err := service.NewRBACAuthorizer(pool)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building authorization service: %w", err)
	}
	policyAccessRepo, err := repository.NewPolicyAccessRepository(pool)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building policy access repository: %w", err)
	}
	policyAccessService, err := service.NewPolicyAccessService(policyAccessRepo, log.Logger)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building policy access service: %w", err)
	}
	policyAuthorizer, err := service.NewPolicyBasedAuthorizer(rbacAuthorizer, policyAccessRepo, nil)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building composite authorization service: %w", err)
	}
	calendarReadRepo, err := repository.NewCalendarReadRepository(pool)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building calendar read repository: %w", err)
	}
	calendarReadService, err := service.NewComplianceCalendarService(calendarReadRepo, policyAuthorizer)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building calendar read service: %w", err)
	}
	organizationProfileRepo, err := repository.NewOrganizationProfileRepository(pool)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building organization profile repository: %w", err)
	}
	organizationProfileService, err := service.NewOrganizationProfileService(organizationProfileRepo)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building organization profile service: %w", err)
	}
	dataQualityService, err := service.NewDataQualityService(repository.NewDataQualityRepository())
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building data quality service: %w", err)
	}
	diagnosticsHandler, err := BuildDiagnosticsHandler(pool, cfg, service.DependencyProbe{
		Key: "postgres", Name: "PostgreSQL", Critical: true,
		Check: func(ctx context.Context) error { return database.HealthCheck(ctx, pool) },
	}, service.DependencyProbe{
		Key: "evidence_scanner", Name: "Evidence malware scanner", Critical: false,
		Check: evidenceScanner.HealthCheck,
	})
	if err != nil {
		return RouterDependencies{}, err
	}

	dependencies := RouterDependencies{
		Auth:          handler.NewAuthHandler(authService),
		Identity:      handler.NewIdentityLifecycleHandler(identityService),
		SCIM:          handler.NewSCIMHandler(scimService),
		Organizations: handler.NewOrganizationHandler(organizationService),
		Frameworks:    handler.NewFrameworkHandler(complianceService),
		Controls: handler.NewControlHandler(
			complianceService,
			handler.WithEvidenceObjectService(evidenceObjectService, cfg.Evidence.MaximumUploadBytes),
			handler.WithEvidenceLifecycleService(evidenceLifecycleService),
		),
		Risks:                handler.NewRiskHandler(riskService),
		Policies:             handler.NewPolicyHandler(policyService),
		Audits:               handler.NewAuditHandler(auditService),
		Incidents:            handler.NewIncidentHandler(incidentService),
		Assets:               handler.NewAssetHandler(assetService),
		Vendors:              handler.NewVendorHandler(vendorService),
		AccessAdministration: handler.NewAccessAdministrationHandler(accessAdministrationService),
		AccessGovernance:     handler.NewAccessGovernanceHandler(accessGovernanceService),
		UserAdministration:   handler.NewUserAdministrationHandler(userAdministrationService),
		FeatureFlags:         handler.NewFeatureFlagHandler(featureFlagService),
		DataGovernance:       handler.NewDataGovernanceHandler(dataGovernanceService),
		DataQuality:          handler.NewDataQualityHandler(dataQualityService),
		OrganizationProfile:  handler.NewOrganizationProfileHandler(organizationProfileService),
		CalendarRead:         handler.NewCalendarReadHandler(calendarReadService),
		Diagnostics:          diagnosticsHandler,
		Permissions:          handler.NewPermissionHandler(rbacAuthorizer),
		Notifications:        handler.NewNotificationHandler(pool, notificationEngine, notificationProtector),
		Integrations:         handler.NewIntegrationHandler(integrationService),
		APIKeyAuthenticator:  integrationService,
		SCIMAuthenticator:    scimService,
		APIKeyRateLimiter:    apiKeyLimiter,
		RequestRateLimiter:   requestLimiter,
		AccessTokenValidator: authService,
		Authorizer:           policyAuthorizer,
		FeatureEvaluator:     featureFlagService,
		EntitlementChecker:   featureFlagService,
		EvidenceScannerCheck: evidenceScanner.HealthCheck,
		HealthCheck: func(ctx context.Context) error {
			return database.HealthCheck(ctx, pool)
		},
		TenantMiddleware: middleware.TenantMiddleware(pool),
		Domains: DomainHandlers{
			Access: handler.NewAccessHandler(policyAccessService, policyAuthorizer),
		},
	}
	if err := dependencies.Validate(); err != nil {
		return RouterDependencies{}, fmt.Errorf("validating router dependencies: %w", err)
	}
	return dependencies, nil
}

// BuildDiagnosticsHandler composes the safe administrator diagnostics slice.
// Production can provide instrumented checks for every live dependency while
// focused router construction retains a PostgreSQL-only default.
func BuildDiagnosticsHandler(
	pool *pgxpool.Pool, cfg *config.Config, probes ...service.DependencyProbe,
) (*handler.DiagnosticsHandler, error) {
	diagnosticsRepo, err := repository.NewDiagnosticsRepository(pool)
	if err != nil {
		return nil, fmt.Errorf("building diagnostics repository: %w", err)
	}
	diagnosticsService, err := service.NewDiagnosticsService(diagnosticsRepo, cfg, probes...)
	if err != nil {
		return nil, fmt.Errorf("building diagnostics service: %w", err)
	}
	supportBundleRepo, err := repository.NewSupportBundleRepository(pool)
	if err != nil {
		return nil, fmt.Errorf("building support bundle consent audit: %w", err)
	}
	supportBundleService, err := service.NewSupportBundleService(diagnosticsService, supportBundleRepo, cfg)
	if err != nil {
		return nil, fmt.Errorf("building support bundle service: %w", err)
	}
	return handler.NewDiagnosticsHandler(diagnosticsService, handler.WithSupportBundleService(supportBundleService)), nil
}

// Validate reports all missing required dependencies in one startup error.
func (d RouterDependencies) Validate() error {
	missing := make([]error, 0, 20)
	if d.Auth == nil || !d.Auth.Ready() {
		missing = append(missing, errors.New("auth handler is required"))
	}
	if d.Auth == nil || !d.Auth.IdentityReady() {
		missing = append(missing, errors.New("identity authentication completion is required"))
	}
	if d.Identity == nil || !d.Identity.Ready() {
		missing = append(missing, errors.New("identity lifecycle handler is required"))
	}
	if d.SCIM == nil || !d.SCIM.Ready() {
		missing = append(missing, errors.New("SCIM handler is required"))
	}
	if d.Organizations == nil || !d.Organizations.Ready() {
		missing = append(missing, errors.New("organization handler is required"))
	}
	if d.Frameworks == nil || !d.Frameworks.Ready() {
		missing = append(missing, errors.New("framework handler is required"))
	}
	if d.Controls == nil || !d.Controls.Ready() {
		missing = append(missing, errors.New("control handler is required"))
	}
	if d.Risks == nil || !d.Risks.Ready() {
		missing = append(missing, errors.New("risk handler is required"))
	}
	if d.Policies == nil || !d.Policies.Ready() {
		missing = append(missing, errors.New("policy handler is required"))
	}
	if d.Audits == nil || !d.Audits.Ready() {
		missing = append(missing, errors.New("audit handler is required"))
	}
	if d.Incidents == nil || !d.Incidents.Ready() {
		missing = append(missing, errors.New("incident handler is required"))
	}
	if d.Assets == nil || !d.Assets.Ready() {
		missing = append(missing, errors.New("asset handler is required"))
	}
	if d.Vendors == nil || !d.Vendors.Ready() {
		missing = append(missing, errors.New("vendor handler is required"))
	}
	if d.AccessAdministration == nil || !d.AccessAdministration.Ready() {
		missing = append(missing, errors.New("access administration handler is required"))
	}
	if d.AccessGovernance == nil || !d.AccessGovernance.Ready() {
		missing = append(missing, errors.New("access governance handler is required"))
	}
	if d.UserAdministration == nil || !d.UserAdministration.Ready() {
		missing = append(missing, errors.New("user administration handler is required"))
	}
	if d.FeatureFlags == nil || !d.FeatureFlags.Ready() {
		missing = append(missing, errors.New("feature flag handler is required"))
	}
	if d.DataGovernance == nil || !d.DataGovernance.Ready() {
		missing = append(missing, errors.New("data governance handler is required"))
	}
	if d.Diagnostics == nil || !d.Diagnostics.Ready() {
		missing = append(missing, errors.New("diagnostics handler is required"))
	}
	if d.DataQuality == nil || !d.DataQuality.Ready() {
		missing = append(missing, errors.New("data quality handler is required"))
	}
	if d.OrganizationProfile == nil || !d.OrganizationProfile.Ready() {
		missing = append(missing, errors.New("organization profile handler is required"))
	}
	if d.CalendarRead == nil || !d.CalendarRead.Ready() {
		missing = append(missing, errors.New("calendar read handler is required"))
	}
	if d.Diagnostics == nil || !d.Diagnostics.SupportBundlesReady() {
		missing = append(missing, errors.New("support bundle consent service is required"))
	}
	if d.Permissions == nil || !d.Permissions.Ready() {
		missing = append(missing, errors.New("permission handler is required"))
	}
	if d.Notifications == nil || !d.Notifications.Ready() {
		missing = append(missing, errors.New("notification handler is required"))
	}
	if d.Integrations == nil || !d.Integrations.Ready() {
		missing = append(missing, errors.New("integration handler is required"))
	}
	if interfaceIsNil(d.APIKeyAuthenticator) {
		missing = append(missing, errors.New("API-key authenticator is required"))
	}
	if interfaceIsNil(d.SCIMAuthenticator) {
		missing = append(missing, errors.New("SCIM authenticator is required"))
	}
	if interfaceIsNil(d.APIKeyRateLimiter) {
		missing = append(missing, errors.New("API-key rate limiter is required"))
	}
	if interfaceIsNil(d.RequestRateLimiter) {
		missing = append(missing, errors.New("request rate limiter is required"))
	}
	if d.AccessTokenValidator == nil {
		missing = append(missing, errors.New("access-token validator is required"))
	}
	if interfaceIsNil(d.Authorizer) {
		missing = append(missing, errors.New("authorizer is required"))
	}
	if interfaceIsNil(d.FeatureEvaluator) {
		missing = append(missing, errors.New("feature evaluator is required"))
	}
	if interfaceIsNil(d.EntitlementChecker) {
		missing = append(missing, errors.New("entitlement checker is required"))
	}
	if d.EvidenceScannerCheck == nil {
		missing = append(missing, errors.New("evidence scanner health check is required"))
	}
	if d.HealthCheck == nil {
		missing = append(missing, errors.New("health check is required"))
	}
	if d.TenantMiddleware == nil {
		missing = append(missing, errors.New("tenant middleware is required"))
	}
	return errors.Join(missing...)
}

// unavailableAPIKeyLimiter keeps the legacy NewRouter constructor fail-closed
// for automation requests. Production composition injects the shared Redis
// implementation from cmd/api so the connection can be closed gracefully.
type unavailableAPIKeyLimiter struct{}

func (unavailableAPIKeyLimiter) Allow(context.Context, string, int) (bool, time.Duration, error) {
	return false, 0, errors.New("API-key rate limiter is unavailable")
}

func interfaceIsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
