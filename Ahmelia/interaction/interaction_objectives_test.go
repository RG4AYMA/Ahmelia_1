package interaction

import (
	"path/filepath"
	"testing"
)

func TestDetectObjectiveDoesNotMutateState(t *testing.T) {
	s := NewConversationState()
	req := SemanticRequest{Kind: KindProduction, Objective: Objective{Text: "mejorar la escena"}, Confidence: .9}
	c := DetectObjective(req, 3)
	if !c.IsObjective || c.Text != "mejorar la escena" {
		t.Fatalf("unexpected candidate: %#v", c)
	}
	if len(s.Snapshot().Objectives) != 0 {
		t.Fatal("detection must not mutate objective history")
	}
}

func TestApplyObjectiveActivatesAndArchivesPrevious(t *testing.T) {
	s := NewConversationState()
	ApplyObjective(s, ObjectiveCandidate{IsObjective: true, Topic: "escena", Text: "mejorar iluminación", SourceTurn: 1}, "decisión 1")
	ApplyObjective(s, ObjectiveCandidate{IsObjective: true, Topic: "escena", Text: "mejorar interacción", SourceTurn: 2}, "decisión 2")
	active, ok := s.ActiveObjectiveRecord()
	if !ok || active.Text != "mejorar interacción" || active.Status != "active" {
		t.Fatalf("unexpected active objective: %#v", active)
	}
	snap := s.Snapshot()
	if len(snap.Objectives) != 2 || snap.Objectives[0].Status != "open" {
		t.Fatalf("expected previous objective to remain historical: %#v", snap.Objectives)
	}
}

func TestObjectiveDoesNotCreateWork(t *testing.T) {
	s := NewConversationState()
	req := SemanticRequest{Kind: KindProposal, Objective: Objective{Text: "diseñar el comportamiento"}, Topic: "IA"}
	c := DetectObjective(req, 4)
	ApplyObjective(s, c, "accepted")
	if s.ActiveWork() != "" {
		t.Fatal("interaction objective must not create or attach operational work")
	}
}

func TestObjectivePersistenceRoundTrip(t *testing.T) {
	s := NewConversationState()
	ApplyObjective(s, ObjectiveCandidate{IsObjective: true, Topic: "Unity", Text: "mejorar enemigos", SourceTurn: 8}, "usar IA adaptativa")
	path := filepath.Join(t.TempDir(), "conversation.json")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded := LoadConversationState(path)
	got, ok := loaded.ActiveObjectiveRecord()
	if !ok || got.Text != "mejorar enemigos" || got.Decision != "usar IA adaptativa" {
		t.Fatalf("objective not restored: %#v", got)
	}
}

func TestQuestionDoesNotBecomeObjective(t *testing.T) {
	req := SemanticRequest{Kind: KindQuestion, Objective: Objective{Text: "¿qué hacemos ahora?"}, Confidence: .99}
	if c := DetectObjective(req, 9); c.IsObjective {
		t.Fatal("questions must not be promoted to objectives")
	}
}
