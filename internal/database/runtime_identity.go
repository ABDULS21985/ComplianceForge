package database

import (
	"context"
	"errors"
	"strings"
)

const runtimeDatabaseLoginIdentityQuery = `SELECT current_user::TEXT, session_user::TEXT`

// ValidateRuntimeDatabaseLoginIdentity binds catalog posture to the actual
// configured credential, not merely a role selected by startup parameters.
// expectedLogin must come from pgx's parsed connection configuration, never
// from current_user or a client-supplied request value.
func ValidateRuntimeDatabaseLoginIdentity(ctx context.Context, connection APIPostureQuerier, expectedLogin string) error {
	if connection == nil || strings.TrimSpace(expectedLogin) == "" {
		return errors.New("runtime database login identity requires a connection and configured login")
	}
	var currentRole, sessionRole string
	if err := connection.QueryRow(ctx, runtimeDatabaseLoginIdentityQuery).Scan(&currentRole, &sessionRole); err != nil {
		// Startup errors must not echo connection credentials or a DSN.
		return errors.New("runtime database login identity query failed")
	}
	if currentRole != expectedLogin || sessionRole != expectedLogin {
		return errors.New("configured database login does not match session and current role; runtime role switching is prohibited")
	}
	return nil
}
