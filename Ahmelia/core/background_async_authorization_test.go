package core

import (
	"strings"
	"testing"
	"time"

	"ahmelia/capability"
	"ahmelia/production"
)

// TestAsyncProductionAfterAuthorizationReturnsBeforeProviderCompletes is the
// live-path regression for the async production contract: a production that
// asked for authorization must resume asynchronously when Async is enabled,
// announce the start immediately, and leave the turn free while the provider
// is still running.
func TestAsyncProductionAfterAuthorizationReturnsBeforeProviderCompletes(t *testing.T) {
	provider := &slowProductionProvider{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
		result: production.ProviderResult{
			Success: true, Observed: true, Verified: true,
			OutcomeStatus: production.OutcomeSucceeded, Producer: "slow-provider",
		},
	}
	e, req, workID := newAsyncProductionEngine(t, provider)
	e.EnableAsyncProduction()

	if _, err := e.workStore.BeginAttempt(workID, "programming.slow", "slow_action", "slow-provider"); err != nil {
		t.Fatalf("begin attempt: %v", err)
	}
	pending := production.PendingExecution{
		Request: req, CapabilityID: "programming.slow", Operation: "slow_action",
		RouteKind: string(capability.IntentProduction), ProviderID: "slow-provider",
		AttemptNumber: 1, Reason: "test authorization continuation",
	}
	if err := e.workStore.SetPendingExecution(workID, pending); err != nil {
		t.Fatalf("set pending execution: %v", err)
	}
	e.conversationState.SetActiveWork(workID)

	var responses []string
	e.SetResponseCallback(func(text string) { responses = append(responses, text) })
	done := make(chan struct{})
	go func() {
		e.Process("Sí, autorizo la ejecución pendiente")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("la autorización async bloqueó el turno")
	}
	if joined := strings.Join(responses, " "); !strings.Contains(joined, "segundo plano") {
		t.Fatalf("no se anunció el trabajo async: %q", joined)
	}
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("el proveedor no arrancó en segundo plano")
	}
	jobs := e.BackgroundJobs()
	if len(jobs) != 1 || jobs[0].Status != BackgroundJobRunning {
		t.Fatalf("jobs = %+v", jobs)
	}
	if !e.CancelBackgroundJob(jobs[0].ID) {
		t.Fatal("no se pudo cancelar el job async")
	}
	close(provider.release)
	select {
	case <-provider.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("el proveedor no terminó tras el release")
	}
	waitForBackgroundJob(t, e, jobs[0].ID, BackgroundJobCanceled)
	deadline := time.Now().Add(2 * time.Second)
	for {
		work, ok := e.workStore.Get(workID)
		if ok && work.Status == production.WorkBlocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("el trabajo cancelado no quedó bloqueado: ok=%t work=%+v", ok, work)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
