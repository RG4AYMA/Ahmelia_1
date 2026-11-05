package core

import (
	"context"
	"strings"
	"testing"

	"ahmelia/planning"
)

// TestNewEngine_CreatesGoalPlanningAdapter cubre el primer criterio de
// B90.3: NewEngine() debe dejar e.goalPlanningAdapter ya construido, sin
// esperar a que main.go llame StartAutonomy().
func TestNewEngine_CreatesGoalPlanningAdapter(t *testing.T) {
	engine := buildTestEngine(t)
	if engine.goalPlanningAdapter == nil {
		t.Fatal("NewEngine() no dejó e.goalPlanningAdapter configurado")
	}
}

// TestBuildAutonomousExecutionCycle_NoRecreaElAdapter cubre el segundo y
// tercer criterio de B90.3: llamar a buildAutonomousExecutionCycle() más de
// una vez (lo que antes de B90.3 pasaba cada vez que StartAutonomousCycle()
// corría) no debe reemplazar e.goalPlanningAdapter por una instancia nueva.
func TestBuildAutonomousExecutionCycle_NoRecreaElAdapter(t *testing.T) {
	engine := buildTestEngine(t)
	first := engine.goalPlanningAdapter

	if _, err := engine.buildAutonomousExecutionCycle(); err != nil {
		t.Fatalf("buildAutonomousExecutionCycle() (1a llamada) error: %v", err)
	}
	if engine.goalPlanningAdapter != first {
		t.Fatal("buildAutonomousExecutionCycle() reemplazó e.goalPlanningAdapter en la 1a llamada")
	}

	if _, err := engine.buildAutonomousExecutionCycle(); err != nil {
		t.Fatalf("buildAutonomousExecutionCycle() (2a llamada) error: %v", err)
	}
	if engine.goalPlanningAdapter != first {
		t.Fatal("buildAutonomousExecutionCycle() reemplazó e.goalPlanningAdapter en la 2a llamada — se estaría perdiendo el estado del adapter en cada reinicio del ciclo")
	}
}

// TestGoalPlanningAdapter_CacheDePlanSobreviveEntreCiclos es la prueba de
// comportamiento real de "no se crea un segundo GoalPlanningAdapter al
// reiniciar el ciclo": si buildAutonomousExecutionCycle() (equivalente a lo
// que corre StartAutonomousCycle) se llama dos veces y el adapter fuera
// reconstruido en la segunda, el Plan cacheado para el Goal actual se habría
// perdido — planner.GeneratePlan() genera Tasks con ID nuevo cada vez
// (ver planning/task.go, generateTaskID), así que la tarea "siguiente" que
// GetNextAction() intenta mapear tendría un ID distinto en cada llamada.
// Con el adapter reutilizado (B90.3), el Plan se genera una sola vez y la
// misma Task (todavía Pending porque el mapper real no la resuelve) aparece
// en el mensaje de error de ambas llamadas.
//
// No depende de que el mapper (CapabilityResolver, B89) resuelva la tarea
// con éxito — solo de que el Plan/Task cacheados sean los mismos entre
// llamadas, que es lo único que B90.3 garantiza. La resolución real de
// capacidad es responsabilidad de B89, no de este test.
func TestGoalPlanningAdapter_CacheDePlanSobreviveEntreCiclos(t *testing.T) {
	engine := buildTestEngine(t)

	// Goal.Name genérico (no "Mejorar Reasoner"/"Crear API Conversacional"/
	// "Optimizar Memory") para caer en la descomposición por defecto de
	// planning.Planner.decomposeGoal — sus tareas ("Análisis", "Diseño",
	// ...) no van a resolver contra CapabilityResolver, que es exactamente
	// lo que este test necesita: un GetNextAction que falle al mapear
	// (nunca llega a task.Start()) pero SÍ deje el Plan cacheado.
	goal := planning.NewGoal("Objetivo genérico de prueba B90.3", "", planning.PriorityMedium, "user")
	engine.planner.RegisterGoal(goal)
	engine.autonomousState.SetCurrentGoal(goal)

	ctx := context.Background()

	_, _, err1 := engine.goalPlanningAdapter.GetNextAction(ctx)
	if err1 == nil {
		t.Fatal("esperaba que GetNextAction() fallara al mapear una tarea genérica contra CapabilityResolver (ver comentario del test)")
	}
	firstTaskID := extractTaskID(t, err1.Error())

	// Simula lo que antes de B90.3 pasaba en cada StartAutonomousCycle():
	// se vuelve a componer el ciclo autónomo.
	if _, err := engine.buildAutonomousExecutionCycle(); err != nil {
		t.Fatalf("buildAutonomousExecutionCycle(): %v", err)
	}

	_, _, err2 := engine.goalPlanningAdapter.GetNextAction(ctx)
	if err2 == nil {
		t.Fatal("esperaba que la segunda llamada también fallara al mapear (mismo motivo que la primera)")
	}
	secondTaskID := extractTaskID(t, err2.Error())

	if firstTaskID != secondTaskID {
		t.Fatalf("el Plan se regeneró entre llamadas: primera tarea intentada %q, segunda %q — el adapter perdió su caché, señal de que se reconstruyó", firstTaskID, secondTaskID)
	}
}

// extractTaskID saca el ID de tarea entre comillas que CapabilityResolver.
// MapTask deja en sus mensajes de error (ver autonomy/capability_resolver.go,
// fmt.Errorf("... %q ...", task.ID, ...)) — evita depender de una API nueva
// que B90.4 todavía no agrega (Plans()/ActiveTasks() en GoalPlanningAdapter).
func extractTaskID(t *testing.T, errMsg string) string {
	t.Helper()
	start := strings.Index(errMsg, `"`)
	if start == -1 {
		t.Fatalf("no se encontró un ID de tarea entre comillas en: %q", errMsg)
	}
	end := strings.Index(errMsg[start+1:], `"`)
	if end == -1 {
		t.Fatalf("no se encontró el cierre de comillas del ID de tarea en: %q", errMsg)
	}
	return errMsg[start+1 : start+1+end]
}
