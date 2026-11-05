package core

import (
	"os"
	"strings"
	"testing"
)

// B96.35 architecture audit: historical recall must sit after semantic
// interpretation exists, but before the generic interaction response gate.
// This prevents a valid historical question from being swallowed by a
// conversational fallback such as "Te escucho". The test audits the actual
// Process source rather than duplicating its implementation in the test.
func TestB9635_HistoricalRecallBridgePrecedesResponseGate(t *testing.T) {
	phaseSource, err := os.ReadFile("turn_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(phaseSource)
	collector := strings.Index(text, "t.responseSpecialistResults = e.collectResponseSpecialistResults(")
	situationEvidence := strings.Index(text, "t.responseSpecialistResults.Contributions(time.Now())")
	inputPhase, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	engineText := string(inputPhase)
	semantic := strings.Index(engineText, "e.inputContextPhase(t)")
	evidence := strings.Index(engineText, "e.evidencePhase(t)")
	situation := strings.Index(engineText, "e.situationPhase(t)")
	cognition := strings.Index(engineText, "e.cognitionPhase(t)")
	dialogue := strings.Index(engineText, "e.dialoguePhase(t)")
	decision := strings.Index(engineText, "e.decisionPhase(t)")
	preExecution := strings.Index(engineText, "e.preExecutionPhase(t)")
	specialist := strings.Index(engineText, "e.renderSpecialistResponseResults(t.semanticReq, t.responseSpecialistResults)")
	gate := strings.Index(engineText, "if conversationalResponseEligible(t.semanticReq, e.lastResponseIntent)")
	if collector < 0 || situationEvidence < 0 || semantic < 0 || evidence < 0 || situation < 0 || cognition < 0 || dialogue < 0 || decision < 0 || preExecution < 0 || specialist < 0 || gate < 0 {
		t.Fatalf("missing current-turn stages: semantic=%d evidence=%d collector=%d situation=%d responseEvidence=%d cognition=%d dialogue=%d decision=%d specialist=%d gate=%d preExecution=%d", semantic, evidence, collector, situation, situationEvidence, cognition, dialogue, decision, specialist, gate, preExecution)
	}
	if !(semantic < evidence && evidence < situation && situation < cognition && cognition < dialogue && dialogue < decision && decision < preExecution) {
		t.Fatalf("invalid Process phase order: semantic=%d evidence=%d situation=%d cognition=%d dialogue=%d decision=%d preExecution=%d", semantic, evidence, situation, cognition, dialogue, decision, preExecution)
	}
	if specialist > gate {
		t.Fatalf("specialist result must reach canonical handling before generic response fallback: specialist=%d gate=%d", specialist, gate)
	}
	resolver, err := os.ReadFile("response_specialist_results.go")
	if err != nil {
		t.Fatal(err)
	}
	resultLayer := string(resolver)
	if !strings.Contains(resultLayer, "ResolveHistoricalRecallExcluding") {
		t.Fatal("historical result layer does not resolve through B96.35")
	}
	if !strings.Contains(resultLayer, "conversationEpisodeID") {
		t.Fatal("historical result layer does not exclude the current episode")
	}
}

// B96.35 authority audit: the Engine must not keep a second historical
// reconstruction path. Direct Reconstruct/legacy Recall calls in Process
// would let the same historical question bypass the evidence model.
func TestB9635_HistoricalAuthorityHasNoLegacyReconstructionCalls(t *testing.T) {
	source, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "e.conversationHistory.Reconstruct(") {
		t.Fatal("engine.go still contains a direct ConversationHistory.Reconstruct call; historical reconstruction must go through B96.35")
	}
	if strings.Contains(text, "e.conversationalMemory.Recall(") {
		t.Fatal("engine.go still contains a legacy ConversationalMemory.Recall call; historical recall must go through B96.35")
	}
}
