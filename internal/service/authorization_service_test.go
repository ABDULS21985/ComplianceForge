package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/database"
)

type authorizationQuerierStub struct {
	allowed bool
	err     error
	args    []any
}

func (*authorizationQuerierStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("unexpected Exec")
}
func (*authorizationQuerierStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected Query")
}
func (q *authorizationQuerierStub) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	q.args = args
	return authorizationRowStub{allowed: q.allowed, err: q.err}
}

type authorizationRowStub struct {
	allowed bool
	err     error
}

func (r authorizationRowStub) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*bool)) = r.allowed
	return nil
}

func TestValidPermissionComponent(t *testing.T) {
	for _, value := range []string{"risks", "control_evidence", "read-all", "v2"} {
		if !validPermissionComponent(value) {
			t.Errorf("validPermissionComponent(%q) = false", value)
		}
	}
	for _, value := range []string{"", "Risks", "risk.read", "risk/read", "contains space", string(make([]byte, 65))} {
		if validPermissionComponent(value) {
			t.Errorf("validPermissionComponent(%q) = true", value)
		}
	}
}

func TestRBACAuthorizerReturnsPersistedPermissionDecision(t *testing.T) {
	for _, test := range []struct {
		name    string
		allowed bool
	}{{"granted role permission", true}, {"default deny", false}} {
		t.Run(test.name, func(t *testing.T) {
			querier := &authorizationQuerierStub{allowed: test.allowed}
			authorizer := &RBACAuthorizer{}
			decision, err := authorizer.Authorize(database.WithQuerier(context.Background(), querier), authz.Request{
				SubjectID: "10000000-0000-0000-0000-000000000001", OrganizationID: "20000000-0000-0000-0000-000000000001",
				Resource: "Controls", Action: "READ",
			})
			if err != nil || decision.Allowed != test.allowed {
				t.Fatalf("decision=%#v err=%v", decision, err)
			}
			if len(querier.args) != 4 || querier.args[2] != "controls" || querier.args[3] != "read" {
				t.Fatalf("query args=%#v", querier.args)
			}
		})
	}
}

func TestRBACAuthorizerFailsClosedOnStoreError(t *testing.T) {
	want := errors.New("database unavailable")
	authorizer := &RBACAuthorizer{}
	decision, err := authorizer.Authorize(database.WithQuerier(context.Background(), &authorizationQuerierStub{err: want}), authz.Request{
		SubjectID: "10000000-0000-0000-0000-000000000001", OrganizationID: "20000000-0000-0000-0000-000000000001", Resource: "controls", Action: "read",
	})
	if err == nil || decision.Allowed {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
}
