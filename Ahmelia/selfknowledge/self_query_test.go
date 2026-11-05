package selfknowledge

import "testing"

func TestSelfQueryNeverUsesUserFavoriteColor(t *testing.T) {
	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")
	answer, ok := sq.Query("¿Tú tienes color favorito?")
	if !ok {
		t.Fatal("self preference question must be understood")
	}
	if answer != "No tengo un color favorito propio registrado todavía." {
		t.Fatalf("unexpected self answer: %q", answer)
	}
}

func TestSelfQueryAnswersCurrentActivityDirectly(t *testing.T) {
	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")
	answer, ok := sq.Query("¿Qué haces?")
	if !ok {
		t.Fatal("current activity question must be understood")
	}
	if answer == "" {
		t.Fatal("current activity answer must not be empty")
	}
}
