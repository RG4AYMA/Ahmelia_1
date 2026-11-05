package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ahmelia/autonomy"
	"ahmelia/environment"
	"ahmelia/planning"
	"ahmelia/situation"
	"ahmelia/solving"
	"ahmelia/system"
)

// B88 — puentes de integración entre el Engine real y el ciclo autónomo.

// engineEnvironmentSnapshotInspector traduce el World Model real al contrato
// de B85. No mantiene una segunda copia del entorno.
type engineEnvironmentSnapshotInspector struct {
	manager *environment.Manager
}

func (i engineEnvironmentSnapshotInspector) Snapshot(ctx context.Context) (autonomy.EnvironmentSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return autonomy.EnvironmentSnapshot{}, err
	}
	if i.manager == nil {
		return autonomy.EnvironmentSnapshot{}, fmt.Errorf("environment.Manager no configurado")
	}

	snapshot := autonomy.EnvironmentSnapshot{Timestamp: time.Now(), CoverageKnown: true}
	processes, processErr := environment.ScanOSProcesses()
	if processErr != nil {
		snapshot.UnavailableDomains = append(snapshot.UnavailableDomains, "windows", "processes")
	} else {
		snapshot.ObservedDomains = append(snapshot.ObservedDomains, "windows", "processes")
		seenApps := make(map[string]struct{})
		for _, process := range processes {
			identity := fmt.Sprintf("%s#%d", strings.ToLower(process.ProcessName), process.ProcessID)
			snapshot.Processes = append(snapshot.Processes, identity)
			if process.WindowTitle == "" {
				continue
			}
			name := strings.ToLower(process.ProcessName)
			if _, ok := seenApps[name]; !ok {
				snapshot.OpenApps = append(snapshot.OpenApps, name)
				seenApps[name] = struct{}{}
			}
			snapshot.Windows = append(snapshot.Windows, autonomy.WindowObservation{ProcessID: process.ProcessID, ProcessName: process.ProcessName, Title: process.WindowTitle})
		}
	}

	focusedName, focusedPID, focusErr := system.GetForegroundProcess()
	if focusErr != nil {
		snapshot.UnavailableDomains = append(snapshot.UnavailableDomains, "focus")
	} else {
		snapshot.ObservedDomains = append(snapshot.ObservedDomains, "focus")
		snapshot.FocusedApp = strings.ToLower(focusedName)
		for index := range snapshot.Windows {
			snapshot.Windows[index].Focused = snapshot.Windows[index].ProcessID == int(focusedPID)
		}
	}

	resources, resourceErr := environment.GetSystemResources()
	if resourceErr != nil {
		snapshot.UnavailableDomains = append(snapshot.UnavailableDomains, "resources")
	} else {
		snapshot.ObservedDomains = append(snapshot.ObservedDomains, "resources")
		snapshot.CPUUsage = resources.CPUPercent
		snapshot.RAMUsage = resources.RAMPercent
	}
	return snapshot, nil
}

// engineCycleObserver adapta el observer B85 al contrato string de B78.
type engineCycleObserver struct {
	observer *autonomy.EnvironmentObserver
	mu       sync.Mutex
	current  []situation.Observation
}

type engineSituationalCycleEvaluator struct {
	engine *Engine
}

func (e engineSituationalCycleEvaluator) EvaluateSituation(ctx context.Context, evidence situation.DecisionEvidence) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if e.engine == nil {
		return false, nil
	}

	// Create autonomous proposal for FocusArbiter if situation is relevant
	if evidence.Relevant && len(evidence.Findings) > 0 {
		sit := e.engine.lastSituation
		if sit.ID != "" {
			if e.engine.goalPlanningAdapter != nil {
				req := autonomy.ActionRequest{
					ID:       "auto-" + time.Now().UTC().Format("20060102T150405.000000000"),
					Action:   "autonomous_action",
					Objective: "autonomous_goal",
					Target:   "autonomous_target",
					Parameters: map[string]string{},
				}
				autoProp := FromAutonomousPlan(e.engine, sit, e.engine.lastSituationContributions, req, evidence)
				if autoProp != nil {
					e.engine.lastAutonomousProposal = autoProp
					if e.engine.flowTrace != nil {
						e.engine.flowTrace.Mark("AUTONOMOUS", fmt.Sprintf("proposal_stored=%s", autoProp.Intention))
					}
				}
			}
		}
	}

	return evidence.Relevant && len(evidence.Findings) > 0, nil
}

func (o *engineCycleObserver) Observe(ctx context.Context) (string, error) {
	if o == nil || o.observer == nil {
		return "", nil
	}
	observations, err := o.observer.Observe(ctx)
	if err != nil {
		return "", err
	}
	if len(observations) == 0 {
		o.mu.Lock()
		o.current = nil
		o.mu.Unlock()
		return "", nil
	}
	parts := make([]string, 0, len(observations))
	structured := make([]situation.Observation, 0, len(observations))
	for i, observation := range observations {
		parts = append(parts, fmt.Sprintf("%s:%s:%v", observation.Type, observation.Source, observation.Payload))
		payload, marshalErr := json.Marshal(observation.Payload)
		if marshalErr != nil {
			payload = []byte("{}")
		}
		observedAt := observation.Timestamp
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		id := strings.TrimSpace(observation.ID)
		if id == "" {
			id = fmt.Sprintf("environment-%s-%d-%d", observation.Type, observedAt.UnixNano(), i)
		}
		source := strings.TrimSpace(observation.Source)
		if source == "" {
			source = "autonomy.EnvironmentObserver"
		}
		structured = append(structured, situation.Observation{
			ID: id, Kind: "environment_observation", Source: source,
			Subject: "environment", Predicate: string(observation.Type),
			Object: string(observation.Type), Value: string(payload),
			ObservedAt: observedAt, Temporal: situation.TemporalRecent, Confidence: .9, Epistemic: situation.EpistemicObserved,
		})
	}
	o.mu.Lock()
	o.current = structured
	o.mu.Unlock()
	return strings.Join(parts, " | "), nil
}

func (o *engineCycleObserver) SituationObservations() []situation.Observation {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]situation.Observation(nil), o.current...)
}

// CyclePlanningBaseline returns the latest observer-owned snapshot, including
// explicit unavailable-domain markers. It is only a bounded planning input;
// the canonical environment owner and planner state remain unchanged.
func (o *engineCycleObserver) CyclePlanningBaseline() string {
	if o == nil || o.observer == nil {
		return ""
	}
	snapshot := o.observer.LastSnapshot()
	if snapshot == nil {
		return ""
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// engineEnvironmentInspector verifica postcondiciones observables para las
// tres Actions que B79 ejecuta realmente.
type engineEnvironmentInspector struct {
	// Optional function seams keep OS-backed reconciliation testable without
	// pretending the test host is Windows. Production uses the real OS sources.
	processScanner    func() ([]environment.OSProcessObservation, error)
	foregroundProcess func() (string, uint32, error)
}

func (i engineEnvironmentInspector) scanProcesses() ([]environment.OSProcessObservation, error) {
	if i.processScanner != nil {
		return i.processScanner()
	}
	return environment.ScanOSProcesses()
}

func (i engineEnvironmentInspector) foreground() (string, uint32, error) {
	if i.foregroundProcess != nil {
		return i.foregroundProcess()
	}
	return system.GetForegroundProcess()
}

func (i engineEnvironmentInspector) InspectAction(ctx context.Context, req autonomy.ActionRequest) (observation autonomy.EnvironmentObservation, err error) {
	// Stamp every outcome, including insufficient/contradictory evidence, with
	// the request identity and capture time. This prevents later consumers from
	// having to infer provenance from free-form Details.
	defer func() {
		observation.ActionID = req.ID
		observation.Operation = req.Action
		observation.Target = strings.TrimSpace(req.Target)
		observation.Source = "core.engine_environment_inspector"
		observation.ObservedAt = time.Now().UTC()
	}()
	if err := ctx.Err(); err != nil {
		return autonomy.EnvironmentObservation{}, err
	}

	// Some OS-backed postconditions have their own structured observation
	// source. They remain conservative: a missing target is not proof that an
	// asynchronous launch failed, and a focus change is confirmed only by the
	// current foreground-process API, never by WorldState's remembered focus.
	switch req.Action {
	case "cambiar_ventana":
		return i.inspectWindowFocus(ctx, req)
	}

	// Normalize the autonomous request into the canonical postcondition
	// contract owned by solving.Verify. This adapter translates parameter names;
	// it does not maintain a second implementation of filesystem truth.
	params := cloneStringMap(req.Parameters)
	switch req.Action {
	case "crear_carpeta":
		if params["ruta"] == "" {
			params["ruta"] = strings.TrimSpace(req.Target)
		}
	case "mover_archivo":
		if params["origen"] == "" {
			params["origen"] = firstParam(req, "source", "")
		}
		if params["destino"] == "" {
			params["destino"] = firstParam(req, "destination", req.Target)
		}
	case "renombrar_archivo":
		if params["ruta"] == "" {
			params["ruta"] = firstParam(req, "origen", firstParam(req, "source", req.Target))
		}
		if params["nombre_nuevo"] == "" {
			destination := firstParam(req, "destino", params["destination"])
			if destination != "" {
				params["nombre_nuevo"] = filepath.Base(destination)
			}
		}
	default:
		return autonomy.EnvironmentObservation{Details: fmt.Sprintf("no existe un contrato de postcondición ambiental para %q", req.Action), Status: autonomy.ObservationInsufficient}, nil
	}

	verification := solving.Verify(req.Action, params, &solving.ExecutionResult{
		Action: req.Action, Outcome: solving.OutcomeExecuted,
		Detail: "observación de reconciliación posterior a la ejecución",
	}, nil)
	switch verification.Outcome {
	case solving.VerificationConfirmed:
		return autonomy.EnvironmentObservation{Confirmed: true, Details: verification.Detail, Status: autonomy.ObservationConfirmed, Facts: mapPostconditionFacts(verification.Facts)}, nil
	case solving.VerificationNotConfirmed:
		return autonomy.EnvironmentObservation{Details: verification.Detail, Status: autonomy.ObservationContradicted, Facts: mapPostconditionFacts(verification.Facts)}, nil
	case solving.VerificationUnchecked:
		return autonomy.EnvironmentObservation{Details: verification.Detail, Status: autonomy.ObservationInsufficient, Facts: mapPostconditionFacts(verification.Facts)}, nil
	default:
		return autonomy.EnvironmentObservation{Details: verification.Detail, Status: autonomy.ObservationInsufficient, Facts: mapPostconditionFacts(verification.Facts)}, nil
	}
}

func (i engineEnvironmentInspector) InspectForReconciliation(ctx context.Context, req autonomy.ActionRequest) (autonomy.EnvironmentObservation, error) {
	return i.InspectAction(ctx, req)
}

func (i engineEnvironmentInspector) inspectWindowFocus(ctx context.Context, req autonomy.ActionRequest) (autonomy.EnvironmentObservation, error) {
	if err := ctx.Err(); err != nil {
		return autonomy.EnvironmentObservation{}, err
	}
	target := strings.ToLower(strings.TrimSpace(req.Target))
	if target == "" {
		return autonomy.EnvironmentObservation{Details: "Alt+Tab no tiene una ventana objetivo previa para comparar; el foco actual por sí solo no demuestra el cambio", Status: autonomy.ObservationInsufficient}, nil
	}
	target = strings.TrimSuffix(filepath.Base(target), filepath.Ext(filepath.Base(target)))
	processes, err := i.scanProcesses()
	if err != nil {
		return autonomy.EnvironmentObservation{Details: fmt.Sprintf("no se pudo enumerar procesos/ventanas: %v", err), Status: autonomy.ObservationInsufficient}, nil
	}
	foundTarget := false
	for _, process := range processes {
		if strings.EqualFold(strings.TrimSuffix(process.ProcessName, filepath.Ext(process.ProcessName)), target) && strings.TrimSpace(process.WindowTitle) != "" {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		return autonomy.EnvironmentObservation{Details: fmt.Sprintf("no se observó una ventana principal del proceso objetivo %q; la observación no demuestra que el cambio haya fallado", req.Target), Status: autonomy.ObservationInsufficient}, nil
	}
	focusedName, focusedPID, err := i.foreground()
	if err != nil {
		return autonomy.EnvironmentObservation{Details: fmt.Sprintf("no se pudo observar la ventana en primer plano: %v", err), Status: autonomy.ObservationInsufficient}, nil
	}
	focusedCanonical := strings.ToLower(strings.TrimSuffix(focusedName, filepath.Ext(focusedName)))
	if focusedCanonical == target {
		return autonomy.EnvironmentObservation{Confirmed: true, Details: fmt.Sprintf("el proceso objetivo %q tiene el foco observado (PID %d)", focusedName, focusedPID), Status: autonomy.ObservationConfirmed, Facts: []autonomy.ObservationFact{{Subject: "foreground_process", Predicate: "process.name", Expected: target, Observed: focusedCanonical}}}, nil
	}
	return autonomy.EnvironmentObservation{Details: fmt.Sprintf("se observó el foco en %q (PID %d), no en el proceso objetivo %q", focusedName, focusedPID, req.Target), Status: autonomy.ObservationContradicted, Facts: []autonomy.ObservationFact{{Subject: "foreground_process", Predicate: "process.name", Expected: target, Observed: focusedCanonical}}}, nil
}

func mapPostconditionFacts(facts []solving.PostconditionFact) []autonomy.ObservationFact {
	mapped := make([]autonomy.ObservationFact, 0, len(facts))
	for _, fact := range facts {
		mapped = append(mapped, autonomy.ObservationFact{Subject: fact.Subject, Predicate: fact.Predicate, Expected: fact.Expected, Observed: fact.Observed})
	}
	return mapped
}

func firstParam(req autonomy.ActionRequest, key, fallback string) string {
	if value := req.Parameters[key]; value != "" {
		return value
	}
	return fallback
}

// goalActionVerifier combines the technical execution result with the existing
// read-only environment observer. A successful executor response alone cannot
// close a Goal task unless the expected external effect is also observed.
type goalActionVerifier struct {
	technical *autonomy.ActionVerifier
	inspector engineEnvironmentInspector
}

func (v goalActionVerifier) Verify(ctx context.Context, req autonomy.ActionRequest, response autonomy.ActionResponse) autonomy.VerificationResult {
	technical := v.technical.Verify(ctx, req, response)
	if !technical.Success {
		return technical
	}
	// Goal execution must use the same epistemic/provenance contract as the
	// autonomous cycle. A parallel boolean-only check would allow Goal to close
	// on evidence that the canonical verifier rejects, and would discard facts.
	env := autonomy.NewEnvironmentVerifier(v.inspector).Verify(ctx, req, technical)
	technical.Success = env.Success
	technical.Output = env.Output
	technical.ErrorReason = env.ErrorReason
	technical.ObservationStatus = env.ObservationStatus
	technical.ObservationDetails = env.Details
	technical.Observation = env.Observation
	return technical
}

// ExplicitGoalActionMapper exige una traducción explícita. Acepta el formato
// estable "action=<action> target=<target>" y parámetros "param.<clave>=<valor>"
// en Title o Description. También permite que Title sea exactamente una de
// las Actions ejecutables cuando Target/Parameters ya vienen estructurados.
type ExplicitGoalActionMapper struct{}

var explicitAutonomyActions = map[string]bool{
	"crear_carpeta":     true,
	"mover_archivo":     true,
	"renombrar_archivo": true,
}

func (ExplicitGoalActionMapper) MapTask(ctx context.Context, goal *planning.Goal, task *planning.Task) (autonomy.ActionRequest, error) {
	if err := ctx.Err(); err != nil {
		return autonomy.ActionRequest{}, err
	}
	if task == nil || goal == nil {
		return autonomy.ActionRequest{}, fmt.Errorf("goal/task no configurado")
	}

	text := strings.TrimSpace(task.Description)
	title := strings.TrimSpace(task.Title)
	action := ""
	target := ""
	params := map[string]string{}

	fields := strings.Fields(strings.ReplaceAll(text, ";", " "))
	fields = append(fields, strings.Fields(strings.ReplaceAll(title, ";", " "))...)
	for _, field := range fields {
		pair := strings.SplitN(field, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key := strings.TrimSpace(pair[0])
		value := strings.Trim(strings.TrimSpace(pair[1]), "\"")
		switch key {
		case "action":
			action = value
		case "target":
			target = value
		default:
			if strings.HasPrefix(key, "param.") {
				params[strings.TrimPrefix(key, "param.")] = value
			}
		}
	}

	if action == "" && explicitAutonomyActions[title] {
		action = title
	}
	if !explicitAutonomyActions[action] {
		return autonomy.ActionRequest{}, fmt.Errorf("la tarea %q no declara una Action autónoma ejecutable explícita", task.ID)
	}
	if target == "" {
		target = params["ruta"]
	}
	if target == "" && action == "mover_archivo" {
		target = params["destino"]
	}
	if target == "" && action == "renombrar_archivo" {
		target = params["destino"]
	}

	objective := strings.TrimSpace(goal.Description)
	if objective == "" {
		objective = strings.TrimSpace(goal.Name)
	}
	return autonomy.ActionRequest{ID: task.ID, Action: action, Objective: objective, Target: target, Parameters: params}, nil
}

// buildAutonomousExecutionCycle es el único punto de composición de B88.
func (e *Engine) buildAutonomousExecutionCycle() (*autonomy.Cycle, error) {
	if e.autonomousState == nil || e.goalMonitor == nil || e.planner == nil || e.world == nil {
		return nil, fmt.Errorf("B88: dependencias del ciclo autónomo incompletas")
	}
	// B90.3 — GoalPlanningAdapter ya no se construye acá: es una
	// dependencia estable de Engine (ver NewEngine), construida una sola
	// vez para que su estado (plans/activeTasks, y el Replanner de B87
	// que sostiene adentro) sobreviva a que este método se llame más de
	// una vez. Antes de B90.3, cada llamada a StartAutonomousCycle()
	// creaba un GoalPlanningAdapter nuevo — con plans/activeTasks
	// vacíos — descartando en silencio cualquier progreso del ciclo
	// anterior.
	if e.goalPlanningAdapter == nil {
		return nil, fmt.Errorf("B88: GoalPlanningAdapter no configurado (esperado desde NewEngine, ver B90.3)")
	}
	goalAdapter := e.goalPlanningAdapter
	// B90.7 — persistingCycleAdapter envuelve el mismo goalAdapter para
	// disparar la persistencia del GoalStore después de cada GetNextAction/
	// UpdateState real; goalAdapter en sí (B82/B90.3) no cambia.
	persistingAdapter := &persistingCycleAdapter{engine: e, goalAdapter: goalAdapter}

	// B92.4 — un DiagnosingRecoveryAdvisor nuevo por cada llamada a este
	// método, mismo criterio que environmentObserver/environmentVerifier
	// más abajo: reconstruir el ciclo entero implica estado de diagnóstico
	// fresco, no arrastrar diagnósticos huérfanos de una corrida anterior.
	// Se conecta en dos puntos:
	//   - como CycleDiagnoser real de Cycle (antes: nil, "B81/Diagnoser
	//     queda fuera de B88") para que DiagnoseFailure (B92.2) corra en
	//     la fase DIAGNOSE de cada RunStep fallido;
	//   - como FailureAdvisor de goalAdapter (B92.4, ver
	//     autonomy/goal_planner.go) para que UpdateState consulte ese
	//     mismo diagnóstico antes de aplicar Replanner.Decide.
	recoveryAdvisor := NewDiagnosingRecoveryAdvisor()
	goalAdapter.SetFailureAdvisor(recoveryAdvisor)

	snapshotInspector := engineEnvironmentSnapshotInspector{manager: e.world}
	environmentObserver := autonomy.NewEnvironmentObserver(snapshotInspector)
	cycleObserver := &engineCycleObserver{observer: environmentObserver}

	environmentInspector := engineEnvironmentInspector{}
	environmentVerifier := autonomy.NewEnvironmentVerifier(environmentInspector)

	learner := autonomy.NewLearningMemoryAdapter(e.eventStore, e.learningState, "autonomy")
	updater := autonomy.NewCompositeCycleUpdater(persistingAdapter, learner)
	autonomyExecutor := NewAutonomyExecutor(e)

	cycle := autonomy.NewCycle(
		e.autonomousState,
		cycleObserver,
		nil, // B88 deja Evaluator explícitamente para B89.
		persistingAdapter,
		autonomyExecutor,
		nil,
		recoveryAdvisor, // B92.4 — antes nil ("B81/Diagnoser queda fuera de B88").
		learner,
		updater,
	)
	cycle.SetEnvironmentVerifier(environmentVerifier)
	cycle.SetSituationalEvaluator(engineSituationalCycleEvaluator{engine: e})
	return cycle, nil
}

// StartAutonomousCycle B88 mantiene el loop integrado sin eliminar el camino
// conversacional B77: ambos usan el mismo AutonomousLoop, que ejecuta el Cycle
// y después el camino conversacional legado en cada tick.
func (e *Engine) StartAutonomousCycle(ctx context.Context) error {
	cycle, err := e.buildAutonomousExecutionCycle()
	if err != nil {
		return err
	}
	e.autonomousLoop = autonomy.NewIntegratedAutonomousLoop(
		autonomy.LoopConfig{}, cycle,
		e.contextObserver,
		e.initiativeEngine,
		e.conversationManager,
		autonomyStateAdapter{state: e.autonomousState},
		autonomyGoalAdapter{monitor: e.goalMonitor},
	)
	e.autonomousLoop.SetRunLock(&e.autonomyRuntimeMu)
	loopCtx, cancel := context.WithCancel(ctx)
	e.autonomyCancel = cancel
	e.autonomousLoop.Start(loopCtx)
	go e.consumeAutonomyDecisions(loopCtx)
	return nil
}
