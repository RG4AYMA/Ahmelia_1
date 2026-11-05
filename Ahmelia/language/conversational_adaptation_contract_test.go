package language

import (
	"strings"
	"testing"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/socialinteraction"
)

func TestSupportiveAdaptationFlowsFromTurnStateIntoPrompt(t *testing.T) {
	neutralReq := interaction.SemanticRequest{
		Raw:  "Estoy completando el formulario y necesito orientación.",
		Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform,
	}
	frustratedReq := neutralReq
	frustratedReq.Raw = "Estoy completando el formulario y necesito orientación. Estoy frustrado."
	frustratedReq.UserSignals.Frustration = 0.75

	neutralState := interaction.NewConversationState()
	neutralState.RecordTurnLinked(neutralReq, "", interaction.RelationIndependent, 0)
	neutralSnapshot := neutralState.Snapshot()
	neutral := cognition.RunConversationalCognitionE2E(neutralReq, neutralSnapshot)
	neutralPrompt := buildConversationalPrompt(neutralReq, neutralSnapshot, interaction.ResponseIntent{}, neutral.Strategy, socialinteraction.ResponsePlan{}, RuntimeFacts{}, nil)

	frustratedState := interaction.NewConversationState()
	frustratedState.RecordTurnLinked(frustratedReq, "", interaction.RelationIndependent, 0)
	frustratedSnapshot := frustratedState.Snapshot()
	if frustratedSnapshot.UserState.Frustration < 0.70 {
		t.Fatalf("semantic frustration signal was lost at ConversationState: %+v", frustratedSnapshot.UserState)
	}
	frustrated := cognition.RunConversationalCognitionE2E(frustratedReq, frustratedSnapshot)
	if frustrated.Adaptation.Mode != cognition.AdaptationModeSupportive {
		t.Fatalf("expected supportive adaptation, got %+v", frustrated.Adaptation)
	}
	if frustrated.Strategy.Adaptation.Mode != cognition.AdaptationModeSupportive {
		t.Fatalf("response strategy did not carry the adaptation: %+v", frustrated.Strategy.Adaptation)
	}
	frustratedPrompt := buildConversationalPrompt(frustratedReq, frustratedSnapshot, interaction.ResponseIntent{}, frustrated.Strategy, socialinteraction.ResponsePlan{}, RuntimeFacts{}, nil)
	if !strings.Contains(frustratedPrompt, "modo=supportive") || !strings.Contains(frustratedPrompt, "no conviertas el relato en una orden de trabajo") {
		t.Fatalf("supportive adaptation missing from generation prompt:\n%s", frustratedPrompt)
	}
	if strings.Contains(neutralPrompt, "no conviertas el relato en una orden de trabajo") {
		t.Fatalf("supportive directive leaked into neutral prompt:\n%s", neutralPrompt)
	}
}
