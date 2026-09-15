package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/models"
)

func TestSupportBundleRepositoryRequiresDatabase(t *testing.T) {
	if _, err := NewSupportBundleRepository(nil); err == nil {
		t.Fatal("support consent audit accepted a nil database")
	}
}

func TestSupportBundleAuditValidationRejectsInvalidEvidenceBeforeSQL(t *testing.T) {
	for _, test := range []struct {
		name   string
		modify func(*models.SupportBundleAudit)
	}{
		{"missing bundle", func(e *models.SupportBundleAudit) { e.BundleID = "" }},
		{"uppercase actor", func(e *models.SupportBundleAudit) { e.ActorID = strings.ToUpper(e.ActorID) }},
		{"URN tenant", func(e *models.SupportBundleAudit) { e.OrganizationID = "urn:uuid:" + e.OrganizationID }},
		{"nil actor", func(e *models.SupportBundleAudit) { e.ActorID = "00000000-0000-0000-0000-000000000000" }},
		{"request detail", func(e *models.SupportBundleAudit) { e.RequestID = "internal-error-secret" }},
		{"wrong scope", func(e *models.SupportBundleAudit) { e.Scope = "raw_logs" }},
		{"wrong profile", func(e *models.SupportBundleAudit) { e.RedactionProfile = "unreviewed_profile" }},
		{"missing time", func(e *models.SupportBundleAudit) { e.GeneratedAt = time.Time{} }},
		{"raw fingerprint", func(e *models.SupportBundleAudit) { e.ConfigurationFingerprint = "raw-secret" }},
		{"uppercase hash", func(e *models.SupportBundleAudit) { e.ArchiveSHA256 = strings.Repeat("B", 64) }},
		{"large artifact", func(e *models.SupportBundleAudit) { e.ArchiveBytes = models.MaximumSupportBundleBytes + 1 }},
		{"empty artifact", func(e *models.SupportBundleAudit) { e.ArchiveBytes = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			evidence := models.SupportBundleAudit{
				BundleID: "10000000-aaaa-4000-8000-000000000001", OrganizationID: "20000000-aaaa-4000-8000-000000000001",
				ActorID: "30000000-aaaa-4000-8000-000000000001", GeneratedAt: time.Now().UTC(),
				Scope: models.SupportBundleScope, RedactionProfile: models.SupportBundleRedactionProfile,
				ConfigurationFingerprint: strings.Repeat("a", 64), ArchiveSHA256: strings.Repeat("b", 64), ArchiveBytes: 4096,
			}
			test.modify(&evidence)
			// No pool: accidentally reaching SQL would panic instead of giving a
			// false green validation result.
			err := (&supportBundleRepo{}).RecordSupportBundleGeneration(context.Background(), evidence)
			if !errors.Is(err, ErrSupportBundleAuditDenied) {
				t.Fatalf("invalid audit evidence error=%v", err)
			}
		})
	}
}
