package interaction

import (
	"testing"
	"time"
)

func TestB96344_ExplicitCommunicationPreferenceCanBeEstablished(t *testing.T) {
	now := time.Now()
	value, accepted, err := EvaluateUserCommunicationProfileUpdate(UserCommunicationProfileUpdate{
		Dimension: CommunicationDetailTolerance, Value: "high", Confidence: 1,
		Source:   UserModelSourceRef{Kind: UserSourceExplicitUser, ActorID: "user-1", EvidenceID: "turn-1"},
		Evidence: []UserCommunicationEvidence{{ID: "turn-1", Source: UserModelSourceRef{Kind: UserSourceExplicitUser}, Statement: "Prefiero explicaciones detalladas", ObservedAt: now}},
		Now:      now,
	}, DefaultUserCommunicationProfilePolicy())
	if err != nil || !accepted {
		t.Fatalf("expected explicit communication preference acceptance: accepted=%v err=%v", accepted, err)
	}
	if value.Status != UserModelStatusActive || value.Dimension != CommunicationDetailTolerance {
		t.Fatalf("unexpected communication profile value: %+v", value)
	}
}

func TestB96344_WeakObservationDoesNotBecomeTrait(t *testing.T) {
	value, accepted, err := EvaluateUserCommunicationProfileUpdate(UserCommunicationProfileUpdate{
		Dimension: CommunicationHumorTolerance, Value: "high", Confidence: 0.70,
		Source:   UserModelSourceRef{Kind: UserSourceObserved, EvidenceID: "obs-1"},
		Evidence: []UserCommunicationEvidence{{ID: "obs-1", Source: UserModelSourceRef{Kind: UserSourceObserved}}},
		Now:      time.Now(),
	}, DefaultUserCommunicationProfilePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if accepted || value.Status != UserModelStatusUncertain {
		t.Fatalf("weak observation must remain uncertain: accepted=%v value=%+v", accepted, value)
	}
}

func TestB96344_StrongInferenceRemainsUncertain(t *testing.T) {
	value, accepted, err := EvaluateUserCommunicationProfileUpdate(UserCommunicationProfileUpdate{
		Dimension: CommunicationDirectness, Value: "high", Confidence: 0.92,
		Source: UserModelSourceRef{Kind: UserSourceInferred, EvidenceID: "pattern-1"},
		Evidence: []UserCommunicationEvidence{
			{ID: "obs-1", Source: UserModelSourceRef{Kind: UserSourceObserved}},
			{ID: "obs-2", Source: UserModelSourceRef{Kind: UserSourceObserved}},
		},
		Now: time.Now(),
	}, DefaultUserCommunicationProfilePolicy())
	if err != nil || !accepted {
		t.Fatalf("expected strong inference to be accepted as candidate: accepted=%v err=%v", accepted, err)
	}
	if value.Status != UserModelStatusUncertain || value.Source.Kind != UserSourceInferred {
		t.Fatalf("inference must retain uncertainty/provenance: %+v", value)
	}
}

func TestB96344_AllCommunicationDimensionsAreExplicit(t *testing.T) {
	dimensions := []UserCommunicationDimension{
		CommunicationFormality, CommunicationDirectness, CommunicationDetailTolerance,
		CommunicationTechnicalDepth, CommunicationHumorTolerance, CommunicationCorrectionPreference,
		CommunicationExplanationPreference, CommunicationInitiativePreference,
	}
	profile := NewUserCommunicationProfile()
	for _, dimension := range dimensions {
		profile.Values[dimension] = UserCommunicationProfileValue{
			Dimension: dimension, Value: "medium", Confidence: 0.9,
			Source: UserModelSourceRef{Kind: UserSourceObserved}, Status: UserModelStatusUncertain,
		}
	}
	if len(profile.Values) != 8 {
		t.Fatalf("expected 8 communication dimensions, got %d", len(profile.Values))
	}
	for _, dimension := range dimensions {
		if err := ValidateUserCommunicationProfileValue(profile.Values[dimension]); err != nil {
			t.Fatalf("dimension %q invalid: %v", dimension, err)
		}
	}
}

func TestB96344_CoreProjectionPreservesCanonicalFields(t *testing.T) {
	now := time.Now()
	profile := UserCommunicationProfile{Values: map[UserCommunicationDimension]UserCommunicationProfileValue{
		CommunicationTechnicalDepth: {
			Dimension: CommunicationTechnicalDepth, Value: "advanced", Confidence: 0.95,
			Source: UserModelSourceRef{Kind: UserSourceExplicitUser, EvidenceID: "turn-2"},
			Status: UserModelStatusActive, UpdatedAt: now,
		},
	}}
	core := profile.CommunicationProfileToCore()
	got, ok := core.Values[string(CommunicationTechnicalDepth)]
	if !ok {
		t.Fatal("expected technical depth in core projection")
	}
	if got.Value != "advanced" || got.Confidence != 0.95 || got.Provenance != profile.Values[CommunicationTechnicalDepth].Source || got.Status != UserModelStatusActive {
		t.Fatalf("core projection lost metadata: got=%+v", got)
	}
}

func TestB96344_DuplicateEvidenceRejected(t *testing.T) {
	err := ValidateUserCommunicationProfileValue(UserCommunicationProfileValue{
		Dimension: CommunicationFormality, Value: "formal", Confidence: 1,
		Source: UserModelSourceRef{Kind: UserSourceExplicitUser}, Status: UserModelStatusActive,
		Evidence: []UserCommunicationEvidence{
			{ID: "same", Source: UserModelSourceRef{Kind: UserSourceExplicitUser}},
			{ID: "same", Source: UserModelSourceRef{Kind: UserSourceExplicitUser}},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate evidence to be rejected")
	}
}
