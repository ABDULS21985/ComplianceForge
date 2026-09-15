package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/service"
)

type governanceHandlerTestService struct {
	AccessGovernanceHandlerService
	called bool
	err    error
}

func (s *governanceHandlerTestService) CreateCampaign(context.Context, string, string, models.AccessReviewCampaignInput) (*models.AccessReviewCampaign, error) {
	s.called = true
	return nil, s.err
}
func TestGovernanceHandlerStrictJSON(t *testing.T) {
	for _, body := range []string{`{"unknown":true}`, `{} {}`, strings.Repeat(" ", 1024*1024) + `{}`} {
		t.Run(body[:1], func(t *testing.T) {
			s := new(governanceHandlerTestService)
			h := NewAccessGovernanceHandler(s)
			w := httptest.NewRecorder()
			h.CreateCampaign(w, httptest.NewRequest(http.MethodPost, "/access/reviews", strings.NewReader(body)))
			if w.Code != 400 || s.called {
				t.Fatalf("status=%d called=%v", w.Code, s.called)
			}
		})
	}
}
func TestGovernanceHandlerErrorMapping(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
	}{{service.ErrGovernanceInvalid, 400}, {service.ErrGovernanceNotFound, 404}, {service.ErrGovernanceConflict, 409}, {service.ErrGovernanceSeparation, 403}, {service.ErrLastTenantAdministrator, 409}} {
		w := httptest.NewRecorder()
		writeGovernanceError(w, httptest.NewRequest("GET", "/access/reviews", nil), tt.err)
		if w.Code != tt.status {
			t.Fatalf("err=%v status=%d want=%d", tt.err, w.Code, tt.status)
		}
	}
}
