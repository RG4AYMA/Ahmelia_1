package core

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ahmelia/agency"
	"ahmelia/deliberation"
	"ahmelia/interaction"
)

type ideaWindowClient struct {
	mu     sync.Mutex
	prompt string
	called chan struct{}
}

func (c *ideaWindowClient) GenerateShadowContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	c.mu.Lock()
	c.prompt = prompt
	c.mu.Unlock()
	select {
	case c.called <- struct{}{}:
	default:
	}
	if id := impulseIDFromPrompt(prompt); id != "" {
		return selfGoalRaw(id), nil
	}
	return `{"reading":"sin impulso","strategy":"answer","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.4,"abstain":true}`, nil
}

func (c *ideaWindowClient) Prompt() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prompt
}

func impulseIDFromPrompt(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.Contains(line, "(impulso") {
			continue
		}
		start := strings.Index(line, "[")
		end := strings.Index(line, "]")
		if start >= 0 && end > start {
			return line[start+1 : end]
		}
	}
	return ""
}

func newIdeaWindowEngine(t *testing.T, restricted bool) (*Engine, *ideaWindowClient) {
	t.Helper()
	e := buildTestEngine(t)
	e.agentGoalLedger = agency.NewLedger("")
	e.agentGoalOn = true
	e.deliberationShadow = deliberation.NewShadowLog("")
	e.deliberationEnabled = true
	client := &ideaWindowClient{called: make(chan struct{}, 1)}
	e.deliberationClient = client

	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.FixedZone("COT", -5*3600))
	e.sceneNow = func() time.Time { return now }
	e.enableOwnerRestrictionsAt(filepath.Join(t.TempDir(), "owner_restrictions.json"))

	req := interaction.Analyze("verificar el estado de Unity", interaction.Context{})
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatalf("begin work: %v", err)
	}
	e.conversationState.SetActiveWork(work.ID)
	if restricted {
		if _, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "test", now); err != nil {
			t.Fatalf("restriction: %v", err)
		}
	}
	return e, client
}

func waitIdeaWindowCall(t *testing.T, client *ideaWindowClient) string {
	t.Helper()
	select {
	case <-client.called:
		return client.Prompt()
	case <-time.After(2 * time.Second):
		t.Fatal("la deliberación sombra no llamó al modelo")
		return ""
	}
}

func TestRestrictionClosesIdeaWindowThroughProcess(t *testing.T) {
	e, client := newIdeaWindowEngine(t, true)
	e.Process("hola")
	prompt := waitIdeaWindowCall(t, client)
	if strings.Contains(prompt, "(impulso") {
		t.Fatalf("la restricción no cerró la ventana de ideas:\n%s", prompt)
	}
	if open := e.AgentGoalProposals(); len(open) != 0 {
		t.Fatalf("no debía registrarse una idea: %+v", open)
	}
}

func TestWithoutRestrictionIdeaWindowOpensThroughProcess(t *testing.T) {
	e, client := newIdeaWindowEngine(t, false)
	e.Process("hola")
	prompt := waitIdeaWindowCall(t, client)
	if !strings.Contains(prompt, "(impulso") {
		t.Fatalf("sin restricción el impulso debía llegar al modelo:\n%s", prompt)
	}

	deadline := time.Now().Add(2 * time.Second)
	for len(e.AgentGoalProposals()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	open := e.AgentGoalProposals()
	if len(open) != 1 {
		t.Fatalf("propuestas = %+v, quería 1", open)
	}
	if open[0].Status != agency.GoalProposed {
		t.Fatalf("estado = %s, quería proposed", open[0].Status)
	}
	if goals := e.planner.Goals(); len(goals) != 0 {
		t.Fatalf("una idea no puede llegar al planificador sin autorización: %+v", goals)
	}
}
