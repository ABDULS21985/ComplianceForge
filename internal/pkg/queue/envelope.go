package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	CurrentSchemaVersion       = 1
	maxEnvelopeMetadataEntries = 64
	maxAMQPSignedInteger       = int64(1<<31 - 1)
)

// Envelope is the versioned wire contract shared by all producers and workers.
// ID is the idempotency key. TenantID is required for tenant-scoped jobs.
type Envelope struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	SchemaVersion int               `json:"schema_version"`
	TenantID      string            `json:"tenant_id,omitempty"`
	CorrelationID string            `json:"correlation_id"`
	CausationID   string            `json:"causation_id,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	Attempt       int               `json:"attempt"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Payload       json.RawMessage   `json:"payload"`
}

// NewEnvelope marshals payload and creates stable message/correlation IDs.
func NewEnvelope(messageType, tenantID string, payload any) (Envelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal queue payload: %w", err)
	}
	id := uuid.NewString()
	return Envelope{
		ID:            id,
		Type:          messageType,
		SchemaVersion: CurrentSchemaVersion,
		TenantID:      tenantID,
		CorrelationID: id,
		CreatedAt:     time.Now().UTC(),
		Attempt:       1,
		Payload:       body,
	}, nil
}

func (e Envelope) normalized() Envelope {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CorrelationID == "" {
		e.CorrelationID = e.ID
	}
	if e.SchemaVersion == 0 {
		e.SchemaVersion = CurrentSchemaVersion
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	} else {
		e.CreatedAt = e.CreatedAt.UTC()
	}
	if e.Attempt < 1 {
		e.Attempt = 1
	}
	if len(e.Payload) == 0 {
		e.Payload = json.RawMessage(`{}`)
	}
	return e
}

func (e Envelope) Validate() error {
	if _, err := uuid.Parse(e.ID); err != nil {
		return fmt.Errorf("message ID must be a UUID: %w", err)
	}
	if err := validateBrokerName(e.Type, 255); err != nil {
		return fmt.Errorf("invalid message type: %w", err)
	}
	if e.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported schema version %d", e.SchemaVersion)
	}
	if e.TenantID != "" {
		if _, err := uuid.Parse(e.TenantID); err != nil {
			return fmt.Errorf("tenant ID must be a UUID: %w", err)
		}
	}
	if strings.TrimSpace(e.CorrelationID) == "" || len(e.CorrelationID) > 255 || !utf8.ValidString(e.CorrelationID) {
		return fmt.Errorf("correlation ID is required and must not exceed 255 bytes")
	}
	if len(e.CausationID) > 255 || !utf8.ValidString(e.CausationID) {
		return fmt.Errorf("causation ID must not exceed 255 bytes")
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if e.Attempt < 1 || int64(e.Attempt) > maxAMQPSignedInteger {
		return fmt.Errorf("attempt must be between 1 and %d", maxAMQPSignedInteger)
	}
	if len(e.Metadata) > maxEnvelopeMetadataEntries {
		return fmt.Errorf("metadata must not contain more than %d entries", maxEnvelopeMetadataEntries)
	}
	for key, value := range e.Metadata {
		if strings.TrimSpace(key) == "" || len(key) > 64 || !utf8.ValidString(key) {
			return fmt.Errorf("metadata keys are required and must not exceed 64 bytes")
		}
		if len(value) > 1024 || !utf8.ValidString(value) {
			return fmt.Errorf("metadata value for %q must not exceed 1024 bytes", key)
		}
	}
	if !json.Valid(e.Payload) {
		return fmt.Errorf("payload must be valid JSON")
	}
	return nil
}

func encodeEnvelope(envelope Envelope) ([]byte, error) {
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal queue envelope: %w", err)
	}
	return body, nil
}

func decodeEnvelope(body []byte) (Envelope, error) {
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, fmt.Errorf("decode queue envelope: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Envelope{}, fmt.Errorf("decode queue envelope: multiple JSON values")
		}
		return Envelope{}, fmt.Errorf("decode queue envelope: %w", err)
	}
	return envelope, nil
}

func publishingForEnvelope(envelope Envelope, body []byte) amqp.Publishing {
	headers := amqp.Table{
		"x-schema-version": int32(envelope.SchemaVersion),
		"x-attempt":        int32(envelope.Attempt),
	}
	if envelope.TenantID != "" {
		headers["x-tenant-id"] = envelope.TenantID
	}
	if envelope.CausationID != "" {
		headers["x-causation-id"] = envelope.CausationID
	}
	return amqp.Publishing{
		Headers:         headers,
		ContentType:     "application/json",
		ContentEncoding: "utf-8",
		DeliveryMode:    amqp.Persistent,
		Priority:        0,
		CorrelationId:   envelope.CorrelationID,
		MessageId:       envelope.ID,
		Timestamp:       envelope.CreatedAt,
		Type:            envelope.Type,
		AppId:           "complianceforge",
		Body:            body,
	}
}

func applyDeliveryMetadata(envelope *Envelope, delivery amqp.Delivery) error {
	if delivery.MessageId == "" {
		return fmt.Errorf("AMQP message ID is required")
	}
	if delivery.MessageId != envelope.ID {
		return fmt.Errorf("AMQP message ID does not match envelope ID")
	}
	if delivery.CorrelationId == "" {
		return fmt.Errorf("AMQP correlation ID is required")
	}
	if delivery.CorrelationId != envelope.CorrelationID {
		return fmt.Errorf("AMQP correlation ID does not match envelope")
	}
	if delivery.Type == "" || delivery.Type != envelope.Type {
		return fmt.Errorf("AMQP message type does not match envelope")
	}
	headerSchema, ok := integerHeader(delivery.Headers["x-schema-version"])
	if !ok || headerSchema != envelope.SchemaVersion {
		return fmt.Errorf("AMQP schema version does not match envelope")
	}
	headerAttempt, ok := integerHeader(delivery.Headers["x-attempt"])
	if !ok || headerAttempt != envelope.Attempt {
		return fmt.Errorf("AMQP attempt does not match envelope")
	}
	headerTenant, tenantHeaderPresent := delivery.Headers["x-tenant-id"]
	if envelope.TenantID != "" {
		tenant, ok := headerTenant.(string)
		if !tenantHeaderPresent || !ok || tenant != envelope.TenantID {
			return fmt.Errorf("AMQP tenant ID does not match envelope")
		}
	} else if tenantHeaderPresent {
		return fmt.Errorf("AMQP tenant ID is present for a non-tenant envelope")
	}
	headerCausation, causationHeaderPresent := delivery.Headers["x-causation-id"]
	if envelope.CausationID != "" {
		causation, ok := headerCausation.(string)
		if !causationHeaderPresent || !ok || causation != envelope.CausationID {
			return fmt.Errorf("AMQP causation ID does not match envelope")
		}
	} else if causationHeaderPresent {
		return fmt.Errorf("AMQP causation ID is present without an envelope causation ID")
	}
	return nil
}

func deduplicationKey(envelope Envelope) string {
	return envelope.ID + ":" + strconv.Itoa(envelope.Attempt)
}

func integerHeader(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		if typed > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(typed)
		return parsed, err == nil
	default:
		return 0, false
	}
}
