package service

import (
	"strings"
	"testing"
)

func TestEvaluateConditionsABAC(t *testing.T) {
	engine := &ABACEngine{}

	tests := []struct {
		name       string
		conditions []map[string]interface{}
		attributes map[string]interface{}
		expected   bool
	}{
		{
			name:       "empty conditions match all",
			conditions: nil,
			attributes: map[string]interface{}{"role": "admin"},
			expected:   true,
		},
		{
			name: "equals match",
			conditions: []map[string]interface{}{
				{"attribute": "role", "operator": "equals", "value": "admin"},
			},
			attributes: map[string]interface{}{"role": "admin"},
			expected:   true,
		},
		{
			name: "equals no match",
			conditions: []map[string]interface{}{
				{"attribute": "role", "operator": "equals", "value": "admin"},
			},
			attributes: map[string]interface{}{"role": "viewer"},
			expected:   false,
		},
		{
			name: "in operator match",
			conditions: []map[string]interface{}{
				{"attribute": "role", "operator": "in", "values": []interface{}{"admin", "ciso", "dpo"}},
			},
			attributes: map[string]interface{}{"role": "dpo"},
			expected:   true,
		},
		{
			name: "in operator no match",
			conditions: []map[string]interface{}{
				{"attribute": "role", "operator": "in", "values": []interface{}{"admin", "ciso"}},
			},
			attributes: map[string]interface{}{"role": "viewer"},
			expected:   false,
		},
		{
			name: "not_in operator match",
			conditions: []map[string]interface{}{
				{"attribute": "classification", "operator": "not_in", "values": []interface{}{"confidential", "restricted"}},
			},
			attributes: map[string]interface{}{"classification": "internal"},
			expected:   true,
		},
		{
			name: "not_in operator no match (value in excluded list)",
			conditions: []map[string]interface{}{
				{"attribute": "classification", "operator": "not_in", "values": []interface{}{"confidential", "restricted"}},
			},
			attributes: map[string]interface{}{"classification": "confidential"},
			expected:   false,
		},
		{
			name: "multiple conditions AND logic",
			conditions: []map[string]interface{}{
				{"attribute": "role", "operator": "equals", "value": "auditor"},
				{"attribute": "department", "operator": "equals", "value": "Internal Audit"},
			},
			attributes: map[string]interface{}{"role": "auditor", "department": "Internal Audit"},
			expected:   true,
		},
		{
			name: "multiple conditions one fails",
			conditions: []map[string]interface{}{
				{"attribute": "role", "operator": "equals", "value": "auditor"},
				{"attribute": "department", "operator": "equals", "value": "Internal Audit"},
			},
			attributes: map[string]interface{}{"role": "auditor", "department": "IT"},
			expected:   false,
		},
		{
			name: "missing attribute returns false",
			conditions: []map[string]interface{}{
				{"attribute": "clearance_level", "operator": "equals", "value": "top_secret"},
			},
			attributes: map[string]interface{}{"role": "admin"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.evaluateConditions(tt.conditions, tt.attributes)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMaskFields(t *testing.T) {
	engine := &ABACEngine{}

	data := map[string]interface{}{
		"id":                   "uuid-123",
		"title":                "Critical Risk",
		"financial_impact_eur": 5000000.0,
		"description":          "Sensitive details",
		"owner_name":           "John Smith",
	}

	permissions := []FieldPermission{
		{FieldName: "financial_impact_eur", Permission: "masked", MaskPattern: "****"},
		{FieldName: "description", Permission: "hidden"},
		{FieldName: "owner_name", Permission: "visible"},
	}

	result := engine.MaskFields(data, permissions)

	// financial_impact_eur should be masked (pattern length != value length → fallback mask)
	maskedVal, ok := result["financial_impact_eur"].(string)
	if !ok || !strings.Contains(maskedVal, "*") {
		t.Errorf("expected financial_impact_eur to be masked with asterisks, got %v", result["financial_impact_eur"])
	}

	// description should be removed
	if _, exists := result["description"]; exists {
		t.Errorf("expected description to be removed (hidden)")
	}

	// owner_name should be unchanged
	if result["owner_name"] != "John Smith" {
		t.Errorf("expected owner_name to be 'John Smith', got %v", result["owner_name"])
	}

	// id and title should be unchanged (no permission rule)
	if result["id"] != "uuid-123" {
		t.Errorf("expected id to be unchanged")
	}
	if result["title"] != "Critical Risk" {
		t.Errorf("expected title to be unchanged")
	}
}

func TestFieldMaskPattern(t *testing.T) {
	engine := &ABACEngine{}

	data := map[string]interface{}{
		"name": "John Smith",
	}

	permissions := []FieldPermission{
		{FieldName: "name", Permission: "masked", MaskPattern: "J*** S****"},
	}

	result := engine.MaskFields(data, permissions)
	if result["name"] != "J*** S****" {
		t.Errorf("expected 'J*** S****', got %v", result["name"])
	}
}
