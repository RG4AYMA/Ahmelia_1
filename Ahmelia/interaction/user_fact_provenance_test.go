package interaction

import (
	"strings"
	"testing"
	"time"
)

func sampleSource(kind UserModelSourceKind, now time.Time) UserModelSourceRef {
	return UserModelSourceRef{
		Kind:       kind,
		SourceID:   "turn-9",
		ActorID:    "ayma240",
		EvidenceID: "ev-1",
		ObservedAt: now,
	}
}

func TestAttributeAuditProjectsAllSixQuestions(t *testing.T) {
	now := time.Now()
	r := UserAttributeRecord{
		Key:        "technical_programming_experience",
		Value:      "high",
		Confidence: 0.82,
		Source:     sampleSource(UserSourceInferred, now),
		Evidence:   []UserAttributeEvidence{{ID: "e1", Source: sampleSource(UserSourceInferred, now)}},
		Status:     UserModelStatusUncertain,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	a := AttributeAudit(r)

	if a.Value != "high" {
		t.Fatalf("expected value 'high', got %q", a.Value)
	}
	if a.Confidence != 0.82 {
		t.Fatalf("expected confidence 0.82, got %v", a.Confidence)
	}
	if a.SourceKind != UserSourceInferred {
		t.Fatalf("expected source kind inferred, got %q", a.SourceKind)
	}
	if !a.Inferred {
		t.Fatal("expected Inferred=true for UserSourceInferred provenance")
	}
	if a.ActorID != "ayma240" {
		t.Fatalf("expected actor id to carry through, got %q", a.ActorID)
	}
	if a.EvidenceCount != 1 {
		t.Fatalf("expected evidence_count 1, got %d", a.EvidenceCount)
	}
	if a.Contradicted {
		t.Fatal("expected Contradicted=false with no contradicting ids given")
	}
	if err := ValidateUserFactAudit(a); err != nil {
		t.Fatalf("expected valid audit, got %v", err)
	}
}

func TestExplicitUserSourceIsNeverMarkedInferred(t *testing.T) {
	now := time.Now()
	r := UserPreferenceRecord{
		Key: "concise_responses", Value: "true", Strength: 0.91, Confidence: 0.95,
		Source: sampleSource(UserSourceExplicitUser, now), Status: UserModelStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	a := PreferenceAudit(r)
	if a.Inferred {
		t.Fatal("expected Inferred=false for an explicit user statement")
	}
}

func TestInteractionPatternAuditUsesAuthoritativeEvidenceCount(t *testing.T) {
	now := time.Now()
	r := UserInteractionPatternRecord{
		Pattern: "incremental_workflow", Confidence: 0.88, EvidenceCount: 37,
		Source: sampleSource(UserSourceObserved, now), Status: UserModelStatusActive,
		Evidence:  []UserInteractionPatternEvidence{{ID: "e1", Source: sampleSource(UserSourceObserved, now)}},
		CreatedAt: now, UpdatedAt: now,
	}
	a := InteractionPatternAudit(r)
	if a.EvidenceCount != 37 {
		t.Fatalf("expected evidence_count to be the authoritative running total 37, not len(Evidence)=%d, got %d", len(r.Evidence), a.EvidenceCount)
	}
}

func TestGoalCapabilityKnowledgeCommunicationAuditsProjectValue(t *testing.T) {
	now := time.Now()
	src := sampleSource(UserSourceObserved, now)

	goal := GoalAudit(UserGoalRecord{Goal: "learn Blender", Category: UserGoalCategoryAspirational, Confidence: 0.76, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if goal.Value != "learn Blender" {
		t.Fatalf("expected goal value to project, got %q", goal.Value)
	}

	cap := CapabilityAudit(UserCapabilityRecord{Capability: "Blender", Level: UserCapabilityIndependent, Confidence: 0.7, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if cap.Value != string(UserCapabilityIndependent) {
		t.Fatalf("expected capability level to project as value, got %q", cap.Value)
	}

	know := KnowledgeAudit(UserKnowledgeRecord{Topic: "Go", Level: UserKnowledgeAdvanced, Confidence: 0.9, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if know.Value != string(UserKnowledgeAdvanced) {
		t.Fatalf("expected knowledge level to project as value, got %q", know.Value)
	}

	comm := CommunicationProfileAudit(UserCommunicationProfileValue{Dimension: CommunicationDirectness, Value: "direct", Confidence: 0.8, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if comm.Value != "direct" {
		t.Fatalf("expected communication profile value to project, got %q", comm.Value)
	}
}

func TestDetectValueContradictionBetweenTwoLiveDifferingValues(t *testing.T) {
	now := time.Now()
	src := sampleSource(UserSourceObserved, now)
	a := PreferenceAudit(UserPreferenceRecord{Key: "response_length", Value: "short", Confidence: 0.7, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	b := PreferenceAudit(UserPreferenceRecord{Key: "response_length", Value: "long", Confidence: 0.7, Source: src, Status: UserModelStatusUncertain, CreatedAt: now, UpdatedAt: now})

	if !DetectValueContradiction(a, b) {
		t.Fatal("expected contradiction between two live, differing values")
	}
}

func TestDetectValueContradictionFalseWhenValuesMatch(t *testing.T) {
	now := time.Now()
	src := sampleSource(UserSourceObserved, now)
	a := PreferenceAudit(UserPreferenceRecord{Key: "response_length", Value: "short", Confidence: 0.7, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	b := PreferenceAudit(UserPreferenceRecord{Key: "response_length", Value: "short", Confidence: 0.9, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})

	if DetectValueContradiction(a, b) {
		t.Fatal("expected no contradiction when values match")
	}
}

func TestDetectValueContradictionFalseWhenOneSideIsSettledHistory(t *testing.T) {
	now := time.Now()
	src := sampleSource(UserSourceObserved, now)
	a := PreferenceAudit(UserPreferenceRecord{Key: "response_length", Value: "short", Confidence: 0.7, Source: src, Status: UserModelStatusSuperseded, CreatedAt: now, UpdatedAt: now})
	b := PreferenceAudit(UserPreferenceRecord{Key: "response_length", Value: "long", Confidence: 0.9, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})

	if DetectValueContradiction(a, b) {
		t.Fatal("expected a superseded value to not count as a live contradiction")
	}
}

func TestUserFactAuditWithContradictingEvidenceIDsIsFlagged(t *testing.T) {
	now := time.Now()
	src := sampleSource(UserSourceObserved, now)
	r := UserAttributeRecord{Key: "k", Value: "v", Confidence: 0.7, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now}

	a := AttributeAudit(r, "ev-conflict-1", "ev-conflict-2")
	if !a.Contradicted {
		t.Fatal("expected Contradicted=true when contradicting evidence ids are supplied")
	}
	if len(a.ContradictingEvidenceIDs) != 2 {
		t.Fatalf("expected 2 contradicting evidence ids, got %d", len(a.ContradictingEvidenceIDs))
	}
	if err := ValidateUserFactAudit(a); err != nil {
		t.Fatalf("expected valid audit, got %v", err)
	}
}

func TestValidateUserFactAuditRejectsInconsistentContradictionFlag(t *testing.T) {
	now := time.Now()
	base := UserFactAudit{
		Confidence: 0.5, SourceKind: UserSourceObserved, Status: UserModelStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}

	inconsistentTrue := base
	inconsistentTrue.Contradicted = true // no ids given
	if err := ValidateUserFactAudit(inconsistentTrue); err == nil {
		t.Fatal("expected error when contradicted=true but no evidence ids given")
	}

	inconsistentFalse := base
	inconsistentFalse.ContradictingEvidenceIDs = []string{"ev-1"} // flag not set
	if err := ValidateUserFactAudit(inconsistentFalse); err == nil {
		t.Fatal("expected error when evidence ids given but contradicted=false")
	}
}

func TestValidateUserFactAuditRejectsInvalidConfidenceSourceStatus(t *testing.T) {
	now := time.Now()
	valid := UserFactAudit{Confidence: 0.5, SourceKind: UserSourceObserved, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now}

	badConfidence := valid
	badConfidence.Confidence = 1.5
	if err := ValidateUserFactAudit(badConfidence); err == nil {
		t.Fatal("expected error for out-of-range confidence")
	}

	badSource := valid
	badSource.SourceKind = "made_up"
	if err := ValidateUserFactAudit(badSource); err == nil {
		t.Fatal("expected error for invalid source kind")
	}

	badStatus := valid
	badStatus.Status = "made_up"
	if err := ValidateUserFactAudit(badStatus); err == nil {
		t.Fatal("expected error for invalid status")
	}

	badEvidenceCount := valid
	badEvidenceCount.EvidenceCount = -1
	if err := ValidateUserFactAudit(badEvidenceCount); err == nil {
		t.Fatal("expected error for negative evidence_count")
	}
}

func TestUserFactAuditExplainIncludesAllSixQuestions(t *testing.T) {
	now := time.Now()
	src := sampleSource(UserSourceInferred, now)
	r := UserKnowledgeRecord{Topic: "Unity", Level: UserKnowledgeFamiliar, Confidence: 0.65, Source: src, Status: UserModelStatusUncertain, CreatedAt: now, UpdatedAt: now}

	explanation := KnowledgeAudit(r, "ev-conflict").Explain()

	for _, want := range []string{"value=", "confidence=", "status=", "source=", "actor=", "inferred=", "evidence_count=", "contradicted=", "observed_at=", "updated_at="} {
		if !strings.Contains(explanation, want) {
			t.Fatalf("expected explanation to mention %q, got %q", want, explanation)
		}
	}
	if !strings.Contains(explanation, "inferred=yes") {
		t.Fatalf("expected inferred=yes for an inferred source, got %q", explanation)
	}
	if !strings.Contains(explanation, "contradicted=yes") {
		t.Fatalf("expected contradicted=yes when contradicting evidence was supplied, got %q", explanation)
	}
}
