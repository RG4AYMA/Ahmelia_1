package interaction

import (
	"errors"
	"strings"
	"time"
)

// B96.34.9 — User State / User Model Boundary.
//
// UserState (user_state.go) captures signals about the current turn only —
// it is recomputed every turn and is expected to change. UserModel
// (user_model_core.go) represents beliefs meant to persist across
// conversations. The design requirement from B96.34.9 is explicit:
//
//	Hoy: UserState.focused = true
//	no significa: UserModel.personality = highly_focused
//
// Every prior B96.34.x domain block (attributes, preferences, goals,
// capabilities, knowledge, communication profile, interaction patterns)
// already gates promotion into Active status behind its own source kind,
// confidence, and evidence-count policy. What was still missing is a single
// place that makes the *state-to-model seam itself* a hard boundary rather
// than a convention every call site has to remember on its own. This file
// is that boundary. It does not introduce a new domain, does not decide any
// domain's promotion policy, and does not mutate UserModel.

// UserStateSnapshotRef is the only representation of a UserState reading
// that may ever be offered toward a UserModel proposal. It deliberately does
// not embed UserState itself: the raw per-turn signal magnitudes
// (Frustration, Enthusiasm, Confusion, ...) never cross this boundary, only
// a coarse, human-auditable note plus a timestamp. This keeps "the state
// changed" from silently smuggling detailed transient data into a durable
// record.
type UserStateSnapshotRef struct {
	ObservedAt      time.Time       `json:"observed_at"`
	InteractionNeed InteractionNeed `json:"interaction_need,omitempty"`
	Note            string          `json:"note"`
}

// DescribeUserStateForModel is the only sanctioned bridge from a UserState
// reading toward a UserModel proposal. Domain blocks (B96.34.2 .. B96.34.8)
// still decide, independently and through their own EvaluateUser*Update or
// ReinforceUserInteractionPattern entry point, whether this note ever
// becomes evidence at all.
func DescribeUserStateForModel(s UserState, now time.Time) UserStateSnapshotRef {
	return UserStateSnapshotRef{
		ObservedAt:      now,
		InteractionNeed: s.InteractionNeed,
		Note:            summarizeUserStateForAudit(s),
	}
}

// UserStateSourceRef builds the only UserModelSourceRef a caller may attach
// to a proposal whose provenance is a current-turn UserState reading. It is
// hard-capped at UserSourceObserved.
//
// This matters concretely: UserSourceExplicitUser is trusted by every
// B96.34.x domain directly into Active status from a single occurrence, and
// UserSourceSystem/UserSourceImported are trusted by several domains at full
// confidence with minimal evidence. A momentary state reading is neither an
// explicit statement nor a system fact, so it can never be labeled as
// either here — it must always pass through a domain's ordinary
// evidence-count and confidence gate like any other observed signal.
func UserStateSourceRef(ref UserStateSnapshotRef, sourceID, evidenceID string) (UserModelSourceRef, error) {
	if strings.TrimSpace(sourceID) == "" {
		return UserModelSourceRef{}, errors.New("user state boundary: source id is required")
	}
	if ref.ObservedAt.IsZero() {
		return UserModelSourceRef{}, errors.New("user state boundary: observed_at is required")
	}
	return UserModelSourceRef{
		Kind:       UserSourceObserved,
		SourceID:   strings.TrimSpace(sourceID),
		EvidenceID: strings.TrimSpace(evidenceID),
		ObservedAt: ref.ObservedAt,
	}, nil
}

// RejectPersistentTraitFromSingleState guards the specific failure mode
// B96.34.9 exists to prevent: one energetic, frustrated, or confused turn
// becoming a permanent UserModel trait by itself. A state-derived proposal
// backed by a single observation may justify Uncertain status, never Active
// — Active requires the domain's own accumulation path (repeated evidence
// via EvaluateUser*Update, or ReinforceUserInteractionPattern's running
// EvidenceCount), regardless of how strong that one reading's confidence is.
//
// This is a boundary check, not a replacement for domain policy: callers
// still run the domain's own Evaluate/Reinforce function, which has its own
// independent thresholds. This function catches the case where a caller
// tries to hand-construct or bypass that policy for a state-only signal.
func RejectPersistentTraitFromSingleState(proposedStatus UserModelItemStatus, evidenceCount int) error {
	if evidenceCount <= 1 && proposedStatus == UserModelStatusActive {
		return errors.New("user state boundary: a single state-derived observation cannot establish an active user model trait")
	}
	return nil
}

func summarizeUserStateForAudit(s UserState) string {
	var parts []string
	if s.Frustration >= 0.60 {
		parts = append(parts, "elevated frustration signal")
	}
	if s.Confusion >= 0.60 {
		parts = append(parts, "elevated confusion signal")
	}
	if s.Urgency >= 0.60 {
		parts = append(parts, "elevated urgency signal")
	}
	if s.Enthusiasm >= 0.60 {
		parts = append(parts, "elevated enthusiasm signal")
	}
	if s.Humor >= 0.60 {
		parts = append(parts, "humor markers")
	}
	if len(parts) == 0 {
		return "no salient state signal"
	}
	return strings.Join(parts, "; ")
}
