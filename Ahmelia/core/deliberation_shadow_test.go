package core

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"ahmelia/cognition"
	"ahmelia/deliberation"
	"ahmelia/interaction"
	"ahmelia/situation"
)

type shadowFakeClient struct {
	mu    sync.Mutex
	raw   string
	err   error
	calls int
}

func (c *shadowFakeClient) GenerateShadowContext(_ context.Context, _ string, _ json.RawMessage, _ int) (string, error) {
	c.mu.Lock()
	c.calls++
	raw, err := c.raw, c.err
	c.mu.Unlock()
	return raw, err
}

func (c *shadowFakeClient) CallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func cognitionBehaviorForTest() cognition.ConversationalBehaviorDecision {
	return cognition.ConversationalBehaviorDecision{
		Version:            "test",
		Action:             cognition.ActionRespond,
		SpeakToCurrentTurn: true,
		Strategy: cognition.ConversationalResponseStrategy{
			Mode: cognition.ResponseModeAnswer,
		},
		Reason: "test decision",
	}
}

func shadowTestTurn() *processTurn {
	return &processTurn{
		input: "abre eso",
		semanticReq: interaction.SemanticRequest{
			Kind:       interaction.KindAction,
			Topic:      "Unity",
			Confidence: 0.9,
			Meaning:    interaction.SemanticMeaning{Evidence: []string{"abrir el proyecto de Unity"}},
		},
		interactionHasWork: true,
		cognitionRan:       true,
		interactionWorkContext: interaction.WorkContext{
			Status:             "running",
			Objective:          "abrir el proyecto de Unity",
			CurrentStep:        "esperar confirmación",
			LastResult:         "",
			LastActionVerified: false,
		},
	}
}

func TestBuildDeliberationWorkspaceProjectsAuthorizedState(t *testing.T) {
	queue := interaction.NewClarificationQueue()
	queue.Push("abrir el proyecto", "¿cuál de los dos proyectos?", nil)
	e := &Engine{
		clarificationQueue: queue,
		lastSituation: situation.Situation{Evidence: []situation.Evidence{
			{Subject: "proyecto", Predicate: "está", Object: "Unity", Source: "world", Confidence: 0.9},
			{Subject: "usuario", Predicate: "pidió", Object: "abrir", Source: "turn", Confidence: 0.5},
		}},
	}

	ws := e.buildDeliberationWorkspace(shadowTestTurn())
	if ws.Empty() {
		t.Fatal("el workspace debe contener el turno y el estado")
	}
	if !ws.HasID("w1") {
		t.Fatal("el turno actual debe tener ID citable")
	}
	out := ws.Render()
	for _, frag := range []string{"turno_actual", "abre eso", "interpretacion", "Unity", "trabajo_activo", "pregunta_pendiente", "situacion"} {
		if !strings.Contains(out, frag) {
			t.Fatalf("el workspace pierde %q: %q", frag, out)
		}
	}
}

func TestDeliberateShadowRecordsAgreementWithoutChangingDecision(t *testing.T) {
	client := &shadowFakeClient{raw: `{"reading":"El usuario retoma el proyecto de Unity","strategy":"answer","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.8,"abstain":false}`}
	e := &Engine{
		deliberationClient:     client,
		deliberationShadow:     deliberation.NewShadowLog(""),
		lastBehavioralDecision: cognitionBehaviorForTest(),
	}
	ws := e.buildDeliberationWorkspace(shadowTestTurn())
	harness := e.currentHarnessDecision()

	rec := e.deliberateShadow(context.Background(), ws, harness, "abre eso", "turn-1")
	if rec.Outcome != deliberation.OutcomeAgree {
		t.Fatalf("outcome=%s, esperado agree (reasons=%v)", rec.Outcome, rec.Reasons)
	}
	if deliberation.Strategy(e.lastBehavioralDecision.Strategy.Mode) != harness.Strategy {
		t.Fatal("la deliberación no puede cambiar la decisión del arnés")
	}
	if got := e.LastShadowDeliberation(); got.TurnID != "turn-1" {
		t.Fatalf("el último registro no quedó accesible: %+v", got)
	}
	if counts := e.ShadowDeliberationCounts(); counts[deliberation.OutcomeAgree] != 1 {
		t.Fatalf("conteos inesperados: %+v", counts)
	}
}

func TestDeliberateShadowMarksUngroundedAndParseFailure(t *testing.T) {
	turn := shadowTestTurn()
	harness := deliberation.HarnessDecision{Action: "respond", Strategy: deliberation.StrategyAnswer, Intent: "answer", Speak: true}

	ungrounded := &shadowFakeClient{raw: `{"reading":"inventado","strategy":"answer","critique":"","expectation":"","evidence_ids":["w99"],"confidence":0.9,"abstain":false}`}
	e := &Engine{deliberationClient: ungrounded, deliberationShadow: deliberation.NewShadowLog("")}
	rec := e.deliberateShadow(context.Background(), e.buildDeliberationWorkspace(turn), harness, turn.input, "t1")
	if rec.Outcome != deliberation.OutcomeUngrounded {
		t.Fatalf("outcome=%s, esperado ungrounded", rec.Outcome)
	}

	broken := &shadowFakeClient{raw: `no es json`}
	e2 := &Engine{deliberationClient: broken, deliberationShadow: deliberation.NewShadowLog("")}
	rec = e2.deliberateShadow(context.Background(), e2.buildDeliberationWorkspace(turn), harness, turn.input, "t2")
	if rec.Outcome != deliberation.OutcomeInvalid {
		t.Fatalf("outcome=%s, esperado invalid", rec.Outcome)
	}
}

func TestDeliberateShadowUnavailableIsNotADisagreement(t *testing.T) {
	client := &shadowFakeClient{err: context.DeadlineExceeded}
	e := &Engine{deliberationClient: client, deliberationShadow: deliberation.NewShadowLog("")}
	rec := e.deliberateShadow(context.Background(), e.buildDeliberationWorkspace(shadowTestTurn()), deliberation.HarnessDecision{Strategy: deliberation.StrategyAnswer}, "hola", "t1")
	if rec.Outcome != deliberation.OutcomeUnavailable {
		t.Fatalf("outcome=%s, esperado unavailable", rec.Outcome)
	}
}

func TestScheduleShadowDeliberationIsOptIn(t *testing.T) {
	client := &shadowFakeClient{raw: `{"reading":"x","strategy":"answer","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.5,"abstain":false}`}
	e := &Engine{
		deliberationClient:     client,
		deliberationShadow:     deliberation.NewShadowLog(""),
		lastBehavioralDecision: cognitionBehaviorForTest(),
	}
	turn := shadowTestTurn()

	e.scheduleShadowDeliberation(turn)
	time.Sleep(30 * time.Millisecond)
	if client.CallCount() != 0 {
		t.Fatal("sin habilitación explícita no debe despertar trabajo de fondo")
	}

	e.EnableShadowDeliberation()
	e.deliberationShadow.SetPath("")
	e.scheduleShadowDeliberation(turn)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if client.CallCount() > 0 && e.LastShadowDeliberation().TurnID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if client.CallCount() == 0 {
		t.Fatal("con habilitación explícita la deliberación debe correr")
	}
}

// TestProcessSchedulesShadowDeliberationAfterTurn is a static boundary check:
// the live turn must schedule the shadow job, and the composition root must
// opt in. It protects against a refactor silently removing the only caller.
func TestProcessSchedulesShadowDeliberationAfterTurn(t *testing.T) {
	data, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func (e *Engine) ProcessWithChannel")
	if start < 0 {
		t.Fatal("ProcessWithChannel no encontrado")
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatal("fin de ProcessWithChannel no encontrado")
	}
	body := src[start : start+end]
	if !strings.Contains(body, "defer e.scheduleShadowDeliberation(t)") {
		t.Fatal("el turno vivo no programa la deliberación en sombra")
	}

	mainSrc, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), "engine.EnableShadowDeliberation()") {
		t.Fatal("el composition root no habilita la deliberación en sombra")
	}
}
