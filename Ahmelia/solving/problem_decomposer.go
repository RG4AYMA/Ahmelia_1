package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	"ahmelia/shared"
)

// ============================================================================
// PROBLEM_DECOMPOSER.GO — Bloque 22, Fase 3 (Cognitive Mind), Capa 3.2
// ============================================================================
//
// Goal Analyzer (Bloque 21) da Subgoals — una lista PLANA de fases
// típicas del dominio detectado ("Diseñar la arquitectura", "Probar",
// etc.). Son títulos de fase, no tareas ejecutables todavía: nadie se
// sienta a "hacer" Diseñar la arquitectura sin más — hay pasos adentro.
//
// Problem Decomposer toma esos Subgoals y los convierte en una
// JERARQUÍA de tareas: cada Subgoal se vuelve una Task de nivel 1, y
// cada Task de nivel 1 se expande en sub-tareas concretas según el
// verbo/acción que describe (definir/diseñar/implementar/probar/...).
// Todavía no sabe nada del CONOCIMIENTO real de Ahmelia sobre el tema
// (eso es Knowledge Mapper, Capa 3.3) ni de qué tarea depende de cuál
// (eso es Dependency Solver, Capa 3.4) — por ahora solo estructura.

// Task es un nodo del árbol de tareas. Subtasks vacío significa hoja
// (no necesita más desglose para ser ejecutable).
type Task struct {
	ID          string
	Description string
	Subtasks    []*Task
}

// TaskPlan es el resultado de descomponer un GoalAnalysis completo.
type TaskPlan struct {
	Goal  string // GoalAnalysis.MainGoal, para no perder de vista qué se estaba resolviendo
	Tasks []*Task
}

// phaseTemplates asocia palabras clave típicas de una fase (las que
// Goal Analyzer ya usa en sus Subgoals — "definir", "diseñar",
// "implementar"...) con la forma habitual de desglosarla en pasos más
// chicos. Igual que domainTemplates en goal_analyzer.go: un punto de
// partida razonable, no una verdad definitiva — Knowledge Mapper lo va
// a poder refinar más adelante con contexto real.
var phaseTemplates = []struct {
	Keywords []string
	Steps    []string
}{
	{[]string{"definir", "aclarar", "identificar el objetivo"}, []string{
		"Listar los requisitos ya conocidos",
		"Identificar qué falta aclarar",
		"Confirmar el alcance final",
	}},
	{[]string{"diseñar", "disenar", "bocetar"}, []string{
		"Bocetar la estructura general",
		"Elegir las piezas o tecnologías clave",
		"Revisar el diseño antes de construir",
	}},
	{[]string{"implementar", "construir", "ejecutar", "codigo"}, []string{
		"Armar el esqueleto base",
		"Construir cada parte por separado",
		"Integrar las partes",
	}},
	{[]string{"probar", "pulir", "optimizar"}, []string{
		"Definir qué casos probar",
		"Ejecutar las pruebas",
		"Corregir lo que falle",
	}},
	{[]string{"documentar", "redactar"}, []string{
		"Reunir el contenido a documentar",
		"Escribir el borrador",
		"Revisar y pulir",
	}},
	{[]string{"recolectar", "relevar"}, []string{
		"Identificar las fuentes",
		"Recolectar los datos",
		"Organizar lo recolectado",
	}},
	{[]string{"analizar", "comparar", "sacar conclusiones"}, []string{
		"Establecer los criterios de comparación",
		"Aplicar los criterios",
		"Sacar conclusiones",
	}},
	{[]string{"revisar", "validar", "verificar"}, []string{
		"Chequear contra el objetivo original",
		"Marcar lo que haya que corregir",
	}},
}

// genericSteps se usa cuando ninguna plantilla de fase matchea — mismo
// criterio que genericSubgoals en goal_analyzer.go: no es un error, es
// que la fase no cae en ningún patrón conocido todavía.
var genericSteps = []string{
	"Preparar lo necesario",
	"Ejecutar el paso",
	"Verificar que salió bien",
}

// Decomposer es el Problem Decomposer en sí. Sin estado propio, mismo
// criterio que Analyzer (Bloque 21).
type Decomposer struct{}

// NewDecomposer arma el Problem Decomposer.
func NewDecomposer() *Decomposer {
	return &Decomposer{}
}

// Decompose convierte un GoalAnalysis (Bloque 21) en un TaskPlan
// jerárquico: una Task de nivel 1 por Subgoal, cada una expandida en
// sub-tareas según su plantilla de fase.
func (d *Decomposer) Decompose(ga *GoalAnalysis) *TaskPlan {
	plan := &TaskPlan{Goal: ga.MainGoal}

	for i, subgoal := range ga.Subgoals {
		task := &Task{
			ID:          fmt.Sprintf("T%d", i+1),
			Description: subgoal,
			Subtasks:    expandPhase(subgoal, fmt.Sprintf("T%d", i+1)),
		}
		plan.Tasks = append(plan.Tasks, task)
	}

	return plan
}

// expandPhase busca la plantilla de fase que matchea la descripción del
// Subgoal y arma las sub-tareas con IDs jerárquicos (T1.1, T1.2, ...).
func expandPhase(description, parentID string) []*Task {
	norm := shared.Normalize(description)

	var steps []string
	for _, tmpl := range phaseTemplates {
		for _, kw := range tmpl.Keywords {
			if strings.Contains(norm, kw) {
				steps = tmpl.Steps
				break
			}
		}
		if steps != nil {
			break
		}
	}
	if steps == nil {
		steps = genericSteps
	}

	subtasks := make([]*Task, 0, len(steps))
	for j, step := range steps {
		subtasks = append(subtasks, &Task{
			ID:          fmt.Sprintf("%s.%d", parentID, j+1),
			Description: step,
		})
	}
	return subtasks
}

// CountTasks cuenta el total de tareas en el plan, incluyendo
// sub-tareas — útil para narrar "son N tareas en total" sin que quien
// llame tenga que recorrer el árbol a mano.
func (tp *TaskPlan) CountTasks() int {
	total := 0
	for _, t := range tp.Tasks {
		total += 1 + len(t.Subtasks)
	}
	return total
}

// Describe arma la narrativa jerárquica completa del plan.
func (tp *TaskPlan) Describe() string {
	if len(tp.Tasks) == 0 {
		return "No tengo subobjetivos para descomponer — analizá primero el objetivo con Goal Analyzer."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Plan para \"%s\" (%d tareas en total): ", tp.Goal, tp.CountTasks())

	parts := make([]string, 0, len(tp.Tasks))
	for _, task := range tp.Tasks {
		if len(task.Subtasks) == 0 {
			parts = append(parts, fmt.Sprintf("%s) %s", task.ID, task.Description))
			continue
		}
		subParts := make([]string, 0, len(task.Subtasks))
		for _, sub := range task.Subtasks {
			subParts = append(subParts, sub.Description)
		}
		parts = append(parts, fmt.Sprintf("%s) %s [%s]", task.ID, task.Description, strings.Join(subParts, " → ")))
	}
	b.WriteString(strings.Join(parts, "; "))
	b.WriteString(".")

	return b.String()
}

// ── Trigger de consulta ──────────────────────────────────────────────

var decomposeTriggers = []string{
	"descompone en tareas",
	"descompón en tareas",
	"dame la jerarquia de tareas",
	"que tareas necesito para",
	"desglosa esto en tareas",
	"divide esto en tareas",
	"plan de tareas para",
}

// ParseDecomposeQuery detecta si el usuario pide la descomposición en
// tareas (Problem Decomposer), no solo el análisis de objetivo (Goal
// Analyzer, Bloque 21) — son preguntas distintas: una pide el "qué",
// esta pide el "cómo se divide en pasos".
func ParseDecomposeQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingDecompose)
}
