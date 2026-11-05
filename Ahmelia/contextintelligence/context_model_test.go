package contextintelligence

import (
	"ahmelia/interaction"
	"testing"
)

func TestB9636ApplyKeepsHistoricalAuthoritySeparate(t *testing.T) {
	s := interaction.NewConversationState()
	s.RecordTurn(interaction.SemanticRequest{Raw: "Decidimos Aurora", Normalized: "decidimos aurora", Topic: "Aurora", Objective: interaction.Objective{Text: "usar Aurora"}, Kind: interaction.KindConversation}, "Sí")
	req := interaction.SemanticRequest{Raw: "¿por qué cambiamos eso?", Normalized: "por que cambiamos eso", References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "current referent"}}}
	res := New().Resolve(req, s.Snapshot())
	out := ApplyToSemanticRequest(req, res)
	if out.References[0].Value == "" {
		t.Fatal("expected resolved reference")
	}
	if len(s.Snapshot().Turns) != 1 {
		t.Fatal("context intelligence mutated authoritative state")
	}
	if !res.Historical.Required {
		t.Fatal("expected historical bridge")
	}
}
