package interaction

import (
	"testing"
	"time"
)

func TestInteractionReferencesExposeCurrentTurnIdentityAndResolution(t *testing.T) {
	conversation := NewConversationState()
	state := NewInteractionState(conversation)
	state.Observe(SemanticRequest{
		Raw:  "haz eso y abre el archivo",
		Kind: KindAction,
		References: []Reference{
			{Kind: ReferenceCurrentInput, Expression: "eso", Resolved: false},
			{Kind: ReferenceRecentArtifact, Expression: "el archivo", Resolved: true, Value: "main.go"},
		},
	}, WorkContext{}, false)

	refs := state.ActiveReferences()
	if len(refs) != 2 {
		t.Fatalf("expected two active references, got %+v", refs)
	}
	if refs[0].ID != "turn-0-ref-1" || refs[0].Resolved {
		t.Fatalf("unexpected unresolved reference projection: %+v", refs[0])
	}
	if refs[1].ID != "turn-0-ref-2" || refs[1].Value != "main.go" || refs[1].Confidence != 1 {
		t.Fatalf("unexpected resolved reference projection: %+v", refs[1])
	}
}

func TestActiveContextIncludesReferenceProjection(t *testing.T) {
	state := NewInteractionState(NewConversationState())
	now := time.Now()
	state.Observe(SemanticRequest{
		Raw:        "continua con eso",
		Kind:       KindContinuation,
		References: []Reference{{Kind: ReferenceCurrentInput, Expression: "eso", Resolved: false}},
	}, WorkContext{}, false)

	ctx := state.ActiveContext()
	if len(ctx.References) != 1 {
		t.Fatalf("expected one active reference, got %+v", ctx.References)
	}
	if ctx.References[0].Kind != ReferenceCurrentInput || ctx.References[0].Expression != "eso" {
		t.Fatalf("unexpected reference in active context: %+v", ctx.References[0])
	}
	if ctx.References[0].UpdatedAt.IsZero() || ctx.UpdatedAt.Before(now) {
		t.Fatalf("expected reference timestamps to be populated: %+v", ctx.References[0])
	}
}

func TestActiveReferencesDoesNotCreateReferenceHistory(t *testing.T) {
	conversation := NewConversationState()
	state := NewInteractionState(conversation)
	state.Observe(SemanticRequest{
		Raw:        "haz eso",
		Kind:       KindContinuation,
		References: []Reference{{Kind: ReferenceCurrentInput, Expression: "eso", Resolved: false}},
	}, WorkContext{}, false)

	refs := state.ActiveReferences()
	refs[0].Resolved = true
	refs[0].Value = "mutated outside"

	again := state.ActiveReferences()
	if again[0].Resolved || again[0].Value != "" {
		t.Fatalf("active reference projection must be detached from internal state: %+v", again[0])
	}
	if len(conversation.Snapshot().Turns) != 0 {
		t.Fatal("reference projection must not create conversation history")
	}
}
