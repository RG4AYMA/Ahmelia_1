package interaction

import "testing"

func TestWorkBridgePendingProductionTurnContinuesUnfinishedObjective(t *testing.T) {
	bridge := NewWorkBridge()
	work := WorkContext{ID: "w1", Status: "pending", Objective: "Quiero implementar una utilidad"}
	req := Analyze("Crea una función Go llamada IsEven", Context{ActiveObjective: work.Objective, ActiveWorkID: work.ID})
	decision := bridge.Resolve(req, ConversationSnapshot{ActiveWorkID: work.ID, ActiveObjective: work.Objective}, work, true)
	if decision.Relation != WorkRelationContinue || decision.WorkID != work.ID {
		t.Fatalf("decision=%+v, want continuation of w1", decision)
	}
	if !decision.AppendObjective {
		t.Fatalf("pending specification should authorize objective composition: %+v", decision)
	}
	merged := ComposeObjective(work.Objective, req.Objective.Text)
	if merged == work.Objective || merged == "" {
		t.Fatalf("objective was not composed: %q", merged)
	}
}

func TestWorkBridgeCorrectionPreservesWorkObjective(t *testing.T) {
	bridge := NewWorkBridge()
	work := WorkContext{ID: "w1", Status: "completed", Objective: "crear un programa llamado alfa"}
	req := Analyze("No, me refería a beta", Context{ActiveObjective: work.Objective, ActiveWorkID: work.ID})
	decision := bridge.Resolve(req, ConversationSnapshot{ActiveWorkID: work.ID, ActiveObjective: work.Objective}, work, true)
	if decision.Relation != WorkRelationContinue || decision.WorkID != work.ID {
		t.Fatalf("correction should preserve its work link: %+v", decision)
	}
	if decision.AppendObjective {
		t.Fatalf("semantic correction must not be appended as specification text: %+v", decision)
	}
}

func TestComposeObjectiveDoesNotDuplicateLabels(t *testing.T) {
	base := "Implementar una utilidad"
	if got := ComposeObjective(base, "Objetivo:"); got != base {
		t.Fatalf("label changed objective: %q", got)
	}
}

func TestWorkBridgePendingProductionExplicitNewWorkStartsNew(t *testing.T) {
	bridge := NewWorkBridge()
	work := WorkContext{ID: "w1", Status: "pending", Objective: "Implementar una utilidad"}
	req := Analyze("Nuevo objetivo: crear otro programa", Context{ActiveObjective: work.Objective, ActiveWorkID: work.ID})
	decision := bridge.Resolve(req, ConversationSnapshot{ActiveWorkID: work.ID, ActiveObjective: work.Objective}, work, true)
	if decision.Relation != WorkRelationStartNew || !decision.ShouldCreate {
		t.Fatalf("decision=%+v, want start_new", decision)
	}
}
