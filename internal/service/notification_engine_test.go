package service

import (
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	engine := &NotificationEngine{}

	tests := []struct {
		name     string
		tmpl     string
		data     map[string]interface{}
		expected string
		wantErr  bool
	}{
		{
			name:     "simple variable",
			tmpl:     "Hello {{.Name}}",
			data:     map[string]interface{}{"Name": "World"},
			expected: "Hello World",
		},
		{
			name:     "multiple variables",
			tmpl:     "Incident {{.IncidentRef}} — {{.Title}} ({{.Severity}})",
			data:     map[string]interface{}{"IncidentRef": "INC-0001", "Title": "Data Breach", "Severity": "critical"},
			expected: "Incident INC-0001 — Data Breach (critical)",
		},
		{
			name:     "missing variable renders empty",
			tmpl:     "Hello {{.Name}}, your score is {{.Score}}",
			data:     map[string]interface{}{"Name": "User"},
			expected: "Hello User, your score is <no value>",
		},
		{
			name:    "invalid template syntax",
			tmpl:    "Hello {{.Name",
			data:    map[string]interface{}{"Name": "World"},
			wantErr: true,
		},
		{
			name:     "empty data",
			tmpl:     "Static text only",
			data:     map[string]interface{}{},
			expected: "Static text only",
		},
		{
			name:     "GDPR breach template",
			tmpl:     "URGENT: GDPR Breach — {{.IncidentRef}} — {{.HoursRemaining}}h remaining. {{.DataSubjectsAffected}} subjects affected.",
			data:     map[string]interface{}{"IncidentRef": "INC-0042", "HoursRemaining": "6", "DataSubjectsAffected": "15000"},
			expected: "URGENT: GDPR Breach — INC-0042 — 6h remaining. 15000 subjects affected.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.RenderTemplate(tt.tmpl, tt.data)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestEvaluateConditions(t *testing.T) {
	engine := &NotificationEngine{}

	tests := []struct {
		name       string
		conditions map[string]interface{}
		eventData  map[string]interface{}
		expected   bool
	}{
		{
			name:       "nil conditions matches all",
			conditions: nil,
			eventData:  map[string]interface{}{"severity": "critical"},
			expected:   true,
		},
		{
			name:       "empty conditions matches all",
			conditions: map[string]interface{}{},
			eventData:  map[string]interface{}{"severity": "critical"},
			expected:   true,
		},
		{
			name:       "matching string condition",
			conditions: map[string]interface{}{"is_data_breach": true},
			eventData:  map[string]interface{}{"is_data_breach": true},
			expected:   true,
		},
		{
			name:       "non-matching condition",
			conditions: map[string]interface{}{"is_data_breach": true},
			eventData:  map[string]interface{}{"is_data_breach": false},
			expected:   false,
		},
		{
			name:       "missing field in event data",
			conditions: map[string]interface{}{"is_data_breach": true},
			eventData:  map[string]interface{}{"severity": "high"},
			expected:   false,
		},
		{
			name:       "multiple conditions all match",
			conditions: map[string]interface{}{"severity": "critical", "is_data_breach": true},
			eventData:  map[string]interface{}{"severity": "critical", "is_data_breach": true, "title": "Breach"},
			expected:   true,
		},
		{
			name:       "multiple conditions partial match",
			conditions: map[string]interface{}{"severity": "critical", "is_data_breach": true},
			eventData:  map[string]interface{}{"severity": "high", "is_data_breach": true},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.evaluateConditions(tt.conditions, tt.eventData)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
