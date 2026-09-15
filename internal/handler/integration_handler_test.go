package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type integrationSyncServiceStub struct {
	IntegrationSvc
	result *service.SyncLog
}

func (stub integrationSyncServiceStub) TriggerSync(context.Context, string, string, string) (*service.SyncLog, error) {
	return stub.result, nil
}

func TestTriggerSyncUsesStandardAsyncJobEnvelope(t *testing.T) {
	createdAt := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	syncID := "10000000-0000-0000-0000-000000000010"
	integrationID := "10000000-0000-0000-0000-000000000020"
	handler := NewIntegrationHandler(integrationSyncServiceStub{result: &service.SyncLog{
		ID: syncID, IntegrationID: integrationID, SyncType: "full", Status: "started", CreatedAt: createdAt,
	}})
	router := chi.NewRouter()
	router.Post("/integrations/{id}/sync", handler.TriggerSync)

	request := httptest.NewRequest(http.MethodPost, "/integrations/"+integrationID+"/sync", nil)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, "10000000-0000-0000-0000-000000000001")
	request = request.WithContext(handlerAllowedContext(ctx))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data service.SyncLog `json:"data"`
		Job  models.AsyncJob `json:"job"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.ID != syncID || payload.Job.ID != syncID || payload.Job.Type != "integration_sync" ||
		payload.Job.Status != "started" || !payload.Job.SubmittedAt.Equal(createdAt) {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestNormalizeIntegrationPayloadSupportsCanonicalAndLegacyShape(t *testing.T) {
	tests := []struct {
		name    string
		payload integrationPayload
	}{
		{
			name: "canonical",
			payload: integrationPayload{
				IntegrationType: "cloud_aws",
				Name:            "AWS production",
				Configuration:   json.RawMessage(`{"region":"eu-west-1"}`),
			},
		},
		{
			name: "frontend legacy alias",
			payload: integrationPayload{
				Type:   "aws",
				Name:   "AWS production",
				Config: json.RawMessage(`{"region":"eu-west-1"}`),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			integration, configuration, err := normalizeIntegrationPayload(test.payload, true)
			if err != nil {
				t.Fatalf("normalizeIntegrationPayload() error = %v", err)
			}
			if integration.IntegrationType != "cloud_aws" || integration.Name != "AWS production" {
				t.Fatalf("integration = %#v", integration)
			}
			if configuration == nil || *configuration != `{"region":"eu-west-1"}` {
				t.Fatalf("configuration = %#v", configuration)
			}
		})
	}
}

func TestNormalizeIntegrationPayloadRejectsUnsafeInput(t *testing.T) {
	tests := []integrationPayload{
		{Type: "unknown", Name: "test", Config: json.RawMessage(`{}`)},
		{Type: "aws", Name: "test", Config: json.RawMessage(`[]`)},
		{Type: "aws", Name: "test", Config: json.RawMessage(`not-json`)},
		{Type: "aws", Name: "test", Config: json.RawMessage(`{}`), Capabilities: []string{"Read Everything"}},
	}
	for _, payload := range tests {
		if _, _, err := normalizeIntegrationPayload(payload, true); err == nil {
			t.Fatalf("normalizeIntegrationPayload(%#v) unexpectedly succeeded", payload)
		}
	}
}

func TestNormalizeConfigurationJSONAcceptsStringEncodedLegacyPayload(t *testing.T) {
	configuration, err := normalizeConfigurationJSON(json.RawMessage(`"{\"token\":\"secret\"}"`))
	if err != nil {
		t.Fatalf("normalizeConfigurationJSON() error = %v", err)
	}
	if configuration == nil || *configuration != `{"token":"secret"}` {
		t.Fatalf("configuration = %#v", configuration)
	}
}

func TestValidateAPIKeyPermissions(t *testing.T) {
	if err := validateAPIKeyPermissions([]string{"read:controls", "export:reports"}); err != nil {
		t.Fatalf("valid permissions rejected: %v", err)
	}
	invalid := [][]string{
		nil,
		{"controls:read"},
		{"admin:controls"},
		{"read:Controls"},
	}
	for _, permissions := range invalid {
		if err := validateAPIKeyPermissions(permissions); err == nil {
			t.Fatalf("permissions %#v unexpectedly accepted", permissions)
		}
	}
}
