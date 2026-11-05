package interaction

import (
	"testing"
	"time"
)

func TestB96343_ExplicitPreferenceCanBeEstablished(t *testing.T) {
	now := time.Now()
	record, accepted, err := EvaluateUserPreferenceUpdate(UserPreferenceUpdate{
		Key: "response_detail", Value: "high", Strength: 0.9, Confidence: 1,
		Source:   UserModelSourceRef{Kind: UserSourceExplicitUser, ActorID: "user-1", EvidenceID: "turn-1"},
		Evidence: []UserPreferenceEvidence{{ID: "turn-1", Source: UserModelSourceRef{Kind: UserSourceExplicitUser}, Statement: "Quiero explicaciones detalladas", ObservedAt: now}}, Now: now,
	}, DefaultUserPreferencePolicy())
	if err != nil || !accepted {
		t.Fatalf("expected explicit preference acceptance: accepted=%v err=%v", accepted, err)
	}
	if record.Status != UserModelStatusActive || record.Strength != 0.9 {
		t.Fatalf("unexpected explicit preference: %+v", record)
	}
}

func TestB96343_StrengthSupportsPreferenceAndAversion(t *testing.T) {
	positive := UserPreferenceRecord{Key: "humor", Value: "dry", Strength: 0.8, Confidence: 0.9, Source: UserModelSourceRef{Kind: UserSourceObserved}, Status: UserModelStatusActive}
	negative := UserPreferenceRecord{Key: "humor", Value: "slapstick", Strength: -0.8, Confidence: 0.9, Source: UserModelSourceRef{Kind: UserSourceObserved}, Status: UserModelStatusActive}
	if err := ValidateUserPreferenceRecord(positive); err != nil {
		t.Fatal(err)
	}
	if err := ValidateUserPreferenceRecord(negative); err != nil {
		t.Fatal(err)
	}
	if positive.Strength <= 0 || negative.Strength >= 0 {
		t.Fatal("expected signed strength to distinguish preference from aversion")
	}
}

func TestB96343_WeakInferenceDoesNotBecomeFact(t *testing.T) {
	record, accepted, err := EvaluateUserPreferenceUpdate(UserPreferenceUpdate{
		Key: "format", Value: "bullet_points", Strength: 0.7, Confidence: 0.80,
		Source:   UserModelSourceRef{Kind: UserSourceInferred, EvidenceID: "pattern-1"},
		Evidence: []UserPreferenceEvidence{{ID: "obs-1", Source: UserModelSourceRef{Kind: UserSourceObserved}}}, Now: time.Now(),
	}, DefaultUserPreferencePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("weak inference must remain an unaccepted uncertain candidate: accepted=%v record=%+v", accepted, record)
	}
}

func TestB96343_StrongRepeatedInferenceRemainsUncertain(t *testing.T) {
	record, accepted, err := EvaluateUserPreferenceUpdate(UserPreferenceUpdate{
		Key: "response_detail", Value: "high", Strength: 0.9, Confidence: 0.92,
		Source: UserModelSourceRef{Kind: UserSourceInferred, EvidenceID: "pattern-2"},
		Evidence: []UserPreferenceEvidence{
			{ID: "obs-1", Source: UserModelSourceRef{Kind: UserSourceObserved}},
			{ID: "obs-2", Source: UserModelSourceRef{Kind: UserSourceObserved}},
		}, Now: time.Now(),
	}, DefaultUserPreferencePolicy())
	if err != nil || !accepted {
		t.Fatalf("expected strong repeated inference to be accepted as candidate: accepted=%v err=%v", accepted, err)
	}
	if record.Status != UserModelStatusUncertain || record.Source.Kind != UserSourceInferred {
		t.Fatalf("inferred preference must retain uncertainty/provenance: %+v", record)
	}
}

func TestB96343_CoreConversionPreservesCoreMetadata(t *testing.T) {
	now := time.Now()
	record := UserPreferenceRecord{
		Key: "language", Value: "Spanish", Strength: 1, Confidence: 1,
		Source: UserModelSourceRef{Kind: UserSourceExplicitUser, EvidenceID: "turn-2"},
		Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	core := record.PreferenceToCore()
	if core.Key != record.Key || core.Value != record.Value || core.Confidence != record.Confidence || core.Provenance != record.Source || core.Status != record.Status {
		t.Fatalf("core conversion lost preference metadata: core=%+v record=%+v", core, record)
	}
}

func TestB96343_InvalidPreferenceRejected(t *testing.T) {
	err := ValidateUserPreferenceRecord(UserPreferenceRecord{
		Key: "x", Value: "y", Strength: 1.1, Confidence: 1,
		Status: UserModelStatusActive, Source: UserModelSourceRef{Kind: UserSourceObserved},
	})
	if err == nil {
		t.Fatal("expected invalid strength error")
	}
}
