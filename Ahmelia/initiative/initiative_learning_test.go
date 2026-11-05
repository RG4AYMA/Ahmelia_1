package initiative

import (
	"reflect"
	"testing"
	"time"
)

func learningOutcome(id string, out InitiativeOutcome, action Action, conf float64, at time.Time) InitiativeOutcomeRecord {
	return InitiativeOutcomeRecord{ID: "out-" + id, InitiativeID: "init-" + id, Outcome: out, Confidence: conf, InitiativeAction: action, InitiativeReason: ReasonNextStepAvailable, ObservedAt: at, Provenance: Provenance{Component: "test"}}
}
func TestB963716_OutcomeBecomesEvidenceNotDecision(t *testing.T) {
	l := NewInitiativeLearner(nil)
	now := time.Now()
	e, err := l.ObserveOutcome(learningOutcome("1", OutcomeAccepted, ActionSuggest, .98, now))
	if err != nil || e.Pattern != PatternAction || e.Key != string(ActionSuggest) {
		t.Fatalf("%+v %v", e, err)
	}
}
func TestB963716_ThresholdPreventsSingleInteractionLearning(t *testing.T) {
	l := NewInitiativeLearner(nil)
	_, _ = l.ObserveOutcome(learningOutcome("1", OutcomeAccepted, ActionSuggest, .98, time.Now()))
	if _, ok, err := l.Propose(PatternAction, string(ActionSuggest), ScopeAction, time.Now()); err != nil || ok {
		t.Fatalf("single evidence must not adapt: ok=%v err=%v", ok, err)
	}
}
func TestB963716_FavorablePattern(t *testing.T) {
	l := NewInitiativeLearner(nil)
	now := time.Now()
	for i := 0; i < 4; i++ {
		e := learningOutcome(string(rune('a'+i)), OutcomeAccepted, ActionSuggest, .95, now.Add(time.Duration(i)*time.Minute))
		if _, err := l.ObserveOutcome(e); err != nil {
			t.Fatal(err)
		}
	}
	a, ok, err := l.Propose(PatternAction, string(ActionSuggest), ScopeAction, now)
	if err != nil || !ok || a.Signal != "favorable" || a.Weight <= .5 {
		t.Fatalf("%+v ok=%v err=%v", a, ok, err)
	}
}
func TestB963716_RejectionIsCautionNotPermanentBan(t *testing.T) {
	l := NewInitiativeLearner(nil)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := l.ObserveOutcome(learningOutcome(string(rune('a'+i)), OutcomeRejected, ActionSuggest, .95, now.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	a, ok, err := l.Propose(PatternAction, string(ActionSuggest), ScopeAction, now)
	if err != nil || !ok || a.Signal != "caution" {
		t.Fatalf("%+v ok=%v err=%v", a, ok, err)
	}
	if len(l.Signals(now)) != 1 {
		t.Fatal("expected one bounded signal")
	}
}
func TestB963716_UnknownDoesNotBecomeLearningPattern(t *testing.T) {
	l := NewInitiativeLearner(nil)
	now := time.Now()
	for i := 0; i < 5; i++ {
		if _, err := l.ObserveOutcome(learningOutcome(string(rune('a'+i)), OutcomeUnknown, ActionSuggest, .99, now)); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, err := l.Propose(PatternAction, string(ActionSuggest), ScopeAction, now); err != nil || ok {
		t.Fatal("unknown outcomes must not create action adaptation")
	}
}
func TestB963716_Expiration(t *testing.T) {
	l := NewInitiativeLearner(nil)
	l.Policy.EvidenceTTL = time.Minute
	now := time.Now()
	for i := 0; i < 3; i++ {
		_, _ = l.ObserveOutcome(learningOutcome(string(rune('a'+i)), OutcomeAccepted, ActionSuggest, .95, now.Add(-2*time.Minute)))
	}
	if _, ok, _ := l.Propose(PatternAction, string(ActionSuggest), ScopeAction, now); ok {
		t.Fatal("expired evidence adapted")
	}
}
func TestB963716_Reversal(t *testing.T) {
	l := NewInitiativeLearner(nil)
	now := time.Now()
	for i := 0; i < 3; i++ {
		_, _ = l.ObserveOutcome(learningOutcome(string(rune('a'+i)), OutcomeAccepted, ActionSuggest, .95, now))
	}
	a, ok, _ := l.Propose(PatternAction, string(ActionSuggest), ScopeAction, now)
	if !ok {
		t.Fatal("proposal missing")
	}
	r, err := l.Reverse(a.ID, now)
	if err != nil || !r.Reversed || r.Validated {
		t.Fatalf("%+v %v", r, err)
	}
	if len(l.Signals(now)) != 0 {
		t.Fatal("reversed adaptation still active")
	}
}
func TestB963716_NoDecisionAuthority(t *testing.T) {
	typ := fmtType(t, AdaptationSignal{})
	for _, name := range []string{"Decision", "Action"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("adaptation signal must not expose %s", name)
		}
	}
}
func fmtType(t *testing.T, v any) reflect.Type { return reflect.TypeOf(v) }
