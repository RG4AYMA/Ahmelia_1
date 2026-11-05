package solving

import (
	"ahmelia/interaction"
	"fmt"
)

// ============================================================================
// DECISION_ENGINE.GO — Bloque 27, Fase 3 (Cognitive Mind), Capa 3.7
// Extendido en Bloque 37 (Confidence Calibration, Nivel II — 3.17):
// agrega Decision.Confidence, un score continuo 0..1 que distingue,
// dentro de un mismo Verdict, cuán sólida es la base real detrás —
// sin cambiar la firma de Decide() ni el criterio categórico existente.
// ============================================================================
//
// Hypothesis Generator (Bloque 25) propone por dónde probar con cada
// gap. Constraint Solver (Bloque 26) dice qué Requirements quedan en
// riesgo. Ninguno de los dos, por separado, contesta la pregunta que en
// la práctica importa: ¿conviene arrancar con el plan tal como está?
// Decision Engine junta esas DOS señales — no agrega ninguna nueva,
// mismo criterio "integrar, no crear" que Hypothesis Generator y
// Constraint Solver — y devuelve un veredicto de tres valores:
//
//   - Avanzar: sin riesgos y con pistas para cualquier gap que exista.
//   - Ajustar el plan: hay riesgo o hay gaps sin pista, pero no las dos
//     cosas a la vez — se puede arrancar, con cautela.
//   - Pedir más información: hay riesgo Y además hay al menos un gap
//     sin ninguna pista — arrancar así sería comprometerse con un plan
//     que ni siquiera tiene un punto de partida para su parte más
//     insegura.
//
// Es el nodo con MÁS dependencias de Fase 3 hasta ahora: necesita el
// HypothesisSet completo (Bloque 25) y el ConstraintReport completo
// (Bloque 26), no solo un dato suelto de cada uno.

// DecisionVerdict es el veredicto de Decision Engine sobre un plan.
type DecisionVerdict string

const (
	// VerdictProceed: no hay señal de riesgo ni de gaps sin pista.
	VerdictProceed DecisionVerdict = "avanzar"
	// VerdictAdjust: hay riesgo O hay gaps sin pista (no ambos a la
	// vez) — se puede arrancar, pero con ajustes.
	VerdictAdjust DecisionVerdict = "ajustar el plan"
	// VerdictSeekInfo: hay riesgo Y gaps sin pista al mismo tiempo —
	// arrancar así sería a ciegas en la parte más insegura del plan.
	VerdictSeekInfo DecisionVerdict = "pedir mas informacion antes de empezar"
)

// Decision es el resultado de evaluar un plan completo.
type Decision struct {
	Goal       string
	Verdict    DecisionVerdict
	Reason     string
	RiskCount  int     // cuántos Requirements quedaron StatusAtRisk (Bloque 26)
	NoLeadGaps int     // cuántos gaps no tienen ninguna Action parecida (Bloque 25)
	Confidence float64 // Bloque 37 (Confidence Calibration) — 0..1, ver calibrateConfidence()
}

// ShouldProceed es el atajo booleano para quien solo necesita saber si
// arrancar o no, sin tener que comparar Verdict contra las tres
// constantes a mano.
func (d *Decision) ShouldProceed() bool {
	return d.Verdict == VerdictProceed
}

// DecisionEngine es el Decision Engine en sí. Se llama DecisionEngine,
// no Solver ni Engine a secas: Solver ya lo usa Dependency Solver
// (Bloque 24) dentro del mismo paquete, y Engine a secas confundiría
// con core.Engine. Sin estado propio, mismo criterio que el resto de
// Fase 3.
type DecisionEngine struct{}

// NewDecisionEngine arma el Decision Engine.
func NewDecisionEngine() *DecisionEngine {
	return &DecisionEngine{}
}

// Decide cruza hs (Bloque 25) y cr (Bloque 26) y arma el veredicto.
// Ambos pueden ser nil — mismo criterio defensivo que el resto de Fase
// 3: sin esos datos, Decision Engine no inventa riesgo ni gaps sin
// pista, cuenta 0 de cada uno (lo que en la práctica empuja el
// veredicto hacia Avanzar, que es lo correcto cuando no hay ninguna
// señal de alarma todavía, no un "no sé" disfrazado de decisión).
func (de *DecisionEngine) Decide(goal string, hs *HypothesisSet, cr *ConstraintReport) *Decision {
	riskCount := 0
	if cr != nil {
		riskCount = cr.RiskCount()
	}

	noLeadGaps := 0
	if hs != nil {
		for _, h := range hs.Hypotheses {
			if !h.HasLead() {
				noLeadGaps++
			}
		}
	}

	d := &Decision{Goal: goal, RiskCount: riskCount, NoLeadGaps: noLeadGaps}
	d.Confidence = calibrateConfidence(hs, cr)

	switch {
	case riskCount > 0 && noLeadGaps > 0:
		d.Verdict = VerdictSeekInfo
		d.Reason = fmt.Sprintf(
			"Hay %d requisito(s) en riesgo Y %d gap(s) sin ninguna pista de por dónde empezar — arrancar así sería comprometerse a ciegas justo en la parte más insegura del plan.",
			riskCount, noLeadGaps,
		)
	case riskCount > 0:
		d.Verdict = VerdictAdjust
		d.Reason = fmt.Sprintf(
			"Hay %d requisito(s) en riesgo, pero todos los gaps del plan tienen al menos una pista de por dónde probar — se puede arrancar, ajustando expectativas de tiempo.",
			riskCount,
		)
	case noLeadGaps > 0:
		d.Verdict = VerdictAdjust
		d.Reason = fmt.Sprintf(
			"No hay requisitos en riesgo detectados, pero %d gap(s) no tienen ninguna pista de por dónde empezar — conviene investigar esa parte antes de comprometerse con el resto del plan.",
			noLeadGaps,
		)
	default:
		d.Verdict = VerdictProceed
		d.Reason = "Sin requisitos en riesgo y, si hay territorio nuevo, todos los gaps tienen al menos una pista de por dónde probar."
	}

	return d
}

// calibrateConfidence — Bloque 37 (Confidence Calibration, Nivel II —
// 3.17). El Verdict de arriba es categórico (Avanzar/Ajustar/Pedir
// info): dos planes que "Ajustan" pueden ser muy distintos entre sí —
// uno con un solo requisito al límite, otro con cinco. Confidence
// cuantifica eso, combinando dos señales que YA llegan a Decide() (no
// se agregó ningún parámetro nuevo a la firma, cero impacto en quien
// ya la llamaba):
//
//   - Señal de hipótesis: promedio de Similarity de cada gap (Bloque
//     36) — un gap sin lead cuenta como 0, no se descarta del
//     promedio. Sin gaps en absoluto, esta señal es 1.0 (nada de qué
//     dudar en esa dimensión).
//   - Señal de restricciones: promedio por check — Satisfiable=1.0,
//     Unverifiable=0.5 (neutro: no poder verificar no es lo mismo que
//     estar en riesgo, mismo criterio que evalConstraintSolver en Self
//     Evaluation, Bloque 29), AtRisk=0.0. Sin requirements, 1.0.
//
// Confidence es el promedio simple de ambas, sin ponderar — mismo
// criterio "ninguna señal vale más que otra a propósito" de Self
// Evaluation. Distinto de Self Evaluation en el propósito: Self
// Evaluation califica qué tan bien le fue a CADA CAPA del proceso;
// esto califica qué tan confiable es EL VEREDICTO final. No se
// reutilizó el código de Self Evaluation porque Decide() no tiene
// acceso a las 8 capas completas en su firma actual (solo hs y cr) —
// unificarlos exigiría cambiar esa firma, fuera de alcance de este
// bloque.
func calibrateConfidence(hs *HypothesisSet, cr *ConstraintReport) float64 {
	leadSignal := 1.0
	if hs != nil && len(hs.Hypotheses) > 0 {
		total := 0.0
		for _, h := range hs.Hypotheses {
			total += h.Similarity // 0 si !HasLead(), ya que Similarity queda en 0
		}
		leadSignal = total / float64(len(hs.Hypotheses))
	}

	constraintSignal := 1.0
	if cr != nil && len(cr.Checks) > 0 {
		total := 0.0
		for _, c := range cr.Checks {
			switch c.Status {
			case StatusSatisfiable:
				total += 1.0
			case StatusUnverifiable:
				total += 0.5
			}
		}
		constraintSignal = total / float64(len(cr.Checks))
	}

	return (leadSignal + constraintSignal) / 2
}

// Describe arma la narrativa completa de la decisión, incluida la
// confianza calibrada — un veredicto "Ajustar" con 78% de confianza es
// información distinta de uno con 35%, aunque el Verdict sea el mismo.
func (d *Decision) Describe() string {
	return fmt.Sprintf("Para \"%s\": %s (confianza: %.0f%%). %s", d.Goal, d.Verdict, d.Confidence*100, d.Reason)
}

// ── Trigger de consulta ──────────────────────────────────────────────

var decisionTriggers = []string{
	"que deberia hacer con",
	"conviene avanzar con",
	"que decision tomarias sobre",
	"deberiamos seguir con",
	"me conviene arrancar con",
}

// ParseDecisionQuery detecta si el usuario pregunta si conviene
// avanzar — distinto de las seis capas anteriores: ninguna daba un
// veredicto único de sí/no/depende, todas describían un aspecto parcial
// del plan (qué es, cómo se divide, cuánto ya se sabe, en qué orden,
// por dónde probar, qué está en riesgo). Esta es la primera que decide.
func ParseDecisionQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingDecision)
}
