package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/middleware"
)

// NewRouter creates the Chi router with all middleware, route groups, and handler
// bindings. Required dependencies are composed and validated before a handler
// is returned, so a process cannot start with missing authentication routes.
func NewRouter(pool *pgxpool.Pool, cfg *config.Config) (http.Handler, error) {
	dependencies, err := BuildDependencies(pool, cfg)
	if err != nil {
		return nil, fmt.Errorf("building API dependencies: %w", err)
	}
	return NewRouterWithDependencies(cfg, dependencies)
}

// NewRouterWithDependencies builds the HTTP surface from explicit
// dependencies. It is exported to support focused composition tests.
func NewRouterWithDependencies(cfg *config.Config, dependencies RouterDependencies) (http.Handler, error) {
	if cfg == nil {
		return nil, errors.New("router configuration is required")
	}
	if err := dependencies.Validate(); err != nil {
		return nil, fmt.Errorf("invalid router dependencies: %w", err)
	}

	r := chi.NewRouter()

	// --- Global middleware chain ---
	r.Use(chimw.RequestID)
	r.Use(middleware.TrustedProxyHeaders(cfg.App.TrustProxyHeaders))
	r.Use(middleware.LoggingMiddleware)
	r.Use(middleware.CORSMiddleware(cfg.CORS.AllowedOrigins))
	r.Use(middleware.DistributedRateLimitMiddleware(dependencies.RequestRateLimiter, cfg.RateLimit.RPS))
	r.Use(chimw.Recoverer)

	// --- Health checks (no auth required) ---
	live := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}
	r.Get("/health", live)
	r.Get("/health/live", live)
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := dependencies.HealthCheck(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "unready"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	// Required handlers are validated above. Remaining domain modules are
	// explicitly optional in this vertical slice.
	authHandler := dependencies.Auth
	organizationHandler := dependencies.Organizations
	frameworkHandler := dependencies.Frameworks
	controlHandler := dependencies.Controls
	riskHandler := dependencies.Risks
	policyHandler := dependencies.Policies
	auditHandler := dependencies.Audits
	permissionHandler := dependencies.Permissions
	incidentHandler := dependencies.Incidents
	assetHandler := dependencies.Assets
	vendorHandler := dependencies.Vendors
	accessAdministrationHandler := dependencies.AccessAdministration
	dashboardHandler := dependencies.Domains.Dashboard
	reportHandler := dependencies.Domains.Report
	notificationHandler := dependencies.Notifications
	dsrHandler := dependencies.Domains.DSR
	nis2Handler := dependencies.Domains.NIS2
	monitoringHandler := dependencies.Domains.Monitoring
	workflowHandler := dependencies.Domains.Workflow
	integrationHandler := dependencies.Integrations
	onboardingHandler := dependencies.Domains.Onboarding
	accessHandler := dependencies.Domains.Access
	remediationHandler := dependencies.Domains.Remediation
	marketplaceHandler := dependencies.Domains.Marketplace
	regulatoryHandler := dependencies.Domains.Regulatory
	biaHandler := dependencies.Domains.BIA
	analyticsHandler := dependencies.Domains.Analytics
	exceptionHandler := dependencies.Domains.Exception
	evidenceTemplateHandler := dependencies.Domains.EvidenceTemplate
	questionnaireHandler := dependencies.Domains.Questionnaire
	vendorPortalHandler := dependencies.Domains.VendorPortal
	ropaHandler := dependencies.Domains.ROPA
	boardHandler := dependencies.Domains.Board
	boardPortalHandler := dependencies.Domains.BoardPortal
	calendarHandler := dependencies.Domains.Calendar
	searchHandler := dependencies.Domains.Search
	collaborationHandler := dependencies.Domains.Collaboration
	mobileHandler := dependencies.Domains.Mobile
	brandingHandler := dependencies.Domains.Branding

	// --- Public routes (no authentication required) ---
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/login", authHandler.Login)
		r.Post("/register", authHandler.Register)
		r.Post("/refresh", authHandler.Refresh)
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware(dependencies.AccessTokenValidator))
			r.Use(dependencies.TenantMiddleware)
			r.With(middleware.RequireAuthorization(dependencies.Authorizer, "users", "read", nil)).Get("/me", authHandler.Me)
			// Logout is a self-service session operation. Requiring users:read
			// keeps it available to every seeded role while still rejecting
			// principals whose role assignment has been revoked.
			r.With(middleware.RequireAuthorization(dependencies.Authorizer, "users", "read", nil)).Post("/logout", authHandler.Logout)
		})
	})

	// --- Public portal routes (token-authenticated, no JWT) ---
	r.Route("/api/v1/vendor-portal", func(r chi.Router) {
		if vendorPortalHandler != nil {
			r.Get("/{token}", vendorPortalHandler.GetQuestionnaire)
			r.Put("/{token}/responses", vendorPortalHandler.UpdateResponses)
			r.Post("/{token}/responses/{questionId}/evidence", vendorPortalHandler.UploadEvidence)
			r.Post("/{token}/submit", vendorPortalHandler.Submit)
			r.Get("/{token}/progress", vendorPortalHandler.GetProgress)
		}
	})

	// --- Public calendar iCal feed (token-authenticated, no JWT) ---
	r.Route("/api/v1/calendar/ical", func(r chi.Router) {
		if calendarHandler != nil {
			r.Get("/{token}", calendarHandler.GetICalFeed)
		}
	})

	// --- Public branding routes (no authentication required) ---
	r.Route("/api/v1/branding", func(r chi.Router) {
		if brandingHandler != nil {
			r.Get("/", brandingHandler.GetBranding)
			r.Get("/css", brandingHandler.GetBrandingCSS)
		}
	})

	r.Route("/api/v1/board-portal", func(r chi.Router) {
		if boardPortalHandler != nil {
			r.Get("/{token}", boardPortalHandler.GetOverview)
			r.Get("/{token}/meetings", boardPortalHandler.GetMeetings)
			r.Get("/{token}/meetings/{id}/pack", boardPortalHandler.GetMeetingPack)
			r.Get("/{token}/decisions", boardPortalHandler.GetDecisions)
		}
	})

	// Programmatic read API. API keys use their own credential, tenant, rate
	// limit, and exact-scope middleware rather than inheriting browser roles.
	r.Route("/api/v1/automation", func(r chi.Router) {
		r.Use(middleware.APIKeyAuth(dependencies.APIKeyAuthenticator, dependencies.APIKeyRateLimiter))
		r.Use(dependencies.TenantMiddleware)

		r.Route("/frameworks", func(r chi.Router) {
			r.With(middleware.RequireAPIKeyPermission("read", "frameworks")).Get("/", frameworkHandler.List)
			r.With(middleware.RequireAPIKeyPermission("read", "frameworks")).Get("/{id}", frameworkHandler.GetByID)
			r.With(middleware.RequireAPIKeyPermission("read", "controls")).Get("/{id}/controls", frameworkHandler.GetControls)
		})
		r.Route("/controls", func(r chi.Router) {
			r.With(middleware.RequireAPIKeyPermission("read", "controls")).Get("/", controlHandler.List)
			r.With(middleware.RequireAPIKeyPermission("read", "controls")).Get("/{id}", controlHandler.GetByID)
			r.With(middleware.RequireAPIKeyPermission("read", "controls")).Get("/{id}/evidence", controlHandler.ListEvidence)
		})
		r.Route("/risks", func(r chi.Router) {
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/", riskHandler.List)
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/matrix", riskHandler.GetMatrix)
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/heatmap", riskHandler.GetHeatmap)
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/categories", riskHandler.ListCategories)
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/appetite", riskHandler.ListAppetite)
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/{id}", riskHandler.GetByID)
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/{id}/assessments", riskHandler.ListAssessments)
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/{id}/treatments", riskHandler.ListTreatments)
			r.With(middleware.RequireAPIKeyPermission("read", "risks")).Get("/{id}/indicators", riskHandler.ListIndicators)
		})
		r.Route("/policies", func(r chi.Router) {
			r.With(middleware.RequireAPIKeyPermission("read", "policies")).Get("/", policyHandler.List)
			r.With(middleware.RequireAPIKeyPermission("read", "policies")).Get("/categories", policyHandler.ListCategories)
			r.With(middleware.RequireAPIKeyPermission("read", "policies")).Get("/due-for-review", policyHandler.GetDueForReview)
			r.With(middleware.RequireAPIKeyPermission("read", "policies")).Get("/{id}", policyHandler.GetByID)
			r.With(middleware.RequireAPIKeyPermission("read", "policies")).Get("/{id}/versions", policyHandler.ListVersions)
			r.With(middleware.RequireAPIKeyPermission("read", "policies")).Get("/{id}/reviews", policyHandler.ListReviews)
		})
		r.Route("/audits", func(r chi.Router) {
			r.With(middleware.RequireAPIKeyPermission("read", "audits")).Get("/", auditHandler.List)
			r.With(middleware.RequireAPIKeyPermission("read", "audits")).Get("/{id}", auditHandler.GetByID)
			r.With(middleware.RequireAPIKeyPermission("read", "audits")).Get("/{id}/findings", auditHandler.ListFindings)
			r.With(middleware.RequireAPIKeyPermission("read", "audits")).Get("/{id}/findings/stats", auditHandler.FindingStats)
			r.With(middleware.RequireAPIKeyPermission("read", "audits")).Get("/{id}/findings/{findingID}", auditHandler.GetFinding)
		})
		r.Route("/incidents", func(r chi.Router) {
			r.With(middleware.RequireAPIKeyPermission("read", "incidents")).Get("/", incidentHandler.List)
			r.With(middleware.RequireAPIKeyPermission("read", "incidents")).Get("/statistics", incidentHandler.Statistics)
			r.With(middleware.RequireAPIKeyPermission("read", "incidents")).Get("/breaches/upcoming", incidentHandler.GetBreachNotifiable)
			r.With(middleware.RequireAPIKeyPermission("read", "incidents")).Get("/{id}", incidentHandler.GetByID)
			r.With(middleware.RequireAPIKeyPermission("read", "incidents")).Get("/{id}/timeline", incidentHandler.ListEvents)
			r.With(middleware.RequireAPIKeyPermission("read", "incidents")).Get("/{id}/assignments", incidentHandler.ListAssignments)
		})
		r.Route("/assets", func(r chi.Router) {
			r.With(middleware.RequireAPIKeyPermission("read", "assets")).Get("/", assetHandler.List)
			r.With(middleware.RequireAPIKeyPermission("read", "assets")).Get("/stats", assetHandler.Stats)
			r.With(middleware.RequireAPIKeyPermission("read", "assets")).Get("/{id}", assetHandler.GetByID)
			r.With(middleware.RequireAPIKeyPermission("read", "assets")).Get("/{id}/events", assetHandler.ListEvents)
		})
		r.Route("/vendors", func(r chi.Router) {
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/", vendorHandler.List)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/statistics", vendorHandler.Statistics)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/due-for-assessment", vendorHandler.ListDueForAssessment)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/contracts/upcoming", vendorHandler.ListDueContracts)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/certifications/expiring", vendorHandler.ListExpiringCertifications)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/{id}", vendorHandler.GetByID)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/{id}/timeline", vendorHandler.ListEvents)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/{id}/contacts", vendorHandler.ListContacts)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/{id}/contracts", vendorHandler.ListContracts)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/{id}/certifications", vendorHandler.ListCertifications)
			r.With(middleware.RequireAPIKeyPermission("read", "vendors")).Get("/{id}/subprocessors", vendorHandler.ListSubprocessors)
		})
	})

	// --- Protected routes (authentication + tenant middleware) ---
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(dependencies.AccessTokenValidator))
		r.Use(dependencies.TenantMiddleware)
		r.Use(authorizeProtectedRoute(dependencies.Authorizer))

		// Organizations
		r.Route("/organizations", func(r chi.Router) {
			r.Post("/", organizationHandler.Create)
			r.Get("/", organizationHandler.List)
			r.Get("/{id}", organizationHandler.GetByID)
			r.Put("/{id}", organizationHandler.Update)
			r.Delete("/{id}", organizationHandler.Delete)
		})

		// Compliance Frameworks
		r.Route("/frameworks", func(r chi.Router) {
			r.Get("/", frameworkHandler.List)
			r.Get("/{id}", frameworkHandler.GetByID)
			r.Post("/{id}/adopt", frameworkHandler.Adopt)
			r.Get("/{id}/controls", frameworkHandler.GetControls)
		})

		// Controls
		r.Route("/controls", func(r chi.Router) {
			r.Get("/", controlHandler.List)
			r.Get("/{id}", controlHandler.GetByID)
			r.Patch("/{id}/implementation", controlHandler.UpdateImplementation)
			r.Post("/{id}/evidence", controlHandler.AttachEvidence)
			r.Get("/{id}/evidence", controlHandler.ListEvidence)
		})

		// Risks
		r.Route("/risks", func(r chi.Router) {
			r.Post("/", riskHandler.Create)
			r.Get("/", riskHandler.List)
			r.Get("/matrix", riskHandler.GetMatrix)
			r.Get("/heatmap", riskHandler.GetHeatmap)
			r.Get("/categories", riskHandler.ListCategories)
			r.Get("/appetite", riskHandler.ListAppetite)
			r.Put("/appetite/{categoryID}", riskHandler.UpsertAppetite)
			r.Post("/appetite/{categoryID}/approve", riskHandler.ApproveAppetite)
			r.Get("/{id}", riskHandler.GetByID)
			r.Put("/{id}", riskHandler.Update)
			r.Patch("/{id}", riskHandler.Update)
			r.Delete("/{id}", riskHandler.Delete)
			r.Put("/{id}/assign", riskHandler.Assign)
			r.Post("/{id}/assessments", riskHandler.CreateAssessment)
			r.Get("/{id}/assessments", riskHandler.ListAssessments)
			r.Post("/{id}/treatments", riskHandler.CreateTreatment)
			r.Get("/{id}/treatments", riskHandler.ListTreatments)
			r.Get("/{id}/treatments/{treatmentID}", riskHandler.GetTreatment)
			r.Patch("/{id}/treatments/{treatmentID}", riskHandler.UpdateTreatment)
			r.Post("/{id}/indicators", riskHandler.CreateIndicator)
			r.Get("/{id}/indicators", riskHandler.ListIndicators)
			r.Post("/{id}/indicators/{indicatorID}/values", riskHandler.RecordIndicatorValue)
			r.Get("/{id}/indicators/{indicatorID}/values", riskHandler.ListIndicatorValues)
		})

		// Policies
		r.Route("/policies", func(r chi.Router) {
			r.Post("/", policyHandler.Create)
			r.Get("/", policyHandler.List)
			r.Get("/categories", policyHandler.ListCategories)
			r.Get("/due-for-review", policyHandler.GetDueForReview)
			r.Get("/{id}", policyHandler.GetByID)
			r.Put("/{id}", policyHandler.Update)
			r.Patch("/{id}", policyHandler.Update)
			r.Delete("/{id}", policyHandler.Delete)
			r.Put("/{id}/assign", policyHandler.AssignOwner)
			r.Post("/{id}/versions", policyHandler.CreateVersion)
			r.Get("/{id}/versions", policyHandler.ListVersions)
			r.Get("/{id}/versions/{versionID}", policyHandler.GetVersion)
			r.Post("/{id}/submit", policyHandler.SubmitForApproval)
			r.Post("/{id}/approval/decision", policyHandler.DecideApproval)
			r.Get("/{id}/approval", policyHandler.GetActiveApproval)
			r.Put("/{id}/publish", policyHandler.Publish)
			r.Post("/{id}/reviews", policyHandler.CreateReview)
			r.Get("/{id}/reviews", policyHandler.ListReviews)
			r.Get("/{id}/reviews/{reviewID}", policyHandler.GetReview)
			r.Patch("/{id}/reviews/{reviewID}", policyHandler.UpdateReview)
			r.Put("/{id}/acknowledge", policyHandler.Acknowledge)
			r.Get("/{id}/attestations", policyHandler.ListAttestations)
			r.Post("/{id}/exceptions", policyHandler.CreateException)
			r.Get("/{id}/exceptions", policyHandler.ListExceptions)
			r.Get("/{id}/exceptions/{exceptionID}", policyHandler.GetException)
			r.Post("/{id}/exceptions/{exceptionID}/decision", policyHandler.DecideException)
			// Backward-compatible aliases use the same workflow-backed behavior.
			r.Put("/{id}/submit-review", policyHandler.SubmitForReview)
			r.Put("/{id}/approve", policyHandler.Approve)
		})

		// Audits
		r.Route("/audits", func(r chi.Router) {
			r.Post("/", auditHandler.Create)
			r.Get("/", auditHandler.List)
			r.Get("/{id}", auditHandler.GetByID)
			r.Patch("/{id}", auditHandler.Update)
			r.Put("/{id}", auditHandler.Update)
			r.Delete("/{id}", auditHandler.Delete)
			r.Put("/{id}/start", auditHandler.Start)
			r.Put("/{id}/complete", auditHandler.Complete)
			r.Put("/{id}/close", auditHandler.Close)
			r.Put("/{id}/cancel", auditHandler.Cancel)
			r.Post("/{id}/findings", auditHandler.CreateFinding)
			r.Get("/{id}/findings", auditHandler.ListFindings)
			r.Get("/{id}/findings/stats", auditHandler.FindingStats)
			r.Get("/{id}/findings/{findingID}", auditHandler.GetFinding)
			r.Patch("/{id}/findings/{findingID}", auditHandler.UpdateFinding)
			r.Put("/{id}/findings/{findingID}", auditHandler.UpdateFinding)
			r.Delete("/{id}/findings/{findingID}", auditHandler.DeleteFinding)
		})

		// Incidents
		r.Route("/incidents", func(r chi.Router) {
			r.Post("/", incidentHandler.Create)
			r.Get("/", incidentHandler.List)
			r.Get("/statistics", incidentHandler.Statistics)
			r.Get("/breaches/upcoming", incidentHandler.GetBreachNotifiable)
			// Backward-compatible deadline alias.
			r.Get("/breach-notifiable", incidentHandler.GetBreachNotifiable)
			r.Get("/{id}", incidentHandler.GetByID)
			r.Put("/{id}", incidentHandler.Update)
			r.Patch("/{id}", incidentHandler.Update)
			r.Delete("/{id}", incidentHandler.Delete)
			r.Put("/{id}/status", incidentHandler.UpdateStatus)
			r.Post("/{id}/transitions", incidentHandler.Transition)
			r.Post("/{id}/cancel", incidentHandler.Cancel)
			r.Post("/{id}/reopen", incidentHandler.Reopen)
			r.Post("/{id}/close", incidentHandler.Close)
			r.Put("/{id}/escalate", incidentHandler.Escalate)
			r.Post("/{id}/breach-assessment", incidentHandler.AssessBreach)
			r.Post("/{id}/notify-dpa", incidentHandler.NotifyDPA)
			r.Get("/{id}/timeline", incidentHandler.ListEvents)
			r.Post("/{id}/assignments", incidentHandler.CreateAssignment)
			r.Get("/{id}/assignments", incidentHandler.ListAssignments)
			r.Post("/{id}/assignments/{assignmentID}/unassign", incidentHandler.Unassign)
		})

		// Asset inventory
		r.Route("/assets", func(r chi.Router) {
			r.Post("/", assetHandler.Create)
			r.Get("/", assetHandler.List)
			r.Get("/stats", assetHandler.Stats)
			r.Get("/{id}", assetHandler.GetByID)
			r.Put("/{id}", assetHandler.Update)
			r.Patch("/{id}", assetHandler.Update)
			r.Delete("/{id}", assetHandler.Delete)
			r.Get("/{id}/events", assetHandler.ListEvents)
		})

		// Vendors
		r.Route("/vendors", func(r chi.Router) {
			r.Post("/", vendorHandler.Create)
			r.Get("/", vendorHandler.List)
			r.Get("/statistics", vendorHandler.Statistics)
			// Backward-compatible route for the existing dashboard client.
			r.Get("/stats", vendorHandler.Statistics)
			r.Get("/due-for-assessment", vendorHandler.ListDueForAssessment)
			r.Get("/contracts/upcoming", vendorHandler.ListDueContracts)
			r.Get("/certifications/expiring", vendorHandler.ListExpiringCertifications)
			r.Get("/{id}", vendorHandler.GetByID)
			r.Put("/{id}", vendorHandler.Update)
			r.Patch("/{id}", vendorHandler.Update)
			r.Delete("/{id}", vendorHandler.Delete)
			r.Post("/{id}/transitions", vendorHandler.Transition)
			r.Post("/{id}/assessments", vendorHandler.RecordAssessment)
			// Compatibility alias for the former assessment stub.
			r.Post("/{id}/assess", vendorHandler.RecordAssessment)
			r.Get("/{id}/timeline", vendorHandler.ListEvents)
			r.Get("/{id}/contacts", vendorHandler.ListContacts)
			r.Post("/{id}/contacts", vendorHandler.SaveContact)
			r.Put("/{id}/contacts/{contactID}", vendorHandler.SaveContact)
			r.Delete("/{id}/contacts/{contactID}", vendorHandler.DeleteContact)
			r.Get("/{id}/contracts", vendorHandler.ListContracts)
			r.Post("/{id}/contracts", vendorHandler.SaveContract)
			r.Put("/{id}/contracts/{contractID}", vendorHandler.SaveContract)
			r.Delete("/{id}/contracts/{contractID}", vendorHandler.DeleteContract)
			r.Get("/{id}/certifications", vendorHandler.ListCertifications)
			r.Post("/{id}/certifications", vendorHandler.SaveCertification)
			r.Put("/{id}/certifications/{certificationID}", vendorHandler.SaveCertification)
			r.Delete("/{id}/certifications/{certificationID}", vendorHandler.DeleteCertification)
			r.Get("/{id}/subprocessors", vendorHandler.ListSubprocessors)
			r.Post("/{id}/subprocessors", vendorHandler.SaveSubprocessor)
			r.Put("/{id}/subprocessors/{subprocessorID}", vendorHandler.SaveSubprocessor)
			r.Delete("/{id}/subprocessors/{subprocessorID}", vendorHandler.DeleteSubprocessor)
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
			r.Get("/", notificationHandler.ListNotifications)
			r.Put("/{id}/read", notificationHandler.MarkAsRead)
			r.Put("/{id}/acknowledge", notificationHandler.Acknowledge)
			r.Put("/read-all", notificationHandler.MarkAllAsRead)
			r.Get("/unread-count", notificationHandler.GetUnreadCount)
			r.Get("/preferences", notificationHandler.GetPreferences)
			r.Put("/preferences", notificationHandler.UpdatePreferences)
		})

		// Notification settings (admin)
		r.Route("/settings/notification-rules", func(r chi.Router) {
			r.Get("/", notificationHandler.ListRules)
			r.Post("/", notificationHandler.CreateRule)
			r.Put("/{id}", notificationHandler.UpdateRule)
			r.Delete("/{id}", notificationHandler.DeleteRule)
		})
		r.Route("/settings/notification-templates", func(r chi.Router) {
			r.Get("/", notificationHandler.ListTemplates)
			r.Post("/", notificationHandler.CreateTemplate)
			r.Put("/{id}", notificationHandler.UpdateTemplate)
			r.Delete("/{id}", notificationHandler.DeleteTemplate)
		})
		r.Route("/settings/notification-channels", func(r chi.Router) {
			r.Get("/", notificationHandler.ListChannels)
			r.Post("/", notificationHandler.CreateChannel)
			r.Put("/{id}", notificationHandler.UpdateChannel)
			r.Delete("/{id}", notificationHandler.DeleteChannel)
			r.Post("/{id}/test", notificationHandler.TestChannel)
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
			r.Get("/", integrationHandler.ListIntegrations)
			r.Post("/", integrationHandler.CreateIntegration)
			r.Get("/{id}", integrationHandler.GetIntegration)
			r.Put("/{id}", integrationHandler.UpdateIntegration)
			r.Delete("/{id}", integrationHandler.DeleteIntegration)
			r.Post("/{id}/test", integrationHandler.TestConnection)
			r.Post("/{id}/sync", integrationHandler.TriggerSync)
			r.Get("/{id}/logs", integrationHandler.GetSyncLogs)
		})

		// SSO & API Keys (under settings)
		r.Get("/settings/sso", integrationHandler.GetSSOConfig)
		r.Put("/settings/sso", integrationHandler.UpdateSSOConfig)
		r.Get("/settings/api-keys", integrationHandler.ListAPIKeys)
		r.Post("/settings/api-keys", integrationHandler.CreateAPIKey)
		r.Delete("/settings/api-keys/{id}", integrationHandler.RevokeAPIKey)

		// Access Policies (ABAC)
		r.Route("/access", func(r chi.Router) {
			r.Get("/my-permissions", permissionHandler.GetMyPermissions)
			r.Get("/permissions", accessAdministrationHandler.ListPermissions)
			r.Get("/roles", accessAdministrationHandler.ListRoles)
			r.Post("/roles", accessAdministrationHandler.CreateRole)
			r.Get("/roles/{id}", accessAdministrationHandler.GetRole)
			r.Patch("/roles/{id}", accessAdministrationHandler.UpdateRole)
			r.Put("/roles/{id}", accessAdministrationHandler.UpdateRole)
			r.Delete("/roles/{id}", accessAdministrationHandler.DeleteRole)
			r.Post("/roles/{id}/clone", accessAdministrationHandler.CloneRole)
			r.Post("/roles/{id}/impact-preview", accessAdministrationHandler.PreviewImpact)
			r.Get("/roles/{id}/assignments", accessAdministrationHandler.ListAssignments)
			r.Post("/roles/{id}/assignments", accessAdministrationHandler.AssignRole)
			r.Delete("/roles/{id}/assignments/{userID}", accessAdministrationHandler.UnassignRole)
			r.Get("/roles/{id}/events", accessAdministrationHandler.ListEvents)
			if accessHandler != nil {
				r.Get("/policies", accessHandler.ListPolicies)
				r.Post("/policies", accessHandler.CreatePolicy)
				r.Put("/policies/{id}", accessHandler.UpdatePolicy)
				r.Delete("/policies/{id}", accessHandler.DeletePolicy)
				r.Post("/policies/{id}/assignments", accessHandler.AssignPolicy)
				r.Delete("/policies/{id}/assignments/{assignmentId}", accessHandler.RemoveAssignment)
				r.Post("/evaluate", accessHandler.TestEvaluate)
				r.Get("/audit-log", accessHandler.GetAuditLog)
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

		// Remediation Plans
		r.Route("/remediation", func(r chi.Router) {
			if remediationHandler != nil {
				r.Route("/plans", func(r chi.Router) {
					r.Get("/", remediationHandler.ListPlans)
					r.Post("/", remediationHandler.CreatePlan)
					r.Post("/generate", remediationHandler.GeneratePlan)
					r.Route("/{id}", func(r chi.Router) {
						r.Get("/", remediationHandler.GetPlan)
						r.Put("/", remediationHandler.UpdatePlan)
						r.Post("/approve", remediationHandler.ApprovePlan)
						r.Get("/progress", remediationHandler.GetPlanProgress)
					})
				})
				r.Route("/actions/{id}", func(r chi.Router) {
					r.Put("/", remediationHandler.UpdateAction)
					r.Post("/complete", remediationHandler.CompleteAction)
				})
			}
		})

		// AI Assistance
		r.Route("/ai", func(r chi.Router) {
			if remediationHandler != nil {
				r.Post("/control-guidance", remediationHandler.GetControlGuidance)
				r.Post("/evidence-suggestion", remediationHandler.GetEvidenceSuggestion)
				r.Post("/policy-draft", remediationHandler.GetPolicyDraft)
				r.Post("/risk-narrative", remediationHandler.GetRiskNarrative)
				r.Get("/usage", remediationHandler.GetAIUsage)
				r.Post("/feedback", remediationHandler.SubmitAIFeedback)
			}
		})

		// Marketplace
		r.Route("/marketplace", func(r chi.Router) {
			if marketplaceHandler != nil {
				// Public-ish (still behind auth in this block)
				r.Route("/packages", func(r chi.Router) {
					r.Get("/", marketplaceHandler.SearchPackages)
					r.Get("/featured", marketplaceHandler.GetFeaturedPackages)
					r.Get("/{publisher}/{slug}", marketplaceHandler.GetPackageDetail)
					r.Get("/{publisher}/{slug}/reviews", marketplaceHandler.GetPackageReviews)
				})
				// Install / uninstall
				r.Post("/install", marketplaceHandler.InstallPackage)
				r.Delete("/install/{id}", marketplaceHandler.UninstallPackage)
				r.Get("/installed", marketplaceHandler.ListInstalled)
				r.Post("/reviews", marketplaceHandler.SubmitReview)
				// Publisher
				r.Post("/publishers", marketplaceHandler.RegisterPublisher)
				r.Route("/publishers/me", func(r chi.Router) {
					r.Get("/stats", marketplaceHandler.GetPublisherStats)
					r.Post("/packages", marketplaceHandler.CreatePackageEntry)
					r.Post("/packages/{id}/versions", marketplaceHandler.PublishVersion)
				})
			}
		})

		// Regulatory Intelligence
		r.Route("/regulatory", func(r chi.Router) {
			if regulatoryHandler != nil {
				r.Route("/changes", func(r chi.Router) {
					r.Get("/", regulatoryHandler.ListChanges)
					r.Route("/{id}", func(r chi.Router) {
						r.Get("/", regulatoryHandler.GetChange)
						r.Post("/assess", regulatoryHandler.AssessImpact)
						r.Get("/assessment", regulatoryHandler.GetAssessment)
						r.Post("/respond", regulatoryHandler.CreateResponsePlan)
					})
				})
				r.Route("/sources", func(r chi.Router) {
					r.Get("/", regulatoryHandler.ListSources)
					r.Post("/", regulatoryHandler.AddSource)
				})
				r.Route("/subscriptions", func(r chi.Router) {
					r.Get("/", regulatoryHandler.ListSubscriptions)
					r.Post("/", regulatoryHandler.Subscribe)
				})
				r.Get("/dashboard", regulatoryHandler.GetDashboard)
				r.Get("/timeline", regulatoryHandler.GetTimeline)
			}
		})

		// Business Impact Analysis
		r.Route("/bia", func(r chi.Router) {
			if biaHandler != nil {
				r.Route("/processes", func(r chi.Router) {
					r.Get("/", biaHandler.ListProcesses)
					r.Post("/", biaHandler.CreateProcess)
					r.Route("/{id}", func(r chi.Router) {
						r.Get("/", biaHandler.GetProcess)
						r.Put("/", biaHandler.UpdateProcess)
						r.Post("/dependencies", biaHandler.MapDependencies)
						r.Get("/dependency-graph", biaHandler.GetDependencyGraph)
					})
				})
				r.Get("/single-points-of-failure", biaHandler.GetSinglePointsOfFailure)
				r.Get("/report", biaHandler.GetBIAReport)
			}
		})

		// Business Continuity
		r.Route("/bc", func(r chi.Router) {
			if biaHandler != nil {
				r.Route("/scenarios", func(r chi.Router) {
					r.Get("/", biaHandler.ListScenarios)
					r.Post("/", biaHandler.CreateScenario)
				})
				r.Route("/plans", func(r chi.Router) {
					r.Get("/", biaHandler.ListBCPlans)
					r.Post("/", biaHandler.CreateBCPlan)
					r.Post("/{id}/approve", biaHandler.ApproveBCPlan)
				})
				r.Route("/exercises", func(r chi.Router) {
					r.Get("/", biaHandler.ListExercises)
					r.Post("/", biaHandler.CreateExercise)
					r.Put("/{id}/complete", biaHandler.CompleteExercise)
				})
				r.Get("/dashboard", biaHandler.GetBCDashboard)
			}
		})

		// Analytics
		r.Route("/analytics", func(r chi.Router) {
			if analyticsHandler != nil {
				r.Get("/snapshots", analyticsHandler.ListSnapshots)
				r.Route("/trends", func(r chi.Router) {
					r.Get("/compliance", analyticsHandler.GetComplianceTrends)
					r.Get("/risks", analyticsHandler.GetRiskTrends)
				})
				r.Route("/predictions", func(r chi.Router) {
					r.Get("/risks/{riskId}", analyticsHandler.GetRiskPrediction)
					r.Get("/breach-probability", analyticsHandler.GetBreachProbability)
				})
				r.Get("/benchmarks", analyticsHandler.GetBenchmarks)
				r.Route("/metrics/{metric}", func(r chi.Router) {
					r.Get("/", analyticsHandler.GetMetricTimeSeries)
					r.Get("/compare", analyticsHandler.CompareMetricPeriods)
				})
				r.Get("/top-movers", analyticsHandler.GetTopMovers)
				r.Get("/distribution/{entity}", analyticsHandler.GetDistribution)
				r.Post("/export", analyticsHandler.ExportData)
				r.Route("/dashboards", func(r chi.Router) {
					r.Get("/", analyticsHandler.ListDashboards)
					r.Post("/", analyticsHandler.CreateDashboard)
					r.Put("/{id}", analyticsHandler.UpdateDashboard)
					r.Delete("/{id}", analyticsHandler.DeleteDashboard)
				})
				r.Get("/widget-types", analyticsHandler.GetWidgetTypes)
			}
		})

		// Exceptions
		r.Route("/exceptions", func(r chi.Router) {
			if exceptionHandler != nil {
				r.Get("/", exceptionHandler.List)
				r.Post("/", exceptionHandler.Create)
				r.Get("/dashboard", exceptionHandler.GetDashboard)
				r.Get("/expiring", exceptionHandler.GetExpiring)
				r.Get("/impact/{id}", exceptionHandler.GetImpactAnalysis)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", exceptionHandler.GetByID)
					r.Put("/", exceptionHandler.Update)
					r.Post("/submit", exceptionHandler.Submit)
					r.Post("/approve", exceptionHandler.Approve)
					r.Post("/reject", exceptionHandler.Reject)
					r.Post("/revoke", exceptionHandler.Revoke)
					r.Post("/renew", exceptionHandler.Renew)
					r.Post("/review", exceptionHandler.Review)
				})
			}
		})

		// Evidence
		r.Route("/evidence", func(r chi.Router) {
			if evidenceTemplateHandler != nil {
				r.Route("/templates", func(r chi.Router) {
					r.Get("/", evidenceTemplateHandler.ListTemplates)
					r.Get("/{id}", evidenceTemplateHandler.GetTemplate)
					r.Post("/", evidenceTemplateHandler.CreateTemplate)
				})
				r.Route("/requirements", func(r chi.Router) {
					r.Get("/", evidenceTemplateHandler.ListRequirements)
					r.Post("/generate", evidenceTemplateHandler.GenerateRequirements)
					r.Put("/{id}", evidenceTemplateHandler.UpdateRequirement)
					r.Post("/{id}/validate", evidenceTemplateHandler.ValidateRequirement)
				})
				r.Get("/gaps", evidenceTemplateHandler.GetEvidenceGaps)
				r.Get("/schedule", evidenceTemplateHandler.GetEvidenceSchedule)
				r.Route("/test-suites", func(r chi.Router) {
					r.Get("/", evidenceTemplateHandler.ListTestSuites)
					r.Post("/", evidenceTemplateHandler.CreateTestSuite)
					r.Post("/{id}/run", evidenceTemplateHandler.RunTestSuite)
					r.Get("/{id}/results", evidenceTemplateHandler.GetTestSuiteResults)
				})
				r.Post("/pre-audit-check", evidenceTemplateHandler.RunPreAuditCheck)
				r.Get("/pre-audit-check/{id}/report", evidenceTemplateHandler.GetPreAuditReport)
			}
		})

		// Questionnaires
		r.Route("/questionnaires", func(r chi.Router) {
			if questionnaireHandler != nil {
				r.Get("/", questionnaireHandler.ListQuestionnaires)
				r.Post("/", questionnaireHandler.CreateQuestionnaire)
				r.Get("/{id}", questionnaireHandler.GetQuestionnaire)
				r.Put("/{id}", questionnaireHandler.UpdateQuestionnaire)
				r.Post("/{id}/clone", questionnaireHandler.CloneQuestionnaire)
			}
		})

		// Vendor Assessments
		r.Route("/vendor-assessments", func(r chi.Router) {
			if questionnaireHandler != nil {
				r.Get("/", questionnaireHandler.ListVendorAssessments)
				r.Post("/", questionnaireHandler.CreateVendorAssessment)
				r.Get("/compare", questionnaireHandler.CompareAssessments)
				r.Get("/dashboard", questionnaireHandler.GetAssessmentDashboard)
				r.Get("/{id}", questionnaireHandler.GetVendorAssessment)
				r.Post("/{id}/review", questionnaireHandler.ReviewVendorAssessment)
				r.Post("/{id}/reminder", questionnaireHandler.SendReminder)
			}
		})

		// Data Privacy / ROPA
		r.Route("/data", func(r chi.Router) {
			if ropaHandler != nil {
				r.Get("/classifications", ropaHandler.ListClassifications)
				r.Post("/classifications", ropaHandler.CreateClassification)
				r.Get("/categories", ropaHandler.ListCategories)
				r.Post("/categories", ropaHandler.CreateCategory)
				r.Route("/processing-activities", func(r chi.Router) {
					r.Get("/", ropaHandler.ListProcessingActivities)
					r.Post("/", ropaHandler.CreateProcessingActivity)
					r.Route("/{id}", func(r chi.Router) {
						r.Get("/", ropaHandler.GetProcessingActivity)
						r.Put("/", ropaHandler.UpdateProcessingActivity)
						r.Post("/flows", ropaHandler.CreateDataFlows)
						r.Get("/flow-diagram", ropaHandler.GetFlowDiagram)
					})
				})
				r.Route("/ropa", func(r chi.Router) {
					r.Post("/export", ropaHandler.ExportROPA)
					r.Get("/exports", ropaHandler.ListExports)
					r.Get("/exports/{id}/download", ropaHandler.DownloadExport)
					r.Get("/dashboard", ropaHandler.GetDashboard)
				})
				r.Get("/high-risk", ropaHandler.GetHighRisk)
				r.Get("/subject-map/{category}", ropaHandler.GetSubjectMap)
			}
		})

		// Board Governance
		r.Route("/board", func(r chi.Router) {
			if boardHandler != nil {
				r.Route("/members", func(r chi.Router) {
					r.Get("/", boardHandler.ListMembers)
					r.Post("/", boardHandler.CreateMember)
					r.Put("/{id}", boardHandler.UpdateMember)
				})
				r.Route("/meetings", func(r chi.Router) {
					r.Get("/", boardHandler.ListMeetings)
					r.Post("/", boardHandler.CreateMeeting)
					r.Put("/{id}", boardHandler.UpdateMeeting)
					r.Post("/{id}/generate-pack", boardHandler.GenerateMeetingPack)
					r.Get("/{id}/download-pack", boardHandler.DownloadMeetingPack)
				})
				r.Route("/decisions", func(r chi.Router) {
					r.Post("/", boardHandler.CreateDecision)
					r.Get("/", boardHandler.ListDecisions)
					r.Put("/{id}/action", boardHandler.UpdateDecisionAction)
				})
				r.Get("/reports", boardHandler.ListReports)
				r.Post("/reports/generate", boardHandler.GenerateReport)
				r.Get("/dashboard", boardHandler.GetBoardDashboard)
				r.Get("/nis2-governance", boardHandler.GetNIS2Governance)
			}
		})

		// Calendar & Scheduling
		r.Route("/calendar", func(r chi.Router) {
			if calendarHandler != nil {
				r.Route("/events", func(r chi.Router) {
					r.Get("/", calendarHandler.ListEvents)
					r.Post("/", calendarHandler.CreateEvent)
					r.Get("/{id}", calendarHandler.GetEvent)
					r.Put("/{id}/complete", calendarHandler.CompleteEvent)
					r.Put("/{id}/reschedule", calendarHandler.RescheduleEvent)
					r.Put("/{id}/assign", calendarHandler.AssignEvent)
				})
				r.Get("/deadlines", calendarHandler.GetDeadlines)
				r.Get("/overdue", calendarHandler.GetOverdue)
				r.Get("/summary", calendarHandler.GetSummary)
				r.Get("/subscriptions", calendarHandler.GetSubscriptions)
				r.Put("/subscriptions", calendarHandler.UpdateSubscriptions)
				r.Route("/sync", func(r chi.Router) {
					r.Get("/status", calendarHandler.GetSyncStatus)
					r.Post("/trigger", calendarHandler.TriggerSync)
				})
			}
		})

		// Global Search
		r.Route("/search", func(r chi.Router) {
			if searchHandler != nil {
				r.Get("/", searchHandler.Search)
				r.Get("/autocomplete", searchHandler.Autocomplete)
				r.Get("/related/{entityType}/{entityId}", searchHandler.GetRelated)
				r.Post("/reindex", searchHandler.Reindex)
			}
		})

		// Knowledge Base
		r.Route("/knowledge", func(r chi.Router) {
			if searchHandler != nil {
				r.Get("/", searchHandler.SearchKnowledge)
				r.Get("/recommended", searchHandler.GetRecommendedArticles)
				r.Get("/for-control/{frameworkCode}/{controlCode}", searchHandler.GetArticlesForControl)
				r.Get("/bookmarks", searchHandler.ListBookmarks)
				r.Post("/bookmarks/{articleId}", searchHandler.CreateBookmark)
				r.Delete("/bookmarks/{articleId}", searchHandler.DeleteBookmark)
				r.Route("/articles", func(r chi.Router) {
					r.Post("/", searchHandler.CreateArticle)
					r.Put("/{id}", searchHandler.UpdateArticle)
					r.Post("/{id}/feedback", searchHandler.SubmitArticleFeedback)
				})
				r.Get("/{slug}", searchHandler.GetKnowledgeArticle)
			}
		})

		// Comments
		r.Route("/comments", func(r chi.Router) {
			if collaborationHandler != nil {
				r.Get("/{entityType}/{entityId}", collaborationHandler.ListComments)
				r.Post("/{entityType}/{entityId}", collaborationHandler.CreateComment)
				r.Put("/{id}", collaborationHandler.UpdateComment)
				r.Delete("/{id}", collaborationHandler.DeleteComment)
				r.Post("/{id}/pin", collaborationHandler.PinComment)
				r.Post("/{id}/react", collaborationHandler.ReactToComment)
			}
		})

		// Activity Feed
		r.Route("/activity", func(r chi.Router) {
			if collaborationHandler != nil {
				r.Get("/feed", collaborationHandler.GetUserFeed)
				r.Get("/org", collaborationHandler.GetOrgFeed)
				r.Get("/unread", collaborationHandler.GetUnreadCount)
				r.Get("/{entityType}/{entityId}", collaborationHandler.GetEntityActivity)
				r.Post("/{entityType}/{entityId}/mark-read", collaborationHandler.MarkEntityRead)
			}
		})

		// Following
		r.Route("/following", func(r chi.Router) {
			if collaborationHandler != nil {
				r.Get("/", collaborationHandler.ListFollowing)
				r.Post("/{entityType}/{entityId}", collaborationHandler.Follow)
				r.Delete("/{entityType}/{entityId}", collaborationHandler.Unfollow)
			}
		})

		// Mobile
		r.Route("/mobile", func(r chi.Router) {
			if mobileHandler != nil {
				r.Get("/dashboard", mobileHandler.GetDashboard)
				r.Route("/approvals", func(r chi.Router) {
					r.Get("/", mobileHandler.ListApprovals)
					r.Post("/{id}/approve", mobileHandler.ApproveItem)
					r.Post("/{id}/reject", mobileHandler.RejectItem)
				})
				r.Get("/incidents/active", mobileHandler.GetActiveIncidents)
				r.Get("/deadlines", mobileHandler.GetDeadlines)
				r.Get("/activity", mobileHandler.GetActivity)
				r.Route("/push", func(r chi.Router) {
					r.Post("/register", mobileHandler.RegisterDevice)
					r.Delete("/unregister", mobileHandler.UnregisterDevice)
					r.Get("/preferences", mobileHandler.GetPushPreferences)
					r.Put("/preferences", mobileHandler.UpdatePushPreferences)
				})
			}
		})

		// Branding (authenticated routes)
		r.Route("/branding", func(r chi.Router) {
			if brandingHandler != nil {
				r.Put("/", brandingHandler.UpdateBranding)
				r.Post("/logo", brandingHandler.UploadLogo)
				r.Delete("/logo/{type}", brandingHandler.DeleteLogo)
				r.Post("/domain/verify", brandingHandler.VerifyDomain)
				r.Get("/domain/status", brandingHandler.GetDomainStatus)
				r.Post("/preview", brandingHandler.PreviewBranding)
			}
		})

		// Admin Partners (white-label)
		r.Route("/admin/partners", func(r chi.Router) {
			if brandingHandler != nil {
				r.Get("/", brandingHandler.ListPartners)
				r.Post("/", brandingHandler.CreatePartner)
				r.Put("/{id}", brandingHandler.UpdatePartner)
				r.Get("/{id}/tenants", brandingHandler.GetPartnerTenants)
			}
		})
	})

	return r, nil
}
