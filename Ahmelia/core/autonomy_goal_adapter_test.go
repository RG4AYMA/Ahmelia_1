package core

import (
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

func TestAutonomyGoalAdapter_CurrentGoalIsImmediatelyEligible(t *testing.T) {
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 20*time.Minute)
	goal := planning.NewGoal("crea carpeta llamada loop_gate", "", planning.PriorityHigh, "user")
	state.SetCurrentGoal(goal)

	has, id := (autonomyGoalAdapter{monitor: monitor}).PendingGoal()
	if !has || id != goal.ID {
		t.Fatalf("PendingGoal()=(%v,%q), want (%v,%q)", has, id, true, goal.ID)
	}
}
