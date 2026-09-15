package service

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/models"
)

func TestMaskAccessFieldsIsRecursiveTypeAwareAndNonMutating(t *testing.T) {
	stamp := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	input := map[string]any{
		"contact": map[string]any{"email": "josé@example.test", "legal_note": "privileged"},
		"accounts": []any{
			map[string]any{"number": "123456789", "balance": 92.50, "active": true},
			map[string]any{"number": "9876", "balance": 14, "active": true},
		},
		"generated_at": stamp,
	}
	permissions := []models.AccessFieldPermission{
		{FieldPath: "contact.email", Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskEmail},
		{FieldPath: "contact.legal_note", Visibility: models.AccessFieldHidden},
		{FieldPath: "accounts.number", Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskLast4},
		{FieldPath: "accounts.balance", Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskRedact},
		{FieldPath: "accounts.active", Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskRedact},
		{FieldPath: "generated_at", Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskRedact},
	}

	masked := MaskAccessFields(input, permissions)
	contact := masked["contact"].(map[string]any)
	if contact["email"] != "j***@example.test" {
		t.Fatalf("email=%q", contact["email"])
	}
	if _, exists := contact["legal_note"]; exists {
		t.Fatal("hidden nested field remains")
	}
	accounts := masked["accounts"].([]any)
	first := accounts[0].(map[string]any)
	second := accounts[1].(map[string]any)
	if first["number"] != "*****6789" || second["number"] != "****" {
		t.Fatalf("masked account numbers=%q/%q", first["number"], second["number"])
	}
	if _, ok := first["balance"].(float64); !ok || first["balance"] != float64(0) {
		t.Fatalf("balance=%#v does not preserve float type", first["balance"])
	}
	if _, ok := second["balance"].(int); !ok || second["balance"] != 0 {
		t.Fatalf("balance=%#v does not preserve int type", second["balance"])
	}
	if first["active"] != false {
		t.Fatalf("active=%#v", first["active"])
	}
	if got, ok := masked["generated_at"].(time.Time); !ok || !got.IsZero() {
		t.Fatalf("generated_at=%#v does not preserve time type", masked["generated_at"])
	}

	originalContact := input["contact"].(map[string]any)
	if originalContact["email"] != "josé@example.test" || originalContact["legal_note"] != "privileged" {
		t.Fatalf("input was mutated: %+v", input)
	}
}

func TestMaskAccessFieldsCustomPatternUsesUnicodeRunes(t *testing.T) {
	input := map[string]any{"customer": map[string]any{"name": "Ångström", "account": "12345678"}}
	masked := MaskAccessFields(input, []models.AccessFieldPermission{
		{FieldPath: "customer.name", Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskCustom, MaskPattern: "{first1}***"},
		{FieldPath: "customer.account", Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskCustom, MaskPattern: "acct-{last4}"},
	})
	want := map[string]any{"customer": map[string]any{"name": "Å***", "account": "acct-5678"}}
	if !reflect.DeepEqual(masked, want) {
		t.Fatalf("masked=%#v want=%#v", masked, want)
	}
}

func TestMaskAccessFieldsIgnoresUnsafeFieldPaths(t *testing.T) {
	input := map[string]any{"secret": "value"}
	masked := MaskAccessFields(input, []models.AccessFieldPermission{
		{FieldPath: "secret..nested", Visibility: models.AccessFieldHidden},
		{FieldPath: "secret[0]", Visibility: models.AccessFieldHidden},
	})
	if !reflect.DeepEqual(masked, input) {
		t.Fatalf("unsafe path changed output: %#v", masked)
	}
}

func TestAccessFieldObligationsRoundTripAndMaskNestedResponseWithoutMutation(t *testing.T) {
	permissions := []models.AccessFieldPermission{
		{ID: "field-email", PolicyID: "policy-1", PolicyPriority: 10, ResourceType: "users", FieldPath: "email", Classification: models.AccessFieldPersonal, Visibility: models.AccessFieldHidden},
		{ID: "field-phone", PolicyID: "policy-1", PolicyPriority: 10, ResourceType: "users", FieldPath: "phone", Classification: models.AccessFieldPersonal, Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskLast4},
		{ID: "field-token", PolicyID: "policy-1", PolicyPriority: 10, ResourceType: "users", FieldPath: "profile.credentials.token", Classification: models.AccessFieldSecuritySensitive, Visibility: models.AccessFieldHidden},
		{ID: "field-balance", PolicyID: "policy-1", PolicyPriority: 10, ResourceType: "users", FieldPath: "balance", Classification: models.AccessFieldFinancial, Visibility: models.AccessFieldMasked, MaskStrategy: models.AccessMaskRedact},
	}
	original := map[string]any{
		"data": []any{map[string]any{
			"email": "ada@example.test", "phone": "+2348012345678", "balance": 12500,
			"profile": map[string]any{"credentials": map[string]any{"token": "top-secret", "label": "hardware"}},
			"manager": map[string]any{"email": "manager@example.test", "phone": "+12025550199"},
		}},
	}
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	obligations := append([]authz.Obligation{{Kind: "watermark", Parameters: map[string]string{"text": "audit copy"}}}, AccessFieldObligations(permissions)...)
	parsed, err := AccessFieldsFromObligations(obligations, "user")
	if err != nil {
		t.Fatalf("parse obligations: %v", err)
	}
	if !reflect.DeepEqual(parsed, permissions) {
		t.Fatalf("parsed permissions=%#v want=%#v", parsed, permissions)
	}

	masked, err := MaskAccessResponse(original, parsed)
	if err != nil {
		t.Fatalf("MaskAccessResponse() error = %v", err)
	}
	encoded, err := json.Marshal(masked)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"ada@example.test", "manager@example.test", "top-secret", "12500", "+2348012345678", "+12025550199"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("classified value %q leaked in %s", secret, encoded)
		}
	}
	for _, expected := range []string{"*******5678", "*******0199", `"balance":0`} {
		if !bytes.Contains(encoded, []byte(expected)) {
			t.Fatalf("expected %q in masked response %s", expected, encoded)
		}
	}
	after, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("input mutated: before=%s after=%s", before, after)
	}
}

func TestAccessFieldsFromObligationsRejectsCorruptOrCrossResourceRules(t *testing.T) {
	valid := AccessFieldObligations([]models.AccessFieldPermission{{
		ID: "field-email", PolicyID: "policy-1", PolicyPriority: 10, ResourceType: "users", FieldPath: "email",
		Classification: models.AccessFieldPersonal, Visibility: models.AccessFieldHidden,
	}})[0]
	invalidVisibility := authz.Obligation{Kind: valid.Kind, Parameters: cloneStringMap(valid.Parameters)}
	invalidVisibility.Parameters["visibility"] = "plaintext"
	tests := []struct {
		name       string
		obligation authz.Obligation
		resource   string
	}{
		{name: "cross resource", obligation: valid, resource: "vendors"},
		{name: "missing parameters", obligation: authz.Obligation{Kind: AccessFieldVisibilityObligation}, resource: "users"},
		{name: "invalid visibility", obligation: invalidVisibility, resource: "users"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if rules, err := AccessFieldsFromObligations([]authz.Obligation{test.obligation}, test.resource); err == nil || len(rules) != 0 {
				t.Fatalf("rules=%#v err=%v", rules, err)
			}
		})
	}
}
