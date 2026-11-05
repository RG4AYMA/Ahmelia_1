package solving

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"

	"ahmelia/shared"
)

// ============================================================================
// HYPOTHESIS_GENERATOR.GO — Bloque 25, Fase 3 (Cognitive Mind), Capa 3.5
// Extendido en Bloque 36 (Multi-Hypothesis Engine, Nivel II) — evoluciona
// el ranking que ya calculaba internamente (antes descartaba todo menos
// el mejor) para exponer el Top 3, sin romper la API que ya consumían
// Bloque 27 (Decision Engine), Bloque 29 (Self Evaluation) y Bloque 30
// (activación automática): ClosestAction/ClosestLabel/Similarity/
// HasLead() se comportan exactamente igual que antes.
// ============================================================================
//
// Knowledge Mapper (Bloque 23) separa capacidad real de territorio
// nuevo. Dependency Solver (Bloque 24) dice cuánto bloquea cada gap a
// lo que viene después. Ninguno de los dos dice todavía POR DÓNDE
// empezar con un gap — Hypothesis Generator es el primero que propone
// algo: para cada Task marcada como gap, busca la Action existente más
// PARECIDA (no una coincidencia exacta — eso ya lo intentó Knowledge
// Mapper y no encontró nada) y arma una hipótesis de por dónde probar
// primero, o reconoce honestamente que no hay ninguna pista.
//
// A diferencia de Knowledge Mapper (match exacto: alguna keyword
// literal de la Action aparece en la Task), acá la comparación es por
// SOLAPAMIENTO de palabras — cuántas palabras de la descripción de la
// Task también aparecen entre las keywords de una Action. Es
// deliberadamente más laxo: el objetivo no es confirmar una capacidad
// (para eso ya está Knowledge Mapper), es sugerir una pista razonable
// para arrancar, aunque no sea perfecta.
//
// Combina las salidas de los DOS bloques anteriores — usa Gaps() de
// Knowledge Mapper (Bloque 23) para saber QUÉ Tasks necesitan hipótesis,
// y Blocked() de Dependency Solver (Bloque 24) para priorizar CUÁL gap
// conviene resolver primero (el que más Tasks deja bloqueadas
// aguas abajo). Es "integrar", no "crear": no agrega ningún dato nuevo,
// solo cruza lo que los dos bloques anteriores ya calcularon.

// Hypothesis es la propuesta de Ahmelia sobre por dónde probar frente a
// UN gap concreto.
// Candidate es UNA Action candidata para resolver un gap — Bloque 36
// (Multi-Hypothesis Engine). Reason no es un campo decorativo vacío:
// se llena desde el primer momento con las palabras reales que
// comparten la Task y la Action (el mismo dato que ya calculaba
// closestAction internamente, antes se descartaba al armar el
// puntaje). Es deliberado que exista desde ya, aunque hoy nadie le
// pregunte a Ahmelia "por qué elegiste esta" — cuando ese bloque
// llegue, no va a hacer falta romper el struct otra vez.
type Candidate struct {
	Action     string
	Label      string
	Similarity float64
	Reason     string
}

type Hypothesis struct {
	TaskID            string
	TaskDesc          string
	ClosestAction     string      // Action de shared.ActionPatterns más parecida por solapamiento de palabras; "" si ninguna comparte ni una palabra
	ClosestLabel      string      // shared.ActionLabels[ClosestAction], para mostrar en español
	Similarity        float64     // 0..1 — proporción de palabras de la Task que también aparecen en las keywords de ClosestAction
	Reason            string      // Bloque 36 — palabras compartidas que sustentan ClosestAction; "" si HasLead()==false
	DownstreamBlocked int         // cuántas Tasks quedan bloqueadas por ESTE gap según Dependency Solver (Bloque 24) — a más alto, más prioridad para resolverlo primero
	Alternatives      []Candidate // Bloque 36 — 2do y 3er mejor candidato (top 3 total, contando ClosestAction), si tienen algo de solapamiento. nil si no hay ninguna alternativa real.
}

// HasLead es true si se encontró alguna Action con algo de solapamiento
// — distingue "tengo una pista, aunque débil" de "no tengo absolutamente
// ninguna referencia".
func (h Hypothesis) HasLead() bool {
	return h.ClosestAction != ""
}

// AlternativesLabel arma "X o Y" (o solo "X" si hay una sola
// alternativa) con las etiquetas en español de Alternatives — para
// Describe() y para cualquier consumidor futuro que quiera mostrar las
// opciones sin recorrer el slice a mano. "" si Alternatives está vacío.
func (h Hypothesis) AlternativesLabel() string {
	if len(h.Alternatives) == 0 {
		return ""
	}
	labels := make([]string, len(h.Alternatives))
	for i, c := range h.Alternatives {
		labels[i] = c.Label
	}
	return strings.Join(labels, " o ")
}

// HypothesisSet es el resultado de generar hipótesis para TODOS los
// gaps de un KnowledgeMap.
type HypothesisSet struct {
	Goal       string
	Hypotheses []Hypothesis
}

// Generator es el Hypothesis Generator en sí. Sin estado propio — igual
// que Analyzer/Decomposer/Solver, cada Generate() es independiente.
type Generator struct{}

// NewGenerator arma el Hypothesis Generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// Generate recorre los Gaps() de km y arma una Hypothesis por cada uno,
// rankeando las Action por solapamiento de palabras y consultando dg
// (puede ser nil) para saber cuántas Tasks deja bloqueadas — eso
// determina el orden de PriorityOrder(). El mejor candidato llena
// ClosestAction/ClosestLabel/Similarity/Reason (comportamiento
// idéntico al de antes de Bloque 36); el 2do y 3er mejor, si tienen
// algo de solapamiento real, quedan en Alternatives.
func (g *Generator) Generate(km *KnowledgeMap, dg *DependencyGraph) *HypothesisSet {
	hs := &HypothesisSet{Goal: km.Goal}

	for _, gap := range km.Gaps() {
		h := Hypothesis{
			TaskID:            gap.TaskID,
			TaskDesc:          gap.TaskDesc,
			DownstreamBlocked: downstreamBlockedCount(dg, gap.TaskID),
		}

		candidates := rankActions(gap.TaskDesc)
		if len(candidates) > 0 {
			best := candidates[0]
			h.ClosestAction = best.Action
			h.ClosestLabel = best.Label
			h.Similarity = best.Similarity
			h.Reason = best.Reason
			if len(candidates) > 1 {
				h.Alternatives = candidates[1:]
			}
		}

		hs.Hypotheses = append(hs.Hypotheses, h)
	}

	return hs
}

// downstreamBlockedCount cuenta, en dg, cuántas Tasks tienen
// BlockedByID == taskID. dg puede ser nil (por ejemplo si todavía no se
// corrió Dependency Solver) — ahí no hay forma de saberlo, así que
// queda en 0, no en un valor inventado.
func downstreamBlockedCount(dg *DependencyGraph, taskID string) int {
	if dg == nil {
		return 0
	}
	n := 0
	for _, node := range dg.Blocked() {
		if node.BlockedByID == taskID {
			n++
		}
	}
	return n
}

// maxCandidates es el tope de rankActions — Top 3 (mejor + 2
// alternativas), decisión deliberada de Bloque 36: las primeras tres
// concentran casi toda la información útil; guardar más significaría
// comparar/ordenar/serializar/cachear candidatos que casi nunca se
// van a usar, sin que Ahmelia esté todavía en una etapa de
// "exploración masiva" que los necesite.
const maxCandidates = 3

// rankActions busca, entre TODAS las Action de shared.ActionPatterns,
// las que más palabras comparten con la descripción de la Task, y
// devuelve hasta maxCandidates, ordenadas de mayor a menor
// solapamiento. Recorre las Action en orden alfabético antes de
// ordenar (no el orden de un map, que cambia entre corridas) para que
// un empate siempre resuelva igual — mismo criterio que tenía
// closestAction antes de este bloque.
func rankActions(taskDesc string) []Candidate {
	taskWords := significantWords(taskDesc)
	if len(taskWords) == 0 {
		return nil
	}

	actions := make([]string, 0, len(shared.ActionPatterns))
	for a := range shared.ActionPatterns {
		actions = append(actions, a)
	}
	sort.Strings(actions)

	type scored struct {
		action      string
		sharedWords []string
		overlap     int
	}
	var scoredList []scored
	for _, a := range actions {
		actionWords := make(map[string]bool)
		for _, kw := range shared.ActionPatterns[a] {
			for w := range significantWords(kw) {
				actionWords[w] = true
			}
		}
		var sharedWords []string
		for w := range taskWords {
			if actionWords[w] {
				sharedWords = append(sharedWords, w)
			}
		}
		if len(sharedWords) == 0 {
			continue // sin ningún solapamiento no es un candidato, es ruido
		}
		sort.Strings(sharedWords) // Reason reproducible entre corridas
		scoredList = append(scoredList, scored{action: a, sharedWords: sharedWords, overlap: len(sharedWords)})
	}

	// Orden estable por overlap descendente — SliceStable preserva el
	// orden alfabético ya aplicado arriba para los empates, mismo
	// desempate que tenía la versión de un solo candidato.
	sort.SliceStable(scoredList, func(i, j int) bool {
		return scoredList[i].overlap > scoredList[j].overlap
	})

	limit := maxCandidates
	if len(scoredList) < limit {
		limit = len(scoredList)
	}

	candidates := make([]Candidate, 0, limit)
	for _, s := range scoredList[:limit] {
		label, ok := shared.ActionLabels[s.action]
		if !ok {
			label = s.action
		}
		candidates = append(candidates, Candidate{
			Action:     s.action,
			Label:      label,
			Similarity: float64(s.overlap) / float64(len(taskWords)),
			Reason:     "comparte: " + strings.Join(s.sharedWords, ", "),
		})
	}
	return candidates
}

// significantWords normaliza un texto y lo parte en palabras,
// descartando las de menos de 3 letras (artículos, preposiciones) que
// solo ensuciarían la comparación sin aportar señal real.
func significantWords(text string) map[string]bool {
	words := strings.Fields(shared.Normalize(text))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if len(w) < 3 {
			continue
		}
		set[w] = true
	}
	return set
}

// PriorityOrder devuelve las hipótesis ordenadas por DownstreamBlocked
// descendente — el gap que más Tasks deja bloqueadas conviene
// resolverlo primero. Ante empate, conserva el orden original (mismo
// orden en que Knowledge Mapper reportó los Gaps).
func (hs *HypothesisSet) PriorityOrder() []Hypothesis {
	ordered := make([]Hypothesis, len(hs.Hypotheses))
	copy(ordered, hs.Hypotheses)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].DownstreamBlocked > ordered[j].DownstreamBlocked
	})
	return ordered
}

// Describe arma la narrativa: por cada gap, la pista más cercana (o la
// admisión honesta de que no hay ninguna), priorizando primero el gap
// que más desbloquea.
func (hs *HypothesisSet) Describe() string {
	if len(hs.Hypotheses) == 0 {
		return "No tengo gaps para generar hipótesis — no hay territorio nuevo en el plan, o todavía no corriste Knowledge Mapper."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Hipótesis para \"%s\": ", hs.Goal)

	var parts []string
	for _, h := range hs.PriorityOrder() {
		if h.HasLead() {
			line := fmt.Sprintf("%s (%s) → lo más parecido que tengo es %s (%.0f%% de palabras en común)%s",
				h.TaskID, h.TaskDesc, h.ClosestLabel, h.Similarity*100, blockedSuffix(h.DownstreamBlocked))
			if len(h.Alternatives) > 0 {
				line += " — también podría ser " + h.AlternativesLabel()
			}
			parts = append(parts, line)
		} else {
			parts = append(parts, fmt.Sprintf("%s (%s) → sin ninguna pista, es territorio realmente nuevo%s",
				h.TaskID, h.TaskDesc, blockedSuffix(h.DownstreamBlocked)))
		}
	}
	b.WriteString(strings.Join(parts, "; "))
	b.WriteString(".")

	return b.String()
}

// blockedSuffix agrega la nota de prioridad solo cuando aporta algo —
// un gap que no bloquea nada aguas abajo no necesita aclaración extra.
func blockedSuffix(downstreamBlocked int) string {
	if downstreamBlocked == 0 {
		return ""
	}
	if downstreamBlocked == 1 {
		return " (deja 1 tarea bloqueada, prioridad alta)"
	}
	return fmt.Sprintf(" (deja %d tareas bloqueadas, prioridad alta)", downstreamBlocked)
}

// ── Trigger de consulta ──────────────────────────────────────────────

var hypothesisTriggers = []string{
	"que hipotesis tienes para",
	"como podrias resolver",
	"por donde empezarias con",
	"que probarias primero en",
	"que se te ocurre para",
}

// ParseHypothesisQuery detecta si el usuario pide hipótesis de cómo
// resolver un gap — distinto de las cuatro capas anteriores: ninguna de
// ellas propone nada, solo describen (qué es, cómo se divide, cuánto ya
// se sabe, en qué orden); esto es la primera que dice "probá por acá".
func ParseHypothesisQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingHypothesis)
}
