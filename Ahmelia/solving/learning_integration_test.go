package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"

	memoryintelligence "ahmelia/memory_intelligence"
)

// buildLearningState arma un LearningState con una sola Action
// registrada (Strategies + ActionConfidence), para controlar el
// escenario en los tests.
func buildLearningState(action string, primary, secondary, fallback string, confidence float64) *memoryintelligence.LearningState {
	ls := memoryintelligence.NewLearningState()
	ls.Strategies[action] = &memoryintelligence.StrategySet{Primary: primary, Secondary: secondary, Fallback: fallback}
	ls.ActionConfidence[action] = confidence
	return ls
}

// TestRecommendFor_SinLearningStateSinRecomendacion confirma que, sin
// LearningState, no hay ninguna recomendación — no inventa una.
func TestRecommendFor_SinLearningStateSinRecomendacion(t *testing.T) {
	li := NewLearningIntegrator(nil)
	rec := li.RecommendFor("abrir_programa", "Abrir programa")

	if rec.HasRecommendation() {
		t.Fatal("HasRecommendation() = true sin LearningState, esperaba false")
	}
}

// TestRecommendFor_ConfianzaAltaMantienePrimaria confirma que, con
// confianza alta, la recomendación es la estrategia primaria —
// IsPrimary=true, nada que destacar.
func TestRecommendFor_ConfianzaAltaMantienePrimaria(t *testing.T) {
	ls := buildLearningState("abrir_programa", "directo", "con reintentos", "alternativa", 90)
	li := NewLearningIntegrator(memoryintelligence.NewLearningStore(ls))
	rec := li.RecommendFor("abrir_programa", "Abrir programa")

	if rec.Strategy != "directo" {
		t.Fatalf("Strategy = %q, esperaba \"directo\" (confianza alta)", rec.Strategy)
	}
	if !rec.IsPrimary {
		t.Fatal("IsPrimary = false con confianza alta, esperaba true")
	}
}

// TestRecommendFor_ConfianzaBajaCambiaASecundaria confirma el aporte
// central del bloque: aprendizaje real cambiando la recomendación.
func TestRecommendFor_ConfianzaBajaCambiaASecundaria(t *testing.T) {
	ls := buildLearningState("abrir_programa", "directo", "con reintentos", "alternativa", 35)
	li := NewLearningIntegrator(memoryintelligence.NewLearningStore(ls))
	rec := li.RecommendFor("abrir_programa", "Abrir programa")

	if rec.Strategy != "con reintentos" {
		t.Fatalf("Strategy = %q, esperaba \"con reintentos\" (confianza 35%%, < 40)", rec.Strategy)
	}
	if rec.IsPrimary {
		t.Fatal("IsPrimary = true con confianza baja, esperaba false")
	}
}

// TestRecommendFor_ConfianzaMuyBajaCambiaAFallback confirma el otro
// umbral (<20).
func TestRecommendFor_ConfianzaMuyBajaCambiaAFallback(t *testing.T) {
	ls := buildLearningState("abrir_programa", "directo", "con reintentos", "alternativa", 10)
	li := NewLearningIntegrator(memoryintelligence.NewLearningStore(ls))
	rec := li.RecommendFor("abrir_programa", "Abrir programa")

	if rec.Strategy != "alternativa" {
		t.Fatalf("Strategy = %q, esperaba \"alternativa\" (confianza 10%%, < 20)", rec.Strategy)
	}
}

// TestRecommendForSelection_SinEleccionDaNil confirma el criterio
// defensivo estándar.
func TestRecommendForSelection_SinEleccionDaNil(t *testing.T) {
	li := NewLearningIntegrator(nil)
	if li.RecommendForSelection(nil) != nil {
		t.Fatal("RecommendForSelection(nil) debería devolver nil")
	}
	if li.RecommendForSelection(&Selection{}) != nil {
		t.Fatal("RecommendForSelection de una Selection sin Chosen debería devolver nil")
	}
}

// TestRecommendForSelection_UsaLaActionElegida confirma que se
// consulta la Action que Strategy Selector eligió de verdad, no
// ClosestAction a secas — mismo criterio que Failure Analysis (44).
func TestRecommendForSelection_UsaLaActionElegida(t *testing.T) {
	ls := buildLearningState("b", "directo", "con reintentos", "alternativa", 15)
	sel := &Selection{Chosen: Candidate{Action: "b", Label: "B"}}

	li := NewLearningIntegrator(memoryintelligence.NewLearningStore(ls))
	rec := li.RecommendForSelection(sel)

	if rec.Action != "b" {
		t.Fatalf("rec.Action = %q, esperaba \"b\"", rec.Action)
	}
	if rec.Strategy != "alternativa" {
		t.Fatalf("Strategy = %q, esperaba \"alternativa\"", rec.Strategy)
	}
}

// TestDescribeRecommendations_OmitePrimariasYSinDatos confirma que la
// narrativa conjunta solo menciona las Action donde el aprendizaje
// cambió algo — no las que se quedan con la estrategia habitual ni
// las que no tienen dato.
func TestDescribeRecommendations_OmitePrimariasYSinDatos(t *testing.T) {
	recs := []*StrategyRecommendation{
		{Action: "a", Label: "Primaria sin cambio", Strategy: "directo", IsPrimary: true},
		{Action: "b", Label: "Sin dato"}, // Strategy vacío
		{Action: "c", Label: "Cambio real", Strategy: "con reintentos", Confidence: 30, IsPrimary: false},
	}
	desc := DescribeRecommendations(recs)

	if strings.Contains(desc, "Primaria sin cambio") {
		t.Errorf("DescribeRecommendations() = %q, no debería mencionar la que se queda en la primaria", desc)
	}
	if strings.Contains(desc, "Sin dato") {
		t.Errorf("DescribeRecommendations() = %q, no debería mencionar la que no tiene dato", desc)
	}
	if !strings.Contains(desc, "Cambio real") {
		t.Errorf("DescribeRecommendations() = %q, debería mencionar la que sí cambió", desc)
	}
}

// TestDescribeRecommendations_TodasPrimariasDaMensajeHonesto confirma
// el mensaje explícito cuando el aprendizaje no sugiere ningún cambio
// en absoluto.
func TestDescribeRecommendations_TodasPrimariasDaMensajeHonesto(t *testing.T) {
	recs := []*StrategyRecommendation{{Action: "a", Label: "A", Strategy: "directo", IsPrimary: true}}
	desc := DescribeRecommendations(recs)
	if !strings.Contains(desc, "habitual") {
		t.Fatalf("DescribeRecommendations() = %q, esperaba avisar que no hay cambios sugeridos", desc)
	}
}

// TestParseLearningIntegrationQuery_NoPisaStrategyQuery confirma que
// el trigger nuevo no colisiona con el de Strategy Selector (Bloque 39).
func TestParseLearningIntegrationQuery_NoPisaStrategyQuery(t *testing.T) {
	if !ParseLearningIntegrationQuery(interaction.Analyze("con que estrategia lo intentarias", interaction.Context{})) {
		t.Fatal("ParseLearningIntegrationQuery debería activarse con su propio trigger")
	}
	if ParseLearningIntegrationQuery(interaction.Analyze("que estrategia elegirias", interaction.Context{})) {
		t.Fatal("ParseLearningIntegrationQuery no debería activarse con el trigger de Strategy Selector (Bloque 39)")
	}
}
