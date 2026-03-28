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
		notificationHandler *handler.NotificationHandler
		dsrHandler          *handler.DSRHandler
		nis2Handler         *handler.NIS2Handler
		monitoringHandler   *handler.MonitoringHandler
		workflowHandler     *handler.WorkflowHandler
		integrationHandler  *handler.IntegrationHandler
		onboardingHandler   *handler.OnboardingHandler
		accessHandler       *handler.AccessHandler
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
				r.Post("/generate", reportHandler.GenerateReport)
				r.Get("/status/{id}", reportHandler.GetRunStatus)
				r.Get("/download/{id}", reportHandler.DownloadReport)
				r.Get("/definitions", reportHandler.ListDefinitions)
				r.Post("/definitions", reportHandler.CreateDefinition)
				r.Put("/definitions/{id}", reportHandler.UpdateDefinition)
				r.Delete("/definitions/{id}", reportHandler.DeleteDefinition)
				r.Post("/definitions/{id}/generate", reportHandler.GenerateFromDefinition)
				r.Get("/schedules", reportHandler.ListSchedules)
				r.Post("/schedules", reportHandler.CreateSchedule)
				r.Put("/schedules/{id}", reportHandler.UpdateSchedule)
				r.Delete("/schedules/{id}", reportHandler.DeleteSchedule)
				r.Get("/history", reportHandler.ListHistory)
			}
		})

		// Notifications (user-facing)
		r.Route("/notifications", func(r chi.Router) {
			if notificationHandler != nil {
				r.Get("/", notificationHandler.ListNotifications)
				r.Put("/{id}/read", notificationHandler.MarkAsRead)
				r.Put("/read-all", notificationHandler.MarkAllAsRead)
				r.Get("/unread-count", notificationHandler.GetUnreadCount)
				r.Get("/preferences", notificationHandler.GetPreferences)
				r.Put("/preferences", notificationHandler.UpdatePreferences)
			}
		})

		// Notification settings (admin)
		r.Route("/settings/notification-rules", func(r chi.Router) {
			if notificationHandler != nil {
				r.Get("/", notificationHandler.ListRules)
				r.Post("/", notificationHandler.CreateRule)
				r.Put("/{id}", notificationHandler.UpdateRule)
				r.Delete("/{id}", notificationHandler.DeleteRule)
			}
		})
		r.Route("/settings/notification-channels", func(r chi.Router) {
			if notificationHandler != nil {
				r.Get("/", notificationHandler.ListChannels)
				r.Post("/", notificationHandler.CreateChannel)
				r.Post("/{id}/test", notificationHandler.TestChannel)
			}
		})

		// DSR (Data Subject Requests)
		r.Route("/dsr", func(r chi.Router) {
			if dsrHandler != nil {
				r.Get("/", dsrHandler.ListRequests)
				r.Post("/", dsrHandler.CreateRequest)
				r.Get("/dashboard", dsrHandler.GetDashboard)
				r.Get("/overdue", dsrHandler.GetOverdue)
				r.Get("/templates", dsrHandler.ListTemplates)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", dsrHandler.GetRequest)
					r.Put("/", dsrHandler.UpdateRequest)
					r.Post("/verify-identity", dsrHandler.VerifyIdentity)
					r.Post("/assign", dsrHandler.AssignRequest)
					r.Post("/extend", dsrHandler.ExtendDeadline)
					r.Post("/complete", dsrHandler.CompleteRequest)
					r.Post("/reject", dsrHandler.RejectRequest)
					r.Put("/tasks/{taskId}", dsrHandler.UpdateTask)
				})
			}
		})

		// NIS2
		r.Route("/nis2", func(r chi.Router) {
			if nis2Handler != nil {
				r.Get("/assessment", nis2Handler.GetAssessment)
				r.Post("/assessment", nis2Handler.CreateAssessment)
				r.Get("/dashboard", nis2Handler.GetDashboard)
				r.Get("/measures", nis2Handler.GetMeasures)
				r.Put("/measures/{id}", nis2Handler.UpdateMeasure)
				r.Get("/management", nis2Handler.GetManagement)
				r.Post("/management", nis2Handler.RecordTraining)
				r.Route("/incidents", func(r chi.Router) {
					r.Get("/", nis2Handler.ListIncidentReports)
					r.Route("/{id}", func(r chi.Router) {
						r.Get("/", nis2Handler.GetIncidentReport)
						r.Post("/early-warning", nis2Handler.SubmitEarlyWarning)
						r.Post("/notification", nis2Handler.SubmitNotification)
						r.Post("/final-report", nis2Handler.SubmitFinalReport)
					})
				})
			}
		})

		// Monitoring
		r.Route("/monitoring", func(r chi.Router) {
			if monitoringHandler != nil {
				r.Get("/dashboard", monitoringHandler.GetDashboard)
				r.Route("/configs", func(r chi.Router) {
					r.Get("/", monitoringHandler.ListCollectionConfigs)
					r.Post("/", monitoringHandler.CreateCollectionConfig)
					r.Put("/{id}", monitoringHandler.UpdateCollectionConfig)
					r.Post("/{id}/run-now", monitoringHandler.RunCollectionNow)
					r.Get("/{id}/history", monitoringHandler.GetCollectionHistory)
				})
				r.Route("/monitors", func(r chi.Router) {
					r.Get("/", monitoringHandler.ListMonitors)
					r.Post("/", monitoringHandler.CreateMonitor)
					r.Put("/{id}", monitoringHandler.UpdateMonitor)
					r.Get("/{id}/results", monitoringHandler.GetMonitorResults)
				})
				r.Route("/drift", func(r chi.Router) {
					r.Get("/", monitoringHandler.ListDriftEvents)
					r.Put("/{id}/acknowledge", monitoringHandler.AcknowledgeDrift)
					r.Put("/{id}/resolve", monitoringHandler.ResolveDrift)
				})
			}
		})

		// Workflows
		r.Route("/workflows", func(r chi.Router) {
			if workflowHandler != nil {
				r.Get("/my-approvals", workflowHandler.GetMyApprovals)
				r.Get("/definitions", workflowHandler.ListDefinitions)
				r.Post("/definitions", workflowHandler.CreateDefinition)
				r.Put("/definitions/{id}", workflowHandler.UpdateDefinition)
				r.Post("/definitions/{id}/activate", workflowHandler.ActivateDefinition)
				r.Get("/instances", workflowHandler.ListInstances)
				r.Get("/instances/{id}", workflowHandler.GetInstance)
				r.Post("/start", workflowHandler.StartWorkflow)
				r.Post("/instances/{id}/cancel", workflowHandler.CancelWorkflow)
				r.Post("/executions/{id}/approve", workflowHandler.ApproveStep)
				r.Post("/executions/{id}/reject", workflowHandler.RejectStep)
				r.Post("/executions/{id}/delegate", workflowHandler.DelegateStep)
				r.Post("/executions/{id}/request-info", workflowHandler.RequestInfo)
				r.Get("/delegations", workflowHandler.ListDelegations)
				r.Post("/delegations", workflowHandler.CreateDelegation)
			}
		})

		// Integrations
		r.Route("/integrations", func(r chi.Router) {
			if integrationHandler != nil {
				r.Get("/", integrationHandler.ListIntegrations)
				r.Post("/", integrationHandler.CreateIntegration)
				r.Get("/{id}", integrationHandler.GetIntegration)
				r.Put("/{id}", integrationHandler.UpdateIntegration)
				r.Delete("/{id}", integrationHandler.DeleteIntegration)
				r.Post("/{id}/test", integrationHandler.TestConnection)
				r.Post("/{id}/sync", integrationHandler.TriggerSync)
				r.Get("/{id}/logs", integrationHandler.GetSyncLogs)
			}
		})

		// SSO & API Keys (under settings)
		if integrationHandler != nil {
			r.Get("/settings/sso", integrationHandler.GetSSOConfig)
			r.Put("/settings/sso", integrationHandler.UpdateSSOConfig)
			r.Get("/settings/api-keys", integrationHandler.ListAPIKeys)
			r.Post("/settings/api-keys", integrationHandler.CreateAPIKey)
			r.Delete("/settings/api-keys/{id}", integrationHandler.RevokeAPIKey)
		}

		// Access Policies (ABAC)
		r.Route("/access", func(r chi.Router) {
			if accessHandler != nil {
				r.Get("/policies", accessHandler.ListPolicies)
				r.Post("/policies", accessHandler.CreatePolicy)
				r.Put("/policies/{id}", accessHandler.UpdatePolicy)
				r.Delete("/policies/{id}", accessHandler.DeletePolicy)
				r.Post("/policies/{id}/assignments", accessHandler.AssignPolicy)
				r.Delete("/policies/{id}/assignments/{assignmentId}", accessHandler.RemoveAssignment)
				r.Post("/evaluate", accessHandler.TestEvaluate)
				r.Get("/audit-log", accessHandler.GetAuditLog)
				r.Get("/my-permissions", accessHandler.GetMyPermissions)
				r.Get("/field-permissions", accessHandler.GetFieldPermissions)
			}
		})

		// Onboarding & Subscription
		r.Route("/onboard", func(r chi.Router) {
			if onboardingHandler != nil {
				r.Get("/progress", onboardingHandler.GetProgress)
				r.Put("/step/{n}", onboardingHandler.SaveStep)
				r.Post("/step/{n}/skip", onboardingHandler.SkipStep)
				r.Post("/complete", onboardingHandler.Complete)
				r.Get("/recommendations", onboardingHandler.GetRecommendations)
			}
		})
		r.Route("/subscription", func(r chi.Router) {
			if onboardingHandler != nil {
				r.Get("/", onboardingHandler.GetSubscription)
				r.Put("/plan", onboardingHandler.ChangePlan)
				r.Post("/cancel", onboardingHandler.Cancel)
				r.Get("/plans", onboardingHandler.ListPlans)
				r.Get("/usage", onboardingHandler.GetUsage)
			}
		})
	})

	return r
}
