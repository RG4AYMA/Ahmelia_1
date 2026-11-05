package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/cognition"
	"ahmelia/interaction"
)

func TestB96Nivel0CapabilityHandoffRequiresCognitiveExecutionPosture(t *testing.T) {
	c := cognition.ConversationalCognitionE2E{
		Valid: true,
		Action: cognition.ConversationalActionSelection{
			Primary:            cognition.ActionRequestExecution,
			ExecutionIntent:    true,
			Blocked:            false,
			NeedsClarification: false,
		},
	}
	if !capabilityHandoffReady(c) {
		t.Fatal("expected a coherent non-blocked execution posture to reach the capability boundary")
	}
}

func TestB96Nivel0CapabilityHandoffFailsClosedOnCognitiveClarification(t *testing.T) {
	c := cognition.ConversationalCognitionE2E{
		Valid: true,
		Action: cognition.ConversationalActionSelection{
			Primary:            cognition.ActionClarify,
			ExecutionIntent:    false,
			Blocked:            true,
			NeedsClarification: true,
		},
	}
	if capabilityHandoffReady(c) {
		t.Fatal("cognitive clarification must not reach the capability execution boundary")
	}
}

func TestB96Nivel0CommandConvergesToExecutableCognition(t *testing.T) {
	e := newB963334AuditEngine(t)
	e.SetResponseCallback(func(string) {})

	e.Process("cambia a chrome")

	if !e.lastConversationalCognition.Valid {
		t.Fatalf("cognition invalid: %v", e.lastConversationalCognition.Errors)
	}
	if e.lastConversationalCognition.Model.Request.CommunicativeActs.Primary != interaction.ActCommand {
		t.Fatalf("expected canonical command act, got %s", e.lastConversationalCognition.Model.Request.CommunicativeActs.Primary)
	}
	if e.lastConversationalCognition.Action.Primary != cognition.ActionRequestExecution {
		t.Fatalf("command did not converge to execution posture: %s", e.lastConversationalCognition.Action.Primary)
	}
	if !e.lastConversationalCognition.Action.ExecutionIntent {
		t.Fatal("execution intent was lost before the capability boundary")
	}
	if !capabilityHandoffReady(e.lastConversationalCognition) {
		t.Fatal("command did not satisfy the cognition-to-runtime coherence contract")
	}
	if e.lastResponseIntent.Kind != interaction.ResponseIntentNone || !e.lastResponseIntent.ShouldAct {
		t.Fatalf("response intent/action boundary diverged: %+v", e.lastResponseIntent)
	}
	if e.lastNaturalDialogue.Move != interaction.MoveWait {
		t.Fatalf("expected command dialogue move=wait, got %s", e.lastNaturalDialogue.Move)
	}
}

func TestB96Nivel0FiveTurnRegressionPreservesActionHandoff(t *testing.T) {
	e := newB963334AuditEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	turns := []string{
		"hola como estas?",
		"nada quería saber si sabías mi color favorito",
		"cual es mi color favorito?",
		"y tu tienes color favorito?",
		"tranquila pronto podrás empezar a ver el mundo, hare todo lo posible para que puedas a empezar a ver por ti misma, asi podras saber cual es tu color favorito.",
		"cambia a chrome",
	}
	for _, input := range turns {
		e.Process(input)
	}

	if e.lastConversationalCognition.Action.Primary != cognition.ActionRequestExecution {
		t.Fatalf("final command lost execution posture after the five-turn conversational regression: %s", e.lastConversationalCognition.Action.Primary)
	}
	if !capabilityHandoffReady(e.lastConversationalCognition) {
		t.Fatal("final command failed the cognition-to-runtime coherence contract after the five-turn regression")
	}
	last := e.history.GetLastAction()
	if last == nil || last.Action != "cambiar_ventana" || last.Target != "chrome" {
		t.Fatalf("final command did not reach Engine.execute with the semantic action contract: %#v", last)
	}
	if len(responses) < 6 {
		t.Fatalf("expected one observable response per regression turn, got %d", len(responses))
	}
}

func TestB96Nivel0ProcessUsesCognitionRuntimeGateBeforeSemanticExecution(t *testing.T) {
	data, err := os.ReadFile("turn_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func (e *Engine) preExecutionPhase(t *processTurn) bool {")
	end := start + strings.Index(src[start:], "\n}\n\nfunc (e *Engine) finishExecutionPhase")
	if start < 0 || end <= start {
		t.Fatal("preExecutionPhase boundaries not found")
	}
	phase := src[start:end]
	capPos := strings.Index(phase, "capabilityDecision := interaction.ResolveCapabilityInteraction(t.semanticReq)")
	authorityPos := strings.Index(phase, "e.authorityExecute(capabilityDecision, t.semanticReq)")
	if capPos < 0 || authorityPos < 0 {
		t.Fatal("canonical capability decision -> authority markers missing from execution phase")
	}
	if !strings.Contains(phase, "if !capabilityHandoffReady(e.lastConversationalCognition)") {
		t.Fatal("execution phase lost the explicit cognition/runtime coherence guard")
	}
	if authorityPos <= capPos {
		t.Fatalf("capability authority did not follow capability decision: capability=%d authority=%d", capPos, authorityPos)
	}
}
