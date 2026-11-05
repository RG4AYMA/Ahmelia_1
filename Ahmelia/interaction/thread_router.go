package interaction

import (
	"strings"
	"time"
)

// ThreadRelation describes how a turn moves through the conversation's
// semantic lanes. It is intentionally independent from work/capabilities.
type ThreadRelation string

const (
	ThreadKeep    ThreadRelation = "keep"
	ThreadSwitch  ThreadRelation = "switch"
	ThreadResume  ThreadRelation = "resume"
	ThreadCreate  ThreadRelation = "create"
	ThreadUnknown ThreadRelation = "unknown"
)

// ThreadDecision is the result of selecting the conversational lane for a
// turn. It never decides what capability should execute.
type ThreadDecision struct {
	Relation   ThreadRelation
	Thread     ConversationThread
	PreviousID string
	Confidence float64
	Reason     string
}

// ThreadRouter maintains the distinction between "new subject" and
// "continuation of the subject". It uses explicit discourse evidence first
// and only creates a lane when the semantic topic gives it something stable
// to attach to.
type ThreadRouter struct {
}

func NewThreadRouter() *ThreadRouter { return &ThreadRouter{} }

// Route selects or creates a semantic conversation lane.
//
// Rules:
//   - explicit resume language wins;
//   - an explicit topic switch preserves the previous lane and creates a new
//     one when a topic is available;
//   - a turn with no topic stays on the active lane;
//   - a new concrete topic that is not a contextual continuation becomes a
//     new lane;
//   - short answers/corrections/continuations stay on the current lane.
func (r *ThreadRouter) Route(state *ConversationState, req SemanticRequest) ThreadDecision {
	if state == nil {
		return ThreadDecision{Relation: ThreadUnknown, Confidence: 0.2, Reason: "conversation state unavailable"}
	}

	active, hasActive := state.ActiveThread()
	topic := strings.TrimSpace(req.Topic)
	previousID := ""
	if hasActive {
		previousID = active.ID
	}

	normalized := strings.ToLower(strings.TrimSpace(req.Normalized))

	if req.BreaksContinuity {
		if topic == "" {
			state.ClearActiveContext()
			return ThreadDecision{Relation: ThreadSwitch, PreviousID: previousID, Confidence: 0.99, Reason: "semantic context boundary; previous lane deactivated"}
		}
		thread := state.EnsureThread(topic, req.Objective.Text)
		relation := ThreadCreate
		if previousID != "" && thread.ID != previousID {
			relation = ThreadSwitch
		}
		return ThreadDecision{Relation: relation, Thread: thread, PreviousID: previousID, Confidence: 0.99, Reason: "semantic context boundary with explicit new topic"}
	}

	if target, ok := explicitResumeTarget(normalized); ok {
		if thread, found := state.FindThread(target); found {
			state.ActivateThread(thread.ID)
			return ThreadDecision{
				Relation: ThreadResume, Thread: thread, PreviousID: previousID,
				Confidence: 0.97, Reason: "explicit request to resume an existing conversation thread",
			}
		}
		// The user explicitly asked to resume something that has no known
		// lane. Keep that semantic intent visible by creating one rather than
		// pretending the reference was resolved.
		thread := state.EnsureThread(target, req.Objective.Text)
		return ThreadDecision{
			Relation: ThreadCreate, Thread: thread, PreviousID: previousID,
			Confidence: 0.74, Reason: "explicit resume target was not in open threads; created a new semantic lane",
		}
	}

	if explicitTopicSwitch(normalized) {
		if topic == "" {
			state.ClearActiveContext()
			return ThreadDecision{Relation: ThreadSwitch, PreviousID: previousID, Confidence: 0.99, Reason: "explicit topic switch without a target; previous context deactivated"}
		}
		thread := state.EnsureThread(topic, req.Objective.Text)
		relation := ThreadCreate
		if previousID != "" && thread.ID != previousID {
			relation = ThreadSwitch
		}
		return ThreadDecision{
			Relation: relation, Thread: thread, PreviousID: previousID,
			Confidence: 0.96, Reason: "explicit topic-switch language with a concrete new topic",
		}
	}

	// Contextual answers, corrections and continuations belong to the active
	// lane even when their own text contains no topic.
	if hasActive && (req.FollowUp || req.Continues ||
		CanonicalActs(req).Has(ActCorrection) || req.NeedsClarification ||
		CanonicalActs(req).Has(ActConfirmation) || CanonicalActs(req).Has(ActRejection)) {
		state.ActivateThread(active.ID)
		return ThreadDecision{
			Relation: ThreadKeep, Thread: active, PreviousID: previousID,
			Confidence: 0.95, Reason: "turn is context-dependent and therefore remains on the active lane",
		}
	}

	if topic == "" {
		if hasActive {
			state.ActivateThread(active.ID)
			return ThreadDecision{
				Relation: ThreadKeep, Thread: active, PreviousID: previousID,
				Confidence: 0.90, Reason: "no new topic was expressed; preserve current conversational focus",
			}
		}
		return ThreadDecision{Relation: ThreadUnknown, PreviousID: previousID, Confidence: 0.55,
			Reason: "no active lane and no stable topic"}
	}

	if existing, found := state.FindThread(topic); found {
		state.ActivateThread(existing.ID)
		relation := ThreadKeep
		if previousID != "" && existing.ID != previousID {
			relation = ThreadResume
		}
		return ThreadDecision{
			Relation: relation, Thread: existing, PreviousID: previousID,
			Confidence: 0.91, Reason: "topic matches an existing conversational lane",
		}
	}

	if hasActive && threadTopicsMatch(active.Topic, topic) {
		state.ActivateThread(active.ID)
		return ThreadDecision{Relation: ThreadKeep, Thread: active, PreviousID: previousID,
			Confidence: 0.94, Reason: "new wording refers to the same active topic"}
	}

	// A topic-bearing question is not automatically a topic switch. Natural
	// conversation frequently asks about a related subject. Only create a
	// separate lane when there is enough evidence that the subject is distinct.
	if hasActive && (CanonicalActs(req).Has(ActQuestion) || CanonicalActs(req).Has(ActSocial) || CanonicalActs(req).Has(ActInform)) {
		return ThreadDecision{
			Relation: ThreadKeep, Thread: active, PreviousID: previousID,
			Confidence: 0.67, Reason: "topic evidence is insufficient to declare a new conversational lane",
		}
	}

	thread := state.EnsureThread(topic, req.Objective.Text)
	return ThreadDecision{
		Relation: ThreadCreate, Thread: thread, PreviousID: previousID,
		Confidence: 0.82, Reason: "concrete topic differs from active lane",
	}
}

func threadTopicsMatch(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	return a != "" && (a == b || strings.Contains(a, b) || strings.Contains(b, a))
}

func explicitTopicSwitch(n string) bool {
	for _, marker := range []string{
		"cambiemos de tema", "cambiando de tema", "hablemos de otra cosa",
		"dejemos eso por ahora", "por otro lado", "ahora hablemos de",
		"cambiando al tema de", "switch topics", "let's change the subject",
		"on another topic", "speaking of something else",
	} {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

func explicitResumeTarget(n string) (string, bool) {
	for _, marker := range []string{
		"volvamos a ", "retomemos ", "sigamos con ", "continuemos con ",
		"back to ", "let's go back to ", "resume ",
	} {
		if idx := strings.Index(n, marker); idx >= 0 {
			target := strings.TrimSpace(n[idx+len(marker):])
			target = strings.Trim(target, " .!?¿¡")
			if target != "" {
				return target, true
			}
		}
	}
	return "", false
}

// ThreadAge is useful to future interaction policy without embedding a clock
// into every caller.
func ThreadAge(t ConversationThread) time.Duration {
	if t.UpdatedAt.IsZero() {
		return 0
	}
	return time.Since(t.UpdatedAt)
}
