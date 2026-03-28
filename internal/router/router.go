package router

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/handler"
	"github.com/complianceforge/platform/internal/middleware"
)

// NewRouter creates the Chi router with all middleware, route groups, and handler
// bindings. It accepts the database pool and application config, wires up
// repositories -> services -> handlers, and returns the fully configured router.
func NewRouter(pool *pgxpool.Pool, cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	// --- Global middleware chain ---
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.LoggingMiddleware)
	r.Use(middleware.CORSMiddleware(cfg.CORS.AllowedOrigins))
	r.Use(middleware.RateLimitMiddleware(cfg.RateLimit.RPS))
	r.Use(chimw.Recoverer)

	// --- Health check (no auth required) ---
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// --- Create handlers ---
	// In a production codebase the repositories and services would be
	// instantiated here and injected into each handler constructor.
	// For now we accept the service interfaces defined in the handler package.
	// The concrete wiring will be added once service implementations exist.

	var (
		authHandler         *handler.AuthHandler
		organizationHandler *handler.OrganizationHandler
		frameworkHandler    *handler.FrameworkHandler
		controlHandler      *handler.ControlHandler
		riskHandler         *handler.RiskHandler
		policyHandler       *handler.PolicyHandler
		auditHandler        *handler.AuditHandler
		incidentHandler     *handler.IncidentHandler
		vendorHandler       *handler.VendorHandler
		dashboardHandler    *handler.DashboardHandler
		reportHandler       *handler.ReportHandler
	)

	// Wire repositories, services, and handlers when implementations are available.
	// Example (uncomment when service layer is implemented):
	//
	// orgRepo := repository.NewOrganizationRepository(pool)
	// orgService := service.NewOrganizationService(orgRepo)
	// organizationHandler = handler.NewOrganizationHandler(orgService)
	//
	// authService := service.NewAuthService(pool, cfg)
	// authHandler = handler.NewAuthHandler(authService)
	//
	// ... etc for each domain

	_ = pool // used by repositories once wired

	// --- Public routes (no authentication required) ---
	r.Route("/api/v1/auth", func(r chi.Router) {
		if authHandler != nil {
			r.Post("/login", authHandler.Login)
			r.Post("/register", authHandler.Register)
			r.Post("/refresh", authHandler.Refresh)
		}
	})

	// --- Protected routes (authentication + tenant middleware) ---
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
		r.Use(middleware.TenantMiddleware(pool))

		// Organizations
		r.Route("/organizations", func(r chi.Router) {
			if organizationHandler != nil {
				r.Post("/", organizationHandler.Create)
				r.Get("/", organizationHandler.List)
				r.Get("/{id}", organizationHandler.GetByID)
				r.Put("/{id}", organizationHandler.Update)
				r.Delete("/{id}", organizationHandler.Delete)
			}
		})

		// Compliance Frameworks
		r.Route("/frameworks", func(r chi.Router) {
			if frameworkHandler != nil {
				r.Post("/", frameworkHandler.Create)
				r.Get("/", frameworkHandler.List)
				r.Post("/import", frameworkHandler.Import)
				r.Get("/{id}", frameworkHandler.GetByID)
				r.Put("/{id}", frameworkHandler.Update)
				r.Delete("/{id}", frameworkHandler.Delete)
				r.Get("/{id}/controls", frameworkHandler.GetControls)
			}
		})

		// Controls
		r.Route("/controls", func(r chi.Router) {
			if controlHandler != nil {
				r.Post("/", controlHandler.Create)
				r.Get("/", controlHandler.List)
				r.Post("/bulk", controlHandler.BulkCreate)
				r.Get("/{id}", controlHandler.GetByID)
				r.Put("/{id}", controlHandler.Update)
				r.Delete("/{id}", controlHandler.Delete)
				r.Put("/{id}/status", controlHandler.UpdateStatus)
			}
		})

		// Risks
		r.Route("/risks", func(r chi.Router) {
			if riskHandler != nil {
				r.Post("/", riskHandler.Create)
				r.Get("/", riskHandler.List)
				r.Get("/matrix", riskHandler.GetMatrix)
				r.Get("/heatmap", riskHandler.GetHeatmap)
				r.Get("/{id}", riskHandler.GetByID)
				r.Put("/{id}", riskHandler.Update)
				r.Delete("/{id}", riskHandler.Delete)
			}
		})

		// Policies
		r.Route("/policies", func(r chi.Router) {
			if policyHandler != nil {
				r.Post("/", policyHandler.Create)
				r.Get("/", policyHandler.List)
				r.Get("/due-for-review", policyHandler.GetDueForReview)
				r.Get("/{id}", policyHandler.GetByID)
				r.Put("/{id}", policyHandler.Update)
				r.Delete("/{id}", policyHandler.Delete)
				r.Put("/{id}/submit-review", policyHandler.SubmitForReview)
				r.Put("/{id}/approve", policyHandler.Approve)
				r.Put("/{id}/publish", policyHandler.Publish)
			}
		})

		// Audits
		r.Route("/audits", func(r chi.Router) {
			if auditHandler != nil {
				r.Post("/", auditHandler.Create)
				r.Get("/", auditHandler.List)
				r.Get("/{id}", auditHandler.GetByID)
				r.Put("/{id}", auditHandler.Update)
				r.Delete("/{id}", auditHandler.Delete)
				r.Post("/{id}/findings", auditHandler.CreateFinding)
				r.Get("/{id}/findings", auditHandler.GetFindings)
				r.Put("/{id}/start", auditHandler.Start)
				r.Put("/{id}/complete", auditHandler.Complete)
			}
		})

		// Incidents
		r.Route("/incidents", func(r chi.Router) {
			if incidentHandler != nil {
				r.Post("/", incidentHandler.Create)
				r.Get("/", incidentHandler.List)
				r.Get("/breach-notifiable", incidentHandler.GetBreachNotifiable)
				r.Get("/{id}", incidentHandler.GetByID)
				r.Put("/{id}", incidentHandler.Update)
				r.Delete("/{id}", incidentHandler.Delete)
				r.Put("/{id}/status", incidentHandler.UpdateStatus)
				r.Put("/{id}/escalate", incidentHandler.Escalate)
			}
		})

		// Vendors
		r.Route("/vendors", func(r chi.Router) {
			if vendorHandler != nil {
				r.Post("/", vendorHandler.Create)
				r.Get("/", vendorHandler.List)
				r.Get("/due-for-assessment", vendorHandler.GetDueForAssessment)
				r.Get("/{id}", vendorHandler.GetByID)
				r.Put("/{id}", vendorHandler.Update)
				r.Delete("/{id}", vendorHandler.Delete)
				r.Post("/{id}/assess", vendorHandler.Assess)
			}
		})

		// Dashboard
		r.Route("/dashboard", func(r chi.Router) {
			if dashboardHandler != nil {
				r.Get("/", dashboardHandler.GetDashboard)
				r.Get("/compliance-score/{frameworkID}", dashboardHandler.GetComplianceScore)
			}
		})

		// Reports
		r.Route("/reports", func(r chi.Router) {
			if reportHandler != nil {
				r.Get("/compliance", reportHandler.GetComplianceReport)
				r.Get("/risk", reportHandler.GetRiskReport)
				r.Get("/audit", reportHandler.GetAuditReport)
				r.Get("/executive-summary", reportHandler.GetExecutiveSummary)
			}
		})
	})

	return r
}
