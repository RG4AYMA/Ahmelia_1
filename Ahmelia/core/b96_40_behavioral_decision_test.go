package core

import (
	"context"
	"testing"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/language"
)

type b9640CountingLanguageClient struct{ calls int }

func (c *b9640CountingLanguageClient) Available() bool { return true }
func (c *b9640CountingLanguageClient) GenerateContext(context.Context, string) (string, error) {
	c.calls++
	return `{"response":"Estoy aquí contigo."}`, nil
}

func TestB9640_FinalWaitDoesNotReachLanguageButInitiativeWaitPreservesDirectTurn(t *testing.T) {
	e := newB963334AuditEngine(t)
	client := &b9640CountingLanguageClient{}
	e.conversationGenerator = language.NewConversationalGenerator(client)
	req := interaction.SemanticRequest{Raw: "para", Normalized: "para", Kind: interaction.KindPause, Confidence: .95}
	intent := interaction.ResponseIntent{Kind: interaction.ResponseIntentWait, ShouldSpeak: true, Confidence: .95}
	contract := interaction.NewResponseContract(req, intent, interaction.MoveAcknowledge)
	contract.Source = "b96.39_response_contract"
	strategy := cognition.ConversationalResponseStrategy{Version: "B96.33.10", Mode: cognition.ResponseModeWait, ShouldGenerateLanguage: true}
	action := cognition.ConversationalActionSelection{Version: "B96.33.9", Primary: cognition.ActionWait, Reason: "explicit control"}
	e.lastBehavioralDecision = cognition.ResolveConversationalBehavior(action, strategy, intent)

	if e.lastBehavioralDecision.SpeakToCurrentTurn || e.lastBehavioralDecision.Strategy.ShouldGenerateLanguage {
		t.Fatalf("WAIT did not close the Language boundary: %+v", e.lastBehavioralDecision)
	}
	if _, ok := e.generateResponseContractWithStrategy(req, contract, language.RuntimeFacts{}, e.lastBehavioralDecision.Strategy); ok {
		t.Fatal("WAIT unexpectedly produced response text")
	}
	if client.calls != 0 {
		t.Fatalf("Language was called after WAIT: calls=%d", client.calls)
	}

	answer := cognition.ConversationalActionSelection{Version: "B96.33.9", Primary: cognition.ActionRespond, Reason: "direct answer"}
	direct := cognition.ResolveConversationalBehavior(answer,
		cognition.ConversationalResponseStrategy{Version: "B96.33.10", Mode: cognition.ResponseModeAnswer, ShouldGenerateLanguage: true},
		interaction.ResponseIntent{Kind: interaction.ResponseIntentAnswer, ShouldSpeak: true}).WithInitiative(cognition.InitiativeWait)
	if !direct.SpeakToCurrentTurn || direct.AdditionalInitiative != cognition.InitiativeWait {
		t.Fatalf("initiative WAIT cancelled a direct turn: %+v", direct)
	}
	e.lastBehavioralDecision = direct
	contract.Intent = interaction.ResponseIntent{Kind: interaction.ResponseIntentAnswer, ShouldSpeak: true, Confidence: .95}
	if _, ok := e.generateResponseContractWithStrategy(req, contract, language.RuntimeFacts{}, direct.Strategy); !ok {
		t.Fatal("direct response should remain renderable when additional initiative waits")
	}
	if client.calls == 0 {
		t.Fatal("direct response did not reach Language")
	}
}
