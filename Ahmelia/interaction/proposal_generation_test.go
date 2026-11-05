package interaction

import (
	"strings"
	"testing"
)

func TestProposalGeneratorAdvancesActiveObjective(t *testing.T) {
	p := NewUserProfile()
	ctx := ProposalGenerationContext{
		Request: SemanticRequest{Raw: "sigamos", Kind: KindConversation},
		Snapshot: ConversationSnapshot{
			ActiveTopic:     "la escena",
			ActiveObjective: "avanzar la escena",
		},
		Profile: p,
	}
	got, ok := GenerateProposal(ctx)
	if !ok {
		t.Fatal("expected proposal")
	}
	if got.Purpose != ProposalPurposeAdvance {
		t.Fatalf("expected advance proposal, got %q", got.Purpose)
	}
	if !got.RequiresConsent {
		t.Fatal("proposal must require user consent")
	}
	if !strings.Contains(got.Proposal, "escena") {
		t.Fatalf("proposal should preserve conversational topic: %q", got.Proposal)
	}
}

func TestProposalGeneratorDoesNotCompeteWithDirectNeed(t *testing.T) {
	ctx := ProposalGenerationContext{
		Request:  SemanticRequest{Raw: "¿qué hora es?", Kind: KindQuestion},
		Snapshot: ConversationSnapshot{ActiveTopic: "escena", ActiveObjective: "avanzar"},
		Profile:  NewUserProfile(),
	}
	if got := DefaultProposalGenerator().Generate(ctx); len(got) != 0 {
		t.Fatalf("expected no proactive proposal during direct question, got %#v", got)
	}
}

func TestProposalGeneratorResumesOpenThread(t *testing.T) {
	ctx := ProposalGenerationContext{
		Request: SemanticRequest{Raw: "sigamos hablando", Kind: KindConversation},
		Snapshot: ConversationSnapshot{
			OpenThreads: []ConversationThread{
				{Topic: "arquitectura de comunicación", Objective: "diseñar interacción", Status: "open"},
			},
		},
		Profile: NewUserProfile(),
	}
	got, ok := GenerateProposal(ctx)
	if !ok {
		t.Fatal("expected proposal")
	}
	found := false
	for _, c := range DefaultProposalGenerator().Generate(ctx) {
		if c.Purpose == ProposalPurposeResume && strings.Contains(c.Proposal, "arquitectura de comunicación") {
			found = true
		}
	}
	if !found && got.Purpose != ProposalPurposeResume {
		t.Fatalf("expected resume proposal, got %#v", got)
	}
}

func TestProposalGeneratorIsDomainNeutral(t *testing.T) {
	ctx := ProposalGenerationContext{
		Request:  SemanticRequest{Raw: "esto está interesante", Kind: KindConversation},
		Snapshot: ConversationSnapshot{},
		Profile:  NewUserProfile(),
	}
	for _, c := range DefaultProposalGenerator().Generate(ctx) {
		if strings.Contains(strings.ToLower(c.Proposal), "programming") ||
			strings.Contains(strings.ToLower(c.Proposal), "unity") ||
			strings.Contains(strings.ToLower(c.Proposal), "blender") {
			t.Fatalf("proposal unexpectedly hard-coded to a domain: %q", c.Proposal)
		}
	}
}
