package interaction

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// B96.34.13 — User Model Conflict Resolution.
//
// ¿Qué ocurre si hay contradicciones?
//
//	2026: "Prefiero respuestas cortas."
//	2027: "Quiero explicaciones mucho más profundas."
//
// No deberíamos tener:
//
//	delete old
//	write new
//
// Podríamos tener:
//
//	Preference
//	context-dependent
//
//	technical_work:
//	    deep
//
//	casual:
//	    short
//
// B96.34.11 already refuses to overwrite blindly: when a proposal disagrees
// with the existing live belief at a key and does not outrank it, both stay
// live as UserModelUpdateHoldForReconciliation. What B96.34.11 explicitly
// does not do is decide *why* two live beliefs disagree — a single true
// contradiction (one of them is simply wrong) and a context-dependent split
// (both are correct, each within its own scope) look identical at that
// point: two live items, same key, different values. This block is the only
// place that tells them apart, and it does so without inventing context on
// its own: the Context on each UserModelConflictItem is supplied by the
// caller (ultimately B96.33's cognition pipeline, the same way
// UserModelQueryContext.Topics in B96.34.12 arrives pre-decided), never
// guessed here from a statement's text.
//
// Nothing is ever deleted. A belief this block does not keep live is marked
// Superseded, exactly as B96.34.11 already promises, so the B96.34.10 audit
// trail stays intact. Where the evidence genuinely cannot decide a winner —
// same context, same priority tier, confidence within margin — this block
// does not force a guess either: both stay live and Uncertain, consistent
// with B96.34.14 — Ahmelia must be able to represent "no sé" as much as
// "sé".

// UserModelConflictItem wraps one B96.34.11 canonical update item with the
// scope it was asserted under. Context is opaque to this block: it is
// whatever label the caller supplies (e.g. "technical_work", "casual"), or
// the empty string, meaning "asserted with no explicit scope, applies
// generally". This block never derives Context from Value, Provenance or
// any other field — doing so would be exactly the invented inference
// B96.34.2 already warns against.
type UserModelConflictItem struct {
	UserModelUpdateItem
	Context string
}

// UserModelConflictKind names what B96.34.13 concluded about one key's live
// disagreement.
type UserModelConflictKind string

const (
	// UserModelConflictGenuineContradiction: every candidate shares one
	// context (including the case where none of them specify one), so
	// there is no scope that could make both true. Exactly one winner is
	// kept Active/Uncertain (as it already was); every other candidate is
	// marked Superseded.
	UserModelConflictGenuineContradiction UserModelConflictKind = "genuine_contradiction"
	// UserModelConflictContextSplit: candidates span more than one
	// context. Candidates in different contexts never compete with one
	// another — each context keeps its own winner. A tie can still occur
	// within a single context group; see PartiallyUnresolved.
	UserModelConflictContextSplit UserModelConflictKind = "context_split"
	// UserModelConflictUnresolved: the (single-context) candidates are
	// evenly matched — same source-priority tier and confidence within
	// the policy's margin — so there is no honest winner. Every candidate
	// stays live, downgraded to Uncertain if it was Active, and none is
	// superseded.
	UserModelConflictUnresolved UserModelConflictKind = "unresolved"
)

// UserModelConflictGroupOutcome is the per-context result folded into
// UserModelConflictOutcome. Most callers only need the aggregate; this is
// exposed so a caller that cares which specific context stayed ambiguous
// can find out.
type UserModelConflictGroupOutcome struct {
	Context    string
	Resolved   bool
	Winner     *UserModelConflictItem
	Superseded []UserModelConflictItem
	Live       []UserModelConflictItem // all candidates in this context that remain live; equals []{*Winner} when Resolved
}

// UserModelConflictOutcome documents what B96.34.13 decided across every
// context group for one key, for the B96.34.10 audit trail.
type UserModelConflictOutcome struct {
	Key                 string
	Kind                UserModelConflictKind
	Groups              []UserModelConflictGroupOutcome
	Resolved            []UserModelConflictItem // one entry per group: the group's winner if resolved, else every live candidate in that group
	Superseded          []UserModelConflictItem
	PartiallyUnresolved bool // true when Kind == ContextSplit but at least one group's own tie was not resolvable
	Reason              string
}

// UserModelConflictPolicy is intentionally the only tunable in this block,
// mirroring B96.34.11's own single-knob policy — the two are meant to agree
// unless a caller has a deliberate reason to diverge.
type UserModelConflictPolicy struct {
	// ContradictionConfidenceMargin is how much higher a same-priority
	// candidate's confidence must be over the runner-up to win outright
	// within one context. Below this margin, the group is Unresolved.
	ContradictionConfidenceMargin float64
}

func DefaultUserModelConflictPolicy() UserModelConflictPolicy {
	return UserModelConflictPolicy{ContradictionConfidenceMargin: 0.20}
}

// ValidateUserModelConflictItem checks structural invariants only, reusing
// B96.34.11's own item validation. Context has no required shape — an empty
// string is a legal, meaningful value ("no explicit scope").
func ValidateUserModelConflictItem(item UserModelConflictItem) error {
	return ValidateUserModelUpdateItem(item.UserModelUpdateItem)
}

func normalizeConflictContext(ctx string) string {
	return strings.ToLower(strings.TrimSpace(ctx))
}

// FindLiveConflicts groups items (typically one domain's accumulated
// UserModelUpdateItem slice, after zero or more B96.34.11 calls) by key and
// returns, for every key with more than one live (Active or Uncertain)
// candidate whose values genuinely disagree, that key's live candidates.
// Keys with no live disagreement — including keys where B96.34.11 already
// reinforced every proposal into a single item — are omitted. It only
// detects; it decides nothing, matching B96.34.10's DetectValueContradiction
// precedent for pairwise checks, generalized to whole slices.
func FindLiveConflicts(items []UserModelUpdateItem) map[string][]UserModelUpdateItem {
	byKey := make(map[string][]UserModelUpdateItem)
	for _, it := range items {
		if !isLiveUserFactStatus(it.Status) {
			continue
		}
		byKey[it.Key] = append(byKey[it.Key], it)
	}
	out := make(map[string][]UserModelUpdateItem)
	for key, group := range byKey {
		if len(group) < 2 {
			continue
		}
		if hasDisagreement(group) {
			out[key] = group
		}
	}
	return out
}

func hasDisagreement(items []UserModelUpdateItem) bool {
	if len(items) < 2 {
		return false
	}
	first := strings.TrimSpace(items[0].Value)
	for _, it := range items[1:] {
		if !strings.EqualFold(first, strings.TrimSpace(it.Value)) {
			return true
		}
	}
	return false
}

// ResolveUserModelConflict is the single entry point B96.34.13 offers. It
// takes every live candidate held at one key — each already annotated with
// its Context by the caller — and returns which ones remain live, which are
// now Superseded, and why. now stamps every Superseded item's UpdatedAt, so
// resolution timing is explicit rather than inferred from the system clock
// mid-computation.
//
// candidates is never mutated: every returned item is a copy.
func ResolveUserModelConflict(candidates []UserModelConflictItem, policy UserModelConflictPolicy, now time.Time) (UserModelConflictOutcome, error) {
	if len(candidates) < 2 {
		return UserModelConflictOutcome{}, errors.New("user model conflict: at least two candidates are required")
	}
	if policy.ContradictionConfidenceMargin < 0 || policy.ContradictionConfidenceMargin > 1 {
		return UserModelConflictOutcome{}, errors.New("user model conflict: contradiction confidence margin must be between 0 and 1")
	}
	key := candidates[0].Key
	for i, c := range candidates {
		if err := ValidateUserModelConflictItem(c); err != nil {
			return UserModelConflictOutcome{}, fmt.Errorf("user model conflict: candidate %d: %w", i, err)
		}
		if !isLiveUserFactStatus(c.Status) {
			return UserModelConflictOutcome{}, fmt.Errorf("user model conflict: candidate %d (%q) is not a live belief", i, c.Key)
		}
		if c.Key != key {
			return UserModelConflictOutcome{}, errors.New("user model conflict: all candidates must share the same key")
		}
	}

	groupsByContext := make(map[string][]UserModelConflictItem)
	var contextOrder []string
	for _, c := range candidates {
		norm := normalizeConflictContext(c.Context)
		if _, seen := groupsByContext[norm]; !seen {
			contextOrder = append(contextOrder, norm)
		}
		groupsByContext[norm] = append(groupsByContext[norm], c)
	}
	sort.Strings(contextOrder)
	isContextSplit := len(contextOrder) > 1

	var groups []UserModelConflictGroupOutcome
	var resolved []UserModelConflictItem
	var superseded []UserModelConflictItem
	anyGroupUnresolved := false

	for _, ctxKey := range contextOrder {
		group := groupsByContext[ctxKey]
		displayContext := group[0].Context
		groupOutcome := resolveConflictGroup(displayContext, group, policy, now, isContextSplit)
		groups = append(groups, groupOutcome)
		if groupOutcome.Resolved {
			resolved = append(resolved, *groupOutcome.Winner)
		} else {
			anyGroupUnresolved = true
			resolved = append(resolved, groupOutcome.Live...)
		}
		superseded = append(superseded, groupOutcome.Superseded...)
	}

	outcome := UserModelConflictOutcome{
		Key:        key,
		Groups:     groups,
		Resolved:   resolved,
		Superseded: superseded,
	}

	switch {
	case len(groups) == 1 && !anyGroupUnresolved:
		outcome.Kind = UserModelConflictGenuineContradiction
		outcome.Reason = "every candidate shares one context; the higher-ranked belief supersedes the rest"
	case len(groups) == 1 && anyGroupUnresolved:
		outcome.Kind = UserModelConflictUnresolved
		outcome.Reason = "every candidate shares one context and none outranks the others by the policy margin; both remain live and uncertain"
	default:
		outcome.Kind = UserModelConflictContextSplit
		outcome.PartiallyUnresolved = anyGroupUnresolved
		if anyGroupUnresolved {
			outcome.Reason = "candidates span more than one context, so they are not a true contradiction; at least one context's own candidates remain tied"
		} else {
			outcome.Reason = "candidates span more than one context, so each context keeps its own belief; neither supersedes the other"
		}
	}

	return outcome, nil
}

// resolveConflictGroup runs the same source-priority-then-confidence-margin
// ranking B96.34.11 already uses for its two-way comparison, generalized to
// however many candidates share this context. Only the top-ranked and
// runner-up need to be compared: priority tiers and confidence are both
// total orders, so if the top candidate clears the runner-up by the policy
// margin, it clears every candidate ranked below the runner-up too.
func resolveConflictGroup(context string, group []UserModelConflictItem, policy UserModelConflictPolicy, now time.Time, promoteOnResolve bool) UserModelConflictGroupOutcome {
	if !hasConflictItemDisagreement(group) {
		// Every candidate in this context already agrees on Value (which
		// includes the common case of a single candidate): there is no
		// contradiction to resolve here, only redundant live copies or a
		// belief B96.34.11 held Uncertain purely because a *different*
		// context's proposal looked, at the time, like a contradiction.
		// Keep the most recently updated one live; older duplicates are
		// settled history, so they are superseded rather than left live
		// and reported as ambiguous.
		winnerIdx := 0
		for i, c := range group {
			if c.UpdatedAt.After(group[winnerIdx].UpdatedAt) {
				winnerIdx = i
			}
		}
		winner := group[winnerIdx]
		if promoteOnResolve {
			// This context turned out not to be a real contradiction at
			// all — the other side of the disagreement lives in a
			// different context. Any Uncertain status B96.34.11 assigned
			// for that reason no longer applies; promote back to Active,
			// still subject to the same single-weak-observation guard
			// every domain already enforces.
			winner.UserModelUpdateItem = promoteResolvedWinner(winner.UserModelUpdateItem)
		}
		var superseded []UserModelConflictItem
		for i, c := range group {
			if i == winnerIdx {
				continue
			}
			s := c
			s.Status = UserModelStatusSuperseded
			s.UpdatedAt = now
			superseded = append(superseded, s)
		}
		return UserModelConflictGroupOutcome{
			Context: context, Resolved: true, Winner: &winner, Superseded: superseded,
			Live: []UserModelConflictItem{winner},
		}
	}

	ranked := append([]UserModelConflictItem(nil), group...)
	sort.SliceStable(ranked, func(i, j int) bool {
		pi, pj := sourcePriority(ranked[i].Provenance.Kind), sourcePriority(ranked[j].Provenance.Kind)
		if pi != pj {
			return pi > pj
		}
		return ranked[i].Confidence > ranked[j].Confidence
	})

	top, runnerUp := ranked[0], ranked[1]
	topPriority := sourcePriority(top.Provenance.Kind)
	runnerUpPriority := sourcePriority(runnerUp.Provenance.Kind)
	decisive := topPriority > runnerUpPriority ||
		(topPriority == runnerUpPriority && top.Confidence-runnerUp.Confidence >= policy.ContradictionConfidenceMargin)

	if !decisive {
		var live []UserModelConflictItem
		for _, c := range group {
			c.UserModelUpdateItem = guardSingleObservationPromotion(c.UserModelUpdateItem)
			if c.Status == UserModelStatusActive {
				c.Status = UserModelStatusUncertain
			}
			live = append(live, c)
		}
		return UserModelConflictGroupOutcome{Context: context, Resolved: false, Live: live}
	}

	winner := top
	winner.UserModelUpdateItem = guardSingleObservationPromotion(winner.UserModelUpdateItem)
	var superseded []UserModelConflictItem
	for _, c := range ranked[1:] {
		s := c
		s.Status = UserModelStatusSuperseded
		s.UpdatedAt = now
		superseded = append(superseded, s)
	}
	return UserModelConflictGroupOutcome{
		Context: context, Resolved: true, Winner: &winner, Superseded: superseded,
		Live: []UserModelConflictItem{winner},
	}
}

// promoteResolvedWinner lifts an Uncertain belief back to Active once a
// context split has explained away the disagreement that caused B96.34.11
// to hold it Uncertain in the first place. It never touches an already
// Active, Superseded or Rejected item, and it still runs through
// guardSingleObservationPromotion — a genuinely single, weak observation
// must stay Uncertain regardless of why the contradiction was resolved.
func promoteResolvedWinner(item UserModelUpdateItem) UserModelUpdateItem {
	if item.Status != UserModelStatusUncertain {
		return item
	}
	candidate := item
	candidate.Status = UserModelStatusActive
	return guardSingleObservationPromotion(candidate)
}

func hasConflictItemDisagreement(items []UserModelConflictItem) bool {
	if len(items) < 2 {
		return false
	}
	first := strings.TrimSpace(items[0].Value)
	for _, it := range items[1:] {
		if !strings.EqualFold(first, strings.TrimSpace(it.Value)) {
			return true
		}
	}
	return false
}
