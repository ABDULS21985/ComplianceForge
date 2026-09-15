package database

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRuntimeDatabaseLoginIdentityRejectsCredentialRoleMasquerading(t *testing.T) {
	for _, test := range []struct {
		name, configured, current, session string
		queryError                         error
		allowed                            bool
	}{
		{name: "dedicated login", configured: "app_api", current: "app_api", session: "app_api", allowed: true},
		{name: "SET ROLE hides privileged login", configured: "schema_owner", current: "app_api", session: "schema_owner"},
		{name: "SET SESSION AUTHORIZATION hides configured login", configured: "schema_owner", current: "app_api", session: "app_api"},
		{name: "session principal mismatch", configured: "app_api", current: "app_api", session: "schema_owner"},
		{name: "blank configured login", current: "app_api", session: "app_api"},
		{name: "query fails safely", configured: "app_api", queryError: errors.New("postgres://secret-user:secret-password@secret-host")},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection := loginIdentityQuerierStub{current: test.current, session: test.session, err: test.queryError}
			err := ValidateRuntimeDatabaseLoginIdentity(context.Background(), connection, test.configured)
			if (err == nil) != test.allowed {
				t.Fatalf("allowed=%v error=%v", test.allowed, err)
			}
			if err != nil && (strings.Contains(err.Error(), "schema_owner") || strings.Contains(err.Error(), "secret-")) {
				t.Fatalf("startup identity error exposed credential details: %v", err)
			}
		})
	}
	if err := ValidateRuntimeDatabaseLoginIdentity(context.Background(), nil, "app_api"); err == nil {
		t.Fatal("nil identity query was allowed")
	}
}

type loginIdentityQuerierStub struct {
	current, session string
	err              error
}

func (q loginIdentityQuerierStub) QueryRow(context.Context, string, ...any) pgx.Row {
	return loginIdentityRowStub(q)
}

type loginIdentityRowStub loginIdentityQuerierStub

func (r loginIdentityRowStub) Scan(destinations ...any) error {
	if r.err != nil {
		return r.err
	}
	*destinations[0].(*string) = r.current
	*destinations[1].(*string) = r.session
	return nil
}
