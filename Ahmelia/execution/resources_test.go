package execution

import (
	"context"
	"testing"
	"time"
)

type resourceWorker struct {
	id        string
	caps      WorkerCapabilities
	resources ResourceSnapshot
}

func (w resourceWorker) ID() string                       { return w.id }
func (w resourceWorker) Capabilities() WorkerCapabilities { return w.caps }
func (w resourceWorker) Heartbeat(context.Context) error  { return nil }
func (w resourceWorker) Execute(ctx context.Context, _ Job, run func(context.Context) error) error {
	return run(ctx)
}
func (w resourceWorker) Resources(context.Context) (ResourceSnapshot, error) { return w.resources, nil }

func TestRegistrySkipsWorkerOverResourceBudget(t *testing.T) {
	r := NewRegistry()
	now := time.Now()
	busy := resourceWorker{id: "busy", caps: WorkerCapabilities{CPU: 8, MemoryMB: 16000, ConcurrentSlots: 1}, resources: ResourceSnapshot{Timestamp: now, CPUPercent: 92, RAMPercent: 40}}
	free := resourceWorker{id: "free", caps: WorkerCapabilities{CPU: 8, MemoryMB: 16000, ConcurrentSlots: 1}, resources: ResourceSnapshot{Timestamp: now, CPUPercent: 20, RAMPercent: 40}}
	if err := r.Register(busy); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(free); err != nil {
		t.Fatal(err)
	}
	w, err := r.SelectHealthy(Requirements{MinCPU: 4, Resources: ResourceRequirements{MaxCPUPercent: 80}}, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if w.ID() != "free" {
		t.Fatalf("expected free worker, got %s", w.ID())
	}
}

func TestResourceRequirementsRejectMissingGPUForGPUUsageConstraint(t *testing.T) {
	req := ResourceRequirements{MaxGPUUsagePercent: 80}
	if req.Allows(ResourceSnapshot{HasGPU: false}) {
		t.Fatal("GPU-constrained job must not run without GPU telemetry")
	}
}
