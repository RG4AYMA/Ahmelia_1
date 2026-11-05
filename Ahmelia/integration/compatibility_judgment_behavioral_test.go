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

func TestRecoveredCreateCompatibilityJudgmentAfterRuntimeReconstruction(t *testing.T) {
	cases := []struct {
		name      string
		writeFile bool
		fileData  string
		want      bool
	}{
		{name: "absent", want: false},
		{name: "different_sha", writeFile: true, fileData: "package main\nvar changed = true\n", want: false},
		{name: "expected_sha", writeFile: true, fileData: "package main\n", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

			content := "package main\n"
			plan := programming.MultiFilePlan{
				ID: "create-main",
				Project: programming.CodeProject{
					ID:   "compatibility-" + tc.name,
					Name: "compatibility-" + tc.name,
					Root: root,
				},
				Changes: []programming.MultiFileChange{{
					ID:        "main",
					Operation: programming.SourceCreate,
					File: programming.SourceFile{
						Path:      "main.go",
						Language:  "Go",
						Role:      programming.RoleEntrypoint,
						ProjectID: "compatibility-" + tc.name,
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

			if tc.writeFile {
				if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(tc.fileData), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Destroy the runtime owner. The next phase has only the persisted
			// expected effect and the current filesystem to work with.
			store = nil
			restarted := production.NewWorkStore(storePath)
			recovered, ok := restarted.Get(work.ID)
			if !ok {
				t.Fatal("work was not recovered")
			}
			if len(recovered.Attempts) != 1 || recovered.Attempts[0].Result != nil {
				t.Fatalf("compatibility judgment must not require a persisted execution result: %+v", recovered.Attempts)
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
			judgment := programming.EvaluateCreateCompatibility(recoveredPlan.Changes[0], observed)
			if judgment.Compatible != tc.want {
				t.Fatalf("compatible=%v, want %v; observed=%+v judgment=%+v", judgment.Compatible, tc.want, observed, judgment)
			}
		})
	}
}
