package core

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"ahmelia/cognition"
	"ahmelia/initiative"
	"ahmelia/interaction"
	"ahmelia/language"
	"ahmelia/naturaldialogue"
)

func TestPhase5InitiativeRuntimeDoesNotConsumeCandidateQuestionAsOutput(t *testing.T) {
	body, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if strings.Contains(s, "renderCanonicalTextResult(semanticReq, \"initiative_question\", candidate.Question") {
		t.Fatal("initiative must not render candidate.Question directly")
	}
	if strings.Contains(s, "conversationGenerator.GenerateGrounded(req, snap, intent") {
		t.Fatal("initiative idle path must not generate language before InterventionProposal + Dialogue")
	}
	if strings.Contains(s, "CandidateText: candidate.Question") {
		t.Fatal("candidate question must not cross into initiative signal content")
	}
}

func TestPhase5InitiativeHasSemanticBoundaryArtifacts(t *testing.T) {
	proposal, err := os.ReadFile("../initiative/intervention_proposal.go")
	if err != nil {
		t.Fatal(err)
	}
	dialogue, err := os.ReadFile("../naturaldialogue/initiative_intervention.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(proposal), "sayAsync") || strings.Contains(string(proposal), "tts.") {
		t.Fatal("initiative proposal must not own output")
	}
	if strings.Contains(string(dialogue), "sayAsync") || strings.Contains(string(dialogue), "tts.") {
		t.Fatal("dialogue proposal adapter must not own output")
	}
}

func TestPhase5InitiativeProposalReachesLanguageWithoutFinishedCandidateText(t *testing.T) {
	proposalSource := `topic=arquitectura`
	_ = proposalSource // semantic evidence is asserted through the Dialogue contract below.

	proposal := initiative.InterventionProposal{
		ID: "proposal-e2e", DecisionID: "decision-e2e", Action: initiative.ActionAsk,
		ContentMode: initiative.ContentQuestion, Category: initiative.ActionCategoryClarification,
		Reason: initiative.ReasonConversationContinue, Topic: "arquitectura", Confidence: .9,
		CreatedAt: time.Now(),
	}
	contract, ok := naturaldialogue.BuildInterventionResponse(interaction.SemanticRequest{Language: "es"}, interaction.ConversationSnapshot{Language: "es"}, proposal)
	if !ok {
		t.Fatal("dialogue rejected intervention proposal")
	}

	client := &phase5FakeLanguageClient{available: true, response: `{"response":"¿Qué parte de la arquitectura quieres explorar ahora?"}`}
	e := &Engine{
		conversationState:     interaction.NewConversationState(),
		conversationGenerator: language.NewConversationalGenerator(client),
	}
	text, rendered := e.generateResponseContractWithStrategy(interaction.SemanticRequest{Language: "es"}, contract, language.RuntimeFacts{}, cognition.ConversationalResponseStrategy{
		Version: "B96.33.10", Mode: cognition.ResponseModeDiscuss, Directness: cognition.DirectnessExploratory,
		Structure: cognition.StructureDiscussion, Initiative: cognition.InitiativeHelpful, ShouldGenerateLanguage: true,
	})
	if !rendered || strings.TrimSpace(text) == "" {
		t.Fatal("language rendering failed")
	}
	if !strings.Contains(client.prompt, "topic=arquitectura") {
		t.Fatalf("language did not receive semantic proposal evidence: %q", client.prompt)
	}
	if strings.Contains(client.prompt, "¿Qué parte de la arquitectura quieres explorar ahora?") {
		t.Fatal("finished output text must not be present upstream of Language")
	}
	if text != "¿Qué parte de la arquitectura quieres explorar ahora?" {
		t.Fatalf("unexpected rendered text=%q", text)
	}
}

type phase5FakeLanguageClient struct {
	available bool
	response  string
	prompt    string
}

func (f *phase5FakeLanguageClient) Available() bool { return f.available }
func (f *phase5FakeLanguageClient) GenerateContext(_ context.Context, prompt string) (string, error) {
	f.prompt = prompt
	return f.response, nil
}
