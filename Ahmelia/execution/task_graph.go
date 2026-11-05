package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"ahmelia/planning"
)

// TaskExecutionSpec binds an existing planning.Task to the execution fabric.
// It does not create a second task model: planning.Task remains the owner of
// task identity and lifecycle.
type ArtifactRequirement struct {
	ID       string
	Checksum string
	Verified bool
}

// ArtifactResolver is the read-only boundary used by workflow execution to
// require upstream artifacts before a dependent task starts. It deliberately
// exposes no production or domain-specific type.
type ArtifactResolver interface {
	ResolveArtifact(id string) (ArtifactRequirement, bool)
}

type TaskExecutionSpec struct {
	Task         *planning.Task
	CapabilityID string
	Operation    string
	Requirements Requirements
	Timeout      time.Duration
	Inputs       []ArtifactRequirement
	// ExecutionFingerprint binds resumable progress to the planned operation,
	// not merely to a reusable Task ID. It is populated by domain compilers.
	ExecutionFingerprint string
}

// TaskGraphResult is the observable result of one graph execution.
type TaskGraphResult struct {
	Completed []string
	Failed    map[string]error
	Attempts  []Attempt
	Cancelled bool
	// NeedsReconciliation identifies tasks whose operation may have taken
	// effect but whose result was not verified. Such tasks remain durably
	// marked as Running until the configured reconciler makes an explicit
	// decision; they must not be replayed as ordinary failed tasks.
	NeedsReconciliation []string
}

// TaskGraphExecutor executes a dependency graph through the canonical
// Execution Fabric. Independent tasks may run concurrently on different
// workers. A failed task blocks its dependents; retry is explicit and bounded.
type ReconcileDecision string

const (
	ReconcileCompleted ReconcileDecision = "completed"
	ReconcileRetry     ReconcileDecision = "retry"
	ReconcileBlocked   ReconcileDecision = "blocked"
)

// ExecutionReconciler resolves a task that was physically running when the
// process stopped. It is intentionally required before an uncertain task is
// executed again, preventing blind duplicate side effects.
type ExecutionReconciler func(context.Context, TaskExecutionSpec, string) (ReconcileDecision, error)

// ReconciliationPendingError means a previous execution may have changed the
// external world, but no reconciler could establish its outcome. Callers must
// preserve the work as resumable and must not convert this state into an
// ordinary terminal failure or dispatch the operation again.
type ReconciliationPendingError struct {
	TaskID string
	JobID  string
	Cause  error
}

func (e *ReconciliationPendingError) Error() string {
	if e == nil {
		return "execution: reconciliation pending"
	}
	if e.Cause != nil {
		return fmt.Sprintf("execution: task %q job %q awaits reconciliation: %v", e.TaskID, e.JobID, e.Cause)
	}
	return fmt.Sprintf("execution: task %q job %q awaits reconciliation", e.TaskID, e.JobID)
}

func (e *ReconciliationPendingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// RetryDisposition lets a task callback state whether repeating the same
// operation is safe after an error. Errors without this contract retain the
// graph's configured bounded-retry behavior; an explicit false stops retries.
type RetryDisposition interface {
	Retryable() bool
}

// ReconciliationDisposition marks an error whose physical side effect may
// already have occurred. It is stronger than Retryable(false): it requires
// the durable checkpoint to retain the uncertain execution identity so a
// later run cannot mistake it for a safe ordinary failure.
type ReconciliationDisposition interface {
	RequiresReconciliation() bool
}

type TaskGraphExecutor struct {
	dispatcher *Dispatcher
	maxRetries int
	artifacts  ArtifactResolver
	reconciler ExecutionReconciler
}

// SetArtifactResolver configures the optional read-only artifact gate. When
// configured, a task cannot start unless every declared input artifact exists
// and is explicitly verified; a checksum, when supplied, must also match.
func (e *TaskGraphExecutor) SetExecutionReconciler(reconciler ExecutionReconciler) {
	if e != nil {
		e.reconciler = reconciler
	}
}

func (e *TaskGraphExecutor) SetArtifactResolver(resolver ArtifactResolver) {
	if e != nil {
		e.artifacts = resolver
	}
}

func NewTaskGraphExecutor(dispatcher *Dispatcher, maxRetries int) (*TaskGraphExecutor, error) {
	if dispatcher == nil {
		return nil, fmt.Errorf("execution: task graph dispatcher nil")
	}
	if maxRetries < 0 {
		return nil, fmt.Errorf("execution: task graph maxRetries inválido")
	}
	return &TaskGraphExecutor{dispatcher: dispatcher, maxRetries: maxRetries}, nil
}

// Run validates the complete graph before executing anything. run receives the
// selected task and execution context; the physical worker is still selected
// by Dispatcher according to the task requirements.
func (e *TaskGraphExecutor) Run(ctx context.Context, specs []TaskExecutionSpec, run func(context.Context, TaskExecutionSpec) error) (TaskGraphResult, error) {
	return e.run(ctx, specs, run, nil, "", nil)
}

// RunResumable executes the same graph while persisting completion state under
// workID. A later process can call it again with the same workID and skip tasks
// already proven complete. Failed tasks remain eligible for bounded retry.
func (e *TaskGraphExecutor) RunResumable(ctx context.Context, workID string, specs []TaskExecutionSpec, run func(context.Context, TaskExecutionSpec) error, store TaskGraphCheckpointStore) (TaskGraphResult, error) {
	if store == nil {
		return TaskGraphResult{}, fmt.Errorf("execution: resumable graph checkpoint store nil")
	}
	if workID == "" {
		return TaskGraphResult{}, fmt.Errorf("execution: resumable graph workID vacío")
	}
	return e.run(ctx, specs, run, store, workID, nil)
}

// RunResumableWithReconciler supplies a per-invocation observer/reconciler. This avoids
// mutating shared executor configuration while a workflow is running.
func (e *TaskGraphExecutor) RunResumableWithReconciler(ctx context.Context, workID string, specs []TaskExecutionSpec, run func(context.Context, TaskExecutionSpec) error, store TaskGraphCheckpointStore, reconciler ExecutionReconciler) (TaskGraphResult, error) {
	if store == nil {
		return TaskGraphResult{}, fmt.Errorf("execution: resumable graph checkpoint store nil")
	}
	if workID == "" {
		return TaskGraphResult{}, fmt.Errorf("execution: resumable graph workID vacío")
	}
	if reconciler == nil {
		return TaskGraphResult{}, fmt.Errorf("execution: per-run reconciler nil")
	}
	return e.run(ctx, specs, run, store, workID, reconciler)
}

func (e *TaskGraphExecutor) run(ctx context.Context, specs []TaskExecutionSpec, run func(context.Context, TaskExecutionSpec) error, store TaskGraphCheckpointStore, workID string, runReconciler ExecutionReconciler) (TaskGraphResult, error) {
	if e == nil || e.dispatcher == nil {
		return TaskGraphResult{}, fmt.Errorf("execution: task graph unavailable")
	}
	if run == nil {
		return TaskGraphResult{}, fmt.Errorf("execution: task graph nil operation")
	}
	if err := ctx.Err(); err != nil {
		return TaskGraphResult{}, err
	}
	if err := validateTaskGraph(specs); err != nil {
		return TaskGraphResult{}, err
	}

	graphFingerprint := taskGraphFingerprint(specs)
	checkpoint := TaskGraphCheckpoint{WorkID: workID, GraphFingerprint: graphFingerprint, Failed: map[string]string{}, Attempts: map[string]int{}, Running: map[string]string{}}
	if store != nil {
		loaded, ok, err := store.Load(workID)
		if err != nil {
			return TaskGraphResult{}, fmt.Errorf("execution: load checkpoint: %w", err)
		}
		if ok {
			if graphFingerprint != "" && loaded.GraphFingerprint != graphFingerprint {
				return TaskGraphResult{}, fmt.Errorf("execution: checkpoint for work %q belongs to a different planned graph; reconciliation or explicit migration required", workID)
			}
			checkpoint = loaded
			if graphFingerprint != "" {
				checkpoint.GraphFingerprint = graphFingerprint
			}
		}
		if checkpoint.Failed == nil {
			checkpoint.Failed = map[string]string{}
		}
		if checkpoint.Attempts == nil {
			checkpoint.Attempts = map[string]int{}
		}
		if checkpoint.Running == nil {
			checkpoint.Running = map[string]string{}
		}
	}

	byID := make(map[string]TaskExecutionSpec, len(specs))
	dependents := make(map[string][]string, len(specs))
	remaining := make(map[string]int, len(specs))
	blocked := make(map[string]bool, len(specs))
	for _, s := range specs {
		byID[s.Task.ID] = s
		remaining[s.Task.ID] = len(s.Task.Dependencies)
		for _, dep := range s.Task.Dependencies {
			dependents[dep] = append(dependents[dep], s.Task.ID)
		}
	}

	activeReconciler := e.reconciler
	if runReconciler != nil {
		activeReconciler = runReconciler
	}
	result := TaskGraphResult{Failed: map[string]error{}}
	completed := map[string]bool{}
	failed := map[string]bool{}
	// A reconciled-blocked operation is terminal for this unchanged graph. Do
	// not treat it like a transient execution error and silently dispatch it on
	// the next invocation. A changed plan has a different graph fingerprint.
	for _, id := range checkpoint.Blocked {
		if _, ok := byID[id]; !ok {
			return TaskGraphResult{}, fmt.Errorf("execution: checkpoint references unknown blocked task %q", id)
		}
		failed[id] = true
		result.Failed[id] = fmt.Errorf("task blocked during execution reconciliation")
		byID[id].Task.Fail("task blocked during execution reconciliation")
	}
	// A persisted Running entry means the previous process may have issued a
	// real side effect and died before recording the outcome. Never rerun it
	// blindly. Reconciliation must establish whether it completed or whether
	// retrying is safe.
	for id, jobID := range checkpoint.Running {
		spec, ok := byID[id]
		if !ok {
			return TaskGraphResult{}, fmt.Errorf("execution: checkpoint references unknown running task %q", id)
		}
		if activeReconciler == nil {
			return TaskGraphResult{NeedsReconciliation: []string{id}}, &ReconciliationPendingError{TaskID: id, JobID: jobID}
		}
		decision, err := activeReconciler(ctx, spec, jobID)
		if err != nil {
			return TaskGraphResult{NeedsReconciliation: []string{id}}, &ReconciliationPendingError{TaskID: id, JobID: jobID, Cause: err}
		}
		delete(checkpoint.Running, id)
		switch decision {
		case ReconcileCompleted:
			completed[id] = true
			// Reconciliation is the evidence that closes an uncertain attempt.
			// The Task may be TaskBlocked because verification failed before the
			// process stopped; leaving it blocked would contradict the checkpoint.
			spec.Task.Complete()
			result.Completed = append(result.Completed, id)
			checkpoint.Completed = appendUniqueString(checkpoint.Completed, id)
		case ReconcileRetry:
		case ReconcileBlocked:
			blockedErr := fmt.Errorf("task blocked during execution reconciliation")
			failed[id] = true
			result.Failed[id] = blockedErr
			spec.Task.Fail(blockedErr.Error())
			checkpoint.Blocked = appendUniqueString(checkpoint.Blocked, id)
			checkpoint.Failed[id] = blockedErr.Error()
		default:
			return TaskGraphResult{}, fmt.Errorf("execution: invalid reconciliation decision %q for task %q", decision, id)
		}
	}
	if store != nil {
		if err := store.Save(checkpoint); err != nil {
			return TaskGraphResult{}, fmt.Errorf("execution: save reconciled checkpoint: %w", err)
		}
	}
	for _, id := range checkpoint.Completed {
		completed[id] = true
	}
	running := map[string]bool{}
	ready := make([]string, 0)
	for id := range remaining {
		if completed[id] {
			continue
		}
		count := 0
		for _, dep := range byID[id].Task.Dependencies {
			if !completed[dep] {
				count++
			}
		}
		remaining[id] = count
		if count == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	type outcome struct {
		id       string
		attempts []Attempt
		err      error
	}
	outcomes := make(chan outcome, len(specs))
	inflight := 0

	start := func(id string) {
		s := byID[id]
		if err := e.validateArtifactInputs(s.Inputs); err != nil {
			failed[id] = true
			result.Failed[id] = err
			for _, dep := range dependents[id] {
				blocked[dep] = true
			}
			if store != nil {
				checkpoint.Failed[id] = err.Error()
				_ = saveTaskGraphCheckpoint(store, checkpoint, completed, failed)
			}
			return
		}
		s.Task.Start()
		running[id] = true
		inflight++
		jobID := fmt.Sprintf("task-%s-attempt-%d", id, checkpoint.Attempts[id]+1)
		if store != nil {
			checkpoint.Running[id] = jobID
			if err := saveTaskGraphCheckpoint(store, checkpoint, completed, failed); err != nil {
				inflight--
				running[id] = false
				delete(checkpoint.Running, id)
				return
			}
		}
		go func() {
			attempts := make([]Attempt, 0, e.maxRetries+1)
			var err error
			for attempt := 0; attempt <= e.maxRetries; attempt++ {
				job := Job{
					ID:           fmt.Sprintf("task-%s-attempt-%d", id, checkpoint.Attempts[id]+attempt+1),
					TaskID:       id,
					CapabilityID: s.CapabilityID,
					Operation:    s.Operation,
					Requirements: s.Requirements,
					CreatedAt:    time.Now(),
					Timeout:      s.Timeout,
				}
				last, dispatchErr := e.dispatcher.Dispatch(ctx, job, func(execCtx context.Context) error {
					return run(execCtx, s)
				})
				attempts = append(attempts, last)
				err = dispatchErr
				if err == nil {
					break
				}
				if ctx.Err() != nil {
					break
				}
				var disposition RetryDisposition
				if errors.As(err, &disposition) && !disposition.Retryable() {
					break
				}
			}
			outcomes <- outcome{id: id, attempts: attempts, err: err}
		}()
	}

	for len(completed)+len(failed) < len(specs) {
		if err := ctx.Err(); err != nil {
			result.Cancelled = true
			if store != nil {
				_ = saveTaskGraphCheckpoint(store, checkpoint, completed, failed)
			}
			return result, err
		}

		// Start every currently-ready task. This is intentionally unbounded at
		// graph level; the Execution Fabric remains the authority that chooses
		// physical workers. Worker-level contention therefore does not create a
		// second scheduler here.
		for len(ready) > 0 {
			id := ready[0]
			ready = ready[1:]
			if blocked[id] || completed[id] || failed[id] || running[id] {
				continue
			}
			start(id)
		}

		if inflight == 0 {
			// No task can progress and no task is running: the graph is blocked.
			for _, s := range specs {
				if !completed[s.Task.ID] && !failed[s.Task.ID] {
					blocked[s.Task.ID] = true
					result.Failed[s.Task.ID] = fmt.Errorf("task blocked by failed dependency")
				}
			}
			return result, nil
		}

		select {
		case <-ctx.Done():
			result.Cancelled = true
			if store != nil {
				_ = saveTaskGraphCheckpoint(store, checkpoint, completed, failed)
			}
			return result, ctx.Err()
		case out := <-outcomes:
			inflight--
			running[out.id] = false
			jobID := checkpoint.Running[out.id]
			delete(checkpoint.Running, out.id)
			result.Attempts = append(result.Attempts, out.attempts...)
			task := byID[out.id].Task
			if out.err != nil {
				task.Fail(out.err.Error())
				failed[out.id] = true
				result.Failed[out.id] = out.err
				var reconciliation ReconciliationDisposition
				requiresReconciliation := errors.As(out.err, &reconciliation) && reconciliation.RequiresReconciliation()
				if requiresReconciliation {
					result.NeedsReconciliation = append(result.NeedsReconciliation, out.id)
				}
				if store != nil && requiresReconciliation {
					// Keep the same job identity durable. On the next invocation,
					// the normal reconciliation boundary must classify the effect
					// before any execution can be dispatched again.
					checkpoint.Running[out.id] = jobID
				}
				for _, dep := range dependents[out.id] {
					blocked[dep] = true
				}
				if store != nil {
					if !requiresReconciliation {
						checkpoint.Failed[out.id] = out.err.Error()
					} else {
						delete(checkpoint.Failed, out.id)
					}
					checkpoint.Attempts[out.id] += len(out.attempts)
					_ = saveTaskGraphCheckpoint(store, checkpoint, completed, failed)
				}
				continue
			}

			if task.Status == planning.TaskInProgress {
				task.Complete()
			}
			completed[out.id] = true
			result.Completed = append(result.Completed, out.id)
			if store != nil {
				checkpoint.Completed = appendUniqueString(checkpoint.Completed, out.id)
				delete(checkpoint.Failed, out.id)
				checkpoint.Attempts[out.id] += len(out.attempts)
				_ = saveTaskGraphCheckpoint(store, checkpoint, completed, failed)
			}
			for _, dep := range dependents[out.id] {
				if blocked[dep] {
					continue
				}
				remaining[dep]--
				if remaining[dep] == 0 {
					ready = append(ready, dep)
					sort.Strings(ready)
				}
			}
		}
	}

	sort.Strings(result.Completed)
	sort.Strings(result.NeedsReconciliation)
	if store != nil && len(completed) == len(specs) {
		if err := store.Delete(workID); err != nil {
			return result, fmt.Errorf("execution: delete completed checkpoint: %w", err)
		}
	}
	return result, nil
}

// taskGraphFingerprint is enabled only when a domain compiler supplies an
// execution fingerprint. Generic task graphs without domain semantics retain
// their existing checkpoint contract; planned workflows fail closed if their
// operation/dependency projection changes under the same WorkID.
func taskGraphFingerprint(specs []TaskExecutionSpec) string {
	configured := false
	for _, spec := range specs {
		if spec.ExecutionFingerprint != "" {
			configured = true
			break
		}
	}
	if !configured {
		return ""
	}
	type taskIdentity struct {
		ID                   string
		GoalID               string
		Title                string
		Description          string
		Order                int
		Dependencies         []string
		CapabilityID         string
		Operation            string
		ExecutionFingerprint string
		Inputs               []ArtifactRequirement
	}
	identities := make([]taskIdentity, 0, len(specs))
	for _, spec := range specs {
		if spec.Task == nil || spec.ExecutionFingerprint == "" {
			return "invalid"
		}
		deps := append([]string(nil), spec.Task.Dependencies...)
		identities = append(identities, taskIdentity{
			ID: spec.Task.ID, GoalID: spec.Task.GoalID, Title: spec.Task.Title,
			Description: spec.Task.Description, Order: spec.Task.Order,
			Dependencies: deps, CapabilityID: spec.CapabilityID,
			Operation: spec.Operation, ExecutionFingerprint: spec.ExecutionFingerprint,
			Inputs: append([]ArtifactRequirement(nil), spec.Inputs...),
		})
	}
	raw, err := json.Marshal(identities)
	if err != nil {
		return "invalid"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func saveTaskGraphCheckpoint(store TaskGraphCheckpointStore, cp TaskGraphCheckpoint, completed map[string]bool, failed map[string]bool) error {
	cp.Completed = cp.Completed[:0]
	for id, ok := range completed {
		if ok {
			cp.Completed = append(cp.Completed, id)
		}
	}
	cp.Failed = map[string]string{}
	for id := range failed {
		cp.Failed[id] = "failed"
	}
	return store.Save(cp)
}

func appendUniqueString(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

func (e *TaskGraphExecutor) validateArtifactInputs(inputs []ArtifactRequirement) error {
	if len(inputs) == 0 {
		return nil
	}
	if e.artifacts == nil {
		return fmt.Errorf("execution: artifact gate configured by task but no resolver is available")
	}
	for _, req := range inputs {
		if req.ID == "" {
			return fmt.Errorf("execution: artifact input has empty ID")
		}
		actual, ok := e.artifacts.ResolveArtifact(req.ID)
		if !ok {
			return fmt.Errorf("execution: required artifact %q not found", req.ID)
		}
		if !actual.Verified {
			return fmt.Errorf("execution: required artifact %q is not verified", req.ID)
		}
		if req.Checksum != "" && actual.Checksum != req.Checksum {
			return fmt.Errorf("execution: required artifact %q checksum mismatch", req.ID)
		}
	}
	return nil
}

func validateTaskGraph(specs []TaskExecutionSpec) error {
	if len(specs) == 0 {
		return fmt.Errorf("execution: task graph vacío")
	}
	byID := make(map[string]TaskExecutionSpec, len(specs))
	for _, s := range specs {
		if s.Task == nil || s.Task.ID == "" {
			return fmt.Errorf("execution: task graph contiene tarea inválida")
		}
		if _, exists := byID[s.Task.ID]; exists {
			return fmt.Errorf("execution: task graph tarea duplicada %q", s.Task.ID)
		}
		byID[s.Task.ID] = s
	}
	state := map[string]uint8{}
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("execution: dependencia circular detectada en %q", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, dep := range byID[id].Task.Dependencies {
			if _, ok := byID[dep]; !ok {
				return fmt.Errorf("execution: dependencia %q de task %q no existe", dep, id)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
