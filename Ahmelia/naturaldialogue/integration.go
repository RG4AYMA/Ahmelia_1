package naturaldialogue

import (
	"ahmelia/interaction"
	"ahmelia/situation"
	"strings"
)

// Integration is the Engine-facing adapter. It only evaluates dialogue structure.
// It never selects a capability, generates text, or authorizes initiative.
type Integration struct{ Threads *ThreadManager }

func NewIntegration() *Integration { return &Integration{Threads: NewThreadManager()} }

func (i *Integration) EvaluateRequest(req interaction.SemanticRequest, snap interaction.ConversationSnapshot, currentTurn TurnOwner) Decision {
	prevTopic := snap.ActiveTopic
	input := req.Raw
	closing, reason := DetectClosing(input)
	transition := Transition(prevTopic, input)
	turn, action := ManageTurn(input, currentTurn == AhmeliaTurn)
	if currentTurn == AhmeliaTurn {
		return Decision{Turn: AhmeliaTurn, Action: Yield, Flow: Engaged, RequiresDirectTurn: true, Reason: "Ahmelia already owns the current turn"}
	}
	if closing {
		action = Close
	}
	q := QuestionType(input)
	isAnswer := IsAnswerToPrevious(req, snap)
	direct := RequestRequiresDirectTurn(req)
	// A direct user need always wins over optional dialogue continuation.
	// An answer to an open question is structurally complete, but may still allow
	// a proactive follow-up if initiative later authorizes it.
	allow := !direct && !closing && action == Continue
	return Decision{
		Turn: turn, Action: action, Flow: FlowFor(turn, closing, transition), Topic: TopicFlow(prevTopic, input),
		FollowUp: FollowUp(input), Question: q, Length: ResponseLength(input), Adaptation: Adapt(input), Rhythm: RhythmFor(input),
		Clarify: NeedsClarification(input), Corrected: IsCorrection(input), Closing: closing, CloseReason: reason,
		Completion: CompleteAnswer(input), RequiresDirectTurn: direct, IsAnswerToPreviousTurn: isAnswer,
		AllowProactiveFollowUp: allow, Reason: dialogueReason(req, direct, isAnswer),
	}
}

// SelectMove is the canonical B96.39 movement boundary. It projects the
// already-selected ResponseIntent into a concrete conversational move.
// B96.33/Interaction establish the semantic purpose; B96.39 chooses the
// dialogue movement needed to realize that purpose. No text, provider,
// capability, or initiative authorization is created here.
func SelectMove(req interaction.SemanticRequest, snap interaction.ConversationSnapshot, intent interaction.ResponseIntent, evidence ...situation.DecisionEvidence) interaction.ResponseMove {
	acts := interaction.CanonicalActs(req)
	if req.NeedsClarification || len(req.References) > 0 && hasUnresolvedReference(req) {
		return interaction.MoveClarify
	}
	if acts.Has(interaction.ActControl) {
		return interaction.MoveWait
	}
	if acts.Has(interaction.ActTopicShift) {
		return interaction.MoveRedirect
	}
	if len(evidence) > 0 && (evidence[0].HasKind("gives_feedback_to") || evidence[0].HasKind("addresses")) && intent.Kind == interaction.ResponseIntentAcknowledge {
		return interaction.MoveAcknowledge
	}
	switch intent.Kind {
	case interaction.ResponseIntentAnswer:
		return interaction.MoveAnswer
	case interaction.ResponseIntentAcknowledge:
		return interaction.MoveAcknowledge
	case interaction.ResponseIntentClarify:
		return interaction.MoveClarify
	case interaction.ResponseIntentPropose:
		return interaction.MovePropose
	case interaction.ResponseIntentContinue:
		if acts.Has(interaction.ActContinuation) && snap.ActiveTopic != "" {
			return interaction.MoveContinue
		}
		return interaction.MoveContinue
	case interaction.ResponseIntentResume:
		return interaction.MoveResume
	case interaction.ResponseIntentInvite:
		return interaction.MoveInvite
	case interaction.ResponseIntentAsk:
		return interaction.MoveAsk
	case interaction.ResponseIntentWait:
		return interaction.MoveWait
	case interaction.ResponseIntentRedirect:
		return interaction.MoveRedirect
	default:
		if acts.Has(interaction.ActCommand) {
			// Capability execution is downstream; B96.39 deliberately does not
			// invent a conversational response for an executable command.
			return interaction.MoveWait
		}
		return interaction.MoveClarify
	}
}

func hasUnresolvedReference(req interaction.SemanticRequest) bool {
	for _, ref := range req.References {
		if strings.TrimSpace(ref.Expression) != "" && !ref.Resolved {
			return true
		}
	}
	return false
}

// Evaluate is retained as a compact compatibility API for isolated B96.39 tests.
func (i *Integration) Evaluate(prevTopic, input string, currentTurn TurnOwner) Decision {
	req := interaction.SemanticRequest{Raw: input, Normalized: input, Kind: interaction.KindConversation}
	if QuestionType(input) == Question || QuestionType(input) == MultiQuestion || QuestionType(input) == InformationRequest {
		req.Kind = interaction.KindQuestion
	}
	snap := interaction.ConversationSnapshot{ActiveTopic: prevTopic}
	return i.EvaluateRequest(req, snap, currentTurn)
}

func IsAnswerToPrevious(req interaction.SemanticRequest, snap interaction.ConversationSnapshot) bool {
	if len(snap.Turns) == 0 {
		return false
	}
	// The live Engine evaluates B96.39 after persisting the current user turn.
	// Therefore the tail may be the current turn; in that case inspect the
	// immediately preceding turn, which is the actual conversational parent.
	last := snap.Turns[len(snap.Turns)-1]
	if strings.TrimSpace(last.UserText) != "" && strings.TrimSpace(last.UserText) == strings.TrimSpace(req.Raw) && len(snap.Turns) >= 2 {
		last = snap.Turns[len(snap.Turns)-2]
	}
	lastActs := last.CommunicativeActs
	if lastActs.Primary == "" {
		if strings.TrimSpace(last.UserText) != "" {
			lastActs = interaction.CanonicalActs(interaction.SemanticRequest{Raw: last.UserText, Normalized: last.UserText})
		} else {
			lastActs = interaction.CanonicalActs(interaction.SemanticRequest{Kind: last.Kind, SpeechAct: last.SpeechAct})
		}
	}
	if !lastActs.Has(interaction.ActQuestion) && !lastActs.Has(interaction.ActOpinionRequest) {
		return false
	}
	if acts := interaction.CanonicalActs(req); acts.Has(interaction.ActQuestion) || acts.Has(interaction.ActCommand) {
		return false
	}
	return true
}
func dialogueReason(req interaction.SemanticRequest, direct, answer bool) string {
	if direct {
		return "current user need requires the response path before proactive dialogue"
	}
	if answer {
		return "current turn answers an open conversational turn; proactive continuation remains optional"
	}
	return "current turn can continue dialogue without displacing a direct need"
}
