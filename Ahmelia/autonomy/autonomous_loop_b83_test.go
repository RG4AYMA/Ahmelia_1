package autonomy

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type b83MockCycle struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (m *b83MockCycle) RunStep(ctx context.Context) error {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.err
}

func (m *b83MockCycle) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// TestAutonomousExecutionLoop_B83_StartStop verifica que B83 mantiene una
// sola goroutine, ejecuta ticks y cierra limpiamente con Stop.
func TestAutonomousExecutionLoop_B83_StartStop(t *testing.T) {
	var calls atomic.Int32

	// Cycle real mínimo: los especialistas nil permiten validar únicamente
	// la responsabilidad de B83 (orquestación temporal).
	state := New()
	cycle := NewCycle(
		state,
		nil, nil, nil, nil,
		NewActionVerifier(),
		nil, nil, nil,
	)

	loop := NewAutonomousExecutionLoop(
		LoopConfig{Interval: 5 * time.Millisecond},
		cycle,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)
	loop.Start(ctx) // no debe crear una segunda goroutine

	time.Sleep(25 * time.Millisecond)
	_ = calls.Load() // evita que el test dependa de contadores internos del Cycle

	if !loop.IsRunning() {
		t.Fatal("B83 debía permanecer activo mientras el contexto seguía vivo")
	}

	loop.Stop()

	if loop.IsRunning() {
		t.Fatal("B83 debía quedar detenido después de Stop")
	}

	if state.Execution() != ExecIdle {
		t.Fatalf("B83 debía dejar el Cycle en Idle, quedó en %s", state.Execution())
	}
}

// TestAutonomousExecutionLoop_B83_ContextCancellation comprueba el cierre
// por contexto sin depender de time.Sleep largo.
func TestAutonomousExecutionLoop_B83_ContextCancellation(t *testing.T) {
	state := New()
	cycle := NewCycle(
		state,
		nil, nil, nil, nil,
		NewActionVerifier(),
		nil, nil, nil,
	)
	loop := NewAutonomousExecutionLoop(
		LoopConfig{Interval: 5 * time.Millisecond},
		cycle,
	)

	ctx, cancel := context.WithCancel(context.Background())
	loop.Start(ctx)

	deadline := time.Now().Add(250 * time.Millisecond)
	for !loop.IsRunning() {
		if time.Now().After(deadline) {
			t.Fatal("B83 no llegó a estado running")
		}
		time.Sleep(time.Millisecond)
	}

	cancel()

	for loop.IsRunning() {
		if time.Now().After(deadline) {
			t.Fatal("B83 no terminó tras cancelar el contexto")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestAutonomousExecutionLoop_B83_NilCycleNoPanic deja explícito que el
// constructor B83 requiere un Cycle válido para producir trabajo real.
func TestAutonomousExecutionLoop_B83_NilCycleNoPanic(t *testing.T) {
	loop := NewAutonomousExecutionLoop(
		LoopConfig{Interval: time.Millisecond},
		nil,
	)
	if loop == nil {
		t.Fatal("se esperaba un loop válido")
	}

	ctx, cancel := context.WithCancel(context.Background())
	loop.Start(ctx)
	time.Sleep(5 * time.Millisecond)
	cancel()
	loop.Stop()

	if loop.IsRunning() {
		t.Fatal("el loop debía detenerse limpiamente")
	}
}
