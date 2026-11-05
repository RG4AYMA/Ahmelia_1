package autonomy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"ahmelia/planning"
)

// GoalActionMapper traduce una tarea de planning en una acción ejecutable de
// Autonomía. B81 no adivina que el título de una tarea sea una acción:
// la traducción debe ser explícita.
// Esto mantiene separada la semántica de planning de los nombres de acciones
// que B79 entiende.
type GoalActionMapper interface {
	MapTask(ctx context.Context, goal *planning.Goal, task *planning.Task) (ActionRequest, error)
}

// FailureAdvisor permite que una capa de diagnóstico externa (B92 —
// Diagnoser + Recovery, ver core/) le diga a GoalPlanningAdapter que UNA
// tarea fallida puntual NO debe pasar por el camino normal de
// retry/skip/regenerate de Replanner — porque la causa real ya fue
// clasificada como algo que ningún reintento puede arreglar (un permiso
// denegado, una anomalía de correlación request/response, etc.), algo que
// Replanner no puede saber por sí solo porque solo mira Task.Attempts y
// las dependencias del plan (ver replanner.go).
//
// Advise se consulta una vez por tarea fallida, justo antes de que
// Replanner.Decide corra, usando solo el mismo taskID que UpdateState ya
// tiene. Responde una única pregunta binaria:
//   - ok == false: sin opinión (no hay diagnóstico guardado para esa
//     tarea, o el diagnóstico la juzgó recuperable) — GoalPlanningAdapter
//     sigue exactamente como antes de que este hook existiera, llamando a
//     Replanner.Decide sin modificar nada. Este es el comportamiento por
//     defecto cuando no se configura ningún advisor (campo nil).
//   - ok == true: reemplazar la decisión de Replanner para esta tarea por
//     action/reason. GoalPlanningAdapter la aplica por el mismo switch que
//     ya usa para las decisiones de Replanner — ningún camino nuevo de
//     mutación de Task/Goal/Plan.
//
// Un FailureAdvisor nunca recibe Goal/Plan/Task y no puede ejecutar nada:
// solo puede vetar/redirigir, para una tarea ya fallida, la autoridad que
// ya tiene Replanner — nunca originar una acción por su cuenta.
type FailureAdvisor interface {
	Advise(taskID string) (action ReplanAction, reason string, ok bool)
}

// GoalPlanningAdapter conecta B73/B72 con planning.Planner y satisface los
// contratos que B78 necesita:
//   - CyclePlanner: obtiene la siguiente acción de una meta.
//   - CycleUpdater: refleja el resultado de esa acción en Task y Goal.
//
// No ejecuta nada y no conoce core.Engine ni solving.Execute.
type GoalPlanningAdapter struct {
	mu sync.Mutex

	state     *AutonomousState
	monitor   *GoalMonitor
	planner   *planning.Planner
	mapper    GoalActionMapper
	selector  *GoalSelector
	replanner *Replanner
	// advisor — Bloque 92.4, ver FailureAdvisor arriba. nil por defecto:
	// ningún caller existente (16 sitios, ver auditoría de B92.4) construye
	// uno, así que preservar el comportamiento exacto de antes de B92.4
	// exige que "no configurado" sea un no-op, nunca un error.
	advisor FailureAdvisor

	plans       map[string]*planning.Plan
	activeTasks map[string]*planning.Task

	// mapFailures/maxMapFailures — Bloque 90.8.3, ver más abajo.
	mapFailures    map[string]int
	maxMapFailures int
}

// SetFailureAdvisor conecta un FailureAdvisor opcional (B92.4). Un setter
// en vez de un parámetro más en NewGoalPlanningAdapter mantiene intacta la
// firma de 6 argumentos que ya usan sus 16 call sites (ver auditoría de
// B92.4) — mismo criterio que Cycle.SetEnvironmentVerifier aplicó en su
// momento para no romper NewCycle. Pasar nil (o nunca llamar a este
// método) es válido y es el estado inicial: UpdateState se comporta
// exactamente como antes de B92.4.
func (a *GoalPlanningAdapter) SetFailureAdvisor(advisor FailureAdvisor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.advisor = advisor
}

// NewGoalPlanningAdapter crea el adaptador Goal -> Plan -> Task -> ActionRequest.
// selector nil usa un GoalSelector por defecto (DefaultGoalSelectionWeights)
// construido sobre el mismo state/monitor — mismo criterio que ActionVerifier
// nil en NewCycle: un caller que no tiene una razón para personalizar la
// selección no debería tener que construir el selector a mano. replanner nil
// usa un Replanner por defecto (DefaultReplanLimits, B87): antes de ese
// bloque, UpdateState bloqueaba el objetivo entero ante la primera tarea
// fallida; un caller que no pasa replanner no debería perder ese
// comportamiento amortiguado por no haber construido uno a mano.
func NewGoalPlanningAdapter(
	state *AutonomousState,
	monitor *GoalMonitor,
	planner *planning.Planner,
	mapper GoalActionMapper,
	selector *GoalSelector,
	replanner *Replanner,
) (*GoalPlanningAdapter, error) {
	if state == nil {
		return nil, fmt.Errorf("autonomy: estado B72 no configurado")
	}
	if monitor == nil {
		return nil, fmt.Errorf("autonomy: GoalMonitor no configurado")
	}
	if planner == nil {
		return nil, fmt.Errorf("autonomy: planning.Planner no configurado")
	}
	if mapper == nil {
		return nil, fmt.Errorf("autonomy: GoalActionMapper no configurado")
	}
	if selector == nil {
		var err error
		selector, err = NewGoalSelector(state, monitor, GoalSelectionWeights{})
		if err != nil {
			return nil, fmt.Errorf("autonomy: selector B86 por defecto: %w", err)
		}
	}
	if replanner == nil {
		replanner = NewReplanner(ReplanLimits{})
	}

	return &GoalPlanningAdapter{
		state:          state,
		monitor:        monitor,
		planner:        planner,
		mapper:         mapper,
		selector:       selector,
		replanner:      replanner,
		plans:          make(map[string]*planning.Plan),
		activeTasks:    make(map[string]*planning.Task),
		mapFailures:    make(map[string]int),
		maxMapFailures: DefaultMaxPlanMapFailures,
	}, nil
}

// ============================================================================
// CORTE DEL LOOP SILENCIOSO — Bloque 90.8.3 (B46 <-> Autonomous Plan
// Unification, tercer sub-bloque)
// ============================================================================
//
// HALLAZGO QUE ESTE BLOQUE CORRIGE (auditoría B90.8): un fallo de
// GoalActionMapper.MapTask (fase PLAN — la tarea nunca llega a
// task.Start(), nunca pasa por Verify/Replanner) se propagaba como error
// de GetNextAction, Cycle.RunStep lo devolvía tal cual, y
// AutonomousLoop.runTick lo descartaba en silencio
// (`_ = al.cycle.RunStep(ctx)`). Para un Goal cuyas Tasks nunca resuelven
// a ninguna capacidad ejecutable (el caso típico: decomposeGoal genera
// texto genérico que CapabilityResolver no puede mapear, ver B89), esto
// significaba reintentar la MISMA tarea, con el MISMO resultado, para
// siempre — sin bloquear el Goal, sin loguear nada, sin que
// GoalMonitor/GoalSelector/B91 pudieran verlo.
//
// DefaultMaxPlanMapFailures / mapFailures aplican al fallo de MAPEO (fase
// PLAN) el mismo desenlace terminal que Replanner (87) ya aplica al fallo
// de EJECUCIÓN (fase VERIFY) tras agotar sus límites: el Goal pasa a
// GoalBlocked, sale de CurrentGoal/PendingGoals, dos de las
// responsabilidades que hasta B90.8 no la unificaban. No reemplaza a
// Replanner ni se activa por el mismo camino — Replanner.Decide nunca ve
// una tarea que jamás llegó a intentarse.

// DefaultMaxPlanMapFailures es cuántos fallos consecutivos de MapTask se
// toleran para un mismo Goal antes de bloquearlo. 3 (no 1): el primer
// fallo puede deberse a una SkillLibrary que todavía estaba cargando o a
// un ctx cancelado de un tick anterior — exactamente el mismo margen que
// ya se le da a fallos transitorios de ejecución en otros puntos de este
// paquete, no una cifra arbitraria.
const DefaultMaxPlanMapFailures = 3

// SetMaxMapFailures ajusta ese tope. n <= 0 restaura
// DefaultMaxPlanMapFailures — mismo criterio de "no dejar un tope en cero
// por no haberlo pasado" que ya aplican NewReplanner/NewGoalMonitor.
func (a *GoalPlanningAdapter) SetMaxMapFailures(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n <= 0 {
		n = DefaultMaxPlanMapFailures
	}
	a.maxMapFailures = n
}

// handleMapFailureLocked registra un fallo de MapTask para goal y decide
// si ya toca bloquearlo. Requiere a.mu ya tomado (mismo criterio que
// ensurePlanLocked). Nunca es llamado con task/mapErr nil por su único
// caller (GetNextAction, justo donde MapTask ya falló).
func (a *GoalPlanningAdapter) handleMapFailureLocked(goal *planning.Goal, task *planning.Task, mapErr error) (ActionRequest, bool, error) {
	a.mapFailures[goal.ID]++
	count := a.mapFailures[goal.ID]

	if count < a.maxMapFailures {
		return ActionRequest{}, false, fmt.Errorf(
			"autonomy: mapear tarea %q (intento %d/%d): %w", task.ID, count, a.maxMapFailures, mapErr,
		)
	}

	// Se agotaron los intentos: el mismo desenlace terminal que
	// ReplanGiveUp aplica a un fallo de ejecución, aplicado acá a un
	// fallo de mapeo — el Goal no puede seguir ofreciéndose a
	// GetNextAction cada tick para fallar exactamente igual.
	goal.SetBlocked(fmt.Sprintf(
		"autonomy: %d fallos consecutivos resolviendo capacidad para la tarea %q: %v",
		count, task.ID, mapErr,
	))
	delete(a.mapFailures, goal.ID)
	delete(a.plans, goal.ID)
	if current := a.state.CurrentGoal(); current != nil && current.ID == goal.ID {
		a.state.SetCurrentGoal(nil)
	} else {
		a.state.RemovePendingGoal(goal.ID)
	}

	return ActionRequest{}, false, fmt.Errorf(
		"autonomy: objetivo %q bloqueado tras %d fallos consecutivos resolviendo capacidad para la tarea %q: %w",
		goal.ID, count, task.ID, mapErr,
	)
}

// GetNextAction implementa CyclePlanner usando el candidato de GoalSelector.
// El plan se genera una sola vez por Goal.
func (a *GoalPlanningAdapter) GetNextAction(ctx context.Context) (ActionRequest, bool, error) {
	if err := ctx.Err(); err != nil {
		return ActionRequest{}, false, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	return a.getNextActionLocked(ctx, a.selectGoal())
}

// GetNextActionForCandidate continues from the candidate used to construct a
// cycle Situation. If canonical selection changed in the meantime, it abstains
// and lets the next cycle rebuild evidence for the new candidate.
func (a *GoalPlanningAdapter) GetNextActionForCandidate(ctx context.Context, candidateID string) (ActionRequest, bool, error) {
	if err := ctx.Err(); err != nil {
		return ActionRequest{}, false, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	goal := a.selectGoal()
	if goal == nil || strings.TrimSpace(candidateID) == "" || goal.ID != candidateID {
		return ActionRequest{}, false, nil
	}
	return a.getNextActionLocked(ctx, goal)
}

func (a *GoalPlanningAdapter) getNextActionLocked(ctx context.Context, goal *planning.Goal) (ActionRequest, bool, error) {
	if goal == nil {
		return ActionRequest{}, false, nil
	}

	if goal.Status == planning.GoalBlocked || goal.Status == planning.GoalCompleted ||
		goal.Status == planning.GoalPaused {
		return ActionRequest{}, false, nil
	}

	plan, err := a.ensurePlanLocked(ctx, goal)
	if err != nil {
		return ActionRequest{}, false, err
	}

	task := nextRunnableTask(plan)
	if task == nil {
		return ActionRequest{}, false, nil
	}

	req, err := a.mapper.MapTask(ctx, goal, task)
	if err != nil {
		return a.handleMapFailureLocked(goal, task, err)
	}
	// Mapeo exitoso: cualquier racha previa de fallos para este Goal
	// queda saldada — B90.8.3 cuenta fallos CONSECUTIVOS, no fallos
	// históricos acumulados.
	delete(a.mapFailures, goal.ID)

	if req.ID == "" {
		req.ID = task.ID
	}
	task.Start()
	a.activeTasks[req.ID] = task

	a.state.SetCurrentGoal(goal)
	a.state.SetActiveTask(task.Title)

	return req, true, nil
}

// UpdateState implementa CycleUpdater. Solo actualiza la tarea que B81
// entregó anteriormente; no intenta inferir una acción distinta.
func (a *GoalPlanningAdapter) UpdateState(ctx context.Context, req ActionRequest, verif VerificationResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	task, ok := a.activeTasks[req.ID]
	if !ok {
		return fmt.Errorf("autonomy: no hay tarea activa para request %q", req.ID)
	}

	delete(a.activeTasks, req.ID)

	plan := a.plans[task.GoalID]
	goal := a.goalByID(task.GoalID)

	if verif.Success {
		task.Complete()
	} else {
		if verificationOutcomeUnknown(verif) {
			reason := unknownVerificationReason(verif)
			task.Fail(reason)
			if goal != nil {
				goal.SetBlocked(reason)
				goal.AddNote(fmt.Sprintf("tarea %q detenida: resultado incierto; requiere reconciliación antes de continuar", task.ID))
				a.replanner.ResetGoal(goal.ID)
				if current := a.state.CurrentGoal(); current != nil && current.ID == goal.ID {
					a.state.SetCurrentGoal(nil)
					a.state.SetActiveTask("")
				}
			}
			return nil
		}
		task.Fail(verif.ErrorReason)

		// B87: antes de este bloque, cualquier tarea fallida bloqueaba el
		// objetivo entero de inmediato (ver comentario de Replanner). Ahora
		// se consulta primero si conviene reintentar, saltar o pedir un
		// plan nuevo — el bloqueo definitivo (ReplanGiveUp) sigue el mismo
		// camino que ya existía, más abajo.
		if plan != nil && goal != nil {
			decision := a.replanner.Decide(goal, plan, task)

			// B92.4: si hay un FailureAdvisor configurado y tiene opinión
			// para esta tarea (ver FailureAdvisor arriba), su decisión
			// reemplaza la de Replanner — nunca se combinan ni se
			// promedian, gana una o la otra. Esto es lo único que agrega
			// B92.4: Replanner sigue siendo quien decide en el caso normal,
			// exactamente igual que antes de este bloque.
			if a.advisor != nil {
				if action, reason, ok := a.advisor.Advise(task.ID); ok {
					decision = ReplanDecision{Action: action, Reason: reason}
				}
			}

			switch decision.Action {
			case ReplanRetryTask:
				task.Retry()
				goal.AddNote(fmt.Sprintf("B87 reintenta %q: %s", task.Title, decision.Reason))
			case ReplanSkipTask:
				// planning.Task no expone un Cancel(): TaskCancelled existe
				// como estado pero, hasta este bloque, nada lo asignaba.
				task.Status = planning.TaskCancelled
				goal.AddNote(fmt.Sprintf("B87 salta %q: %s", task.Title, decision.Reason))
			case ReplanRegeneratePlan:
				delete(a.plans, goal.ID)
				a.replanner.RecordRegeneration(goal.ID)
				goal.AddNote(fmt.Sprintf("B87 regenera el plan de %q: %s", goal.Name, decision.Reason))
				// El plan en mano queda descartado: GetNextAction pedirá
				// uno nuevo la próxima vez. No tiene sentido recalcular
				// progreso/bloqueo sobre un plan que ya no se va a usar.
				return nil
			case ReplanGiveUp:
				// Sin acción acá: el bloqueo lo aplica el camino de abajo,
				// igual que antes de B87.
			}
		}
	}

	if plan == nil || goal == nil {
		return nil
	}

	completed := 0
	blocked := false
	for _, candidate := range plan.Tasks {
		if candidate.IsCompleted() {
			completed++
		}
		if candidate.IsBlocked() {
			blocked = true
		}
	}
	goal.UpdateProgress(completed, len(plan.Tasks))

	if blocked {
		goal.SetBlocked(verif.ErrorReason)
		a.replanner.ResetGoal(goal.ID)
		return nil
	}
	if completed == len(plan.Tasks) && len(plan.Tasks) > 0 {
		goal.Complete()
		a.replanner.ResetGoal(goal.ID)
	}

	return nil
}

// RollbackBeforeDispatch restores the in-memory task state when Core cannot
// persist the selected action before the physical execution boundary. The
// action has not been dispatched, so this is not a retry of an external effect.
func (a *GoalPlanningAdapter) RollbackBeforeDispatch(requestID string) bool {
	if a == nil || strings.TrimSpace(requestID) == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	task, ok := a.activeTasks[requestID]
	if !ok || task == nil || task.Status != planning.TaskInProgress {
		return false
	}
	task.Status = planning.TaskPending
	task.StartedAt = time.Time{}
	if task.Attempts > 0 {
		task.Attempts--
	}
	delete(a.activeTasks, requestID)
	if a.state != nil && a.state.ActiveTask() == task.Title {
		a.state.SetActiveTask("")
	}
	return true
}

func unknownVerificationReason(result VerificationResult) string {
	reason := strings.TrimSpace(result.ErrorReason)
	if reason == "" {
		reason = "no hay evidencia suficiente para determinar el resultado físico"
	}
	status := "unknown"
	if result.AuthorityResult != nil {
		if reported := strings.TrimSpace(result.AuthorityResult.AuthorityOutcomeStatus()); reported != "" {
			status = reported
		}
	}
	observation := strings.TrimSpace(string(result.ObservationStatus))
	if observation == "" {
		observation = "not_reported"
	}
	return fmt.Sprintf("resultado incierto; no reintentar sin reconciliación: %s [authority=%s observation=%s]", reason, status, observation)
}

// ============================================================================
// ENSUREPLAN() — Bloque 90.8.1 (B46 <-> Autonomous Plan Unification)
// ============================================================================
//
// HALLAZGO QUE ESTE BLOQUE CORRIGE (auditoría B90.8, no un descuido de
// B82): antes de este método, GetNextAction generaba su propio Plan para
// un Goal la primera vez que lo tocaba (líneas de arriba, hoy
// reemplazadas), sin saber que engine.go (Bloque 46, camino conversacional
// "quiero crear un proyecto X") YA había generado un Plan distinto para
// ese mismo Goal.ID llamando a planner.GeneratePlan(goal) por su cuenta.
// Resultado: dos *planning.Plan independientes, con Tasks independientes,
// para el mismo Goal — el ciclo autónomo ejecutaba uno (silenciosamente,
// casi siempre sin poder resolver ninguna capacidad real, ver
// CapabilityResolver/B89) mientras la conversación ("cómo vamos con X",
// "completé tarea en X") seguía el otro. Dos fuentes de verdad para lo
// mismo.
//
// EnsurePlan es ahora el ÚNICO punto de entrada para obtener el Plan de
// un Goal, para cualquier caller (B46 conversacional o el ciclo
// autónomo): quien llegue primero genera y cachea; todos los que lleguen
// después —sin importar desde qué lado del sistema— reciben el MISMO
// *planning.Plan (mismo puntero), con las MISMAS Tasks (mismos punteros,
// ya registrados en el Planner). GetNextAction ya no tiene su propia
// lógica de generación: llama a este mismo camino (ensurePlanLocked, ya
// con el mutex tomado).
// MapTaskForWorkflow exposes the same canonical task-to-action mapping used
// by GetNextAction, without changing task lifecycle state. Workflow compilation
// must not call GetNextAction because that method intentionally starts the task.
func (a *GoalPlanningAdapter) MapTaskForWorkflow(ctx context.Context, goal *planning.Goal, task *planning.Task) (ActionRequest, error) {
	if a == nil || a.mapper == nil {
		return ActionRequest{}, fmt.Errorf("autonomy: goal action mapper unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ActionRequest{}, err
	}
	if goal == nil || task == nil {
		return ActionRequest{}, fmt.Errorf("autonomy: goal/task no configurado")
	}
	if task.GoalID != goal.ID {
		return ActionRequest{}, fmt.Errorf("autonomy: tarea %q pertenece al Goal %q, no al Goal %q", task.ID, task.GoalID, goal.ID)
	}
	req, err := a.mapper.MapTask(ctx, goal, task)
	if err != nil {
		return ActionRequest{}, err
	}
	if req.ID == "" {
		req.ID = task.ID
	}
	return req, nil
}

func (a *GoalPlanningAdapter) EnsurePlan(goal *planning.Goal) (*planning.Plan, error) {
	return a.EnsurePlanContext(context.Background(), goal)
}

// EnsurePlanContext propagates the originating request and its observed
// baseline into the canonical planner. A cached canonical plan remains reused.
func (a *GoalPlanningAdapter) EnsurePlanContext(ctx context.Context, goal *planning.Goal) (*planning.Plan, error) {
	if goal == nil {
		return nil, fmt.Errorf("autonomy: goal no configurado")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ensurePlanLocked(ctx, goal)
}

// ensurePlanLocked es la lógica real de EnsurePlan — separada para que
// GetNextAction pueda reusarla sin re-tomar a.mu (ya lo tiene tomado) y
// sin duplicar una segunda copia de "generar + registrar + cachear".
// Requiere que el caller ya sostenga a.mu.
func (a *GoalPlanningAdapter) ensurePlanLocked(ctx context.Context, goal *planning.Goal) (*planning.Plan, error) {
	if plan, ok := a.plans[goal.ID]; ok {
		return plan, nil
	}

	plan := a.planner.GeneratePlanContext(ctx, goal)
	if plan == nil {
		return nil, fmt.Errorf("autonomy: planner devolvió un plan nil para %q", goal.ID)
	}

	// La planificación sigue siendo propiedad de planning.Planner, pero la
	// traducción Goal -> capacidad ya pertenece a este adaptador. Si el
	// Planner produjo una descomposición genérica y el propio Goal ya expresa
	// una acción autónoma ejecutable, no debemos destruir esa intención para
	// después pedirle a CapabilityResolver que resuelva "Análisis",
	// "Diseño", etc. Se prueba el contrato existente sobre una Task
	// transitoria; solo si el mapper confirma que el Goal completo es una
	// acción ejecutable se materializa una única Task con esa intención.
	//
	// Esto no introduce conocimiento de capabilities en planning.Planner ni
	// una segunda resolución: reutiliza exactamente GoalActionMapper, que ya
	// es la autoridad de Goal -> ActionRequest.
	if actionPlan := a.actionPlanFromGoalLocked(goal); actionPlan != nil {
		plan = actionPlan
	}

	if !plan.Feasible {
		return nil, fmt.Errorf("autonomy: plan no viable para %q: %v", goal.ID, plan.Issues)
	}

	// Registrar las Tasks en el Planner es parte de "generar el Plan
	// único" — antes de este bloque, esto solo lo hacía a mano el camino
	// de B46 en engine.go; ahora vive en el único lugar que genera Plans,
	// para que también ocurra cuando quien genera primero es el ciclo
	// autónomo (no solo cuando es la conversación).
	for _, tsk := range plan.Tasks {
		if tsk != nil {
			a.planner.RegisterTask(tsk)
		}
	}

	a.plans[goal.ID] = plan
	return plan, nil
}

// selectGoal delega en GoalSelector (B86) cuál objetivo merece la próxima
// acción. Hasta este bloque, acá vivía la selección ciega ("el primer
// Active que aparezca en el Snapshot") que no miraba Priority, Deadline ni
// Dependencies — ver la nota de cabecera de goal_selector.go para el
// detalle de qué reemplaza y por qué. Esta función queda como el único
// punto de entrada de selección para GetNextAction/goalByID, sin que el
// resto del adaptador necesite saber que la decisión ahora vive en otro
// archivo.
func (a *GoalPlanningAdapter) actionPlanFromGoalLocked(goal *planning.Goal) *planning.Plan {
	// Este puente corrige específicamente la integración real con
	// CapabilityResolver. Los mappers de prueba/otros dominios conservan
	// exactamente el contrato histórico de Planner -> Task -> Mapper y no
	// deben sufrir una llamada exploratoria adicional.
	if _, ok := a.mapper.(*CapabilityResolver); !ok {
		return nil
	}
	if goal == nil || strings.TrimSpace(goal.Name) == "" {
		return nil
	}

	probe := planning.NewTask(goal.ID, goal.Name, goal.Name, 1, 0)
	if _, err := a.mapper.MapTask(context.Background(), goal, probe); err != nil {
		return nil
	}

	plan := &planning.Plan{
		ID:        fmt.Sprintf("plan-action-%s", goal.ID),
		GoalID:    goal.ID,
		CreatedAt: time.Now(),
		Tasks:     []*planning.Task{probe},
		Priority:  goal.Priority,
		Feasible:  true,
		Issues:    []string{},
	}
	return plan
}

func (a *GoalPlanningAdapter) selectGoal() *planning.Goal {
	return a.selector.SelectGoal()
}

// PreviewNextGoal returns a detached copy of the current candidate from the adapter's
// canonical selector. It does not set CurrentGoal, generate a plan, start a
// task, or otherwise advance execution; GetNextAction remains the mutating
// owner and re-evaluates the selector when planning actually begins.
func (a *GoalPlanningAdapter) PreviewNextGoal() *planning.Goal {
	if a == nil || a.selector == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	goal := a.selectGoal()
	if goal == nil {
		return nil
	}
	copyGoal := *goal
	copyGoal.Dependencies = append([]string(nil), goal.Dependencies...)
	copyGoal.DependentOn = append([]string(nil), goal.DependentOn...)
	copyGoal.Blockers = append([]string(nil), goal.Blockers...)
	copyGoal.Notes = append([]string(nil), goal.Notes...)
	return &copyGoal
}

func (a *GoalPlanningAdapter) goalByID(id string) *planning.Goal {
	if goal := a.state.CurrentGoal(); goal != nil && goal.ID == id {
		return goal
	}
	for _, snapshot := range a.monitor.Snapshot() {
		if snapshot.Goal != nil && snapshot.Goal.ID == id {
			return snapshot.Goal
		}
	}
	return nil
}

func nextRunnableTask(plan *planning.Plan) *planning.Task {
	if plan == nil {
		return nil
	}

	tasks := make(map[string]*planning.Task, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task != nil {
			tasks[task.ID] = task
		}
	}

	for _, task := range plan.Tasks {
		if task == nil || task.Status != planning.TaskPending {
			continue
		}
		if task.CanStart(tasks) {
			return task
		}
	}
	return nil
}

// ============================================================================
// PLANS() / ACTIVETASKS() / LOADSTATE() — Bloque 90.4, Fase 6 (Persistencia
// del subsistema autónomo)
// ============================================================================
//
// GoalPlanningAdapter no gana ninguna lógica de persistencia en este
// bloque — solo expone (Plans/ActiveTasks) y recibe (LoadState) el estado
// que ya sostenía desde B82. Quién decide qué guardar, cómo reconciliar
// identidad contra Planner, y la regla de "ninguna Task en curso sobrevive
// a un reinicio" (TaskInProgress -> TaskPending al cargar) es enteramente
// responsabilidad de core/autonomy_persistence.go (B90.6) — este archivo
// no sabe que existe un JSON en disco.
//
// Plans()/ActiveTasks() devuelven un mapa NUEVO (mismo criterio que
// Planner.Goals()/Tasks() de B90.2): el caller no puede agregar/quitar
// entradas mutando el mapa recibido, pero los *Plan/*Task son los mismos
// punteros que el adapter sigue usando — copiar los structs reintroduciría
// el problema de identidad que B90 existe para evitar.

// Plans devuelve una copia del mapa de planes cacheados (Goal.ID -> *Plan).
// Solo lectura: no genera, no muta, no descarta ningún plan.
func (a *GoalPlanningAdapter) Plans() map[string]*planning.Plan {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := make(map[string]*planning.Plan, len(a.plans))
	for goalID, plan := range a.plans {
		out[goalID] = plan
	}
	return out
}

// ActiveTasks devuelve una copia del mapa de tareas activas
// (ActionRequest.ID -> *Task). Solo lectura, mismo criterio que Plans().
func (a *GoalPlanningAdapter) ActiveTasks() map[string]*planning.Task {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := make(map[string]*planning.Task, len(a.activeTasks))
	for reqID, task := range a.activeTasks {
		out[reqID] = task
	}
	return out
}

// LoadState reemplaza integralmente plans/activeTasks por los que le pasa
// la reconciliación de B90 (ya resueltos contra el mismo mapa de *Task/
// *Goal que alimentó al Planner, para preservar identidad — ver B90.6).
// No es un merge: llamar LoadState reemplaza lo que el adapter tuviera
// antes, tal como GetNextAction/UpdateState lo dejaron. nil se trata como
// "vacío", no como "no tocar nada" — un caller que quiere limpiar el
// estado sin cargar nada pasa nil explícito, no omite la llamada.
//
// LoadState copia los mapas recibidos (mismo criterio defensivo que
// Plans()/ActiveTasks(): el adapter nunca retiene una referencia al mapa
// del caller, para que una mutación externa posterior al mapa pasado no
// se filtre al estado interno por fuera de a.mu).
func (a *GoalPlanningAdapter) LoadState(plans map[string]*planning.Plan, activeTasks map[string]*planning.Task) {
	a.mu.Lock()
	defer a.mu.Unlock()

	newPlans := make(map[string]*planning.Plan, len(plans))
	for goalID, plan := range plans {
		newPlans[goalID] = plan
	}
	newActiveTasks := make(map[string]*planning.Task, len(activeTasks))
	for reqID, task := range activeTasks {
		newActiveTasks[reqID] = task
	}

	a.plans = newPlans
	a.activeTasks = newActiveTasks
}

// Replanner devuelve el Replanner (B87) que este adapter sostiene
// internamente — construido por NewGoalPlanningAdapter (con
// DefaultReplanLimits si el caller no pasó uno propio) y, hasta este
// bloque, sin ninguna forma de alcanzarlo desde afuera. B90.5 lo expone
// para que core/autonomy_persistence.go pueda leer/cargar
// Regenerations() sin que Replanner deje de ser un campo privado del
// adapter — mismo criterio de "Engine -> GoalPlanningAdapter -> Replanner"
// que ya se acordó en el diseño de B90 en vez de dispersar la propiedad.
func (a *GoalPlanningAdapter) Replanner() *Replanner {
	return a.replanner
}
