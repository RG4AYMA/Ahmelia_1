package solving

import "ahmelia/interaction"

import "testing"

// TestDecide_AvanzarSinRiesgoNiGapsSinPista confirma la ruta central:
// sin riesgos y sin gaps sin pista, el veredicto es Avanzar.
func TestDecide_AvanzarSinRiesgoNiGapsSinPista(t *testing.T) {
	hs := &HypothesisSet{Hypotheses: []Hypothesis{
		{TaskID: "T1", ClosestAction: "abrir_programa"}, // HasLead()==true
	}}
	cr := &ConstraintReport{Checks: []ConstraintCheck{
		{Requirement: "r1", Status: StatusSatisfiable},
	}}

	de := NewDecisionEngine()
	d := de.Decide("x", hs, cr)

	if d.Verdict != VerdictProceed {
		t.Fatalf("esperaba VerdictProceed, obtuve %q (razon: %s)", d.Verdict, d.Reason)
	}
	if !d.ShouldProceed() {
		t.Error("ShouldProceed() debería ser true para VerdictProceed")
	}
}

// TestDecide_PedirInfoConRiesgoYGapsSinPista confirma el caso central
// del bloque: riesgo Y gaps sin pista a la vez producen VerdictSeekInfo,
// no Ajustar.
func TestDecide_PedirInfoConRiesgoYGapsSinPista(t *testing.T) {
	hs := &HypothesisSet{Hypotheses: []Hypothesis{
		{TaskID: "T1", ClosestAction: ""}, // sin pista
	}}
	cr := &ConstraintReport{Checks: []ConstraintCheck{
		{Requirement: "r1", Status: StatusAtRisk},
	}}

	de := NewDecisionEngine()
	d := de.Decide("x", hs, cr)

	if d.Verdict != VerdictSeekInfo {
		t.Fatalf("esperaba VerdictSeekInfo con riesgo + gap sin pista, obtuve %q", d.Verdict)
	}
	if d.ShouldProceed() {
		t.Error("ShouldProceed() debería ser false para VerdictSeekInfo")
	}
}

// TestDecide_AjustarConSoloRiesgo confirma que riesgo SIN gaps sin
// pista (todos los gaps tienen alguna Action parecida) produce Ajustar,
// no PedirInfo — hay riesgo, pero también hay por dónde arrancar.
func TestDecide_AjustarConSoloRiesgo(t *testing.T) {
	hs := &HypothesisSet{Hypotheses: []Hypothesis{
		{TaskID: "T1", ClosestAction: "abrir_programa"},
	}}
	cr := &ConstraintReport{Checks: []ConstraintCheck{
		{Requirement: "r1", Status: StatusAtRisk},
	}}

	de := NewDecisionEngine()
	d := de.Decide("x", hs, cr)

	if d.Verdict != VerdictAdjust {
		t.Fatalf("esperaba VerdictAdjust con solo riesgo, obtuve %q", d.Verdict)
	}
}

// TestDecide_AjustarConSoloGapsSinPista confirma que gaps sin pista SIN
// riesgo detectado también producen Ajustar, no Avanzar directo — un
// blind spot igual merece cautela aunque no haya un Requirement
// explícito en juego.
func TestDecide_AjustarConSoloGapsSinPista(t *testing.T) {
	hs := &HypothesisSet{Hypotheses: []Hypothesis{
		{TaskID: "T1", ClosestAction: ""},
	}}
	cr := &ConstraintReport{Checks: []ConstraintCheck{
		{Requirement: "r1", Status: StatusSatisfiable},
	}}

	de := NewDecisionEngine()
	d := de.Decide("x", hs, cr)

	if d.Verdict != VerdictAdjust {
		t.Fatalf("esperaba VerdictAdjust con solo gaps sin pista, obtuve %q", d.Verdict)
	}
}

// TestDecide_SinDatosNoRompe confirma que Decide(goal, nil, nil) no
// rompe y, sin ninguna señal de alarma, cae en Avanzar — mismo criterio
// defensivo que el resto de Fase 3.
func TestDecide_SinDatosNoRompe(t *testing.T) {
	de := NewDecisionEngine()
	d := de.Decide("x", nil, nil)

	if d.Verdict != VerdictProceed {
		t.Fatalf("sin HypothesisSet ni ConstraintReport, esperaba VerdictProceed, obtuve %q", d.Verdict)
	}
	if d.RiskCount != 0 || d.NoLeadGaps != 0 {
		t.Errorf("esperaba RiskCount=0 y NoLeadGaps=0, obtuve %d/%d", d.RiskCount, d.NoLeadGaps)
	}
}

// TestParseDecisionQuery_DistintoDeLosOtrosSeis confirma que los
// triggers de Decision Engine no se pisan con los seis bloques
// anteriores de Fase 3.
func TestParseDecisionQuery_DistintoDeLosOtrosSeis(t *testing.T) {
	if !ParseDecisionQuery(interaction.Analyze("que deberia hacer con armar una tienda online", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Decision Engine")
	}
	if ParseDecisionQuery(interaction.Analyze("analiza este objetivo: crear un juego", interaction.Context{})) {
		t.Error("un pedido de Goal Analyzer no debería disparar Decision Engine")
	}
	if ParseDecisionQuery(interaction.Analyze("que tareas necesito para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Problem Decomposer no debería disparar Decision Engine")
	}
	if ParseDecisionQuery(interaction.Analyze("cuanto sabes de armar una tienda online", interaction.Context{})) {
		t.Error("un pedido de Knowledge Mapper no debería disparar Decision Engine")
	}
	if ParseDecisionQuery(interaction.Analyze("que orden debo seguir para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Dependency Solver no debería disparar Decision Engine")
	}
	if ParseDecisionQuery(interaction.Analyze("que hipotesis tienes para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Hypothesis Generator no debería disparar Decision Engine")
	}
	if ParseDecisionQuery(interaction.Analyze("que restricciones tiene armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Constraint Solver no debería disparar Decision Engine")
	}
}
