package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	"ahmelia/shared"
)

// ============================================================================
// GOAL_ANALYZER.GO — Bloque 21, Fase 3 (Cognitive Mind), Capa 3.1
// ============================================================================
//
// Paquete NUEVO (solving/), a propósito distinto de reasoning/. reasoning/
// ya existe y es RETROSPECTIVO: mira acciones que Ahmelia YA ejecutó.
//
//   reasoning/hypothesis.go  → por qué YA falló una acción pasada
//   reasoning/decision.go    → confianza en una acción YA ejecutada
//   reasoning/evaluator.go   → problemas de comportamiento YA observados
//   reasoning/critic.go      → revisión post-mortem de una acción YA hecha
//
// Goal Analyzer es PROSPECTIVO: parte de una petición que el usuario
// TODAVÍA no pidió ejecutar, y la convierte en algo sobre lo que se
// puede razonar (objetivo + subobjetivos + requisitos + supuestos). Es
// el primer nodo del grafo de Fase 3 — todo lo que sigue (Problem
// Decomposer, Knowledge Mapper, Dependency Solver, ...) parte de acá.
// Por eso no comparte paquete ni tipos con reasoning/: son dos
// preguntas distintas ("¿qué pasó?" vs. "¿qué habría que hacer?").

// Requirement es algo que la solución va a necesitar cumplir. Implicit
// distingue lo que el usuario pidió literalmente ("que sea rápido") de
// lo que Ahmelia infiere aunque nadie lo haya dicho con esas palabras.
type Requirement struct {
	Description string
	Implicit    bool
}

// GoalAnalysis es el resultado de analizar una petición: el objetivo
// principal, en qué subobjetivos se divide, qué requisitos tiene y qué
// supuestos hizo Ahmelia por falta de información explícita.
type GoalAnalysis struct {
	Request      string // la petición original, sin procesar
	Domain       string // "software", "videojuego", "automatizacion", "investigacion", "documentacion", o "general" si no matcheó ninguno
	MainGoal     string
	Subgoals     []string
	Requirements []Requirement
	Assumptions  []string
}

// domainTemplate asocia palabras clave con la forma típica de dividir
// ese tipo de pedido en subobjetivos. No pretende ser exhaustivo — es
// un punto de partida razonable que Knowledge Mapper (Capa 3.3, futuro
// bloque) va a poder refinar con lo que Ahmelia sepa del contexto real.
type domainTemplate struct {
	Keywords []string
	Subgoals []string
}

var domainTemplates = map[string]domainTemplate{
	"software": {
		Keywords: []string{"app", "aplicacion", "programa", "sistema", "script", "codigo", "software", "api", "web"},
		Subgoals: []string{
			"Definir alcance y funcionalidades",
			"Diseñar la arquitectura",
			"Implementar el código",
			"Probar",
			"Documentar",
		},
	},
	"videojuego": {
		Keywords: []string{"juego", "videojuego", "unity", "unreal", "godot", "nivel", "personaje jugable"},
		Subgoals: []string{
			"Definir mecánicas y género",
			"Diseñar niveles y arte",
			"Implementar la lógica de juego",
			"Probar jugabilidad",
			"Pulir y optimizar",
		},
	},
	"automatizacion": {
		Keywords: []string{"automatizar", "automatizacion", "tarea repetitiva", "flujo automatico", "bot"},
		Subgoals: []string{
			"Identificar el proceso manual actual",
			"Diseñar el flujo automatizado",
			"Implementar",
			"Validar contra el proceso manual",
		},
	},
	"investigacion": {
		Keywords: []string{"investigar", "investigacion", "comparar opciones", "analizar alternativas", "evaluar opciones"},
		Subgoals: []string{
			"Definir las preguntas de investigación",
			"Recolectar fuentes y datos",
			"Analizar y comparar",
			"Sacar conclusiones",
		},
	},
	"documentacion": {
		Keywords: []string{"documentar", "documentacion", "manual", "readme", "instructivo"},
		Subgoals: []string{
			"Relevar qué hay que documentar",
			"Definir la estructura del documento",
			"Redactar",
			"Revisar",
		},
	},
}

// genericSubgoals se usa cuando ninguna plantilla de dominio matchea —
// no significa que Ahmelia no entendió, significa que el pedido no cae
// en ningún dominio conocido todavía.
var genericSubgoals = []string{
	"Aclarar el objetivo exacto",
	"Identificar los pasos necesarios",
	"Ejecutar",
	"Verificar el resultado",
}

// requirementRules detecta requisitos IMPLÍCITOS a partir de palabras
// sueltas en el pedido — el usuario no dijo "requisito: que sea rápido",
// dijo "rápido", y de ahí se infiere el requisito.
var requirementRules = []struct {
	Keywords    []string
	Requirement string
}{
	{[]string{"rapido", "urgente", "lo antes posible", "ya mismo"}, "Priorizar velocidad de entrega sobre exhaustividad"},
	{[]string{"muchos usuarios", "escalable", "escalabilidad", "alta demanda"}, "Debe soportar escala (más de un usuario o carga concurrente)"},
	{[]string{"seguro", "seguridad", "proteger"}, "Contemplar seguridad desde el diseño, no como agregado posterior"},
	{[]string{"sin internet", "offline", "sin conexion"}, "Debe funcionar sin conexión a Internet"},
	{[]string{"gratis", "gratuito", "sin costo", "sin pagar"}, "Sin dependencias de pago"},
	{[]string{"multiplataforma", "cualquier sistema", "windows y mac", "windows y linux"}, "Debe funcionar en más de un sistema operativo"},
}

// Analyzer es el Goal Analyzer en sí. No guarda estado entre llamadas —
// cada Analyze() es independiente, como los DescribeX() de
// memory_intelligence.
type Analyzer struct{}

// NewAnalyzer arma el Goal Analyzer. Sin dependencias — es, a propósito,
// el único nodo de Fase 3 sin nada que consultar antes de correr (por
// eso es el punto de partida del grafo, igual que Dependency Solver, la
// Capa 3.4, lo va a marcar como nodo raíz).
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Analyze convierte una petición en lenguaje natural en un
// GoalAnalysis. Recibe el texto ORIGINAL (con acentos/mayúsculas), no
// normalizado — MainGoal necesita quedar legible, y Normalize() sirve
// solo internamente para detectar keywords sin que un acento le arruine
// el match.
func (a *Analyzer) Analyze(request string) *GoalAnalysis {
	clean := strings.TrimSpace(request)
	clean = stripTrigger(clean)
	norm := shared.Normalize(clean)

	ga := &GoalAnalysis{Request: clean}

	ga.MainGoal = stripFillers(clean)
	ga.Domain = detectDomain(norm)

	if tmpl, ok := domainTemplates[ga.Domain]; ok {
		ga.Subgoals = append([]string{}, tmpl.Subgoals...)
	} else {
		ga.Subgoals = append([]string{}, genericSubgoals...)
	}

	ga.Requirements = detectRequirements(norm)
	ga.Assumptions = buildAssumptions(ga)

	return ga
}

// stripTrigger saca el envoltorio de meta-pregunta ("analiza este
// objetivo: ...", "descompone en tareas: ...", "que sabes hacer de...")
// si está presente, dejando solo el pedido real que hay que analizar.
// Contempla los triggers de Goal Analyzer, Problem Decomposer (Bloque
// 22), Knowledge Mapper (Bloque 23), Dependency Solver (Bloque 24),
// Hypothesis Generator (Bloque 25), Constraint Solver (Bloque 26),
// Decision Engine (Bloque 27), Analogical Reasoning (Bloque 28), Self
// Evaluation (Bloque 29) y Hypothesis Comparator (Bloque 38) — las diez
// llaman a Analyze() con el input crudo completo, así que las diez
// necesitan que esto les saque el envoltorio antes de armar MainGoal.
// Si no hay trigger, devuelve el texto sin tocar — Analyze() también se
// puede llamar directo con un pedido normal.
func stripTrigger(text string) string {
	// GLM 20/10: bytes invalidos para UTF-8 (una linea de stdin con
	// encoding local CP1252) se decodifican como RuneError y ToLower
	// re-encoda cada byte invalido como U+FFFD de 3 bytes: lower
	// crece sobre text, el corte text=cut sale de rango y el panic
	// mato el proceso entero (run 4 de compuerta V6: text[20:] sobre
	// len=17). Sanitizado temprano los offsets quedan auto alineados;
	// el clamp del loop es refuerzo defensivo.
	text = strings.ToValidUTF8(text, "\uFFFD")
	lower := strings.ToLower(text)
	all := append(append([]string{}, goalAnalysisTriggers...), decomposeTriggers...)
	all = append(all, knowledgeMapTriggers...)
	all = append(all, dependencySolveTriggers...)
	all = append(all, hypothesisTriggers...)
	all = append(all, constraintTriggers...)
	all = append(all, decisionTriggers...)
	all = append(all, analogyTriggers...)
	all = append(all, selfEvalTriggers...)
	all = append(all, comparisonTriggers...)
	all = append(all, strategyTriggers...)
	all = append(all, adaptiveConfidenceTriggers...)
	all = append(all, reflectionTriggers...)
	all = append(all, counterfactualTriggers...)
	all = append(all, causalExplainTriggers...)
	all = append(all, causalProjectTriggers...)
	all = append(all, failureTriggers...)
	all = append(all, learningIntegrationTriggers...)
	all = append(all, curriculumTriggers...)
	all = append(all, validatorTriggers...)
	all = append(all, knowledgeMemoryTriggers...)
	all = append(all, plannerTriggers...)
	all = append(all, discoveryTriggers...)
	all = append(all, sourceValidationTriggers...)
	all = append(all, skillBuilderTriggers...)
	all = append(all, skillBuildTriggers...)
	all = append(all, skillContextTriggers...)
	all = append(all, semanticSkillSearchTriggers...)
	all = append(all, updateCheckTriggers...)
	all = append(all, skillVersionUpdateTriggers...)
	all = append(all, skillVersionHistoryTriggers...)
	all = append(all, conflictTriggers...)
	for _, trigger := range all {
		idx := strings.Index(lower, trigger)
		if idx == -1 {
			continue
		}
		// GLM 20/10 (run real de headless compuerta V6 real): el corte
		// asumía que lower y text comparten offsets byte a byte; con
		// texto cuyo ToLower cambia el largo (o triggers más largos que
		// el resto) quedaba text[20:] sobre len=17 -> panic del proceso
		// entero (fall-stop, no fail-closed). Fix: clamped + continue.
		cut := idx + len(trigger)
		if cut < 0 || cut > len(text) {
			continue
		}
		rest := strings.TrimSpace(text[cut:])
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(strings.ToLower(rest), "de ") {
			rest = rest[3:]
		}
		return strings.TrimSpace(rest)
	}
	return text
}

// stripFillers saca las muletillas típicas de un pedido ("quiero",
// "necesito", "podrías ayudarme a"...) para que MainGoal quede como una
// frase directa en vez de repetir cómo lo pidió el usuario. Case
// insensitive pero preserva el resto de la frase tal cual vino.
func stripFillers(text string) string {
	fillers := []string{
		"quiero que ", "quiero ", "necesito que ", "necesito ",
		"quisiera ", "podrias ayudarme a ", "podrías ayudarme a ",
		"ayudame a ", "ayúdame a ", "me gustaria ", "me gustaría ",
		"hazme ", "hacéme ", "creame ", "creáme ", "puedes ", "podes ",
	}
	lower := strings.ToLower(text)
	for _, f := range fillers {
		if strings.HasPrefix(lower, f) {
			text = text[len(f):]
			lower = lower[len(f):]
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// detectDomain cuenta matches de keywords por plantilla y devuelve la
// que más coincidencias tuvo. Ante empate o cero matches, "general".
func detectDomain(normalizedText string) string {
	best := "general"
	bestScore := 0
	for domain, tmpl := range domainTemplates {
		score := 0
		for _, kw := range tmpl.Keywords {
			if strings.Contains(normalizedText, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = domain
		}
	}
	return best
}

func detectRequirements(normalizedText string) []Requirement {
	var reqs []Requirement
	for _, rule := range requirementRules {
		for _, kw := range rule.Keywords {
			if strings.Contains(normalizedText, kw) {
				reqs = append(reqs, Requirement{Description: rule.Requirement, Implicit: true})
				break
			}
		}
	}
	return reqs
}

// buildAssumptions lista lo que Ahmelia está asumiendo por AUSENCIA de
// información, no por presencia de una palabra clave. Es al revés de
// detectRequirements: ahí importa qué SÍ se dijo, acá importa qué NO.
func buildAssumptions(ga *GoalAnalysis) []string {
	var assumptions []string

	if ga.Domain == "general" {
		assumptions = append(assumptions, "No se detectó un dominio específico (software/videojuego/automatización/etc.) — se trata como un pedido genérico")
	}

	hasDeadline := false
	for _, w := range []string{"para hoy", "para mañana", "antes de", "fecha limite", "deadline", "en X dias"} {
		if strings.Contains(shared.Normalize(ga.Request), shared.Normalize(w)) {
			hasDeadline = true
			break
		}
	}
	if !hasDeadline {
		assumptions = append(assumptions, "No se mencionó un plazo — se asume sin fecha límite específica")
	}

	if ga.Domain == "software" {
		norm := shared.Normalize(ga.Request)
		platformMentioned := strings.Contains(norm, "windows") || strings.Contains(norm, "mac") ||
			strings.Contains(norm, "linux") || strings.Contains(norm, "web") || strings.Contains(norm, "movil") ||
			strings.Contains(norm, "android") || strings.Contains(norm, "ios")
		if !platformMentioned {
			assumptions = append(assumptions, "No se especificó plataforma — se asume escritorio/Go, en línea con el resto del proyecto Ahmelia")
		}
	}

	return assumptions
}

// Describe arma la narrativa legible del análisis completo — pensada
// para responder a "analiza este objetivo: ...".
func (ga *GoalAnalysis) Describe() string {
	if strings.TrimSpace(ga.MainGoal) == "" {
		return "¿Qué objetivo querés que analice? Decime la petición completa (ej: \"analiza este objetivo: crear una app de tareas\")."
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Objetivo principal: %s (dominio: %s). ", ga.MainGoal, ga.Domain)

	if len(ga.Subgoals) > 0 {
		b.WriteString("Subobjetivos: ")
		b.WriteString(strings.Join(ga.Subgoals, "; "))
		b.WriteString(". ")
	}

	if len(ga.Requirements) > 0 {
		var reqs []string
		for _, r := range ga.Requirements {
			reqs = append(reqs, r.Description)
		}
		b.WriteString("Requisitos detectados: ")
		b.WriteString(strings.Join(reqs, "; "))
		b.WriteString(". ")
	}

	if len(ga.Assumptions) > 0 {
		b.WriteString("Supuestos: ")
		b.WriteString(strings.Join(ga.Assumptions, "; "))
		b.WriteString(".")
	}

	return strings.TrimSpace(b.String())
}

// ── Trigger de consulta ──────────────────────────────────────────────

var goalAnalysisTriggers = []string{
	"analiza este objetivo",
	"analiza esta peticion",
	"analiza este pedido",
	"descompone este objetivo",
	"descompon este objetivo",
	"cuales son los subobjetivos",
	"que necesitaria para",
	"como abordarias esto",
	"analiza esta idea",
}

// ParseGoalAnalysisQuery detecta si el usuario está pidiendo el análisis
// explícitamente (vs. una petición normal de ejecución, que todavía no
// pasa por Goal Analyzer — eso es integración de un bloque futuro).
// Solo devuelve si el trigger está presente; extraer y limpiar el texto
// a analizar es responsabilidad de Analyze() (recibe el input crudo
// completo, incluyendo el trigger, y stripFillers ya sabe recortar
// muletillas — no hace falta duplicar esa lógica acá).
func ParseGoalAnalysisQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingGoalAnalysis)
}
