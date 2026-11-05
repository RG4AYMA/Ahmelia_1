package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/shared"
)

// ============================================================================
// KNOWLEDGE_MAPPER.GO — Bloque 23, Fase 3 (Cognitive Mind), Capa 3.3
// ============================================================================
//
// Problem Decomposer (Bloque 22) da una jerarquía de Tasks — pasos
// concretos, pero sin decir si Ahmelia YA sabe hacer alguno. Knowledge
// Mapper cruza cada Task contra lo que Ahmelia YA tiene:
//
//   - shared.ActionPatterns (Bloque clasificador) — ¿esta Task se
//     parece a una Action que el clasificador ya reconoce y engine.go ya
//     ejecuta de verdad? (crear_carpeta, abrir_programa, ...)
//   - memory_intelligence.LearningState.ActionConfidence (Bloque 7/8) —
//     si matcheó una Action conocida, ¿qué tan bien le ha ido
//     históricamente con ella?
//
// El resultado no es "sí" o "no" — es un mapa Task→conocimiento: qué
// tareas ya son capacidades reales (con su nivel de confianza) y cuáles
// son gaps (nada en el repertorio actual se le parece). Eso es
// justamente el insumo que va a necesitar Dependency Solver (Capa 3.4,
// próximo bloque): una Task que depende de un gap es distinta de una
// que depende de algo que Ahmelia ya sabe hacer bien.

// KnowledgeMatch es lo que Ahmelia sabe sobre UNA Task concreta.
type KnowledgeMatch struct {
	TaskID     string
	TaskDesc   string
	Action     string  // Action de shared.ActionPatterns que matcheó, "" si no hay ninguna
	Label      string  // shared.ActionLabels[Action], para mostrar en español
	Confidence float64 // learningState.ActionConfidence[Action]; -1 si no hay historial (Action nueva o Learning no disponible)
	Known      bool    // true si matcheó una Action real y ejecutable hoy
}

// KnowledgeMap es el resultado de mapear un TaskPlan completo (Bloque 22)
// contra el conocimiento actual de Ahmelia.
type KnowledgeMap struct {
	Goal    string
	Matches []KnowledgeMatch
}

// Mapper es el Knowledge Mapper en sí. A diferencia de Analyzer
// (Bloque 21) y Decomposer (Bloque 22), SÍ tiene una dependencia real:
// necesita LearningState para saber la confianza histórica. Puede
// recibir nil (por ejemplo en tests) — ahí todo Match.Known que
// matchee queda con Confidence=-1, no rompe nada.
type Mapper struct {
	Learning *memoryintelligence.LearningStore
}

// NewMapper arma el Knowledge Mapper con la fuente de confianza real
// (Bloque 7/8) — misma referencia que ya vive en el Engine, no una
// copia.
func NewMapper(learning *memoryintelligence.LearningStore) *Mapper {
	return &Mapper{Learning: learning}
}

// Map recorre TODAS las Tasks del plan (nivel 1 y sub-tareas) y arma un
// KnowledgeMatch para cada una.
func (m *Mapper) Map(plan *TaskPlan) *KnowledgeMap {
	km := &KnowledgeMap{Goal: plan.Goal}
	for _, task := range plan.Tasks {
		km.Matches = append(km.Matches, m.mapTask(task))
		for _, sub := range task.Subtasks {
			km.Matches = append(km.Matches, m.mapTask(sub))
		}
	}
	return km
}

// mapTask busca, entre TODAS las Action de shared.ActionPatterns, si
// alguna de sus keywords aparece en la descripción de la Task. La
// primera que matchea gana — no hay ranking de especificidad todavía
// (mismo nivel de sofisticación que el resto de Fase 3 por ahora: es
// una primera pasada, no la versión final).
func (m *Mapper) mapTask(task *Task) KnowledgeMatch {
	norm := shared.Normalize(task.Description)
	match := KnowledgeMatch{TaskID: task.ID, TaskDesc: task.Description, Confidence: -1}

	for action, keywords := range shared.ActionPatterns {
		matched := false
		for _, kw := range keywords {
			if strings.Contains(norm, kw) {
				matched = true
				break
			}
		}
		if matched {
			match.Action = action
			match.Known = true
			if label, ok := shared.ActionLabels[action]; ok {
				match.Label = label
			} else {
				match.Label = action
			}
			break
		}
	}

	if match.Known && m.Learning != nil {
		if m.Learning.HasConfidence(match.Action) {
			match.Confidence = m.Learning.Confidence(match.Action)
		}
	}

	return match
}

// KnownCount / GapCount separan lo que ya es capacidad real de lo que
// es un vacío de conocimiento — el resumen que Describe() necesita, sin
// que quien llame tenga que recorrer Matches a mano.
func (km *KnowledgeMap) KnownCount() int {
	n := 0
	for _, m := range km.Matches {
		if m.Known {
			n++
		}
	}
	return n
}

func (km *KnowledgeMap) GapCount() int {
	return len(km.Matches) - km.KnownCount()
}

// Gaps devuelve solo las Tasks sin ninguna Action conocida — lo que
// Dependency Solver (Capa 3.4) va a necesitar tratar distinto de las
// que ya son capacidades reales.
func (km *KnowledgeMap) Gaps() []KnowledgeMatch {
	var gaps []KnowledgeMatch
	for _, m := range km.Matches {
		if !m.Known {
			gaps = append(gaps, m)
		}
	}
	return gaps
}

// Describe arma la narrativa: cuánto de lo que hay que hacer ya es
// capacidad conocida (con qué confianza) y cuánto es territorio nuevo.
func (km *KnowledgeMap) Describe() string {
	if len(km.Matches) == 0 {
		return "No tengo tareas para mapear — descomponé el objetivo primero con Problem Decomposer."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "De %d tareas para \"%s\", %d ya las sé hacer y %d son territorio nuevo. ",
		len(km.Matches), km.Goal, km.KnownCount(), km.GapCount())

	var known []string
	for _, mt := range km.Matches {
		if !mt.Known {
			continue
		}
		if mt.Confidence >= 0 {
			known = append(known, fmt.Sprintf("%s (%s, %.0f%% de éxito)", mt.TaskDesc, mt.Label, mt.Confidence))
		} else {
			known = append(known, fmt.Sprintf("%s (%s, sin historial todavía)", mt.TaskDesc, mt.Label))
		}
	}
	if len(known) > 0 {
		b.WriteString("Ya conocidas: ")
		b.WriteString(strings.Join(known, "; "))
		b.WriteString(". ")
	}

	if gaps := km.Gaps(); len(gaps) > 0 {
		var gapDescs []string
		for _, g := range gaps {
			gapDescs = append(gapDescs, g.TaskDesc)
		}
		b.WriteString("Territorio nuevo: ")
		b.WriteString(strings.Join(gapDescs, "; "))
		b.WriteString(".")
	}

	return strings.TrimSpace(b.String())
}

// ── Trigger de consulta ──────────────────────────────────────────────

var knowledgeMapTriggers = []string{
	"que sabes hacer de",
	"que ya sabes de",
	"cuanto sabes de",
	"que tan preparada estas para",
	"que conoces sobre como hacer",
}

// ParseKnowledgeMapQuery detecta si el usuario pregunta específicamente
// cuánto de un pedido ya es conocimiento real de Ahmelia — distinto de
// Goal Analyzer (qué es el pedido) y de Problem Decomposer (cómo se
// divide): esto pregunta "de eso, ¿qué tanto ya sabés hacer?".
func ParseKnowledgeMapQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingKnowledgeMap)
}
