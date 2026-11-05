package solving

import (
	"fmt"
	"strings"
)

// ============================================================================
// REASONING_GATE.GO — Bloque 30, Fase 3.0 (Integración Cognitiva)
// ============================================================================
//
// Hasta acá, Fase 3 (Cognitive Mind, Bloques 21-29) solo se activaba
// con una meta-pregunta exacta ("descomponé esta tarea"). Este bloque
// conecta las nueve capas al flujo normal: si Ahmelia no sabe responder
// algo (knowledgeEngine.Query devuelve KindUnknown, ver core/engine.go),
// antes de admitir que no sabe, evalúa si es un objetivo complejo que
// Cognitive Mind puede planificar en vez de una pregunta que
// simplemente no tiene respuesta.
//
// El gate NO es un booleano fijo (dominio reconocido sí/no) — es un
// puntaje, a propósito: hoy Fase 3 es la única capa de razonamiento
// que existe, pero el roadmap ya contempla más (multimedia, ingeniería,
// robótica...). Un puntaje puede crecer con cada dominio nuevo sin que
// el gate en sí necesite reescribirse — cada fase futura solo suma sus
// propios puntos a la misma cuenta.
//
// NOTA sobre alcance de este bloque: esto activa Cognitive Mind
// automáticamente, pero NO cachea nada entre capas ni comparte un
// "Reasoning Context" — cada activación recorre la cadena completa
// desde cero, mismo costo que ya tenían las nueve meta-preguntas de
// Bloques 21-29. Esa optimización queda para un bloque aparte
// (Reasoning Cache), mismo criterio que Context Builder (Bloque 9) →
// Context Cache (Bloque 10) en Fase 2.6.

// ReasoningThreshold es el puntaje mínimo para activar Cognitive Mind
// automáticamente. Exportado (no una constante interna) para que se
// pueda ajustar sin tocar la lógica de scoring si en la práctica
// resulta muy sensible o muy estricto.
const ReasoningThreshold = 2

// NeedSignal es UNA razón concreta que sumó puntos — igual que
// ModeScore (Bloque 16) y LayerEvaluation (Bloque 29): nunca un
// puntaje sin justificar.
type NeedSignal struct {
	Reason string
	Points int
}

// ReasoningNeed es el resultado de puntuar un GoalAnalysis. El nombre
// es deliberado: no mide "¿activo sí/no?", mide CUÁNTO necesita pensar
// Ahmelia antes de responder — la misma cuenta que, en fases futuras
// (multimedia, ingeniería, simulación...), podría decidir no solo si
// activar Cognitive Mind sino qué otras mentes activar además. Ese
// diseño más amplio queda fuera de este bloque a propósito — hoy solo
// existe Fase 3, así que el único destino posible es Cognitive Mind.
type ReasoningNeed struct {
	Score   int
	Signals []NeedSignal
}

// ShouldActivate es el atajo booleano sobre el puntaje.
func (n *ReasoningNeed) ShouldActivate() bool {
	return n.Score >= ReasoningThreshold
}

// Describe arma una línea legible del puntaje — para logging/debug y
// para que Cognitive Mind, cuando responde, pueda explicar por qué se
// activó sin que sea una caja negra.
func (n *ReasoningNeed) Describe() string {
	if len(n.Signals) == 0 {
		return fmt.Sprintf("puntaje %d, sin señales", n.Score)
	}
	reasons := make([]string, len(n.Signals))
	for i, s := range n.Signals {
		reasons[i] = fmt.Sprintf("%s (+%d)", s.Reason, s.Points)
	}
	return fmt.Sprintf("puntaje %d: %s", n.Score, strings.Join(reasons, ", "))
}

// ScoreReasoningNeed puntúa un GoalAnalysis ya calculado (no vuelve a
// analizar el texto — reusa lo que goalAnalyzer.Analyze() ya dejó,
// mismo criterio "integrar, no crear" del resto de Fase 3).
//
// Criterios (documentados en detalle porque son ajustables, no una
// fórmula cerrada):
//   - Dominio específico reconocido (no "general"): +2. Señal fuerte —
//     Goal Analyzer encontró vocabulario real de un dominio conocido.
//   - Cada Requirement detectado: +1. Un pedido con requisitos
//     implícitos ("rápido", "seguro") es un pedido con restricciones
//     reales, aunque el dominio no haya matcheado.
//   - Objetivo largo (≥8 palabras en el pedido original): +1. Un
//     pedido corto ("hola", "qué hora es") casi nunca es un objetivo
//     complejo; uno largo tiene más chance de serlo. Deliberadamente
//     NO se usa "más de un subobjetivo": GoalAnalysis.Subgoals siempre
//     tiene contenido (mínimo la plantilla genérica de 4 pasos), así
//     que ese conteo no distingue nada — activaría con cualquier input.
func ScoreReasoningNeed(ga *GoalAnalysis) *ReasoningNeed {
	need := &ReasoningNeed{}
	if ga == nil {
		return need
	}

	if ga.Domain != "" && ga.Domain != "general" {
		need.Signals = append(need.Signals, NeedSignal{
			Reason: fmt.Sprintf("dominio reconocido: %s", ga.Domain),
			Points: 2,
		})
		need.Score += 2
	}

	for _, r := range ga.Requirements {
		need.Signals = append(need.Signals, NeedSignal{
			Reason: "requisito detectado: " + r.Description,
			Points: 1,
		})
		need.Score++
	}

	if words := len(strings.Fields(ga.Request)); words >= 8 {
		need.Signals = append(need.Signals, NeedSignal{
			Reason: fmt.Sprintf("objetivo largo (%d palabras)", words),
			Points: 1,
		})
		need.Score++
	}

	return need
}
