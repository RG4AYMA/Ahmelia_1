package interaction

import (
	"strings"
	"testing"
	"time"
)

func goalEvidence(id string) UserGoalEvidence {
	return UserGoalEvidence{ID: id, Source: UserModelSourceRef{Kind: UserSourceObserved, SourceID: "turn-" + id}, ObservedAt: time.Unix(100, 0)}
}

func TestEvaluateUserGoalUpdateExplicitAspirationalDoesNotBecomeActiveGoal(t *testing.T) {
	// Mirrors the design example directly: "Algún día me gustaría hacer X"
	// should record as an Aspirational-category goal, not an active one, even
	// though the explicit statement itself is trusted at face value.
	now := time.Unix(200, 0)
	record, accepted, err := EvaluateUserGoalUpdate(UserGoalUpdate{
		Goal:       "aprender Unreal Engine a fondo",
		Category:   UserGoalCategoryAspirational,
		Confidence: 0.76,
		Source:     UserModelSourceRef{Kind: UserSourceExplicitUser, ActorID: "user"},
		Now:        now,
	}, DefaultUserGoalPolicy())
	if err != nil || !accepted {
		t.Fatalf("explicit aspirational goal rejected: accepted=%v err=%v", accepted, err)
	}
	if record.Status != UserModelStatusActive {
		t.Fatalf("expected record status active (we trust they said it), got %v", record.Status)
	}
	if record.Category != UserGoalCategoryAspirational {
		t.Fatalf("expected category to remain aspirational, got %v", record.Category)
	}
}

func TestEvaluateUserGoalUpdateWeakObservationRejected(t *testing.T) {
	record, accepted, err := EvaluateUserGoalUpdate(UserGoalUpdate{
		Goal:       "migrar todo el stack a Rust",
		Category:   UserGoalCategoryLongTerm,
		Confidence: 0.6,
		Source:     UserModelSourceRef{Kind: UserSourceObserved},
		Evidence:   []UserGoalEvidence{goalEvidence("one")},
	}, DefaultUserGoalPolicy())
	if err != nil || accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("weak observation = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestEvaluateUserGoalUpdateInferredNonActiveRemainsUncertain(t *testing.T) {
	record, accepted, err := EvaluateUserGoalUpdate(UserGoalUpdate{
		Goal:       "consolidar Ahmelia como asistente multi-dominio",
		Category:   UserGoalCategoryLongTerm,
		Confidence: 0.87,
		Source:     UserModelSourceRef{Kind: UserSourceInferred},
		Evidence:   []UserGoalEvidence{goalEvidence("a"), goalEvidence("b")},
	}, DefaultUserGoalPolicy())
	if err != nil || !accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("inferred long_term goal = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestEvaluateUserGoalUpdateInferredActiveRequiresStrongerEvidenceRejected(t *testing.T) {
	// Same confidence/evidence count that is enough for a LongTerm inferred
	// goal must NOT be enough to establish an Active goal: a casual pattern
	// of comments should not silently become ACTIVE_GOAL.
	record, accepted, err := EvaluateUserGoalUpdate(UserGoalUpdate{
		Goal:       "reescribir el motor de razonamiento",
		Category:   UserGoalCategoryActive,
		Confidence: 0.9,
		Source:     UserModelSourceRef{Kind: UserSourceInferred},
		Evidence:   []UserGoalEvidence{goalEvidence("a"), goalEvidence("b")},
	}, DefaultUserGoalPolicy())
	if err != nil || accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("under-threshold active inference = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestEvaluateUserGoalUpdateObservedActiveWithSufficientEvidenceAccepted(t *testing.T) {
	record, accepted, err := EvaluateUserGoalUpdate(UserGoalUpdate{
		Goal:       "cerrar el bloque B96.34 completo",
		Category:   UserGoalCategoryActive,
		Confidence: 0.95,
		Source:     UserModelSourceRef{Kind: UserSourceObserved},
		Evidence: []UserGoalEvidence{
			goalEvidence("a"), goalEvidence("b"), goalEvidence("c"), goalEvidence("d"),
		},
	}, DefaultUserGoalPolicy())
	if err != nil || !accepted || record.Status != UserModelStatusActive {
		t.Fatalf("observed active goal with strong evidence = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestValidateUserGoalRecordRejectsDuplicateEvidence(t *testing.T) {
	record := UserGoalRecord{
		Goal:       "dominar Blender",
		Category:   UserGoalCategoryAspirational,
		Confidence: 0.7,
		Source:     UserModelSourceRef{Kind: UserSourceObserved},
		Evidence:   []UserGoalEvidence{goalEvidence("same"), goalEvidence("same")},
		Status:     UserModelStatusUncertain,
	}
	if err := ValidateUserGoalRecord(record); err == nil || !strings.Contains(err.Error(), "duplicate evidence") {
		t.Fatalf("expected duplicate evidence error, got %v", err)
	}
}

func TestGoalRecordProjectsToCore(t *testing.T) {
	record := UserGoalRecord{
		Goal:       "lanzar B96.24 Autonomous Loop",
		Category:   UserGoalCategoryLongTerm,
		Confidence: 1,
		Source:     UserModelSourceRef{Kind: UserSourceExplicitUser},
		Status:     UserModelStatusActive,
	}
	core := record.GoalToCore()
	if core.Key != "lanzar B96.24 Autonomous Loop" || core.Value != string(UserGoalCategoryLongTerm) {
		t.Fatalf("core projection lost goal semantics: %#v", core)
	}
}

func TestAllGoalCategoriesValidate(t *testing.T) {
	categories := []UserGoalCategory{
		UserGoalCategoryActive, UserGoalCategoryLongTerm, UserGoalCategoryAspirational, UserGoalCategoryCompleted,
	}
	for _, category := range categories {
		if !validGoalCategory(category) {
			t.Fatalf("category %q should validate", category)
		}
	}
}

func TestEvaluateUserGoalUpdateRejectsInvalidCategory(t *testing.T) {
	_, accepted, err := EvaluateUserGoalUpdate(UserGoalUpdate{
		Goal:       "x",
		Category:   UserGoalCategory("someday_maybe"),
		Confidence: 1,
		Source:     UserModelSourceRef{Kind: UserSourceExplicitUser},
	}, DefaultUserGoalPolicy())
	if accepted || err == nil {
		t.Fatalf("expected rejection for invalid category, got accepted=%v err=%v", accepted, err)
	}
}

func TestDefaultUserGoalPolicyIsInternallyConsistent(t *testing.T) {
	policy := DefaultUserGoalPolicy()
	if policy.MinActiveCategoryEvidenceCount < policy.MinEvidenceCount {
		t.Fatalf("active category bar (%d) must be >= base evidence bar (%d)",
			policy.MinActiveCategoryEvidenceCount, policy.MinEvidenceCount)
	}
}
