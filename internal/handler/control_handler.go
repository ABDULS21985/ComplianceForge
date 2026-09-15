package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type ControlService interface {
	ListControls(context.Context, string, string, models.PaginationRequest) ([]models.Control, int, error)
	GetControl(context.Context, string, string) (*models.Control, error)
	UpdateControlImplementation(context.Context, string, string, models.ControlImplementationPatch) (*models.ControlImplementation, error)
	AttachControlEvidence(context.Context, string, string, string, models.AttachControlEvidenceInput) (*models.ControlEvidence, error)
	ListControlEvidence(context.Context, string, string, models.PaginationRequest) ([]models.ControlEvidence, int, error)
}

type EvidenceObjectService interface {
	Ready() bool
	Upload(context.Context, string, string, string, string, string, io.Reader, service.EvidenceUploadMetadata) (*models.ControlEvidence, error)
	Supersede(context.Context, string, string, string, string, string, string, io.Reader, service.EvidenceUploadMetadata) (*models.ControlEvidence, error)
	Download(context.Context, string, string, string) (*service.EvidenceDownload, error)
	Review(context.Context, string, string, string, string, models.ReviewControlEvidenceInput) (*models.ControlEvidence, error)
}

type EvidenceLifecycleService interface {
	Ready() bool
	History(context.Context, string, string, string) (*models.EvidenceLifecycleRecord, error)
	VerifyIntegrity(context.Context, string, string, string, string, string) (*models.EvidenceIntegrityResult, error)
	RecordDownloadAuthorization(context.Context, string, string, string, string, string, string) error
}

type ControlHandler struct {
	service                    ControlService
	evidenceObjects            EvidenceObjectService
	evidenceLifecycle          EvidenceLifecycleService
	maximumEvidenceUploadBytes int64
}

func WithEvidenceLifecycleService(lifecycle EvidenceLifecycleService) ControlHandlerOption {
	return func(handler *ControlHandler) {
		handler.evidenceLifecycle = lifecycle
	}
}

type ControlHandlerOption func(*ControlHandler)

func WithEvidenceObjectService(objects EvidenceObjectService, maximumUploadBytes int64) ControlHandlerOption {
	return func(handler *ControlHandler) {
		handler.evidenceObjects = objects
		if maximumUploadBytes > 0 {
			handler.maximumEvidenceUploadBytes = maximumUploadBytes
		}
	}
}

func NewControlHandler(service ControlService, options ...ControlHandlerOption) *ControlHandler {
	handler := &ControlHandler{service: service, maximumEvidenceUploadBytes: 25 << 20}
	for _, option := range options {
		option(handler)
	}
	return handler
}
func (h *ControlHandler) Ready() bool {
	return h != nil && h.service != nil && h.evidenceObjects != nil &&
		h.evidenceObjects.Ready() && h.evidenceLifecycle != nil &&
		h.evidenceLifecycle.Ready() && h.maximumEvidenceUploadBytes > 0
}

func (h *ControlHandler) List(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListControls(r.Context(), middleware.GetOrgIDFromContext(r.Context()), r.URL.Query().Get("framework_id"), p)
	if err != nil {
		writeComplianceError(w, err, "Failed to list controls")
		return
	}
	writeClassifiedPaginated(w, r, "controls", items, total, p)
}

func (h *ControlHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetControl(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeComplianceError(w, err, "Failed to get control")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "controls", item)
}

func (h *ControlHandler) UpdateImplementation(w http.ResponseWriter, r *http.Request) {
	var patch models.ControlImplementationPatch
	if err := decodeComplianceJSON(w, r, &patch); err != nil {
		return
	}
	item, err := h.service.UpdateControlImplementation(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeComplianceError(w, err, "Failed to update control implementation")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "controls", item)
}

func (h *ControlHandler) AttachEvidence(w http.ResponseWriter, r *http.Request) {
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr == nil && mediaType == "multipart/form-data" {
		h.uploadEvidenceObject(w, r)
		return
	}
	var input models.AttachControlEvidenceInput
	if err := decodeComplianceJSON(w, r, &input); err != nil {
		return
	}
	if input.ObjectKey != nil || input.FileName != nil || input.FileSizeBytes != nil || input.MIMEType != nil || input.FileHash != nil {
		writeError(w, http.StatusBadRequest, "Invalid evidence metadata", "File metadata is server managed; use a multipart evidence upload")
		return
	}
	item, err := h.service.AttachControlEvidence(r.Context(), middleware.GetOrgIDFromContext(r.Context()), middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "id"), input)
	if err != nil {
		writeComplianceError(w, err, "Failed to attach control evidence")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "controls", item)
}

func (h *ControlHandler) ListEvidence(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	items, total, err := h.service.ListControlEvidence(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"), p)
	if err != nil {
		writeComplianceError(w, err, "Failed to list control evidence")
		return
	}
	writeClassifiedPaginated(w, r, "controls", items, total, p)
}

func (h *ControlHandler) DownloadEvidence(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if h.evidenceObjects == nil || !h.evidenceObjects.Ready() {
		writeError(w, http.StatusServiceUnavailable, "Evidence downloads are unavailable", "The evidence object service is not ready")
		return
	}
	if h.evidenceLifecycle == nil || !h.evidenceLifecycle.Ready() {
		writeError(w, http.StatusServiceUnavailable, "Evidence custody is unavailable", "The evidence lifecycle service is not ready")
		return
	}
	if evidenceDownloadRequiresWatermark(r.Context()) {
		// An authorization allow is conditional on every returned obligation.
		// Until a content-type-specific watermark renderer can prove it modified
		// the object, denying the download is the only safe behavior.
		writeError(w, http.StatusServiceUnavailable, "Secure evidence export is unavailable", "The required watermark could not be applied")
		return
	}
	download, err := h.evidenceObjects.Download(r.Context(), middleware.GetOrgIDFromContext(r.Context()), chi.URLParam(r, "id"), chi.URLParam(r, "evidenceID"))
	if err != nil {
		writeComplianceError(w, err, "Failed to download evidence")
		return
	}
	if download == nil || (download.SignedURL == "" && download.Body == nil) ||
		(download.SignedURL != "" && (download.Body != nil || !validEvidenceSignedRedirect(download.SignedURL))) {
		if download != nil && download.Body != nil {
			_ = download.Body.Close()
		}
		writeError(w, http.StatusServiceUnavailable, "Evidence download is unavailable", "The evidence delivery response is invalid")
		return
	}
	deliveryMode := "private_stream"
	if download.SignedURL != "" {
		deliveryMode = "signed_url"
	}
	if err := h.evidenceLifecycle.RecordDownloadAuthorization(
		r.Context(), middleware.GetOrgIDFromContext(r.Context()),
		middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "id"),
		chi.URLParam(r, "evidenceID"), middleware.GetRequestIDFromContext(r.Context()), deliveryMode,
	); err != nil {
		if download.Body != nil {
			_ = download.Body.Close()
		}
		writeComplianceError(w, err, "Failed to record evidence custody")
		return
	}
	if download.SignedURL != "" {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// The destination is emitted by the deployment-configured object-store
		// presigner after tenant/object authorization and integrity checks, never
		// from a browser redirect parameter. The guard above rejects malformed,
		// relative, credential-bearing, non-HTTPS and control-character URLs.
		http.Redirect(w, r, download.SignedURL, http.StatusTemporaryRedirect) // #nosec G710 -- Reviewed server-side presigner destination, not a client-selected redirect; bounded absolute HTTPS is enforced above.
		return
	}
	if download.Body == nil {
		writeError(w, http.StatusServiceUnavailable, "Evidence download is unavailable", "The evidence content could not be opened")
		return
	}
	writeAttachmentStream(w, download.Filename, download.ContentType, download.SizeBytes, download.Body)
}

func validEvidenceSignedRedirect(raw string) bool {
	if len(raw) == 0 || len(raw) > 16<<10 || strings.Contains(raw, "\\") {
		return false
	}
	for _, character := range raw {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !parsed.IsAbs() || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return false
	}
	for _, character := range parsed.Path {
		if unicode.IsControl(character) {
			return false
		}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	for key, values := range query {
		for _, value := range append([]string{key}, values...) {
			for _, character := range value {
				if unicode.IsControl(character) {
					return false
				}
			}
		}
	}
	return true
}

func evidenceDownloadRequiresWatermark(ctx context.Context) bool {
	decision, ok := middleware.GetAuthorizationDecision(ctx)
	if !ok {
		return false
	}
	for _, obligation := range decision.Obligations {
		if obligation.Kind == "watermark" {
			return true
		}
	}
	return false
}

func (h *ControlHandler) ReviewEvidence(w http.ResponseWriter, r *http.Request) {
	if h.evidenceObjects == nil || !h.evidenceObjects.Ready() {
		writeError(w, http.StatusServiceUnavailable, "Evidence review is unavailable", "The evidence object service is not ready")
		return
	}
	var input models.ReviewControlEvidenceInput
	if err := decodeComplianceJSON(w, r, &input); err != nil {
		return
	}
	input.RequestID = middleware.GetRequestIDFromContext(r.Context())
	item, err := h.evidenceObjects.Review(r.Context(), middleware.GetOrgIDFromContext(r.Context()), middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "id"), chi.URLParam(r, "evidenceID"), input)
	if err != nil {
		writeComplianceError(w, err, "Failed to review evidence")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "controls", item)
}

func (h *ControlHandler) EvidenceHistory(w http.ResponseWriter, r *http.Request) {
	if h.evidenceLifecycle == nil || !h.evidenceLifecycle.Ready() {
		writeError(w, http.StatusServiceUnavailable, "Evidence history is unavailable", "The evidence lifecycle service is not ready")
		return
	}
	item, err := h.evidenceLifecycle.History(
		r.Context(), middleware.GetOrgIDFromContext(r.Context()),
		chi.URLParam(r, "id"), chi.URLParam(r, "evidenceID"),
	)
	if err != nil {
		writeComplianceError(w, err, "Failed to load evidence history")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "controls", item)
}

func (h *ControlHandler) VerifyEvidenceIntegrity(w http.ResponseWriter, r *http.Request) {
	if h.evidenceLifecycle == nil || !h.evidenceLifecycle.Ready() {
		writeError(w, http.StatusServiceUnavailable, "Evidence verification is unavailable", "The evidence lifecycle service is not ready")
		return
	}
	item, err := h.evidenceLifecycle.VerifyIntegrity(
		r.Context(), middleware.GetOrgIDFromContext(r.Context()),
		middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "id"),
		chi.URLParam(r, "evidenceID"), middleware.GetRequestIDFromContext(r.Context()),
	)
	if errors.Is(err, service.ErrEvidenceCustodyChainInvalid) {
		writeError(w, http.StatusConflict, "Evidence custody verification failed", "The immutable custody history is inconsistent; no object-integrity verdict was recorded")
		return
	}
	if err != nil && !errors.Is(err, service.ErrEvidenceIntegrityFailed) {
		writeComplianceError(w, err, "Failed to verify evidence integrity")
		return
	}
	if errors.Is(err, service.ErrEvidenceIntegrityFailed) {
		writeError(w, http.StatusConflict, "Evidence integrity verification failed", "The stored object no longer matches its immutable checksum and size")
		return
	}
	writeClassifiedJSON(w, r, http.StatusOK, "controls", item)
}

func (h *ControlHandler) SupersedeEvidence(w http.ResponseWriter, r *http.Request) {
	h.writeEvidenceObject(w, r, chi.URLParam(r, "evidenceID"))
}

func (h *ControlHandler) uploadEvidenceObject(w http.ResponseWriter, r *http.Request) {
	h.writeEvidenceObject(w, r, "")
}

func (h *ControlHandler) writeEvidenceObject(w http.ResponseWriter, r *http.Request, supersedesEvidenceID string) {
	if h.evidenceObjects == nil || !h.evidenceObjects.Ready() {
		writeError(w, http.StatusServiceUnavailable, "Evidence uploads are unavailable", "The evidence malware-scanning service is not ready")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maximumEvidenceUploadBytes+(1<<20))
	// #nosec G120 -- MaxBytesReader caps the complete request, while the object
	// pipeline independently enforces the stricter file-byte limit.
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maximumError *http.MaxBytesError
		if errors.As(err, &maximumError) {
			writeError(w, http.StatusRequestEntityTooLarge, "Evidence upload is too large", "Reduce the file size and try again")
			return
		}
		writeError(w, http.StatusBadRequest, "Invalid evidence upload", "The multipart request could not be parsed")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	if unknown := unknownEvidenceMultipartFields(r); unknown != "" {
		writeError(w, http.StatusBadRequest, "Invalid evidence upload", fmt.Sprintf("Unknown multipart field %q", unknown))
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) != 1 {
		writeError(w, http.StatusBadRequest, "Invalid evidence upload", "Exactly one file is required")
		return
	}
	file, err := files[0].Open()
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid evidence upload", "The evidence file could not be opened")
		return
	}
	defer file.Close()

	metadata, err := parseEvidenceUploadMetadata(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid evidence upload", err.Error())
		return
	}
	contentType := files[0].Header.Get("Content-Type")
	var item *models.ControlEvidence
	if supersedesEvidenceID == "" {
		item, err = h.evidenceObjects.Upload(r.Context(), middleware.GetOrgIDFromContext(r.Context()), middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "id"), files[0].Filename, contentType, file, metadata)
	} else {
		item, err = h.evidenceObjects.Supersede(r.Context(), middleware.GetOrgIDFromContext(r.Context()), middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "id"), supersedesEvidenceID, files[0].Filename, contentType, file, metadata)
	}
	if err != nil {
		writeComplianceError(w, err, "Failed to upload evidence")
		return
	}
	writeClassifiedJSON(w, r, http.StatusCreated, "controls", item)
}

func unknownEvidenceMultipartFields(r *http.Request) string {
	permittedValues := map[string]bool{"title": true, "description": true, "evidence_type": true, "valid_from": true, "valid_until": true, "metadata": true, "version_reason": true}
	for field, values := range r.MultipartForm.Value {
		if !permittedValues[field] || len(values) > 1 {
			return field
		}
	}
	for field := range r.MultipartForm.File {
		if field != "file" {
			return field
		}
	}
	return ""
}

func parseEvidenceUploadMetadata(r *http.Request) (service.EvidenceUploadMetadata, error) {
	metadata := service.EvidenceUploadMetadata{
		Title: strings.TrimSpace(r.FormValue("title")), EvidenceType: strings.TrimSpace(r.FormValue("evidence_type")),
	}
	if description := strings.TrimSpace(r.FormValue("description")); description != "" {
		metadata.Description = &description
	}
	for field, target := range map[string]**time.Time{"valid_from": &metadata.ValidFrom, "valid_until": &metadata.ValidUntil} {
		value := strings.TrimSpace(r.FormValue(field))
		if value == "" {
			continue
		}
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return service.EvidenceUploadMetadata{}, fmt.Errorf("%s must use YYYY-MM-DD", field)
		}
		*target = &parsed
	}
	if value := strings.TrimSpace(r.FormValue("metadata")); value != "" {
		metadata.Metadata = json.RawMessage(value)
	}
	metadata.VersionReason = strings.TrimSpace(r.FormValue("version_reason"))
	return metadata, nil
}

func decodeComplianceJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return err
	}
	return nil
}
