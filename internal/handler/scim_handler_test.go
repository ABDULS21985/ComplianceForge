package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/complianceforge/platform/internal/models"
	protocol "github.com/complianceforge/platform/internal/scim"
	"github.com/complianceforge/platform/internal/service"
)

type scimHandlerServiceStub struct {
	SCIMHandlerService
	user *models.SCIMUser
	err  error
}

func (s scimHandlerServiceStub) GetUser(context.Context, string, string) (*models.SCIMUser, error) {
	return s.user, s.err
}

func TestSCIMHandlerETagAndRFCErrorContracts(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	user := &models.SCIMUser{Schemas: []string{models.SCIMUserSchema}, ID: "92000000-0000-0000-0000-000000000001",
		UserName: "user@example.test", Active: true, Version: 7,
		Meta: models.SCIMMeta{Created: now, LastModified: now, Location: "/api/scim/v2/Users/92000000-0000-0000-0000-000000000001", Version: protocol.VersionETag(7)}}
	handler := NewSCIMHandler(scimHandlerServiceStub{user: user})
	router := chi.NewRouter()
	router.Get("/Users/{id}", handler.GetUser)

	request := httptest.NewRequest(http.MethodGet, "/Users/"+user.ID, nil)
	request.Header.Set("If-None-Match", `W/"7"`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotModified || response.Header().Get("ETag") != `W/"7"` {
		t.Fatalf("status=%d etag=%q body=%s", response.Code, response.Header().Get("ETag"), response.Body.String())
	}

	errorHandler := NewSCIMHandler(scimHandlerServiceStub{err: service.ErrSCIMVersion})
	errorRouter := chi.NewRouter()
	errorRouter.Get("/Users/{id}", errorHandler.GetUser)
	errorResponse := httptest.NewRecorder()
	errorRouter.ServeHTTP(errorResponse, httptest.NewRequest(http.MethodGet, "/Users/"+user.ID, nil))
	var problem models.SCIMError
	if errorResponse.Code != http.StatusPreconditionFailed || errorResponse.Header().Get("Content-Type") != scimMediaType ||
		json.Unmarshal(errorResponse.Body.Bytes(), &problem) != nil || problem.Schemas[0] != models.SCIMErrorSchema {
		t.Fatalf("status=%d problem=%#v body=%s", errorResponse.Code, problem, errorResponse.Body.String())
	}
}

func TestSCIMHandlerRequiresMediaTypeIfMatchAndBoundedJSON(t *testing.T) {
	handler := NewSCIMHandler(scimHandlerServiceStub{})
	for _, test := range []struct {
		name, method, contentType, ifMatch, body string
		want                                     int
	}{
		{name: "wrong media", method: http.MethodPost, contentType: "application/json", body: `{}`, want: http.StatusUnsupportedMediaType},
		{name: "missing precondition", method: http.MethodPut, contentType: scimMediaType, body: `{}`, want: http.StatusPreconditionRequired},
		{name: "oversized", method: http.MethodPost, contentType: scimMediaType,
			body: `{"schemas":["` + models.SCIMUserSchema + `"],"userName":"` + strings.Repeat("x", maximumSCIMBodySize) + `"}`, want: http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/Users", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set("If-Match", test.ifMatch)
			response := httptest.NewRecorder()
			if test.method == http.MethodPut {
				handler.ReplaceUser(response, request)
			} else {
				handler.CreateUser(response, request)
			}
			if response.Code != test.want || response.Header().Get("Content-Type") != scimMediaType {
				t.Fatalf("status=%d want=%d content-type=%q body=%s", response.Code, test.want,
					response.Header().Get("Content-Type"), response.Body.String())
			}
		})
	}
}
