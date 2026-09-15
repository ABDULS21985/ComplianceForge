package repository

import "testing"

func TestNewDiagnosticsRepositoryRequiresPool(t *testing.T) {
	if _, err := NewDiagnosticsRepository(nil); err == nil {
		t.Fatal("constructor accepted nil pool")
	}
}
