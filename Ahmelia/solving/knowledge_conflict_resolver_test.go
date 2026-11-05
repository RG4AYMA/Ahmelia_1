package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestResolve_MenosDeDosFuentesNoAplica confirma que, con 0 o 1 fuente
// confiable, Applies queda en false — comparar no tiene sentido con
// una sola fuente.
func TestResolve_MenosDeDosFuentesNoAplica(t *testing.T) {
	cr := NewConflictResolver()

	if report := cr.Resolve(nil); report.Applies {
		t.Error("vs nil no debería marcar Applies=true")
	}

	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", strings.Repeat("contenido real de sobra para pasar el minimo. ", 10)), StatusOK: true, HasContent: true},
	}}
	report := cr.Resolve(vs)
	if report.Applies {
		t.Error("con una sola fuente confiable, Applies debería ser false")
	}
	if report.HasConflict() {
		t.Error("sin Applies, HasConflict() debería ser false también")
	}
}

// TestResolve_MismaFuenteNoSeComparaConsigoMisma confirma que dos
// documentos de la MISMA Source se deduplican antes de comparar — no
// es una segunda opinión, es la misma fuente hablando dos veces.
func TestResolve_MismaFuenteNoSeComparaConsigoMisma(t *testing.T) {
	cr := NewConflictResolver()
	text := strings.Repeat("contenido real de sobra para pasar el minimo requerido. ", 10)
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", text), StatusOK: true, HasContent: true},
		{Document: docStub("https://go.dev/pkg/", "Go Docs", text), StatusOK: true, HasContent: true},
	}}
	report := cr.Resolve(vs)
	if report.Applies {
		t.Error("dos documentos de la misma Source no deberían habilitar la comparación")
	}
}

// TestResolve_ContenidoSimilarNoDiverge confirma que dos fuentes
// distintas con vocabulario muy similar NO se marcan como divergentes.
func TestResolve_ContenidoSimilarNoDiverge(t *testing.T) {
	cr := NewConflictResolver()
	textA := "Unreal Engine requiere Windows diez para instalar el editor correctamente en tu computadora personal."
	textB := "Unreal Engine requiere Windows diez para instalar el editor correctamente en tu maquina personal tambien."
	vs := &ValidationSet{Goal: "unreal engine", Validations: []*SourceValidation{
		{Document: docStub("https://dev.epicgames.com/", "Epic Games Docs", textA), StatusOK: true, HasContent: true},
		{Document: docStub("https://docs.unrealengine.com/", "Unreal Docs", textB), StatusOK: true, HasContent: true},
	}}
	// nota: textos cortos, así que forzamos HasContent para el test —
	// lo que importa acá es el Overlap, no MinContentLength (53).

	report := cr.Resolve(vs)
	if !report.Applies {
		t.Fatal("con dos fuentes distintas confiables, Applies debería ser true")
	}
	if report.HasConflict() {
		t.Errorf("textos con vocabulario casi idéntico no deberían divergir, pares: %+v", report.Pairs)
	}
}

// TestResolve_ContenidoDistintoDiverge confirma el caso central: dos
// fuentes distintas sin vocabulario compartido SÍ se marcan como
// divergentes.
func TestResolve_ContenidoDistintoDiverge(t *testing.T) {
	cr := NewConflictResolver()
	textA := "Unreal Engine requiere Windows diez para instalar el editor correctamente."
	textB := "Python permite programar scripts sencillos para automatizar tareas cotidianas."
	vs := &ValidationSet{Goal: "unreal engine", Validations: []*SourceValidation{
		{Document: docStub("https://dev.epicgames.com/", "Epic Games Docs", textA), StatusOK: true, HasContent: true},
		{Document: docStub("https://docs.unrealengine.com/", "Unreal Docs", textB), StatusOK: true, HasContent: true},
	}}

	report := cr.Resolve(vs)
	if !report.Applies {
		t.Fatal("con dos fuentes distintas confiables, Applies debería ser true")
	}
	if !report.HasConflict() {
		t.Fatalf("textos sin vocabulario compartido deberían divergir, pares: %+v", report.Pairs)
	}
}

// TestResolve_FuenteNoConfiableSeExcluye confirma que una fuente que
// no pasó IsReliable() nunca entra a la comparación.
func TestResolve_FuenteNoConfiableSeExcluye(t *testing.T) {
	cr := NewConflictResolver()
	vs := &ValidationSet{Goal: "unreal engine", Validations: []*SourceValidation{
		{Document: docStub("https://dev.epicgames.com/", "Epic Games Docs", "algo"), StatusOK: true, HasContent: true},
		{Document: docStub("https://docs.unrealengine.com/", "Unreal Docs", "poco"), StatusOK: false, HasContent: false},
	}}
	report := cr.Resolve(vs)
	if report.Applies {
		t.Error("con solo una fuente confiable (la otra falló), Applies debería ser false")
	}
}

// TestJaccardOverlap_TextoVacioNoRompe confirma el criterio defensivo:
// texto vacío o sin keywords significativas da Overlap 0, no un panic
// por división entre cero.
func TestJaccardOverlap_TextoVacioNoRompe(t *testing.T) {
	if got := jaccardOverlap("", ""); got != 0 {
		t.Errorf("esperaba 0 con ambos textos vacíos, obtuve %f", got)
	}
	if got := jaccardOverlap("algo real", ""); got != 0 {
		t.Errorf("esperaba 0 con un texto vacío, obtuve %f", got)
	}
}

// TestJaccardOverlap_TextoIdenticoDaUno confirma que el mismo texto
// comparado consigo mismo da superposición máxima.
func TestJaccardOverlap_TextoIdenticoDaUno(t *testing.T) {
	text := "unreal engine requiere windows diez para instalar el editor"
	if got := jaccardOverlap(text, text); got != 1 {
		t.Errorf("esperaba superposición 1.0 con el mismo texto, obtuve %f", got)
	}
}

// TestConflictReport_Describe_NoAplica confirma el mensaje cuando
// Applies es false.
func TestConflictReport_Describe_NoAplica(t *testing.T) {
	report := &ConflictReport{Goal: "go"}
	desc := report.Describe()
	if !strings.Contains(desc, "no aplica") {
		t.Errorf("esperaba el mensaje de \"no aplica\", obtuve: %q", desc)
	}
}

// TestConflictReport_Describe_SinDivergencia confirma el mensaje
// cuando hay pares comparados pero ninguno divergió.
func TestConflictReport_Describe_SinDivergencia(t *testing.T) {
	report := &ConflictReport{
		Goal: "go", Applies: true,
		Pairs: []SourcePairComparison{{SourceA: "A", SourceB: "B", Overlap: 0.9, Diverges: false}},
	}
	desc := report.Describe()
	if !strings.Contains(desc, "sin señales de divergencia") {
		t.Errorf("esperaba el mensaje de \"sin divergencia\", obtuve: %q", desc)
	}
}

// TestConflictReport_Describe_ConDivergencia confirma que el mensaje
// nombra las fuentes divergentes y es explícito sobre no decidir cuál
// es correcta.
func TestConflictReport_Describe_ConDivergencia(t *testing.T) {
	report := &ConflictReport{
		Goal: "go", Applies: true,
		Pairs: []SourcePairComparison{{SourceA: "A", SourceB: "B", Overlap: 0.05, Diverges: true}},
	}
	desc := report.Describe()
	if !strings.Contains(desc, "A vs B") {
		t.Errorf("esperaba que el mensaje nombrara el par divergente, obtuve: %q", desc)
	}
	if !strings.Contains(desc, "revisión manual") {
		t.Errorf("esperaba que el mensaje fuera explícito sobre marcar para revisión manual, obtuve: %q", desc)
	}
}

// TestParseConflictQuery_DistintoDeSourceValidationYUpdateCheck
// confirma que el trigger no se pisa con Source Validation (53) ni
// Update Detector (58).
func TestParseConflictQuery_DistintoDeSourceValidationYUpdateCheck(t *testing.T) {
	if !ParseConflictQuery(interaction.Analyze("hay conflicto entre las fuentes sobre go", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Knowledge Conflict Resolver")
	}
	if ParseConflictQuery(interaction.Analyze("esa fuente es confiable", interaction.Context{})) {
		t.Error("un pedido de Source Validation (53) no debería disparar Conflict Resolver")
	}
	if ParseConflictQuery(interaction.Analyze("sigue actualizada la nota sobre go", interaction.Context{})) {
		t.Error("un pedido de Update Detector (58) no debería disparar Conflict Resolver")
	}
}
