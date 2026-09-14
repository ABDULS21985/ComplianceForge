package handler

import (
	"encoding/json"
	"testing"
)

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
