package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDataQualityDefinitionsAreFixedAndOwned(t *testing.T) {
	definitions := DataQualityCheckDefinitions()
	if len(definitions) != 18 {
		t.Fatalf("fixed ruleset has %d checks", len(definitions))
	}
	seen := make(map[DataQualityCheckKey]bool, len(definitions))
	for index, definition := range definitions {
		if definition.Key == "" || definition.Definition == "" || seen[definition.Key] {
			t.Fatalf("invalid fixed definition at %d: %#v", index, definition)
		}
		seen[definition.Key] = true
		want := DataQualityStructural
		if index >= 16 {
			want = DataQualityLifecycle
		}
		if definition.Category != want {
			t.Fatalf("check %s category=%s, want %s", definition.Key, definition.Category, want)
		}
	}
	definitions[0].Definition = "injected tenant business message"
	definitions[0].Key = "unknown"
	if fresh := DataQualityCheckDefinitions(); fresh[0].Key != DQControlAdoptionScope || strings.Contains(fresh[0].Definition, "injected") {
		t.Fatalf("caller mutated the fixed catalog: %#v", fresh[0])
	}
}

func TestDataQualityInternalCountsCannotExposeScopeOnMarshal(t *testing.T) {
	encoded, err := json.Marshal(DataQualityCounts{
		OrganizationID: "private-organization-id", ActorID: "private-actor-id", SchemaVersion: 58,
		Counts: []DataQualityCount{{Key: DQFindingAuditScope, Count: 41}},
	})
	if err != nil || string(encoded) != "{}" {
		t.Fatalf("internal scope became a response: %s, error=%v", encoded, err)
	}
}
