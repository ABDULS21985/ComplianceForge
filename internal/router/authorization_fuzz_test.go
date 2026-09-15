package router

import (
	"net/http"
	"strings"
	"testing"
)

func FuzzProtectedRoutePermissionMapping(f *testing.F) {
	f.Add(http.MethodGet, "/api/v1/risks")
	f.Add(http.MethodDelete, "/api/v1/vendors/10000000-0000-0000-0000-000000000001")
	f.Add(http.MethodGet, "/api/v1/settings/diagnostics")
	f.Add("TRACE", "/api/v1/unknown")
	f.Add(http.MethodPost, "/api/v1/access/my-permissions")

	f.Fuzz(func(t *testing.T, method, path string) {
		if len(method) > 32 || len(path) > 8192 {
			t.Skip()
		}
		permission, ok := permissionForProtectedRequest(method, path)
		if !ok {
			return
		}
		if permission.Resource == "" || permission.Action == "" || !supportedResourceActions[permission.Resource][permission.Action] {
			t.Fatalf("mapper returned unsupported permission: %#v", permission)
		}
		trimmed := strings.Trim(strings.TrimPrefix(path, "/api/v1/"), "/")
		if trimmed == "" {
			t.Fatal("empty namespace received a permission")
		}
		segment := strings.Split(trimmed, "/")[0]
		if _, known := protectedResourceAliases[segment]; !known {
			t.Fatalf("unknown namespace %q received a permission", segment)
		}
	})
}
