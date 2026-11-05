package integration

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"ahmelia/capability"
	"ahmelia/environment"
	"ahmelia/interaction"
	"ahmelia/production"
)

// stubbornGenerator replays the exact defective synthesis observed in the
// Windows log (attempt 1) for EVERY call, and records every prompt received.
type stubbornGenerator struct {
	mu      sync.Mutex
	prompts []string
}

const f10Source = "package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n"
const f10Test = "package main\n\nimport (\n    \"testing\"\n\n    \"f10_repair\"\n)\n\nfunc TestMain(m *testing.M) {\n    m.Run()\n}\n\nfunc TestMainFunc(t *testing.T) {\n    mainFunc := f10_repair.Main\n    mainFunc()\n}\n"

func (g *stubbornGenerator) GenerateStructuredContext(_ context.Context, prompt string, _ json.RawMessage, _ int) (string, error) {
	g.mu.Lock()
	g.prompts = append(g.prompts, prompt)
	g.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"files": []map[string]string{
		{"path": "f10_repair.go", "content": f10Source, "role": "source"},
		{"path": "f10_repair_test.go", "content": f10Test, "role": "test"},
	}})
	return string(b), nil
}

func newF10Runtime(t *testing.T, gen StructuredCodeGenerator) (*ProductionRuntime, interaction.SemanticRequest, capability.Route) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AHMELIA_WORKSPACE_ROOT", filepath.Join(home, "workspace"))
	provider, err := NewProgrammingProviderWithSynthesizer(environment.NewManager(), NewOllamaCodeObjectiveSynthesizerWithClient(gen))
	if err != nil {
		t.Fatal(err)
	}
	reg := capability.NewRegistry()
	cap := capability.Capability{ID: "programming.create", Domain: capability.DomainLocal, Action: "create", Description: "crear programa", Permission: capability.PermissionAutomatic}
	if err := reg.Register(cap); err != nil {
		t.Fatal(err)
	}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, cap.ID); err != nil {
		t.Fatal(err)
	}
	rt, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, production.NewWorkStore(filepath.Join(t.TempDir(), "work.json")), production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	text := "Quiero crear un programa Go llamado f10_repair"
	req := interaction.SemanticRequest{
		Raw: text, Normalized: "crear programa go f10_repair", Kind: interaction.KindProduction,
		Objective: interaction.Objective{Text: text}, Language: "es", Confidence: 0.99,
		Action: interaction.ActionIntent{Name: "create", Confidence: 0.99}, Topic: "programa Go", RequestedAction: "create",
		CommunicativeActs: interaction.CommunicativeActs{Primary: interaction.ActCommand, Confidence: 0.99, Evidence: []interaction.CommunicativeActEvidence{{Act: interaction.ActCommand, Confidence: 0.99, Source: "test"}}},
	}
	route := capability.NewResolver(reg).Resolve(interaction.ResolveCapabilityInteraction(req), req)
	return rt, req, route
}
