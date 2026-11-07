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
	deadline := time.Now().Add(2 * time.Second)
	for {
		work, ok := e.workStore.Get(workID)
		if ok && work.Status == production.WorkBlocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("canceled work was not blocked: ok=%t work=%+v", ok, work)
		}
		time.Sleep(5 * time.Millisecond)
	}
	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })
	e.Process("hola")
	if joined := strings.Join(responses, " "); !strings.Contains(joined, "Cancelé el trabajo en segundo plano") {
		t.Fatalf("cancel was not announced: %q", joined)
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
	beforeEvents := 0
	if e.eventStore != nil {
		beforeEvents = len(e.eventStore.Snapshot())
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
	if e.eventStore != nil && len(e.eventStore.Snapshot()) <= beforeEvents {
		t.Fatal("async completion did not record capability experience")
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

func TestConversationalCancelStopsRunningJob(t *testing.T) {
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
	res := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalGranted,
		WorkID: workID, ExperimentalBypassCognition: true, Async: true,
	}, req)
	if res.BackgroundJobID == "" {
		t.Fatalf("async production job missing: %+v", res)
	}
	<-provider.started

	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })
	e.Process("cancela el trabajo")
	if joined := strings.Join(responses, " "); !strings.Contains(joined, "Cancelé el trabajo en segundo plano") {
		t.Fatalf("conversational cancel was not acknowledged: %q", joined)
	}
	close(provider.release)
	job := waitForBackgroundJob(t, e, res.BackgroundJobID, BackgroundJobCanceled)
	if job.Result.Success || job.Result.Verified {
		t.Fatalf("late result was applied after conversational cancel: %+v", job.Result)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		work, ok := e.workStore.Get(workID)
		if ok && work.Status == production.WorkBlocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("conversationally canceled work was not blocked: ok=%t work=%+v", ok, work)
		}
		time.Sleep(5 * time.Millisecond)
	}

	before := len(responses)
	e.Process("hola")
	if joined := strings.Join(responses[before:], " "); strings.Contains(joined, "Cancelé el trabajo en segundo plano") {
		t.Fatalf("suppressed cancellation was announced twice: %q", joined)
	}
}

func TestNewProductionObjectiveDuringJobIsRejected(t *testing.T) {
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
	res := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalGranted,
		WorkID: workID, ExperimentalBypassCognition: true, Async: true,
	}, req)
	if res.BackgroundJobID == "" {
		t.Fatalf("async production job missing: %+v", res)
	}
	<-provider.started

	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })
	e.Process("Crea otro programa Go que reste dos números")
	if joined := strings.Join(responses, " "); !strings.Contains(joined, "trabajo de programación en segundo plano en curso") {
		t.Fatalf("new objective was not rejected: %q", joined)
	}
	if jobs := e.BackgroundJobs(); len(jobs) != 1 {
		t.Fatalf("a second production job was started: %+v", jobs)
	}

	_ = e.CancelBackgroundJob(res.BackgroundJobID)
	close(provider.release)
	_ = waitForBackgroundJob(t, e, res.BackgroundJobID, BackgroundJobCanceled)
}

type twoPhaseProvider struct {
	calls        atomic.Int32
	firstStarted chan struct{}
	firstRelease chan struct{}
}

func (p *twoPhaseProvider) ID() string { return "slow-provider" }

func (p *twoPhaseProvider) Execute(ctx context.Context, _ production.ProviderRequest) production.ProviderResult {
	if p.calls.Add(1) == 1 {
		close(p.firstStarted)
		select {
		case <-p.firstRelease:
		case <-ctx.Done():
			return production.ProviderResult{
				Success: false, OutcomeStatus: production.OutcomeFailed,
				FailureReason: ctx.Err().Error(), Producer: p.ID(),
			}
		}
	}
	return production.ProviderResult{
		Success: true, Observed: true, Verified: true,
		OutcomeStatus: production.OutcomeSucceeded, Producer: p.ID(),
	}
}

func waitForCompletedBackgroundJob(t *testing.T, e *Engine) BackgroundJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, job := range e.BackgroundJobs() {
			if job.Status == BackgroundJobCompleted {
				return job
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no completed background job: %+v", e.BackgroundJobs())
	return BackgroundJob{}
}

func TestNewObjectiveAfterCancelStartsReplacement(t *testing.T) {
	provider := &twoPhaseProvider{firstStarted: make(chan struct{}), firstRelease: make(chan struct{})}
	e, req, workID := newAsyncProductionEngine(t, provider)
	e.EnableAsyncProduction()
	decision := interaction.ResolveCapabilityInteraction(req)
	res := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Context: context.Background(), Approval: autonomy.ApprovalGranted,
		WorkID: workID, ExperimentalBypassCognition: true, Async: true,
	}, req)
	if res.BackgroundJobID == "" {
		t.Fatalf("first async job missing: %+v", res)
	}
	<-provider.firstStarted
	e.Process("cancela el trabajo")
	close(provider.firstRelease)
	_ = waitForBackgroundJob(t, e, res.BackgroundJobID, BackgroundJobCanceled)

	e.Process("Crea otro programa Go que reste dos números")
	deadline := time.Now().Add(5 * time.Second)
	for provider.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if provider.calls.Load() < 2 {
		t.Fatalf("replacement objective never reached the provider: %+v", e.BackgroundJobs())
	}
	second := waitForCompletedBackgroundJob(t, e)
	if second.Result.Success != true || second.Result.Verified != true {
		t.Fatalf("replacement job did not complete: %+v", second)
	}
	if jobs := e.BackgroundJobs(); len(jobs) < 2 {
		t.Fatalf("expected canceled + completed jobs, got %+v", jobs)
	}
}

func TestBackgroundResultNotifiesProactively(t *testing.T) {
	provider := &slowProductionProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result: production.ProviderResult{
			Success: true, Observed: true, Verified: true,
			OutcomeStatus: production.OutcomeSucceeded, Producer: "slow-provider",
		},
	}
	e, req, workID := newAsyncProductionEngine(t, provider)
	notifications := make(chan string, 2)
	e.SetBackgroundNotifier(func(text string) { notifications <- text })
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

	select {
	case text := <-notifications:
		if !strings.Contains(text, "Terminé el trabajo en segundo plano") {
			t.Fatalf("unexpected proactive notification: %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proactive notification was not delivered")
	}

	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })
	e.Process("hola")
	if joined := strings.Join(responses, " "); strings.Contains(joined, "Terminé el trabajo en segundo plano") {
		t.Fatalf("proactively notified result was announced again: %q", joined)
	}
}

func TestAsyncProductionDoesNotHideAuthorization(t *testing.T) {
	provider := &slowProductionProvider{started: make(chan struct{}),
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
