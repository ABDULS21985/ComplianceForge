package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/complianceforge/platform/internal/database"
	"github.com/complianceforge/platform/internal/middleware"
	emailpkg "github.com/complianceforge/platform/internal/pkg/email"
	"github.com/complianceforge/platform/internal/pkg/secretbox"
	"github.com/complianceforge/platform/internal/service"
)

type handlerEmailSender struct {
	messages []emailpkg.Message
}

func (s *handlerEmailSender) Send(_ context.Context, message emailpkg.Message) error {
	s.messages = append(s.messages, message)
	return nil
}

func TestNotificationHandlerAgainstMigratedPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgA, orgB, userA := uuid.NewString(), uuid.NewString(), uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id,name,slug,status,tier) VALUES
		($1,'Notification Handler A',$3,'active','starter'),
		($2,'Notification Handler B',$4,'active','starter')`,
		orgA, orgB, "notification-handler-a-"+suffix, "notification-handler-b-"+suffix); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=ANY($1::uuid[])`, []string{orgA, orgB})
	})
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,organization_id,email,status)
		VALUES ($1,$2,$3,'active')`, userA, orgA, "notification-handler-"+suffix+"@example.test"); err != nil {
		t.Fatal(err)
	}

	sender := &handlerEmailSender{}
	protector, err := secretbox.NewHex(strings.Repeat("ef", 32))
	if err != nil {
		t.Fatal(err)
	}
	engine := service.NewNotificationEngineWithProtector(pool, service.NewEventBus(), sender, protector)
	handler := NewNotificationHandler(pool, engine, protector)
	if !handler.Ready() {
		t.Fatal("notification handler is not ready")
	}

	withTenant := func(orgID string, callback func(context.Context)) {
		t.Helper()
		if err := database.WithTenantConnection(ctx, pool, orgID, func(tenantCtx context.Context) error {
			tenantCtx = context.WithValue(tenantCtx, middleware.ContextKeyOrgID, orgID)
			tenantCtx = context.WithValue(tenantCtx, middleware.ContextKeyUserID, userA)
			callback(tenantCtx)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	request := func(requestCtx context.Context, method, path, body string) (*http.Request, *httptest.ResponseRecorder) {
		t.Helper()
		result := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body)).WithContext(requestCtx)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		return req, result
	}

	withTenant(orgA, func(tenantCtx context.Context) {
		req, response := request(tenantCtx, http.MethodPut, "/notifications/preferences", `{
			"email_enabled":false,"in_app_enabled":true,"slack_enabled":false,
			"digest_frequency":"daily","quiet_hours_start":"22:00",
			"quiet_hours_end":"07:00","quiet_hours_timezone":"UTC"}`)
		handler.UpdatePreferences(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("UpdatePreferences status=%d body=%s", response.Code, response.Body.String())
		}

		req, response = request(tenantCtx, http.MethodGet, "/notifications/preferences", "")
		handler.GetPreferences(response, req)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"digest_frequency":"daily"`) {
			t.Fatalf("GetPreferences status=%d body=%s", response.Code, response.Body.String())
		}
	})

	var channelID string
	withTenant(orgA, func(tenantCtx context.Context) {
		req, response := request(tenantCtx, http.MethodPost, "/settings/notification-channels", `{
			"name":"Platform email","channel_type":"email","config":{},"is_active":true}`)
		handler.CreateChannel(response, req)
		if response.Code != http.StatusCreated {
			t.Fatalf("CreateChannel status=%d body=%s", response.Code, response.Body.String())
		}
		var payload map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		channelID = payload["id"]

		req, response = requestWithRouteIDBody(t, tenantCtx, http.MethodPut,
			"/settings/notification-channels/"+channelID, channelID,
			`{"name":"Primary platform email","channel_type":"email","config":{},"is_active":true}`)
		handler.UpdateChannel(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("UpdateChannel status=%d body=%s", response.Code, response.Body.String())
		}

		req, response = request(tenantCtx, http.MethodPost, "/settings/notification-channels", `{
			"name":"Unsafe","channel_type":"webhook",
			"config":{"url":"http://169.254.169.254/latest/meta-data","secret":"01234567890123456789012345678901"}}`)
		handler.CreateChannel(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("unsafe CreateChannel status=%d body=%s", response.Code, response.Body.String())
		}

		webhookSecret := "01234567890123456789012345678901"
		req, response = request(tenantCtx, http.MethodPost, "/settings/notification-channels", `{
			"name":"Secure webhook","channel_type":"webhook",
			"config":{"url":"https://webhooks.example.com/tenant/token123","secret":"`+webhookSecret+`"}}`)
		handler.CreateChannel(response, req)
		if response.Code != http.StatusCreated {
			t.Fatalf("secure CreateChannel status=%d body=%s", response.Code, response.Body.String())
		}
		var webhookResult map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &webhookResult); err != nil {
			t.Fatal(err)
		}
		var storedConfig string
		if err := database.QuerierFromContext(tenantCtx, pool).QueryRow(tenantCtx,
			`SELECT configuration::text FROM notification_channels WHERE id=$1`, webhookResult["id"]).Scan(&storedConfig); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(storedConfig, webhookSecret) || strings.Contains(storedConfig, "token123") || !strings.Contains(storedConfig, `"sealed"`) {
			t.Fatalf("channel configuration is not sealed: %s", storedConfig)
		}

		req, response = request(tenantCtx, http.MethodGet, "/settings/notification-channels", "")
		handler.ListChannels(response, req)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), webhookSecret) || strings.Contains(response.Body.String(), "token123") {
			t.Fatalf("ListChannels leaked credentials: status=%d body=%s", response.Code, response.Body.String())
		}
	})

	var templateID string
	withTenant(orgA, func(tenantCtx context.Context) {
		req, response := request(tenantCtx, http.MethodPost, "/settings/notification-templates", `{
			"name":"Handler test","event_type":"risk.escalated",
			"subject_template":"Risk {{.risk_title}}",
			"body_text_template":"Review {{.risk_title}}",
			"body_html_template":"<p>Review {{.risk_title}}</p>",
			"variables":["risk_title"]}`)
		handler.CreateTemplate(response, req)
		if response.Code != http.StatusCreated {
			t.Fatalf("CreateTemplate status=%d body=%s", response.Code, response.Body.String())
		}
		var created map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		templateID = created["id"]

		req, response = request(tenantCtx, http.MethodGet, "/settings/notification-templates", "")
		handler.ListTemplates(response, req)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), templateID) {
			t.Fatalf("ListTemplates status=%d body=%s", response.Code, response.Body.String())
		}

		req, response = requestWithRouteIDBody(t, tenantCtx, http.MethodPut,
			"/settings/notification-templates/"+templateID, templateID, `{
			"name":"Handler risk escalation","event_type":"risk.escalated",
			"subject_template":"Risk {{.risk_title}}",
			"body_text_template":"Review risk {{.risk_title}}",
			"variables":["risk_title"]}`)
		handler.UpdateTemplate(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("UpdateTemplate status=%d body=%s", response.Code, response.Body.String())
		}
	})
	var ruleID string
	withTenant(orgA, func(tenantCtx context.Context) {
		body := `{"name":"Critical risk","event_type":"risk.escalated","severity_filter":["critical"],` +
			`"conditions":{"requires_review":true},"channel_ids":["` + channelID + `"],` +
			`"recipient_type":"user","recipient_ids":["` + userA + `"],` +
			`"template_id":"` + templateID + `","is_active":true,"cooldown_minutes":60,` +
			`"escalation_after_minutes":15,"escalation_channel_ids":["` + channelID + `"]}`
		req, response := request(tenantCtx, http.MethodPost, "/settings/notification-rules", body)
		handler.CreateRule(response, req)
		if response.Code != http.StatusCreated {
			t.Fatalf("CreateRule status=%d body=%s", response.Code, response.Body.String())
		}
		var createdRule map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &createdRule); err != nil {
			t.Fatal(err)
		}
		ruleID = createdRule["id"]

		req, response = request(tenantCtx, http.MethodGet, "/settings/notification-rules", "")
		handler.ListRules(response, req)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), channelID) ||
			!strings.Contains(response.Body.String(), `"escalation_after_minutes":15`) {
			t.Fatalf("ListRules status=%d body=%s", response.Code, response.Body.String())
		}

		req, response = requestWithRouteID(t, tenantCtx, http.MethodPost,
			"/settings/notification-channels/"+channelID+"/test", channelID)
		handler.TestChannel(response, req)
		if response.Code != http.StatusOK || len(sender.messages) != 1 {
			t.Fatalf("TestChannel status=%d messages=%d body=%s", response.Code, len(sender.messages), response.Body.String())
		}

		req, response = requestWithRouteID(t, tenantCtx, http.MethodDelete,
			"/settings/notification-channels/"+channelID, channelID)
		handler.DeleteChannel(response, req)
		if response.Code != http.StatusConflict {
			t.Fatalf("DeleteChannel while referenced status=%d body=%s", response.Code, response.Body.String())
		}
		req, response = requestWithRouteID(t, tenantCtx, http.MethodDelete,
			"/settings/notification-templates/"+templateID, templateID)
		handler.DeleteTemplate(response, req)
		if response.Code != http.StatusConflict {
			t.Fatalf("DeleteTemplate while referenced status=%d body=%s", response.Code, response.Body.String())
		}

		req, response = requestWithRouteID(t, tenantCtx, http.MethodDelete,
			"/settings/notification-rules/"+ruleID, ruleID)
		handler.DeleteRule(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("DeleteRule status=%d body=%s", response.Code, response.Body.String())
		}
		req, response = requestWithRouteID(t, tenantCtx, http.MethodDelete,
			"/settings/notification-templates/"+templateID, templateID)
		handler.DeleteTemplate(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("DeleteTemplate status=%d body=%s", response.Code, response.Body.String())
		}
		req, response = requestWithRouteID(t, tenantCtx, http.MethodDelete,
			"/settings/notification-channels/"+channelID, channelID)
		handler.DeleteChannel(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("DeleteChannel status=%d body=%s", response.Code, response.Body.String())
		}
	})

	notificationID := uuid.NewString()
	eventID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO notifications
		(id,organization_id,event_id,event_type,event_payload,recipient_user_id,channel_type,
		 subject,body,body_text,status,delivery_key,scheduled_for)
		VALUES ($1::uuid,$2,$4,'system.test','{}',$3,'in_app','Test','Body','Body','delivered',
		        encode(digest(($1::uuid)::text,'sha256'),'hex'),NOW())`,
		notificationID, orgA, userA, eventID); err != nil {
		t.Fatal(err)
	}
	withTenant(orgB, func(tenantCtx context.Context) {
		req, response := requestWithRouteID(t, tenantCtx, http.MethodPut,
			"/notifications/"+notificationID+"/read", notificationID)
		handler.MarkAsRead(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant MarkAsRead status=%d body=%s", response.Code, response.Body.String())
		}
		req, response = requestWithRouteID(t, tenantCtx, http.MethodPut,
			"/notifications/"+notificationID+"/acknowledge", notificationID)
		handler.Acknowledge(response, req)
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant Acknowledge status=%d body=%s", response.Code, response.Body.String())
		}
	})
	withTenant(orgA, func(tenantCtx context.Context) {
		req, response := requestWithRouteID(t, tenantCtx, http.MethodPut,
			"/notifications/"+notificationID+"/read", notificationID)
		handler.MarkAsRead(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("MarkAsRead status=%d body=%s", response.Code, response.Body.String())
		}
		req, response = requestWithRouteID(t, tenantCtx, http.MethodPut,
			"/notifications/"+notificationID+"/acknowledge", notificationID)
		handler.Acknowledge(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("Acknowledge status=%d body=%s", response.Code, response.Body.String())
		}
		var acknowledgedAt *time.Time
		if err := database.QuerierFromContext(tenantCtx, pool).QueryRow(tenantCtx,
			`SELECT acknowledged_at FROM notifications WHERE id=$1`, notificationID).Scan(&acknowledgedAt); err != nil || acknowledgedAt == nil {
			t.Fatalf("acknowledged_at=%v err=%v", acknowledgedAt, err)
		}
	})
}

func requestWithRouteID(
	t *testing.T,
	ctx context.Context,
	method, path, id string,
) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", id)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeContext)
	return httptest.NewRequest(method, path, nil).WithContext(ctx), httptest.NewRecorder()
}

func requestWithRouteIDBody(
	t *testing.T,
	ctx context.Context,
	method, path, id, body string,
) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", id)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeContext)
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	return request, httptest.NewRecorder()
}
