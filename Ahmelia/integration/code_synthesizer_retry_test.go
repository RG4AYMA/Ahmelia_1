package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/experience"
	"ahmelia/programming"
	"ahmelia/tools"
)

type fakeSynthesisClient struct {
	responses []string
	prompts   []string
	calls     int
}

func (f *fakeSynthesisClient) GenerateStructuredContext(_ context.Context, prompt string, _ json.RawMessage, _ int) (string, error) {
	f.calls++
	f.prompts = append(f.prompts, prompt)
	if f.calls > len(f.responses) {
		return f.responses[len(f.responses)-1], nil
	}
	return f.responses[f.calls-1], nil
}

const (
	synthesisWithoutTest = `{"files":[{"path":"main.go","content":"package main\n\nfunc Add(a, b int) int { return a + b }\n","role":"source"}]}`
	synthesisWithTest    = `{"files":[{"path":"main.go","content":"package main\n\nfunc Add(a, b int) int { return a + b }\n","role":"source"},{"path":"main_test.go","content":"package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal(\"bad\") } }\n","role":"test"}]}`
)

func TestCodeSynthesizerRetriesWhenGoTestIsMissing(t *testing.T) {
	client := &fakeSynthesisClient{responses: []string{synthesisWithoutTest, synthesisWithTest}}
	synthesizer := NewStructuredCodeObjectiveSynthesizerWithClient(client)
	objective := programming.CodeObjective{
		Raw: "Crea un programa Go con Add(a,b int) int y TestAdd.", Language: "Go",
		ProjectID: "retry", ProjectRoot: t.TempDir(),
	}
	plan, err := synthesizer.Synthesize(context.Background(), objective)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("llamadas = %d, quería 2", client.calls)
	}
	if len(client.prompts) != 2 || !strings.Contains(client.prompts[1], "previous response was rejected") {
		t.Fatalf("el reintento no recibió el error explícito: %q", client.prompts)
	}
	hasTest := false
	for _, change := range plan.Changes {
		if change.File.Path == "main_test.go" {
			hasTest = true
		}
	}
	if !hasTest {
		t.Fatalf("el plan no incluyó la prueba: %+v", plan.Changes)
	}
}

func TestCodeSynthesizerRecordsExperienceAndInjectsLessons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experience.jsonl")
	store, err := experience.Open(path)
	if err != nil {
		t.Fatalf("open experience: %v", err)
	}
	firstClient := &fakeSynthesisClient{responses: []string{synthesisWithoutTest, synthesisWithTest}}
	first := NewStructuredCodeObjectiveSynthesizerWithClient(firstClient)
	first.Experience = store
	if _, err := first.Synthesize(context.Background(), programming.CodeObjective{
		Raw: "Crea un programa Go con Add(a,b int) int y TestAdd.", Language: "Go",
		ProjectID: "learning", ProjectRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("primer synthesize: %v", err)
	}

	records, err := experience.Open(path)
	if err != nil {
		t.Fatalf("reopen experience: %v", err)
	}
	history, err := records.Records("programming")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	retryRecorded, successRecorded := false, false
	for _, record := range history {
		if record.Outcome == "retry" && record.Error != "" && record.Lesson != "" {
			retryRecorded = true
		}
		if record.Outcome == "success" {
			successRecorded = true
		}
	}
	if !retryRecorded || !successRecorded {
		t.Fatalf("experiencia incompleta: retry=%t success=%t history=%+v", retryRecorded, successRecorded, history)
	}

	secondClient := &fakeSynthesisClient{responses: []string{synthesisWithTest}}
	second := NewStructuredCodeObjectiveSynthesizerWithClient(secondClient)
	second.Experience = records
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module learning\n\ngo 1.20\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a - b }\n"), 0o600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if _, err := second.Synthesize(context.Background(), programming.CodeObjective{
		Raw: "Repara el proyecto existente.", Language: "Go", Existing: true,
		ProjectID: "learning", ProjectRoot: root,
		RecoveryContext: &tools.RecoveryContext{
			AttemptNumber: 2,
			FailureReason: "TestAdd failed: Add(2, 3) = -1; want 5",
			ObservedState: []string{"--- FAIL: TestAdd"},
		},
	}); err != nil {
		t.Fatalf("segundo synthesize: %v", err)
	}
	if len(secondClient.prompts) != 1 {
		t.Fatalf("prompts = %d", len(secondClient.prompts))
	}
	prompt := secondClient.prompts[0]
	if !strings.Contains(prompt, "LESSONS FROM PREVIOUS ATTEMPTS") {
		t.Fatalf("el prompt no incluyó lecciones previas: %s", prompt)
	}
	if !strings.Contains(prompt, "Read the failing test assertion first") {
		t.Fatalf("el prompt no incluyó la lección de recuperación: %s", prompt)
	}
	history, err = records.Records("programming")
	if err != nil {
		t.Fatalf("records finales: %v", err)
	}
	recoveryRecorded := false
	for _, record := range history {
		if record.Stage == "recovery" && strings.Contains(record.Error, "TestAdd failed") {
			recoveryRecorded = true
		}
	}
	if !recoveryRecorded {
		t.Fatalf("el fallo real de recuperación no quedó en memoria: %+v", history)
	}

	evidenceClient := &fakeSynthesisClient{responses: []string{synthesisWithTest}}
	evidenceSynthesizer := NewStructuredCodeObjectiveSynthesizerWithClient(evidenceClient)
	evidenceSynthesizer.Experience = records
	if _, err := evidenceSynthesizer.Synthesize(context.Background(), programming.CodeObjective{
		Raw: "Repara el proyecto existente.", Language: "Go", Existing: true,
		ProjectID: "learning", ProjectRoot: root,
		RecoveryEvidence: "--- FAIL: TestAdd\n    main_test.go:10: Add(2, 3) = -1; want 5",
	}); err != nil {
		t.Fatalf("synthesize con evidencia: %v", err)
	}
	history, err = records.Records("programming")
	if err != nil {
		t.Fatalf("records con evidencia: %v", err)
	}
	evidenceRecorded := false
	for _, record := range history {
		if record.Stage == "recovery" && strings.Contains(record.Error, "FAIL: TestAdd") && strings.Contains(record.Lesson, "failing test assertion") {
			evidenceRecorded = true
		}
	}
	if !evidenceRecorded {
		t.Fatalf("la evidencia de texto del Engine no quedó en memoria: %+v", history)
	}
}

func TestCodeSynthesizerAcceptsExistingTestDuringRepair(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module repair\n\ngo 1.20\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main_test.go"), []byte("package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal(\"bad\") } }\n"), 0o600); err != nil {
		t.Fatalf("write main_test.go: %v", err)
	}
	client := &fakeSynthesisClient{responses: []string{synthesisWithoutTest}}
	synthesizer := NewStructuredCodeObjectiveSynthesizerWithClient(client)
	plan, err := synthesizer.Synthesize(context.Background(), programming.CodeObjective{
		Raw: "Repara el proyecto existente.", Language: "Go", Existing: true,
		ProjectID: "repair", ProjectRoot: root,
		RecoveryContext: &tools.RecoveryContext{AttemptNumber: 2, FailureReason: "TestAdd failed"},
	})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("llamadas = %d, quería 1: el test existente no debe exigir reenvío", client.calls)
	}
	if len(plan.Changes) == 0 {
		t.Fatal("el plan quedó vacío")
	}
}

func TestCodeSynthesizerStopsAfterThreeRejectedAttempts(t *testing.T) {
	client := &fakeSynthesisClient{responses: []string{synthesisWithoutTest}}
	store, _ := experience.Open(filepath.Join(t.TempDir(), "experience.jsonl"))
	synthesizer := NewStructuredCodeObjectiveSynthesizerWithClient(client)
	synthesizer.Experience = store
	_, err := synthesizer.Synthesize(context.Background(), programming.CodeObjective{
		Raw: "Crea un programa Go con Add(a,b int) int y TestAdd.", Language: "Go",
		ProjectID: "bounded", ProjectRoot: t.TempDir(),
	})
	if err == nil {
		t.Fatal("esperaba rechazo tras tres intentos")
	}
	if client.calls != 3 {
		t.Fatalf("llamadas = %d, quería 3", client.calls)
	}
	history, _ := store.Records("programming")
	if len(history) != 3 {
		t.Fatalf("registros = %d, quería 3: %+v", len(history), history)
	}
	for i, record := range history {
		want := "retry"
		if i == len(history)-1 {
			want = "failure"
		}
		if record.Outcome != want || record.Lesson == "" {
			t.Fatalf("registro %d = %+v, quería outcome %q con lección", i, record, want)
		}
	}
}
