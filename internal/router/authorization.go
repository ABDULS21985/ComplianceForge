package router

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/middleware"
)

type routePermission struct {
	Resource string
	Action   string
}

type denyAuthorizer struct{}

func (denyAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Reason: "protected route has no reviewed permission mapping"}, nil
}

// protectedResourceAliases is the auditable mapping from every authenticated
// API namespace currently mounted by router.go to the stable RBAC resources
// seeded in rbac_defaults.sql. New namespaces fail closed until added here.
var protectedResourceAliases = map[string]string{
	"organizations":      "organizations",
	"frameworks":         "frameworks",
	"controls":           "controls",
	"risks":              "risks",
	"policies":           "policies",
	"audits":             "audits",
	"incidents":          "incidents",
	"vendors":            "vendors",
	"dashboard":          "reports",
	"reports":            "reports",
	"notifications":      "users",
	"settings":           "settings",
	"dsr":                "incidents",
	"nis2":               "controls",
	"monitoring":         "controls",
	"workflows":          "policies",
	"integrations":       "settings",
	"access":             "settings",
	"onboard":            "organizations",
	"subscription":       "organizations",
	"remediation":        "controls",
	"ai":                 "controls",
	"marketplace":        "frameworks",
	"regulatory":         "frameworks",
	"bia":                "risks",
	"bc":                 "risks",
	"analytics":          "reports",
	"exceptions":         "risks",
	"evidence":           "controls",
	"questionnaires":     "vendors",
	"vendor-assessments": "vendors",
	"data":               "incidents",
	"board":              "reports",
	"calendar":           "audits",
	"search":             "reports",
	"knowledge":          "policies",
	"comments":           "controls",
	"activity":           "reports",
	"following":          "controls",
	"mobile":             "reports",
	"branding":           "settings",
	"admin":              "settings",
}

var supportedResourceActions = map[string]map[string]bool{
	"organizations": {"read": true, "update": true, "configure": true},
	"frameworks":    {"create": true, "read": true, "update": true, "delete": true, "export": true},
	"controls":      {"create": true, "read": true, "update": true, "delete": true, "approve": true, "assign": true, "export": true},
	"risks":         {"create": true, "read": true, "update": true, "delete": true, "approve": true, "assign": true, "export": true},
	"policies":      {"create": true, "read": true, "update": true, "delete": true, "approve": true, "assign": true, "export": true},
	"audits":        {"create": true, "read": true, "update": true, "delete": true, "approve": true, "assign": true, "export": true},
	"incidents":     {"create": true, "read": true, "update": true, "delete": true, "approve": true, "assign": true, "export": true},
	"vendors":       {"create": true, "read": true, "update": true, "delete": true, "approve": true, "export": true},
	"reports":       {"create": true, "read": true, "export": true},
	"users":         {"create": true, "read": true, "update": true, "delete": true, "assign": true},
	"settings":      {"read": true, "configure": true},
}

func authorizeProtectedRoute(authorizer authz.Authorizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			permission, ok := permissionForProtectedRequest(r.Method, r.URL.Path)
			if !ok {
				middleware.RequireAuthorization(denyAuthorizer{}, "unmapped_route", "unmapped_action", nil)(next).ServeHTTP(w, r)
				return
			}
			middleware.RequireAuthorization(authorizer, permission.Resource, permission.Action, resolveProtectedResourceID)(next).ServeHTTP(w, r)
		})
	}
}

func permissionForProtectedRequest(method, path string) (routePermission, bool) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/v1/"), "/")
	if trimmed == "" {
		return routePermission{}, false
	}
	segment := strings.Split(trimmed, "/")[0]
	resource, ok := protectedResourceAliases[segment]
	if !ok {
		return routePermission{}, false
	}

	action := actionForMethod(method)
	if action == "" {
		return routePermission{}, false
	}
	lowerPath := strings.ToLower(trimmed)
	if segment == "frameworks" && strings.Contains(lowerPath, "/controls") {
		resource = "controls"
	}
	if containsActionSegment(lowerPath, "download", "export", "generate-pack") {
		action = "export"
	}
	if containsActionSegment(lowerPath, "approve", "review", "reject", "publish", "assess") {
		action = "approve"
	}
	if containsActionSegment(lowerPath, "assign", "delegate") {
		action = "assign"
	}
	if containsActionSegment(lowerPath,
		"start", "complete", "cancel", "submit", "revoke", "renew", "extend",
		"verify-identity", "activate", "acknowledge", "resolve", "reschedule",
		"run-now", "test", "sync", "trigger", "reminder", "skip", "mark-read",
		"pin", "react", "follow", "unfollow",
	) {
		action = "update"
	}
	if segment == "frameworks" && strings.HasSuffix(lowerPath, "/adopt") {
		action = "update"
	}
	if segment == "controls" && strings.Contains(lowerPath, "/evidence") {
		action = mapReadOrUpdate(method)
	}
	if segment == "policies" && containsActionSegment(lowerPath, "decision") {
		action = "approve"
	}
	// An acknowledgement is the authenticated principal's own read receipt.
	// Requiring policy update would prevent read-only employees from complying.
	if segment == "policies" && containsActionSegment(lowerPath, "acknowledge") {
		action = "read"
	}

	if supportedResourceActions[resource][action] {
		return routePermission{Resource: resource, Action: action}, true
	}
	if (action == "approve" || action == "assign") && supportedResourceActions[resource]["update"] {
		return routePermission{Resource: resource, Action: "update"}, true
	}
	switch resource {
	case "settings":
		if method == http.MethodGet || method == http.MethodHead {
			action = "read"
		} else {
			action = "configure"
		}
	case "organizations":
		if method == http.MethodGet || method == http.MethodHead {
			action = "read"
		} else if method == http.MethodPut || method == http.MethodPatch {
			action = "update"
		} else {
			action = "configure"
		}
	case "reports":
		if method == http.MethodGet || method == http.MethodHead {
			action = "read"
		} else {
			action = "create"
		}
	default:
		return routePermission{}, false
	}
	return routePermission{Resource: resource, Action: action}, supportedResourceActions[resource][action]
}

func actionForMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return "read"
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return ""
	}
}

func mapReadOrUpdate(method string) string {
	if method == http.MethodGet || method == http.MethodHead {
		return "read"
	}
	return "update"
}

func containsActionSegment(path string, candidates ...string) bool {
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		for _, candidate := range candidates {
			if segment == candidate {
				return true
			}
		}
	}
	return false
}

func resolveProtectedResourceID(r *http.Request) string {
	for _, key := range []string{"id", "frameworkID", "riskId", "entityId", "articleId", "taskId", "assignmentId"} {
		if value := chi.URLParam(r, key); value != "" {
			return value
		}
	}
	// Group middleware runs before Chi has populated the leaf route's URL
	// parameters. UUIDs are the canonical entity identifiers, so recover the
	// first one from the already-normalized request path for the audit decision.
	for _, segment := range strings.Split(strings.Trim(r.URL.Path, "/"), "/") {
		if _, err := uuid.Parse(segment); err == nil {
			return segment
		}
	}
	return ""
}
