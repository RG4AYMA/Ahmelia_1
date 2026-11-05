package voiceprotocol

import (
	"strings"
	"testing"
)

func TestEncodeDecodePreservesMultilineTurnAsOneFrame(t *testing.T) {
	input := "Quiero que implementes una utilidad.\nObjetivo:\nCrea una función Go.\nCondiciones:\nEjecuta los tests."
	frame, err := Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(frame), "\n") {
		t.Fatalf("frame contains a physical newline: %q", frame)
	}
	got, err := Decode(string(frame))
	if err != nil {
		t.Fatal(err)
	}
	if got != input {
		t.Fatalf("got %q, want %q", got, input)
	}
}

func TestDecodeRejectsNonTurnMessage(t *testing.T) {
	if _, err := Decode(`{"type":"log","text":"hello"}`); err == nil {
		t.Fatal("expected unsupported message type error")
	}
}
