package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/environment"
	"ahmelia/experience"
	"ahmelia/interaction"
	"ahmelia/linguistics"
	"ahmelia/production"
	"ahmelia/programming"
	"ahmelia/tools"
)

func findProjectFile(root, name string) string {
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && (info.Name() == name || (name == "*_test.go" && strings.HasSuffix(info.Name(), "_test.go"))) {
			found = path
		}
		return nil
	})
	return found
}

func findSourceFile(root string) string {
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			found = path
		}
		return nil
	})
	return found
}

func runGoTest(root string) (string, error) {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runGoAcceptance(t *testing.T, contract programming.GoAcceptanceContract, root, stage string) programming.AcceptanceResult {
	t.Helper()
	result, err := contract.Verify(context.Background(), root)
	if err != nil {
		t.Fatalf("aceptación %s: %v", stage, err)
	}
	for _, check := range result.Checks {
		t.Logf("aceptación %s: %s success=%t detail=%s", stage, check.Name, check.Success, check.Detail)
	}
	return result
}

type loggingSynthesisClient struct {
	inner *linguistics.OllamaClient
	t     *testing.T
	calls int
}

func (c *loggingSynthesisClient) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	c.calls++
	raw, err := c.inner.GenerateStructuredContext(ctx, prompt, schema, numPredict)
	visible := raw
	if len(visible) > 1200 {
		visible = visible[:1200] + "…"
	}
	c.t.Logf("synthesis call %d prompt_bytes=%d err=%v raw=%s", c.calls, len(prompt), err, visible)
	return raw, err
}

func (c *loggingSynthesisClient) ModelName() string { return c.inner.ModelName() }

func (c *loggingSynthesisClient) GenerationSource() string { return c.inner.GenerationSource() }

// TestLiveProgrammingCreateAndRepair exercises the real Ollama synthesis
// boundary through the programming provider: create a small Go project, damage
// its behaviour, ask for a repair, and verify with go test. It is opt-in
// because it invokes the local model.
func TestLiveProgrammingCreateAndRepair(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_CODE") != "1" {
		t.Skip("definir AHMELIA_LIVE_CODE=1 para ejecutar contra Ollama")
	}
	temp := t.TempDir()
	root := filepath.Join(temp, "sumador")
	experiencePath := filepath.Join(temp, "programming_experience.jsonl")
	inner := linguistics.NewOllamaClient()
	client := &loggingSynthesisClient{inner: inner, t: t}
	experienceStore, _ := experience.Open(experiencePath)
	t.Logf("modelo de código: %s; experiencia=%s", inner.ModelName(), experiencePath)
	acceptance := programming.NewGoSumadorAcceptance()
	synthesizer := NewStructuredCodeObjectiveSynthesizerWithClient(client)
	synthesizer.Experience = experienceStore
	world := environment.NewManager()
	provider, err := NewProgrammingProviderWithSynthesizer(world, synthesizer)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	req := production.ProviderRequest{
		Input:   "Crea un programa Go llamado sumador con una función Add(a, b int) int y una prueba TestAdd que verifique Add(2, 3) == 5. Sin dependencias externas. Incluye go.mod.",
		Project: production.ProductionProject{ID: "sumador", Name: "sumador", Path: root},
		ObjectiveFrame: interaction.ObjectiveFrame{
			PrimaryGoal: "crear un programa sumador", PrimaryOperation: "create", Subject: "programa", Workspace: "local",
			Constraints: []string{"sin dependencias externas"}, RequiredActions: []string{"sumar", "probar"},
			Verification: []string{"behavioral_result"}, Recovery: []string{"diagnosticar"}, Confidence: 0.9, Provenance: "semantic_request",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	start := time.Now()
	created := provider.Execute(ctx, req)
	t.Logf("crear: %s success=%t verified=%t error=%v source=%s", time.Since(start), created.Success, created.Verified, created.Err, created.GenerationSource)
	if created.Err != nil {
		t.Fatalf("crear: %v", created.Err)
	}
	if !strings.HasPrefix(created.GenerationSource, "ollama:") {
		t.Fatalf("fuente de generación = %q, quería ollama:<modelo>", created.GenerationSource)
	}
	mainPath := findSourceFile(root)
	testPath := findProjectFile(root, "*_test.go")
	if mainPath == "" || testPath == "" {
		t.Fatalf("el proyecto no tiene fuente y prueba: root=%s", root)
	}
	out, testErr := runGoTest(root)
	t.Logf("go test inicial: err=%v\n%s", testErr, out)
	if testErr != nil {
		t.Fatalf("el código creado no pasa sus pruebas: %v\n%s", testErr, out)
	}
	initialAcceptance := runGoAcceptance(t, acceptance, root, "creación")
	if !initialAcceptance.Success {
		t.Fatalf("la aceptación independiente falló tras la creación: %+v", initialAcceptance.Checks)
	}

	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	damaged := strings.Replace(string(content), "a + b", "a - b", 1)
	if damaged == string(content) {
		t.Fatalf("no pude dañar la suma; contenido:\n%s", content)
	}
	if err := os.WriteFile(mainPath, []byte(damaged), 0o600); err != nil {
		t.Fatalf("write damaged: %v", err)
	}
	brokenOut, brokenErr := runGoTest(root)
	t.Logf("go test dañado: err=%v\n%s", brokenErr, brokenOut)
	if brokenErr == nil {
		t.Fatal("el daño no hizo fallar las pruebas")
	}
	brokenAcceptance := runGoAcceptance(t, acceptance, root, "daño")
	if brokenAcceptance.Success {
		t.Fatal("la aceptación independiente no detectó el daño")
	}

	recovery := &tools.RecoveryContext{
		AttemptNumber: 2,
		Objective:     "reparar sumador",
		ExpectedState: []string{"go test ./... pasa"},
		ObservedState: []string{brokenOut},
		FailureReason: "TestAdd espera Add(2,3)==5",
		Evidence: []tools.RecoveryEvidence{{
			Kind: "test_failure", Source: "go test", Stage: "test",
			Summary: "La implementación de Add devuelve una resta",
			Payload: json.RawMessage(fmt.Sprintf(`{"file":%q,"output":%q}`, filepath.ToSlash(mainPath), brokenOut)),
		}},
	}
	repairReq := req
	repairReq.AttemptNumber = 2
	repairReq.RecoveryContext = recovery
	repairReq.RecoveryEvidence = brokenOut
	repairReq.Input = "El proyecto existente falla: TestAdd espera Add(2,3)==5 y " + filepath.Base(mainPath) + " tiene la operación equivocada. Repara " + filepath.Base(mainPath) + " sin crear un proyecto nuevo."
	t.Logf("lecciones antes de reparar: %v", experienceStore.Lessons("programming", 5))
	start = time.Now()
	repaired := provider.Execute(ctx, repairReq)
	t.Logf("reparar: %s success=%t verified=%t error=%v source=%s", time.Since(start), repaired.Success, repaired.Verified, repaired.Err, repaired.GenerationSource)
	if repaired.Err != nil {
		t.Fatalf("reparar: %v", repaired.Err)
	}
	finalOut, finalErr := runGoTest(root)
	t.Logf("go test reparado: err=%v\n%s", finalErr, finalOut)
	if finalErr != nil {
		t.Fatalf("la reparación no pasó las pruebas: %v\n%s", finalErr, finalOut)
	}
	finalAcceptance := runGoAcceptance(t, acceptance, root, "reparación")
	if !finalAcceptance.Success {
		t.Fatalf("la aceptación independiente falló tras la reparación: %+v", finalAcceptance.Checks)
	}

	records, err := experienceStore.Records("programming")
	if err != nil {
		t.Fatalf("leer experiencia: %v", err)
	}
	retryRecorded, successRecorded := false, false
	for _, record := range records {
		t.Logf("experiencia: attempt=%d stage=%s outcome=%s lesson=%q error=%q", record.Attempt, record.Stage, record.Outcome, record.Lesson, record.Error)
		if record.Outcome == "retry" && record.Lesson != "" {
			retryRecorded = true
		}
		if record.Outcome == "success" {
			successRecorded = true
		}
	}
	if !retryRecorded || !successRecorded {
		t.Fatalf("memoria de experiencia incompleta: retry=%t success=%t records=%d", retryRecorded, successRecorded, len(records))
	}
}
