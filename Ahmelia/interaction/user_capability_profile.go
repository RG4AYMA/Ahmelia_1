package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// UserCapabilityLevel expresses whether, and how independently, a user can
// execute actions involving a capability. It is intentionally separate from
// UserKnowledgeLevel: knowing what something is does not imply the ability
// to perform it, and being able to perform it does not imply deep knowledge
// of why it works.
type UserCapabilityLevel string

const (
	UserCapabilityNone        UserCapabilityLevel = "none"
	UserCapabilityAssisted    UserCapabilityLevel = "assisted"
	UserCapabilityIndependent UserCapabilityLevel = "independent"
	UserCapabilityProficient  UserCapabilityLevel = "proficient"
	UserCapabilityExpert      UserCapabilityLevel = "expert"
)

// UserCapabilityEvidence records the basis for a capability belief.
type UserCapabilityEvidence struct {
	ID         string             `json:"id"`
	Source     UserModelSourceRef `json:"source"`
	Statement  string             `json:"statement,omitempty"`
	ObservedAt time.Time          `json:"observed_at"`
}

// UserCapabilityRecord is the richer B96.34.6 representation. Confidence says
// how strongly Ahmelia believes the record; Level says what is believed about
// the user's ability to execute. These dimensions must not be conflated with
// each other, nor with UserKnowledgeRecord.Level from B96.34.5.
type UserCapabilityRecord struct {
	Capability string                   `json:"capability"`
	Level      UserCapabilityLevel      `json:"level"`
	Confidence float64                  `json:"confidence"`
	Source     UserModelSourceRef       `json:"source"`
	Evidence   []UserCapabilityEvidence `json:"evidence,omitempty"`
	Status     UserModelItemStatus      `json:"status"`
	CreatedAt  time.Time                `json:"created_at"`
	UpdatedAt  time.Time                `json:"updated_at"`
}

// UserCapabilityUpdate proposes a capability record without mutating the model.
type UserCapabilityUpdate struct {
	Capability string
	Level      UserCapabilityLevel
	Confidence float64
	Source     UserModelSourceRef
	Evidence   []UserCapabilityEvidence
	Now        time.Time
}

// UserCapabilityPolicy controls only local admission of capability proposals.
// Cross-item update/reconciliation remains B96.34.11.
//
// Thresholds are deliberately stricter than DefaultUserKnowledgePolicy: a
// capability belief can change how Ahmelia collaborates (e.g. offering to
// implement directly instead of explaining first), so a wrong capability
// belief is more costly than a wrong knowledge belief.
type UserCapabilityPolicy struct {
	MinInferredConfidence float64
	MinObservedConfidence float64
	MinEvidenceCount      int
}

func DefaultUserCapabilityPolicy() UserCapabilityPolicy {
	return UserCapabilityPolicy{
		MinInferredConfidence: 0.88,
		MinObservedConfidence: 0.92,
		MinEvidenceCount:      3,
	}
}

// EvaluateUserCapabilityUpdate validates and classifies a proposal. Inferred
// capability can be accepted as a candidate, but remains uncertain and does
// not become an established user fact by itself.
func EvaluateUserCapabilityUpdate(update UserCapabilityUpdate, policy UserCapabilityPolicy) (UserCapabilityRecord, bool, error) {
	if strings.TrimSpace(update.Capability) == "" {
		return UserCapabilityRecord{}, false, errors.New("user capability: capability is required")
	}
	if !validCapabilityLevel(update.Level) {
		return UserCapabilityRecord{}, false, errors.New("user capability: invalid level")
	}
	if !validConfidence(update.Confidence) {
		return UserCapabilityRecord{}, false, errors.New("user capability: confidence must be between 0 and 1")
	}
	if !validSourceKind(update.Source.Kind) {
		return UserCapabilityRecord{}, false, errors.New("user capability: invalid source kind")
	}
	if policy.MinEvidenceCount < 1 {
		return UserCapabilityRecord{}, false, errors.New("user capability: minimum evidence count must be positive")
	}

	record := UserCapabilityRecord{
		Capability: strings.TrimSpace(update.Capability),
		Level:      update.Level,
		Confidence: update.Confidence,
		Source:     update.Source,
		Evidence:   append([]UserCapabilityEvidence(nil), update.Evidence...),
		Status:     UserModelStatusUncertain,
		CreatedAt:  update.Now,
		UpdatedAt:  update.Now,
	}

	switch update.Source.Kind {
	case UserSourceExplicitUser:
		record.Status = UserModelStatusActive
		return record, true, nil
	case UserSourceObserved:
		if update.Confidence >= policy.MinObservedConfidence && len(update.Evidence) >= policy.MinEvidenceCount {
			record.Status = UserModelStatusActive
			return record, true, nil
		}
	case UserSourceInferred:
		if update.Confidence >= policy.MinInferredConfidence && len(update.Evidence) >= policy.MinEvidenceCount {
			return record, true, nil
		}
	case UserSourceSystem, UserSourceImported:
		if update.Confidence >= 1.0 && len(update.Evidence) >= 1 {
			record.Status = UserModelStatusActive
			return record, true, nil
		}
	}

	return record, false, nil
}

// CapabilityToCore projects to the minimal B96.34.1 contract. The richer
// record remains the canonical B96.34.6 semantic representation.
func (r UserCapabilityRecord) CapabilityToCore() UserCapability {
	return UserCapability{
		Key:        r.Capability,
		Value:      string(r.Level),
		Confidence: r.Confidence,
		Provenance: r.Source,
		Status:     r.Status,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

// ValidateUserCapabilityRecord validates structure only; it does not determine
// whether the user actually has the claimed capability.
func ValidateUserCapabilityRecord(r UserCapabilityRecord) error {
	if strings.TrimSpace(r.Capability) == "" {
		return errors.New("user capability: capability is required")
	}
	if !validCapabilityLevel(r.Level) {
		return fmt.Errorf("user capability %q: invalid level", r.Capability)
	}
	if !validConfidence(r.Confidence) {
		return fmt.Errorf("user capability %q: confidence must be between 0 and 1", r.Capability)
	}
	if !validSourceKind(r.Source.Kind) {
		return fmt.Errorf("user capability %q: invalid source kind", r.Capability)
	}
	if !validStatus(r.Status) {
		return fmt.Errorf("user capability %q: invalid status", r.Capability)
	}

	seen := make(map[string]struct{}, len(r.Evidence))
	for i, evidence := range r.Evidence {
		if strings.TrimSpace(evidence.ID) == "" {
			return fmt.Errorf("user capability %q: evidence %d has empty id", r.Capability, i)
		}
		if _, ok := seen[evidence.ID]; ok {
			return fmt.Errorf("user capability %q: duplicate evidence id %q", r.Capability, evidence.ID)
		}
		seen[evidence.ID] = struct{}{}
		if !validSourceKind(evidence.Source.Kind) {
			return fmt.Errorf("user capability %q: evidence %q has invalid source", r.Capability, evidence.ID)
		}
	}
	return nil
}

func validCapabilityLevel(level UserCapabilityLevel) bool {
	switch level {
	case UserCapabilityNone, UserCapabilityAssisted, UserCapabilityIndependent,
		UserCapabilityProficient, UserCapabilityExpert:
		return true
	default:
		return false
	}
}
