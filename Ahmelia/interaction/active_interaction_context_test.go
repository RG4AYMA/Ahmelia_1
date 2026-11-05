package interaction

import (
	"testing"
	"time"
)

func TestActiveInteractionContextProjectsCurrentSemanticFocus(t *testing.T) {
	conversation := NewConversationState()
	thread := conversation.EnsureThread("escena", "mejorar la escena")
	conversation.RecordTurn(SemanticRequest{
		Raw:        "continúa con la escena",
		Kind:       KindContinuation,
		SpeechAct:  SpeechContinue,
		Topic:      "escena",
		Objective:  Objective{Text: "mejorar la escena"},
		Confidence: 0.91,
		References: []Reference{{Kind: ReferenceCurrentTopic, Expression: "la escena", Resolved: true, Value: "escena"}},
	}, "")

	state := NewInteractionState(conversation)
	state.Observe(SemanticRequest{
		Raw:        "continúa con la escena",
		Kind:       KindContinuation,
		SpeechAct:  SpeechContinue,
		Topic:      "escena",
		Objective:  Objective{Text: "mejorar la escena"},
		Confidence: 0.91,
		References: []Reference{{Kind: ReferenceCurrentTopic, Expression: "la escena", Resolved: true, Value: "escena"}},
	}, WorkContext{ID: "work-7", Status: "running", Objective: "mejorar la escena", Provider: "blender", CurrentStep: "lighting", LastResult: "ok", UpdatedAt: time.Now()}, true)

	ctx := state.ActiveContext()
	if ctx.SessionID == "" || ctx.TurnID != 1 {
		t.Fatalf("expected current interaction identity, got %+v", ctx)
	}
	if ctx.Topic != "escena" || ctx.Objective != "mejorar la escena" || ctx.ThreadID != thread.ID {
		t.Fatalf("expected active semantic focus, got %+v", ctx)
	}
	if ctx.WorkID != "work-7" || ctx.WorkStatus != "running" || ctx.CurrentStep != "lighting" {
		t.Fatalf("expected current work view, got %+v", ctx)
	}
	if ctx.RequestKind != KindContinuation || ctx.SpeechAct != SpeechContinue {
		t.Fatalf("expected current semantic request, got %+v", ctx)
	}
	if ctx.UnresolvedRefs != 0 || !ctx.HasActiveThread {
		t.Fatalf("expected resolved active context, got %+v", ctx)
	}
}

func TestActiveInteractionContextCountsOnlyUnresolvedReferences(t *testing.T) {
	state := NewInteractionState(NewConversationState())
	state.Observe(SemanticRequest{
		Raw:        "haz eso",
		Kind:       KindContinuation,
		Confidence: 0.62,
		References: []Reference{
			{Kind: ReferenceCurrentInput, Expression: "eso", Resolved: false},
			{Kind: ReferenceCurrentTopic, Expression: "el tema", Resolved: true, Value: "escena"},
		},
	}, WorkContext{}, false)

	ctx := state.ActiveContext()
	if ctx.UnresolvedRefs != 1 {
		t.Fatalf("expected one unresolved reference, got %d", ctx.UnresolvedRefs)
	}
}

func TestActiveInteractionContextDoesNotBecomeWorkAuthority(t *testing.T) {
	conversation := NewConversationState()
	state := NewInteractionState(conversation)
	state.Observe(SemanticRequest{Kind: KindProduction, Objective: Objective{Text: "crear escena"}}, WorkContext{ID: "work-9", Status: "running", Objective: "crear escena"}, true)

	ctx := state.ActiveContext()
	if ctx.WorkID != "work-9" {
		t.Fatal("expected work to be visible in active context")
	}
	if conversation.ActiveWork() != "" {
		t.Fatal("active interaction context must not mutate conversation/work ownership")
	}
}
