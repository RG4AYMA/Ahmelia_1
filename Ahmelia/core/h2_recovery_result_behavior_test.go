package core

import (
	"context"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/tools"
)

type h2RetryDecider struct{}

func (h2RetryDecider) Decide(context.Context, tools.Failure) tools.RecoveryAction {
	return tools.RecoveryRetry
}

type h2RetryThenSuccessProvider struct{ calls int }

func (p *h2RetryThenSuccessProvider) ID() string { return "h2-controlled-provider" }
func (p *h2RetryThenSuccessProvider) Execute(_ context.Context, req production.ProviderRequest) production.ProviderResult {
	p.calls++
	if req.WorkID == "" || req.AttemptNumber != p.calls {
		return production.ProviderResult{Success: false, OutcomeStatus: production.OutcomeFailed, FailureReason: "h2_attempt_identity_mismatch"}
	}
	if p.calls == 1 {
		return production.ProviderResult{
			Success: false, OutcomeStatus: production.OutcomeFailed, Observed: true,
			FailureReason: "h2_first_attempt_failure", OutcomeCode: "h2-attempt-1-failed",
			Summary: "first attempt failed",
		}
	}
	return production.ProviderResult{
		Success: true, OutcomeStatus: production.OutcomeSucceeded, Observed: true, Verified: true,
		OutcomeCode: "h2-attempt-2-succeeded", Summary: "second attempt succeeded",
		Producer: p.ID(),
	}
}

// This exercises one outer Engine authority call. The first provider result
// requests Retry; the nested authority pass succeeds. We then compare the
// nested attempt's durable result with the authority result returned outside.
func TestH2AuthorityReturnsFinalRetryResultToCaller(t *testing.T) {
	e := newB963334AuditEngine(t)

	reg := capability.NewRegistry()
	cap := capability.Capability{
		ID: "h2.production", Domain: capability.DomainLocal, Action: "create",
		Description: "create production artifact", Lane: capability.LaneProduction,
		Permission: capability.PermissionAutomatic,
	}
	if err := reg.Register(cap); err != nil {
		t.Fatal(err)
	}
	provider := &h2RetryThenSuccessProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, cap.ID); err != nil {
		t.Fatal(err)
	}

	store := production.NewWorkStore(filepath.Join(t.TempDir(), "h2-work.json"))
	e.workStore = NewPersistentWorkStore(store)
	e.capabilityRegistry = reg
	e.capabilityResolver = capability.NewResolver(reg)
	e.productionProviders = providers
	runtime, err := integration.NewProductionRuntime(
		e.capabilityResolver, reg, providers, &production.ProductionContext{}, e.workStore,
		production.NewTechnicalRecoveryServiceWithDecider(h2RetryDecider{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	e.productionRuntime = runtime
	req := interaction.SemanticRequest{
		Raw: "crear artefacto para H2", Normalized: "crear artefacto para h2",
		Kind:            interaction.KindProduction,
		Objective:       interaction.Objective{Text: "crear artefacto para H2"},
		Action:          interaction.ActionIntent{Name: "create", Target: "h2-target", Confidence: 1},
		RequestedAction: "create", Topic: "production artifact",
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActCommand, Confidence: 1},
	}
	decision := interaction.ResolveCapabilityInteraction(req)
	if !decision.Executable {
		t.Fatalf("H2 fixture request not executable: %+v", decision)
	}

	initialWork, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	outer := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalGranted,
		RecoveryAttempt: 1, WorkID: initialWork.ID, ExperimentalBypassCognition: true,
	}, req)

	persisted, ok := e.workStore.Get(initialWork.ID)
	if !ok {
		t.Fatalf("canonical Work %q not found", initialWork.ID)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls=%d; want first attempt plus one retry; outer=%+v", provider.calls, outer)
	}
	if len(persisted.Attempts) != 2 {
		t.Fatalf("persisted attempts=%d; want 2: %+v", len(persisted.Attempts), persisted.Attempts)
	}

	first, second := persisted.Attempts[0], persisted.Attempts[1]
	if first.Number != 1 || second.Number != 2 {
		t.Fatalf("WorkAttempt.Number values=(%d,%d); want (1,2)", first.Number, second.Number)
	}
	if first.RecoveryAction != string(tools.RecoveryRetry) {
		t.Fatalf("attempt 1 recovery action=%q; want retry", first.RecoveryAction)
	}
	if first.JobID == "" || second.JobID == "" || first.ExecutionAttemptID == "" || second.ExecutionAttemptID == "" {
		t.Fatalf("attempt execution identities missing: first=%+v second=%+v", first, second)
	}
	if first.JobID == second.JobID || first.ExecutionAttemptID == second.ExecutionAttemptID {
		t.Fatalf("physical execution identities were reused: first=%+v second=%+v", first, second)
	}
	if first.Result == nil || first.Result.OutcomeCode != "h2-attempt-1-failed" || first.Result.OutcomeStatus != production.OutcomeFailed {
		t.Fatalf("attempt 1 result lost: %+v", first.Result)
	}
	if second.Result == nil || second.Result.OutcomeCode != "h2-attempt-2-succeeded" || second.Result.OutcomeStatus != production.OutcomeSucceeded {
		t.Fatalf("attempt 2 result lost: %+v", second.Result)
	}
	if persisted.Status != production.WorkCompleted || persisted.LastActionResult == nil || persisted.LastActionResult.OutcomeCode != "h2-attempt-2-succeeded" {
		t.Fatalf("Work did not persist attempt 2 as final result: status=%q last=%+v", persisted.Status, persisted.LastActionResult)
	}

	// These are the caller-visible fields under test. A failure here records
	// H2 behavior; it deliberately does not change production or mask it.
	if !outer.Handled || outer.OutcomeStatus != production.OutcomeSucceeded || !outer.Succeeded ||
		!outer.Observed || !outer.Verified || outer.AttemptNumber != second.Number ||
		outer.OutcomeCode != "h2-attempt-2-succeeded" || outer.Summary != "second attempt succeeded" {
		t.Fatalf("caller received a result other than the final retry result: outer=%+v attempt1=%+v attempt2=%+v persisted_last=%+v", outer, first, second, persisted.LastActionResult)
	}
}

func TestH2AuthorizedProductionRetryBeginsNewAttemptAfterFirstFailure(t *testing.T) {
	e := newB963334AuditEngine(t)
	reg := capability.NewRegistry()
	cap := capability.Capability{
		ID: "h2.production", Domain: capability.DomainLocal, Action: "create_file",
		Description: "create production artifact", Lane: capability.LaneProduction,
		Permission: capability.PermissionAutomatic,
	}
	if err := reg.Register(cap); err != nil {
		t.Fatal(err)
	}
	provider := &h2RetryThenSuccessProvider{}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, cap.ID); err != nil {
		t.Fatal(err)
	}
	e.capabilityRegistry = reg
	e.capabilityResolver = capability.NewResolver(reg)
	e.productionProviders = providers
	runtime, err := integration.NewProductionRuntime(
		e.capabilityResolver, reg, providers, &production.ProductionContext{}, e.workStore.Raw(),
		production.NewTechnicalRecoveryServiceWithDecider(h2RetryDecider{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	e.productionRuntime = runtime
	req := interaction.SemanticRequest{
		Raw: "crear artefacto para H2", Normalized: "crear artefacto para h2",
		Kind:            interaction.KindProduction,
		Objective:       interaction.Objective{Text: "crear artefacto para H2"},
		Action:          interaction.ActionIntent{Name: "create_file", Target: "h2-target", Confidence: 1},
		RequestedAction: "create_file", Topic: "production artifact",
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActCommand, Confidence: 1},
	}
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := e.workStore.BeginAttempt(work.ID, string(cap.ID), cap.Action, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	pending := production.PendingExecution{
		Request: req, CapabilityID: string(cap.ID), Operation: cap.Action,
		RouteKind: string(capability.IntentProduction), ProviderID: provider.ID(), AttemptNumber: attempt,
	}
	if err := e.workStore.SetPendingExecution(work.ID, pending); err != nil {
		t.Fatal(err)
	}
	e.conversationState.SetActiveWork(work.ID)
	approval := interaction.Analyze("Bueno, la autorizo entonces", interaction.Context{})
	turn := &processTurn{
		input: approval.Raw, semanticReq: approval,
		authorizationActs: interaction.CanonicalActs(approval),
	}
	if !e.resumePendingAuthorizationIfConfirmed(turn) {
		t.Fatal("explicit approval did not reach the pending execution")
	}
	updated, ok := e.workStore.Get(work.ID)
	if !ok {
		t.Fatal("authorized Work disappeared")
	}
	if provider.calls != 2 || len(updated.Attempts) != 2 {
		t.Fatalf("provider calls=%d attempts=%d; want 2 distinct attempts", provider.calls, len(updated.Attempts))
	}
	if updated.Attempts[0].Number != 1 || updated.Attempts[0].Status != "failed" || updated.Attempts[1].Number != 2 || updated.Attempts[1].Status != "succeeded" {
		t.Fatalf("authorization retry did not preserve both attempt outcomes: %+v", updated.Attempts)
	}
	if updated.Status != production.WorkCompleted || updated.LastActionResult == nil || updated.LastActionResult.AttemptNumber != 2 || !updated.LastActionResult.Verified {
		t.Fatalf("final authorized work result is not the verified retry: status=%s result=%+v", updated.Status, updated.LastActionResult)
	}
}
