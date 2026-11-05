package autonomy

import (
	"testing"
	"time"

	"ahmelia/planning"
)

func newGoalWithID(id string, status planning.GoalStatus, lastUpdated time.Time) *planning.Goal {
	g := planning.NewGoal("Goal "+id, "", planning.PriorityMedium, "user")
	g.ID = id
	g.Status = status
	g.LastUpdated = lastUpdated
	return g
}

// TestGoalMonitor_SnapshotVacioSinObjetivos confirma que sin
// CurrentGoal ni PendingGoals, Snapshot() no inventa nada.
func TestGoalMonitor_SnapshotVacioSinObjetivos(t *testing.T) {
	s := New()
	m := NewGoalMonitor(s, time.Minute)

	if got := m.Snapshot(); len(got) != 0 {
		t.Fatalf("Snapshot() = %v, esperaba vacío", got)
	}
}

// TestGoalMonitor_ActivoRecienteNoEsStale confirma que un objetivo
// Active recién actualizado no se marca Stale, aunque el umbral sea
// bajo — LastUpdated == ahora, Idle ~ 0.
func TestGoalMonitor_ActivoRecienteNoEsStale(t *testing.T) {
	s := New()
	g := newGoalWithID("g1", planning.GoalActive, time.Now())
	s.SetCurrentGoal(g)

	m := NewGoalMonitor(s, time.Hour)
	snaps := m.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("Snapshot() len = %d, esperaba 1", len(snaps))
	}
	if snaps[0].Stale {
		t.Fatal("un objetivo Active recién actualizado no debería ser Stale")
	}
}

// TestGoalMonitor_ActivoInactivoSuperaUmbralEsStale es el escenario
// central del roadmap: "quiero terminar el prototipo", 30 minutos
// después hablando de otra cosa -> Stale.
func TestGoalMonitor_ActivoInactivoSuperaUmbralEsStale(t *testing.T) {
	s := New()
	g := newGoalWithID("g1", planning.GoalActive, time.Now().Add(-31*time.Minute))
	s.SetCurrentGoal(g)

	m := NewGoalMonitor(s, 20*time.Minute)
	snaps := m.Snapshot()
	if len(snaps) != 1 || !snaps[0].Stale {
		t.Fatalf("esperaba un snapshot Stale=true, obtuve %+v", snaps)
	}
	if snaps[0].Idle < 30*time.Minute {
		t.Fatalf("Idle = %v, esperaba al menos 30 minutos", snaps[0].Idle)
	}
}

// TestGoalMonitor_CompletadoNuncaEsStale confirma que un objetivo
// Completed, por más inactivo que esté, no se marca Stale — no tiene
// sentido "recordar" algo que ya terminó.
func TestGoalMonitor_CompletadoNuncaEsStale(t *testing.T) {
	s := New()
	g := newGoalWithID("g1", planning.GoalCompleted, time.Now().Add(-2*time.Hour))
	s.AddPendingGoal(g)

	m := NewGoalMonitor(s, 20*time.Minute)
	if stale := m.StaleGoals(); len(stale) != 0 {
		t.Fatalf("StaleGoals() = %v, esperaba vacío para un objetivo Completed", stale)
	}
}

// TestGoalMonitor_PausadoNuncaEsStale confirma lo mismo para Paused:
// una pausa deliberada no es lo mismo que un objetivo olvidado.
func TestGoalMonitor_PausadoNuncaEsStale(t *testing.T) {
	s := New()
	g := newGoalWithID("g1", planning.GoalPaused, time.Now().Add(-2*time.Hour))
	s.AddPendingGoal(g)

	m := NewGoalMonitor(s, 20*time.Minute)
	if stale := m.StaleGoals(); len(stale) != 0 {
		t.Fatalf("StaleGoals() = %v, esperaba vacío para un objetivo Paused", stale)
	}
}

// TestGoalMonitor_BloqueadoSeReportaConRazon confirma que Blocked
// refleja el último Blocker registrado, y que un objetivo bloqueado NO
// se cuenta como Stale (son señales distintas: "nadie lo tocó" vs "no
// se puede avanzar").
func TestGoalMonitor_BloqueadoSeReportaConRazon(t *testing.T) {
	s := New()
	g := newGoalWithID("g1", planning.GoalActive, time.Now().Add(-1*time.Hour))
	g.SetBlocked("falta la licencia de Unity")
	s.AddPendingGoal(g)

	m := NewGoalMonitor(s, 20*time.Minute)
	blocked := m.BlockedGoals()
	if len(blocked) != 1 {
		t.Fatalf("BlockedGoals() len = %d, esperaba 1", len(blocked))
	}
	if blocked[0].BlockingReason != "falta la licencia de Unity" {
		t.Fatalf("BlockingReason = %q, esperaba %q", blocked[0].BlockingReason, "falta la licencia de Unity")
	}
	if stale := m.StaleGoals(); len(stale) != 0 {
		t.Fatalf("StaleGoals() = %v, un objetivo Blocked no debería contar como Stale", stale)
	}
}

// TestGoalMonitor_DedupeEntreCurrentYPending confirma que si el mismo
// Goal (mismo ID) aparece como CurrentGoal y también en la cola de
// pendientes, Snapshot() lo reporta una sola vez.
func TestGoalMonitor_DedupeEntreCurrentYPending(t *testing.T) {
	s := New()
	g := newGoalWithID("g1", planning.GoalActive, time.Now())
	s.SetCurrentGoal(g)
	s.AddPendingGoal(g) // mismo puntero/ID, escenario defensivo

	m := NewGoalMonitor(s, time.Hour)
	if got := m.Snapshot(); len(got) != 1 {
		t.Fatalf("Snapshot() len = %d, esperaba 1 (deduplicado)", len(got))
	}
}

// TestGoalMonitor_VariosPendientesOrdenYFiltro confirma que
// StaleGoals() filtra correctamente entre varios objetivos con
// distintos estados, sin perder ninguno de los que corresponde.
func TestGoalMonitor_VariosPendientesOrdenYFiltro(t *testing.T) {
	s := New()
	fresh := newGoalWithID("fresh", planning.GoalActive, time.Now())
	stale := newGoalWithID("stale", planning.GoalActive, time.Now().Add(-40*time.Minute))
	blocked := newGoalWithID("blocked", planning.GoalActive, time.Now().Add(-40*time.Minute))
	blocked.SetBlocked("esperando respuesta del usuario")
	done := newGoalWithID("done", planning.GoalCompleted, time.Now().Add(-3*time.Hour))

	s.AddPendingGoal(fresh)
	s.AddPendingGoal(stale)
	s.AddPendingGoal(blocked)
	s.AddPendingGoal(done)

	m := NewGoalMonitor(s, 20*time.Minute)

	staleGot := m.StaleGoals()
	if len(staleGot) != 1 || staleGot[0].Goal.ID != "stale" {
		t.Fatalf("StaleGoals() = %v, esperaba solo %q", staleGot, "stale")
	}

	blockedGot := m.BlockedGoals()
	if len(blockedGot) != 1 || blockedGot[0].Goal.ID != "blocked" {
		t.Fatalf("BlockedGoals() = %v, esperaba solo %q", blockedGot, "blocked")
	}
}

// TestNewGoalMonitor_UmbralPorDefectoSiNoSePasa confirma que un
// staleAfter <= 0 cae a DefaultStaleAfter en vez de volverse cero
// (todo Stale al instante sería un bug silencioso).
func TestNewGoalMonitor_UmbralPorDefectoSiNoSePasa(t *testing.T) {
	s := New()
	g := newGoalWithID("g1", planning.GoalActive, time.Now())
	s.SetCurrentGoal(g)

	m := NewGoalMonitor(s, 0)
	if m.staleAfter != DefaultStaleAfter {
		t.Fatalf("staleAfter = %v, esperaba DefaultStaleAfter (%v)", m.staleAfter, DefaultStaleAfter)
	}
	// Con el default (20 min), un objetivo recién actualizado no debe
	// ser Stale.
	if snaps := m.Snapshot(); snaps[0].Stale {
		t.Fatal("con el umbral por defecto, un objetivo recién actualizado no debería ser Stale")
	}
}
