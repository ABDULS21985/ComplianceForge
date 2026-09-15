package models

import (
	"encoding/json"
	"time"
)

type EvidenceLifecycleStatus string

const (
	EvidenceLifecycleActive     EvidenceLifecycleStatus = "active"
	EvidenceLifecycleSuperseded EvidenceLifecycleStatus = "superseded"
	EvidenceLifecycleExpired    EvidenceLifecycleStatus = "expired"
)

type EvidenceCustodyEventType string

const (
	EvidenceCustodyUploaded           EvidenceCustodyEventType = "uploaded"
	EvidenceCustodyDownloadAuthorized EvidenceCustodyEventType = "download_authorized"
	EvidenceCustodyReviewed           EvidenceCustodyEventType = "reviewed"
	EvidenceCustodySuperseded         EvidenceCustodyEventType = "superseded"
	EvidenceCustodyExpired            EvidenceCustodyEventType = "expired"
	EvidenceCustodyDeleted            EvidenceCustodyEventType = "deleted"
	EvidenceCustodyIntegrityVerified  EvidenceCustodyEventType = "integrity_verified"
	EvidenceCustodyIntegrityFailed    EvidenceCustodyEventType = "integrity_failed"
	EvidenceCustodyLegalHoldPlaced    EvidenceCustodyEventType = "legal_hold_placed"
	EvidenceCustodyLegalHoldReleased  EvidenceCustodyEventType = "legal_hold_released"
)

// EvidenceReview is an immutable reviewer decision. The mutable summary fields
// on ControlEvidence are only a projection of the newest record.
type EvidenceReview struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	EvidenceID     string          `json:"evidence_id"`
	Decision       string          `json:"decision"`
	Comment        *string         `json:"comment,omitempty"`
	ReviewerID     string          `json:"reviewer_id"`
	EvidenceSHA256 string          `json:"evidence_sha256"`
	RequestID      *string         `json:"request_id,omitempty"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
}

// EvidenceCustodyEvent is one entry in an append-only per-evidence hash chain.
// Hashes use lowercase hexadecimal strings in the API rather than raw bytes.
type EvidenceCustodyEvent struct {
	ID             string                   `json:"id"`
	OrganizationID string                   `json:"organization_id"`
	EvidenceID     string                   `json:"evidence_id"`
	SeriesID       string                   `json:"series_id"`
	Sequence       int64                    `json:"sequence"`
	PreviousHash   string                   `json:"previous_hash"`
	EventHash      string                   `json:"event_hash"`
	EventType      EvidenceCustodyEventType `json:"event_type"`
	ActorUserID    *string                  `json:"actor_user_id,omitempty"`
	ActorType      string                   `json:"actor_type"`
	Reason         string                   `json:"reason"`
	ObjectSHA256   *string                  `json:"object_sha256,omitempty"`
	RequestID      *string                  `json:"request_id,omitempty"`
	Details        json.RawMessage          `json:"details"`
	CreatedAt      time.Time                `json:"created_at"`
}

// EvidenceCustodyEventInput is an internal append request. Sequence and hashes
// are always assigned by the database chain trigger.
type EvidenceCustodyEventInput struct {
	EventType    EvidenceCustodyEventType
	ActorUserID  *string
	ActorType    string
	Reason       string
	ObjectSHA256 *string
	RequestID    *string
	Details      map[string]any
}

type EvidenceCustodyChainVerification struct {
	EvidenceID           string `json:"evidence_id"`
	Valid                bool   `json:"valid"`
	EventCount           int64  `json:"event_count"`
	HeadSequence         int64  `json:"head_sequence"`
	HeadHash             string `json:"head_hash"`
	FirstInvalidSequence *int64 `json:"first_invalid_sequence,omitempty"`
}

type EvidenceLifecycleRecord struct {
	Evidence        ControlEvidence                  `json:"evidence"`
	Versions        []ControlEvidence                `json:"versions"`
	Reviews         []EvidenceReview                 `json:"reviews"`
	CustodyEvents   []EvidenceCustodyEvent           `json:"custody_events"`
	Chain           EvidenceCustodyChainVerification `json:"chain"`
	LegalHoldActive bool                             `json:"legal_hold_active"`
}

// EvidenceIntegrityTarget is the bounded metadata required for an object and
// custody verification. It avoids loading unbounded history for every check.
type EvidenceIntegrityTarget struct {
	EvidenceID     string                           `json:"evidence_id"`
	OrganizationID string                           `json:"organization_id"`
	ObjectKey      *string                          `json:"-"`
	SHA256         *string                          `json:"sha256,omitempty"`
	SizeBytes      *int64                           `json:"size_bytes,omitempty"`
	Chain          EvidenceCustodyChainVerification `json:"chain"`
}

type EvidenceIntegrityResult struct {
	EvidenceID string    `json:"evidence_id"`
	Valid      bool      `json:"valid"`
	SHA256     string    `json:"sha256"`
	SizeBytes  int64     `json:"size_bytes"`
	VerifiedAt time.Time `json:"verified_at"`
}

type ExpiredEvidenceNotice struct {
	EvidenceID              string    `json:"evidence_id"`
	OrganizationID          string    `json:"organization_id"`
	ControlImplementationID string    `json:"control_implementation_id"`
	Title                   string    `json:"title"`
	ExpiresAt               time.Time `json:"expires_at"`
	CollectedBy             *string   `json:"collected_by,omitempty"`
	ControlCode             *string   `json:"control_code,omitempty"`
	NotificationQueued      bool      `json:"-"`
}
