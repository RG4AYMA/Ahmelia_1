package core

import (
	"testing"

	"ahmelia/tools"
)

func TestDispatchProductionRecoveryRoutesEachDecisionToItsOwner(t *testing.T) {
	cases := []struct {
		name   string
		action tools.RecoveryAction
		want   string
	}{
		{name: "retry", action: tools.RecoveryRetry, want: "retry"},
		{name: "replan", action: tools.RecoveryReplan, want: "replan"},
		{name: "block", action: tools.RecoveryBlock, want: "block"},
		{name: "ask_user", action: tools.RecoveryAskUser, want: "ask_user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner := ""
			handlers := productionRecoveryHandlers{
				Retry:   func() bool { owner = "retry"; return true },
				Replan:  func() bool { owner = "replan"; return true },
				Block:   func() bool { owner = "block"; return true },
				AskUser: func() bool { owner = "ask_user"; return true },
			}
			if !dispatchProductionRecovery(tools.RecoveryDecision{Action: tc.action, Reason: "test"}, handlers) {
				t.Fatal("dispatch returned false")
			}
			if owner != tc.want {
				t.Fatalf("owner=%q want=%q", owner, tc.want)
			}
		})
	}
}

func TestDispatchProductionRecoveryDoesNotApplyUnknownDecision(t *testing.T) {
	called := false
	handlers := productionRecoveryHandlers{
		Retry:   func() bool { called = true; return true },
		Replan:  func() bool { called = true; return true },
		Block:   func() bool { called = true; return true },
		AskUser: func() bool { called = true; return true },
	}
	if dispatchProductionRecovery(tools.RecoveryDecision{Action: "invalid"}, handlers) {
		t.Fatal("invalid recovery action must fail closed")
	}
	if called {
		t.Fatal("invalid recovery action reached an owner")
	}
}
