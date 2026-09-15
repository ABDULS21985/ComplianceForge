package openapi

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSupportBundleDocumentsExactChecksumSizeAndSafeUnavailable(t *testing.T) {
	root := repositoryRoot(t)
	generated, err := Generate(filepath.Join(root, "api/openapi/base.json"), filepath.Join(root, "api/openapi/routes.csv"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(generated, &document); err != nil {
		t.Fatal(err)
	}
	op := operation(document, "/api/v1/settings/diagnostics/support-bundle", "post")
	responses := op["responses"].(map[string]any)
	headers := responses["200"].(map[string]any)["headers"].(map[string]any)
	checksum := headers["X-Support-Bundle-SHA256"].(map[string]any)["schema"].(map[string]any)
	if checksum["type"] != "string" || checksum["pattern"] != `^[0-9a-f]{64}$` {
		t.Fatalf("checksum schema=%v", checksum)
	}
	length := headers["Content-Length"].(map[string]any)["schema"].(map[string]any)
	if length["maximum"] != float64(65536) {
		t.Fatalf("Content-Length schema=%v", length)
	}
	if responses["503"].(map[string]any)["$ref"] != "#/components/responses/DependencyUnavailable" {
		t.Fatalf("503=%v", responses["503"])
	}
	// Do not impose a support-ZIP limit or checksum header on evidence downloads.
	download := operation(document, "/api/v1/controls/{id}/evidence/{evidenceID}/download", "get")
	downloadHeaders := download["responses"].(map[string]any)["200"].(map[string]any)["headers"].(map[string]any)
	if downloadHeaders["X-Support-Bundle-SHA256"] != nil {
		t.Fatal("support header leaked to evidence download")
	}
}
