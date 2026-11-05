package interaction

import (
	"strings"
	"time"
)

// InitiativeCandidate is a proposed conversational move. It is not an action.
type InitiativeCandidate struct {
	Kind      ResponseMove
	Question  string
	Reason    string
	Priority  float64
	Interrupt bool
	CreatedAt time.Time
}

// InitiativeDecision is the policy result for one conversational turn.
// The policy decides whether an initiative is appropriate; it does not
// execute capabilities and does not decide the user's authority.
type InitiativeDecision struct {
	ShouldInitiate bool
	Candidate      InitiativeCandidate
	Reason         string
}

// InitiativePolicy is a legacy compatibility policy kept for callers that
// still use the pre-B96.37 interaction API. It is not an authorization owner
// in the canonical runtime; B96.37.2 owns INTERVENE/WAIT/DEFER. New runtime
// paths must use initiative.Policy directly.
// It deliberately knows nothing about Programming, Blender, Unity, etc.
type InitiativePolicy struct {
	MinTurnsBetweenInitiatives int
	MinOpenConversationTurns   int
}

func DefaultInitiativePolicy() InitiativePolicy {
	return InitiativePolicy{
		MinTurnsBetweenInitiatives: 4,
		MinOpenConversationTurns:   1,
	}
}

// EvaluateInitiative is retained as a compatibility projection for legacy
// interaction callers. The canonical Engine path does not consume its
// ShouldInitiate result for autonomous turn authorization.
// It remains a candidate-oriented compatibility API, not the B96.37.2 owner.
func (p InitiativePolicy) EvaluateInitiative(req SemanticRequest, ctx ConversationSnapshot, profile *UserProfile) InitiativeDecision {
	if p.MinTurnsBetweenInitiatives < 1 {
		p.MinTurnsBetweenInitiatives = 1
	}
	if p.MinOpenConversationTurns < 1 {
		p.MinOpenConversationTurns = 1
	}

	// Initiative must never hijack a turn that has an immediate job to do.
	acts := CanonicalActs(req)
	if acts.Has(ActQuestion) || acts.Has(ActCommand) || acts.Has(ActControl) || acts.Has(ActCorrection) || req.NeedsClarification {
		return InitiativeDecision{Reason: "current turn requires a direct response"}
	}

	// Strong urgency/frustration/confusion is a signal to help, not socialize.
	state := ctx.UserState
	if state.Urgency >= 0.65 || state.Frustration >= 0.65 || state.Confusion >= 0.65 {
		return InitiativeDecision{Reason: "user needs focused assistance"}
	}

	// Do not repeatedly initiate. A recent assistant question is itself a
	// conversational commitment and needs room for the user to answer it.
	recent := len(ctx.Turns)
	if recent > 0 {
		turnsSinceQuestion := recent
		for i := recent - 1; i >= 0; i-- {
			if containsQuestionMark(ctx.Turns[i].Assistant) {
				turnsSinceQuestion = recent - 1 - i
				break
			}
		}
		if turnsSinceQuestion < p.MinTurnsBetweenInitiatives {
			return InitiativeDecision{Reason: "initiative cooldown after recent question"}
		}
	}

	if profile == nil {
		return InitiativeDecision{Reason: "no user model available"}
	}

	// The opportunity is generated from the evolving user model, not from a
	// fixed questionnaire. B96.22.21 will own richer question generation.
	candidate, ok := GenerateQuestion(QuestionGenerationContext{
		Request: SemanticRequest{Raw: "", CommunicativeActs: CommunicativeActs{Primary: ActSocial, Confidence: 0.80}}, Snapshot: ctx, Profile: profile,
	})
	if !ok {
		return InitiativeDecision{Reason: "no useful conversational opportunity found"}
	}
	q := candidate.Question

	priority := 0.45
	if ctx.ActiveTopic != "" {
		priority += 0.10
	}
	if state.Engagement >= 0.70 {
		priority += 0.10
	}

	return InitiativeDecision{
		ShouldInitiate: true,
		Candidate: InitiativeCandidate{
			Kind: MoveAsk, Question: q,
			Reason:   "a useful gap in the user model can be explored without interrupting work",
			Priority: priority, Interrupt: false, CreatedAt: time.Now(),
		},
		Reason: "contextual personal initiative is appropriate",
	}
}

// GenerateInitiative remains a legacy candidate-oriented compatibility API; it cannot authorize an autonomous turn.
// It now delegates policy rather than embedding initiation rules itself.
func GenerateInitiative(req SemanticRequest, ctx ConversationSnapshot, profile *UserProfile) []InitiativeCandidate {
	decision := DefaultInitiativePolicy().EvaluateInitiative(req, ctx, profile)
	if !decision.ShouldInitiate {
		return nil
	}
	return []InitiativeCandidate{decision.Candidate}
}

func containsQuestionMark(s string) bool {
	return strings.ContainsAny(s, "?!¿")
}
