package core

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/production"
)

type slowProductionProvider struct {
	started chan struct{}
	release chan struct{}
	result  production.ProviderResult
	calls   atomic.Int32
}

func (p *slowProductionProvider) ID() string { return "slow-provider" }

func (p *slowProductionProvider) Execute(ctx context.Context, _ production.ProviderRequest) production.ProviderResult {
	p.calls.Add(1)
	close(p.started)
	select {
	case <-p.release:
		return p.result
	case <-ctx.Done():
		return production.ProviderResult{
			Success: false, OutcomeStatus: production.OutcomeFailed,
			FailureReason: ctx.Err().Error(), Producer: p.ID(),
		}
	}
}

func newAsyncProductionEngine(t *testing.T, provider production.Provider) (*Engine, interaction.SemanticRequest, string) {
	return newAsyncProductionEngineWithPermission(t, provider, capability.PermissionAutomatic)
}

func newAsyncProductionEngineWithPermission(t *testing.T, provider production.Provider, permission capability.PermissionLevel) (*Engine, interaction.SemanticRequest, string) {
	t.Helper()
	e := buildTestEngine(t)
	registry := capability.NewRegistry()
	c := capability.Capability{
		ID: "programming.slow", Domain: capability.DomainLocal, Action: "slow_action",
		Description: "slow production operation", Lane: capability.LaneProduction,
		Permission: permission,
	}
	if err := registry.Register(c); err != nil {
		t.Fatalf("register capability: %v", err)
	}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	runtime, err := integration.NewProductionRuntime(
		capability.NewResolver(registry), registry, providers,
		&production.ProductionContext{}, e.workStore.Raw(), production.NewTechnicalRecoveryService(),
	)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	e.capabilityRegistry = registry
	e.capabilityResolver = capability.NewResolver(registry)
	e.productionRuntime = runtime

	req := interaction.Analyze("ejecuta la operación lenta", interaction.Context{})
	req.Kind = interaction.KindProduction
	req.Objective.Text = "ejecutar la operación lenta"
	req.Action = interaction.ActionIntent{Name: c.Action, Target: "slow-target", Confidence: 1}
	req.RequestedAction = c.Action
	req.CommunicativeActs = interaction.CommunicativeActs{Primary: interaction.ActCommand, Confidence: 1}
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatalf("begin work: %v", err)
	}
	e.conversationState.SetActiveWork(work.ID)
	return e, req, work.ID
}

func TestAsyncProductionReturnsBeforeProviderCompletes(t *testing.T) {
	provider := &slowProductionProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result: production.ProviderResult{
			Success: true, Observed: true, Verified: true,
			OutcomeStatus: production.OutcomeSucceeded, Producer: "slow-provider",
		},
	}
	e, req, workID := newAsyncProductionEngine(t, provider)
	decision := interaction.ResolveCapabilityInteraction(req)
	if !decision.Executable {
		t.Fatalf("decision is not executable: %+v", decision)
	}

	start := time.Now()
	res := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalGranted,
		WorkID: workID, ExperimentalBypassCognition: true, Async: true,
	}, req)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("async authority blocked for %s", elapsed)
	}
	if !res.Handled || res.BackgroundJobID == "" || res.OutcomeCode != "working" {
		t.Fatalf("unexpected async authority result: %+v", res)
	}
	<-provider.started

	done := make(chan struct{})
	go func() {
		e.Process("hola")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Process blocked while async production was running")
	}
	if !e.CancelBackgroundJob(res.BackgroundJobID) {
		t.Fatal("async production job could not be canceled")
	}
	close(provider.release)
	job := waitForBackgroundJob(t, e, res.BackgroundJobID, BackgroundJobCanceled)
	if job.Result.Success || job.Result.Verified {
		t.Fatalf("late production result was applied after cancel: %+v", job.Result)
	}
}

func TestAsyncProductionCompletesAndPersistsResult(t *testing.T) {
	provider := &slowProductionProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result: production.ProviderResult{
			Success: true, Observed: true, Verified: true,
			OutcomeStatus: production.OutcomeSucceeded, Producer: "slow-provider",
		},
	}
	e, req, workID := newAsyncProductionEngine(t, provider)
	decision := interaction.ResolveCapabilityInteraction(req)
	if !decision.Executable {
		t.Fatalf("decision is not executable: %+v", decision)
	}
	res := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalGranted,
		WorkID: workID, ExperimentalBypassCognition: true, Async: true,
	}, req)
	if res.BackgroundJobID == "" {
		t.Fatalf("async production job missing: %+v", res)
	}
	<-provider.started
	close(provider.release)
	job := waitForBackgroundJob(t, e, res.BackgroundJobID, BackgroundJobCompleted)
	if !job.Result.Success || !job.Result.Verified {
		t.Fatalf("job result not stored: %+v", job.Result)
	}
	work, ok := e.workStore.Get(workID)
	if !ok || work.LastActionResult == nil || !work.LastActionResult.Success {
		t.Fatalf("work result not persisted: ok=%t work=%+v", ok, work)
	}
}

func TestAsyncProductionResultIsAnnouncedOnNextTurn(t *testing.T) {
	provider := &slowProductionProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result: production.ProviderResult{
			Success: true, Observed: true, Verified: true,
			OutcomeStatus: production.OutcomeSucceeded, Producer: "slow-provider",
			Summary: "proyecto sumador verificado",
		},
	}
	e, req, workID := newAsyncProductionEngine(t, provider)
	decision := interaction.ResolveCapabilityInteraction(req)
	res := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalGranted,
		WorkID: workID, ExperimentalBypassCognition: true, Async: true,
	}, req)
	if res.BackgroundJobID == "" {
		t.Fatalf("async production job missing: %+v", res)
	}
	<-provider.started
	close(provider.release)
	_ = waitForBackgroundJob(t, e, res.BackgroundJobID, BackgroundJobCompleted)

	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })
	e.Process("hola")
	joined := strings.Join(responses, " ")
	if !strings.Contains(joined, "Terminé el trabajo en segundo plano") {
		t.Fatalf("background result was not announced: %q", joined)
	}
}

func TestAsyncProductionDoesNotHideAuthorization(t *testing.T) {
	provider := &slowProductionProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result: production.ProviderResult{
			Success: true, Observed: true, Verified: true,
			OutcomeStatus: production.OutcomeSucceeded, Producer: "slow-provider",
		},
	}
	e, req, workID := newAsyncProductionEngineWithPermission(t, provider, capability.PermissionAskFirst)
	e.capabilityApprovalResolver = nil
	decision := interaction.ResolveCapabilityInteraction(req)
	res := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalNone,
		WorkID: workID, ExperimentalBypassCognition: true, Async: true,
	}, req)
	if !res.Handled || !res.AuthorizationRequired {
		t.Fatalf("authorization-required production did not stay synchronous: %+v", res)
	}
	if res.BackgroundJobID != "" {
		t.Fatalf("authorization-required production was hidden in a background job: %+v", res)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("provider executed before authorization: %d", provider.calls.Load())
	}
}
