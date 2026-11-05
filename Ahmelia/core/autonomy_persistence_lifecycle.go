package core

import (
	"context"
	"fmt"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// ============================================================================
// AUTONOMY_PERSISTENCE_LIFECYCLE.GO — Bloque 90.7, Fase 6 (Persistencia del
// subsistema autónomo)
// ============================================================================
//
// B90.6 (core/autonomy_persistence.go) ya resuelve snapshot -> runtime
// (ApplyAutonomyPersistenceSnapshot). Este archivo agrega el sentido
// contrario, runtime -> snapshot, sin tocar nada de esa lógica:
//
//   - PersistAutonomyState: el único punto de composición del snapshot a
//     partir del runtime real (Planner + AutonomousState + GoalPlanningAdapter
//     + su Replanner) y su guardado atómico en AutonomyStatePath.
//   - persistingCycleAdapter: decora GoalPlanningAdapter (CyclePlanner +
//     CycleUpdater) para disparar ese guardado después de cada mutación real
//     del ciclo autónomo — sin agregarle una sola línea a goal_planner.go
//     (B82/B90.4) ni a autonomy_persistence.go (B90.6).
//   - El wiring de carga al iniciar vive en NewEngine (justo después de
//     construir goalPlanningAdapter, mientras planner/autonomousState/
//     adapter siguen vacíos) y el de guardado final en Shutdown().
//
// Criterio de "cambio significativo" (según el diseño acordado de B90.7):
// GetNextAction real (ok=true) implica selección/cambio de objetivo actual
// más el arranque de una Task — GoalPlanningAdapter.GetNextAction ya hace
// state.SetCurrentGoal + task.Start() en ese caso. UpdateState siempre
// implica al menos un cambio de estado de Task (Complete/Fail/Retry/Cancel)
// y, según el camino, replanificación, regeneración, finalización o bloqueo
// del Goal (ver goal_planner.go). Cubrir esos dos puntos cubre exactamente
// la lista de disparadores del diseño, sin inventar un tercero.

// buildAutonomyPersistenceSnapshot arma el snapshot actual leyendo los
// accessors que B90.2/B90.4/B90.5 ya exponen (Planner.Goals/Tasks,
// GoalPlanningAdapter.Plans/Replanner().Regenerations, AutonomousState.
// CurrentGoal/PendingGoals).
//
// Hallazgo real de auditoría (B90.7, no un descuido): GoalPlanningAdapter.
// GetNextAction genera cada Plan con planner.GeneratePlan(goal) (ver
// planning/planner.go, decomposeGoal), que arma sus propias Tasks nuevas
// vía NewTask pero nunca las registra en el Planner — RegisterTask hoy
// solo se llama desde B90.6 al reconciliar un snapshot y desde el camino
// conversacional de Bloque 46 (creación de proyecto vía chat, que arma un
// executivePlan aparte y SÍ registra sus propias Tasks, sin relación con
// el plan cacheado del adapter). Sin corregir esto, planner.Tasks() jamás
// incluiría las Tasks que un ciclo autónomo real genera, y el snapshot
// terminaría con Plans que referencian Task.ID inexistentes en Tasks —
// exactamente lo que ApplyAutonomyPersistenceSnapshot (B90.6) rechaza al
// cargar, dejando el reinicio sin nada que reconciliar. Se corrige acá,
// en el único punto donde B90.7 compone el snapshot, registrando las
// Tasks de cada plan cacheado antes de leer Planner.Tasks() — sin tocar
// GetNextAction (B82) ni GeneratePlan (Bloque 46). RegisterTask es
// idempotente (sobreescribe la misma entrada por ID): volver a llamarlo
// sobre una Task ya registrada no tiene efecto ni duplica nada.
func buildAutonomyPersistenceSnapshot(
	planner *planning.Planner,
	state *autonomy.AutonomousState,
	adapter *autonomy.GoalPlanningAdapter,
) *AutonomyPersistenceSnapshot {
	snap := NewAutonomyPersistenceSnapshot()

	plans := adapter.Plans()
	for _, plan := range plans {
		if plan == nil {
			continue
		}
		for _, tsk := range plan.Tasks {
			if tsk != nil {
				planner.RegisterTask(tsk)
			}
		}
	}

	snap.Goals = planner.Goals()
	snap.Tasks = planner.Tasks()

	persisted := make([]PersistedPlan, 0, len(plans))
	for goalID, plan := range plans {
		if plan == nil {
			continue
		}
		taskIDs := make([]string, 0, len(plan.Tasks))
		for _, tsk := range plan.Tasks {
			if tsk != nil {
				taskIDs = append(taskIDs, tsk.ID)
			}
		}
		persisted = append(persisted, PersistedPlan{
			GoalID:        goalID,
			PlanID:        plan.ID,
			CreatedAt:     plan.CreatedAt,
			TaskIDs:       taskIDs,
			TotalEstimate: plan.TotalEstimate,
			Priority:      plan.Priority,
			Feasible:      plan.Feasible,
			Issues:        plan.Issues,
		})
	}
	snap.Plans = persisted

	if current := state.CurrentGoal(); current != nil {
		snap.CurrentGoalID = current.ID
	}
	pending := state.PendingGoals()
	pendingIDs := make([]string, 0, len(pending))
	for _, g := range pending {
		if g != nil {
			pendingIDs = append(pendingIDs, g.ID)
		}
	}
	snap.PendingGoalIDs = pendingIDs

	snap.ReplanRegenerations = adapter.Replanner().Regenerations()

	return snap
}

// PersistAutonomyState es el punto único de persistencia que pide B90.7:
// compone el snapshot actual del runtime autónomo y lo guarda de forma
// atómica (AutonomyPersistenceSnapshot.Save, B90.1) en AutonomyStatePath.
// No hace ninguna reconciliación ni toca activeTasks (eso sigue siendo
// exclusivamente B90.6, al cargar) — solo lee y escribe.
func (e *Engine) PersistAutonomyState() error {
	if e.planner == nil || e.autonomousState == nil || e.goalPlanningAdapter == nil {
		return fmt.Errorf("B90.7: planner/autonomousState/goalPlanningAdapter no configurados")
	}
	snap := buildAutonomyPersistenceSnapshot(e.planner, e.autonomousState, e.goalPlanningAdapter)
	return snap.Save(AutonomyStatePath)
}

func (e *Engine) persistAutonomyState() error {
	if e == nil {
		return fmt.Errorf("B90.7: Engine no configurado")
	}
	if e.goalStore != nil {
		return e.goalStore.Persist()
	}
	return e.PersistAutonomyState()
}

// persistAutonomyStateBestEffort guarda tras una mutación real del ciclo
// (ver persistingCycleAdapter) sin propagar el error hacia el ciclo
// autónomo: una falla de disco al persistir no debe descartar una acción
// que ya se aplicó correctamente en memoria ni bloquear el siguiente tick.
// El próximo guardado exitoso (la siguiente mutación, o Shutdown) vuelve a
// dejar el snapshot al día — asumido explícitamente como parte del diseño
// de B90.7, no un silenciamiento accidental de errores.
func (e *Engine) persistAutonomyStateBestEffort(origin string) {
	if err := e.persistAutonomyState(); err != nil {
		fmt.Printf("⚠️ [B90.7] no se pudo persistir el estado autónomo tras %s: %v\n", origin, err)
	}
}

// persistingCycleAdapter decora *autonomy.GoalPlanningAdapter para que
// buildAutonomousExecutionCycle (B88/B89) lo use en vez del adapter crudo
// como CyclePlanner y como parte del CycleUpdater compuesto — el único
// lugar donde B90.7 se conecta al ciclo real, sin que Cycle ni
// GoalPlanningAdapter necesiten saber que existe persistencia.
type persistingCycleAdapter struct {
	engine      *Engine
	goalAdapter *autonomy.GoalPlanningAdapter
}

func (p *persistingCycleAdapter) GetNextAction(ctx context.Context) (autonomy.ActionRequest, bool, error) {
	if p == nil || p.engine == nil || p.engine.autonomousState == nil || p.goalAdapter == nil {
		return autonomy.ActionRequest{}, false, fmt.Errorf("B90.7: adapter de persistencia autónoma no configurado")
	}
	previousGoal := p.engine.autonomousState.CurrentGoal()
	previousTask := p.engine.autonomousState.ActiveTask()
	req, ok, err := p.goalAdapter.GetNextAction(ctx)
	if err == nil && ok {
		// ok=true implica que GoalPlanningAdapter ya hizo
		// state.SetCurrentGoal(goal) + task.Start(): selección/cambio de
		// objetivo real. Su guardado debe completarse antes del dispatch.
		if persistErr := p.engine.persistAutonomyState(); persistErr != nil {
			rolledBack := p.goalAdapter.RollbackBeforeDispatch(req.ID)
			p.engine.autonomousState.SetCurrentGoal(previousGoal)
			p.engine.autonomousState.SetActiveTask(previousTask)
			if !rolledBack {
				return autonomy.ActionRequest{}, false, fmt.Errorf("B90.7: no se pudo guardar ni revertir la tarea antes del dispatch: %w", persistErr)
			}
			return autonomy.ActionRequest{}, false, fmt.Errorf("B90.7: dispatch cancelado porque falló el guardado previo: %w", persistErr)
		}
	}
	return req, ok, err
}

func (p *persistingCycleAdapter) GetNextActionForCandidate(ctx context.Context, candidateID string) (autonomy.ActionRequest, bool, error) {
	if p == nil || p.engine == nil || p.engine.autonomousState == nil || p.goalAdapter == nil {
		return autonomy.ActionRequest{}, false, fmt.Errorf("B90.7: adapter de persistencia autónoma no configurado")
	}
	previousGoal := p.engine.autonomousState.CurrentGoal()
	previousTask := p.engine.autonomousState.ActiveTask()
	req, ok, err := p.goalAdapter.GetNextActionForCandidate(ctx, candidateID)
	if err == nil && ok {
		if persistErr := p.engine.persistAutonomyState(); persistErr != nil {
			rolledBack := p.goalAdapter.RollbackBeforeDispatch(req.ID)
			p.engine.autonomousState.SetCurrentGoal(previousGoal)
			p.engine.autonomousState.SetActiveTask(previousTask)
			if !rolledBack {
				return autonomy.ActionRequest{}, false, fmt.Errorf("B90.7: no se pudo guardar ni revertir la tarea candidata antes del dispatch: %w", persistErr)
			}
			return autonomy.ActionRequest{}, false, fmt.Errorf("B90.7: dispatch de tarea candidata cancelado porque falló el guardado previo: %w", persistErr)
		}
	}
	return req, ok, err
}

func (p *persistingCycleAdapter) UpdateState(ctx context.Context, req autonomy.ActionRequest, verif autonomy.VerificationResult) error {
	err := p.goalAdapter.UpdateState(ctx, req, verif)
	if err == nil {
		// Cubre el resto de la lista de B90.7 de un solo lugar: tarea
		// completada/fallida/reintentada/saltada, replanificación,
		// regeneración, y finalización/bloqueo del Goal — todo eso ocurre
		// adentro de GoalPlanningAdapter.UpdateState (ver goal_planner.go).
		p.engine.persistAutonomyStateBestEffort("UpdateState")
	}
	return err
}
