package autonomy

import (
	"fmt"
	"sync"

	"ahmelia/planning"
)

// ============================================================================
// REPLANNER.GO — Bloque 87, Fase 6 (Integración profunda de autonomía)
// ============================================================================
//
// Alcance honesto (mismo criterio que ya trazaron 73/86 para sí mismos):
// hasta este bloque, GoalPlanningAdapter.UpdateState (82) tenía una sola
// respuesta posible ante CUALQUIER tarea fallida: marcar el Goal entero como
// GoalBlocked de forma permanente (ver TestGoalPlanningAdapter_FailureBlocksGoal
// antes de este bloque). Eso significa que un fallo transitorio de ejecución
// (una carpeta que no se pudo crear porque el disco estaba momentáneamente
// ocupado, un proceso que tardó en responder) mataba el objetivo entero para
// siempre, sin que 86 (Goal Selector) ni 75 (Initiative Engine) pudieran
// hacer nada al respecto — quedaba excluido de Candidates() para siempre.
//
// Replanner decide qué hacer con UNA tarea fallida dentro de UN plan, antes
// de que GoalPlanningAdapter decida si el Goal se bloquea. No ejecuta nada
// (no repite la acción, no llama al ExecutorAdapter): la decisión se aplica
// en goal_planner.go, que es quien ya posee el mapa de planes cacheados y
// las tareas activas. Este archivo solo dice QUÉ conviene hacer y POR QUÉ.
//
// Deliberadamente AUSENTE de este archivo (no se fabrica lo que el resto del
// proyecto no respalda todavía):
//   - generar un plan realmente distinto al fallar. planning.Planner.
//     GeneratePlan decompone por Goal.Name con un switch estático
//     (planner.go, decomposeGoal): no sabe qué tarea específica falló ni
//     por qué. "Regenerar el plan" en este bloque significa descartar el
//     plan cacheado y pedir uno nuevo — con el mismo Planner de hoy eso
//     casi siempre reconstruye el mismo grafo de tareas. Es una limitación
//     real y conocida, no algo que este archivo intente disimular. Lo que
//     Replanner sí controla es CUÁNDO vale la pena pedir ese plan nuevo (o
//     reintentar/saltar primero), no cómo el Planner arma ese plan.
//   - decidir si vale la pena avisarle al usuario que un objetivo quedó
//     bloqueado. Eso lo sigue resolviendo 75/76 vía
//     GoalMonitor.BlockedGoals(), igual que antes de este bloque.
//   - inventar una causa de la falla. La única entrada real de este bloque
//     es VerificationResult.ErrorReason ya producido por 78/80; Replanner
//     no intenta clasificar el error (red, permisos, lógico) porque nada en
//     el proyecto expone hoy esa clasificación.

// ReplanAction es la decisión que Replanner toma frente a una tarea fallida.
type ReplanAction string

const (
	// ReplanRetryTask reintenta la MISMA tarea: el fallo se trata como
	// transitorio mientras queden intentos disponibles.
	ReplanRetryTask ReplanAction = "retry_task"

	// ReplanSkipTask cancela la tarea fallida y deja que el resto del plan
	// continúe — solo válido cuando ninguna tarea pendiente depende de ella.
	ReplanSkipTask ReplanAction = "skip_task"

	// ReplanRegeneratePlan descarta el plan cacheado del objetivo para que
	// GoalPlanningAdapter pida uno nuevo a planning.Planner la próxima vez
	// que se le pida una acción.
	ReplanRegeneratePlan ReplanAction = "regenerate_plan"

	// ReplanGiveUp agota las opciones anteriores: el objetivo se bloquea,
	// mismo desenlace que existía antes de este bloque para toda falla.
	ReplanGiveUp ReplanAction = "give_up"
)

// ReplanDecision es el veredicto de Replanner para una tarea fallida,
// acompañado del motivo real — mismo criterio de auditabilidad que
// GoalCandidate.Reasons en goal_selector.go: quien aplique la decisión (o
// un log de diagnóstico) puede explicarla, no solo consumirla a ciegas.
type ReplanDecision struct {
	Action ReplanAction
	Reason string
}

// ReplanLimits son los topes operativos de Replanner — expuestos, no
// hardcodeados en Decide(), mismo criterio que GoalSelectionWeights (86) y
// DefaultStaleAfter (73): quien instancie el replanner puede ajustarlos sin
// tocar este archivo.
type ReplanLimits struct {
	// MaxTaskRetries es cuántas veces (además del intento original) se
	// reintenta la misma tarea antes de considerar reintentar agotado.
	// Se compara contra Task.Attempts, que planning.Task YA incrementa en
	// cada Start() — Replanner no mantiene su propio contador de intentos
	// por tarea, mismo criterio de "no segunda fuente de verdad" que ya
	// aplican 73 y 86 sobre LastUpdated/Priority/Deadline.
	MaxTaskRetries int

	// MaxPlanRegenerations es cuántas veces, por objetivo, se acepta pedir
	// un plan nuevo antes de darse por vencido. A diferencia de
	// MaxTaskRetries, planning.Goal no expone ningún campo para este
	// conteo (no existe un "Goal.Regenerations"), así que es la única
	// pieza de estado nueva que este bloque agrega — igual que 73 agregó
	// el cálculo de Stale porque tampoco existía en otro lado.
	MaxPlanRegenerations int
}

// DefaultReplanLimits: un reintento adicional además del intento original
// (Attempts <= 2 todavía reintenta) y una sola regeneración de plan por
// objetivo antes de bloquear — mismo orden de magnitud conservador que
// DefaultGoalSelectionWeights.StickinessBonus: preferir insistir un poco
// antes de tirar el objetivo entero a GoalBlocked.
var DefaultReplanLimits = ReplanLimits{
	MaxTaskRetries:       2,
	MaxPlanRegenerations: 1,
}

// Replanner decide, tarea por tarea, si un fallo de ejecución amerita
// reintentar, saltar, regenerar el plan o darse por vencido.
type Replanner struct {
	mu     sync.Mutex
	limits ReplanLimits

	// regenerations cuenta, por Goal.ID, cuántas veces ya se regeneró el
	// plan. Es la única pieza de estado propia de este bloque (ver
	// comentario de MaxPlanRegenerations); se limpia con ResetGoal cuando
	// el objetivo se completa, se bloquea definitivamente o desaparece del
	// universo que AutonomousState conoce, para no crecer sin límite.
	regenerations map[string]int
}

// NewReplanner crea el replanner de B87. limits == (zero value) usa
// DefaultReplanLimits, mismo criterio de "no dejar un tope en cero por no
// haberlo pasado" que NewGoalMonitor/NewGoalSelector ya aplican a sus
// propios parámetros.
func NewReplanner(limits ReplanLimits) *Replanner {
	if limits == (ReplanLimits{}) {
		limits = DefaultReplanLimits
	}
	return &Replanner{
		limits:        limits,
		regenerations: make(map[string]int),
	}
}

// Decide examina una tarea que acaba de fallar (task.Status ya es
// planning.TaskBlocked, puesto por Task.Fail antes de llamar acá) y decide
// el próximo paso. No muta goal, plan ni task — GoalPlanningAdapter aplica
// la decisión, porque es quien posee el mapa de planes cacheados.
func (r *Replanner) Decide(goal *planning.Goal, plan *planning.Plan, task *planning.Task) ReplanDecision {
	if goal == nil || plan == nil || task == nil {
		return ReplanDecision{Action: ReplanGiveUp, Reason: "faltan datos para decidir (goal/plan/task nil)"}
	}

	if task.Attempts < r.limits.MaxTaskRetries {
		return ReplanDecision{
			Action: ReplanRetryTask,
			Reason: fmt.Sprintf("intento %d de %d agotados: se reintenta la misma tarea", task.Attempts, r.limits.MaxTaskRetries),
		}
	}

	if !hasPendingDependent(plan, task.ID) {
		return ReplanDecision{
			Action: ReplanSkipTask,
			Reason: "reintentos agotados y ninguna tarea pendiente depende de esta: el plan puede seguir sin ella",
		}
	}

	r.mu.Lock()
	regenerations := r.regenerations[goal.ID]
	r.mu.Unlock()

	if regenerations < r.limits.MaxPlanRegenerations {
		return ReplanDecision{
			Action: ReplanRegeneratePlan,
			Reason: fmt.Sprintf("reintentos agotados y hay tareas dependientes: se pide un plan nuevo (regeneración %d de %d)", regenerations+1, r.limits.MaxPlanRegenerations),
		}
	}

	return ReplanDecision{
		Action: ReplanGiveUp,
		Reason: "reintentos y regeneraciones de plan agotados para esta tarea",
	}
}

// RecordRegeneration incrementa el contador de regeneraciones del objetivo.
// GoalPlanningAdapter la llama únicamente cuando efectivamente descarta el
// plan cacheado y va a pedir uno nuevo — no cuando Decide() simplemente
// devuelve ReplanRegeneratePlan, para que un caller que consulte Decide()
// sin aplicar la decisión (p. ej. un log de diagnóstico) no infle el
// contador por el solo hecho de preguntar.
func (r *Replanner) RecordRegeneration(goalID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.regenerations[goalID]++
}

// ResetGoal limpia el contador de regeneraciones de un objetivo. Se llama
// cuando el objetivo se completa, se bloquea definitivamente (ReplanGiveUp)
// o deja de existir para AutonomousState — mismo criterio de limpieza que
// GoalPlanningAdapter ya aplica sobre activeTasks al borrar la entrada tras
// UpdateState.
func (r *Replanner) ResetGoal(goalID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.regenerations, goalID)
}

// ============================================================================
// REGENERATIONS() / LOADREGENERATIONS() — Bloque 90.5, Fase 6 (Persistencia
// del subsistema autónomo)
// ============================================================================
//
// Mismo criterio exacto que Planner.Goals()/Tasks() (B90.2) y
// GoalPlanningAdapter.Plans()/ActiveTasks()/LoadState() (B90.4): Replanner
// no gana ninguna lógica de persistencia acá, solo expone y recibe el
// único estado propio que ya sostenía desde B87 (ver comentario de
// `regenerations` arriba). Decidir CUÁNDO guardar, y reconciliar esto
// contra Goal.ID reales al cargar, sigue siendo enteramente
// responsabilidad de core/autonomy_persistence.go (B90.6) — este archivo
// sigue sin saber que existe un JSON en disco.

// Regenerations devuelve una copia defensiva del contador de
// regeneraciones por Goal.ID. Solo lectura: no incrementa, no limpia nada
// (eso sigue siendo RecordRegeneration/ResetGoal).
func (r *Replanner) Regenerations() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make(map[string]int, len(r.regenerations))
	for goalID, count := range r.regenerations {
		out[goalID] = count
	}
	return out
}

// LoadRegenerations reemplaza integralmente el contador de regeneraciones
// por el que le pasa la reconciliación de B90 — no es un merge, mismo
// contrato que GoalPlanningAdapter.LoadState. nil se trata como "vacío",
// no como "no tocar nada".
func (r *Replanner) LoadRegenerations(regenerations map[string]int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make(map[string]int, len(regenerations))
	for goalID, count := range regenerations {
		out[goalID] = count
	}
	r.regenerations = out
}

// hasPendingDependent recorre las tareas del plan que todavía no terminaron
// (ni Completed ni Cancelled) y devuelve true si alguna declara taskID como
// Dependency — el mismo criterio de lectura que nextRunnableTask/Task.
// CanStart ya usan, solo que acá se pregunta "¿alguien más la necesita?" en
// vez de "¿yo puedo empezar?".
func hasPendingDependent(plan *planning.Plan, taskID string) bool {
	for _, candidate := range plan.Tasks {
		if candidate == nil || candidate.ID == taskID {
			continue
		}
		if candidate.Status == planning.TaskCompleted || candidate.Status == planning.TaskCancelled {
			continue
		}
		for _, dep := range candidate.Dependencies {
			if dep == taskID {
				return true
			}
		}
	}
	return false
}
