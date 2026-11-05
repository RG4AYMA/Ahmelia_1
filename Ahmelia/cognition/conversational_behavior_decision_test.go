package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func behaviorTestRequest() interaction.SemanticRequest {
	return interaction.SemanticRequest{
		Raw: "¿Qué significa este campo del formulario?", Normalized: "que significa este campo del formulario",
		Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion,
		QuestionType: "definition", Confidence: .94,
	}
}

func TestB96401_StateChangesConductAndLanguageOnlyExpressesIt(t *testing.T) {
	req := behaviorTestRequest()
	neutral := RunConversationalCognitionE2E(req, interaction.ConversationSnapshot{})
	frustrated := RunConversationalCognitionE2E(req, interaction.ConversationSnapshot{
		UserState: interaction.UserState{Frustration: .75},
	})
	if !neutral.Valid || !frustrated.Valid {
		t.Fatalf("invalid cognition: neutral=%v frustrated=%v", neutral.Errors, frustrated.Errors)
	}
	if neutral.Action.Primary != ActionRespond {
		t.Fatalf("neutral action = %s, want respond", neutral.Action.Primary)
	}
	if frustrated.Adaptation.Mode != AdaptationModeSupportive || frustrated.Action.Primary != ActionDiscuss {
		t.Fatalf("frustration did not alter conduct: adaptation=%s action=%s evidence=%v", frustrated.Adaptation.Mode, frustrated.Action.Primary, frustrated.Action.Evidence)
	}
	if neutral.Behavior.Action == frustrated.Behavior.Action || !frustrated.Behavior.SpeakToCurrentTurn {
		t.Fatalf("expected a changed but communicative final decision: neutral=%+v frustrated=%+v", neutral.Behavior, frustrated.Behavior)
	}
	if frustrated.Behavior.Strategy.Mode != ResponseModeDiscuss || !frustrated.Behavior.Strategy.ShouldGenerateLanguage {
		t.Fatalf("language strategy did not receive the final conduct: %+v", frustrated.Behavior)
	}
}

func TestB96401_ConfusionUsesSameGeneralAdaptationContract(t *testing.T) {
	out := RunConversationalCognitionE2E(behaviorTestRequest(), interaction.ConversationSnapshot{
		UserState: interaction.UserState{Confusion: .75},
	})
	if !out.Valid {
		t.Fatalf("invalid cognition: %v", out.Errors)
	}
	if out.Adaptation.Mode != AdaptationModeSupportive || out.Action.Primary != ActionDiscuss || !out.Behavior.SpeakToCurrentTurn {
		t.Fatalf("confusion did not use the general supportive participation: adaptation=%+v action=%+v behavior=%+v", out.Adaptation, out.Action, out.Behavior)
	}
}

func TestB96401_WaitSuppressesLanguageButInitiativeWaitDoesNotCancelDirectTurn(t *testing.T) {
	stop := behaviorTestRequest()
	stop.CommunicativeActs = interaction.CommunicativeActs{Primary: interaction.ActControl}
	stop.Kind = interaction.KindPause
	stop.SpeechAct = interaction.SpeechCommand
	out := RunConversationalCognitionE2E(stop, interaction.ConversationSnapshot{})
	if !out.Valid || out.Behavior.Action != ActionWait {
		t.Fatalf("expected cognitive wait decision: %+v errors=%v", out.Behavior, out.Errors)
	}
	if out.Behavior.SpeakToCurrentTurn || out.Behavior.Strategy.ShouldGenerateLanguage {
		t.Fatalf("wait decision leaked language generation: %+v", out.Behavior)
	}

	response := RunConversationalCognitionE2E(behaviorTestRequest(), interaction.ConversationSnapshot{
		UserState: interaction.UserState{Frustration: .75},
	})
	intent := interaction.ResponseIntent{Kind: interaction.ResponseIntentAcknowledge, ShouldSpeak: true}
	direct := ResolveConversationalBehavior(response.Action, response.Strategy, intent).WithInitiative(InitiativeWait)
	if !direct.SpeakToCurrentTurn || direct.Action != ActionDiscuss || direct.AdditionalInitiative != InitiativeWait {
		t.Fatalf("additional-initiative wait contradicted current-turn response: %+v", direct)
	}
}

func TestB96401_ContinuityCarriesStateIntoNextDecision(t *testing.T) {
	req := behaviorTestRequest()
	first := RunConversationalCognitionE2E(req, interaction.ConversationSnapshot{UserState: interaction.UserState{Frustration: .75}})
	nextContext := interaction.ConversationSnapshot{
		UserState:   first.Model.Context.UserState,
		Turns:       []interaction.TurnRecord{{ID: 12, UserText: req.Raw, Assistant: "Te ayudo con calma.", Topic: "formulario"}},
		ActiveTopic: "formulario",
	}
	next := RunConversationalCognitionE2E(interaction.SemanticRequest{
		Raw: "No entiendo el siguiente campo", Normalized: "no entiendo el siguiente campo",
		Kind: interaction.KindQuestion, SpeechAct: interaction.SpeechQuestion,
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActQuestion}, Confidence: .92,
	}, nextContext)
	if !first.Valid || !next.Valid {
		t.Fatalf("invalid cognition: first=%v next=%v", first.Errors, next.Errors)
	}
	if next.Model.Context.UserState.Frustration != .75 || next.Model.Context.ActiveTopic != "formulario" || !next.Model.Situation.HasHistory {
		t.Fatalf("prior context was not available to the next decision: model=%+v", next.Model)
	}
	if next.Adaptation.Mode != AdaptationModeSupportive || next.Action.Primary != ActionDiscuss {
		t.Fatalf("next decision did not use carried state and new question semantics: adaptation=%+v action=%+v", next.Adaptation, next.Action)
	}
}
