package solving

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
)

// ============================================================================
// CURRICULUM_BUILDER.GO — Bloque 47, Fase 4 (Knowledge Acquisition
// Mind), primer módulo
// ============================================================================
//
// Fase 4 arranca acá y no por Knowledge Discovery (4.1): Ahmelia hoy
// solo puede ABRIR una URL, no leer su contenido — no hay ninguna
// fuente real de "buscar información en Internet" para conectar
// todavía. Ese módulo queda pendiente de esa capacidad, y construirlo
// ahora sería simular una fuente de datos que no existe.
//
// Curriculum Builder no necesita nada de eso. Organiza señales que YA
// existen desde Fase 3 — Gaps() (Knowledge Mapper, 23), HasLead()
// (Hypothesis Generator, 25), HasAnalogy() (Analogical Reasoning, 28),
// DownstreamBlocked (25) — en un orden de aprendizaje. "Integrar, no
// crear", una vez más: cero cálculos nuevos.
//
// LÍMITE HONESTO, documentado a propósito: esto NO arma un árbol de
// prerrequisitos ("para Unreal necesitás C++, vectores, render...").
// Eso requeriría conocimiento externo real sobre CADA dominio que
// Ahmelia no tiene — inventarlo sería fabricar un curriculum que
// suena bien pero no está fundamentado en nada. Lo que sí puede hacer
// honestamente, con los datos que ya tiene: identificar CUÁLES gaps
// necesitan aprendizaje genuino (ni pista en el catálogo de
// capacidades, ni antecedente real en el historial) y ORDENARLOS por
// impacto real (cuántas otras tareas desbloquea cada uno) — un
// curriculum de PRIORIDAD, no de PRERREQUISITOS. Ese árbol más rico es
// justamente lo que Knowledge Discovery (4.1) podría aportar el día
// que exista una fuente real de información externa.

// LearningItem es un gap que entró al curriculum — algo que Ahmelia
// necesita aprender de cero, no algo que ya sabe hacer con otro nombre
// (eso ya lo resuelve Hypothesis Generator) ni algo parecido a lo que
// ya vivió (eso ya lo resuelve Analogical Reasoning).
type LearningItem struct {
	TaskID   string
	TaskDesc string
	Reason   string
	Priority int // == HypothesisGenerator.DownstreamBlocked — cuántas tareas desbloquea aprender esto
}

// Curriculum es el resultado de construir el plan de aprendizaje para
// TODOS los gaps genuinos de un plan.
type Curriculum struct {
	Goal  string
	Items []LearningItem
}

// CurriculumBuilder es el módulo en sí. Sin estado propio, mismo
// criterio que el resto de Fase 3.
type CurriculumBuilder struct{}

// NewCurriculumBuilder arma el constructor de curriculum.
func NewCurriculumBuilder() *CurriculumBuilder {
	return &CurriculumBuilder{}
}

// Build cruza hs (Bloque 25) y as (Bloque 28): solo entran al
// curriculum los gaps SIN pista en ActionPatterns Y SIN ningún
// antecedente real en el EventLog — los dos filtros existentes que ya
// distinguen "no sé pero se parece a algo" de "esto es territorio
// realmente nuevo". Ordenado por Priority descendente (mismo criterio
// que PriorityOrder() en Bloque 25): lo que más desbloquea, primero.
// hs/as pueden ser nil — mismo criterio defensivo de todo Fase 3: sin
// datos, el curriculum sale vacío, no inventado.
func (cb *CurriculumBuilder) Build(hs *HypothesisSet, as *AnalogySet) *Curriculum {
	c := &Curriculum{}
	if hs == nil {
		return c
	}
	c.Goal = hs.Goal

	hasAnalogy := make(map[string]bool)
	if as != nil {
		for _, a := range as.Analogies {
			if a.HasAnalogy() {
				hasAnalogy[a.TaskID] = true
			}
		}
	}

	for _, h := range hs.Hypotheses {
		if h.HasLead() || hasAnalogy[h.TaskID] {
			continue
		}
		c.Items = append(c.Items, LearningItem{
			TaskID:   h.TaskID,
			TaskDesc: h.TaskDesc,
			Reason:   "sin pista en el catálogo de capacidades y sin ningún antecedente real en el historial — esto hay que aprenderlo de cero",
			Priority: h.DownstreamBlocked,
		})
	}

	sort.SliceStable(c.Items, func(i, j int) bool {
		return c.Items[i].Priority > c.Items[j].Priority
	})

	return c
}

// Describe arma la narrativa del curriculum, en orden de prioridad.
func (c *Curriculum) Describe() string {
	if len(c.Items) == 0 {
		return fmt.Sprintf("No hay nada que aprender de cero para \"%s\" — todo gap tiene una pista en el catálogo de capacidades o un antecedente real en el historial.", c.Goal)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Para aprender de cero, en orden de prioridad, hacia \"%s\": ", c.Goal)

	parts := make([]string, len(c.Items))
	for i, item := range c.Items {
		if item.Priority > 0 {
			parts[i] = fmt.Sprintf("%s (%s) — %s (desbloquea %d tarea(s))", item.TaskID, item.TaskDesc, item.Reason, item.Priority)
		} else {
			parts[i] = fmt.Sprintf("%s (%s) — %s", item.TaskID, item.TaskDesc, item.Reason)
		}
	}
	b.WriteString(strings.Join(parts, "; "))
	b.WriteString(".")

	return b.String()
}

// ── Trigger de consulta ──────────────────────────────────────────────

var curriculumTriggers = []string{
	"que necesito aprender para",
	"que deberia estudiar para",
	"arma un plan de aprendizaje para",
	"que me falta aprender para",
}

// ParseCurriculumQuery detecta si el usuario pregunta qué hace falta
// aprender — distinta de Hypothesis Generator (25, "por dónde
// empezarías") y Analogical Reasoning (28, "hiciste algo parecido"):
// esta pregunta específicamente por lo que NO tiene ni pista ni
// antecedente, priorizado para aprenderlo.
func ParseCurriculumQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingCurriculum)
}
