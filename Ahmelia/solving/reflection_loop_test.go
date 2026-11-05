package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestReflect_CloseCallSinOverrideGeneraReconsideracion confirma el
// caso central del bloque: un gap con un close call real (Bloque 38)
// donde el track record NO cambió nada (Bloque 39) genera una
// Reflection con ShouldReconsider=true.
func TestReflect_CloseCallSinOverrideGeneraReconsideracion(t *testing.T) {
	comp := &Comparison{
		TaskID:   "T1",
		TaskDesc: "abrir el programa de diseño",
		Notes: []ComparisonNote{
			{Candidate: Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.7}, Rank: 1},
			{Candidate: Candidate{Action: "abrir_unity", Label: "Abrir Unity", Similarity: 0.6}, Rank: 2, Gap: 0.1, WasClose: true},
		},
	}
	sel := &Selection{
		TaskID:             "T1",
		TaskDesc:           "abrir el programa de diseño",
		Chosen:             Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.7},
		OverrodeSimilarity: false,
	}

	r := NewReflector()
	rs := r.Reflect([]*Comparison{comp}, []*Selection{sel}, nil)

	if len(rs.Reflections) != 1 {
		t.Fatalf("esperaba 1 Reflection, obtuve %d", len(rs.Reflections))
	}
	ref := rs.Reflections[0]
	if !ref.ShouldReconsider {
		t.Error("con un close call sin override, esperaba ShouldReconsider=true")
	}
	if !strings.Contains(ref.Answer, "Abrir Unity") {
		t.Errorf("esperaba que la respuesta mencionara la alternativa concreta, obtuve: %q", ref.Answer)
	}
}

// TestReflect_OverrideNoNecesitaReconsiderar confirma que, cuando
// Strategy Selector ya cambió el resultado por track record real
// (Bloque 39), ShouldReconsider queda en false — la decisión ya tiene
// evidencia real detrás, no hace falta cuestionarla de nuevo.
func TestReflect_OverrideNoNecesitaReconsiderar(t *testing.T) {
	comp := &Comparison{
		TaskID:   "T1",
		TaskDesc: "abrir el programa de diseño",
		Notes: []ComparisonNote{
			{Candidate: Candidate{Action: "abrir_unity", Label: "Abrir Unity", Similarity: 0.6}, Rank: 1},
			{Candidate: Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.7}, Rank: 2, Gap: -0.1, WasClose: true},
		},
	}
	sel := &Selection{
		TaskID:             "T1",
		TaskDesc:           "abrir el programa de diseño",
		Chosen:             Candidate{Action: "abrir_unity", Label: "Abrir Unity", Similarity: 0.6},
		OverrodeSimilarity: true,
		Reason:             "Abrir Blender tenía mayor similitud (70%), pero Abrir Unity tiene historial real de 90% de éxito — elijo Abrir Unity por track record.",
	}

	r := NewReflector()
	rs := r.Reflect([]*Comparison{comp}, []*Selection{sel}, nil)

	ref := rs.Reflections[0]
	if ref.ShouldReconsider {
		t.Error("con un override respaldado por track record, esperaba ShouldReconsider=false")
	}
	if ref.Answer != sel.Reason {
		t.Errorf("esperaba que Answer reusara sel.Reason tal cual, obtuve: %q", ref.Answer)
	}
}

// TestReflect_SinCloseCallNiOverrideNoGeneraReflection confirma que un
// gap sin close call y sin override no genera ninguna Reflection — no
// hay pregunta real que hacerse ahí.
func TestReflect_SinCloseCallNiOverrideNoGeneraReflection(t *testing.T) {
	comp := &Comparison{
		TaskID: "T1",
		Notes: []ComparisonNote{
			{Candidate: Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.9}, Rank: 1},
			{Candidate: Candidate{Action: "abrir_unity", Label: "Abrir Unity", Similarity: 0.3}, Rank: 2, Gap: 0.6, WasClose: false},
		},
	}
	sel := &Selection{
		TaskID:             "T1",
		Chosen:             Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.9},
		OverrodeSimilarity: false,
	}

	r := NewReflector()
	rs := r.Reflect([]*Comparison{comp}, []*Selection{sel}, nil)

	if len(rs.Reflections) != 0 {
		t.Fatalf("sin close call ni override no esperaba ninguna Reflection, obtuve %d", len(rs.Reflections))
	}
}

// TestReflect_SinCandidatoElegidoSeOmite confirma que un gap donde
// Selection no tiene ninguna elección (!HasChoice()) se omite, sin
// romper.
func TestReflect_SinCandidatoElegidoSeOmite(t *testing.T) {
	comp := &Comparison{TaskID: "T1"}
	sel := &Selection{TaskID: "T1"} // Chosen.Action == "" → !HasChoice()

	r := NewReflector()
	rs := r.Reflect([]*Comparison{comp}, []*Selection{sel}, nil)

	if len(rs.Reflections) != 0 {
		t.Fatalf("sin elección, no esperaba ninguna Reflection, obtuve %d", len(rs.Reflections))
	}
}

// TestReflect_EmparejaPorTaskIDNoPorOrden confirma que Comparison y
// Selection se emparejan por TaskID, no por posición en el slice —
// importante porque ambos vienen de llamadas separadas (CompareAll,
// SelectAll) que, aunque hoy conserven el mismo orden, no deberían
// tener que hacerlo para que esto funcione.
func TestReflect_EmparejaPorTaskIDNoPorOrden(t *testing.T) {
	compT2 := &Comparison{
		TaskID: "T2",
		Notes: []ComparisonNote{
			{Candidate: Candidate{Action: "a", Label: "A", Similarity: 0.5}, Rank: 1},
			{Candidate: Candidate{Action: "b", Label: "B", Similarity: 0.45}, Rank: 2, Gap: 0.05, WasClose: true},
		},
	}
	compT1 := &Comparison{TaskID: "T1"} // sin Notes, nada real

	selT1 := &Selection{TaskID: "T1", Chosen: Candidate{Action: "x", Label: "X"}}
	selT2 := &Selection{TaskID: "T2", Chosen: Candidate{Action: "a", Label: "A"}}

	r := NewReflector()
	// Orden deliberadamente cruzado entre los dos slices.
	rs := r.Reflect([]*Comparison{compT1, compT2}, []*Selection{selT2, selT1}, nil)

	if len(rs.Reflections) != 1 || rs.Reflections[0].TaskID != "T2" {
		t.Fatalf("esperaba una sola Reflection para T2 (emparejada por TaskID), obtuve %+v", rs.Reflections)
	}
}

// TestReflect_IncluyeNotaDeAdaptiveConfidence confirma que, si se pasa
// un *AdaptiveAction (Bloque 40), Describe() lo incluye al final.
func TestReflect_IncluyeNotaDeAdaptiveConfidence(t *testing.T) {
	action := &AdaptiveAction{Tier: TierExplore, Reason: "Confianza 60% — nota de cierre."}

	r := NewReflector()
	rs := r.Reflect(nil, nil, action)

	desc := rs.Describe()
	if !strings.Contains(desc, "nota de cierre") {
		t.Errorf("esperaba que Describe() incluyera la nota de AdaptiveAction, obtuve: %q", desc)
	}
}

// TestReflectionSet_NeedsReconsideration confirma que el filtro
// devuelve solo las Reflection con ShouldReconsider=true.
func TestReflectionSet_NeedsReconsideration(t *testing.T) {
	rs := &ReflectionSet{Reflections: []Reflection{
		{TaskID: "T1", ShouldReconsider: true},
		{TaskID: "T2", ShouldReconsider: false},
		{TaskID: "T3", ShouldReconsider: true},
	}}

	needs := rs.NeedsReconsideration()

	if len(needs) != 2 {
		t.Fatalf("esperaba 2 reflexiones que necesitan reconsideración, obtuve %d", len(needs))
	}
}

// TestParseReflectionQuery_DistintoDeComparatorYStrategy confirma que
// el trigger no se pisa con Comparator (38) ni Strategy Selector (39).
func TestParseReflectionQuery_DistintoDeComparatorYStrategy(t *testing.T) {
	if !ParseReflectionQuery(interaction.Analyze("te arrepientes de alguna eleccion", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Reflection Loop")
	}
	if ParseReflectionQuery(interaction.Analyze("compara las hipotesis", interaction.Context{})) {
		t.Error("un pedido de Comparator no debería disparar Reflection Loop")
	}
	if ParseReflectionQuery(interaction.Analyze("que estrategia elegirias", interaction.Context{})) {
		t.Error("un pedido de Strategy Selector no debería disparar Reflection Loop")
	}
}
