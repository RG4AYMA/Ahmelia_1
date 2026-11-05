package core

import (
	"os"
	"testing"
	"time"

	"ahmelia/interaction"

	"ahmelia/memory"
)

func TestPhase6WorkflowSelectionPreservesMultiTurnContinuation(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	_ = os.Remove("memory/conversation_state.json")
	t.Cleanup(func() { _ = os.Remove("memory/conversation_state.json") })

	e := buildTestEngine(t)
	responses := make([]string, 0, 1)
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	e.sess.SetFileResults([]memory.SessionFile{{Path: "C:/tmp/uno.txt", LastModified: time.Now()}, {Path: "C:/tmp/dos.txt", LastModified: time.Now()}})

	e.Process("2")

	if !e.sess.WaitingForFileAction() {
		t.Fatalf("workflow selection did not advance to file-action state: %+v", e.sess.Snapshot())
	}
	if e.sess.LastSelectedFile != "C:/tmp/dos.txt" {
		t.Fatalf("selected file lost across workflow transition: %q", e.sess.LastSelectedFile)
	}
	if len(responses) == 0 {
		t.Fatal("workflow decision did not reach ResponseContract/Language output")
	}
}

func TestPhase6WorkflowStateDoesNotCreateCapabilityByItself(t *testing.T) {
	e := buildTestEngine(t)
	e.sess.SetFileResults([]memory.SessionFile{{Path: "C:/tmp/uno.txt"}})

	req := interaction.SemanticRequest{Raw: "2"}
	decision, evidence := e.evaluateWorkflow(req, "2")
	if evidence.State != interaction.WorkflowFileSelect {
		t.Fatalf("unexpected workflow state evidence: %+v", evidence)
	}
	if decision.Capability.Executable {
		t.Fatalf("selection workflow must not execute a capability before selection: %+v", decision)
	}
}
