package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLongMemoryQuarantinesCorruptFileAndRecoversBackup(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer os.Chdir(old)
	_ = os.MkdirAll("memory", 0o755)

	m := LoadLongMemory()
	m.Data["k1"] = &Knowledge{ID: "k1", Input: "hola"}
	if err := m.SaveErr(); err != nil {
		t.Fatal(err)
	}
	m.Data["k2"] = &Knowledge{ID: "k2", Input: "adios"}
	_ = m.SaveErr() // ahora existe .bak con k1
	_ = os.WriteFile(LongMemoryPath, []byte("{corrupto"), 0o644)

	r := LoadLongMemory()
	if _, ok := r.Data["k1"]; !ok {
		t.Fatalf("backup not recovered: %+v", r.Data)
	}
	matches, _ := filepath.Glob(LongMemoryPath + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatal("corrupt file not quarantined")
	}
}
