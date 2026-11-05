package language

import (
	"strings"
	"testing"
)

// TestRecordTopic_NoDuplicaYCuentaTurnos confirma el bookkeeping básico:
// un mismo tema repetido no se agrega dos veces a topicsDiscussed, pero
// cada llamada sí cuenta como un turno.
func TestRecordTopic_NoDuplicaYCuentaTurnos(t *testing.T) {
	cf := NewConversationFlow()
	cf.RecordTopic("Unity")
	cf.RecordTopic("Unity")
	cf.RecordTopic("inventario")

	state := cf.GetCurrentState()
	if state["turn_number"] != 3 {
		t.Fatalf("turn_number = %v, esperaba 3 (uno por cada RecordTopic)", state["turn_number"])
	}
	topics := state["topics_discussed"].([]string)
	if len(topics) != 2 {
		t.Fatalf("topics_discussed = %v, esperaba 2 (Unity no debería duplicarse)", topics)
	}
}

// TestCreateSummary_VacioYConContenido cubre los dos casos: sesión
// recién arrancada vs. con temas registrados.
func TestCreateSummary_VacioYConContenido(t *testing.T) {
	cf := NewConversationFlow()
	if s := cf.CreateSummary(); !strings.Contains(s, "No hay conversación") {
		t.Fatalf("CreateSummary() vacío = %q, esperaba mensaje explícito", s)
	}

	cf.RecordTopic("Unity")
	cf.RecordTopic("inventario")
	s := cf.CreateSummary()
	if !strings.Contains(s, "Unity") || !strings.Contains(s, "inventario") {
		t.Fatalf("CreateSummary() con temas = %q, esperaba que listara ambos", s)
	}
}

// TestParseConversationSummaryQuery_DetectaVariantesSinPisarWorkingMemory
// confirma el trigger, y que frases del Working Memory (cognition,
// Bloque 13) no lo disparan por accidente — son preguntas distintas
// (TTL de 5 min vs. historial completo de la sesión).
func TestParseConversationSummaryQuery_DetectaVariantesSinPisarWorkingMemory(t *testing.T) {
	positive := []string{
		"resumen de la conversacion",
		"que hemos hablado",
		"resumime la charla",
	}
	for _, q := range positive {
		if !ParseConversationSummaryQuery(q) {
			t.Errorf("ParseConversationSummaryQuery(%q) = false, esperaba true", q)
		}
	}

	negative := []string{
		"de que hablabamos",         // trigger de Working Memory (Bloque 13), TTL corto
		"de que estabamos hablando", // ídem
		"abre chrome",
	}
	for _, q := range negative {
		if ParseConversationSummaryQuery(q) {
			t.Errorf("ParseConversationSummaryQuery(%q) = true, esperaba false (choca con Working Memory)", q)
		}
	}
}

// TestNewContextAdapter_Defaults confirma los valores iniciales
// documentados (nivel técnico 3, largo "detailed").
func TestNewContextAdapter_Defaults(t *testing.T) {
	ca := NewContextAdapter()
	if ca.UserTechLevel != 3 {
		t.Errorf("UserTechLevel = %d, esperaba 3", ca.UserTechLevel)
	}
	if ca.PreferredLength != "detailed" {
		t.Errorf("PreferredLength = %q, esperaba 'detailed'", ca.PreferredLength)
	}
}

// TestAdjustLength_ModoBriefTruncaDeVerdad confirma el único
// comportamiento realmente determinístico y útil de AdjustLength: en
// modo "brief", una respuesta larga se corta a 200 caracteres + "...".
func TestAdjustLength_ModoBriefTruncaDeVerdad(t *testing.T) {
	ca := NewContextAdapter()
	ca.PreferredLength = "brief"

	long := strings.Repeat("a", 250)
	result := ca.AdjustLength(long)

	if len(result) != 203 { // 200 + "..."
		t.Fatalf("len(result) = %d, esperaba 203 (200 caracteres + '...')", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Fatalf("result no termina en '...': %q", result[len(result)-10:])
	}
}

// TestAdjustLength_ModoBriefNoTocaRespuestaCorta confirma que no trunca
// algo que ya entra dentro del límite — AdjustLength no debería agregar
// "..." a una respuesta que nunca se cortó.
func TestAdjustLength_ModoBriefNoTocaRespuestaCorta(t *testing.T) {
	ca := NewContextAdapter()
	ca.PreferredLength = "brief"

	short := "Listo, ya abrí Chrome."
	if result := ca.AdjustLength(short); result != short {
		t.Fatalf("AdjustLength(%q) = %q, una respuesta corta no debería modificarse", short, result)
	}
}

// TestParseContextAdapterQuery_DetectaVariantes confirma el trigger.
func TestParseContextAdapterQuery_DetectaVariantes(t *testing.T) {
	positive := []string{
		"como me estas percibiendo",
		"como me ves",
	}
	for _, q := range positive {
		if !ParseContextAdapterQuery(q) {
			t.Errorf("ParseContextAdapterQuery(%q) = false, esperaba true", q)
		}
	}

	negative := []string{
		"en que modo estas conversando", // trigger de Bloque 16
		"abre chrome",
	}
	for _, q := range negative {
		if ParseContextAdapterQuery(q) {
			t.Errorf("ParseContextAdapterQuery(%q) = true, esperaba false", q)
		}
	}
}
