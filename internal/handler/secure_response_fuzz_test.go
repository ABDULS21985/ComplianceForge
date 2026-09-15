package handler

import (
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzAttachmentResponseHeaders(f *testing.F) {
	for _, seed := range []struct{ name, contentType string }{
		{"evidence.pdf", "application/pdf"},
		{"../../secret\r\nX-Injection: yes", "text/html"},
		{"résumé.csv", "text/csv"},
		{"", "application/octet-stream"},
	} {
		f.Add(seed.name, seed.contentType)
	}

	f.Fuzz(func(t *testing.T, name, contentType string) {
		if len(name) > 8192 || len(contentType) > 2048 {
			t.Skip()
		}
		response := httptest.NewRecorder()
		writeAttachment(response, name, contentType, []byte("bounded fuzz payload"))
		if response.Code != http.StatusOK {
			t.Fatalf("attachment status = %d", response.Code)
		}
		safeName := sanitizeAttachmentFilename(name)
		if safeName == "" || len(safeName) > 200 || !utf8.ValidString(safeName) || strings.ContainsAny(safeName, "\r\n/\\") {
			t.Fatalf("unsafe filename remained: %q", safeName)
		}
		mediaType, parameters, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
		if err != nil || mediaType != "attachment" || parameters["filename"] == "" {
			t.Fatalf("invalid content disposition %q: %v", response.Header().Get("Content-Disposition"), err)
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("required attachment security headers are missing")
		}
	})
}

func FuzzStylesheetResponseRejectsHTMLSentinels(f *testing.F) {
	for _, seed := range []string{"body{color:#000}", "</style><script>alert(1)</script>", "/* <!-- */", "a{content:'\x00'}"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, css string) {
		if len(css) > maxStylesheetBytes+1024 {
			t.Skip()
		}
		response := httptest.NewRecorder()
		writeStylesheet(response, css)
		invalid := len(css) > maxStylesheetBytes || !utf8.ValidString(css) || containsHTMLSentinel(css)
		if invalid && response.Code == http.StatusOK {
			t.Fatal("unsafe stylesheet was accepted")
		}
		if !invalid && response.Code != http.StatusOK {
			t.Fatalf("safe stylesheet was rejected with %d", response.Code)
		}
	})
}
