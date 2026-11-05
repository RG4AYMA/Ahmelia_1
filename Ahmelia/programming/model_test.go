package programming

import (
	"testing"

	"ahmelia/planning"
	"ahmelia/production"
)

func TestProgrammingModel_ReusesProductionTask(t *testing.T) {
	goal := planning.NewGoal("program", "build software", planning.PriorityHigh, "user")
	p, err := production.NewProduction("prod-1", goal)
	if err != nil {
		t.Fatal(err)
	}
	task := planning.NewTask(goal.ID, "create main", "create entrypoint", 1, 0)
	if err := p.SetCurrentTask(task); err != nil {
		t.Fatal(err)
	}

	project := CodeProject{ID: "proj-1", Name: "demo", Root: "/tmp/demo", Language: "go"}
	binding, err := NewProductionBinding(p, project)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Task != task {
		t.Fatal("Programming debe reutilizar la misma Task")
	}
	if binding.Production != p {
		t.Fatal("Programming debe reutilizar la misma Production")
	}
}

func TestProgrammingModel_DomainContractsValidate(t *testing.T) {
	cases := []interface{ Validate() error }{
		(CodeProject{ID: "p", Root: "/tmp/p"}),
		(SourceFile{Path: "main.go", Language: "go", ProjectID: "p"}),
		(CodeArtifact{ID: "a", Type: ArtifactSource, ProjectID: "p"}),
		(Build{ID: "b", ProjectID: "p", Command: "go build ./..."}),
		(TestRun{ID: "t", ProjectID: "p", Command: "go test ./..."}),
		(Runtime{ID: "r", ProjectID: "p", Command: "./app", Status: RuntimeStopped}),
		(Dependency{Name: "x", Manager: "go", ProjectID: "p"}),
		(Diagnostic{ID: "d", ProjectID: "p", Message: "undefined: x"}),
		(Patch{ID: "patch", ProjectID: "p", File: "main.go"}),
		(Refactor{ID: "ref", ProjectID: "p", Kind: RefactorRename}),
	}
	for _, c := range cases {
		if err := c.Validate(); err != nil {
			t.Fatalf("contrato inválido: %v", err)
		}
	}
}

func TestProgrammingModel_InvalidContractsFailClosed(t *testing.T) {
	if err := (CodeProject{}).Validate(); err == nil {
		t.Fatal("project inválido aceptado")
	}
	if err := (SourceFile{Path: "main.go"}).Validate(); err == nil {
		t.Fatal("source file incompleto aceptado")
	}
	if err := (Build{ID: "b", ProjectID: "p"}).Validate(); err == nil {
		t.Fatal("build sin command aceptado")
	}
	if err := (Diagnostic{ID: "d", ProjectID: "p"}).Validate(); err == nil {
		t.Fatal("diagnostic sin message aceptado")
	}
}

func TestProgrammingCapabilities_AreVocabularyOnly(t *testing.T) {
	if CapabilityReadFile == CapabilityWriteFile {
		t.Fatal("capabilities distintas colisionaron")
	}
	if CapabilityBuild == CapabilityRunTests {
		t.Fatal("capabilities distintas colisionaron")
	}
}
