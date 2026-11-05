package interaction

import (
	"strings"
	"testing"
	"time"
)

func TestReinforceUserInteractionPatternFirstObservationStartsUncertain(t *testing.T) {
	record, err := ReinforceUserInteractionPattern(UserInteractionPatternRecord{}, UserInteractionPatternObservation{
		Pattern:    "incremental_workflow",
		Confidence: 0.6,
		Source:     UserModelSourceRef{Kind: UserSourceObserved},
		Now:        time.Unix(100, 0),
	}, DefaultUserInteractionPatternPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if record.EvidenceCount != 1 || record.Confidence != 0.6 || record.Status != UserModelStatusUncertain {
		t.Fatalf("unexpected first observation record: %#v", record)
	}
}

func TestReinforceUserInteractionPatternExplicitStartsActive(t *testing.T) {
	record, err := ReinforceUserInteractionPattern(UserInteractionPatternRecord{}, UserInteractionPatternObservation{
		Pattern:    "incremental_workflow",
		Confidence: 0.7,
		Source:     UserModelSourceRef{Kind: UserSourceExplicitUser, ActorID: "user"},
		Now:        time.Unix(100, 0),
	}, DefaultUserInteractionPatternPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != UserModelStatusActive {
		t.Fatalf("expected explicit statement to establish active pattern, got %v", record.Status)
	}
}

func TestReinforceUserInteractionPatternAccumulatesEvidenceCountAndAveragesConfidence(t *testing.T) {
	policy := DefaultUserInteractionPatternPolicy()
	record, err := ReinforceUserInteractionPattern(UserInteractionPatternRecord{}, UserInteractionPatternObservation{
		Pattern: "iterates_rapidly", Confidence: 0.5, Source: UserModelSourceRef{Kind: UserSourceObserved}, Now: time.Unix(100, 0),
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	record, err = ReinforceUserInteractionPattern(record, UserInteractionPatternObservation{
		Pattern: "iterates_rapidly", Confidence: 0.9, Source: UserModelSourceRef{Kind: UserSourceObserved}, Now: time.Unix(200, 0),
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if record.EvidenceCount != 2 {
		t.Fatalf("expected evidence count 2, got %d", record.EvidenceCount)
	}
	wantConfidence := (0.5 + 0.9) / 2
	if diff := record.Confidence - wantConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("expected averaged confidence %.4f, got %.4f", wantConfidence, record.Confidence)
	}
	// A single strong observation must not swing an established pattern into
	// certainty on its own.
	if record.Confidence >= 0.9 {
		t.Fatalf("single observation swung confidence too far: %v", record.Confidence)
	}
}

func TestReinforceUserInteractionPatternBecomesActiveAfterEnoughEvidence(t *testing.T) {
	policy := DefaultUserInteractionPatternPolicy() // needs 5 evidence, >=0.80 confidence
	var record UserInteractionPatternRecord
	var err error
	for i := 0; i < 5; i++ {
		record, err = ReinforceUserInteractionPattern(record, UserInteractionPatternObservation{
			Pattern: "validates_with_tests", Confidence: 0.9, Source: UserModelSourceRef{Kind: UserSourceObserved}, Now: time.Unix(int64(100+i), 0),
		}, policy)
		if err != nil {
			t.Fatal(err)
		}
	}
	if record.EvidenceCount != 5 {
		t.Fatalf("expected evidence count 5, got %d", record.EvidenceCount)
	}
	if record.Status != UserModelStatusActive {
		t.Fatalf("expected pattern to become active after sustained strong evidence, got %v", record.Status)
	}
}

func TestReinforceUserInteractionPatternStaysUncertainBelowThreshold(t *testing.T) {
	policy := DefaultUserInteractionPatternPolicy()
	var record UserInteractionPatternRecord
	var err error
	for i := 0; i < 4; i++ {
		record, err = ReinforceUserInteractionPattern(record, UserInteractionPatternObservation{
			Pattern: "prefers_deep_reasoning_first", Confidence: 0.9, Source: UserModelSourceRef{Kind: UserSourceObserved}, Now: time.Unix(int64(100+i), 0),
		}, policy)
		if err != nil {
			t.Fatal(err)
		}
	}
	if record.Status != UserModelStatusUncertain {
		t.Fatalf("expected pattern to remain uncertain below evidence threshold (count=%d), got %v", record.EvidenceCount, record.Status)
	}
}

func TestReinforceUserInteractionPatternRejectsPatternNameMismatch(t *testing.T) {
	existing := UserInteractionPatternRecord{Pattern: "incremental_workflow", EvidenceCount: 3, Confidence: 0.6}
	_, err := ReinforceUserInteractionPattern(existing, UserInteractionPatternObservation{
		Pattern: "big_bang_rewrites", Confidence: 0.6, Source: UserModelSourceRef{Kind: UserSourceObserved}, Now: time.Unix(100, 0),
	}, DefaultUserInteractionPatternPolicy())
	if err == nil {
		t.Fatal("expected error for mismatched pattern name")
	}
}

func TestReinforceUserInteractionPatternRetainsBoundedEvidenceSample(t *testing.T) {
	policy := DefaultUserInteractionPatternPolicy() // MaxRetainedEvidence: 10
	var record UserInteractionPatternRecord
	var err error
	for i := 0; i < 15; i++ {
		id := "e" + string(rune('a'+i))
		record, err = ReinforceUserInteractionPattern(record, UserInteractionPatternObservation{
			Pattern: "incremental_workflow", Confidence: 0.85, Source: UserModelSourceRef{Kind: UserSourceObserved},
			Evidence: &UserInteractionPatternEvidence{ID: id, Source: UserModelSourceRef{Kind: UserSourceObserved}, ObservedAt: time.Unix(int64(100+i), 0)},
			Now:      time.Unix(int64(100+i), 0),
		}, policy)
		if err != nil {
			t.Fatal(err)
		}
	}
	if record.EvidenceCount != 15 {
		t.Fatalf("expected evidence_count to keep the true total (15), got %d", record.EvidenceCount)
	}
	if len(record.Evidence) != policy.MaxRetainedEvidence {
		t.Fatalf("expected retained sample capped at %d, got %d", policy.MaxRetainedEvidence, len(record.Evidence))
	}
	// FIFO: the oldest retained item should be the 6th observation (index 5),
	// since only the most recent MaxRetainedEvidence are kept.
	if record.Evidence[0].ID != "ef" {
		t.Fatalf("expected oldest retained evidence to be 'ef', got %q", record.Evidence[0].ID)
	}
}

func TestValidateUserInteractionPatternRecordRejectsZeroEvidenceCount(t *testing.T) {
	record := UserInteractionPatternRecord{
		Pattern: "incremental_workflow", Confidence: 0.7, EvidenceCount: 0,
		Source: UserModelSourceRef{Kind: UserSourceObserved}, Status: UserModelStatusUncertain,
	}
	if err := ValidateUserInteractionPatternRecord(record); err == nil || !strings.Contains(err.Error(), "evidence_count") {
		t.Fatalf("expected evidence_count error, got %v", err)
	}
}

func TestValidateUserInteractionPatternRecordRejectsDuplicateEvidence(t *testing.T) {
	dup := UserInteractionPatternEvidence{ID: "same", Source: UserModelSourceRef{Kind: UserSourceObserved}}
	record := UserInteractionPatternRecord{
		Pattern: "incremental_workflow", Confidence: 0.7, EvidenceCount: 2,
		Source: UserModelSourceRef{Kind: UserSourceObserved}, Status: UserModelStatusUncertain,
		Evidence: []UserInteractionPatternEvidence{dup, dup},
	}
	if err := ValidateUserInteractionPatternRecord(record); err == nil || !strings.Contains(err.Error(), "duplicate evidence") {
		t.Fatalf("expected duplicate evidence error, got %v", err)
	}
}

func TestPatternRecordProjectsToCore(t *testing.T) {
	record := UserInteractionPatternRecord{
		Pattern: "incremental_workflow", Confidence: 0.88, EvidenceCount: 37,
		Source: UserModelSourceRef{Kind: UserSourceObserved}, Status: UserModelStatusActive,
	}
	core := record.PatternToCore()
	if core.Pattern != "incremental_workflow" || core.EvidenceCount != 37 || core.Confidence != 0.88 {
		t.Fatalf("core projection lost pattern semantics: %#v", core)
	}
}

func TestInteractionPatternProjectsIntoUserModelValidly(t *testing.T) {
	record := UserInteractionPatternRecord{
		Pattern: "incremental_workflow", Confidence: 0.88, EvidenceCount: 37,
		Source: UserModelSourceRef{Kind: UserSourceObserved}, Status: UserModelStatusActive,
	}
	m := NewUserModel("user-1")
	m.InteractionPatterns = append(m.InteractionPatterns, record.PatternToCore())
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid model with interaction pattern, got %v", err)
	}
}
