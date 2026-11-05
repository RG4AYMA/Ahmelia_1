package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/selfknowledge"
)

// B96.22.30 — Interaction E2E. These tests exercise the real Engine.Process
// boundary rather than individual interaction packages. The purpose is to
// verify that meaning survives the complete path from human input through
// semantic evaluation, conversation state, deterministic cognition and the
// existing capability/work boundaries.
func newInteractionE2EEngine(t *testing.T) *Engine {
	isolateTestWorkspace(t)
	t.Helper()
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_WORKSPACE_ROOT", filepath.Join(t.TempDir(), "workspace"))
	_ = os.Remove("memory/conversation_state.json")
	_ = os.Remove("memory/user_profile.json")
	t.Cleanup(func() {
		_ = os.Remove("memory/conversation_state.json")
		_ = os.Remove("memory/user_profile.json")
	})

	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(4)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "interaction-e2e")
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	registerCoreTestProduction(e, t)
	// This fixture verifies the Engine integration without depending on the
	// runtime Ollama goal decomposer. Replanning is deterministic in these E2E
	// scenarios; decomposer behavior is covered at its own boundary.
	e.planner.SetGoalTaskDecomposer(nil)
	installF9LanguageStubs(t, e)
	approveTestProductionExecution(t, e)
	return e
}

func TestB9630E2E_QuestionFlowsThroughInteractionCore(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("¿cuánto es 2 + 2?")

	if len(responses) == 0 {
		t.Fatal("expected a response")
	}
	if got := responses[len(responses)-1]; got != "4" {
		t.Fatalf("got %q, want deterministic answer 4", got)
	}

	snap := e.conversationState.Snapshot()
	if len(snap.Turns) == 0 {
		t.Fatal("interaction turn was not persisted")
	}
	if snap.Turns[len(snap.Turns)-1].Kind == "" {
		t.Fatal("semantic turn kind was not recorded")
	}
}

func TestB9630E2E_ConversationLearnsFromShortAnswer(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("¿cuál es mi bebida favorita?")
	e.Process("café")

	model, err := interaction.LoadUserModelStore(interaction.UserModelPath, "ayma240")
	if err != nil {
		t.Fatalf("formal UserModel could not be loaded: %v", err)
	}
	prefs, err := interaction.GetRelevantPreferences(model, interaction.UserModelActorCognition, interaction.UserModelQueryContext{Topics: []string{"bebida favorita"}})
	if err != nil || len(prefs) != 1 {
		t.Fatalf("short answer was not captured by B96.34: err=%v prefs=%v", err, prefs)
	}
	if !strings.EqualFold(prefs[0].Value, "café") {
		t.Fatalf("favorite drink=%q, want café", prefs[0].Value)
	}

	snap := e.conversationState.Snapshot()
	if len(snap.Turns) < 2 {
		t.Fatalf("expected two conversation turns, got %d", len(snap.Turns))
	}
}

func TestB9630E2E_CorrectionPreservesConversationContinuity(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("Quiero crear un programa Go llamado alfa")
	e.Process("No, me refería a beta")

	snap := e.conversationState.Snapshot()
	if len(snap.Turns) < 2 {
		t.Fatalf("expected correction to remain in conversation state, got %d turns", len(snap.Turns))
	}
	last := snap.Turns[len(snap.Turns)-1]
	if last.Relation != "correction" && last.Kind != "correction" {
		t.Fatalf("correction semantic marker was lost: relation=%q kind=%q", last.Relation, last.Kind)
	}
	if !strings.Contains(strings.ToLower(last.UserText), "beta") {
		t.Fatalf("correction turn lost user text: %q", last.UserText)
	}
	if len(responses) == 0 {
		t.Fatal("expected the canonical response gate to answer the correction")
	}
	got := strings.ToLower(strings.TrimSpace(responses[len(responses)-1]))
	if got == "" {
		t.Fatal("canonical correction response was empty")
	}
	if strings.Contains(got, "no supe qué hacer") {
		t.Fatalf("correction bypassed the canonical response gate: %q", responses[len(responses)-1])
	}
	if e.lastResponseIntent.Kind != interaction.ResponseIntentAcknowledge {
		t.Fatalf("response intent=%q, want acknowledge for correction", e.lastResponseIntent.Kind)
	}
	if e.lastNaturalDialogue.Move != interaction.MoveAcknowledge {
		t.Fatalf("dialogue move=%q, want acknowledge for correction", e.lastNaturalDialogue.Move)
	}
}

func TestB9639E2E_OwnershipCorrectionReachesResponseGate(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("tranquila pronto podrás empezar a ver el mundo, hare todo lo posible para que puedas empezar a ver por ti misma, asi podras saber cual es tu color favorito")
	e.Process("pero no es para mí es para ti para que puedas saber que te gusta")

	if e.lastResponseIntent.Kind != interaction.ResponseIntentAcknowledge {
		t.Fatalf("response intent=%q, want acknowledge for ownership correction", e.lastResponseIntent.Kind)
	}
	if e.lastNaturalDialogue.Move != interaction.MoveAcknowledge {
		t.Fatalf("dialogue move=%q, want acknowledge for ownership correction", e.lastNaturalDialogue.Move)
	}
	if len(responses) == 0 {
		t.Fatal("expected a response to the ownership correction")
	}
	got := strings.ToLower(strings.TrimSpace(responses[len(responses)-1]))
	if strings.Contains(got, "no supe qué hacer") {
		t.Fatalf("ownership correction still bypassed the response gate: %q", responses[len(responses)-1])
	}
}

func TestB9630E2E_UnknownQuestionDoesNotInventAnswer(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("¿qué descubrió ayer una civilización desconocida en Marte?")

	if len(responses) == 0 {
		t.Fatal("expected an honest response")
	}
	got := strings.ToLower(responses[len(responses)-1])
	if strings.Contains(got, "civilización descubrió") || strings.Contains(got, "marte descubrió") {
		t.Fatalf("response appears to invent knowledge: %q", responses[len(responses)-1])
	}
	if !strings.Contains(got, "no") && !strings.Contains(got, "entiendo") {
		t.Fatalf("expected uncertainty/honesty response, got %q", responses[len(responses)-1])
	}
}

func TestB9630E2E_InteractionStateSurvivesIndependentTurn(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("¿qué hacemos ahora?")
	e.Process("gracias")

	snap := e.conversationState.Snapshot()
	if len(snap.Turns) < 2 {
		t.Fatalf("conversation continuity was lost; turns=%d", len(snap.Turns))
	}
	if snap.Turns[0].UserText == "" || snap.Turns[1].UserText == "" {
		t.Fatal("raw conversational turns were not retained")
	}
}

func TestB9639E2E_LegacyDialogueManagerCannotShortCircuit(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("cambiemos de tema")

	if e.lastNaturalDialogue.Move != interaction.MoveRedirect {
		t.Fatalf("B96.39 move=%q, want redirect", e.lastNaturalDialogue.Move)
	}
	if len(responses) == 0 {
		t.Fatal("expected a conversational response")
	}
	for _, response := range responses {
		if response == "Dale, cambiamos de tema. ¿De qué querés hablar?" {
			t.Fatalf("legacy hardcoded dialogue response still reached runtime: %q", response)
		}
	}
}
