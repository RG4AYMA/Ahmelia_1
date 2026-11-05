package interaction

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// UserGoalCategory expresses the nature and commitment level of a goal. It is
// independent from UserModelItemStatus: Category says what kind of goal this
// is (a casual aspiration is not the same claim as an active goal driving
// present behavior); Status says whether Ahmelia currently trusts the belief.
// A single explicit statement like "algún día me gustaría hacer X" can
// legitimately produce an Active-status record whose Category is
// Aspirational — the user really did say that — without ever promoting the
// goal itself to Active category on the strength of one remark.
type UserGoalCategory string

const (
	UserGoalCategoryActive       UserGoalCategory = "active"
	UserGoalCategoryLongTerm     UserGoalCategory = "long_term"
	UserGoalCategoryAspirational UserGoalCategory = "aspirational"
	UserGoalCategoryCompleted    UserGoalCategory = "completed"
)

// UserGoalEvidence records the basis for a goal belief.
type UserGoalEvidence struct {
	ID         string             `json:"id"`
	Source     UserModelSourceRef `json:"source"`
	Statement  string             `json:"statement,omitempty"`
	ObservedAt time.Time          `json:"observed_at"`
}

// UserGoalRecord is the richer B96.34.7 representation. Confidence says how
// strongly Ahmelia believes the record; Category says what kind of goal is
// believed. These dimensions must not be conflated with each other, nor with
// UserKnowledgeRecord.Level or UserCapabilityRecord.Level.
type UserGoalRecord struct {
	Goal       string              `json:"goal"`
	Category   UserGoalCategory    `json:"category"`
	Confidence float64             `json:"confidence"`
	Source     UserModelSourceRef  `json:"source"`
	Evidence   []UserGoalEvidence  `json:"evidence,omitempty"`
	Status     UserModelItemStatus `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

// UserGoalUpdate proposes a goal record without mutating the model.
type UserGoalUpdate struct {
	Goal       string
	Category   UserGoalCategory
	Confidence float64
	Source     UserModelSourceRef
	Evidence   []UserGoalEvidence
	Now        time.Time
}

// UserGoalPolicy controls only local admission of goal proposals. Cross-item
// update/reconciliation remains B96.34.11.
//
// MinActiveCategoryEvidenceCount exists because "not every comment becomes a
// goal" is explicitly a design requirement: a single behavioral observation
// or inference must not be enough to establish an ACTIVE goal, since active
// goals are expected to eventually drive autonomous planning (B96.24). A
// non-explicit source proposing Category Active is held to a stricter
// evidence bar than the same source proposing LongTerm, Aspirational, or
// Completed.
type UserGoalPolicy struct {
	MinObservedConfidence          float64
	MinInferredConfidence          float64
	MinEvidenceCount               int
	MinActiveCategoryEvidenceCount int
}

func DefaultUserGoalPolicy() UserGoalPolicy {
	return UserGoalPolicy{
		MinObservedConfidence:          0.90,
		MinInferredConfidence:          0.85,
		MinEvidenceCount:               2,
		MinActiveCategoryEvidenceCount: 4,
	}
}

// EvaluateUserGoalUpdate validates and classifies a proposal. Inferred or
// observed goals can be accepted as candidates, but non-explicit proposals of
// Category Active must clear a stricter evidence bar than other categories,
// and insufficient evidence never establishes an active record by itself.
func EvaluateUserGoalUpdate(update UserGoalUpdate, policy UserGoalPolicy) (UserGoalRecord, bool, error) {
	if strings.TrimSpace(update.Goal) == "" {
		return UserGoalRecord{}, false, errors.New("user goal: goal is required")
	}
	if !validGoalCategory(update.Category) {
		return UserGoalRecord{}, false, errors.New("user goal: invalid category")
	}
	if !validConfidence(update.Confidence) {
		return UserGoalRecord{}, false, errors.New("user goal: confidence must be between 0 and 1")
	}
	if !validSourceKind(update.Source.Kind) {
		return UserGoalRecord{}, false, errors.New("user goal: invalid source kind")
	}
	if policy.MinEvidenceCount < 1 {
		return UserGoalRecord{}, false, errors.New("user goal: minimum evidence count must be positive")
	}
	if policy.MinActiveCategoryEvidenceCount < policy.MinEvidenceCount {
		return UserGoalRecord{}, false, errors.New("user goal: active category evidence bar must be at least the base evidence bar")
	}

	record := UserGoalRecord{
		Goal:       strings.TrimSpace(update.Goal),
		Category:   update.Category,
		Confidence: update.Confidence,
		Source:     update.Source,
		Evidence:   append([]UserGoalEvidence(nil), update.Evidence...),
		Status:     UserModelStatusUncertain,
		CreatedAt:  update.Now,
		UpdatedAt:  update.Now,
	}

	switch update.Source.Kind {
	case UserSourceExplicitUser:
		// The user's own words about their own goals are trusted directly,
		// whatever category they describe (active, aspirational, etc.).
		record.Status = UserModelStatusActive
		return record, true, nil
	case UserSourceObserved:
		requiredEvidence := policy.MinEvidenceCount
		if update.Category == UserGoalCategoryActive {
			requiredEvidence = policy.MinActiveCategoryEvidenceCount
		}
		if update.Confidence >= policy.MinObservedConfidence && len(update.Evidence) >= requiredEvidence {
			record.Status = UserModelStatusActive
			return record, true, nil
		}
	case UserSourceInferred:
		requiredEvidence := policy.MinEvidenceCount
		if update.Category == UserGoalCategoryActive {
			requiredEvidence = policy.MinActiveCategoryEvidenceCount
		}
		if update.Confidence >= policy.MinInferredConfidence && len(update.Evidence) >= requiredEvidence {
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

// GoalToCore projects to the minimal B96.34.1 contract. The richer record
// remains the canonical B96.34.7 semantic representation.
func (r UserGoalRecord) GoalToCore() UserGoal {
	return UserGoal{
		Key:        r.Goal,
		Value:      string(r.Category),
		Confidence: r.Confidence,
		Provenance: r.Source,
		Status:     r.Status,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

// ValidateUserGoalRecord validates structure only; it does not determine
// whether the user actually holds the claimed goal.
func ValidateUserGoalRecord(r UserGoalRecord) error {
	if strings.TrimSpace(r.Goal) == "" {
		return errors.New("user goal: goal is required")
	}
	if !validGoalCategory(r.Category) {
		return fmt.Errorf("user goal %q: invalid category", r.Goal)
	}
	if !validConfidence(r.Confidence) {
		return fmt.Errorf("user goal %q: confidence must be between 0 and 1", r.Goal)
	}
	if !validSourceKind(r.Source.Kind) {
		return fmt.Errorf("user goal %q: invalid source kind", r.Goal)
	}
	if !validStatus(r.Status) {
		return fmt.Errorf("user goal %q: invalid status", r.Goal)
	}

	seen := make(map[string]struct{}, len(r.Evidence))
	for i, evidence := range r.Evidence {
		if strings.TrimSpace(evidence.ID) == "" {
			return fmt.Errorf("user goal %q: evidence %d has empty id", r.Goal, i)
		}
		if _, ok := seen[evidence.ID]; ok {
			return fmt.Errorf("user goal %q: duplicate evidence id %q", r.Goal, evidence.ID)
		}
		seen[evidence.ID] = struct{}{}
		if !validSourceKind(evidence.Source.Kind) {
			return fmt.Errorf("user goal %q: evidence %q has invalid source", r.Goal, evidence.ID)
		}
	}
	return nil
}

func validGoalCategory(category UserGoalCategory) bool {
	switch category {
	case UserGoalCategoryActive, UserGoalCategoryLongTerm, UserGoalCategoryAspirational, UserGoalCategoryCompleted:
		return true
	default:
		return false
	}
}
