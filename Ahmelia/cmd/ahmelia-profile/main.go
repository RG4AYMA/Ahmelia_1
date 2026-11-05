package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"ahmelia/resource"
)

type MatrixConfig struct {
	Configs []resource.ModelConfig `json:"configs"`
}

type Prompts struct {
	Interpretation string `json:"interpretation"`
	Generation     string `json:"generation"`
}

func main() {
	matrixPath := flag.String("matrix", "", "Ruta al JSON de matriz de configuraciones (requerido)")
	promptsPath := flag.String("prompts", "testdata/profile/prompts.json", "Ruta al JSON de prompts")
	outputPath := flag.String("output", "memory/hardware_profile.json", "Ruta de salida del perfil")
	repetitions := flag.Int("reps", 5, "Número de repeticiones por configuración (default 5, la primera se descarta)")
	timeout := flag.Duration("timeout", 10*time.Minute, "Timeout por medición individual")
	vramBudget := flag.Float64("vram-budget", 1800, "Presupuesto VRAM en MB para over_budget (default 1800)")
	flag.Parse()

	if *matrixPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -matrix es requerido")
		flag.Usage()
		os.Exit(1)
	}

	if *repetitions < 2 {
		fmt.Fprintln(os.Stderr, "Error: -reps debe ser >= 2 (se descarta la primera)")
		os.Exit(1)
	}

	matrixData, err := os.ReadFile(*matrixPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error leyendo matriz: %v\n", err)
		os.Exit(1)
	}
	var matrix MatrixConfig
	if err := json.Unmarshal(matrixData, &matrix); err != nil {
		fmt.Fprintf(os.Stderr, "Error parseando matriz: %v\n", err)
		os.Exit(1)
	}
	if len(matrix.Configs) == 0 {
		fmt.Fprintln(os.Stderr, "Error: matriz vacía")
		os.Exit(1)
	}

	promptsData, err := os.ReadFile(*promptsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error leyendo prompts: %v\n", err)
		os.Exit(1)
	}
	var prompts Prompts
	if err := json.Unmarshal(promptsData, &prompts); err != nil {
		fmt.Fprintf(os.Stderr, "Error parseando prompts: %v\n", err)
		os.Exit(1)
	}

	prober := resource.NewOllamaProber()
	prober.VRAMBudgetMB = *vramBudget

	hostInfo := resource.HostInfo{
		GoVersion: runtime.Version(),
		NumCPU:    runtime.NumCPU(),
	}

	var results []resource.ConfigResult

	fmt.Printf("=== Ahmelia Profile ===\n")
	fmt.Printf("Matrix: %s (%d configs)\n", *matrixPath, len(matrix.Configs))
	fmt.Printf("Reps: %d (discard first)\n", *repetitions)
	fmt.Printf("VRAM Budget: %.0f MB\n", *vramBudget)
	fmt.Printf("Output: %s\n\n", *outputPath)

	for i, cfg := range matrix.Configs {
		cfg = cfg.Normalize()
		fmt.Printf("[%d/%d] %s\n", i+1, len(matrix.Configs), cfg.Key())

		var measurements []resource.Measurement
		configFailed := false

		for rep := 1; rep <= *repetitions; rep++ {
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)

			prompt := prompts.Interpretation
			if rep%2 == 0 {
				prompt = prompts.Generation
			}

			m, err := prober.Measure(ctx, cfg, prompt)
			cancel()

			if err != nil {
				if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
					fmt.Printf("  Rep %d: CANCELLED/TIMEOUT\n", rep)
					m.Error = ctx.Err().Error()
				} else {
					fmt.Printf("  Rep %d: ERROR: %v\n", rep, err)
					m.Error = err.Error()
				}
				configFailed = true
			} else {
				fmt.Printf("  Rep %d: %.2f tok/s, %.0fms first, RAM=%.0fMB VRAM=%.0fMB%s\n",
					rep, m.TokensPerSecond, m.FirstResponseMs, m.RAMMB, m.VRAMMB,
					map[bool]string{true: " OVER_BUDGET", false: ""}[m.OverBudget])
			}
			m.Repetition = rep
			measurements = append(measurements, m)

			if rep < *repetitions {
				time.Sleep(500 * time.Millisecond)
			}
		}

		validCount := 0
		for _, m := range measurements {
			if m.Repetition > 1 && m.Error == "" {
				validCount++
			}
		}
		if validCount == 0 {
			fmt.Printf("  Configuración %s: TODAS las repeticiones válidas fallaron, se omite del promedio\n", cfg.Key())
			configFailed = true
		}

		cr := resource.BuildConfigResult(cfg, measurements, *vramBudget)
		results = append(results, cr)

		if configFailed {
			fmt.Printf("  Config marcada como incompleta\n")
		}
		fmt.Println()
	}

	profile := resource.HardwareProfile{
		SchemaVersion: resource.SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		HostInfo:      hostInfo,
		VRAMBudgetMB:  *vramBudget,
		Results:       results,
	}

	if err := resource.EnsureProfileDir(*outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error creando directorio: %v\n", err)
		os.Exit(1)
	}

	if err := resource.SaveProfile(profile, *outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error guardando perfil: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Perfil guardado en %s\n", *outputPath)
	printSummary(results)
}

func printSummary(results []resource.ConfigResult) {
	fmt.Println("\n=== RESUMEN ===")
	for _, cr := range results {
		s := cr.Stats
		status := "OK"
		if cr.OverBudget {
			status = "OVER_BUDGET"
		}
		if s.ValidRepetitions == 0 {
			status = "SIN DATOS VÁLIDOS"
		}
		fmt.Printf("%-30s | tok/s p50=%.1f p95=%.1f | first p50=%.0fms p95=%.0fms | RAM p50=%.0fMB | VRAM p50=%.0fMB | reps=%d | %s\n",
			cr.Config.Key(),
			s.TokensPerSecondP50, s.TokensPerSecondP95,
			s.FirstResponseMsP50, s.FirstResponseMsP95,
			s.RAMMBP50, s.VRAMMBP50,
			s.ValidRepetitions, status)
	}
}

func MatrixConfigForTests() []resource.ModelConfig {
	cfgs := []resource.ModelConfig{
		{Model: "gemma3:4b", GPULayers: 0, NumThread: 3, Temperature: 0},
		{Model: "gemma3:4b", GPULayers: 0, NumThread: 4, Temperature: 0},
		{Model: "gemma3:4b", GPULayers: 18, NumThread: 4, Temperature: 0},
	}
	for i := range cfgs {
		cfgs[i] = cfgs[i].Normalize()
	}
	return cfgs
}

func WriteTestMatrix(path string) error {
	cfgs := MatrixConfigForTests()
	matrix := MatrixConfig{Configs: cfgs}
	data, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
