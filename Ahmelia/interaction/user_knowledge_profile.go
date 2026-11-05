package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// UserKnowledgeLevel expresses the user's relationship to a knowledge topic.
// It is intentionally separate from capability: knowing about a topic does
// not imply the ability to perform an action involving it.
type UserKnowledgeLevel string

const (
	UserKnowledgeUnknown  UserKnowledgeLevel = "unknown"
	UserKnowledgeAware    UserKnowledgeLevel = "aware"
	UserKnowledgeFamiliar UserKnowledgeLevel = "familiar"
	UserKnowledgeWorking  UserKnowledgeLevel = "working"
	UserKnowledgeAdvanced UserKnowledgeLevel = "advanced"
	UserKnowledgeExpert   UserKnowledgeLevel = "expert"
)

// UserKnowledgeEvidence records the basis for a knowledge belief.
type UserKnowledgeEvidence struct {
	ID         string             `json:"id"`
	Source     UserModelSourceRef `json:"source"`
	Statement  string             `json:"statement,omitempty"`
	ObservedAt time.Time          `json:"observed_at"`
}

// UserKnowledgeRecord is the richer B96.34.5 representation. Confidence says
// how strongly Ahmelia believes the record; Level says what is believed about
// the user's knowledge. These dimensions must not be conflated.
type UserKnowledgeRecord struct {
	Topic      string                  `json:"topic"`
	Level      UserKnowledgeLevel      `json:"level"`
	Confidence float64                 `json:"confidence"`
	Source     UserModelSourceRef      `json:"source"`
	Evidence   []UserKnowledgeEvidence `json:"evidence,omitempty"`
	Status     UserModelItemStatus     `json:"status"`
	CreatedAt  time.Time               `json:"created_at"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

// UserKnowledgeUpdate proposes a knowledge record without mutating the model.
type UserKnowledgeUpdate struct {
	Topic      string
	Level      UserKnowledgeLevel
	Confidence float64
	Source     UserModelSourceRef
	Evidence   []UserKnowledgeEvidence
	Now        time.Time
}

// UserKnowledgePolicy controls only local admission of knowledge proposals.
// Cross-item update/reconciliation remains B96.34.11.
type UserKnowledgePolicy struct {
	MinInferredConfidence float64
	MinObservedConfidence float64
	MinEvidenceCount      int
}

func DefaultUserKnowledgePolicy() UserKnowledgePolicy {
	return UserKnowledgePolicy{
		MinInferredConfidence: 0.85,
		MinObservedConfidence: 0.90,
		MinEvidenceCount:      2,
	}
}

// EvaluateUserKnowledgeUpdate validates and classifies a proposal. Inferred
// knowledge can be accepted as a candidate, but remains uncertain and does
// not become an established user fact by itself.
func EvaluateUserKnowledgeUpdate(update UserKnowledgeUpdate, policy UserKnowledgePolicy) (UserKnowledgeRecord, bool, error) {
	if strings.TrimSpace(update.Topic) == "" {
		return UserKnowledgeRecord{}, false, errors.New("user knowledge: topic is required")
	}
	if !validKnowledgeLevel(update.Level) {
		return UserKnowledgeRecord{}, false, errors.New("user knowledge: invalid level")
	}
	if !validConfidence(update.Confidence) {
		return UserKnowledgeRecord{}, false, errors.New("user knowledge: confidence must be between 0 and 1")
	}
	if !validSourceKind(update.Source.Kind) {
		return UserKnowledgeRecord{}, false, errors.New("user knowledge: invalid source kind")
	}
	if policy.MinEvidenceCount < 1 {
		return UserKnowledgeRecord{}, false, errors.New("user knowledge: minimum evidence count must be positive")
	}

	record := UserKnowledgeRecord{
		Topic:      strings.TrimSpace(update.Topic),
		Level:      update.Level,
		Confidence: update.Confidence,
		Source:     update.Source,
		Evidence:   append([]UserKnowledgeEvidence(nil), update.Evidence...),
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

// KnowledgeToCore projects to the minimal B96.34.1 contract. The richer
// record remains the canonical B96.34.5 semantic representation.
func (r UserKnowledgeRecord) KnowledgeToCore() UserKnowledgeItem {
	return UserKnowledgeItem{
		Key:        r.Topic,
		Value:      string(r.Level),
		Confidence: r.Confidence,
		Provenance: r.Source,
		Status:     r.Status,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

// ValidateUserKnowledgeRecord validates structure only; it does not determine
// whether the user actually has the claimed knowledge.
func ValidateUserKnowledgeRecord(r UserKnowledgeRecord) error {
	if strings.TrimSpace(r.Topic) == "" {
		return errors.New("user knowledge: topic is required")
	}
	if !validKnowledgeLevel(r.Level) {
		return fmt.Errorf("user knowledge %q: invalid level", r.Topic)
	}
	if !validConfidence(r.Confidence) {
		return fmt.Errorf("user knowledge %q: confidence must be between 0 and 1", r.Topic)
	}
	if !validSourceKind(r.Source.Kind) {
		return fmt.Errorf("user knowledge %q: invalid source kind", r.Topic)
	}
	if !validStatus(r.Status) {
		return fmt.Errorf("user knowledge %q: invalid status", r.Topic)
	}

	seen := make(map[string]struct{}, len(r.Evidence))
	for i, evidence := range r.Evidence {
		if strings.TrimSpace(evidence.ID) == "" {
			return fmt.Errorf("user knowledge %q: evidence %d has empty id", r.Topic, i)
		}
		if _, ok := seen[evidence.ID]; ok {
			return fmt.Errorf("user knowledge %q: duplicate evidence id %q", r.Topic, evidence.ID)
		}
		seen[evidence.ID] = struct{}{}
		if !validSourceKind(evidence.Source.Kind) {
			return fmt.Errorf("user knowledge %q: evidence %q has invalid source", r.Topic, evidence.ID)
		}
	}
	return nil
}

func validKnowledgeLevel(level UserKnowledgeLevel) bool {
	switch level {
	case UserKnowledgeUnknown, UserKnowledgeAware, UserKnowledgeFamiliar,
		UserKnowledgeWorking, UserKnowledgeAdvanced, UserKnowledgeExpert:
		return true
	default:
		return false
	}
}
