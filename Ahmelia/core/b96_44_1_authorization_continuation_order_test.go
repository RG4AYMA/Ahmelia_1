package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
)

// B96.44.1 architectural regression: a confirmation for a persisted pending
// execution is an operational continuation. It must be consumed after the
// conversation/work ownership boundary but before evidence, history, Situation
// or Cognition can reinterpret it as an ordinary conversational acknowledgement.
func TestB96441_AuthorizationContinuationPrecedesConversationalResponseGate(t *testing.T) {
	engineSrc, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	engine := string(engineSrc)
	process := strings.Index(engine, "func (e *Engine) ProcessWithChannel(input string, channel interaction.InputChannel) {")
	if process < 0 {
		t.Fatal("ProcessWithChannel not found")
	}
	body := engine[process:]
	conversation := strings.Index(body, "conversationCleanup := e.conversationStatePhase(t)")
	resume := strings.Index(body, "if e.resumePendingAuthorizationIfConfirmed(t) {")
	evidence := strings.Index(body, "// INPUT → CONTEXT → EVIDENCE")
	if conversation < 0 || resume < 0 || evidence < 0 {
		t.Fatalf("required pre-cognition boundaries missing: conversation=%d resume=%d evidence=%d", conversation, resume, evidence)
	}
	if resume < conversation || resume > evidence {
		t.Fatalf("authorization continuation is not between conversation state and evidence: conversation=%d resume=%d evidence=%d", conversation, resume, evidence)
	}

	pipelineSrc, err := os.ReadFile("turn_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	pipeline := string(pipelineSrc)
	decision := strings.Index(pipeline, "func (e *Engine) decisionPhase(t *processTurn) bool {")
	if decision < 0 {
		t.Fatal("decisionPhase not found")
	}
	decisionBody := pipeline[decision:]
	if strings.Contains(decisionBody, "if e.resumePendingAuthorizationIfConfirmed(t) {") {
		t.Fatal("authorization continuation must have one canonical pre-cognition boundary, not a duplicate decision-phase path")
	}
}

func TestB96441_RawConfirmationActSurvivesContextEnrichmentBoundary(t *testing.T) {
	e := newB963334AuditEngine(t)
	turn := &processTurn{input: "sí, autorizo", channel: interaction.ChannelText}
	cleanup := e.inputContextPhase(turn)
	defer cleanup()
	if !turn.authorizationActs.Has(interaction.ActConfirmation) {
		t.Fatalf("raw authorization confirmation was not captured at input boundary: %+v", turn.authorizationActs)
	}
}
