package situation

import (
	"testing"
	"time"
)

func TestEvaluateCreatesSharedDecisionEvidenceWithoutMutatingSituation(t *testing.T) {
	now := time.Now()
	s := Situation{ID: "sit-test", Version: 3, CreatedAt: now, UpdatedAt: now, Relevance: 1, Confidence: .9,
		Relations:   []Relation{{Source: "user", Target: "ahmelia", Type: "gives_feedback_to", EvidenceIDs: []string{"e1"}, Confidence: .9, Relevance: .95}},
		Uncertainty: []string{"u1"}, Conflicts: []string{"c1"}}
	c := Contribution{ID: "sit-test:conversation", Perspective: "conversation", Confidence: .9,
		Relations: s.Relations, Claims: []string{"feedback"}, Provenance: []string{"test"}}
	got := Evaluate(s, []Contribution{c})
	if got.SituationID != s.ID || got.SituationVersion != s.Version {
		t.Fatalf("identity=%q/%d", got.SituationID, got.SituationVersion)
	}
	if len(got.Findings) != 1 || got.Findings[0].Kind != "gives_feedback_to" {
		t.Fatalf("findings=%+v", got.Findings)
	}
	if !got.HasKind("gives_feedback_to") {
		t.Fatal("expected feedback finding")
	}
	if len(s.Relations) != 1 || len(s.Uncertainty) != 1 || len(s.Conflicts) != 1 {
		t.Fatal("Evaluate mutated Situation")
	}
}

// TestEvaluatePreservesContributionUncertaintyAndConflict cierra la fuga que
// dejaba una advertencia de especialista fuera de DecisionEvidence: una
// contribución sin relaciones (o con ellas) puede declarar incertidumbre o
// conflicto y ambos deben sobrevivir, con sus criterios, sin mutar Situation.
func TestEvaluatePreservesContributionUncertaintyAndConflict(t *testing.T) {
	s := Situation{ID: "sit-test", Version: 1, Relevance: 1, Confidence: .8}
	c := Contribution{
		Perspective: "competence",
		Uncertainty: []string{"capability still unverifiable"},
		Conflicts:   []string{"capability regressed"},
	}
	got := Evaluate(s, []Contribution{c})
	if !containsString(got.Uncertainty, "capability still unverifiable") {
		t.Fatalf("contribution uncertainty lost: %+v", got.Uncertainty)
	}
	if !containsString(got.Conflicts, "capability regressed") {
		t.Fatalf("contribution conflict lost: %+v", got.Conflicts)
	}
	if len(s.Uncertainty) != 0 || len(s.Conflicts) != 0 {
		t.Fatal("Evaluate mutated the source Situation")
	}
	foundUncertainty, foundConflict := false, false
	for _, criterion := range got.Criteria {
		switch criterion.ID {
		case "uncertainty":
			foundUncertainty = true
		case "conflict":
			foundConflict = true
		}
	}
	if !foundUncertainty || !foundConflict {
		t.Fatalf("contribution warnings did not become criteria: %+v", got.Criteria)
	}
}

func TestB96404EvaluationSelectsContextualCriteria(t *testing.T) {
	s := Situation{ID: "sit-test", Version: 1, Relevance: 1, Confidence: .9, Uncertainty: []string{"low certainty"}}
	c := Contribution{Perspective: "conversation", Confidence: .9, Relations: []Relation{{Source: "user", Target: "future_state", Type: "proposes", EvidenceIDs: []string{"e1"}, Confidence: .9, Relevance: .9}}}
	e := Evaluate(s, []Contribution{c})
	if len(e.Criteria) == 0 {
		t.Fatal("expected contextual criteria")
	}
	if !e.HasKind("proposes") {
		t.Fatal("expected proposal finding")
	}
	foundAgency := false
	for _, cr := range e.Criteria {
		if cr.ID == "agency" {
			foundAgency = true
		}
	}
	if !foundAgency {
		t.Fatal("expected user-agency criterion")
	}
}
