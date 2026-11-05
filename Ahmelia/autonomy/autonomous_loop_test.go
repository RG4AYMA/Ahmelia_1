package autonomy

import (
	"context"
	"testing"
	"time"
)

type mockStateProvider struct {
	lastActivity time.Time
}

func (m *mockStateProvider) LastUserActivity() time.Time {
	return m.lastActivity
}

type mockGoalProvider struct {
	hasPending bool
	goalID     string
}

func (m *mockGoalProvider) PendingGoal() (bool, string) {
	return m.hasPending, m.goalID
}

func TestAutonomousLoop_StartAndShutdown(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	initEngine := NewInitiativeEngine(InitiativeConfig{})
	convMgr := NewConversationManager()

	loop := NewAutonomousLoop(
		LoopConfig{Interval: 20 * time.Millisecond},
		observer,
		initEngine,
		convMgr,
		&mockStateProvider{lastActivity: time.Now()},
		&mockGoalProvider{},
	)

	ctx, cancel := context.WithCancel(context.Background())

	loop.Start(ctx)
	if !loop.IsRunning() {
		t.Error("se esperaba que el loop estuviera activo")
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	if loop.IsRunning() {
		t.Error("se esperaba que el loop estuviera detenido tras cancelar el contexto")
	}
}

func TestAutonomousLoop_CycleProcessingAndDeduplication(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	initEngine := NewInitiativeEngine(InitiativeConfig{})
	convMgr := NewConversationManager()

	// Registrar observación en B74 usando su firma posicional correcta
	observer.Observe(
		TypeGoalStateChanged,
		"GoalMonitor",
		map[string]interface{}{"goal_id": "goal_99"},
	)

	loop := NewAutonomousLoop(
		LoopConfig{Interval: 10 * time.Millisecond},
		observer,
		initEngine,
		convMgr,
		&mockStateProvider{lastActivity: time.Now()},
		&mockGoalProvider{},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)

	select {
	case decision := <-loop.OutputChan():
		if decision.Strategy != StrategyAppendContext {
			t.Errorf("se esperaba StrategyAppendContext, se obtuvo %s", decision.Strategy)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("tiempo de espera agotado sin recibir decisión conversacional")
	}

	// Verificar deduplicación: la observación debe estar procesada
	// The output channel publishes the decision before the same tick finishes
	// marking its source observations. Wait for that lifecycle commit instead
	// of racing the sender goroutine.
	deadline := time.Now().Add(200 * time.Millisecond)
	for len(observer.GetUnprocessedObservations()) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	unprocessed := observer.GetUnprocessedObservations()
	if len(unprocessed) != 0 {
		t.Errorf("se esperaban 0 observaciones sin procesar tras el ciclo, se obtuvieron %d", len(unprocessed))
	}
}

func TestAutonomousLoop_PanicRecovery(t *testing.T) {
	observer := NewContextObserver(ObserverConfig{})
	initEngine := NewInitiativeEngine(InitiativeConfig{})
	convMgr := NewConversationManager()

	panicGoalProv := &panicMockGoalProvider{}

	loop := NewAutonomousLoop(
		LoopConfig{Interval: 10 * time.Millisecond},
		observer,
		initEngine,
		convMgr,
		&mockStateProvider{lastActivity: time.Now()},
		panicGoalProv,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	if !loop.IsRunning() {
		t.Error("el loop debió mantenerse activo recuperándose del pánico interno")
	}
}

type panicMockGoalProvider struct{}

func (p *panicMockGoalProvider) PendingGoal() (bool, string) {
	panic("falla simulada para probar el recover")
}
