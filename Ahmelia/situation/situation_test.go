package situation

import (
	"testing"
	"time"

	"ahmelia/interaction"
)

func TestBuildPreservesAuthoritiesAndRelations(t *testing.T) {
	now := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	req := interaction.SemanticRequest{Raw: "quiero retomar el proyecto", Normalized: "quiero retomar el proyecto", Confidence: .95, SpeechAct: interaction.SpeechPropose, Topic: "proyecto", Objective: interaction.Objective{Text: "retomar el proyecto"}}
	s := Build(Input{Request: req, Conversation: interaction.ConversationSnapshot{ActiveTopic: "proyecto", ActiveObjective: "retomar el proyecto", ActiveWorkID: "work-1", LastTurnAt: now}, Work: interaction.WorkContext{ID: "work-1", Status: "running", Objective: "retomar el proyecto", CurrentStep: "paso 2", UpdatedAt: now}, HasWork: true, World: WorldEvidence{Available: true, App: "Visual Studio Code", Project: "Ahmelia", ObservedAt: now, Source: "world.snapshot"}, Now: now})
	if s.ID == "" || s.Anchor.Description == "" {
		t.Fatal("missing situation identity/anchor")
	}
	if len(s.Evidence) < 6 {
		t.Fatalf("expected rich evidence, got %d", len(s.Evidence))
	}
	if len(s.Relations) < 6 {
		t.Fatalf("expected relations, got %d", len(s.Relations))
	}
	for _, e := range s.Evidence {
		if !e.Authoritative {
			t.Fatalf("source evidence unexpectedly non-authoritative: %+v", e)
		}
	}
	if len(s.Conflicts) != 0 {
		t.Fatalf("unexpected conflict: %v", s.Conflicts)
	}
}

func TestBuildDoesNotInventUnresolvedReference(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "abre eso", Normalized: "abre eso", Confidence: .2, NeedsClarification: true, References: []interaction.Reference{{Expression: "eso", Resolved: false}}}
	s := Build(Input{Request: req, Now: time.Now()})
	if len(s.Uncertainty) == 0 {
		t.Fatal("expected uncertainty")
	}
	for _, r := range s.Relations {
		if r.Target == "eso" {
			t.Fatal("unresolved reference became a factual relation")
		}
	}
}

func TestSpecialistsContributeWithoutVoting(t *testing.T) {
	s := Build(Input{Request: interaction.SemanticRequest{Raw: "hola ahmelia como estas", Normalized: "hola ahmelia como estas", Confidence: .98, SpeechAct: interaction.SpeechSocial}, Now: time.Now()})
	a := ConversationalPerspective(s)
	b := OperationalPerspective(s)
	if a.Perspective == b.Perspective {
		t.Fatal("specialists must remain distinct")
	}
	if a.ID == b.ID {
		t.Fatal("specialist contribution IDs must differ")
	}
	if len(a.Claims) == 0 {
		t.Fatal("conversation specialist produced no claim")
	}
}

func TestBuildRepresentsIntentionalSocialRelation(t *testing.T) {
	now := time.Now()
	req := interaction.SemanticRequest{Raw: "hola ahmelia como estas", Normalized: "hola ahmelia como estas", Confidence: .98, SpeechAct: interaction.SpeechSocial, Subject: interaction.UserObservationSubjectSelf}
	s := Build(Input{Request: req, Now: now})
	found := false
	for _, r := range s.Relations {
		if r.Source == "user" && r.Target == "ahmelia" && r.Type == "addresses" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected user→Ahmelia intentional/social relation, relations=%+v", s.Relations)
	}
}

func TestB96402_IntentionalSocialEvidenceUsesStructuralSpeechAct(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "una propuesta", Normalized: "una propuesta", SpeechAct: interaction.SpeechPropose, Confidence: 0.9}
	s := Build(Input{Request: req, Now: time.Now()})
	found := false
	for _, r := range s.Relations {
		if r.Type == "proposes" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected proposes relation from structural SpeechAct")
	}
}

func TestB96402_IntentionalSocialEvidenceDoesNotDependOnProposalKeywords(t *testing.T) {
	req := interaction.SemanticRequest{Raw: "una frase cualquiera", Normalized: "una frase cualquiera", SpeechAct: interaction.SpeechUnknown, Confidence: 0.2}
	s := Build(Input{Request: req, Now: time.Now()})
	for _, r := range s.Relations {
		if r.Type == "proposes" {
			t.Fatal("unknown SpeechAct must not create proposes relation from text keywords")
		}
	}
}
