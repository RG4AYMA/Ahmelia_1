package core

import (
	"testing"
	"time"

	"ahmelia/initiative"
	"ahmelia/interaction"
)

func TestFocusArbiterKeepsCurrentUserTurnEligibleWithoutChannelMetadata(t *testing.T) {
	arbiter := NewFocusArbiter(initiative.NewInterruptionPolicy())
	turn := &ProposalContract{
		ID:        "interactive-current-turn",
		Kind:      ProposalKindInteractive,
		CreatedAt: time.Now(),
		Intention: "respond",
		Priority:  ProposalPriorityHigh,
		Channel:   interaction.ChannelUnknown,
		Evidence:  []ProposalEvidence{{Confidence: 0.9}},
	}

	result, err := arbiter.Arbitrate([]*ProposalContract{turn}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Selected != turn {
		t.Fatalf("current user turn was rejected as an interruption: reasoning=%q candidates=%+v", result.Reasoning, result.Candidates)
	}
}

func TestFocusArbiterStillAppliesInterruptionPolicyToInitiative(t *testing.T) {
	arbiter := NewFocusArbiter(initiative.NewInterruptionPolicy())
	initiativeProposal := &ProposalContract{
		ID:        "unsolicited-initiative",
		Kind:      ProposalKindInitiative,
		CreatedAt: time.Now(),
		Intention: "check in",
		Priority:  ProposalPriorityHigh,
		Channel:   interaction.ChannelUnknown,
		Evidence:  []ProposalEvidence{{Confidence: 0.9}},
	}

	result, err := arbiter.Arbitrate([]*ProposalContract{initiativeProposal}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Selected != nil {
		t.Fatal("initiative bypassed interruption policy despite unknown channel")
	}
}
