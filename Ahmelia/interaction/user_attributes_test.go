package interaction

import (
	"testing"
	"time"
)

func TestB96342_ExplicitAttributeCanBeEstablished(t *testing.T) {
	now := time.Now()
	record, accepted, err := EvaluateUserAttributeUpdate(UserAttributeUpdate{
		Key: "language", Value: "Spanish", Confidence: 1,
		Source:   UserModelSourceRef{Kind: UserSourceExplicitUser, ActorID: "user-1", EvidenceID: "turn-1"},
		Evidence: []UserAttributeEvidence{{ID: "turn-1", Source: UserModelSourceRef{Kind: UserSourceExplicitUser}, Statement: "Prefiero español", ObservedAt: now}}, Now: now,
	}, DefaultUserAttributePolicy())
	if err != nil || !accepted {
		t.Fatalf("expected explicit attribute acceptance: accepted=%v err=%v", accepted, err)
	}
	if record.Status != UserModelStatusActive || record.Source.Kind != UserSourceExplicitUser {
		t.Fatalf("unexpected explicit record: %+v", record)
	}
}

func TestB96342_SingleInferenceDoesNotBecomeFact(t *testing.T) {
	record, accepted, err := EvaluateUserAttributeUpdate(UserAttributeUpdate{
		Key: "technical_programming_experience", Value: "high", Confidence: 0.82,
		Source:   UserModelSourceRef{Kind: UserSourceInferred, EvidenceID: "obs-1"},
		Evidence: []UserAttributeEvidence{{ID: "obs-1", Source: UserModelSourceRef{Kind: UserSourceObserved}}}, Now: time.Now(),
	}, DefaultUserAttributePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatalf("single weak inference must not establish an attribute: %+v", record)
	}
	if record.Status != UserModelStatusUncertain {
		t.Fatalf("weak inference should remain uncertain: %+v", record)
	}
}

func TestB96342_RepeatedEvidenceCanSupportInference(t *testing.T) {
	record, accepted, err := EvaluateUserAttributeUpdate(UserAttributeUpdate{
		Key: "technical_programming_experience", Value: "high", Confidence: 0.90,
		Source: UserModelSourceRef{Kind: UserSourceInferred, EvidenceID: "pattern-1"},
		Evidence: []UserAttributeEvidence{
			{ID: "obs-1", Source: UserModelSourceRef{Kind: UserSourceObserved}},
			{ID: "obs-2", Source: UserModelSourceRef{Kind: UserSourceObserved}},
		}, Now: time.Now(),
	}, DefaultUserAttributePolicy())
	if err != nil || !accepted {
		t.Fatalf("expected repeated strong evidence to support inference: accepted=%v err=%v", accepted, err)
	}
	if record.Status != UserModelStatusUncertain {
		t.Fatalf("inferred attributes remain epistemically uncertain: %+v", record)
	}
}

func TestB96342_CoreConversionPreservesProvenance(t *testing.T) {
	now := time.Now()
	record := UserAttributeRecord{
		Key: "language", Value: "Spanish", Confidence: 1,
		Source: UserModelSourceRef{Kind: UserSourceExplicitUser, EvidenceID: "turn-2"},
		Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	core := record.AttributeToCore()
	if core.Key != record.Key || core.Value != record.Value || core.Provenance != record.Source || core.Status != record.Status {
		t.Fatalf("core conversion lost attribute metadata: core=%+v record=%+v", core, record)
	}
}

func TestB96342_InvalidAttributeRejected(t *testing.T) {
	err := ValidateUserAttributeRecord(UserAttributeRecord{Key: "x", Value: "y", Confidence: 1.2, Status: UserModelStatusActive, Source: UserModelSourceRef{Kind: UserSourceObserved}})
	if err == nil {
		t.Fatal("expected invalid confidence error")
	}
}
