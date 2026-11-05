package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"

	memoryintelligence "ahmelia/memory_intelligence"
)

// TestAnalyzeAction_SinLearningStateNoTieneHistoria confirma que, sin
// LearningState, ninguna Action reporta fallos — no inventa datos.
func TestAnalyzeAction_SinLearningStateNoTieneHistoria(t *testing.T) {
	fa := NewFailureAnalyzer(nil)
	risk := fa.AnalyzeAction("abrir_programa", "Abrir programa")

	if risk.HasHistory() {
		t.Fatal("HasHistory() = true sin LearningState, esperaba false")
	}
}

// TestAnalyzeAction_SinRegistroParaEsaAction confirma que una Action
// que nunca falló (o nunca se ejecutó) tampoco tiene historial,
// aunque LearningState sí exista.
func TestAnalyzeAction_SinRegistroParaEsaAction(t *testing.T) {
	ls := memoryintelligence.NewLearningState()
	fa := NewFailureAnalyzer(memoryintelligence.NewLearningStore(ls))
	risk := fa.AnalyzeAction("mover_archivo", "Mover archivo")

	if risk.HasHistory() {
		t.Fatal("HasHistory() = true para una Action sin registros, esperaba false")
	}
}

// TestAnalyzeAction_CuentaFallosYCategoriaMasComun confirma el aporte
// central del bloque: no solo CUÁNTO falló, sino POR QUÉ (categoría
// más común).
func TestAnalyzeAction_CuentaFallosYCategoriaMasComun(t *testing.T) {
	ls := memoryintelligence.NewLearningState()
	ls.FailurePatternsByAction["abrir_programa"] = map[string]int{
		memoryintelligence.CategoryProgramNotFound: 3,
		memoryintelligence.CategoryWindowNotFound:  1,
	}

	fa := NewFailureAnalyzer(memoryintelligence.NewLearningStore(ls))
	risk := fa.AnalyzeAction("abrir_programa", "Abrir programa")

	if risk.TotalFailures != 4 {
		t.Fatalf("TotalFailures = %d, esperaba 4 (3+1)", risk.TotalFailures)
	}
	if risk.MostCommonCategory != memoryintelligence.CategoryProgramNotFound {
		t.Fatalf("MostCommonCategory = %q, esperaba %q", risk.MostCommonCategory, memoryintelligence.CategoryProgramNotFound)
	}
	if risk.MostCommonCount != 3 {
		t.Fatalf("MostCommonCount = %d, esperaba 3", risk.MostCommonCount)
	}
}

// TestAnalyzeAction_DesempateAlfabetico confirma que un empate exacto
// entre dos categorías siempre resuelve igual (determinismo, no el
// orden aleatorio de un map).
func TestAnalyzeAction_DesempateAlfabetico(t *testing.T) {
	ls := memoryintelligence.NewLearningState()
	ls.FailurePatternsByAction["a"] = map[string]int{
		memoryintelligence.CategoryWindowNotFound: 2,
		memoryintelligence.CategoryFileNotFound:   2,
	}

	fa := NewFailureAnalyzer(memoryintelligence.NewLearningStore(ls))
	for i := 0; i < 5; i++ {
		risk := fa.AnalyzeAction("a", "A")
		if risk.MostCommonCategory != memoryintelligence.CategoryFileNotFound {
			t.Fatalf("corrida %d: MostCommonCategory = %q, esperaba %q (alfabéticamente primero en empate)",
				i, risk.MostCommonCategory, memoryintelligence.CategoryFileNotFound)
		}
	}
}

// TestAnalyzeSelection_SinEleccionDaNil confirma el criterio
// defensivo estándar.
func TestAnalyzeSelection_SinEleccionDaNil(t *testing.T) {
	fa := NewFailureAnalyzer(nil)
	if fa.AnalyzeSelection(nil) != nil {
		t.Fatal("AnalyzeSelection(nil) debería devolver nil")
	}
	if fa.AnalyzeSelection(&Selection{}) != nil {
		t.Fatal("AnalyzeSelection de una Selection sin Chosen debería devolver nil")
	}
}

// TestAnalyzeSelection_UsaLaActionElegidaNoLaDeMayorSimilarity
// confirma que Failure Analysis mira lo que Strategy Selector eligió
// de verdad, no ClosestAction a secas.
func TestAnalyzeSelection_UsaLaActionElegidaNoLaDeMayorSimilarity(t *testing.T) {
	ls := memoryintelligence.NewLearningState()
	ls.FailurePatternsByAction["b"] = map[string]int{memoryintelligence.CategoryFileNotFound: 1}

	sel := &Selection{Chosen: Candidate{Action: "b", Label: "B"}} // Strategy Selector ya decidió B, no A
	fa := NewFailureAnalyzer(memoryintelligence.NewLearningStore(ls))
	risk := fa.AnalyzeSelection(sel)

	if risk.Action != "b" {
		t.Fatalf("risk.Action = %q, esperaba \"b\" (la Action elegida)", risk.Action)
	}
}

// TestDescribeRisks_OmiteLasSinHistoria confirma que la narrativa
// conjunta no menciona Actions sin ningún fallo registrado.
func TestDescribeRisks_OmiteLasSinHistoria(t *testing.T) {
	risks := []*FailureRisk{
		{Action: "a", Label: "Sin fallos"}, // sin historia -> se omite
		{Action: "b", Label: "Con fallos", TotalFailures: 2, MostCommonCategory: memoryintelligence.CategoryFileNotFound, MostCommonCount: 2},
	}
	desc := DescribeRisks(risks)

	if strings.Contains(desc, "Sin fallos") {
		t.Errorf("DescribeRisks() = %q, no debería mencionar la Action sin historia", desc)
	}
	if !strings.Contains(desc, "Con fallos") {
		t.Errorf("DescribeRisks() = %q, debería mencionar la Action con historia", desc)
	}
}

// TestDescribeRisks_TodasSinHistoriaDaMensajeHonesto confirma el
// mensaje explícito cuando nadie tiene fallos registrados.
func TestDescribeRisks_TodasSinHistoriaDaMensajeHonesto(t *testing.T) {
	risks := []*FailureRisk{{Action: "a", Label: "A"}}
	desc := DescribeRisks(risks)
	if !strings.Contains(desc, "Ninguna") {
		t.Fatalf("DescribeRisks() = %q, esperaba avisar que nadie tiene fallos registrados", desc)
	}
}

// TestParseFailureQuery_NoPisaOtrosTriggers confirma que no colisiona
// con Strategy Selector (39) ni Causal Reasoning (43).
func TestParseFailureQuery_NoPisaOtrosTriggers(t *testing.T) {
	if !ParseFailureQuery(interaction.Analyze("que riesgo de fallo tiene esto", interaction.Context{})) {
		t.Fatal("ParseFailureQuery debería activarse con su propio trigger")
	}
	if ParseFailureQuery(interaction.Analyze("que estrategia elegirias", interaction.Context{})) {
		t.Fatal("ParseFailureQuery no debería activarse con el trigger de Strategy Selector")
	}
	if ParseFailureQuery(interaction.Analyze("por que esta bloqueada", interaction.Context{})) {
		t.Fatal("ParseFailureQuery no debería activarse con el trigger de Causal Reasoning")
	}
}
