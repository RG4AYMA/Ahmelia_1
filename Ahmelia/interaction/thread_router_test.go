package interaction

import "testing"

func TestThreadRouterPreservesInterruptedTopic(t *testing.T) {
	s := NewConversationState()
	first := SemanticRequest{Raw: "quiero avanzar la escena", Normalized: "quiero avanzar la escena",
		Kind: KindProduction, Objective: Objective{Text: "quiero avanzar la escena"}, Topic: "escena"}
	d := (&ThreadRouter{}).Route(s, first)
	if d.Thread.ID == "" {
		t.Fatal("expected first thread")
	}
	id := d.Thread.ID

	interrupt := SemanticRequest{Raw: "qué es la fotosíntesis?", Normalized: "qué es la fotosíntesis?",
		Kind: KindQuestion, Topic: "fotosíntesis"}
	d2 := (&ThreadRouter{}).Route(s, interrupt)
	if d2.Thread.ID != id {
		t.Fatalf("unrelated question should not destroy active work thread: got %q want %q", d2.Thread.ID, id)
	}
}

func TestThreadRouterExplicitSwitchCreatesNewLaneAndKeepsOld(t *testing.T) {
	s := NewConversationState()
	r := &ThreadRouter{}
	a := r.Route(s, SemanticRequest{Raw: "hablemos sobre la escena", Normalized: "hablemos sobre la escena",
		Kind: KindConversation, Topic: "la escena"})
	b := r.Route(s, SemanticRequest{Raw: "cambiemos de tema, hablemos de música", Normalized: "cambiemos de tema, hablemos de música",
		Kind: KindConversation, Topic: "música"})
	if b.Thread.ID == "" || b.Thread.ID == a.Thread.ID {
		t.Fatal("expected a distinct thread after explicit switch")
	}
	if len(s.OpenThreadsSnapshot()) != 2 {
		t.Fatalf("expected both threads to remain alive")
	}
}

func TestThreadRouterResumesExistingLane(t *testing.T) {
	s := NewConversationState()
	r := &ThreadRouter{}
	a := r.Route(s, SemanticRequest{Raw: "hablar sobre la escena", Normalized: "hablar sobre la escena",
		Kind: KindConversation, Topic: "escena"})
	_ = r.Route(s, SemanticRequest{Raw: "cambiemos de tema, hablemos de música", Normalized: "cambiemos de tema, hablemos de música",
		Kind: KindConversation, Topic: "música"})
	c := r.Route(s, SemanticRequest{Raw: "volvamos a la escena", Normalized: "volvamos a la escena",
		Kind: KindConversation, Topic: "escena"})
	if c.Thread.ID != a.Thread.ID || c.Relation != ThreadResume {
		t.Fatalf("expected resume of scene thread, got %+v", c)
	}
}
