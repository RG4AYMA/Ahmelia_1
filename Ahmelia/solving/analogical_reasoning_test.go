package solving

import (
	"ahmelia/interaction"
	"testing"

	memoryintelligence "ahmelia/memory_intelligence"
)

// TestFindAnalogies_EncuentraPorTargetParecido confirma la ruta
// central: un gap cuya descripción comparte palabras con el Target de
// un evento real del EventLog recibe ese Action+Target como analogía.
func TestFindAnalogies_EncuentraPorTargetParecido(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Configurar el servidor blender remoto", Known: false},
	}}
	log := &memoryintelligence.EventLog{Events: []memoryintelligence.RawEvent{
		{Action: "abrir_programa", Target: "blender", Outcome: memoryintelligence.Outcome{Success: true}},
	}}

	ar := NewAnalogicalReasoner()
	as := ar.FindAnalogies(km, log)

	if len(as.Analogies) != 1 {
		t.Fatalf("esperaba 1 analogía, obtuve %d", len(as.Analogies))
	}
	a := as.Analogies[0]
	if !a.HasAnalogy() {
		t.Fatalf("esperaba encontrar una analogía por 'blender', obtuve %+v", a)
	}
	if a.PastAction != "abrir_programa" || a.PastTarget != "blender" {
		t.Errorf("esperaba PastAction=abrir_programa PastTarget=blender, obtuve %q/%q", a.PastAction, a.PastTarget)
	}
	if a.Occurrences != 1 {
		t.Errorf("esperaba Occurrences=1, obtuve %d", a.Occurrences)
	}
}

// TestFindAnalogies_SinAntecedenteQuedaHonesto confirma que un gap sin
// ninguna palabra en común con ningún Target del EventLog queda con
// HasAnalogy()==false.
func TestFindAnalogies_SinAntecedenteQuedaHonesto(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Soldar componentes electronicos", Known: false},
	}}
	log := &memoryintelligence.EventLog{Events: []memoryintelligence.RawEvent{
		{Action: "abrir_programa", Target: "blender", Outcome: memoryintelligence.Outcome{Success: true}},
	}}

	ar := NewAnalogicalReasoner()
	as := ar.FindAnalogies(km, log)

	if as.Analogies[0].HasAnalogy() {
		t.Fatalf("no debería encontrar ninguna analogía para 'soldar componentes electronicos', obtuve %+v", as.Analogies[0])
	}
}

// TestFindAnalogies_SinEventLogNoRompe confirma que FindAnalogies(km,
// nil) no rompe — mismo criterio defensivo que el resto de Fase 3.
func TestFindAnalogies_SinEventLogNoRompe(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Diseñar el circuito del PCB", Known: false},
	}}

	ar := NewAnalogicalReasoner()
	as := ar.FindAnalogies(km, nil)

	if as.Analogies[0].HasAnalogy() {
		t.Fatalf("sin EventLog no debería encontrar ninguna analogía, obtuve %+v", as.Analogies[0])
	}
}

// TestFindAnalogies_UsaComputeExperienceDelBloque3 confirma que el
// SuccessRate reportado viene de memory_intelligence.ComputeExperience
// (Bloque 3), no de un cálculo propio — con 1 éxito y 1 fallo sobre el
// mismo par, el balance debe ser 50%.
func TestFindAnalogies_UsaComputeExperienceDelBloque3(t *testing.T) {
	km := &KnowledgeMap{Goal: "x", Matches: []KnowledgeMatch{
		{TaskID: "T1", TaskDesc: "Renderizar la escena en blender", Known: false},
	}}
	log := &memoryintelligence.EventLog{Events: []memoryintelligence.RawEvent{
		{Action: "abrir_programa", Target: "blender", Outcome: memoryintelligence.Outcome{Success: true}},
		{Action: "abrir_programa", Target: "blender", Outcome: memoryintelligence.Outcome{Success: false, Category: "program_not_found", Reason: "no encontrado"}},
	}}

	ar := NewAnalogicalReasoner()
	as := ar.FindAnalogies(km, log)

	a := as.Analogies[0]
	if a.Occurrences != 2 {
		t.Fatalf("esperaba Occurrences=2, obtuve %d", a.Occurrences)
	}
	if a.SuccessRate != 0.5 {
		t.Fatalf("esperaba SuccessRate=0.5, obtuve %.2f", a.SuccessRate)
	}
}

// TestParseAnalogyQuery_DistintoDeHypothesisGenerator confirma que los
// triggers de Analogical Reasoning no se pisan con Hypothesis
// Generator (Bloque 25), la comparación más parecida conceptualmente.
func TestParseAnalogyQuery_DistintoDeHypothesisGenerator(t *testing.T) {
	if !ParseAnalogyQuery(interaction.Analyze("hiciste algo parecido a armar una tienda online", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Analogical Reasoning")
	}
	if ParseAnalogyQuery(interaction.Analyze("que hipotesis tienes para armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Hypothesis Generator no debería disparar Analogical Reasoning")
	}
}
