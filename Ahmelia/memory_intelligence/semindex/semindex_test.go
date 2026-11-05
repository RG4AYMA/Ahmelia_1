package semindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHybridFindsAccentAndSubwordVariants(t *testing.T) {
	ix, _, _ := Open("", HashEmbedder{})
	ctx := context.Background()
	_ = ix.Add(ctx, Entry{ID: "a", Kind: KindEpisodic, Text: "El build de la capacidad de programación falló en la etapa de compilación"})
	_ = ix.Add(ctx, Entry{ID: "b", Kind: KindEpisodic, Text: "Conversamos sobre música para el desayuno"})
	h, _ := ix.Search(ctx, "falla de compilacion en programacion", Options{K: 2})
	if len(h) == 0 || h[0].Entry.ID != "a" {
		t.Fatalf("expected a first, got %+v", h)
	}
}

func TestEpisodicDecaysSemanticDoesNot(t *testing.T) {
	ix, _, _ := Open("", HashEmbedder{})
	ctx := context.Background()
	old := time.Now().Add(-60 * 24 * time.Hour)
	_ = ix.Add(ctx, Entry{ID: "e", Kind: KindEpisodic, Text: "proyecto atlas pendiente", At: old})
	_ = ix.Add(ctx, Entry{ID: "s", Kind: KindSemantic, Text: "proyecto atlas pendiente", At: old})
	h, _ := ix.Search(ctx, "proyecto atlas", Options{K: 2})
	if len(h) != 2 || h[0].Entry.ID != "s" {
		t.Fatalf("semantic must outrank decayed episodic: %+v", h)
	}
}

func TestJournalSurvivesRestartAndCorruptLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "idx.jsonl")
	ix, _, _ := Open(p, HashEmbedder{})
	ctx := context.Background()
	_ = ix.Add(ctx, Entry{ID: "a", Kind: KindSemantic, Text: "hola mundo"})
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("{corrupto\n")
	f.Close()
	ix2, bad, err := Open(p, HashEmbedder{})
	if err != nil || bad != 1 || len(ix2.byID) != 1 {
		t.Fatalf("err=%v bad=%d n=%d", err, bad, len(ix2.byID))
	}
	_ = ix2.Delete("a")
	if err := ix2.Compact(); err != nil {
		t.Fatal(err)
	}
	ix3, _, _ := Open(p, HashEmbedder{})
	if len(ix3.byID) != 0 {
		t.Fatal("delete not durable")
	}
}

type otherEmb struct{ HashEmbedder }

func (otherEmb) ID() string { return "other" }

func TestVectorsFromOtherSpaceAreNeverCompared(t *testing.T) {
	p := filepath.Join(t.TempDir(), "i.jsonl")
	ix, _, _ := Open(p, HashEmbedder{})
	_ = ix.Add(context.Background(), Entry{ID: "a", Kind: KindSemantic, Text: "gamma delta"})
	ix2, _, _ := Open(p, otherEmb{})
	h, _ := ix2.Search(context.Background(), "gamma", Options{})
	if len(h) != 1 || h[0].DenseRank != 0 {
		t.Fatalf("dense must not participate across spaces: %+v", h)
	}
	n, err := ix2.Reembed(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("reembed n=%d err=%v", n, err)
	}
}
