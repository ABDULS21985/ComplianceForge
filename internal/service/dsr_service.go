package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// DSRService manages GDPR Data Subject Requests (Articles 15-22).
type DSRService struct {
	pool      *pgxpool.Pool
	bus       *EventBus
	encKey    []byte // AES-256 key (32 bytes) for PII encryption
}

// CreateDSRInput holds the input for creating a new DSR.
type CreateDSRInput struct {
	RequestType        string `json:"request_type"`
	Priority           string `json:"priority"`
	DataSubjectName    string `json:"data_subject_name"`
	DataSubjectEmail   string `json:"data_subject_email"`
	DataSubjectPhone   string `json:"data_subject_phone"`
	DataSubjectAddress string `json:"data_subject_address"`
	RequestDescription string `json:"request_description"`
	RequestSource      string `json:"request_source"`
	ReceivedDate       string `json:"received_date"`
}

// DSRRequest represents a full DSR record with decrypted PII.
type DSRRequest struct {
	ID                 string          `json:"id"`
	OrgID              string          `json:"organization_id"`
	RequestRef         string          `json:"request_ref"`
	RequestType        string          `json:"request_type"`
	Status             string          `json:"status"`
	Priority           string          `json:"priority"`
	DataSubjectName    string          `json:"data_subject_name"`
	DataSubjectEmail   string          `json:"data_subject_email"`
	RequestDescription string          `json:"request_description"`
	RequestSource      string          `json:"request_source"`
	ReceivedDate       string          `json:"received_date"`
	ResponseDeadline   string          `json:"response_deadline"`
	ExtendedDeadline   *string         `json:"extended_deadline"`
	AssignedTo         *string         `json:"assigned_to"`
	SLAStatus          string          `json:"sla_status"`
	DaysRemaining      int             `json:"days_remaining"`
	WasExtended        bool            `json:"was_extended"`
	Tasks              []DSRTask       `json:"tasks"`
	AuditTrail         []DSRAuditEntry `json:"audit_trail"`
	CreatedAt          string          `json:"created_at"`
}

// DSRTask represents a workflow task for a DSR.
type DSRTask struct {
	ID          string  `json:"id"`
	TaskType    string  `json:"task_type"`
	Description string  `json:"description"`
	SystemName  *string `json:"system_name"`
	AssignedTo  *string `json:"assigned_to"`
	Status      string  `json:"status"`
	DueDate     *string `json:"due_date"`
	CompletedAt *string `json:"completed_at"`
	SortOrder   int     `json:"sort_order"`
}

// DSRAuditEntry represents an immutable audit trail entry.
type DSRAuditEntry struct {
	ID          string  `json:"id"`
	Action      string  `json:"action"`
	PerformedBy *string `json:"performed_by"`
	Description string  `json:"description"`
	CreatedAt   string  `json:"created_at"`
}

// DSRDashboard provides aggregate DSR statistics.
type DSRDashboard struct {
	Total             int            `json:"total"`
	ByType            map[string]int `json:"by_type"`
	ByStatus          map[string]int `json:"by_status"`
	OverdueCount      int            `json:"overdue_count"`
	AtRiskCount       int            `json:"at_risk_count"`
	AvgCompletionDays float64        `json:"avg_completion_days"`
	CompletedOnTime   int            `json:"completed_on_time"`
	CompletedLate     int            `json:"completed_late"`
}

// NewDSRService creates a new DSRService. It reads the PII encryption key
// from the DSR_ENCRYPTION_KEY environment variable (base64-encoded, 32 bytes).
func NewDSRService(pool *pgxpool.Pool, bus *EventBus) *DSRService {
	keyB64 := os.Getenv("DSR_ENCRYPTION_KEY")
	var key []byte
	if keyB64 != "" {
		var err error
		key, err = base64.StdEncoding.DecodeString(keyB64)
		if err != nil || len(key) != 32 {
			log.Warn().Msg("DSR_ENCRYPTION_KEY invalid, using zero key (not for production)")
			key = make([]byte, 32)
		}
	} else {
		log.Warn().Msg("DSR_ENCRYPTION_KEY not set, PII encryption disabled (not for production)")
		key = make([]byte, 32)
	}
	return &DSRService{pool: pool, bus: bus, encKey: key}
}

// encryptPII encrypts a plaintext string using AES-256-GCM.
func (s *DSRService) encryptPII(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptPII decrypts a base64-encoded AES-256-GCM ciphertext.
func (s *DSRService) decryptPII(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// CreateRequest creates a new DSR with encrypted PII, default task checklist,
// and audit trail entry.
func (s *DSRService) CreateRequest(ctx context.Context, orgID string, req CreateDSRInput) (*DSRRequest, error) {
	// Encrypt PII fields.
	nameEnc, err := s.encryptPII(req.DataSubjectName)
	if err != nil {
		return nil, fmt.Errorf("encrypt name: %w", err)
	}
	emailEnc, err := s.encryptPII(req.DataSubjectEmail)
	if err != nil {
		return nil, fmt.Errorf("encrypt email: %w", err)
	}
	var phoneEnc, addressEnc *string
	if req.DataSubjectPhone != "" {
		enc, err := s.encryptPII(req.DataSubjectPhone)
		if err != nil {
			return nil, fmt.Errorf("encrypt phone: %w", err)
		}
		phoneEnc = &enc
	}
	if req.DataSubjectAddress != "" {
		enc, err := s.encryptPII(req.DataSubjectAddress)
		if err != nil {
			return nil, fmt.Errorf("encrypt address: %w", err)
		}
		addressEnc = &enc
	}

	priority := req.Priority
	if priority == "" {
		priority = "standard"
	}

	// Insert the DSR (request_ref and response_deadline are auto-generated by triggers).
	var id, requestRef, status, slaStatus string
	var responseDeadline time.Time
	var daysRemaining int
	var createdAt time.Time

	err = s.pool.QueryRow(ctx, `
		INSERT INTO dsr_requests (
			organization_id, request_type, priority,
			data_subject_name_encrypted, data_subject_email_encrypted,
			data_subject_phone_encrypted, data_subject_address_encrypted,
			request_description, request_source, received_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, request_ref, status, sla_status, response_deadline, days_remaining, created_at`,
		orgID, req.RequestType, priority,
		nameEnc, emailEnc, phoneEnc, addressEnc,
		req.RequestDescription, req.RequestSource, req.ReceivedDate,
	).Scan(&id, &requestRef, &status, &slaStatus, &responseDeadline, &daysRemaining, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert DSR request: %w", err)
	}

	// Create default task checklist.
	if err := s.createTaskChecklist(ctx, orgID, id, req.RequestType); err != nil {
		log.Error().Err(err).Str("dsr_id", id).Msg("failed to create task checklist")
	}

	// Insert audit trail entry.
	_, err = s.pool.Exec(ctx, `
		INSERT INTO dsr_audit_trail (organization_id, dsr_request_id, action, description)
		VALUES ($1, $2, 'request_received', $3)`,
		orgID, id, fmt.Sprintf("DSR %s received: %s request via %s", requestRef, req.RequestType, req.RequestSource))
	if err != nil {
		log.Error().Err(err).Str("dsr_id", id).Msg("failed to create audit trail entry")
	}

	// Emit event.
	s.bus.Publish(Event{
		Type:       "dsr.received",
		Severity:   "medium",
		OrgID:      orgID,
		EntityType: "dsr_request",
		EntityID:   id,
		EntityRef:  requestRef,
		Data: map[string]interface{}{
			"request_type": req.RequestType,
			"priority":     priority,
			"deadline":     responseDeadline.Format("2006-01-02"),
		},
		Timestamp: time.Now(),
	})

	log.Info().
		Str("dsr_id", id).
		Str("ref", requestRef).
		Str("type", req.RequestType).
		Msg("DSR request created")

	result := &DSRRequest{
		ID:                 id,
		OrgID:              orgID,
		RequestRef:         requestRef,
		RequestType:        req.RequestType,
		Status:             status,
		Priority:           priority,
		DataSubjectName:    req.DataSubjectName,
		DataSubjectEmail:   req.DataSubjectEmail,
		RequestDescription: req.RequestDescription,
		RequestSource:      req.RequestSource,
		ReceivedDate:       req.ReceivedDate,
		ResponseDeadline:   responseDeadline.Format("2006-01-02"),
		SLAStatus:          slaStatus,
		DaysRemaining:      daysRemaining,
		CreatedAt:          createdAt.Format(time.RFC3339),
	}

	return result, nil
}

// GetRequest retrieves a DSR with tasks and audit trail, decrypting PII.
func (s *DSRService) GetRequest(ctx context.Context, orgID, requestID string) (*DSRRequest, error) {
	var r DSRRequest
	var nameEnc, emailEnc string
	var receivedDate, responseDeadline time.Time
	var extDeadline *time.Time
	var createdAt time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, request_ref, request_type, status, priority,
		       data_subject_name_encrypted, data_subject_email_encrypted,
		       request_description, COALESCE(request_source, ''), received_date,
		       response_deadline, extended_deadline, assigned_to,
		       sla_status, COALESCE(days_remaining, 0), COALESCE(was_extended, false), created_at
		FROM dsr_requests
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, requestID, orgID,
	).Scan(
		&r.ID, &r.OrgID, &r.RequestRef, &r.RequestType, &r.Status, &r.Priority,
		&nameEnc, &emailEnc,
		&r.RequestDescription, &r.RequestSource, &receivedDate,
		&responseDeadline, &extDeadline, &r.AssignedTo,
		&r.SLAStatus, &r.DaysRemaining, &r.WasExtended, &createdAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("DSR request not found")
		}
		return nil, fmt.Errorf("get DSR request: %w", err)
	}

	// Decrypt PII.
	r.DataSubjectName, _ = s.decryptPII(nameEnc)
	r.DataSubjectEmail, _ = s.decryptPII(emailEnc)

	r.ReceivedDate = receivedDate.Format("2006-01-02")
	r.ResponseDeadline = responseDeadline.Format("2006-01-02")
	if extDeadline != nil {
		ed := extDeadline.Format("2006-01-02")
		r.ExtendedDeadline = &ed
	}
	r.CreatedAt = createdAt.Format(time.RFC3339)

	// Recalculate SLA status based on current date.
	effectiveDeadline := responseDeadline
	if extDeadline != nil {
		effectiveDeadline = *extDeadline
	}
	now := time.Now()
	r.DaysRemaining = int(effectiveDeadline.Sub(now).Hours() / 24)
	if r.Status != "completed" && r.Status != "rejected" && r.Status != "withdrawn" {
		if now.After(effectiveDeadline) {
			r.SLAStatus = "overdue"
		} else if r.DaysRemaining <= 7 {
			r.SLAStatus = "at_risk"
		} else {
			r.SLAStatus = "on_track"
		}
	}

	// Fetch tasks.
	taskRows, err := s.pool.Query(ctx, `
		SELECT id, task_type, description, system_name, assigned_to, status,
		       due_date, completed_at, sort_order
		FROM dsr_tasks
		WHERE dsr_request_id = $1 AND organization_id = $2
		ORDER BY sort_order`, requestID, orgID)
	if err != nil {
		return nil, fmt.Errorf("query DSR tasks: %w", err)
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var t DSRTask
		var dueDate *time.Time
		var completedAt *time.Time
		if err := taskRows.Scan(&t.ID, &t.TaskType, &t.Description, &t.SystemName,
			&t.AssignedTo, &t.Status, &dueDate, &completedAt, &t.SortOrder); err != nil {
			return nil, fmt.Errorf("scan DSR task: %w", err)
		}
		if dueDate != nil {
			dd := dueDate.Format("2006-01-02")
			t.DueDate = &dd
		}
		if completedAt != nil {
			ca := completedAt.Format(time.RFC3339)
			t.CompletedAt = &ca
		}
		r.Tasks = append(r.Tasks, t)
	}

	// Fetch audit trail.
	auditRows, err := s.pool.Query(ctx, `
		SELECT id, action, performed_by, COALESCE(description, ''), created_at
		FROM dsr_audit_trail
		WHERE dsr_request_id = $1 AND organization_id = $2
		ORDER BY created_at DESC`, requestID, orgID)
	if err != nil {
		return nil, fmt.Errorf("query DSR audit trail: %w", err)
	}
	defer auditRows.Close()

	for auditRows.Next() {
		var a DSRAuditEntry
		var at time.Time
		if err := auditRows.Scan(&a.ID, &a.Action, &a.PerformedBy, &a.Description, &at); err != nil {
			return nil, fmt.Errorf("scan DSR audit entry: %w", err)
		}
		a.CreatedAt = at.Format(time.RFC3339)
		r.AuditTrail = append(r.AuditTrail, a)
	}

	return &r, nil
}

// ListRequests returns a paginated list of DSRs with optional filters.
func (s *DSRService) ListRequests(ctx context.Context, orgID string, page, pageSize int, filters map[string]string) ([]DSRRequest, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Build dynamic WHERE clause.
	whereClause := "organization_id = $1 AND deleted_at IS NULL"
	args := []interface{}{orgID}
	argIdx := 2

	if v, ok := filters["status"]; ok && v != "" {
		whereClause += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, v)
		argIdx++
	}
	if v, ok := filters["request_type"]; ok && v != "" {
		whereClause += fmt.Sprintf(" AND request_type = $%d", argIdx)
		args = append(args, v)
		argIdx++
	}
	if v, ok := filters["sla_status"]; ok && v != "" {
		whereClause += fmt.Sprintf(" AND sla_status = $%d", argIdx)
		args = append(args, v)
		argIdx++
	}

	var total int
	err := s.pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM dsr_requests WHERE %s", whereClause), args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count DSR requests: %w", err)
	}

	args = append(args, pageSize, offset)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, organization_id, request_ref, request_type, status, priority,
		       data_subject_name_encrypted, data_subject_email_encrypted,
		       request_description, COALESCE(request_source, ''), received_date,
		       response_deadline, extended_deadline, assigned_to,
		       sla_status, COALESCE(days_remaining, 0), COALESCE(was_extended, false), created_at
		FROM dsr_requests
		WHERE %s
		ORDER BY received_date DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list DSR requests: %w", err)
	}
	defer rows.Close()

	var results []DSRRequest
	for rows.Next() {
		var r DSRRequest
		var nameEnc, emailEnc string
		var receivedDate, responseDeadline time.Time
		var extDeadline *time.Time
		var createdAt time.Time
		if err := rows.Scan(
			&r.ID, &r.OrgID, &r.RequestRef, &r.RequestType, &r.Status, &r.Priority,
			&nameEnc, &emailEnc,
			&r.RequestDescription, &r.RequestSource, &receivedDate,
			&responseDeadline, &extDeadline, &r.AssignedTo,
			&r.SLAStatus, &r.DaysRemaining, &r.WasExtended, &createdAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan DSR request: %w", err)
		}
		r.DataSubjectName, _ = s.decryptPII(nameEnc)
		r.DataSubjectEmail, _ = s.decryptPII(emailEnc)
		r.ReceivedDate = receivedDate.Format("2006-01-02")
		r.ResponseDeadline = responseDeadline.Format("2006-01-02")
		if extDeadline != nil {
			ed := extDeadline.Format("2006-01-02")
			r.ExtendedDeadline = &ed
		}
		r.CreatedAt = createdAt.Format(time.RFC3339)
		results = append(results, r)
	}

	return results, total, nil
}

// VerifyIdentity marks a DSR's data subject identity as verified (Art. 12(6)).
func (s *DSRService) VerifyIdentity(ctx context.Context, orgID, requestID, method, verifiedByUserID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dsr_requests
		SET data_subject_id_verified = true,
		    identity_verification_method = $1,
		    identity_verified_at = NOW(),
		    identity_verified_by = $2,
		    status = CASE WHEN status = 'received' THEN 'identity_verification' ELSE status END
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL`,
		method, verifiedByUserID, requestID, orgID)
	if err != nil {
		return fmt.Errorf("verify identity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DSR request not found")
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO dsr_audit_trail (organization_id, dsr_request_id, action, performed_by, description)
		VALUES ($1, $2, 'identity_verified', $3, $4)`,
		orgID, requestID, verifiedByUserID,
		fmt.Sprintf("Identity verified via %s", method))

	// Update verify_identity task if it exists.
	_, _ = s.pool.Exec(ctx, `
		UPDATE dsr_tasks SET status = 'completed', completed_at = NOW(), completed_by = $1
		WHERE dsr_request_id = $2 AND organization_id = $3 AND task_type = 'verify_identity' AND status != 'completed'`,
		verifiedByUserID, requestID, orgID)

	log.Info().Str("dsr_id", requestID).Str("method", method).Msg("DSR identity verified")
	return nil
}

// AssignRequest assigns a DSR to a user and transitions status to in_progress.
func (s *DSRService) AssignRequest(ctx context.Context, orgID, requestID, assigneeID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dsr_requests
		SET assigned_to = $1,
		    status = CASE WHEN status IN ('received', 'identity_verification') THEN 'in_progress' ELSE status END
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL`,
		assigneeID, requestID, orgID)
	if err != nil {
		return fmt.Errorf("assign DSR: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DSR request not found")
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO dsr_audit_trail (organization_id, dsr_request_id, action, performed_by, description)
		VALUES ($1, $2, 'assigned', $3, $4)`,
		orgID, requestID, assigneeID,
		fmt.Sprintf("Request assigned to user %s", assigneeID))

	log.Info().Str("dsr_id", requestID).Str("assignee", assigneeID).Msg("DSR request assigned")
	return nil
}

// ExtendDeadline applies an Art. 12(3) extension (additional 60 days).
func (s *DSRService) ExtendDeadline(ctx context.Context, orgID, requestID, reason string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dsr_requests
		SET extended_deadline = received_date + INTERVAL '90 days',
		    extension_reason = $1,
		    extension_notified_at = NOW(),
		    was_extended = true,
		    status = 'extended',
		    sla_status = 'on_track'
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
		  AND was_extended = false`,
		reason, requestID, orgID)
	if err != nil {
		return fmt.Errorf("extend DSR deadline: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DSR request not found or already extended")
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO dsr_audit_trail (organization_id, dsr_request_id, action, description)
		VALUES ($1, $2, 'deadline_extended', $3)`,
		orgID, requestID, fmt.Sprintf("Deadline extended by 60 days. Reason: %s", reason))

	log.Info().Str("dsr_id", requestID).Msg("DSR deadline extended")
	return nil
}

// CompleteTask marks a DSR task as completed.
func (s *DSRService) CompleteTask(ctx context.Context, orgID, taskID, completedByUserID, notes, evidencePath string) error {
	var requestID string
	err := s.pool.QueryRow(ctx, `
		UPDATE dsr_tasks
		SET status = 'completed', completed_at = NOW(), completed_by = $1, notes = $2, evidence_path = $3
		WHERE id = $4 AND organization_id = $5 AND status != 'completed'
		RETURNING dsr_request_id`,
		completedByUserID, notes, evidencePath, taskID, orgID,
	).Scan(&requestID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("task not found or already completed")
		}
		return fmt.Errorf("complete DSR task: %w", err)
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO dsr_audit_trail (organization_id, dsr_request_id, action, performed_by, description)
		VALUES ($1, $2, 'task_completed', $3, $4)`,
		orgID, requestID, completedByUserID,
		fmt.Sprintf("Task %s completed", taskID))

	log.Info().Str("task_id", taskID).Str("dsr_id", requestID).Msg("DSR task completed")
	return nil
}

// CompleteRequest marks a DSR as completed with response details.
func (s *DSRService) CompleteRequest(ctx context.Context, orgID, requestID, responseMethod, documentPath string) error {
	var wasOnTime bool
	err := s.pool.QueryRow(ctx, `
		UPDATE dsr_requests
		SET status = 'completed', completed_at = NOW(), response_method = $1,
		    response_document_path = $2,
		    was_completed_on_time = (
		        CASE WHEN COALESCE(extended_deadline, response_deadline) >= CURRENT_DATE THEN true ELSE false END
		    )
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL
		  AND status NOT IN ('completed', 'rejected', 'withdrawn')
		RETURNING was_completed_on_time`,
		responseMethod, documentPath, requestID, orgID,
	).Scan(&wasOnTime)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("DSR request not found or already completed")
		}
		return fmt.Errorf("complete DSR: %w", err)
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO dsr_audit_trail (organization_id, dsr_request_id, action, description)
		VALUES ($1, $2, 'response_sent', $3)`,
		orgID, requestID,
		fmt.Sprintf("Response sent via %s. On time: %t", responseMethod, wasOnTime))

	s.bus.Publish(Event{
		Type:       "dsr.completed",
		Severity:   "low",
		OrgID:      orgID,
		EntityType: "dsr_request",
		EntityID:   requestID,
		Data:       map[string]interface{}{"on_time": wasOnTime},
		Timestamp:  time.Now(),
	})

	log.Info().Str("dsr_id", requestID).Bool("on_time", wasOnTime).Msg("DSR request completed")
	return nil
}

// RejectRequest rejects a DSR with a reason and legal basis (e.g., Art. 12(5)).
func (s *DSRService) RejectRequest(ctx context.Context, orgID, requestID, reason, legalBasis string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dsr_requests
		SET status = 'rejected', completed_at = NOW(),
		    rejection_reason = $1, rejection_legal_basis = $2
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL
		  AND status NOT IN ('completed', 'rejected', 'withdrawn')`,
		reason, legalBasis, requestID, orgID)
	if err != nil {
		return fmt.Errorf("reject DSR: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DSR request not found or already finalized")
	}

	_, _ = s.pool.Exec(ctx, `
		INSERT INTO dsr_audit_trail (organization_id, dsr_request_id, action, description)
		VALUES ($1, $2, 'request_rejected', $3)`,
		orgID, requestID,
		fmt.Sprintf("Rejected. Reason: %s. Legal basis: %s", reason, legalBasis))

	log.Info().Str("dsr_id", requestID).Str("reason", reason).Msg("DSR request rejected")
	return nil
}

// GetDashboard returns aggregate DSR statistics for the organization.
func (s *DSRService) GetDashboard(ctx context.Context, orgID string) (*DSRDashboard, error) {
	d := &DSRDashboard{
		ByType:   make(map[string]int),
		ByStatus: make(map[string]int),
	}

	// Total count.
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM dsr_requests
		WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).Scan(&d.Total)

	// By type.
	typeRows, err := s.pool.Query(ctx, `
		SELECT request_type, COUNT(*) FROM dsr_requests
		WHERE organization_id = $1 AND deleted_at IS NULL
		GROUP BY request_type`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query DSR by type: %w", err)
	}
	defer typeRows.Close()
	for typeRows.Next() {
		var t string
		var c int
		if err := typeRows.Scan(&t, &c); err != nil {
			return nil, fmt.Errorf("scan DSR type: %w", err)
		}
		d.ByType[t] = c
	}

	// By status.
	statusRows, err := s.pool.Query(ctx, `
		SELECT status, COUNT(*) FROM dsr_requests
		WHERE organization_id = $1 AND deleted_at IS NULL
		GROUP BY status`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query DSR by status: %w", err)
	}
	defer statusRows.Close()
	for statusRows.Next() {
		var st string
		var c int
		if err := statusRows.Scan(&st, &c); err != nil {
			return nil, fmt.Errorf("scan DSR status: %w", err)
		}
		d.ByStatus[st] = c
	}

	// SLA stats.
	_ = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE sla_status = 'overdue'),
			COUNT(*) FILTER (WHERE sla_status = 'at_risk')
		FROM dsr_requests
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND status NOT IN ('completed', 'rejected', 'withdrawn')`, orgID).
		Scan(&d.OverdueCount, &d.AtRiskCount)

	// Completion time stats.
	_ = s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(AVG(EXTRACT(EPOCH FROM (completed_at - created_at)) / 86400), 0),
			COUNT(*) FILTER (WHERE was_completed_on_time = true),
			COUNT(*) FILTER (WHERE was_completed_on_time = false)
		FROM dsr_requests
		WHERE organization_id = $1 AND deleted_at IS NULL AND status = 'completed'`, orgID).
		Scan(&d.AvgCompletionDays, &d.CompletedOnTime, &d.CompletedLate)

	return d, nil
}

// CheckSLACompliance returns all DSRs that are at-risk or overdue.
func (s *DSRService) CheckSLACompliance(ctx context.Context, orgID string) ([]DSRRequest, error) {
	// First, update SLA statuses based on current date.
	_, err := s.pool.Exec(ctx, `
		UPDATE dsr_requests
		SET sla_status = CASE
			WHEN COALESCE(extended_deadline, response_deadline) < CURRENT_DATE THEN 'overdue'
			WHEN COALESCE(extended_deadline, response_deadline) - CURRENT_DATE <= 7 THEN 'at_risk'
			ELSE 'on_track'
		END,
		days_remaining = COALESCE(extended_deadline, response_deadline) - CURRENT_DATE
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND status NOT IN ('completed', 'rejected', 'withdrawn')`, orgID)
	if err != nil {
		return nil, fmt.Errorf("update SLA statuses: %w", err)
	}

	// Return at-risk and overdue.
	filters := map[string]string{}
	// We query both at_risk and overdue manually since ListRequests only supports one filter.
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, request_ref, request_type, status, priority,
		       data_subject_name_encrypted, data_subject_email_encrypted,
		       request_description, COALESCE(request_source, ''), received_date,
		       response_deadline, extended_deadline, assigned_to,
		       sla_status, COALESCE(days_remaining, 0), COALESCE(was_extended, false), created_at
		FROM dsr_requests
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND sla_status IN ('at_risk', 'overdue')
		  AND status NOT IN ('completed', 'rejected', 'withdrawn')
		ORDER BY response_deadline ASC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query at-risk DSRs: %w", err)
	}
	defer rows.Close()
	_ = filters // suppress unused variable

	var results []DSRRequest
	for rows.Next() {
		var r DSRRequest
		var nameEnc, emailEnc string
		var receivedDate, responseDeadline time.Time
		var extDeadline *time.Time
		var createdAt time.Time
		if err := rows.Scan(
			&r.ID, &r.OrgID, &r.RequestRef, &r.RequestType, &r.Status, &r.Priority,
			&nameEnc, &emailEnc,
			&r.RequestDescription, &r.RequestSource, &receivedDate,
			&responseDeadline, &extDeadline, &r.AssignedTo,
			&r.SLAStatus, &r.DaysRemaining, &r.WasExtended, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan at-risk DSR: %w", err)
		}
		r.DataSubjectName, _ = s.decryptPII(nameEnc)
		r.DataSubjectEmail, _ = s.decryptPII(emailEnc)
		r.ReceivedDate = receivedDate.Format("2006-01-02")
		r.ResponseDeadline = responseDeadline.Format("2006-01-02")
		if extDeadline != nil {
			ed := extDeadline.Format("2006-01-02")
			r.ExtendedDeadline = &ed
		}
		r.CreatedAt = createdAt.Format(time.RFC3339)
		results = append(results, r)
	}

	return results, nil
}

// createTaskChecklist generates default tasks based on DSR request type.
func (s *DSRService) createTaskChecklist(ctx context.Context, orgID, requestID, requestType string) error {
	type taskDef struct {
		taskType    string
		description string
		sortOrder   int
	}

	commonTasks := []taskDef{
		{"verify_identity", "Verify data subject identity (Art. 12(6))", 1},
		{"locate_data", "Locate data subject's personal data across all systems", 2},
		{"review_exemptions", "Review applicable exemptions and legal restrictions", 3},
	}

	var specificTasks []taskDef

	switch requestType {
	case "access":
		specificTasks = []taskDef{
			{"extract_data", "Extract personal data from all identified systems", 4},
			{"review_data", "Review extracted data for third-party PII and exemptions", 5},
			{"compile_response", "Compile data subject access response package", 6},
			{"send_response", "Send response to data subject", 7},
		}
	case "erasure":
		specificTasks = []taskDef{
			{"review_exemptions", "Assess retention obligations and erasure exemptions", 4},
			{"notify_processors", "Notify data processors of erasure obligation (Art. 28)", 5},
			{"execute_erasure", "Execute data deletion across all systems", 6},
			{"confirm_erasure", "Verify deletion completed in all systems", 7},
			{"notify_third_parties", "Notify third parties of erasure (Art. 19)", 8},
			{"send_response", "Confirm erasure to data subject", 9},
		}
	case "rectification":
		specificTasks = []taskDef{
			{"review_data", "Review current data and requested corrections", 4},
			{"execute_correction", "Apply corrections across all systems", 5},
			{"verify_correction", "Verify corrections applied accurately", 6},
			{"notify_third_parties", "Notify recipients of rectification (Art. 19)", 7},
			{"send_response", "Confirm rectification to data subject", 8},
		}
	case "portability":
		specificTasks = []taskDef{
			{"extract_data", "Extract personal data from automated processing systems", 4},
			{"extract_machine_readable", "Export data in structured, machine-readable format (CSV/JSON)", 5},
			{"review_data", "Review export for completeness and accuracy", 6},
			{"send_response", "Transmit data package to data subject or nominated controller", 7},
		}
	case "restriction":
		specificTasks = []taskDef{
			{"review_exemptions", "Assess grounds for restriction (Art. 18(1))", 4},
			{"notify_processors", "Notify processors to restrict processing", 5},
			{"send_response", "Confirm restriction to data subject", 6},
		}
	case "objection":
		specificTasks = []taskDef{
			{"review_exemptions", "Assess compelling legitimate grounds vs objection", 4},
			{"compile_response", "Prepare response with legal assessment", 5},
			{"send_response", "Send decision to data subject", 6},
		}
	case "automated_decision":
		specificTasks = []taskDef{
			{"review_data", "Identify automated decision-making processes affecting subject", 4},
			{"review_exemptions", "Assess Art. 22(2) exceptions", 5},
			{"compile_response", "Prepare explanation of logic, significance, and consequences", 6},
			{"send_response", "Provide information and human review option to subject", 7},
		}
	default:
		specificTasks = []taskDef{
			{"compile_response", "Prepare response", 4},
			{"send_response", "Send response to data subject", 5},
		}
	}

	allTasks := append(commonTasks, specificTasks...)

	for _, t := range allTasks {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO dsr_tasks (organization_id, dsr_request_id, task_type, description, sort_order)
			VALUES ($1, $2, $3, $4, $5)`,
			orgID, requestID, t.taskType, t.description, t.sortOrder)
		if err != nil {
			return fmt.Errorf("insert task %s: %w", t.taskType, err)
		}
	}

	return nil
}
