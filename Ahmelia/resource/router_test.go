package resource

import (
	"testing"
)

func TestRouter_Pick_WithoutProfile(t *testing.T) {
	cfg := DefaultRouterConfig("gemma3:4b")
	r := NewRouter(cfg, nil)

	// Interpretation
	res := r.Pick(TaskClassInterpretation)
	if res.Source != "default" {
		t.Fatalf("Interpretation source = %s, want default", res.Source)
	}
	if res.Model != "gemma3:4b" {
		t.Fatalf("Interpretation model = %s, want gemma3:4b", res.Model)
	}
	if res.Level != ModelLevelMedium {
		t.Fatalf("Interpretation level = %s, want medium", res.Level)
	}

	// Generation
	res = r.Pick(TaskClassGeneration)
	if res.Source != "default" {
		t.Fatalf("Generation source = %s, want default", res.Source)
	}

	// Unknown class
	res = r.Pick(TaskClass("unknown"))
	if res.Source != "default" {
		t.Fatalf("Unknown class source = %s, want default", res.Source)
	}
}

func TestRouter_Pick_WithProfile_Measured(t *testing.T) {
	cfg := DefaultRouterConfig("gemma3:4b")

	// Perfil simulado con modelo pequeño en GPU cumpliendo presupuesto interpretación
	profile := &HardwareProfile{
		VRAMBudgetMB: 1800,
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "gemma3:1b", GPULayers: 20, NumThread: 4}.Normalize(),
				[]Measurement{
					{Repetition: 1, FirstResponseMs: 5000, TokensPerSecond: 10},
					{Repetition: 2, FirstResponseMs: 800, TokensPerSecond: 25},
					{Repetition: 3, FirstResponseMs: 750, TokensPerSecond: 26},
				},
				0,
			),
			BuildConfigResult(
				ModelConfig{Model: "gemma3:4b", GPULayers: 18, NumThread: 4}.Normalize(),
				[]Measurement{
					{Repetition: 1, FirstResponseMs: 8000, TokensPerSecond: 8},
					{Repetition: 2, FirstResponseMs: 3000, TokensPerSecond: 12},
					{Repetition: 3, FirstResponseMs: 2800, TokensPerSecond: 13},
				},
				0,
			),
		},
	}

	r := NewRouter(cfg, profile)

	// Interpretation (presupuesto 2000ms) -> gemma3:1b (p95 ~800ms) cumple
	res := r.Pick(TaskClassInterpretation)
	if res.Source != "measured" {
		t.Fatalf("Interpretation source = %s, want measured", res.Source)
	}
	if res.Model != "gemma3:1b" {
		t.Fatalf("Interpretation model = %s, want gemma3:1b", res.Model)
	}
	if res.Level != ModelLevelSmall {
		t.Fatalf("Interpretation level = %s, want small", res.Level)
	}
	if !res.Matched {
		t.Fatal("Interpretation should be matched (p95 800 <= 2000)")
	}

	// Generation (presupuesto 15000ms) -> gemma3:4b (p95 ~3000ms) cumple
	res = r.Pick(TaskClassGeneration)
	if res.Source != "measured" {
		t.Fatalf("Generation source = %s, want measured", res.Source)
	}
	if res.Model != "gemma3:4b" {
		t.Fatalf("Generation model = %s, want gemma3:4b", res.Model)
	}
	if res.Level != ModelLevelMedium {
		t.Fatalf("Generation level = %s, want medium", res.Level)
	}
	if !res.Matched {
		t.Fatal("Generation should be matched (p95 3000 <= 15000)")
	}
}

func TestRouter_Pick_NoModelMeetsBudget(t *testing.T) {
	cfg := DefaultRouterConfig("gemma3:4b")

	// Perfil donde NINGÚN modelo cumple presupuesto ajustado
	profile := &HardwareProfile{
		VRAMBudgetMB: 1800,
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "gemma3:1b", GPULayers: 20}.Normalize(),
				[]Measurement{
					{Repetition: 1, FirstResponseMs: 5000},
					{Repetition: 2, FirstResponseMs: 5000}, // p95 = 5000 > 2000
				},
				0,
			),
		},
	}

	r := NewRouter(cfg, profile)

	// Interpretation con presupuesto 2000ms, pero p95 = 5000
	res := r.Pick(TaskClassInterpretation)
	if res.Matched {
		t.Fatal("Should not be matched (p95 > budget)")
	}
	if res.Source != "measured" {
		t.Fatalf("Source = %s, want measured (fastest available)", res.Source)
	}
	// Debe devolver el más rápido disponible (gemma3:1b)
	if res.Model != "gemma3:1b" {
		t.Fatalf("Model = %s, want gemma3:1b (fastest)", res.Model)
	}
}

func TestRouter_Pick_UnknownClass(t *testing.T) {
	cfg := DefaultRouterConfig("gemma3:4b")
	r := NewRouter(cfg, nil)

	res := r.Pick(TaskClass("nonexistent"))
	if res.Source != "default" {
		t.Fatalf("Source = %s, want default", res.Source)
	}
}

func TestRouter_LevelsAvailable(t *testing.T) {
	profile := &HardwareProfile{
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "gemma3:1b", GPULayers: 20}.Normalize(),
				[]Measurement{{Repetition: 2, FirstResponseMs: 800}},
				0,
			),
		},
	}
	cfg := DefaultRouterConfig("gemma3:4b")
	r := NewRouter(cfg, profile)

	available := r.LevelsAvailable()
	if !available[ModelLevelSmall] {
		t.Fatal("Small should be available")
	}
	if available[ModelLevelMedium] {
		t.Fatal("Medium should not be available (not in profile)")
	}
	if available[ModelLevelLarge] {
		t.Fatal("Large should not be available")
	}
}

func TestRouter_GetStatsForModel(t *testing.T) {
	profile := &HardwareProfile{
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "gemma3:4b", GPULayers: 18}.Normalize(),
				[]Measurement{
					{Repetition: 2, FirstResponseMs: 3000},
					{Repetition: 3, FirstResponseMs: 2800},
				},
				0,
			),
		},
	}
	cfg := DefaultRouterConfig("gemma3:4b")
	r := NewRouter(cfg, profile)

	stats := r.GetStatsForModel("gemma3:4b")
	if stats == nil {
		t.Fatal("stats should not be nil")
	}
	if stats.ValidRepetitions != 2 {
		t.Fatalf("ValidRepetitions = %d, want 2", stats.ValidRepetitions)
	}

	stats = r.GetStatsForModel("nonexistent")
	if stats != nil {
		t.Fatal("stats for nonexistent should be nil")
	}
}

func TestRouter_Validate(t *testing.T) {
	// Config válida
	cfg := DefaultRouterConfig("gemma3:4b")
	r := NewRouter(cfg, nil)
	if err := r.Validate(); err != nil {
		t.Fatalf("valid config failed: %v", err)
	}

	// Sin modelo por defecto
	cfg2 := DefaultRouterConfig("")
	r2 := NewRouter(cfg2, nil)
	if err := r2.Validate(); err == nil {
		t.Fatal("empty default model should fail validation")
	}

	// Presupuesto <= 0
	cfg3 := DefaultRouterConfig("gemma3:4b")
	cfg3.PerTaskClass[TaskClassInterpretation] = TaskClassConfig{
		MaxFirstResponseMs: 0,
		Levels:             []ModelLevel{ModelLevelSmall},
	}
	r3 := NewRouter(cfg3, nil)
	if err := r3.Validate(); err == nil {
		t.Fatal("zero budget should fail validation")
	}

	// Sin niveles
	cfg4 := DefaultRouterConfig("gemma3:4b")
	cfg4.PerTaskClass[TaskClassInterpretation] = TaskClassConfig{
		MaxFirstResponseMs: 2000,
		Levels:             []ModelLevel{},
	}
	r4 := NewRouter(cfg4, nil)
	if err := r4.Validate(); err == nil {
		t.Fatal("empty levels should fail validation")
	}
}

func TestRouter_SortLevelsByQuality(t *testing.T) {
	levels := []ModelLevel{ModelLevelSmall, ModelLevelLarge, ModelLevelMedium}
	SortLevelsByQuality(levels)
	expected := []ModelLevel{ModelLevelLarge, ModelLevelMedium, ModelLevelSmall}
	for i, l := range levels {
		if l != expected[i] {
			t.Fatalf("levels[%d] = %s, want %s", i, l, expected[i])
		}
	}
}

func TestRouter_BuildLevelMap_WithMultipleConfigs(t *testing.T) {
	profile := &HardwareProfile{
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "gemma3:1b", GPULayers: 20, NumThread: 4}.Normalize(),
				[]Measurement{{Repetition: 2, FirstResponseMs: 800}},
				0,
			),
			BuildConfigResult(
				ModelConfig{Model: "gemma3:4b", GPULayers: 18, NumThread: 4}.Normalize(),
				[]Measurement{{Repetition: 2, FirstResponseMs: 3000}},
				0,
			),
			BuildConfigResult(
				ModelConfig{Model: "gemma3:1b", GPULayers: 0, NumThread: 4}.Normalize(),
				[]Measurement{{Repetition: 2, FirstResponseMs: 5000}},
				0,
			),
		},
	}
	cfg := DefaultRouterConfig("gemma3:4b")
	r := NewRouter(cfg, profile)

	// Debe haber small (1b GPU) y medium (4b GPU)
	if r.levelMap[ModelLevelSmall] != "gemma3:1b" {
		t.Fatalf("Small = %s, want gemma3:1b", r.levelMap[ModelLevelSmall])
	}
	if r.levelMap[ModelLevelMedium] != "gemma3:4b" {
		t.Fatalf("Medium = %s, want gemma3:4b", r.levelMap[ModelLevelMedium])
	}
}

func TestRouter_FindFastestModel(t *testing.T) {
	profile := &HardwareProfile{
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "slow-model", GPULayers: 0}.Normalize(),
				[]Measurement{{Repetition: 2, FirstResponseMs: 5000}},
				0,
			),
			BuildConfigResult(
				ModelConfig{Model: "fast-model", GPULayers: 18}.Normalize(),
				[]Measurement{{Repetition: 2, FirstResponseMs: 1000}},
				0,
			),
		},
	}
	cfg := DefaultRouterConfig("gemma3:4b")
	r := NewRouter(cfg, profile)

	fastest := r.findFastestModel([]ModelLevel{ModelLevelSmall, ModelLevelMedium})
	if fastest != "fast-model" {
		t.Fatalf("fastest = %s, want fast-model", fastest)
	}
}

func TestRouter_Pick_BudgetRespected(t *testing.T) {
	cfg := DefaultRouterConfig("gemma3:4b")

	profile := &HardwareProfile{
		VRAMBudgetMB: 1800,
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "gemma3:1b", GPULayers: 20}.Normalize(),
				[]Measurement{
					{Repetition: 2, FirstResponseMs: 1000},
					{Repetition: 3, FirstResponseMs: 1200},
				},
				0,
			),
			BuildConfigResult(
				ModelConfig{Model: "gemma3:4b", GPULayers: 18}.Normalize(),
				[]Measurement{
					{Repetition: 2, FirstResponseMs: 8000},
					{Repetition: 3, FirstResponseMs: 8500},
				},
				0,
			),
		},
	}

	r := NewRouter(cfg, profile)

	// Interpretation (2000ms) -> 1b (p95 ~1200ms)
	res := r.Pick(TaskClassInterpretation)
	if res.Matched && res.Model == "gemma3:1b" {
		// OK
	} else {
		t.Fatalf("Interpretation: matched=%v model=%s", res.Matched, res.Model)
	}

	// Generation (15000ms) -> 4b (p95 ~8500ms)
	res = r.Pick(TaskClassGeneration)
	if res.Matched && res.Model == "gemma3:4b" {
		// OK
	} else {
		t.Fatalf("Generation: matched=%v model=%s", res.Matched, res.Model)
	}
}

func TestRouter_Pick_ProposalClass(t *testing.T) {
	cfg := DefaultRouterConfig("gemma3:4b")

	profile := &HardwareProfile{
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "gemma3:4b", GPULayers: 18}.Normalize(),
				[]Measurement{
					{Repetition: 2, FirstResponseMs: 3000},
					{Repetition: 3, FirstResponseMs: 3500},
				},
				0,
			),
		},
	}

	r := NewRouter(cfg, profile)

	res := r.Pick(TaskClassProposal)
	if res.Source != "measured" {
		t.Fatalf("Proposal source = %s, want measured", res.Source)
	}
	if res.Model != "gemma3:4b" {
		t.Fatalf("Proposal model = %s, want gemma3:4b", res.Model)
	}
	if res.Budget != 30000 {
		t.Fatalf("Proposal budget = %d, want 30000", res.Budget)
	}
}

func TestRouter_DefaultRouterConfig(t *testing.T) {
	cfg := DefaultRouterConfig("test-model")
	if cfg.DefaultModel != "test-model" {
		t.Fatalf("DefaultModel = %s, want test-model", cfg.DefaultModel)
	}
	if cfg.DefaultLevel != ModelLevelMedium {
		t.Fatalf("DefaultLevel = %s, want medium", cfg.DefaultLevel)
	}
	if _, ok := cfg.PerTaskClass[TaskClassInterpretation]; !ok {
		t.Fatal("Interpretation class missing")
	}
	if _, ok := cfg.PerTaskClass[TaskClassGeneration]; !ok {
		t.Fatal("Generation class missing")
	}
	if _, ok := cfg.PerTaskClass[TaskClassProposal]; !ok {
		t.Fatal("Proposal class missing")
	}
}

func TestRouter_CustomConfig(t *testing.T) {
	cfg := RouterConfig{
		DefaultModel: "custom-model",
		DefaultLevel: ModelLevelLarge,
		PerTaskClass: map[TaskClass]TaskClassConfig{
			TaskClassInterpretation: {
				MaxFirstResponseMs: 1000,
				Levels:             []ModelLevel{ModelLevelSmall},
			},
		},
	}
	r := NewRouter(cfg, nil)
	if err := r.Validate(); err != nil {
		t.Fatalf("valid config failed: %v", err)
	}
	res := r.Pick(TaskClassInterpretation)
	if res.Model != "custom-model" {
		t.Fatalf("model = %s, want custom-model", res.Model)
	}
	if res.Budget != 1000 {
		t.Fatalf("budget = %d, want 1000", res.Budget)
	}
}
