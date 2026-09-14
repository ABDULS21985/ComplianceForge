package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteAttachmentEncodesFilenameAndForcesActiveContentOpaque(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeAttachment(recorder, "../report\r\nX-Evil: yes.html", "text/html", []byte("<script>alert(1)</script>"))

	result := recorder.Result()
	defer result.Body.Close()
	if result.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("Content-Type = %q", result.Header.Get("Content-Type"))
	}
	if disposition := result.Header.Get("Content-Disposition"); !strings.HasPrefix(disposition, "attachment;") || strings.ContainsAny(disposition, "\r\n") {
		t.Fatalf("unsafe Content-Disposition = %q", disposition)
	}
	if result.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff policy")
	}
}

func TestWriteStylesheetRejectsHTMLSentinel(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeStylesheet(recorder, "body{color:red}</style><script>alert(1)</script>")
	if recorder.Code != 500 || !strings.Contains(recorder.Body.String(), "Branding CSS is invalid") {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestWriteStylesheetSetsStrictResponseHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeStylesheet(recorder, ":root { --cf-primary: #fff; }")
	if recorder.Code != 200 || recorder.Header().Get("Content-Type") != "text/css; charset=utf-8" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected stylesheet response: status=%d headers=%v", recorder.Code, recorder.Header())
	}
}
