package openapi

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestAccessReviewReplayUsesBodyRequestIDNotImportHeader(t *testing.T) {
	root := repositoryRoot(t)
	generated, err := Generate(filepath.Join(root, "api/openapi/base.json"), filepath.Join(root, "api/openapi/routes.csv"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(generated, &document); err != nil {
		t.Fatal(err)
	}
	op := operation(document, "/api/v1/access/governance/campaigns/{id}/items/{itemID}/decision", "post")
	for _, parameter := range op["parameters"].([]any) {
		if parameter.(map[string]any)["$ref"] == "#/components/parameters/IdempotencyKey" {
			t.Fatal("body-request-id replay must not falsely require the import-only header")
		}
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	required := schemas["AccessReviewDecisionInput"].(map[string]any)["required"].([]any)
	found := false
	for _, field := range required {
		if field == "request_id" {
			found = true
		}
	}
	if !found {
		t.Fatal("decision must require body request_id")
	}
	window := schemas["ManagedRoleWindowInput"].(map[string]any)
	found = false
	for _, field := range window["required"].([]any) {
		if field == "assignment_id" {
			found = true
		}
	}
	if !found {
		t.Fatal("window update must bind the assignment incarnation")
	}
}
