package initiative

import (
	"strings"
	"testing"
	"time"
)

func TestInterventionProposalContainsSemanticContentOnly(t *testing.T) {
	now := time.Now()
	d := InitiativeDecision{
		ID: "decision-1", Decision: Intervene, Action: ActionAsk,
		Source: SourceConversationalContext, Reason: ReasonConversationContinue,
		Target: "architecture", Timing: TimingNow, Priority: PriorityNormal,
		Urgency: UrgencyLow, Confidence: .9, CreatedAt: now, PolicyVersion: "B96.37.2",
	}
	contract, err := NewActionContractBuilder().Build(d)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := BuildInterventionProposal(contract, InitiativeOpportunity{Context: "active architecture thread", Target: "architecture"}, "architecture")
	if err != nil {
		t.Fatal(err)
	}
	if !proposal.Valid() {
		t.Fatal("proposal must be valid")
	}
	if proposal.ContentMode != ContentQuestion {
		t.Fatalf("content mode=%q", proposal.ContentMode)
	}
	if proposal.Topic != "architecture" {
		t.Fatalf("topic=%q", proposal.Topic)
	}
	for _, field := range []string{proposal.Target, proposal.Topic, proposal.Context, string(proposal.Reason)} {
		if strings.Contains(field, "?") {
			t.Fatalf("proposal contains finished question text: %q", field)
		}
	}
}

func TestInterventionProposalRejectsWaitAndDefer(t *testing.T) {
	now := time.Now()
	for _, decision := range []Decision{Wait, Defer} {
		d := InitiativeDecision{ID: "decision", Decision: decision, Timing: TimingWait, CreatedAt: now, PolicyVersion: "B96.37.2"}
		if decision == Defer {
			d.Timing = TimingDefer
		}
		contract, err := NewActionContractBuilder().Build(d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := BuildInterventionProposal(contract, InitiativeOpportunity{}, ""); err == nil {
			t.Fatalf("%s must not create an intervention proposal", decision)
		}
	}
}
