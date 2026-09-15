package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestAutomatedEvidenceProofMetadataBindsRunAndResults(t *testing.T) {
	collected := []byte(`{"enabled":true,"count":7}`)
	validation := []byte(`[{"criteria_index":0,"passed":true}]`)
	encoded, err := automatedEvidenceProofMetadata("run-123", collected, validation)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]map[string]string
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		t.Fatal(err)
	}
	proof := metadata["automated_proof"]
	want := map[string]string{
		"proof_schema":              "automated-evidence/v1",
		"collection_run_id":         "run-123",
		"collected_data_sha256":     fmt.Sprintf("%x", sha256.Sum256(collected)),
		"validation_results_sha256": fmt.Sprintf("%x", sha256.Sum256(validation)),
	}
	if !reflect.DeepEqual(proof, want) {
		t.Fatalf("proof=%v want=%v", proof, want)
	}

	changed, err := automatedEvidenceProofMetadata("run-123", []byte(`{"enabled":false,"count":7}`), validation)
	if err != nil {
		t.Fatal(err)
	}
	if string(changed) == string(encoded) {
		t.Fatal("changed collected proof produced the same metadata")
	}
	for _, test := range []struct {
		runID      string
		collected  []byte
		validation []byte
	}{
		{runID: "", collected: collected, validation: validation},
		{runID: "run-123", collected: []byte(`{`), validation: validation},
		{runID: "run-123", collected: collected, validation: []byte(`not-json`)},
	} {
		if _, err := automatedEvidenceProofMetadata(test.runID, test.collected, test.validation); err == nil {
			t.Fatalf("invalid proof inputs accepted: %+v", test)
		}
	}
}

func TestValidateEvidence(t *testing.T) {
	collector := &EvidenceCollector{}

	tests := []struct {
		name            string
		data            interface{}
		criteria        []AcceptanceCriterion
		expectedPass    bool
		expectedResults int
	}{
		{
			name: "equals operator passes",
			data: map[string]interface{}{"status": "enabled"},
			criteria: []AcceptanceCriterion{
				{Field: "status", Operator: "equals", Value: "enabled"},
			},
			expectedPass:    true,
			expectedResults: 1,
		},
		{
			name: "equals operator fails",
			data: map[string]interface{}{"status": "disabled"},
			criteria: []AcceptanceCriterion{
				{Field: "status", Operator: "equals", Value: "enabled"},
			},
			expectedPass:    false,
			expectedResults: 1,
		},
		{
			name: "greater_than passes",
			data: map[string]interface{}{"count": float64(10)},
			criteria: []AcceptanceCriterion{
				{Field: "count", Operator: "greater_than", Value: float64(5)},
			},
			expectedPass:    true,
			expectedResults: 1,
		},
		{
			name: "greater_than fails",
			data: map[string]interface{}{"count": float64(3)},
			criteria: []AcceptanceCriterion{
				{Field: "count", Operator: "greater_than", Value: float64(5)},
			},
			expectedPass:    false,
			expectedResults: 1,
		},
		{
			name: "less_than passes",
			data: map[string]interface{}{"error_rate": float64(0.01)},
			criteria: []AcceptanceCriterion{
				{Field: "error_rate", Operator: "less_than", Value: float64(0.05)},
			},
			expectedPass:    true,
			expectedResults: 1,
		},
		{
			name: "contains passes",
			data: map[string]interface{}{"message": "All checks passed successfully"},
			criteria: []AcceptanceCriterion{
				{Field: "message", Operator: "contains", Value: "passed"},
			},
			expectedPass:    true,
			expectedResults: 1,
		},
		{
			name: "contains fails",
			data: map[string]interface{}{"message": "Check failed"},
			criteria: []AcceptanceCriterion{
				{Field: "message", Operator: "contains", Value: "passed"},
			},
			expectedPass:    false,
			expectedResults: 1,
		},
		{
			name: "exists passes",
			data: map[string]interface{}{"certificate": "cert-data-here"},
			criteria: []AcceptanceCriterion{
				{Field: "certificate", Operator: "exists", Value: nil},
			},
			expectedPass:    true,
			expectedResults: 1,
		},
		{
			name: "exists fails",
			data: map[string]interface{}{"other_field": "value"},
			criteria: []AcceptanceCriterion{
				{Field: "certificate", Operator: "exists", Value: nil},
			},
			expectedPass:    false,
			expectedResults: 1,
		},
		{
			name: "multiple criteria all pass",
			data: map[string]interface{}{"status": "active", "count": float64(10), "message": "OK"},
			criteria: []AcceptanceCriterion{
				{Field: "status", Operator: "equals", Value: "active"},
				{Field: "count", Operator: "greater_than", Value: float64(0)},
				{Field: "message", Operator: "contains", Value: "OK"},
			},
			expectedPass:    true,
			expectedResults: 3,
		},
		{
			name: "multiple criteria one fails",
			data: map[string]interface{}{"status": "active", "count": float64(0), "message": "OK"},
			criteria: []AcceptanceCriterion{
				{Field: "status", Operator: "equals", Value: "active"},
				{Field: "count", Operator: "greater_than", Value: float64(5)},
				{Field: "message", Operator: "contains", Value: "OK"},
			},
			expectedPass:    false,
			expectedResults: 3,
		},
		{
			name:            "empty criteria passes",
			data:            map[string]interface{}{"anything": "value"},
			criteria:        []AcceptanceCriterion{},
			expectedPass:    true,
			expectedResults: 0,
		},
		{
			name: "missing field fails",
			data: map[string]interface{}{"other": "value"},
			criteria: []AcceptanceCriterion{
				{Field: "status", Operator: "equals", Value: "active"},
			},
			expectedPass:    false,
			expectedResults: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, allPassed := collector.ValidateEvidence(tt.data, tt.criteria)
			if allPassed != tt.expectedPass {
				t.Errorf("expected allPassed=%v, got %v", tt.expectedPass, allPassed)
			}
			if len(results) != tt.expectedResults {
				t.Errorf("expected %d results, got %d", tt.expectedResults, len(results))
			}
		})
	}
}
