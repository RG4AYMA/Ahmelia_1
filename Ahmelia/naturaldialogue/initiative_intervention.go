package naturaldialogue

import (
	"fmt"
	"strings"

	"ahmelia/initiative"
	"ahmelia/interaction"
)

// BuildInterventionResponse is the Dialogue boundary for authorized
// initiative. Initiative supplies only a semantic InterventionProposal.
// Dialogue chooses the conversational movement and creates the canonical
// ResponseContract. No finished sentence is accepted here.
func BuildInterventionResponse(req interaction.SemanticRequest, snap interaction.ConversationSnapshot, proposal initiative.InterventionProposal) (interaction.ResponseContract, bool) {
	if !proposal.Valid() {
		return interaction.ResponseContract{}, false
	}

	intent := responseIntentForInitiative(proposal, req, snap)
	move := moveForInitiative(proposal.Action)
	if !interaction.ResponseMoveIsValid(move) || intent.Kind == interaction.ResponseIntentNone {
		return interaction.ResponseContract{}, false
	}

	contract := interaction.NewResponseContract(req, intent, move)
	contract.Source = "initiative_intervention"
	contract.Confidence = proposal.Confidence
	contract.ShouldSpeak = true
	contract.ShouldAct = false

	// Evidence is semantic guidance only. It is deliberately not a candidate
	// sentence. Language receives the purpose/topic/reason and realizes the
	// final wording downstream.
	semantic := fmt.Sprintf("initiative action=%s content_mode=%s reason=%s", proposal.Action, proposal.ContentMode, proposal.Reason)
	if topic := strings.TrimSpace(proposal.Topic); topic != "" {
		semantic += "; topic=" + topic
	}
	if ctx := strings.TrimSpace(proposal.Context); ctx != "" {
		semantic += "; context=" + ctx
	}
	contract.AddEvidence(semantic, "initiative_proposal", true, proposal.Confidence)
	return contract, true
}

func responseIntentForInitiative(proposal initiative.InterventionProposal, req interaction.SemanticRequest, snap interaction.ConversationSnapshot) interaction.ResponseIntent {
	lang := req.Language
	if lang == "" {
		lang = snap.Language
	}
	intent := interaction.ResponseIntent{
		Confidence:     proposal.Confidence,
		Language:       lang,
		ThreadID:       snap.ActiveThreadID,
		Objective:      strings.TrimSpace(snap.ActiveObjective),
		ShouldSpeak:    true,
		ShouldContinue: true,
		Reason:         "authorized initiative intervention",
	}
	switch proposal.Action {
	case initiative.ActionAsk:
		intent.Kind = interaction.ResponseIntentAsk
		intent.NeedsUserInput = true
	case initiative.ActionSuggest, initiative.ActionPropose, initiative.ActionPresentPossibility:
		intent.Kind = interaction.ResponseIntentPropose
		intent.NeedsUserInput = proposal.Action != initiative.ActionSuggest
	case initiative.ActionResume:
		intent.Kind = interaction.ResponseIntentResume
		intent.ShouldResume = true
	case initiative.ActionComment, initiative.ActionContribute, initiative.ActionShareDiscovery, initiative.ActionWarn, initiative.ActionDisagree:
		intent.Kind = interaction.ResponseIntentAcknowledge
	case initiative.ActionRemind, initiative.ActionContinue:
		intent.Kind = interaction.ResponseIntentContinue
	case initiative.ActionRedirect:
		intent.Kind = interaction.ResponseIntentRedirect
	default:
		intent.Kind = interaction.ResponseIntentNone
	}
	if intent.Confidence == 0 {
		intent.Confidence = .5
	}
	return intent
}

func moveForInitiative(action initiative.Action) interaction.ResponseMove {
	switch action {
	case initiative.ActionAsk:
		return interaction.MoveAsk
	case initiative.ActionSuggest:
		return interaction.MovePropose
	case initiative.ActionPropose, initiative.ActionPresentPossibility:
		return interaction.MovePropose
	case initiative.ActionResume:
		return interaction.MoveResume
	case initiative.ActionComment, initiative.ActionContribute, initiative.ActionShareDiscovery:
		return interaction.MoveInvite
	case initiative.ActionWarn, initiative.ActionDisagree:
		return interaction.MoveAcknowledge
	case initiative.ActionRemind, initiative.ActionContinue:
		return interaction.MoveContinue
	case initiative.ActionRedirect:
		return interaction.MoveRedirect
	default:
		return interaction.MoveWait
	}
}
