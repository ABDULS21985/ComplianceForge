package safehttp

import (
	"net"
	"net/url"
	"strings"
	"testing"
)

func FuzzValidateURL(f *testing.F) {
	for _, seed := range []struct{ raw, allowed string }{
		{"https://example.com/hook", "example.com"},
		{"http://127.0.0.1/admin", ""},
		{"https://user:secret@example.com", "example.com"},
		{"https://[::1]/", ""},
		{"https://example.com/#fragment", "example.com"},
		{"\x00", ""},
	} {
		f.Add(seed.raw, seed.allowed)
	}

	f.Fuzz(func(t *testing.T, raw, allowedHost string) {
		if len(raw) > 8192 || len(allowedHost) > 512 {
			t.Skip()
		}
		destination, err := url.Parse(raw)
		if err != nil {
			return
		}
		policy := Policy{}
		if allowedHost != "" {
			policy.AllowedHosts = []string{allowedHost}
		}
		if ValidateURL(destination, policy) != nil {
			return
		}
		if !strings.EqualFold(destination.Scheme, "https") || destination.Host == "" || destination.User != nil || destination.Fragment != "" {
			t.Fatalf("unsafe URL was accepted: %q", raw)
		}
		hostname := strings.ToLower(strings.TrimSuffix(destination.Hostname(), "."))
		if hostname == "" || hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") ||
			strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".internal") {
			t.Fatalf("local hostname was accepted: %q", hostname)
		}
		if parsed := net.ParseIP(hostname); parsed != nil && !isPublicIP(parsed) {
			t.Fatalf("non-public IP was accepted: %q", hostname)
		}
		normalized := normalizePolicy(policy)
		if len(normalized.AllowedHosts) > 0 && !contains(normalized.AllowedHosts, hostname) {
			t.Fatalf("host outside allowlist was accepted: %q", hostname)
		}
	})
}
