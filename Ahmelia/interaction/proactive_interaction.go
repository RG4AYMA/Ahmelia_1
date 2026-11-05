package interaction

import (
	"strings"
	"time"
)

// ProactiveInteraction is the decision envelope used when Ahmelia considers
// initiating communication without being directly asked to do so.
//
// It deliberately stops before language generation or capability execution.
// The envelope records WHY an interaction is useful, WHAT kind of move is
// proposed, and whether the moment is safe to enter. Later layers can replace
// the content provider without changing this boundary.
type ProactiveInteraction struct {
	Allowed   bool
	Move      ResponseMove
	Content   string
	Reason    string
	Priority  float64
	Interrupt bool
	Topic     string
	CreatedAt time.Time
}

// ProactiveInteractionPolicy is the B96.22.20 boundary between initiative
// detection and actual proactive communication. B96.22.19 decides whether
// Ahmelia has a conversational opportunity; this layer decides whether that
// opportunity is eligible to become an interaction in the current context.
type ProactiveInteractionPolicy struct {
	MinPriority            float64
	RequireNonInterruptive bool
}

// DefaultProactiveInteractionPolicy returns conservative defaults. Proactivity
// must earn its way into the conversation instead of competing with the user.
func DefaultProactiveInteractionPolicy() ProactiveInteractionPolicy {
	return ProactiveInteractionPolicy{
		MinPriority:            0.45,
		RequireNonInterruptive: true,
	}
}

// EvaluateProactiveInteraction converts a conversational initiative into a
// domain-neutral proactive interaction.
//
// The function intentionally does not generate questions or proposals. It
// validates timing and interaction boundaries, while B96.22.21/.22 own the
// creation of those contents.
func (p ProactiveInteractionPolicy) EvaluateProactiveInteraction(
	initiative InitiativeDecision,
	ctx ConversationSnapshot,
) ProactiveInteraction {
	if !initiative.ShouldInitiate {
		return ProactiveInteraction{Reason: initiative.Reason}
	}

	c := initiative.Candidate
	if !ResponseMoveIsValid(c.Kind) {
		return ProactiveInteraction{Reason: "initiative contains an invalid response move"}
	}
	if strings.TrimSpace(c.Question) == "" {
		return ProactiveInteraction{Reason: "initiative has no communicative content"}
	}
	if p.RequireNonInterruptive && c.Interrupt {
		return ProactiveInteraction{Reason: "proactive interaction may not interrupt the user"}
	}
	if c.Priority < p.MinPriority {
		return ProactiveInteraction{Reason: "proactive opportunity is below the interaction priority threshold"}
	}

	// A newly detected need always outranks social initiative. B96.22.19
	// normally filters these states already; keeping the boundary here makes
	// this layer safe when called independently by a future UI/voice loop.
	state := ctx.UserState
	if state.Urgency >= 0.65 || state.Frustration >= 0.65 || state.Confusion >= 0.65 {
		return ProactiveInteraction{Reason: "user state requires focused assistance"}
	}

	return ProactiveInteraction{
		Allowed:   true,
		Move:      c.Kind,
		Content:   strings.TrimSpace(c.Question),
		Reason:    c.Reason,
		Priority:  c.Priority,
		Interrupt: c.Interrupt,
		Topic:     strings.TrimSpace(ctx.ActiveTopic),
		CreatedAt: c.CreatedAt,
	}
}

// ShouldSpeakNow is a small, reusable timing guard for future voice/UI loops.
// It answers only whether an already-approved proactive interaction can enter
// the user-facing channel right now. It does not decide what to say.
func (p ProactiveInteractionPolicy) ShouldSpeakNow(interaction ProactiveInteraction, now time.Time) bool {
	if !interaction.Allowed || strings.TrimSpace(interaction.Content) == "" {
		return false
	}
	if p.RequireNonInterruptive && interaction.Interrupt {
		return false
	}
	if now.IsZero() || interaction.CreatedAt.IsZero() {
		return true
	}
	// An interaction candidate that has been waiting for an unusually long
	// time should be regenerated from fresh context instead of being replayed
	// blindly after the conversation has moved on.
	return now.Sub(interaction.CreatedAt) <= 2*time.Minute
}
