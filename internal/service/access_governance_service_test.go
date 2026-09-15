package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/complianceforge/platform/internal/models"
	"github.com/complianceforge/platform/internal/repository"
)

type governanceTestStore struct {
	repository.AccessGovernanceRepository
	called bool
	input  models.AccessReviewCampaignInput
}

func (s *governanceTestStore) CreateCampaign(_ context.Context, _, _ string, in models.AccessReviewCampaignInput) (*models.AccessReviewCampaign, error) {
	s.called = true
	s.input = in
	return &models.AccessReviewCampaign{ID: uuid.NewString()}, nil
}
func TestGovernanceClockBoundsAndSeparation(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	org, actor, reviewer, subject := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	in := models.AccessReviewCampaignInput{Name: "Quarterly review", ReviewerID: reviewer, SubjectIDs: []string{subject}, DueAt: now.Add(time.Hour), Reason: "Quarterly certification"}
	for _, tt := range []struct {
		name   string
		mutate func(*models.AccessReviewCampaignInput)
	}{
		{"self-review", func(v *models.AccessReviewCampaignInput) { v.SubjectIDs = []string{reviewer} }},
		{"sponsor-review", func(v *models.AccessReviewCampaignInput) { v.ReviewerID = actor }},
		{"past", func(v *models.AccessReviewCampaignInput) { v.DueAt = now.Add(-time.Second) }},
		{"unbounded", func(v *models.AccessReviewCampaignInput) { v.DueAt = now.Add(91 * 24 * time.Hour) }},
		{"duplicate-subject", func(v *models.AccessReviewCampaignInput) { v.SubjectIDs = []string{subject, subject} }},
		{"nil-uuid", func(v *models.AccessReviewCampaignInput) { v.ReviewerID = uuid.Nil.String() }},
		{"uppercase-uuid", func(v *models.AccessReviewCampaignInput) { v.ReviewerID = strings.ToUpper(reviewer) }},
		{"urn-uuid", func(v *models.AccessReviewCampaignInput) { v.ReviewerID = "urn:uuid:" + reviewer }},
		{"compact-uuid", func(v *models.AccessReviewCampaignInput) { v.ReviewerID = strings.ReplaceAll(reviewer, "-", "") }},
		{"braces-uuid", func(v *models.AccessReviewCampaignInput) { v.ReviewerID = "{" + reviewer + "}" }},
		{"no-reason", func(v *models.AccessReviewCampaignInput) { v.Reason = " " }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := new(governanceTestStore)
			app, _ := NewAccessGovernanceService(store, func() time.Time { return now })
			v := in
			tt.mutate(&v)
			if _, err := app.CreateCampaign(context.Background(), org, actor, v); err == nil || store.called {
				t.Fatalf("invalid request reached store: err=%v called=%v", err, store.called)
			}
		})
	}
	store := new(governanceTestStore)
	app, _ := NewAccessGovernanceService(store, func() time.Time { return now })
	if _, err := app.CreateCampaign(context.Background(), org, actor, in); err != nil || !store.called {
		t.Fatalf("valid campaign err=%v called=%v", err, store.called)
	}
	for _, end := range []time.Time{now, now.Add(-time.Second), now.Add(91 * 24 * time.Hour)} {
		if governanceWindow(now, now, end) {
			t.Fatalf("invalid window accepted %v", end)
		}
	}
}
func TestGovernanceStrictDecisionsAndSafeErrors(t *testing.T) {
	store := new(governanceTestStore)
	app, _ := NewAccessGovernanceService(store, nil)
	id := uuid.NewString()
	for _, in := range []models.AccessReviewDecisionInput{
		{Decision: "approve", Reason: "Not a supported decision", RequestID: "request", SnapshotSHA256: strings.Repeat("a", 64)},
		{Decision: "retain", Reason: "Business need", RequestID: "request", SnapshotSHA256: strings.Repeat("a", 63)},
		{Decision: "retain", Reason: "Business need", RequestID: strings.Repeat("x", 65), SnapshotSHA256: strings.Repeat("a", 64)},
	} {
		if _, err := app.DecideItem(context.Background(), id, id, id, id, in); !errors.Is(err, ErrGovernanceInvalid) {
			t.Fatalf("invalid decision err=%v", err)
		}
	}
	if !errors.Is(governanceError(repository.ErrAccessGovernanceConflict), ErrGovernanceConflict) || !errors.Is(governanceError(repository.ErrAccessGovernanceSeparation), ErrGovernanceSeparation) {
		t.Fatal("unsafe error mapping")
	}
	if got := governancePagination(models.PaginationRequest{Page: -1, PageSize: 1000}); got.Page != 1 || got.PageSize != 100 {
		t.Fatalf("pagination=%+v", got)
	}
}
