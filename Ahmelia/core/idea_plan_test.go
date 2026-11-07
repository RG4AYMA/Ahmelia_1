package core

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"ahmelia/agency"
	"ahmelia/deliberation"
	"ahmelia/planning"
)

type fakePlanClient struct {
	mu      sync.Mutex
	raw     string
	err     error
	calls   int
	called  chan struct{}
	release <-chan struct{}
}

func (f *fakePlanClient) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	select {
	case f.called <- struct{}{}:
	default:
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return f.raw, f.err
}

func (f *fakePlanClient) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type countingPlanDecomposer struct {
	calls int
}

func (c *countingPlanDecomposer) DecomposeGoal(context.Context, planning.Goal) ([]planning.GoalTaskBlueprint, error) {
	c.calls++
	return []planning.GoalTaskBlueprint{
		{Title: "Plan determinista", Description: "No debía usarse si hay plan del modelo", TimeEstimate: time.Minute},
	}, nil
}

func newIdeaPlanFixture(t *testing.T, client *fakePlanClient) (*Engine, *deliberation.Workspace, string) {
	t.Helper()
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	e := &Engine{
		agentGoalLedger: agency.NewLedger(""),
		agentGoalOn:     true,
		planner:         planning.NewPlanner(),
		planClient:      client,
	}
	e.sceneNow = func() time.Time { return now }

	ws := &deliberation.Workspace{}
	impulse := agency.Impulse{Kind: agency.ImpulseUnfinishedWork, Reason: "trabajo activo sin verificar", Strength: 0.8}
	ws.Impulses = []agency.Impulse{impulse}
	ws.Add("impulso", impulse.Text(), "agency", 0.8)
	impulseID := workspaceImpulseID(ws)
	if impulseID == "" {
		t.Fatal("el workspace debía tener un impulso")
	}
	return e, ws, impulseID
}

func ideaPlanRecord(impulseID string) deliberation.Record {
	goal := agency.GoalProposal{
		Title:           "Verificar el estado de Unity",
		Motive:          "El trabajo activo quedó sin verificar",
		NextStep:        "Inspeccionar el estado del editor",
		ExpectedMinutes: 15,
		Risk:            agency.RiskLow,
		Mode:            agency.ModeExplore,
		EvidenceIDs:     []string{impulseID},
		Confidence:      0.8,
	}
	return deliberation.Record{
		Outcome: deliberation.OutcomeAgree,
		Proposal: deliberation.Proposal{
			SelfGoal: &goal, Strategy: deliberation.StrategyAnswer, Grounded: true,
		},
	}
}

func waitPlanProposal(t *testing.T, e *Engine, agencyID string) goalPlanProposal {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		e.planMu.Lock()
		proposal, ok := e.planProposals[agencyID]
		e.planMu.Unlock()
		if ok {
			return proposal
		}
		if time.Now().After(deadline) {
			t.Fatal("el plan de fondo no quedó registrado")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func validPlanJSON() string {
	return `{"tasks":[
		{"title":"Inspeccionar el proyecto","description":"Leer el estado actual","depends_on":[],"estimated_minutes":5},
		{"title":"Verificar el resultado","description":"Comprobar el efecto observado","depends_on":[0],"estimated_minutes":10}
	]}`
}

func TestBackgroundPlanIsAdoptedBeforeFirstGeneration(t *testing.T) {
	client := &fakePlanClient{raw: validPlanJSON(), called: make(chan struct{}, 1)}
	e, ws, impulseID := newIdeaPlanFixture(t, client)
	e.recordAgentGoal(ideaPlanRecord(impulseID), ws, "t1")

	open := e.AgentGoalProposals()
	if len(open) != 1 {
		t.Fatalf("propuestas = %+v, quería 1", open)
	}
	proposal := waitPlanProposal(t, e, open[0].ID)
	if proposal.err != "" || len(proposal.blueprints) != 2 {
		t.Fatalf("propuesta de plan = %+v", proposal)
	}

	if _, err := e.AuthorizeAgentGoal(open[0].ID, "prueba"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	updated, registered, err := e.RegisterAgentGoal(open[0].ID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if updated.ID == "" || registered.ID == "" {
		t.Fatalf("registro inesperado: updated=%+v registered=%+v", updated, registered)
	}

	decomposer := &countingPlanDecomposer{}
	e.planner.SetGoalTaskDecomposer(decomposer)
	plan := e.planner.GeneratePlanContext(context.Background(), &registered)
	if decomposer.calls != 0 {
		t.Fatalf("el decomposer se llamó %d veces; el plan de fondo debía ganar", decomposer.calls)
	}
	if len(plan.Tasks) != 2 || plan.Tasks[0].Title != "Inspeccionar el proyecto" || plan.Tasks[1].Title != "Verificar el resultado" {
		t.Fatalf("plan inesperado: %+v", plan.Tasks)
	}
	if goals := e.planner.Goals(); len(goals) != 1 {
		t.Fatalf("el planificador debía tener solo el objetivo registrado: %+v", goals)
	}
}

func waitPlanConsumed(t *testing.T, e *Engine, agencyID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		e.planMu.Lock()
		_, hasProposal := e.planProposals[agencyID]
		_, hasGoal := e.planGoalIDs[agencyID]
		e.planMu.Unlock()
		if !hasProposal && !hasGoal {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("el plan de fondo no se consumió")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLateBackgroundPlanIsAdoptedBeforeGeneration(t *testing.T) {
	release := make(chan struct{})
	client := &fakePlanClient{raw: validPlanJSON(), called: make(chan struct{}, 1), release: release}
	e, ws, impulseID := newIdeaPlanFixture(t, client)
	e.recordAgentGoal(ideaPlanRecord(impulseID), ws, "t1")

	select {
	case <-client.called:
	case <-time.After(2 * time.Second):
		t.Fatal("el plan de fondo no arrancó")
	}
	open := e.AgentGoalProposals()
	if len(open) != 1 {
		t.Fatalf("propuestas = %+v, quería 1", open)
	}
	if _, err := e.AuthorizeAgentGoal(open[0].ID, "prueba"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	_, registered, err := e.RegisterAgentGoal(open[0].ID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	close(release)
	waitPlanConsumed(t, e, open[0].ID)

	decomposer := &countingPlanDecomposer{}
	e.planner.SetGoalTaskDecomposer(decomposer)
	plan := e.planner.GeneratePlanContext(context.Background(), &registered)
	if decomposer.calls != 0 {
		t.Fatalf("el decomposer se llamó %d veces; el plan tardío debía adoptarse antes de generar", decomposer.calls)
	}
	if len(plan.Tasks) != 2 || plan.Tasks[0].Title != "Inspeccionar el proyecto" {
		t.Fatalf("plan inesperado: %+v", plan.Tasks)
	}
}

func TestInvalidBackgroundPlanFallsBackToDeterministic(t *testing.T) {
	invalid := `{"tasks":[{"title":"Paso inválido","description":"Depende de sí mismo","depends_on":[0],"estimated_minutes":5}]}`
	client := &fakePlanClient{raw: invalid, called: make(chan struct{}, 1)}
	e, ws, impulseID := newIdeaPlanFixture(t, client)
	e.recordAgentGoal(ideaPlanRecord(impulseID), ws, "t1")

	open := e.AgentGoalProposals()
	if len(open) != 1 {
		t.Fatalf("propuestas = %+v", open)
	}
	proposal := waitPlanProposal(t, e, open[0].ID)
	if proposal.err == "" {
		t.Fatalf("el plan inválido debía quedar rechazado: %+v", proposal)
	}

	if _, err := e.AuthorizeAgentGoal(open[0].ID, "prueba"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	_, registered, err := e.RegisterAgentGoal(open[0].ID)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	decomposer := &countingPlanDecomposer{}
	e.planner.SetGoalTaskDecomposer(decomposer)
	plan := e.planner.GeneratePlanContext(context.Background(), &registered)
	if decomposer.calls != 1 {
		t.Fatalf("decomposer calls = %d, quería 1", decomposer.calls)
	}
	if len(plan.Tasks) != 1 || !strings.Contains(plan.Tasks[0].Title, "determinista") {
		t.Fatalf("plan inesperado: %+v", plan.Tasks)
	}
}
