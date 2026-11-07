package consult

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Worker is the asynchronous lane for consultation. It keeps at most one model
// call in flight, stores the latest proposal with its fingerprint and expires
// it after a TTL. The caller never waits for the model: it submits and later
// asks for a fresh proposal.
type Worker struct {
	mu          sync.Mutex
	client      Client
	ttl         time.Duration
	now         func() time.Time
	running     bool
	generation  uint64
	fingerprint string
	proposal    Proposal
	decidedAt   time.Time
	lastErr     error
	cancel      context.CancelFunc
}

// NewWorker builds a worker with a default TTL of two minutes. A nil client
// makes Submit inert, which is the safe default.
func NewWorker(client Client, ttl time.Duration) *Worker {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &Worker{client: client, ttl: ttl, now: time.Now}
}

// SetClock injects a clock for tests. It must be called before Submit.
func (w *Worker) SetClock(now func() time.Time) {
	if w == nil || now == nil {
		return
	}
	w.mu.Lock()
	w.now = now
	w.mu.Unlock()
}

// Submit starts one consultation for the fingerprint. It never blocks and
// returns false when another call is already in flight or the client is nil.
func (w *Worker) Submit(ctx context.Context, req Request, fingerprint string) bool {
	if w == nil || w.client == nil {
		return false
	}
	fp := strings.TrimSpace(fingerprint)
	if fp == "" {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return false
	}
	if _, ok := w.freshLocked(fp); ok {
		w.mu.Unlock()
		return false
	}
	w.generation++
	gen := w.generation
	w.running = true
	w.fingerprint = fp
	w.proposal = Proposal{}
	w.decidedAt = time.Time{}
	w.lastErr = nil
	callCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			if w.generation == gen {
				w.running = false
				w.cancel = nil
			}
			w.mu.Unlock()
			cancel()
		}()

		proposal, err := Consult(callCtx, w.client, req)
		w.mu.Lock()
		if w.generation != gen {
			w.mu.Unlock()
			return
		}
		w.running = false
		w.lastErr = err
		if err == nil {
			w.proposal = proposal
			w.decidedAt = w.now()
		}
		w.mu.Unlock()
	}()
	return true
}

// Fresh returns the proposal for the fingerprint if it is current and inside
// the TTL. An error or a different fingerprint is not fresh.
func (w *Worker) Fresh(fingerprint string) (Proposal, bool) {
	if w == nil {
		return Proposal{}, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.freshLocked(strings.TrimSpace(fingerprint))
}

func (w *Worker) freshLocked(fingerprint string) (Proposal, bool) {
	if fingerprint == "" || w.fingerprint != fingerprint || w.decidedAt.IsZero() || w.lastErr != nil {
		return Proposal{}, false
	}
	now := w.now
	if now == nil {
		now = time.Now
	}
	if now().Sub(w.decidedAt) > w.ttl {
		return Proposal{}, false
	}
	return w.proposal, true
}

// LastError reports the last model error. It is evidence, not a proposal.
func (w *Worker) LastError() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastErr
}

// Running reports whether a call is in flight.
func (w *Worker) Running() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// Invalidate cancels the in-flight call and drops the stored proposal. A new
// turn or a changed context uses it so a stale reading cannot be consumed.
func (w *Worker) Invalidate() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.generation++
	w.running = false
	w.fingerprint = ""
	w.proposal = Proposal{}
	w.decidedAt = time.Time{}
	w.lastErr = nil
	cancel := w.cancel
	w.cancel = nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Stop is Invalidate with a name that says the worker is being torn down.
func (w *Worker) Stop() { w.Invalidate() }
