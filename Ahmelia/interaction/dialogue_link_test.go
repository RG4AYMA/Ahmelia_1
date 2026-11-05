package interaction

import "testing"

func TestLinkTurnShortAnswerToQuestion(t *testing.T) {
	prev := TurnRecord{
		ID: 7, UserText: "¿Qué bebida te gusta?", Kind: KindQuestion,
		SpeechAct: SpeechQuestion, Topic: "gustos",
	}
	cur := Analyze("café", Context{})
	link := LinkTurn(cur, prev, true, ConversationSnapshot{})
	if link.Relation != RelationAnswer || link.ParentTurnID != 7 {
		t.Fatalf("expected answer linked to turn 7, got %+v", link)
	}
}

func TestLinkTurnNewCommandWinsOverQuestionAnswer(t *testing.T) {
	prev := TurnRecord{ID: 3, UserText: "¿Qué prefieres?", Kind: KindQuestion, SpeechAct: SpeechQuestion}
	cur := Analyze("abre Chrome", Context{})
	link := LinkTurn(cur, prev, true, ConversationSnapshot{})
	if link.Relation != RelationNewTopic {
		t.Fatalf("expected new topic, got %+v", link)
	}
}

func TestWorkAnswerPreservesObjectiveAndDecision(t *testing.T) {
	prev := TurnRecord{
		ID: 12, UserText: "¿Qué priorizamos para avanzar la escena?",
		Kind: KindQuestion, SpeechAct: SpeechQuestion,
		Objective: "avanzar la escena", Topic: "escena",
	}
	cur := Analyze("la interacción", Context{})
	out := ResolveAnswerSemantics(cur, prev)
	if out.Kind != KindContinuation {
		t.Fatalf("expected continuation, got %q", out.Kind)
	}
	if out.Objective.Text != "avanzar la escena" {
		t.Fatalf("objective lost: %q", out.Objective.Text)
	}
	if len(out.Objective.Constraints) != 1 ||
		out.Objective.Constraints[0] != "user_decision: la interacción" {
		t.Fatalf("decision not preserved: %+v", out.Objective.Constraints)
	}
}

func TestConversationStatePersistsDialogueLink(t *testing.T) {
	s := NewConversationState()
	req := Analyze("café", Context{})
	id := s.RecordTurnLinked(req, "", RelationAnswer, 9)
	got, ok := s.LastTurn()
	if !ok || got.ID != id {
		t.Fatalf("missing recorded turn")
	}
	if got.Relation != RelationAnswer || got.ParentTurnID != 9 {
		t.Fatalf("link not persisted in state: %+v", got)
	}
}

func TestLinkTurnNewQuestionStartsNewTurn(t *testing.T) {
	previous := TurnRecord{ID: 7, UserText: "¿Cuál es mi color favorito?", CommunicativeActs: ClassifyCommunicativeActs(SemanticRequest{Raw: "¿Cuál es mi color favorito?", Normalized: "¿cuál es mi color favorito?"})}
	current := Analyze("¿Tú tienes color favorito?", Context{Language: "es"})
	link := LinkTurn(current, previous, true, ConversationSnapshot{})
	if link.Relation != RelationNewTopic {
		t.Fatalf("relation=%q want new_topic", link.Relation)
	}
}
