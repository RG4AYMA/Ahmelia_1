package language

import "testing"

func TestResponseComposerDoesNotUseStaleTopicForUnrelatedTurn(t *testing.T) {
	cf := NewConversationFlow()
	cf.RecordTopic("Aurora")
	rc := NewResponseComposer(nil, nil, nil, nil, cf, nil)
	plan := rc.CreatePlan("")
	if plan.HasFollowUp {
		t.Fatalf("stale active topic leaked into unrelated response: %#v", plan)
	}
	plan = rc.CreatePlan("general")
	if plan.HasFollowUp {
		t.Fatalf("mismatched topic inherited stale follow-up: %#v", plan)
	}
	plan = rc.CreatePlan("Aurora")
	if !plan.HasFollowUp {
		t.Fatal("matching active topic should still permit follow-up generation")
	}
}
