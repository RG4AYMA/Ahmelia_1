package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/selfknowledge"
)

func newB963334AuditEngine(t *testing.T) *Engine {
	t.Helper()
	oldDir, _ := os.Getwd()
	_ = os.Chdir(t.TempDir())
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	_ = os.Remove(interaction.UserModelPath)
	_ = os.Remove(interaction.ConversationStatePath)
	t.Cleanup(func() {
		_ = os.Remove(interaction.UserModelPath)
		_ = os.Remove(interaction.ConversationStatePath)
	})
	// Tests must not spawn the real TTS process. Speak/SpeakAsync starts
	// an external Python process that inherits the test working directory.
	// On Windows, that child can still be alive when t.TempDir() cleanup
	// runs, causing RemoveAll to fail with ERROR_SHARING_VIOLATION.
	// B96.34 behavioral tests validate Engine behavior, not audio output.
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(16)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "test")

	// B96.34 test isolation: NewEngine normally loads the durable UserModel
	// from the current working directory. The core package contains legacy
	// tests that temporarily change the process-wide cwd, and some runtime
	// paths can outlive the assertion that created them. That made this
	// behavioral fixture occasionally inherit another test's model (the
	// classic symptom was favorite_color=azul with SourceID=turn-1 before
	// the red observation was even evaluated). The B96.34 behavioral tests
	// must start from a fresh canonical store; they still exercise the real
	// Engine -> UserObservation -> RunUserModelUpdateE2E path. Persistence
	// is covered separately by TestB9634BehavioralPersistenceAndQuery.
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	store, err := interaction.NewUserModelStore(interaction.NewUserModel("default-user"))
	if err != nil {
		t.Fatalf("create isolated B96.34 UserModelStore: %v", err)
	}
	e.userModelStore = store
	installF9LanguageStubs(t, e)
	approveTestProductionExecution(t, e)
	return e
}

// Architectural Reality Test: an ambiguous executable request must be
// stopped by B96.33 before the capability history records an execution.
func TestB9633BehavioralAuthorityBlocksAmbiguousAction(t *testing.T) {
	e := newB963334AuditEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("abre eso")

	if !e.lastConversationalCognition.Valid {
		t.Fatalf("B96.33 cognition invalid: %v", e.lastConversationalCognition.Errors)
	}
	if !e.lastConversationalCognition.Interpretation.HasUnresolvedRefs {
		t.Fatal("expected B96.33 to expose the unresolved reference")
	}
	if e.lastConversationalCognition.Action.Primary != "clarify" || !e.lastConversationalCognition.Action.Blocked {
		t.Fatalf("expected cognitive clarification/block, got action=%s blocked=%v", e.lastConversationalCognition.Action.Primary, e.lastConversationalCognition.Action.Blocked)
	}
	if len(e.history.GetLast(16)) != 0 {
		t.Fatalf("ambiguous request reached execution history: %#v", e.history.GetLast(16))
	}
	if len(responses) == 0 || !strings.Contains(strings.ToLower(responses[0]), "refieres") {
		t.Fatalf("expected clarification response, got %v", responses)
	}
}

// Architectural Reality Test: B96.34 must survive a new Engine instance and
// answer from its scoped query API, not from direct JSON access or the legacy
// UserProfile.
func TestB9634BehavioralPersistenceAndQuery(t *testing.T) {
	e := newB963334AuditEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	e.Process("Mi color favorito es azul")
	if e.lastConversationalCognition.Version != "B96.33.12" || !e.lastConversationalCognition.Valid {
		t.Fatalf("B96.33 was not part of the first turn: %+v", e.lastConversationalCognition)
	}
	if _, err := os.Stat(interaction.UserModelPath); err != nil {
		t.Fatalf("B96.34 did not persist its model: %v", err)
	}

	// A new Engine proves the second interaction is not reading an in-memory
	// cache from the first Engine. ConversationState is also cleared so the
	// answer cannot come from the prior dialogue turn.
	_ = os.Remove(interaction.ConversationStatePath)
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(16)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "test")
	e2 := NewEngine(sess, memory.LoadLongMemory(), &shortMem, history, selfQuery)
	responses = nil
	e2.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	e2.Process("¿Cuál es mi color favorito?")

	if len(responses) == 0 || responses[len(responses)-1] != "Tu color favorito es azul." {
		t.Fatalf("B96.34 query did not change observable behavior: %v", responses)
	}
	if e2.lastConversationalCognition.Intent.Primary != "question" || e2.lastConversationalCognition.Goal.Primary != "get_information" {
		t.Fatalf("B96.33 did not classify the user-model query as information seeking: intent=%s goal=%s", e2.lastConversationalCognition.Intent.Primary, e2.lastConversationalCognition.Goal.Primary)
	}
}
