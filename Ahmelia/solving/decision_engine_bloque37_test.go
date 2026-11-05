package solving

import (
	"strings"
	"testing"
)

// TestDecide_ConfianzaMaximaSinGapsNiRequirements confirma el caso
// ideal: sin hipótesis ni constraints, ninguna señal de incertidumbre,
// Confidence debe ser 1.0 exacto.
func TestDecide_ConfianzaMaximaSinGapsNiRequirements(t *testing.T) {
	de := NewDecisionEngine()
	d := de.Decide("x", nil, nil)

	if d.Confidence != 1.0 {
		t.Fatalf("Confidence = %.2f, esperaba 1.0 (sin señales de incertidumbre)", d.Confidence)
	}
}

// TestDecide_ConfianzaBajaConGapsSinLead confirma que un gap sin lead
// (Similarity=0) arrastra la confianza hacia abajo, no queda excluido
// del promedio.
func TestDecide_ConfianzaBajaConGapsSinLead(t *testing.T) {
	hs := &HypothesisSet{Hypotheses: []Hypothesis{
		{TaskID: "T1"}, // sin ClosestAction -> Similarity=0, HasLead()==false
	}}

	de := NewDecisionEngine()
	d := de.Decide("x", hs, nil)

	// leadSignal = 0/1 = 0; constraintSignal = 1.0 (sin checks) -> Confidence = 0.5
	if d.Confidence != 0.5 {
		t.Fatalf("Confidence = %.2f, esperaba 0.5 (gap sin lead arrastra la señal a 0, constraints en 1.0)", d.Confidence)
	}
}

// TestDecide_ConfianzaUsaSimilarityReal confirma que Confidence no es
// binario (con lead / sin lead) — un lead con alta Similarity da más
// confianza que uno con baja Similarity.
func TestDecide_ConfianzaUsaSimilarityReal(t *testing.T) {
	hsAlta := &HypothesisSet{Hypotheses: []Hypothesis{
		{TaskID: "T1", ClosestAction: "abrir_programa", Similarity: 0.9},
	}}
	hsBaja := &HypothesisSet{Hypotheses: []Hypothesis{
		{TaskID: "T1", ClosestAction: "abrir_programa", Similarity: 0.2},
	}}

	de := NewDecisionEngine()
	dAlta := de.Decide("x", hsAlta, nil)
	dBaja := de.Decide("x", hsBaja, nil)

	if dAlta.Confidence <= dBaja.Confidence {
		t.Fatalf("Confidence con Similarity alta (%.2f) debería ser mayor que con Similarity baja (%.2f)",
			dAlta.Confidence, dBaja.Confidence)
	}
}

// TestDecide_UnverifiableEsNeutroNoPenaliza confirma que un requisito
// Unverifiable pesa 0.5 (neutro), distinto de AtRisk (0.0) — no poder
// verificar algo no es lo mismo que saber que está mal, mismo criterio
// que Self Evaluation (Bloque 29).
func TestDecide_UnverifiableEsNeutroNoPenaliza(t *testing.T) {
	crUnverifiable := &ConstraintReport{Checks: []ConstraintCheck{
		{Requirement: "r1", Status: StatusUnverifiable},
	}}
	crAtRisk := &ConstraintReport{Checks: []ConstraintCheck{
		{Requirement: "r1", Status: StatusAtRisk},
	}}

	de := NewDecisionEngine()
	dUnverifiable := de.Decide("x", nil, crUnverifiable)
	dAtRisk := de.Decide("x", nil, crAtRisk)

	if dUnverifiable.Confidence != 0.75 { // (1.0 leadSignal + 0.5 constraintSignal) / 2
		t.Errorf("Confidence con Unverifiable = %.2f, esperaba 0.75", dUnverifiable.Confidence)
	}
	if dAtRisk.Confidence != 0.5 { // (1.0 + 0.0) / 2
		t.Errorf("Confidence con AtRisk = %.2f, esperaba 0.5", dAtRisk.Confidence)
	}
	if dUnverifiable.Confidence <= dAtRisk.Confidence {
		t.Fatal("Unverifiable debería dar más confianza que AtRisk (neutro vs. malo)")
	}
}

// TestDecide_MismoVerdictDistintaConfianza confirma el punto central
// del bloque: dos decisiones con el mismo Verdict (Ajustar) pueden
// tener confianza muy distinta según qué tan sólidas son las señales
// de fondo.
func TestDecide_MismoVerdictDistintaConfianza(t *testing.T) {
	hsFuerte := &HypothesisSet{Hypotheses: []Hypothesis{
		{TaskID: "T1", ClosestAction: "abrir_programa", Similarity: 0.95},
	}}
	crConUnRiesgo := &ConstraintReport{Checks: []ConstraintCheck{
		{Requirement: "r1", Status: StatusAtRisk},
		{Requirement: "r2", Status: StatusSatisfiable},
		{Requirement: "r3", Status: StatusSatisfiable},
		{Requirement: "r4", Status: StatusSatisfiable},
	}}

	de := NewDecisionEngine()
	d := de.Decide("x", hsFuerte, crConUnRiesgo)

	if d.Verdict != VerdictAdjust {
		t.Fatalf("Verdict = %q, esperaba VerdictAdjust (hay riesgo, hay lead)", d.Verdict)
	}
	if d.Confidence < 0.8 {
		t.Fatalf("Confidence = %.2f, esperaba alta (>0.8) pese al Verdict Adjust: solo 1 de 4 requisitos en riesgo y el lead es fuerte (0.95)", d.Confidence)
	}
}

// TestDescribe_IncluyeConfianza confirma que la narrativa menciona el
// porcentaje, no solo el veredicto.
func TestDescribe_IncluyeConfianza(t *testing.T) {
	de := NewDecisionEngine()
	d := de.Decide("armar el juego", nil, nil)

	desc := d.Describe()
	if !strings.Contains(desc, "confianza") || !strings.Contains(desc, "100%") {
		t.Fatalf("Describe() = %q, esperaba que mencione \"confianza\" y el porcentaje", desc)
	}
}
