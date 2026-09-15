package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/models"
)

func validPolicyInputForTest() models.AccessPolicyInput {
	return models.AccessPolicyInput{
		Name:        "Risk owners in region",
		Description: "Restrict risk access to the responsible organizational scope.",
		Priority:    100,
		Effect:      models.AccessPolicyEffectAllow,
		IsActive:    true,
		SubjectConditions: []models.AccessCondition{
			{Attribute: "roles", Operator: models.AccessOperatorContainsAny, Value: json.RawMessage(`["risk_manager"]`)},
		},
		ResourceType: "risks",
		ResourceConditions: []models.AccessCondition{
			{Attribute: "department", Operator: models.AccessOperatorEqualsSubject, Value: json.RawMessage(`"department"`)},
		},
		Actions: []string{"read", "update"},
		EnvironmentConditions: []models.AccessCondition{
			{Attribute: "ip_address", Operator: models.AccessOperatorInCIDR, Value: json.RawMessage(`["192.0.2.0/24"]`)},
		},
		Reason: "Introduce regional risk segregation.",
	}
}

func TestValidateAccessPolicyInput(t *testing.T) {
	if err := ValidateAccessPolicyInput(validPolicyInputForTest()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*models.AccessPolicyInput)
	}{
		{name: "effect", mutate: func(input *models.AccessPolicyInput) { input.Effect = "override" }},
		{name: "resource", mutate: func(input *models.AccessPolicyInput) { input.ResourceType = "arbitrary_table" }},
		{name: "duplicate action", mutate: func(input *models.AccessPolicyInput) { input.Actions = []string{"read", "read"} }},
		{name: "mixed wildcard", mutate: func(input *models.AccessPolicyInput) { input.Actions = []string{"*", "read"} }},
		{name: "unknown attribute", mutate: func(input *models.AccessPolicyInput) {
			input.SubjectConditions[0].Attribute = "password_hash"
		}},
		{name: "unknown operator", mutate: func(input *models.AccessPolicyInput) {
			input.ResourceConditions[0].Operator = "javascript"
		}},
		{name: "bad CIDR", mutate: func(input *models.AccessPolicyInput) {
			input.EnvironmentConditions[0].Value = json.RawMessage(`"not-a-cidr"`)
		}},
		{name: "reversed window", mutate: func(input *models.AccessPolicyInput) {
			from := policyTestNow
			until := from.Add(-time.Minute)
			input.ValidFrom, input.ValidUntil = &from, &until
		}},
		{name: "missing reason", mutate: func(input *models.AccessPolicyInput) { input.Reason = "" }},
		{name: "oversized condition", mutate: func(input *models.AccessPolicyInput) {
			input.SubjectConditions[0].Value = json.RawMessage(`"` + strings.Repeat("x", 4097) + `"`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validPolicyInputForTest()
			test.mutate(&input)
			if err := ValidateAccessPolicyInput(input); !errors.Is(err, ErrInvalidAccessPolicy) {
				t.Fatalf("err=%v, want ErrInvalidAccessPolicy", err)
			}
		})
	}
}

func TestValidateAccessPolicyAssignmentInput(t *testing.T) {
	userID := "00000000-0000-0000-0000-000000000101"
	valid := models.AccessPolicyAssignmentInput{AssigneeType: models.AccessAssigneeUser, AssigneeID: &userID, Reason: "Assign policy to user."}
	if err := ValidateAccessPolicyAssignmentInput(valid); err != nil {
		t.Fatal(err)
	}
	allUsers := models.AccessPolicyAssignmentInput{AssigneeType: models.AccessAssigneeAllUsers, Reason: "Apply tenant-wide constraint."}
	if err := ValidateAccessPolicyAssignmentInput(allUsers); err != nil {
		t.Fatal(err)
	}

	invalidAll := allUsers
	invalidAll.AssigneeID = &userID
	if err := ValidateAccessPolicyAssignmentInput(invalidAll); !errors.Is(err, ErrInvalidAccessPolicy) {
		t.Fatalf("err=%v", err)
	}
	missingUser := valid
	missingUser.AssigneeID = nil
	if err := ValidateAccessPolicyAssignmentInput(missingUser); !errors.Is(err, ErrInvalidAccessPolicy) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateAccessObjectGrantInput(t *testing.T) {
	input := models.AccessObjectGrantInput{
		SubjectID:        "00000000-0000-0000-0000-000000000101",
		ResourceType:     "audits",
		ResourceID:       "00000000-0000-0000-0000-000000000201",
		Actions:          []string{"read", "export"},
		SponsorID:        "00000000-0000-0000-0000-000000000301",
		ValidFrom:        policyTestNow,
		ValidUntil:       policyTestNow.Add(30 * 24 * time.Hour),
		AllowDownload:    true,
		RequireWatermark: true,
		WatermarkText:    "External auditor copy",
		Reason:           "Grant evidence review for the annual audit.",
	}
	if err := ValidateAccessObjectGrantInput(input); err != nil {
		t.Fatal(err)
	}

	input.SponsorID = input.SubjectID
	if err := ValidateAccessObjectGrantInput(input); !errors.Is(err, ErrInvalidAccessPolicy) {
		t.Fatalf("self-sponsored err=%v", err)
	}
	input.SponsorID = "00000000-0000-0000-0000-000000000301"
	input.AllowDownload = false
	if err := ValidateAccessObjectGrantInput(input); !errors.Is(err, ErrInvalidAccessPolicy) {
		t.Fatalf("watermark-without-download err=%v", err)
	}
}

func TestValidateAccessFieldPermission(t *testing.T) {
	rule := models.AccessFieldPermission{
		ResourceType: "risks", FieldPath: "financial.amount",
		Classification: models.AccessFieldFinancial, Visibility: models.AccessFieldMasked,
		MaskStrategy: models.AccessMaskCustom, MaskPattern: "EUR *** {last4}",
	}
	if err := ValidateAccessFieldPermission(rule); err != nil {
		t.Fatal(err)
	}

	rule.MaskPattern = "{execute}"
	if err := ValidateAccessFieldPermission(rule); !errors.Is(err, ErrInvalidAccessPolicy) {
		t.Fatalf("unknown placeholder err=%v", err)
	}
	rule.MaskPattern = ""
	rule.Visibility = models.AccessFieldHidden
	rule.MaskStrategy = models.AccessMaskRedact
	if err := ValidateAccessFieldPermission(rule); !errors.Is(err, ErrInvalidAccessPolicy) {
		t.Fatalf("hidden mask err=%v", err)
	}
}
