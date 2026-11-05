package core

import (
	"context"
	"os"
	"strings"
	"testing"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/language"
)

func TestPhase7MetacognitionHasNoDirectNarrativeOutputPath(t *testing.T) {
	body, err := os.ReadFile("metacognitive_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if strings.Contains(s, ".Narrative(") {
		t.Fatal("metacognitive runtime must consume diagnostic state, not subsystem Narrative output")
	}
	if strings.Contains(s, "sayAsync(") || strings.Contains(s, "tts.Speak") {
		t.Fatal("metacognitive subsystem bridge must not own final output")
	}

	engine, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	engineText := string(engine)
	if !strings.Contains(engineText, "if e.evaluateMetacognition(t.semanticReq) {") {
		t.Fatal("interactive decision does not evaluate metacognitive diagnostics")
	}
	if !strings.Contains(s, "return e.renderResponseContract(req, contract,") {
		t.Fatal("metacognitive diagnostics do not reach the shared response contract renderer")
	}
}

func TestPhase7MetacognitionReachesLanguageWithoutFinishedSubsystemNarrative(t *testing.T) {
	client := &phase7FakeLanguageClient{available: true, response: `{"response":"Ahora mismo estoy procesando tu pregunta y revisando lo que tengo activo."}`}
	e := &Engine{
		conversationState:     interaction.NewConversationState(),
		internalState:         cognition.NewStateManager(),
		pipeline:              cognition.NewPipeline(),
		conversationGenerator: language.NewConversationalGenerator(client),
	}
	e.internalState.Enter(cognition.StateThinking, "usando significado semántico")
	e.pipeline.Enter(cognition.StageListen, "turno")
	e.pipeline.Enter(cognition.StageComprehend, "pregunta metacognitiva")

	req := interaction.SemanticRequest{
		Raw:        "¿Qué estás haciendo?",
		Normalized: "que estas haciendo",
		Language:   "es",
		Confidence: .99,
		CommunicativeActs: interaction.CommunicativeActs{
			Primary: interaction.ActQuestion,
		},
	}
	if !e.evaluateMetacognition(req) {
		t.Fatal("metacognition did not reach the canonical response renderer")
	}
	if !strings.Contains(client.prompt, "diagnostic[internal_state] current=Pensando") {
		t.Fatalf("Language did not receive diagnostic evidence: %q", client.prompt)
	}
	if strings.Contains(client.prompt, "Ahora mismo estoy procesando tu pregunta") {
		t.Fatal("finished language output appeared upstream of Language")
	}
}

func TestCanonicalSystemResponseUsesLanguageRendererWithGroundedDecision(t *testing.T) {
	client := &phase7FakeLanguageClient{available: true, response: `{"response":"La ejecución sigue pendiente; todavía no he modificado ningún archivo."}`}
	e := &Engine{
		conversationState:     interaction.NewConversationState(),
		conversationGenerator: language.NewConversationalGenerator(client),
		flowTrace:             NewTurnFlowTrace("authorization pending"),
	}
	req := interaction.SemanticRequest{Raw: "¿En qué quedó?", Language: "es"}
	contract := interaction.NewResponseContract(req, interaction.ResponseIntent{Kind: interaction.ResponseIntentClarify, ShouldSpeak: true}, interaction.MoveClarify)
	contract.Source = "deterministic_system_response"
	contract.AddEvidence("La ejecución de programming.create_file sigue pendiente de autorización; no se ha ejecutado.", contract.Source, true, .99)

	got, ok := e.generateResponseContractWithStrategy(req, contract, language.RuntimeFacts{}, cognition.ConversationalResponseStrategy{})
	if !ok || got != "La ejecución sigue pendiente; todavía no he modificado ningún archivo." {
		t.Fatalf("system decision was not rendered by Language: got=%q ok=%t", got, ok)
	}
	if !strings.Contains(client.prompt, "La ejecución de programming.create_file sigue pendiente de autorización; no se ha ejecutado.") {
		t.Fatalf("Language did not receive the grounded operational decision: %q", client.prompt)
	}
	if !strings.Contains(strings.Join(e.flowTrace.Snapshot(), "\n"), "renderer=model:") {
		t.Fatal("trace does not show the configured language renderer")
	}
}

type phase7FakeLanguageClient struct {
	available bool
	response  string
	prompt    string
}

func (f *phase7FakeLanguageClient) Available() bool { return f.available }
func (f *phase7FakeLanguageClient) GenerateContext(_ context.Context, prompt string) (string, error) {
	f.prompt = prompt
	return f.response, nil
}

func TestPhase7MetacognitionFallbackDoesNotLeakDiagnosticEvidence(t *testing.T) {
	contract := interaction.ResponseContract{
		Source: "metacognitive_diagnostic",
		Move:   interaction.MoveAnswer,
		Evidence: []interaction.ResponseEvidence{{
			Text:     "diagnostic[internal_state] current=Pensando",
			Relevant: true,
		}},
	}
	fallback := responseFallback(contract)
	if strings.Contains(fallback, "diagnostic[") || strings.Contains(fallback, "current=Pensando") {
		t.Fatalf("metacognitive fallback leaked semantic evidence: %q", fallback)
	}
}
