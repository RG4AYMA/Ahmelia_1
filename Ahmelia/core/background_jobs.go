package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"ahmelia/production"
)

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
	ID         string
	Name       string
	Status     BackgroundJobStatus
	StartedAt  time.Time
	FinishedAt time.Time
	Result     production.ActionResult
	Err        error
}

type backgroundJob struct {
	BackgroundJob
	cancel context.CancelFunc
}

type backgroundJobRegistry struct {
	mu   sync.Mutex
	jobs map[string]*backgroundJob
	seq  atomic.Uint64
}

func (r *backgroundJobRegistry) start(ctx context.Context, name string, work func(context.Context) (production.ActionResult, error)) string {
	if ctx == nil {
		ctx = context.Background()
	}
	jobCtx, cancel := context.WithCancel(ctx)
	id := fmt.Sprintf("job_%d_%d", time.Now().UnixNano(), r.seq.Add(1))
	job := &backgroundJob{
		BackgroundJob: BackgroundJob{
			ID:        id,
			Name:      name,
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
		defer r.mu.Unlock()
		current, ok := r.jobs[id]
		if !ok {
			return
		}
		current.FinishedAt = time.Now().UTC()
		if current.Status == BackgroundJobCanceled || jobCtx.Err() != nil {
			current.Status = BackgroundJobCanceled
			current.Err = jobCtx.Err()
			// A late provider result is never applied as if the job were still current.
			current.Result = production.ActionResult{}
			return
		}
		if err != nil {
			current.Status = BackgroundJobFailed
			current.Err = err
			return
		}
		current.Status = BackgroundJobCompleted
		current.Result = result
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

func (r *backgroundJobRegistry) cancel(id string) bool {
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
	return e.backgroundJobs.start(ctx, name, work)
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

// CancelBackgroundJob cancels a running job. A late result is discarded.
func (e *Engine) CancelBackgroundJob(id string) bool {
	if e == nil {
		return false
	}
	return e.backgroundJobs.cancel(id)
}
