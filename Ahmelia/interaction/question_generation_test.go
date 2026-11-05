package interaction

import "testing"

func TestQuestionGeneratorLearnsFromUserModelGaps(t *testing.T) {
	p := NewUserProfile()
	got, ok := GenerateQuestion(QuestionGenerationContext{
		Request:  SemanticRequest{Kind: KindConversation, Raw: "sigamos"},
		Snapshot: ConversationSnapshot{UserState: UserState{Engagement: .8}},
		Profile:  p,
	})
	if !ok {
		t.Fatal("expected a learning question")
	}
	if got.Purpose != QuestionPurposeLearn || got.Category == "" || got.Key == "" || got.Question == "" {
		t.Fatalf("unexpected learning candidate: %+v", got)
	}
}

func TestQuestionGeneratorUsesActiveObjectiveBeforePersonalLearning(t *testing.T) {
	p := NewUserProfile()
	got, ok := GenerateQuestion(QuestionGenerationContext{
		Request: SemanticRequest{Kind: KindConversation, Raw: "sigamos"},
		Snapshot: ConversationSnapshot{
			ActiveObjective: "avanzar esta escena",
			ActiveTopic:     "la escena",
		},
		Profile: p,
	})
	if !ok {
		t.Fatal("expected a contextual question")
	}
	if got.Purpose != QuestionPurposeContinue {
		t.Fatalf("expected continuation question, got %+v", got)
	}
	if got.Topic != "la escena" {
		t.Fatalf("expected topic to survive generation, got %q", got.Topic)
	}
}

func TestQuestionGeneratorDoesNotCompeteWithImmediateUserNeed(t *testing.T) {
	p := NewUserProfile()
	for _, kind := range []RequestKind{
		KindQuestion, KindAction, KindProduction, KindReasoning,
		KindCorrection, KindClarification, KindPause, KindStop,
	} {
		got := DefaultQuestionGenerator().Generate(QuestionGenerationContext{
			Request: SemanticRequest{Kind: kind, Raw: "hazlo"},
			Profile: p,
		})
		if len(got) != 0 {
			t.Fatalf("kind %q should not generate proactive questions: %+v", kind, got)
		}
	}
}

func TestQuestionGeneratorCanExploreLatestConversation(t *testing.T) {
	p := NewUserProfile()
	// Any existing canonical profile fact is enough to prevent the empty-profile
	// learning prompt; the generator no longer maintains a fixed questionnaire.
	p.Remember("attribute", "proyecto", "known", "test", .9)
	got, ok := GenerateQuestion(QuestionGenerationContext{
		Request:  SemanticRequest{Kind: KindConversation, Raw: "hoy quiero hablar de algo interesante"},
		Snapshot: ConversationSnapshot{ActiveTopic: "nuestro proyecto"},
		Profile:  p,
	})
	if !ok || got.Purpose != QuestionPurposeExplore {
		t.Fatalf("expected exploratory question, got %+v", got)
	}
}

func TestQuestionGeneratorRanksAndBoundsCandidates(t *testing.T) {
	p := NewUserProfile()
	got := DefaultQuestionGenerator().Generate(QuestionGenerationContext{
		Request:  SemanticRequest{Kind: KindConversation, Raw: "sigamos"},
		Snapshot: ConversationSnapshot{ActiveObjective: "avanzar", ActiveTopic: "proyecto"},
		Profile:  p,
	})
	if len(got) == 0 || len(got) > 3 {
		t.Fatalf("unexpected candidate count: %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Priority > got[i-1].Priority {
			t.Fatal("questions must be ranked by descending priority")
		}
	}
}

func TestGenerateQuestionDoesNotCompeteWithDirectSemanticQuestion(t *testing.T) {
	req := SemanticRequest{Raw: "¿qué horario usamos para avanzar en el proyecto?", Kind: KindQuestion, SpeechAct: SpeechQuestion}
	profile := NewUserProfile()
	got := DefaultQuestionGenerator().Generate(QuestionGenerationContext{
		Request:  req,
		Snapshot: ConversationSnapshot{ActiveTopic: "proyecto", ActiveObjective: "avanzar"},
		Profile:  profile,
	})
	if len(got) != 0 {
		t.Fatalf("direct question produced proactive candidates: %+v", got)
	}
}
