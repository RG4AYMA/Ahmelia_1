package interaction

import "testing"

func TestDetermineResponseIntentAnswersQuestion(t *testing.T) {
	ctx := ConversationSnapshot{
		Language: "es",
		Turns:    []TurnRecord{{ID: 7, Kind: KindQuestion, SpeechAct: SpeechQuestion, Assistant: "¿Qué prefieres?"}},
	}
	r := Analyze("la interacción", Context{Language: "es"})
	got := DetermineResponseIntent(r, ctx)
	if got.Kind != ResponseIntentAnswer {
		t.Fatalf("kind=%q", got.Kind)
	}
	if !got.AddressesQuestion || got.ParentTurnID != 7 || !got.ShouldContinue {
		t.Fatalf("intent=%+v", got)
	}
}

func TestDetermineResponseIntentDoesNotSwallowProduction(t *testing.T) {
	r := Analyze("crea un programa Go llamado prueba", Context{Language: "es"})
	got := DetermineResponseIntent(r, ConversationSnapshot{Language: "es"})
	if got.Kind != ResponseIntentNone || !got.ShouldAct {
		t.Fatalf("production intent=%+v", got)
	}
}

func TestDetermineResponseIntentProposalNeedsUserInput(t *testing.T) {
	r := Analyze("podríamos mejorar la escena", Context{Language: "es", ActiveObjective: "avanzar escena"})
	got := DetermineResponseIntent(r, ConversationSnapshot{Language: "es", ActiveObjective: "avanzar escena"})
	if got.Kind != ResponseIntentPropose || !got.NeedsUserInput || !got.ShouldContinue {
		t.Fatalf("intent=%+v", got)
	}
}

func TestDetermineResponseIntentCorrectionPrecedesSurfaceAction(t *testing.T) {
	r := Analyze("no, eso no era lo que quería", Context{Language: "es", ActiveObjective: "avanzar escena"})
	got := DetermineResponseIntent(r, ConversationSnapshot{Language: "es", ActiveObjective: "avanzar escena"})
	if got.Kind != ResponseIntentAcknowledge || !got.ShouldContinue {
		t.Fatalf("intent=%+v", got)
	}
}

func TestDetermineResponseIntentConversationIsCapabilityAgnostic(t *testing.T) {
	r := Analyze("oye, ¿cómo estás?", Context{Language: "es"})
	got := DetermineResponseIntent(r, ConversationSnapshot{Language: "es"})
	if got.Kind != ResponseIntentAnswer && got.Kind != ResponseIntentInvite {
		t.Fatalf("intent=%+v", got)
	}
}

func TestDetermineResponseIntentNewQuestionDoesNotLinkToPreviousQuestion(t *testing.T) {
	previous := SemanticRequest{Raw: "¿Cuál es mi color favorito?", Normalized: "¿cuál es mi color favorito?", Subject: UserObservationSubjectUser}
	prevActs := ClassifyCommunicativeActs(previous)
	ctx := ConversationSnapshot{Turns: []TurnRecord{{ID: 1, UserText: previous.Raw, CommunicativeActs: prevActs}}}
	current := Analyze("¿Tú tienes color favorito?", Context{Language: "es"})
	intent := DetermineResponseIntent(current, ctx)
	if intent.Kind != ResponseIntentAnswer {
		t.Fatalf("intent=%q want answer", intent.Kind)
	}
	if linkedAnswer(current, ctx) {
		t.Fatal("new explicit question must not be linked as an answer to previous question")
	}
}

func TestResponseIntentConsumesSituationalHintOnlyWhenSemanticSurfaceIsInsufficient(t *testing.T) {
	r := SemanticRequest{Raw: "future state", Normalized: "future state", Confidence: 0.20, CommunicativeActs: CommunicativeActs{Primary: ActUnknown}}
	got := DetermineResponseIntent(r, ConversationSnapshot{}, ResponseIntentHint{Kind: "share_idea", Confidence: 0.88, Reason: "structured situation contribution"})
	if got.Kind != ResponseIntentPropose {
		t.Fatalf("expected situational hint to refine response intent to propose, got %s", got.Kind)
	}

	high := SemanticRequest{Raw: "hello", Normalized: "hello", Confidence: 0.98, CommunicativeActs: CommunicativeActs{Primary: ActSocial, Confidence: 0.98}}
	gotHigh := DetermineResponseIntent(high, ConversationSnapshot{}, ResponseIntentHint{Kind: "share_idea", Confidence: 0.99, Reason: "must not override high confidence"})
	if gotHigh.Kind != ResponseIntentAcknowledge {
		t.Fatalf("high-confidence semantic act was overridden by situational hint: %s", gotHigh.Kind)
	}
}
