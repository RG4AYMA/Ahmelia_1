package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestEvaluate_CambiaPorSimilitudSolamente confirma el caso central:
// si el candidato con mayor Similarity es distinto del efectivamente
// elegido (porque el track record lo cambió), ChangedBySimilarityOnly
// queda en true, con el ganador correcto.
func TestEvaluate_CambiaPorSimilitudSolamente(t *testing.T) {
	sel := &Selection{
		TaskID:   "T1",
		TaskDesc: "abrir el programa de diseño",
		Chosen:   Candidate{Action: "abrir_unity", Label: "Abrir Unity", Similarity: 0.6},
		Scores: []CandidateScore{
			{Candidate: Candidate{Action: "abrir_unity", Label: "Abrir Unity", Similarity: 0.6}, TrackRecord: 0.9, Combined: 0.75},
			{Candidate: Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.8}, TrackRecord: -1, Combined: 0.8},
		},
	}

	ce := NewCounterfactualEvaluator()
	cf := ce.Evaluate(sel)

	if cf == nil {
		t.Fatal("esperaba un Counterfactual, obtuve nil")
	}
	if !cf.ChangedBySimilarityOnly {
		t.Error("esperaba ChangedBySimilarityOnly=true (Blender tiene mayor Similarity que el elegido)")
	}
	if cf.WinnerBySimilarityOnly.Action != "abrir_blender" {
		t.Errorf("esperaba WinnerBySimilarityOnly='abrir_blender', obtuve %q", cf.WinnerBySimilarityOnly.Action)
	}
}

// TestEvaluate_SinDatosDeHistorialLoDiceHonesto confirma que, cuando
// NINGÚN candidato tiene track record, HasTrackRecordData queda en
// false — no se inventa un ganador para ese escenario.
func TestEvaluate_SinDatosDeHistorialLoDiceHonesto(t *testing.T) {
	sel := &Selection{
		TaskID: "T1",
		Chosen: Candidate{Action: "a", Label: "A", Similarity: 0.7},
		Scores: []CandidateScore{
			{Candidate: Candidate{Action: "a", Label: "A", Similarity: 0.7}, TrackRecord: -1, Combined: 0.7},
			{Candidate: Candidate{Action: "b", Label: "B", Similarity: 0.5}, TrackRecord: -1, Combined: 0.5},
		},
	}

	ce := NewCounterfactualEvaluator()
	cf := ce.Evaluate(sel)

	if cf.HasTrackRecordData {
		t.Error("sin track record para ningún candidato, esperaba HasTrackRecordData=false")
	}
	if !strings.Contains(cf.Narrative, "no se puede evaluar") {
		t.Errorf("esperaba que la narrativa admitiera la falta de datos, obtuve: %q", cf.Narrative)
	}
}

// TestEvaluate_ConTrackRecordQueNoCambiaNada confirma que, cuando el
// elegido también gana en el escenario de solo-historial,
// ChangedByTrackRecordOnly queda en false.
func TestEvaluate_ConTrackRecordQueNoCambiaNada(t *testing.T) {
	sel := &Selection{
		TaskID: "T1",
		Chosen: Candidate{Action: "a", Label: "A", Similarity: 0.7},
		Scores: []CandidateScore{
			{Candidate: Candidate{Action: "a", Label: "A", Similarity: 0.7}, TrackRecord: 0.9, Combined: 0.8},
			{Candidate: Candidate{Action: "b", Label: "B", Similarity: 0.5}, TrackRecord: 0.3, Combined: 0.4},
		},
	}

	ce := NewCounterfactualEvaluator()
	cf := ce.Evaluate(sel)

	if !cf.HasTrackRecordData {
		t.Fatal("esperaba HasTrackRecordData=true")
	}
	if cf.ChangedByTrackRecordOnly {
		t.Error("A gana en ambos escenarios, esperaba ChangedByTrackRecordOnly=false")
	}
}

// TestEvaluate_SinDosCandidatosDevuelveNil confirma que, con menos de
// dos candidatos evaluados, Evaluate() devuelve nil — no hay ningún
// "qué habría pasado si" que tenga sentido con un solo candidato.
func TestEvaluate_SinDosCandidatosDevuelveNil(t *testing.T) {
	sel := &Selection{
		TaskID: "T1",
		Chosen: Candidate{Action: "a", Label: "A"},
		Scores: []CandidateScore{{Candidate: Candidate{Action: "a", Label: "A"}, Combined: 0.7}},
	}

	ce := NewCounterfactualEvaluator()
	if ce.Evaluate(sel) != nil {
		t.Error("con un solo candidato, esperaba nil")
	}
}

// TestEvaluate_SinEleccionDevuelveNil confirma el mismo criterio
// defensivo para una Selection sin HasChoice().
func TestEvaluate_SinEleccionDevuelveNil(t *testing.T) {
	ce := NewCounterfactualEvaluator()
	if ce.Evaluate(&Selection{TaskID: "T1"}) != nil {
		t.Error("sin elección, esperaba nil")
	}
	if ce.Evaluate(nil) != nil {
		t.Error("con Selection nil, esperaba nil")
	}
}

// TestEvaluateReflections_ReusaElFiltroDeReflectionLoop confirma que
// EvaluateReflections solo produce Counterfactual para los TaskID que
// aparecen en reflections — un gap con Selection pero SIN Reflection
// (porque Bloque 41 ya decidió que no había nada real que reflexionar)
// no genera ningún Counterfactual tampoco.
func TestEvaluateReflections_ReusaElFiltroDeReflectionLoop(t *testing.T) {
	selConMaterial := &Selection{
		TaskID: "T1",
		Chosen: Candidate{Action: "a", Label: "A", Similarity: 0.6},
		Scores: []CandidateScore{
			{Candidate: Candidate{Action: "a", Label: "A", Similarity: 0.6}, Combined: 0.6},
			{Candidate: Candidate{Action: "b", Label: "B", Similarity: 0.55}, Combined: 0.55},
		},
	}
	selSinMaterial := &Selection{
		TaskID: "T2",
		Chosen: Candidate{Action: "c", Label: "C", Similarity: 0.9},
		Scores: []CandidateScore{
			{Candidate: Candidate{Action: "c", Label: "C", Similarity: 0.9}, Combined: 0.9},
			{Candidate: Candidate{Action: "d", Label: "D", Similarity: 0.1}, Combined: 0.1},
		},
	}
	reflections := []Reflection{{TaskID: "T1", ShouldReconsider: true}} // T2 nunca llegó a Reflection Loop

	ce := NewCounterfactualEvaluator()
	cfs := ce.EvaluateReflections(reflections, []*Selection{selConMaterial, selSinMaterial})

	if len(cfs) != 1 || cfs[0].TaskID != "T1" {
		t.Fatalf("esperaba un solo Counterfactual para T1, obtuve %+v", cfs)
	}
}

// TestDescribeCounterfactuals_VacioEsHonesto confirma que, sin ningún
// Counterfactual, el mensaje lo dice explícitamente en vez de devolver
// una cadena vacía.
func TestDescribeCounterfactuals_VacioEsHonesto(t *testing.T) {
	desc := DescribeCounterfactuals(nil)
	if !strings.Contains(desc, "No hay ninguna elección") {
		t.Errorf("esperaba el mensaje de \"nada que evaluar\", obtuve: %q", desc)
	}
}

// TestParseCounterfactualQuery_DistintoDeReflectionLoop confirma que
// el trigger no se pisa con Reflection Loop (Bloque 41).
func TestParseCounterfactualQuery_DistintoDeReflectionLoop(t *testing.T) {
	if !ParseCounterfactualQuery(interaction.Analyze("que hubiera pasado si elegias otra cosa", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Counterfactual Evaluation")
	}
	if ParseCounterfactualQuery(interaction.Analyze("te arrepientes de alguna eleccion", interaction.Context{})) {
		t.Error("un pedido de Reflection Loop no debería disparar Counterfactual Evaluation")
	}
}
