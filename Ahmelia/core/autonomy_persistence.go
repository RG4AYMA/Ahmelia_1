package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// ============================================================================
// AUTONOMY_PERSISTENCE.GO — Bloque 90.1, Fase 6 (Persistencia del subsistema
// autónomo)
// ============================================================================
//
// Auditoría previa (ver sesión de diseño de B90): hoy nada de lo que vive en
// autonomy/+planning/ sobrevive un reinicio del proceso — planning.Planner y
// autonomy.AutonomousState arrancan siempre vacíos en NewEngine(), y
// Engine.Shutdown() nunca toca objetivos/planes/tareas. Este archivo define
// el snapshot serializable único (mismo patrón que solving.SkillLibrary /
// memory/skill_library.json).
//
// ESTADO REAL (sincronizado por GLM 19/10 tras la corrección v2 de la
// auditoría — antes este párrafo decía "SOLO el modelo" y quedaba
// desactualizado): B90.2-B90.8 están IMPLEMENTADOS E INTEGRADOS — carga en
// NewEngine (engine.go:1606-1628: LoadAutonomyPersistenceSnapshot + Apply +
// rebuildExecutiveViews, con manejo honesto de archivo ausente/corrupto),
// guardado tras cada mutación (core/autonomy_persistence_lifecycle.go —
// PersistAutonomyState / persistAutonomyStateBestEffort) y red de seguridad
// en Shutdown (engine.go:459-470). PENDIENTE SOLO la verificación EN VIVO V1
// (crear meta → cierre controlado → relanzar → la meta sigue): sin esa E2E
// no se declara verificado en runtime. Escalera: documentado→implementado;
// V1 la sube a verificado.
//
// Alcance de identidad (acordado en la sesión de diseño):
//   - Goals y Tasks se persisten completos ([]*planning.Goal / []*planning.Task
//     tal cual, todos los campos son exportados) — son la única fuente de
//     identidad real; Plan se persiste con TaskIDs (no Tasks embebidos) para
//     no duplicar cada Task dos veces en el JSON ni crear un segundo puntero
//     al mismo Task al deserializar.
//   - CurrentGoalID/PendingGoalIDs son IDs, no punteros — la reconstrucción
//     (B90.6) los resuelve contra el mismo mapa goalsByID que alimenta al
//     Planner, para que AutonomousState y Planner terminen apuntando al
//     mismo objeto.
//   - ReplanRegenerations es el contador de Replanner (B87) por Goal.ID.
//
// Deliberadamente FUERA de este snapshot (límite honesto, no descuido, ver
// sesión de diseño de B90):
//   - executiveGoals / executivePlans / executiveProgress (Bloque 46,
//     conversacional) — arquitectura histórica distinta, su posible
//     unificación con el ciclo autónomo es un bloque futuro con su propia
//     auditoría, no algo que B90 resuelve de paso.
//   - ExecutionState / ActiveTask / ActiveTopic / PendingQuestions /
//     PendingAssumptions de AutonomousState — reflejo de una conversación o
//     ejecución en vuelo; ninguna acción en curso sobrevive a un proceso
//     muerto, resucitarlas sería mentir sobre el estado real. Vuelven a su
//     valor inicial (ExecIdle, vacío) en cada arranque.
//   - DependencyGraph.dependencies (planning/dependency.go) — AddDependency
//     nunca se llama fuera de tests, es peso muerto en el flujo real hoy; se
//     reconstruye trivialmente reproduciendo RegisterGoal/RegisterTask sobre
//     lo restaurado, no hace falta persistir los edges.
//   - activeTasks de GoalPlanningAdapter NO se persisten como "en curso":
//     cualquier Task que estaba TaskInProgress al morir el proceso se
//     normaliza a TaskPending al cargar (B90.6) — ninguna ejecución
//     sobrevive a un reinicio, las tareas incompletas vuelven a ser
//     candidatas para ejecución.

// AutonomyPersistenceVersion identifica el formato del snapshot. Un cambio
// de forma incompatible en AutonomyPersistenceSnapshot (agregar un campo no
// rompe nada; renombrar o cambiar el tipo de uno existente sí) exige subir
// este número y agregar un migrator explícito antes de leer snapshots
// viejos — no se asume nunca que el JSON en disco coincide con la versión
// actual del código.
const AutonomyPersistenceVersion = 1

// AutonomyStatePath — mismo criterio que solving.SkillLibraryPath /
// memoryintelligence.LearningStatePath: una sola ruta conocida bajo memory/.
const AutonomyStatePath = "memory/autonomy_state.json"

// PersistedPlan es la representación en disco de un planning.Plan. Guarda
// TaskIDs en vez de []*planning.Task embebido a propósito (ver nota de
// cabecera): el dueño único de los punteros de Task es la lista Tasks del
// snapshot; un Plan que arrastrara sus propias copias de Task rompería esa
// invariante apenas se deserializara.
type PersistedPlan struct {
	GoalID        string                `json:"goal_id"`
	PlanID        string                `json:"plan_id"`
	CreatedAt     time.Time             `json:"created_at"`
	TaskIDs       []string              `json:"task_ids"`
	TotalEstimate time.Duration         `json:"total_estimate"`
	Priority      planning.GoalPriority `json:"priority"`
	Feasible      bool                  `json:"feasible"`
	Issues        []string              `json:"issues"`
}

// AutonomyPersistenceSnapshot es el snapshot único del subsistema autónomo
// — composición de planning.Planner (Goals+Tasks), GoalPlanningAdapter
// (Plans) y autonomy.Replanner (ReplanRegenerations), reconciliados por
// identidad de ID. No es una serialización independiente de cada
// componente: es UNA representación persistente del estado ejecutivo (ver
// sesión de diseño de B90).
type AutonomyPersistenceSnapshot struct {
	Version int `json:"version"`

	Goals []*planning.Goal `json:"goals"`
	Tasks []*planning.Task `json:"tasks"`
	Plans []PersistedPlan  `json:"plans"`

	CurrentGoalID  string   `json:"current_goal_id"`
	PendingGoalIDs []string `json:"pending_goal_ids"`

	ReplanRegenerations map[string]int `json:"replan_regenerations"`
}

// NewAutonomyPersistenceSnapshot arma un snapshot vacío pero con todos los
// slices/mapas inicializados (nunca nil) — mismo criterio defensivo que
// planning.NewGoal: un snapshot recién creado debe poder serializarse o
// recorrerse sin que el caller tenga que chequear nil primero.
func NewAutonomyPersistenceSnapshot() *AutonomyPersistenceSnapshot {
	return &AutonomyPersistenceSnapshot{
		Version:             AutonomyPersistenceVersion,
		Goals:               []*planning.Goal{},
		Tasks:               []*planning.Task{},
		Plans:               []PersistedPlan{},
		PendingGoalIDs:      []string{},
		ReplanRegenerations: map[string]int{},
	}
}

// LoadAutonomyPersistenceSnapshot carga memory/autonomy_state.json si
// existe; si no existe todavía (primera corrida de B90, mismo caso que
// LoadSkillLibrary/LoadRelationship la primera vez), devuelve un snapshot
// vacío sin error — no es una condición de fallo, es el estado inicial
// esperado del sistema.
func LoadAutonomyPersistenceSnapshot(path string) (*AutonomyPersistenceSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewAutonomyPersistenceSnapshot(), nil
		}
		return nil, err
	}

	snap := NewAutonomyPersistenceSnapshot()
	if err := json.Unmarshal(data, snap); err != nil {
		return nil, err
	}

	// Nil-safety defensiva: un JSON válido pero con un campo omitido (ej.
	// un snapshot escrito a mano, o un formato anterior sin
	// replan_regenerations todavía) no debe dejar el snapshot con mapas o
	// slices nil — mismo criterio que LoadSkillLibrary aplica a Skills.
	if snap.Goals == nil {
		snap.Goals = []*planning.Goal{}
	}
	if snap.Tasks == nil {
		snap.Tasks = []*planning.Task{}
	}
	if snap.Plans == nil {
		snap.Plans = []PersistedPlan{}
	}
	if snap.PendingGoalIDs == nil {
		snap.PendingGoalIDs = []string{}
	}
	if snap.ReplanRegenerations == nil {
		snap.ReplanRegenerations = map[string]int{}
	}

	return snap, nil
}

// Save persiste el snapshot de forma atómica: escribe a un archivo temporal
// en el mismo directorio y recién después lo renombra sobre el destino
// final. os.Rename dentro del mismo filesystem es atómico a nivel de SO —
// un crash o cierre a mitad del guardado deja el .tmp corrupto/incompleto,
// pero memory/autonomy_state.json nunca queda parcialmente escrito. Esto es
// deliberadamente más estricto que solving.SkillLibrary.Save (que escribe
// directo con ioutil.WriteFile): B90 guarda tras cada mutación real, no
// solo en Shutdown, así que la ventana de riesgo de un guardado interrumpido
// es mayor y justifica el paso extra (ver sesión de diseño de B90).
func (s *AutonomyPersistenceSnapshot) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ============================================================================
// APPLYAUTONOMYPERSISTENCESNAPSHOT — Bloque 90.6, Fase 6 (Persistencia del
// subsistema autónomo)
// ============================================================================
//
// Reconcilia un AutonomyPersistenceSnapshot ya cargado (LoadAutonomy
// PersistenceSnapshot) contra runtime real: registra Goals/Tasks en
// planning.Planner, resuelve CurrentGoal/PendingGoals en AutonomousState,
// carga Plans/ActiveTasks en GoalPlanningAdapter y ReplanRegenerations en
// su Replanner — todo preservando una única identidad de puntero por ID
// (ver nota de cabecera de este archivo: "Goal que ve el Planner" y "Goal
// que ve AutonomousState" tienen que ser el mismo objeto).
//
// No genera ningún Goal/Task nuevo (nunca llama planning.NewGoal/NewTask):
// solo re-registra los que ya vienen deserializados en el snapshot. No
// ejecuta ninguna acción (nunca llama GetNextAction/mapper/task.Start): es
// composición de estado, no un paso del ciclo autónomo.
//
// Reemplaza, no mergea: CurrentGoal/PendingGoals/Plans/ActiveTasks/
// ReplanRegenerations quedan exactamente como el snapshot los describe,
// nunca combinados con lo que AutonomousState/GoalPlanningAdapter/
// Replanner tuvieran antes de llamar a esta función (SetCurrentGoal y
// LoadState/LoadRegenerations ya son reemplazo puro por diseño de
// B90.4/B90.5; acá además se vacía explícitamente PendingGoals antes de
// repoblarla). La única pieza sin una forma de "reemplazar" hoy es
// planning.Planner (RegisterGoal/RegisterTask son inserciones puras, sin
// ningún Clear()) — por eso esta función EXIGE un Planner recién
// construido (sin Goals/Tasks propios) y falla explícito si no lo está,
// en vez de inventarle un método de limpieza que nadie pidió o, peor,
// mergear en silencio.
//
// Falla honesto: si el snapshot referencia un ID que no puede resolver (un
// Plan con un TaskID ausente de snap.Tasks, un CurrentGoalID/
// PendingGoalID ausente de snap.Goals, IDs duplicados), devuelve error
// ANTES de tocar planner/state/adapter/replanner — nunca un runtime a
// medio reconciliar. Toda la resolución de identidad se hace primero
// sobre estructuras locales (Fase 1); solo si TODO resuelve se aplica al
// runtime real (Fase 2), que ya no tiene ningún camino de error.
func ApplyAutonomyPersistenceSnapshot(
	snap *AutonomyPersistenceSnapshot,
	planner *planning.Planner,
	state *autonomy.AutonomousState,
	adapter *autonomy.GoalPlanningAdapter,
) error {
	if snap == nil {
		return fmt.Errorf("B90.6: snapshot nil")
	}
	if planner == nil || state == nil || adapter == nil {
		return fmt.Errorf("B90.6: planner/state/adapter no configurados")
	}
	if len(planner.Goals()) != 0 || len(planner.Tasks()) != 0 {
		return fmt.Errorf(
			"B90.6: el Planner ya tiene Goals/Tasks propios — ApplyAutonomyPersistenceSnapshot " +
				"no los limpia (planning.Planner no expone ningún método de reset hoy); debe " +
				"llamarse sobre un Planner recién construido, antes de que cualquier otra cosa lo registre",
		)
	}

	// --- Fase 1: resolver identidad y validar, sin tocar el runtime. ---

	goalsByID := make(map[string]*planning.Goal, len(snap.Goals))
	for _, g := range snap.Goals {
		if g == nil || g.ID == "" {
			return fmt.Errorf("B90.6: snapshot inválido — Goal nil o sin ID")
		}
		if _, dup := goalsByID[g.ID]; dup {
			return fmt.Errorf("B90.6: snapshot inválido — Goal.ID %q repetido", g.ID)
		}
		goalsByID[g.ID] = g
	}

	tasksByID := make(map[string]*planning.Task, len(snap.Tasks))
	for _, tsk := range snap.Tasks {
		if tsk == nil || tsk.ID == "" {
			return fmt.Errorf("B90.6: snapshot inválido — Task nil o sin ID")
		}
		if _, dup := tasksByID[tsk.ID]; dup {
			return fmt.Errorf("B90.6: snapshot inválido — Task.ID %q repetido", tsk.ID)
		}
		if _, ok := goalsByID[tsk.GoalID]; !ok {
			return fmt.Errorf("B90.6: snapshot inválido — Task %q referencia Goal.ID %q inexistente", tsk.ID, tsk.GoalID)
		}
		tasksByID[tsk.ID] = tsk
	}

	// Ninguna acción sobrevive a un reinicio (ver regla acordada de B90):
	// cualquier Task que había quedado TaskInProgress cuando el proceso
	// murió se normaliza a TaskPending ANTES de registrarse en el
	// Planner. Estados terminales (TaskCompleted/TaskBlocked/
	// TaskCancelled) y TaskPending se conservan tal cual venían.
	for _, tsk := range tasksByID {
		if tsk.Status == planning.TaskInProgress {
			tsk.Status = planning.TaskPending
		}
	}

	plans := make(map[string]*planning.Plan, len(snap.Plans))
	for _, pp := range snap.Plans {
		if _, ok := goalsByID[pp.GoalID]; !ok {
			return fmt.Errorf("B90.6: snapshot inválido — Plan %q referencia Goal.ID %q inexistente", pp.PlanID, pp.GoalID)
		}
		if _, dup := plans[pp.GoalID]; dup {
			return fmt.Errorf("B90.6: snapshot inválido — dos planes cacheados para el mismo Goal.ID %q", pp.GoalID)
		}
		tasks := make([]*planning.Task, 0, len(pp.TaskIDs))
		for _, taskID := range pp.TaskIDs {
			tsk, ok := tasksByID[taskID]
			if !ok {
				return fmt.Errorf("B90.6: snapshot inválido — Plan %q referencia Task.ID %q inexistente", pp.PlanID, taskID)
			}
			tasks = append(tasks, tsk)
		}
		plans[pp.GoalID] = &planning.Plan{
			ID:            pp.PlanID,
			GoalID:        pp.GoalID,
			CreatedAt:     pp.CreatedAt,
			Tasks:         tasks,
			TotalEstimate: pp.TotalEstimate,
			Priority:      pp.Priority,
			Feasible:      pp.Feasible,
			Issues:        pp.Issues,
		}
	}

	var currentGoal *planning.Goal
	if snap.CurrentGoalID != "" {
		g, ok := goalsByID[snap.CurrentGoalID]
		if !ok {
			return fmt.Errorf("B90.6: snapshot inválido — CurrentGoalID %q no está entre los Goals persistidos", snap.CurrentGoalID)
		}
		currentGoal = g
	}

	pendingGoals := make([]*planning.Goal, 0, len(snap.PendingGoalIDs))
	for _, id := range snap.PendingGoalIDs {
		g, ok := goalsByID[id]
		if !ok {
			return fmt.Errorf("B90.6: snapshot inválido — PendingGoalID %q no está entre los Goals persistidos", id)
		}
		pendingGoals = append(pendingGoals, g)
	}

	// --- Fase 2: todo resolvió — aplicar al runtime real. Ningún paso de
	// acá en adelante puede fallar. ---

	for _, g := range snap.Goals {
		planner.RegisterGoal(g)
	}
	for _, tsk := range snap.Tasks {
		planner.RegisterTask(tsk)
	}

	state.SetCurrentGoal(currentGoal)
	// AutonomousState.PendingGoals no tiene un "reemplazar de una", solo
	// Add/Remove por ID — se vacía explícitamente antes de repoblar para
	// que esta función reemplace de verdad, nunca mergee con lo que
	// hubiera antes de reconciliar.
	for _, existing := range state.PendingGoals() {
		state.RemovePendingGoal(existing.ID)
	}
	for _, g := range pendingGoals {
		state.AddPendingGoal(g)
	}

	// Ninguna Task activa sobrevive a un reinicio (ver regla acordada de
	// B90): el snapshot ni siquiera tiene un campo ActiveTasks — no hay
	// ningún ActionRequest.ID vivo al que asociarlas tras la muerte del
	// proceso. El mapa se carga siempre vacío, nunca "lo que había antes
	// de morir".
	adapter.LoadState(plans, map[string]*planning.Task{})
	adapter.Replanner().LoadRegenerations(snap.ReplanRegenerations)

	return nil
}
