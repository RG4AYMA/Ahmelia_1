package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/execution"
	"ahmelia/planning"
	"ahmelia/production"
)

type workflowStoreStub struct {
	progress  []string
	completed bool
	blocked   bool
}

func (s *workflowStoreStub) SetProgress(_ string, current string, completed, pending []string) error {
	s.progress = append(s.progress, current+"|"+join(completed)+"|"+join(pending))
	return nil
}
func (s *workflowStoreStub) Complete(_ string, success bool, _ string) error {
	s.completed = success
	return nil
}
func (s *workflowStoreStub) Block(_ string, _ string) error { s.blocked = true; return nil }
func join(v []string) string {
	out := ""
	for i, x := range v {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}

type workflowReconciliationStore struct {
	workflowStoreStub
	attempt      production.WorkAttempt
	reconciled   bool
	lastResult   production.ActionResult
	reconcileErr error
}

func (s *workflowReconciliationStore) LastAttempt(_ string) (production.WorkAttempt, bool) {
	return s.attempt, true
}

func (s *workflowReconciliationStore) ReconcileUnknownAttempt(_ string, number int, result production.ActionResult) error {
	if s.reconcileErr != nil {
		return s.reconcileErr
	}
	if number != s.attempt.Number {
		return fmt.Errorf("unexpected attempt number %d", number)
	}
	s.reconciled = true
	s.lastResult = result
	s.attempt.Status = "succeeded"
	s.attempt.Result = &result
	return nil
}

func TestWorkflowRunnerConfirmedReconciliationConvergesCanonicalWorkAttempt(t *testing.T) {
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 4, MemoryMB: 4096}}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified"}}
	runner, err := NewWorkflowRunner(graph, store, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	observation := autonomy.EnvironmentObservation{Status: autonomy.ObservationConfirmed, Confirmed: true, Details: "observed expected effect", ActionID: "action-1", Operation: "create", Target: "/tmp/effect", Source: "test.filesystem", ObservedAt: time.Now().UTC(), Facts: []autonomy.ObservationFact{{Subject: "/tmp/effect", Predicate: "filesystem.exists", Expected: "true", Observed: "true"}}}
	if err := runner.ReconcileConfirmedAttempt("work-reconcile", "checkpoint-job", observation); err != nil {
		t.Fatal(err)
	}
	if !store.reconciled {
		t.Fatal("confirmed observation did not update canonical WorkAttempt")
	}
	if store.lastResult.WorkID != "work-reconcile" || store.lastResult.AttemptNumber != 1 || !store.lastResult.Observed || !store.lastResult.Verified || store.lastResult.OutcomeStatus != production.OutcomeSucceeded {
		t.Fatalf("canonical reconciliation result lost identity/epistemic facts: %+v", store.lastResult)
	}
	if store.lastResult.ObservationActionID != "action-1" || store.lastResult.ObservationOperation != "create" || store.lastResult.ObservationTarget != "/tmp/effect" || store.lastResult.ObservationSource != "test.filesystem" || store.lastResult.ObservationCapturedAt.IsZero() || len(store.lastResult.ObservationFacts) != 1 || store.lastResult.ObservationFacts[0].Expected != store.lastResult.ObservationFacts[0].Observed {
		t.Fatalf("reconciled canonical result lost structured evidence/provenance: %+v", store.lastResult)
	}
}

func TestWorkflowRunnerRefusesConfirmedCheckpointWhenCanonicalAttemptIdentityDiffers(t *testing.T) {
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 4, MemoryMB: 4096}}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified", JobID: "canonical-job"}}
	runner, err := NewWorkflowRunner(graph, store, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	err = runner.ReconcileConfirmedAttempt("work-identity", "checkpoint-job", autonomy.EnvironmentObservation{Status: autonomy.ObservationConfirmed, Confirmed: true, ActionID: "action-1", Operation: "create", Target: "resource", Source: "test.filesystem", ObservedAt: time.Now().UTC(), Facts: []autonomy.ObservationFact{{Subject: "resource", Predicate: "exists", Expected: "true", Observed: "true"}}})
	if err == nil {
		t.Fatal("expected execution identity mismatch to prevent canonical closure")
	}
	if store.reconciled {
		t.Fatal("canonical WorkAttempt was mutated despite identity mismatch")
	}
}

func TestWorkflowRunnerCompletesCanonicalWorkAfterVerifiedTasks(t *testing.T) {
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 4, MemoryMB: 4096}}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &workflowStoreStub{}
	runner, err := NewWorkflowRunner(graph, store, checkpoints)
	if err != nil {
		t.Fatal(err)
	}

	a := planning.NewTask("g", "a", "", 1, time.Second)
	b := planning.NewTask("g", "b", "", 2, time.Second)
	b.Dependencies = []string{a.ID}
	specs := []execution.TaskExecutionSpec{{Task: a, CapabilityID: "test", Operation: "a"}, {Task: b, CapabilityID: "test", Operation: "b"}}
	result, err := runner.Run(context.Background(), "work-1", specs, func(_ context.Context, spec execution.TaskExecutionSpec) error { spec.Task.Complete(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Completed) != 2 || !store.completed || store.blocked {
		t.Fatalf("unexpected workflow result: %+v completed=%v blocked=%v", result, store.completed, store.blocked)
	}
}

func TestWorkflowRunnerBlocksCanonicalWorkOnTaskFailure(t *testing.T) {
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 4, MemoryMB: 4096}}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &workflowStoreStub{}
	runner, err := NewWorkflowRunner(graph, store, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	task := planning.NewTask("g", "failing", "", 1, time.Second)
	specs := []execution.TaskExecutionSpec{{Task: task, CapabilityID: "test", Operation: "fail"}}
	result, err := runner.Run(context.Background(), "work-2", specs, func(context.Context, execution.TaskExecutionSpec) error { return context.Canceled })
	if err != nil {
		t.Fatal("graph-level error should be represented as result/cancellation only when context is cancelled externally")
	}
	if len(result.Failed) == 0 || !store.blocked {
		t.Fatalf("expected blocked workflow: %+v blocked=%v", result, store.blocked)
	}
}

func TestWorkflowRunnerReconciliationFailsClosedWithoutCanonicalAttempt(t *testing.T) {
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	worker := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 2, MemoryMB: 1024}}
	if err := d.RegisterWorker(worker); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewWorkflowRunner(graph, &workflowStoreStub{}, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	observation := autonomy.EnvironmentObservation{Confirmed: true, Status: autonomy.ObservationConfirmed, Details: "effect appears present"}
	if err := runner.ReconcileConfirmedAttempt("work-without-canonical-attempt", "job-1", observation); err == nil {
		t.Fatal("reconciliation must not silently succeed without a canonical WorkAttempt owner")
	}
}

func TestWorkflowRunnerReconciliationRejectsNonConfirmedObservation(t *testing.T) {
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	worker := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 2, MemoryMB: 1024}}
	if err := d.RegisterWorker(worker); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified"}}
	runner, err := NewWorkflowRunner(graph, store, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	observation := autonomy.EnvironmentObservation{Confirmed: false, Status: autonomy.ObservationContradicted, Details: "effect absent"}
	if err := runner.ReconcileConfirmedAttempt("work-reconcile", "", observation); err == nil {
		t.Fatal("contradicted observation must not be converted into a successful reconciliation")
	}
	if store.reconciled {
		t.Fatal("rejected observation mutated canonical WorkAttempt")
	}
}

func TestWorkflowRunnerRejectsConfirmedObservationWithoutStructuredFacts(t *testing.T) {
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 4, MemoryMB: 4096}}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified"}}
	runner, err := NewWorkflowRunner(graph, store, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	observation := autonomy.EnvironmentObservation{Confirmed: true, Status: autonomy.ObservationConfirmed, Details: "free text only"}
	if err := runner.ReconcileConfirmedAttempt("work-no-facts", "", observation); err == nil {
		t.Fatal("expected missing structured facts to fail closed")
	}
	if store.reconciled {
		t.Fatal("canonical attempt mutated despite missing structured facts")
	}
}

func TestWorkflowRunnerRejectsStaleStructuredObservation(t *testing.T) {
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 4, MemoryMB: 4096}}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified"}}
	runner, err := NewWorkflowRunner(graph, store, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	observation := autonomy.EnvironmentObservation{Confirmed: true, Status: autonomy.ObservationConfirmed, ActionID: "action-1", Operation: "create", Target: "resource", Source: "test.filesystem", ObservedAt: time.Now().UTC().Add(-time.Minute), Facts: []autonomy.ObservationFact{{Subject: "resource", Predicate: "exists", Expected: "true", Observed: "true"}}}
	if err := runner.ReconcileConfirmedAttempt("work-stale", "", observation); err == nil {
		t.Fatal("expected stale evidence to be rejected")
	}
	if store.reconciled {
		t.Fatal("stale evidence mutated the canonical attempt")
	}
}
