package language

import (
	"ahmelia/interaction"
	"context"
	"testing"
	"time"
)

func TestConversationalGenerationDefaultAllowsLocalModelLatency(t *testing.T) {
	t.Setenv(conversationalGenerationBudgetEnv, "")
	if got, want := ConversationalGenerationBudget(), 15*time.Second; got != want {
		t.Fatalf("default conversational budget=%s, want %s", got, want)
	}
}

type fakeConversationalClient struct {
	response  string
	available bool
	prompt    string
}

func (f *fakeConversationalClient) Available() bool { return f.available }
func (f *fakeConversationalClient) GenerateContext(_ context.Context, prompt string) (string, error) {
	return f.Generate(prompt)
}

func (f *fakeConversationalClient) Generate(prompt string) (string, error) {
	f.prompt = prompt
	return f.response, nil
}

func TestConversationalGeneratorRendersStructuredResponse(t *testing.T) {
	c := &fakeConversationalClient{available: true, response: `{"response":"Sí, te entiendo. Tienes varias cosas que quieres sacar adelante y es normal que cueste decidir por dónde empezar."}`}
	g := NewConversationalGenerator(c)
	req := interaction.SemanticRequest{Raw: "tengo ganas de hacer tantas cosas que a veces no sé por cuál empezar", Kind: interaction.KindInform}
	intent := interaction.ResponseIntent{Kind: interaction.ResponseIntentInvite}
	got, err := g.Generate(req, interaction.ConversationSnapshot{}, intent)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected response")
	}
	if c.prompt == "" {
		t.Fatal("expected prompt")
	}
}

func TestConversationalGeneratorUnavailableFailsClosed(t *testing.T) {
	c := &fakeConversationalClient{available: false}
	g := NewConversationalGenerator(c)
	_, err := g.Generate(interaction.SemanticRequest{Raw: "hola", Kind: interaction.KindConversation}, interaction.ConversationSnapshot{}, interaction.ResponseIntent{Kind: interaction.ResponseIntentInvite})
	if err == nil {
		t.Fatal("expected unavailable error")
	}
}
