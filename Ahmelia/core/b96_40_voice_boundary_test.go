package core

import (
	"os"
	"strings"
	"testing"
)

func TestB9640VoicePublishesTurnAfterTTSBoundary(t *testing.T) {
	path := "../ai/Voice/voice_google_backup.py"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read voice bridge: %v", err)
	}
	text := string(src)
	searches := []struct {
		name  string
		start string
	}{
		{name: "wake", start: "wake, comando = extract_command(texto)"},
		{name: "direct", start: "if read_voice_mode() == \"direct\":"},
	}
	for _, branch := range searches {
		start := strings.Index(text, branch.start)
		if start < 0 {
			t.Fatalf("voice branch %q not found", branch.name)
		}
		end := strings.Index(text[start:], "return")
		if end < 0 {
			t.Fatalf("voice branch %q has no return", branch.name)
		}
		chunk := text[start : start+end]
		wait := strings.Index(chunk, "wait_for_tts_to_finish()")
		printMarker := "emit_turn(comando"
		if branch.name == "direct" {
			printMarker = "emit_turn(texto"
		}
		print := strings.Index(chunk, printMarker)
		if wait < 0 || print < 0 || wait > print {
			t.Fatalf("voice branch %q publishes input before TTS boundary", branch.name)
		}
		turnWait := strings.Index(chunk, "wait_for_turn_response()")
		if turnWait < 0 || turnWait < print {
			t.Fatalf("voice branch %q reopens input without waiting for Go turn processing and response", branch.name)
		}
	}
}
