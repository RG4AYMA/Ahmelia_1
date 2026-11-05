package core

import (
	"os"
	"path/filepath"
	"testing"

	"ahmelia/production"
)

func TestLocalActionVerificationUsesCanonicalPostconditionEngine(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "created")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	result := verifyLocalActionResult("crear_carpeta", map[string]string{"ruta": folder}, production.ProviderResult{
		Success: true, Summary: "creada", OutcomeCode: "completed",
	})
	if !result.Success || !result.Observed || !result.Verified {
		t.Fatalf("physical execution was not closed by real postcondition evidence: %+v", result)
	}
	if result.VerificationSource != "solving.verification_engine" {
		t.Fatalf("unexpected verification source=%q", result.VerificationSource)
	}
}

func TestLocalActionVerificationRejectsFalseWorldState(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	result := verifyLocalActionResult("crear_carpeta", map[string]string{"ruta": missing}, production.ProviderResult{
		Success: true, Summary: "creada", OutcomeCode: "completed",
	})
	if result.Success || result.Verified || !result.Observed || result.OutcomeStatus != production.OutcomeFailed {
		t.Fatalf("false postcondition was not represented as known failure: %+v", result)
	}
}

func TestVolumeObservationSemantics(t *testing.T) {
	cases := []struct {
		name                           string
		action                         string
		before, after                  int
		beforeOK                       bool
		muted                          bool
		expected                       int
		expectedOK                     bool
		wantVerified, wantKnownFailure bool
	}{
		{"bajar confirmado", "volumen_bajar", 70, 60, true, false, 0, false, true, false},
		{"bajar contradicho", "volumen_bajar", 70, 70, true, false, 0, false, false, true},
		{"bajar sin antes", "volumen_bajar", 0, 60, false, false, 0, false, false, false},
		{"silencio confirmado", "volumen_silencio", 70, 70, true, true, 0, false, true, false},
		{"silencio contradicho", "volumen_silencio", 70, 70, true, false, 0, false, false, true},
		{"set confirmado", "volumen_set", 70, 40, true, false, 40, true, true, false},
		{"set contradicho", "volumen_set", 70, 50, true, false, 40, true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verified, knownFailure, _ := evaluateVolumeObservation(tc.action, tc.before, tc.beforeOK, tc.after, tc.muted, tc.expected, tc.expectedOK)
			if verified != tc.wantVerified || knownFailure != tc.wantKnownFailure {
				t.Fatalf("got verified=%t knownFailure=%t", verified, knownFailure)
			}
		})
	}
}
