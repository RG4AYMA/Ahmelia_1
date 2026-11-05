package autonomy

import (
	"fmt"
	"sort"
	"time"

	"ahmelia/planning"
)

// ============================================================================
// GOAL_SELECTOR.GO — Bloque 86, Fase 6 (Integración profunda de autonomía)
// ============================================================================
//
// Alcance honesto (mismo criterio que trazó B72 para sí mismo): este bloque
// decide CUÁL objetivo, entre los que B72 ya conoce (CurrentGoal +
// PendingGoals), merece la próxima acción de Ahmelia. No crea objetivos
// (Bloque 46, Executive Project Manager, en core/engine.go sigue siendo el
// único lugar que llama planning.NewGoal), no genera planes (B82,
// goal_planner.go), no ejecuta ni verifica nada. Reemplaza la selección
// ciega que hoy vivía en GoalPlanningAdapter.selectGoal() (B82): "el primer
// Active que aparezca en el Snapshot, en el orden en que entró a la cola" —
// eso no mira Priority, Deadline ni Dependencies, tres campos que
// planning.Goal expone desde el principio y que, hasta este bloque, ningún
// código a nivel de objetivo consultaba para decidir un orden real.
//
// Auditoría previa a este archivo (evita segunda fuente de verdad, mismo
// criterio que ya viene de B72):
//   - planning.Goal.Priority / Deadline / Confidence ya existen — se leen
//     tal cual, sin normalizarlos a una escala paralela inventada acá.
//   - planning.Goal.CanStart(map[string]*Goal) ya existe. Hallazgo real de
//     esta auditoría: hasta este bloque NADIE lo llamaba a nivel de
//     objetivo — goal_planner.go (B82) solo valida dependencias a nivel de
//     Task (vía nextRunnableTask/task.CanStart), nunca a nivel de Goal. Un
//     objetivo con Dependencies sin completar podía terminar elegido igual.
//     Se cierra ese hueco acá, no se lo deja para "más adelante" otra vez.
//   - GoalMonitor (73) ya calcula Idle/Stale/Blocked por objetivo vía
//     Snapshot() — este bloque lo consulta en vez de recalcular
//     time.Since(LastUpdated) por su cuenta.
//   - AutonomousState.CurrentGoal()/PendingGoals() (72) siguen siendo la
//     única fuente de objetivos conocidos. Este bloque no mantiene su
//     propia lista ni la cachea entre llamadas — cada Candidates() relee
//     el Snapshot del momento, mismo criterio que ya usa GoalMonitor.
//
// Deliberadamente AUSENTE de este archivo (pertenece a bloques futuros del
// roadmap, no se anticipa con código especulativo):
//   - generar un plan nuevo o modificar Dependencies (B87, Replanificación
//     autónoma);
//   - decidir QUÉ hacer con un objetivo Blocked más allá de no elegirlo
//     (eso lo comunican 75/76, vía GoalMonitor.BlockedGoals());
//   - usar señales del entorno (B85, EnvironmentSnapshot) para puntuar un
//     objetivo — no existe hoy ningún campo que relacione un Goal con una
//     app/archivo/proceso del entorno, así que inventar esa relación acá
//     sería fabricar una señal que el modelo de datos no respalda.

// GoalSelectionWeights son los pesos operativos de Candidates()/SelectGoal
// — expuestos, no hardcodeados en score(), mismo criterio que
// DefaultStaleAfter en goal_monitor.go: quien instancie el selector puede
// ajustarlos sin tocar este archivo.
type GoalSelectionWeights struct {
	// StickinessBonus favorece seguir con el objetivo que YA es
	// CurrentGoal frente a un candidato de score apenas mayor — evita que
	// Ahmelia salte de objetivo en objetivo por diferencias mínimas
	// (goal-thrashing). Se aplica solo al Goal que ya es CurrentGoal, no a
	// cualquier candidato con el mismo ID coincidente por casualidad.
	StickinessBonus float64

	// DeadlineUrgencyWindow es la ventana antes del Deadline dentro de la
	// cual un objetivo empieza a sumar puntaje por urgencia. Fuera de esa
	// ventana (o sin Deadline puesto — time.Time{} es "sin fecha", mismo
	// criterio que BlockedAt/CompletedAt en planning.Goal), la urgencia por
	// fecha es cero: no se inventa presión donde el objetivo no la tiene.
	DeadlineUrgencyWindow time.Duration
}

// DefaultGoalSelectionWeights: stickiness moderada (medio punto de
// prioridad, para que un objetivo de prioridad claramente mayor SÍ pueda
// desplazar al que está en curso) y una ventana de urgencia de 24h — el
// mismo orden de magnitud que DefaultStaleAfter (73) multiplicado por un
// factor de antelación razonable, no un número sin relación con el resto
// del bloque.
var DefaultGoalSelectionWeights = GoalSelectionWeights{
	StickinessBonus:       0.5,
	DeadlineUrgencyWindow: 24 * time.Hour,
}

// GoalCandidate es un objetivo evaluado junto con el motivo real de su
// puntaje — expuesto para que quien llame (un log de diagnóstico, o B87 al
// mirar el segundo candidato) pueda explicar la elección en vez de
// consumirla a ciegas. Reasons son las señales que efectivamente aplicaron,
// en el orden en que se sumaron — no una explicación post-hoc inventada
// después de calcular el número.
type GoalCandidate struct {
	Goal    *planning.Goal
	Score   float64
	Reasons []string
}

// GoalSelector decide qué objetivo Active, entre los que B72 conoce ahora,
// debería recibir la próxima acción autónoma.
type GoalSelector struct {
	state   *AutonomousState
	monitor *GoalMonitor
	weights GoalSelectionWeights
}

// NewGoalSelector crea el selector sobre un AutonomousState/GoalMonitor ya
// existentes — no construye ninguno de los dos por su cuenta, mismo
// criterio que NewGoalPlanningAdapter (B82). weights == (zero value) usa
// DefaultGoalSelectionWeights, mismo criterio de "no dejar un umbral en
// cero por no haberlo pasado" que NewGoalMonitor aplica a staleAfter.
func NewGoalSelector(state *AutonomousState, monitor *GoalMonitor, weights GoalSelectionWeights) (*GoalSelector, error) {
	if state == nil {
		return nil, fmt.Errorf("autonomy: estado B72 no configurado")
	}
	if monitor == nil {
		return nil, fmt.Errorf("autonomy: GoalMonitor no configurado")
	}
	if weights == (GoalSelectionWeights{}) {
		weights = DefaultGoalSelectionWeights
	}
	return &GoalSelector{state: state, monitor: monitor, weights: weights}, nil
}

// SelectGoal elige, entre todos los objetivos que B72 conoce ahora mismo,
// el que debería recibir la próxima acción. Devuelve nil si ninguno es
// elegible (todos Blocked/Completed/Paused, o con Dependencies sin
// completar) — nil es una respuesta válida ("no hay nada que hacer ahora"),
// no un error.
func (s *GoalSelector) SelectGoal() *planning.Goal {
	candidates := s.Candidates()
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0].Goal
}

// Candidates devuelve todos los objetivos elegibles ahora, ordenados de
// mayor a menor Score. Expuesto además de SelectGoal para que quien
// necesite explicar o auditar la elección (o B87, al replanificar, mirar
// el segundo/tercer candidato) no dependa de recalcular el mismo cómputo.
func (s *GoalSelector) Candidates() []GoalCandidate {
	snapshots := s.monitor.Snapshot()
	if len(snapshots) == 0 {
		return nil
	}

	// byID es el universo para CanStart: únicamente los objetivos que B72
	// conoce ahora mismo (CurrentGoal + PendingGoals). Un objetivo
	// Completed ya se sacó de ahí en su momento (ver core/engine.go, rama
	// de completar la última tarea del plan), así que una Dependency que
	// apunte a un ID ausente de este mapa se trata como satisfecha — mismo
	// comportamiento que planning.Goal.CanStart ya define por su cuenta
	// (`if dep, ok := allGoals[depID]; ok { ... }`), y el mismo criterio
	// que goal_planner.go ya aplica a nivel de Task en nextRunnableTask.
	byID := make(map[string]*planning.Goal, len(snapshots))
	for _, snap := range snapshots {
		if snap.Goal != nil {
			byID[snap.Goal.ID] = snap.Goal
		}
	}

	current := s.state.CurrentGoal()
	now := time.Now()

	var out []GoalCandidate
	for _, snap := range snapshots {
		g := snap.Goal
		if g == nil {
			continue
		}
		// Completed/Paused/Blocked no compiten por atención: Completed no
		// tiene sentido retomarlo, Paused es una decisión ya tomada de no
		// tocarlo, y Blocked se reporta vía GoalMonitor.BlockedGoals() para
		// que 75/76 lo comuniquen — no para que este bloque reintente el
		// mismo objetivo bloqueado en silencio.
		if g.Status != planning.GoalActive {
			continue
		}
		if !g.CanStart(byID) {
			continue
		}

		score, reasons := s.score(g, snap, now, current)
		out = append(out, GoalCandidate{Goal: g, Score: score, Reasons: reasons})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}

func (s *GoalSelector) score(
	g *planning.Goal,
	snap GoalSnapshot,
	now time.Time,
	current *planning.Goal,
) (float64, []string) {
	var score float64
	var reasons []string

	// Señal 1: Priority — la única escala de "importancia" que
	// planning.Goal ya modela (1..5, PriorityLow..PriorityUrgent). Se usa
	// directo, sin normalizar a otro rango: inventar una escala paralela
	// ocultaría cuál es la señal real que decide el orden.
	score += float64(g.Priority)
	reasons = append(reasons, fmt.Sprintf("priority=%d", g.Priority))

	// Señal 2: urgencia por Deadline. Sin Deadline (time.Time{}) no suma
	// nada — un objetivo sin fecha puesta no es "urgente por defecto".
	if !g.Deadline.IsZero() {
		remaining := g.Deadline.Sub(now)
		switch {
		case remaining <= 0:
			score += 3
			reasons = append(reasons, "deadline_vencido")
		case remaining <= s.weights.DeadlineUrgencyWindow:
			// Lineal dentro de la ventana: cuanto más cerca del
			// vencimiento, más puntaje — sin curva ni umbral adicional
			// inventado.
			frac := 1 - (float64(remaining) / float64(s.weights.DeadlineUrgencyWindow))
			score += 2 * frac
			reasons = append(reasons, "deadline_proximo")
		}
	}

	// Señal 3: Confidence — entre dos objetivos igual de urgentes, el que
	// Ahmelia tiene más motivos para creer que va a poder completar
	// (Goal.Confidence, ya alimentado por quien lo creó) se prioriza sobre
	// uno más incierto.
	score += g.Confidence
	reasons = append(reasons, fmt.Sprintf("confidence=%.2f", g.Confidence))

	// Señal 4: continuidad — si este objetivo YA es el CurrentGoal, un
	// bonus fijo evita abandonarlo a medio hacer por una diferencia de
	// score mínima frente a otro candidato recién aparecido.
	if current != nil && current.ID == g.ID {
		score += s.weights.StickinessBonus
		reasons = append(reasons, "objetivo_en_curso")
	}

	// Señal 5: Stale (73, GoalMonitor) — un objetivo Active que lleva
	// tiempo sin tocarse suma un puntaje chico: no es urgencia nueva, es
	// "esto se está por olvidar", que amerita volver antes que otro
	// objetivo recién tocado con el mismo Priority/Deadline/Confidence.
	if snap.Stale {
		score += 0.5
		reasons = append(reasons, "inactivo_hace_tiempo")
	}

	return score, reasons
}
