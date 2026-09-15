package database

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestAPIPrivilegeManifestMatchesStartupAllowlist(t *testing.T) {
	manifest, err := os.ReadFile("../../deployments/postgres/runtime-grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "DO $api_tenant_tables$") {
		t.Fatal("API privileges must be explicit, not all RLS tables from the catalog")
	}
	grants := tableGrantMatrix(string(manifest), "complianceforge_api")
	queryMatrix := make(map[string][]string)
	rowPattern := regexp.MustCompile(`\('public\.([a-z0-9_]+)'(?:::TEXT)?, ARRAY\[([^]]+)\]::TEXT\[\]\)`)
	for _, row := range rowPattern.FindAllStringSubmatch(apiDatabasePostureQuery, -1) {
		queryMatrix[row[1]] = normalizedVerbs(row[2])
	}
	if !reflect.DeepEqual(grants, queryMatrix) {
		t.Fatalf("runtime API grants and startup allowlist differ:\ngrants=%v\nquery=%v", grants, queryMatrix)
	}
	for _, table := range []string{
		"access_audit_log", "access_policy_certifications", "access_policy_change_events",
		"asset_events", "audit_logs", "data_governance_events", "directory_change_events",
		"directory_imports", "feature_flag_change_events", "identity_security_events",
		"incident_events", "integration_sync_logs", "risk_assessments", "risk_indicator_values",
		"role_change_events", "scim_resource_events", "scim_token_events", "vendor_events",
		"access_review_items", "access_review_decisions", "access_sod_violations", "access_governance_events",
	} {
		if got := grants[table]; !reflect.DeepEqual(got, []string{"INSERT", "SELECT"}) {
			t.Fatalf("immutable/append-only %s verbs = %v", table, got)
		}
	}
	for _, disabledTable := range []string{"ai_interaction_logs", "marketplace_packages", "workflow_definitions", "board_meetings", "analytics_snapshots"} {
		if got := grants[disabledTable]; len(got) != 0 {
			t.Fatalf("uncomposed module %s received API verbs %v", disabledTable, got)
		}
	}
}

func TestReviewedRuntimeSchemaPinMatchesDeploymentManifest(t *testing.T) {
	manifest, err := os.ReadFile("../../deployments/postgres/runtime-grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	if reviewedRuntimeSchemaVersion != SupportedSchemaVersion {
		t.Fatalf("supported schema %d has no reviewed runtime privilege pin (currently %d)", SupportedSchemaVersion, reviewedRuntimeSchemaVersion)
	}
	if !strings.Contains(string(manifest), fmt.Sprintf("current_version IS DISTINCT FROM %d", reviewedRuntimeSchemaVersion)) {
		t.Fatal("SQL manifest schema guard differs from runtime posture pin")
	}
}

func TestWorkerPrivilegeManifestMatchesStartupAllowlist(t *testing.T) {
	manifest, err := os.ReadFile("../../deployments/postgres/runtime-grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	grants := tableGrantMatrix(string(manifest), "complianceforge_scheduler")
	queryMatrix := make(map[string][]string)
	rowPattern := regexp.MustCompile(`\('public\.([a-z0-9_]+)'(?:::TEXT)?, '([A-Z]+)'(?:::TEXT)?\)`)
	for _, row := range rowPattern.FindAllStringSubmatch(workerDatabasePostureQuery, -1) {
		queryMatrix[row[1]] = append(queryMatrix[row[1]], row[2])
	}
	for table, verbs := range queryMatrix {
		queryMatrix[table] = uniqueVerbs(verbs)
	}
	if !reflect.DeepEqual(grants, queryMatrix) {
		t.Fatalf("runtime worker grants and startup allowlist differ:\ngrants=%v\nquery=%v", grants, queryMatrix)
	}
}

func tableGrantMatrix(manifest, role string) map[string][]string {
	matrix := make(map[string][]string)
	grantPattern := regexp.MustCompile(`(?s)\bGRANT ([A-Z ,]+) ON TABLE\s+([^;]+?)\s+TO ` + regexp.QuoteMeta(role) + `;`)
	tablePattern := regexp.MustCompile(`\bpublic\.([a-z0-9_]+)`)
	for _, grant := range grantPattern.FindAllStringSubmatch(manifest, -1) {
		for _, table := range tablePattern.FindAllStringSubmatch(grant[2], -1) {
			matrix[table[1]] = append(matrix[table[1]], normalizedVerbs(grant[1])...)
		}
	}
	for table, verbs := range matrix {
		matrix[table] = uniqueVerbs(verbs)
	}
	return matrix
}

func normalizedVerbs(raw string) []string {
	var verbs []string
	for _, value := range strings.Split(raw, ",") {
		verbs = append(verbs, strings.Trim(strings.TrimSpace(value), "'"))
	}
	return uniqueVerbs(verbs)
}

func uniqueVerbs(verbs []string) []string {
	seen := make(map[string]bool)
	for _, verb := range verbs {
		seen[verb] = true
	}
	result := make([]string, 0, len(seen))
	for verb := range seen {
		result = append(result, verb)
	}
	sort.Strings(result)
	return result
}
