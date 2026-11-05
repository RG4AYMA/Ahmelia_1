package autonomy

import (
	"time"

	"ahmelia/planning"
)

// ============================================================================
// GOAL_MONITOR.GO — Bloque 73, Fase 6 (Autonomía Cognitiva e Iniciativa)
// ============================================================================
//
// Alcance honesto de este bloque (mismo límite que trazó B72 para sí
// mismo): Goal Monitor VIGILA — calcula qué objetivos llevan tiempo sin
// tocarse y cuáles están bloqueados. NO decide si vale la pena
// interrumpir con eso, y NO redacta ningún mensaje del tipo "tenemos
// pendiente el prototipo, ¿retomamos?". El propio roadmap lo separa
// así: "Y POSTERIORMENTE podrá decidir: ...". Ese "posteriormente" es
// 75 (Initiative Engine) — es quien cruza esta información con
// relevancia/momento/riesgo y decide NoAction/Wait/AskUser/Suggest/etc.
// Meter esa decisión acá sería repetir el error que B72 evitó
// explícitamente: un bloque que hace más de lo que puede auditar solo.
//
// Auditoría previa (evita segunda fuente de verdad, mismo criterio que
// B72): planning.Goal YA tiene LastUpdated (se actualiza en AddNote,
// UpdateProgress, SetBlocked, Unblock, Complete) y Blockers. Goal
// Monitor no mantiene su propio timestamp de actividad por objetivo —
// lee LastUpdated directamente. Lo único que este bloque agrega es el
// cálculo de "inactivo" (Idle, comparado contra un umbral) y el filtro
// por Status, que no existían en planning.Goal porque no es su
// responsabilidad decidir eso.
//
// Fuente de objetivos a vigilar: AutonomousState.CurrentGoal() +
// AutonomousState.PendingGoals() (Bloque 72) — Goal Monitor no
// mantiene su propia lista, solo la consulta cada vez que se le pide
// un Snapshot().

// DefaultStaleAfter es el umbral por defecto de inactividad a partir
// del cual un objetivo Active se considera "stale" (candidato a
// recordar más adelante, decisión que sigue sin ser de este bloque).
// Expuesto como constante, no hardcodeado en Snapshot(), mismo criterio
// que los umbrales de análisis de aclaración del pipeline B96.33: quien
// instancie el monitor puede ajustarlo sin tocar este archivo.
const DefaultStaleAfter = 20 * time.Minute

// GoalSnapshot es el estado observado de UN objetivo al momento de
// llamar a Snapshot() — una lectura, no algo que se actualice solo.
type GoalSnapshot struct {
	Goal    *planning.Goal
	Idle    time.Duration // time.Since(Goal.LastUpdated)
	Stale   bool          // Idle >= umbral Y Status == GoalActive
	Blocked bool          // Status == GoalBlocked
	// BlockingReason es el último Blocker registrado (el más reciente
	// que se agregó), o "" si Blocked es false. No concatena todos los
	// Blockers — mismo criterio que el resto del proyecto de mostrar la
	// señal más útil, no todo el historial.
	BlockingReason string
}

// GoalMonitor vigila los objetivos que AutonomousState ya conoce. Sin
// estado propio de objetivos — cada Snapshot() relee CurrentGoal() y
// PendingGoals() en el momento, no cachea nada, así que nunca puede
// quedar desincronizado de lo que B72 sostiene.
type GoalMonitor struct {
	state      *AutonomousState
	staleAfter time.Duration
}

// NewGoalMonitor crea el monitor sobre un AutonomousState ya existente.
// staleAfter <= 0 usa DefaultStaleAfter — evita que un caller que
// simplemente no pasó el argumento termine con un umbral de cero
// (todo "stale" instantáneamente, lo cual sería un bug silencioso).
func NewGoalMonitor(state *AutonomousState, staleAfter time.Duration) *GoalMonitor {
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	return &GoalMonitor{state: state, staleAfter: staleAfter}
}

// Snapshot calcula el estado observado de cada objetivo vigilado ahora
// mismo: CurrentGoal (si existe) primero, después PendingGoals en el
// orden en que están en la cola. Deduplicado por ID — si por algún
// motivo el mismo Goal terminó en ambos lugares, aparece una sola vez
// (como CurrentGoal, que es la posición más específica).
// CurrentGoal devuelve el objetivo actualmente activo según AutonomousState.
// GoalMonitor expone esta lectura para los adaptadores de iniciativa sin
// filtrar el estado interno de AutonomousState fuera de este paquete.
func (m *GoalMonitor) CurrentGoal() *planning.Goal {
	if m == nil || m.state == nil {
		return nil
	}
	return m.state.CurrentGoal()
}

func (m *GoalMonitor) Snapshot() []GoalSnapshot {
	seen := make(map[string]bool)
	var out []GoalSnapshot

	if g := m.state.CurrentGoal(); g != nil {
		out = append(out, m.snapshotOf(g))
		seen[g.ID] = true
	}
	for _, g := range m.state.PendingGoals() {
		if g == nil || seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		out = append(out, m.snapshotOf(g))
	}
	return out
}

func (m *GoalMonitor) snapshotOf(g *planning.Goal) GoalSnapshot {
	idle := time.Since(g.LastUpdated)
	snap := GoalSnapshot{
		Goal:    g,
		Idle:    idle,
		Blocked: g.Status == planning.GoalBlocked,
	}
	if snap.Blocked && len(g.Blockers) > 0 {
		snap.BlockingReason = g.Blockers[len(g.Blockers)-1]
	}
	// Solo un objetivo Active y realmente inactivo cuenta como Stale.
	// GoalCompleted no tiene sentido recordarlo; GoalPaused ya está
	// pausado a propósito (el usuario/Ahmelia decidió no tocarlo, no es
	// lo mismo que "se olvidó"); GoalBlocked se reporta vía Blocked, no
	// vía Stale — mezclar ambos ocultaría la diferencia real entre
	// "nadie lo tocó" y "no se puede avanzar".
	snap.Stale = g.Status == planning.GoalActive && idle >= m.staleAfter
	return snap
}

// StaleGoals filtra Snapshot() a los objetivos Stale — el insumo
// directo que 75 (Initiative Engine) va a cruzar con relevancia/riesgo
// antes de decidir si vale la pena decir algo.
func (m *GoalMonitor) StaleGoals() []GoalSnapshot {
	return filterSnapshots(m.Snapshot(), func(s GoalSnapshot) bool { return s.Stale })
}

// BlockedGoals filtra Snapshot() a los objetivos Blocked.
func (m *GoalMonitor) BlockedGoals() []GoalSnapshot {
	return filterSnapshots(m.Snapshot(), func(s GoalSnapshot) bool { return s.Blocked })
}

func filterSnapshots(all []GoalSnapshot, keep func(GoalSnapshot) bool) []GoalSnapshot {
	var out []GoalSnapshot
	for _, s := range all {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}
