package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// EvidenceCollector handles automated evidence collection and validation.
type EvidenceCollector struct {
	pool *pgxpool.Pool
	bus  *EventBus
}

// CollectionConfig defines how evidence is collected for a control.
type CollectionConfig struct {
	ID                      string              `json:"id"`
	OrgID                   string              `json:"organization_id"`
	ControlImplementationID string              `json:"control_implementation_id"`
	Name                    string              `json:"name"`
	CollectionMethod        string              `json:"collection_method"`
	ScheduleCron            string              `json:"schedule_cron"`
	AcceptanceCriteria      []AcceptanceCriterion `json:"acceptance_criteria"`
	IsActive                bool                `json:"is_active"`
	ConsecutiveFailures     int                 `json:"consecutive_failures"`
	FailureThreshold        int                 `json:"failure_threshold"`
	LastCollectionAt        *string             `json:"last_collection_at"`
	LastCollectionStatus    string              `json:"last_collection_status"`
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

// NewEvidenceCollector creates a new EvidenceCollector.
func NewEvidenceCollector(pool *pgxpool.Pool, bus *EventBus) *EvidenceCollector {
	return &EvidenceCollector{pool: pool, bus: bus}
}

// RunCollection executes an evidence collection for a given config, validates
// results, and creates an evidence record if all criteria pass.
func (ec *EvidenceCollector) RunCollection(ctx context.Context, configID string) (*CollectionRun, error) {
	startTime := time.Now()

	// Fetch the config.
	var orgID, controlImplID, name, method string
	var criteriaJSON []byte
	var failureThreshold int

	err := ec.pool.QueryRow(ctx, `
		SELECT organization_id, control_implementation_id, name, collection_method,
		       acceptance_criteria, failure_threshold
		FROM evidence_collection_configs
		WHERE id = $1 AND is_active = true`, configID,
	).Scan(&orgID, &controlImplID, &name, &method, &criteriaJSON, &failureThreshold)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("collection config not found or inactive")
		}
		return nil, fmt.Errorf("fetch collection config: %w", err)
	}

	var criteria []AcceptanceCriterion
	if err := json.Unmarshal(criteriaJSON, &criteria); err != nil {
		criteria = nil
	}

	// Create the collection run record.
	var runID string
	startedAtStr := startTime.Format(time.RFC3339)
	err = ec.pool.QueryRow(ctx, `
		INSERT INTO evidence_collection_runs (
			organization_id, config_id, control_implementation_id,
			status, started_at
		) VALUES ($1, $2, $3, 'running', $4)
		RETURNING id`,
		orgID, configID, controlImplID, startTime,
	).Scan(&runID)
	if err != nil {
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
	validationJSON, _ := json.Marshal(validationResults)
	collectedJSON, _ := json.Marshal(collectedData)

	// Update the run record.
	_, err = ec.pool.Exec(ctx, `
		UPDATE evidence_collection_runs
		SET status = $1, completed_at = $2, duration_ms = $3,
		    collected_data = $4, validation_results = $5, all_criteria_passed = $6
		WHERE id = $7`,
		runStatus, completedAt, durationMs,
		collectedJSON, validationJSON, allPassed, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("failed to update collection run")
	}

	// Update the config's last collection status.
	if allPassed || len(criteria) == 0 {
		_, _ = ec.pool.Exec(ctx, `
			UPDATE evidence_collection_configs
			SET last_collection_at = $1, last_collection_status = $2, consecutive_failures = 0
			WHERE id = $3`,
			completedAt, runStatus, configID)

		// Create an evidence record for the control implementation.
		_, _ = ec.pool.Exec(ctx, `
			INSERT INTO control_evidence (
				organization_id, control_implementation_id,
				title, description, evidence_type, collection_method,
				collected_at, is_current, review_status
			) VALUES ($1, $2, $3, $4, 'report', 'automated', $5, true, 'pending')`,
			orgID, controlImplID,
			fmt.Sprintf("Automated evidence: %s", name),
			fmt.Sprintf("Collected via %s method. All %d acceptance criteria passed.", method, len(criteria)),
			completedAt)
	} else {
		// Increment failure counter.
		var newFailures int
		_ = ec.pool.QueryRow(ctx, `
			UPDATE evidence_collection_configs
			SET last_collection_at = $1, last_collection_status = $2,
			    consecutive_failures = consecutive_failures + 1
			WHERE id = $3
			RETURNING consecutive_failures`,
			completedAt, runStatus, configID).Scan(&newFailures)

		// If failure threshold reached, emit alert.
		if newFailures >= failureThreshold {
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
	rows, err := ec.pool.Query(ctx, `
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

	err = ec.pool.QueryRow(ctx, `
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

	tag, err := ec.pool.Exec(ctx, `
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

	var total int
	err := ec.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM evidence_collection_runs
		WHERE config_id = $1 AND organization_id = $2`, configID, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count collection runs: %w", err)
	}

	rows, err := ec.pool.Query(ctx, `
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
