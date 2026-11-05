package core

import (
	"path/filepath"
	"testing"

	"ahmelia/interaction"
	"ahmelia/production"
)

func TestPersistentWorkStoreForwardsSkillIDToDurableAttempt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.json")
	store := NewPersistentWorkStore(production.NewWorkStore(path))
	work, err := store.Begin(interaction.SemanticRequest{Objective: interaction.Objective{Text: "crear herramienta"}})
	if err != nil {
		t.Fatal(err)
	}
	const skillID = "software.development"
	if _, err := store.BeginAttempt(work.ID, "programming.create_file", "create_file", "programming-provider", skillID); err != nil {
		t.Fatal(err)
	}

	reloaded := NewPersistentWorkStore(production.NewWorkStore(path))
	got, ok := reloaded.Get(work.ID)
	if !ok || len(got.Attempts) != 1 {
		t.Fatalf("durable work attempt missing after reload: work=%+v ok=%t", got, ok)
	}
	if got.Attempts[0].SkillID != skillID {
		t.Fatalf("skill identity not preserved through core adapter and reload: got=%q want=%q", got.Attempts[0].SkillID, skillID)
	}
}
