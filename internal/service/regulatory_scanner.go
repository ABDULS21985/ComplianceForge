package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// RegulatorySource represents a monitored regulatory feed.
type RegulatorySource struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	SourceType      string  `json:"source_type"` // rss, api, manual
	URL             string  `json:"url"`
	Jurisdiction    string  `json:"jurisdiction"`
	IsActive        bool    `json:"is_active"`
	ScanFreqMinutes int     `json:"scan_frequency_minutes"`
	LastScannedAt   *string `json:"last_scanned_at"`
	LastError       *string `json:"last_error"`
	CreatedAt       string  `json:"created_at"`
}

// RegulatoryChange represents a detected regulatory change.
type RegulatoryChange struct {
	ID                string   `json:"id"`
	SourceID          string   `json:"source_id"`
	ExternalID        string   `json:"external_id"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary"`
	URL               string   `json:"url"`
	PublishedAt       string   `json:"published_at"`
	Jurisdiction      string   `json:"jurisdiction"`
	ChangeType        string   `json:"change_type"` // new_regulation, amendment, guidance, enforcement
	Severity          string   `json:"severity"`    // critical, high, medium, low, info
	Status            string   `json:"status"`      // new, classified, assessed, acknowledged, closed
	AffectedFrameworks []string `json:"affected_frameworks"`
	ContentHash       string   `json:"content_hash"`
	CreatedAt         string   `json:"created_at"`
}

// RegulatoryChangeDetail is the full detail for a change including assessments.
type RegulatoryChangeDetail struct {
	RegulatoryChange
	SourceName   string             `json:"source_name"`
	FullContent  string             `json:"full_content"`
	AIClassification *string        `json:"ai_classification"`
	Assessments  []ImpactAssessment `json:"assessments"`
}

// ImpactAssessment records the per-org impact of a regulatory change.
type ImpactAssessment struct {
	ID                 string   `json:"id"`
	ChangeID           string   `json:"change_id"`
	OrgID              string   `json:"organization_id"`
	ImpactLevel        string   `json:"impact_level"` // critical, high, medium, low, none
	AffectedControls   []string `json:"affected_controls"`
	AffectedPolicies   []string `json:"affected_policies"`
	RequiredActions     string   `json:"required_actions"`
	Deadline           *string  `json:"deadline"`
	AssessedBy         string   `json:"assessed_by"` // ai, manual
	Status             string   `json:"status"`      // pending, reviewed, actioned
	CreatedAt          string   `json:"created_at"`
}

// RegulatoryDashboard provides an overview of regulatory changes for an org.
type RegulatoryDashboard struct {
	TotalChanges       int                      `json:"total_changes"`
	NewChanges         int                      `json:"new_changes"`
	CriticalChanges    int                      `json:"critical_changes"`
	HighChanges        int                      `json:"high_changes"`
	PendingAssessments int                      `json:"pending_assessments"`
	RecentChanges      []RegulatoryChange       `json:"recent_changes"`
	ByJurisdiction     map[string]int           `json:"by_jurisdiction"`
	ByChangeType       map[string]int           `json:"by_change_type"`
	Timeline           []map[string]interface{} `json:"timeline"`
}

// rssItem represents a single item in an RSS feed.
type rssFeed struct {
	XMLName xml.Name  `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Items []rssItem `xml:"item"`
}
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// RegulatoryScanner monitors regulatory sources for changes.
type RegulatoryScanner struct {
	pool      *pgxpool.Pool
	bus       *EventBus
	aiService *AIService
	client    *http.Client
}

// NewRegulatoryScanner creates a new RegulatoryScanner.
func NewRegulatoryScanner(pool *pgxpool.Pool, bus *EventBus, aiService *AIService) *RegulatoryScanner {
	return &RegulatoryScanner{
		pool:      pool,
		bus:       bus,
		aiService: aiService,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// ScanAllSources iterates all active regulatory sources and scans them.
func (rs *RegulatoryScanner) ScanAllSources(ctx context.Context) error {
	rows, err := rs.pool.Query(ctx, `
		SELECT id, name, source_type, url, jurisdiction, scan_frequency_minutes
		FROM regulatory_sources
		WHERE is_active = true
		  AND (last_scanned_at IS NULL OR last_scanned_at + (scan_frequency_minutes || ' minutes')::interval < NOW())
		ORDER BY last_scanned_at ASC NULLS FIRST
	`)
	if err != nil {
		return fmt.Errorf("querying active sources: %w", err)
	}
	defer rows.Close()

	type sourceRecord struct {
		id           string
		name         string
		sourceType   string
		url          string
		jurisdiction string
		scanFreq     int
	}
	var sources []sourceRecord
	for rows.Next() {
		var s sourceRecord
		if err := rows.Scan(&s.id, &s.name, &s.sourceType, &s.url, &s.jurisdiction, &s.scanFreq); err != nil {
			return fmt.Errorf("scanning source: %w", err)
		}
		sources = append(sources, s)
	}

	for _, src := range sources {
		var scanErr error
		switch src.sourceType {
		case "rss":
			srcObj := RegulatorySource{
				ID:           src.id,
				Name:         src.name,
				SourceType:   src.sourceType,
				URL:          src.url,
				Jurisdiction: src.jurisdiction,
			}
			_, scanErr = rs.ScanRSSFeed(ctx, srcObj)
		default:
			log.Warn().Str("type", src.sourceType).Str("source", src.name).Msg("regulatory_scanner: unsupported source type")
			continue
		}

		if scanErr != nil {
			log.Error().Err(scanErr).Str("source", src.name).Msg("regulatory_scanner: scan failed")
			_, _ = rs.pool.Exec(ctx, `
				UPDATE regulatory_sources
				SET last_scanned_at = NOW(), last_error = $2
				WHERE id = $1
			`, src.id, scanErr.Error())
		} else {
			_, _ = rs.pool.Exec(ctx, `
				UPDATE regulatory_sources
				SET last_scanned_at = NOW(), last_error = NULL
				WHERE id = $1
			`, src.id)
		}
	}

	log.Info().Int("sources_scanned", len(sources)).Msg("regulatory_scanner: scan cycle complete")
	return nil
}

// ScanRSSFeed fetches an RSS feed, parses entries, deduplicates, and creates
// regulatory change records.
func (rs *RegulatoryScanner) ScanRSSFeed(ctx context.Context, source RegulatorySource) ([]RegulatoryChange, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := rs.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching RSS feed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading feed body: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parsing RSS XML: %w", err)
	}

	var changes []RegulatoryChange
	for _, item := range feed.Channel.Items {
		// Compute content hash for deduplication.
		hash := sha256.Sum256([]byte(item.Title + item.Link + item.Description))
		contentHash := hex.EncodeToString(hash[:])

		// Check if we already have this change.
		var existingID *string
		_ = rs.pool.QueryRow(ctx, `
			SELECT id FROM regulatory_changes
			WHERE content_hash = $1 OR (source_id = $2 AND external_id = $3)
		`, contentHash, source.ID, item.GUID).Scan(&existingID)
		if existingID != nil {
			continue
		}

		externalID := item.GUID
		if externalID == "" {
			externalID = contentHash[:16]
		}

		publishedAt := time.Now().UTC()
		if item.PubDate != "" {
			if parsed, pErr := time.Parse(time.RFC1123Z, item.PubDate); pErr == nil {
				publishedAt = parsed
			} else if parsed, pErr := time.Parse(time.RFC1123, item.PubDate); pErr == nil {
				publishedAt = parsed
			}
		}

		var changeID string
		err := rs.pool.QueryRow(ctx, `
			INSERT INTO regulatory_changes (
				source_id, external_id, title, summary, url,
				published_at, jurisdiction, change_type, severity, status,
				full_content, content_hash, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, 'new_regulation', 'medium', 'new', $8, $9, NOW())
			RETURNING id
		`, source.ID, externalID, item.Title, item.Description, item.Link,
			publishedAt, source.Jurisdiction, item.Description, contentHash,
		).Scan(&changeID)
		if err != nil {
			log.Warn().Err(err).Str("title", item.Title).Msg("regulatory_scanner: failed to insert change")
			continue
		}

		change := RegulatoryChange{
			ID:           changeID,
			SourceID:     source.ID,
			ExternalID:   externalID,
			Title:        item.Title,
			Summary:      item.Description,
			URL:          item.Link,
			PublishedAt:  publishedAt.Format(time.RFC3339),
			Jurisdiction: source.Jurisdiction,
			ChangeType:   "new_regulation",
			Severity:     "medium",
			Status:       "new",
			ContentHash:  contentHash,
		}
		changes = append(changes, change)

		if rs.bus != nil {
			rs.bus.Publish(Event{
				Type:       "regulatory.change_detected",
				Severity:   "medium",
				OrgID:      "",
				EntityType: "regulatory_change",
				EntityID:   changeID,
				EntityRef:  item.Title,
				Data:       map[string]interface{}{"jurisdiction": source.Jurisdiction, "url": item.Link},
				Timestamp:  time.Now().UTC(),
			})
		}
	}

	log.Info().Str("source", source.Name).Int("new_changes", len(changes)).Msg("regulatory_scanner: RSS scan complete")
	return changes, nil
}

// ClassifyChange uses AI to classify the severity and affected frameworks of a change.
func (rs *RegulatoryScanner) ClassifyChange(ctx context.Context, change RegulatoryChange) error {
	if rs.aiService == nil || rs.aiService.apiKey == "" {
		// Without AI, mark as classified with defaults.
		_, err := rs.pool.Exec(ctx, `
			UPDATE regulatory_changes SET status = 'classified' WHERE id = $1
		`, change.ID)
		return err
	}

	prompt := fmt.Sprintf(`Classify this regulatory change:

Title: %s
Summary: %s
Jurisdiction: %s

Respond in JSON with:
- severity: critical/high/medium/low/info
- change_type: new_regulation/amendment/guidance/enforcement
- affected_frameworks: array of framework codes (e.g., ["GDPR", "ISO27001", "NIS2"])
- key_impacts: brief description of key impacts`, change.Title, change.Summary, change.Jurisdiction)

	response, _, _, _, err := rs.aiService.callClaude(ctx, "", "regulatory_classification", prompt, "")
	if err != nil {
		log.Warn().Err(err).Str("change_id", change.ID).Msg("regulatory_scanner: AI classification failed")
		return nil
	}

	// Parse AI response for classification data.
	var classification struct {
		Severity           string   `json:"severity"`
		ChangeType         string   `json:"change_type"`
		AffectedFrameworks []string `json:"affected_frameworks"`
		KeyImpacts         string   `json:"key_impacts"`
	}
	if err := json.Unmarshal([]byte(response), &classification); err != nil {
		// Store raw response even if not parseable JSON.
		_, _ = rs.pool.Exec(ctx, `
			UPDATE regulatory_changes
			SET status = 'classified', ai_classification = $2
			WHERE id = $1
		`, change.ID, response)
		return nil
	}

	fwJSON, _ := json.Marshal(classification.AffectedFrameworks)
	_, err = rs.pool.Exec(ctx, `
		UPDATE regulatory_changes
		SET severity = $2, change_type = $3, affected_frameworks = $4,
		    ai_classification = $5, status = 'classified'
		WHERE id = $1
	`, change.ID, classification.Severity, classification.ChangeType, fwJSON, response)
	if err != nil {
		return fmt.Errorf("updating classification: %w", err)
	}

	log.Info().Str("change_id", change.ID).Str("severity", classification.Severity).Msg("regulatory_scanner: change classified")
	return nil
}

// AssessImpact evaluates the impact of a regulatory change on a specific organisation.
func (rs *RegulatoryScanner) AssessImpact(ctx context.Context, orgID, changeID string) (*ImpactAssessment, error) {
	// Fetch the change.
	var title, summary, jurisdiction string
	var fwJSON []byte
	err := rs.pool.QueryRow(ctx, `
		SELECT title, summary, jurisdiction, affected_frameworks
		FROM regulatory_changes WHERE id = $1
	`, changeID).Scan(&title, &summary, &jurisdiction, &fwJSON)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("change not found")
		}
		return nil, fmt.Errorf("querying change: %w", err)
	}

	var frameworks []string
	if fwJSON != nil {
		_ = json.Unmarshal(fwJSON, &frameworks)
	}

	// Find org's relevant controls.
	var affectedControls []string
	if len(frameworks) > 0 {
		cRows, err := rs.pool.Query(ctx, `
			SELECT DISTINCT ci.control_code
			FROM control_implementations ci
			JOIN compliance_frameworks cf ON ci.framework_id = cf.id
			WHERE ci.organization_id = $1 AND cf.code = ANY($2)
			LIMIT 50
		`, orgID, frameworks)
		if err == nil {
			defer cRows.Close()
			for cRows.Next() {
				var code string
				if err := cRows.Scan(&code); err == nil {
					affectedControls = append(affectedControls, code)
				}
			}
		}
	}

	// Find affected policies.
	var affectedPolicies []string
	pRows, err := rs.pool.Query(ctx, `
		SELECT DISTINCT title FROM policies
		WHERE organization_id = $1 AND status = 'approved'
		LIMIT 20
	`, orgID)
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var t string
			if err := pRows.Scan(&t); err == nil {
				affectedPolicies = append(affectedPolicies, t)
			}
		}
	}

	// Determine impact level.
	impactLevel := "low"
	assessedBy := "rule_based"
	requiredActions := fmt.Sprintf("Review regulatory change '%s' and assess applicability to your organisation.", title)

	if len(affectedControls) > 10 {
		impactLevel = "high"
	} else if len(affectedControls) > 5 {
		impactLevel = "medium"
	}

	// Try AI assessment if available.
	if rs.aiService != nil && rs.aiService.apiKey != "" {
		prompt := fmt.Sprintf(`Assess the impact of this regulatory change on an organisation:

Change: %s
Summary: %s
Jurisdiction: %s
Affected frameworks: %v
Organisation's affected controls count: %d

Respond in JSON:
- impact_level: critical/high/medium/low/none
- required_actions: brief description of required actions
- deadline_recommendation: ISO date or null`, title, summary, jurisdiction, frameworks, len(affectedControls))

		aiResp, _, _, _, aiErr := rs.aiService.callClaude(ctx, orgID, "impact_assessment", prompt, "")
		if aiErr == nil {
			assessedBy = "ai"
			var aiResult struct {
				ImpactLevel    string  `json:"impact_level"`
				RequiredActions string  `json:"required_actions"`
				Deadline       *string `json:"deadline_recommendation"`
			}
			if json.Unmarshal([]byte(aiResp), &aiResult) == nil {
				if aiResult.ImpactLevel != "" {
					impactLevel = aiResult.ImpactLevel
				}
				if aiResult.RequiredActions != "" {
					requiredActions = aiResult.RequiredActions
				}
			}
		}
	}

	controlsJSON, _ := json.Marshal(affectedControls)
	policiesJSON, _ := json.Marshal(affectedPolicies)

	var assessment ImpactAssessment
	err = rs.pool.QueryRow(ctx, `
		INSERT INTO regulatory_impact_assessments (
			change_id, organization_id, impact_level,
			affected_controls, affected_policies,
			required_actions, assessed_by, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', NOW())
		RETURNING id, TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	`, changeID, orgID, impactLevel, controlsJSON, policiesJSON, requiredActions, assessedBy).Scan(
		&assessment.ID, &assessment.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting assessment: %w", err)
	}

	assessment.ChangeID = changeID
	assessment.OrgID = orgID
	assessment.ImpactLevel = impactLevel
	assessment.AffectedControls = affectedControls
	assessment.AffectedPolicies = affectedPolicies
	assessment.RequiredActions = requiredActions
	assessment.AssessedBy = assessedBy
	assessment.Status = "pending"

	// Update the change status.
	_, _ = rs.pool.Exec(ctx, `
		UPDATE regulatory_changes SET status = 'assessed' WHERE id = $1 AND status != 'closed'
	`, changeID)

	if rs.bus != nil {
		rs.bus.Publish(Event{
			Type:       "regulatory.impact_assessed",
			Severity:   impactLevel,
			OrgID:      orgID,
			EntityType: "regulatory_change",
			EntityID:   changeID,
			Data:       map[string]interface{}{"impact_level": impactLevel, "affected_controls": len(affectedControls)},
			Timestamp:  time.Now().UTC(),
		})
	}

	log.Info().Str("change_id", changeID).Str("org_id", orgID).Str("impact", impactLevel).Msg("regulatory_scanner: impact assessed")
	return &assessment, nil
}

// ListChanges returns paginated regulatory changes with optional filters.
func (rs *RegulatoryScanner) ListChanges(ctx context.Context, filters map[string]interface{}, page, pageSize int) ([]RegulatoryChange, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if severity, ok := filters["severity"].(string); ok && severity != "" {
		where += fmt.Sprintf(" AND rc.severity = $%d", argIdx)
		args = append(args, severity)
		argIdx++
	}
	if status, ok := filters["status"].(string); ok && status != "" {
		where += fmt.Sprintf(" AND rc.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if jurisdiction, ok := filters["jurisdiction"].(string); ok && jurisdiction != "" {
		where += fmt.Sprintf(" AND rc.jurisdiction = $%d", argIdx)
		args = append(args, jurisdiction)
		argIdx++
	}
	if changeType, ok := filters["change_type"].(string); ok && changeType != "" {
		where += fmt.Sprintf(" AND rc.change_type = $%d", argIdx)
		args = append(args, changeType)
		argIdx++
	}

	var total int
	countQ := fmt.Sprintf("SELECT COUNT(*) FROM regulatory_changes rc %s", where)
	err := rs.pool.QueryRow(ctx, countQ, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting changes: %w", err)
	}

	listArgs := append(args, pageSize, offset)
	listQ := fmt.Sprintf(`
		SELECT rc.id, rc.source_id, rc.external_id, rc.title, rc.summary, rc.url,
			TO_CHAR(rc.published_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			rc.jurisdiction, rc.change_type, rc.severity, rc.status,
			rc.affected_frameworks, rc.content_hash,
			TO_CHAR(rc.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM regulatory_changes rc
		%s
		ORDER BY rc.published_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)

	rows, err := rs.pool.Query(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying changes: %w", err)
	}
	defer rows.Close()

	var changes []RegulatoryChange
	for rows.Next() {
		var c RegulatoryChange
		var fwJSON []byte
		if err := rows.Scan(
			&c.ID, &c.SourceID, &c.ExternalID, &c.Title, &c.Summary, &c.URL,
			&c.PublishedAt, &c.Jurisdiction, &c.ChangeType, &c.Severity, &c.Status,
			&fwJSON, &c.ContentHash, &c.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning change: %w", err)
		}
		if fwJSON != nil {
			_ = json.Unmarshal(fwJSON, &c.AffectedFrameworks)
		}
		changes = append(changes, c)
	}

	return changes, total, nil
}

// GetChange returns the full detail for a single regulatory change.
func (rs *RegulatoryScanner) GetChange(ctx context.Context, changeID string) (*RegulatoryChangeDetail, error) {
	var detail RegulatoryChangeDetail
	var fwJSON []byte
	var fullContentPtr *string

	err := rs.pool.QueryRow(ctx, `
		SELECT rc.id, rc.source_id, rc.external_id, rc.title, rc.summary, rc.url,
			TO_CHAR(rc.published_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			rc.jurisdiction, rc.change_type, rc.severity, rc.status,
			rc.affected_frameworks, rc.content_hash,
			TO_CHAR(rc.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			rs.name, rc.full_content, rc.ai_classification
		FROM regulatory_changes rc
		JOIN regulatory_sources rs ON rs.id = rc.source_id
		WHERE rc.id = $1
	`, changeID).Scan(
		&detail.ID, &detail.SourceID, &detail.ExternalID, &detail.Title,
		&detail.Summary, &detail.URL, &detail.PublishedAt,
		&detail.Jurisdiction, &detail.ChangeType, &detail.Severity, &detail.Status,
		&fwJSON, &detail.ContentHash, &detail.CreatedAt,
		&detail.SourceName, &fullContentPtr, &detail.AIClassification,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("change not found")
		}
		return nil, fmt.Errorf("querying change: %w", err)
	}
	if fwJSON != nil {
		_ = json.Unmarshal(fwJSON, &detail.AffectedFrameworks)
	}
	if fullContentPtr != nil {
		detail.FullContent = *fullContentPtr
	}

	// Fetch assessments.
	aRows, err := rs.pool.Query(ctx, `
		SELECT ia.id, ia.change_id, ia.organization_id, ia.impact_level,
			ia.affected_controls, ia.affected_policies,
			ia.required_actions, ia.deadline, ia.assessed_by, ia.status,
			TO_CHAR(ia.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM regulatory_impact_assessments ia
		WHERE ia.change_id = $1
		ORDER BY ia.created_at DESC
	`, changeID)
	if err != nil {
		return nil, fmt.Errorf("querying assessments: %w", err)
	}
	defer aRows.Close()

	for aRows.Next() {
		var a ImpactAssessment
		var controlsJSON, policiesJSON []byte
		if err := aRows.Scan(
			&a.ID, &a.ChangeID, &a.OrgID, &a.ImpactLevel,
			&controlsJSON, &policiesJSON,
			&a.RequiredActions, &a.Deadline, &a.AssessedBy, &a.Status,
			&a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning assessment: %w", err)
		}
		if controlsJSON != nil {
			_ = json.Unmarshal(controlsJSON, &a.AffectedControls)
		}
		if policiesJSON != nil {
			_ = json.Unmarshal(policiesJSON, &a.AffectedPolicies)
		}
		detail.Assessments = append(detail.Assessments, a)
	}

	return &detail, nil
}

// ListSources returns all configured regulatory sources.
func (rs *RegulatoryScanner) ListSources(ctx context.Context) ([]RegulatorySource, error) {
	rows, err := rs.pool.Query(ctx, `
		SELECT id, name, source_type, url, jurisdiction, is_active,
			scan_frequency_minutes,
			CASE WHEN last_scanned_at IS NOT NULL THEN TO_CHAR(last_scanned_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') END,
			last_error,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM regulatory_sources
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("querying sources: %w", err)
	}
	defer rows.Close()

	var sources []RegulatorySource
	for rows.Next() {
		var s RegulatorySource
		if err := rows.Scan(&s.ID, &s.Name, &s.SourceType, &s.URL, &s.Jurisdiction,
			&s.IsActive, &s.ScanFreqMinutes, &s.LastScannedAt, &s.LastError, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning source: %w", err)
		}
		sources = append(sources, s)
	}

	return sources, nil
}

// CreateSource adds a new regulatory monitoring source.
func (rs *RegulatoryScanner) CreateSource(ctx context.Context, source RegulatorySource) (*RegulatorySource, error) {
	if source.ScanFreqMinutes < 1 {
		source.ScanFreqMinutes = 60
	}

	err := rs.pool.QueryRow(ctx, `
		INSERT INTO regulatory_sources (
			name, source_type, url, jurisdiction, is_active,
			scan_frequency_minutes, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id, TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	`, source.Name, source.SourceType, source.URL, source.Jurisdiction,
		source.IsActive, source.ScanFreqMinutes,
	).Scan(&source.ID, &source.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating source: %w", err)
	}

	log.Info().Str("source_id", source.ID).Str("name", source.Name).Msg("regulatory_scanner: source created")
	return &source, nil
}

// SubscribeToSource subscribes an organisation to receive impact assessments
// from a specific regulatory source.
func (rs *RegulatoryScanner) SubscribeToSource(ctx context.Context, orgID, sourceID string) error {
	_, err := rs.pool.Exec(ctx, `
		INSERT INTO regulatory_subscriptions (
			organization_id, source_id, is_active, created_at
		) VALUES ($1, $2, true, NOW())
		ON CONFLICT (organization_id, source_id) DO UPDATE
		SET is_active = true, updated_at = NOW()
	`, orgID, sourceID)
	if err != nil {
		return fmt.Errorf("subscribing to source: %w", err)
	}

	log.Info().Str("org_id", orgID).Str("source_id", sourceID).Msg("regulatory_scanner: subscribed")
	return nil
}

// GetDashboard returns an overview of regulatory changes relevant to an organisation.
func (rs *RegulatoryScanner) GetDashboard(ctx context.Context, orgID string) (*RegulatoryDashboard, error) {
	dash := &RegulatoryDashboard{
		ByJurisdiction: make(map[string]int),
		ByChangeType:   make(map[string]int),
	}

	// Overall counts from subscribed sources.
	err := rs.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE rc.status = 'new')::int,
			COUNT(*) FILTER (WHERE rc.severity = 'critical')::int,
			COUNT(*) FILTER (WHERE rc.severity = 'high')::int
		FROM regulatory_changes rc
		JOIN regulatory_sources rsrc ON rsrc.id = rc.source_id
		JOIN regulatory_subscriptions sub ON sub.source_id = rsrc.id AND sub.organization_id = $1
		WHERE sub.is_active = true
	`, orgID).Scan(&dash.TotalChanges, &dash.NewChanges, &dash.CriticalChanges, &dash.HighChanges)
	if err != nil {
		return nil, fmt.Errorf("querying dashboard counts: %w", err)
	}

	// Pending assessments.
	_ = rs.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM regulatory_impact_assessments
		WHERE organization_id = $1 AND status = 'pending'
	`, orgID).Scan(&dash.PendingAssessments)

	// By jurisdiction.
	jRows, err := rs.pool.Query(ctx, `
		SELECT rc.jurisdiction, COUNT(*)::int
		FROM regulatory_changes rc
		JOIN regulatory_sources rsrc ON rsrc.id = rc.source_id
		JOIN regulatory_subscriptions sub ON sub.source_id = rsrc.id AND sub.organization_id = $1
		WHERE sub.is_active = true
		GROUP BY rc.jurisdiction
	`, orgID)
	if err == nil {
		defer jRows.Close()
		for jRows.Next() {
			var j string
			var c int
			if err := jRows.Scan(&j, &c); err == nil {
				dash.ByJurisdiction[j] = c
			}
		}
	}

	// By change type.
	tRows, err := rs.pool.Query(ctx, `
		SELECT rc.change_type, COUNT(*)::int
		FROM regulatory_changes rc
		JOIN regulatory_sources rsrc ON rsrc.id = rc.source_id
		JOIN regulatory_subscriptions sub ON sub.source_id = rsrc.id AND sub.organization_id = $1
		WHERE sub.is_active = true
		GROUP BY rc.change_type
	`, orgID)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var t string
			var c int
			if err := tRows.Scan(&t, &c); err == nil {
				dash.ByChangeType[t] = c
			}
		}
	}

	// Recent changes (last 10).
	rRows, err := rs.pool.Query(ctx, `
		SELECT rc.id, rc.source_id, rc.external_id, rc.title, rc.summary, rc.url,
			TO_CHAR(rc.published_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			rc.jurisdiction, rc.change_type, rc.severity, rc.status,
			rc.affected_frameworks, rc.content_hash,
			TO_CHAR(rc.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM regulatory_changes rc
		JOIN regulatory_sources rsrc ON rsrc.id = rc.source_id
		JOIN regulatory_subscriptions sub ON sub.source_id = rsrc.id AND sub.organization_id = $1
		WHERE sub.is_active = true
		ORDER BY rc.published_at DESC
		LIMIT 10
	`, orgID)
	if err == nil {
		defer rRows.Close()
		for rRows.Next() {
			var c RegulatoryChange
			var fwJSON []byte
			if err := rRows.Scan(
				&c.ID, &c.SourceID, &c.ExternalID, &c.Title, &c.Summary, &c.URL,
				&c.PublishedAt, &c.Jurisdiction, &c.ChangeType, &c.Severity, &c.Status,
				&fwJSON, &c.ContentHash, &c.CreatedAt,
			); err == nil {
				if fwJSON != nil {
					_ = json.Unmarshal(fwJSON, &c.AffectedFrameworks)
				}
				dash.RecentChanges = append(dash.RecentChanges, c)
			}
		}
	}

	// Monthly timeline for last 6 months.
	tmRows, err := rs.pool.Query(ctx, `
		SELECT TO_CHAR(rc.published_at, 'YYYY-MM') AS month, COUNT(*)::int
		FROM regulatory_changes rc
		JOIN regulatory_sources rsrc ON rsrc.id = rc.source_id
		JOIN regulatory_subscriptions sub ON sub.source_id = rsrc.id AND sub.organization_id = $1
		WHERE sub.is_active = true AND rc.published_at >= NOW() - INTERVAL '6 months'
		GROUP BY month
		ORDER BY month
	`, orgID)
	if err == nil {
		defer tmRows.Close()
		for tmRows.Next() {
			var month string
			var cnt int
			if err := tmRows.Scan(&month, &cnt); err == nil {
				dash.Timeline = append(dash.Timeline, map[string]interface{}{
					"month": month,
					"count": cnt,
				})
			}
		}
	}

	return dash, nil
}
