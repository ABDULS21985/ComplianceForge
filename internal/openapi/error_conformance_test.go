package openapi

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHTTPErrorPathsUseCanonicalEnvelope is a source-level regression gate for
// response paths that historically bypassed models.ErrorResponse. Runtime
// schema validation covers representative responses; this scan makes it hard
// to reintroduce http.Error, RFC-7807-shaped middleware bodies, or the legacy
// one-field {"error": ...} payload in another endpoint.
func TestHTTPErrorPathsUseCanonicalEnvelope(t *testing.T) {
	root := repositoryRoot(t)
	directories := []string{
		filepath.Join(root, "cmd"),
		filepath.Join(root, "internal", "handler"),
		filepath.Join(root, "internal", "middleware"),
		filepath.Join(root, "internal", "observability"),
		filepath.Join(root, "internal", "router"),
	}
	for _, directory := range directories {
		err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			source := string(contents)
			forbidden := []string{
				"http.Error(",
				"application/problem+json",
				"`{\"error\"",
				"map[string]interface{}{\n\t\t\t\"error\"",
				"map[string]any{\n\t\t\t\"error\"",
			}
			for _, token := range forbidden {
				if strings.Contains(source, token) {
					relative, relErr := filepath.Rel(root, path)
					if relErr != nil {
						relative = path
					}
					t.Errorf("%s contains forbidden legacy HTTP error token %q; use the canonical API response writer", relative, token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", directory, err)
		}
	}
}
