package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// UserInteractionPatternEvidence records one observed instance supporting a
// pattern. Unlike other B96.34 evidence lists, this is deliberately a bounded
// audit sample, not a full history: EvidenceCount on the record is the
// authoritative running total, and can exceed len(Evidence).
type UserInteractionPatternEvidence struct {
	ID         string             `json:"id"`
	Source     UserModelSourceRef `json:"source"`
	Statement  string             `json:"statement,omitempty"`
	ObservedAt time.Time          `json:"observed_at"`
}

// UserInteractionPatternRecord is the richer B96.34.6/34.7-style
// representation, but with a fundamentally different lifecycle: a pattern is
// never evaluated from a single proposal. It is reinforced across many
// observations, and Confidence is a probabilistic running estimate rather
// than a value fixed at admission time. Design requirement: "no debe
// convertirse en una etiqueta rígida."
type UserInteractionPatternRecord struct {
	Pattern       string                           `json:"pattern"`
	Confidence    float64                          `json:"confidence"`
	EvidenceCount int                              `json:"evidence_count"`
	Source        UserModelSourceRef               `json:"source"`
	Evidence      []UserInteractionPatternEvidence `json:"evidence,omitempty"`
	Status        UserModelItemStatus              `json:"status"`
	CreatedAt     time.Time                        `json:"created_at"`
	UpdatedAt     time.Time                        `json:"updated_at"`
}

// UserInteractionPatternObservation is one new data point about a pattern. It
// does not mutate any existing record; ReinforceUserInteractionPattern
// combines it with the prior record (if any) into a new one.
type UserInteractionPatternObservation struct {
	Pattern    string
	Confidence float64
	Source     UserModelSourceRef
	Evidence   *UserInteractionPatternEvidence // optional discrete evidence item
	Now        time.Time
}

// UserInteractionPatternPolicy controls only local admission of a pattern
// into Active status, and how much raw evidence is retained for audit.
// Cross-item update/reconciliation remains B96.34.11.
type UserInteractionPatternPolicy struct {
	MinEvidenceCountForActive int
	MinConfidenceForActive    float64
	MaxRetainedEvidence       int
}

func DefaultUserInteractionPatternPolicy() UserInteractionPatternPolicy {
	return UserInteractionPatternPolicy{
		MinEvidenceCountForActive: 5,
		MinConfidenceForActive:    0.80,
		MaxRetainedEvidence:       10,
	}
}

// ReinforceUserInteractionPattern folds one new observation into an existing
// pattern record, or starts a new one if existing.Pattern is empty. The
// pattern name is immutable once a record exists: an observation for a
// different pattern name must go through its own record.
//
// Confidence moves as a running average weighted by prior EvidenceCount, so
// no single observation can swing a pattern with substantial history — the
// pattern stays probabilistic rather than becoming a rigid label from one
// data point. Explicit user statements ("prefiero trabajar incremental") are
// still trusted directly into Active status, consistent with every other
// B96.34 domain.
func ReinforceUserInteractionPattern(existing UserInteractionPatternRecord, observation UserInteractionPatternObservation, policy UserInteractionPatternPolicy) (UserInteractionPatternRecord, error) {
	pattern := strings.TrimSpace(observation.Pattern)
	if pattern == "" {
		return UserInteractionPatternRecord{}, errors.New("interaction pattern: pattern is required")
	}
	if existing.Pattern != "" && existing.Pattern != pattern {
		return UserInteractionPatternRecord{}, fmt.Errorf("interaction pattern: observation for %q does not match existing record %q", pattern, existing.Pattern)
	}
	if !validConfidence(observation.Confidence) {
		return UserInteractionPatternRecord{}, errors.New("interaction pattern: confidence must be between 0 and 1")
	}
	if !validSourceKind(observation.Source.Kind) {
		return UserInteractionPatternRecord{}, errors.New("interaction pattern: invalid source kind")
	}
	if policy.MinEvidenceCountForActive < 1 {
		return UserInteractionPatternRecord{}, errors.New("interaction pattern: minimum evidence count for active must be positive")
	}
	if policy.MaxRetainedEvidence < 0 {
		return UserInteractionPatternRecord{}, errors.New("interaction pattern: max retained evidence cannot be negative")
	}

	record := existing
	record.Pattern = pattern
	record.Source = observation.Source
	record.UpdatedAt = observation.Now

	if existing.EvidenceCount == 0 {
		record.Confidence = observation.Confidence
		record.EvidenceCount = 1
		record.CreatedAt = observation.Now
		record.Status = UserModelStatusUncertain
	} else {
		record.EvidenceCount = existing.EvidenceCount + 1
		record.Confidence = (existing.Confidence*float64(existing.EvidenceCount) + observation.Confidence) / float64(record.EvidenceCount)
	}

	if observation.Evidence != nil {
		record.Evidence = append(append([]UserInteractionPatternEvidence(nil), record.Evidence...), *observation.Evidence)
		if policy.MaxRetainedEvidence > 0 && len(record.Evidence) > policy.MaxRetainedEvidence {
			record.Evidence = record.Evidence[len(record.Evidence)-policy.MaxRetainedEvidence:]
		} else if policy.MaxRetainedEvidence == 0 {
			record.Evidence = nil
		}
	}

	switch {
	case record.Status == UserModelStatusActive:
		// Already established; this block does not demote a record on the
		// strength of one weaker observation. Reconciliation is B96.34.11/13.
	case observation.Source.Kind == UserSourceExplicitUser:
		record.Status = UserModelStatusActive
	case record.EvidenceCount >= policy.MinEvidenceCountForActive && record.Confidence >= policy.MinConfidenceForActive:
		record.Status = UserModelStatusActive
	default:
		record.Status = UserModelStatusUncertain
	}

	return record, nil
}

// PatternToCore projects to the minimal B96.34.1 contract. The richer record
// remains the canonical B96.34.8 semantic representation.
func (r UserInteractionPatternRecord) PatternToCore() UserInteractionPattern {
	return UserInteractionPattern{
		Pattern:       r.Pattern,
		Confidence:    r.Confidence,
		EvidenceCount: r.EvidenceCount,
		Provenance:    r.Source,
		Status:        r.Status,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

// ValidateUserInteractionPatternRecord validates structure only; it does not
// determine whether the user actually exhibits the claimed pattern.
func ValidateUserInteractionPatternRecord(r UserInteractionPatternRecord) error {
	if strings.TrimSpace(r.Pattern) == "" {
		return errors.New("interaction pattern: pattern is required")
	}
	if !validConfidence(r.Confidence) {
		return fmt.Errorf("interaction pattern %q: confidence must be between 0 and 1", r.Pattern)
	}
	if r.EvidenceCount < 1 {
		return fmt.Errorf("interaction pattern %q: evidence_count must be >= 1", r.Pattern)
	}
	if !validSourceKind(r.Source.Kind) {
		return fmt.Errorf("interaction pattern %q: invalid source kind", r.Pattern)
	}
	if !validStatus(r.Status) {
		return fmt.Errorf("interaction pattern %q: invalid status", r.Pattern)
	}
	if len(r.Evidence) > r.EvidenceCount {
		return fmt.Errorf("interaction pattern %q: retained evidence sample cannot exceed evidence_count", r.Pattern)
	}

	seen := make(map[string]struct{}, len(r.Evidence))
	for i, evidence := range r.Evidence {
		if strings.TrimSpace(evidence.ID) == "" {
			return fmt.Errorf("interaction pattern %q: evidence %d has empty id", r.Pattern, i)
		}
		if _, ok := seen[evidence.ID]; ok {
			return fmt.Errorf("interaction pattern %q: duplicate evidence id %q", r.Pattern, evidence.ID)
		}
		seen[evidence.ID] = struct{}{}
		if !validSourceKind(evidence.Source.Kind) {
			return fmt.Errorf("interaction pattern %q: evidence %q has invalid source", r.Pattern, evidence.ID)
		}
	}
	return nil
}
