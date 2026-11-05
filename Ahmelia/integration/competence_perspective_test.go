package integration

import (
	"strings"
	"testing"

	"ahmelia/competence"
	"ahmelia/situation"
)

func baseSituation() situation.Situation {
	return situation.Situation{ID: "sit-test", Version: 1, Confidence: 0.8}
}

func assessment(capability string, standing competence.Standing, reason string) competence.Assessment {
	return competence.Assessment{Capability: capability, Standing: standing, Reason: reason}
}

// TestCompetencePerspectiveHonestWithNoEvidence es la contracara del patrón
// de fabricación: cero evidencia ⇒ claim de ausencia, nunca capacidades.
func TestCompetencePerspectiveHonestWithNoEvidence(t *testing.T) {
	c := CompetencePerspective(baseSituation(), nil)

	if c.ID != "sit-test:competence" || c.Perspective != "competence" {
		t.Fatalf("identity mismatch: %q / %q", c.ID, c.Perspective)
	}
	if len(c.Provenance) == 0 || c.Provenance[0] != "competence.Overlay" {
		t.Fatalf("provenance must trace the derivation sources: %v", c.Provenance)
	}
	if !strings.Contains(c.Claims[0], "no hay evidencia de competencia derivada") {
		t.Fatalf("honest claim of absence expected, got: %q", c.Claims)
	}
	// Con cero evidencia jamás aparece el formato de evaluación por capacidad
	// ("estado derivado"): la perspectiva vacía no da evaluaciones.
	if strings.Contains(strings.Join(c.Claims, " "), "estado derivado") {
		t.Fatalf("no capability assessment may surface without evidence: %v", c.Claims)
	}
	if c.Confidence != 0.50 {
		t.Fatalf("no-evidence confidence must be the honesty floor, got %.2f", c.Confidence)
	}
}

// TestCompetencePerspectiveRankingsByStanding: la confianza deriva del orden
// de la escalera (más exigencia ⇒ más respaldo) y cada estado aporta su
// lectura tipada sin re-derivación ni string-sniffing.
func TestCompetencePerspectiveRankingsByStanding(t *testing.T) {
	c := CompetencePerspective(baseSituation(), []competence.Assessment{
		assessment("media.organize", competence.StandingTrialed, "real: 6/6 en 1 contexto(s) (lo=0.79)"),
		assessment("deploy.check", competence.StandingDemonstrated, "real: 10/10 en 3 contextos (lo=0.83)"),
		assessment("net.retry", competence.StandingCompiled, "práctica insuficiente: 2/6 (lo=0.12)"),
	})

	if len(c.Claims) != 3 {
		t.Fatalf("one claim per derived assessment, got %d", len(c.Claims))
	}
	for _, a := range c.Claims {
		if !strings.Contains(a, "estado derivado") {
			t.Fatalf("claims must report the DERIVED standing verbatim: %q", a)
		}
	}
	// El mejor estado manda en la confianza de la lectura completa.
	if c.Confidence != 0.90 {
		t.Fatalf("demonstrated present must lift confidence to its band, got %.2f", c.Confidence)
	}
	// Cada estado no-firmemente-verificado deja explícito QUÉ le falta.
	joined := strings.Join(c.Uncertainty, "; ")
	for _, fragment := range []string{"contextos reales aún insuficientes", "práctica insuficiente"} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("uncertainty must name what is missing: %q", joined)
		}
	}
}

// TestCompetencePerspectiveFeedsDecisionEvidence cierra el eslabón que
// faltaba: la lectura tipada no queda en claims; cada assessment produce una
// relación que situation.Evaluate convierte en DecisionFinding citables por
// el consumidor de propuestas. La capacidad y el Standing viajan juntos.
func TestCompetencePerspectiveFeedsDecisionEvidence(t *testing.T) {
	c := CompetencePerspective(baseSituation(), []competence.Assessment{
		assessment("deploy.check", competence.StandingDemonstrated, "real: 10/10 en 3 contextos (lo=0.83)"),
		assessment("media.organize", competence.StandingHeard, "texto sin pasos válidos"),
	})

	evidence := situation.Evaluate(baseSituation(), []situation.Contribution{c})
	if len(evidence.Findings) != 2 {
		t.Fatalf("one finding per assessment relation, got %d: %+v", len(evidence.Findings), evidence.Findings)
	}
	byTarget := make(map[string]situation.DecisionFinding, len(evidence.Findings))
	for _, f := range evidence.Findings {
		byTarget[f.Target] = f
		if f.Perspective != "competence" || f.Source != "situation" {
			t.Fatalf("finding lost its competence provenance: %+v", f)
		}
	}
	demonstrated, ok := byTarget["deploy.check"]
	if !ok {
		t.Fatalf("missing demonstrated capability finding: %+v", evidence.Findings)
	}
	if demonstrated.Kind != "capability_standing_demonstrated" || demonstrated.Epistemic != situation.EpistemicDerived {
		t.Fatalf("demonstrated finding must carry type and derived epistemic status: %+v", demonstrated)
	}
	if demonstrated.Confidence != 0.90 {
		t.Fatalf("demonstrated finding confidence = %.2f, want 0.90", demonstrated.Confidence)
	}
	heard, ok := byTarget["media.organize"]
	if !ok {
		t.Fatalf("missing heard capability finding: %+v", evidence.Findings)
	}
	if heard.Confidence >= demonstrated.Confidence {
		t.Fatalf("heard (%0.2f) must not outrank demonstrated (%0.2f)", heard.Confidence, demonstrated.Confidence)
	}
}

// TestCompetencePerspectiveNoEvidenceEmitsNoRelations: la honestidad no es
// solo texto; sin evidencia no se crea una relación que un consumidor pueda
// interpretar como capacidad afirmada.
func TestCompetencePerspectiveNoEvidenceEmitsNoRelations(t *testing.T) {
	c := CompetencePerspective(baseSituation(), nil)
	if len(c.Relations) != 0 {
		t.Fatalf("zero evidence must emit zero relations, got %d", len(c.Relations))
	}
	if findings := situation.Evaluate(baseSituation(), []situation.Contribution{c}).Findings; len(findings) != 0 {
		t.Fatalf("zero evidence must produce zero findings, got %+v", findings)
	}
}

// TestCompetencePerspectiveRegressedIsAConflictNotAStat: regressed es un
// conflicto con riesgo adjunto — jamás se apunta como éxito parcial.
func TestCompetencePerspectiveRegressedIsAConflictNotAStat(t *testing.T) {
	c := CompetencePerspective(baseSituation(), []competence.Assessment{
		assessment("media.organize", competence.StandingRegressed, "falla un escenario que antes pasaba"),
	})

	if len(c.Conflicts) != 1 || !strings.Contains(c.Conflicts[0], "media.organize") {
		t.Fatalf("regressed standing must surface as conflict: %v", c.Conflicts)
	}
	if len(c.Risks) != 1 || !strings.Contains(c.Risks[0], "falso") {
		t.Fatalf("re-claiming a regressed capability must be flagged as risky: %v", c.Risks)
	}
	if c.Confidence != 0.65 {
		t.Fatalf("regressed band confidence, got %.2f", c.Confidence)
	}
}

// TestCompetencePerspectiveUnverifiableStaysLow: sin oráculo no hay
// afirmación fuerte — la lectura lo dice tal cual la política lo exige.
func TestCompetencePerspectiveUnverifiableStaysLow(t *testing.T) {
	c := CompetencePerspective(baseSituation(), []competence.Assessment{
		assessment("media.organize", competence.StandingUnverifiable, "sin oráculo declarado no hay afirmación de competencia"),
	})

	if c.Confidence >= 0.60 {
		t.Fatalf("unverifiable must not carry strong confidence, got %.2f", c.Confidence)
	}
	if !strings.Contains(strings.Join(c.Uncertainty, "; "), "sin oráculo declarado") {
		t.Fatalf("uncertainty must state the oracle gap: %v", c.Uncertainty)
	}
}

// TestCompetencePerspectiveDoesNotRedefineStandings: la perspectiva NO
// recalcula nada — con los mismos assessments, las claims son el eco fiel
// del formato derivado (determinismo de la traducción, pureza de la pieza).
func TestCompetencePerspectiveDoesNotRedefineStandings(t *testing.T) {
	in := []competence.Assessment{
		assessment("media.organize", competence.StandingDemonstrated, "real: 10/10 en 2 contextos (lo=0.80)"),
	}
	first := CompetencePerspective(baseSituation(), in)
	second := CompetencePerspective(baseSituation(), in)

	if len(first.Claims) != len(second.Claims) {
		t.Fatal("same evidence must produce the same claim count")
	}
	for i := range first.Claims {
		if first.Claims[i] != second.Claims[i] {
			t.Fatalf("translation drifted: %q vs %q", first.Claims[i], second.Claims[i])
		}
	}
}
