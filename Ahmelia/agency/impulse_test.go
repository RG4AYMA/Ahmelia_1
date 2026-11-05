package agency

import (
	"testing"
	"time"
)

func TestComputeImpulsesCalmInputIsEmpty(t *testing.T) {
	got := ComputeImpulses(ImpulseInput{Now: time.Now()})
	if len(got) != 0 {
		t.Fatalf("esperaba cero impulsos, obtuve %+v", got)
	}
}

func TestComputeImpulsesUncertainty(t *testing.T) {
	got := ComputeImpulses(ImpulseInput{Uncertainty: true})
	if len(got) != 1 || got[0].Kind != ImpulseKnowledgeGap {
		t.Fatalf("esperaba knowledge_gap, obtuve %+v", got)
	}
	if got[0].Strength != 0.70 {
		t.Fatalf("fuerza=%v, esperaba 0.70", got[0].Strength)
	}
}

func TestComputeImpulsesUncertaintyAndPendingQuestion(t *testing.T) {
	got := ComputeImpulses(ImpulseInput{
		Uncertainty:     true,
		PendingQuestion: "¿qué proyecto?",
	})
	if len(got) != 1 || got[0].Kind != ImpulseKnowledgeGap {
		t.Fatalf("esperaba knowledge_gap, obtuve %+v", got)
	}
	if got[0].Strength != 0.85 {
		t.Fatalf("fuerza=%v, esperaba 0.85", got[0].Strength)
	}
	if len(got[0].Evidence) != 1 || got[0].Evidence[0] != "clarification:¿qué proyecto?" {
		t.Fatalf("evidencia=%v", got[0].Evidence)
	}
}

func TestComputeImpulsesPendingQuestionAloneIsNotAnOpening(t *testing.T) {
	got := ComputeImpulses(ImpulseInput{PendingQuestion: "¿qué proyecto?"})
	if len(got) != 0 {
		t.Fatalf("una pregunta que debe responder el usuario no abre meta propia: %+v", got)
	}
}

func TestComputeImpulsesUnfinishedWork(t *testing.T) {
	got := ComputeImpulses(ImpulseInput{
		InteractionActive: true,
		InteractionStatus: "in_progress",
	})
	if len(got) != 1 || got[0].Kind != ImpulseUnfinishedWork {
		t.Fatalf("esperaba unfinished_work, obtuve %+v", got)
	}

	got = ComputeImpulses(ImpulseInput{
		InteractionActive: true,
		InteractionStatus: "completed",
	})
	if len(got) != 0 {
		t.Fatalf("trabajo cerrado no debe generar impulso, obtuve %+v", got)
	}
}

func TestComputeImpulsesFailedAction(t *testing.T) {
	got := ComputeImpulses(ImpulseInput{
		ActionRan:          true,
		LastActionSuccess:  false,
		LastActionVerified: false,
	})
	if len(got) != 1 || got[0].Kind != ImpulseFailedAction {
		t.Fatalf("esperaba failed_action, obtuve %+v", got)
	}
	if got[0].Strength != 0.80 {
		t.Fatalf("fuerza=%v, esperaba 0.80", got[0].Strength)
	}

	got = ComputeImpulses(ImpulseInput{
		ActionRan:          true,
		LastActionSuccess:  true,
		LastActionVerified: false,
	})
	if len(got) != 1 || got[0].Strength != 0.65 {
		t.Fatalf("esperaba fallo suave 0.65, obtuve %+v", got)
	}
}

func TestComputeImpulsesBlockedGoal(t *testing.T) {
	got := ComputeImpulses(ImpulseInput{BlockedGoals: []string{"organizar biblioteca"}})
	if len(got) != 1 || got[0].Kind != ImpulseBlockedGoal {
		t.Fatalf("esperaba blocked_goal, obtuve %+v", got)
	}
	if got[0].Evidence[0] != "organizar biblioteca" {
		t.Fatalf("evidencia=%v", got[0].Evidence)
	}
}

func TestComputeImpulsesRepeatedTopicAndOrdering(t *testing.T) {
	got := ComputeImpulses(ImpulseInput{
		Uncertainty:       true,
		InteractionActive: true,
		InteractionStatus: "working",
		Topic:             "blender",
		TopicTurns:        5,
	})
	if len(got) != 3 {
		t.Fatalf("esperaba 3 impulsos (tope), obtuve %d: %+v", len(got), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Strength < got[i].Strength {
			t.Fatalf("impulsos desordenados: %+v", got)
		}
	}
	// El más fuerte es knowledge_gap (0.70) o failed_action (0.80) según los
	// campos; aquí failed_action no corrió, así que knowledge_gap gana.
	if got[0].Kind != ImpulseKnowledgeGap {
		t.Fatalf("primero=%s, esperaba knowledge_gap (%+v)", got[0].Kind, got)
	}
}

func TestStrongestThreshold(t *testing.T) {
	impulses := []Impulse{{Kind: ImpulseRepeatedTopic, Strength: 0.4}}
	if _, ok := Strongest(impulses, 0.5); ok {
		t.Fatal("no debería superar el umbral")
	}
	impulses = append(impulses, Impulse{Kind: ImpulseFailedAction, Strength: 0.8})
	if imp, ok := Strongest(impulses, 0.5); !ok || imp.Kind != ImpulseFailedAction {
		t.Fatalf("esperaba failed_action, obtuve %+v ok=%v", imp, ok)
	}
}
