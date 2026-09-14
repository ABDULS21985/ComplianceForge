package repository

import (
	"errors"
	"testing"
)

func TestEntitlementLimitErrorSupportsStableClassification(t *testing.T) {
	err := &EntitlementLimitError{Metric: "users", Limit: 5, Usage: 5, Additional: 1}
	if !errors.Is(err, ErrEntitlementLimitExceeded) {
		t.Fatal("entitlement limit error did not unwrap to the stable sentinel")
	}
	if got := err.Error(); got == "" || got == ErrEntitlementLimitExceeded.Error() {
		t.Fatalf("error lacks actionable context: %q", got)
	}
}
