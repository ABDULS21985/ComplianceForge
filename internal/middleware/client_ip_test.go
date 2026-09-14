package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyHeaders(t *testing.T) {
	tests := []struct {
		name       string
		trusted    bool
		header     string
		remoteAddr string
		want       string
	}{
		{name: "direct request ignores spoofed header", header: "203.0.113.9", remoteAddr: "192.0.2.10:1234", want: "192.0.2.10"},
		{name: "trusted proxy accepts exact real IP", trusted: true, header: "203.0.113.9", remoteAddr: "172.20.0.3:1234", want: "203.0.113.9"},
		{name: "trusted proxy rejects forwarded chain", trusted: true, header: "198.51.100.2, 203.0.113.9", remoteAddr: "172.20.0.3:1234", want: "172.20.0.3"},
		{name: "malformed remote", remoteAddr: "not-an-address", want: "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got string
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = GetClientIPFromContext(r.Context())
			})
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Real-IP", test.header)
			TrustedProxyHeaders(test.trusted)(next).ServeHTTP(httptest.NewRecorder(), request)
			if got != test.want {
				t.Fatalf("client IP = %q, want %q", got, test.want)
			}
		})
	}
}
