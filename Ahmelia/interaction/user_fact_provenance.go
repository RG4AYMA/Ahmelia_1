package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// B96.34.10 — Confidence & Provenance.
//
// Every relevant UserModel item should be able to answer:
//
//	¿De dónde salió?         -> SourceKind, SourceID, ActorID
//	¿Cuándo se obtuvo?       -> ObservedAt / CreatedAt / UpdatedAt
//	¿Quién lo afirmó?        -> ActorID
//	¿Fue inferido?           -> Inferred
//	¿Qué tan seguro estamos? -> Confidence
//	¿Ha sido contradicho?    -> Contradicted, ContradictingEvidenceIDs
//
// B96.34.1 through B96.34.8 already carry the fields these questions need
// (UserModelSourceRef's Kind/SourceID/ActorID/ObservedAt, Confidence,
// CreatedAt, UpdatedAt, Status) — but each domain answers them
// independently, in its own record shape. This block does not replace or
// restructure any of those shapes. It adds one canonical, cross-domain read
// projection, UserFactAudit, plus a one-way adapter per existing domain
// record, so any consumer (e.g. the future query surface in B96.34.12) can
// ask the same six questions of an attribute, a preference, a goal, a
// capability, a knowledge item, a communication-profile value, or an
// interaction pattern, and get back the same shape.
//
// It also adds the one question no domain record could answer on its own:
// whether a value currently has known contradicting evidence. That question
// is answerable here without this block deciding what to do about it —
// reconciliation remains B96.34.11, conflict resolution B96.34.13.

// UserFactAudit is the canonical, cross-domain answer bundle to the six
// B96.34.10 questions. It is a read-only projection, not a new storage type.
type UserFactAudit struct {
	Value                    string              `json:"value"`
	Confidence               float64             `json:"confidence"`
	EvidenceCount            int                 `json:"evidence_count"`
	SourceKind               UserModelSourceKind `json:"source_kind"`
	SourceID                 string              `json:"source_id,omitempty"`
	ActorID                  string              `json:"actor_id,omitempty"`
	ObservedAt               time.Time           `json:"observed_at,omitempty"`
	Inferred                 bool                `json:"inferred"`
	Status                   UserModelItemStatus `json:"status"`
	Contradicted             bool                `json:"contradicted"`
	ContradictingEvidenceIDs []string            `json:"contradicting_evidence_ids,omitempty"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
}

func newUserFactAudit(value string, confidence float64, evidenceCount int, source UserModelSourceRef, status UserModelItemStatus, createdAt, updatedAt time.Time, contradictingEvidenceIDs []string) UserFactAudit {
	return UserFactAudit{
		Value:                    value,
		Confidence:               confidence,
		EvidenceCount:            evidenceCount,
		SourceKind:               source.Kind,
		SourceID:                 source.SourceID,
		ActorID:                  source.ActorID,
		ObservedAt:               source.ObservedAt,
		Inferred:                 source.Kind == UserSourceInferred,
		Status:                   status,
		Contradicted:             len(contradictingEvidenceIDs) > 0,
		ContradictingEvidenceIDs: append([]string(nil), contradictingEvidenceIDs...),
		CreatedAt:                createdAt,
		UpdatedAt:                updatedAt,
	}
}

// AttributeAudit, PreferenceAudit, GoalAudit, CapabilityAudit,
// KnowledgeAudit, CommunicationProfileAudit and InteractionPatternAudit each
// project one existing domain record into the canonical audit shape. Every
// adapter is one-way and read-only: none mutates its source record, and
// none decides whether the underlying value is true.
//
// contradictingEvidenceIDs is optional and supplied by the caller — this
// block does not scan for contradictions on its own; see
// DetectValueContradiction for the one detection helper it does provide.

func AttributeAudit(r UserAttributeRecord, contradictingEvidenceIDs ...string) UserFactAudit {
	return newUserFactAudit(r.Value, r.Confidence, len(r.Evidence), r.Source, r.Status, r.CreatedAt, r.UpdatedAt, contradictingEvidenceIDs)
}

func PreferenceAudit(r UserPreferenceRecord, contradictingEvidenceIDs ...string) UserFactAudit {
	return newUserFactAudit(r.Value, r.Confidence, len(r.Evidence), r.Source, r.Status, r.CreatedAt, r.UpdatedAt, contradictingEvidenceIDs)
}

func GoalAudit(r UserGoalRecord, contradictingEvidenceIDs ...string) UserFactAudit {
	return newUserFactAudit(r.Goal, r.Confidence, len(r.Evidence), r.Source, r.Status, r.CreatedAt, r.UpdatedAt, contradictingEvidenceIDs)
}

func CapabilityAudit(r UserCapabilityRecord, contradictingEvidenceIDs ...string) UserFactAudit {
	return newUserFactAudit(string(r.Level), r.Confidence, len(r.Evidence), r.Source, r.Status, r.CreatedAt, r.UpdatedAt, contradictingEvidenceIDs)
}

func KnowledgeAudit(r UserKnowledgeRecord, contradictingEvidenceIDs ...string) UserFactAudit {
	return newUserFactAudit(string(r.Level), r.Confidence, len(r.Evidence), r.Source, r.Status, r.CreatedAt, r.UpdatedAt, contradictingEvidenceIDs)
}

func CommunicationProfileAudit(r UserCommunicationProfileValue, contradictingEvidenceIDs ...string) UserFactAudit {
	return newUserFactAudit(r.Value, r.Confidence, len(r.Evidence), r.Source, r.Status, r.CreatedAt, r.UpdatedAt, contradictingEvidenceIDs)
}

func InteractionPatternAudit(r UserInteractionPatternRecord, contradictingEvidenceIDs ...string) UserFactAudit {
	// EvidenceCount here is B96.34.8's authoritative running total, not
	// len(r.Evidence), which is only a bounded audit sample.
	return newUserFactAudit(r.Pattern, r.Confidence, r.EvidenceCount, r.Source, r.Status, r.CreatedAt, r.UpdatedAt, contradictingEvidenceIDs)
}

// DetectValueContradiction answers "¿ha sido contradicho?" between two live
// audits that describe the same fact slot (e.g. two candidate values for the
// same preference key, observed at different times or from different
// sources). It only detects and reports; it does not decide which value
// should win, does not merge them, and does not mutate either input. That
// decision belongs to B96.34.11 (update policy) and B96.34.13 (conflict
// resolution).
//
// Two audits are considered contradictory only if both are still live
// beliefs (Active or Uncertain — a Superseded or Rejected value is settled
// history, not a live contradiction) and their values differ.
func DetectValueContradiction(a, b UserFactAudit) bool {
	if !isLiveUserFactStatus(a.Status) || !isLiveUserFactStatus(b.Status) {
		return false
	}
	return strings.TrimSpace(a.Value) != strings.TrimSpace(b.Value)
}

func isLiveUserFactStatus(status UserModelItemStatus) bool {
	return status == UserModelStatusActive || status == UserModelStatusUncertain
}

// Explain renders a short, human-auditable answer to all six B96.34.10
// questions for one fact. It is intended for debugging/audit surfaces, not
// for end-user-facing text.
func (a UserFactAudit) Explain() string {
	origin := string(a.SourceKind)
	if origin == "" {
		origin = "unknown"
	}
	inferred := "no"
	if a.Inferred {
		inferred = "yes"
	}
	contradicted := "no"
	if a.Contradicted {
		contradicted = fmt.Sprintf("yes (%d)", len(a.ContradictingEvidenceIDs))
	}
	actor := a.ActorID
	if actor == "" {
		actor = "unknown"
	}
	return fmt.Sprintf(
		"value=%q confidence=%.2f status=%s source=%s actor=%s inferred=%s evidence_count=%d contradicted=%s observed_at=%s updated_at=%s",
		a.Value, a.Confidence, a.Status, origin, actor, inferred, a.EvidenceCount, contradicted,
		formatUserFactAuditTime(a.ObservedAt), formatUserFactAuditTime(a.UpdatedAt),
	)
}

func formatUserFactAuditTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339)
}

// ValidateUserFactAudit checks structural invariants only. It does not judge
// whether the underlying fact is true.
func ValidateUserFactAudit(a UserFactAudit) error {
	if !validConfidence(a.Confidence) {
		return errors.New("user fact audit: confidence must be between 0 and 1")
	}
	if !validSourceKind(a.SourceKind) {
		return errors.New("user fact audit: invalid source kind")
	}
	if !validStatus(a.Status) {
		return errors.New("user fact audit: invalid status")
	}
	if a.EvidenceCount < 0 {
		return errors.New("user fact audit: evidence_count cannot be negative")
	}
	if a.Contradicted && len(a.ContradictingEvidenceIDs) == 0 {
		return errors.New("user fact audit: contradicted is true but no contradicting evidence ids were given")
	}
	if !a.Contradicted && len(a.ContradictingEvidenceIDs) > 0 {
		return errors.New("user fact audit: contradicting evidence ids given but contradicted is false")
	}
	return nil
}
