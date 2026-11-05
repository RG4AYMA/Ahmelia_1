package solving

import (
	"fmt"
	"sort"
	"strings"

	"ahmelia/shared"
)

// ============================================================================
// GOAL_SKILL_RESOLVER.GO — Bloque 62, primer bloque de la Fase 6
// (Conocimiento → Capacidad)
// ============================================================================
//
// Motivación real: 54–61 construyeron un sistema completo de
// aprendizaje, memoria y explicación de conocimiento, pero cada acceso
// a él depende de un trigger textual fijo ("como instalo", "que sabes
// sobre"...). Este bloque no reemplaza esos triggers — los complementa
// con una vía de entrada distinta: partir de un pedido de ACCIÓN
// ("instala Go", "crea una carpeta llamada proyectos") y resolver qué
// capacidad concreta hace falta, sin exigir que el usuario use una de
// las frases ya cableadas de los bloques anteriores.
//
// LÍMITE HONESTO, deliberado y central: este bloque NUNCA ejecuta nada.
// Solo responde "¿qué es lo que se necesita, y qué tan preparada está
// Ahmelia para eso?". Ejecutar (Skill Execution) es un bloque futuro
// (63, todavía sin una sola línea de código) — este resolver es
// justamente el primer paso necesario antes de que eso pueda existir
// con sentido: antes de ejecutar algo hace falta saber QUÉ ejecutar y
// si Ahmelia realmente tiene con qué.
//
// "Integrar, no crear" — nada de esto reimplementa matching que ya
// existe:
//   - Analyzer (Goal Analyzer, Bloque 21) separa el objetivo real de
//     las muletillas de conversación ("quiero que...", "podrías
//     ayudarme a...") — MainGoal ya viene limpio.
//   - startsWithActionVerb / actionVerbs (Bloque 54) — el mismo
//     vocabulario que ya se usa para reconocer una línea imperativa
//     dentro de un procedimiento se reusa acá para reconocer un pedido
//     de acción en la petición completa del usuario.
//   - shared.ActionPatterns + shared.WiredActions — para saber si el
//     pedido ya matchea una acción local que Ahmelia puede ejecutar
//     HOY, sin necesitar ninguna Skill aprendida.
//   - detectOperations (Bloque 54) — para clasificar la capacidad
//     pedida contra el mismo vocabulario Wired/Named/Unknown que ya se
//     usa para las Operations de una Skill. Un solo lugar que sabe
//     hacer esa clasificación, no dos versiones que puedan
//     desalinearse.
//   - SemanticSkillSearcher (Bloque 57) — para buscar contra la
//     memoria local ya aprendida, mismo criterio de "con más de un
//     match, no elegir arbitrariamente" que ya aplican 57 y 61.
//   - Skill.HasProcedure() / CapabilityCoverage() (Bloque 54) — para
//     la distinción central "conocimiento general vs. procedimiento
//     vs. capacidad respaldada", que ya existía y este es el primer
//     bloque en consumirla desde afuera de 54/55.

// GoalResolutionKind clasifica honestamente qué tan preparada está
// Ahmelia frente a la capacidad pedida. No es un score inventado — son
// categorías discretas derivadas de datos reales (existe una acción
// local Wired, existe una Skill, tiene Steps, cuántas Operations están
// Wired).
type GoalResolutionKind string

const (
	// KindLocalAction — el pedido ya matchea una acción local Wired
	// (shared.WiredActions): no hace falta ninguna Skill aprendida,
	// Ahmelia ya sabe hacer esto de punta a punta hoy mismo.
	KindLocalAction GoalResolutionKind = "accion_local_disponible"
	// KindProceduralFull — existe una Skill con Steps y TODAS las
	// Operations que detectó tienen respaldo Wired.
	KindProceduralFull GoalResolutionKind = "capacidad_completa"
	// KindProceduralPartial — existe una Skill con Steps, pero solo
	// ALGUNAS de sus Operations están Wired.
	KindProceduralPartial GoalResolutionKind = "capacidad_parcial"
	// KindProceduralUnbacked — existe una Skill con Steps pero NINGUNA
	// Operation está Wired: Ahmelia sabe el procedimiento, no puede
	// ejecutar ninguna parte todavía.
	KindProceduralUnbacked GoalResolutionKind = "procedimiento_sin_respaldo"
	// KindKnowledgeOnly — existe una Skill pero sin Steps: solo
	// conocimiento general sobre el tema, ni procedimiento ni acción.
	KindKnowledgeOnly GoalResolutionKind = "solo_conocimiento"
	// KindAmbiguous — más de una Skill matcheó y no hay forma honesta
	// de elegir una sin arbitrariedad.
	KindAmbiguous GoalResolutionKind = "ambiguo"
	// KindUnknown — ni una acción local ni ninguna Skill guardada
	// respaldan el pedido.
	KindUnknown GoalResolutionKind = "sin_conocimiento"
)

// GoalResolution es el resultado de resolver un pedido en lenguaje
// natural contra lo que Ahmelia YA tiene — capacidades locales
// (actions/*, vía shared.WiredActions) y Skills aprendidas
// (skill_library.json). Nunca contiene una decisión de ejecutar: eso
// es responsabilidad de un bloque futuro (Skill Execution).
type GoalResolution struct {
	Request     string // pedido original, sin procesar
	Capability  string // frase de capacidad extraída (MainGoal ya limpio de muletillas)
	Kind        GoalResolutionKind
	LocalAction string       // solo si Kind == KindLocalAction
	Skill       *Skill       // solo si se resolvió a una única Skill
	Ambiguous   []SkillMatch // solo si Kind == KindAmbiguous
	WiredOps    int          // solo si Skill != nil && Skill.HasProcedure()
	TotalOps    int          // ídem
}

// ParseGoalCapabilityQuery detecta si el texto TIENE FORMA de pedido de
// acción — deliberadamente NO es una lista de frases fijas (a
// diferencia de los triggers de 54/57/61): reusa startsWithActionVerb,
// el mismo chequeo léxico que ya usa el Bloque 54 para reconocer una
// línea imperativa dentro de un procedimiento fuente. Aplicado acá a
// la petición completa del usuario, cumple el requisito de "no
// depender de una frase exacta" sin inventar un segundo vocabulario.
//
// Conservador a propósito: solo mira si el pedido, una vez sacadas las
// muletillas de cortesía habituales (mismo stripFillers que usa
// Analyze()), ARRANCA con un verbo imperativo reconocido. Un verbo
// mencionado en cualquier lugar de una oración larga no alcanza — eso
// evitaría que este bloque le gane el turno a rutas más específicas ya
// cableadas (Semantic Search, Skill Procedure, Update Check...) que se
// consultan antes en engine.go.
func ParseGoalCapabilityQuery(normalizedText string) bool {
	clean := stripFillers(normalizedText)
	return startsWithActionVerb(clean)
}

// extractGoalTarget saca el verbo imperativo inicial (mismo
// vocabulario actionVerbs de 54) de un MainGoal ya normalizado,
// dejando solo el objeto — mismo patrón que ExtractProcedureTarget
// (61) y ExtractPersonName (Social Memory). Necesario porque
// SemanticSkillSearcher.Search (57) matchea el query COMPLETO como
// substring literal contra Content/Label — buscar la frase entera
// ("instala go") casi nunca aparece tal cual en una fuente real
// ("Install Go"), pero el objeto solo ("go") sí. Sin esto, ResolveGoal
// prácticamente nunca encontraría una Skill ya guardada.
func extractGoalTarget(normalizedMainGoal string) string {
	for _, v := range actionVerbs {
		if normalizedMainGoal == v {
			return ""
		}
		if strings.HasPrefix(normalizedMainGoal, v+" ") {
			return strings.TrimSpace(normalizedMainGoal[len(v)+1:])
		}
	}
	return normalizedMainGoal
}

// ResolveGoal es el punto de entrada del bloque. Recibe el pedido tal
// cual llegó (sin normalizar — Analyzer.Analyze necesita el texto
// original para que Capability quede legible) y la SkillLibrary local.
// Nunca ejecuta nada — solo clasifica.
func ResolveGoal(request string, skills *SkillLibrary) *GoalResolution {
	analysis := NewAnalyzer().Analyze(request)
	capability := strings.TrimSpace(analysis.MainGoal)
	if capability == "" {
		capability = strings.TrimSpace(request)
	}

	res := &GoalResolution{Request: request, Capability: capability}

	// 1. ¿Ya es una acción local Wired? Si sí, no hace falta ninguna
	// Skill — es la resolución más fuerte posible: Ahmelia ya sabe
	// hacer esto de punta a punta hoy. Orden estable (ActionPatterns
	// es un map) para no depender del orden de iteración de Go.
	normCap := shared.Normalize(capability)
	actionKeys := make([]string, 0, len(shared.ActionPatterns))
	for k := range shared.ActionPatterns {
		actionKeys = append(actionKeys, k)
	}
	sort.Strings(actionKeys)
	for _, action := range actionKeys {
		if !shared.WiredActions[action] {
			continue // Named-pero-no-Wired (ej. renombrar_archivo) no cuenta acá — cae a búsqueda de Skill.
		}
		for _, trigger := range shared.ActionPatterns[action] {
			if strings.Contains(normCap, shared.Normalize(trigger)) {
				res.Kind = KindLocalAction
				res.LocalAction = action
				return res
			}
		}
	}

	// 2. Buscar contra la memoria de Skills aprendidas — reusa 57 tal
	// cual, mismo criterio de no elegir arbitrariamente con ambigüedad.
	// Se busca por el OBJETO (target), no la frase completa — ver
	// extractGoalTarget.
	target := extractGoalTarget(normCap)
	search := NewSemanticSkillSearcher().Search(target, skills)
	switch {
	case !search.HasMatches():
		res.Kind = KindUnknown
		return res
	case len(search.Matches) > 1:
		res.Kind = KindAmbiguous
		res.Ambiguous = search.Matches
		return res
	}

	skill := search.Matches[0].Note
	res.Skill = skill

	if !skill.HasProcedure() {
		res.Kind = KindKnowledgeOnly
		return res
	}

	wired, total := skill.CapabilityCoverage()
	res.WiredOps, res.TotalOps = wired, total
	switch {
	case wired == total && total > 0:
		res.Kind = KindProceduralFull
	case wired == 0:
		res.Kind = KindProceduralUnbacked
	default:
		res.Kind = KindProceduralPartial
	}
	return res
}

// Describe arma la narrativa honesta del resultado — nunca sugiere que
// algo se ejecutó ni que Ahmelia "ya lo hizo". Distingue con claridad
// las seis categorías de GoalResolutionKind.
// describeOperationBreakdown arma el detalle por operación individual
// — Text → Wired/Named/Unknown — la pieza que faltaba para completar
// lo que Skill.Operations (54) ya calculaba pero nunca se mostraba
// desglosado, solo como conteo agregado (WiredOps/TotalOps). No
// recalcula nada — reusa exactamente los mismos Operation ya
// detectados por detectOperations() (54).
func (r *GoalResolution) describeOperationBreakdown() string {
	if r.Skill == nil || len(r.Skill.Operations) == 0 {
		return ""
	}
	var lines []string
	for _, op := range r.Skill.Operations {
		lines = append(lines, fmt.Sprintf("  - %s → %s", op.Text, operationStatusLabel(op)))
	}
	return "\n" + strings.Join(lines, "\n")
}

// operationStatusLabel traduce el OperationStatus crudo (54) a una
// frase — mismo vocabulario Wired/Named/Unknown, sin inventar un
// cuarto estado. Para Wired, además suma el SafetyTier (65) — recién
// ahí importa: Named/Unknown no son ejecutables, así que su tier de
// seguridad es irrelevante todavía.
func operationStatusLabel(op Operation) string {
	switch op.Status {
	case OpWired:
		tier := shared.SafetyTierOf(op.Action)
		if tier == "" {
			return "reconocida, con acción conectada (Wired) — sin clasificar en seguridad, pediría confirmación igual"
		}
		return fmt.Sprintf("reconocida, con acción conectada (Wired, riesgo: %s)", tier)
	case OpNamed:
		return "reconocida por nombre, sin acción conectada (Named)"
	default:
		return "no reconocida (Unknown)"
	}
}

func (r *GoalResolution) Describe() string {
	// Integration Fix (Skill Model Refinement) — antes de decir "puedo
	// ejecutar esto" hace falta avisar si la propia fuente declaró
	// límites de alcance ("no funciona en X", "no garantiza Y") — se
	// agrega como sufijo a los tres casos donde de verdad importa (los
	// que mencionan operaciones ejecutables); KindKnowledgeOnly y
	// KindUnknown no ofrecen ejecutar nada, así que no hace falta ahí.
	limitationsNote := func() string {
		if r.Skill != nil && len(r.Skill.Limitations) > 0 {
			return " Ojo: " + strings.Join(r.Skill.Limitations, "; ") + "."
		}
		return ""
	}

	switch r.Kind {
	case KindLocalAction:
		return fmt.Sprintf("Puedo hacer eso directamente — %q ya es una capacidad local disponible.", r.Capability)

	case KindProceduralFull:
		return fmt.Sprintf(
			"Tengo un procedimiento aprendido para %q (%s) y reconozco TODAS sus %d operación(es) como algo que sé ejecutar. Todavía no lo ejecuté — decime si querés que lo intente.%s%s",
			r.Capability, r.Skill.EffectiveLabel(), r.TotalOps, limitationsNote(), r.describeOperationBreakdown(),
		)

	case KindProceduralPartial:
		return fmt.Sprintf(
			"Tengo un procedimiento aprendido para %q (%s), pero solo reconozco %d de %d operación(es) como algo que sé ejecutar — el resto lo puedo explicar, no ejecutar todavía.%s%s",
			r.Capability, r.Skill.EffectiveLabel(), r.WiredOps, r.TotalOps, limitationsNote(), r.describeOperationBreakdown(),
		)

	case KindProceduralUnbacked:
		return fmt.Sprintf(
			"Sé el procedimiento para %q (%s) — lo puedo explicar paso a paso — pero hoy no tengo ninguna operación conectada para ejecutar ninguna parte.%s%s",
			r.Capability, r.Skill.EffectiveLabel(), limitationsNote(), r.describeOperationBreakdown(),
		)

	case KindKnowledgeOnly:
		return fmt.Sprintf(
			"Tengo información sobre %q, pero no un procedimiento estructurado — sé algo del tema, no cómo hacerlo.",
			r.Skill.EffectiveLabel(),
		)

	case KindAmbiguous:
		var b strings.Builder
		fmt.Fprintf(&b, "Encontré más de una nota relacionada con %q y no quiero adivinar cuál es — ", r.Capability)
		b.WriteString((&SkillSearchResult{Query: r.Capability, Matches: r.Ambiguous}).Describe())
		return b.String()

	default: // KindUnknown
		return fmt.Sprintf("No tengo ninguna capacidad local ni ningún procedimiento aprendido para %q todavía.", r.Capability)
	}
}
