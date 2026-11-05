package core

import (
	"strings"
	"testing"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/situation"
	"ahmelia/solving"
)

func TestReasoningNeedAssessmentIsNotUserFacingResponseEvidence(t *testing.T) {
	bundle := specialistEvidenceBundle{Reasoning: &cognition.ReasoningContext{
		ReasoningNeed: &solving.ReasoningNeed{Score: 1, Signals: []solving.NeedSignal{{Reason: "objetivo largo", Points: 1}}},
	}}
	for _, evidence := range bundle.ResponseEvidence() {
		if strings.Contains(evidence.Text, "Necesidad de razonamiento") || evidence.Source == "cognitive_reasoning" && strings.Contains(evidence.Text, "puntaje") {
			t.Fatalf("internal reasoning assessment leaked into response evidence: %+v", evidence)
		}
	}
}

func TestPhase2KnowledgeCoreBecomesEvidence(t *testing.T) {
	e := buildTestEngine(t)
	bundle := e.collectSpecialistEvidence(interaction.Analyze("preséntate", interaction.Context{}))
	if bundle.Knowledge == nil {
		t.Fatal("expected Knowledge Core evidence")
	}
	if strings.TrimSpace(bundle.Knowledge.Content) == "" {
		t.Fatal("knowledge evidence content is empty")
	}
	for _, ev := range bundle.ResponseEvidence() {
		if ev.Source == "" || !ev.Relevant {
			t.Fatalf("invalid evidence contract: %+v", ev)
		}
	}
}

func TestLearningQuestionReachesItsTypedSelfKnowledgeOwner(t *testing.T) {
	for _, input := range []string{"¿Qué aprendiste?", "¿Cómo evolucionaste?", "¿Qué cambio basado en lo que funcionó?"} {
		t.Run(input, func(t *testing.T) {
			e := buildTestEngine(t)
			req := interaction.Analyze(input, interaction.Context{})
			if !interaction.HasSemanticQuery(req, interaction.QueryHistoryLearning) {
				t.Fatalf("query=%q, want typed learning query", req.Meaning.Query)
			}
			if !shouldCollectSpecialistEvidence(req, false) {
				t.Fatal("self-owned learning question was filtered before its knowledge owner")
			}
			bundle := e.collectSpecialistEvidence(req)
			if bundle.Knowledge == nil || bundle.Knowledge.Source != "memory" {
				t.Fatalf("typed learning owner was bypassed: %+v", bundle)
			}
			e.conversationGenerator = nil
			var response string
			e.onResponse = func(text string) { response = text }
			e.Process(input)
			if strings.TrimSpace(response) == "" || strings.Contains(strings.ToLower(response), "no supe qué hacer") {
				t.Fatalf("learning query did not produce grounded learning evidence: %q", response)
			}
		})
	}
}

func TestPhase2LegacyReasonerBecomesEvidence(t *testing.T) {
	e := buildTestEngine(t)
	bundle := e.collectSpecialistEvidence(interaction.Analyze("¿Qué analizas?", interaction.Context{}))
	if strings.TrimSpace(bundle.ReasonerText) == "" {
		t.Fatal("expected legacy Reasoner capability to remain available as evidence")
	}
	for _, ev := range bundle.ResponseEvidence() {
		if ev.Source == "reasoning:reasoner" && strings.TrimSpace(ev.Text) == "" {
			t.Fatal("reasoner evidence was empty")
		}
	}
}

func TestPhase2CognitiveMindBecomesEvidence(t *testing.T) {
	e := buildTestEngine(t)
	input := "necesito automatizar una tarea repetitiva del reporte semanal de ventas"
	bundle := e.collectSpecialistEvidence(interaction.Analyze(input, interaction.Context{}))
	if bundle.Reasoning == nil {
		t.Fatal("expected Cognitive Mind reasoning context")
	}
	if !bundle.Reasoning.Activated {
		t.Fatalf("expected Cognitive Mind activation, context=%+v", bundle.Reasoning)
	}
	if bundle.Reasoning.Decision == nil {
		t.Fatal("activated Cognitive Mind must expose a decision as specialist evidence")
	}
	if got := bundle.ResponseEvidence(); len(got) == 0 {
		t.Fatal("expected reasoning evidence")
	}
	for _, ev := range bundle.ResponseEvidence() {
		if strings.Contains(ev.Text, "piense un poco más") {
			t.Fatalf("legacy response wording leaked into evidence: %q", ev.Text)
		}
	}
}

func TestPhase2ReasoningCacheStillWorks(t *testing.T) {
	e := buildTestEngine(t)
	input := "necesito automatizar una tarea repetitiva del reporte semanal de ventas"

	first := e.collectSpecialistEvidence(interaction.Analyze(input, interaction.Context{}))
	if first.Reasoning == nil || !first.Reasoning.Activated {
		t.Fatalf("expected first reasoning activation: %+v", first.Reasoning)
	}
	statsAfterFirst := e.reasoningCache.Stats()
	if statsAfterFirst.Misses < 1 {
		t.Fatalf("expected first call to miss cache, stats=%+v", statsAfterFirst)
	}

	second := e.collectSpecialistEvidence(interaction.Analyze(input, interaction.Context{}))
	if second.Reasoning == nil || !second.Reasoning.Activated {
		t.Fatalf("expected cached reasoning activation: %+v", second.Reasoning)
	}
	statsAfterSecond := e.reasoningCache.Stats()
	if statsAfterSecond.Hits <= statsAfterFirst.Hits {
		t.Fatalf("expected second call to hit cache, stats=%+v", statsAfterSecond)
	}
}

func TestPhase2SpecialistEvidenceReachesCognition(t *testing.T) {
	e := buildTestEngine(t)
	input := interaction.Analyze("necesito automatizar una tarea repetitiva del reporte semanal de ventas", interaction.Context{})
	bundle := e.collectSpecialistEvidence(input)
	if bundle.Empty() {
		t.Fatal("expected specialist evidence")
	}

	ctx := interaction.ConversationSnapshot{}
	s := situation.Build(situation.Input{Request: input})
	contribution := bundle.Contribution(s.CreatedAt)
	cognitive := cognition.RunConversationalCognitionE2E(
		input,
		ctx,
		cognition.SituationalCognitionContext{
			Situation:     s,
			Contributions: []situation.Contribution{contribution},
		},
	)
	found := false
	for _, finding := range cognitive.DecisionEvidence.Findings {
		if finding.Perspective == "specialist_evidence" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("specialist evidence did not reach cognition decision surface: %+v", cognitive.DecisionEvidence.Findings)
	}
}

func TestPhase2CanonicalLanguageBoundary(t *testing.T) {
	e := buildTestEngine(t)
	req := interaction.Analyze("necesito automatizar una tarea repetitiva del reporte semanal de ventas", interaction.Context{})
	bundle := e.collectSpecialistEvidence(req)
	e.lastResponseIntent = interaction.DetermineResponseIntent(req, interaction.ConversationSnapshot{})
	e.lastNaturalDialogue.Move = interaction.MoveAnswer
	if !e.renderSpecialistEvidence(req, bundle) {
		t.Fatal("specialist evidence did not converge through ResponseContract/Language boundary")
	}
}

func TestPhase2ProcessE2EReasoningUsesCanonicalResponsePath(t *testing.T) {
	e := buildTestEngine(t)
	e.Process("¿Qué analizas?")

	if e.lastResponseIntent.Kind != interaction.ResponseIntentAnswer {
		t.Fatalf("expected canonical answer intent, got %+v", e.lastResponseIntent)
	}
	if !e.lastDecisionEvidence.HasKind("supports") {
		t.Fatalf("expected specialist evidence to survive the real Process() cognition cycle: %+v", e.lastDecisionEvidence.Findings)
	}
	snap := e.conversationState.Snapshot()
	if len(snap.Turns) == 0 {
		t.Fatal("Process() did not persist a turn")
	}
	last := snap.Turns[len(snap.Turns)-1]
	if strings.TrimSpace(last.Assistant) == "" {
		t.Fatal("canonical response path produced no assistant response")
	}
}
