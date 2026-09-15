package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type evidenceSchedulerRepositoryStub struct {
	tenants       []string
	notices       []models.ExpiredEvidenceNotice
	discoveryErr  error
	expirationErr error
	expiredTenant string
}

func (stub *evidenceSchedulerRepositoryStub) DueEvidenceTenants(context.Context, int) ([]string, error) {
	return stub.tenants, stub.discoveryErr
}

func (stub *evidenceSchedulerRepositoryStub) ExpireDueEvidence(_ context.Context, tenantID string, _ int) ([]models.ExpiredEvidenceNotice, error) {
	stub.expiredTenant = tenantID
	return stub.notices, stub.expirationErr
}

func TestExpireStaleEvidencePublishesOnlyWhenNoDurableNotificationExists(t *testing.T) {
	bus := service.NewEventBus()
	defer bus.Close()
	events := bus.Subscribe("evidence.expired")
	ownerID, controlCode := "owner-1", "CTRL-1"
	stub := &evidenceSchedulerRepositoryStub{notices: []models.ExpiredEvidenceNotice{
		{
			EvidenceID: "durable", OrganizationID: "tenant-a", ControlImplementationID: "implementation-1",
			Title: "Already queued", ExpiresAt: time.Now().UTC(), NotificationQueued: true,
		},
		{
			EvidenceID: "fallback", OrganizationID: "tenant-a", ControlImplementationID: "implementation-2",
			Title: "Publish fallback", ExpiresAt: time.Now().UTC(), CollectedBy: &ownerID, ControlCode: &controlCode,
		},
	}}
	scheduler := &EvidenceScheduler{bus: bus, lifecycle: stub}
	if err := scheduler.expireStaleEvidenceForTenant(context.Background(), "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if stub.expiredTenant != "tenant-a" {
		t.Fatalf("expired tenant=%q", stub.expiredTenant)
	}
	select {
	case event := <-events:
		if event.EntityID != "fallback" || event.OrgID != "tenant-a" || event.Data["owner_id"] != ownerID || event.Data["control_code"] != controlCode {
			t.Fatalf("fallback event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("fallback expiry event was not published")
	}
	select {
	case duplicate := <-events:
		t.Fatalf("durably queued expiry was published twice: %+v", duplicate)
	default:
	}
}

func TestExpireStaleEvidencePropagatesRepositoryFailure(t *testing.T) {
	want := errors.New("expiry unavailable")
	stub := &evidenceSchedulerRepositoryStub{expirationErr: want}
	scheduler := &EvidenceScheduler{bus: service.NewEventBus(), lifecycle: stub}
	defer scheduler.bus.Close()
	if err := scheduler.expireStaleEvidenceForTenant(context.Background(), "tenant-a"); !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
}

func TestEvidenceSchedulerRejectsIncompleteConfiguration(t *testing.T) {
	scheduler := &EvidenceScheduler{}
	if err := scheduler.runTenantChecks(context.Background(), nil); err == nil || err.Error() != "evidence scheduler is not configured" {
		t.Fatalf("error=%v", err)
	}
}
