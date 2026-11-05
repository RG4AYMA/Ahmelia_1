package contextintelligence

import (
	"ahmelia/interaction"
	"testing"
)

func TestB9636ExplicitTopicBoundaryInvalidatesOldReferent(t *testing.T) {
	s := interaction.NewConversationState()
	s.RecordTurn(interaction.SemanticRequest{Raw: "Qué habíamos decidido sobre Aurora?", Normalized: "que habiamos decidido sobre aurora", Topic: "Aurora", Kind: interaction.KindQuestion}, "Decidimos usar Aurora")
	req := interaction.SemanticRequest{Raw: "Eso ya no importa, hablemos de otra cosa", Normalized: "eso ya no importa hablemos de otra cosa", FollowUp: true, References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "current referent"}}}
	r := New().Resolve(req, s.Snapshot())
	if r.Continuity.Kind != ContinuityNewTopic {
		t.Fatalf("expected new_topic, got %v", r.Continuity)
	}
	if len(r.References) != 0 {
		t.Fatalf("old deictic reference leaked across boundary: %#v", r.References)
	}
	if len(r.Candidates) != 0 {
		t.Fatalf("old context remained selectable after boundary: %#v", r.Candidates)
	}
	out := ApplyToSemanticRequest(req, r)
	if !out.BreaksContinuity || out.FollowUp || out.Continues || out.AddressesCurrent {
		t.Fatalf("boundary flags not projected correctly: %#v", out)
	}
}

func TestB9636HistoricalBridgeDoesNotInheritOldTopicAcrossBoundary(t *testing.T) {
	s := interaction.NewConversationState()
	s.RecordTurn(interaction.SemanticRequest{Raw: "Decidimos usar Aurora", Normalized: "decidimos usar aurora", Topic: "Aurora", Kind: interaction.KindConversation}, "Ok")
	req := interaction.SemanticRequest{Raw: "Eso ya no importa, hablemos de otra cosa", Normalized: "eso ya no importa hablemos de otra cosa"}
	r := New().Resolve(req, s.Snapshot())
	if r.Historical.Required {
		t.Fatalf("topic boundary was incorrectly promoted to historical recall: %#v", r.Historical)
	}
	if r.Historical.Query != req.Raw {
		t.Fatalf("historical query was rewritten across boundary: %q", r.Historical.Query)
	}
}

func TestB9636ResolvesDecisionDeixisFromConversationState(t *testing.T) {
	s := interaction.NewConversationState()
	turn := s.RecordTurn(interaction.SemanticRequest{Raw: "Decidimos usar Aurora", Normalized: "decidimos usar aurora", Topic: "Aurora", Kind: interaction.KindConversation}, "Ok")
	s.AddDecision("Aurora", "usar Aurora para supervisar el estado del sistema", turn)
	r := New().Resolve(interaction.SemanticRequest{Raw: "¿Por qué cambiamos esa decisión?", Normalized: "por que cambiamos esa decision"}, s.Snapshot())
	found := false
	for _, ref := range r.References {
		if ref.Expression == "esa decisión" && ref.Kind == ReferenceDecision && ref.Resolved && ref.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("decision deixis was not resolved: %#v", r.References)
	}
	if !r.Historical.Required {
		t.Fatalf("expected historical bridge for decision evolution")
	}
}

func TestB9636CurrentContextRecallDoesNotCrossIntoHistoricalMemory(t *testing.T) {
	s := interaction.NewConversationState()
	s.EnsureThread("Aurora", "supervisar el estado del sistema")
	r := New().Resolve(interaction.SemanticRequest{
		Raw:        "¿Qué era lo que estábamos haciendo antes?",
		Normalized: "que era lo que estabamos haciendo antes",
		Kind:       interaction.KindQuestion,
	}, s.Snapshot())
	if r.Historical.Required {
		t.Fatalf("current-context recall must not request cross-session history: %#v", r.Historical)
	}
}
