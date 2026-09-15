package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
)

type classifiedFrameworkService struct {
	FrameworkService
	item *models.ComplianceFramework
}

func (s classifiedFrameworkService) GetFramework(context.Context, string, string) (*models.ComplianceFramework, error) {
	return s.item, nil
}

type classifiedPolicyService struct {
	PolicyService
	item *models.Policy
}

func (s classifiedPolicyService) GetByID(context.Context, string, string) (*models.Policy, error) {
	return s.item, nil
}

type classifiedAuditService struct {
	AuditService
	audit   *models.Audit
	finding *models.AuditFinding
}

func (s classifiedAuditService) GetByID(context.Context, string, string) (*models.Audit, error) {
	return s.audit, nil
}

func (s classifiedAuditService) GetFinding(context.Context, string, string, string) (*models.AuditFinding, error) {
	return s.finding, nil
}

type classifiedAssetService struct {
	AssetManagementService
	item *models.Asset
}

func (s classifiedAssetService) GetByID(context.Context, string, string) (*models.Asset, error) {
	return s.item, nil
}

type classifiedReportEngine struct {
	models.ReportEngine
	definitions []models.ReportDefinition
	file        *models.ReportFile
}

func (s classifiedReportEngine) ListDefinitions(context.Context, string, models.PaginationRequest) ([]models.ReportDefinition, int, error) {
	return s.definitions, len(s.definitions), nil
}

func (s classifiedReportEngine) DownloadReport(context.Context, string, string) (*models.ReportFile, error) {
	return s.file, nil
}

type classifiedDiagnosticsService struct {
	item *models.DiagnosticsSnapshot
}

func (s classifiedDiagnosticsService) GetSnapshot(context.Context, string) (*models.DiagnosticsSnapshot, error) {
	return s.item, nil
}

func TestRemainingClassifiedDomainHandlersMaskNestedValuesWithoutMutation(t *testing.T) {
	frameworkDescription := "Framework acquisition notes"
	framework := &models.ComplianceFramework{
		BaseModel:   models.BaseModel{ID: classifiedItemID},
		Code:        "PRIVATE-FW",
		Name:        "Private framework",
		Description: &frameworkDescription,
		Metadata:    []byte(`{"owner":{"email":"framework.private@example.test"},"source":"catalog"}`),
	}
	policy := &models.Policy{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		Title:       "Unreleased acquisition policy",
		Metadata:    []byte(`{"legal":{"contact":"legal.private@example.test"},"stage":"draft"}`),
	}
	audit := &models.Audit{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		Title:       "Acquisition audit",
		Scope:       "Privileged transaction scope",
		Metadata:    []byte(`{"team":{"email":"audit.private@example.test"},"region":"west"}`),
	}
	finding := &models.AuditFinding{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		AuditID:     classifiedItemID,
		Title:       "Private control gap",
		Description: "Credentials exposed in legacy system",
		Metadata:    []byte(`{"remediation":{"owner":{"email":"remediation.private@example.test"}},"stage":"open"}`),
	}
	assetIP := "10.42.0.9"
	asset := &models.Asset{
		TenantModel: models.TenantModel{BaseModel: models.BaseModel{ID: classifiedItemID}, OrganizationID: classifiedOrgID},
		Name:        "Production customer database",
		IPAddress:   &assetIP,
		Owner:       &models.AssetPerson{ID: classifiedUserID, Email: "asset.private@example.test"},
	}
	report := models.ReportDefinition{
		ID: classifiedItemID, OrganizationID: classifiedOrgID, Name: "Board investigation report",
		Parameters: map[string]any{
			"delivery": map[string]any{"recipient": "board.private@example.test"},
			"format":   "pdf",
		},
	}
	settings := &models.DiagnosticsSnapshot{
		OrganizationID: classifiedOrgID,
		Dependencies: []models.DependencyDiagnostic{{
			Key: "database", Name: "PostgreSQL", Message: "primary.internal.example.test",
		}},
		Configuration: []models.ConfigurationDiagnostic{{
			Key: "support", Message: "Configured", Remediation: "ops.private@example.test",
		}},
	}

	tests := []struct {
		name      string
		resource  string
		handler   http.Handler
		rules     []models.AccessFieldPermission
		secrets   []string
		expected  []string
		unchanged func() bool
	}{
		{
			name: "frameworks", resource: "frameworks",
			handler: http.HandlerFunc(NewFrameworkHandler(classifiedFrameworkService{item: framework}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("frameworks", "description", models.AccessFieldHidden, ""),
				classifiedRule("frameworks", "metadata.owner.email", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{frameworkDescription, "framework.private@example.test"},
			expected: []string{"f***@example.test", `"source":"catalog"`},
			unchanged: func() bool {
				return *framework.Description == frameworkDescription && strings.Contains(string(framework.Metadata), "framework.private@example.test")
			},
		},
		{
			name: "policies", resource: "policies",
			handler: http.HandlerFunc(NewPolicyHandler(classifiedPolicyService{item: policy}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("policies", "title", models.AccessFieldHidden, ""),
				classifiedRule("policies", "metadata.legal.contact", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{policy.Title, "legal.private@example.test"},
			expected: []string{"l***@example.test", `"stage":"draft"`},
			unchanged: func() bool {
				return policy.Title == "Unreleased acquisition policy" && strings.Contains(string(policy.Metadata), "legal.private@example.test")
			},
		},
		{
			name: "audits", resource: "audits",
			handler: http.HandlerFunc(NewAuditHandler(classifiedAuditService{audit: audit}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("audits", "scope", models.AccessFieldHidden, ""),
				classifiedRule("audits", "metadata.team.email", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{audit.Scope, "audit.private@example.test"},
			expected: []string{"a***@example.test", `"region":"west"`},
			unchanged: func() bool {
				return audit.Scope == "Privileged transaction scope" && strings.Contains(string(audit.Metadata), "audit.private@example.test")
			},
		},
		{
			// Finding routes currently receive the parent audits decision. This
			// test locks in safe serialization without pretending a standalone
			// findings policy reached the handler.
			name: "audit findings", resource: "audits",
			handler: http.HandlerFunc(NewAuditHandler(classifiedAuditService{finding: finding}).GetFinding),
			rules: []models.AccessFieldPermission{
				classifiedRule("audits", "description", models.AccessFieldHidden, ""),
				classifiedRule("audits", "metadata.remediation.owner.email", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{finding.Description, "remediation.private@example.test"},
			expected: []string{"r***@example.test", `"stage":"open"`},
			unchanged: func() bool {
				return finding.Description == "Credentials exposed in legacy system" && strings.Contains(string(finding.Metadata), "remediation.private@example.test")
			},
		},
		{
			name: "assets", resource: "assets",
			handler: http.HandlerFunc(NewAssetHandler(classifiedAssetService{item: asset}).GetByID),
			rules: []models.AccessFieldPermission{
				classifiedRule("assets", "ip_address", models.AccessFieldHidden, ""),
				classifiedRule("assets", "owner.email", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{assetIP, "asset.private@example.test"},
			expected: []string{"a***@example.test", "Production customer database"},
			unchanged: func() bool {
				return *asset.IPAddress == assetIP && asset.Owner.Email == "asset.private@example.test"
			},
		},
		{
			name: "reports", resource: "reports",
			handler: http.HandlerFunc(NewReportHandler(classifiedReportEngine{definitions: []models.ReportDefinition{report}}).ListDefinitions),
			rules: []models.AccessFieldPermission{
				classifiedRule("reports", "name", models.AccessFieldHidden, ""),
				classifiedRule("reports", "parameters.delivery.recipient", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{report.Name, "board.private@example.test"},
			expected: []string{"b***@example.test", `"format":"pdf"`, `"pagination"`},
			unchanged: func() bool {
				return report.Name == "Board investigation report" && report.Parameters["delivery"].(map[string]any)["recipient"] == "board.private@example.test"
			},
		},
		{
			name: "settings", resource: "settings",
			handler: http.HandlerFunc(NewDiagnosticsHandler(classifiedDiagnosticsService{item: settings}).GetSnapshot),
			rules: []models.AccessFieldPermission{
				classifiedRule("settings", "dependencies.message", models.AccessFieldHidden, ""),
				classifiedRule("settings", "configuration.remediation", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"primary.internal.example.test", "ops.private@example.test"},
			expected: []string{"o***@example.test", `"name":"PostgreSQL"`},
			unchanged: func() bool {
				return settings.Dependencies[0].Message == "primary.internal.example.test" && settings.Configuration[0].Remediation == "ops.private@example.test"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveClassifiedHandler(test.resource, test.rules, test.handler)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertClassifiedValues(t, response, test.secrets, test.expected)
			if !test.unchanged() {
				t.Fatal("handler mutated the shared model returned by its service")
			}

			response = serveWithoutAuthorizationDecision(test.handler)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("missing-decision status=%d body=%s", response.Code, response.Body.String())
			}
			assertClassifiedValues(t, response, test.secrets, nil)
		})
	}
}

func TestReportAttachmentFailsClosedWhenFieldMaskingIsRequired(t *testing.T) {
	secret := []byte("classified-report-content")
	handler := http.HandlerFunc(NewReportHandler(classifiedReportEngine{file: &models.ReportFile{
		FileName: "board-report.pdf", ContentType: "application/pdf", Data: secret,
	}}).DownloadReport)
	response := serveClassifiedHandler("reports", []models.AccessFieldPermission{
		classifiedRule("reports", "title", models.AccessFieldHidden, ""),
	}, handler)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), string(secret)) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if disposition := response.Header().Get("Content-Disposition"); disposition != "" {
		t.Fatalf("attachment header was emitted before masking decision: %q", disposition)
	}

	response = serveClassifiedHandler("reports", nil, handler)
	if response.Code != http.StatusOK || response.Body.String() != string(secret) {
		t.Fatalf("unrestricted attachment status=%d body=%q", response.Code, response.Body.String())
	}
}

func serveWithoutAuthorizationDecision(next http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/classified/"+classifiedItemID, nil)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, classifiedOrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, classifiedUserID)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", classifiedItemID)
	route.URLParams.Add("findingID", classifiedItemID)
	request = request.WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	response := httptest.NewRecorder()
	next.ServeHTTP(response, request)
	return response
}

func assertClassifiedValues(t *testing.T, response *httptest.ResponseRecorder, secrets, expected []string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("classified value %q leaked: %s", secret, response.Body.String())
		}
	}
	for _, value := range expected {
		if !strings.Contains(response.Body.String(), value) {
			t.Fatalf("expected %q in %s", value, response.Body.String())
		}
	}
}
