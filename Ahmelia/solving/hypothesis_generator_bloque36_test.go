package solving

import (
	"strings"
	"testing"
)

// TestGenerate_AlternativasConEmpateReal reusa el mismo input del test
// original de Bloque 25 ("Abre volumen del programa nuevo"), que ya
// generaba un empate real de overlap=1 entre abrir_programa y varias
// Action de volumen — antes ese empate se resolvía y se descartaba
// todo menos el ganador; ahora el 2do y 3er lugar quedan en
// Alternatives.
func TestGenerate_AlternativasConEmpateReal(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Abre volumen del programa nuevo", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)
	h := hs.Hypotheses[0]

	// Comportamiento sin cambios respecto de Bloque 25 (ver test
	// TestGenerate_EncuentraPistaPorSolapamiento):
	if h.ClosestAction != "abrir_programa" {
		t.Fatalf("ClosestAction = %q, esperaba \"abrir_programa\" (comportamiento no debía cambiar)", h.ClosestAction)
	}
	// Lo nuevo de Bloque 36:
	if len(h.Alternatives) != 2 {
		t.Fatalf("len(Alternatives) = %d, esperaba 2 (top 3 total: mejor + 2 alternativas)", len(h.Alternatives))
	}
	for _, alt := range h.Alternatives {
		if alt.Action == h.ClosestAction {
			t.Fatalf("Alternatives no debería repetir el mejor candidato (%q)", alt.Action)
		}
		if alt.Similarity <= 0 {
			t.Errorf("alternativa %q con Similarity=%.2f, esperaba >0 (solo se guardan candidatos con solapamiento real)", alt.Action, alt.Similarity)
		}
	}
}

// TestGenerate_SinAlternativasConUnSoloCandidato confirma que un gap
// que solo comparte palabras con UNA Action deja Alternatives en nil,
// no en un slice vacío artificial ni con ruido inventado.
func TestGenerate_SinAlternativasConUnSoloCandidato(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "encuentra el documento perdido", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)
	h := hs.Hypotheses[0]

	if h.ClosestAction != "buscar_archivo" {
		t.Fatalf("ClosestAction = %q, esperaba \"buscar_archivo\"", h.ClosestAction)
	}
	if len(h.Alternatives) != 0 {
		t.Fatalf("len(Alternatives) = %d, esperaba 0 (un solo candidato real)", len(h.Alternatives))
	}
}

// TestGenerate_SinPistaSinAlternativas confirma que, sin ningún
// candidato en absoluto, ni ClosestAction ni Alternatives tienen nada
// inventado.
func TestGenerate_SinPistaSinAlternativas(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "xyzabc completamente inconexo", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)
	h := hs.Hypotheses[0]

	if h.HasLead() {
		t.Fatalf("HasLead() = true para una descripción sin solapamiento, obtuve ClosestAction=%q", h.ClosestAction)
	}
	if len(h.Alternatives) != 0 {
		t.Fatalf("len(Alternatives) = %d, esperaba 0 sin ningún candidato", len(h.Alternatives))
	}
}

// TestGenerate_ReasonSePuebla confirma que Reason no queda vacío para
// el mejor candidato — el motivo central de agregar el campo ahora en
// vez de después.
func TestGenerate_ReasonSePuebla(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Abre volumen del programa nuevo", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)
	h := hs.Hypotheses[0]

	if h.Reason == "" {
		t.Fatal("Reason vacío para un candidato con HasLead()==true")
	}
	if !strings.Contains(h.Reason, "abre") {
		t.Fatalf("Reason = %q, esperaba que mencione la palabra compartida \"abre\"", h.Reason)
	}
}

// TestAlternativesLabel_FormatoConO confirma el formato "X o Y" para
// dos alternativas.
func TestAlternativesLabel_FormatoConO(t *testing.T) {
	h := Hypothesis{
		Alternatives: []Candidate{
			{Label: "Bajar volumen"},
			{Label: "Volumen al máximo"},
		},
	}
	got := h.AlternativesLabel()
	want := "Bajar volumen o Volumen al máximo"
	if got != want {
		t.Fatalf("AlternativesLabel() = %q, esperaba %q", got, want)
	}
}

// TestAlternativesLabel_VacioSinAlternativas confirma que no inventa
// texto cuando no hay ninguna.
func TestAlternativesLabel_VacioSinAlternativas(t *testing.T) {
	h := Hypothesis{}
	if got := h.AlternativesLabel(); got != "" {
		t.Fatalf("AlternativesLabel() = %q, esperaba vacío sin Alternatives", got)
	}
}

// TestDescribe_MencionaAlternativasCuandoHay confirma que Describe()
// se hace eco de las alternativas en la narrativa, no solo el struct.
func TestDescribe_MencionaAlternativasCuandoHay(t *testing.T) {
	km := &KnowledgeMap{Goal: "probar volumen", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Abre volumen del programa nuevo", Known: false},
	}}

	g := NewGenerator()
	hs := g.Generate(km, nil)
	desc := hs.Describe()

	if !strings.Contains(desc, "también podría ser") {
		t.Fatalf("Describe() = %q, esperaba que mencione las alternativas", desc)
	}
}
