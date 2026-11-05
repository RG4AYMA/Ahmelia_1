package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// UserAttributeEvidence records why an attribute exists. Evidence is kept
// separate from the attribute value so inferred beliefs never become facts by
// accident.
type UserAttributeEvidence struct {
	ID         string             `json:"id"`
	Source     UserModelSourceRef `json:"source"`
	Statement  string             `json:"statement,omitempty"`
	ObservedAt time.Time          `json:"observed_at"`
}

// UserAttributeRecord is the richer B96.34.2 attribute contract. It extends
// the minimal B96.34.1 representation without changing the meaning of the
// core UserModel boundary.
type UserAttributeRecord struct {
	Key        string                  `json:"key"`
	Value      string                  `json:"value"`
	Confidence float64                 `json:"confidence"`
	Source     UserModelSourceRef      `json:"source"`
	Evidence   []UserAttributeEvidence `json:"evidence,omitempty"`
	Status     UserModelItemStatus     `json:"status"`
	CreatedAt  time.Time               `json:"created_at"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

// UserAttributeUpdate is an explicit proposal to establish or revise an
// attribute. The policy below decides whether the proposal may become a
// durable model fact.
type UserAttributeUpdate struct {
	Key        string
	Value      string
	Confidence float64
	Source     UserModelSourceRef
	Evidence   []UserAttributeEvidence
	Now        time.Time
}

// UserAttributePolicy controls promotion of evidence into durable attributes.
// A single weak observation can remain evidence, but cannot establish a
// permanent attribute. Explicit user statements are the strongest source.
type UserAttributePolicy struct {
	MinInferredConfidence float64
	MinObservedConfidence float64
	MinEvidenceCount      int
}

func DefaultUserAttributePolicy() UserAttributePolicy {
	return UserAttributePolicy{
		MinInferredConfidence: 0.85,
		MinObservedConfidence: 0.90,
		MinEvidenceCount:      2,
	}
}

// EvaluateUserAttributeUpdate classifies an update without mutating a model.
// This separation is intentional: later B96.34.11 owns the complete update
// policy and conflict/reconciliation behavior.
func EvaluateUserAttributeUpdate(update UserAttributeUpdate, policy UserAttributePolicy) (UserAttributeRecord, bool, error) {
	if strings.TrimSpace(update.Key) == "" {
		return UserAttributeRecord{}, false, errors.New("user attribute: key is required")
	}
	if strings.TrimSpace(update.Value) == "" {
		return UserAttributeRecord{}, false, errors.New("user attribute: value is required")
	}
	if update.Confidence < 0 || update.Confidence > 1 {
		return UserAttributeRecord{}, false, errors.New("user attribute: confidence must be between 0 and 1")
	}
	if !validSourceKind(update.Source.Kind) {
		return UserAttributeRecord{}, false, errors.New("user attribute: invalid source kind")
	}
	if policy.MinEvidenceCount < 1 {
		return UserAttributeRecord{}, false, errors.New("user attribute: minimum evidence count must be positive")
	}

	record := UserAttributeRecord{
		Key:        strings.TrimSpace(update.Key),
		Value:      strings.TrimSpace(update.Value),
		Confidence: update.Confidence,
		Source:     update.Source,
		Evidence:   append([]UserAttributeEvidence(nil), update.Evidence...),
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
			record.Status = UserModelStatusUncertain
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

// AttributeToCore converts an accepted attribute into the B96.34.1 core
// representation. It is deliberately one-way at this stage.
func (r UserAttributeRecord) AttributeToCore() UserAttribute {
	return UserAttribute{
		Key:        r.Key,
		Value:      r.Value,
		Confidence: r.Confidence,
		Provenance: r.Source,
		Status:     r.Status,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

// ValidateUserAttributeRecord validates structure without deciding truth.
func ValidateUserAttributeRecord(r UserAttributeRecord) error {
	if strings.TrimSpace(r.Key) == "" {
		return errors.New("user attribute: key is required")
	}
	if strings.TrimSpace(r.Value) == "" {
		return errors.New("user attribute: value is required")
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("user attribute %q: confidence must be between 0 and 1", r.Key)
	}
	if !validSourceKind(r.Source.Kind) {
		return fmt.Errorf("user attribute %q: invalid source kind", r.Key)
	}
	if !validStatus(r.Status) {
		return fmt.Errorf("user attribute %q: invalid status", r.Key)
	}
	for i, evidence := range r.Evidence {
		if strings.TrimSpace(evidence.ID) == "" {
			return fmt.Errorf("user attribute %q: evidence %d has empty id", r.Key, i)
		}
		if !validSourceKind(evidence.Source.Kind) {
			return fmt.Errorf("user attribute %q: evidence %q has invalid source", r.Key, evidence.ID)
		}
	}
	return nil
}
