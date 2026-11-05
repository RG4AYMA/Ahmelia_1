package interaction

import (
	"strings"
	"testing"
	"time"
)

func knowledgeEvidence(id string) UserKnowledgeEvidence {
	return UserKnowledgeEvidence{ID: id, Source: UserModelSourceRef{Kind: UserSourceObserved, SourceID: "turn-" + id}, ObservedAt: time.Unix(100, 0)}
}

func TestEvaluateUserKnowledgeUpdateExplicit(t *testing.T) {
	now := time.Unix(200, 0)
	record, accepted, err := EvaluateUserKnowledgeUpdate(UserKnowledgeUpdate{
		Topic:      "Go",
		Level:      UserKnowledgeWorking,
		Confidence: 0.7,
		Source:     UserModelSourceRef{Kind: UserSourceExplicitUser, ActorID: "user"},
		Now:        now,
	}, DefaultUserKnowledgePolicy())
	if err != nil || !accepted || record.Status != UserModelStatusActive {
		t.Fatalf("explicit knowledge = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestEvaluateUserKnowledgeUpdateWeakObservationRejected(t *testing.T) {
	record, accepted, err := EvaluateUserKnowledgeUpdate(UserKnowledgeUpdate{
		Topic:      "Rust",
		Level:      UserKnowledgeExpert,
		Confidence: 0.99,
		Source:     UserModelSourceRef{Kind: UserSourceObserved},
		Evidence:   []UserKnowledgeEvidence{knowledgeEvidence("one")},
	}, DefaultUserKnowledgePolicy())
	if err != nil || accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("weak observation = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestEvaluateUserKnowledgeUpdateInferenceRemainsUncertain(t *testing.T) {
	record, accepted, err := EvaluateUserKnowledgeUpdate(UserKnowledgeUpdate{
		Topic:      "distributed systems",
		Level:      UserKnowledgeAdvanced,
		Confidence: 0.91,
		Source:     UserModelSourceRef{Kind: UserSourceInferred},
		Evidence:   []UserKnowledgeEvidence{knowledgeEvidence("a"), knowledgeEvidence("b")},
	}, DefaultUserKnowledgePolicy())
	if err != nil || !accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("inferred knowledge = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestValidateUserKnowledgeRecordRejectsDuplicateEvidence(t *testing.T) {
	record := UserKnowledgeRecord{
		Topic: "databases", Level: UserKnowledgeFamiliar, Confidence: 0.8,
		Source:   UserModelSourceRef{Kind: UserSourceObserved},
		Evidence: []UserKnowledgeEvidence{knowledgeEvidence("same"), knowledgeEvidence("same")},
		Status:   UserModelStatusUncertain,
	}
	if err := ValidateUserKnowledgeRecord(record); err == nil || !strings.Contains(err.Error(), "duplicate evidence") {
		t.Fatalf("expected duplicate evidence error, got %v", err)
	}
}

func TestKnowledgeLevelsAreDistinctFromCapability(t *testing.T) {
	if string(UserKnowledgeWorking) == "can_execute" {
		t.Fatal("knowledge level must not encode execution capability")
	}
	record := UserKnowledgeRecord{Topic: "Python", Level: UserKnowledgeAdvanced, Confidence: 1, Source: UserModelSourceRef{Kind: UserSourceExplicitUser}, Status: UserModelStatusActive}
	core := record.KnowledgeToCore()
	if core.Key != "Python" || core.Value != string(UserKnowledgeAdvanced) {
		t.Fatalf("core projection lost knowledge semantics: %#v", core)
	}
}

func TestAllKnowledgeLevelsValidate(t *testing.T) {
	levels := []UserKnowledgeLevel{UserKnowledgeUnknown, UserKnowledgeAware, UserKnowledgeFamiliar, UserKnowledgeWorking, UserKnowledgeAdvanced, UserKnowledgeExpert}
	for _, level := range levels {
		if !validKnowledgeLevel(level) {
			t.Fatalf("level %q should validate", level)
		}
	}
}
