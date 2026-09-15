package worker

import (
	"strings"
	"testing"
)

func TestSearchIndexUpsertMatchesCanonicalSchema(t *testing.T) {
	t.Parallel()

	normalized := strings.ToLower(searchIndexUpsertSQL)
	for _, forbidden := range []string{" content", "indexed_at", "on conflict (entity_type, entity_id)"} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("search upsert still references legacy schema fragment %q", forbidden)
		}
	}
	for _, required := range []string{
		"body",
		"created_date",
		"updated_date",
		"on conflict (organization_id, entity_type, entity_id)",
		"excluded.body",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("search upsert does not contain canonical schema fragment %q", required)
		}
	}
}

func TestIndexableEntitiesUseCanonicalRelations(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, len(indexableEntities))
	for _, entity := range indexableEntities {
		if _, duplicate := seen[entity.TypeName]; duplicate {
			t.Fatalf("duplicate indexable entity type %q", entity.TypeName)
		}
		seen[entity.TypeName] = struct{}{}
		if entity.Table == "controls" || entity.Table == "evidence_items" {
			t.Fatalf("entity %q references removed relation %q", entity.TypeName, entity.Table)
		}
	}

	control := indexableEntityByType(t, "control")
	if !strings.Contains(control.Table, "control_implementations") ||
		!strings.Contains(control.Table, "framework_controls") {
		t.Fatalf("control index source must join canonical implementation and catalog relations: %s", control.Table)
	}
	evidence := indexableEntityByType(t, "evidence")
	if evidence.Table != "control_evidence" {
		t.Fatalf("evidence index source = %q, want control_evidence", evidence.Table)
	}
	policy := indexableEntityByType(t, "policy")
	if !strings.Contains(policy.Table, "policy_versions") || policy.ContentCol != "body" {
		t.Fatalf("policy index source must use the canonical current policy version: %+v", policy)
	}
}

func indexableEntityByType(t *testing.T, typeName string) indexableEntity {
	t.Helper()
	for _, entity := range indexableEntities {
		if entity.TypeName == typeName {
			return entity
		}
	}
	t.Fatalf("indexable entity %q not found", typeName)
	return indexableEntity{}
}
