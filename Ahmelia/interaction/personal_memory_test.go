package interaction

import (
	"path/filepath"
	"testing"
)

func TestPersonalMemoryPersistsEvidenceHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "personal_memory.json")
	m := NewPersonalMemory()
	first := m.Record("preference", "favorite_color", "azul", "explicit_self_report", "session-1", 3, 0.72)
	second := m.Record("preference", "favorite_color", "azul", "answer_to_personal_question", "session-1", 5, 0.91)

	if first.ID == second.ID {
		t.Fatal("personal memory entries must have distinct IDs")
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded := LoadPersonalMemory(path)
	got := loaded.Find("preference", "favorite_color")
	if len(got) != 2 {
		t.Fatalf("expected two evidence entries, got %d", len(got))
	}
	if got[1].TurnID != 5 || got[1].Source != "answer_to_personal_question" {
		t.Fatalf("unexpected latest evidence: %+v", got[1])
	}
}

func TestPersonalMemoryRecentReturnsIndependentCopy(t *testing.T) {
	m := NewPersonalMemory()
	m.Record("habit", "work_time", "noche", "conversation", "s", 1, 0.7)
	got := m.Recent(1)
	got[0].Value = "alterado"
	again := m.Recent(1)
	if again[0].Value != "noche" {
		t.Fatal("recent snapshot must not mutate live memory")
	}
}

func TestPersonalMemoryLoadMissingReturnsCleanStore(t *testing.T) {
	m := LoadPersonalMemory(filepath.Join(t.TempDir(), "missing.json"))
	if m == nil || len(m.Recent(1)) != 0 {
		t.Fatal("missing personal memory should fall back to an empty store")
	}
}
