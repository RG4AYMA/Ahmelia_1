package resource

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLinguisticsGate_EnterExit(t *testing.T) {
	gov := NewGovernor(GovernorConfig{
		PreemptGrace: 50 * time.Millisecond,
	})
	gate := NewLinguisticsGate(gov)
	defer gate.Close()

	ctx := context.Background()
	gateCtx, exit, err := gate.Enter(ctx, "interpretation", 1) // learning priority
	if err != nil {
		t.Fatalf("Enter error: %v", err)
	}
	if gateCtx == nil {
		t.Fatal("gateCtx is nil")
	}

	// Verificar que el governor tiene la concesión
	stats := gov.Stats()
	if stats.Active == nil {
		t.Fatal("governor should have active lease")
	}
	if stats.Active.Kind != KindLearning {
		t.Fatalf("active kind = %s, want learning", stats.Active.Kind)
	}

	// Exit debe ser idempotente
	exit()
	exit() // no debe fallar

	// Después de exit, el governor debe estar libre
	stats = gov.Stats()
	if stats.Active != nil {
		t.Fatal("governor should be free after exit")
	}
}

func TestLinguisticsGate_NilGovernor(t *testing.T) {
	gate := NewLinguisticsGate(nil)

	ctx, exit, err := gate.Enter(context.Background(), "test", 2)
	if err != nil {
		t.Fatalf("Enter error with nil governor: %v", err)
	}
	if ctx == nil {
		t.Fatal("ctx is nil")
	}
	exit() // no debe fallar
}

func TestLinguisticsGate_ConcurrentClasses(t *testing.T) {
	gov := NewGovernor(GovernorConfig{
		PreemptGrace: 50 * time.Millisecond,
	})
	gate := NewLinguisticsGate(gov)
	defer gate.Close()

	var wg sync.WaitGroup
	classes := []struct {
		class    string
		priority int
	}{
		{"interpretation", 1},
		{"generation", 2},
		{"proposal", 2},
	}

	for _, c := range classes {
		wg.Add(1)
		go func(cl struct {
			class    string
			priority int
		}) {
			defer wg.Done()
			_, exit, err := gate.Enter(context.Background(), cl.class, cl.priority)
			if err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
			exit()
		}(c)
	}

	wg.Wait()

	stats := gov.Stats()
	if stats.Active != nil {
		t.Fatal("all leases should be released")
	}
	if stats.QueueLength != 0 {
		t.Fatalf("queue length = %d, want 0", stats.QueueLength)
	}
}

func TestLinguisticsGate_ExitIdempotent(t *testing.T) {
	gov := NewGovernor(GovernorConfig{})
	gate := NewLinguisticsGate(gov)
	defer gate.Close()

	_, exit, err := gate.Enter(context.Background(), "test", 2)
	if err != nil {
		t.Fatalf("Enter: %v", err)
	}

	exit()
	exit()
	exit() // no debe fallar

	// Verificar que el governor está libre
	stats := gov.Stats()
	if stats.Active != nil {
		t.Fatal("governor should be free after multiple exits")
	}
}

func TestLinguisticsGate_CloseCleansUp(t *testing.T) {
	gov := NewGovernor(GovernorConfig{})
	gate := NewLinguisticsGate(gov)

	_, _, err := gate.Enter(context.Background(), "test", 2)
	if err != nil {
		t.Fatalf("Enter: %v", err)
	}

	gate.Close()

	stats := gov.Stats()
	if stats.Active != nil {
		t.Fatal("active lease after Close")
	}
	if stats.QueueLength != 0 {
		t.Fatalf("queue length after Close = %d", stats.QueueLength)
	}
}

func TestLinguisticsGate_PriorityMapping(t *testing.T) {
	tests := []struct {
		priority int
		expected Priority
	}{
		{0, PriorityOwn},
		{1, PriorityLearning},
		{2, PriorityUserGoal},
		{3, PriorityUrgent},
		{4, PriorityUserTurn},
		{5, PriorityUserGoal},  // default
		{-1, PriorityUserGoal}, // default
	}

	for _, tc := range tests {
		gov := NewGovernor(GovernorConfig{})
		gate := NewLinguisticsGate(gov)

		_, exit, err := gate.Enter(context.Background(), "test", tc.priority)
		if err != nil {
			t.Fatalf("Enter: %v", err)
		}

		stats := gov.Stats()
		if stats.Active == nil {
			t.Fatal("no active lease")
		}
		if stats.Active.Priority != tc.expected {
			t.Fatalf("priority %d -> expected %d, got %d", tc.priority, tc.expected, stats.Active.Priority)
		}
		exit()
	}
}

func TestLinguisticsGate_ContextCancellation(t *testing.T) {
	gov := NewGovernor(GovernorConfig{})
	gate := NewLinguisticsGate(gov)
	defer gate.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar inmediatamente

	_, _, err := gate.Enter(ctx, "test", 2)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestLinguisticsGate_WithRealGovernor(t *testing.T) {
	// Test de integración con governor real y preempt
	gov := NewGovernor(GovernorConfig{
		PreemptGrace: 50 * time.Millisecond,
	})
	gate := NewLinguisticsGate(gov)
	defer gate.Close()

	// Ocupar con prioridad media (user_goal = 2)
	_, exit1, err := gate.Enter(context.Background(), "generation", 2)
	if err != nil {
		t.Fatalf("Enter generation: %v", err)
	}

	stats := gov.Stats()
	if stats.Active == nil || stats.Active.Kind != KindUserGoal {
		t.Fatal("generation not active")
	}

	// Intentar adquirir con la máxima prioridad (user turn = 4, valor 0)
	ctx2, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var exit2 func()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, exit2, _ = gate.Enter(ctx2, "interpretation", 4)
	}()

	// Dar tiempo a que se encole
	time.Sleep(20 * time.Millisecond)

	// PreemptIfNeeded debe ejecutarse
	gov.PreemptIfNeeded()

	// Esperar un poco
	time.Sleep(100 * time.Millisecond)

	// La petición de máxima prioridad (user turn) debe haber preemptado a
	// generation (user goal): el turno del usuario nunca espera.
	wg.Wait()
	stats = gov.Stats()
	if stats.Active == nil || stats.Active.Priority != PriorityUserTurn {
		t.Fatalf("la petición de máxima prioridad debió preemptar: %+v", stats.Active)
	}

	// Limpiar
	if exit2 != nil {
		exit2()
	}
	exit1()
	wg.Wait()
}
