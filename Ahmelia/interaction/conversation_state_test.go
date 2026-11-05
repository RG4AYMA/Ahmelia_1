package interaction

import "testing"

func TestConversationStatePreservesContinuityAcrossTurns(t *testing.T) {
	s := NewConversationState()
	first := Analyze("Quiero avanzar esta escena.", Context{})
	s.RecordTurn(first, "¿Qué parte quieres priorizar?")
	s.OpenThread("escena", first.Objective.Text)

	second := Analyze("La interacción.", Context{
		PreviousInput:   first.Raw,
		PreviousKind:    first.Kind,
		ActiveObjective: first.Objective.Text,
		CurrentTopic:    "escena",
	})
	s.RecordTurn(second, "Perfecto, seguimos con la interacción.")

	snap := s.Snapshot()
	if snap.ActiveTopic == "" || snap.ActiveObjective == "" {
		t.Fatal("conversation state lost active topic/objective")
	}
	if len(snap.Turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(snap.Turns))
	}
	if snap.Turns[1].UserText != "La interacción." {
		t.Fatalf("second turn was not preserved")
	}
}

func TestConversationStateCanPauseAndResumeThread(t *testing.T) {
	s := NewConversationState()
	s.OpenThread("escena", "avanzar escena")
	s.PauseActiveThread()
	if ok := s.ResumeThread("escena"); !ok {
		t.Fatal("expected scene thread to resume")
	}
	if s.Snapshot().ActiveTopic != "escena" {
		t.Fatal("resume did not restore active topic")
	}
}

func TestConversationStateQuestionDoesNotOverwriteObjective(t *testing.T) {
	s := NewConversationState()
	s.RecordTurn(SemanticRequest{
		Raw:       "crear un proyecto de prueba",
		Kind:      KindProduction,
		Objective: Objective{Text: "crear un proyecto de prueba"},
		Topic:     "proyecto",
	}, "De acuerdo, lo preparo.")

	s.RecordTurn(SemanticRequest{
		Raw:       "¿qué haces?",
		Kind:      KindQuestion,
		Objective: Objective{Text: "¿qué haces?"},
	}, "Estoy aquí contigo.")

	snap := s.Snapshot()
	if snap.ActiveObjective != "crear un proyecto de prueba" {
		t.Fatalf("la pregunta conversacional no debe reemplazar el objetivo activo: %q", snap.ActiveObjective)
	}
}

func TestConversationIDsRemainUniqueAcrossRapidCreation(t *testing.T) {
	seen := make(map[string]struct{}, 256)
	for i := 0; i < 256; i++ {
		id := newConversationID()
		if _, exists := seen[id]; exists {
			t.Fatalf("conversation ID collision at iteration %d: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestRecordTurnPreservesSubjectOwnership(t *testing.T) {
	s := NewConversationState()
	r := Analyze("¿Tú tienes color favorito?", Context{Language: "es"})
	id := s.RecordTurn(r, "")
	if id == 0 {
		t.Fatal("expected turn id")
	}
	turn, ok := s.LastTurn()
	if !ok {
		t.Fatal("expected recorded turn")
	}
	if turn.Subject != UserObservationSubjectSelf {
		t.Fatalf("recorded subject=%q want self", turn.Subject)
	}
}
