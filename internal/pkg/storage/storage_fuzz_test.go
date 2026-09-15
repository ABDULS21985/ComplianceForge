package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzLocalStoragePathContainment(f *testing.F) {
	for _, seed := range []string{
		"tenant/evidence.pdf", "../outside", "/etc/passwd", ".", "", "tenant/../../outside", "bad\x00path", `tenant\..\outside`,
	} {
		f.Add(seed)
	}
	root := filepath.Clean(filepath.Join(string(filepath.Separator), "complianceforge-fuzz-storage"))
	store := &LocalStorageService{basePath: root}

	f.Fuzz(func(t *testing.T, candidate string) {
		if len(candidate) > 8192 {
			t.Skip()
		}
		fullPath, relativePath, err := store.resolve(candidate)
		if err != nil {
			return
		}
		verifiedRelative, relErr := filepath.Rel(root, fullPath)
		if relErr != nil || verifiedRelative == "." || verifiedRelative == ".." ||
			strings.HasPrefix(verifiedRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(verifiedRelative) {
			t.Fatalf("resolved path escaped root: input=%q full=%q relative=%q", candidate, fullPath, relativePath)
		}
		if relativePath != verifiedRelative || strings.ContainsRune(relativePath, '\x00') {
			t.Fatalf("inconsistent resolved path: input=%q relative=%q verified=%q", candidate, relativePath, verifiedRelative)
		}
	})
}
