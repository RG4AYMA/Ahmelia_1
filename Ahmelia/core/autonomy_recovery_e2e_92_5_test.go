package core

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// ============================================================================
// AUTONOMY_RECOVERY_E2E_92_5_TEST.GO — Bloque 92.5 (cierre de B92 — Diagnoser
// + Recovery)
// ============================================================================
//
// Este archivo no agrega ninguna pieza de producción: solo ejercita, con un
// autonomy.Cycle REAL (no mocks del ciclo en sí, ver b925*Runtime más
// abajo), la cadena completa que B92 construyó a lo largo de B92.1-B92.4:
//
//	Goal -> Plan -> Ejecución -> Fallo -> FailureAdvisor (B92.4) ->
//	Recovery/Replanner (B92.3+B87) -> Nuevo Plan -> Goal Planner ->
//	Ejecución -> Verificación -> Persistencia (B90.7)
//
// y además reinicio (B90.6) sobre el estado ya replanteado, y continuación
// del ciclo autónomo después de eso — cerrando así, con evidencia real y no
// solo con el argumento de diseño, los ocho puntos que quedaron pendientes
// al cerrar B92.4.

// --- Infraestructura mínima compartida por los tests de este archivo ---

// b925ScriptedExecutor implementa autonomy.ExecutorAdapter con una cola de
// respuestas fijada de antemano, consumida en orden FIFO real —no por un
// índice acumulado— para que un test pueda agregarle respuestas a mitad de
// camino (después de que ya se resolvieron llamadas anteriores) sin tener
// que precalcular cuántas llamadas habrá antes de ese punto. Agotada la
// cola, responde éxito por defecto: un test solo necesita fijar los fallos
// que le interesan, no cada paso.
type b925ScriptedExecutor struct {
	mu     sync.Mutex
	script []autonomy.ActionResponse
	calls  int
}

func (e *b925ScriptedExecutor) ExecuteAction(_ context.Context, req autonomy.ActionRequest) autonomy.ActionResponse {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if len(e.script) > 0 {
		resp := e.script[0]
		e.script = e.script[1:]
		resp.RequestID = req.ID
		return resp
	}
	return autonomy.ActionResponse{RequestID: req.ID, Executed: true, Output: "ok"}
}

func (e *b925ScriptedExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

// b925NoopMapper mapea cualquier Task a una ActionRequest trivial —
// GoalActionMapper (B89, CapabilityResolver) es un bloque aparte, no algo
// que B92.5 necesite ejercitar de verdad.
type b925NoopMapper struct{}

func (b925NoopMapper) MapTask(_ context.Context, _ *planning.Goal, task *planning.Task) (autonomy.ActionRequest, error) {
	return autonomy.ActionRequest{ID: task.ID, Action: "noop"}, nil
}

// b925PersistingAdapter reproduce, sin necesitar un *core.Engine completo,
// el mismo criterio que persistingCycleAdapter ya aplica en producción
// (B90.7, autonomy_persistence_lifecycle.go): persistir a disco después de
// cada GetNextAction/UpdateState real. Es el nodo "Persistencia" del
// diagrama de B92.5.
type b925PersistingAdapter struct {
	planner *planning.Planner
	state   *autonomy.AutonomousState
	adapter *autonomy.GoalPlanningAdapter
	path    string
}

func (p *b925PersistingAdapter) GetNextAction(ctx context.Context) (autonomy.ActionRequest, bool, error) {
	req, ok, err := p.adapter.GetNextAction(ctx)
	if err == nil && ok {
		p.persist()
	}
	return req, ok, err
}

func (p *b925PersistingAdapter) UpdateState(ctx context.Context, req autonomy.ActionRequest, verif autonomy.VerificationResult) error {
	err := p.adapter.UpdateState(ctx, req, verif)
	if err == nil {
		p.persist()
	}
	return err
}

func (p *b925PersistingAdapter) persist() {
	snap := buildAutonomyPersistenceSnapshot(p.planner, p.state, p.adapter)
	_ = snap.Save(p.path) // best-effort, igual que Engine.persistAutonomyStateBestEffort (B90.7)
}

// b925Runtime agrupa todo lo que arma un runtime autónomo completo para
// estos tests: Planner+AutonomousState+GoalPlanningAdapter reales, el
// FailureAdvisor de B92.4, el executor guionado, el envoltorio de
// persistencia, y un autonomy.Cycle real conectando todo — el mismo tipo
// de cableado que buildAutonomousExecutionCycle (B88, autonomy_b88.go)
// hace en producción, reducido a lo que un test necesita sin un *Engine.
type b925Runtime struct {
	planner  *planning.Planner
	state    *autonomy.AutonomousState
	adapter  *autonomy.GoalPlanningAdapter
	advisor  *DiagnosingRecoveryAdvisor
	executor *b925ScriptedExecutor
	cycle    *autonomy.Cycle
	path     string
}

func newB925Runtime(t *testing.T, statePath string) *b925Runtime {
	t.Helper()
	planner := planning.NewPlanner()
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)

	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, b925NoopMapper{}, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}

	advisor := NewDiagnosingRecoveryAdvisor()
	adapter.SetFailureAdvisor(advisor)

	persisting := &b925PersistingAdapter{planner: planner, state: state, adapter: adapter, path: statePath}
	executor := &b925ScriptedExecutor{}

	cycle := autonomy.NewCycle(
		state,
		nil, // observer — no hace falta observación real para estos tests
		nil, // evaluator
		persisting,
		executor,
		nil, // verifier -> default autonomy.NewActionVerifier()
		advisor,
		nil, // learner
		persisting,
	)

	return &b925Runtime{
		planner: planner, state: state, adapter: adapter,
		advisor: advisor, executor: executor, cycle: cycle, path: statePath,
	}
}

// newB925Goal registra un Goal con descomposición genérica (4 tareas
// encadenadas: Análisis -> Diseño -> Implementación -> Testing, cada una
// dependiente de la anterior — ver planning.Planner.decomposeGoal, rama
// default) como Goal actual del runtime. Usar un Name fuera del switch de
// decomposeGoal es intencional: así el escenario no depende de que ese
// switch nunca cambie.
func newB925Goal(name string, rt *b925Runtime) *planning.Goal {
	goal := planning.NewGoal(name, "escenario E2E de B92.5", planning.PriorityHigh, "ahmelia")
	rt.planner.RegisterGoal(goal)
	rt.state.SetCurrentGoal(goal)
	return goal
}

// --- Test 1: la cadena completa, con un tramo recuperable que termina en
//     replanificación real, corriendo sobre un autonomy.Cycle real. ---

// TestB925_E2E_RecoverableRetryThenRegenerate_ReplacesPlanWithoutDuplicating
// recorre el primer tramo del diagrama de B92.5:
//
//	Goal -> Plan -> Ejecución -> Fallo (recuperable) -> FailureAdvisor
//	(sin opinión: Replanner decide solo) -> retry -> Fallo de nuevo ->
//	Replanner regenera el plan -> Nuevo Plan -> Goal Planner -> Ejecución
//	-> Verificación (éxito) -> Persistencia
//
// y confirma, con el runtime real (no con el argumento de diseño de
// B92.4), tres de los puntos pedidos para el cierre de B92:
//   - el Goal conserva su identidad (mismo *planning.Goal, mismo ID) en
//     todo el tramo, pese a que el Plan y las Tasks cambian por completo;
//   - no aparece un plan duplicado: adapter.Plans() tiene, en todo
//     momento, como máximo UNA entrada para este Goal.ID, y tras la
//     regeneración es un *planning.Plan distinto (Plan.ID nuevo) del
//     original — el replanning reemplaza, no acumula;
//   - un fallo recuperable efectivamente continúa: el Goal nunca se
//     bloquea en este tramo y termina progresando sobre el plan nuevo.
func TestB925_E2E_RecoverableRetryThenRegenerate_ReplacesPlanWithoutDuplicating(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "autonomy_state.json")
	rt := newB925Runtime(t, statePath)
	goal := newB925Goal("B92.5 recuperable", rt)

	// Guion: los dos primeros intentos de la primera tarea fallan de forma
	// recuperable ("la acción no fue ejecutada por el motor" — ver
	// core/autonomy_diagnoser.go, categoría executor_error/timeout,
	// Recoverable=true). Con MaxTaskRetries=2 (límite por defecto de
	// Replanner, ver autonomy.DefaultReplanLimits), el primer fallo
	// reintenta y el segundo, al no quedar reintentos y existir una tarea
	// dependiente pendiente, dispara ReplanRegeneratePlan. De ahí en
	// adelante (script agotado) el executor responde éxito por defecto.
	rt.executor.script = []autonomy.ActionResponse{
		{Executed: false},
		{Executed: false},
	}

	originalPlan, err := rt.adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan inicial: %v", err)
	}
	originalTaskIDs := make(map[string]bool, len(originalPlan.Tasks))
	for _, tsk := range originalPlan.Tasks {
		originalTaskIDs[tsk.ID] = true
	}

	// Iteración 1: falla, recuperable, Replanner reintenta.
	if err := rt.cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep #1: %v", err)
	}
	if goal.Status == planning.GoalBlocked {
		t.Fatal("un fallo recuperable no debe bloquear el Goal (iteración 1)")
	}
	if len(rt.adapter.Plans()) != 1 {
		t.Fatalf("adapter.Plans() = %d entradas tras un retry, esperaba 1 (sin duplicar)", len(rt.adapter.Plans()))
	}
	if rt.adapter.Plans()[goal.ID].ID != originalPlan.ID {
		t.Fatal("un simple retry no debería haber reemplazado el plan todavía")
	}

	// Iteración 2: segundo fallo de la MISMA tarea -> Replanner regenera.
	if err := rt.cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep #2 (regenera plan): %v", err)
	}
	if goal.Status == planning.GoalBlocked {
		t.Fatal("una regeneración de plan no es un bloqueo: el Goal debe seguir activo")
	}
	if rt.adapter.Replanner().Regenerations()[goal.ID] != 1 {
		t.Fatalf("Replanner().Regenerations()[goal.ID] = %d, esperaba 1", rt.adapter.Replanner().Regenerations()[goal.ID])
	}

	// Iteración 3: GetNextAction genera el plan NUEVO (el viejo fue
	// descartado en la iteración 2) y su primera tarea se ejecuta con
	// éxito (script agotado -> éxito por defecto).
	if err := rt.cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep #3 (primera tarea del plan nuevo): %v", err)
	}

	newPlan, ok := rt.adapter.Plans()[goal.ID]
	if !ok {
		t.Fatal("después de regenerar, el Goal debe volver a tener un plan cacheado")
	}

	// --- Identidad del Goal preservada ---
	if rt.state.CurrentGoal() != goal {
		t.Fatal("el Goal actual dejó de ser el mismo *planning.Goal: se perdió identidad")
	}

	// --- Sin duplicados: una sola entrada viva para este Goal, y es un
	//     plan REALMENTE distinto del original (no el mismo reciclado). ---
	if len(rt.adapter.Plans()) != 1 {
		t.Fatalf("adapter.Plans() = %d entradas tras regenerar, esperaba exactamente 1", len(rt.adapter.Plans()))
	}
	if newPlan.ID == originalPlan.ID {
		t.Fatal("el replanning no reemplazó nada: sigue siendo el mismo Plan.ID original")
	}
	for _, tsk := range newPlan.Tasks {
		if originalTaskIDs[tsk.ID] {
			t.Fatalf("la Task %q del plan nuevo reutiliza un ID del plan original: el replanning debía generar Tasks nuevas", tsk.ID)
		}
	}

	// --- El fallo recuperable efectivamente continuó: al menos una Task
	//     del plan nuevo ya avanzó. ---
	progressed := false
	for _, tsk := range newPlan.Tasks {
		if tsk.IsCompleted() {
			progressed = true
			break
		}
	}
	if !progressed {
		t.Fatal("tras la regeneración, el ciclo debía poder completar al menos una Task del plan nuevo")
	}

	// --- Hallazgo conocido, no una regresión de B92.5: planning.Planner
	//     no tiene forma de dar de baja Tasks huérfanas (ver
	//     planning/planner.go, RegisterTask es solo-agregar). Las 4 Tasks
	//     del plan original, que ensurePlanLocked ya había registrado en
	//     el Planner ANTES de que la regeneración las abandonara (ver
	//     autonomy/goal_planner.go, ensurePlanLocked), siguen viviendo en
	//     planner.Tasks() para siempre, en estado TaskBlocked, sin que
	//     ningún Plan actual las referencie. buildAutonomyPersistenceSnapshot
	//     (B90.7) las persiste igual, porque lee Planner.Tasks() completo.
	//     Esto es una limitación preexistente de B87 (Replanner ya
	//     regeneraba planes antes de que existiera B92), no algo que este
	//     bloque introduzca ni deba resolver — se deja documentado acá
	//     porque es la primera vez que un test la ejercita de punta a
	//     punta. No es lo mismo que "un plan duplicado": adapter.Plans()
	//     (la fuente de verdad de qué plan está VIVO para cada Goal, ver
	//     GoalPlanningAdapter.Plans()) nunca tuvo más de una entrada para
	//     este Goal en ningún momento del test.
	orphaned := 0
	for _, tsk := range rt.planner.Tasks() {
		if originalTaskIDs[tsk.ID] {
			orphaned++
		}
	}
	if orphaned != len(originalTaskIDs) {
		t.Fatalf("se esperaban %d Tasks huérfanas del plan original todavía en planner.Tasks() (hallazgo documentado arriba), se encontraron %d — si esto cambió, puede ser que Planner haya ganado limpieza real, actualizar este comentario", len(originalTaskIDs), orphaned)
	}
}

// --- Test 2: fallo no recuperable, cierre correcto, sin ejecución
//     absurda, persistencia y reinicio sobre el estado ya replanteado. ---

// TestB925_E2E_NonRecoverableFailure_EndsCorrectlyAndSurvivesRestart cubre
// el segundo tramo pedido para el cierre de B92:
//   - un fallo no recuperable (permiso denegado) hace que el
//     FailureAdvisor (B92.4) venza a Replanner incluso con reintentos
//     disponibles, y el Goal termina Blocked de inmediato — "un fallo no
//     recuperable termina correctamente";
//   - una vez blocked (el "NeedsUser" del pedido: nada en este runtime
//     distingue blocked-por-diagnóstico de blocked-por-otra-causa, ambos
//     usan el mismo GoalBlocked ya existente — ver StrategyAskUser en
//     translateFailureStrategyToReplanAction, core/autonomy_recovery_replanner_integration.go),
//     el ciclo NO vuelve a pedirle una acción a ese Goal: GetNextAction
//     devuelve ok=false y el executor no se vuelve a llamar — "no genera
//     ejecución absurda";
//   - el estado (Goal Blocked, Plan con la Task fallida, progreso
//     parcial) persiste a disco y sobrevive un reinicio completo
//     (planner/state/adapter nuevos, sin memoria compartida con el
//     runtime original) con la identidad del Goal preservada por ID;
//   - sobre el runtime restaurado, un Cycle NUEVO puede seguir corriendo
//     sin fallar ni reactivar por accidente el Goal bloqueado — "el ciclo
//     autónomo puede continuar después del replanning".
func TestB925_E2E_NonRecoverableFailure_EndsCorrectlyAndSurvivesRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "autonomy_state.json")
	rt := newB925Runtime(t, statePath)
	goal := newB925Goal("B92.5 no recuperable", rt)
	originalGoalID := goal.ID

	// Primer intento: éxito (script vacío -> éxito por defecto), para que
	// el Goal muestre progreso parcial real antes del bloqueo — así el
	// test de reinicio también confirma que ese progreso sobrevive.
	if err := rt.cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep #1 (primera tarea, éxito): %v", err)
	}

	// Segundo intento: falla con "permiso denegado" -> Diagnoser: categoría
	// permission, Recoverable=false, Suggested=StrategyAskUser (ver
	// core/autonomy_diagnoser.go) -> FailureAdvisor vence a Replanner
	// (Attempts=1 < MaxTaskRetries=2: sin el advisor, Replanner hubiera
	// reintentado).
	rt.executor.script = append(rt.executor.script, autonomy.ActionResponse{
		Executed: true, Error: errors.New("permiso denegado"),
	})
	if err := rt.cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep #2 (permiso denegado): %v", err)
	}

	if goal.Status != planning.GoalBlocked {
		t.Fatalf("goal.Status = %q, esperaba %q: un permiso denegado no debe reintentarse pese a tener reintentos disponibles", goal.Status, planning.GoalBlocked)
	}

	callsBeforeIdleCheck := rt.executor.callCount()

	// "NeedsUser no genera ejecución absurda": con el Goal blocked, varias
	// vueltas más de RunStep no deben volver a tocar el executor.
	for i := 0; i < 3; i++ {
		if err := rt.cycle.RunStep(context.Background()); err != nil {
			t.Fatalf("RunStep sobre Goal blocked (vuelta %d): %v", i, err)
		}
	}
	if got := rt.executor.callCount(); got != callsBeforeIdleCheck {
		t.Fatalf("el executor se llamó %d veces más sobre un Goal blocked: no debía ejecutarse nada", got-callsBeforeIdleCheck)
	}
	if req, hasAction, err := rt.adapter.GetNextAction(context.Background()); err != nil || hasAction {
		t.Fatalf("GetNextAction sobre Goal blocked: req=%+v hasAction=%v err=%v, esperaba hasAction=false", req, hasAction, err)
	}

	// --- Persistencia + reinicio completo ---
	restoredPlanner := planning.NewPlanner()
	restoredState := autonomy.New()
	restoredMonitor := autonomy.NewGoalMonitor(restoredState, 0)
	restoredAdapter, err := autonomy.NewGoalPlanningAdapter(restoredState, restoredMonitor, restoredPlanner, b925NoopMapper{}, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter (restaurado): %v", err)
	}

	snap, err := LoadAutonomyPersistenceSnapshot(statePath)
	if err != nil {
		t.Fatalf("LoadAutonomyPersistenceSnapshot: %v", err)
	}
	if err := ApplyAutonomyPersistenceSnapshot(snap, restoredPlanner, restoredState, restoredAdapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot: %v", err)
	}

	restoredGoal := restoredState.CurrentGoal()
	if restoredGoal == nil {
		t.Fatal("tras el reinicio, CurrentGoal quedó nil")
	}
	if restoredGoal.ID != originalGoalID {
		t.Fatalf("restoredGoal.ID = %q, esperaba %q: el Goal debe conservar su identidad por ID a través del reinicio", restoredGoal.ID, originalGoalID)
	}
	if restoredGoal.Status != planning.GoalBlocked {
		t.Fatalf("restoredGoal.Status = %q, esperaba %q: el estado ya replanteado (bloqueado) debe sobrevivir el reinicio", restoredGoal.Status, planning.GoalBlocked)
	}

	restoredPlan, ok := restoredAdapter.Plans()[restoredGoal.ID]
	if !ok {
		t.Fatal("tras el reinicio, el Goal restaurado debía tener un plan cacheado (el que ya estaba replanteado)")
	}
	completedAfterRestart := 0
	for _, tsk := range restoredPlan.Tasks {
		if tsk.IsCompleted() {
			completedAfterRestart++
		}
	}
	if completedAfterRestart != 1 {
		t.Fatalf("Tasks completadas tras el reinicio = %d, esperaba 1 (el progreso previo al bloqueo debía sobrevivir)", completedAfterRestart)
	}

	// --- El ciclo autónomo puede seguir corriendo después del reinicio,
	//     sin reactivar por accidente el Goal bloqueado. ---
	restoredAdvisor := NewDiagnosingRecoveryAdvisor()
	restoredAdapter.SetFailureAdvisor(restoredAdvisor)
	restoredExecutor := &b925ScriptedExecutor{}
	restoredPersisting := &b925PersistingAdapter{planner: restoredPlanner, state: restoredState, adapter: restoredAdapter, path: statePath}
	restoredCycle := autonomy.NewCycle(
		restoredState, nil, nil, restoredPersisting, restoredExecutor, nil, restoredAdvisor, nil, restoredPersisting,
	)

	if err := restoredCycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep sobre el runtime restaurado: %v", err)
	}
	if restoredExecutor.callCount() != 0 {
		t.Fatal("el runtime restaurado no debía ejecutar nada: el único Goal que tiene sigue blocked")
	}
	if restoredState.Execution() != autonomy.ExecIdle {
		t.Fatalf("el ciclo restaurado no cerró en Idle: quedó en %s", restoredState.Execution())
	}
}

// --- Test 3: continuación positiva tras un reinicio, con el ciclo
//     completando el resto de un Goal cuyo plan ya había sido
//     regenerado antes de reiniciar. ---

// TestB925_E2E_ContinuesAfterRestart_OnAlreadyRegeneratedPlan es la mitad
// positiva de "el ciclo autónomo puede continuar después del replanning":
// a diferencia del test anterior (que confirma que un Goal blocked NO
// vuelve a ejecutarse), este confirma que un Goal SANO, cuyo plan ya fue
// regenerado antes del reinicio, sigue avanzando con normalidad sobre un
// Cycle nuevo, construido desde cero sobre el runtime restaurado.
func TestB925_E2E_ContinuesAfterRestart_OnAlreadyRegeneratedPlan(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "autonomy_state.json")
	rt := newB925Runtime(t, statePath)
	goal := newB925Goal("B92.5 continuidad", rt)

	// Mismo guion de regeneración que el primer test: dos fallos
	// recuperables de la primera tarea fuerzan ReplanRegeneratePlan.
	rt.executor.script = []autonomy.ActionResponse{
		{Executed: false},
		{Executed: false},
	}
	for i := 0; i < 2; i++ {
		if err := rt.cycle.RunStep(context.Background()); err != nil {
			t.Fatalf("RunStep #%d (forzando regeneración): %v", i+1, err)
		}
	}
	if rt.adapter.Replanner().Regenerations()[goal.ID] != 1 {
		t.Fatalf("Replanner().Regenerations()[goal.ID] = %d, esperaba 1 antes de reiniciar", rt.adapter.Replanner().Regenerations()[goal.ID])
	}

	// La regeneración (ReplanRegeneratePlan, autonomy/goal_planner.go)
	// descarta el plan cacheado y no genera uno nuevo hasta el próximo
	// GetNextAction/EnsurePlan — inmediatamente después de las dos
	// llamadas de arriba, adapter.Plans() todavía NO tiene entrada para
	// este Goal. Materializamos el plan nuevo con EnsurePlan (sin pasar
	// por RunStep, que además lo ejecutaría) para poder guardar el
	// snapshot en el punto exacto que este test quiere reiniciar:
	// "recién replanteado, plan nuevo cacheado, todavía sin ejecutar
	// nada de él".
	if _, err := rt.adapter.EnsurePlan(goal); err != nil {
		t.Fatalf("EnsurePlan tras regenerar: %v", err)
	}

	// Persistimos explícitamente el estado justo después de regenerar,
	// antes de que el ciclo tenga oportunidad de avanzar el plan nuevo —
	// el escenario que queremos reiniciar es "recién replanteado, todavía
	// sin ejecutar nada del plan nuevo".
	snap := buildAutonomyPersistenceSnapshot(rt.planner, rt.state, rt.adapter)
	if err := snap.Save(statePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	restoredPlanner := planning.NewPlanner()
	restoredState := autonomy.New()
	restoredMonitor := autonomy.NewGoalMonitor(restoredState, 0)
	restoredAdapter, err := autonomy.NewGoalPlanningAdapter(restoredState, restoredMonitor, restoredPlanner, b925NoopMapper{}, nil, nil)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter (restaurado): %v", err)
	}
	loaded, err := LoadAutonomyPersistenceSnapshot(statePath)
	if err != nil {
		t.Fatalf("LoadAutonomyPersistenceSnapshot: %v", err)
	}
	if err := ApplyAutonomyPersistenceSnapshot(loaded, restoredPlanner, restoredState, restoredAdapter); err != nil {
		t.Fatalf("ApplyAutonomyPersistenceSnapshot: %v", err)
	}

	restoredGoal := restoredState.CurrentGoal()
	if restoredGoal == nil || restoredGoal.ID != goal.ID {
		t.Fatalf("CurrentGoal restaurado = %+v, esperaba el mismo ID %q", restoredGoal, goal.ID)
	}
	restoredPlan := restoredAdapter.Plans()[restoredGoal.ID]
	if restoredPlan == nil || len(restoredPlan.Tasks) != 4 {
		t.Fatalf("el plan restaurado debía ser el plan REGENERADO completo (4 Tasks), got %+v", restoredPlan)
	}
	for _, tsk := range restoredPlan.Tasks {
		if tsk.IsCompleted() {
			t.Fatal("el plan regenerado no había avanzado todavía antes de guardar el snapshot")
		}
	}

	// Cycle nuevo sobre el runtime restaurado, advisor y executor nuevos
	// (sin script: siempre éxito) — el ciclo autónomo debe poder terminar
	// las 4 Tasks del plan ya regenerado sin ningún fallo adicional.
	restoredAdvisor := NewDiagnosingRecoveryAdvisor()
	restoredAdapter.SetFailureAdvisor(restoredAdvisor)
	restoredExecutor := &b925ScriptedExecutor{}
	restoredPersisting := &b925PersistingAdapter{planner: restoredPlanner, state: restoredState, adapter: restoredAdapter, path: statePath}
	restoredCycle := autonomy.NewCycle(
		restoredState, nil, nil, restoredPersisting, restoredExecutor, nil, restoredAdvisor, nil, restoredPersisting,
	)

	for i := 0; i < len(restoredPlan.Tasks); i++ {
		if err := restoredCycle.RunStep(context.Background()); err != nil {
			t.Fatalf("RunStep post-reinicio #%d: %v", i+1, err)
		}
	}

	if restoredGoal.Status != planning.GoalCompleted {
		t.Fatalf("restoredGoal.Status = %q, esperaba %q: el ciclo debía poder completar el resto del plan ya regenerado tras el reinicio", restoredGoal.Status, planning.GoalCompleted)
	}
	for _, tsk := range restoredPlan.Tasks {
		if !tsk.IsCompleted() {
			t.Fatalf("Task %q (%s) quedó sin completar tras la continuación", tsk.ID, tsk.Title)
		}
	}
}
