package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/resource"
)

func TestMain_WithFakeServer(t *testing.T) {
	// Servidor falso Ollama
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			var reqBody resource.GenerateRequest
			json.NewDecoder(r.Body).Decode(&reqBody)

			// Respuesta con métricas simuladas
			resp := resource.GenerateResponse{
				Response:     "Respuesta de prueba del modelo para medición de rendimiento.",
				Done:         true,
				EvalCount:    50,
				EvalDuration: 2_000_000_000, // 2 segundos
			}
			json.NewEncoder(w).Encode(resp)
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			resp := resource.PSResponse{
				Models: []resource.PSModel{
					{Name: "gemma3:4b", Size: 4 * 1024 * 1024 * 1024, SizeVRAM: 1800 * 1024 * 1024},
				},
			}
			json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Matriz de prueba
	tmpDir := t.TempDir()
	matrixPath := filepath.Join(tmpDir, "matrix.json")
	matrix := MatrixConfig{
		Configs: []resource.ModelConfig{
			{Model: "gemma3:4b", GPULayers: 0, NumThread: 4, Temperature: 0},
		},
	}
	data, _ := json.MarshalIndent(matrix, "", "  ")
	os.WriteFile(matrixPath, data, 0o644)

	// Prompts
	promptsPath := filepath.Join(tmpDir, "prompts.json")
	prompts := map[string]string{
		"interpretation": "Analiza: \"test\". Devuelve JSON.",
		"generation":     "Genera respuesta. Devuelve JSON.",
	}
	promptsData, _ := json.MarshalIndent(prompts, "", "  ")
	os.WriteFile(promptsPath, promptsData, 0o644)

	// Output
	outputPath := filepath.Join(tmpDir, "hardware_profile.json")

	// Ejecutar lógica principal (extraída para test)
	err := runProfile(context.Background(), runProfileOptions{
		MatrixPath:  matrixPath,
		PromptsPath: promptsPath,
		OutputPath:  outputPath,
		Repetitions: 3,
		Timeout:     5 * time.Second,
		VRAMBudget:  1800,
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("runProfile error: %v", err)
	}

	// Verificar que se creó el perfil
	profile, err := resource.LoadProfile(outputPath)
	if err != nil {
		t.Fatalf("LoadProfile error: %v", err)
	}

	if len(profile.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(profile.Results))
	}

	cr := profile.Results[0]
	if cr.Stats.ValidRepetitions != 2 { // 3 reps - 1 discard = 2
		t.Fatalf("ValidRepetitions = %d, want 2", cr.Stats.ValidRepetitions)
	}
	if cr.OverBudget {
		t.Fatal("config should not be over budget (VRAM 1800 <= budget 1800)")
	}

	t.Logf("Profile generated successfully: %+v", cr.Stats)
}

func TestMain_ConfigOverBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resource.GenerateResponse{
				Response: "ok", Done: true, EvalCount: 10, EvalDuration: 1_000_000_000,
			})
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			// VRAM 3GB > budget 1800MB
			json.NewEncoder(w).Encode(resource.PSResponse{
				Models: []resource.PSModel{{Name: "test", Size: 4 * 1024 * 1024 * 1024, SizeVRAM: 3 * 1024 * 1024 * 1024}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	matrixPath := filepath.Join(tmpDir, "matrix.json")
	matrix := MatrixConfig{
		Configs: []resource.ModelConfig{{Model: "test", GPULayers: 0, NumThread: 4}},
	}
	os.WriteFile(matrixPath, mustMarshal(matrix), 0o644)

	promptsPath := filepath.Join(tmpDir, "prompts.json")
	os.WriteFile(promptsPath, mustMarshal(map[string]string{"interpretation": "test"}), 0o644)

	outputPath := filepath.Join(tmpDir, "profile.json")

	err := runProfile(context.Background(), runProfileOptions{
		MatrixPath:  matrixPath,
		PromptsPath: promptsPath,
		OutputPath:  outputPath,
		Repetitions: 3,
		Timeout:     5 * time.Second,
		VRAMBudget:  1800,
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("runProfile error: %v", err)
	}

	profile, err := resource.LoadProfile(outputPath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	if !profile.Results[0].OverBudget {
		t.Fatal("config should be marked over budget (VRAM 3072 > 1800)")
	}
}

func TestMain_ConfigNotFoundInPS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resource.GenerateResponse{Response: "ok", Done: true})
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resource.PSResponse{Models: []resource.PSModel{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	matrixPath := filepath.Join(tmpDir, "matrix.json")
	matrix := MatrixConfig{Configs: []resource.ModelConfig{{Model: "unknown", GPULayers: 0, NumThread: 4}}}
	os.WriteFile(matrixPath, mustMarshal(matrix), 0o644)

	promptsPath := filepath.Join(tmpDir, "prompts.json")
	os.WriteFile(promptsPath, mustMarshal(map[string]string{"interpretation": "test"}), 0o644)

	outputPath := filepath.Join(tmpDir, "profile.json")

	err := runProfile(context.Background(), runProfileOptions{
		MatrixPath:  matrixPath,
		PromptsPath: promptsPath,
		OutputPath:  outputPath,
		Repetitions: 3,
		Timeout:     5 * time.Second,
		VRAMBudget:  1800,
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("runProfile: %v", err)
	}

	profile, err := resource.LoadProfile(outputPath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	// RAM/VRAM deben ser 0 cuando no se encuentra en /api/ps
	if profile.Results[0].Measurements[1].RAMMB != 0 || profile.Results[0].Measurements[1].VRAMMB != 0 {
		t.Fatalf("RAM/VRAM should be 0 when model not in /api/ps")
	}
}

func TestMain_DiscardsFirstRepetition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resource.GenerateResponse{
				Response: "ok", Done: true, EvalCount: 10, EvalDuration: 1_000_000_000,
			})
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resource.PSResponse{
				Models: []resource.PSModel{{Name: "test", Size: 4 * 1024 * 1024 * 1024, SizeVRAM: 1000 * 1024 * 1024}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	matrixPath := filepath.Join(tmpDir, "matrix.json")
	matrix := MatrixConfig{Configs: []resource.ModelConfig{{Model: "test", GPULayers: 0, NumThread: 4}}}
	os.WriteFile(matrixPath, mustMarshal(matrix), 0o644)

	promptsPath := filepath.Join(tmpDir, "prompts.json")
	os.WriteFile(promptsPath, mustMarshal(map[string]string{"interpretation": "test"}), 0o644)

	outputPath := filepath.Join(tmpDir, "profile.json")

	err := runProfile(context.Background(), runProfileOptions{
		MatrixPath:  matrixPath,
		PromptsPath: promptsPath,
		OutputPath:  outputPath,
		Repetitions: 4, // 4 reps -> 3 válidas después de descartar primera
		Timeout:     5 * time.Second,
		VRAMBudget:  1800,
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("runProfile: %v", err)
	}

	profile, err := resource.LoadProfile(outputPath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	cr := profile.Results[0]
	if cr.Stats.ValidRepetitions != 3 {
		t.Fatalf("ValidRepetitions = %d, want 3 (4 reps - 1 discard)", cr.Stats.ValidRepetitions)
	}
}

func TestMain_SkipsConfigWithAllFailedReps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	matrixPath := filepath.Join(tmpDir, "matrix.json")
	matrix := MatrixConfig{Configs: []resource.ModelConfig{{Model: "test", GPULayers: 0, NumThread: 4}}}
	os.WriteFile(matrixPath, mustMarshal(matrix), 0o644)

	promptsPath := filepath.Join(tmpDir, "prompts.json")
	os.WriteFile(promptsPath, mustMarshal(map[string]string{"interpretation": "test"}), 0o644)

	outputPath := filepath.Join(tmpDir, "profile.json")

	err := runProfile(context.Background(), runProfileOptions{
		MatrixPath:  matrixPath,
		PromptsPath: promptsPath,
		OutputPath:  outputPath,
		Repetitions: 3,
		Timeout:     5 * time.Second,
		VRAMBudget:  1800,
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("runProfile: %v", err)
	}

	profile, err := resource.LoadProfile(outputPath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	cr := profile.Results[0]
	if cr.Stats.ValidRepetitions != 0 {
		t.Fatalf("ValidRepetitions = %d, want 0 (all failed)", cr.Stats.ValidRepetitions)
	}
}

func TestWriteTestMatrix(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_matrix.json")

	err := WriteTestMatrix(path)
	if err != nil {
		t.Fatalf("WriteTestMatrix: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var m MatrixConfig
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(m.Configs) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(m.Configs))
	}

	// Verificar que incluye la configuración GPU 18 (límite seguro)
	foundGPU18 := false
	for _, c := range m.Configs {
		if c.GPULayers == 18 {
			foundGPU18 = true
		}
	}
	if !foundGPU18 {
		t.Fatal("matrix should include GPU layers = 18 (safe limit)")
	}
}

func TestRunProfile_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resource.GenerateResponse{Response: "slow", Done: true})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	matrixPath := filepath.Join(tmpDir, "matrix.json")
	matrix := MatrixConfig{Configs: []resource.ModelConfig{{Model: "test", GPULayers: 0, NumThread: 4}}}
	os.WriteFile(matrixPath, mustMarshal(matrix), 0o644)

	promptsPath := filepath.Join(tmpDir, "prompts.json")
	os.WriteFile(promptsPath, mustMarshal(map[string]string{"interpretation": "test"}), 0o644)

	outputPath := filepath.Join(tmpDir, "profile.json")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancelar inmediatamente

	err := runProfile(ctx, runProfileOptions{
		MatrixPath:  matrixPath,
		PromptsPath: promptsPath,
		OutputPath:  outputPath,
		Repetitions: 3,
		Timeout:     5 * time.Second,
		VRAMBudget:  1800,
		BaseURL:     server.URL,
	})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type runProfileOptions struct {
	MatrixPath  string
	PromptsPath string
	OutputPath  string
	Repetitions int
	Timeout     time.Duration
	VRAMBudget  float64
	BaseURL     string
}

func runProfile(ctx context.Context, opts runProfileOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Cargar matriz
	matrixData, err := os.ReadFile(opts.MatrixPath)
	if err != nil {
		return err
	}
	var matrix MatrixConfig
	if err := json.Unmarshal(matrixData, &matrix); err != nil {
		return err
	}

	// Cargar prompts
	promptsData, err := os.ReadFile(opts.PromptsPath)
	if err != nil {
		return err
	}
	var prompts Prompts
	if err := json.Unmarshal(promptsData, &prompts); err != nil {
		return err
	}

	// Prober con BaseURL personalizado para tests
	prober := resource.NewOllamaProber()
	prober.BaseURL = opts.BaseURL
	prober.VRAMBudgetMB = opts.VRAMBudget
	prober.HTTPClient = &http.Client{Timeout: opts.Timeout}

	hostInfo := resource.HostInfo{
		GoVersion: "test",
		NumCPU:    4,
	}

	var results []resource.ConfigResult

	for _, cfg := range matrix.Configs {
		cfg = cfg.Normalize()
		var measurements []resource.Measurement

		for rep := 1; rep <= opts.Repetitions; rep++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			repCtx, cancel := context.WithTimeout(ctx, opts.Timeout)

			prompt := prompts.Interpretation
			if rep%2 == 0 {
				prompt = prompts.Generation
			}

			m, err := prober.Measure(repCtx, cfg, prompt)
			cancel()

			if err != nil {
				if repCtx.Err() != nil {
					m.Error = repCtx.Err().Error()
				} else {
					m.Error = err.Error()
				}
			}
			m.Repetition = rep
			measurements = append(measurements, m)
		}

		cr := resource.BuildConfigResult(cfg, measurements, opts.VRAMBudget)
		results = append(results, cr)
	}

	profile := resource.HardwareProfile{
		SchemaVersion: resource.SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		HostInfo:      hostInfo,
		VRAMBudgetMB:  opts.VRAMBudget,
		Results:       results,
	}

	return resource.SaveProfile(profile, opts.OutputPath)
}

func mustMarshal(v interface{}) []byte {
	data, _ := json.MarshalIndent(v, "", "  ")
	return data
}

// TestShippedPromptsFile_Parses verifica que el prompts.json distribuido en
// testdata/profile parsea con el struct Prompts del binario.
// Regresión: el archivo estaba en formato anidado (description+prompt) que el
// parser plano no podía leer, y ningún test cargaba el archivo real.
func TestShippedPromptsFile_Parses(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "profile", "prompts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}
	var prompts Prompts
	if err := json.Unmarshal(data, &prompts); err != nil {
		t.Fatalf("parsear %s con struct Prompts: %v", path, err)
	}
	if strings.TrimSpace(prompts.Interpretation) == "" {
		t.Fatal("interpretation vacío en prompts.json")
	}
	if strings.TrimSpace(prompts.Generation) == "" {
		t.Fatal("generation vacío en prompts.json")
	}
}

// TestShippedMatrixFile_Parses verifica que la matriz distribuida en
// testdata/profile/matrix.json parsea con MatrixConfig y trae al menos una
// configuración con modelo. Los valores concretos los ajusta el propietario.
func TestShippedMatrixFile_Parses(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "profile", "matrix.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}
	var matrix MatrixConfig
	if err := json.Unmarshal(data, &matrix); err != nil {
		t.Fatalf("parsear %s con MatrixConfig: %v", path, err)
	}
	if len(matrix.Configs) == 0 {
		t.Fatal("matrix.json sin configuraciones")
	}
	for i, c := range matrix.Configs {
		if strings.TrimSpace(c.Model) == "" {
			t.Fatalf("config %d sin modelo", i)
		}
	}
}
