package programming

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func baseAcceptanceFiles() map[string]string {
	return map[string]string{
		"go.mod": "module sumador\n\ngo 1.20\n",
		"main.go": `package main

func Add(a, b int) int {
	return a + b
}

func main() {}
`,
		"main_test.go": `package main

import "testing"

func TestOwn(t *testing.T) {
	if Add(0, 0) != 0 {
		t.Fatal("zero")
	}
}
`,
	}
}

func writeAcceptanceFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func acceptanceCheck(result AcceptanceResult, name string) (AcceptanceCheck, bool) {
	for _, check := range result.Checks {
		if check.Name == name {
			return check, true
		}
	}
	return AcceptanceCheck{}, false
}

func TestGoAcceptancePassesCorrectProject(t *testing.T) {
	root := writeAcceptanceFiles(t, baseAcceptanceFiles())
	result, err := NewGoSumadorAcceptance().Verify(context.Background(), root)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !result.Success {
		t.Fatalf("acceptance failed: %+v", result.Checks)
	}
	if check, ok := acceptanceCheck(result, "acceptance_test"); !ok || !check.Success {
		t.Fatalf("acceptance test check: %+v", result.Checks)
	}
	if _, err := os.Stat(filepath.Join(root, "zz_ahmelia_acceptance_test.go")); !os.IsNotExist(err) {
		t.Fatal("original project was mutated with the acceptance test")
	}
}

func TestGoAcceptanceCatchesWrongBehaviorEvenWhenOwnTestPasses(t *testing.T) {
	files := baseAcceptanceFiles()
	files["main.go"] = strings.Replace(files["main.go"], "return a + b", "return a - b", 1)
	root := writeAcceptanceFiles(t, files)

	result, err := NewGoSumadorAcceptance().Verify(context.Background(), root)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Success {
		t.Fatalf("acceptance accepted a subtraction: %+v", result.Checks)
	}
	if check, ok := acceptanceCheck(result, "project_tests"); !ok || !check.Success {
		t.Fatalf("the model's own weak test should pass: %+v", result.Checks)
	}
	if check, ok := acceptanceCheck(result, "acceptance_test"); !ok || check.Success {
		t.Fatalf("independent acceptance should fail: %+v", result.Checks)
	}
}

func TestGoAcceptanceRejectsTestFunctionInSource(t *testing.T) {
	files := baseAcceptanceFiles()
	files["leak.go"] = `package main

import "testing"

func TestLeak(t *testing.T) {}
`
	root := writeAcceptanceFiles(t, files)

	result, err := NewGoSumadorAcceptance().Verify(context.Background(), root)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Success {
		t.Fatalf("acceptance accepted a Test function in a source file: %+v", result.Checks)
	}
	if check, ok := acceptanceCheck(result, "source_test_separation"); !ok || check.Success {
		t.Fatalf("source separation check: %+v", result.Checks)
	}
}

func TestGoAcceptanceRejectsExternalModules(t *testing.T) {
	files := baseAcceptanceFiles()
	files["go.mod"] = "module sumador\n\ngo 1.20\n\nrequire example.com/nope v1.0.0\n"
	root := writeAcceptanceFiles(t, files)

	result, err := NewGoSumadorAcceptance().Verify(context.Background(), root)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Success {
		t.Fatalf("acceptance accepted external modules: %+v", result.Checks)
	}
	if check, ok := acceptanceCheck(result, "external_dependencies"); !ok || check.Success {
		t.Fatalf("external dependency check: %+v", result.Checks)
	}
}
