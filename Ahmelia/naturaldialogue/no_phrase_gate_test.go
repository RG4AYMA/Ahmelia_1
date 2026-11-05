package naturaldialogue

import (
	"os"
	"strings"
	"testing"
)

func TestNoPhraseGateNaturalDialogueG2(t *testing.T) {
	data, err := os.ReadFile("initiative_intervention.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Whitelist: the initiative bridge only maps semantic Action/ContentMode to
	// dialogue movement. No raw user-text classification belongs here.
	for _, bad := range []string{"strings.Contains(", "strings.HasPrefix(", "strings.HasSuffix("} {
		if strings.Contains(src, bad) {
			t.Fatalf("dialogue intervention uses phrase gate %q", bad)
		}
	}
}
