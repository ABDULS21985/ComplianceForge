package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckedInContractIsDeterministicAndValid(t *testing.T) {
	root := repositoryRoot(t)
	base := filepath.Join(root, "api", "openapi", "base.json")
	routes := filepath.Join(root, "api", "openapi", "routes.csv")
	artifact := filepath.Join(root, "api", "openapi", "openapi.json")

	generated, err := Generate(base, routes)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	committed, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatalf("read generated contract: %v", err)
	}
	if !bytes.Equal(generated, committed) {
		t.Fatal("generated contract is stale; run `go run ./cmd/openapi generate`")
	}
	loaded, err := Load(context.Background(), artifact)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := len(loaded.Operations), 392; got != want {
		t.Fatalf("operation count = %d, want %d", got, want)
	}
}

func TestGenerateDocumentsSCIMProtocolIsolationAndConcurrency(t *testing.T) {
	root := repositoryRoot(t)
	generated, err := Generate(filepath.Join(root, "api/openapi/base.json"), filepath.Join(root, "api/openapi/routes.csv"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(generated, &document); err != nil {
		t.Fatal(err)
	}
	create := operation(document, "/api/scim/v2/Users", "post")
	security := create["security"].([]any)[0].(map[string]any)
	if security["scimBearerAuth"] == nil || security["bearerAuth"] != nil || security["apiKeyAuth"] != nil {
		t.Fatalf("SCIM security=%#v", security)
	}
	requestContent := create["requestBody"].(map[string]any)["content"].(map[string]any)
	if requestContent["application/scim+json"] == nil || requestContent["application/json"] != nil {
		t.Fatalf("SCIM request content=%#v", requestContent)
	}
	patch := operation(document, "/api/scim/v2/Users/{id}", "patch")
	parameters := patch["parameters"].([]any)
	foundIfMatch := false
	for _, parameter := range parameters {
		if parameter.(map[string]any)["name"] == "If-Match" {
			foundIfMatch = true
		}
	}
	if !foundIfMatch || patch["responses"].(map[string]any)["412"] == nil || patch["responses"].(map[string]any)["428"] == nil {
		t.Fatalf("SCIM optimistic concurrency contract is incomplete: parameters=%#v", parameters)
	}
}

func TestLoadRejectsMissingAndDuplicateOperationIDs(t *testing.T) {
	artifact := filepath.Join(repositoryRoot(t), "api", "openapi", "openapi.json")
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "missing",
			mutate: func(document map[string]any) {
				operation(document, "/health/live", "get")["operationId"] = ""
			},
			want: "operationId",
		},
		{
			name: "duplicate",
			mutate: func(document map[string]any) {
				operation(document, "/health/live", "get")["operationId"] = operation(document, "/health/ready", "get")["operationId"]
			},
			want: "same operation id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyData := append([]byte(nil), data...)
			var document map[string]any
			if err := json.Unmarshal(copyData, &document); err != nil {
				t.Fatal(err)
			}
			test.mutate(document)
			invalid, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(t.TempDir(), "openapi.json")
			if err := os.WriteFile(filename, invalid, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = Load(context.Background(), filename)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestGenerateRejectsUnsafeCatalogMetadata(t *testing.T) {
	base := filepath.Join(repositoryRoot(t), "api", "openapi", "base.json")
	tests := []struct {
		name string
		row  string
		want string
	}{
		{"mutable automation route", "POST,/api/v1/automation/risks,postAutomationRisks,automation,apiKey,GenericMutation,GenericResponse,200,feature-gated,", "read-only automation API mutable"},
		{"unknown flag", "GET,/health/live,getHealthLive,health,public,,HealthResponse,200,paginatted,", "unknown flag"},
		{"duplicate query", "GET,/api/v1/risks,getRisks,risks,bearer,,RiskList,200,paginated,status:string:false;status:string:false", "duplicate query parameter"},
		{"conflicting multipart flags", "POST,/api/v1/files,postFiles,controls,bearer,ControlEvidenceCreate,ControlEvidence,201,multipart-body;multipart-only,", "cannot combine multipart-body and multipart-only"},
		{"multipart without schema", "POST,/api/v1/files,postFiles,controls,bearer,,ControlEvidence,201,multipart-only,", "multipart transport without a request schema"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := filepath.Join(t.TempDir(), "routes.csv")
			if err := os.WriteFile(catalog, []byte(test.row+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Generate(base, catalog)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Generate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestGenerateDocumentsEvidenceTransportSecurity(t *testing.T) {
	root := repositoryRoot(t)
	generated, err := Generate(
		filepath.Join(root, "api", "openapi", "base.json"),
		filepath.Join(root, "api", "openapi", "routes.csv"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(generated, &document); err != nil {
		t.Fatal(err)
	}

	upload := operation(document, "/api/v1/controls/{id}/evidence", "post")
	requestContent := upload["requestBody"].(map[string]any)["content"].(map[string]any)
	for _, mediaType := range []string{"application/json", "multipart/form-data"} {
		if requestContent[mediaType] == nil {
			t.Fatalf("evidence upload omits %s request content", mediaType)
		}
	}
	if upload["responses"].(map[string]any)["413"] == nil {
		t.Fatal("evidence upload omits bounded-payload response")
	}

	download := operation(document, "/api/v1/controls/{id}/evidence/{evidenceID}/download", "get")
	if binary, _ := download["x-binary-response"].(bool); !binary {
		t.Fatal("evidence download is not marked as a binary response")
	}
	responses := download["responses"].(map[string]any)
	successContent := responses["200"].(map[string]any)["content"].(map[string]any)
	if successContent["application/octet-stream"] == nil {
		t.Fatal("evidence download omits opaque binary content")
	}
	redirectHeaders := responses["307"].(map[string]any)["headers"].(map[string]any)
	if redirectHeaders["Location"] == nil || redirectHeaders["Referrer-Policy"] == nil {
		t.Fatal("evidence signed-download redirect omits security headers")
	}
}

func TestGenerateDocumentsMultipartOnlyTransport(t *testing.T) {
	root := repositoryRoot(t)
	catalog := filepath.Join(t.TempDir(), "routes.csv")
	row := "POST,/api/v1/test-evidence,postTestEvidence,controls,bearer,ControlEvidenceCreate,ControlEvidence,201,multipart-only;payload-limited,\n"
	if err := os.WriteFile(catalog, []byte(row), 0o600); err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(filepath.Join(root, "api", "openapi", "base.json"), catalog)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(generated, &document); err != nil {
		t.Fatal(err)
	}
	requestContent := operation(document, "/api/v1/test-evidence", "post")["requestBody"].(map[string]any)["content"].(map[string]any)
	if requestContent["multipart/form-data"] == nil || len(requestContent) != 1 {
		t.Fatalf("multipart-only request content=%#v", requestContent)
	}
}

func operation(document map[string]any, path, method string) map[string]any {
	return document["paths"].(map[string]any)[path].(map[string]any)[method].(map[string]any)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
