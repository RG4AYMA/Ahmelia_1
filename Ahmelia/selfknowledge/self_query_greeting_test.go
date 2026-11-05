package selfknowledge

import (
	"strings"
	"testing"
)

func TestSelfQueryAnswersGreetingStateInFirstPerson(t *testing.T) {
	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")
	answer, ok := sq.Query("hola Ahmelia, ¿cómo estás?")
	if !ok {
		t.Fatal("¿cómo estás? debe resolverse como presencia en primera persona")
	}
	if !strings.Contains(answer, "Estoy bien") {
		t.Fatalf("respuesta de presencia inesperada: %q", answer)
	}
}

func TestSelfQueryGreetingOnlyStillAnswers(t *testing.T) {
	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")
	answer, ok := sq.Query("hola Ahmelia")
	if !ok {
		t.Fatal("un saludo solo debe resolverse como presencia")
	}
	if !strings.Contains(answer, "Hola") {
		t.Fatalf("respuesta de saludo inesperada: %q", answer)
	}
}

func TestSelfQueryFeminineAttributeConcordance(t *testing.T) {
	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")
	answer, ok := sq.Query("¿Cuál es tu bebida favorita?")
	if !ok {
		t.Fatal("pregunta por atributo propio debe resolverse")
	}
	if answer != "No tengo una bebida favorita propia registrada todavía." {
		t.Fatalf("concordancia inesperada: %q", answer)
	}
}
