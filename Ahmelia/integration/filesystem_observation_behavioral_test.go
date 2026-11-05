package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/environment"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/programming"
)

func TestRecoveredCreateObservationWithoutExecutionResult(t *testing.T) {
	home := t.TempDir()
	storePath := filepath.Join(home, "work.json")
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}

	store := production.NewWorkStore(storePath)
	req := interaction.SemanticRequest{Objective: interaction.Objective{Text: "Create main.go"}}
	work, err := store.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachProduction(work.ID, "", "", "programming"); err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginAttempt(work.ID, "programming.create_file", "create_file", "programming")
	if err != nil {
		t.Fatal(err)
	}

	content := "package main\n\nfunc main() {}\n"
	plan := programming.MultiFilePlan{
		ID: "create-main",
		Project: programming.CodeProject{
			ID:   "recovered-create",
			Name: "recovered-create",
			Root: root,
		},
		Changes: []programming.MultiFileChange{{
			ID:        "main",
			Operation: programming.SourceCreate,
			File: programming.SourceFile{
				Path:      "main.go",
				Language:  "Go",
				Role:      programming.RoleEntrypoint,
				ProjectID: "recovered-create",
			},
			Content: content,
		}},
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAttemptEffectSpecification(work.ID, attempt, encoded); err != nil {
		t.Fatal(err)
	}

	// Runtime destruction boundary: no provider result is persisted and no
	// execution completion is recorded. Only the durable effect specification
	// survives into the reconstructed WorkStore.
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	store = nil // simulate loss of the in-memory runtime owner
	restarted := production.NewWorkStore(storePath)
	recovered, ok := restarted.Get(work.ID)
	if !ok {
		t.Fatal("work was not recovered")
	}
	if len(recovered.Attempts) != 1 || recovered.Attempts[0].Result != nil {
		t.Fatalf("experiment must have no persisted execution result: %+v", recovered.Attempts)
	}
	var recoveredPlan programming.MultiFilePlan
	if err := json.Unmarshal(recovered.Attempts[0].EffectSpecification, &recoveredPlan); err != nil {
		t.Fatal(err)
	}

	world := environment.NewManager()
	workspaceSource, err := production.NewEnvironmentWorkspaceSource(world)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := production.NewWorkspace("programming-"+recoveredPlan.Project.ID, workspaceSource)
	if err != nil {
		t.Fatal(err)
	}
	programmingWorkspace, err := programming.NewProgrammingWorkspace(workspace, recoveredPlan.Project)
	if err != nil {
		t.Fatal(err)
	}

	observed, err := programming.ObserveCreate(programmingWorkspace, recoveredPlan.Changes[0])
	if err != nil {
		t.Fatal(err)
	}
	if !programming.CreateCompatible(recoveredPlan.Changes[0], observed) {
		t.Fatalf("correct CREATE was not compatible: %+v", observed)
	}
}

func TestRecoveredCreateObservationStates(t *testing.T) {
	cases := []struct {
		name    string
		present bool
		content string
		want    bool
	}{
		{name: "absent", want: false},
		{name: "correct", present: true, content: "package main\n", want: true},
		{name: "different", present: true, content: "package main\nvar changed = true\n", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, "project")
			if err := os.MkdirAll(root, 0755); err != nil {
				t.Fatal(err)
			}
			project := programming.CodeProject{ID: "p", Name: "p", Root: root}
			world := environment.NewManager()
			source, err := production.NewEnvironmentWorkspaceSource(world)
			if err != nil {
				t.Fatal(err)
			}
			workspace, err := production.NewWorkspace("programming-p", source)
			if err != nil {
				t.Fatal(err)
			}
			pw, err := programming.NewProgrammingWorkspace(workspace, project)
			if err != nil {
				t.Fatal(err)
			}
			change := programming.MultiFileChange{
				ID:        "main",
				Operation: programming.SourceCreate,
				File:      programming.SourceFile{Path: "main.go", Language: "Go", Role: programming.RoleEntrypoint, ProjectID: "p"},
				Content:   "package main\n",
			}
			if tc.present {
				if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(tc.content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			observed, err := programming.ObserveCreate(pw, change)
			if err != nil {
				t.Fatal(err)
			}
			if got := programming.CreateCompatible(change, observed); got != tc.want {
				t.Fatalf("compatible=%v, want %v; observation=%+v", got, tc.want, observed)
			}
		})
	}

}
