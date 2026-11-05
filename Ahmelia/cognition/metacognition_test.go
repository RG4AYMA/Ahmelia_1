package cognition

import (
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
)

func TestEvaluateMetacognitionConsumesDiagnosticsNotFinishedResponse(t *testing.T) {
	now := time.Now()
	d := interaction.NewDiagnosticResult(interaction.DiagnosticInternalState, "cognition.internal_state", "runtime", .95, now)
	d.AddFact("current", "Pensando", .95)
	d.AddFact("detail", "usando significado semántico", .90)

	req := interaction.SemanticRequest{
		Raw:        "¿Qué estás haciendo?",
		Normalized: "que estas haciendo",
		Language:   "es",
		Confidence: .99,
		CommunicativeActs: interaction.CommunicativeActs{
			Primary: interaction.ActQuestion,
		},
	}
	decision, ok := EvaluateMetacognition(req, interaction.ConversationSnapshot{Language: "es"}, []interaction.DiagnosticResult{d})
	if !ok {
		t.Fatal("expected metacognition decision")
	}
	if decision.Intent.Kind != interaction.ResponseIntentAnswer {
		t.Fatalf("intent=%s, want answer", decision.Intent.Kind)
	}
	if decision.Move != interaction.MoveAnswer {
		t.Fatalf("move=%s, want answer", decision.Move)
	}
	if len(decision.Evidence) != 2 {
		t.Fatalf("evidence=%d, want 2", len(decision.Evidence))
	}
	for _, evidence := range decision.Evidence {
		if !strings.HasPrefix(evidence.Text, "diagnostic[") {
			t.Fatalf("diagnostic crossed cognition as non-semantic text: %q", evidence.Text)
		}
	}
	if strings.Contains(strings.Join([]string{decision.Evidence[0].Text, decision.Evidence[1].Text}, " | "), "Estoy pensando") {
		t.Fatal("finished conversational wording leaked into diagnostic evidence")
	}
}

func TestEvaluateMetacognitionRejectsEmptyDiagnostics(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw: "¿Qué estás haciendo?", Normalized: "que estas haciendo", Language: "es", Confidence: .99,
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActQuestion},
	}
	if _, ok := EvaluateMetacognition(req, interaction.ConversationSnapshot{}, nil); ok {
		t.Fatal("metacognition must not answer without diagnostic evidence")
	}
}
