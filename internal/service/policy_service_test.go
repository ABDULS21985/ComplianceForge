package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/complianceforge/platform/internal/models"
)

const (
	policyTestOrgID      = "11000000-0000-0000-0000-000000000001"
	policyTestUserID     = "22000000-0000-0000-0000-000000000002"
	policyTestApproverID = "33000000-0000-0000-0000-000000000003"
	policyTestID         = "44000000-0000-0000-0000-000000000004"
	policyTestVersionID  = "55000000-0000-0000-0000-000000000005"
	policyTestReviewID   = "66000000-0000-0000-0000-000000000006"
	policyTestException  = "77000000-0000-0000-0000-000000000007"
)

type policyRepositoryStub struct {
	PolicyManagementRepository
	policy            *models.Policy
	workflow          *models.PolicyApprovalWorkflow
	review            *models.PolicyReview
	exception         *models.PolicyException
	createErr         error
	createdInput      models.PolicyCreateInput
	createdWordCount  int
	decisionInput     models.PolicyApprovalDecisionInput
	exceptionDecision models.PolicyExceptionDecisionInput
	attestationInput  models.PolicyAttestationInput
	submitInput       models.PolicySubmitInput
	updateCalls       int
}

func (s *policyRepositoryStub) Create(_ context.Context, _, _ string, input models.PolicyCreateInput, words int) (*models.Policy, error) {
	s.createdInput, s.createdWordCount = input, words
	if s.createErr != nil {
		return nil, s.createErr
	}
	return s.policy, nil
}

func (s *policyRepositoryStub) GetByID(context.Context, string, string) (*models.Policy, error) {
	if s.policy == nil {
		return nil, pgx.ErrNoRows
	}
	copy := *s.policy
	return &copy, nil
}

func (s *policyRepositoryStub) Update(_ context.Context, _ string, policy *models.Policy) (*models.Policy, error) {
	s.updateCalls++
	copy := *policy
	s.policy = &copy
	return &copy, nil
}

func (s *policyRepositoryStub) GetActiveApprovalWorkflow(context.Context, string, string) (*models.PolicyApprovalWorkflow, error) {
	if s.workflow == nil {
		return nil, pgx.ErrNoRows
	}
	return s.workflow, nil
}

func (s *policyRepositoryStub) CreateApprovalWorkflow(_ context.Context, _, _, _ string, input models.PolicySubmitInput) (*models.PolicyApprovalWorkflow, error) {
	s.submitInput = input
	if s.workflow == nil {
		s.workflow = &models.PolicyApprovalWorkflow{WorkflowType: input.WorkflowType, Status: "in_progress"}
	}
	return s.workflow, nil
}

func (s *policyRepositoryStub) DecideApproval(_ context.Context, _, _, _, _ string, input models.PolicyApprovalDecisionInput) (*models.PolicyApprovalWorkflow, error) {
	s.decisionInput = input
	return s.workflow, nil
}

func (s *policyRepositoryStub) Acknowledge(_ context.Context, _, _, _, _ string, input models.PolicyAttestationInput) (*models.PolicyAttestation, error) {
	s.attestationInput = input
	return &models.PolicyAttestation{ID: "88000000-0000-0000-0000-000000000008", Status: "attested"}, nil
}

func (s *policyRepositoryStub) GetReview(context.Context, string, string, string) (*models.PolicyReview, error) {
	if s.review == nil {
		return nil, pgx.ErrNoRows
	}
	copy := *s.review
	return &copy, nil
}

func (s *policyRepositoryStub) UpdateReview(_ context.Context, _, _ string, review *models.PolicyReview) (*models.PolicyReview, error) {
	copy := *review
	return &copy, nil
}

func (s *policyRepositoryStub) GetException(context.Context, string, string, string) (*models.PolicyException, error) {
	if s.exception == nil {
		return nil, pgx.ErrNoRows
	}
	copy := *s.exception
	return &copy, nil
}

func (s *policyRepositoryStub) UpdateExceptionDecision(_ context.Context, _, _, _, _ string, input models.PolicyExceptionDecisionInput) (*models.PolicyException, error) {
	s.exceptionDecision = input
	copy := *s.exception
	copy.Status = input.Decision
	return &copy, nil
}

func newPolicyServiceForTest(repo PolicyManagementRepository) *PolicyService {
	return NewPolicyService(repo, zerolog.Nop())
}

func testPolicy(status string) *models.Policy {
	return &models.Policy{
		TenantModel:                models.TenantModel{BaseModel: models.BaseModel{ID: policyTestID}, OrganizationID: policyTestOrgID},
		PolicyRef:                  "POL-0001",
		Title:                      "Information security policy",
		Status:                     status,
		Classification:             "internal",
		ReviewFrequencyMonths:      12,
		AttestationFrequencyMonths: 12,
		RequiresAttestation:        true,
		Metadata:                   []byte(`{}`),
	}
}

func TestPolicyCreateNormalizesCanonicalDefaults(t *testing.T) {
	repo := &policyRepositoryStub{policy: testPolicy(models.PolicyStateDraft)}
	service := newPolicyServiceForTest(repo)
	content := "read understand comply"

	created, err := service.Create(context.Background(), policyTestOrgID, policyTestUserID, models.PolicyCreateInput{
		Title:          "  Information security policy  ",
		InitialVersion: models.PolicyVersionInput{ContentText: &content},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != policyTestID || repo.createdWordCount != 3 {
		t.Fatalf("created=%#v word_count=%d", created, repo.createdWordCount)
	}
	input := repo.createdInput
	if input.Title != "Information security policy" || input.Classification != "internal" ||
		input.ReviewFrequencyMonths != 12 || input.AttestationFrequencyMonths != 12 ||
		input.AppliesToAll == nil || !*input.AppliesToAll || input.IsMandatory == nil || !*input.IsMandatory ||
		input.RequiresAttestation == nil || !*input.RequiresAttestation || string(input.Metadata) != `{}` ||
		input.InitialVersion.Language != "en" || input.InitialVersion.ChangeType == nil || *input.InitialVersion.ChangeType != "major" {
		t.Fatalf("canonical defaults not applied: %#v", input)
	}
}

func TestPolicyCreateRejectsMissingContentAndMapsReferences(t *testing.T) {
	service := newPolicyServiceForTest(&policyRepositoryStub{})
	_, err := service.Create(context.Background(), policyTestOrgID, policyTestUserID, models.PolicyCreateInput{Title: "No content"})
	if !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("missing content error=%v", err)
	}

	content := "valid"
	repo := &policyRepositoryStub{createErr: pgx.ErrNoRows}
	service = newPolicyServiceForTest(repo)
	_, err = service.Create(context.Background(), policyTestOrgID, policyTestUserID, models.PolicyCreateInput{Title: "Valid", InitialVersion: models.PolicyVersionInput{ContentText: &content}})
	if !errors.Is(err, ErrPolicyInvalidReference) {
		t.Fatalf("reference error=%v", err)
	}
}

func TestPolicyPublishedContentIsImmutableButOwnerCanChange(t *testing.T) {
	repo := &policyRepositoryStub{policy: testPolicy(models.PolicyStatePublished)}
	service := newPolicyServiceForTest(repo)
	title := "Mutated"
	if _, err := service.Update(context.Background(), policyTestOrgID, policyTestID, models.PolicyPatch{Title: &title}); !errors.Is(err, ErrPolicyInvalidTransition) {
		t.Fatalf("published update error=%v", err)
	}
	owner := policyTestApproverID
	updated, err := service.AssignOwner(context.Background(), policyTestOrgID, policyTestID, &owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.OwnerUserID == nil || *updated.OwnerUserID != owner || repo.updateCalls != 1 {
		t.Fatalf("updated=%#v calls=%d", updated, repo.updateCalls)
	}
}

func TestPolicyApprovalDecisionRequiresCurrentAssignee(t *testing.T) {
	assigned := policyTestApproverID
	repo := &policyRepositoryStub{workflow: &models.PolicyApprovalWorkflow{
		CurrentStep: 1,
		Steps:       []models.PolicyApprovalStep{{ApproverUserID: &assigned, Status: "pending"}},
	}}
	service := newPolicyServiceForTest(repo)

	_, err := service.DecideApproval(context.Background(), policyTestOrgID, policyTestID, policyTestUserID, "compliance_manager", models.PolicyApprovalDecisionInput{Decision: "approve"})
	if !errors.Is(err, ErrPolicyApprovalForbidden) {
		t.Fatalf("unassigned decision error=%v", err)
	}
	result, err := service.DecideApproval(context.Background(), policyTestOrgID, policyTestID, assigned, "compliance_manager", models.PolicyApprovalDecisionInput{Decision: "approve"})
	if err != nil || result == nil || repo.decisionInput.Decision != "approved" {
		t.Fatalf("result=%#v decision=%#v err=%v", result, repo.decisionInput, err)
	}
}

func TestPolicyRetirementUsesApprovalWorkflow(t *testing.T) {
	repo := &policyRepositoryStub{policy: testPolicy(models.PolicyStatePublished)}
	service := newPolicyServiceForTest(repo)
	workflow, err := service.SubmitForApproval(context.Background(), policyTestOrgID, policyTestID, policyTestUserID, models.PolicySubmitInput{
		WorkflowType: "retirement", Approvers: []models.PolicyApprovalStepInput{{ApproverUserID: stringPointerForPolicyTest(policyTestApproverID)}},
	})
	if err != nil || workflow.WorkflowType != "retirement" || repo.submitInput.WorkflowType != "retirement" {
		t.Fatalf("workflow=%#v input=%#v err=%v", workflow, repo.submitInput, err)
	}
}

func TestPolicyAcknowledgementDefaultsAndValidatesDecline(t *testing.T) {
	repo := &policyRepositoryStub{policy: testPolicy(models.PolicyStatePublished)}
	service := newPolicyServiceForTest(repo)
	item, err := service.Acknowledge(context.Background(), policyTestOrgID, policyTestID, policyTestUserID, "127.0.0.1", models.PolicyAttestationInput{Decision: "attest"})
	if err != nil || item.Status != "attested" || repo.attestationInput.AttestationMethod != "digital_click" || string(repo.attestationInput.Metadata) != `{}` {
		t.Fatalf("item=%#v input=%#v err=%v", item, repo.attestationInput, err)
	}
	_, err = service.Acknowledge(context.Background(), policyTestOrgID, policyTestID, policyTestUserID, "", models.PolicyAttestationInput{Decision: "decline"})
	if !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("decline error=%v", err)
	}
}

func TestPolicyReviewAndExceptionTransitions(t *testing.T) {
	outcome := "no_change"
	complete := "completed"
	repo := &policyRepositoryStub{
		policy:    testPolicy(models.PolicyStatePublished),
		review:    &models.PolicyReview{BaseModel: models.BaseModel{ID: policyTestReviewID}, Status: "in_progress"},
		exception: &models.PolicyException{BaseModel: models.BaseModel{ID: policyTestException}, Status: "requested"},
	}
	service := newPolicyServiceForTest(repo)
	review, err := service.UpdateReview(context.Background(), policyTestOrgID, policyTestID, policyTestReviewID, models.PolicyReviewPatch{Status: &complete, Outcome: &outcome})
	if err != nil || review.Status != "completed" {
		t.Fatalf("review=%#v err=%v", review, err)
	}

	_, err = service.DecideException(context.Background(), policyTestOrgID, policyTestID, policyTestException, policyTestApproverID, models.PolicyExceptionDecisionInput{Decision: "approve"})
	if !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("approval without expiry error=%v", err)
	}
	expiry := time.Now().UTC().AddDate(0, 1, 0)
	exception, err := service.DecideException(context.Background(), policyTestOrgID, policyTestID, policyTestException, policyTestApproverID, models.PolicyExceptionDecisionInput{Decision: "approve", ExpiryDate: &expiry})
	if err != nil || exception.Status != "approved" || repo.exceptionDecision.Decision != "approved" {
		t.Fatalf("exception=%#v input=%#v err=%v", exception, repo.exceptionDecision, err)
	}
}

func stringPointerForPolicyTest(value string) *string { return &value }
