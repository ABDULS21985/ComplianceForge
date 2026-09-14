package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/middleware"
	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/repository"
	"github.com/complianceforge/platform/internal/service"
)

// RouterDependencies contains the release-critical vertical slice. These
// dependencies are mandatory and validated before any HTTP server is built.
// DomainHandlers are explicitly optional until each remaining module has a
// schema-aligned composition path of its own.
type RouterDependencies struct {
	Auth                 *handler.AuthHandler
	Organizations        *handler.OrganizationHandler
	Frameworks           *handler.FrameworkHandler
	Controls             *handler.ControlHandler
	Risks                *handler.RiskHandler
	Policies             *handler.PolicyHandler
	Notifications        *handler.NotificationHandler
	Integrations         *handler.IntegrationHandler
	AccessTokenValidator middleware.AccessTokenValidator
	Authorizer           authz.Authorizer
	HealthCheck          func(context.Context) error
	TenantMiddleware     func(http.Handler) http.Handler
	Domains              DomainHandlers
}

// DomainHandlers records modules that are not yet part of the required P0
// composition slice. Keeping them in the dependency object makes their
// disabled state explicit instead of creating hidden nil handlers in NewRouter.
type DomainHandlers struct {
	Audit            *handler.AuditHandler
	Incident         *handler.IncidentHandler
	Vendor           *handler.VendorHandler
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
	_ service.UserRepository                  = repository.UserRepository(nil)
	_ handler.AuthService                     = (*service.AuthService)(nil)
	_ middleware.AccessTokenValidator         = (*service.AuthService)(nil)
	_ authz.Authorizer                        = (*service.RBACAuthorizer)(nil)
	_ service.OrganizationRepository          = repository.OrganizationRepository(nil)
	_ handler.OrganizationService             = (*service.OrganizationService)(nil)
	_ service.FrameworkCatalogRepository      = repository.FrameworkRepository(nil)
	_ service.ControlImplementationRepository = repository.ControlRepository(nil)
	_ handler.FrameworkService                = (*service.FrameworkService)(nil)
	_ handler.ControlService                  = (*service.FrameworkService)(nil)
	_ service.RiskManagementRepository        = repository.RiskRepository(nil)
	_ service.RiskRepository                  = repository.RiskRepository(nil)
	_ handler.RiskService                     = (*service.RiskService)(nil)
	_ service.PolicyManagementRepository      = repository.PolicyRepository(nil)
	_ handler.PolicyService                   = (*service.PolicyService)(nil)
	_ handler.IntegrationSvc                  = (*service.IntegrationService)(nil)
)

// BuildDependencies composes repositories -> services -> handlers for the
// release-critical API slice.
func BuildDependencies(pool *pgxpool.Pool, cfg *config.Config) (RouterDependencies, error) {
	if pool == nil {
		return RouterDependencies{}, errors.New("database pool is required")
	}
	if cfg == nil {
		return RouterDependencies{}, errors.New("configuration is required")
	}

	var userRepo service.UserRepository = repository.NewUserRepository(pool)
	authService := service.NewAuthService(userRepo, cfg.JWT, log.Logger)

	var orgRepo service.OrganizationRepository = repository.NewOrganizationRepository(pool)
	organizationService := service.NewOrganizationService(orgRepo, log.Logger)
	var frameworkRepo service.FrameworkCatalogRepository = repository.NewFrameworkRepository(pool)
	var controlRepo service.ControlImplementationRepository = repository.NewControlRepository(pool)
	complianceService := service.NewFrameworkService(frameworkRepo, controlRepo, log.Logger)
	var riskRepo service.RiskManagementRepository = repository.NewRiskRepository(pool)
	riskService := service.NewRiskService(riskRepo, log.Logger)
	var policyRepo service.PolicyManagementRepository = repository.NewPolicyRepository(pool)
	policyService := service.NewPolicyService(policyRepo, log.Logger)
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
	authorizer, err := service.NewRBACAuthorizer(pool)
	if err != nil {
		return RouterDependencies{}, fmt.Errorf("building authorization service: %w", err)
	}

	dependencies := RouterDependencies{
		Auth:                 handler.NewAuthHandler(authService),
		Organizations:        handler.NewOrganizationHandler(organizationService),
		Frameworks:           handler.NewFrameworkHandler(complianceService),
		Controls:             handler.NewControlHandler(complianceService),
		Risks:                handler.NewRiskHandler(riskService),
		Policies:             handler.NewPolicyHandler(policyService),
		Notifications:        handler.NewNotificationHandler(pool, notificationEngine, notificationProtector),
		Integrations:         handler.NewIntegrationHandler(integrationService),
		AccessTokenValidator: authService,
		Authorizer:           authorizer,
		HealthCheck: func(ctx context.Context) error {
			return database.HealthCheck(ctx, pool)
		},
		TenantMiddleware: middleware.TenantMiddleware(pool),
	}
	if err := dependencies.Validate(); err != nil {
		return RouterDependencies{}, fmt.Errorf("validating router dependencies: %w", err)
	}
	return dependencies, nil
}

// Validate reports all missing required dependencies in one startup error.
func (d RouterDependencies) Validate() error {
	missing := make([]error, 0, 8)
	if d.Auth == nil || !d.Auth.Ready() {
		missing = append(missing, errors.New("auth handler is required"))
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
	if d.Notifications == nil || !d.Notifications.Ready() {
		missing = append(missing, errors.New("notification handler is required"))
	}
	if d.Integrations == nil || !d.Integrations.Ready() {
		missing = append(missing, errors.New("integration handler is required"))
	}
	if d.AccessTokenValidator == nil {
		missing = append(missing, errors.New("access-token validator is required"))
	}
	if interfaceIsNil(d.Authorizer) {
		missing = append(missing, errors.New("authorizer is required"))
	}
	if d.HealthCheck == nil {
		missing = append(missing, errors.New("health check is required"))
	}
	if d.TenantMiddleware == nil {
		missing = append(missing, errors.New("tenant middleware is required"))
	}
	return errors.Join(missing...)
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
