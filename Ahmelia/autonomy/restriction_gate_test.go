package autonomy

import (
	"context"
	"testing"
	"time"
)

type fixedRestrictions bool

func (f fixedRestrictions) OwnerWorkInitiativeRestricted(time.Time) bool { return bool(f) }

func newRestrictionGateLoop(observer *ContextObserver, hasPending bool, goalID string) *AutonomousLoop {
	return NewAutonomousLoop(
		LoopConfig{Interval: time.Second},
		observer,
		NewInitiativeEngine(InitiativeConfig{}),
		NewConversationManager(),
		&mockStateProvider{lastActivity: time.Now().Add(-time.Hour)},
		&mockGoalProvider{hasPending: hasPending, goalID: goalID},
	)
}

func TestWorkRestrictionClosesResumeGoal(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	observer.Observe(TypeGoalStateChanged, "GoalMonitor", map[string]interface{}{
		"goal_id": "goal_1", "status": "active",
	})
	loop := newRestrictionGateLoop(observer, true, "goal_1")
	loop.SetOwnerRestrictions(fixedRestrictions(true), func() time.Time {
		return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	})

	loop.runIntegratedTick(context.Background())

	if got := len(loop.OutputChan()); got != 0 {
		t.Fatalf("decisiones emitidas = %d, quería 0 con la iniciativa cerrada", got)
	}
	if got := observer.GetUnprocessedObservations(); len(got) != 0 {
		t.Fatalf("observaciones sin procesar = %d, quería 0", len(got))
	}
}

func TestWorkRestrictionKeepsCriticalSignals(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	observer.Observe(TypeSystemSignal, "monitor", map[string]interface{}{"severity": "CRITICAL"})
	loop := newRestrictionGateLoop(observer, false, "")
	loop.SetOwnerRestrictions(fixedRestrictions(true), nil)

	loop.runIntegratedTick(context.Background())

	select {
	case decision := <-loop.OutputChan():
		if decision.Strategy != StrategyInterruptLightly {
			t.Fatalf("estrategia = %s, quería %s", decision.Strategy, StrategyInterruptLightly)
		}
	default:
		t.Fatal("la restricción de trabajo no puede silenciar una señal crítica")
	}
}

func TestWithoutRestrictionResumeGoalStillFires(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	loop := newRestrictionGateLoop(observer, true, "goal_1")

	loop.runIntegratedTick(context.Background())

	select {
	case decision := <-loop.OutputChan():
		if decision.Strategy != StrategyResumeTopic {
			t.Fatalf("estrategia = %s, quería %s", decision.Strategy, StrategyResumeTopic)
		}
	default:
		t.Fatal("sin restricción la reanudación debía seguir emitiéndose")
	}
}
