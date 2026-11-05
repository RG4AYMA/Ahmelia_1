package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// ============================================================================
// AUTONOMY_RACE_E2E_93_4_TEST.GO — Bloque 93.4
// ============================================================================
//
// Este es el test estrella del cierre de la concurrencia:
//
//     Engine.Process()
//          |
//          +---- usuario interactúa
//          |
//     AutonomousLoop
//          |
//          +---- environment observation
//          +---- Goal -> Plan -> Task -> Execute
//          +---- Failure -> Diagnose -> Replan -> Execute
//          |
//          +---- shared AutonomousState / Planner / EventStore / LearningStore
//
// El objetivo no es probar cada componente por separado (eso ya vive en
// B91-B93.3), sino comprobar que DOS caminos reales del mismo Engine pueden
// estar vivos a la vez sin pisarse. La prueba debe ejecutarse con -race.
//
// Criterios de aceptación:
//   - 0 data races (go test -race)
//   - 0 deadlocks (timeouts explícitos)
//   - 0 Goals duplicados
//   - 0 Plans duplicados en el adapter (uno vivo por Goal.ID)
//   - progreso del Goal consistente con el Plan vivo
//   - estado de ejecución vuelve a IDLE
//   - la cadena Failure -> Diagnose -> Replan -> Execute ocurre realmente
//   - Process() puede entrar mientras el tick autónomo está ejecutándose
//
// B93.4 no usa un mock del Engine: buildTestEngine() construye el Engine real.
// Solo se inyecta un mapper/executor de prueba para poder hacer determinista
// la rama de Recovery/Replanning sin tocar el mundo real durante el test.

// b934EnvironmentObserver hace observable que Cycle.RunStep pasó por la fase
// ENVIRONMENT OBSERVATION. No tiene estado mutable compartido con el Engine;
// el contador atómico solo sirve como evidencia del E2E.
type b934EnvironmentObserver struct {
	observations atomic.Int64
}

func (o *b934EnvironmentObserver) Observe(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	o.observations.Add(1)
	return "entorno de prueba observado", nil
}

// b934ScriptedExecutor produce exactamente el escenario estrella:
//  1. fallo recuperable
//  2. segundo fallo recuperable -> Replanner regenera
//  3. éxito sobre el plan nuevo
//
// La primera ejecución queda bloqueada a propósito. Eso crea una ventana
// determinista en la que el test lanza Engine.Process() desde otra goroutine.
// Process() no puede mutar el mismo estado hasta que el tick autónomo termine,
// porque ambos respetan Engine.autonomyRuntimeMu. Si B93.4 se quitara, esta
// misma ventana sería el lugar ideal para que -race encontrara el conflicto.
type b934ScriptedExecutor struct {
	mu sync.Mutex

	script []autonomy.ActionResponse
	calls  int

	firstStarted chan struct{}
	releaseFirst chan struct{}
	startOnce    sync.Once
	onThird      func()
}

func newB934ScriptedExecutor() *b934ScriptedExecutor {
	return &b934ScriptedExecutor{
		script: []autonomy.ActionResponse{
			{Executed: false, Error: nil},
			{Executed: false, Error: nil},
			{Executed: true, Output: "ok"},
		},
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
}

func (e *b934ScriptedExecutor) ExecuteAction(ctx context.Context, req autonomy.ActionRequest) autonomy.ActionResponse {
	e.mu.Lock()
	e.calls++
	call := e.calls
	var response autonomy.ActionResponse
	if len(e.script) > 0 {
		response = e.script[0]
		e.script = e.script[1:]
	} else {
		response = autonomy.ActionResponse{Executed: true, Output: "ok"}
	}
	e.mu.Unlock()

	if call == 1 {
		e.startOnce.Do(func() { close(e.firstStarted) })
		select {
		case <-e.releaseFirst:
		case <-ctx.Done():
			response.Executed = false
			response.Error = ctx.Err()
		}
	}

	response.RequestID = req.ID
	if call == 3 && response.Executed && e.onThird != nil {
		e.onThird()
	}
	return response
}

func (e *b934ScriptedExecutor) release() {
	select {
	case <-e.releaseFirst:
	default:
		close(e.releaseFirst)
	}
}

func (e *b934ScriptedExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func waitB934(t *testing.T, timeout time.Duration, condition func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout esperando %s", what)
}

func TestB934_RaceE2E_ProcessAutonomousRecoveryReplanSharedState(t *testing.T) {
	engine := buildTestEngine(t)
	engine.onResponse = func(string) {}

	// Este E2E valida concurrencia + Recovery/Replanning, no el adaptador
	// lingüístico. Process("que aprendiste") tiene una ruta determinista en
	// Memory Intelligence, pero la frontera B96.22 puede consultar
	// Phi-4-mini cuando considera que el turno necesita profundidad
	// lingüística. Si dejamos ese proveedor real conectado, el test pasa a
	// depender de un servidor Ollama externo y una inferencia de duración
	// impredecible; peor aún, como Process() comparte el ownership lock con
	// el tick autónomo, una inferencia lenta puede impedir que Recovery llegue
	// a la tercera ejecución. El E2E debe aislar esa dependencia externa.
	engine.linguisticInterpreter = nil

	// El adapter de producción usa CapabilityResolver, cuyo objetivo es
	// resolver capacidades reales. Para este E2E queremos aislar la
	// concurrencia y Recovery/Replanning, no volver a probar B89. El mapper
	// trivial mantiene Goal/Plan/Task/ActiveTask completamente reales.
	adapter, err := autonomy.NewGoalPlanningAdapter(
		engine.autonomousState,
		engine.goalMonitor,
		engine.planner,
		b925NoopMapper{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewGoalPlanningAdapter: %v", err)
	}
	advisor := NewDiagnosingRecoveryAdvisor()
	adapter.SetFailureAdvisor(advisor)
	engine.goalPlanningAdapter = adapter

	goal := planning.NewGoal(
		"B93.4 Race E2E",
		"Goal compartido entre interacción y autonomía",
		planning.PriorityHigh,
		"ahmelia",
	)
	engine.planner.RegisterGoal(goal)
	engine.autonomousState.SetCurrentGoal(goal)

	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatalf("EnsurePlan: %v", err)
	}
	if len(plan.Tasks) < 2 {
		t.Fatalf("el escenario necesita al menos 2 Tasks: %d", len(plan.Tasks))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executor := newB934ScriptedExecutor()
	thirdDone := make(chan struct{})
	executor.onThird = func() { close(thirdDone) }
	environment := &b934EnvironmentObserver{}
	learner := autonomy.NewLearningMemoryAdapter(engine.eventStore, engine.learningState, "b93.4-test")
	updater := autonomy.NewCompositeCycleUpdater(adapter, learner)

	cycle := autonomy.NewCycle(
		engine.autonomousState,
		environment,
		nil,
		adapter,
		executor,
		nil,
		advisor,
		learner,
		updater,
	)

	loop := autonomy.NewAutonomousExecutionLoop(
		autonomy.LoopConfig{Interval: 100 * time.Millisecond, BufferSize: 4},
		cycle,
	)
	loop.SetRunLock(&engine.autonomyRuntimeMu)

	loop.Start(ctx)

	// Esperamos la primera ejecución. El executor la mantiene bloqueada, por
	// lo que el tick autónomo sostiene el ownership lock de Engine mientras
	// Process() intenta entrar desde otra goroutine.
	select {
	case <-executor.firstStarted:
	case <-time.After(2 * time.Second):
		loop.Stop()
		t.Fatal("el AutonomousLoop no llegó a ExecuteAction #1")
	}

	userDone := make(chan struct{})
	go func() {
		// Esta pregunta recorre el camino real de Process() hasta
		// Memory Intelligence/LearningStore. Por eso no solo escribe el
		// estado conversacional: también comparte EventStore/LearningStore
		// con el ciclo autónomo.
		engine.Process("que aprendiste")
		close(userDone)
	}()

	// Confirmamos que la goroutine del usuario tuvo tiempo de quedar en
	// competencia por el mismo Engine mientras el tick sigue reteniendo el
	// lock. No esperamos su finalización todavía: eso ocurriría después de
	// liberar el primer ExecuteAction.
	time.Sleep(10 * time.Millisecond)
	executor.release()

	// La cadena completa debe llegar a dos fallos + un éxito. Si el loop se
	// queda bloqueado, el timeout convierte el posible deadlock en un fallo
	// explícito del test en vez de colgar la suite.
	select {
	case <-thirdDone:
	case <-time.After(5 * time.Second):
		loop.Stop()
		t.Fatalf("timeout esperando la tercera ejecución del escenario de Recovery")
	}

	// El callback de la tercera ejecución ocurre dentro de ExecuteAction,
	// antes de que Cycle marque/actualice el progreso. Stop() espera al tick
	// completo, así que no puede dejar una cuarta ejecución intercalada.
	loop.Stop()

	select {
	case <-userDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Engine.Process() quedó bloqueado durante el E2E concurrente")
	}

	// ---------------------------------------------------------------------
	// Evidencia del camino completo
	// ---------------------------------------------------------------------
	if environment.observations.Load() == 0 {
		t.Fatal("el Cycle no realizó ninguna observación del entorno")
	}
	if executor.callCount() != 3 {
		t.Fatalf("ExecuteAction() = %d llamadas, esperaba exactamente 3 (fallo, fallo, éxito)", executor.callCount())
	}

	regenerations := adapter.Replanner().Regenerations()[goal.ID]
	if regenerations != 1 {
		t.Fatalf("regeneraciones[%q] = %d, esperaba 1", goal.ID, regenerations)
	}

	currentPlans := adapter.Plans()
	if len(currentPlans) != 1 {
		t.Fatalf("Plans() = %d entradas, esperaba exactamente 1 Plan vivo para el Goal", len(currentPlans))
	}
	currentPlan, ok := currentPlans[goal.ID]
	if !ok || currentPlan == nil {
		t.Fatal("el Goal perdió su Plan vivo después del replanning")
	}
	if currentPlan.ID == plan.ID {
		t.Fatalf("el replanning no reemplazó el Plan: siguió usando %q", plan.ID)
	}

	goals := engine.planner.Goals()
	matches := 0
	for _, candidate := range goals {
		if candidate != nil && candidate.ID == goal.ID {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("Goal.ID %q aparece %d veces en Planner, esperaba exactamente 1", goal.ID, matches)
	}

	// El éxito final debe haber completado la primera Task del Plan NUEVO.
	// Las demás siguen pendientes por diseño; por eso el progreso esperado
	// es consistente (no necesariamente 100%).
	completed := 0
	taskIDs := make(map[string]struct{}, len(currentPlan.Tasks))
	for _, task := range currentPlan.Tasks {
		if task == nil {
			continue
		}
		if _, duplicate := taskIDs[task.ID]; duplicate {
			t.Fatalf("Task.ID duplicado dentro del Plan vivo: %q", task.ID)
		}
		taskIDs[task.ID] = struct{}{}
		if task.IsCompleted() {
			completed++
		}
	}

	if goal.TasksTotal != len(currentPlan.Tasks) {
		t.Fatalf("Goal.TasksTotal = %d, Plan.Tasks = %d: progreso desincronizado", goal.TasksTotal, len(currentPlan.Tasks))
	}
	if goal.TasksCompleted != completed {
		t.Fatalf("Goal.TasksCompleted = %d, pero el Plan reporta %d Tasks completadas", goal.TasksCompleted, completed)
	}
	if goal.Progress < 0 || goal.Progress > 1 {
		t.Fatalf("Goal.Progress fuera de rango: %v", goal.Progress)
	}
	if goal.Progress != float64(goal.TasksCompleted)/float64(goal.TasksTotal) {
		t.Fatalf("Goal.Progress = %v no coincide con TasksCompleted/TasksTotal = %d/%d", goal.Progress, goal.TasksCompleted, goal.TasksTotal)
	}

	if state := engine.autonomousState.Execution(); state != autonomy.ExecIdle {
		t.Fatalf("estado autónomo final = %s, esperaba IDLE; posible transición inválida/intercalación", state)
	}
}
