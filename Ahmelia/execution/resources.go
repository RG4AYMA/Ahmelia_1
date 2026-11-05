package execution

import (
	"context"
	"fmt"
	"time"
)

// ResourceSnapshot is the current load of a worker. It is deliberately
// separate from WorkerCapabilities: capabilities describe what a worker can
// do; resources describe whether it should do it now.
type ResourceSnapshot struct {
	Timestamp       time.Time
	CPUPercent      float64
	RAMPercent      float64
	GPUMemPercent   float64
	GPUUsagePercent float64
	HasGPU          bool
}

// ResourceProvider is an optional worker contract. Workers that expose live
// telemetry become resource-aware without changing the canonical Worker
// interface or forcing every worker implementation to have sensors.
type ResourceProvider interface {
	Resources(context.Context) (ResourceSnapshot, error)
}

// ResourceRequirements constrain scheduling by current load, not hardware.
// Zero values mean "no constraint".
type ResourceRequirements struct {
	MaxCPUPercent      float64
	MaxRAMPercent      float64
	MaxGPUUsagePercent float64
	MaxGPUMemPercent   float64
}

func (r ResourceRequirements) Allows(s ResourceSnapshot) bool {
	if r.MaxCPUPercent > 0 && s.CPUPercent > r.MaxCPUPercent {
		return false
	}
	if r.MaxRAMPercent > 0 && s.RAMPercent > r.MaxRAMPercent {
		return false
	}
	if r.MaxGPUUsagePercent > 0 && (!s.HasGPU || s.GPUUsagePercent > r.MaxGPUUsagePercent) {
		return false
	}
	if r.MaxGPUMemPercent > 0 && (!s.HasGPU || s.GPUMemPercent > r.MaxGPUMemPercent) {
		return false
	}
	return true
}

func readWorkerResources(ctx context.Context, w Worker) (ResourceSnapshot, bool, error) {
	rp, ok := w.(ResourceProvider)
	if !ok {
		return ResourceSnapshot{}, false, nil
	}
	snapshot, err := rp.Resources(ctx)
	if err != nil {
		return ResourceSnapshot{}, true, fmt.Errorf("execution: worker %q resource telemetry: %w", w.ID(), err)
	}
	if snapshot.Timestamp.IsZero() {
		snapshot.Timestamp = time.Now()
	}
	return snapshot, true, nil
}
