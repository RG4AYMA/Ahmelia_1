package core

import (
	"ahmelia/autonomy"
	"ahmelia/planning"
	"ahmelia/shared"
)

// ============================================================================
// AUTONOMY_EXECUTIVE_REBUILD.GO — Bloque 90.8.2 (B46 <-> Autonomous Plan
// Unification, segundo sub-bloque)
// ============================================================================
//
// HALLAZGO QUE ESTE BLOQUE CORRIGE (auditoría B90.8, no un descuido de
// B90.6): tras un reinicio, ApplyAutonomyPersistenceSnapshot (B90.6)
// reconstruye correctamente planner.Goals()/Tasks(), AutonomousState y
// GoalPlanningAdapter.Plans() — el estado que el ciclo autónomo necesita
// para seguir funcionando. Pero las VISTAS EJECUTIVAS de Bloque 46
// (executiveGoals/executivePlans/executiveProgress en core/engine.go, el
// índice por NOMBRE que usan "cómo vamos con X" / "completé tarea en
// X") nunca se reconstruían: quedaban con los mapas vacíos que
// NewEngine() siempre crea al arrancar (engine.go, cerca de la línea
// 449), sin que nada los repoblara. Resultado real: después de
// reiniciar, un proyecto que Ahmelia sigue reconociendo internamente
// (por Goal.ID, vía planner/autonomousState) se vuelve invisible para la
// conversación — "cómo vamos con [nombre]" respondía como si el
// proyecto nunca hubiera existido.
//
// rebuildExecutiveViews es la corrección: SOLO indexa lo que
// ApplyAutonomyPersistenceSnapshot ya dejó cargado. Misma regla que fijó
// B90.8.1 — GoalPlanningAdapter (vía EnsurePlan) sigue siendo el único
// que genera un *planning.Plan para un Goal; esta función nunca llama
// EnsurePlan ni planner.GeneratePlan, así que no puede crear un Plan
// nuevo ni duplicar el que ya existe. Es una vista, no una segunda
// fuente de estado.
//
// LÍMITE HONESTO, nombrado en vez de fingido: AutonomyPersistenceSnapshot
// (B90.1) no guarda cuántas veces falló una Task antes de completarse o
// quedar bloqueada — solo su estado final (ver PersistedTask). Por eso
// Progress.TasksFailed/ErrorCount arrancan en 0 tras reconstruir, aunque
// esa Task sí haya fallado alguna vez antes del reinicio: no hay una
// forma honesta de reconstruir un conteo que nunca se persistió. Lo
// único que se reconstruye con certeza es TasksCompleted, porque
// Task.Status == TaskCompleted es un hecho presente en el snapshot, no
// una inferencia sobre el pasado.
func rebuildExecutiveViews(
	planner *planning.Planner,
	adapter *autonomy.GoalPlanningAdapter,
	executiveGoals map[string]*planning.Goal,
	executivePlans map[string]*planning.Plan,
	executiveProgress map[string]*planning.Progress,
) {
	if planner == nil || adapter == nil {
		return
	}

	for _, goal := range planner.Goals() {
		if goal == nil {
			continue
		}
		// Mismo criterio de clave que usa B46 al crear el proyecto
		// (engine.go, ParseCreateProjectQuery): shared.Normalize(name).
		// Si dos Goals restaurados normalizaran al mismo nombre (caso
		// que hoy tampoco se evita al crear), el último en planner.Goals()
		// gana el índice por nombre — no es un caso nuevo introducido
		// acá, es el mismo comportamiento que ya existía.
		executiveGoals[shared.Normalize(goal.Name)] = goal
	}

	for goalID, plan := range adapter.Plans() {
		if plan == nil {
			continue
		}
		executivePlans[goalID] = plan

		progress := planning.NewProgress(goalID, len(plan.Tasks), plan.TotalEstimate)
		for _, tsk := range plan.Tasks {
			if tsk != nil && tsk.IsCompleted() {
				progress.UpdateTaskCompleted()
			}
		}
		executiveProgress[goalID] = progress
	}
}
