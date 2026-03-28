package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// OnboardingService manages the multi-step onboarding wizard and subscription lifecycle.
type OnboardingService struct {
	pool *pgxpool.Pool
}

// OnboardingProgress tracks a tenant's progress through the onboarding wizard.
type OnboardingProgress struct {
	ID                   string                   `json:"id"`
	OrgID                string                   `json:"organization_id"`
	CurrentStep          int                      `json:"current_step"`
	TotalSteps           int                      `json:"total_steps"`
	CompletedSteps       []map[string]interface{} `json:"completed_steps"`
	IsCompleted          bool                     `json:"is_completed"`
	OrgProfileData       map[string]interface{}   `json:"org_profile_data"`
	IndustryAssessment   map[string]interface{}   `json:"industry_assessment_data"`
	SelectedFrameworkIDs []string                 `json:"selected_framework_ids"`
	TeamInvitations      []map[string]interface{} `json:"team_invitations"`
	RiskAppetiteData     map[string]interface{}   `json:"risk_appetite_data"`
	QuickAssessmentData  map[string]interface{}   `json:"quick_assessment_data"`
}

// FrameworkRecommendation is a single framework suggestion returned by the recommendation engine.
type FrameworkRecommendation struct {
	FrameworkID   string `json:"framework_id"`
	FrameworkCode string `json:"framework_code"`
	FrameworkName string `json:"framework_name"`
	Priority      int    `json:"priority"`
	Reason        string `json:"reason"`
	Mandatory     bool   `json:"mandatory"`
	OverlapInfo   string `json:"overlap_info"`
}

// SubscriptionPlan describes an available billing plan.
type SubscriptionPlan struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Slug          string                 `json:"slug"`
	Tier          string                 `json:"tier"`
	PriceMonthly  float64                `json:"pricing_eur_monthly"`
	PriceAnnual   float64                `json:"pricing_eur_annual"`
	MaxUsers      int                    `json:"max_users"`
	MaxFrameworks int                    `json:"max_frameworks"`
	MaxRisks      int                    `json:"max_risks"`
	MaxVendors    int                    `json:"max_vendors"`
	MaxStorageGB  int                    `json:"max_storage_gb"`
	Features      map[string]interface{} `json:"features"`
}

// UsageSummary aggregates current resource consumption vs plan limits.
type UsageSummary struct {
	Users      UsageMetric `json:"users"`
	Frameworks UsageMetric `json:"frameworks"`
	Risks      UsageMetric `json:"risks"`
	Vendors    UsageMetric `json:"vendors"`
	StorageGB  UsageMetric `json:"storage_gb"`
}

// UsageMetric is a single current/max/percent tuple.
type UsageMetric struct {
	Current int     `json:"current"`
	Max     int     `json:"max"`
	Percent float64 `json:"percent"`
}

// NewOnboardingService constructs a new OnboardingService.
func NewOnboardingService(pool *pgxpool.Pool) *OnboardingService {
	return &OnboardingService{pool: pool}
}

// ---------------------------------------------------------------------------
// Onboarding wizard
// ---------------------------------------------------------------------------

// GetProgress returns the current onboarding progress for an organization.
func (s *OnboardingService) GetProgress(ctx context.Context, orgID string) (*OnboardingProgress, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, current_step, total_steps,
		       completed_steps, is_completed,
		       org_profile_data, industry_assessment_data,
		       selected_framework_ids, team_invitations,
		       risk_appetite_data, quick_assessment_data
		FROM onboarding_progress
		WHERE organization_id = $1`, orgID)

	p := &OnboardingProgress{}
	var completedStepsJSON, orgProfileJSON, industryJSON []byte
	var invitationsJSON, riskJSON, quickJSON []byte
	var frameworkIDs []string

	err := row.Scan(
		&p.ID, &p.OrgID, &p.CurrentStep, &p.TotalSteps,
		&completedStepsJSON, &p.IsCompleted,
		&orgProfileJSON, &industryJSON,
		&frameworkIDs, &invitationsJSON,
		&riskJSON, &quickJSON,
	)
	if err == pgx.ErrNoRows {
		// Create a fresh progress record.
		p = &OnboardingProgress{
			OrgID:                orgID,
			CurrentStep:          1,
			TotalSteps:           6,
			CompletedSteps:       []map[string]interface{}{},
			SelectedFrameworkIDs: []string{},
			TeamInvitations:      []map[string]interface{}{},
		}
		err = s.pool.QueryRow(ctx, `
			INSERT INTO onboarding_progress
				(organization_id, current_step, total_steps, completed_steps,
				 is_completed, selected_framework_ids, team_invitations)
			VALUES ($1, 1, 6, '[]'::jsonb, false, '{}', '[]'::jsonb)
			RETURNING id`, orgID).Scan(&p.ID)
		if err != nil {
			return nil, fmt.Errorf("create onboarding progress: %w", err)
		}
		return p, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get onboarding progress: %w", err)
	}

	// Unmarshal JSONB columns.
	_ = json.Unmarshal(completedStepsJSON, &p.CompletedSteps)
	_ = json.Unmarshal(orgProfileJSON, &p.OrgProfileData)
	_ = json.Unmarshal(industryJSON, &p.IndustryAssessment)
	_ = json.Unmarshal(invitationsJSON, &p.TeamInvitations)
	_ = json.Unmarshal(riskJSON, &p.RiskAppetiteData)
	_ = json.Unmarshal(quickJSON, &p.QuickAssessmentData)
	p.SelectedFrameworkIDs = frameworkIDs
	if p.CompletedSteps == nil {
		p.CompletedSteps = []map[string]interface{}{}
	}
	if p.SelectedFrameworkIDs == nil {
		p.SelectedFrameworkIDs = []string{}
	}
	if p.TeamInvitations == nil {
		p.TeamInvitations = []map[string]interface{}{}
	}
	return p, nil
}

// SaveStepData persists the data collected in a specific wizard step.
// Step mapping: 1=org_profile, 2=industry_assessment, 3=selected_frameworks,
// 4=team_invitations, 5=risk_appetite, 6=quick_assessment.
func (s *OnboardingService) SaveStepData(ctx context.Context, orgID string, step int, data map[string]interface{}) error {
	columnMap := map[int]string{
		1: "org_profile_data",
		2: "industry_assessment_data",
		3: "selected_framework_ids",
		4: "team_invitations",
		5: "risk_appetite_data",
		6: "quick_assessment_data",
	}
	column, ok := columnMap[step]
	if !ok {
		return fmt.Errorf("invalid onboarding step: %d", step)
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal step data: %w", err)
	}

	// For step 3 we expect a string slice stored under "framework_ids".
	if step == 3 {
		if ids, exists := data["framework_ids"]; exists {
			idsJSON, _ := json.Marshal(ids)
			dataJSON = idsJSON
		}
	}

	completedEntry, _ := json.Marshal(map[string]interface{}{
		"step":         step,
		"completed_at": time.Now().UTC().Format(time.RFC3339),
	})

	query := fmt.Sprintf(`
		UPDATE onboarding_progress
		SET %s = $1,
		    current_step = GREATEST(current_step, $2),
		    completed_steps = (
		        SELECT jsonb_agg(elem)
		        FROM (
		            SELECT elem FROM jsonb_array_elements(
		                COALESCE(completed_steps, '[]'::jsonb)
		            ) AS elem
		            WHERE (elem->>'step')::int != $3
		            UNION ALL
		            SELECT $4::jsonb
		        ) sub
		    ),
		    updated_at = NOW()
		WHERE organization_id = $5`, column)

	nextStep := step + 1
	if nextStep > 6 {
		nextStep = 6
	}

	tag, err := s.pool.Exec(ctx, query, dataJSON, nextStep, step, completedEntry, orgID)
	if err != nil {
		return fmt.Errorf("save step %d data: %w", step, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no onboarding progress found for org %s", orgID)
	}

	log.Info().Str("org_id", orgID).Int("step", step).Msg("onboarding step saved")
	return nil
}

// GetRecommendations returns framework recommendations based on industry assessment answers.
func (s *OnboardingService) GetRecommendations(ctx context.Context, orgID string, assessmentData map[string]interface{}) ([]FrameworkRecommendation, error) {
	// Rule engine: each assessment key maps to one or more framework recommendations.
	type rule struct {
		assessmentKey string
		frameworkCode string
		frameworkName string
		reason        string
		mandatory     bool
		basePriority  int
		overlapInfo   string
	}

	rules := []rule{
		{"processes_payment_cards", "PCI-DSS-4.0", "PCI DSS v4.0", "Your organisation processes payment card data — PCI DSS compliance is mandatory.", true, 1, ""},
		{"processes_eu_personal_data", "UK-GDPR", "UK GDPR", "You process personal data of EU/UK residents — UK GDPR compliance is mandatory.", true, 2, ""},
		{"essential_service_provider", "NIS2", "NIS2 Directive", "As an essential-service provider you fall under NIS2 scope.", false, 3, "NIS2 controls overlap ~40%% with ISO 27001 Annex A."},
		{"essential_service_provider", "NCSC-CAF", "NCSC Cyber Assessment Framework", "NCSC CAF is recommended for operators of essential services in the UK.", false, 4, "CAF outcomes map well to NIS2 Article 21 measures."},
		{"uk_public_sector", "CE-PLUS", "Cyber Essentials Plus", "UK public-sector organisations must hold a current Cyber Essentials certificate.", true, 5, "Cyber Essentials is a subset of ISO 27001 controls."},
		{"requires_iso_certification", "ISO-27001", "ISO/IEC 27001:2022", "Your organisation requires formal ISO 27001 certification.", false, 6, ""},
		{"us_federal_contracts", "NIST-800-53", "NIST SP 800-53 Rev 5", "US federal contract obligations require NIST 800-53 controls.", false, 7, "~80%% overlap with ISO 27001 when using Annex A mapping."},
		{"target_maturity", "NIST-CSF-2.0", "NIST Cybersecurity Framework 2.0", "NIST CSF 2.0 provides a maturity-model approach aligned with your target maturity.", false, 8, "CSF functions map to ISO 27001 clauses."},
		{"itil_required", "ITIL-4", "ITIL 4", "Your IT service-management needs indicate ITIL 4 adoption.", false, 9, "ITIL 4 practices complement ISO 27001 operational controls."},
		{"board_governance_required", "COBIT-2019", "COBIT 2019", "Board-level governance requirements suggest COBIT 2019.", false, 10, "COBIT governance objectives align with ISO 27001 leadership clauses."},
	}

	// Look up framework IDs from the database.
	frameworkLookup := make(map[string]string) // code → id
	rows, err := s.pool.Query(ctx, `SELECT id, code FROM frameworks WHERE organization_id = $1 OR is_template = true`, orgID)
	if err != nil {
		return nil, fmt.Errorf("query frameworks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, code string
		if err := rows.Scan(&id, &code); err != nil {
			continue
		}
		frameworkLookup[code] = id
	}

	var recs []FrameworkRecommendation
	for _, r := range rules {
		val, exists := assessmentData[r.assessmentKey]
		if !exists {
			continue
		}
		// Accept bool true or string "true" / "yes".
		triggered := false
		switch v := val.(type) {
		case bool:
			triggered = v
		case string:
			triggered = v == "true" || v == "yes"
		case float64:
			triggered = v != 0
		}
		if !triggered {
			continue
		}
		fwID := frameworkLookup[r.frameworkCode]
		recs = append(recs, FrameworkRecommendation{
			FrameworkID:   fwID,
			FrameworkCode: r.frameworkCode,
			FrameworkName: r.frameworkName,
			Priority:      r.basePriority,
			Reason:        r.reason,
			Mandatory:     r.mandatory,
			OverlapInfo:   r.overlapInfo,
		})
	}

	// Mandatory frameworks first, then by priority ascending.
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Mandatory != recs[j].Mandatory {
			return recs[i].Mandatory
		}
		return recs[i].Priority < recs[j].Priority
	})

	log.Info().Str("org_id", orgID).Int("recommendations", len(recs)).Msg("framework recommendations generated")
	return recs, nil
}

// CompleteOnboarding finalises the wizard in a single transaction.
func (s *OnboardingService) CompleteOnboarding(ctx context.Context, orgID, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Load progress.
	var orgProfileJSON, industryJSON, riskJSON, quickJSON, invitationsJSON []byte
	var frameworkIDs []string
	err = tx.QueryRow(ctx, `
		SELECT org_profile_data, industry_assessment_data, selected_framework_ids,
		       team_invitations, risk_appetite_data, quick_assessment_data
		FROM onboarding_progress
		WHERE organization_id = $1 AND is_completed = false`, orgID).Scan(
		&orgProfileJSON, &industryJSON, &frameworkIDs,
		&invitationsJSON, &riskJSON, &quickJSON,
	)
	if err != nil {
		return fmt.Errorf("load onboarding progress: %w", err)
	}

	var orgProfile map[string]interface{}
	var riskAppetite map[string]interface{}
	var quickAssessment map[string]interface{}
	var invitations []map[string]interface{}
	_ = json.Unmarshal(orgProfileJSON, &orgProfile)
	_ = json.Unmarshal(riskJSON, &riskAppetite)
	_ = json.Unmarshal(quickJSON, &quickAssessment)
	_ = json.Unmarshal(invitationsJSON, &invitations)

	// 2. Update organization with profile data.
	if orgProfile != nil {
		name, _ := orgProfile["name"].(string)
		industry, _ := orgProfile["industry"].(string)
		size, _ := orgProfile["size"].(string)
		_, err = tx.Exec(ctx, `
			UPDATE organizations
			SET name = COALESCE(NULLIF($1,''), name),
			    industry = COALESCE(NULLIF($2,''), industry),
			    company_size = COALESCE(NULLIF($3,''), company_size),
			    updated_at = NOW()
			WHERE id = $4`, name, industry, size, orgID)
		if err != nil {
			return fmt.Errorf("update org profile: %w", err)
		}
	}

	// 3. Adopt selected frameworks.
	for _, fwID := range frameworkIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO organization_frameworks (organization_id, framework_id, status, adopted_at, adopted_by)
			VALUES ($1, $2, 'active', NOW(), $3)
			ON CONFLICT (organization_id, framework_id) DO UPDATE SET status = 'active', adopted_at = NOW()`,
			orgID, fwID, userID)
		if err != nil {
			return fmt.Errorf("adopt framework %s: %w", fwID, err)
		}
	}

	// 4. Initialize control implementations for adopted frameworks.
	for _, fwID := range frameworkIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO control_implementations (organization_id, control_id, status, created_by)
			SELECT $1, c.id, 'not_started', $3
			FROM controls c
			WHERE c.framework_id = $2
			ON CONFLICT (organization_id, control_id) DO NOTHING`,
			orgID, fwID, userID)
		if err != nil {
			return fmt.Errorf("init controls for framework %s: %w", fwID, err)
		}
	}

	// 5. Apply quick assessment answers — set initial implementation status.
	if quickAssessment != nil {
		for controlCode, answer := range quickAssessment {
			status := "not_started"
			switch v := answer.(type) {
			case string:
				switch v {
				case "yes", "implemented":
					status = "implemented"
				case "partial":
					status = "partially_implemented"
				case "planned":
					status = "planned"
				}
			}
			_, _ = tx.Exec(ctx, `
				UPDATE control_implementations ci
				SET status = $1, updated_at = NOW()
				FROM controls c
				WHERE ci.control_id = c.id
				  AND ci.organization_id = $2
				  AND c.code = $3`, status, orgID, controlCode)
		}
	}

	// 6. Create risk matrix from appetite data.
	if riskAppetite != nil {
		appetiteJSON, _ := json.Marshal(riskAppetite)
		_, err = tx.Exec(ctx, `
			INSERT INTO risk_matrices (organization_id, name, config, created_by, created_at)
			VALUES ($1, 'Default Risk Matrix', $2, $3, NOW())
			ON CONFLICT (organization_id, name) DO UPDATE SET config = $2, updated_at = NOW()`,
			orgID, appetiteJSON, userID)
		if err != nil {
			return fmt.Errorf("create risk matrix: %w", err)
		}
	}

	// 7. Send team invitations.
	for _, inv := range invitations {
		email, _ := inv["email"].(string)
		role, _ := inv["role"].(string)
		if email == "" {
			continue
		}
		if role == "" {
			role = "viewer"
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO team_invitations (organization_id, email, role, invited_by, status, created_at)
			VALUES ($1, $2, $3, $4, 'pending', NOW())
			ON CONFLICT (organization_id, email) DO UPDATE SET role = $3, status = 'pending', updated_at = NOW()`,
			orgID, email, role, userID)
		if err != nil {
			log.Warn().Err(err).Str("email", email).Msg("failed to create team invitation")
		}
	}

	// 8. Mark onboarding complete.
	_, err = tx.Exec(ctx, `
		UPDATE onboarding_progress
		SET is_completed = true, completed_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1`, orgID)
	if err != nil {
		return fmt.Errorf("mark onboarding complete: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit onboarding: %w", err)
	}

	log.Info().Str("org_id", orgID).Str("user_id", userID).Msg("onboarding completed")
	return nil
}

// ---------------------------------------------------------------------------
// Subscription management
// ---------------------------------------------------------------------------

// ListPlans returns all available subscription plans.
func (s *OnboardingService) ListPlans(ctx context.Context) ([]SubscriptionPlan, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, slug, tier,
		       pricing_eur_monthly, pricing_eur_annual,
		       max_users, max_frameworks, max_risks, max_vendors, max_storage_gb,
		       features
		FROM subscription_plans
		WHERE is_active = true
		ORDER BY pricing_eur_monthly ASC`)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var plans []SubscriptionPlan
	for rows.Next() {
		var p SubscriptionPlan
		var featuresJSON []byte
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Slug, &p.Tier,
			&p.PriceMonthly, &p.PriceAnnual,
			&p.MaxUsers, &p.MaxFrameworks, &p.MaxRisks, &p.MaxVendors, &p.MaxStorageGB,
			&featuresJSON,
		); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		_ = json.Unmarshal(featuresJSON, &p.Features)
		plans = append(plans, p)
	}
	return plans, nil
}

// GetSubscription returns the current subscription for an organization.
func (s *OnboardingService) GetSubscription(ctx context.Context, orgID string) (*map[string]interface{}, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT s.id, s.organization_id, s.plan_id,
		       sp.name AS plan_name, sp.slug AS plan_slug, sp.tier,
		       s.billing_cycle, s.status,
		       s.current_period_start, s.current_period_end,
		       s.cancel_at_period_end, s.created_at
		FROM subscriptions s
		JOIN subscription_plans sp ON sp.id = s.plan_id
		WHERE s.organization_id = $1 AND s.status IN ('active','trialing')
		ORDER BY s.created_at DESC
		LIMIT 1`, orgID)

	var id, subOrgID, planID, planName, planSlug, tier, billingCycle, status string
	var periodStart, periodEnd, createdAt time.Time
	var cancelAtEnd bool

	err := row.Scan(&id, &subOrgID, &planID, &planName, &planSlug, &tier,
		&billingCycle, &status, &periodStart, &periodEnd, &cancelAtEnd, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("no active subscription for org %s", orgID)
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	result := map[string]interface{}{
		"id":                    id,
		"organization_id":      subOrgID,
		"plan_id":              planID,
		"plan_name":            planName,
		"plan_slug":            planSlug,
		"tier":                 tier,
		"billing_cycle":        billingCycle,
		"status":               status,
		"current_period_start": periodStart,
		"current_period_end":   periodEnd,
		"cancel_at_period_end": cancelAtEnd,
		"created_at":           createdAt,
	}
	return &result, nil
}

// ChangePlan switches the organization to a different subscription plan.
func (s *OnboardingService) ChangePlan(ctx context.Context, orgID, planSlug, billingCycle string) error {
	if billingCycle != "monthly" && billingCycle != "annual" {
		return fmt.Errorf("invalid billing cycle: %s (must be monthly or annual)", billingCycle)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Resolve plan.
	var planID string
	err = tx.QueryRow(ctx, `SELECT id FROM subscription_plans WHERE slug = $1 AND is_active = true`, planSlug).Scan(&planID)
	if err != nil {
		return fmt.Errorf("plan not found: %s", planSlug)
	}

	// Cancel current subscription.
	_, err = tx.Exec(ctx, `
		UPDATE subscriptions SET status = 'canceled', canceled_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND status IN ('active','trialing')`, orgID)
	if err != nil {
		return fmt.Errorf("cancel current plan: %w", err)
	}

	// Create new subscription.
	now := time.Now().UTC()
	var periodEnd time.Time
	if billingCycle == "annual" {
		periodEnd = now.AddDate(1, 0, 0)
	} else {
		periodEnd = now.AddDate(0, 1, 0)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO subscriptions
			(organization_id, plan_id, billing_cycle, status,
			 current_period_start, current_period_end, created_at)
		VALUES ($1, $2, $3, 'active', $4, $5, NOW())`,
		orgID, planID, billingCycle, now, periodEnd)
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}

	// Update the organization's tier cache.
	_, err = tx.Exec(ctx, `
		UPDATE organizations
		SET subscription_tier = (SELECT tier FROM subscription_plans WHERE id = $1),
		    updated_at = NOW()
		WHERE id = $2`, planID, orgID)
	if err != nil {
		return fmt.Errorf("update org tier: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit plan change: %w", err)
	}

	log.Info().Str("org_id", orgID).Str("plan", planSlug).Str("cycle", billingCycle).Msg("subscription plan changed")
	return nil
}

// CancelSubscription marks the current subscription for cancellation.
func (s *OnboardingService) CancelSubscription(ctx context.Context, orgID, reason string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscriptions
		SET cancel_at_period_end = true,
		    cancellation_reason = $1,
		    updated_at = NOW()
		WHERE organization_id = $2 AND status = 'active'`, reason, orgID)
	if err != nil {
		return fmt.Errorf("cancel subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no active subscription to cancel for org %s", orgID)
	}
	log.Info().Str("org_id", orgID).Str("reason", reason).Msg("subscription cancellation scheduled")
	return nil
}

// CheckLimit verifies whether the organization can create another resource of the given type.
func (s *OnboardingService) CheckLimit(ctx context.Context, orgID, resource string) (current int, max int, canCreate bool, err error) {
	countQuery := ""
	limitColumn := ""

	switch resource {
	case "users":
		countQuery = `SELECT COUNT(*) FROM users WHERE organization_id = $1 AND is_active = true`
		limitColumn = "max_users"
	case "frameworks":
		countQuery = `SELECT COUNT(*) FROM organization_frameworks WHERE organization_id = $1 AND status = 'active'`
		limitColumn = "max_frameworks"
	case "risks":
		countQuery = `SELECT COUNT(*) FROM risks WHERE organization_id = $1`
		limitColumn = "max_risks"
	case "vendors":
		countQuery = `SELECT COUNT(*) FROM vendors WHERE organization_id = $1`
		limitColumn = "max_vendors"
	case "storage_gb":
		countQuery = `SELECT COALESCE(SUM(file_size_bytes)::bigint / 1073741824, 0) FROM evidence_files WHERE organization_id = $1`
		limitColumn = "max_storage_gb"
	default:
		return 0, 0, false, fmt.Errorf("unknown resource type: %s", resource)
	}

	// Get current count.
	err = s.pool.QueryRow(ctx, countQuery, orgID).Scan(&current)
	if err != nil {
		return 0, 0, false, fmt.Errorf("count %s: %w", resource, err)
	}

	// Get plan limit.
	limitQuery := fmt.Sprintf(`
		SELECT sp.%s
		FROM subscriptions s
		JOIN subscription_plans sp ON sp.id = s.plan_id
		WHERE s.organization_id = $1 AND s.status IN ('active','trialing')
		ORDER BY s.created_at DESC LIMIT 1`, limitColumn)

	err = s.pool.QueryRow(ctx, limitQuery, orgID).Scan(&max)
	if err != nil {
		return current, 0, false, fmt.Errorf("get limit for %s: %w", resource, err)
	}

	// max of -1 means unlimited.
	if max < 0 {
		canCreate = true
	} else {
		canCreate = current < max
	}
	return current, max, canCreate, nil
}

// GetUsageSummary returns an aggregated view of resource usage vs limits.
func (s *OnboardingService) GetUsageSummary(ctx context.Context, orgID string) (*UsageSummary, error) {
	resources := []string{"users", "frameworks", "risks", "vendors", "storage_gb"}
	metrics := make(map[string]UsageMetric, len(resources))

	for _, res := range resources {
		cur, mx, _, err := s.CheckLimit(ctx, orgID, res)
		if err != nil {
			log.Warn().Err(err).Str("resource", res).Msg("usage check failed, defaulting to zero")
			metrics[res] = UsageMetric{}
			continue
		}
		pct := float64(0)
		if mx > 0 {
			pct = float64(cur) / float64(mx) * 100
		}
		metrics[res] = UsageMetric{Current: cur, Max: mx, Percent: pct}
	}

	return &UsageSummary{
		Users:      metrics["users"],
		Frameworks: metrics["frameworks"],
		Risks:      metrics["risks"],
		Vendors:    metrics["vendors"],
		StorageGB:  metrics["storage_gb"],
	}, nil
}

// RecordUsage appends a usage event for metering / billing analytics.
func (s *OnboardingService) RecordUsage(ctx context.Context, orgID, eventType string, quantity float64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO usage_events (organization_id, event_type, quantity, recorded_at)
		VALUES ($1, $2, $3, NOW())`, orgID, eventType, quantity)
	if err != nil {
		return fmt.Errorf("record usage event: %w", err)
	}
	return nil
}
