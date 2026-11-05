package core

import (
	"encoding/json"
	"os"
	"testing"

	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/selfknowledge"
)

// B96.34 ownership audit: conversational evidence must not mutate the legacy
// UserProfile. It may remain as a compatibility/authority source while the
// formal UserModelStore owns current user beliefs.
func TestB9634LegacyUserProfileIsNotAConversationalWriteTarget(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	dir := t.TempDir()
	oldDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	legacy := interaction.NewUserProfile()
	legacy.Remember("preference", "favorite_color", "green", "legacy_fixture", 0.95)
	raw, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("memory", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(interaction.UserProfilePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(8)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "legacy-ownership")
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	installF9LanguageStubs(t, e)

	e.Process("Mi color favorito es azul")

	after, err := os.ReadFile(interaction.UserProfilePath)
	if err != nil {
		t.Fatalf("legacy UserProfile disappeared: %v", err)
	}
	if string(after) != string(raw) {
		t.Fatal("legacy UserProfile was mutated by conversational processing; B96.34 must own current beliefs")
	}

	store, err := interaction.LoadUserModelStore(interaction.UserModelPath, legacy.PrimaryAuthority.ID)
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := interaction.GetRelevantPreferences(store, interaction.UserModelActorCognition, interaction.UserModelQueryContext{Topics: []string{"color favorito"}})
	if err != nil || len(prefs) != 1 || prefs[0].Value != "azul" {
		t.Fatalf("formal B96.34 UserModel did not become the current belief source: err=%v prefs=%v", err, prefs)
	}
}
