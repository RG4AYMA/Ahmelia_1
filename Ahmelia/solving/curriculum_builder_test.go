package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestBuild_ExcluyeGapsConLead confirma que un gap con HasLead()==true
// (Hypothesis Generator, 25) no entra al curriculum — ya tiene una
// pista, no necesita aprendizaje genuino.
func TestBuild_ExcluyeGapsConLead(t *testing.T) {
	hs := &HypothesisSet{Goal: "x", Hypotheses: []Hypothesis{
		{TaskID: "T1", TaskDesc: "abrir el programa", ClosestAction: "abrir_programa"},
	}}

	cb := NewCurriculumBuilder()
	c := cb.Build(hs, nil)

	if len(c.Items) != 0 {
		t.Fatalf("un gap con lead no debería entrar al curriculum, obtuve %+v", c.Items)
	}
}

// TestBuild_ExcluyeGapsConAnalogia confirma que un gap con
// HasAnalogy()==true (Analogical Reasoning, 28) tampoco entra, aunque
// no tenga lead — ya hay un antecedente real en el historial.
func TestBuild_ExcluyeGapsConAnalogia(t *testing.T) {
	hs := &HypothesisSet{Goal: "x", Hypotheses: []Hypothesis{
		{TaskID: "T1", TaskDesc: "renderizar en blender"}, // sin ClosestAction, sin lead
	}}
	as := &AnalogySet{Goal: "x", Analogies: []Analogy{
		{TaskID: "T1", PastAction: "abrir_programa", PastTarget: "blender"}, // HasAnalogy()==true
	}}

	cb := NewCurriculumBuilder()
	c := cb.Build(hs, as)

	if len(c.Items) != 0 {
		t.Fatalf("un gap con analogía no debería entrar al curriculum, obtuve %+v", c.Items)
	}
}

// TestBuild_IncluyeGapsGenuinos confirma el caso central: un gap sin
// lead y sin analogía entra al curriculum.
func TestBuild_IncluyeGapsGenuinos(t *testing.T) {
	hs := &HypothesisSet{Goal: "x", Hypotheses: []Hypothesis{
		{TaskID: "T1", TaskDesc: "diseñar el circuito del PCB", DownstreamBlocked: 3},
	}}
	as := &AnalogySet{Goal: "x", Analogies: []Analogy{
		{TaskID: "T1"}, // sin PastAction → HasAnalogy()==false
	}}

	cb := NewCurriculumBuilder()
	c := cb.Build(hs, as)

	if len(c.Items) != 1 {
		t.Fatalf("esperaba 1 item genuino, obtuve %d", len(c.Items))
	}
	if c.Items[0].Priority != 3 {
		t.Errorf("esperaba Priority=3 (reusado de DownstreamBlocked), obtuve %d", c.Items[0].Priority)
	}
}

// TestBuild_OrdenaPorPrioridadDescendente confirma que, con varios
// items genuinos, el orden final es por Priority descendente — lo que
// más desbloquea, primero.
func TestBuild_OrdenaPorPrioridadDescendente(t *testing.T) {
	hs := &HypothesisSet{Goal: "x", Hypotheses: []Hypothesis{
		{TaskID: "T1", TaskDesc: "gap chico", DownstreamBlocked: 1},
		{TaskID: "T2", TaskDesc: "gap grande", DownstreamBlocked: 5},
		{TaskID: "T3", TaskDesc: "gap mediano", DownstreamBlocked: 2},
	}}

	cb := NewCurriculumBuilder()
	c := cb.Build(hs, nil)

	if len(c.Items) != 3 {
		t.Fatalf("esperaba 3 items, obtuve %d", len(c.Items))
	}
	if c.Items[0].TaskID != "T2" || c.Items[1].TaskID != "T3" || c.Items[2].TaskID != "T1" {
		t.Fatalf("esperaba orden T2,T3,T1 por prioridad descendente, obtuve %s,%s,%s",
			c.Items[0].TaskID, c.Items[1].TaskID, c.Items[2].TaskID)
	}
}

// TestBuild_SinHypothesisSetDevuelveVacio confirma el criterio
// defensivo: hs nil no rompe, curriculum sale vacío.
func TestBuild_SinHypothesisSetDevuelveVacio(t *testing.T) {
	cb := NewCurriculumBuilder()
	c := cb.Build(nil, nil)

	if len(c.Items) != 0 {
		t.Fatalf("sin HypothesisSet, esperaba curriculum vacío, obtuve %+v", c.Items)
	}
}

// TestDescribe_VacioEsHonesto confirma que un curriculum vacío lo dice
// explícito, no una cadena vacía.
func TestDescribe_VacioEsHonesto(t *testing.T) {
	c := &Curriculum{Goal: "armar una tienda"}
	desc := c.Describe()

	if !strings.Contains(desc, "No hay nada que aprender de cero") {
		t.Errorf("esperaba el mensaje honesto de curriculum vacío, obtuve: %q", desc)
	}
}

// TestDescribe_MencionaCuantoDesbloquea confirma que Describe()
// menciona el impacto (Priority) cuando es mayor a cero.
func TestDescribe_MencionaCuantoDesbloquea(t *testing.T) {
	c := &Curriculum{Goal: "x", Items: []LearningItem{
		{TaskID: "T1", TaskDesc: "gap real", Reason: "sin pista ni antecedente", Priority: 4},
	}}

	desc := c.Describe()

	if !strings.Contains(desc, "desbloquea 4 tarea") {
		t.Errorf("esperaba que Describe() mencionara el impacto, obtuve: %q", desc)
	}
}

// TestParseCurriculumQuery_DistintoDeHypothesisYAnalogical confirma
// que el trigger no se pisa con Hypothesis Generator (25) ni
// Analogical Reasoning (28).
func TestParseCurriculumQuery_DistintoDeHypothesisYAnalogical(t *testing.T) {
	if !ParseCurriculumQuery(interaction.Analyze("que necesito aprender para armar una tienda online", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Curriculum Builder")
	}
	if ParseCurriculumQuery(interaction.Analyze("que hipotesis tienes para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Hypothesis Generator no debería disparar Curriculum Builder")
	}
	if ParseCurriculumQuery(interaction.Analyze("hiciste algo parecido a armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Analogical Reasoning no debería disparar Curriculum Builder")
	}
}
