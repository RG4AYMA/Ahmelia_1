package interaction

import "testing"

func TestThreadRouterExplicitTopicSwitchWithoutTargetClearsActiveContext(t *testing.T) {
	s := NewConversationState()
	s.EnsureThread("Aurora", "supervisar el sistema")
	d := NewThreadRouter().Route(s, SemanticRequest{Raw: "Eso ya no importa, hablemos de otra cosa", Normalized: "eso ya no importa hablemos de otra cosa", BreaksContinuity: true, FollowUp: true})
	if d.Relation != ThreadSwitch {
		t.Fatalf("relation=%v", d.Relation)
	}
	snap := s.Snapshot()
	if snap.ActiveTopic != "" || snap.ActiveObjective != "" || snap.ActiveThreadID != "" {
		t.Fatalf("active context leaked: %#v", snap)
	}
}

func TestSemanticQuestionCoverageAndArithmeticClassification(t *testing.T) {
	for _, raw := range []string{"quien te creo?", "que sabes hacer?"} {
		r := Analyze(raw, Context{})
		if r.Kind != KindQuestion {
			t.Fatalf("%q classified as %v", raw, r.Kind)
		}
	}
	if r := Analyze("4 + 4", Context{}); r.Kind != KindReasoning {
		t.Fatalf("arithmetic classified as %v", r.Kind)
	}
	if answer, ok := SolveQuestion(Analyze("4 + 4", Context{})); !ok || answer != "8" {
		t.Fatalf("arithmetic answer=%q ok=%v", answer, ok)
	}
}
