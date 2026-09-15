package database

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestValidateAPIDatabasePostureAcceptsSeparatedRuntimeRole(t *testing.T) {
	posture := apiDatabasePosture{roleName: "complianceforge_api_runtime"}
	if err := validateAPIDatabasePosture(posture); err != nil {
		t.Fatalf("validateAPIDatabasePosture() error = %v", err)
	}
}

func TestValidateAPIDatabasePostureRejectsPrivilegeAndCapabilityDrift(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*apiDatabasePosture)
		wantErr string
	}{
		{name: "empty role", mutate: func(posture *apiDatabasePosture) { posture.roleName = " " }, wantErr: "empty runtime role"},
		{name: "superuser", mutate: func(posture *apiDatabasePosture) { posture.superuser = true }, wantErr: "SUPERUSER"},
		{name: "bypass rls", mutate: func(posture *apiDatabasePosture) { posture.bypassRLS = true }, wantErr: "BYPASSRLS"},
		{name: "database owner", mutate: func(posture *apiDatabasePosture) { posture.ownsCurrentDatabase = true }, wantErr: "current database owner"},
		{name: "database create", mutate: func(posture *apiDatabasePosture) { posture.canCreateCurrentDatabase = true }, wantErr: "create schemas"},
		{name: "schema owner", mutate: func(posture *apiDatabasePosture) { posture.ownsPublicSchema = true }, wantErr: "public schema owner"},
		{name: "schema create", mutate: func(posture *apiDatabasePosture) { posture.canCreatePublicSchema = true }, wantErr: "create objects"},
		{name: "missing API group", mutate: func(posture *apiDatabasePosture) { posture.missingAPIMembership = true }, wantErr: "not a member of complianceforge_api"},
		{name: "unexpected membership", mutate: func(posture *apiDatabasePosture) { posture.unexpectedMemberships = []string{"legacy_writer"} }, wantErr: "unexpected memberships"},
		{
			name: "capability owner member",
			mutate: func(posture *apiDatabasePosture) {
				posture.evidenceOwnerMemberships = []string{"complianceforge_evidence_chain_owner"}
			},
			wantErr: "member of evidence capability owner roles",
		},
		{
			name: "public relation owner",
			mutate: func(posture *apiDatabasePosture) {
				posture.ownedPublicRelations = []string{"public.risks"}
			},
			wantErr: "owners of public relations",
		},
		{
			name: "protected relation owner",
			mutate: func(posture *apiDatabasePosture) {
				posture.ownedProtectedRelations = []string{"public.evidence_custody_events"}
			},
			wantErr: "owns protected evidence relations",
		},
		{
			name: "missing API table privilege",
			mutate: func(posture *apiDatabasePosture) {
				posture.missingTablePrivileges = []string{"public.risks:SELECT"}
			},
			wantErr: "required API table privileges",
		},
		{
			name: "unexpected API table privilege",
			mutate: func(posture *apiDatabasePosture) {
				posture.unexpectedTablePrivileges = []string{"public.scheduler_leases:DELETE"}
			},
			wantErr: "outside the API allowlist",
		},
		{
			name: "unexpected sequence privilege",
			mutate: func(posture *apiDatabasePosture) {
				posture.unexpectedSequenceGrants = []string{"public.legacy_id_seq:USAGE"}
			},
			wantErr: "unexpected sequence privileges",
		},
		{
			name: "direct immutable write",
			mutate: func(posture *apiDatabasePosture) {
				posture.writableImmutableLedgers = []string{"public.evidence_reviews"}
			},
			wantErr: "write immutable evidence ledgers directly",
		},
		{
			name: "missing wrapper",
			mutate: func(posture *apiDatabasePosture) {
				posture.missingCapabilities = []string{"public.submit_evidence_review(uuid,uuid,uuid,uuid,text,text,text)"}
			},
			wantErr: "lacks required evidence capabilities",
		},
		{
			name: "unsafe wrapper",
			mutate: func(posture *apiDatabasePosture) {
				posture.misconfiguredCapabilities = []string{"public.append_evidence_access_custody_event(uuid,uuid,uuid,uuid,text,text,text)"}
			},
			wantErr: "unsafe definitions",
		},
		{
			name: "worker tenant capability",
			mutate: func(posture *apiDatabasePosture) {
				posture.workerCapabilities = []string{"public.evidence_due_tenants(integer,uuid)"}
			},
			wantErr: "worker-only capabilities",
		},
		{name: "PUBLIC capability", mutate: func(p *apiDatabasePosture) {
			p.publicPrivileges = []string{"resolve_api_key_tenant(text):EXECUTE:PUBLIC"}
		}, wantErr: "PUBLIC has runtime"},
		{name: "unreviewed definer", mutate: func(p *apiDatabasePosture) { p.unexpectedDefiners = []string{"set_tenant(uuid)"} }, wantErr: "unreviewed SECURITY DEFINER"},
		{name: "unsafe queue RLS", mutate: func(p *apiDatabasePosture) { p.queueIsolationViolations = []string{"queue_outbox:FORCE_RLS"} }, wantErr: "queue isolation"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			posture := apiDatabasePosture{roleName: "complianceforge_api_runtime"}
			test.mutate(&posture)
			err := validateAPIDatabasePosture(posture)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateAPIDatabasePosture() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestAPIDatabasePostureQueryPinsMigration58TrustBoundary(t *testing.T) {
	for _, expected := range []string{
		"complianceforge_evidence_chain_owner",
		"complianceforge_evidence_registry_owner",
		"public.evidence_reviews",
		"public.evidence_custody_chain_heads",
		"public.evidence_custody_events",
		"public.evidence_scheduler_tenants",
		"public.schema_migrations",
		"public.queue_inbox",
		"required_api_table_privileges",
		"api_table_manifest",
		"public.access_review_campaigns",
		"public.effective_user_roles",
		"VERSION_OR_DIRTY",
		"pg_catalog,public,pg_temp",
		"public.resolve_api_key_tenant(text)",
		"public.resolve_scim_token_tenant(text)",
		"queue_outbox_tenant_insert",
		"relforcerowsecurity",
		"public.submit_evidence_review(uuid,uuid,uuid,uuid,text,text,text)",
		"public.append_evidence_access_custody_event(uuid,uuid,uuid,uuid,text,text,text)",
		"public.verify_evidence_custody_chain(uuid,uuid)",
		"public.notification_due_tenants(integer)",
		"public.incident_due_tenants(integer)",
		"public.vendor_due_tenants(integer)",
		"public.evidence_due_tenants(integer,uuid)",
		"public.expire_due_evidence(uuid,integer)",
	} {
		if !strings.Contains(apiDatabasePostureQuery, expected) {
			t.Fatalf("posture query does not cover %q", expected)
		}
	}
}

func TestValidateAPIDatabasePostureRejectsNilQuerier(t *testing.T) {
	err := ValidateAPIDatabasePosture(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "requires a database connection") {
		t.Fatalf("ValidateAPIDatabasePosture() error = %v", err)
	}
}

func TestValidateAPIDatabasePostureReportsCatalogFailure(t *testing.T) {
	want := errors.New("catalog unavailable")
	err := ValidateAPIDatabasePosture(context.Background(), postureQuerierStub{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("ValidateAPIDatabasePosture() error = %v, want wrapped %v", err, want)
	}
}

type postureQuerierStub struct{ err error }

func (q postureQuerierStub) QueryRow(context.Context, string, ...any) pgx.Row {
	return pgxRowStub{err: q.err}
}

type pgxRowStub struct{ err error }

func (r pgxRowStub) Scan(...any) error { return r.err }
