package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// B96.34.11 — User Model Update Policy.
//
// Every prior B96.34.x domain block (attributes, preferences, goals,
// capabilities, knowledge, interaction patterns, communication profile)
// already decides, independently, whether one proposal is even a candidate:
// EvaluateUser*Update / ReinforceUserInteractionPattern gate Confidence,
// Status and evidence count before anything reaches this file. What none of
// them do — by design, each says so explicitly — is decide what happens when
// an *accepted* candidate meets whatever is already durably recorded at the
// same key. That is this block's only job:
//
//	Explicit statement
//	       ↓
//	HIGH confidence
//	       ↓
//	Can update
//
//	Single behavioral observation
//	       ↓
//	LOW confidence
//	       ↓
//	Do not establish permanent trait
//
//	Repeated evidence
//	       ↓
//	Pattern detected
//	       ↓
//	Increase confidence
//
//	Contradiction
//	       ↓
//	Do not overwrite blindly
//	       ↓
//	Reconcile
//
// This file does not introduce a new belief domain and does not decide the
// semantic split of a context-dependent contradiction (a preference that is
// genuinely different in two contexts, e.g. "concise at work" vs. "detailed
// casually") — that richer modeling is B96.34.13. What it guarantees is the
// mechanical half of "reconcile": a contradiction never silently replaces a
// more-trusted belief, and nothing is ever deleted — a losing belief is
// marked Superseded, never removed, so the audit trail B96.34.10 promises
// stays intact.

// UserModelUpdateItem is the minimal cross-domain shape this block
// reconciles. Every domain's *UpdateItem adapter below projects its own
// record into this shape so the same gating and reconciliation logic
// applies uniformly, instead of five near-identical copies of it living in
// five domain files.
type UserModelUpdateItem struct {
	Key           string
	Value         string
	Confidence    float64
	EvidenceCount int
	Provenance    UserModelSourceRef
	Status        UserModelItemStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// AttributeUpdateItem, PreferenceUpdateItem, GoalUpdateItem,
// CapabilityUpdateItem, KnowledgeUpdateItem, InteractionPatternUpdateItem and
// CommunicationProfileUpdateItem each adapt one domain's already-accepted
// record into the canonical shape. None of them re-judges the domain's own
// admission decision; they only relabel its fields so B96.34.11 can compare
// across domains.
func AttributeUpdateItem(r UserAttributeRecord) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: r.Key, Value: r.Value, Confidence: r.Confidence,
		EvidenceCount: len(r.Evidence), Provenance: r.Source, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func PreferenceUpdateItem(r UserPreferenceRecord) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: r.Key, Value: r.Value, Confidence: r.Confidence,
		EvidenceCount: len(r.Evidence), Provenance: r.Source, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func GoalUpdateItem(r UserGoalRecord) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: r.Goal, Value: string(r.Category), Confidence: r.Confidence,
		EvidenceCount: len(r.Evidence), Provenance: r.Source, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func CapabilityUpdateItem(r UserCapabilityRecord) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: r.Capability, Value: string(r.Level), Confidence: r.Confidence,
		EvidenceCount: len(r.Evidence), Provenance: r.Source, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func KnowledgeUpdateItem(r UserKnowledgeRecord) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: r.Topic, Value: string(r.Level), Confidence: r.Confidence,
		EvidenceCount: len(r.Evidence), Provenance: r.Source, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// InteractionPatternUpdateItem is included for uniformity, but in practice
// rarely reaches the contradiction branch below: ReinforceUserInteractionPattern
// (B96.34.8) already folds every observation for a given pattern name into a
// single running estimate before this block ever sees it.
func InteractionPatternUpdateItem(r UserInteractionPatternRecord) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: r.Pattern, Value: r.Pattern, Confidence: r.Confidence,
		EvidenceCount: r.EvidenceCount, Provenance: r.Source, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func CommunicationProfileUpdateItem(r UserCommunicationProfileValue) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: string(r.Dimension), Value: r.Value, Confidence: r.Confidence,
		EvidenceCount: len(r.Evidence), Provenance: r.Source, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// UserModelUpdateAction names what B96.34.11 decided to do with a proposal.
type UserModelUpdateAction string

const (
	// UserModelUpdateEstablish: no live belief existed at this key.
	UserModelUpdateEstablish UserModelUpdateAction = "establish"
	// UserModelUpdateReinforce: the proposal agrees with the existing live
	// belief; confidence moves as a running estimate, it does not just reset.
	UserModelUpdateReinforce UserModelUpdateAction = "reinforce"
	// UserModelUpdateSupersede: the proposal disagrees with the existing live
	// belief and outranks it; the old belief is marked Superseded, not deleted.
	UserModelUpdateSupersede UserModelUpdateAction = "supersede"
	// UserModelUpdateHoldForReconciliation: the proposal disagrees and does
	// not outrank the existing belief. Neither is discarded; both stay live.
	// B96.34.13 owns deciding whether this is really a single true
	// contradiction or a context-dependent split.
	UserModelUpdateHoldForReconciliation UserModelUpdateAction = "hold_for_reconciliation"
)

// UserModelUpdateOutcome documents what happened, for the audit trail
// B96.34.10 expects every model change to leave behind.
type UserModelUpdateOutcome struct {
	Action     UserModelUpdateAction
	Applied    UserModelUpdateItem
	Superseded *UserModelUpdateItem
	Reason     string
}

// UserModelUpdatePolicy is intentionally the only tunable in this block.
type UserModelUpdatePolicy struct {
	// ContradictionConfidenceMargin is how much higher a same-priority
	// proposal's confidence must be over the existing live belief's
	// confidence to supersede it outright. Below this margin, "do not
	// overwrite blindly" wins and both beliefs stay live.
	ContradictionConfidenceMargin float64
}

func DefaultUserModelUpdatePolicy() UserModelUpdatePolicy {
	return UserModelUpdatePolicy{ContradictionConfidenceMargin: 0.20}
}

// sourcePriority ranks provenance kinds for contradiction resolution only.
// It never appears in any domain's own admission policy (B96.34.2..34.8);
// it exists solely to answer "which of two disagreeing live beliefs is more
// trusted", consistent with every domain already trusting
// UserSourceExplicitUser above every other source.
func sourcePriority(kind UserModelSourceKind) int {
	switch kind {
	case UserSourceExplicitUser:
		return 4
	case UserSourceSystem, UserSourceImported:
		return 3
	case UserSourceObserved:
		return 2
	case UserSourceInferred:
		return 1
	default:
		return 0
	}
}

// guardSingleObservationPromotion is the final, domain-agnostic backstop for
// "a single behavioral observation must not establish a permanent trait". It
// mirrors B96.34.9's RejectPersistentTraitFromSingleState, generalized to
// every domain rather than only state-derived proposals: every current
// domain policy already requires MinEvidenceCount >= 2 before granting
// Active to an Observed/Inferred proposal, so this rarely fires today — it
// exists so the invariant holds even if a future or misconfigured domain
// policy does not enforce it.
func guardSingleObservationPromotion(item UserModelUpdateItem) UserModelUpdateItem {
	if item.Status == UserModelStatusActive && item.EvidenceCount <= 1 &&
		(item.Provenance.Kind == UserSourceObserved || item.Provenance.Kind == UserSourceInferred) {
		item.Status = UserModelStatusUncertain
	}
	return item
}

func normalizeBeliefValue(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.Join(strings.Fields(v), " ")
	v = strings.Trim(v, ".!?¿¡,;:")
	return strings.TrimSpace(v)
}

// valuesAgree accepts exact agreement and the specific safe case where one
// value is the clean semantic prefix of the other and the remainder is a
// recognized conversational/discourse tail. It must not turn arbitrary
// substring overlap into agreement ("rojo" is not agreement with
// "rojo y azul").
func valuesAgree(a, b string) bool {
	a = normalizeBeliefValue(a)
	b = normalizeBeliefValue(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return cleanBeliefVariant(a, b) || cleanBeliefVariant(b, a)
}

func cleanBeliefVariant(shorter, longer string) bool {
	if len(shorter) >= len(longer) || !strings.HasPrefix(longer, shorter) {
		return false
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(longer, shorter))
	for _, marker := range []string{
		"ya te lo había dicho", "ya te lo habia dicho",
		"como te dije", "como siempre", "de nuevo", "otra vez",
		"porque", "aunque", "pero",
	} {
		if strings.HasPrefix(remainder, marker) {
			return true
		}
	}
	return false
}

// findLiveIndex returns the index of the most recently updated live (Active
// or Uncertain) item at key, if any. Superseded and Rejected items are
// history, never a target for reinforcement or contradiction.
func findLiveIndex(items []UserModelUpdateItem, key string) (int, bool) {
	idx := -1
	var latest time.Time
	for i, it := range items {
		if it.Key != key {
			continue
		}
		if it.Status != UserModelStatusActive && it.Status != UserModelStatusUncertain {
			continue
		}
		if idx == -1 || it.UpdatedAt.After(latest) {
			idx = i
			latest = it.UpdatedAt
		}
	}
	return idx, idx != -1
}

// reinforceUserModelUpdateItem folds a newly agreeing proposal into the
// prior live belief at the same key. Confidence moves as a running average
// weighted by accumulated evidence — "repeated evidence -> increase
// confidence", not "latest observation replaces confidence outright" —
// consistent with how B96.34.8 already accumulates pattern confidence.
func reinforceUserModelUpdateItem(prior, proposal UserModelUpdateItem) UserModelUpdateItem {
	priorWeight := prior.EvidenceCount
	if priorWeight < 1 {
		priorWeight = 1
	}
	proposalWeight := proposal.EvidenceCount
	if proposalWeight < 1 {
		proposalWeight = 1
	}
	totalWeight := priorWeight + proposalWeight

	reinforced := prior
	reinforced.Confidence = (prior.Confidence*float64(priorWeight) + proposal.Confidence*float64(proposalWeight)) / float64(totalWeight)
	reinforced.EvidenceCount = prior.EvidenceCount + proposal.EvidenceCount
	reinforced.Provenance = proposal.Provenance
	reinforced.UpdatedAt = proposal.UpdatedAt
	if prior.Status == UserModelStatusActive || proposal.Status == UserModelStatusActive {
		reinforced.Status = UserModelStatusActive
	} else {
		reinforced.Status = UserModelStatusUncertain
	}
	return reinforced
}

// ReconcileUserModelUpdate is the single entry point B96.34.11 offers. It
// takes whatever is already durably recorded for one key (existing, from any
// one domain's slice, each item produced by that domain's own *UpdateItem
// adapter) and one new accepted proposal for the same key, and returns the
// updated slice plus an outcome record explaining what happened.
//
// existing is never mutated: the returned slice is a new copy. Nothing is
// ever removed from it — a losing belief is marked Superseded in place, and
// a held contradiction is appended alongside the belief it did not outrank.
func ReconcileUserModelUpdate(existing []UserModelUpdateItem, proposal UserModelUpdateItem, policy UserModelUpdatePolicy) ([]UserModelUpdateItem, UserModelUpdateOutcome, error) {
	if err := ValidateUserModelUpdateItem(proposal); err != nil {
		return existing, UserModelUpdateOutcome{}, err
	}
	if policy.ContradictionConfidenceMargin < 0 || policy.ContradictionConfidenceMargin > 1 {
		return existing, UserModelUpdateOutcome{}, errors.New("user model update policy: contradiction confidence margin must be between 0 and 1")
	}

	proposal = guardSingleObservationPromotion(proposal)
	result := append([]UserModelUpdateItem(nil), existing...)

	idx, ok := findLiveIndex(result, proposal.Key)
	if !ok {
		result = append(result, proposal)
		return result, UserModelUpdateOutcome{
			Action:  UserModelUpdateEstablish,
			Applied: proposal,
			Reason:  "no prior live belief at this key",
		}, nil
	}

	prior := result[idx]

	if valuesAgree(prior.Value, proposal.Value) {
		// Prefer the shorter clean representation when the incoming evidence
		// exposes a conversational tail that was previously persisted.
		if len(normalizeBeliefValue(proposal.Value)) < len(normalizeBeliefValue(prior.Value)) && cleanBeliefVariant(normalizeBeliefValue(proposal.Value), normalizeBeliefValue(prior.Value)) {
			prior.Value = strings.TrimSpace(proposal.Value)
		}
		reinforced := reinforceUserModelUpdateItem(prior, proposal)
		result[idx] = reinforced
		return result, UserModelUpdateOutcome{
			Action:  UserModelUpdateReinforce,
			Applied: reinforced,
			Reason:  "proposal agrees with the existing live belief; evidence accumulates",
		}, nil
	}

	// Values disagree: the B96.34.11 contradiction case. Do not overwrite
	// blindly — rank by provenance first, and only let confidence break a
	// tie within the same priority tier, by a real margin.
	priorPriority := sourcePriority(prior.Provenance.Kind)
	newPriority := sourcePriority(proposal.Provenance.Kind)
	supersedes := newPriority > priorPriority ||
		(newPriority == priorPriority && proposal.Confidence-prior.Confidence >= policy.ContradictionConfidenceMargin)

	if supersedes {
		superseded := prior
		superseded.Status = UserModelStatusSuperseded
		superseded.UpdatedAt = proposal.UpdatedAt
		result[idx] = superseded
		result = append(result, proposal)
		return result, UserModelUpdateOutcome{
			Action:     UserModelUpdateSupersede,
			Applied:    proposal,
			Superseded: &superseded,
			Reason:     "new proposal outranks the prior live belief",
		}, nil
	}

	held := proposal
	held.Status = UserModelStatusUncertain
	result = append(result, held)
	return result, UserModelUpdateOutcome{
		Action:  UserModelUpdateHoldForReconciliation,
		Applied: held,
		Reason:  "conflicting proposal did not outrank the existing belief; both remain live pending B96.34.13",
	}, nil
}

// ValidateUserModelUpdateItem checks structural invariants only. Like every
// other Validate* in B96.34, it does not judge whether the belief is true.
func ValidateUserModelUpdateItem(item UserModelUpdateItem) error {
	if strings.TrimSpace(item.Key) == "" {
		return errors.New("user model update: key is required")
	}
	if strings.TrimSpace(item.Value) == "" {
		return errors.New("user model update: value is required")
	}
	if !validConfidence(item.Confidence) {
		return fmt.Errorf("user model update %q: confidence must be between 0 and 1", item.Key)
	}
	if !validSourceKind(item.Provenance.Kind) {
		return fmt.Errorf("user model update %q: invalid source kind", item.Key)
	}
	if !validStatus(item.Status) {
		return fmt.Errorf("user model update %q: invalid status", item.Key)
	}
	if item.EvidenceCount < 0 {
		return fmt.Errorf("user model update %q: evidence_count cannot be negative", item.Key)
	}
	return nil
}
