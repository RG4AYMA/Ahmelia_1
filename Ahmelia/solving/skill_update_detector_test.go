package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"

	"ahmelia/gateway"
)

// TestCheck_SinNotaExistenteNoAplica confirma que, sin nota guardada,
// NeedsUpdate() queda en false — "no aplica" no es lo mismo que "no
// cambió".
func TestCheck_SinNotaExistenteNoAplica(t *testing.T) {
	ud := NewUpdateDetector()
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	check := ud.Check(item, nil, nil)

	if check.NeedsUpdate() {
		t.Fatal("sin nota existente, NeedsUpdate() debería ser false")
	}
	if !strings.Contains(check.Reason, "no hay ninguna nota guardada") {
		t.Errorf("esperaba el motivo de \"no aplica\", obtuve: %q", check.Reason)
	}
}

// TestCheck_ContenidoIdenticoNoNecesitaActualizar confirma el caso
// central: si Build() con documentos frescos arma exactamente el mismo
// Content que la nota guardada, Changed queda en false.
func TestCheck_ContenidoIdenticoNoNecesitaActualizar(t *testing.T) {
	existing := &Skill{SkillID: "Go", Label: "aprender go", Content: "[Go Docs] mismo contenido de siempre."}
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", "mismo contenido de siempre."), StatusOK: true, HasContent: true},
	}}
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	ud := NewUpdateDetector()
	check := ud.Check(item, existing, vs)

	if check.NeedsUpdate() {
		t.Fatalf("con contenido idéntico, NeedsUpdate() debería ser false, Reason=%q", check.Reason)
	}
	if !strings.Contains(check.Reason, "coincide") {
		t.Errorf("esperaba el motivo de \"sin cambios\", obtuve: %q", check.Reason)
	}
}

// TestCheck_ContenidoDistintoNecesitaActualizar confirma que un
// Content distinto del que trae Build() ahora marca Changed=true.
func TestCheck_ContenidoDistintoNecesitaActualizar(t *testing.T) {
	existing := &Skill{SkillID: "Go", Label: "aprender go", Content: "[Go Docs] contenido viejo de hace meses."}
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", "contenido nuevo, la documentación cambió."), StatusOK: true, HasContent: true},
	}}
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	ud := NewUpdateDetector()
	check := ud.Check(item, existing, vs)

	if !check.NeedsUpdate() {
		t.Fatal("con contenido distinto, NeedsUpdate() debería ser true")
	}
	if check.Candidate == nil {
		t.Fatal("esperaba que Candidate quedara poblado con lo que Build() armó ahora")
	}
}

// TestCheck_Delta_SinCambioEstructuralExplicaQueEsReformulacion
// (Integration Fix) confirma que, cuando el Content cambió pero
// ningún campo estructurado contable varió (ambos textos son prosa
// sin Steps/Prerequisites/etc.), Delta lo dice explícitamente en vez
// de quedar vacío sin explicación, Y StructuralChange queda en false
// — la señal que usa Skill Versioning (59) para no archivar una
// versión nueva por una reformulación cosmética.
func TestCheck_Delta_SinCambioEstructuralExplicaQueEsReformulacion(t *testing.T) {
	existing := &Skill{SkillID: "Go", Label: "aprender go", Content: "[Go Docs] Go es un lenguaje de programación viejo."}
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", "Go es un lenguaje de programación moderno."), StatusOK: true, HasContent: true},
	}}
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	check := NewUpdateDetector().Check(item, existing, vs)

	if !check.NeedsUpdate() {
		t.Fatal("esperaba Changed=true (Content distinto)")
	}
	if !strings.Contains(check.Delta, "reformulado") {
		t.Errorf("Delta debería explicar que ningún campo estructurado varió, obtuve: %q", check.Delta)
	}
	if check.StructuralChange {
		t.Error("StructuralChange debería ser false para una reformulación puramente cosmética")
	}
}

// TestCheck_Delta_DetectaPasosNuevos (Integration Fix) confirma el
// caso central que motivó este cambio: la fuente nueva agrega una
// lista de pasos real que la guardada no tenía — Delta debe reportar
// la transición de State y el conteo de pasos, no solo "cambió".
func TestCheck_Delta_DetectaPasosNuevos(t *testing.T) {
	existing := &Skill{SkillID: "Go", Label: "aprender go", Content: "[Go Docs] Go es un lenguaje de programación."}
	nuevoTexto := "Para instalar Go, segui estos pasos:\n" +
		"1. Descarga el instalador\n" +
		"2. Ejecuta el instalador\n"
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", nuevoTexto), StatusOK: true, HasContent: true},
	}}
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	check := NewUpdateDetector().Check(item, existing, vs)

	if !check.NeedsUpdate() {
		t.Fatal("esperaba Changed=true")
	}
	if !strings.Contains(check.Delta, "pasos: 0 → 2") {
		t.Errorf("Delta debería reportar el conteo de pasos nuevo, obtuve: %q", check.Delta)
	}
	if !strings.Contains(check.Delta, "estado:") {
		t.Errorf("Delta debería reportar la transición de State (sin estructura → procedural), obtuve: %q", check.Delta)
	}
	if !check.StructuralChange {
		t.Error("StructuralChange debería ser true — hay pasos y estado nuevos, no es una reformulación cosmética")
	}
}

// TestUpdateCheck_Describe_ConDeltaLoIncluyeEnLaNarrativa (Integration
// Fix) confirma que Describe() muestra el Delta cuando existe, sin
// romper el formato para cuando no hay Delta calculado (ej. un
// UpdateCheck armado a mano, como TestUpdateCheck_Describe_ConCambio).
func TestUpdateCheck_Describe_ConDeltaLoIncluyeEnLaNarrativa(t *testing.T) {
	check := &UpdateCheck{
		Existing: &Skill{Label: "aprender go"},
		Changed:  true,
		Delta:    "pasos: 0 → 2",
		Reason:   "el contenido que traje ahora es distinto del que tenía guardado — la fuente pudo haber cambiado desde que armé la nota.",
	}
	desc := check.Describe()
	if !strings.Contains(desc, "pasos: 0 → 2") {
		t.Errorf("Describe() debería incluir el Delta, obtuve: %q", desc)
	}
}

// TestCheck_SinDocumentosConfiablesAhoraMantieneLoExistente confirma
// que, si el fetch nuevo no trae NINGÚN documento confiable, no se
// asume que la fuente desapareció ni que cambió — se mantiene la nota
// existente tal cual, sin marcar Changed.
func TestCheck_SinDocumentosConfiablesAhoraMantieneLoExistente(t *testing.T) {
	existing := &Skill{SkillID: "Go", Label: "aprender go", Content: "[Go Docs] lo que había."}
	vs := &ValidationSet{Goal: "aprender go", Validations: []*SourceValidation{
		{Document: docStub("https://go.dev/", "Go Docs", "poco"), StatusOK: true, HasContent: false},
	}}
	item := LearningItem{TaskID: "Go", TaskDesc: "aprender go"}

	ud := NewUpdateDetector()
	check := ud.Check(item, existing, vs)

	if check.NeedsUpdate() {
		t.Fatal("sin documentos confiables ahora, no debería marcarse Changed")
	}
	if check.Candidate != nil {
		t.Error("sin documentos confiables, Candidate debería quedar nil")
	}
	if !strings.Contains(check.Reason, "no encontré ningún documento confiable") {
		t.Errorf("esperaba el motivo correspondiente, obtuve: %q", check.Reason)
	}
}

// TestDescribe_SinNotaExistente confirma el formato de Describe()
// cuando Existing es nil.
func TestUpdateCheck_Describe_SinNotaExistente(t *testing.T) {
	check := &UpdateCheck{SkillID: "Go", Reason: "no hay ninguna nota guardada para este tema todavía — comparar no aplica, hace falta construirla primero."}
	desc := check.Describe()

	if !strings.HasPrefix(desc, "Go:") {
		t.Errorf("esperaba que Describe() empezara con el SkillID, obtuve: %q", desc)
	}
}

// TestDescribe_ConCambio confirma que Describe() marca "cambió" cuando
// corresponde.
func TestUpdateCheck_Describe_ConCambio(t *testing.T) {
	check := &UpdateCheck{
		Existing: &Skill{Label: "aprender go"},
		Changed:  true,
		Reason:   "el contenido que traje ahora es distinto del que tenía guardado — la fuente pudo haber cambiado desde que armé la nota.",
	}
	desc := check.Describe()

	if !strings.Contains(desc, "cambió") {
		t.Errorf("esperaba que Describe() marcara \"cambió\", obtuve: %q", desc)
	}
}

// TestParseUpdateCheckQuery_DistintoDeSkillBuildYSkillContext confirma
// que el trigger no se pisa con Skill Build (54) ni Skill Context (55).
func TestParseUpdateCheckQuery_DistintoDeSkillBuildYSkillContext(t *testing.T) {
	if !ParseUpdateCheckQuery(interaction.Analyze("sigue actualizada la nota sobre go", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Skill Update Detector")
	}
	if ParseUpdateCheckQuery(interaction.Analyze("guarda una nota sobre go", interaction.Context{})) {
		t.Error("un pedido de Skill Build (54) no debería disparar Skill Update Detector")
	}
	if ParseUpdateCheckQuery(interaction.Analyze("actualiza el curriculum para go", interaction.Context{})) {
		t.Error("un pedido de Skill Integration (55) no debería disparar Skill Update Detector")
	}
}

// docStub arma un *gateway.Document mínimo para los tests de este
// archivo, sin necesitar un fetch real — mismo criterio que el resto
// de Fase 4.1 (construir structs a mano en vez de salir a la red para
// probar lógica pura).
func docStub(url, source, text string) *gateway.Document {
	return &gateway.Document{URL: url, Source: source, Text: text, StatusCode: 200}
}
