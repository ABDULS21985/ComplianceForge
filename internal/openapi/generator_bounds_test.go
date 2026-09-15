package openapi

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperatorSelectedContractParsingIsBounded(t *testing.T) {
	for _, test := range []struct {
		name         string
		base, routes []byte
		want         string
	}{
		{"base byte ceiling", bytes.Repeat([]byte(" "), maximumOpenAPIBaseBytes+1), nil, "base exceeds"},
		{"catalog byte ceiling", []byte(`{"openapi":"3.1.0"}`), bytes.Repeat([]byte("# catalog comment\n"), maximumRouteCatalogBytes/18+1), "catalog exceeds"},
		{"trailing document", []byte(`{"openapi":"3.1.0"} {"unexpected":true}`), nil, "exactly one JSON"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			base, routes := filepath.Join(directory, "operator-base.json"), filepath.Join(directory, "operator-routes.csv")
			if err := os.WriteFile(base, test.base, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(routes, test.routes, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Generate(base, routes); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("bounded generation error=%v, want %q", err, test.want)
			}
		})
	}
}
