package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	memoryintelligence "ahmelia/memory_intelligence"
)

// ============================================================================
// ANALOGICAL_REASONING.GO — Bloque 28, Fase 3 (Cognitive Mind), Capa 3.8
// ============================================================================
//
// Hypothesis Generator (Bloque 25) busca, para cada gap, la Action de
// shared.ActionPatterns más parecida — pero ActionPatterns es una lista
// ESTÁTICA de keywords, nunca se entera de lo que Ahmelia efectivamente
// vivió. Analogical Reasoning mira para otro lado: en vez del catálogo
// de capacidades declaradas, busca en el EventLog (Fase 1-2 de Pattern
// Learning, memory_intelligence/eventlog.go) — lo que Ahmelia REALMENTE
// ejecutó, con qué Target, y cómo le fue — algo cuyo Target se parezca
// a la descripción del gap. "Territorio nuevo" y "nunca lo intenté" no
// son lo mismo: un gap puede no matchear ninguna Action conocida y aun
// así parecerse a un Target que Ahmelia ya enfrentó de verdad.
//
// Reusa ComputeExperience (Bloque 3, memory_intelligence/experience.go)
// para las estadísticas del par Action+Target más parecido — mismo
// criterio "integrar, no crear" del resto de Fase 3: no se inventa un
// cálculo de éxito/fracaso nuevo, se llama al que ya existe.

// Analogy es la pista histórica encontrada para UN gap concreto.
type Analogy struct {
	TaskID      string
	TaskDesc    string
	PastAction  string  // Action del EventLog más parecida por Target; "" si ninguna comparte palabras
	PastTarget  string  // Target real que se ejecutó — el "esto ya lo hice" concreto
	Similarity  float64 // 0..1 — proporción de palabras de la Task que también aparecen en PastTarget
	Occurrences int     // ComputeExperience(log, PastAction, PastTarget).Attempts
	SuccessRate float64 // ComputeExperience(...).SuccessRate; solo tiene sentido si Occurrences > 0
}

// HasAnalogy distingue "encontré algo parecido en la historia real" de
// "no hay ningún antecedente" — mismo patrón que Hypothesis.HasLead()
// en Bloque 25, pero mirando el EventLog en vez de ActionPatterns.
func (a Analogy) HasAnalogy() bool {
	return a.PastAction != ""
}

// AnalogySet es el resultado de buscar analogías para TODOS los gaps de
// un KnowledgeMap.
type AnalogySet struct {
	Goal      string
	Analogies []Analogy
}

// Found devuelve solo las analogías encontradas, en orden — lo que de
// verdad aporta algo por sobre lo que ya decía Hypothesis Generator.
func (as *AnalogySet) Found() []Analogy {
	var found []Analogy
	for _, a := range as.Analogies {
		if a.HasAnalogy() {
			found = append(found, a)
		}
	}
	return found
}

// AnalogicalReasoner es el Analogical Reasoning en sí. Se llama
// AnalogicalReasoner y no Reasoner a secas para no competir con
// reasoning.Reasoner (paquete distinto, pero mismo espíritu retrospectivo
// que este bloque comparte a propósito — mejor un nombre explícito que
// una ambigüedad). Sin estado propio, mismo criterio que el resto de
// Fase 3.
type AnalogicalReasoner struct{}

// NewAnalogicalReasoner arma el Analogical Reasoning.
func NewAnalogicalReasoner() *AnalogicalReasoner {
	return &AnalogicalReasoner{}
}

// FindAnalogies recorre los Gaps() de km y busca, para cada uno, el par
// Action+Target del EventLog cuyo Target comparte más palabras con la
// descripción del gap. log puede ser nil (por ejemplo instalación
// nueva, sin historial todavía) — mismo criterio defensivo que el resto
// de Fase 3: sin EventLog no hay forma de encontrar una analogía, así
// que todas quedan sin PastAction, no con un dato inventado.
func (ar *AnalogicalReasoner) FindAnalogies(km *KnowledgeMap, log *memoryintelligence.EventLog) *AnalogySet {
	as := &AnalogySet{Goal: km.Goal}

	for _, gap := range km.Gaps() {
		as.Analogies = append(as.Analogies, ar.findAnalogy(gap, log))
	}

	return as
}

// findAnalogy busca, entre TODOS los pares Action+Target distintos que
// aparecen en log, el que más palabras comparte con la descripción del
// gap. Recorre los eventos en el orden en que están grabados (no hay
// map de por medio) así que, ante empate, gana el par que apareció
// primero en el historial — determinístico sin necesidad de ordenar.
func (ar *AnalogicalReasoner) findAnalogy(gap KnowledgeMatch, log *memoryintelligence.EventLog) Analogy {
	base := Analogy{TaskID: gap.TaskID, TaskDesc: gap.TaskDesc}

	if log == nil || len(log.Events) == 0 {
		return base
	}

	taskWords := significantWords(gap.TaskDesc)
	if len(taskWords) == 0 {
		return base
	}

	seenPairs := make(map[string]bool)
	bestAction, bestTarget := "", ""
	bestScore := 0

	for _, ev := range log.Events {
		if ev.Target == "" {
			continue
		}
		pairKey := ev.Action + "|" + ev.Target
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		targetWords := significantWords(ev.Target)
		overlap := 0
		for w := range taskWords {
			if targetWords[w] {
				overlap++
			}
		}
		if overlap > bestScore {
			bestScore = overlap
			bestAction, bestTarget = ev.Action, ev.Target
		}
	}

	if bestAction == "" {
		return base
	}

	base.PastAction = bestAction
	base.PastTarget = bestTarget
	base.Similarity = float64(bestScore) / float64(len(taskWords))

	if stats, found := memoryintelligence.ComputeExperience(log, bestAction, bestTarget); found {
		base.Occurrences = stats.Attempts
		base.SuccessRate = stats.SuccessRate
	}

	return base
}

// Describe arma la narrativa: por cada gap, la analogía encontrada (con
// cuántas veces se intentó y qué tan bien salió) o la admisión de que
// no hay ningún antecedente real en el historial.
func (as *AnalogySet) Describe() string {
	if len(as.Analogies) == 0 {
		return fmt.Sprintf("No tengo gaps para buscar analogías en \"%s\" — no hay territorio nuevo en el plan, o todavía no corriste Knowledge Mapper.", as.Goal)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Analogías para \"%s\": ", as.Goal)

	var parts []string
	for _, a := range as.Analogies {
		if a.HasAnalogy() {
			step := a.PastAction
			if a.PastTarget != "" {
				step = a.PastAction + " " + a.PastTarget
			}
			parts = append(parts, fmt.Sprintf("%s (%s) → se parece a %s, que %s (%.0f%% de palabras en común)",
				a.TaskID, a.TaskDesc, step, occurrenceSummary(a), a.Similarity*100))
		} else {
			parts = append(parts, fmt.Sprintf("%s (%s) → sin ningún antecedente real en el historial", a.TaskID, a.TaskDesc))
		}
	}
	b.WriteString(strings.Join(parts, "; "))
	b.WriteString(".")

	return b.String()
}

// occurrenceSummary narra el historial del par más parecido — distingue
// "lo intenté y me fue bien/mal" de "coincide en palabras pero nunca lo
// ejecuté de verdad" (Occurrences==0, por ejemplo si ComputeExperience
// no encontró el par exacto pese a que el Target vino de un evento real
// — no debería pasar, pero mejor una frase honesta que asumir datos).
func occurrenceSummary(a Analogy) string {
	if a.Occurrences == 0 {
		return "ya ejecuté al menos una vez, sin balance de éxito registrado"
	}
	return fmt.Sprintf("ya hice %d vez/veces con %.0f%% de éxito", a.Occurrences, a.SuccessRate*100)
}

// ── Trigger de consulta ──────────────────────────────────────────────

var analogyTriggers = []string{
	"ya resolviste algo parecido a",
	"hiciste algo parecido a",
	"tienes experiencia parecida a",
	"esto se parece a algo que ya hiciste en",
	"que tan parecido es esto a algo que ya hiciste en",
}

// ParseAnalogyQuery detecta si el usuario pregunta por antecedentes
// reales en el historial — distinto de Hypothesis Generator (Bloque
// 25), que compara contra el catálogo declarado de Actions, no contra
// lo que Ahmelia efectivamente vivió.
func ParseAnalogyQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingAnalogy)
}
