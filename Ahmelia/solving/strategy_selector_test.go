package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"

	memoryintelligence "ahmelia/memory_intelligence"
)

// TestSelect_SinLeadNoHayEleccion confirma que un gap sin candidato no
// selecciona nada.
func TestSelect_SinLeadNoHayEleccion(t *testing.T) {
	h := Hypothesis{TaskID: "T1", TaskDesc: "algo raro"}
	s := NewSelector(nil)
	sel := s.Select(h)

	if sel.HasChoice() {
		t.Fatal("HasChoice() = true sin ningún candidato, esperaba false")
	}
}

// TestSelect_SinLearningStateUsaSoloSimilarity confirma el caso base:
// sin LearningState, el resultado coincide con tomar directamente el
// de mayor Similarity — no inventa una preferencia sin datos.
func TestSelect_SinLearningStateUsaSoloSimilarity(t *testing.T) {
	h := Hypothesis{
		TaskID: "T1", TaskDesc: "x",
		ClosestAction: "a", ClosestLabel: "A", Similarity: 0.9,
		Alternatives: []Candidate{{Action: "b", Label: "B", Similarity: 0.5}},
	}
	s := NewSelector(nil)
	sel := s.Select(h)

	if sel.Chosen.Action != "a" {
		t.Fatalf("Chosen.Action = %q, esperaba \"a\" (mayor Similarity, sin LearningState)", sel.Chosen.Action)
	}
	if sel.OverrodeSimilarity {
		t.Fatal("OverrodeSimilarity = true sin ningún track record, no debería cambiar nada")
	}
}

// TestSelect_TrackRecordFuerteVenceMayorSimilarity confirma el caso
// central del bloque: un candidato con menor Similarity pero mejor
// historial real gana.
func TestSelect_TrackRecordFuerteVenceMayorSimilarity(t *testing.T) {
	ls := memoryintelligence.NewLearningState()
	ls.ActionConfidence["b"] = 95 // B: historial excelente

	h := Hypothesis{
		TaskID: "T1", TaskDesc: "x",
		ClosestAction: "a", ClosestLabel: "A", Similarity: 0.6, // A: sin historial
		Alternatives: []Candidate{{Action: "b", Label: "B", Similarity: 0.55}},
	}
	s := NewSelector(memoryintelligence.NewLearningStore(ls))
	sel := s.Select(h)

	if sel.Chosen.Action != "b" {
		t.Fatalf("Chosen.Action = %q, esperaba \"b\" (track record real debería pesar)", sel.Chosen.Action)
	}
	if !sel.OverrodeSimilarity {
		t.Fatal("OverrodeSimilarity debería ser true — B no es el de mayor Similarity")
	}
	if !strings.Contains(sel.Reason, "track record") {
		t.Fatalf("Reason = %q, esperaba que mencione el track record", sel.Reason)
	}
}

// TestSelect_TrackRecordDebilNoVenceMayorSimilarity confirma que un
// historial malo no alcanza para ganarle a una ventaja grande de
// Similarity — el peso es 50/50, no track-record-siempre-gana.
func TestSelect_TrackRecordDebilNoVenceMayorSimilarity(t *testing.T) {
	ls := memoryintelligence.NewLearningState()
	ls.ActionConfidence["b"] = 55 // apenas positivo, no una ventaja fuerte

	h := Hypothesis{
		TaskID: "T1", TaskDesc: "x",
		ClosestAction: "a", ClosestLabel: "A", Similarity: 0.95,
		Alternatives: []Candidate{{Action: "b", Label: "B", Similarity: 0.3}},
	}
	s := NewSelector(memoryintelligence.NewLearningStore(ls))
	sel := s.Select(h)

	if sel.Chosen.Action != "a" {
		t.Fatalf("Chosen.Action = %q, esperaba \"a\" — la ventaja de Similarity es demasiado grande para que la revierta un track record apenas positivo", sel.Chosen.Action)
	}
}

// TestSelect_ScoresIncluyenTodosLosCandidatos confirma que Scores
// registra a TODOS los candidatos evaluados, no solo al elegido —
// transparencia del proceso, no una caja negra.
func TestSelect_ScoresIncluyenTodosLosCandidatos(t *testing.T) {
	h := Hypothesis{
		TaskID: "T1", TaskDesc: "x",
		ClosestAction: "a", ClosestLabel: "A", Similarity: 0.9,
		Alternatives: []Candidate{
			{Action: "b", Label: "B", Similarity: 0.5},
			{Action: "c", Label: "C", Similarity: 0.3},
		},
	}
	s := NewSelector(nil)
	sel := s.Select(h)

	if len(sel.Scores) != 3 {
		t.Fatalf("len(Scores) = %d, esperaba 3 (elegido + 2 alternativas)", len(sel.Scores))
	}
}

// TestHasTrackRecord_DistingueSinDatoDeDatoBajo confirma que -1 (sin
// historial) es distinguible de un historial bajo real (ej. 0.1).
func TestHasTrackRecord_DistingueSinDatoDeDatoBajo(t *testing.T) {
	sinDato := CandidateScore{TrackRecord: -1}
	conDatoBajo := CandidateScore{TrackRecord: 0.1}

	if sinDato.HasTrackRecord() {
		t.Fatal("HasTrackRecord() = true con TrackRecord=-1, esperaba false")
	}
	if !conDatoBajo.HasTrackRecord() {
		t.Fatal("HasTrackRecord() = false con TrackRecord=0.1, esperaba true (0.1 es un dato real, aunque bajo)")
	}
}

// TestSelectAll_NilNoRompe confirma el criterio defensivo estándar.
func TestSelectAll_NilNoRompe(t *testing.T) {
	s := NewSelector(nil)
	if got := s.SelectAll(nil); got != nil {
		t.Fatalf("SelectAll(nil) = %v, esperaba nil", got)
	}
}

// TestDescribeSelections_OmiteSinEleccion confirma que la narrativa
// conjunta no lista gaps sin ningún candidato.
func TestDescribeSelections_OmiteSinEleccion(t *testing.T) {
	selections := []*Selection{
		{TaskID: "T1", TaskDesc: "x"}, // sin Chosen -> se omite
		{TaskID: "T2", TaskDesc: "y", Chosen: Candidate{Action: "a", Label: "A"}, Reason: "porque sí"},
	}
	desc := DescribeSelections(selections)
	if strings.Contains(desc, "T1") {
		t.Errorf("DescribeSelections() = %q, no debería mencionar T1 (sin elección)", desc)
	}
	if !strings.Contains(desc, "T2") {
		t.Errorf("DescribeSelections() = %q, debería mencionar T2", desc)
	}
}

// TestParseStrategyQuery_NoPisaComparisonQuery confirma que los dos
// triggers de Bloque 38 y 39 no colisionan.
func TestParseStrategyQuery_NoPisaComparisonQuery(t *testing.T) {
	if ParseStrategyQuery(interaction.Analyze("compara las hipotesis", interaction.Context{})) {
		t.Fatal("ParseStrategyQuery no debería activarse con el trigger de Comparator (Bloque 38)")
	}
	if !ParseStrategyQuery(interaction.Analyze("que estrategia elegirias", interaction.Context{})) {
		t.Fatal("ParseStrategyQuery debería activarse con su propio trigger")
	}
}
