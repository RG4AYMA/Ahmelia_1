package solving

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpectedFactsForFolderIsDeclaredBeforeExecution(t *testing.T) {
	target := filepath.Join(t.TempDir(), "Proyectos")
	facts, reason := ExpectedFacts("crear_carpeta", map[string]string{"ruta": target})
	if reason != "" {
		t.Fatalf("reason inesperado: %s", reason)
	}
	if len(facts) != 1 || facts[0].Subject != target || facts[0].Predicate != "filesystem.object_type" || facts[0].Expected != "directory" || facts[0].Observed != "" {
		t.Fatalf("expectativa de carpeta incompleta: %+v", facts)
	}
}

func TestExpectedFactsForMoveResolvesDestinationBeforeExecution(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "informe.pdf")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(base, "PDF")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	facts, reason := ExpectedFacts("mover_archivo", map[string]string{"origen": src, "destino": destDir})
	if reason != "" {
		t.Fatalf("reason inesperado: %s", reason)
	}
	if len(facts) != 2 {
		t.Fatalf("esperaba dos hechos, obtuve %+v", facts)
	}
	if facts[0].Subject != src || facts[0].Expected != "false" {
		t.Fatalf("hecho de origen incorrecto: %+v", facts[0])
	}
	if want := filepath.Join(destDir, filepath.Base(src)); facts[1].Subject != want || facts[1].Expected != "true" {
		t.Fatalf("resolución de destino incorrecta: quiero %s, obtuve %+v", want, facts[1])
	}
}

func TestObserveExpectedUsesDeclaredResolutionNotCurrentOne(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "informe.pdf")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(base, "PDF")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	params := map[string]string{"origen": src, "destino": destDir}
	declared, _ := ExpectedFacts("mover_archivo", params)

	// Simulate an external change: the destination directory becomes a file
	// between declaration and observation. The observation must not silently
	// reinterpret the declared destination.
	if err := os.RemoveAll(destDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destDir, []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := ObserveExpected("mover_archivo", params, declared)
	if res.Outcome != VerificationUnchecked {
		t.Fatalf("un cambio de resolución debe quedar insuficiente, obtuve %q (%s)", res.Outcome, res.Detail)
	}
}

func TestExpectedFactsWithoutOracleReturnsNoExpectation(t *testing.T) {
	if facts, reason := ExpectedFacts("abrir_programa", map[string]string{"raw": "blender"}); len(facts) != 0 || reason == "" {
		t.Fatalf("una acción sin oráculo no puede producir expectativa: facts=%+v reason=%q", facts, reason)
	}
}
