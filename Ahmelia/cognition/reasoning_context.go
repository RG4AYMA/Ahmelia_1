package cognition

import "ahmelia/solving"

// ============================================================================
// REASONING_CONTEXT.GO — Bloque 31, Fase 3.0 (Integración Cognitiva)
// ============================================================================
//
// Hoy (post Bloque 30), Process() tiene CUATRO caminos de salida
// separados dentro de core/engine.go — Knowledge Core, Opinion,
// Cognitive Mind (activación automática, Bloque 30) y Quick Answer —
// cada uno arma su propia respuesta y hace su propio return, sin que
// exista un lugar único por donde toda esa información pase. Antes de
// unificar esos cuatro caminos (Bloque 33, "Pipeline Cognitivo"), hace
// falta el contrato de datos que todos van a compartir — si primero se
// unifica y recién después se define el contrato, el pipeline unificado
// termina moviendo parámetros sueltos (ga, plan, decision, ...) y
// después hay que romperlo para meter esto adentro.
//
// Este bloque es SOLO eso: el contrato. ReasoningContext es el
// contenedor de todo lo que las nueve capas de Fase 3 (Bloques 21-29) y
// el gate de activación automática (Bloque 30) pueden producir para un
// turno de conversación puntual, más el input crudo que las originó.
//
// Lo que este bloque DELIBERADAMENTE NO hace, para no mezclarse con el
// que sigue:
//   - Nadie en core/engine.go construye un ReasoningContext todavía —
//     Process() sigue llamando a goalAnalyzer/problemDecomposer/etc.
//     directo, exactamente como en Bloque 30. Eso es Bloque 33.
//   - No hay Reasoning Cache (Bloque 32) — este contrato no persiste
//     entre turnos, y ni siquiera existe todavía el código que lo
//     llenaría una vez por turno.
//   - No compone lenguaje ni aplica Warmth/Humor/Relationship — eso es
//     Bloque 34 (Response Composer 2.0) y 35, y van a LEER de acá una
//     vez que exista quien lo llene.
//   - No tiene lógica de negocio propia: ningún método interpreta el
//     contenido de sus campos, solo los sostiene. Por eso el test de
//     este bloque es acotado — confirma que el contrato compila, que
//     cognition puede importar solving sin ciclo (solving no importa
//     cognition, se verificó antes de escribir esto), y que el
//     zero-value es seguro de usar.

// ReasoningContext es el estado de razonamiento de UN turno. Vive en
// cognition, no en solving, porque es cognition quien lo va a construir
// y pasar entre etapas del Pipeline (Bloque 11) — solving solo aporta
// los TIPOS de cada capa, no sabe que este contenedor existe.
type ReasoningContext struct {
	// Input es el pedido crudo del usuario que originó este turno —
	// mismo texto que hoy se le pasa a goalAnalyzer.Analyze() en
	// core/engine.go.
	Input string

	// Activated es true si Cognitive Mind llegó a ejecutarse para este
	// turno (equivalente a need.ShouldActivate() en Bloque 30). Un
	// ReasoningContext con Activated=false y el resto de los campos en
	// nil es un turno donde no hizo falta razonar — no es un error ni
	// un contrato a medio llenar.
	Activated bool

	// ReasoningNeed es el puntaje de Bloque 30 — por qué (o por qué no)
	// se activó Cognitive Mind para este turno.
	ReasoningNeed *solving.ReasoningNeed

	// Fase 3 — Cognitive Mind (Bloques 21-29), una capa por campo, en
	// el mismo orden en que se producen en la cadena.
	GoalAnalysis     *solving.GoalAnalysis
	TaskPlan         *solving.TaskPlan
	KnowledgeMap     *solving.KnowledgeMap
	DependencyGraph  *solving.DependencyGraph
	HypothesisSet    *solving.HypothesisSet
	ConstraintReport *solving.ConstraintReport
	Decision         *solving.Decision
	AnalogySet       *solving.AnalogySet
	SelfEvaluation   *solving.SelfEvaluationReport
}

// NewReasoningContext arma un ReasoningContext vacío para un input
// puntual — el resto de los campos se llenan capa por capa a medida
// que se van ejecutando (eso empieza en Bloque 33, no acá).
func NewReasoningContext(input string) *ReasoningContext {
	return &ReasoningContext{Input: input}
}
