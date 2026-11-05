package language

import (
	"context"
	"testing"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/socialinteraction"
)

type b96GuardrailClient struct {
	responses []string
	calls     int
}

func (c *b96GuardrailClient) Available() bool { return true }
func (c *b96GuardrailClient) GenerateContext(_ context.Context, _ string) (string, error) {
	if c.calls >= len(c.responses) {
		return `{"response":"respuesta agotada"}`, nil
	}
	out := c.responses[c.calls]
	c.calls++
	return out, nil
}

func b96FeedbackRequest(raw string) interaction.SemanticRequest {
	return interaction.SemanticRequest{
		Raw: raw,
		CommunicativeActs: interaction.CommunicativeActs{
			Primary: interaction.ActFeedback,
		},
	}
}

func TestNoInventionGuardrailDetectsFutureCommitmentInFeedback(t *testing.T) {
	req := b96FeedbackRequest("eso no quedó bien")
	if !violatesFeedbackInventionGuardrail(req, "Voy a mejorar eso") {
		t.Fatal("expected feedback future commitment to be rejected")
	}
}

func TestNoInventionGuardrailAllowsFutureCommitmentOutsideFeedback(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "podemos hacer una propuesta",
		CommunicativeActs: interaction.CommunicativeActs{
			Primary: interaction.ActProposal,
		},
	}
	if violatesFeedbackInventionGuardrail(req, "Haré la propuesta") {
		t.Fatal("proposal future commitment must not be blocked by feedback guardrail")
	}
}

func TestNoInventionGuardrailRetryUsesCleanResponse(t *testing.T) {
	client := &b96GuardrailClient{responses: []string{
		`{"response":"Voy a mejorar eso"}`,
		`{"response":"Entiendo la observación y la tomo en cuenta."}`,
	}}
	g := NewConversationalGenerator(client)
	req := b96FeedbackRequest("eso no quedó bien")
	got, err := g.GenerateGrounded(req, interaction.ConversationSnapshot{}, interaction.ResponseIntent{}, cognition.ConversationalResponseStrategy{}, socialinteraction.ResponsePlan{}, RuntimeFacts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Entiendo la observación y la tomo en cuenta." {
		t.Fatalf("expected clean retry response, got %q", got)
	}
	if client.calls != 2 {
		t.Fatalf("expected exactly two LLM calls, got %d", client.calls)
	}
}

func TestNoInventionGuardrailFallsBackAfterTwoViolations(t *testing.T) {
	req := b96FeedbackRequest("eso no quedó bien")
	client := &b96GuardrailClient{responses: []string{
		`{"response":"Voy a mejorar eso"}`,
		`{"response":"Seguiré trabajando en eso"}`,
	}}
	g := NewConversationalGenerator(client)
	got, err := g.GenerateGrounded(req, interaction.ConversationSnapshot{}, interaction.ResponseIntent{}, cognition.ConversationalResponseStrategy{}, socialinteraction.ResponsePlan{}, RuntimeFacts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := deterministicFeedbackAcknowledge(req)
	if got != expected {
		t.Fatalf("expected deterministic fallback %q, got %q", expected, got)
	}
	if client.calls != 2 {
		t.Fatalf("expected exactly two LLM calls, got %d", client.calls)
	}
}
