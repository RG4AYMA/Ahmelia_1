package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"
)

// ============================================================================
// ADAPTIVE_CONFIDENCE.GO — Bloque 40, Fase 3 Profundizada (Nivel II)
// ============================================================================
//
// Confidence Calibration (Bloque 37) calculó un número real —
// Decision.Confidence, 0..1 — pero hasta este bloque ese número era
// puramente descriptivo: aparecía en Describe() y ahí se quedaba,
// nadie lo usaba para decidir nada distinto. Es exactamente el mismo
// patrón que Formality en Bloque 35 (Fase 2.8) antes de que Warmth e
// Initiative se aplicaran de verdad: un valor real, calculado con
// datos reales, que no cambiaba ni una sola decisión.
//
// Adaptive Confidence es el primero que USA Confidence para decidir
// CÓMO comportarse, no solo qué decir. Tres bandas, tomadas del
// roadmap tal como se propuso (>0.85 responder, ~0.55 explorar segunda
// hipótesis, ~0.25 pedir más información), convertidas acá en una
// partición de dos umbrales sin huecos entre bandas:
//
//   - Confidence >= 0.85           → Responder directo.
//   - 0.4 <= Confidence < 0.85     → Explorar alternativa antes de
//     comprometerse del todo (el 0.55 del roadmap cae adentro de esta
//     banda; 0.4 es el punto de corte elegido para que las tres bandas
//     cubran TODO el rango 0..1 sin dejar huecos, ya que el roadmap
//     daba tres puntos de ejemplo, no una partición completa).
//   - Confidence < 0.4             → Pedir más información.
//
// "Explorar alternativa" reusa Strategy Selector (Bloque 39): si algún
// gap tiene más de un candidato evaluado (Selection.Scores), el
// segundo mejor —no necesariamente el 2do por Similarity, el 2do por
// Combined, el mismo criterio que ya usa Selector— se ofrece como algo
// a tener en cuenta. "Integrar, no crear": no se inventa ninguna señal
// nueva, se cruzan Decision (37) y []*Selection (39).

// ConfidenceTier es la banda de confianza en la que cayó una Decision.
type ConfidenceTier string

const (
	TierRespond  ConfidenceTier = "responder"
	TierExplore  ConfidenceTier = "explorar alternativa"
	TierSeekInfo ConfidenceTier = "pedir mas informacion"
)

// RespondThreshold y ExploreThreshold son los dos cortes que separan
// las tres bandas. Ver el comentario de arriba para la justificación —
// no son los tres números sueltos del roadmap, son la partición
// completa que se arma a partir de ellos.
const (
	RespondThreshold = 0.85
	ExploreThreshold = 0.4
)

// GapAlternative es una alternativa concreta para UN gap puntual —
// aplanada desde Selection (Bloque 39) para que AdaptiveAction no
// tenga que exponer todo el struct de Selection a quien solo quiere
// saber "qué otra cosa podría probar".
type GapAlternative struct {
	TaskID    string
	TaskDesc  string
	Candidate Candidate
}

// AdaptiveAction es el resultado de aplicar Confidence Calibration a
// una Decision concreta.
type AdaptiveAction struct {
	Tier         ConfidenceTier
	Confidence   float64
	Reason       string
	Alternatives []GapAlternative // solo se llena en TierExplore, y solo con gaps que de verdad tenían más de un candidato
}

// AdaptiveConfidence es el módulo en sí. Sin estado propio, mismo
// criterio que el resto de Fase 3 — no hay nada que persista entre
// llamadas.
type AdaptiveConfidence struct{}

// NewAdaptiveConfidence arma el módulo.
func NewAdaptiveConfidence() *AdaptiveConfidence {
	return &AdaptiveConfidence{}
}

// Decide cruza d (Bloque 27/37) con selections (Bloque 39, puede venir
// vacío o nil) y devuelve qué comportamiento corresponde. d puede ser
// nil — mismo criterio defensivo de todo Fase 3: sin Decision no hay
// Confidence que mirar, así que se trata como la banda más cautelosa
// (TierSeekInfo) en vez de asumir que todo está bien.
func (ac *AdaptiveConfidence) Decide(d *Decision, selections []*Selection) *AdaptiveAction {
	if d == nil {
		return &AdaptiveAction{
			Tier:   TierSeekInfo,
			Reason: "No hay ninguna Decision calculada todavía — sin Confidence que mirar, lo más honesto es pedir más información antes de seguir.",
		}
	}

	action := &AdaptiveAction{Confidence: d.Confidence}

	switch {
	case d.Confidence >= RespondThreshold:
		action.Tier = TierRespond
		action.Reason = fmt.Sprintf("Confianza %.0f%% — suficiente para responder directo, sin necesidad de explorar otra cosa.", d.Confidence*100)

	case d.Confidence >= ExploreThreshold:
		action.Tier = TierExplore
		action.Reason = fmt.Sprintf("Confianza %.0f%% — respondo, pero vale la pena tener presente una alternativa antes de comprometerme del todo.", d.Confidence*100)
		for _, sel := range selections {
			if sel == nil || !sel.HasChoice() {
				continue
			}
			for _, sc := range sel.Scores {
				if sc.Candidate.Action == sel.Chosen.Action {
					continue
				}
				action.Alternatives = append(action.Alternatives, GapAlternative{
					TaskID:    sel.TaskID,
					TaskDesc:  sel.TaskDesc,
					Candidate: sc.Candidate,
				})
				break // el primer candidato distinto del elegido alcanza — no hace falta el resto
			}
		}

	default:
		action.Tier = TierSeekInfo
		action.Reason = fmt.Sprintf("Confianza %.0f%% — demasiado baja para comprometerme; conviene pedir más información antes de avanzar.", d.Confidence*100)
	}

	return action
}

// Describe arma la narrativa de la acción adaptativa.
func (a *AdaptiveAction) Describe() string {
	if a.Tier != TierExplore || len(a.Alternatives) == 0 {
		return a.Reason
	}

	parts := make([]string, len(a.Alternatives))
	for i, alt := range a.Alternatives {
		parts[i] = fmt.Sprintf("%s: también podría probar %s", alt.TaskID, alt.Candidate.Label)
	}
	return fmt.Sprintf("%s %s.", a.Reason, strings.Join(parts, "; "))
}

// ── Trigger de consulta ──────────────────────────────────────────────

var adaptiveConfidenceTriggers = []string{
	"que tan seguro estas de esto",
	"deberia confiar en esta respuesta",
	"que harias con esta confianza",
	"conviene explorar otra opcion antes de",
}

// ParseAdaptiveConfidenceQuery detecta si el usuario pregunta si vale
// la pena confiar en la respuesta o convendría explorar más — distinta
// de ParseDecisionQuery (Bloque 27, que pregunta si conviene avanzar
// con el plan en general, no qué tan sólida es esa recomendación
// puntual).
func ParseAdaptiveConfidenceQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingAdaptiveConfidence)
}
