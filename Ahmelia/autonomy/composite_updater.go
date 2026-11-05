package autonomy

import (
	"context"
	"fmt"
)

// CompositeCycleUpdater compone varios CycleUpdater sobre el mismo resultado.
// B88 lo usa para que GoalPlanningAdapter y LearningMemoryAdapter reciban el
// cierre de una misma acción sin que Cycle tenga que conocer implementaciones.
type CompositeCycleUpdater struct {
	updaters []CycleUpdater
}

func NewCompositeCycleUpdater(updaters ...CycleUpdater) *CompositeCycleUpdater {
	filtered := make([]CycleUpdater, 0, len(updaters))
	for _, u := range updaters {
		if u != nil {
			filtered = append(filtered, u)
		}
	}
	return &CompositeCycleUpdater{updaters: filtered}
}

func (u *CompositeCycleUpdater) UpdateState(ctx context.Context, req ActionRequest, verif VerificationResult) error {
	for i, updater := range u.updaters {
		if err := updater.UpdateState(ctx, req, verif); err != nil {
			return fmt.Errorf("autonomy: composite updater [%d]: %w", i, err)
		}
	}
	return nil
}
