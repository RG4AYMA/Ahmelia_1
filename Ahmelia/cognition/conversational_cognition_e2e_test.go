package cognition

import (
	"reflect"
	"testing"

	"ahmelia/interaction"
)

func TestB963312_E2E_ClearQuestion(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "¿qué es una variable?", Normalized: "que es una variable",
		Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion,
		QuestionType: "definition", Confidence: 0.96,
	}
	ctx := interaction.ConversationSnapshot{ActiveTopic: "programación"}
	before := ctx

	out := RunConversationalCognitionE2E(req, ctx)
	if !out.Valid {
		t.Fatalf("expected valid E2E, got errors: %v", out.Errors)
	}
	if out.Intent.Primary != IntentQuestion || out.Action.Primary != ActionRespond {
		t.Fatalf("unexpected question pipeline: intent=%v action=%v", out.Intent.Primary, out.Action.Primary)
	}
	if out.Strategy.Mode != ResponseModeAnswer || !out.Strategy.ShouldGenerateLanguage {
		t.Fatalf("unexpected response strategy: %+v", out.Strategy)
	}
	if !out.Adaptation.Transient {
		t.Fatalf("adaptation must remain transient: %+v", out.Adaptation)
	}
	if !reflect.DeepEqual(ctx, before) {
		t.Fatalf("E2E must not mutate conversation snapshot")
	}
}

func TestB963312_E2E_UnresolvedReferenceStopsExecution(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hazlo con ese", Normalized: "hazlo con ese",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "hacerlo", Confidence: 0.95,
		References: []interaction.Reference{{Kind: interaction.ReferenceCurrentInput, Expression: "ese", Resolved: false}},
	}

	out := RunConversationalCognitionE2E(req, interaction.ConversationSnapshot{})
	if !out.Valid {
		t.Fatalf("expected valid gated E2E, got errors: %v", out.Errors)
	}
	if !out.Interpretation.HasUnresolvedRefs || !out.Action.Blocked {
		t.Fatalf("expected unresolved reference to block action: interpretation=%+v action=%+v", out.Interpretation, out.Action)
	}
	if out.Strategy.Mode != ResponseModeClarify || !out.Adaptation.Caution {
		t.Fatalf("expected clarification/caution: strategy=%+v adaptation=%+v", out.Strategy, out.Adaptation)
	}
}

func TestB963312_E2E_ProposalPreservesAgency(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "podríamos cambiar los enemigos", Normalized: "podriamos cambiar los enemigos",
		Kind: interaction.KindProposal, SpeechAct: interaction.SpeechPropose,
		Objective: interaction.Objective{Text: "cambiar enemigos"}, Confidence: 0.94,
	}

	out := RunConversationalCognitionE2E(req, interaction.ConversationSnapshot{ActiveWorkID: "w1"})
	if !out.Valid {
		t.Fatalf("expected valid E2E, got errors: %v", out.Errors)
	}
	if out.Action.Primary == ActionRequestExecution || out.Strategy.Mode == ResponseModeRequestExecution {
		t.Fatalf("proposal must not become execution: action=%v strategy=%v", out.Action.Primary, out.Strategy.Mode)
	}
	if !out.Strategy.PreserveUserAgency || !out.Adaptation.PreserveUserAgency {
		t.Fatalf("proposal must preserve agency: strategy=%+v adaptation=%+v", out.Strategy, out.Adaptation)
	}
}

func TestB963312_E2E_NewTopicBreaksOldWorkAdaptation(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "hablemos de música", Normalized: "hablemos de musica",
		Kind: interaction.KindConversation, SpeechAct: interaction.SpeechInform,
		Topic: "música", ExplicitDomain: "music", Confidence: 0.95,
	}
	ctx := interaction.ConversationSnapshot{ActiveWorkID: "w1", ActiveTopic: "enemigos", ActiveObjective: "mejorar enemigos"}

	out := RunConversationalCognitionE2E(req, ctx)
	if !out.Valid {
		t.Fatalf("expected valid E2E, got errors: %v", out.Errors)
	}
	if out.Relevance.Primary != RelevanceNewTopic || out.Adaptation.Mode != AdaptationModeResetContext || out.Adaptation.Continuity {
		t.Fatalf("expected local reset: relevance=%v adaptation=%+v", out.Relevance.Primary, out.Adaptation)
	}
}

func TestB963312_E2E_ActiveWorkActionRemainsDownstream(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "cambia enemigos", Normalized: "cambia enemigos",
		Kind: interaction.KindAction, SpeechAct: interaction.SpeechCommand,
		RequestedAction: "cambiar enemigos", Objective: interaction.Objective{Text: "cambiar enemigos"},
		Confidence: 0.95,
	}
	ctx := interaction.ConversationSnapshot{ActiveWorkID: "w1", ActiveTopic: "enemigos"}

	out := RunConversationalCognitionE2E(req, ctx)
	if !out.Valid {
		t.Fatalf("expected valid E2E, got errors: %v", out.Errors)
	}
	if out.Action.Primary != ActionRequestExecution || out.Strategy.Mode != ResponseModeRequestExecution {
		t.Fatalf("expected conceptual execution request: action=%v strategy=%v", out.Action.Primary, out.Strategy.Mode)
	}
	if !out.Strategy.ExecutionBoundary || !out.Adaptation.PreserveUserAgency {
		t.Fatalf("execution boundary/agency must remain explicit: strategy=%+v adaptation=%+v", out.Strategy, out.Adaptation)
	}
}
