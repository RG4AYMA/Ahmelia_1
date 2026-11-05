package integration

import (
	"fmt"
	"time"

	"ahmelia/competence"
	"ahmelia/situation"
)

// CompetencePerspective aporta la lectura de EVIDENCIA DE COMPETENCIA al
// modelo compartido de situación (T4 del plan unificado, M5/E2 — pieza 2:
// Standing contribuye a la situación). Participa del mismo contrato que los
// demás especialistas: ES una lectura independiente, no vota, no muta la
// situación ni sus dueños, y JAMÁS ejecuta, autoriza o modifica trabajo.
//
// Recibe assessments YA DERIVADOS (Overlay.Capabilities()); no reinterpreta
// ni recalcula estados — el Standing vive en competence y aquí solo se
// traduce a claims/uncertainties/conflicts tipados y a una relación por
// capacidad. Con evidencia vacía declara la ausencia HONESTAMENTE: nunca
// inventa capacidades y no emite relaciones que un consumidor pueda leer
// como capacidades afirmadas.
//
// Integración viva: core/situation_convergence.go añade
// integration.CompetencePerspective(sit, overlay.Capabilities()) — el mismo
// overlay adjunto al SelfQuery. La relación tipada permite que
// situation.Evaluate la convierta en DecisionFinding y que el consumidor de
// propuestas la cite como evidencia; no vota, no autoriza ni ejecuta.
func CompetencePerspective(s situation.Situation, assessments []competence.Assessment) situation.Contribution {
	c := situation.Contribution{
		ID:          s.ID + ":competence",
		Perspective: "competence",
		CreatedAt:   time.Now(),
		Provenance:  []string{"competence.Overlay", "competence.Ledger", "T4-plan"},
	}
	if len(assessments) == 0 {
		// Honestidad en presencia de cero evidencia: la perspectiva existe,
		// pero no inventa capacidades ni fuerza confianza alta.
		c.Claims = append(c.Claims, "no hay evidencia de competencia derivada todavía; no se afirma ninguna capacidad")
		c.Uncertainty = append(c.Uncertainty, "sin registros de práctica verificada con que derivar estados")
		c.Confidence = confidenceNoEvidence
		return c
	}

	best := confidenceNoEvidence
	for _, a := range assessments {
		c.Claims = append(c.Claims, fmt.Sprintf("sobre la capacidad %s: estado derivado %s — %s", a.Capability, a.Standing, a.Reason))
		standingConfidence := confidenceForStanding(a.Standing)
		best = maxConfidence(best, standingConfidence)
		c.Relations = append(c.Relations, situation.Relation{
			Source:     "ahmelia",
			Target:     a.Capability,
			Type:       fmt.Sprintf("capability_standing_%s", a.Standing),
			Provenance: "competence.Overlay",
			Temporal:   situation.TemporalCurrent,
			Relevance:  0.8,
			Confidence: standingConfidence,
			Epistemic:  situation.EpistemicDerived,
		})
		switch a.Standing {
		case competence.StandingTrialed:
			c.Uncertainty = append(c.Uncertainty, fmt.Sprintf("%s: práctica verificada existente, contextos reales aún insuficientes", a.Capability))
		case competence.StandingCompiled:
			c.Uncertainty = append(c.Uncertainty, fmt.Sprintf("%s: práctica insuficiente según la política vigente", a.Capability))
		case competence.StandingRegressed:
			c.Conflicts = append(c.Conflicts, fmt.Sprintf("competencia regresada en %s: lo que pasaba deja de pasar hasta re-verificar", a.Capability))
			c.Risks = append(c.Risks, fmt.Sprintf("afirmar %s de nuevo sería falso mientras la regresión no se resuelva", a.Capability))
		case competence.StandingUnverifiable:
			c.Uncertainty = append(c.Uncertainty, fmt.Sprintf("%s: sin oráculo declarado, el máximo afirmable es unverifiable", a.Capability))
		case competence.StandingHeard:
			c.Uncertainty = append(c.Uncertainty, fmt.Sprintf("%s: conocida solo por texto, sin pasos válidos contra capacidades registradas", a.Capability))
		}
	}
	c.Confidence = best
	return c
}

// Pesos de confianza derivados del ORDEN de la escalera y de lo que cada
// estado exige (política de Derive): no son invención — un estado más alto
// exige más evidencia, así que respalda más confianza en esta lectura.
const (
	confidenceNoEvidence   = 0.50
	confidenceDemonstrated = 0.90
	confidenceRegressed    = 0.65
	confidenceTrialed      = 0.75
	confidenceCompiled     = 0.60
	confidenceHeard        = 0.40
)

func confidenceForStanding(standing competence.Standing) float64 {
	switch standing {
	case competence.StandingDemonstrated:
		return confidenceDemonstrated
	case competence.StandingTrialed:
		return confidenceTrialed
	case competence.StandingCompiled:
		return confidenceCompiled
	case competence.StandingRegressed:
		return confidenceRegressed
	case competence.StandingHeard:
		return confidenceHeard
	default:
		return confidenceNoEvidence
	}
}

func maxConfidence(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
