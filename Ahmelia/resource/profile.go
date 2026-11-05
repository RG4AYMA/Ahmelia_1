package resource

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"ahmelia/memory"
)

// SchemaVersion es la versión del esquema de persistencia.
// Incrementar solo con cambios incompatibles hacia atrás.
const SchemaVersion = 1

// ModelConfig describe una configuración de modelo a medir.
type ModelConfig struct {
	Model       string  `json:"model"`       // ej. "gemma3:4b"
	GPULayers   int     `json:"gpu_layers"`  // 0 = solo CPU; >0 = capas en GPU
	NumThread   int     `json:"num_thread"`  // hilos de CPU (0 = default de Ollama)
	Temperature float64 `json:"temperature"` // 0 = determinista
}

// Normalize aplica valores por defecto y valida.
func (m ModelConfig) Normalize() ModelConfig {
	if m.Temperature < 0 {
		m.Temperature = 0
	}
	if m.NumThread < 0 {
		m.NumThread = 0
	}
	if m.GPULayers < 0 {
		m.GPULayers = 0
	}
	return m
}

// Key devuelve una clave estable para usar como índice en mapas.
func (m ModelConfig) Key() string {
	return fmt.Sprintf("%s|gl=%d|nt=%d", m.Model, m.GPULayers, m.NumThread)
}

// Measurement es una única medición cruda.
type Measurement struct {
	TokensPerSecond float64 `json:"tokens_per_second"`
	FirstResponseMs float64 `json:"first_response_ms"`
	RAMMB           float64 `json:"ram_mb"`
	VRAMMB          float64 `json:"vram_mb"`
	Repetition      int     `json:"repetition"`      // 1-based; 1 incluye carga del modelo
	OverBudget      bool    `json:"over_budget"`     // true si VRAMMB > VRAMBudgetMB
	Timestamp       string  `json:"timestamp"`       // RFC3339
	Error           string  `json:"error,omitempty"` // si falló la medición
}

// AggregatedStats contiene p50 y p95 de una serie (sin la repetición 1).
type AggregatedStats struct {
	TokensPerSecondP50 float64 `json:"tokens_per_second_p50"`
	TokensPerSecondP95 float64 `json:"tokens_per_second_p95"`
	FirstResponseMsP50 float64 `json:"first_response_ms_p50"`
	FirstResponseMsP95 float64 `json:"first_response_ms_p95"`
	RAMMBP50           float64 `json:"ram_mb_p50"`
	RAMMBP95           float64 `json:"ram_mb_p95"`
	VRAMMBP50          float64 `json:"vram_mb_p50"`
	VRAMMBP95          float64 `json:"vram_mb_p95"`
	ValidRepetitions   int     `json:"valid_repetitions"` // repeticiones válidas (excluye 1 y errores)
}

// ConfigResult agrupa una configuración con sus mediciones y estadísticos.
type ConfigResult struct {
	Config       ModelConfig     `json:"config"`
	Measurements []Measurement   `json:"measurements"`
	Stats        AggregatedStats `json:"stats"`
	OverBudget   bool            `json:"over_budget"` // true si TODAS las repeticiones válidas están over_budget
}

// HardwareProfile es el artefacto persistido.
type HardwareProfile struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   string         `json:"generated_at"` // RFC3339
	HostInfo      HostInfo       `json:"host_info"`
	VRAMBudgetMB  float64        `json:"vram_budget_mb"` // presupuesto VRAM usado para over_budget
	Results       []ConfigResult `json:"results"`
}

// HostInfo captura contexto del equipo de medición.
type HostInfo struct {
	GoVersion  string `json:"go_version"`
	NumCPU     int    `json:"num_cpu"`
	GpuName    string `json:"gpu_name,omitempty"`
	TotalRAMMB int    `json:"total_ram_mb,omitempty"`
}

// DefaultProfilePath es la ruta canónica del perfil.
const DefaultProfilePath = "memory/hardware_profile.json"

// LoadProfile carga el perfil desde disco.
// Devuelve error si el archivo no existe, está corrupto o la versión no coincide.
func LoadProfile(path string) (HardwareProfile, error) {
	var zero HardwareProfile
	if path == "" {
		path = DefaultProfilePath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("leer perfil: %w", err)
	}
	var profile HardwareProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return zero, fmt.Errorf("parsear perfil: %w", err)
	}
	if profile.SchemaVersion != SchemaVersion {
		return zero, fmt.Errorf("versión de esquema incompatible: got %d want %d", profile.SchemaVersion, SchemaVersion)
	}
	return profile, nil
}

// SaveProfile guarda el perfil atómicamente usando memory.WriteFileAtomic.
// Nunca sobrescribe un archivo corrupto: si la lectura falla, el error se propaga y el archivo original queda intacto.
func SaveProfile(profile HardwareProfile, path string) error {
	if path == "" {
		path = DefaultProfilePath
	}
	profile.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	profile.SchemaVersion = SchemaVersion

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal perfil: %w", err)
	}

	if err := memory.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("escribir perfil atómico: %w", err)
	}
	return nil
}

// percentile calcula el percentil p (0-100) de una slice ordenada.
// Si la slice está vacía, devuelve 0.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := p / 100.0 * float64(len(sorted)-1)
	i := int(idx)
	if i >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(i)
	return sorted[i] + frac*(sorted[i+1]-sorted[i])
}

// ComputeStats calcula p50 y p95 a partir de mediciones válidas (repetition > 1, sin error, no over_budget).
// Descarta la primera repetición (carga del modelo) y las marcadas over_budget.
func ComputeStats(measurements []Measurement, vramBudgetMB float64) AggregatedStats {
	var tps, fr, ram, vram []float64
	for _, m := range measurements {
		if m.Repetition <= 1 {
			continue // descarta carga
		}
		if m.Error != "" {
			continue
		}
		if vramBudgetMB > 0 && m.VRAMMB > vramBudgetMB {
			continue // over budget
		}
		tps = append(tps, m.TokensPerSecond)
		fr = append(fr, m.FirstResponseMs)
		ram = append(ram, m.RAMMB)
		vram = append(vram, m.VRAMMB)
	}

	sort.Float64s(tps)
	sort.Float64s(fr)
	sort.Float64s(ram)
	sort.Float64s(vram)

	return AggregatedStats{
		TokensPerSecondP50: percentile(tps, 50),
		TokensPerSecondP95: percentile(tps, 95),
		FirstResponseMsP50: percentile(fr, 50),
		FirstResponseMsP95: percentile(fr, 95),
		RAMMBP50:           percentile(ram, 50),
		RAMMBP95:           percentile(ram, 95),
		VRAMMBP50:          percentile(vram, 50),
		VRAMMBP95:          percentile(vram, 95),
		ValidRepetitions:   len(tps),
	}
}

// BuildConfigResult calcula stats y over_budget global para una configuración.
func BuildConfigResult(config ModelConfig, measurements []Measurement, vramBudgetMB float64) ConfigResult {
	stats := ComputeStats(measurements, vramBudgetMB)
	overBudgetAll := true
	for _, m := range measurements {
		if m.Repetition <= 1 || m.Error != "" {
			continue
		}
		if vramBudgetMB <= 0 || m.VRAMMB <= vramBudgetMB {
			overBudgetAll = false
			break
		}
	}
	return ConfigResult{
		Config:       config,
		Measurements: measurements,
		Stats:        stats,
		OverBudget:   overBudgetAll,
	}
}

// EnsureProfileDir asegura que el directorio del perfil exista.
func EnsureProfileDir(path string) error {
	if path == "" {
		path = DefaultProfilePath
	}
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
