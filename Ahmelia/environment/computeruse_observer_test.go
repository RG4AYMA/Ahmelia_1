package environment

import "testing"

func TestComputerUseSnapshotUsesWorldModelEvidence(t *testing.T) {
	m := NewManager()
	m.RegisterFocused("code")

	snap, ok := m.ComputerUseSnapshot()
	if !ok {
		t.Fatal("expected an observable active application")
	}
	if snap.Source != "environment.world_model" {
		t.Fatalf("unexpected source %q", snap.Source)
	}
	if snap.Fingerprint == "" || snap.ID == "" {
		t.Fatal("expected stable observation identity")
	}
	if snap.UI["app"] == "" {
		t.Fatal("expected app evidence")
	}
}

func TestComputerUseSnapshotFingerprintChangesWithObservableState(t *testing.T) {
	m := NewManager()
	focused := "code"
	m.foregroundProcess = func() (string, uint32, error) { return focused, 0, nil }
	m.RegisterFocused("code")
	first, ok := m.ComputerUseSnapshot()
	if !ok {
		t.Fatal("expected first observation")
	}
	focused = "chrome"
	m.RegisterFocused("chrome")
	second, ok := m.ComputerUseSnapshot()
	if !ok {
		t.Fatal("expected second observation")
	}
	if first.Fingerprint == second.Fingerprint {
		t.Fatal("observable state change must change fingerprint")
	}
}
