package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/pkg/queue"
)

type dsrOutboxStub struct{}

func (dsrOutboxStub) Enqueue(context.Context, database.Querier, string, queue.Envelope) error {
	return nil
}

func TestNewDSRServiceWithOutboxPreservesExplicitKeyContract(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	service, err := NewDSRServiceWithOutbox(new(pgxpool.Pool), NewEventBus(), key, dsrOutboxStub{}, "complianceforge.worker")
	if err != nil {
		t.Fatal(err)
	}
	if service.outbox == nil || service.outboxQueue != "complianceforge.worker" || !bytes.Equal(service.encKey, bytes.Repeat([]byte{0x42}, 32)) {
		t.Fatalf("outbox-enabled DSR service is misconfigured: %+v", service)
	}
	if _, err := NewDSRServiceWithOutbox(new(pgxpool.Pool), NewEventBus(), key, nil, "complianceforge.worker"); err == nil {
		t.Fatal("expected missing outbox error")
	}
	if _, err := NewDSRServiceWithOutbox(new(pgxpool.Pool), NewEventBus(), key, dsrOutboxStub{}, " "); err == nil {
		t.Fatal("expected missing queue name error")
	}
}
