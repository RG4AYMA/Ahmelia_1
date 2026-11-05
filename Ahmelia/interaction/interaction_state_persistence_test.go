package interaction

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInteractionStatePersistenceRoundTrip(t *testing.T) {
	conversation := NewConversationState()
	req := SemanticRequest{
		Raw:        "continúa con la escena",
		Kind:       KindContinuation,
		SpeechAct:  SpeechCommand,
		Topic:      "escena",
		Language:   "es",
		Channel:    ChannelText,
		Confidence: 0.97,
		Objective:  Objective{Text: "mejorar la escena"},
	}
	turnID := conversation.RecordTurn(req, "Seguimos con la escena.")
	conversation.AddDecision("escena", "priorizar interacción", turnID)
	conversation.AddCommitment("shared", "revisar la interacción", turnID)
	thread := conversation.EnsureThread("escena", "mejorar la escena")
	conversation.SetActiveWork("work-23")

	state := NewInteractionState(conversation)
	state.Observe(req, WorkContext{ID: "work-23", Status: "running", Provider: "test", CurrentStep: "design", LastResult: "ok"}, true)

	path := filepath.Join(t.TempDir(), "interaction-state.json")
	if err := state.Save(path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded := LoadInteractionState(path)
	snap := loaded.Snapshot()
	if snap.SessionID != conversation.SessionID {
		t.Fatalf("session mismatch: got %q want %q", snap.SessionID, conversation.SessionID)
	}
	if snap.Semantic.Raw != req.Raw || snap.Focus.Topic != "escena" {
		t.Fatalf("semantic/focus state not restored: %+v", snap)
	}
	if snap.Work.ID != "work-23" || !snap.HasWork {
		t.Fatalf("work projection not restored: %+v", snap.Work)
	}
	if len(snap.Conversation.Turns) != 1 || len(snap.Conversation.Decisions) != 1 || len(snap.Conversation.Commitments) != 1 {
		t.Fatalf("conversation state not restored: turns=%d decisions=%d commitments=%d", len(snap.Conversation.Turns), len(snap.Conversation.Decisions), len(snap.Conversation.Commitments))
	}
	if snap.Conversation.OpenThreads[0].ID != thread.ID {
		t.Fatalf("thread identity not restored")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat persisted state: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatalf("interaction state must be private, mode=%o", info.Mode().Perm())
	}
}

func TestInteractionStatePersistenceKeepsPendingResumeAnchors(t *testing.T) {
	conversation := NewConversationState()
	req := SemanticRequest{Raw: "trabajemos en Unity", Kind: KindContinuation, Topic: "Unity", Channel: ChannelText, Confidence: 0.9}
	conversation.RecordTurn(req, "Sí.")
	state := NewInteractionState(conversation)
	state.Observe(req, WorkContext{ID: "work-unity", Status: "running"}, true)
	point := state.Interrupt("cambio temporal")
	if point.ID == "" {
		t.Fatal("expected resume anchor")
	}

	path := filepath.Join(t.TempDir(), "interaction-state.json")
	if err := state.Save(path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	loaded := LoadInteractionState(path)
	pending := loaded.PendingResumePoints()
	if len(pending) != 1 || pending[0].ID != point.ID {
		t.Fatalf("pending resume anchor not restored: %+v", pending)
	}
	resumed, ok := loaded.Resume()
	if !ok || resumed.ID != point.ID {
		t.Fatalf("restored interaction did not resume: %+v ok=%v", resumed, ok)
	}
	if got := loaded.Snapshot().Metadata.Status; got != InteractionActive {
		t.Fatalf("expected active after resume, got %q", got)
	}
}

func TestLoadInteractionStateMissingOrCorruptReturnsCleanState(t *testing.T) {
	missing := LoadInteractionState(filepath.Join(t.TempDir(), "missing.json"))
	if missing == nil || missing.Snapshot().SessionID == "" {
		t.Fatal("missing state should return a clean initialized state")
	}

	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	corrupt := LoadInteractionState(path)
	if corrupt == nil || corrupt.Snapshot().Metadata.Status != InteractionIdle {
		t.Fatal("corrupt state should return a clean idle state")
	}
}
