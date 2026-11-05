package interaction

import (
	"strings"
	"testing"
	"time"
)

func capabilityEvidence(id string) UserCapabilityEvidence {
	return UserCapabilityEvidence{ID: id, Source: UserModelSourceRef{Kind: UserSourceObserved, SourceID: "turn-" + id}, ObservedAt: time.Unix(100, 0)}
}

func TestEvaluateUserCapabilityUpdateExplicit(t *testing.T) {
	now := time.Unix(200, 0)
	record, accepted, err := EvaluateUserCapabilityUpdate(UserCapabilityUpdate{
		Capability: "go_backend_development",
		Level:      UserCapabilityIndependent,
		Confidence: 0.7,
		Source:     UserModelSourceRef{Kind: UserSourceExplicitUser, ActorID: "user"},
		Now:        now,
	}, DefaultUserCapabilityPolicy())
	if err != nil || !accepted || record.Status != UserModelStatusActive {
		t.Fatalf("explicit capability = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestEvaluateUserCapabilityUpdateWeakObservationRejected(t *testing.T) {
	record, accepted, err := EvaluateUserCapabilityUpdate(UserCapabilityUpdate{
		Capability: "blender_rigging",
		Level:      UserCapabilityExpert,
		Confidence: 0.95,
		Source:     UserModelSourceRef{Kind: UserSourceObserved},
		Evidence:   []UserCapabilityEvidence{capabilityEvidence("one")},
	}, DefaultUserCapabilityPolicy())
	if err != nil || accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("weak observation = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestEvaluateUserCapabilityUpdateInferenceRemainsUncertain(t *testing.T) {
	record, accepted, err := EvaluateUserCapabilityUpdate(UserCapabilityUpdate{
		Capability: "unity_gameplay_scripting",
		Level:      UserCapabilityProficient,
		Confidence: 0.9,
		Source:     UserModelSourceRef{Kind: UserSourceInferred},
		Evidence:   []UserCapabilityEvidence{capabilityEvidence("a"), capabilityEvidence("b"), capabilityEvidence("c")},
	}, DefaultUserCapabilityPolicy())
	if err != nil || !accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("inferred capability = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestEvaluateUserCapabilityUpdateInferenceBelowStricterThresholdRejected(t *testing.T) {
	// Same confidence and evidence count that would satisfy the knowledge
	// policy (0.85, 2 items) must NOT satisfy the stricter capability policy.
	record, accepted, err := EvaluateUserCapabilityUpdate(UserCapabilityUpdate{
		Capability: "unreal_blueprint_scripting",
		Level:      UserCapabilityIndependent,
		Confidence: 0.85,
		Source:     UserModelSourceRef{Kind: UserSourceInferred},
		Evidence:   []UserCapabilityEvidence{capabilityEvidence("x"), capabilityEvidence("y")},
	}, DefaultUserCapabilityPolicy())
	if err != nil || accepted || record.Status != UserModelStatusUncertain {
		t.Fatalf("under-threshold inference = %#v, accepted=%v, err=%v", record, accepted, err)
	}
}

func TestValidateUserCapabilityRecordRejectsDuplicateEvidence(t *testing.T) {
	record := UserCapabilityRecord{
		Capability: "database_migration",
		Level:      UserCapabilityAssisted,
		Confidence: 0.8,
		Source:     UserModelSourceRef{Kind: UserSourceObserved},
		Evidence:   []UserCapabilityEvidence{capabilityEvidence("same"), capabilityEvidence("same")},
		Status:     UserModelStatusUncertain,
	}
	if err := ValidateUserCapabilityRecord(record); err == nil || !strings.Contains(err.Error(), "duplicate evidence") {
		t.Fatalf("expected duplicate evidence error, got %v", err)
	}
}

func TestCapabilityVocabularyIsNotACopyOfKnowledgeVocabulary(t *testing.T) {
	// Knowledge and capability are different Go types (UserKnowledgeLevel vs
	// UserCapabilityLevel), used in different record types, so a shared word
	// like "expert" at the top of both scales is not a type-safety risk.
	// What this guards against is a copy-paste that left capability using the
	// exact same graded vocabulary as knowledge, which would signal the two
	// dimensions were never actually differentiated ("can do it" vs
	// "understands it").
	knowledgeLevels := []string{
		string(UserKnowledgeUnknown), string(UserKnowledgeAware), string(UserKnowledgeFamiliar),
		string(UserKnowledgeWorking), string(UserKnowledgeAdvanced), string(UserKnowledgeExpert),
	}
	capabilityLevels := []string{
		string(UserCapabilityNone), string(UserCapabilityAssisted), string(UserCapabilityIndependent),
		string(UserCapabilityProficient), string(UserCapabilityExpert),
	}
	if strings.Join(knowledgeLevels, ",") == strings.Join(capabilityLevels, ",") {
		t.Fatal("capability vocabulary must not be an undifferentiated copy of the knowledge vocabulary")
	}
	// Type safety itself is the real boundary: a knowledge level cannot be
	// assigned where a capability level is expected without an explicit cast.
	var _ UserCapabilityLevel = UserCapabilityLevel(UserKnowledgeExpert) // compiles only via explicit conversion
}

func TestCapabilityRecordProjectsToCore(t *testing.T) {
	record := UserCapabilityRecord{
		Capability: "python_scripting",
		Level:      UserCapabilityExpert,
		Confidence: 1,
		Source:     UserModelSourceRef{Kind: UserSourceExplicitUser},
		Status:     UserModelStatusActive,
	}
	core := record.CapabilityToCore()
	if core.Key != "python_scripting" || core.Value != string(UserCapabilityExpert) {
		t.Fatalf("core projection lost capability semantics: %#v", core)
	}
}

func TestAllCapabilityLevelsValidate(t *testing.T) {
	levels := []UserCapabilityLevel{
		UserCapabilityNone, UserCapabilityAssisted, UserCapabilityIndependent,
		UserCapabilityProficient, UserCapabilityExpert,
	}
	for _, level := range levels {
		if !validCapabilityLevel(level) {
			t.Fatalf("level %q should validate", level)
		}
	}
}

func TestEvaluateUserCapabilityUpdateRejectsInvalidLevel(t *testing.T) {
	_, accepted, err := EvaluateUserCapabilityUpdate(UserCapabilityUpdate{
		Capability: "x",
		Level:      UserCapabilityLevel("mastered"),
		Confidence: 1,
		Source:     UserModelSourceRef{Kind: UserSourceExplicitUser},
	}, DefaultUserCapabilityPolicy())
	if accepted || err == nil {
		t.Fatalf("expected rejection for invalid level, got accepted=%v err=%v", accepted, err)
	}
}
