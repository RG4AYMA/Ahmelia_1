package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"
)

// ============================================================================
// DEPENDENCY_SOLVER.GO — Bloque 24, Fase 3 (Cognitive Mind), Capa 3.4
// ============================================================================
//
// Knowledge Mapper (Bloque 23) dice, Task por Task, qué ya es capacidad
// real y qué es gap (territorio nuevo) — pero no dice en qué ORDEN hay
// que resolver las tareas, ni si un gap temprano deja bloqueadas a las
// que vienen después. Dependency Solver arma esa relación: construye el
// grafo de dependencias entre Tasks (secuencial, dentro de la jerarquía
// que ya trae TaskPlan) y marca cada Task como bloqueada si depende —
// directa o transitivamente — de un gap sin resolver.
//
// Primera pasada, mismo criterio que el resto de Fase 3: el modelo de
// dependencia es el más simple que tiene sentido hoy — orden secuencial
// dentro del plan (T1 antes que T2, T1.1 antes que T1.2, todas las
// subtareas de T1 antes que T2), no un grafo arbitrario con
// dependencias declaradas a mano. Constraint Solver (Capa 3.6, más
// adelante) es el que va a poder expresar relaciones más finas si hace
// falta — acá alcanza con "lo que viene después de un gap queda
// bloqueado por ese gap".
//
// A diferencia de Knowledge Mapper (que solo necesitaba LearningState),
// Dependency Solver consume la SALIDA de Knowledge Mapper (el
// KnowledgeMap) además del TaskPlan — es el primer nodo de Fase 3 que
// depende de otro nodo de Fase 3, no solo de un módulo de fuera.

// DependencyEdge es una relación "TaskID depende de DependsOnID" dentro
// del mismo TaskPlan.
type DependencyEdge struct {
	TaskID      string
	DependsOnID string
}

// DependencyNode es el estado de UNA Task dentro del grafo.
type DependencyNode struct {
	TaskID      string
	TaskDesc    string
	IsGap       bool   // true si Knowledge Mapper marcó esta Task como territorio nuevo
	Blocked     bool   // true si depende, directa o transitivamente, de un gap anterior en el orden
	BlockedByID string // TaskID del gap más cercano que la bloquea; "" si no está bloqueada
}

// DependencyGraph es el resultado de resolver dependencias sobre un
// TaskPlan + su KnowledgeMap.
type DependencyGraph struct {
	Goal  string
	Edges []DependencyEdge
	Nodes map[string]*DependencyNode // TaskID -> nodo, para lookup directo sin recorrer Order/Edges
	Order []string                   // TaskIDs en el orden secuencial armado (mismo orden que Knowledge Mapper usa: nivel 1 + subtareas)
}

// Solver es el Dependency Solver en sí. Sin estado propio — igual que
// Analyzer y Decomposer, cada Solve() es independiente; lo que varía
// entre llamadas es el TaskPlan/KnowledgeMap que recibe, no el Solver.
type Solver struct{}

// NewSolver arma el Dependency Solver.
func NewSolver() *Solver {
	return &Solver{}
}

// Solve arma el grafo secuencial de dependencias del plan y propaga el
// bloqueo: apenas aparece un gap en el orden, todo lo que viene después
// queda Blocked, apuntando al gap más cercano (no al primero de todos,
// para que la explicación sea siempre lo más específica posible).
//
// km puede ser nil (por ejemplo si todavía no se corrió Knowledge
// Mapper) — igual que Mapper(nil) en Bloque 23, no rompe: sin
// KnowledgeMap no hay forma de saber qué Task es gap, así que el grafo
// sale completo (Order/Edges) pero sin ninguna Task marcada Blocked.
func (s *Solver) Solve(plan *TaskPlan, km *KnowledgeMap) *DependencyGraph {
	dg := &DependencyGraph{Goal: plan.Goal, Nodes: make(map[string]*DependencyNode)}

	gapByID := make(map[string]bool)
	if km != nil {
		for _, m := range km.Matches {
			gapByID[m.TaskID] = !m.Known
		}
	}

	// Mismo recorrido que Mapper.Map (Bloque 23): nivel 1 y sus
	// subtareas, en orden — así Order queda alineado con Matches.
	var flat []*Task
	for _, task := range plan.Tasks {
		flat = append(flat, task)
		flat = append(flat, task.Subtasks...)
	}

	var lastGapID string
	for i, task := range flat {
		node := &DependencyNode{
			TaskID:   task.ID,
			TaskDesc: task.Description,
			IsGap:    gapByID[task.ID],
		}

		if i > 0 {
			dg.Edges = append(dg.Edges, DependencyEdge{TaskID: task.ID, DependsOnID: flat[i-1].ID})
		}

		if lastGapID != "" {
			node.Blocked = true
			node.BlockedByID = lastGapID
		}

		if node.IsGap {
			lastGapID = task.ID
		}

		dg.Nodes[task.ID] = node
		dg.Order = append(dg.Order, task.ID)
	}

	return dg
}

// BlockedCount cuenta cuántas Tasks quedaron bloqueadas — sin que quien
// llame tenga que recorrer Nodes a mano, mismo criterio que
// KnownCount()/GapCount() en Bloque 23.
func (dg *DependencyGraph) BlockedCount() int {
	n := 0
	for _, node := range dg.Nodes {
		if node.Blocked {
			n++
		}
	}
	return n
}

// Blocked devuelve, en orden, las Tasks bloqueadas — lo que Hypothesis
// Generator o Constraint Solver (próximas capas) van a necesitar para
// razonar sobre qué resolver primero.
func (dg *DependencyGraph) Blocked() []*DependencyNode {
	var blocked []*DependencyNode
	for _, id := range dg.Order {
		if node := dg.Nodes[id]; node.Blocked {
			blocked = append(blocked, node)
		}
	}
	return blocked
}

// RootGaps devuelve las Tasks que son gap en sí mismas (territorio
// nuevo), en orden — distinto de Blocked(): un gap no está "bloqueado"
// por otra Task, ES la causa raíz de que las que vienen después sí lo
// estén.
func (dg *DependencyGraph) RootGaps() []*DependencyNode {
	var gaps []*DependencyNode
	for _, id := range dg.Order {
		if node := dg.Nodes[id]; node.IsGap {
			gaps = append(gaps, node)
		}
	}
	return gaps
}

// Describe arma la narrativa: el orden secuencial del plan, y si algún
// gap deja tareas bloqueadas más adelante.
func (dg *DependencyGraph) Describe() string {
	if len(dg.Order) == 0 {
		return "No tengo tareas para ordenar — mapeá el conocimiento primero con Knowledge Mapper."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Orden para \"%s\": ", dg.Goal)

	parts := make([]string, 0, len(dg.Order))
	for _, id := range dg.Order {
		parts = append(parts, fmt.Sprintf("%s) %s", id, dg.Nodes[id].TaskDesc))
	}
	b.WriteString(strings.Join(parts, " → "))
	b.WriteString(". ")

	gaps := dg.RootGaps()
	if len(gaps) == 0 {
		b.WriteString("Sin territorio nuevo en el camino — no hay bloqueos.")
		return strings.TrimSpace(b.String())
	}

	var gapDescs []string
	for _, g := range gaps {
		gapDescs = append(gapDescs, fmt.Sprintf("%s (%s)", g.TaskID, g.TaskDesc))
	}
	fmt.Fprintf(&b, "Territorio nuevo en: %s. ", strings.Join(gapDescs, "; "))

	blocked := dg.Blocked()
	if len(blocked) == 0 {
		b.WriteString("No deja nada bloqueado más adelante.")
	} else {
		var blockedDescs []string
		for _, bl := range blocked {
			blockedDescs = append(blockedDescs, fmt.Sprintf("%s (por %s)", bl.TaskID, bl.BlockedByID))
		}
		fmt.Fprintf(&b, "Deja bloqueadas: %s.", strings.Join(blockedDescs, "; "))
	}

	return strings.TrimSpace(b.String())
}

// ── Trigger de consulta ──────────────────────────────────────────────

var dependencySolveTriggers = []string{
	"que orden debo seguir para",
	"en que orden hago",
	"que me esta bloqueando en",
	"que bloqueos hay en",
	"detecta bloqueos en",
}

// ParseDependencySolveQuery detecta si el usuario pregunta por el orden
// de ejecución o los bloqueos — distinto de las tres capas anteriores:
// Goal Analyzer pregunta "qué es", Problem Decomposer "cómo se divide",
// Knowledge Mapper "cuánto ya sé hacer", y esto pregunta "en qué orden
// y qué me frena".
func ParseDependencySolveQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingDependency)
}
