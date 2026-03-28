package service

import (
	"fmt"
	"testing"
)

func TestPackageDataValidation(t *testing.T) {
	tests := []struct {
		name  string
		data  map[string]interface{}
		valid bool
	}{
		{
			name: "valid package with controls",
			data: map[string]interface{}{
				"controls": []interface{}{
					map[string]interface{}{"code": "CUSTOM-01", "title": "Custom Control", "description": "A custom control"},
				},
			},
			valid: true,
		},
		{
			name: "control missing title",
			data: map[string]interface{}{
				"controls": []interface{}{
					map[string]interface{}{"code": "CUSTOM-01"},
				},
			},
			valid: false,
		},
		{
			name: "control missing code",
			data: map[string]interface{}{
				"controls": []interface{}{
					map[string]interface{}{"title": "Custom Control"},
				},
			},
			valid: false,
		},
		{
			name:  "empty package",
			data:  map[string]interface{}{},
			valid: true, // empty is valid (nothing to import)
		},
		{
			name: "valid mappings",
			data: map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{"source": "A.5.1", "target": "CUSTOM-01", "type": "equivalent", "strength": 0.9},
				},
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePackageData(tt.data)
			if tt.valid && err != nil {
				t.Errorf("expected valid but got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected invalid but got no error")
			}
		})
	}
}

// validatePackageData checks that controls have required fields.
func validatePackageData(data map[string]interface{}) error {
	if controls, ok := data["controls"].([]interface{}); ok {
		for _, c := range controls {
			ctrl, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if _, hasCode := ctrl["code"]; !hasCode {
				return fmt.Errorf("control missing required field: code")
			}
			if _, hasTitle := ctrl["title"]; !hasTitle {
				return fmt.Errorf("control missing required field: title")
			}
		}
	}
	return nil
}
