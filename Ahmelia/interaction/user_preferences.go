package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// UserPreferenceEvidence records the observable or explicit basis for a
// preference. Evidence is kept attached to the preference so a preference
// remains auditable and can later be reconciled by the central update policy.
type UserPreferenceEvidence struct {
	ID         string             `json:"id"`
	Source     UserModelSourceRef `json:"source"`
	Statement  string             `json:"statement,omitempty"`
	ObservedAt time.Time          `json:"observed_at"`
}

// UserPreferenceRecord is the richer B96.34.3 representation of a durable
// preference. Strength is signed: +1 is a very strong preference, 0 is
// neutral/unknown, and -1 is a very strong aversion.
type UserPreferenceRecord struct {
	Key        string                   `json:"key"`
	Value      string                   `json:"value"`
	Strength   float64                  `json:"strength"`
	Confidence float64                  `json:"confidence"`
	Source     UserModelSourceRef       `json:"source"`
	Evidence   []UserPreferenceEvidence `json:"evidence,omitempty"`
	Status     UserModelItemStatus      `json:"status"`
	CreatedAt  time.Time                `json:"created_at"`
	UpdatedAt  time.Time                `json:"updated_at"`
}

// UserPreferenceUpdate is a proposal to establish or revise a preference.
// B96.34.3 classifies the proposal but does not mutate the UserModel.
type UserPreferenceUpdate struct {
	Key        string
	Value      string
	Strength   float64
	Confidence float64
	Source     UserModelSourceRef
	Evidence   []UserPreferenceEvidence
	Now        time.Time
}

// UserPreferencePolicy is intentionally local to preference safety. The
// complete cross-item update/reconciliation policy remains B96.34.11.
type UserPreferencePolicy struct {
	MinInferredConfidence float64
	MinObservedConfidence float64
	MinEvidenceCount      int
}

func DefaultUserPreferencePolicy() UserPreferencePolicy {
	return UserPreferencePolicy{
		MinInferredConfidence: 0.85,
		MinObservedConfidence: 0.90,
		MinEvidenceCount:      2,
	}
}

// EvaluateUserPreferenceUpdate validates and classifies a preference proposal.
// Inference can be accepted as a candidate, but it never silently becomes an
// explicit user fact: inferred preferences remain uncertain until a later
// update policy decides otherwise.
func EvaluateUserPreferenceUpdate(update UserPreferenceUpdate, policy UserPreferencePolicy) (UserPreferenceRecord, bool, error) {
	if strings.TrimSpace(update.Key) == "" {
		return UserPreferenceRecord{}, false, errors.New("user preference: key is required")
	}
	if strings.TrimSpace(update.Value) == "" {
		return UserPreferenceRecord{}, false, errors.New("user preference: value is required")
	}
	if update.Strength < -1 || update.Strength > 1 {
		return UserPreferenceRecord{}, false, errors.New("user preference: strength must be between -1 and 1")
	}
	if update.Confidence < 0 || update.Confidence > 1 {
		return UserPreferenceRecord{}, false, errors.New("user preference: confidence must be between 0 and 1")
	}
	if !validSourceKind(update.Source.Kind) {
		return UserPreferenceRecord{}, false, errors.New("user preference: invalid source kind")
	}
	if policy.MinEvidenceCount < 1 {
		return UserPreferenceRecord{}, false, errors.New("user preference: minimum evidence count must be positive")
	}

	record := UserPreferenceRecord{
		Key:        strings.TrimSpace(update.Key),
		Value:      strings.TrimSpace(update.Value),
		Strength:   update.Strength,
		Confidence: update.Confidence,
		Source:     update.Source,
		Evidence:   append([]UserPreferenceEvidence(nil), update.Evidence...),
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

// PreferenceToCore converts an accepted preference to the B96.34.1 core
// contract. Strength is preserved in the value only by convention at this
// stage; the richer record remains the canonical B96.34.3 semantic form.
func (r UserPreferenceRecord) PreferenceToCore() UserPreference {
	return UserPreference{
		Key:        r.Key,
		Value:      r.Value,
		Confidence: r.Confidence,
		Provenance: r.Source,
		Status:     r.Status,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

// ValidateUserPreferenceRecord validates structure without deciding whether
// the preference is true or currently applicable.
func ValidateUserPreferenceRecord(r UserPreferenceRecord) error {
	if strings.TrimSpace(r.Key) == "" {
		return errors.New("user preference: key is required")
	}
	if strings.TrimSpace(r.Value) == "" {
		return errors.New("user preference: value is required")
	}
	if r.Strength < -1 || r.Strength > 1 {
		return fmt.Errorf("user preference %q: strength must be between -1 and 1", r.Key)
	}
	if !validConfidence(r.Confidence) {
		return fmt.Errorf("user preference %q: confidence must be between 0 and 1", r.Key)
	}
	if !validSourceKind(r.Source.Kind) {
		return fmt.Errorf("user preference %q: invalid source kind", r.Key)
	}
	if !validStatus(r.Status) {
		return fmt.Errorf("user preference %q: invalid status", r.Key)
	}
	for i, evidence := range r.Evidence {
		if strings.TrimSpace(evidence.ID) == "" {
			return fmt.Errorf("user preference %q: evidence %d has empty id", r.Key, i)
		}
		if !validSourceKind(evidence.Source.Kind) {
			return fmt.Errorf("user preference %q: evidence %q has invalid source", r.Key, evidence.ID)
		}
	}
	return nil
}
