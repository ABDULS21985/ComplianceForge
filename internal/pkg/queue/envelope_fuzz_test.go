package queue

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func FuzzEnvelopeDecodeValidateRoundTrip(f *testing.F) {
	valid := Envelope{
		ID: "10000000-0000-0000-0000-000000000001", Type: "compliance.test",
		SchemaVersion: CurrentSchemaVersion, TenantID: "20000000-0000-0000-0000-000000000001",
		CorrelationID: "correlation-1", CreatedAt: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		Attempt: 1, Metadata: map[string]string{"source": "fuzz-seed"}, Payload: json.RawMessage(`{"ok":true}`),
	}
	body, err := encodeEnvelope(valid)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(body)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"id":"not-a-uuid","unknown":true}`))
	f.Add([]byte{0xff, 0xfe, 0x00})

	f.Fuzz(func(t *testing.T, candidate []byte) {
		if len(candidate) > 1<<20 {
			t.Skip()
		}
		envelope, err := decodeEnvelope(candidate)
		if err != nil || envelope.Validate() != nil {
			return
		}
		encoded, err := encodeEnvelope(envelope)
		if err != nil {
			t.Fatalf("valid envelope did not encode: %v", err)
		}
		roundTrip, err := decodeEnvelope(encoded)
		if err != nil {
			t.Fatalf("encoded envelope did not decode: %v", err)
		}
		if err := roundTrip.Validate(); err != nil {
			t.Fatalf("round-trip envelope became invalid: %v", err)
		}
		if roundTrip.ID != envelope.ID || roundTrip.Type != envelope.Type ||
			roundTrip.TenantID != envelope.TenantID || roundTrip.CorrelationID != envelope.CorrelationID ||
			roundTrip.Attempt != envelope.Attempt || !bytes.Equal(roundTrip.Payload, envelope.Payload) {
			t.Fatalf("round-trip changed security-relevant identity")
		}
	})
}
