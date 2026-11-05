package tts

import (
	"sync"
	"testing"
)

func TestSpeechMuExists(t *testing.T) {
	var mu *sync.Mutex = &speechMu
	if mu == nil {
		t.Fatal("speech mutex missing")
	}
}
