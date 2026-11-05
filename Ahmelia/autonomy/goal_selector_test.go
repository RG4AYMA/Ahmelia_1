package autonomy

import (
	"testing"
	"time"

	"ahmelia/planning"
)

func newSelector(t *testing.T, s *AutonomousState, m *GoalMonitor) *GoalSelector {
	t.Helper()
	sel, err := NewGoalSelector(s, m, GoalSelectionWeights{})
	if err != nil {
		t.Fatalf("NewGoalSelector: %v", err)
	}
	return sel
}

// TestGoalSelector_SinObjetivosDevuelveNil confirma que sin CurrentGoal ni
// PendingGoals, SelectGoal() no inventa nada — nil es la respuesta válida.
func TestGoalSelector_SinObjetivosDevuelveNil(t *testing.T) {
	s := New()
	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	if got := sel.SelectGoal(); got != nil {
		t.Fatalf("SelectGoal() = %v, esperaba nil", got)
	}
	if got := sel.Candidates(); len(got) != 0 {
		t.Fatalf("Candidates() = %v, esperaba vacío", got)
	}
}

// TestGoalSelector_MayorPrioridadGana confirma la señal más básica: entre
// dos objetivos Active sin Deadline ni diferencia de Confidence, gana el
// de mayor Priority.
func TestGoalSelector_MayorPrioridadGana(t *testing.T) {
	s := New()
	low := newGoalWithID("low", planning.GoalActive, time.Now())
	low.Priority = planning.PriorityLow
	high := newGoalWithID("high", planning.GoalActive, time.Now())
	high.Priority = planning.PriorityCritical

	s.AddPendingGoal(low)
	s.AddPendingGoal(high)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "high" {
		t.Fatalf("SelectGoal() = %v, esperaba %q", got, "high")
	}
}

// TestGoalSelector_DeadlineVencidoSumaUrgencia confirma que un objetivo con
// Deadline ya vencido gana aunque tenga menor Priority que el competidor.
func TestGoalSelector_DeadlineVencidoSumaUrgencia(t *testing.T) {
	s := New()
	urgent := newGoalWithID("urgent", planning.GoalActive, time.Now())
	urgent.Priority = planning.PriorityLow
	urgent.Deadline = time.Now().Add(-1 * time.Hour) // ya venció

	important := newGoalWithID("important", planning.GoalActive, time.Now())
	important.Priority = planning.PriorityHigh // 3, sin deadline

	s.AddPendingGoal(important)
	s.AddPendingGoal(urgent)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "urgent" {
		t.Fatalf("SelectGoal() = %v, esperaba %q (deadline vencido debe pesar más que Priority baja)", got, "urgent")
	}
}

// TestGoalSelector_DeadlineLejanoNoSumaUrgencia confirma que un Deadline
// fuera de la ventana de urgencia no suma puntaje — no se inventa presión
// donde el objetivo todavía no la tiene.
func TestGoalSelector_DeadlineLejanoNoSumaUrgencia(t *testing.T) {
	s := New()
	lejano := newGoalWithID("lejano", planning.GoalActive, time.Now())
	lejano.Priority = planning.PriorityMedium
	lejano.Deadline = time.Now().Add(30 * 24 * time.Hour) // muy lejos

	mayor := newGoalWithID("mayor", planning.GoalActive, time.Now())
	mayor.Priority = planning.PriorityHigh

	s.AddPendingGoal(lejano)
	s.AddPendingGoal(mayor)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "mayor" {
		t.Fatalf("SelectGoal() = %v, esperaba %q (deadline lejano no debe pesar)", got, "mayor")
	}
}

// TestGoalSelector_DependenciaSinCompletarExcluye confirma el hallazgo real
// de la auditoría: un objetivo con Dependencies sin completar no debe ser
// elegible, aunque tenga la Priority más alta de todos.
func TestGoalSelector_DependenciaSinCompletarExcluye(t *testing.T) {
	s := New()
	base := newGoalWithID("base", planning.GoalActive, time.Now())
	base.Priority = planning.PriorityLow

	dependiente := newGoalWithID("dependiente", planning.GoalActive, time.Now())
	dependiente.Priority = planning.PriorityUrgent
	dependiente.Dependencies = []string{"base"}

	s.AddPendingGoal(base)
	s.AddPendingGoal(dependiente)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "base" {
		t.Fatalf("SelectGoal() = %v, esperaba %q (dependiente no debería ser elegible todavía)", got, "base")
	}

	candidates := sel.Candidates()
	for _, c := range candidates {
		if c.Goal.ID == "dependiente" {
			t.Fatalf("Candidates() incluyó %q con una dependencia sin completar", "dependiente")
		}
	}
}

// TestGoalSelector_DependenciaCompletadaHabilita confirma que, una vez que
// la dependencia deja de estar en el universo conocido de B72 (se completó
// y se sacó de CurrentGoal/PendingGoals, ver core/engine.go), el objetivo
// dependiente vuelve a ser elegible — un ID de dependencia ausente se trata
// como satisfecho, mismo comportamiento que planning.Goal.CanStart ya
// define.
func TestGoalSelector_DependenciaCompletadaHabilita(t *testing.T) {
	s := New()
	dependiente := newGoalWithID("dependiente", planning.GoalActive, time.Now())
	dependiente.Priority = planning.PriorityMedium
	dependiente.Dependencies = []string{"base-ya-completada-y-removida"}

	s.AddPendingGoal(dependiente)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "dependiente" {
		t.Fatalf("SelectGoal() = %v, esperaba %q (dependencia ausente = satisfecha)", got, "dependiente")
	}
}

// TestGoalSelector_BloqueadoPausadoCompletadoExcluidos confirma que ningún
// Status distinto de Active compite por atención, sin importar Priority.
func TestGoalSelector_BloqueadoPausadoCompletadoExcluidos(t *testing.T) {
	s := New()
	bloqueado := newGoalWithID("bloqueado", planning.GoalActive, time.Now())
	bloqueado.Priority = planning.PriorityUrgent
	bloqueado.SetBlocked("falta la licencia")

	pausado := newGoalWithID("pausado", planning.GoalPaused, time.Now())
	pausado.Priority = planning.PriorityUrgent

	completado := newGoalWithID("completado", planning.GoalCompleted, time.Now())
	completado.Priority = planning.PriorityUrgent

	activo := newGoalWithID("activo", planning.GoalActive, time.Now())
	activo.Priority = planning.PriorityLow

	s.AddPendingGoal(bloqueado)
	s.AddPendingGoal(pausado)
	s.AddPendingGoal(completado)
	s.AddPendingGoal(activo)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "activo" {
		t.Fatalf("SelectGoal() = %v, esperaba %q (único Active elegible)", got, "activo")
	}
	if len(sel.Candidates()) != 1 {
		t.Fatalf("Candidates() len = %d, esperaba 1", len(sel.Candidates()))
	}
}

// TestGoalSelector_StickinessEvitaThrashingPorDiferenciaMinima confirma que
// el objetivo en curso (CurrentGoal) no se abandona por un candidato con
// score apenas mayor — el bonus de continuidad existe justo para esto.
func TestGoalSelector_StickinessEvitaThrashingPorDiferenciaMinima(t *testing.T) {
	s := New()
	enCurso := newGoalWithID("en-curso", planning.GoalActive, time.Now())
	enCurso.Priority = planning.PriorityMedium
	enCurso.Confidence = 0.5

	candidato := newGoalWithID("candidato", planning.GoalActive, time.Now())
	candidato.Priority = planning.PriorityMedium
	candidato.Confidence = 0.6 // apenas mayor, sin stickiness ganaría por 0.1

	s.SetCurrentGoal(enCurso)
	s.AddPendingGoal(candidato)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "en-curso" {
		t.Fatalf("SelectGoal() = %v, esperaba %q (stickiness debe retenerlo)", got, "en-curso")
	}
}

// TestGoalSelector_PrioridadClaramenteMayorDesplazaAlEnCurso confirma que
// la continuidad no es absoluta: una diferencia de Priority real (no
// marginal) sí puede desplazar al CurrentGoal — a diferencia del
// comportamiento anterior (B82), que nunca lo hacía.
func TestGoalSelector_PrioridadClaramenteMayorDesplazaAlEnCurso(t *testing.T) {
	s := New()
	enCurso := newGoalWithID("en-curso", planning.GoalActive, time.Now())
	enCurso.Priority = planning.PriorityLow

	urgente := newGoalWithID("urgente", planning.GoalActive, time.Now())
	urgente.Priority = planning.PriorityUrgent

	s.SetCurrentGoal(enCurso)
	s.AddPendingGoal(urgente)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "urgente" {
		t.Fatalf("SelectGoal() = %v, esperaba %q (diferencia real de Priority debe desplazar al en curso)", got, "urgente")
	}
}

// TestGoalSelector_StaleSumaPuntajeChico confirma que, en igualdad del
// resto de las señales, el objetivo Stale (73) se prefiere sobre el que
// se tocó hace instantes.
func TestGoalSelector_StaleSumaPuntajeChico(t *testing.T) {
	s := New()
	reciente := newGoalWithID("reciente", planning.GoalActive, time.Now())
	reciente.Priority = planning.PriorityMedium

	inactivo := newGoalWithID("inactivo", planning.GoalActive, time.Now().Add(-2*time.Hour))
	inactivo.Priority = planning.PriorityMedium

	s.AddPendingGoal(reciente)
	s.AddPendingGoal(inactivo)

	m := NewGoalMonitor(s, 20*time.Minute)
	sel := newSelector(t, s, m)

	got := sel.SelectGoal()
	if got == nil || got.ID != "inactivo" {
		t.Fatalf("SelectGoal() = %v, esperaba %q (Stale debe desempatar)", got, "inactivo")
	}
}

// TestGoalSelector_CandidatesOrdenDescendenteConRazones confirma que
// Candidates() devuelve TODOS los elegibles en orden descendente de Score,
// y que Reasons refleja señales reales (no vacío) para cada uno.
func TestGoalSelector_CandidatesOrdenDescendenteConRazones(t *testing.T) {
	s := New()
	a := newGoalWithID("a", planning.GoalActive, time.Now())
	a.Priority = planning.PriorityLow
	b := newGoalWithID("b", planning.GoalActive, time.Now())
	b.Priority = planning.PriorityHigh
	c := newGoalWithID("c", planning.GoalActive, time.Now())
	c.Priority = planning.PriorityMedium

	s.AddPendingGoal(a)
	s.AddPendingGoal(b)
	s.AddPendingGoal(c)

	m := NewGoalMonitor(s, time.Hour)
	sel := newSelector(t, s, m)

	candidates := sel.Candidates()
	if len(candidates) != 3 {
		t.Fatalf("Candidates() len = %d, esperaba 3", len(candidates))
	}
	for i := 1; i < len(candidates); i++ {
		if candidates[i-1].Score < candidates[i].Score {
			t.Fatalf("Candidates() no está en orden descendente: %+v", candidates)
		}
	}
	for _, c := range candidates {
		if len(c.Reasons) == 0 {
			t.Fatalf("candidato %q sin Reasons", c.Goal.ID)
		}
	}
	if candidates[0].Goal.ID != "b" {
		t.Fatalf("Candidates()[0] = %q, esperaba %q (mayor Priority)", candidates[0].Goal.ID, "b")
	}
}

// TestNewGoalSelector_PesosPorDefectoSiNoSePasan confirma que un
// GoalSelectionWeights vacío cae a DefaultGoalSelectionWeights en vez de
// dejar ambos pesos en cero (lo que anularía Deadline/Stickiness en
// silencio).
func TestNewGoalSelector_PesosPorDefectoSiNoSePasan(t *testing.T) {
	s := New()
	m := NewGoalMonitor(s, time.Hour)

	sel, err := NewGoalSelector(s, m, GoalSelectionWeights{})
	if err != nil {
		t.Fatalf("NewGoalSelector: %v", err)
	}
	if sel.weights != DefaultGoalSelectionWeights {
		t.Fatalf("weights = %+v, esperaba DefaultGoalSelectionWeights (%+v)", sel.weights, DefaultGoalSelectionWeights)
	}
}

// TestNewGoalSelector_RequiereStateYMonitor confirma que el constructor no
// arma un selector sobre dependencias nil en silencio — mismo criterio de
// fail-safe que NewGoalPlanningAdapter y NewGoalMonitor ya aplican.
func TestNewGoalSelector_RequiereStateYMonitor(t *testing.T) {
	s := New()
	m := NewGoalMonitor(s, time.Hour)

	if _, err := NewGoalSelector(nil, m, GoalSelectionWeights{}); err == nil {
		t.Fatal("esperaba error con state nil")
	}
	if _, err := NewGoalSelector(s, nil, GoalSelectionWeights{}); err == nil {
		t.Fatal("esperaba error con monitor nil")
	}
}
