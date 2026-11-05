package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"

	memoryintelligence "ahmelia/memory_intelligence"
)

// TestValidate_SinEventosEsSinDatos confirma que una Action sin ningún
// evento en el EventLog queda StatusNoData, no un 0% de confianza
// engañoso.
func TestValidate_SinEventosEsSinDatos(t *testing.T) {
	log := &memoryintelligence.EventLog{}
	v := NewValidator(memoryintelligence.NewEventStore(log), memoryintelligence.NewLearningStore(memoryintelligence.NewLearningState()))

	val := v.Validate("abrir_programa", "Abrir programas")
	if val.Status != StatusNoData {
		t.Fatalf("esperaba StatusNoData sin eventos, obtuve %q", val.Status)
	}
	if val.IsReliable() {
		t.Fatal("sin datos no debería ser IsReliable()")
	}
}

// TestValidate_PocasMuestrasEsTentativa confirma el caso central: menos
// de MinSampleSize intentos, aunque la confianza calculada sea alta
// (100%), queda como tentativa, no validada.
func TestValidate_PocasMuestrasEsTentativa(t *testing.T) {
	log := &memoryintelligence.EventLog{Events: []memoryintelligence.RawEvent{
		{Action: "crear_carpeta", Outcome: memoryintelligence.Outcome{Success: true}},
	}}
	ls := memoryintelligence.NewLearningState()
	ls.ActionConfidence["crear_carpeta"] = 100

	v := NewValidator(memoryintelligence.NewEventStore(log), memoryintelligence.NewLearningStore(ls))
	val := v.Validate("crear_carpeta", "Crear carpetas")

	if val.Status != StatusTentative {
		t.Fatalf("esperaba StatusTentative con 1 muestra (< MinSampleSize=%d), obtuve %q", MinSampleSize, val.Status)
	}
	if val.IsReliable() {
		t.Fatal("una validación tentativa no debería ser IsReliable()")
	}
	if val.Confidence != 100 {
		t.Fatalf("esperaba que Confidence se tome de LearningState igual (100), obtuve %.0f", val.Confidence)
	}
}

// TestValidate_MuchasMuestrasEsValidada confirma que, con
// MinSampleSize intentos o más, la validación pasa a StatusValidated y
// IsReliable()==true.
func TestValidate_MuchasMuestrasEsValidada(t *testing.T) {
	log := &memoryintelligence.EventLog{}
	for i := 0; i < MinSampleSize; i++ {
		log.Events = append(log.Events, memoryintelligence.RawEvent{
			Action:  "buscar_archivo",
			Outcome: memoryintelligence.Outcome{Success: true},
		})
	}
	ls := memoryintelligence.NewLearningState()
	ls.ActionConfidence["buscar_archivo"] = 90

	v := NewValidator(memoryintelligence.NewEventStore(log), memoryintelligence.NewLearningStore(ls))
	val := v.Validate("buscar_archivo", "Buscar archivos")

	if val.Status != StatusValidated {
		t.Fatalf("esperaba StatusValidated con %d muestras, obtuve %q", MinSampleSize, val.Status)
	}
	if !val.IsReliable() {
		t.Fatal("con suficientes muestras, IsReliable() debería ser true")
	}
}

// TestValidateAll_EncadenaSobreRecommendations confirma que
// ValidateAll consume la salida real de Learning Integration (Bloque
// 45), no un tipo inventado aparte.
func TestValidateAll_EncadenaSobreRecommendations(t *testing.T) {
	log := &memoryintelligence.EventLog{Events: []memoryintelligence.RawEvent{
		{Action: "abrir_programa", Outcome: memoryintelligence.Outcome{Success: true}},
	}}
	v := NewValidator(memoryintelligence.NewEventStore(log), memoryintelligence.NewLearningStore(memoryintelligence.NewLearningState()))

	recs := []*StrategyRecommendation{
		{Action: "abrir_programa", Label: "Abrir programas"},
	}
	vals := v.ValidateAll(recs)

	if len(vals) != 1 || vals[0].Action != "abrir_programa" {
		t.Fatalf("esperaba 1 validación para abrir_programa, obtuve %+v", vals)
	}
}

// TestDescribeValidations_ResumeValidadasListaTentativasYSinDatos
// confirma que Describe agrupa las validadas en un conteo y detalla
// las tentativas/sin-datos explícitamente, no todas mezcladas igual.
func TestDescribeValidations_ResumeValidadasListaTentativasYSinDatos(t *testing.T) {
	vals := []*Validation{
		{Label: "Buscar archivos", Status: StatusValidated},
		{Label: "Crear carpetas", Status: StatusTentative, Confidence: 100, Samples: 1},
		{Label: "Controlar volumen", Status: StatusNoData},
	}
	desc := DescribeValidations(vals)

	if !strings.Contains(desc, "1 Action") {
		t.Errorf("esperaba mencionar el conteo de validadas, obtuve: %q", desc)
	}
	if !strings.Contains(desc, "Crear carpetas") {
		t.Errorf("esperaba detallar la tentativa, obtuve: %q", desc)
	}
	if !strings.Contains(desc, "Controlar volumen") {
		t.Errorf("esperaba listar la sin-datos, obtuve: %q", desc)
	}
}

// TestParseValidatorQuery_DistintoDeLearningIntegration confirma que
// los triggers de Learning Validator no se pisan con Learning
// Integration (45).
func TestParseValidatorQuery_DistintoDeLearningIntegration(t *testing.T) {
	if !ParseValidatorQuery(interaction.Analyze("puedo confiar en esa confianza", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Learning Validator")
	}
	if ParseValidatorQuery(interaction.Analyze("con que estrategia lo intentarias", interaction.Context{})) {
		t.Error("un pedido de Learning Integration no debería disparar Learning Validator")
	}
}
