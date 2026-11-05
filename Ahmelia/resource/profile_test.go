package resource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ahmelia/memory"
)

func TestModelConfig_Normalize(t *testing.T) {
	cases := []struct {
		name     string
		input    ModelConfig
		expected ModelConfig
	}{
		{
			name:  "defaults",
			input: ModelConfig{Model: "gemma3:4b"},
			expected: ModelConfig{
				Model:       "gemma3:4b",
				GPULayers:   0,
				NumThread:   0,
				Temperature: 0,
			},
		},
		{
			name: "negative values clamped",
			input: ModelConfig{
				Model:       "test",
				GPULayers:   -1,
				NumThread:   -2,
				Temperature: -0.5,
			},
			expected: ModelConfig{
				Model:       "test",
				GPULayers:   0,
				NumThread:   0,
				Temperature: 0,
			},
		},
		{
			name: "positive values preserved",
			input: ModelConfig{
				Model:       "gemma3:4b",
				GPULayers:   20,
				NumThread:   4,
				Temperature: 0.7,
			},
			expected: ModelConfig{
				Model:       "gemma3:4b",
				GPULayers:   20,
				NumThread:   4,
				Temperature: 0.7,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input.Normalize()
			if got != tc.expected {
				t.Fatalf("Normalize() = %+v, want %+v", got, tc.expected)
			}
		})
	}
}

func TestModelConfig_Key(t *testing.T) {
	c := ModelConfig{Model: "gemma3:4b", GPULayers: 10, NumThread: 4}
	got := c.Key()
	want := "gemma3:4b|gl=10|nt=4"
	if got != want {
		t.Fatalf("Key() = %q, want %q", got, want)
	}
}

func TestPercentile(t *testing.T) {
	cases := []struct {
		name     string
		input    []float64
		p        float64
		expected float64
	}{
		{"empty", []float64{}, 50, 0},
		{"single", []float64{10}, 50, 10},
		{"two elements p50", []float64{10, 20}, 50, 15},
		{"two elements p95", []float64{10, 20}, 95, 19.5},
		{"p0", []float64{10, 20, 30}, 0, 10},
		{"p100", []float64{10, 20, 30}, 100, 30},
		{"p50 odd", []float64{10, 20, 30}, 50, 20},
		{"p50 even", []float64{10, 20, 30, 40}, 50, 25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sorted := make([]float64, len(tc.input))
			copy(sorted, tc.input)
			// percentile espera slice ordenado
			for i := 0; i < len(sorted)-1; i++ {
				if sorted[i] > sorted[i+1] {
					t.Fatalf("test input not sorted: %v", tc.input)
				}
			}
			got := percentile(sorted, tc.p)
			if got != tc.expected {
				t.Fatalf("percentile(%v, %.0f) = %v, want %v", tc.input, tc.p, got, tc.expected)
			}
		})
	}
}

func TestComputeStats_ExcludesRepetition1AndErrors(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	measurements := []Measurement{
		{TokensPerSecond: 10, FirstResponseMs: 100, RAMMB: 1000, VRAMMB: 500, Repetition: 1, Timestamp: now},                   // carga
		{TokensPerSecond: 20, FirstResponseMs: 200, RAMMB: 1100, VRAMMB: 600, Repetition: 2, Timestamp: now},                   // válida
		{TokensPerSecond: 30, FirstResponseMs: 300, RAMMB: 1200, VRAMMB: 700, Repetition: 3, Timestamp: now},                   // válida
		{TokensPerSecond: 40, FirstResponseMs: 400, RAMMB: 1300, VRAMMB: 800, Repetition: 4, Error: "timeout", Timestamp: now}, // error
	}
	stats := ComputeStats(measurements, 0)
	// Solo repeticiones 2 y 3 cuentan (2 valores)
	if stats.ValidRepetitions != 2 {
		t.Fatalf("ValidRepetitions = %d, want 2", stats.ValidRepetitions)
	}
	// p50 de [20,30] = 25, p95 ≈ 29.5
	if stats.TokensPerSecondP50 != 25 {
		t.Fatalf("TokensPerSecondP50 = %v, want 25", stats.TokensPerSecondP50)
	}
	if stats.TokensPerSecondP95 != 29.5 {
		t.Fatalf("TokensPerSecondP95 = %v, want 29.5", stats.TokensPerSecondP95)
	}
}

func TestComputeStats_ExcludesOverBudget(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	measurements := []Measurement{
		{TokensPerSecond: 10, FirstResponseMs: 100, RAMMB: 1000, VRAMMB: 500, Repetition: 1, Timestamp: now},
		{TokensPerSecond: 20, FirstResponseMs: 200, RAMMB: 1100, VRAMMB: 600, Repetition: 2, Timestamp: now},  // VRAM 600
		{TokensPerSecond: 30, FirstResponseMs: 300, RAMMB: 1200, VRAMMB: 1900, Repetition: 3, Timestamp: now}, // over budget (1900 > 1800)
	}
	stats := ComputeStats(measurements, 1800)
	if stats.ValidRepetitions != 1 {
		t.Fatalf("ValidRepetitions = %d, want 1 (one over budget)", stats.ValidRepetitions)
	}
	if stats.TokensPerSecondP50 != 20 {
		t.Fatalf("TokensPerSecondP50 = %v, want 20", stats.TokensPerSecondP50)
	}
}

func TestBuildConfigResult_OverBudgetAll(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	measurements := []Measurement{
		{TokensPerSecond: 10, FirstResponseMs: 100, RAMMB: 1000, VRAMMB: 500, Repetition: 1, Timestamp: now},
		{TokensPerSecond: 20, FirstResponseMs: 200, RAMMB: 1100, VRAMMB: 1900, Repetition: 2, Timestamp: now}, // over
		{TokensPerSecond: 30, FirstResponseMs: 300, RAMMB: 1200, VRAMMB: 2000, Repetition: 3, Timestamp: now}, // over
	}
	cr := BuildConfigResult(ModelConfig{Model: "test"}, measurements, 1800)
	if !cr.OverBudget {
		t.Fatal("OverBudget should be true when all valid reps are over budget")
	}
}

func TestBuildConfigResult_NotOverBudget(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	measurements := []Measurement{
		{TokensPerSecond: 10, FirstResponseMs: 100, RAMMB: 1000, VRAMMB: 500, Repetition: 1, Timestamp: now},
		{TokensPerSecond: 20, FirstResponseMs: 200, RAMMB: 1100, VRAMMB: 1900, Repetition: 2, Timestamp: now}, // over
		{TokensPerSecond: 30, FirstResponseMs: 300, RAMMB: 1200, VRAMMB: 600, Repetition: 3, Timestamp: now},  // ok
	}
	cr := BuildConfigResult(ModelConfig{Model: "test"}, measurements, 1800)
	if cr.OverBudget {
		t.Fatal("OverBudget should be false when at least one valid rep is under budget")
	}
}

func TestSaveAndLoadProfile_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "hardware_profile.json")

	profile := HardwareProfile{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		HostInfo: HostInfo{
			GoVersion:  "go1.23",
			NumCPU:     4,
			GpuName:    "GTX 1050 Ti",
			TotalRAMMB: 8192,
		},
		VRAMBudgetMB: 1800,
		Results: []ConfigResult{
			BuildConfigResult(
				ModelConfig{Model: "gemma3:4b", GPULayers: 0, NumThread: 4}.Normalize(),
				[]Measurement{
					{TokensPerSecond: 10, FirstResponseMs: 500, RAMMB: 4000, VRAMMB: 0, Repetition: 1, Timestamp: time.Now().UTC().Format(time.RFC3339)},
					{TokensPerSecond: 12.5, FirstResponseMs: 300, RAMMB: 4100, VRAMMB: 0, Repetition: 2, Timestamp: time.Now().UTC().Format(time.RFC3339)},
					{TokensPerSecond: 13.0, FirstResponseMs: 290, RAMMB: 4150, VRAMMB: 0, Repetition: 3, Timestamp: time.Now().UTC().Format(time.RFC3339)},
				},
				0,
			),
		},
	}

	if err := SaveProfile(profile, path); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	loaded, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	if loaded.SchemaVersion != profile.SchemaVersion {
		t.Fatalf("SchemaVersion mismatch: %d vs %d", loaded.SchemaVersion, profile.SchemaVersion)
	}
	if len(loaded.Results) != len(profile.Results) {
		t.Fatalf("Results count mismatch: %d vs %d", len(loaded.Results), len(profile.Results))
	}
	if loaded.VRAMBudgetMB != profile.VRAMBudgetMB {
		t.Fatalf("VRAMBudgetMB mismatch: %v vs %v", loaded.VRAMBudgetMB, profile.VRAMBudgetMB)
	}
	if loaded.HostInfo.GpuName != profile.HostInfo.GpuName {
		t.Fatalf("GpuName mismatch: %q vs %q", loaded.HostInfo.GpuName, profile.HostInfo.GpuName)
	}

	// Verificar que los stats se conservan
	if loaded.Results[0].Stats.ValidRepetitions != profile.Results[0].Stats.ValidRepetitions {
		t.Fatalf("ValidRepetitions mismatch: %d vs %d", loaded.Results[0].Stats.ValidRepetitions, profile.Results[0].Stats.ValidRepetitions)
	}
}

func TestLoadProfile_CorruptFile_ReturnsErrorAndDoesNotOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "hardware_profile.json")

	// Escribir un JSON inválido
	corrupt := []byte("{not valid json")
	if err := memory.WriteFileAtomic(path, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	// Intentar cargar debe fallar
	_, err := LoadProfile(path)
	if err == nil {
		t.Fatal("LoadProfile should fail on corrupt file")
	}

	// Verificar que el archivo original sigue intacto (no se sobrescribió)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != string(corrupt) {
		t.Fatalf("corrupt file was overwritten: got %q", string(data))
	}
}

func TestLoadProfile_WrongSchemaVersion_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "hardware_profile.json")

	// Perfil con versión antigua
	oldProfile := HardwareProfile{
		SchemaVersion: SchemaVersion - 1,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(oldProfile, "", "  ")
	if err := memory.WriteFileAtomic(path, data, 0o644); err != nil {
		t.Fatalf("write old profile: %v", err)
	}

	_, err := LoadProfile(path)
	if err == nil {
		t.Fatal("LoadProfile should fail on wrong schema version")
	}
}

func TestSaveProfile_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "hardware_profile.json")

	profile := HardwareProfile{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		VRAMBudgetMB:  1800,
	}

	// Primera escritura
	if err := SaveProfile(profile, path); err != nil {
		t.Fatalf("first save: %v", err)
	}
	firstMod := getModTime(path)

	// Segunda escritura (simula actualización)
	profile.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	if err := SaveProfile(profile, path); err != nil {
		t.Fatalf("second save: %v", err)
	}
	secondMod := getModTime(path)

	// El archivo debe haberse reescrito (mod time cambia)
	if !secondMod.After(firstMod) {
		t.Logf("warning: mod time did not change (may be same second): first=%v second=%v", firstMod, secondMod)
	}

	// Verificar que no hay .tmp residuales (el .bak es esperado por WriteFileAtomic)
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if e.Name() != "hardware_profile.json" && e.Name() != "hardware_profile.json.bak" {
			t.Fatalf("unexpected residual file: %s", e.Name())
		}
	}
}

func getModTime(path string) time.Time {
	info, _ := os.Stat(path)
	return info.ModTime()
}

func TestEnsureProfileDir(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sub", "dir", "profile.json")

	if err := EnsureProfileDir(path); err != nil {
		t.Fatalf("EnsureProfileDir: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("directory not created: %v", err)
	}
}
