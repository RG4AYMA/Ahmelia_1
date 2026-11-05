package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"
)

// ============================================================================
// SKILL_PROCEDURE_EXPLAINER.GO — Bloque 61, primer bloque después del
// cierre de Fase 4.1 (que terminó en el 60, Knowledge Conflict Resolver)
// ============================================================================
//
// Auditoría real que motivó este bloque: Skill.Steps (54) — el
// procedimiento de verdad, no el resumen — nunca se leía en ningún
// lugar del proyecto fuera de skill_builder.go mismo y los bloques que
// solo lo persisten (58/59). Lo mismo con Operations. Confirmado
// también en vivo: "cómo instalo Go", "cuáles son los pasos para
// instalar Go", "muéstrame los pasos de Go" no los maneja ningún
// Source — caen al fallback genérico de razonamiento o a "no
// entendí". Es decir: 56/57 ya podían decir "tengo una Skill
// procedural con 4 pasos", pero nada podía decir CUÁLES eran esos 4
// pasos. Este bloque cierra ese hueco — y solo ese hueco.
//
// LÍMITE HONESTO, deliberado: esto es "decir lo que sabe", no "hacer lo
// que sabe". No toca Operations para intentar ejecutar nada — eso
// queda para un futuro Bloque 62 (Skill Execution; se corre ese número
// del 61 al 62 porque el 61 nunca llegó a tener código real, así que
// renombrarlo ahora no rompe nada). Si la Skill encontrada no tiene
// Steps (State vacío o solo Structured), este bloque lo dice
// honestamente — nunca inventa un procedimiento que la fuente no
// tenía.
//
// "Integrar, no crear": reusa SemanticSkillSearcher (57) para
// encontrar la Skill — no reimplementa ningún matching nuevo. Con más
// de un resultado, no adivina cuál es "el" procedimiento — mismo
// criterio que SkillSearchResult.Describe() (57): lista las opciones
// en vez de elegir arbitrariamente.

// skillProcedureTriggers — frases que piden el procedimiento real, no
// un resumen. Distintas de semanticSkillSearchTriggers (57, "qué
// sabes sobre X" pregunta por CONOCIMIENTO) y de skillBuildTriggers
// (54, "aprende X" dispara salir a Internet) — esto es "ya lo sabés,
// decime cómo se hace", puramente local.
var skillProcedureTriggers = []string{
	"como instalo",
	"como hago",
	"como uso",
	"como usar",
	"como se instala",
	"como se hace",
	"pasos para",
	"cuales son los pasos de",
	"cuales son los pasos para",
	"muestrame los pasos de",
	"dame el procedimiento de",
	"el procedimiento de",
	"explicame como",
	"dame los pasos de",
}

// ParseSkillProcedureQuery detecta si se pide el procedimiento real
// guardado — distinto de ParseSemanticSkillSearchQuery (57, pregunta
// por conocimiento/menciones) y de ParseSkillBuildQuery (54, dispara
// salir a Internet).
func ParseSkillProcedureQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSkillProcedure)
}

// ExtractProcedureTarget devuelve lo que queda después del trigger que
// matcheó — el mismo patrón que dialogue.ExtractPersonName (Social
// Memory) usa para "que sabes de X". Bug real encontrado en vivo: sin
// esto, engine.go le pasaba analysis.MainGoal (goalAnalyzer, Bloque 21)
// a ExplainProcedure — pero goalAnalyzer.stripTrigger() no conoce los
// triggers de ESTE bloque, así que "como instalo go" llegaba a
// Search() como "Como instalo go" completo en vez de solo "go", y
// nunca encontraba la Skill guardada aunque existiera.
func ExtractProcedureTarget(normalizedText string) string {
	for _, trigger := range skillProcedureTriggers {
		if idx := strings.Index(normalizedText, trigger); idx != -1 {
			return strings.TrimSpace(normalizedText[idx+len(trigger):])
		}
	}
	return normalizedText
}

// ExplainProcedure busca la Skill correspondiente a query (reusando
// SemanticSkillSearcher, 57) y arma la narrativa del procedimiento
// real — Steps enumerados, más Prerequisites/Pitfalls/FailureRecovery/
// ExpectedOutcome cuando existan. Nunca inventa pasos: si la Skill
// encontrada no tiene Steps, lo dice explícitamente. Con más de un
// resultado, lista las opciones en vez de elegir una al azar.
func ExplainProcedure(query string, skills *SkillLibrary) string {
	search := NewSemanticSkillSearcher().Search(query, skills)
	if !search.HasMatches() {
		return fmt.Sprintf("No tengo ningún procedimiento guardado sobre \"%s\" todavía.", query)
	}
	if len(search.Matches) > 1 {
		return "Encontré más de una nota relacionada con \"" + query + "\" y no quiero adivinar cuál pedís — " + search.Describe()
	}
	return describeProcedure(search.Matches[0].Note)
}

// describeProcedure arma la narrativa de UNA Skill ya identificada sin
// ambigüedad. Separado de ExplainProcedure para que Skill Execution
// (futuro 62) pueda reusarlo tal cual una vez tenga la Skill en mano,
// sin tener que volver a buscarla.
func describeProcedure(skill *Skill) string {
	if !skill.HasProcedure() {
		// Límite honesto central de este bloque: la fuente no traía
		// una lista de pasos reconocible — no se inventa una. Si al
		// menos hay una condición de uso (StateStructured), se
		// comparte esa, que es información real y no fabricada.
		if len(skill.WhenToUse) > 0 {
			return fmt.Sprintf("Tengo información sobre \"%s\", pero no un procedimiento paso a paso estructurado — lo que sí sé es cuándo se usa: %s.",
				skill.EffectiveLabel(), strings.Join(skill.WhenToUse, "; "))
		}
		return fmt.Sprintf("Tengo información sobre \"%s\", pero no tengo un procedimiento suficientemente estructurado para indicarte pasos — la fuente no traía una lista reconocible.",
			skill.EffectiveLabel())
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Según lo que aprendí sobre \"%s\", el procedimiento es:\n", skill.EffectiveLabel())
	if len(skill.Prerequisites) > 0 {
		fmt.Fprintf(&b, "Antes necesitás: %s.\n", strings.Join(skill.Prerequisites, "; "))
	}
	for i, step := range skill.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step)
	}
	if len(skill.Pitfalls) > 0 {
		fmt.Fprintf(&b, "Ojo con esto: %s.\n", strings.Join(skill.Pitfalls, "; "))
	}
	if len(skill.FailureRecovery) > 0 {
		fmt.Fprintf(&b, "Si algo falla: %s.\n", strings.Join(skill.FailureRecovery, "; "))
	}
	if len(skill.ExpectedOutcome) > 0 {
		fmt.Fprintf(&b, "Resultado esperado: %s.\n", strings.Join(skill.ExpectedOutcome, "; "))
	}
	if len(skill.Limitations) > 0 {
		fmt.Fprintf(&b, "Limitaciones conocidas: %s.\n", strings.Join(skill.Limitations, "; "))
	}
	fmt.Fprintf(&b, "Fuente: %s.", strings.Join(skill.Sources, ", "))
	return strings.TrimRight(b.String(), "\n")
}
