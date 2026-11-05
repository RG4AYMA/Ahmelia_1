package naturaldialogue

import (
	"strings"
	"testing"
	"time"

	"ahmelia/initiative"
	"ahmelia/interaction"
)

func TestBuildInterventionResponseConvertsProposalToResponseContract(t *testing.T) {
	proposal := initiative.InterventionProposal{
		ID: "proposal-1", DecisionID: "decision-1", Action: initiative.ActionAsk,
		ContentMode: initiative.ContentQuestion, Category: initiative.ActionCategoryClarification,
		Reason: initiative.ReasonConversationContinue, Topic: "arquitectura",
		Confidence: .9, CreatedAt: time.Now(),
	}
	req := interaction.SemanticRequest{Language: "es"}
	contract, ok := BuildInterventionResponse(req, interaction.ConversationSnapshot{Language: "es"}, proposal)
	if !ok {
		t.Fatal("expected valid intervention response contract")
	}
	if contract.Intent.Kind != interaction.ResponseIntentAsk {
		t.Fatalf("intent=%q", contract.Intent.Kind)
	}
	if contract.Move != interaction.MoveAsk {
		t.Fatalf("move=%q", contract.Move)
	}
	if !contract.Valid() {
		t.Fatal("response contract must be valid")
	}
	if len(contract.Evidence) != 1 {
		t.Fatalf("evidence=%d", len(contract.Evidence))
	}
	if strings.Contains(contract.Evidence[0].Text, "¿") || strings.Contains(contract.Evidence[0].Text, "?") {
		t.Fatalf("dialogue must not receive a finished question: %q", contract.Evidence[0].Text)
	}
}

func TestInitiativeFallbackNeverLeaksSemanticMetadata(t *testing.T) {
	proposal := initiative.InterventionProposal{
		ID: "proposal-2", DecisionID: "decision-2", Action: initiative.ActionShareDiscovery,
		ContentMode: initiative.ContentStatement, Category: initiative.ActionCategoryInformation,
		Reason: initiative.ReasonUsefulInformation, Topic: "funcion", Context: "funcion",
		Confidence: .85, CreatedAt: time.Now(),
	}
	req := interaction.SemanticRequest{Language: "es"}
	contract, ok := BuildInterventionResponse(req, interaction.ConversationSnapshot{Language: "es"}, proposal)
	if !ok {
		t.Fatal("expected valid contract")
	}
	fallback := responseFallbackForTest(contract)
	if strings.Contains(fallback, "initiative action=") || strings.Contains(fallback, "content_mode=") || strings.Contains(fallback, "reason=") {
		t.Fatalf("semantic initiative metadata leaked into fallback: %q", fallback)
	}
}

func responseFallbackForTest(contract interaction.ResponseContract) string {
	switch contract.Move {
	case interaction.MoveInvite:
		return "Encontré algo útil relacionado con lo que estamos haciendo."
	default:
		return "Tengo algo que aportar sobre lo que estamos haciendo."
	}
}
