package dialogue

import (
	"os"
	"testing"
	"time"
)

func TestB9635CompleteEpisodePersistsAndReconstructs(t *testing.T) {
	path := t.TempDir() + "/history.json"
	h := NewConversationHistory()
	sid := "session-a"
	id := h.BeginSession(sid, time.Now().Add(-2*time.Hour))
	if !h.RecordTurn(id, HistoricalTurn{ID: 1, At: time.Now().Add(-2 * time.Hour), UserText: "Trabajamos en la escena principal de Blender", Assistant: "Sí", Topic: "Blender", Objective: "mejorar escena principal"}) {
		t.Fatal("record failed")
	}
	if !h.RecordTurn(id, HistoricalTurn{ID: 2, At: time.Now().Add(-90 * time.Minute), UserText: "Después hablamos de audio", Assistant: "Quedó pendiente", Topic: "audio"}) {
		t.Fatal("record failed")
	}
	if !h.CompleteSession(id, "Quedó pendiente mejorar la escena principal", time.Now().Add(-time.Hour)) {
		t.Fatal("complete failed")
	}
	if err := h.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded := LoadConversationHistory(path, nil)
	matches := loaded.Search("escena Blender", time.Now(), 5)
	if len(matches) == 0 || matches[0].Episode.SessionID != sid {
		t.Fatalf("expected historical match, got %+v", matches)
	}
	ctx := loaded.Reconstruct("escena principal", time.Now(), 3)
	if ctx.Reconstructed == "" {
		t.Fatal("expected reconstructed context")
	}
	_ = os.Remove(path)
}

func TestB9635RankingUsesRelevanceAndRecency(t *testing.T) {
	h := NewConversationHistory()
	now := time.Now()
	old := h.BeginSession("old", now.Add(-120*24*time.Hour))
	h.RecordTurn(old, HistoricalTurn{UserText: "Blender modelado escena principal", Topic: "Blender", At: now.Add(-120 * 24 * time.Hour)})
	h.CompleteSession(old, "old", now.Add(-120*24*time.Hour))
	recent := h.BeginSession("recent", now.Add(-2*24*time.Hour))
	h.RecordTurn(recent, HistoricalTurn{UserText: "Blender", Topic: "Blender", At: now.Add(-2 * 24 * time.Hour)})
	h.CompleteSession(recent, "recent", now.Add(-2*24*time.Hour))
	m := h.Search("Blender", now, 2)
	if len(m) != 2 || m[0].Episode.SessionID != "recent" {
		t.Fatalf("expected recent relevant result first, got %+v", m)
	}
}

func TestB9635RetentionPolicyIsFormal(t *testing.T) {
	h := NewConversationHistory()
	h.Retention = RetentionPolicy{MaxEpisodes: 2, MaxAge: 365 * 24 * time.Hour, MaxTurns: 10}
	now := time.Now()
	for i := 0; i < 3; i++ {
		id := h.BeginSession(string(rune('a'+i)), now.Add(time.Duration(i)*time.Hour))
		h.RecordTurn(id, HistoricalTurn{UserText: "tema", At: now})
		h.CompleteSession(id, "", now)
	}
	h.mu.Lock()
	h.applyRetentionLocked(now)
	h.mu.Unlock()
	if len(h.Episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(h.Episodes))
	}
}

func TestB9635MigratesB62Once(t *testing.T) {
	path := t.TempDir() + "/history.json"
	legacy := NewConversationalMemory()
	legacy.Touch("Unity")
	legacy.SetNote("Unity", "faltan materiales")
	h := LoadConversationHistory(path, legacy)
	if !h.LegacyB62Migrated {
		t.Fatal("expected migration marker")
	}
	if len(h.Episodes) != 1 || !h.Episodes[0].MigratedB62 {
		t.Fatalf("expected migrated episode, got %+v", h.Episodes)
	}
	h2 := LoadConversationHistory(path, legacy)
	if len(h2.Episodes) != 1 {
		t.Fatalf("migration must be idempotent, got %d", len(h2.Episodes))
	}
}

func TestB9635CrossSessionE2E(t *testing.T) {
	path := t.TempDir() + "/history.json"
	h := NewConversationHistory()
	id := h.BeginSession("first", time.Now().Add(-24*time.Hour))
	h.RecordTurn(id, HistoricalTurn{UserText: "Decidimos mejorar la iluminación del menú", Topic: "escena"})
	h.CompleteSession(id, "pendiente iluminación", time.Now().Add(-24*time.Hour))
	if err := h.Save(path); err != nil {
		t.Fatal(err)
	}
	h2 := LoadConversationHistory(path, nil)
	ctx := h2.Reconstruct("iluminación menú", time.Now(), 5)
	if len(ctx.Matches) != 1 || ctx.Matches[0].Episode.SessionID != "first" {
		t.Fatalf("cross-session retrieval failed: %+v", ctx)
	}
}
