package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/authz"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const (
	controlEvidenceHandlerOrg     = "10000000-0000-0000-0000-000000000001"
	controlEvidenceHandlerUser    = "20000000-0000-0000-0000-000000000002"
	controlEvidenceHandlerControl = "30000000-0000-0000-0000-000000000003"
	controlEvidenceHandlerID      = "40000000-0000-0000-0000-000000000004"
)

type controlServiceStub struct {
	attachInput models.AttachControlEvidenceInput
}

func (*controlServiceStub) ListControls(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error) {
	return nil, 0, nil
}
func (*controlServiceStub) GetControl(context.Context, string, string) (*models.Control, error) {
	return &models.Control{}, nil
}
func (*controlServiceStub) UpdateControlImplementation(context.Context, string, string, models.ControlImplementationPatch) (*models.ControlImplementation, error) {
	return &models.ControlImplementation{}, nil
}
func (stub *controlServiceStub) AttachControlEvidence(_ context.Context, _, _, _ string, input models.AttachControlEvidenceInput) (*models.ControlEvidence, error) {
	stub.attachInput = input
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: controlEvidenceHandlerID}}, nil
}
func (*controlServiceStub) ListControlEvidence(context.Context, string, string, models.PaginationRequest) ([]models.ControlEvidence, int, error) {
	return nil, 0, nil
}

type evidenceObjectHandlerStub struct {
	ready          bool
	organizationID string
	userID         string
	controlID      string
	filename       string
	contentType    string
	contents       []byte
	metadata       service.EvidenceUploadMetadata
	download       *service.EvidenceDownload
	reviewInput    models.ReviewControlEvidenceInput
	err            error
}

func (stub *evidenceObjectHandlerStub) Ready() bool { return stub.ready }

func (stub *evidenceObjectHandlerStub) Upload(_ context.Context, organizationID, userID, controlID, filename, contentType string, reader io.Reader, metadata service.EvidenceUploadMetadata) (*models.ControlEvidence, error) {
	stub.organizationID, stub.userID, stub.controlID = organizationID, userID, controlID
	stub.filename, stub.contentType, stub.metadata = filename, contentType, metadata
	stub.contents, _ = io.ReadAll(reader)
	if stub.err != nil {
		return nil, stub.err
	}
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: controlEvidenceHandlerID}, Title: metadata.Title}, nil
}

func (stub *evidenceObjectHandlerStub) Supersede(ctx context.Context, organizationID, userID, controlID, _ string, filename, contentType string, reader io.Reader, metadata service.EvidenceUploadMetadata) (*models.ControlEvidence, error) {
	return stub.Upload(ctx, organizationID, userID, controlID, filename, contentType, reader, metadata)
}

func (stub *evidenceObjectHandlerStub) Download(context.Context, string, string, string) (*service.EvidenceDownload, error) {
	return stub.download, stub.err
}

func (stub *evidenceObjectHandlerStub) Review(_ context.Context, organizationID, userID, controlID, _ string, input models.ReviewControlEvidenceInput) (*models.ControlEvidence, error) {
	stub.organizationID, stub.userID, stub.controlID, stub.reviewInput = organizationID, userID, controlID, input
	if stub.err != nil {
		return nil, stub.err
	}
	return &models.ControlEvidence{BaseModel: models.BaseModel{ID: controlEvidenceHandlerID}, ReviewStatus: input.Status}, nil
}

type evidenceLifecycleHandlerStub struct {
	ready  bool
	result *models.EvidenceIntegrityResult
	err    error
}

func (stub evidenceLifecycleHandlerStub) Ready() bool { return stub.ready }
func (evidenceLifecycleHandlerStub) History(context.Context, string, string, string) (*models.EvidenceLifecycleRecord, error) {
	return &models.EvidenceLifecycleRecord{}, nil
}
func (stub evidenceLifecycleHandlerStub) VerifyIntegrity(context.Context, string, string, string, string, string) (*models.EvidenceIntegrityResult, error) {
	if stub.result == nil && stub.err == nil {
		stub.result = &models.EvidenceIntegrityResult{}
	}
	return stub.result, stub.err
}
func (evidenceLifecycleHandlerStub) RecordDownloadAuthorization(context.Context, string, string, string, string, string, string) error {
	return nil
}

func newControlEvidenceHandler(objects EvidenceObjectService) *ControlHandler {
	return NewControlHandler(&controlServiceStub{},
		WithEvidenceObjectService(objects, 1<<20),
		WithEvidenceLifecycleService(evidenceLifecycleHandlerStub{ready: true}),
	)
}

func TestControlHandlerDistinguishesCustodyCorruptionFromObjectMismatch(t *testing.T) {
	for _, test := range []struct {
		name        string
		err         error
		wantMessage string
	}{
		{name: "custody chain", err: service.ErrEvidenceCustodyChainInvalid, wantMessage: "custody history is inconsistent"},
		{name: "object mismatch", err: service.ErrEvidenceIntegrityFailed, wantMessage: "stored object no longer matches"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := NewControlHandler(&controlServiceStub{},
				WithEvidenceObjectService(&evidenceObjectHandlerStub{ready: true}, 1<<20),
				WithEvidenceLifecycleService(evidenceLifecycleHandlerStub{ready: true, err: test.err}),
			)
			response := httptest.NewRecorder()
			handler.VerifyEvidenceIntegrity(response, controlEvidenceRequest(http.MethodPost, "/verify-integrity", nil))
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), test.wantMessage) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestControlHandlerRedactsOperationalIntegrityFailure(t *testing.T) {
	providerError := "s3 rejected credential AKIA-secret"
	handler := NewControlHandler(&controlServiceStub{},
		WithEvidenceObjectService(&evidenceObjectHandlerStub{ready: true}, 1<<20),
		WithEvidenceLifecycleService(evidenceLifecycleHandlerStub{
			ready: true,
			err:   fmt.Errorf("%w: %s", service.ErrEvidenceIntegrityUnavailable, providerError),
		}),
	)
	response := httptest.NewRecorder()
	handler.VerifyEvidenceIntegrity(response, controlEvidenceRequest(http.MethodPost, "/verify-integrity", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), providerError) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func controlEvidenceRequest(method, target string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, target, body)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, controlEvidenceHandlerOrg)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, controlEvidenceHandlerUser)
	ctx = context.WithValue(ctx, middleware.ContextKeyRequestID, "request-evidence-1")
	ctx = middleware.ContextWithAuthorizationDecision(ctx, authz.Decision{Allowed: true})
	route := chi.NewRouteContext()
	route.URLParams.Add("id", controlEvidenceHandlerControl)
	route.URLParams.Add("evidenceID", controlEvidenceHandlerID)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
	return request.WithContext(ctx)
}

func multipartEvidenceRequest(t *testing.T, fields map[string][]string, filename string, contents []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(name, value); err != nil {
				t.Fatalf("WriteField() error = %v", err)
			}
		}
	}
	if filename != "" {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
		header.Set("Content-Type", "application/pdf")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("CreatePart() error = %v", err)
		}
		if _, err := part.Write(contents); err != nil {
			t.Fatalf("part Write() error = %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart Close() error = %v", err)
	}
	request := controlEvidenceRequest(http.MethodPost, "/controls/"+controlEvidenceHandlerControl+"/evidence", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestControlHandlerStreamsMultipartEvidenceIntoSecurityService(t *testing.T) {
	objects := &evidenceObjectHandlerStub{ready: true}
	handler := newControlEvidenceHandler(objects)
	request := multipartEvidenceRequest(t, map[string][]string{
		"title": {"Access review"}, "description": {"Quarterly review"}, "evidence_type": {"document"},
		"valid_from": {"2026-09-01"}, "valid_until": {"2026-12-01"}, "metadata": {`{"source":"manual"}`},
	}, "review.pdf", []byte("%PDF-evidence"))
	response := httptest.NewRecorder()
	handler.AttachEvidence(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if objects.organizationID != controlEvidenceHandlerOrg || objects.userID != controlEvidenceHandlerUser || objects.controlID != controlEvidenceHandlerControl ||
		objects.filename != "review.pdf" || objects.contentType != "application/pdf" || string(objects.contents) != "%PDF-evidence" {
		t.Fatalf("upload call = %#v", objects)
	}
	if objects.metadata.Title != "Access review" || objects.metadata.EvidenceType != "document" || objects.metadata.ValidFrom == nil || objects.metadata.ValidUntil == nil {
		t.Fatalf("metadata = %#v", objects.metadata)
	}
}

func TestControlHandlerRejectsMalformedMultipartBeforeSecurityService(t *testing.T) {
	tests := []struct {
		name     string
		fields   map[string][]string
		filename string
	}{
		{name: "missing file", fields: map[string][]string{"title": {"Evidence"}, "evidence_type": {"document"}}},
		{name: "unknown field", fields: map[string][]string{"title": {"Evidence"}, "evidence_type": {"document"}, "attacker": {"value"}}, filename: "file.pdf"},
		{name: "duplicate field", fields: map[string][]string{"title": {"One", "Two"}, "evidence_type": {"document"}}, filename: "file.pdf"},
		{name: "bad date", fields: map[string][]string{"title": {"Evidence"}, "evidence_type": {"document"}, "valid_from": {"tomorrow"}}, filename: "file.pdf"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects := &evidenceObjectHandlerStub{ready: true}
			handler := newControlEvidenceHandler(objects)
			response := httptest.NewRecorder()
			handler.AttachEvidence(response, multipartEvidenceRequest(t, test.fields, test.filename, []byte("%PDF")))
			if response.Code != http.StatusBadRequest || objects.filename != "" {
				t.Fatalf("status=%d body=%s upload=%#v", response.Code, response.Body.String(), objects)
			}
		})
	}
}

func TestControlHandlerRejectsClientSuppliedFileMetadataJSON(t *testing.T) {
	metadataService := &controlServiceStub{}
	handler := NewControlHandler(metadataService)
	request := controlEvidenceRequest(http.MethodPost, "/controls/id/evidence", strings.NewReader(`{"title":"Spoofed","evidence_type":"document","file_name":"server.pdf","file_hash":"attacker"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.AttachEvidence(response, request)
	if response.Code != http.StatusBadRequest || metadataService.attachInput.Title != "" {
		t.Fatalf("status=%d body=%s input=%#v", response.Code, response.Body.String(), metadataService.attachInput)
	}
}

func TestControlHandlerEvidenceDownloadRedirectAndStream(t *testing.T) {
	t.Run("signed redirect", func(t *testing.T) {
		objects := &evidenceObjectHandlerStub{ready: true, download: &service.EvidenceDownload{SignedURL: "https://signed.example.test/object"}}
		handler := newControlEvidenceHandler(objects)
		response := httptest.NewRecorder()
		handler.DownloadEvidence(response, controlEvidenceRequest(http.MethodGet, "/download", nil))
		if response.Code != http.StatusTemporaryRedirect || response.Header().Get("Location") != objects.download.SignedURL || response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("status=%d headers=%#v", response.Code, response.Header())
		}
	})

	t.Run("private stream", func(t *testing.T) {
		objects := &evidenceObjectHandlerStub{ready: true, download: &service.EvidenceDownload{
			Body: io.NopCloser(strings.NewReader("evidence")), Filename: "control.pdf", ContentType: "application/octet-stream", SizeBytes: 8,
		}}
		handler := newControlEvidenceHandler(objects)
		response := httptest.NewRecorder()
		handler.DownloadEvidence(response, controlEvidenceRequest(http.MethodGet, "/download", nil))
		if response.Code != http.StatusOK || response.Body.String() != "evidence" || response.Header().Get("X-Content-Type-Options") != "nosniff" ||
			response.Header().Get("Cache-Control") != "private, no-store" || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "attachment;") {
			t.Fatalf("status=%d headers=%#v body=%q", response.Code, response.Header(), response.Body.String())
		}
	})
}

type evidenceDownloadAuditCounter struct {
	evidenceLifecycleHandlerStub
	calls int
}

func (stub *evidenceDownloadAuditCounter) RecordDownloadAuthorization(context.Context, string, string, string, string, string, string) error {
	stub.calls++
	return nil
}

type evidenceDownloadCloseTracker struct {
	io.Reader
	closed bool
}

func (body *evidenceDownloadCloseTracker) Close() error { body.closed = true; return nil }

func TestControlHandlerRejectsInvalidPresignerResponseBeforeCustodyOrRedirect(t *testing.T) {
	for _, raw := range []string{
		"/relative", "//unreviewed.example.test/object", "http://unreviewed.example.test/object", "javascript:alert(1)",
		"https://user:REDIRECT_SECRET@unreviewed.example.test/object", "https://unreviewed.example.test/object#fragment",
		"https://unreviewed.example.test/object\r\nX-Injected: REDIRECT_SECRET", "https://unreviewed.example.test/%0d%0a",
		"https://unreviewed.example.test/object?token=REDIRECT_SECRET%0a", "https://unreviewed.example.test/object?bad=%zz",
		"https://unreviewed.example.test/back\\slash", "https://unreviewed.example.test/space object", "https:///missing-host",
		"https://unreviewed.example.test/" + strings.Repeat("x", 16<<10),
	} {
		t.Run(fmt.Sprintf("invalid URL length %d %q", len(raw), raw[:min(len(raw), 55)]), func(t *testing.T) {
			audit := &evidenceDownloadAuditCounter{evidenceLifecycleHandlerStub: evidenceLifecycleHandlerStub{ready: true}}
			objects := &evidenceObjectHandlerStub{ready: true, download: &service.EvidenceDownload{SignedURL: raw}}
			handler := NewControlHandler(&controlServiceStub{}, WithEvidenceObjectService(objects, 1<<20), WithEvidenceLifecycleService(audit))
			response := httptest.NewRecorder()
			handler.DownloadEvidence(response, controlEvidenceRequest(http.MethodGet, "/download", nil))
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Location") != "" ||
				response.Header().Get("Cache-Control") != "no-store" || audit.calls != 0 || strings.Contains(response.Body.String(), "REDIRECT_SECRET") {
				t.Fatalf("invalid response crossed custody/redirect boundary status=%d headers=%v audits=%d body=%s", response.Code, response.Header(), audit.calls, response.Body.String())
			}
		})
	}
	for _, download := range []*service.EvidenceDownload{nil, {}} {
		audit := &evidenceDownloadAuditCounter{evidenceLifecycleHandlerStub: evidenceLifecycleHandlerStub{ready: true}}
		objects := &evidenceObjectHandlerStub{ready: true, download: download}
		handler := NewControlHandler(&controlServiceStub{}, WithEvidenceObjectService(objects, 1<<20), WithEvidenceLifecycleService(audit))
		response := httptest.NewRecorder()
		handler.DownloadEvidence(response, controlEvidenceRequest(http.MethodGet, "/download", nil))
		if response.Code != http.StatusServiceUnavailable || audit.calls != 0 {
			t.Fatalf("missing delivery response recorded custody: status=%d audits=%d", response.Code, audit.calls)
		}
	}
	body := &evidenceDownloadCloseTracker{Reader: strings.NewReader("PRIVATE_BYTES")}
	audit := &evidenceDownloadAuditCounter{evidenceLifecycleHandlerStub: evidenceLifecycleHandlerStub{ready: true}}
	objects := &evidenceObjectHandlerStub{ready: true, download: &service.EvidenceDownload{SignedURL: "https://signed.example.test/object", Body: body}}
	handler := NewControlHandler(&controlServiceStub{}, WithEvidenceObjectService(objects, 1<<20), WithEvidenceLifecycleService(audit))
	response := httptest.NewRecorder()
	handler.DownloadEvidence(response, controlEvidenceRequest(http.MethodGet, "/download", nil))
	if response.Code != http.StatusServiceUnavailable || audit.calls != 0 || !body.closed || strings.Contains(response.Body.String(), "PRIVATE_BYTES") {
		t.Fatalf("mixed response was not closed/denied status=%d audits=%d closed=%v", response.Code, audit.calls, body.closed)
	}
}

func TestControlHandlerEvidenceDownloadFailsClosedWhenWatermarkIsRequired(t *testing.T) {
	objects := &evidenceObjectHandlerStub{ready: true, download: &service.EvidenceDownload{
		Body: io.NopCloser(strings.NewReader("must-not-leak")), Filename: "control.pdf", SizeBytes: 13,
	}}
	handler := newControlEvidenceHandler(objects)
	request := controlEvidenceRequest(http.MethodGet, "/download", nil)
	request = request.WithContext(middleware.ContextWithAuthorizationDecision(request.Context(), authz.Decision{
		Allowed: true, Obligations: []authz.Obligation{{Kind: "watermark", Parameters: map[string]string{"text": "External auditor copy"}}},
	}))
	response := httptest.NewRecorder()
	handler.DownloadEvidence(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "must-not-leak") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestControlHandlerEvidenceReviewUsesAuthenticatedTenantAndActor(t *testing.T) {
	objects := &evidenceObjectHandlerStub{ready: true}
	handler := newControlEvidenceHandler(objects)
	request := controlEvidenceRequest(http.MethodPost, "/review", strings.NewReader(`{"status":"accepted","comment":"Reviewed"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ReviewEvidence(response, request)
	if response.Code != http.StatusOK || objects.organizationID != controlEvidenceHandlerOrg || objects.userID != controlEvidenceHandlerUser ||
		objects.controlID != controlEvidenceHandlerControl || objects.reviewInput.Status != "accepted" {
		t.Fatalf("status=%d call=%#v body=%s", response.Code, objects, response.Body.String())
	}
	var decoded models.ControlEvidence
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil || decoded.ID != controlEvidenceHandlerID {
		t.Fatalf("response=%s error=%v", response.Body.String(), err)
	}
}

func TestControlHandlerEvidenceOperationsFailClosedWithoutSecurityService(t *testing.T) {
	handler := NewControlHandler(&controlServiceStub{})
	for _, test := range []struct {
		name   string
		invoke func(http.ResponseWriter, *http.Request)
		req    *http.Request
	}{
		{name: "upload", invoke: handler.AttachEvidence, req: multipartEvidenceRequest(t, map[string][]string{"title": {"Evidence"}}, "evidence.pdf", []byte("%PDF"))},
		{name: "download", invoke: handler.DownloadEvidence, req: controlEvidenceRequest(http.MethodGet, "/download", nil)},
		{name: "review", invoke: handler.ReviewEvidence, req: controlEvidenceRequest(http.MethodPost, "/review", strings.NewReader(`{"status":"accepted"}`))},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			test.invoke(response, test.req)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
