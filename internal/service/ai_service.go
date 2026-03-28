package service

import (
	"bytes"
	"context"
	"encoding/json"
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

// GapInput describes a compliance gap to be remediated.
type GapInput struct {
	ControlCode  string `json:"control_code"`
	ControlTitle string `json:"control_title"`
	GapType      string `json:"gap_type"`
	Severity     string `json:"severity"`
	Description  string `json:"description"`
	Framework    string `json:"framework"`
}

// AIUsageStats aggregates token and cost metrics for an organisation.
type AIUsageStats struct {
	TotalInteractions int     `json:"total_interactions"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalCostEUR      float64 `json:"total_cost_eur"`
	AvgLatencyMs      float64 `json:"avg_latency_ms"`
	ByType            map[string]int `json:"by_type"`
}

// claudeRequest is the JSON body sent to the Anthropic Messages API.
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
}

// claudeMessage is a single message in the Anthropic Messages API.
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the JSON response from the Anthropic Messages API.
type claudeResponse struct {
	ID      string `json:"id"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// AIService wraps the Anthropic Claude API for compliance-focused AI tasks.
type AIService struct {
	pool      *pgxpool.Pool
	apiKey    string
	model     string
	maxTokens int
	client    *http.Client
}

// NewAIService creates a new AIService. If apiKey is empty, methods will return
// static fallback guidance instead of calling the Claude API.
func NewAIService(pool *pgxpool.Pool, apiKey, model string) *AIService {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &AIService{
		pool:      pool,
		apiKey:    apiKey,
		model:     model,
		maxTokens: 4096,
		client:    &http.Client{Timeout: 120 * time.Second},
	}
}

// GenerateRemediationPlan asks Claude to produce a structured remediation plan
// for the provided compliance gaps.
func (ai *AIService) GenerateRemediationPlan(ctx context.Context, orgID string, gaps []GapInput, frameworks []string, industry, orgSize string) (string, error) {
	if ai.apiKey == "" {
		return ai.fallbackRemediationPlan(gaps), nil
	}

	gapDesc := ""
	for i, g := range gaps {
		gapDesc += fmt.Sprintf("%d. [%s] %s — %s (severity: %s, framework: %s)\n", i+1, g.ControlCode, g.ControlTitle, g.Description, g.Severity, g.Framework)
	}

	prompt := fmt.Sprintf(`You are a compliance remediation expert. Generate a detailed, actionable remediation plan for the following compliance gaps.

Organisation context:
- Industry: %s
- Size: %s
- Applicable frameworks: %v

Gaps:
%s

For each gap provide:
1. Priority (critical/high/medium/low)
2. Recommended actions with estimated effort
3. Required evidence
4. Dependencies between actions
5. Timeline estimate

Output as structured JSON with keys: priority_actions, timeline_weeks, estimated_total_effort_hours, actions (array with: control_code, action, effort_hours, priority, dependencies, evidence_needed).`, industry, orgSize, frameworks, gapDesc)

	response, _, _, _, err := ai.callClaude(ctx, orgID, "remediation_plan", prompt, "")
	if err != nil {
		return "", fmt.Errorf("generating remediation plan: %w", err)
	}
	return response, nil
}

// GenerateControlGuidance asks Claude for implementation guidance on a specific control.
func (ai *AIService) GenerateControlGuidance(ctx context.Context, orgID, controlCode, controlTitle, frameworkCode, industry, orgSize string) (string, error) {
	if ai.apiKey == "" {
		return ai.fallbackControlGuidance(controlCode, controlTitle), nil
	}

	prompt := fmt.Sprintf(`You are a compliance implementation advisor. Provide practical guidance for implementing the following control.

Control: %s — %s
Framework: %s
Industry: %s
Organisation size: %s

Include:
1. What the control requires
2. Step-by-step implementation guide
3. Common evidence artefacts
4. Typical pitfalls and how to avoid them
5. Estimated effort for initial implementation
6. Ongoing maintenance requirements`, controlCode, controlTitle, frameworkCode, industry, orgSize)

	response, _, _, _, err := ai.callClaude(ctx, orgID, "control_guidance", prompt, "")
	if err != nil {
		return "", fmt.Errorf("generating control guidance: %w", err)
	}
	return response, nil
}

// SuggestEvidence asks Claude to suggest evidence artefacts for a control.
func (ai *AIService) SuggestEvidence(ctx context.Context, controlCode, controlTitle string) (string, error) {
	if ai.apiKey == "" {
		return ai.fallbackEvidenceSuggestion(controlCode, controlTitle), nil
	}

	prompt := fmt.Sprintf(`You are a compliance evidence specialist. Suggest specific evidence artefacts for the following control.

Control: %s — %s

List 5-10 evidence items with:
- Evidence name
- Description
- File type (document, screenshot, log, config, report)
- Collection frequency (one-time, monthly, quarterly, annual, continuous)
- Acceptance criteria`, controlCode, controlTitle)

	response, _, _, _, err := ai.callClaude(ctx, "", "evidence_suggestion", prompt, "")
	if err != nil {
		return "", fmt.Errorf("suggesting evidence: %w", err)
	}
	return response, nil
}

// DraftPolicySection asks Claude to draft a section of a compliance policy.
func (ai *AIService) DraftPolicySection(ctx context.Context, orgID, policyType, section, orgContext string) (string, error) {
	if ai.apiKey == "" {
		return ai.fallbackPolicyDraft(policyType, section), nil
	}

	prompt := fmt.Sprintf(`You are a compliance policy writer. Draft the following section for a %s policy.

Section: %s
Organisation context: %s

Write in clear, professional language suitable for a formal policy document. Include:
- Purpose of the section
- Specific requirements and obligations
- Roles and responsibilities where applicable
- References to relevant standards`, policyType, section, orgContext)

	response, _, _, _, err := ai.callClaude(ctx, orgID, "policy_draft", prompt, "")
	if err != nil {
		return "", fmt.Errorf("drafting policy section: %w", err)
	}
	return response, nil
}

// AssessRiskNarrative asks Claude to produce a risk assessment narrative.
func (ai *AIService) AssessRiskNarrative(ctx context.Context, orgID, riskTitle, riskDesc, orgContext string) (string, error) {
	if ai.apiKey == "" {
		return ai.fallbackRiskNarrative(riskTitle), nil
	}

	prompt := fmt.Sprintf(`You are a risk assessment specialist. Produce a detailed risk narrative for the following risk.

Risk title: %s
Risk description: %s
Organisation context: %s

Include:
1. Risk analysis (likelihood, impact, inherent risk level)
2. Potential business impacts
3. Existing control considerations
4. Recommended treatment options
5. Residual risk assessment
6. Monitoring recommendations`, riskTitle, riskDesc, orgContext)

	response, _, _, _, err := ai.callClaude(ctx, orgID, "risk_narrative", prompt, "")
	if err != nil {
		return "", fmt.Errorf("assessing risk narrative: %w", err)
	}
	return response, nil
}

// callClaude makes an HTTP POST to the Anthropic Messages API and returns the
// response text, input/output tokens, latency in ms, and any error.
func (ai *AIService) callClaude(ctx context.Context, orgID, interactionType, prompt string, userID string) (string, int, int, int, error) {
	reqBody := claudeRequest{
		Model:     ai.model,
		MaxTokens: ai.maxTokens,
		Messages: []claudeMessage{
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("marshalling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("creating HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", ai.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	startTime := time.Now()
	resp, err := ai.client.Do(req)
	latencyMs := int(time.Since(startTime).Milliseconds())
	if err != nil {
		return "", 0, 0, latencyMs, fmt.Errorf("calling Claude API: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, latencyMs, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("body", string(respBytes)).Msg("ai_service: Claude API error")
		return "", 0, 0, latencyMs, fmt.Errorf("Claude API returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBytes, &claudeResp); err != nil {
		return "", 0, 0, latencyMs, fmt.Errorf("unmarshalling Claude response: %w", err)
	}

	responseText := ""
	for _, c := range claudeResp.Content {
		if c.Type == "text" {
			responseText += c.Text
		}
	}

	inputTokens := claudeResp.Usage.InputTokens
	outputTokens := claudeResp.Usage.OutputTokens

	// Cost estimate: approximate EUR pricing per 1K tokens.
	costEUR := float64(inputTokens)*0.003/1000.0 + float64(outputTokens)*0.015/1000.0

	// Log the interaction asynchronously.
	go func() {
		bgCtx := context.Background()
		if logErr := ai.logInteraction(bgCtx, orgID, interactionType, prompt, responseText, ai.model, inputTokens, outputTokens, latencyMs, userID, costEUR); logErr != nil {
			log.Error().Err(logErr).Msg("ai_service: failed to log interaction")
		}
	}()

	return responseText, inputTokens, outputTokens, latencyMs, nil
}

// logInteraction inserts a record into ai_interaction_logs.
func (ai *AIService) logInteraction(ctx context.Context, orgID, interactionType, prompt, response, model string, inputTokens, outputTokens, latencyMs int, userID string, costEUR float64) error {
	var uid interface{} = nil
	if userID != "" {
		uid = userID
	}
	var oid interface{} = nil
	if orgID != "" {
		oid = orgID
	}

	_, err := ai.pool.Exec(ctx, `
		INSERT INTO ai_interaction_logs (
			organization_id, user_id, interaction_type, model,
			prompt_text, response_text,
			input_tokens, output_tokens, latency_ms,
			cost_eur, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
	`, oid, uid, interactionType, model, prompt, response, inputTokens, outputTokens, latencyMs, costEUR)
	if err != nil {
		return fmt.Errorf("inserting ai_interaction_log: %w", err)
	}
	return nil
}

// GetUsageStats returns aggregate AI usage statistics for an organisation.
func (ai *AIService) GetUsageStats(ctx context.Context, orgID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var total int
	var totalInput, totalOutput int64
	var totalCost float64
	var avgLatency float64
	err := ai.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COALESCE(SUM(input_tokens), 0)::bigint,
			COALESCE(SUM(output_tokens), 0)::bigint,
			COALESCE(SUM(cost_eur), 0)::float8,
			COALESCE(AVG(latency_ms), 0)::float8
		FROM ai_interaction_logs
		WHERE organization_id = $1
	`, orgID).Scan(&total, &totalInput, &totalOutput, &totalCost, &avgLatency)
	if err != nil {
		return nil, fmt.Errorf("querying usage stats: %w", err)
	}

	stats["total_interactions"] = total
	stats["total_input_tokens"] = totalInput
	stats["total_output_tokens"] = totalOutput
	stats["total_cost_eur"] = totalCost
	stats["avg_latency_ms"] = avgLatency

	// Breakdown by interaction type.
	byType := make(map[string]int)
	rows, err := ai.pool.Query(ctx, `
		SELECT interaction_type, COUNT(*)::int
		FROM ai_interaction_logs
		WHERE organization_id = $1
		GROUP BY interaction_type
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying usage by type: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t string
		var c int
		if err := rows.Scan(&t, &c); err != nil {
			return nil, fmt.Errorf("scanning usage row: %w", err)
		}
		byType[t] = c
	}
	stats["by_type"] = byType

	// Monthly breakdown for the last 6 months.
	monthlyRows, err := ai.pool.Query(ctx, `
		SELECT
			TO_CHAR(created_at, 'YYYY-MM') AS month,
			COUNT(*)::int,
			COALESCE(SUM(cost_eur), 0)::float8
		FROM ai_interaction_logs
		WHERE organization_id = $1 AND created_at >= NOW() - INTERVAL '6 months'
		GROUP BY month
		ORDER BY month
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying monthly stats: %w", err)
	}
	defer monthlyRows.Close()

	var monthly []map[string]interface{}
	for monthlyRows.Next() {
		var month string
		var cnt int
		var cost float64
		if err := monthlyRows.Scan(&month, &cnt, &cost); err != nil {
			return nil, fmt.Errorf("scanning monthly row: %w", err)
		}
		monthly = append(monthly, map[string]interface{}{
			"month":        month,
			"interactions": cnt,
			"cost_eur":     cost,
		})
	}
	stats["monthly"] = monthly

	return stats, nil
}

// RateFeedback records user feedback on an AI interaction.
func (ai *AIService) RateFeedback(ctx context.Context, logID string, rating int, feedback string) error {
	tag, err := ai.pool.Exec(ctx, `
		UPDATE ai_interaction_logs
		SET feedback_rating = $2, feedback_text = $3
		WHERE id = $1
	`, logID, rating, feedback)
	if err != nil {
		return fmt.Errorf("updating feedback: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fallback methods (used when API key is empty)
// ---------------------------------------------------------------------------

func (ai *AIService) fallbackRemediationPlan(gaps []GapInput) string {
	result := "## Remediation Plan (Generated Offline)\n\n"
	for i, g := range gaps {
		result += fmt.Sprintf("### %d. %s — %s\n", i+1, g.ControlCode, g.ControlTitle)
		result += fmt.Sprintf("- **Gap:** %s\n", g.Description)
		result += fmt.Sprintf("- **Severity:** %s\n", g.Severity)
		result += "- **Recommended actions:** Review control requirements, assign an owner, gather baseline evidence, implement technical/procedural controls, schedule review.\n"
		result += "- **Estimated effort:** 2-4 weeks depending on complexity.\n\n"
	}
	return result
}

func (ai *AIService) fallbackControlGuidance(controlCode, controlTitle string) string {
	return fmt.Sprintf(`## Implementation Guidance for %s — %s

1. **Requirements:** Review the control specification in the applicable framework standard.
2. **Implementation steps:** Assign an owner, define scope, implement technical and procedural measures, document the implementation.
3. **Evidence:** Collect configuration screenshots, policy documents, access logs, and review records.
4. **Common pitfalls:** Incomplete scope definition, lack of ongoing monitoring, insufficient documentation.
5. **Estimated effort:** 1-3 weeks for initial implementation.
6. **Maintenance:** Quarterly reviews and evidence refresh recommended.`, controlCode, controlTitle)
}

func (ai *AIService) fallbackEvidenceSuggestion(controlCode, controlTitle string) string {
	return fmt.Sprintf(`## Suggested Evidence for %s — %s

1. **Policy document** — Approved policy covering this control area (document, annual)
2. **Configuration export** — System configuration showing control implementation (config, quarterly)
3. **Access review log** — Periodic access review records (log, quarterly)
4. **Audit report** — Internal/external audit findings (report, annual)
5. **Training records** — Staff awareness and training completion (document, annual)`, controlCode, controlTitle)
}

func (ai *AIService) fallbackPolicyDraft(policyType, section string) string {
	return fmt.Sprintf(`## %s Policy — %s

### Purpose
This section defines the requirements for %s within the context of the organisation's %s policy.

### Requirements
- [Define specific requirements relevant to %s]
- Ensure alignment with applicable regulatory and framework requirements.
- Assign clear roles and responsibilities.

### Roles and Responsibilities
- **Policy Owner:** Responsible for maintaining and reviewing this section.
- **All Staff:** Responsible for complying with the requirements defined herein.

*Note: This is a template draft. Customise to your organisation's context.*`, policyType, section, section, policyType, section)
}

func (ai *AIService) fallbackRiskNarrative(riskTitle string) string {
	return fmt.Sprintf(`## Risk Assessment — %s

### Analysis
- **Likelihood:** To be assessed based on organisational context.
- **Impact:** To be assessed based on business impact analysis.
- **Inherent risk level:** Requires further evaluation.

### Recommended Treatment
1. Identify and document existing controls.
2. Evaluate residual risk after controls.
3. Consider additional mitigation measures if residual risk exceeds appetite.
4. Establish monitoring and review schedule.

*Note: This is a template narrative. Engage domain experts for a comprehensive assessment.*`, riskTitle)
}
