package initiative

import (
	"testing"
	"time"
)

func feedbackDecision() InitiativeDecision {
	return InitiativeDecision{ID: "init-1", Decision: Intervene, Action: ActionSuggest, Reason: ReasonNextStepAvailable, Source: SourceConversationalContext, CreatedAt: time.Now(), PolicyVersion: "B96.37.2"}
}
func TestB963715_StatesAndClassification(t *testing.T) {
	for _, x := range []InitiativeOutcome{OutcomeUnknown, OutcomeEngaged, OutcomeAccepted, OutcomeRejected, OutcomeIgnored, OutcomeInterrupted, OutcomeCompleted, OutcomeFailed, OutcomeExpired} {
		if x == "" {
			t.Fatal("empty outcome")
		}
	}
	cases := []struct {
		r ResponseClass
		o InitiativeOutcome
	}{{ResponseAffirmation, OutcomeAccepted}, {ResponseRejection, OutcomeRejected}, {ResponseContinuation, OutcomeEngaged}}
	for _, c := range cases {
		o, _, _ := ClassifyOutcome(OutcomeObservation{Initiative: feedbackDecision(), ResponseClass: c.r})
		if o != c.o {
			t.Fatalf("got %s want %s", o, c.o)
		}
	}
}
func TestB963715_SilenceIsNotRejection(t *testing.T) {
	e := NewOutcomeEvaluator(nil)
	x, err := e.Evaluate(OutcomeObservation{Initiative: feedbackDecision(), ObservedAt: time.Now()})
	if err != nil || x.Outcome != OutcomeUnknown {
		t.Fatalf("got %+v err=%v", x, err)
	}
	y, err := e.ExpireUnanswered(feedbackDecision(), time.Now())
	if err != nil || y.Outcome != OutcomeIgnored {
		t.Fatalf("expiration got %+v err=%v", y, err)
	}
}

func TestB963715_ExplicitAcceptanceHighConfidence(t *testing.T) {
	e := NewOutcomeEvaluator(nil)
	x, err := e.ObserveResponse(feedbackDecision(), "Sí, hagámoslo", ResponseAffirmation, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if x.Outcome != OutcomeAccepted || x.Confidence < .9 {
		t.Fatalf("bad acceptance: %+v", x)
	}
}
func TestB963715_Rejection(t *testing.T) {
	e := NewOutcomeEvaluator(nil)
	x, err := e.ObserveResponse(feedbackDecision(), "No, prefiero seguir con esto", ResponseRejection, time.Now())
	if err != nil || x.Outcome != OutcomeRejected {
		t.Fatalf("%+v %v", x, err)
	}
}
func TestB963715_InterruptionAndCompletion(t *testing.T) {
	e := NewOutcomeEvaluator(nil)
	x, _ := e.Evaluate(OutcomeObservation{Initiative: feedbackDecision(), Interrupted: true, ObservedAt: time.Now()})
	if x.Outcome != OutcomeInterrupted {
		t.Fatal(x)
	}
	y, _ := e.Evaluate(OutcomeObservation{Initiative: feedbackDecision(), Completed: true, ObservedAt: time.Now()})
	if y.Outcome != OutcomeCompleted {
		t.Fatal(y)
	}
}
func TestB963715_NoLearningOrDecisionAuthority(t *testing.T) {
	e := NewOutcomeEvaluator(nil)
	x, _ := e.Evaluate(OutcomeObservation{Initiative: feedbackDecision(), Engaged: true, ObservedAt: time.Now()})
	if x.Outcome != OutcomeEngaged {
		t.Fatal(x)
	}
	if feedbackDecision().Decision != Intervene {
		t.Fatal("feedback mutated decision authority")
	}
}
func TestB963715_ExpirationField(t *testing.T) {
	e := NewOutcomeEvaluator(nil)
	x, _ := e.Evaluate(OutcomeObservation{Initiative: feedbackDecision(), ObservedAt: time.Now()})
	if x.ExpiresAt.IsZero() {
		t.Fatal("missing feedback expiration")
	}
}
