package interaction

import "testing"

func TestClassifyGeneralQuestionKnowledge(t *testing.T) {
	r := Analyze("¿por qué el cielo es azul?", Context{})
	d := ClassifyGeneralQuestion(r, Context{})
	if d.Route != QuestionRouteKnowledge {
		t.Fatalf("expected knowledge route, got %+v", d)
	}
}

func TestClassifyGeneralQuestionContext(t *testing.T) {
	ctx := Context{ActiveObjective: "mejorar la escena", ActiveWorkID: "work-1"}
	r := SemanticRequest{Raw: "¿qué sigue con este proyecto?", Normalized: "que sigue con este proyecto?", Kind: KindQuestion, SpeechAct: SpeechQuestion}
	d := ClassifyGeneralQuestion(r, ctx)
	if d.Route != QuestionRouteContext {
		t.Fatalf("expected context route, got %+v", d)
	}
}

func TestClassifyGeneralQuestionReasoning(t *testing.T) {
	r := Analyze("¿cómo puedo diseñar una arquitectura que mantenga continuidad y seguridad entre varias conversaciones?", Context{})
	d := ClassifyGeneralQuestion(r, Context{})
	if d.Route != QuestionRouteReasoning {
		t.Fatalf("expected reasoning route, got %+v", d)
	}
}

func TestClassifyGeneralQuestionDoesNotClaimKnowledge(t *testing.T) {
	r := Analyze("¿cuál es tu bebida favorita?", Context{})
	d := ClassifyGeneralQuestion(r, Context{})
	if d.Route != QuestionRouteKnowledge {
		t.Fatalf("expected knowledge route rather than fabricated direct answer, got %+v", d)
	}
}
