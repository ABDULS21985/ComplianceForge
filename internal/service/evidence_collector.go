package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/complianceforge/platform/internal/database"
)

// EvidenceCollector handles automated evidence collection and validation.
type EvidenceCollector struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// CollectionConfig defines how evidence is collected for a control.
type CollectionConfig struct {
	ID                      string                `json:"id"`
	OrgID                   string                `json:"organization_id"`
	ControlImplementationID string                `json:"control_implementation_id"`
	Name                    string                `json:"name"`
	CollectionMethod        string                `json:"collection_method"`
	ScheduleCron            string                `json:"schedule_cron"`
	AcceptanceCriteria      []AcceptanceCriterion `json:"acceptance_criteria"`
	IsActive                bool                  `json:"is_active"`
	ConsecutiveFailures     int                   `json:"consecutive_failures"`
	FailureThreshold        int                   `json:"failure_threshold"`
	LastCollectionAt        *string               `json:"last_collection_at"`
	LastCollectionStatus    string                `json:"last_collection_status"`
}

// AcceptanceCriterion defines a single validation rule for collected evidence.
type AcceptanceCriterion struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // equals, not_equals, greater_than, less_than, contains, exists
	Value    interface{} `json:"value"`
}

// CollectionRun records a single evidence collection attempt.
type CollectionRun struct {
	ID                string             `json:"id"`
	ConfigID          string             `json:"config_id"`
	Status            string             `json:"status"`
	StartedAt         *string            `json:"started_at"`
	CompletedAt       *string            `json:"completed_at"`
	DurationMs        *int               `json:"duration_ms"`
	CollectedData     interface{}        `json:"collected_data"`
	ValidationResults []ValidationResult `json:"validation_results"`
	AllCriteriaPassed bool               `json:"all_criteria_passed"`
	ErrorMessage      *string            `json:"error_message"`
}

// ValidationResult records the outcome of one acceptance criterion check.
type ValidationResult struct {
	CriteriaIndex int         `json:"criteria_index"`
	Passed        bool        `json:"passed"`
	ActualValue   interface{} `json:"actual_value"`
	Message       string      `json:"message"`
}

const evidenceCollectionRunLease = 30 * time.Minute

// NewEvidenceCollector creates a new EvidenceCollector.
func NewEvidenceCollector(pool *pgxpool.Pool, bus *EventBus) *EvidenceCollector {
	return &EvidenceCollector{pool: pool, bus: bus}
}

// RunCollection executes an evidence collection for a given config, validates
// results, and creates an evidence record if all criteria pass.
func (ec *EvidenceCollector) RunCollection(ctx context.Context, configID string) (*CollectionRun, error) {
	if ec == nil || ec.pool == nil || ec.bus == nil {
		return nil, fmt.Errorf("evidence collector is not configured")
	}
	startTime := time.Now()
	querier := database.QuerierFromContext(ctx, ec.pool)

	// Fetch the config.
	var orgID, controlImplID, name, method string
	var criteriaJSON []byte
	var failureThreshold int
	var configUpdatedAt time.Time

	err := querier.QueryRow(ctx, `
		SELECT organization_id, control_implementation_id, name, collection_method,
		       acceptance_criteria, failure_threshold, updated_at
		FROM evidence_collection_configs
		WHERE id = $1 AND is_active = true`, configID,
	).Scan(&orgID, &controlImplID, &name, &method, &criteriaJSON, &failureThreshold, &configUpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("collection config not found or inactive")
		}
		return nil, fmt.Errorf("fetch collection config: %w", err)
	}

	var criteria []AcceptanceCriterion
	if err := json.Unmarshal(criteriaJSON, &criteria); err != nil {
		return nil, fmt.Errorf("decode collection acceptance criteria: %w", err)
	}

	// Create the collection run record.
	var runID string
	startedAtStr := startTime.Format(time.RFC3339)
	err = withEvidenceCollectorTransaction(ctx, querier, func(tx pgx.Tx) error {
		var active bool
		var currentImplementationID string
		var currentUpdatedAt time.Time
		if err := tx.QueryRow(ctx, `SELECT control_implementation_id,updated_at,is_active
			FROM evidence_collection_configs
			WHERE organization_id=$1::uuid AND id=$2::uuid FOR UPDATE`, orgID, configID).
			Scan(&currentImplementationID, &currentUpdatedAt, &active); err != nil {
			return err
		}
		if !active || currentImplementationID != controlImplID || !currentUpdatedAt.Equal(configUpdatedAt) {
			return pgx.ErrNoRows
		}
		if _, err := tx.Exec(ctx, `UPDATE evidence_collection_runs
			SET status='timeout',completed_at=$3,
			    duration_ms=LEAST(2147483647,GREATEST(0,
			        FLOOR(EXTRACT(EPOCH FROM ($3-COALESCE(started_at,created_at)))*1000)::bigint))::integer,
			    error_message='Evidence collection run lease expired before completion'
			WHERE organization_id=$1::uuid AND config_id=$2::uuid
			  AND status IN ('scheduled','running')
			  AND COALESCE(started_at,created_at)<$4`,
			orgID, configID, startTime, startTime.Add(-evidenceCollectionRunLease)); err != nil {
			return fmt.Errorf("expire stale collection run: %w", err)
		}
		return tx.QueryRow(ctx, `INSERT INTO evidence_collection_runs (
			organization_id,config_id,control_implementation_id,status,started_at)
			VALUES ($1::uuid,$2::uuid,$3::uuid,'running',$4)
			ON CONFLICT (organization_id,config_id)
				WHERE status IN ('scheduled','running') DO NOTHING
			RETURNING id`, orgID, configID, controlImplID, startTime).Scan(&runID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("collection config changed, became inactive, or already has an active run")
		}
		return nil, fmt.Errorf("create collection run: %w", err)
	}

	// Execute collection based on method.
	// For now, this is a placeholder that simulates collecting data.
	// In production, each method would invoke a different collector:
	//   - api_fetch: HTTP GET to configured endpoint
	//   - file_watch: read file from configured path
	//   - script_execution: run a script and capture output
	//   - webhook_receive: data already received via webhook
	//   - email_parse: parse email inbox for evidence
	collectedData := map[string]interface{}{
		"collection_method": method,
		"config_name":       name,
		"collected_at":      startTime.Format(time.RFC3339),
		"status":            "collected",
	}

	// Validate against acceptance criteria.
	validationResults, allPassed := ec.ValidateEvidence(collectedData, criteria)

	durationMs := int(time.Since(startTime).Milliseconds())
	completedAt := time.Now()
	completedAtStr := completedAt.Format(time.RFC3339)

	// Determine run status.
	runStatus := "success"
	if !allPassed && len(criteria) > 0 {
		runStatus = "validation_failed"
	}

	// Marshal results for storage.
	validationJSON, err := json.Marshal(validationResults)
	if err != nil {
		return nil, ec.failCollectionRun(ctx, querier, orgID, runID, completedAt, durationMs,
			fmt.Errorf("marshal validation results: %w", err))
	}
	collectedJSON, err := json.Marshal(collectedData)
	if err != nil {
		return nil, ec.failCollectionRun(ctx, querier, orgID, runID, completedAt, durationMs,
			fmt.Errorf("marshal collected evidence: %w", err))
	}
	evidenceMetadata, err := automatedEvidenceProofMetadata(runID, collectedJSON, validationJSON)
	if err != nil {
		return nil, ec.failCollectionRun(ctx, querier, orgID, runID, completedAt, durationMs,
			fmt.Errorf("marshal collected evidence proof metadata: %w", err))
	}

	var newFailures int
	persistErr := withEvidenceCollectorTransaction(ctx, querier, func(tx pgx.Tx) error {
		var currentImplementationID string
		var currentUpdatedAt time.Time
		var active bool
		if err := tx.QueryRow(ctx, `SELECT control_implementation_id,updated_at,is_active
			FROM evidence_collection_configs
			WHERE organization_id=$1::uuid AND id=$2::uuid
			FOR UPDATE`, orgID, configID).
			Scan(&currentImplementationID, &currentUpdatedAt, &active); err != nil {
			return fmt.Errorf("lock collection config: %w", err)
		}
		if !active || currentImplementationID != controlImplID || !currentUpdatedAt.Equal(configUpdatedAt) {
			return errors.New("collection config changed while the run was executing")
		}

		var evidenceID *string
		if allPassed || len(criteria) == 0 {
			createdEvidenceID := ""
			if err := tx.QueryRow(ctx, `INSERT INTO control_evidence (
				organization_id,control_implementation_id,title,description,
				evidence_type,collection_method,collected_at,metadata
			) SELECT $1::uuid,implementation.id,$3,$4,'report','automated',$5,$6::jsonb
			FROM control_implementations AS implementation
			WHERE implementation.organization_id=$1::uuid
			  AND implementation.id=$2::uuid AND implementation.deleted_at IS NULL
			RETURNING id`, orgID, controlImplID,
				fmt.Sprintf("Automated evidence: %s", name),
				fmt.Sprintf("Collected via %s method. All %d acceptance criteria passed.", method, len(criteria)),
				completedAt, evidenceMetadata).Scan(&createdEvidenceID); err != nil {
				return fmt.Errorf("create collected evidence: %w", err)
			}
			evidenceID = &createdEvidenceID
			tag, err := tx.Exec(ctx, `UPDATE evidence_collection_configs
				SET last_collection_at=$3,last_collection_status=$4,consecutive_failures=0
				WHERE organization_id=$1::uuid AND id=$2::uuid`,
				orgID, configID, completedAt, runStatus)
			if err := requireOneEvidenceRow(err, tag.RowsAffected()); err != nil {
				return fmt.Errorf("update successful collection config: %w", err)
			}
		} else {
			if err := tx.QueryRow(ctx, `UPDATE evidence_collection_configs
				SET last_collection_at=$3,last_collection_status=$4,
				    consecutive_failures=consecutive_failures+1
				WHERE organization_id=$1::uuid AND id=$2::uuid
				RETURNING consecutive_failures`, orgID, configID, completedAt, runStatus).
				Scan(&newFailures); err != nil {
				return fmt.Errorf("update failed collection config: %w", err)
			}
		}

		tag, err := tx.Exec(ctx, `UPDATE evidence_collection_runs
			SET status=$3,completed_at=$4,duration_ms=$5,collected_data=$6::jsonb,
			    validation_results=$7::jsonb,all_criteria_passed=$8,error_message=NULL,
			    evidence_id=$9::uuid
			WHERE organization_id=$1::uuid AND id=$2::uuid AND status='running'`,
			orgID, runID, runStatus, completedAt, durationMs,
			collectedJSON, validationJSON, allPassed, evidenceID)
		if err != nil {
			return fmt.Errorf("finalize collection run: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return errors.New("finalize collection run: active run no longer exists")
		}
		return nil
	})
	if persistErr != nil {
		return nil, ec.failCollectionRun(ctx, querier, orgID, runID, completedAt, durationMs, persistErr)
	}

	if runStatus == "validation_failed" && newFailures >= failureThreshold {
		ec.bus.Publish(Event{
			Type:       "evidence.collection_threshold_breached",
			Severity:   "high",
			OrgID:      orgID,
			EntityType: "evidence_collection_config",
			EntityID:   configID,
			EntityRef:  name,
			Data: map[string]interface{}{
				"consecutive_failures": newFailures,
				"threshold":            failureThreshold,
			},
			Timestamp: time.Now(),
		})
	}

	run := &CollectionRun{
		ID:                runID,
		ConfigID:          configID,
		Status:            runStatus,
		StartedAt:         &startedAtStr,
		CompletedAt:       &completedAtStr,
		DurationMs:        &durationMs,
		CollectedData:     collectedData,
		ValidationResults: validationResults,
		AllCriteriaPassed: allPassed,
	}

	log.Info().
		Str("run_id", runID).
		Str("config_id", configID).
		Str("status", runStatus).
		Bool("all_passed", allPassed).
		Int("duration_ms", durationMs).
		Msg("evidence collection run completed")

	return run, nil
}

func automatedEvidenceProofMetadata(runID string, collectedData, validationResults []byte) ([]byte, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || !json.Valid(collectedData) || !json.Valid(validationResults) {
		return nil, errors.New("automated evidence proof inputs are invalid")
	}
	collectedDigest := fmt.Sprintf("%x", sha256.Sum256(collectedData))
	validationDigest := fmt.Sprintf("%x", sha256.Sum256(validationResults))
	return json.Marshal(map[string]map[string]string{
		"automated_proof": {
			"proof_schema":              "automated-evidence/v1",
			"collection_run_id":         runID,
			"collected_data_sha256":     collectedDigest,
			"validation_results_sha256": validationDigest,
		},
	})
}

func (ec *EvidenceCollector) failCollectionRun(
	ctx context.Context,
	querier database.Querier,
	orgID string,
	runID string,
	completedAt time.Time,
	durationMs int,
	cause error,
) error {
	message := cause.Error()
	if len(message) > 4000 {
		message = message[:4000]
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	tag, updateErr := querier.Exec(cleanupCtx, `
		UPDATE evidence_collection_runs
		SET status='failed', completed_at=$2, duration_ms=$3, error_message=$4
		WHERE id=$1::uuid AND organization_id=$5::uuid AND status='running'`,
		runID, completedAt, durationMs, message, orgID)
	if err := requireOneEvidenceRow(updateErr, tag.RowsAffected()); err != nil {
		return errors.Join(cause, fmt.Errorf("record failed collection run: %w", err))
	}
	return cause
}

type evidenceTransactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func withEvidenceCollectorTransaction(
	ctx context.Context,
	querier database.Querier,
	fn func(pgx.Tx) error,
) (returnErr error) {
	beginner, ok := querier.(evidenceTransactionBeginner)
	if !ok {
		return errors.New("evidence collector database executor cannot start a transaction")
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin evidence collection transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			returnErr = errors.Join(returnErr, fmt.Errorf("rollback evidence collection transaction: %w", rollbackErr))
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit evidence collection transaction: %w", err)
	}
	return nil
}

func requireOneEvidenceRow(err error, rowsAffected int64) error {
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return fmt.Errorf("expected one affected row, got %d", rowsAffected)
	}
	return nil
}

// ValidateEvidence evaluates each acceptance criterion against collected data
// and returns the results with an overall pass/fail.
func (ec *EvidenceCollector) ValidateEvidence(data interface{}, criteria []AcceptanceCriterion) ([]ValidationResult, bool) {
	if len(criteria) == 0 {
		return nil, true
	}

	// Convert data to map for field access.
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		// Try JSON round-trip for struct types.
		b, err := json.Marshal(data)
		if err != nil {
			results := make([]ValidationResult, len(criteria))
			for i := range criteria {
				results[i] = ValidationResult{
					CriteriaIndex: i,
					Passed:        false,
					Message:       "cannot convert collected data to map for validation",
				}
			}
			return results, false
		}
		dataMap = make(map[string]interface{})
		_ = json.Unmarshal(b, &dataMap)
	}

	allPassed := true
	results := make([]ValidationResult, 0, len(criteria))

	for i, c := range criteria {
		result := ValidationResult{CriteriaIndex: i}
		actual, exists := dataMap[c.Field]
		result.ActualValue = actual

		switch c.Operator {
		case "exists":
			result.Passed = exists
			if result.Passed {
				result.Message = fmt.Sprintf("field '%s' exists", c.Field)
			} else {
				result.Message = fmt.Sprintf("field '%s' does not exist", c.Field)
			}

		case "equals":
			result.Passed = exists && fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", c.Value)
			if result.Passed {
				result.Message = fmt.Sprintf("field '%s' equals '%v'", c.Field, c.Value)
			} else {
				result.Message = fmt.Sprintf("field '%s': expected '%v', got '%v'", c.Field, c.Value, actual)
			}

		case "not_equals":
			result.Passed = !exists || fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", c.Value)
			if result.Passed {
				result.Message = fmt.Sprintf("field '%s' does not equal '%v'", c.Field, c.Value)
			} else {
				result.Message = fmt.Sprintf("field '%s' unexpectedly equals '%v'", c.Field, c.Value)
			}

		case "greater_than":
			actualFloat, aErr := toFloat64(actual)
			expectedFloat, eErr := toFloat64(c.Value)
			if aErr != nil || eErr != nil {
				result.Passed = false
				result.Message = fmt.Sprintf("field '%s': cannot compare as numbers", c.Field)
			} else {
				result.Passed = actualFloat > expectedFloat
				result.Message = fmt.Sprintf("field '%s': %v > %v = %t", c.Field, actualFloat, expectedFloat, result.Passed)
			}

		case "less_than":
			actualFloat, aErr := toFloat64(actual)
			expectedFloat, eErr := toFloat64(c.Value)
			if aErr != nil || eErr != nil {
				result.Passed = false
				result.Message = fmt.Sprintf("field '%s': cannot compare as numbers", c.Field)
			} else {
				result.Passed = actualFloat < expectedFloat
				result.Message = fmt.Sprintf("field '%s': %v < %v = %t", c.Field, actualFloat, expectedFloat, result.Passed)
			}

		case "contains":
			actualStr := fmt.Sprintf("%v", actual)
			expectedStr := fmt.Sprintf("%v", c.Value)
			result.Passed = exists && len(actualStr) > 0 && contains(actualStr, expectedStr)
			if result.Passed {
				result.Message = fmt.Sprintf("field '%s' contains '%v'", c.Field, c.Value)
			} else {
				result.Message = fmt.Sprintf("field '%s' does not contain '%v'", c.Field, c.Value)
			}

		default:
			result.Passed = false
			result.Message = fmt.Sprintf("unknown operator: %s", c.Operator)
		}

		if !result.Passed {
			allPassed = false
		}
		results = append(results, result)
	}

	return results, allPassed
}

// ListConfigs returns all evidence collection configs for an organization.
func (ec *EvidenceCollector) ListConfigs(ctx context.Context, orgID string) ([]CollectionConfig, error) {
	querier := database.QuerierFromContext(ctx, ec.pool)
	rows, err := querier.Query(ctx, `
		SELECT id, organization_id, control_implementation_id, name, collection_method,
		       COALESCE(schedule_cron, ''), acceptance_criteria, is_active,
		       consecutive_failures, failure_threshold,
		       last_collection_at, COALESCE(last_collection_status, '')
		FROM evidence_collection_configs
		WHERE organization_id = $1
		ORDER BY name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list collection configs: %w", err)
	}
	defer rows.Close()

	var configs []CollectionConfig
	for rows.Next() {
		var c CollectionConfig
		var criteriaJSON []byte
		var lastAt *time.Time
		if err := rows.Scan(&c.ID, &c.OrgID, &c.ControlImplementationID, &c.Name,
			&c.CollectionMethod, &c.ScheduleCron, &criteriaJSON, &c.IsActive,
			&c.ConsecutiveFailures, &c.FailureThreshold,
			&lastAt, &c.LastCollectionStatus); err != nil {
			return nil, fmt.Errorf("scan collection config: %w", err)
		}
		_ = json.Unmarshal(criteriaJSON, &c.AcceptanceCriteria)
		if c.AcceptanceCriteria == nil {
			c.AcceptanceCriteria = []AcceptanceCriterion{}
		}
		if lastAt != nil {
			la := lastAt.Format(time.RFC3339)
			c.LastCollectionAt = &la
		}
		configs = append(configs, c)
	}

	return configs, nil
}

// CreateConfig creates a new evidence collection config.
func (ec *EvidenceCollector) CreateConfig(ctx context.Context, orgID string, config CollectionConfig) (*CollectionConfig, error) {
	criteriaJSON, err := json.Marshal(config.AcceptanceCriteria)
	if err != nil {
		return nil, fmt.Errorf("marshal criteria: %w", err)
	}

	failureThreshold := config.FailureThreshold
	if failureThreshold <= 0 {
		failureThreshold = 3
	}

	err = database.QuerierFromContext(ctx, ec.pool).QueryRow(ctx, `
		INSERT INTO evidence_collection_configs (
			organization_id, control_implementation_id, name, collection_method,
			schedule_cron, acceptance_criteria, failure_threshold, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		orgID, config.ControlImplementationID, config.Name, config.CollectionMethod,
		config.ScheduleCron, criteriaJSON, failureThreshold, true,
	).Scan(&config.ID)
	if err != nil {
		return nil, fmt.Errorf("insert collection config: %w", err)
	}

	config.OrgID = orgID
	config.IsActive = true
	config.FailureThreshold = failureThreshold
	config.ConsecutiveFailures = 0

	log.Info().Str("config_id", config.ID).Str("name", config.Name).Msg("evidence collection config created")
	return &config, nil
}

// UpdateConfig updates an existing evidence collection config.
func (ec *EvidenceCollector) UpdateConfig(ctx context.Context, orgID, configID string, config CollectionConfig) error {
	criteriaJSON, err := json.Marshal(config.AcceptanceCriteria)
	if err != nil {
		return fmt.Errorf("marshal criteria: %w", err)
	}

	tag, err := database.QuerierFromContext(ctx, ec.pool).Exec(ctx, `
		UPDATE evidence_collection_configs
		SET name = $1, collection_method = $2, schedule_cron = $3,
		    acceptance_criteria = $4, failure_threshold = $5, is_active = $6
		WHERE id = $7 AND organization_id = $8`,
		config.Name, config.CollectionMethod, config.ScheduleCron,
		criteriaJSON, config.FailureThreshold, config.IsActive,
		configID, orgID)
	if err != nil {
		return fmt.Errorf("update collection config: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("collection config not found")
	}

	log.Info().Str("config_id", configID).Msg("evidence collection config updated")
	return nil
}

// GetRunHistory returns a paginated list of collection runs for a config.
func (ec *EvidenceCollector) GetRunHistory(ctx context.Context, orgID, configID string, page, pageSize int) ([]CollectionRun, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	querier := database.QuerierFromContext(ctx, ec.pool)
	var total int
	err := querier.QueryRow(ctx, `
		SELECT COUNT(*) FROM evidence_collection_runs
		WHERE config_id = $1 AND organization_id = $2`, configID, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count collection runs: %w", err)
	}

	rows, err := querier.Query(ctx, `
		SELECT id, config_id, status, started_at, completed_at, duration_ms,
		       collected_data, validation_results, all_criteria_passed, error_message
		FROM evidence_collection_runs
		WHERE config_id = $1 AND organization_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`, configID, orgID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list collection runs: %w", err)
	}
	defer rows.Close()

	var runs []CollectionRun
	for rows.Next() {
		var r CollectionRun
		var startedAt, completedAt *time.Time
		var collectedJSON, validationJSON []byte
		var allPassed *bool
		if err := rows.Scan(&r.ID, &r.ConfigID, &r.Status, &startedAt, &completedAt,
			&r.DurationMs, &collectedJSON, &validationJSON, &allPassed, &r.ErrorMessage); err != nil {
			return nil, 0, fmt.Errorf("scan collection run: %w", err)
		}
		if startedAt != nil {
			sa := startedAt.Format(time.RFC3339)
			r.StartedAt = &sa
		}
		if completedAt != nil {
			ca := completedAt.Format(time.RFC3339)
			r.CompletedAt = &ca
		}
		if collectedJSON != nil {
			_ = json.Unmarshal(collectedJSON, &r.CollectedData)
		}
		if validationJSON != nil {
			_ = json.Unmarshal(validationJSON, &r.ValidationResults)
		}
		if allPassed != nil {
			r.AllCriteriaPassed = *allPassed
		}
		runs = append(runs, r)
	}

	return runs, total, nil
}

// toFloat64 converts an interface{} to float64 for numeric comparisons.
func toFloat64(v interface{}) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case json.Number:
		return n.Float64()
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// contains checks if s contains substr (simple string containment).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (substr == "" || searchString(s, substr))
}

// searchString performs a naive substring search.
func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
