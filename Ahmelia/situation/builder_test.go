package situation

import (
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/ownership"
)

func TestB96404SelfSocialQuestionProducesStateRelation(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:          "hola ahmelia como estas?",
		Normalized:   "hola ahmelia como estas?",
		Subject:      interaction.UserObservationSubjectSelf,
		QuestionType: "procedure_or_explanation",
		Confidence:   .98,
		CommunicativeActs: interaction.CommunicativeActs{
			Primary:    interaction.ActQuestion,
			Secondary:  []interaction.CommunicativeAct{interaction.ActSocial},
			Confidence: .98,
		},
	}
	s := Build(Input{Request: req, Now: time.Unix(1, 0)})
	for _, r := range s.Relations {
		if r.Type == "asks_about_state" && r.Source == "user" && r.Target == "ahmelia" {
			return
		}
	}
	t.Fatalf("expected asks_about_state relation, got %#v", s.Relations)
}

func TestB96404FutureRelationSurvivesConversationalPerspective(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:        "haré todo lo posible para que puedas empezar a ver por ti misma",
		Normalized: "haré todo lo posible para que puedas empezar a ver por ti misma",
		Confidence: .90,
		Relations:  []interaction.SemanticRelation{{Source: "user", Predicate: "anticipates", Target: "ahmelia:future_state", Confidence: .88}},
	}
	s := Build(Input{Request: req, Now: time.Unix(1, 0)})
	c := ConversationalPerspective(s)
	for _, r := range c.Relations {
		if r.Type == "anticipates" {
			return
		}
	}
	t.Fatalf("expected anticipates relation to survive specialist projection")
}

func TestBuildConsumesDirectOwnershipResolutionWhenSemanticRequestHasNone(t *testing.T) {
	resolved := ownership.Resolve("tu proposito")
	got := Build(Input{
		Request:           interaction.SemanticRequest{Raw: "tu proposito", Normalized: "tu proposito", Confidence: .9},
		SubjectResolution: &resolved,
		Now:               time.Now(),
	})
	found := false
	for _, evidence := range got.Evidence {
		if evidence.Authority == "ownership.Resolver" && evidence.Subject == "self" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("situation did not retain direct ownership resolver evidence: %+v", got.Evidence)
	}
}
