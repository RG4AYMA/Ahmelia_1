package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/programming"
)

var errInterruptedWork = errors.New("work interrupted by process restart; reconciliation required")
var errBackgroundJobCanceled = errors.New("background job canceled by owner")

// BackgroundJobStatus is the lifecycle of one background execution.
type BackgroundJobStatus string

const (
	BackgroundJobRunning   BackgroundJobStatus = "running"
	BackgroundJobCompleted BackgroundJobStatus = "completed"
	BackgroundJobCanceled  BackgroundJobStatus = "canceled"
	BackgroundJobFailed    BackgroundJobStatus = "failed"
)

// BackgroundJob is a snapshot of one job. A canceled job never exposes a
// completed result: the late result is discarded instead of being applied.
type BackgroundJob struct {
	ID             string
	Name           string
	WorkID         string
	Status         BackgroundJobStatus
	StartedAt      time.Time
	FinishedAt     time.Time
	Result         production.ActionResult
	Err            error
	SuppressNotice bool
}

type backgroundJob struct {
	BackgroundJob
	cancel context.CancelFunc
}

type backgroundResult struct {
	JobID       string
	WorkID      string
	Result      production.ActionResult
	Err         error
	Recovered   bool
	Interrupted bool
	Canceled    bool
}

type backgroundResultQueue struct {
	mu    sync.Mutex
	items []backgroundResult
	store *backgroundResultStore
}

func (q *backgroundResultQueue) setStore(store *backgroundResultStore) error {
	items, err := store.Load()
	if err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.store = store
	q.items = items
	return nil
}

func (q *backgroundResultQueue) enqueue(item backgroundResult) {
	q.mu.Lock()
	key := backgroundResultKey(item)
	for _, existing := range q.items {
		if backgroundResultKey(existing) == key {
			q.mu.Unlock()
			return
		}
	}
	q.items = append(q.items, item)
	_ = q.store.Save(q.items)
	q.mu.Unlock()
}

func (q *backgroundResultQueue) take() []backgroundResult {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	out := q.items
	q.items = nil
	_ = q.store.Save(nil)
	return out
}

type backgroundJobRegistry struct {
	mu   sync.Mutex
	jobs map[string]*backgroundJob
	seq  atomic.Uint64
}

func (r *backgroundJobRegistry) start(ctx context.Context, name, workID string, work func(context.Context) (production.ActionResult, error), onComplete func(BackgroundJob)) string {
	if ctx == nil {
		ctx = context.Background()
	}
	jobCtx, cancel := context.WithCancel(ctx)
	id := fmt.Sprintf("job_%d_%d", time.Now().UnixNano(), r.seq.Add(1))
	job := &backgroundJob{
		BackgroundJob: BackgroundJob{
			ID:        id,
			Name:      name,
			WorkID:    workID,
			Status:    BackgroundJobRunning,
			StartedAt: time.Now().UTC(),
		},
		cancel: cancel,
	}

	r.mu.Lock()
	if r.jobs == nil {
		r.jobs = make(map[string]*backgroundJob)
	}
	r.jobs[id] = job
	r.mu.Unlock()

	go func() {
		result, err := work(jobCtx)
		r.mu.Lock()
		current, ok := r.jobs[id]
		if !ok {
			r.mu.Unlock()
			return
		}
		current.FinishedAt = time.Now().UTC()
		if current.Status == BackgroundJobCanceled || jobCtx.Err() != nil {
			current.Status = BackgroundJobCanceled
			current.Err = jobCtx.Err()
			// A late provider result is never applied as if the job were still current.
			current.Result = production.ActionResult{}
		} else if err != nil {
			current.Status = BackgroundJobFailed
			current.Err = err
		} else {
			current.Status = BackgroundJobCompleted
			current.Result = result
		}
		snapshot := current.BackgroundJob
		r.mu.Unlock()
		if onComplete != nil {
			onComplete(snapshot)
		}
	}()

	return id
}

func (r *backgroundJobRegistry) snapshot(id string) (BackgroundJob, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return BackgroundJob{}, false
	}
	return job.BackgroundJob, true
}

func (r *backgroundJobRegistry) cancel(id string, suppressNotice bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return false
	}
	if job.Status != BackgroundJobRunning {
		return false
	}
	job.cancel()
	job.Status = BackgroundJobCanceled
	job.FinishedAt = time.Now().UTC()
	job.Err = context.Canceled
	job.Result = production.ActionResult{}
	job.SuppressNotice = suppressNotice
	return true
}

func (r *backgroundJobRegistry) all() []BackgroundJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]BackgroundJob, 0, len(r.jobs))
	for _, job := range r.jobs {
		out = append(out, job.BackgroundJob)
	}
	return out
}

// StartBackgroundJob runs work outside the interactive turn lock. The caller
// owns the work function and its authority; this registry only tracks lifecycle,
// cancellation and the late-result rule.
func (e *Engine) StartBackgroundJob(ctx context.Context, name string, work func(context.Context) (production.ActionResult, error)) string {
	if e == nil || work == nil {
		return ""
	}
	return e.backgroundJobs.start(ctx, name, "", work, nil)
}

// StartBackgroundJobWithCompletion is the same as StartBackgroundJob but
// invokes onComplete with the final snapshot after the job leaves running.
func (e *Engine) StartBackgroundJobWithCompletion(ctx context.Context, name string, work func(context.Context) (production.ActionResult, error), onComplete func(BackgroundJob)) string {
	if e == nil || work == nil {
		return ""
	}
	return e.backgroundJobs.start(ctx, name, "", work, onComplete)
}

// StartBackgroundJobForWork is the production variant: it binds the job to the
// durable WorkSession so cancellation and recovery can find its owner.
func (e *Engine) StartBackgroundJobForWork(ctx context.Context, name, workID string, work func(context.Context) (production.ActionResult, error), onComplete func(BackgroundJob)) string {
	if e == nil || work == nil {
		return ""
	}
	return e.backgroundJobs.start(ctx, name, workID, work, onComplete)
}

// deliverBackgroundResults announces completed background work at the start
// of the next turn. It never applies a canceled or failed job's result.
func (e *Engine) deliverBackgroundResults(input string, channel interaction.InputChannel) {
	if e == nil {
		return
	}
	results := e.backgroundResults.take()
	if len(results) == 0 {
		return
	}
	req := interaction.Parse(input, channel)
	for _, item := range results {
		text := backgroundResultText(item)
		if strings.TrimSpace(text) == "" {
			continue
		}
		_ = e.renderCanonicalSystemResponse(req, "background_result", text, interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
	}
}

func backgroundResultText(item backgroundResult) string {
	if item.Canceled || errors.Is(item.Err, errBackgroundJobCanceled) {
		return "Cancelé el trabajo en segundo plano. No aplicaré un resultado tardío; el trabajo queda bloqueado para reconciliar."
	}
	if item.Interrupted || errors.Is(item.Err, errInterruptedWork) {
		return "El trabajo en segundo plano quedó interrumpido por un reinicio. No lo repetiré sin reconciliar antes el estado del proyecto."
	}
	if item.Err != nil {
		return "El trabajo en segundo plano falló: " + item.Err.Error()
	}
	if item.Recovered {
		summary := strings.TrimSpace(item.Result.Summary)
		if item.Result.Success && item.Result.Verified {
			if summary == "" {
				summary = "archivos y pruebas verificados"
			}
			return "Reconcilié el trabajo interrumpido: " + summary
		}
		reason := strings.TrimSpace(item.Result.FailureReason)
		if reason == "" {
			reason = "el efecto no se pudo verificar"
		}
		return "El trabajo interrumpido no se pudo reconciliar: " + reason
	}
	summary := strings.TrimSpace(item.Result.Summary)
	if item.Result.Success && item.Result.Verified {
		if summary == "" {
			summary = "terminado y verificado"
		}
		return "Terminé el trabajo en segundo plano: " + summary
	}
	reason := strings.TrimSpace(item.Result.FailureReason)
	if reason == "" {
		reason = strings.TrimSpace(item.Result.OutcomeCode)
	}
	if reason == "" {
		reason = "sin detalle"
	}
	return "El trabajo en segundo plano terminó con un problema: " + reason
}

// EnableBackgroundResultPersistence loads pending results from path and
// persists the queue after every enqueue/take. It is called by the composition
// root; tests can use a temporary path.
func (e *Engine) EnableBackgroundResultPersistence(path string) error {
	if e == nil {
		return fmt.Errorf("background results: engine nil")
	}
	return e.backgroundResults.setStore(OpenBackgroundResultStore(path))
}

// EnableAsyncProduction opts this Engine into background execution for
// programming production. It is explicit because notification and
// reconciliation are not yet complete for every production family.
func (e *Engine) EnableAsyncProduction() {
	if e != nil {
		e.asyncProduction = true
	}
}

// DisableAsyncProduction returns the Engine to synchronous production.
func (e *Engine) DisableAsyncProduction() {
	if e != nil {
		e.asyncProduction = false
	}
}

// BackgroundJob returns a snapshot of one job.
func (e *Engine) BackgroundJob(id string) (BackgroundJob, bool) {
	if e == nil {
		return BackgroundJob{}, false
	}
	return e.backgroundJobs.snapshot(id)
}

// BackgroundJobs returns snapshots of all known jobs.
func (e *Engine) BackgroundJobs() []BackgroundJob {
	if e == nil {
		return nil
	}
	return e.backgroundJobs.all()
}

// blockCanceledWork stops the durable Work and releases conversational
// ownership so a replacement objective can start a new Work instead of
// reusing a canceled one.
func (e *Engine) blockCanceledWork(workID string) {
	workID = strings.TrimSpace(workID)
	if workID == "" || e == nil || e.workStore == nil {
		return
	}
	if work, ok := e.workStore.Get(workID); ok && work.Status != production.WorkBlocked {
		_ = e.workStore.Block(workID, "background job canceled; reconcile before retry")
	}
	if e.conversationState != nil && e.conversationState.ActiveWork() == workID {
		e.conversationState.SetActiveWork("")
		_ = e.conversationState.Save(interaction.ConversationStatePath)
	}
}

// SetBackgroundNotifier installs the proactive delivery sink for completed
// background work. If nil, results stay in the durable queue and are announced
// at the next turn. A real channel adapter can call this method.
func (e *Engine) SetBackgroundNotifier(notifier func(string)) {
	if e != nil {
		e.backgroundNotifier = notifier
	}
}

// enqueueBackgroundResult delivers proactively when a notifier exists;
// otherwise it persists the result for the next turn.
func (e *Engine) enqueueBackgroundResult(item backgroundResult) {
	if e == nil {
		return
	}
	if e.backgroundNotifier != nil {
		if text := strings.TrimSpace(backgroundResultText(item)); text != "" {
			e.backgroundNotifier(text)
			return
		}
	}
	e.backgroundResults.enqueue(item)
}

func (e *Engine) RecoverPendingWork(ctx context.Context) (reconciled, blocked int) {
	if e == nil {
		return 0, 0
	}
	reconciled = e.ReconcileInterruptedProgrammingWork(ctx)
	blocked = e.RecoverInterruptedWork()
	return reconciled, blocked
}

// CancelBackgroundJob cancels a running job. A late result is discarded.
func (e *Engine) CancelBackgroundJob(id string) bool {
	if e == nil {
		return false
	}
	return e.backgroundJobs.cancel(id, false)
}

// ReconcileInterruptedProgrammingWork resolves durable programming attempts
// whose effect specification survived a restart. It verifies the expected
// files and runs build/tests, then marks the attempt succeeded or failed.
// Attempts it cannot observe remain blocked for the conservative path.
func (e *Engine) ReconcileInterruptedProgrammingWork(ctx context.Context) int {
	if e == nil || e.workStore == nil {
		return 0
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reconciled := 0
	for _, work := range e.workStore.List() {
		for _, attempt := range work.Attempts {
			if attempt.Status != "unverified" || len(attempt.EffectSpecification) == 0 {
				continue
			}
			var plan programming.MultiFilePlan
			if err := json.Unmarshal(attempt.EffectSpecification, &plan); err != nil {
				continue
			}
			result, err := programming.ReconcileMultiFilePlan(ctx, plan)
			if err != nil {
				continue
			}
			result.WorkID = work.ID
			result.AttemptNumber = attempt.Number
			if err := e.workStore.ReconcileUnknownAttempt(work.ID, attempt.Number, result); err != nil {
				continue
			}
			if result.Success {
				_ = e.workStore.Complete(work.ID, true, "reconciled after restart")
			} else {
				_ = e.workStore.Complete(work.ID, false, result.FailureReason)
			}
			e.enqueueBackgroundResult(backgroundResult{
				WorkID: work.ID, Result: result, Recovered: true,
			})
			reconciled++
			break
		}
	}
	return reconciled
}

// RecoverInterruptedWork reconciles the durable WorkStore after a restart.
// Any attempt left running or unverified is blocked and announced; it is never
// silently repeated. A later, explicit reconciliation may resolve it.
func (e *Engine) RecoverInterruptedWork() int {
	if e == nil || e.workStore == nil {
		return 0
	}
	recovered := 0
	for _, work := range e.workStore.List() {
		already := false
		for _, event := range work.Events {
			if strings.Contains(event.Detail, "work interrupted by process restart") {
				already = true
				break
			}
		}
		if already {
			continue
		}
		interrupted := false
		jobID := ""
		for _, attempt := range work.Attempts {
			if attempt.Status != "running" && attempt.Status != "unverified" {
				continue
			}
			interrupted = true
			if jobID == "" {
				jobID = attempt.JobID
			}
			break
		}
		if !interrupted {
			continue
		}
		_ = e.workStore.Block(work.ID, "work interrupted by process restart; reconcile before retry")
		e.enqueueBackgroundResult(backgroundResult{
			JobID: jobID, WorkID: work.ID,
			Err:         fmt.Errorf("%w: %s", errInterruptedWork, work.ID),
			Interrupted: true,
		})
		recovered++
	}
	return recovered
}
