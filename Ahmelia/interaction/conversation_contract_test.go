package interaction

import "testing"

func TestDecideResponseKeepsOpenConversationOutOfDomainRouting(t *testing.T) {
	r := Analyze("¿Qué podemos hacer ahora?", Context{Language: "es", ActiveObjective: "terminar la escena"})
	d := DecideResponse(r, Context{Language: "es", ActiveObjective: "terminar la escena"})
	if !d.Handle || d.Kind != ResponseOptions {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reply == "" {
		t.Fatal("expected a natural response")
	}
}

func TestDecideResponseDoesNotCreateWorkFromProposal(t *testing.T) {
	ctx := Context{Language: "es", ActiveObjective: "proyecto actual", ActiveWorkID: "work_1"}
	r := Analyze("Creo que podríamos mejorar ese proyecto", ctx)
	d := DecideResponse(r, ctx)
	if !d.Handle || d.Kind != ResponseAcknowledge {
		t.Fatalf("decision=%+v", d)
	}
}

func TestDecideResponseCorrectionIsConversationLevel(t *testing.T) {
	ctx := Context{Language: "es", ActiveObjective: "objetivo actual"}
	r := Analyze("No, eso no era lo que quería.", ctx)
	d := DecideResponse(r, ctx)
	if !d.Handle || d.Kind != ResponseCorrectionAck {
		t.Fatalf("decision=%+v", d)
	}
}

func TestPauseIsExplicitAndDomainNeutral(t *testing.T) {
	for _, input := range []string{
		"Espera, no hagas nada todavía.",
		"No hagas nada.",
		"Wait.",
	} {
		r := Parse(input, ChannelText)
		if r.Kind != KindPause {
			t.Fatalf("%q => %q", input, r.Kind)
		}
	}
}

func TestEnglishConversationDoesNotNeedEnglishProvider(t *testing.T) {
	ctx := Context{Language: "es"}
	r := Analyze("I want you to help me with something, but I haven't told you what yet.", ctx)
	d := DecideResponse(r, ctx)
	if !d.Handle || d.Kind != ResponseInvite {
		t.Fatalf("decision=%+v", d)
	}
	if r.Language != "en" {
		t.Fatalf("language=%q", r.Language)
	}
}

func TestProductionWithQuotedGreetingPassesResponseGate(t *testing.T) {
	input := `Crea un pequeño programa Go llamado prueba_contexto que muestre "Hola contexto" y añade un test.`
	ctx := Context{Language: "es"}
	r := Analyze(input, ctx)
	if r.Kind != KindProduction {
		t.Fatalf("kind=%q, want production", r.Kind)
	}
	d := DecideResponse(r, ctx)
	if d.Handle {
		t.Fatalf("production request was intercepted by response gate: %+v", d)
	}
}

func TestAnalyzeTurnRelationRecognizesShortAnswer(t *testing.T) {
	previous := TurnRecord{
		Kind:      KindQuestion,
		SpeechAct: SpeechQuestion,
		Assistant: "¿Qué color te gusta de verdad?",
		Topic:     "gustos",
	}
	current := Analyze("azul", Context{Language: "es"})
	if got := AnalyzeTurnRelation(current, previous, true); got != RelationAnswer {
		t.Fatalf("esperaba RelationAnswer, recibí %q", got)
	}
}

func TestAnalyzeTurnRelationKeepsLongIndependentTurnIndependent(t *testing.T) {
	previous := TurnRecord{
		Kind:      KindQuestion,
		SpeechAct: SpeechQuestion,
		Assistant: "¿Qué color te gusta?",
	}
	current := Analyze("abre Chrome y luego revisa el proyecto de ayer", Context{Language: "es"})
	if got := AnalyzeTurnRelation(current, previous, true); got == RelationAnswer {
		t.Fatalf("una orden independiente no debe interpretarse como respuesta a la pregunta: %q", got)
	}
}

func TestInferPersonalAnswer(t *testing.T) {
	got, ok := InferPersonalAnswer("¿Cuál es mi bebida favorita?", "café")
	if !ok || got.Key != "bebida favorita" || got.Value != "café" {
		t.Fatalf("respuesta personal incorrecta: %+v ok=%v", got, ok)
	}
}

func TestInferPersonalAnswerFromTurnDoesNotLearnFromSelfQuestion(t *testing.T) {
	prev := Analyze("¿Tú tienes color favorito?", Context{})
	turn := TurnRecord{UserText: prev.Raw, Subject: prev.Subject, CommunicativeActs: prev.CommunicativeActs}
	if _, ok := InferPersonalAnswerFromTurn(turn, "negro"); ok {
		t.Fatal("self-owned question must never create a user preference from a short answer")
	}
}
