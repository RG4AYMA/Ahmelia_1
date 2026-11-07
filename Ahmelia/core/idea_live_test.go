package core

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"ahmelia/agency"
	"ahmelia/integration"
	"ahmelia/linguistics"
	"ahmelia/planning"
)

// TestLiveIdeaGenerationIsProposedNotExecuted exercises the idea-generator lane
// with the real model: the LLM articulates a self-goal from a harness impulse,
// the harness validates and records it as a proposal, and nothing reaches the
// planner until the owner authorizes it. It is opt-in because it calls Ollama.
func TestLiveIdeaGenerationIsProposedNotExecuted(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_IDEA") != "1" {
		t.Skip("definir AHMELIA_LIVE_IDEA=1 para ejecutar contra el modelo real")
	}
	client := linguistics.NewOllamaClient()
	e := &Engine{
		lastBehavioralDecision: cognitionBehaviorForTest(),
		deliberationClient:     client,
		planClient:             client,
		agentGoalLedger:        agency.NewLedger(""),
		agentGoalOn:            true,
		planner:                planning.NewPlanner(),
	}
	turn := shadowTestTurn()
	ws := e.buildDeliberationWorkspace(turn)
	if workspaceImpulseID(ws) == "" {
		t.Fatalf("el turno debía abrir un impulso: %+v", ws.Items)
	}

	start := time.Now()
	rec := e.deliberateShadow(context.Background(), ws, e.currentHarnessDecision(), turn.input, "live-idea")
	t.Logf("modelo=%s outcome=%s latencia=%s", client.ModelName(), rec.Outcome, time.Since(start))
	if rec.Error != "" {
		t.Logf("error=%s", rec.Error)
	}
	if rec.Proposal.SelfGoal != nil {
		t.Logf("idea generada: %s", rec.Proposal.SelfGoal.Summary())
		t.Logf("evidencia: %v rechazada: %v", rec.Proposal.SelfGoal.EvidenceIDs, rec.Proposal.SelfGoal.RejectedEvidenceIDs)
	} else {
		t.Logf("sin idea: %s", rec.Proposal.SelfGoalRejected)
	}

	if goals := e.planner.Goals(); len(goals) != 0 {
		t.Fatalf("el planificador no puede recibir una idea sin autorización: %+v", goals)
	}
	open := e.AgentGoalProposals()
	if rec.Proposal.SelfGoal == nil {
		if len(open) != 0 {
			t.Fatalf("sin propuesta válida no debía haber registro: %+v", open)
		}
		return
	}
	if len(open) != 1 {
		t.Fatalf("propuestas = %+v, quería 1", open)
	}
	if open[0].Status != agency.GoalProposed {
		t.Fatalf("estado = %s, quería proposed", open[0].Status)
	}

	var planProposal goalPlanProposal
	planDeadline := time.Now().Add(4*time.Minute + 30*time.Second)
	for {
		e.planMu.Lock()
		proposal, ok := e.planProposals[open[0].ID]
		e.planMu.Unlock()
		if ok {
			planProposal = proposal
			break
		}
		if time.Now().After(planDeadline) {
			t.Log("plan de fondo: no llegó dentro del presupuesto")
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if planProposal.err != "" {
		t.Logf("plan de fondo rechazado: %s", planProposal.err)
	} else if len(planProposal.blueprints) > 0 {
		t.Logf("plan de fondo: %d tareas; primera=%q", len(planProposal.blueprints), planProposal.blueprints[0].Title)
	}

	if _, err := e.AuthorizeAgentGoal(open[0].ID, "prueba en vivo"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if goals := e.planner.Goals(); len(goals) != 0 {
		t.Fatalf("autorizar no registra ni ejecuta: %+v", goals)
	}
	_, registered, err := e.RegisterAgentGoal(open[0].ID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	plan := e.planner.GeneratePlanContext(context.Background(), &registered)
	t.Logf("plan del planner: %d tareas; aceptado_por_modelo=%t", len(plan.Tasks), len(planProposal.blueprints) > 0 && planProposal.err == "")
	if len(e.planner.Tasks()) != 0 {
		t.Fatalf("generar un plan no registra ni ejecuta tareas: %+v", e.planner.Tasks())
	}
}

// TestLiveGoalDecompositionIsBounded measures the LLM planning lane with the
// real model. The planner owns validation and the deterministic fallback; a
// slow or invalid decomposition must never leave the agent without a plan.
func TestLiveGoalDecompositionIsBounded(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_IDEA") != "1" {
		t.Skip("definir AHMELIA_LIVE_IDEA=1 para ejecutar contra el modelo real")
	}
	client := linguistics.NewOllamaClient()
	warmStart := time.Now()
	warmCtx, warmCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	warmErr := client.WaitReady(warmCtx)
	warmCancel()
	t.Logf("warmup=%s err=%v", time.Since(warmStart), warmErr)
	timeout := 45 * time.Second
	if raw := os.Getenv("AHMELIA_LIVE_PLAN_TIMEOUT"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			timeout = parsed
		}
	}
	t.Logf("presupuesto de planificación=%s", timeout)
	planner := planning.NewPlanner()
	planner.SetGoalTaskDecomposer(integration.NewOllamaGoalTaskDecomposerWithTimeout(client, timeout))
	goal := planning.NewGoal(
		"Verificar el estado de Unity antes de reportar",
		"Inspeccionar el proyecto, comprobar que el editor responde y reportar evidencia.",
		planning.PriorityMedium,
		"ahmelia",
	)

	start := time.Now()
	plan := planner.GeneratePlanContext(context.Background(), goal)
	t.Logf("modelo=%s latencia=%s feasible=%t tareas=%d", client.ModelName(), time.Since(start), plan.Feasible, len(plan.Tasks))
	if len(plan.Tasks) == 0 {
		t.Fatal("el planificador debe producir un plan determinista aunque el modelo falle")
	}
	adaptive := true
	for _, issue := range plan.Issues {
		if strings.Contains(issue, "adaptive decomposition unavailable") {
			adaptive = false
			t.Logf("fallback determinista: %s", issue)
		}
	}
	t.Logf("plan adaptativo del modelo: %t; tareas=%d", adaptive, len(plan.Tasks))
}
