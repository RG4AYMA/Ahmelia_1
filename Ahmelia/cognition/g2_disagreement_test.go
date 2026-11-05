package cognition

import (
	"testing"

	"ahmelia/interaction"
	"ahmelia/situation"
)

func TestG2AuthoritativeDisagreementAcrossLanguagesUsesStructuredEvidence(t *testing.T) {
	cases := []struct {
		name string
		req  interaction.SemanticRequest
	}{
		{name: "spanish-unseen-wording", req: interaction.SemanticRequest{
			Language: "es", Kind: interaction.KindInform, Topic: "color favorito", Confidence: .95,
			Relations: []interaction.SemanticRelation{{Source: "user", Predicate: "declares_preference", Target: "color favorito", Value: "azul turquesa", Confidence: .93}},
		}},
		{name: "english-unseen-wording", req: interaction.SemanticRequest{
			Language: "en", Kind: interaction.KindInform, Topic: "favorite color", Confidence: .95,
			Relations: []interaction.SemanticRelation{{Source: "user", Predicate: "declares_preference", Target: "favorite color", Value: "amber", Confidence: .93}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, intent, goal, relevance, interpretation, decision := buildB96338(tc.req, interaction.ConversationSnapshot{})
			authoritative := []situation.Evidence{{
				ID: "user-model-pref-1", Subject: "user", Predicate: "declares_preference", Object: tc.req.Relations[0].Target,
				Value: "negro", Confidence: .92, Authoritative: true, Source: "B96.34.UserPreference",
			}}
			conflict := EvaluateConversationalConflict(model, intent, goal, relevance, interpretation, decision, authoritative)
			if len(conflict.AuthoritativeFindings) != 1 {
				t.Fatalf("authoritative findings=%d, want 1; conflict=%+v", len(conflict.AuthoritativeFindings), conflict)
			}
			finding := conflict.AuthoritativeFindings[0]
			if finding.Kind != "contradicts_authoritative_evidence" {
				t.Fatalf("finding kind=%q", finding.Kind)
			}
			if len(finding.EvidenceIDs) != 2 || finding.EvidenceIDs[1] != "user-model-pref-1" {
				t.Fatalf("evidence ids=%v", finding.EvidenceIDs)
			}
			if finding.Confidence < .90 {
				t.Fatalf("confidence=%v", finding.Confidence)
			}
		})
	}
}

func TestG2OpinionWithoutAuthoritativeContradictionDoesNotEmitFinding(t *testing.T) {
	req := interaction.SemanticRequest{
		Language: "es", Kind: interaction.KindInform, Topic: "color favorito", Confidence: .95,
		Relations: []interaction.SemanticRelation{{Source: "user", Predicate: "declares_preference", Target: "color favorito", Value: "verde", Confidence: .94}},
	}
	model, intent, goal, relevance, interpretation, decision := buildB96338(req, interaction.ConversationSnapshot{})
	conflict := EvaluateConversationalConflict(model, intent, goal, relevance, interpretation, decision, nil)
	if len(conflict.AuthoritativeFindings) != 0 {
		t.Fatalf("unexpected authoritative findings=%+v", conflict.AuthoritativeFindings)
	}
}
