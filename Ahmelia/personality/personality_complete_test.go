package personality

import (
	"os"
	"strings"
	"testing"
)

// TestNewRelationship_TypeEsCompanion confirma el cambio de nombre
// acordado: ya no "autonomous_assistant_and_partner".
func TestNewRelationship_TypeEsCompanion(t *testing.T) {
	r := NewRelationship()
	if r.Type != "Cognitive Companion" {
		t.Fatalf("Type = %q, esperaba \"Cognitive Companion\"", r.Type)
	}
}

// TestAddMoment_GuardaEnSharedHistory confirma que AddMoment agrega
// el momento, con su lección.
func TestAddMoment_GuardaEnSharedHistory(t *testing.T) {
	r := NewRelationship()
	r.AddMoment("Terminamos la Fase 2.6", "hito", "Ahmelia integró su propio proceso cognitivo")

	if len(r.SharedHistory) != 1 {
		t.Fatalf("SharedHistory tiene %d momentos, esperaba 1", len(r.SharedHistory))
	}
	if r.SharedHistory[0].LessonLearned == "" {
		t.Fatal("LessonLearned vacío, esperaba la lección pasada a AddMoment")
	}
}

// TestGrowTrust_TopeEnUno confirma el tope superior — TrustLevel no
// puede superar 1.0 aunque se sumen deltas de sobra.
func TestGrowTrust_TopeEnUno(t *testing.T) {
	r := NewRelationship()
	r.TrustLevel = 0.99
	r.GrowTrust(0.5)
	if r.TrustLevel != 1.0 {
		t.Fatalf("TrustLevel = %.2f, esperaba tope en 1.0", r.TrustLevel)
	}
}

// TestGrowTrust_PisoEnCero confirma el piso inferior — no debería
// pasar en el uso real (siempre se llama con delta positivo), pero
// GrowTrust no asume esa disciplina externa.
func TestGrowTrust_PisoEnCero(t *testing.T) {
	r := NewRelationship()
	r.TrustLevel = 0.01
	r.GrowTrust(-0.5)
	if r.TrustLevel != 0.0 {
		t.Fatalf("TrustLevel = %.2f, esperaba piso en 0.0", r.TrustLevel)
	}
}

// TestNarrative_SinMomentos confirma el mensaje honesto cuando
// SharedHistory está vacío, sin inventar un momento.
func TestNarrative_SinMomentos(t *testing.T) {
	r := NewRelationship()
	n := r.Narrative()
	if !strings.Contains(n, "Todavía no") {
		t.Fatalf("Narrative() sin momentos = %q, esperaba avisar que no hay ninguno", n)
	}
}

// TestNarrative_MuestraSoloLosUltimosCinco confirma que Narrative()
// no lista todo el historial — es memoria, no un log completo.
func TestNarrative_MuestraSoloLosUltimosCinco(t *testing.T) {
	r := NewRelationship()
	for i := 0; i < 8; i++ {
		r.AddMoment("momento", "hito", "")
	}
	n := r.Narrative()
	count := strings.Count(n, "- ")
	if count != 5 {
		t.Fatalf("Narrative() listó %d momentos, esperaba 5 (los últimos, no los 8)", count)
	}
}

// TestSaveLoad_Roundtrip confirma persistencia real a disco — el
// punto entero de este bloque es que la memoria compartida sobreviva
// un reinicio.
func TestSaveLoad_Roundtrip(t *testing.T) {
	path := t.TempDir() + "/relationship_test.json"
	defer os.Remove(path)

	r := NewRelationship()
	r.AddMoment("Primera conversación natural", "hito", "El diálogo dejó de ser solo preguntas y respuestas")
	r.GrowTrust(0.05)
	if err := r.Save(path); err != nil {
		t.Fatalf("Save() falló: %v", err)
	}

	loaded := LoadRelationship(path)
	if len(loaded.SharedHistory) != 1 {
		t.Fatalf("después de Load(), SharedHistory tiene %d momentos, esperaba 1", len(loaded.SharedHistory))
	}
	if loaded.TrustLevel != r.TrustLevel {
		t.Fatalf("TrustLevel tras Load() = %.2f, esperaba %.2f", loaded.TrustLevel, r.TrustLevel)
	}
}

// TestLoadRelationship_ArchivoInexistente confirma que, sin archivo
// todavía (primera corrida de este bloque), arranca una relación
// nueva en vez de fallar.
func TestLoadRelationship_ArchivoInexistente(t *testing.T) {
	r := LoadRelationship("/ruta/que/no/existe/relationship.json")
	if r.Type != "Cognitive Companion" {
		t.Fatal("LoadRelationship sin archivo debería devolver una relación nueva, no fallar")
	}
}

// TestParseRelationshipQuery_Positivos confirma los triggers reales.
func TestParseRelationshipQuery_Positivos(t *testing.T) {
	positives := []string{
		"como es nuestra relacion",
		"que tanto confias en mi",
		"que momentos recordas",
	}
	for _, p := range positives {
		if !ParseRelationshipQuery(p) {
			t.Errorf("ParseRelationshipQuery(%q) = false, esperaba true", p)
		}
	}
}

// TestParseRememberQuery_ConContenido confirma que se extrae el texto
// posterior al trigger.
func TestParseRememberQuery_ConContenido(t *testing.T) {
	content, ok := ParseRememberQuery("recorda esto: hoy terminamos la fase 2.6")
	if !ok {
		t.Fatal("ParseRememberQuery no detectó el trigger")
	}
	if content != "hoy terminamos la fase 2.6" {
		t.Fatalf("content = %q, esperaba \"hoy terminamos la fase 2.6\"", content)
	}
}

// TestParseRememberQuery_SinContenido confirma que, si el pedido vino
// sin nada después, ok=true pero content="" — para que engine.go pida
// aclaración en vez de guardar un momento vacío.
func TestParseRememberQuery_SinContenido(t *testing.T) {
	content, ok := ParseRememberQuery("recorda esto")
	if !ok {
		t.Fatal("ParseRememberQuery no detectó el trigger sin contenido")
	}
	if content != "" {
		t.Fatalf("content = %q, esperaba vacío", content)
	}
}

// TestParseRememberQuery_NoPisaOtrosTriggers confirma que texto sin el
// trigger no dispara nada.
func TestParseRememberQuery_NoPisaOtrosTriggers(t *testing.T) {
	if _, ok := ParseRememberQuery("que estas haciendo"); ok {
		t.Fatal("ParseRememberQuery no debería activarse sin el trigger real")
	}
}
