package tts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVoiceTurnActivityMarkerTracksQueuedTurnLifecycles(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.MkdirAll("memory", 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join("memory", "voice_turn_active.txt")

	if err := BeginVoiceTurn(); err != nil {
		t.Fatalf("begin first voice turn: %v", err)
	}
	if err := BeginVoiceTurn(); err != nil {
		t.Fatalf("begin second voice turn: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("active voice turn marker missing: %v", err)
	}
	EndVoiceTurn()
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("marker cleared while another queued voice turn remains: %v", err)
	}
	EndVoiceTurn()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker remains after all voice turns ended: err=%v", err)
	}
}
