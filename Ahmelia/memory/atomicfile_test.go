package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicKeepsBackupAndNoPartialFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	if err := WriteFileAtomic(p, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	bk, _ := os.ReadFile(p + ".bak")
	if string(b) != "v2" || string(bk) != "v1" {
		t.Fatalf("%s %s", b, bk)
	}
	if _, err := os.Stat(p + ".tmp"); err == nil {
		t.Fatal("tmp left behind")
	}
}

func TestQuarantineCorruptPreservesBytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "long.json")
	_ = os.WriteFile(p, []byte("{corrupto"), 0o644)
	dst, err := QuarantineCorrupt(p)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "{corrupto" {
		t.Fatal("bytes lost")
	}
}
