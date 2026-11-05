package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func b96332Req(raw string, kind interaction.RequestKind, speech interaction.SpeechAct) interaction.SemanticRequest {
	return interaction.SemanticRequest{
		Raw: raw, Normalized: raw, Kind: kind, SpeechAct: speech,
		Confidence: 0.90, Language: "es",
	}
}

func TestB96332_ExplicitCommandIsActionOriented(t *testing.T) {
	m := NewConversationalCognitiveModel(
		b96332Req("cambia el sistema de enemigos", interaction.KindAction, interaction.SpeechCommand),
		interaction.ConversationSnapshot{},
	)
	e := EvaluateCommunicativeIntent(m)
	if e.Primary != IntentRequestAction || !e.ActionOriented {
		t.Fatalf("expected request_action, got %+v", e)
	}
	if e.NeedsClarification {
		t.Fatalf("explicit command should not require clarification: %+v", e)
	}
}

func TestB96332_ProposalDoesNotBecomeAuthorization(t *testing.T) {
	r := b96332Req("estoy pensando en cambiar el sistema de enemigos", interaction.KindProposal, interaction.SpeechPropose)
	r.Action.Name = "modify_project"
	m := NewConversationalCognitiveModel(r, interaction.ConversationSnapshot{})
	e := EvaluateCommunicativeIntent(m)
	if e.Primary != IntentShareIdea {
		t.Fatalf("expected share_idea, got %+v", e)
	}
	if e.ActionOriented {
		t.Fatalf("proposal must not be converted into operational action: %+v", e)
	}
}

func TestB96332_OpinionQuestionBecomesAskOpinion(t *testing.T) {
	r := b96332Req("¿qué opinas del sistema?", interaction.KindQuestion, interaction.SpeechQuestion)
	r.QuestionType = "opinion"
	r.Meaning = interaction.SemanticMeaning{Concepts: []interaction.SemanticConcept{interaction.ConceptQuestion, interaction.ConceptOpinion}}
	m := NewConversationalCognitiveModel(r, interaction.ConversationSnapshot{})
	e := EvaluateCommunicativeIntent(m)
	if e.Primary != IntentAskOpinion || !e.QuestionLike {
		t.Fatalf("expected ask_opinion, got %+v", e)
	}
}

func TestB96332_DecisionMeaningIsDecisionOriented(t *testing.T) {
	r := b96332Req("decidamos cambiar el sistema", interaction.KindConversation, interaction.SpeechInform)
	r.Meaning = interaction.SemanticMeaning{Concepts: []interaction.SemanticConcept{interaction.ConceptDecision}}
	m := NewConversationalCognitiveModel(r, interaction.ConversationSnapshot{})
	e := EvaluateCommunicativeIntent(m)
	if e.Primary != IntentDecide || !e.DecisionOriented {
		t.Fatalf("expected decide, got %+v", e)
	}
}

func TestB96332_QuestionUsesCanonicalOpinionAct(t *testing.T) {
	r := b96332Req("¿qué opinas del sistema?", interaction.KindQuestion, interaction.SpeechQuestion)
	r.QuestionType = "fact"
	m := NewConversationalCognitiveModel(r, interaction.ConversationSnapshot{})
	e := EvaluateCommunicativeIntent(m)
	if e.Primary != IntentAskOpinion || !e.QuestionLike {
		t.Fatalf("expected question, got %+v", e)
	}
}

func TestB96332_ContextSupportsFollowUp(t *testing.T) {
	ctx := interaction.ConversationSnapshot{
		Turns: []interaction.TurnRecord{{
			ID: 4, Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion,
		}},
		ActiveTopic: "enemigos",
	}
	r := b96332Req("la segunda", interaction.KindContinuation, interaction.SpeechContinue)
	r.FollowUp = true
	r.AddressesCurrent = true
	m := NewConversationalCognitiveModel(r, ctx)
	e := EvaluateCommunicativeIntent(m)
	if e.Primary != IntentContinue {
		t.Fatalf("expected continue, got %+v", e)
	}
}

func TestB96332_UnknownSemanticEvidenceRequestsClarification(t *testing.T) {
	m := NewConversationalCognitiveModel(
		interaction.SemanticRequest{Raw: "no sé"},
		interaction.ConversationSnapshot{},
	)
	e := EvaluateCommunicativeIntent(m)
	if e.Primary != IntentUnknown || !e.NeedsClarification {
		t.Fatalf("expected unknown + clarification, got %+v", e)
	}
}

func TestB96332_DoesNotMutateInputs(t *testing.T) {
	r := b96332Req("propongo revisar esto", interaction.KindProposal, interaction.SpeechPropose)
	ctx := interaction.ConversationSnapshot{ActiveTopic: "sistema"}
	before := ctx.ActiveTopic
	_ = EvaluateCommunicativeIntent(NewConversationalCognitiveModel(r, ctx))
	if ctx.ActiveTopic != before {
		t.Fatal("evaluator mutated conversation context")
	}
}

func TestB9633ConsumesInteractionCommunicativeActForExploration(t *testing.T) {
	r := interaction.Analyze("Estoy pensando en cambiar el sistema pero aún no me decido", interaction.Context{})
	model := ConversationalCognitiveModel{Request: r, Context: interaction.ConversationSnapshot{Language: "es"}, Signals: ConversationalCognitiveSignals{Confidence: 0.9}}
	got := EvaluateCommunicativeIntent(model)
	if got.Primary != IntentShareIdea && got.Primary != IntentExplore {
		t.Fatalf("primary intent = %q, want idea/exploration; acts=%+v candidates=%+v", got.Primary, r.CommunicativeActs, got.Candidates)
	}
	if got.ActionOriented {
		t.Fatal("exploration must not become an action authorization")
	}
}

func TestB9633DeclarationDoesNotBecomeKnowledgeRequest(t *testing.T) {
	r := interaction.Analyze("Mi color favorito es negro", interaction.Context{})
	model := ConversationalCognitiveModel{Request: r, Context: interaction.ConversationSnapshot{Language: "es"}, Signals: ConversationalCognitiveSignals{Confidence: 0.9}}
	got := EvaluateCommunicativeIntent(model)
	if got.Primary != IntentInform {
		t.Fatalf("primary intent = %q, want inform; acts=%+v candidates=%+v", got.Primary, r.CommunicativeActs, got.Candidates)
	}
}
