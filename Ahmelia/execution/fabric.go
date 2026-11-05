package execution

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Job is the provider-neutral unit of work dispatched by the Execution Fabric.
// It deliberately carries requirements rather than a concrete machine choice.
type Job struct {
	ID           string
	WorkID       string
	TaskID       string
	CapabilityID string
	Operation    string
	Requirements Requirements
	CreatedAt    time.Time
	Timeout      time.Duration
}

type Requirements struct {
	Domains      []string
	Capabilities []string
	MinCPU       int
	MinMemoryMB  int
	RequiresGPU  bool
	Resources    ResourceRequirements
}

// Attempt records one physical execution lease and its lifecycle.
type Attempt struct {
	ID        string
	JobID     string
	WorkerID  string
	LeaseID   string
	StartedAt time.Time
	EndedAt   time.Time
	Success   bool
	Err       error
}

// Worker is a physical/local/remote execution endpoint. Providers remain
// domain implementations; workers provide the execution substrate.
type Worker interface {
	ID() string
	Capabilities() WorkerCapabilities
	Heartbeat(context.Context) error
	Execute(context.Context, Job, func(context.Context) error) error
}

type WorkerCapabilities struct {
	Domains         map[string]bool
	Capabilities    map[string]bool
	CPU             int
	MemoryMB        int
	GPU             bool
	ConcurrentSlots int
}

func (c WorkerCapabilities) CanRun(r Requirements) bool {
	if c.CPU < r.MinCPU || c.MemoryMB < r.MinMemoryMB || (r.RequiresGPU && !c.GPU) {
		return false
	}
	for _, d := range r.Domains {
		if !c.Domains[d] {
			return false
		}
	}
	for _, cap := range r.Capabilities {
		if !c.Capabilities[cap] {
			return false
		}
	}
	return true
}

// Registry is the single authority for known execution workers.
type Registry struct {
	mu      sync.RWMutex
	workers map[string]Worker
	beats   map[string]time.Time
	active  map[string]int
}

func NewRegistry() *Registry {
	return &Registry{workers: map[string]Worker{}, beats: map[string]time.Time{}, active: map[string]int{}}
}

func (r *Registry) Register(w Worker) error {
	if r == nil || w == nil || w.ID() == "" {
		return fmt.Errorf("execution: invalid worker")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workers[w.ID()]; exists {
		return fmt.Errorf("execution: worker %q already registered", w.ID())
	}
	r.workers[w.ID()] = w
	r.beats[w.ID()] = time.Now()
	return nil
}

func (r *Registry) Heartbeat(ctx context.Context, id string) error {
	r.mu.RLock()
	w, ok := r.workers[id]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("execution: worker %q not found", id)
	}
	if err := w.Heartbeat(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	r.beats[id] = time.Now()
	r.mu.Unlock()
	return nil
}

func (r *Registry) Snapshot() []WorkerStatus {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]WorkerStatus, 0, len(r.workers))
	for id, w := range r.workers {
		caps := w.Capabilities()
		slots := caps.ConcurrentSlots
		if slots < 1 {
			slots = 1
		}
		active := r.active[id]
		available := slots - active
		if available < 0 {
			available = 0
		}
		out = append(out, WorkerStatus{ID: id, Capabilities: caps, LastHeartbeat: r.beats[id], ActiveSlots: active, AvailableSlots: available})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type WorkerStatus struct {
	ID             string
	Capabilities   WorkerCapabilities
	LastHeartbeat  time.Time
	ActiveSlots    int
	AvailableSlots int
}

func (r *Registry) isHealthyLocked(id string, now time.Time, maxAge time.Duration) bool {
	if maxAge <= 0 {
		return true
	}
	beat := r.beats[id]
	return !beat.IsZero() && now.Sub(beat) <= maxAge
}

func (r *Registry) RefreshHeartbeats(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.RLock()
	workers := make([]Worker, 0, len(r.workers))
	for _, w := range r.workers {
		workers = append(workers, w)
	}
	r.mu.RUnlock()
	for _, w := range workers {
		if w == nil {
			continue
		}
		_ = r.Heartbeat(ctx, w.ID())
	}
}

func (r *Registry) Select(req Requirements) (Worker, error) {
	return r.SelectHealthy(req, time.Now(), 0)
}

// SelectHealthy chooses a healthy worker with the most available execution
// capacity. Ties are deterministic. This is the first fleet-level scheduling
// policy: capability fit comes first, then current load.
func (r *Registry) SelectHealthy(req Requirements, now time.Time, heartbeatMaxAge time.Duration) (Worker, error) {
	if r == nil {
		return nil, fmt.Errorf("execution: registry nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.workers))
	for id := range r.workers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var selected Worker
	bestAvailable := -1
	for _, id := range ids {
		if !r.isHealthyLocked(id, now, heartbeatMaxAge) {
			continue
		}
		w := r.workers[id]
		caps := w.Capabilities()
		if !caps.CanRun(req) {
			continue
		}
		if req.Resources != (ResourceRequirements{}) {
			snapshot, supported, err := readWorkerResources(context.Background(), w)
			if err != nil || (supported && !req.Resources.Allows(snapshot)) {
				continue
			}
		}
		slots := caps.ConcurrentSlots
		if slots < 1 {
			slots = 1
		}
		available := slots - r.active[id]
		if available <= 0 || available <= bestAvailable {
			continue
		}
		selected, bestAvailable = w, available
	}
	if selected == nil {
		return nil, fmt.Errorf("execution: no healthy worker with available capacity satisfies requirements")
	}
	r.active[selected.ID()]++
	return selected, nil
}

func (r *Registry) Release(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active[id] > 0 {
		r.active[id]--
	}
}

// Dispatcher owns job dispatch, lease lifetime, and attempt recording. It does
// not decide what the job means; the caller already supplied the resolved job.
type Dispatcher struct {
	registry        *Registry
	heartbeatMaxAge time.Duration
	mu              sync.Mutex
	attempts        []Attempt
	seq             uint64
}

func NewDispatcher(r *Registry) (*Dispatcher, error) {
	if r == nil {
		return nil, fmt.Errorf("execution: registry nil")
	}
	return &Dispatcher{registry: r, heartbeatMaxAge: 30 * time.Second}, nil
}
func (d *Dispatcher) RegisterWorker(w Worker) error {
	if d == nil || d.registry == nil {
		return fmt.Errorf("execution: dispatcher unavailable")
	}
	return d.registry.Register(w)
}

func (d *Dispatcher) Dispatch(ctx context.Context, job Job, run func(context.Context) error) (Attempt, error) {
	if d == nil || d.registry == nil {
		return Attempt{}, fmt.Errorf("execution: dispatcher unavailable")
	}
	if job.ID == "" {
		return Attempt{}, fmt.Errorf("execution: job ID vacío")
	}
	if run == nil {
		return Attempt{}, fmt.Errorf("execution: nil job operation")
	}
	// Liveness is refreshed at the execution boundary so a worker that is
	// healthy but simply idle between jobs is not misclassified as stale.
	// Remote workers can still fail their heartbeat and remain ineligible.
	d.registry.RefreshHeartbeats(ctx)
	worker, err := d.registry.SelectHealthy(job.Requirements, time.Now(), d.heartbeatMaxAge)
	if err != nil {
		return Attempt{}, err
	}
	defer d.registry.Release(worker.ID())
	d.mu.Lock()
	d.seq++
	id := fmt.Sprintf("%s-execution", job.ID)
	lease := fmt.Sprintf("lease-%d", d.seq)
	d.mu.Unlock()
	a := Attempt{ID: id, JobID: job.ID, WorkerID: worker.ID(), LeaseID: lease, StartedAt: time.Now()}
	execCtx := ctx
	cancel := func() {}
	if job.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, job.Timeout)
	}
	defer cancel()
	err = worker.Execute(execCtx, job, run)
	a.EndedAt = time.Now()
	a.Err = err
	a.Success = err == nil
	d.mu.Lock()
	d.attempts = append(d.attempts, a)
	d.mu.Unlock()
	return a, err
}

func (d *Dispatcher) Attempts() []Attempt {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Attempt(nil), d.attempts...)
}

// LocalWorker is the canonical first worker. Remote workers can implement the
// same contract without changing the cognitive or provider layers.
type LocalWorker struct {
	WorkerID string
	Caps     WorkerCapabilities
}

func NewLocalWorker(id string, c WorkerCapabilities) (*LocalWorker, error) {
	if id == "" {
		return nil, fmt.Errorf("execution: local worker ID vacío")
	}
	return &LocalWorker{WorkerID: id, Caps: c}, nil
}
func (w *LocalWorker) ID() string                       { return w.WorkerID }
func (w *LocalWorker) Capabilities() WorkerCapabilities { return w.Caps }
func (w *LocalWorker) Heartbeat(context.Context) error  { return nil }
func (w *LocalWorker) Execute(ctx context.Context, _ Job, run func(context.Context) error) error {
	return run(ctx)
}
