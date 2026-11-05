package core

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/production"
)

type controlledAutonomyAuthorityProvider struct {
	result production.ProviderResult
}

func (p controlledAutonomyAuthorityProvider) ID() string { return "core.local_action" }
func (p controlledAutonomyAuthorityProvider) Execute(_ context.Context, _ production.ProviderRequest) production.ProviderResult {
	return p.result
}

type authorityResultCapture struct {
	result autonomy.VerificationResult
	calls  int
}

func (c *authorityResultCapture) RecordOutcome(_ context.Context, _ autonomy.ActionRequest, result autonomy.VerificationResult) error {
	c.result = result
	c.calls++
	return nil
}
func (c *authorityResultCapture) DiagnoseFailure(_ context.Context, _ autonomy.ActionRequest, result autonomy.VerificationResult) error {
	c.result = result
	c.calls++
	return nil
}

type authorityResultUpdateCapture struct {
	result autonomy.VerificationResult
	calls  int
}

func (c *authorityResultUpdateCapture) UpdateState(_ context.Context, _ autonomy.ActionRequest, result autonomy.VerificationResult) error {
	c.result = result
	c.calls++
	return nil
}

func TestAutonomyAuthorityResultSurvivesEngineExecutorCycleAndVerifier(t *testing.T) {
	cases := []struct {
		name                 string
		provider             production.ProviderResult
		wantSuccess          bool // Cycle verification result, not provider business success.
		wantAuthoritySuccess bool
		wantObserved         bool
		wantVerified         bool
		wantSummary          string
		wantFailure          string
		wantOutcome          production.OutcomeStatus
		wantCode             string
	}{
		{
			name:        "succeeded and verified",
			provider:    production.ProviderResult{Success: true, OutcomeStatus: production.OutcomeSucceeded, Verified: true, Observed: true, Summary: "confirmed effect", OutcomeCode: "confirmed"},
			wantSuccess: true, wantAuthoritySuccess: true, wantObserved: true, wantVerified: true,
			wantSummary: "confirmed effect", wantOutcome: production.OutcomeSucceeded, wantCode: "confirmed",
		},
		{
			name:        "business failure despite observation",
			provider:    production.ProviderResult{Success: false, OutcomeStatus: production.OutcomeFailed, Verified: true, Observed: true, FailureReason: "operation rejected", OutcomeCode: "failed_observed"},
			wantSuccess: false, wantAuthoritySuccess: false, wantObserved: true, wantVerified: true,
			wantFailure: "operation rejected", wantOutcome: production.OutcomeFailed, wantCode: "failed_observed",
		},
		{
			name:        "unknown outcome without observation",
			provider:    production.ProviderResult{Success: true, OutcomeStatus: production.OutcomeUnknown, Verified: false, Observed: false, Summary: "effect uncertain", OutcomeCode: "unknown"},
			wantSuccess: false, wantAuthoritySuccess: true, wantObserved: false, wantVerified: false,
			wantSummary: "effect uncertain", wantFailure: "execution_observed_but_outcome_unverified", wantOutcome: production.OutcomeUnknown, wantCode: "unknown",
		},
		{
			name:        "contradictory result",
			provider:    production.ProviderResult{Success: false, OutcomeStatus: production.OutcomeFailed, Verified: false, Observed: true, FailureReason: "observed postcondition contradicts request", OutcomeCode: "contradicted"},
			wantSuccess: false, wantAuthoritySuccess: false, wantObserved: true, wantVerified: false,
			wantFailure: "observed postcondition contradicts request", wantOutcome: production.OutcomeFailed, wantCode: "contradicted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newB963334AuditEngine(t)
			if err := e.productionProviders.RegisterCapabilities(controlledAutonomyAuthorityProvider{result: tc.provider}, capability.ID("crear_carpeta")); err != nil {
				t.Fatalf("replace local provider with controlled authority provider: %v", err)
			}

			capture := &authorityResultCapture{}
			updateCapture := &authorityResultUpdateCapture{}
			planner := &onceActionPlanner{req: autonomy.ActionRequest{ID: "task-causal-1", Action: "crear_carpeta", Target: "controlled-target"}}
			cycle := autonomy.NewCycle(autonomy.New(), nil, nil, planner, NewAutonomyExecutor(e), nil, capture, capture, updateCapture)
			if err := cycle.RunStep(context.Background()); err != nil {
				t.Fatalf("RunStep() error: %v", err)
			}
			if updateCapture.calls != 1 {
				t.Fatalf("expected exactly one state-update handoff, got %d", updateCapture.calls)
			}
			wantDiagnosticOrLearning := tc.wantOutcome != production.OutcomeUnknown
			if wantDiagnosticOrLearning && capture.calls != 1 {
				t.Fatalf("expected one diagnosis/learning handoff, got %d", capture.calls)
			}
			if !wantDiagnosticOrLearning && capture.calls != 0 {
				t.Fatalf("unknown outcome must not be treated as known failure or success learning, got %d calls", capture.calls)
			}
			result := updateCapture.result.AuthorityResult
			if result == nil {
				t.Fatal("Cycle/Verifier discarded the structured authority result")
			}
			if _, ok := result.(production.AuthorityExecutionResult); !ok {
				t.Fatalf("expected the canonical production.AuthorityExecutionResult to cross the boundary intact, got %T", result)
			}
			if got := result.AuthorityOutcomeStatus(); got != string(tc.wantOutcome) {
				t.Errorf("authority outcome status=%q, want %q", got, tc.wantOutcome)
			}
			if got := result.AuthorityOutcomeCode(); got != tc.wantCode {
				t.Errorf("authority outcome code=%q, want %q", got, tc.wantCode)
			}
			if !result.AuthorityHandled() || !result.AuthorityExecuted() {
				t.Errorf("authority control/execution facts were lost: handled=%v executed=%v", result.AuthorityHandled(), result.AuthorityExecuted())
			}
			if got := result.AuthoritySucceeded(); got != tc.wantAuthoritySuccess {
				t.Errorf("authority succeeded=%v, want %v", got, tc.wantAuthoritySuccess)
			}
			if got := result.AuthorityObserved(); got != tc.wantObserved {
				t.Errorf("authority observed=%v, want %v", got, tc.wantObserved)
			}
			if got := result.AuthorityVerified(); got != tc.wantVerified {
				t.Errorf("authority verified=%v, want %v", got, tc.wantVerified)
			}
			if got := result.AuthoritySummary(); got != tc.wantSummary {
				t.Errorf("authority summary=%q, want %q", got, tc.wantSummary)
			}
			if got := result.AuthorityFailureReason(); got != tc.wantFailure {
				t.Errorf("authority failure reason=%q, want %q", got, tc.wantFailure)
			}
			if updateCapture.result.Success != tc.wantSuccess {
				t.Errorf("verification success=%v, want %v (authority succeeded=%v verified=%v)", updateCapture.result.Success, tc.wantSuccess, result.AuthoritySucceeded(), result.AuthorityVerified())
			}
			if result.AuthorityAttemptNumber() < 0 {
				t.Errorf("invalid authority attempt number: %d", result.AuthorityAttemptNumber())
			}
		})
	}
}
