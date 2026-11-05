package integration

import (
	"context"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/tools"
)

type executionSucceedsVerificationFailsProvider struct{}

func (executionSucceedsVerificationFailsProvider) ID() string { return "behavioral-failure" }
func (executionSucceedsVerificationFailsProvider) Execute(context.Context, production.ProviderRequest) production.ProviderResult {
	return production.ProviderResult{Success: true, Observed: true, Verified: false, Summary: "ejecutado pero no verificado"}
}

type executionSucceedsWithoutObservationProvider struct{}

func (executionSucceedsWithoutObservationProvider) ID() string { return "missing-observation" }
func (executionSucceedsWithoutObservationProvider) Execute(context.Context, production.ProviderRequest) production.ProviderResult {
	return production.ProviderResult{Success: true, Observed: false, Verified: true, Summary: "ejecutado sin evidencia de observación"}
}

func newBehavioralClosureRuntime(t *testing.T, provider production.Provider) (*ProductionRuntime, *capability.Registry) {
	t.Helper()
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "demo.behavioral", Domain: capability.DomainLocal, Action: "behavioral_action", Description: "behavioral closure", Permission: capability.PermissionAutomatic}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	store := production.NewWorkStore(filepath.Join(t.TempDir(), "work.json"))
	runtime, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, store, production.NewTechnicalRecoveryServiceWithDecider(fixedRecoveryDecider{action: tools.RecoveryRetry}))
	if err != nil {
		t.Fatal(err)
	}
	return runtime, reg
}

func behavioralRequest(t *testing.T, reg *capability.Registry) (capability.Route, interaction.SemanticRequest) {
	t.Helper()
	req := productionTestRequest()
	req.Action.Name = "behavioral_action"
	req.RequestedAction = "behavioral_action"
	decision := interaction.ResolveCapabilityInteraction(req)
	route := capability.NewResolver(reg).Resolve(decision, req)
	return route, req
}

func TestProductionRuntimeDoesNotCompleteWorkWhenVerificationFails(t *testing.T) {
	runtime, reg := newBehavioralClosureRuntime(t, executionSucceedsVerificationFailsProvider{})
	route, req := behavioralRequest(t, reg)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Success || result.Verified || result.OutcomeStatus != production.OutcomeUnknown {
		t.Fatalf("unverified execution was not preserved as unknown: %+v", result)
	}
	if result.Recovery != nil {
		t.Fatalf("unknown verification must not trigger recovery by itself: %+v", result.Recovery)
	}
	work, ok := runtime.work.Get(result.WorkID)
	if !ok {
		t.Fatal("work was not persisted")
	}
	if work.Status == production.WorkCompleted || work.Status == production.WorkFailed {
		t.Fatalf("work was closed despite unknown verification: %+v", work)
	}
	if len(work.Attempts) != 1 || work.Attempts[0].Status != "unverified" {
		t.Fatalf("unknown attempt should be durably marked unverified while Work remains open: %+v", work.Attempts)
	}
	if result.AttemptNumber != work.Attempts[0].Number || result.AttemptNumber != 1 {
		t.Fatalf("result lost exact attempt identity: result attempt=%d stored=%d", result.AttemptNumber, work.Attempts[0].Number)
	}
	if _, err := runtime.work.BeginAttempt(result.WorkID, "demo.behavioral", "behavioral_action", "behavioral-failure"); err == nil {
		t.Fatal("a new attempt was allowed before the unknown effect was reconciled")
	}
	reconciled := production.ActionResult{
		Success: false, OutcomeStatus: production.OutcomeFailed, Observed: true, Verified: false,
		WorkID: result.WorkID, AttemptNumber: result.AttemptNumber,
		ObservationDetails: "the expected effect is absent", FailureReason: "reconciled_effect_absent",
	}
	if err := runtime.work.ReconcileUnknownAttempt(result.WorkID, result.AttemptNumber, reconciled); err != nil {
		t.Fatalf("explicit evidence-based reconciliation failed: %v", err)
	}
	work, _ = runtime.work.Get(result.WorkID)
	if work.Attempts[0].Status != "failed" {
		t.Fatalf("reconciled attempt status=%q, want failed", work.Attempts[0].Status)
	}
	if _, err := runtime.work.BeginAttempt(result.WorkID, "demo.behavioral", "behavioral_action", "behavioral-failure"); err != nil {
		t.Fatalf("known reconciled outcome should permit a new planned attempt: %v", err)
	}
}

func TestProductionRuntimeRejectsVerificationWithoutObservation(t *testing.T) {
	runtime, reg := newBehavioralClosureRuntime(t, executionSucceedsWithoutObservationProvider{})
	route, req := behavioralRequest(t, reg)
	result := runtime.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if !result.Success || result.Verified || result.OutcomeStatus != production.OutcomeUnknown {
		t.Fatalf("verification without observation was not preserved as unknown: %+v", result)
	}
	if result.FailureReason != "execution_observed_but_outcome_unverified" {
		t.Fatalf("unexpected unknown reason: %q", result.FailureReason)
	}
}
