package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

const maximumSupportBundleRequestBytes = 1024

var supportNumericVersionPattern = regexp.MustCompile(`^[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}$`)

type SupportBundleService interface {
	Generate(context.Context, string, string, string, models.SupportBundleRequest) (*models.SupportBundleArtifact, error)
}

func WithSupportBundleService(service SupportBundleService) DiagnosticsHandlerOption {
	return func(handler *DiagnosticsHandler) { handler.supportBundles = service }
}

func (h *DiagnosticsHandler) SupportBundlesReady() bool {
	return h != nil && supportBundleProviderReady(h.supportBundles)
}

// GenerateSupportBundle handles POST /api/v1/settings/diagnostics/support-bundle.
// settings:configure and ABAC are enforced by the protected router. Consent is
// explicit per generation; a GET cannot trigger generation or record consent.
func (h *DiagnosticsHandler) GenerateSupportBundle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	organizationID := middleware.GetOrgIDFromContext(r.Context())
	actorID := middleware.GetUserIDFromContext(r.Context())
	if organizationID == "" || actorID == "" {
		writeError(w, http.StatusUnauthorized, "Missing authentication context", "")
		return
	}
	permissions, err := classifiedResponsePermissions(r, "settings")
	if err == nil {
		for _, permission := range permissions {
			if permission.Visibility != models.AccessFieldVisible {
				err = errClassifiedAttachmentMaskingUnsupported
				break
			}
		}
	}
	if err == nil {
		decision, _ := middleware.GetAuthorizationDecision(r.Context())
		for _, obligation := range decision.Obligations {
			if obligation.Kind != service.AccessFieldVisibilityObligation {
				err = errClassifiedAttachmentMaskingUnsupported
				break
			}
		}
	}
	if err != nil {
		writeClassifiedFailure(w, r, "settings", err)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusBadRequest, "Content-Type must be application/json", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximumSupportBundleRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req models.SupportBundleRequest
	if err := decoder.Decode(&req); err != nil {
		writeSupportBundleBodyError(w, err)
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected trailing JSON value")
		}
		writeSupportBundleBodyError(w, err)
		return
	}
	if !req.Consent || req.Scope != models.SupportBundleScope {
		writeError(w, http.StatusBadRequest, "Explicit consent to health_and_posture is required", "")
		return
	}
	if !h.SupportBundlesReady() {
		writeError(w, http.StatusServiceUnavailable, "Support bundles are temporarily unavailable", "")
		return
	}
	artifact, err := h.supportBundles.Generate(r.Context(), organizationID, actorID,
		middleware.GetRequestIDFromContext(r.Context()), req)
	if errors.Is(err, service.ErrInvalidSupportBundleRequest) {
		writeError(w, http.StatusBadRequest, "Invalid support bundle consent request", "")
		return
	}
	if err != nil || !validSupportBundleArtifact(artifact, organizationID) {
		log.Error().Str("organization_id", organizationID).
			Str("request_id", middleware.GetRequestIDFromContext(r.Context())).
			Msg("consented support bundle generation failed")
		writeError(w, http.StatusServiceUnavailable, "Support bundles are temporarily unavailable", "")
		return
	}
	w.Header().Set("X-Support-Bundle-SHA256", artifact.SHA256)
	writeClassifiedAttachment(w, r, "settings", "complianceforge-support-"+artifact.ID+".zip", "application/octet-stream", artifact.Bytes)
}

func writeSupportBundleBodyError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "Support bundle request exceeds 1024 bytes", "")
		return
	}
	writeError(w, http.StatusBadRequest, "Invalid support bundle JSON request", "")
}

func supportBundleProviderReady(provider SupportBundleService) bool {
	if provider == nil {
		return false
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func validSupportBundleArtifact(artifact *models.SupportBundleArtifact, organizationID string) bool {
	if artifact == nil || len(artifact.Bytes) < 4 || len(artifact.Bytes) > service.MaximumSupportBundleBytes {
		return false
	}
	id, err := uuid.Parse(artifact.ID)
	if err != nil || id == uuid.Nil || id.String() != artifact.ID || string(artifact.Bytes[:4]) != "PK\x03\x04" {
		return false
	}
	sum := sha256.Sum256(artifact.Bytes)
	if artifact.SHA256 != hex.EncodeToString(sum[:]) {
		return false
	}
	reader, err := zip.NewReader(bytes.NewReader(artifact.Bytes), int64(len(artifact.Bytes)))
	if err != nil || len(reader.File) != 4 {
		return false
	}
	members := make(map[string][]byte, 4)
	var total uint64
	for _, file := range reader.File {
		switch file.Name {
		case "configuration.json", "health.json", "README.txt", "manifest.json":
		default:
			return false
		}
		if _, duplicate := members[file.Name]; duplicate || file.Method != zip.Store || file.UncompressedSize64 > service.MaximumSupportBundleBytes {
			return false
		}
		total += file.UncompressedSize64
		if total > service.MaximumSupportBundleBytes {
			return false
		}
		entry, err := file.Open()
		if err != nil {
			return false
		}
		data, err := io.ReadAll(io.LimitReader(entry, service.MaximumSupportBundleBytes+1))
		closeErr := entry.Close()
		if err != nil || closeErr != nil || len(data) > service.MaximumSupportBundleBytes {
			return false
		}
		members[file.Name] = data
	}
	if string(members["README.txt"]) != models.SupportBundleReadme || !canonicalSupportArchive(members, artifact.Bytes) {
		return false
	}
	var manifest models.SupportBundleManifest
	if !decodeSupportMember(members["manifest.json"], &manifest) || manifest.SchemaVersion != 1 ||
		manifest.BundleID != artifact.ID || manifest.OrganizationID != organizationID ||
		manifest.Scope != models.SupportBundleScope || manifest.RedactionProfile != models.SupportBundleRedactionProfile ||
		manifest.GeneratedAt.IsZero() || !manifest.ConsentRecordedAt.Equal(manifest.GeneratedAt) ||
		manifest.AutomaticallyTransmitted || len(manifest.Files) != 3 ||
		!slices.Equal(manifest.Excluded, models.SupportBundleExclusions()) {
		return false
	}
	seen := make(map[string]bool, 3)
	for _, file := range manifest.Files {
		data, exists := members[file.Name]
		if !exists || file.Name == "manifest.json" || seen[file.Name] || file.Bytes != len(data) {
			return false
		}
		seen[file.Name] = true
		sum := sha256.Sum256(data)
		if file.SHA256 != hex.EncodeToString(sum[:]) {
			return false
		}
	}
	var health models.SupportBundleHealth
	if !decodeSupportMember(members["health.json"], &health) || health.OrganizationID != organizationID ||
		!validSupportStatus(health.OverallStatus) || !validSupportStatus(health.Migration.Status) ||
		!validSupportStatus(health.Queue.Status) || !validSupportStatus(health.Notifications.Status) || !validSupportStatus(health.Connectors.Status) {
		return false
	}
	if !nonnegativeSupportValues(health.Migration.CurrentVersion, health.Migration.SupportedVersion, health.Migration.PendingCount,
		health.Queue.Pending, health.Queue.Leased, health.Queue.Dead, health.Queue.ExpiredLeases, health.Queue.OldestReadyAgeSeconds,
		health.Queue.InboxProcessing, health.Queue.InboxExpiredLeases, health.Notifications.Due, health.Notifications.ExpiredLeases,
		health.Notifications.TerminalFailures, health.Notifications.OldestDueAgeSeconds, health.Connectors.Total,
		health.Connectors.Healthy, health.Connectors.Degraded, health.Connectors.Unhealthy, health.Connectors.Unknown, health.Connectors.RecentFailedSyncs) {
		return false
	}
	dependencyKeys := make(map[string]bool, len(health.Dependencies))
	for _, dependency := range health.Dependencies {
		switch dependency.Key {
		case "postgres", "redis", "rabbitmq", "evidence_scanner", "object_storage", "ai_provider":
		default:
			return false
		}
		if dependencyKeys[dependency.Key] || !validSupportStatus(dependency.Status) || dependency.LatencyMS < 0 || dependency.LatencyMS > 60_000 {
			return false
		}
		dependencyKeys[dependency.Key] = true
	}
	var configuration models.SupportBundleConfiguration
	if !decodeSupportMember(members["configuration.json"], &configuration) || configuration.SchemaVersion != 1 ||
		(configuration.ServiceVersion != "unknown" && !supportNumericVersionPattern.MatchString(configuration.ServiceVersion)) {
		return false
	}
	switch configuration.Environment {
	case "production", "staging", "development", "test", "unknown":
	default:
		return false
	}
	for _, budget := range []struct{ value, maximum int64 }{
		{configuration.DatabaseMaximumConns, 100_000}, {configuration.DatabaseMinimumConns, 100_000},
		{configuration.MaximumEvidenceBytes, 1 << 40}, {configuration.ScannerTimeoutSeconds, 300},
		{configuration.RequestReadSeconds, 3600}, {configuration.RequestWriteSeconds, 3600}, {configuration.ShutdownTimeoutSeconds, 300},
	} {
		if budget.value < 0 || budget.value > budget.maximum {
			return false
		}
	}
	if len(configuration.Posture) != 10 {
		return false
	}
	postureKeys := make(map[string]bool, len(configuration.Posture))
	for _, posture := range configuration.Posture {
		switch posture.Key {
		case "cors_origins", "database_transport", "durable_object_storage", "encryption_key_separation", "evidence_security",
			"identity_origins", "queue_transport", "redis_transport", "smtp_transport", "telemetry":
		default:
			return false
		}
		if postureKeys[posture.Key] || !validSupportStatus(posture.Status) {
			return false
		}
		postureKeys[posture.Key] = true
	}
	configHash := sha256.Sum256(members["configuration.json"])
	return manifest.ConfigurationFingerprint == hex.EncodeToString(configHash[:])
}

func decodeSupportMember(data []byte, output any) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(output) != nil || !errors.Is(decoder.Decode(new(any)), io.EOF) {
		return false
	}
	// A strict, canonical encoding also rejects ignored duplicate JSON keys,
	// alternate escaped metadata and trailing data that ordinary decoding drops.
	canonical, err := json.MarshalIndent(output, "", "  ")
	return err == nil && bytes.Equal(data, append(canonical, '\n'))
}

func nonnegativeSupportValues(values ...int64) bool {
	for _, value := range values {
		if value < 0 {
			return false
		}
	}
	return true
}

// Rebuilding the bounded, fixed-order ZIP rejects unreviewed archive comments,
// extra fields, path metadata, timestamps and bytes after the end-of-directory.
// It is deliberately a v1-format check, not a general-purpose ZIP validator.
func canonicalSupportArchive(members map[string][]byte, archive []byte) bool {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"configuration.json", "health.json", "README.txt", "manifest.json"} {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store,
			Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return false
		}
		if _, err := entry.Write(members[name]); err != nil {
			return false
		}
	}
	return writer.Close() == nil && bytes.Equal(buffer.Bytes(), archive)
}

func validSupportStatus(status models.DiagnosticStatus) bool {
	switch status {
	case models.DiagnosticStatusHealthy, models.DiagnosticStatusWarning, models.DiagnosticStatusCritical, models.DiagnosticStatusUnknown:
		return true
	default:
		return false
	}
}
