package contextintelligence

import (
	"ahmelia/interaction"
	"testing"
)

func TestB9636ResolvesCurrentReferenceAndContinuity(t *testing.T) {
	s := interaction.NewConversationState()
	req := interaction.SemanticRequest{Raw: "¿por qué cambiamos eso?", Normalized: "por que cambiamos eso", FollowUp: true, References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "current referent"}}}
	s.RecordTurn(interaction.SemanticRequest{Raw: "Decidimos usar Aurora", Normalized: "decidimos usar aurora", Topic: "Aurora", Objective: interaction.Objective{Text: "usar Aurora"}, Kind: interaction.KindConversation}, "Sí")
	r := New().Resolve(req, s.Snapshot())
	if !r.Valid {
		t.Fatalf("expected valid: %#v", r.Errors)
	}
	if r.Continuity.Kind != ContinuityContinue {
		t.Fatalf("continuity=%v", r.Continuity.Kind)
	}
	if len(r.References) == 0 || !r.References[0].Resolved {
		t.Fatalf("reference not resolved: %#v", r.References)
	}
}
func TestB9636HistoricalBridgeDoesNotPersist(t *testing.T) {
	s := interaction.NewConversationState()
	s.RecordTurn(interaction.SemanticRequest{Raw: "Decidimos Aurora", Normalized: "decidimos aurora", Topic: "Aurora", Kind: interaction.KindConversation}, "Ok")
	before := len(s.Snapshot().Turns)
	r := New().Resolve(interaction.SemanticRequest{Raw: "¿por qué cambiamos eso?", Normalized: "por que cambiamos eso"}, s.Snapshot())
	after := len(s.Snapshot().Turns)
	if !r.Historical.Required || r.Historical.Query == "" {
		t.Fatal("historical bridge did not activate")
	}
	if before != after {
		t.Fatal("B96.36 mutated ConversationState")
	}
}
