package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/config"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const supportHandlerOrg = "20000000-0000-0000-0000-000000000001"

type supportBundleHandlerStub struct {
	artifact *models.SupportBundleArtifact
	err      error
	calls    int
	org      string
	actor    string
	request  string
	consent  models.SupportBundleRequest
}

func (s *supportBundleHandlerStub) Generate(_ context.Context, org, actor, request string, consent models.SupportBundleRequest) (*models.SupportBundleArtifact, error) {
	s.calls++
	s.org, s.actor, s.request, s.consent = org, actor, request, consent
	return s.artifact, s.err
}

type supportBundleAuditFixture struct{}

func (supportBundleAuditFixture) RecordSupportBundleGeneration(context.Context, models.SupportBundleAudit) error {
	return nil
}

func supportHandlerArtifact(t *testing.T) *models.SupportBundleArtifact {
	t.Helper()
	snapshots := &diagnosticsHandlerServiceStub{result: &models.DiagnosticsSnapshot{OrganizationID: supportHandlerOrg}}
	svc, err := service.NewSupportBundleService(snapshots, supportBundleAuditFixture{}, &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := svc.Generate(context.Background(), supportHandlerOrg, "10000000-0000-0000-0000-000000000001", "", models.SupportBundleRequest{Consent: true, Scope: models.SupportBundleScope})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func supportHandlerRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/settings/diagnostics/support-bundle", strings.NewReader(body))
	r = r.WithContext(diagnosticsHandlerRequest().Context())
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestSupportBundleHandlerDownloadsOnlyExplicitConsentedArtifact(t *testing.T) {
	artifact := supportHandlerArtifact(t)
	provider := &supportBundleHandlerStub{artifact: artifact}
	handler := NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{}, WithSupportBundleService(provider))
	request := supportHandlerRequest(`{"consent":true,"scope":"health_and_posture"}`)
	requestID := uuid.NewString()
	request = request.WithContext(context.WithValue(request.Context(), middleware.ContextKeyRequestID, requestID))
	response := httptest.NewRecorder()
	handler.GenerateSupportBundle(response, request)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), artifact.Bytes) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/octet-stream" || response.Header().Get("Cache-Control") != "private, no-store" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("X-Support-Bundle-SHA256") != artifact.SHA256 ||
		!strings.Contains(response.Header().Get("Content-Disposition"), artifact.ID+".zip") {
		t.Fatalf("unsafe download headers=%v", response.Header())
	}
	if provider.calls != 1 || provider.org != supportHandlerOrg || provider.actor != "10000000-0000-0000-0000-000000000001" || provider.request != requestID || !provider.consent.Consent {
		t.Fatalf("wrong trusted generation context=%+v", provider)
	}
}

func TestSupportBundleHandlerRejectsInvalidBodyBeforeGeneration(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"empty", "", http.StatusBadRequest},
		{"no consent", `{"scope":"health_and_posture"}`, http.StatusBadRequest},
		{"false consent", `{"consent":false,"scope":"health_and_posture"}`, http.StatusBadRequest},
		{"null consent", `{"consent":null,"scope":"health_and_posture"}`, http.StatusBadRequest},
		{"wrong scope", `{"consent":true,"scope":"raw_logs"}`, http.StatusBadRequest},
		{"no scope", `{"consent":true}`, http.StatusBadRequest},
		{"org injection", `{"consent":true,"scope":"health_and_posture","organization_id":"other"}`, http.StatusBadRequest},
		{"actor injection", `{"consent":true,"scope":"health_and_posture","actor_id":"other"}`, http.StatusBadRequest},
		{"array", `[]`, http.StatusBadRequest},
		{"invalid bool", `{"consent":"true","scope":"health_and_posture"}`, http.StatusBadRequest},
		{"trailing value", `{"consent":true,"scope":"health_and_posture"}{}`, http.StatusBadRequest},
		{"oversized", `{"consent":true,"scope":"health_and_posture"}` + strings.Repeat(" ", maximumSupportBundleRequestBytes), http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &supportBundleHandlerStub{}
			response := httptest.NewRecorder()
			NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{}, WithSupportBundleService(provider)).GenerateSupportBundle(response, supportHandlerRequest(test.body))
			if response.Code != test.status || provider.calls != 0 || response.Header().Get("X-Support-Bundle-SHA256") != "" {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.calls, response.Body.String())
			}
		})
	}
}

func TestSupportBundleHandlerEnforcesAuthenticationAndMediaType(t *testing.T) {
	for _, test := range []struct {
		name   string
		modify func(*http.Request) *http.Request
		status int
	}{
		{"no auth", func(r *http.Request) *http.Request { return r.WithContext(context.Background()) }, http.StatusUnauthorized},
		{"no actor", func(r *http.Request) *http.Request {
			return r.WithContext(context.WithValue(r.Context(), middleware.ContextKeyUserID, ""))
		}, http.StatusUnauthorized},
		{"no org", func(r *http.Request) *http.Request {
			return r.WithContext(context.WithValue(r.Context(), middleware.ContextKeyOrgID, ""))
		}, http.StatusUnauthorized},
		{"no media type", func(r *http.Request) *http.Request { r.Header.Del("Content-Type"); return r }, http.StatusBadRequest},
		{"text", func(r *http.Request) *http.Request { r.Header.Set("Content-Type", "text/plain"); return r }, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &supportBundleHandlerStub{}
			response := httptest.NewRecorder()
			NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{}, WithSupportBundleService(provider)).GenerateSupportBundle(response,
				test.modify(supportHandlerRequest(`{"consent":true,"scope":"health_and_posture"}`)))
			if response.Code != test.status || provider.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, provider.calls, response.Body.String())
			}
		})
	}
}

func TestSupportBundleHandlerFailsClosedOnMissingDeniedAndRestrictiveObligations(t *testing.T) {
	for _, name := range []string{"missing", "denied", "hidden", "masked", "malformed", "watermark"} {
		t.Run(name, func(t *testing.T) {
			provider := &supportBundleHandlerStub{artifact: supportHandlerArtifact(t)}
			request := supportHandlerRequest(`{"consent":true,"scope":"health_and_posture"}`)
			ctx := request.Context()
			switch name {
			case "missing":
				ctx = context.WithValue(context.Background(), middleware.ContextKeyOrgID, supportHandlerOrg)
				ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "10000000-0000-0000-0000-000000000001")
			case "denied":
				ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{})
			case "hidden", "masked":
				visibility, strategy := models.AccessFieldHidden, models.AccessMaskStrategy("")
				if name == "masked" {
					visibility, strategy = models.AccessFieldMasked, models.AccessMaskRedact
				}
				ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true,
					Obligations: service.AccessFieldObligations([]models.AccessFieldPermission{classifiedRule("settings", "queue", visibility, strategy)})})
			case "malformed":
				ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true, Obligations: []authz.Obligation{{Kind: service.AccessFieldVisibilityObligation}}})
			case "watermark":
				ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true, Obligations: []authz.Obligation{{Kind: "watermark"}}})
			}
			response := httptest.NewRecorder()
			NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{}, WithSupportBundleService(provider)).GenerateSupportBundle(response, request.WithContext(ctx))
			if response.Code != http.StatusServiceUnavailable || provider.calls != 0 || response.Header().Get("Content-Disposition") != "" || response.Header().Get("X-Support-Bundle-SHA256") != "" {
				t.Fatalf("classification did not fail closed: status=%d calls=%d headers=%v", response.Code, provider.calls, response.Header())
			}
		})
	}
}

func TestSupportBundleHandlerRedactsErrorsAndRejectsMissingProviders(t *testing.T) {
	var nilProvider *supportBundleHandlerStub
	for _, provider := range []SupportBundleService{
		nil, nilProvider, &supportBundleHandlerStub{err: errors.New("postgres://SUPPORT_SECRET@internal")},
		&supportBundleHandlerStub{}, &supportBundleHandlerStub{artifact: &models.SupportBundleArtifact{ID: "../secret", Bytes: []byte("PK\x03\x04secret")}},
	} {
		response := httptest.NewRecorder()
		NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{}, WithSupportBundleService(provider)).GenerateSupportBundle(response,
			supportHandlerRequest(`{"consent":true,"scope":"health_and_posture"}`))
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "SUPPORT_SECRET") || response.Header().Get("Content-Disposition") != "" {
			t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
		}
	}
}

func TestSupportArtifactBoundaryRejectsTenantMismatchAndTamperedMembers(t *testing.T) {
	artifact := supportHandlerArtifact(t)
	if !validSupportBundleArtifact(artifact, supportHandlerOrg) || validSupportBundleArtifact(artifact, uuid.NewString()) {
		t.Fatal("artifact boundary did not bind the tenant")
	}
	reader, err := zip.NewReader(bytes.NewReader(artifact.Bytes), int64(len(artifact.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	members := make(map[string][]byte)
	for _, file := range reader.File {
		entry, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(entry)
		_ = entry.Close()
		if err != nil {
			t.Fatal(err)
		}
		members[file.Name] = data
	}
	members["health.json"] = []byte(`{"organization_id":"tampered"}`)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(members[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buffer.Bytes())
	tampered := &models.SupportBundleArtifact{ID: artifact.ID, SHA256: hex.EncodeToString(sum[:]), Bytes: buffer.Bytes()}
	if validSupportBundleArtifact(tampered, supportHandlerOrg) {
		t.Fatal("artifact accepted altered member even with recomputed archive hash")
	}
	var manifest models.SupportBundleManifest
	if json.Unmarshal(members["manifest.json"], &manifest) != nil {
		t.Fatal("invalid original manifest")
	}
}

func supportHandlerMembers(t *testing.T, artifact *models.SupportBundleArtifact) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(artifact.Bytes), int64(len(artifact.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	members := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		entry, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(entry)
		closeErr := entry.Close()
		if err != nil || closeErr != nil {
			t.Fatal(errors.Join(err, closeErr))
		}
		members[file.Name] = data
	}
	return members
}

func supportHandlerEncode(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func supportHandlerReseal(t *testing.T, members map[string][]byte) {
	t.Helper()
	var manifest models.SupportBundleManifest
	if err := json.Unmarshal(members["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Files {
		data := members[manifest.Files[i].Name]
		sum := sha256.Sum256(data)
		manifest.Files[i].Bytes = len(data)
		manifest.Files[i].SHA256 = hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(members["configuration.json"])
	manifest.ConfigurationFingerprint = hex.EncodeToString(sum[:])
	members["manifest.json"] = supportHandlerEncode(t, manifest)
}

func supportHandlerRebuild(t *testing.T, id string, members map[string][]byte, comment string, extra []byte) *models.SupportBundleArtifact {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if err := writer.SetComment(comment); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"configuration.json", "health.json", "README.txt", "manifest.json"} {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Extra: extra,
			Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(members[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buffer.Bytes())
	return &models.SupportBundleArtifact{ID: id, SHA256: hex.EncodeToString(sum[:]), Bytes: buffer.Bytes()}
}

func TestSupportArtifactBoundaryRejectsRehashedUnreviewedMetadata(t *testing.T) {
	original := supportHandlerArtifact(t)
	for _, test := range []struct {
		name           string
		mutate         func(*testing.T, map[string][]byte)
		mutateManifest func(*testing.T, map[string][]byte)
		comment        string
		extra          []byte
	}{
		{name: "README injection", mutate: func(_ *testing.T, members map[string][]byte) {
			members["README.txt"] = append(members["README.txt"], []byte("SUPPORT_SECRET")...)
		}},
		{name: "unknown health field", mutate: func(_ *testing.T, members map[string][]byte) {
			members["health.json"] = bytes.Replace(members["health.json"], []byte("{\n"), []byte("{\n  \"provider_error\": \"SUPPORT_SECRET\",\n"), 1)
		}},
		{name: "ignored duplicate key", mutate: func(_ *testing.T, members map[string][]byte) {
			members["health.json"] = bytes.Replace(members["health.json"], []byte("{\n"), []byte("{\n  \"overall_status\": \"SUPPORT_SECRET\",\n"), 1)
		}},
		{name: "wrong tenant with valid hashes", mutate: func(t *testing.T, members map[string][]byte) {
			var health models.SupportBundleHealth
			if err := json.Unmarshal(members["health.json"], &health); err != nil {
				t.Fatal(err)
			}
			health.OrganizationID = uuid.NewString()
			members["health.json"] = supportHandlerEncode(t, health)
		}},
		{name: "unreviewed dependency", mutate: func(t *testing.T, members map[string][]byte) {
			var health models.SupportBundleHealth
			if err := json.Unmarshal(members["health.json"], &health); err != nil {
				t.Fatal(err)
			}
			health.Dependencies = append(health.Dependencies, models.SupportDependency{Key: "SUPPORT_SECRET", Status: models.DiagnosticStatusHealthy})
			members["health.json"] = supportHandlerEncode(t, health)
		}},
		{name: "negative health aggregate", mutate: func(t *testing.T, members map[string][]byte) {
			var health models.SupportBundleHealth
			if err := json.Unmarshal(members["health.json"], &health); err != nil {
				t.Fatal(err)
			}
			health.Queue.Pending = -1
			members["health.json"] = supportHandlerEncode(t, health)
		}},
		{name: "unknown configuration field", mutate: func(_ *testing.T, members map[string][]byte) {
			members["configuration.json"] = bytes.Replace(members["configuration.json"], []byte("{\n"), []byte("{\n  \"connection_string\": \"SUPPORT_SECRET\",\n"), 1)
		}},
		{name: "configuration build label", mutate: func(t *testing.T, members map[string][]byte) {
			var configuration models.SupportBundleConfiguration
			if err := json.Unmarshal(members["configuration.json"], &configuration); err != nil {
				t.Fatal(err)
			}
			configuration.ServiceVersion = "1.2.3+SUPPORT_SECRET"
			members["configuration.json"] = supportHandlerEncode(t, configuration)
		}},
		{name: "unbounded configuration budget", mutate: func(t *testing.T, members map[string][]byte) {
			var configuration models.SupportBundleConfiguration
			if err := json.Unmarshal(members["configuration.json"], &configuration); err != nil {
				t.Fatal(err)
			}
			configuration.DatabaseMaximumConns = 100_001
			members["configuration.json"] = supportHandlerEncode(t, configuration)
		}},
		{name: "duplicate posture", mutate: func(t *testing.T, members map[string][]byte) {
			var configuration models.SupportBundleConfiguration
			if err := json.Unmarshal(members["configuration.json"], &configuration); err != nil {
				t.Fatal(err)
			}
			configuration.Posture[1] = configuration.Posture[0]
			members["configuration.json"] = supportHandlerEncode(t, configuration)
		}},
		{name: "unreviewed manifest exclusions", mutateManifest: func(t *testing.T, members map[string][]byte) {
			var manifest models.SupportBundleManifest
			if err := json.Unmarshal(members["manifest.json"], &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.Excluded = append(manifest.Excluded, "SUPPORT_SECRET")
			members["manifest.json"] = supportHandlerEncode(t, manifest)
		}},
		{name: "unknown manifest field", mutateManifest: func(_ *testing.T, members map[string][]byte) {
			members["manifest.json"] = bytes.Replace(members["manifest.json"], []byte("{\n"), []byte("{\n  \"operator_note\": \"SUPPORT_SECRET\",\n"), 1)
		}},
		{name: "ZIP comment", comment: "SUPPORT_SECRET"},
		{name: "ZIP extra field", extra: []byte{0xfe, 0xca, 3, 0, 'P', 'I', 'I'}},
	} {
		t.Run(test.name, func(t *testing.T) {
			members := supportHandlerMembers(t, original)
			if test.mutate != nil {
				test.mutate(t, members)
			}
			supportHandlerReseal(t, members)
			if test.mutateManifest != nil {
				test.mutateManifest(t, members)
			}
			artifact := supportHandlerRebuild(t, original.ID, members, test.comment, test.extra)
			if validSupportBundleArtifact(artifact, supportHandlerOrg) {
				t.Fatal("accepted unreviewed metadata even with valid recomputed hashes")
			}
			provider := &supportBundleHandlerStub{artifact: artifact}
			response := httptest.NewRecorder()
			NewDiagnosticsHandler(&diagnosticsHandlerServiceStub{}, WithSupportBundleService(provider)).GenerateSupportBundle(response,
				supportHandlerRequest(`{"consent":true,"scope":"health_and_posture"}`))
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("X-Support-Bundle-SHA256") != "" ||
				response.Header().Get("Content-Disposition") != "" || strings.Contains(response.Body.String(), "SUPPORT_SECRET") {
				t.Fatalf("unsafe failure status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
			}
		})
	}
	unchanged := supportHandlerMembers(t, original)
	supportHandlerReseal(t, unchanged)
	if !validSupportBundleArtifact(supportHandlerRebuild(t, original.ID, unchanged, "", nil), supportHandlerOrg) {
		t.Fatal("unchanged canonical reconstruction was rejected")
	}
	trailing := &models.SupportBundleArtifact{ID: original.ID, Bytes: append(bytes.Clone(original.Bytes), []byte("SUPPORT_SECRET")...)}
	sum := sha256.Sum256(trailing.Bytes)
	trailing.SHA256 = hex.EncodeToString(sum[:])
	if validSupportBundleArtifact(trailing, supportHandlerOrg) {
		t.Fatal("accepted data after ZIP end-of-directory")
	}
}
