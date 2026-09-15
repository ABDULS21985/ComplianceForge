package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/middleware"
	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type classifiedCollaborationAliasService struct {
	CollaborationService
	comments  []Comment
	following []FollowedEntity
}

func (s classifiedCollaborationAliasService) ListComments(
	context.Context, string, string, string, models.PaginationRequest,
) ([]Comment, int, error) {
	return s.comments, len(s.comments), nil
}

func (s classifiedCollaborationAliasService) ListFollowing(context.Context, string, string) ([]FollowedEntity, error) {
	return s.following, nil
}

type classifiedBIAAliasService struct {
	BIAService
	process *BIAProcessDetail
	plans   []BCPlan
}

func (s classifiedBIAAliasService) GetProcess(context.Context, string, string) (*BIAProcessDetail, error) {
	return s.process, nil
}

func (s classifiedBIAAliasService) ListBCPlans(
	context.Context, string, models.PaginationRequest,
) ([]BCPlan, int, error) {
	return s.plans, len(s.plans), nil
}

type classifiedExceptionAliasService struct {
	ExceptionService
	item *ExceptionDetail
}

func (s classifiedExceptionAliasService) GetException(context.Context, string, string) (*ExceptionDetail, error) {
	return s.item, nil
}

type classifiedDSRAliasService struct {
	DSRService
	item *DSRRequestDetail
}

func (s classifiedDSRAliasService) GetRequest(context.Context, string, string) (*DSRRequestDetail, error) {
	return s.item, nil
}

type classifiedQuestionnaireAliasService struct {
	QuestionnaireService
	questionnaire *QuestionnaireDetail
	assessment    *VendorAssessmentDetail
}

func (s classifiedQuestionnaireAliasService) GetQuestionnaire(
	context.Context, string, string,
) (*QuestionnaireDetail, error) {
	return s.questionnaire, nil
}

func (s classifiedQuestionnaireAliasService) GetVendorAssessment(
	context.Context, string, string,
) (*VendorAssessmentDetail, error) {
	return s.assessment, nil
}

type classifiedROPAAliasService struct {
	ROPAService
	activity *ProcessingActivityDetail
	file     *ROPAExportFile
}

func (s classifiedROPAAliasService) GetProcessingActivity(
	context.Context, string, string,
) (*ProcessingActivityDetail, error) {
	return s.activity, nil
}

func (s classifiedROPAAliasService) DownloadExport(context.Context, string, string) (*ROPAExportFile, error) {
	return s.file, nil
}

type classifiedOnboardingAliasService struct {
	OnboardingSvc
	progress     map[string]any
	subscription map[string]any
}

func (s classifiedOnboardingAliasService) GetProgress(context.Context, string) (interface{}, error) {
	return s.progress, nil
}

func (s classifiedOnboardingAliasService) GetSubscription(context.Context, string) (interface{}, error) {
	return s.subscription, nil
}

type classifiedPGXRow struct {
	values []any
	err    error
}

func (r classifiedPGXRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return scanClassifiedPGXValues(dest, r.values)
}

type classifiedPGXRows struct {
	values [][]any
	index  int
	closed bool
}

func (r *classifiedPGXRows) Close()                                       { r.closed = true }
func (r *classifiedPGXRows) Err() error                                   { return nil }
func (r *classifiedPGXRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("") }
func (r *classifiedPGXRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *classifiedPGXRows) RawValues() [][]byte                          { return nil }
func (r *classifiedPGXRows) Conn() *pgx.Conn                              { return nil }

func (r *classifiedPGXRows) Next() bool {
	if r.closed || r.index+1 >= len(r.values) {
		r.closed = true
		return false
	}
	r.index++
	return true
}

func (r *classifiedPGXRows) Scan(dest ...any) error {
	if r.index < 0 || r.index >= len(r.values) {
		return fmt.Errorf("classified test rows have no current row")
	}
	return scanClassifiedPGXValues(dest, r.values[r.index])
}

func (r *classifiedPGXRows) Values() ([]any, error) {
	if r.index < 0 || r.index >= len(r.values) {
		return nil, fmt.Errorf("classified test rows have no current row")
	}
	return r.values[r.index], nil
}

type classifiedNotificationQuerier struct {
	preferenceValues []any
	ruleRows         [][]any
	notificationRows [][]any
}

func (q classifiedNotificationQuerier) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unexpected Exec in classified notification response test")
}

func (q classifiedNotificationQuerier) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	var rows [][]any
	switch {
	case strings.Contains(query, "FROM notification_rules"):
		rows = q.ruleRows
	case strings.Contains(query, "FROM notifications"):
		rows = q.notificationRows
	default:
		return nil, fmt.Errorf("unexpected classified notification query")
	}
	return &classifiedPGXRows{values: rows, index: -1}, nil
}

func (q classifiedNotificationQuerier) QueryRow(_ context.Context, query string, _ ...any) pgx.Row {
	switch {
	case strings.Contains(query, "FROM notification_preferences"):
		return classifiedPGXRow{values: q.preferenceValues}
	case strings.Contains(query, "COUNT(*) FROM notification_rules"):
		return classifiedPGXRow{values: []any{len(q.ruleRows)}}
	case strings.Contains(query, "COUNT(*) FROM notifications"):
		return classifiedPGXRow{values: []any{len(q.notificationRows)}}
	default:
		return classifiedPGXRow{err: fmt.Errorf("unexpected classified notification query row")}
	}
}

func scanClassifiedPGXValues(dest, values []any) error {
	if len(dest) != len(values) {
		return fmt.Errorf("classified test scan destination count %d does not match value count %d", len(dest), len(values))
	}
	for index := range dest {
		target := reflect.ValueOf(dest[index])
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return fmt.Errorf("classified test scan destination %d is not a pointer", index)
		}
		if values[index] == nil {
			target.Elem().SetZero()
			continue
		}
		source := reflect.ValueOf(values[index])
		if !source.Type().AssignableTo(target.Elem().Type()) {
			return fmt.Errorf("classified test value %d (%s) cannot scan into %s", index, source.Type(), target.Elem().Type())
		}
		target.Elem().Set(source)
	}
	return nil
}

func TestAuthenticatedAliasHandlersMaskNestedValuesWithoutMutation(t *testing.T) {
	comments := []Comment{{
		ID: classifiedItemID, OrganizationID: classifiedOrgID,
		Content: "restricted control discussion", AuthorName: "Visible reviewer",
		Reactions: []CommentReaction{{Emoji: "ack", UserID: "comment.private@example.test", Count: 1}},
	}}
	following := []FollowedEntity{{
		EntityType: "control", EntityID: classifiedItemID,
		EntityTitle: "Restricted acquisition control", FollowedAt: "2026-09-14T12:00:00Z",
	}}
	bia := &BIAProcessDetail{
		BIAProcess: BIAProcess{
			ID: classifiedItemID, OrganizationID: classifiedOrgID, Name: "Payments",
			OperationalImpact: "private continuity impact", Criticality: "critical",
		},
		Dependencies: []ProcessDependency{{
			ProcessID: "dependent-1", ProcessName: "Settlement", Type: "upstream",
			Description: "bia.private@example.test",
		}},
	}
	bcPlans := []BCPlan{{
		ID: classifiedItemID, OrganizationID: classifiedOrgID, Name: "Payments recovery",
		ScenarioID: "private-scenario", Status: "approved",
		Procedures: []BCProcedure{{Order: 1, Title: "Escalate", ResponsibleRole: "bc.private@example.test"}},
	}}
	exception := &ExceptionDetail{
		Exception: Exception{
			ID: classifiedItemID, OrganizationID: classifiedOrgID, Title: "Temporary payment bypass",
			Justification: "private exception justification", RiskLevel: "high",
		},
		Reviews: []ExceptionReview{{ID: "review-1", Comments: "review.private@example.test", Status: "continued"}},
	}
	dsr := &DSRRequestDetail{
		DSRRequest: DSRRequest{
			ID: classifiedItemID, OrganizationID: classifiedOrgID, RequestType: "access",
			DataSubjectName: "Private Subject", DataSubjectEmail: "subject.private@example.test",
		},
		Tasks: []DSRTask{{ID: "task-1", Title: "Collect records", Notes: "dsr.private@example.test"}},
	}
	questionnaire := &QuestionnaireDetail{Questionnaire: Questionnaire{
		ID: classifiedItemID, OrganizationID: classifiedOrgID, Title: "Security review",
		Description: "private questionnaire description",
		Sections: []QuestionSection{{ID: "section-1", Title: "Security", Questions: []Question{{
			ID: "question-1", Text: "Describe controls", HelpText: "question.private@example.test",
		}}}},
	}}
	assessment := &VendorAssessmentDetail{
		VendorAssessment: VendorAssessment{
			ID: classifiedItemID, OrganizationID: classifiedOrgID, VendorID: "vendor-1",
			QuestionnaireID: classifiedItemID, VendorName: "Payment vendor", PortalToken: "private-portal-token",
		},
		Responses: []QuestionResponse{{QuestionID: "question-1", Answer: "yes", Notes: "assessment.private@example.test"}},
	}
	activity := &ProcessingActivityDetail{
		ProcessingActivity: ProcessingActivity{
			ID: classifiedItemID, OrganizationID: classifiedOrgID, Name: "Customer analytics",
			DataProcessor: "private processor agreement", LegalBasis: "consent",
		},
		DataFlows: []DataFlow{{
			ID: "flow-1", Source: "CRM", Destination: "Warehouse", DataType: "contact",
			Notes: "ropa.private@example.test",
		}},
	}
	progress := map[string]any{
		"owner_email": "onboard.private@example.test",
		"steps":       []any{map[string]any{"name": "Company profile", "contact": "step.private@example.test"}},
	}
	subscription := map[string]any{
		"plan": "enterprise", "billing_contact": "billing.private@example.test",
		"invoices": []any{map[string]any{"reference": "INV-001", "recipient": "invoice.private@example.test"}},
	}

	quietStart := "22:00"
	quietEnd := "06:00"
	timezone := "Africa/Lagos"
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	ruleConditions := []byte(`{"recipient":{"email":"rule.private@example.test"},"safe":"retained"}`)
	notifications := classifiedNotificationQuerier{
		preferenceValues: []any{
			"preference-1", classifiedUserID, classifiedOrgID, "*",
			true, true, false, "immediate", &quietStart, &quietEnd, &timezone, now, now,
		},
		ruleRows: [][]any{{
			"rule-1", classifiedOrgID, "Private escalation rule", "risk.updated",
			[]string{"high"}, ruleConditions, []string{"channel-1"}, "users", []string{classifiedUserID},
			(*string)(nil), true, 15, (*int)(nil), []string{}, now, now,
		}},
		notificationRows: [][]any{{
			"notification-1", classifiedOrgID, "risk.updated", classifiedUserID, "in_app",
			"notification.private@example.test", "private notification body", "delivered",
			now, (*time.Time)(nil), (*time.Time)(nil),
		}},
	}

	collaboration := classifiedCollaborationAliasService{comments: comments, following: following}
	biaService := classifiedBIAAliasService{process: bia, plans: bcPlans}
	questionnaireService := classifiedQuestionnaireAliasService{questionnaire: questionnaire, assessment: assessment}
	onboarding := classifiedOnboardingAliasService{progress: progress, subscription: subscription}
	notificationHandler := NewNotificationHandler(nil, nil)

	tests := []struct {
		name      string
		resource  string
		handler   http.Handler
		params    map[string]string
		querier   database.Querier
		rules     []models.AccessFieldPermission
		secrets   []string
		expected  []string
		unchanged func() bool
	}{
		{
			name: "collaboration comments", resource: "controls",
			handler: http.HandlerFunc(NewCollaborationHandler(collaboration).ListComments),
			params:  map[string]string{"entityType": "control", "entityId": classifiedItemID},
			rules: []models.AccessFieldPermission{
				classifiedRule("controls", "content", models.AccessFieldHidden, ""),
				classifiedRule("controls", "reactions.user_id", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"restricted control discussion", "comment.private@example.test"},
			expected: []string{"c***@example.test", "Visible reviewer", `"pagination"`},
			unchanged: func() bool {
				return comments[0].Content == "restricted control discussion" && comments[0].Reactions[0].UserID == "comment.private@example.test"
			},
		},
		{
			name: "collaboration following", resource: "controls",
			handler: http.HandlerFunc(NewCollaborationHandler(collaboration).ListFollowing),
			rules: []models.AccessFieldPermission{
				classifiedRule("controls", "entity_title", models.AccessFieldHidden, ""),
			},
			secrets: []string{"Restricted acquisition control"}, expected: []string{`"entity_type":"control"`},
			unchanged: func() bool { return following[0].EntityTitle == "Restricted acquisition control" },
		},
		{
			name: "business impact analysis", resource: "risks",
			handler: http.HandlerFunc(NewBIAHandler(biaService).GetProcess), params: map[string]string{"id": classifiedItemID},
			rules: []models.AccessFieldPermission{
				classifiedRule("risks", "operational_impact", models.AccessFieldHidden, ""),
				classifiedRule("risks", "dependencies.description", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"private continuity impact", "bia.private@example.test"},
			expected: []string{"b***@example.test", `"name":"Payments"`},
			unchanged: func() bool {
				return bia.OperationalImpact == "private continuity impact" && bia.Dependencies[0].Description == "bia.private@example.test"
			},
		},
		{
			name: "business continuity", resource: "risks",
			handler: http.HandlerFunc(NewBIAHandler(biaService).ListBCPlans),
			rules: []models.AccessFieldPermission{
				classifiedRule("risks", "scenario_id", models.AccessFieldHidden, ""),
				classifiedRule("risks", "procedures.responsible_role", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"private-scenario", "bc.private@example.test"},
			expected: []string{"b***@example.test", `"pagination"`},
			unchanged: func() bool {
				return bcPlans[0].ScenarioID == "private-scenario" && bcPlans[0].Procedures[0].ResponsibleRole == "bc.private@example.test"
			},
		},
		{
			name: "risk exceptions", resource: "risks",
			handler: http.HandlerFunc(NewExceptionHandler(classifiedExceptionAliasService{item: exception}).GetByID),
			params:  map[string]string{"id": classifiedItemID},
			rules: []models.AccessFieldPermission{
				classifiedRule("risks", "justification", models.AccessFieldHidden, ""),
				classifiedRule("risks", "reviews.comments", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"private exception justification", "review.private@example.test"},
			expected: []string{"r***@example.test", `"risk_level":"high"`},
			unchanged: func() bool {
				return exception.Justification == "private exception justification" && exception.Reviews[0].Comments == "review.private@example.test"
			},
		},
		{
			name: "data subject requests", resource: "incidents",
			handler: http.HandlerFunc(NewDSRHandler(classifiedDSRAliasService{item: dsr}).GetRequest),
			params:  map[string]string{"id": classifiedItemID},
			rules: []models.AccessFieldPermission{
				classifiedRule("incidents", "data_subject_email", models.AccessFieldHidden, ""),
				classifiedRule("incidents", "tasks.notes", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"subject.private@example.test", "dsr.private@example.test"},
			expected: []string{"d***@example.test", `"title":"Collect records"`},
			unchanged: func() bool {
				return dsr.DataSubjectEmail == "subject.private@example.test" && dsr.Tasks[0].Notes == "dsr.private@example.test"
			},
		},
		{
			name: "records of processing", resource: "incidents",
			handler: http.HandlerFunc(NewROPAHandler(classifiedROPAAliasService{activity: activity}).GetProcessingActivity),
			params:  map[string]string{"id": classifiedItemID},
			rules: []models.AccessFieldPermission{
				classifiedRule("incidents", "data_processor", models.AccessFieldHidden, ""),
				classifiedRule("incidents", "data_flows.notes", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"private processor agreement", "ropa.private@example.test"},
			expected: []string{"r***@example.test", `"source":"CRM"`},
			unchanged: func() bool {
				return activity.DataProcessor == "private processor agreement" && activity.DataFlows[0].Notes == "ropa.private@example.test"
			},
		},
		{
			name: "vendor questionnaires", resource: "vendors",
			handler: http.HandlerFunc(NewQuestionnaireHandler(questionnaireService).GetQuestionnaire),
			params:  map[string]string{"id": classifiedItemID},
			rules: []models.AccessFieldPermission{
				classifiedRule("vendors", "description", models.AccessFieldHidden, ""),
				classifiedRule("vendors", "sections.questions.help_text", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"private questionnaire description", "question.private@example.test"},
			expected: []string{"q***@example.test", `"text":"Describe controls"`},
			unchanged: func() bool {
				return questionnaire.Description == "private questionnaire description" && questionnaire.Sections[0].Questions[0].HelpText == "question.private@example.test"
			},
		},
		{
			name: "vendor assessments", resource: "vendors",
			handler: http.HandlerFunc(NewQuestionnaireHandler(questionnaireService).GetVendorAssessment),
			params:  map[string]string{"id": classifiedItemID},
			rules: []models.AccessFieldPermission{
				classifiedRule("vendors", "portal_token", models.AccessFieldHidden, ""),
				classifiedRule("vendors", "responses.notes", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"private-portal-token", "assessment.private@example.test"},
			expected: []string{"a***@example.test", `"vendor_name":"Payment vendor"`},
			unchanged: func() bool {
				return assessment.PortalToken == "private-portal-token" && assessment.Responses[0].Notes == "assessment.private@example.test"
			},
		},
		{
			name: "organization onboarding", resource: "organizations",
			handler: http.HandlerFunc(NewOnboardingHandler(onboarding).GetProgress),
			rules: []models.AccessFieldPermission{
				classifiedRule("organizations", "owner_email", models.AccessFieldHidden, ""),
				classifiedRule("organizations", "steps.contact", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"onboard.private@example.test", "step.private@example.test"},
			expected: []string{"s***@example.test", "Company profile"},
			unchanged: func() bool {
				return progress["owner_email"] == "onboard.private@example.test" && progress["steps"].([]any)[0].(map[string]any)["contact"] == "step.private@example.test"
			},
		},
		{
			name: "organization subscription", resource: "organizations",
			handler: http.HandlerFunc(NewOnboardingHandler(onboarding).GetSubscription),
			rules: []models.AccessFieldPermission{
				classifiedRule("organizations", "billing_contact", models.AccessFieldHidden, ""),
				classifiedRule("organizations", "invoices.recipient", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:  []string{"billing.private@example.test", "invoice.private@example.test"},
			expected: []string{"i***@example.test", `"plan":"enterprise"`},
			unchanged: func() bool {
				return subscription["billing_contact"] == "billing.private@example.test" && subscription["invoices"].([]any)[0].(map[string]any)["recipient"] == "invoice.private@example.test"
			},
		},
		{
			name: "user notifications", resource: "users",
			handler: http.HandlerFunc(notificationHandler.GetPreferences), querier: notifications,
			rules: []models.AccessFieldPermission{
				classifiedRule("users", "quiet_hours_timezone", models.AccessFieldMasked, models.AccessMaskRedact),
			},
			secrets: []string{timezone}, expected: []string{`"quiet_hours_timezone":"***"`, `"digest_frequency":"immediate"`},
			unchanged: func() bool { return *notifications.preferenceValues[10].(*string) == timezone },
		},
		{
			name: "notification administration", resource: "settings",
			handler: http.HandlerFunc(notificationHandler.ListRules), querier: notifications,
			rules: []models.AccessFieldPermission{
				classifiedRule("settings", "name", models.AccessFieldHidden, ""),
				classifiedRule("settings", "conditions.recipient.email", models.AccessFieldMasked, models.AccessMaskEmail),
			},
			secrets:   []string{"Private escalation rule", "rule.private@example.test"},
			expected:  []string{"r***@example.test", `"safe":"retained"`, `"pagination"`},
			unchanged: func() bool { return strings.Contains(string(ruleConditions), "rule.private@example.test") },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveClassifiedAliasHandler(test.resource, test.rules, test.handler, test.params, test.querier, true)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertClassifiedValues(t, response, test.secrets, test.expected)
			if !test.unchanged() {
				t.Fatal("handler mutated data owned by its service or database result")
			}

			response = serveClassifiedAliasHandler(test.resource, test.rules, test.handler, test.params, test.querier, false)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("missing-decision status=%d body=%s", response.Code, response.Body.String())
			}
			assertClassifiedValues(t, response, test.secrets, nil)
		})
	}
}

func TestNotificationListDelaysUnreadHeaderUntilMaskingSucceeds(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	querier := classifiedNotificationQuerier{notificationRows: [][]any{{
		"notification-1", classifiedOrgID, "risk.updated", classifiedUserID, "in_app",
		"notification.private@example.test", "private notification body", "delivered",
		now, (*time.Time)(nil), (*time.Time)(nil),
	}}}
	handler := http.HandlerFunc(NewNotificationHandler(nil, nil).ListNotifications)
	rules := []models.AccessFieldPermission{
		classifiedRule("users", "body", models.AccessFieldHidden, ""),
		classifiedRule("users", "subject", models.AccessFieldMasked, models.AccessMaskEmail),
	}

	response := serveClassifiedAliasHandler("users", rules, handler, nil, querier, true)
	if response.Code != http.StatusOK || response.Header().Get("X-Unread-Count") != "1" {
		t.Fatalf("allowed status=%d unread=%q body=%s", response.Code, response.Header().Get("X-Unread-Count"), response.Body.String())
	}
	assertClassifiedValues(t, response,
		[]string{"private notification body", "notification.private@example.test"},
		[]string{"n***@example.test", `"pagination"`},
	)

	response = serveClassifiedAliasHandler("users", rules, handler, nil, querier, false)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("X-Unread-Count") != "" {
		t.Fatalf("fail-closed status=%d unread=%q body=%s", response.Code, response.Header().Get("X-Unread-Count"), response.Body.String())
	}
	assertClassifiedValues(t, response,
		[]string{"private notification body", "notification.private@example.test"}, nil,
	)
}

func TestROPAExportAttachmentFailsClosedBeforeHeadersOrBytes(t *testing.T) {
	secret := []byte("classified-ropa-export")
	file := &ROPAExportFile{
		ExportID: classifiedItemID, FileName: "private-ropa.csv",
		ContentType: "text/csv", FileData: secret,
	}
	handler := http.HandlerFunc(NewROPAHandler(classifiedROPAAliasService{file: file}).DownloadExport)
	params := map[string]string{"id": classifiedItemID}

	response := serveClassifiedAliasHandler("incidents", []models.AccessFieldPermission{
		classifiedRule("incidents", "data_processor", models.AccessFieldHidden, ""),
	}, handler, params, nil, true)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), string(secret)) {
		t.Fatalf("mask-required status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Disposition") != "" || response.Header().Get("Content-Length") != "" {
		t.Fatalf("attachment headers emitted before masking decision: %v", response.Header())
	}
	if string(file.FileData) != string(secret) {
		t.Fatal("attachment source data was mutated")
	}

	response = serveClassifiedAliasHandler("incidents", nil, handler, params, nil, true)
	if response.Code != http.StatusOK || response.Body.String() != string(secret) {
		t.Fatalf("unrestricted status=%d body=%q", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Disposition"), "private-ropa.csv") {
		t.Fatalf("missing safe attachment disposition: %v", response.Header())
	}
}

func serveClassifiedAliasHandler(
	resource string,
	rules []models.AccessFieldPermission,
	next http.Handler,
	params map[string]string,
	querier database.Querier,
	withDecision bool,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/classified-alias", nil)
	ctx := context.WithValue(request.Context(), middleware.ContextKeyOrgID, classifiedOrgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, classifiedUserID)
	if querier != nil {
		ctx = database.WithQuerier(ctx, querier)
	}
	route := chi.NewRouteContext()
	for key, value := range params {
		route.URLParams.Add(key, value)
	}
	request = request.WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	response := httptest.NewRecorder()
	if !withDecision {
		next.ServeHTTP(response, request)
		return response
	}
	middleware.RequireAuthorization(
		classifiedAuthorizer{obligations: service.AccessFieldObligations(rules)}, resource, "read", nil,
	)(next).ServeHTTP(response, request)
	return response
}
