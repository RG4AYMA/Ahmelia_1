package core

import (
	"context"
	"fmt"
	"testing"

	"ahmelia/agency"
	"ahmelia/deliberation"
	"ahmelia/interaction"
	"ahmelia/planning"
)

func agencyTestEngine(client *shadowFakeClient) *Engine {
	return &Engine{
		lastBehavioralDecision: cognitionBehaviorForTest(),
		deliberationClient:     client,
		agentGoalLedger:        agency.NewLedger(""),
		agentGoalOn:            true,
		planner:                planning.NewPlanner(),
	}
}

func workspaceImpulseID(ws *deliberation.Workspace) string {
	if ws == nil {
		return ""
	}
	for _, item := range ws.Items {
		if item.Kind == "impulso" {
			return item.ID
		}
	}
	return ""
}

func selfGoalRaw(impulseID string) string {
	return fmt.Sprintf(`{"reading":"Hay trabajo sin cerrar","strategy":"answer","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.8,"abstain":false,`+
		`"self_goal":{"title":"Aprender a verificar el estado de Unity","motive":"El trabajo quedó sin verificar","next_step":"Revisar la evidencia local","expected_minutes":15,"risk":"low","mode":"explore","evidence_ids":[%q],"confidence":0.7}}`, impulseID)
}

func TestBuildImpulsesFromHarnessUncertainty(t *testing.T) {
	e := &Engine{lastBehavioralDecision: cognitionBehaviorForTest()}
	e.lastBehavioralDecision.Uncertainty = true
	imps := e.buildImpulses(&processTurn{input: "no sé"})
	if len(imps) == 0 {
		t.Fatal("esperaba un impulso por incertidumbre")
	}
	if imps[0].Kind != agency.ImpulseKnowledgeGap {
		t.Fatalf("impulso=%+v", imps[0])
	}
}

func TestDeliberateShadowRecordsOwnerGatedSelfGoal(t *testing.T) {
	turn := shadowTestTurn()
	client := &shadowFakeClient{}
	e := agencyTestEngine(client)
	ws := e.buildDeliberationWorkspace(turn)
	impulseID := workspaceImpulseID(ws)
	if impulseID == "" {
		t.Fatalf("el turno debía abrir un impulso: %+v", ws.Items)
	}
	client.raw = selfGoalRaw(impulseID)

	rec := e.deliberateShadow(context.Background(), ws, e.currentHarnessDecision(), turn.input, "t1")
	if rec.Proposal.SelfGoal == nil {
		t.Fatalf("la meta propia válida se perdió: %q", rec.Proposal.SelfGoalRejected)
	}
	open := e.AgentGoalProposals()
	if len(open) != 1 {
		t.Fatalf("propuestas=%+v", open)
	}
	if open[0].Goal.Title != "Aprender a verificar el estado de Unity" {
		t.Fatalf("meta=%+v", open[0].Goal)
	}
	if open[0].Impulse.Kind != agency.ImpulseUnfinishedWork {
		t.Fatalf("impulso registrado=%+v", open[0].Impulse)
	}
	if goals := e.planner.Goals(); len(goals) != 0 {
		t.Fatalf("la propuesta no puede registrarse en el planificador: %+v", goals)
	}
}

func TestDeliberateShadowDropsSelfGoalWithoutImpulse(t *testing.T) {
	turn := &processTurn{
		input:        "hola",
		semanticReq:  interactionSemanticRequestForAgencyTest(),
		cognitionRan: true,
	}
	e := agencyTestEngine(&shadowFakeClient{raw: selfGoalRaw("w1")})
	ws := e.buildDeliberationWorkspace(turn)
	if workspaceImpulseID(ws) != "" {
		t.Fatalf("este turno no debía abrir impulso: %+v", ws.Items)
	}
	rec := e.deliberateShadow(context.Background(), ws, e.currentHarnessDecision(), turn.input, "t2")
	if rec.Proposal.SelfGoal != nil {
		t.Fatalf("sin impulso la meta debe descartarse: %+v", rec.Proposal.SelfGoal)
	}
	if rec.Proposal.SelfGoalRejected == "" {
		t.Fatal("el rechazo debe quedar visible")
	}
	if open := e.AgentGoalProposals(); len(open) != 0 {
		t.Fatalf("no debía registrarse: %+v", open)
	}
}

func TestAuthorizeAgentGoalDoesNotRegisterOrExecute(t *testing.T) {
	turn := shadowTestTurn()
	client := &shadowFakeClient{}
	e := agencyTestEngine(client)
	ws := e.buildDeliberationWorkspace(turn)
	client.raw = selfGoalRaw(workspaceImpulseID(ws))
	e.deliberateShadow(context.Background(), ws, e.currentHarnessDecision(), turn.input, "t3")

	open := e.AgentGoalProposals()
	if len(open) != 1 {
		t.Fatalf("propuestas=%+v", open)
	}
	got, err := e.AuthorizeAgentGoal(open[0].ID, "aceptada como candidata")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got.Status != agency.GoalAuthorized {
		t.Fatalf("estado=%s", got.Status)
	}
	if len(e.AgentGoalProposals()) != 0 {
		t.Fatal("una meta autorizada no sigue pendiente")
	}
	if goals := e.planner.Goals(); len(goals) != 0 {
		t.Fatalf("autorizar no registra en el planificador: %+v", goals)
	}
	if counts := e.AgentGoalCounts(); counts[agency.GoalAuthorized] != 1 {
		t.Fatalf("counts=%v", counts)
	}
}

func interactionSemanticRequestForAgencyTest() interaction.SemanticRequest {
	return interaction.SemanticRequest{Kind: interaction.KindInform, Topic: "saludo", Confidence: 0.6}
}
