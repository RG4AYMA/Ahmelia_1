package interaction

import (
	"strings"
	"time"
)

// ClarificationStatus tracks whether a pending question is still waiting
// for the user, already answered, or was dropped explicitly (a
// correction/cancel).
type ClarificationStatus string

const (
	ClarificationOpen     ClarificationStatus = "open"
	ClarificationResolved ClarificationStatus = "resolved"
	ClarificationDropped  ClarificationStatus = "dropped"
)

// PendingClarification is a domain-neutral question Ahmelia asked while
// working toward an Objective, still waiting for the user's answer.
//
// This is deliberately a separate concept from memory.PendingClarification,
// the former learning-goal compatibility path. The canonical contract lives at the
// interaction boundary (B96.22) and is consumed without a legacy Dialogue Manager.
// This one lives at the interaction boundary (B96.22) so ANY consumer — ProductionRuntime, an
// action handler, a future capability — can ask something and trust that
// the objective survives no matter what the user does in between, per the
// user's own example:
//
//	"Quiero avanzar esta escena."
//	Ahmelia: "¿Priorizamos la iluminación o la interacción del personaje?"
//	"La interacción."
//	  -> OBJETIVO: avanzar escena / DECISIÓN: prioridad = interacción
type PendingClarification struct {
	// OriginalGoal is preserved verbatim (the raw objective text), the same
	// discipline documented in memory.PendingClarification: merging a
	// stripped-down goal loses information a later stage may need.
	OriginalGoal string
	Question     string
	Assumptions  []string
	AskedAt      time.Time
	Status       ClarificationStatus
	id           uint64
}

// ClarificationQueue holds every PendingClarification still open, oldest
// first. It is a queue on purpose, not a single slot: per an explicit
// product decision, if the user asks for something else while a question
// is pending, Ahmelia must attend the new thing WITHOUT dropping the
// pending question, and resume it — the oldest one first — right after
// finishing the new thing, in the same turn's closing message.
type ClarificationQueue struct {
	items  []PendingClarification
	nextID uint64
}

func NewClarificationQueue() *ClarificationQueue {
	return &ClarificationQueue{}
}

// Push queues a new question tied to originalGoal. assumptions is optional
// context about what's unresolved (mirrors memory.PendingClarification's
// PendingAssumptions) and is used by Answers to recognize a reply that
// echoes one of the offered options. Returns the id of the new entry.
func (q *ClarificationQueue) Push(originalGoal, question string, assumptions []string) uint64 {
	if q == nil {
		return 0
	}
	q.nextID++
	q.items = append(q.items, PendingClarification{
		OriginalGoal: strings.TrimSpace(originalGoal),
		Question:     question,
		Assumptions:  assumptions,
		AskedAt:      time.Now(),
		Status:       ClarificationOpen,
		id:           q.nextID,
	})
	return q.nextID
}

// Oldest returns a copy of the longest-waiting open clarification, or nil
// if none is open. Callers needing to resolve/drop it use its ID.
func (q *ClarificationQueue) Oldest() *PendingClarification {
	if q == nil {
		return nil
	}
	for i := range q.items {
		if q.items[i].Status == ClarificationOpen {
			cp := q.items[i]
			return &cp
		}
	}
	return nil
}

// HasOpen reports whether any clarification is still waiting.
func (q *ClarificationQueue) HasOpen() bool {
	return q.Oldest() != nil
}

// Resolve marks the given entry (by ID) as answered and returns a copy of
// it, so the caller can merge OriginalGoal with the user's answer. Returns
// nil if the id doesn't match an open entry.
func (q *ClarificationQueue) Resolve(id uint64) *PendingClarification {
	return q.settle(id, ClarificationResolved)
}

// Drop marks the given entry as abandoned (e.g. the user corrected/cancelled
// instead of answering) without merging it into anything.
func (q *ClarificationQueue) Drop(id uint64) *PendingClarification {
	return q.settle(id, ClarificationDropped)
}

func (q *ClarificationQueue) settle(id uint64, status ClarificationStatus) *PendingClarification {
	if q == nil || id == 0 {
		return nil
	}
	for i := range q.items {
		if q.items[i].id == id && q.items[i].Status == ClarificationOpen {
			q.items[i].Status = status
			cp := q.items[i]
			return &cp
		}
	}
	return nil
}

// ID exposes the internal identity of a clarification so a caller that got
// it from Oldest() can Resolve/Drop it later without holding a pointer into
// the queue's internal slice.
func (pc *PendingClarification) ID() uint64 {
	if pc == nil {
		return 0
	}
	return pc.id
}

// Answers reports whether raw plausibly answers this pending clarification
// rather than being an unrelated new turn. It stays conservative and
// domain-neutral, matching the product decision: a clear new
// production/action/stop/pause always counts as "something else" (handled
// on its own, this clarification stays open); anything shorter or that
// echoes one of the offered Assumptions counts as an answer.
func (pc *PendingClarification) Answers(kind RequestKind, raw string) bool {
	if pc == nil {
		return false
	}
	switch kind {
	case KindProduction, KindAction, KindStop, KindPause:
		return false
	}
	n := normalizeForClassification(normalize(raw))
	if n == "" {
		return false
	}
	for _, a := range pc.Assumptions {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && strings.Contains(n, a) {
			return true
		}
	}
	// A short reply ("la interacción", "sí", "la primera") reads as a direct
	// answer to a question just asked; a longer, independently-classifiable
	// sentence reads as a new turn.
	return len(strings.Fields(n)) <= 6
}

// AnswersRequest is the canonical semantic API. It uses CommunicativeActs
// and keeps Answers(RequestKind, ...) only as a detached compatibility API.
func (pc *PendingClarification) AnswersRequest(req SemanticRequest) bool {
	if pc == nil {
		return false
	}
	acts := CanonicalActs(req)
	if acts.Has(ActCommand) || acts.Has(ActControl) {
		return false
	}
	n := normalizeForClassification(normalize(req.Raw))
	if n == "" {
		return false
	}
	for _, a := range pc.Assumptions {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && strings.Contains(n, a) {
			return true
		}
	}
	return len(strings.Fields(n)) <= 6
}

// Merge combines the preserved OriginalGoal with the user's answer, using
// the same rule documented for memory.PendingClarification: a bare
// confirmation keeps the goal untouched, anything else appends the answer.
func (pc *PendingClarification) Merge(answer string, isBareConfirmation bool) string {
	if pc == nil {
		return answer
	}
	if pc.OriginalGoal == "" {
		return answer
	}
	if isBareConfirmation {
		return pc.OriginalGoal
	}
	return strings.TrimSpace(pc.OriginalGoal + " " + answer)
}
