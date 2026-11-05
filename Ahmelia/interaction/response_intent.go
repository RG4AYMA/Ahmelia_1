package interaction

import "strings"

// ResponseIntentKind describes the communicative purpose Ahmelia should
// satisfy for the current turn. It is intentionally above capabilities and
// below language generation: it answers "what kind of response is needed?",
// not "which provider should execute?".
type ResponseIntentKind string

const (
	ResponseIntentNone        ResponseIntentKind = ""
	ResponseIntentAnswer      ResponseIntentKind = "answer"
	ResponseIntentAcknowledge ResponseIntentKind = "acknowledge"
	ResponseIntentClarify     ResponseIntentKind = "clarify"
	ResponseIntentPropose     ResponseIntentKind = "propose"
	ResponseIntentContinue    ResponseIntentKind = "continue"
	ResponseIntentResume      ResponseIntentKind = "resume"
	ResponseIntentInvite      ResponseIntentKind = "invite"
	ResponseIntentAsk         ResponseIntentKind = "ask"
	ResponseIntentWait        ResponseIntentKind = "wait"
	ResponseIntentRedirect    ResponseIntentKind = "redirect"
)

// ResponseIntent is the semantic contract between understanding and response
// planning. It describes the conversational job Ahmelia has to perform.
//
// It deliberately contains no generated sentence and no capability/provider.
// The same intent can eventually be rendered by text, voice, UI or a future
// embodied interface.
// ResponseIntentHint is a transient, non-authoritative interpretation supplied
// by a downstream cognitive consumer after the normal semantic surface proved
// insufficient. It lets the existing response-intent authority consume
// structured cognitive evidence without importing the cognition package.
type ResponseIntentHint struct {
	Kind       string
	Confidence float64
	Reason     string
}

type ResponseIntent struct {
	Kind               ResponseIntentKind
	Confidence         float64
	Reason             string
	AddressesQuestion  bool
	AddressesObjective bool
	NeedsUserInput     bool
	ShouldContinue     bool
	ShouldResume       bool
	ShouldSpeak        bool
	ShouldAct          bool
	ThreadID           string
	ParentTurnID       uint64
	Objective          string
	QuestionType       string
	Language           string
}

// DetermineResponseIntent computes the communicative purpose from semantic
// evidence and durable conversation state. It is the single domain-neutral
// decision point for B96.22.12.
//
// Important: this function does not decide whether a capability exists. A
// production request can therefore return no conversational interception
// intent and continue to the work/capability layers.
func DetermineResponseIntent(r SemanticRequest, ctx ConversationSnapshot, hints ...ResponseIntentHint) ResponseIntent {
	lang := r.Language
	if lang == "" {
		lang = ctx.Language
	}

	intent := ResponseIntent{
		Confidence:   r.Confidence,
		Language:     lang,
		ThreadID:     ctx.ActiveThreadID,
		Objective:    strings.TrimSpace(r.Objective.Text),
		QuestionType: r.QuestionType,
	}

	if intent.Objective == "" {
		intent.Objective = strings.TrimSpace(ctx.ActiveObjective)
	}

	// The communicative-act contract is the only semantic authority for the
	// response decision. Legacy RequestKind/SpeechAct are intentionally not
	// consulted here: keeping a fallback would recreate two competing sources
	// of truth.
	acts := CanonicalActs(r)
	if len(hints) > 0 && hintCanRefineResponse(r, acts, hints[0]) {
		if refined, ok := responseIntentFromHint(intent, hints[0]); ok {
			return refined
		}
	}
	if acts.Has(ActCorrection) {
		intent.Kind = ResponseIntentAcknowledge
		intent.ShouldSpeak = true
		intent.AddressesObjective = ctx.ActiveObjective != ""
		intent.ShouldContinue = true
		intent.Reason = "the user is repairing shared meaning"
		intent.Confidence = maxFloat(intent.Confidence, 0.96)
		return intent
	}
	if acts.Has(ActControl) {
		intent.Kind = ResponseIntentWait
		intent.ShouldSpeak = true
		intent.Reason = "the user changed the interaction state"
		intent.Confidence = maxFloat(intent.Confidence, 0.96)
		return intent
	}

	if linkedAnswer(r, ctx) {
		intent.Kind = ResponseIntentAnswer
		intent.AddressesQuestion = true
		intent.AddressesObjective = ctx.ActiveObjective != ""
		intent.ShouldSpeak = true
		intent.ShouldContinue = true
		intent.ParentTurnID = latestQuestionID(ctx)
		intent.Reason = "the turn answers an open conversational question"
		intent.Confidence = maxFloat(intent.Confidence, 0.90)
		return intent
	}

	switch acts.Primary {
	case ActOpinionRequest:
		intent.Kind = ResponseIntentAsk
		intent.AddressesQuestion = true
		intent.ShouldSpeak = true
		intent.ShouldContinue = true
		intent.Reason = "the user explicitly asks for Ahmelia's opinion"
	case ActQuestion:
		intent.Kind = ResponseIntentAnswer
		intent.AddressesQuestion = true
		intent.ShouldSpeak = true
		intent.ShouldContinue = true
		intent.Reason = "the user asks a question"
	case ActDeclaration, ActObservation, ActInform, ActFeedback, ActConfirmation, ActRejection, ActLearning:
		intent.Kind = ResponseIntentAcknowledge
		intent.ShouldSpeak = true
		intent.ShouldContinue = true
		intent.Reason = "the user contributed information to the shared interaction"
	case ActIdea, ActExploration, ActProposal:
		intent.Kind = ResponseIntentPropose
		intent.ShouldSpeak = true
		intent.NeedsUserInput = r.CommunicativeActs.Has(ActProposal)
		intent.ShouldContinue = true
		intent.Reason = "the user opened an idea, exploration, or proposal"
	case ActTopicShift:
		intent.Kind = ResponseIntentRedirect
		intent.ShouldSpeak = true
		intent.ShouldContinue = true
		intent.Reason = "the user explicitly changed topic"
	case ActContinuation:
		intent.Kind = ResponseIntentContinue
		intent.AddressesObjective = ctx.ActiveObjective != ""
		intent.ShouldContinue = true
		intent.ShouldAct = acts.Has(ActCommand)
		intent.ShouldSpeak = true
		intent.Reason = "the user continues the conversational thread"
	case ActDecision:
		intent.Kind = ResponseIntentAcknowledge
		intent.ShouldSpeak = true
		intent.ShouldContinue = true
		intent.Reason = "the user communicated a decision for shared context"
	case ActSocial:
		intent.Kind = ResponseIntentAcknowledge
		intent.ShouldSpeak = true
		intent.ShouldContinue = true
		intent.Reason = "the user opened a social interaction"
	case ActCommand:
		intent.Kind = ResponseIntentNone
		intent.ShouldAct = true
		intent.Reason = "the turn contains an executable command"
	default:
		if strings.TrimSpace(r.Raw) != "" {
			intent.Kind = ResponseIntentClarify
			intent.ShouldSpeak = true
			intent.NeedsUserInput = true
			intent.Reason = "semantic evidence is insufficient to determine the conversational move"
		}
	}

	if intent.Confidence == 0 {
		intent.Confidence = 0.50
	}
	return intent
}

func hintCanRefineResponse(r SemanticRequest, acts CommunicativeActs, hint ResponseIntentHint) bool {
	if strings.TrimSpace(hint.Kind) == "" || hint.Confidence < 0.60 {
		return false
	}
	// Hints may only repair an insufficient semantic surface. They cannot
	// override a high-confidence structured communicative act.
	return acts.Primary == ActUnknown || r.Confidence < 0.60
}

func responseIntentFromHint(base ResponseIntent, hint ResponseIntentHint) (ResponseIntent, bool) {
	base.Confidence = maxFloat(base.Confidence, hint.Confidence)
	base.Reason = "response intent refined by structured cognitive evidence: " + hint.Reason
	base.ShouldSpeak = true
	base.ShouldContinue = true
	switch strings.ToLower(strings.TrimSpace(hint.Kind)) {
	case "share_idea":
		base.Kind = ResponseIntentPropose
		base.NeedsUserInput = true
		return base, true
	case "feedback":
		base.Kind = ResponseIntentAcknowledge
		return base, true
	case "social":
		base.Kind = ResponseIntentAcknowledge
		return base, true
	default:
		return ResponseIntent{}, false
	}
}

func linkedAnswer(r SemanticRequest, ctx ConversationSnapshot) bool {
	if len(ctx.Turns) == 0 {
		return false
	}
	last := ctx.Turns[len(ctx.Turns)-1]
	lastActs := last.CommunicativeActs
	if lastActs.Primary == "" && strings.TrimSpace(last.UserText) != "" {
		lastActs = CanonicalActs(SemanticRequest{Raw: last.UserText, Normalized: normalize(last.UserText)})
	}
	if !lastActs.Has(ActQuestion) && !strings.HasSuffix(strings.TrimSpace(last.Assistant), "?") {
		return false
	}
	// A new explicit question is a new conversational request, not an answer
	// to the previous question. Without this boundary, consecutive questions
	// can be linked backwards and the previous question's domain can leak into
	// the current one (for example user-preference data into a self question).
	currentActs := CanonicalActs(r)
	if currentActs.Has(ActQuestion) || currentActs.Has(ActOpinionRequest) {
		return false
	}
	if isExplicitNewCommand(r.Raw) {
		return false
	}
	return answerConfidence(r, last) >= 0.70
}

func latestQuestionID(ctx ConversationSnapshot) uint64 {
	for i := len(ctx.Turns) - 1; i >= 0; i-- {
		t := ctx.Turns[i]
		acts := t.CommunicativeActs
		if acts.Primary == "" && strings.TrimSpace(t.UserText) != "" {
			acts = CanonicalActs(SemanticRequest{Raw: t.UserText, Normalized: normalize(t.UserText)})
		}
		if acts.Has(ActQuestion) ||
			strings.HasSuffix(strings.TrimSpace(t.Assistant), "?") {
			return t.ID
		}
	}
	return 0
}
