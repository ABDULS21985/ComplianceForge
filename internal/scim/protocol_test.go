package scim

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/complianceforge/platform/internal/models"
)

func TestVersionETagRoundTripAndStrictParsing(t *testing.T) {
	for _, version := range []int64{1, 42, 9223372036854775807} {
		value := VersionETag(version)
		parsed, err := ParseVersionETag(value)
		if err != nil || parsed != version {
			t.Fatalf("ParseVersionETag(%q)=(%d,%v)", value, parsed, err)
		}
	}
	for _, value := range []string{"", "*", "1", `W/"0"`, `W/"-1"`, `W/"1", W/"2"`} {
		if _, err := ParseVersionETag(value); !errors.Is(err, ErrInvalidSyntax) {
			t.Errorf("ParseVersionETag(%q) error=%v", value, err)
		}
	}
}

func TestNormalizeUserDefaultsActiveAndValidatesEnterpriseSchema(t *testing.T) {
	var user models.SCIMUser
	if err := json.Unmarshal([]byte(`{
		"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],
		"userName":"Ada@Example.Test"
	}`), &user); err != nil {
		t.Fatal(err)
	}
	if !user.Active {
		t.Fatal("omitted active did not default to true")
	}
	if err := NormalizeUser(&user); err != nil {
		t.Fatal(err)
	}
	if user.UserName != "ada@example.test" {
		t.Fatalf("userName=%q", user.UserName)
	}

	user.Enterprise = &models.SCIMEnterpriseUser{EmployeeNumber: "42"}
	if err := NormalizeUser(&user); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("enterprise extension without schema error=%v", err)
	}
}

func TestValidatePatchRequest(t *testing.T) {
	valid := models.SCIMPatchRequest{Schemas: []string{models.SCIMPatchOperationSchema}, Operations: []models.SCIMPatchOperation{{
		Operation: "Replace", Path: "active", Value: json.RawMessage("false"),
	}}}
	if err := ValidatePatchRequest(valid); err != nil {
		t.Fatal(err)
	}
	valid.Operations[0].Path = ""
	valid.Operations[0].Value = json.RawMessage(`{"active":false}`)
	if err := ValidatePatchRequest(valid); err != nil {
		t.Fatalf("pathless attribute patch error=%v", err)
	}
	valid.Operations[0].Value = json.RawMessage(`false`)
	if err := ValidatePatchRequest(valid); !errors.Is(err, ErrInvalidSyntax) {
		t.Fatalf("pathless scalar error=%v", err)
	}
}

func TestServiceProviderConfigExplicitlyDisablesBulk(t *testing.T) {
	configuration := ServiceProviderConfig()
	if configuration.Bulk.Supported || !configuration.Patch.Supported || !configuration.Filter.Supported || !configuration.ETag.Supported {
		t.Fatalf("configuration=%#v", configuration)
	}
	if configuration.Meta.Location != "/api/scim/v2/ServiceProviderConfig" || configuration.Meta.Version != `W/"1"` {
		t.Fatalf("configuration meta=%#v", configuration.Meta)
	}
}

func TestNormalizeListRequestPreservesExplicitZeroAndCapsLargeCount(t *testing.T) {
	request, err := NormalizeListRequest(1, 0, "")
	if err != nil || request.Count != 0 {
		t.Fatalf("explicit zero count request=%#v error=%v", request, err)
	}
	request, err = NormalizeListRequest(2, models.SCIMMaximumResultsPerPage+1, "")
	if err != nil || request.Count != models.SCIMMaximumResultsPerPage {
		t.Fatalf("capped request=%#v error=%v", request, err)
	}
}
