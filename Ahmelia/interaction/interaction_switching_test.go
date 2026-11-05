package interaction

import "testing"

func TestInteractionSwitcherExplicitSwitchPreservesPreviousLane(t *testing.T) {
	s := NewConversationState()
	switcher := NewInteractionSwitcher()

	first := switcher.SwitchTopic(s, "Unity enemigo", "mejorar enemigos")
	if first.CurrentID == "" {
		t.Fatal("expected first interaction lane")
	}

	second := switcher.Switch(s, SemanticRequest{
		Kind:       KindConversation,
		Raw:        "cambiemos de tema, hablemos de música",
		Normalized: "cambiemos de tema, hablemos de música",
		Topic:      "música",
	})
	if second.Relation != ThreadSwitch && second.Relation != ThreadCreate {
		t.Fatalf("expected switch/create relation, got %q", second.Relation)
	}
	if second.CurrentID == first.CurrentID {
		t.Fatal("expected a distinct lane after explicit switch")
	}

	threads := s.OpenThreadsSnapshot()
	if len(threads) != 2 {
		t.Fatalf("expected two preserved lanes, got %d", len(threads))
	}
}

func TestInteractionSwitcherResumeRestoresLaneAndObjective(t *testing.T) {
	s := NewConversationState()
	switcher := NewInteractionSwitcher()
	first := switcher.SwitchTopic(s, "escena", "mejorar iluminación")
	if first.CurrentID == "" {
		t.Fatal("expected scene lane")
	}
	second := switcher.SwitchTopic(s, "música", "elegir ambiente")
	if second.CurrentID == first.CurrentID {
		t.Fatal("expected separate music lane")
	}

	resume := switcher.Switch(s, SemanticRequest{
		Kind:       KindConversation,
		Raw:        "volvamos a la escena",
		Normalized: "volvamos a la escena",
		Topic:      "escena",
	})
	if resume.Relation != ThreadResume {
		t.Fatalf("expected resume, got %q", resume.Relation)
	}
	if resume.CurrentID != first.CurrentID {
		t.Fatalf("expected original lane %q, got %q", first.CurrentID, resume.CurrentID)
	}
	if got := s.Snapshot().ActiveObjective; got != "mejorar iluminación" {
		t.Fatalf("expected objective restoration, got %q", got)
	}
}

func TestInteractionSwitcherContextualAnswerDoesNotSwitchLane(t *testing.T) {
	s := NewConversationState()
	switcher := NewInteractionSwitcher()
	first := switcher.SwitchTopic(s, "proyecto", "avanzar proyecto")

	got := switcher.Switch(s, SemanticRequest{
		Kind:       KindConversation,
		Raw:        "la segunda",
		Normalized: "la segunda",
		Continues:  true,
	})
	if got.CurrentID != first.CurrentID || got.Relation != ThreadKeep {
		t.Fatalf("contextual answer should keep lane, got %+v", got)
	}
}

func TestInteractionSwitcherNeverCreatesWork(t *testing.T) {
	s := NewConversationState()
	switcher := NewInteractionSwitcher()
	_ = switcher.SwitchTopic(s, "Unity", "mejorar enemigos")
	if got := s.ActiveWork(); got != "" {
		t.Fatalf("interaction switching must not create operational work, got %q", got)
	}
}
