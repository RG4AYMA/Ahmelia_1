package core

import (
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/situation"
)

func TestG32MentalStateUsesLiveMentalAgendaContent(t *testing.T) {
	e := buildTestEngine(t)
	e.mentalAgenda.SetFocus("G3 mental focus", "regression")
	e.mentalAgenda.Worry("G3 live concern", 3)

	req := interaction.Analyze("como te sientes", interaction.Context{})
	if req.Meaning.Query != interaction.QueryMentalState {
		t.Fatalf("semantic query=%q, want %q", req.Meaning.Query, interaction.QueryMentalState)
	}
	e.lastResponseIntent = interaction.ResponseIntent{Kind: interaction.ResponseIntentAnswer, ShouldSpeak: true, Confidence: 0.95, Language: req.Language}
	e.lastDecisionEvidence = situation.DecisionEvidence{Findings: []situation.DecisionFinding{{Kind: "asks_about_state"}}}

	var rendered string
	e.conversationGenerator = nil
	e.onResponse = func(msg string) { rendered = msg }
	turn := &processTurn{input: req.Raw, normInput: req.Normalized, semanticReq: req}
	if !e.evaluateMetacognition(turn.semanticReq) {
		t.Fatal("mental-state route did not handle the request")
	}
	if !strings.Contains(rendered, "G3 mental focus") {
		t.Fatalf("rendered answer lost live MentalAgenda content: %q", rendered)
	}
	if strings.Contains(rendered, "Estoy funcionando y lista para seguir contigo.") {
		t.Fatalf("route fell back to fixed runtime string despite live MentalAgenda data: %q", rendered)
	}
}
