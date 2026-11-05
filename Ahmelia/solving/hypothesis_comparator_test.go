package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestCompare_SinLeadNoTieneNotes confirma que un gap sin candidato
// devuelve Notes vacío, no algo inventado.
func TestCompare_SinLeadNoTieneNotes(t *testing.T) {
	h := Hypothesis{TaskID: "T1", TaskDesc: "algo raro"}
	cmp := NewComparator()
	c := cmp.Compare(h)

	if len(c.Notes) != 0 {
		t.Fatalf("len(Notes) = %d, esperaba 0 sin HasLead()", len(c.Notes))
	}
}

// TestCompare_UnSoloCandidatoUnaSolaNote confirma que, sin
// Alternatives, solo aparece el elegido — Notes con longitud 1.
func TestCompare_UnSoloCandidatoUnaSolaNote(t *testing.T) {
	h := Hypothesis{
		TaskID: "T1", TaskDesc: "x",
		ClosestAction: "buscar_archivo", ClosestLabel: "Buscar archivo", Similarity: 0.5,
	}
	cmp := NewComparator()
	c := cmp.Compare(h)

	if len(c.Notes) != 1 {
		t.Fatalf("len(Notes) = %d, esperaba 1 (solo el elegido, sin alternativas)", len(c.Notes))
	}
	if c.Notes[0].Rank != 1 {
		t.Fatalf("Notes[0].Rank = %d, esperaba 1", c.Notes[0].Rank)
	}
}

// TestCompare_GapSeCalculaBienConAlternativas confirma que el Gap de
// cada alternativa es la diferencia real de Similarity contra el
// elegido, no un valor fijo.
func TestCompare_GapSeCalculaBienConAlternativas(t *testing.T) {
	h := Hypothesis{
		TaskID: "T1", TaskDesc: "x",
		ClosestAction: "a", ClosestLabel: "A", Similarity: 0.9,
		Alternatives: []Candidate{
			{Action: "b", Label: "B", Similarity: 0.8},
			{Action: "c", Label: "C", Similarity: 0.3},
		},
	}
	cmp := NewComparator()
	c := cmp.Compare(h)

	if len(c.Notes) != 3 {
		t.Fatalf("len(Notes) = %d, esperaba 3 (elegido + 2 alternativas)", len(c.Notes))
	}
	if got := c.Notes[1].Gap; got < 0.09 || got > 0.11 {
		t.Errorf("Gap de B = %.4f, esperaba ~0.10 (0.9 - 0.8)", got)
	}
	if got := c.Notes[2].Gap; got < 0.59 || got > 0.61 {
		t.Errorf("Gap de C = %.2f, esperaba ~0.60 (0.9 - 0.3)", got)
	}
}

// TestCompare_WasCloseSoloBajoElUmbral confirma la clasificación
// "cerca"/"lejos" según CloseCallThreshold.
func TestCompare_WasCloseSoloBajoElUmbral(t *testing.T) {
	h := Hypothesis{
		TaskID: "T1", TaskDesc: "x",
		ClosestAction: "a", ClosestLabel: "A", Similarity: 0.9,
		Alternatives: []Candidate{
			{Action: "b", Label: "B", Similarity: 0.8}, // gap 0.10 -> cerca
			{Action: "c", Label: "C", Similarity: 0.3}, // gap 0.60 -> lejos
		},
	}
	cmp := NewComparator()
	c := cmp.Compare(h)

	if !c.Notes[1].WasClose {
		t.Error("B con gap 0.10 debería marcarse WasClose=true (umbral 0.15)")
	}
	if c.Notes[2].WasClose {
		t.Error("C con gap 0.60 no debería marcarse WasClose=true")
	}
	if !c.HasCloseCall() {
		t.Fatal("HasCloseCall() debería ser true — B estuvo cerca")
	}
}

// TestCompareAll_NilNoRompe confirma el criterio defensivo estándar.
func TestCompareAll_NilNoRompe(t *testing.T) {
	cmp := NewComparator()
	if got := cmp.CompareAll(nil); got != nil {
		t.Fatalf("CompareAll(nil) = %v, esperaba nil", got)
	}
}

// TestDescribeAll_OmiteLosSinAlternativas confirma que la narrativa
// conjunta no lista gaps que no tuvieron nada que comparar — evita
// llenar la respuesta de "nada que comparar" repetido.
func TestDescribeAll_OmiteLosSinAlternativas(t *testing.T) {
	comparisons := []*Comparison{
		{TaskID: "T1", TaskDesc: "x", Notes: []ComparisonNote{
			{Candidate: Candidate{Label: "A"}, Rank: 1},
		}}, // un solo candidato -> se omite
		{TaskID: "T2", TaskDesc: "y", Notes: []ComparisonNote{
			{Candidate: Candidate{Label: "B", Similarity: 0.9}, Rank: 1},
			{Candidate: Candidate{Label: "C", Similarity: 0.7}, Rank: 2, Gap: 0.2},
		}}, // sí tiene alternativa real -> aparece
	}

	desc := DescribeAll(comparisons)
	if strings.Contains(desc, "T1") {
		t.Errorf("DescribeAll() = %q, no debería mencionar T1 (un solo candidato, nada que comparar)", desc)
	}
	if !strings.Contains(desc, "T2") {
		t.Errorf("DescribeAll() = %q, debería mencionar T2 (tiene alternativa real)", desc)
	}
}

// TestDescribeAll_TodosVaciosDaMensajeHonesto confirma el caso límite:
// si ningún gap tuvo alternativas, el mensaje lo dice, no queda vacío.
func TestDescribeAll_TodosVaciosDaMensajeHonesto(t *testing.T) {
	comparisons := []*Comparison{
		{TaskID: "T1", TaskDesc: "x", Notes: []ComparisonNote{{Rank: 1}}},
	}
	desc := DescribeAll(comparisons)
	if desc == "" {
		t.Fatal("DescribeAll() vacío, esperaba un mensaje honesto")
	}
	if !strings.Contains(desc, "Ninguno") {
		t.Fatalf("DescribeAll() = %q, esperaba que avise que nadie tuvo alternativas", desc)
	}
}

// TestParseComparisonQuery_NoPisaHypothesisQuery confirma que la nueva
// meta-pregunta no colisiona con la de Bloque 25.
func TestParseComparisonQuery_NoPisaHypothesisQuery(t *testing.T) {
	if ParseComparisonQuery(interaction.Analyze("que hipotesis tenes para esto", interaction.Context{})) {
		t.Fatal("ParseComparisonQuery no debería activarse con el trigger de Hypothesis Generator (Bloque 25)")
	}
	if !ParseComparisonQuery(interaction.Analyze("compara las hipotesis", interaction.Context{})) {
		t.Fatal("ParseComparisonQuery debería activarse con su propio trigger")
	}
}
