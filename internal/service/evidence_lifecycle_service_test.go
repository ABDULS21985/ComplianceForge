package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
	storagepkg "github.com/complianceforge/platform/internal/pkg/storage"
)

type evidenceLifecycleStoreStub struct {
	record          *models.EvidenceLifecycleRecord
	historyErr      error
	integrityTarget *models.EvidenceIntegrityTarget
	integrityErr    error
	event           models.EvidenceCustodyEventInput
	eventErr        error
}

func (stub *evidenceLifecycleStoreStub) GetEvidenceLifecycle(context.Context, string, string, string) (*models.EvidenceLifecycleRecord, error) {
	return stub.record, stub.historyErr
}

func (stub *evidenceLifecycleStoreStub) GetEvidenceIntegrity(context.Context, string, string, string) (*models.EvidenceIntegrityTarget, error) {
	if stub.integrityErr != nil {
		return nil, stub.integrityErr
	}
	if stub.integrityTarget != nil {
		return stub.integrityTarget, nil
	}
	if stub.record == nil {
		return nil, pgx.ErrNoRows
	}
	return &models.EvidenceIntegrityTarget{
		EvidenceID: stub.record.Evidence.ID, OrganizationID: stub.record.Evidence.OrganizationID,
		ObjectKey: stub.record.Evidence.ObjectKey, SHA256: stub.record.Evidence.FileHash,
		SizeBytes: stub.record.Evidence.FileSizeBytes, Chain: stub.record.Chain,
	}, nil
}

func (stub *evidenceLifecycleStoreStub) AppendEvidenceCustodyEvent(_ context.Context, _, _, _ string, input models.EvidenceCustodyEventInput) (*models.EvidenceCustodyEvent, error) {
	stub.event = input
	if stub.eventErr != nil {
		return nil, stub.eventErr
	}
	return &models.EvidenceCustodyEvent{EventType: input.EventType}, nil
}

type evidenceIntegrityStoreStub struct {
	key, hash string
	size      int64
	err       error
}

func (stub *evidenceIntegrityStoreStub) Verify(_ context.Context, key, hash string, size int64) error {
	stub.key, stub.hash, stub.size = key, hash, size
	return stub.err
}

func evidenceLifecycleRecordForTest() *models.EvidenceLifecycleRecord {
	key, hash, series := "evidence/tenant/object", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "50000000-0000-0000-0000-000000000005"
	size := int64(42)
	return &models.EvidenceLifecycleRecord{Evidence: models.ControlEvidence{
		BaseModel: models.BaseModel{ID: evidenceServiceObjectID}, OrganizationID: evidenceServiceOrgID,
		ObjectKey: &key, FileHash: &hash, FileSizeBytes: &size, SeriesID: &series,
	}, Chain: models.EvidenceCustodyChainVerification{Valid: true, EventCount: 1, HeadSequence: 1}}
}

func newEvidenceLifecycleServiceForTest(t *testing.T, repository *evidenceLifecycleStoreStub, storage *evidenceIntegrityStoreStub) *EvidenceLifecycleService {
	t.Helper()
	result, err := NewEvidenceLifecycleService(repository, storage, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	result.now = func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) }
	return result
}

func TestEvidenceLifecycleHistoryNormalizesCollectionsAndTenantBoundary(t *testing.T) {
	record := evidenceLifecycleRecordForTest()
	repository := &evidenceLifecycleStoreStub{record: record}
	service := newEvidenceLifecycleServiceForTest(t, repository, &evidenceIntegrityStoreStub{})
	result, err := service.History(context.Background(), evidenceServiceOrgID, evidenceServiceControlID, evidenceServiceObjectID)
	if err != nil || result == nil || result.Versions == nil || result.Reviews == nil || result.CustodyEvents == nil {
		t.Fatalf("History()=%#v error=%v", result, err)
	}
	record.Evidence.OrganizationID = "90000000-0000-0000-0000-000000000009"
	if result, err = service.History(context.Background(), evidenceServiceOrgID, evidenceServiceControlID, evidenceServiceObjectID); result != nil || !errors.Is(err, ErrEvidenceLifecycleNotFound) {
		t.Fatalf("cross-tenant History()=%#v error=%v", result, err)
	}
	repository.historyErr = pgx.ErrNoRows
	if _, err = service.History(context.Background(), evidenceServiceOrgID, evidenceServiceControlID, evidenceServiceObjectID); !errors.Is(err, ErrEvidenceLifecycleNotFound) {
		t.Fatalf("missing History() error=%v", err)
	}
}

func TestEvidenceLifecycleIntegrityVerificationRecordsSuccessAndFailure(t *testing.T) {
	for _, test := range []struct {
		name      string
		verifyErr error
		wantEvent models.EvidenceCustodyEventType
		wantValid bool
		wantErr   error
	}{
		{name: "verified", wantEvent: models.EvidenceCustodyIntegrityVerified, wantValid: true},
		{name: "mismatch", verifyErr: storagepkg.ErrIntegrityMismatch, wantEvent: models.EvidenceCustodyIntegrityFailed, wantErr: ErrEvidenceIntegrityFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &evidenceLifecycleStoreStub{record: evidenceLifecycleRecordForTest()}
			storage := &evidenceIntegrityStoreStub{err: test.verifyErr}
			service := newEvidenceLifecycleServiceForTest(t, repository, storage)
			result, err := service.VerifyIntegrity(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID, "request-123")
			if !errors.Is(err, test.wantErr) || result == nil || result.Valid != test.wantValid || repository.event.EventType != test.wantEvent {
				t.Fatalf("VerifyIntegrity()=%#v error=%v event=%#v", result, err, repository.event)
			}
			if storage.key != "evidence/tenant/object" || storage.size != 42 || repository.event.ActorUserID == nil || *repository.event.ActorUserID != evidenceServiceUserID {
				t.Fatalf("storage=%#v event=%#v", storage, repository.event)
			}
			if !reflect.DeepEqual(repository.event.Details, map[string]any{"verification_method": "sha256_and_size"}) {
				t.Fatalf("event details=%#v", repository.event.Details)
			}
		})
	}
}

func TestEvidenceLifecycleIntegrityFailsClosedOnInvalidCustodyChain(t *testing.T) {
	record := evidenceLifecycleRecordForTest()
	record.Chain.Valid = false
	record.Chain.FirstInvalidSequence = func() *int64 { value := int64(2); return &value }()
	repository := &evidenceLifecycleStoreStub{record: record}
	storage := &evidenceIntegrityStoreStub{}
	service := newEvidenceLifecycleServiceForTest(t, repository, storage)

	result, err := service.VerifyIntegrity(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID, "request")
	if result != nil || !errors.Is(err, ErrEvidenceCustodyChainInvalid) {
		t.Fatalf("VerifyIntegrity()=%#v error=%v", result, err)
	}
	if storage.key != "" || repository.event.EventType != "" {
		t.Fatalf("invalid chain reached object verification or custody append: storage=%#v event=%#v", storage, repository.event)
	}
}

func TestEvidenceLifecycleIntegrityDoesNotRecordOperationalStorageFailureAsTampering(t *testing.T) {
	repository := &evidenceLifecycleStoreStub{record: evidenceLifecycleRecordForTest()}
	storageFailure := errors.New("object store timeout")
	service := newEvidenceLifecycleServiceForTest(t, repository, &evidenceIntegrityStoreStub{err: storageFailure})

	result, err := service.VerifyIntegrity(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID, "request")
	if result != nil || !errors.Is(err, storageFailure) || errors.Is(err, ErrEvidenceIntegrityFailed) {
		t.Fatalf("VerifyIntegrity()=%#v error=%v", result, err)
	}
	if repository.event.EventType != "" {
		t.Fatalf("operational failure recorded a custody verdict: %#v", repository.event)
	}
}

func TestEvidenceLifecycleIntegrityTargetRemainsTenantAndObjectScoped(t *testing.T) {
	for _, test := range []struct {
		name       string
		repository *evidenceLifecycleStoreStub
	}{
		{name: "missing", repository: &evidenceLifecycleStoreStub{integrityErr: pgx.ErrNoRows}},
		{name: "wrong tenant", repository: &evidenceLifecycleStoreStub{integrityTarget: &models.EvidenceIntegrityTarget{
			EvidenceID: evidenceServiceObjectID, OrganizationID: "90000000-0000-0000-0000-000000000009",
		}}},
		{name: "wrong evidence", repository: &evidenceLifecycleStoreStub{integrityTarget: &models.EvidenceIntegrityTarget{
			EvidenceID: "90000000-0000-0000-0000-000000000009", OrganizationID: evidenceServiceOrgID,
		}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := &evidenceIntegrityStoreStub{}
			service := newEvidenceLifecycleServiceForTest(t, test.repository, storage)
			result, err := service.VerifyIntegrity(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID, "request")
			if result != nil || !errors.Is(err, ErrEvidenceLifecycleNotFound) || storage.key != "" {
				t.Fatalf("VerifyIntegrity()=%#v error=%v storage=%#v", result, err, storage)
			}
		})
	}
}

func TestEvidenceLifecycleNeverReportsSuccessWithoutDurableCustodyEvent(t *testing.T) {
	repository := &evidenceLifecycleStoreStub{record: evidenceLifecycleRecordForTest(), eventErr: errors.New("database offline")}
	service := newEvidenceLifecycleServiceForTest(t, repository, &evidenceIntegrityStoreStub{})
	result, err := service.VerifyIntegrity(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID, "request")
	if result != nil || err == nil || errors.Is(err, ErrEvidenceIntegrityFailed) {
		t.Fatalf("VerifyIntegrity()=%#v error=%v", result, err)
	}
}

func TestEvidenceLifecycleRecordsBoundedDownloadAuthorization(t *testing.T) {
	repository := &evidenceLifecycleStoreStub{}
	service := newEvidenceLifecycleServiceForTest(t, repository, &evidenceIntegrityStoreStub{})
	if err := service.RecordDownloadAuthorization(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID, "request-42", "signed_url"); err != nil {
		t.Fatal(err)
	}
	if repository.event.EventType != models.EvidenceCustodyDownloadAuthorized || repository.event.Details["delivery_mode"] != "signed_url" || repository.event.RequestID == nil {
		t.Fatalf("event=%#v", repository.event)
	}
	for _, mode := range []string{"", "public", "PRIVATE_STREAM"} {
		if err := service.RecordDownloadAuthorization(context.Background(), evidenceServiceOrgID, evidenceServiceUserID, evidenceServiceControlID, evidenceServiceObjectID, "request", mode); !errors.Is(err, ErrInvalidComplianceID) {
			t.Fatalf("mode %q error=%v", mode, err)
		}
	}
}

func TestNewEvidenceLifecycleServiceRequiresDependencies(t *testing.T) {
	if _, err := NewEvidenceLifecycleService(nil, &evidenceIntegrityStoreStub{}, zerolog.Nop()); err == nil {
		t.Fatal("nil repository accepted")
	}
	if _, err := NewEvidenceLifecycleService(&evidenceLifecycleStoreStub{}, nil, zerolog.Nop()); err == nil {
		t.Fatal("nil storage accepted")
	}
	var typedRepository *evidenceLifecycleStoreStub
	if _, err := NewEvidenceLifecycleService(typedRepository, &evidenceIntegrityStoreStub{}, zerolog.Nop()); err == nil {
		t.Fatal("typed nil repository accepted")
	}
}
