package database

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateWorkerDatabasePostureAcceptsSeparatedCompleteRole(t *testing.T) {
	posture := workerDatabasePosture{roleName: "complianceforge_worker_runtime"}
	if err := validateWorkerDatabasePosture(posture, true); err != nil {
		t.Fatalf("validateWorkerDatabasePosture() error = %v", err)
	}
}

func TestValidateWorkerDatabasePostureRejectsMissingRuntimeRequirements(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*workerDatabasePosture)
		wantErr string
	}{
		{name: "missing table grant", mutate: func(p *workerDatabasePosture) { p.missingTablePrivileges = []string{"public.queue_outbox:UPDATE"} }, wantErr: "table privileges"},
		{name: "missing function grant", mutate: func(p *workerDatabasePosture) {
			p.missingCapabilities = []string{"public.expire_due_evidence(uuid,integer)"}
		}, wantErr: "required worker capabilities"},
		{name: "unsafe function", mutate: func(p *workerDatabasePosture) {
			p.misconfiguredCapabilities = []string{"public.evidence_due_tenants(integer,uuid)"}
		}, wantErr: "unsafe definitions"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			posture := workerDatabasePosture{roleName: "worker"}
			test.mutate(&posture)
			err := validateWorkerDatabasePosture(posture, false)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateWorkerDatabasePosture() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateWorkerDatabasePostureRejectsProductionIdentityDrift(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*workerDatabasePosture)
		wantErr string
	}{
		{name: "empty role", mutate: func(p *workerDatabasePosture) { p.roleName = " " }, wantErr: "empty runtime role"},
		{name: "superuser", mutate: func(p *workerDatabasePosture) { p.superuser = true }, wantErr: "SUPERUSER"},
		{name: "bypass RLS", mutate: func(p *workerDatabasePosture) { p.bypassRLS = true }, wantErr: "BYPASSRLS"},
		{name: "database owner", mutate: func(p *workerDatabasePosture) { p.ownsCurrentDatabase = true }, wantErr: "database owner"},
		{name: "database create", mutate: func(p *workerDatabasePosture) { p.canCreateCurrentDatabase = true }, wantErr: "create schemas"},
		{name: "schema owner", mutate: func(p *workerDatabasePosture) { p.ownsPublicSchema = true }, wantErr: "schema owner"},
		{name: "schema create", mutate: func(p *workerDatabasePosture) { p.canCreatePublicSchema = true }, wantErr: "create objects"},
		{name: "missing group", mutate: func(p *workerDatabasePosture) { p.missingSchedulerMembership = true }, wantErr: "not a member"},
		{name: "unexpected group", mutate: func(p *workerDatabasePosture) { p.unexpectedMemberships = []string{"legacy_writer"} }, wantErr: "unexpected memberships"},
		{name: "API group", mutate: func(p *workerDatabasePosture) { p.forbiddenMemberships = []string{"complianceforge_api"} }, wantErr: "forbidden API/owner memberships"},
		{name: "public relation owner", mutate: func(p *workerDatabasePosture) { p.ownedPublicRelations = []string{"public.notifications"} }, wantErr: "owners of public relations"},
		{name: "relation owner", mutate: func(p *workerDatabasePosture) { p.ownedProtectedRelations = []string{"public.queue_outbox"} }, wantErr: "owners of protected relations"},
		{name: "unexpected table grant", mutate: func(p *workerDatabasePosture) { p.unexpectedTablePrivileges = []string{"public.users:UPDATE"} }, wantErr: "outside the worker allowlist"},
		{name: "unexpected sequence grant", mutate: func(p *workerDatabasePosture) { p.unexpectedSequenceGrants = []string{"public.legacy_id_seq:USAGE"} }, wantErr: "unexpected sequence privileges"},
		{name: "ledger writer", mutate: func(p *workerDatabasePosture) { p.writableEvidenceLedgers = []string{"public.evidence_reviews"} }, wantErr: "write protected evidence ledgers"},
		{name: "API capability", mutate: func(p *workerDatabasePosture) {
			p.forbiddenCapabilities = []string{"public.submit_evidence_review(uuid,uuid,uuid,uuid,text,text,text)"}
		}, wantErr: "API-only evidence capabilities"},
		{name: "PUBLIC function", mutate: func(p *workerDatabasePosture) {
			p.publicPrivileges = []string{"incident_due_tenants(integer):EXECUTE:PUBLIC"}
		}, wantErr: "PUBLIC has runtime"},
		{name: "unreviewed definer", mutate: func(p *workerDatabasePosture) { p.unexpectedDefiners = []string{"resolve_scim_token_tenant(text)"} }, wantErr: "unreviewed SECURITY DEFINER"},
		{name: "unsafe queue policy", mutate: func(p *workerDatabasePosture) {
			p.queueIsolationViolations = []string{"queue_inbox_scheduler_all:POLICY"}
		}, wantErr: "queue isolation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			posture := workerDatabasePosture{roleName: "worker"}
			test.mutate(&posture)
			err := validateWorkerDatabasePosture(posture, true)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateWorkerDatabasePosture() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestWorkerDatabasePostureQueryCoversComposedWorkerClasses(t *testing.T) {
	for _, expected := range []string{
		"public.queue_outbox",
		"public.queue_inbox",
		"public.scheduler_leases",
		"public.analytics_snapshots",
		"public.calendar_events",
		"public.compliance_exceptions",
		"public.dsr_requests",
		"public.evidence_collection_configs",
		"public.notifications",
		"public.policy_versions",
		"public.report_schedules",
		"public.search_index",
		"public.workflow_step_executions",
		"public.effective_user_roles",
		"public.access_sod_rules",
		"public.access_sod_exceptions",
		"public.schema_migrations",
		"public.notification_due_tenants(integer)",
		"public.incident_due_tenants(integer)",
		"public.vendor_due_tenants(integer)",
		"public.evidence_due_tenants(integer,uuid)",
		"public.expire_due_evidence(uuid,integer)",
	} {
		if !strings.Contains(workerDatabasePostureQuery, expected) {
			t.Fatalf("worker posture query does not cover %q", expected)
		}
	}
}

func TestValidateWorkerDatabasePostureRejectsNilAndCatalogFailure(t *testing.T) {
	if err := ValidateWorkerDatabasePosture(context.Background(), nil, true); err == nil {
		t.Fatal("expected nil-query failure")
	}
	want := errors.New("catalog unavailable")
	if err := ValidateWorkerDatabasePosture(context.Background(), postureQuerierStub{err: want}, true); !errors.Is(err, want) {
		t.Fatalf("ValidateWorkerDatabasePosture() error = %v, want wrapped %v", err, want)
	}
}
