package core

import (
	"strings"
	"testing"

	"ahmelia/interaction"
)

func TestResponseFallbackInitiativeNeverReturnsSemanticMetadata(t *testing.T) {
	contract := interaction.ResponseContract{Source: "initiative_intervention", Move: interaction.MoveInvite}
	got := responseFallback(contract)
	if got == "" {
		t.Fatal("expected initiative fallback")
	}
	for _, marker := range []string{"initiative action=", "content_mode=", "reason=", "topic=", "context="} {
		if strings.Contains(got, marker) {
			t.Fatalf("fallback leaked internal metadata %q: %q", marker, got)
		}
	}
}
