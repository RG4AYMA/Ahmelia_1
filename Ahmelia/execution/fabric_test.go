package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDispatcherSelectsWorkerByRequirements(t *testing.T) {
	r := NewRegistry()
	small, _ := NewLocalWorker("small", WorkerCapabilities{CPU: 1, MemoryMB: 512})
	gpu, _ := NewLocalWorker("gpu", WorkerCapabilities{CPU: 8, MemoryMB: 16384, GPU: true, Domains: map[string]bool{"3d": true}, Capabilities: map[string]bool{"blender": true}})
	if err := r.Register(small); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(gpu); err != nil {
		t.Fatal(err)
	}
	d, _ := NewDispatcher(r)
	job := Job{ID: "job-1", Requirements: Requirements{Domains: []string{"3d"}, Capabilities: []string{"blender"}, MinCPU: 4, MinMemoryMB: 4096, RequiresGPU: true}}
	got, err := d.Dispatch(context.Background(), job, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkerID != "gpu" || !got.Success || got.LeaseID == "" || got.ID != "job-1-execution" {
		t.Fatalf("unexpected attempt: %+v", got)
	}
}

func TestDispatcherRecordsFailureAndPreservesAttempt(t *testing.T) {
	r := NewRegistry()
	w, _ := NewLocalWorker("local", WorkerCapabilities{CPU: 1, MemoryMB: 1})
	_ = r.Register(w)
	d, _ := NewDispatcher(r)
	want := errors.New("boom")
	got, err := d.Dispatch(context.Background(), Job{ID: "job-2"}, func(context.Context) error { return want })
	if !errors.Is(err, want) || got.Success || got.EndedAt.IsZero() {
		t.Fatalf("unexpected failure attempt: %+v err=%v", got, err)
	}
	attempts := d.Attempts()
	if len(attempts) != 1 || !errors.Is(attempts[0].Err, want) {
		t.Fatalf("attempt history lost: %+v", attempts)
	}
}

func TestDispatcherTimeoutIsEnforcedByFabric(t *testing.T) {
	r := NewRegistry()
	w, _ := NewLocalWorker("local", WorkerCapabilities{CPU: 1, MemoryMB: 1})
	_ = r.Register(w)
	d, _ := NewDispatcher(r)
	_, err := d.Dispatch(context.Background(), Job{ID: "job-3", Timeout: 10 * time.Millisecond}, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

func TestRegistryRejectsDuplicateWorkerIdentity(t *testing.T) {
	r := NewRegistry()
	a, _ := NewLocalWorker("same", WorkerCapabilities{CPU: 1, MemoryMB: 1})
	b, _ := NewLocalWorker("same", WorkerCapabilities{CPU: 2, MemoryMB: 2})
	if err := r.Register(a); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(b); err == nil {
		t.Fatal("duplicate worker identity accepted")
	}
}

func TestRegistrySkipsStaleWorkersAndUsesHealthyWorker(t *testing.T) {
	r := NewRegistry()
	stale, _ := NewLocalWorker("stale", WorkerCapabilities{CPU: 4, MemoryMB: 4096})
	healthy, _ := NewLocalWorker("healthy", WorkerCapabilities{CPU: 4, MemoryMB: 4096})
	if err := r.Register(stale); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(healthy); err != nil {
		t.Fatal(err)
	}
	// Make the first worker stale without touching the worker itself.
	r.mu.Lock()
	r.beats[stale.ID()] = time.Now().Add(-time.Minute)
	r.mu.Unlock()
	got, err := r.SelectHealthy(Requirements{MinCPU: 1}, time.Now(), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != healthy.ID() {
		t.Fatalf("selected stale worker %q", got.ID())
	}
}

func TestDispatcherHonorsWorkerConcurrentCapacity(t *testing.T) {
	r := NewRegistry()
	w, _ := NewLocalWorker("single-slot", WorkerCapabilities{CPU: 1, MemoryMB: 1, ConcurrentSlots: 1})
	if err := r.Register(w); err != nil {
		t.Fatal(err)
	}
	d, _ := NewDispatcher(r)
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := d.Dispatch(context.Background(), Job{ID: "job-capacity-1"}, func(context.Context) error {
			close(started)
			<-release
			return nil
		})
		firstDone <- err
	}()
	<-started
	if _, err := d.Dispatch(context.Background(), Job{ID: "job-capacity-2"}, func(context.Context) error { return nil }); err == nil {
		t.Fatal("second job bypassed worker capacity")
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if _, err := d.Dispatch(context.Background(), Job{ID: "job-capacity-3"}, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("worker capacity was not released: %v", err)
	}
}

func TestDispatcherRefreshesIdleWorkerLivenessBeforeDispatch(t *testing.T) {
	w, err := NewLocalWorker("idle", WorkerCapabilities{CPU: 1, MemoryMB: 1, ConcurrentSlots: 1})
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDispatcher(NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	// The worker has been registered, then left idle longer than the normal
	// heartbeat freshness window. Dispatch must refresh liveness at the
	// execution boundary instead of treating idleness as worker death.
	time.Sleep(35 * time.Millisecond)
	d.heartbeatMaxAge = 10 * time.Millisecond
	_, err = d.Dispatch(context.Background(), Job{ID: "job-1"}, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("dispatch after idle heartbeat refresh failed: %v", err)
	}
}
