package interaction

import (
	"path/filepath"
	"testing"
)

func TestUserProfilePersistsEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	p := NewUserProfile()
	p.Remember("preference", "favorite_color", "azul", "conversation", 0.72)
	p.Remember("preference", "favorite_color", "azul", "conversation", 0.72)
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded := LoadUserProfile(path)
	f, ok := loaded.Recall("preference", "favorite_color")
	if !ok || f.Value != "azul" || f.Evidence != 2 {
		t.Fatalf("unexpected persisted fact: %+v, %v", f, ok)
	}
}

func TestPersonalObservationIsConservative(t *testing.T) {
	o := DetectPersonalObservation("Mi color favorito es azul")
	if !o.OK || o.Key != "color favorito" || o.Value != "azul" {
		t.Fatalf("unexpected observation: %+v", o)
	}
}

func TestUserModelSnapshotIsIndependent(t *testing.T) {
	p := NewUserProfile()
	p.Remember("communication", "preferred_response_style", "directo", "conversation", 0.8)
	s := p.Snapshot()
	s.Communication["preferred_response_style"] = ProfileFact{Value: "alterado"}
	got, ok := p.Recall("communication", "preferred_response_style")
	if !ok || got.Value != "directo" {
		t.Fatalf("snapshot mutated live profile: %+v", got)
	}
	if !p.HasReliable("communication", "preferred_response_style") {
		t.Fatal("expected reliable communication preference")
	}
}

func TestUserModelDoesNotPromoteWeakObservationToReliable(t *testing.T) {
	p := NewUserProfile()
	p.Remember("preference", "favorite_color", "azul", "conversation", 0.50)
	if p.HasReliable("preference", "favorite_color") {
		t.Fatal("weak evidence must not be treated as reliable")
	}
}

func TestCommunicationPreferenceObservation(t *testing.T) {
	o := DetectPersonalObservation("Prefiero que me hables de forma directa")
	if !o.OK || o.Category != "communication" || o.Key != "preferred_response_style" {
		t.Fatalf("unexpected communication observation: %+v", o)
	}
}

func TestDetectPersonalObservationTrimsTerminalPunctuation(t *testing.T) {
	o := DetectPersonalObservation("Mi color favorito es negro.")
	if !o.OK || o.Value != "negro" {
		t.Fatalf("value=%q ok=%v want negro", o.Value, o.OK)
	}
}

func TestDetectPersonalObservationExtractsCleanValueFromDiscourseTail(t *testing.T) {
	cases := map[string]string{
		"Mi color favorito es negro, ya te lo había dicho": "negro",
		"Mi color favorito es negro ya te lo había dicho":  "negro",
		"Mi color favorito es negro porque me relaja":      "negro",
		"Mi color favorito es azul y verde":                "azul y verde",
	}
	for input, want := range cases {
		o := DetectPersonalObservation(input)
		if !o.OK || o.Value != want {
			t.Fatalf("input=%q value=%q ok=%v want=%q", input, o.Value, o.OK, want)
		}
	}
}
