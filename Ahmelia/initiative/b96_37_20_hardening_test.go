package initiative

import (
	"testing"
	"time"
)

func TestB963720_TraceDeepCopyIsReadOnly(t *testing.T) {
	now := time.Now()
	o := NewInitiativeObservability(nil)
	tr, err := o.Create("i-copy", SourceConversationalContext, Provenance{CreatedAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	s := InitiativeSignal{ID: "sig-copy", Type: "x", Value: "original", Provenance: Provenance{CreatedAt: now}}
	if _, err = o.RecordSignal(tr.ID, s, now); err != nil {
		t.Fatal(err)
	}
	got, ok := o.Find(tr.ID)
	if !ok {
		t.Fatal("trace missing")
	}
	got.Events[0].Signal.Value = "mutated"
	got.Events[0].Evaluation = map[string]string{"x": "mutated"}
	again, ok := o.Find(tr.ID)
	if !ok {
		t.Fatal("trace missing after mutation")
	}
	if again.Events[0].Signal.Value != "original" {
		t.Fatal("trace exposed nested pointer state")
	}
}

func TestB963720_OpportunityAndActionIDsAreStableReferences(t *testing.T) {
	now := time.Now()
	op := BuildOpportunity([]InitiativeSignal{{ID: "sig-id", Source: SourceConversationalContext, Reason: ReasonRelevantComment, Relevant: true, Confidence: .9, ObservedAt: now}}, SignalInput{Now: now})
	if op.ID == "" {
		t.Fatal("opportunity id missing")
	}
	d := InitiativeDecision{ID: "decision-id", Decision: Intervene, Opportunity: op, Action: ActionComment, Source: op.Source, Reason: op.Reason, Timing: TimingNow, CreatedAt: now, PolicyVersion: "B96.37.2"}
	c, err := NewActionContractBuilder().Build(d)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "action-decision-id" {
		t.Fatalf("unexpected action id %q", c.ID)
	}
}
