package resource

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// TaskClass identifica la clase de tarea para enrutamiento.
type TaskClass string

const (
	TaskClassInterpretation TaskClass = "interpretation" // interpretación lingüística (corta, JSON estricto)
	TaskClassGeneration     TaskClass = "generation"     // redacción conversacional (larga, natural)
	TaskClassProposal       TaskClass = "proposal"       // propuesta de código/plan (media)
)

// ModelLevel identifica un nivel de modelo en la jerarquía de calidad.
type ModelLevel string

const (
	ModelLevelSmall  ModelLevel = "small"  // ~1B en GPU (interpretación)
	ModelLevelMedium ModelLevel = "medium" // gemma3:4b en CPU/GPU parcial (redacción)
	ModelLevelLarge  ModelLevel = "large"  // externo / frontera (fallback)
)

// RouterConfig configura el enrutador por clase de tarea.
type RouterConfig struct {
	// PerTaskClass define para cada clase: tiempo máximo hasta primera respuesta (ms)
	// y lista de niveles de modelo ordenados de mayor a menor calidad.
	PerTaskClass map[TaskClass]TaskClassConfig
	// DefaultModel es el modelo actual por defecto (fuente "default").
	DefaultModel string
	// DefaultLevel es el nivel del modelo por defecto.
	DefaultLevel ModelLevel
}

type TaskClassConfig struct {
	MaxFirstResponseMs int          // presupuesto ms para primera respuesta
	Levels             []ModelLevel // ordenados de mayor a menor calidad
}

// PickResult es el resultado de la selección.
type PickResult struct {
	Level   ModelLevel
	Model   string
	Source  string // "measured" | "default"
	Class   TaskClass
	Budget  int  // ms asignados
	Matched bool // true si algún nivel medido cumplió el presupuesto
}

// Router selecciona el nivel de modelo apropiado para una clase de tarea.
type Router struct {
	config   RouterConfig
	profile  *HardwareProfile
	levelMap map[ModelLevel]string // nivel -> modelo (desde perfil medido)
}

// NewRouter crea un enrutador.
// Si profile es nil, usa solo la configuración por defecto.
func NewRouter(cfg RouterConfig, profile *HardwareProfile) *Router {
	r := &Router{
		config:   cfg,
		profile:  profile,
		levelMap: make(map[ModelLevel]string),
	}
	if profile != nil {
		r.buildLevelMap(profile)
	}
	return r
}

// buildLevelMap extrae modelo -> nivel desde el perfil medido.
func (r *Router) buildLevelMap(p *HardwareProfile) {
	// Mapeo heurístico: buscar en resultados cuál coincide con cada nivel
	for _, cr := range p.Results {
		cfg := cr.Config
		model := cfg.Model
		level := r.inferLevel(cfg)
		if level != "" && r.levelMap[level] == "" {
			r.levelMap[level] = model
		}
	}
}

// inferLevel deduce el nivel a partir de ModelConfig.
func (r *Router) inferLevel(cfg ModelConfig) ModelLevel {
	if cfg.GPULayers > 0 {
		if strings.Contains(cfg.Model, "1b") || strings.Contains(cfg.Model, "1B") {
			return ModelLevelSmall
		}
		return ModelLevelMedium
	}
	// CPU only
	if strings.Contains(cfg.Model, "1b") || strings.Contains(cfg.Model, "1B") {
		return ModelLevelSmall
	}
	return ModelLevelMedium
}

// Pick devuelve el mejor nivel para la clase de tarea.
// - Si hay perfil: usa p95 medido y elige el primer nivel que cumple presupuesto.
// - Si no hay perfil: devuelve modelo por defecto con fuente "default".
func (r *Router) Pick(class TaskClass) PickResult {
	tc, ok := r.config.PerTaskClass[class]
	if !ok {
		return PickResult{
			Level:  r.config.DefaultLevel,
			Model:  r.config.DefaultModel,
			Source: "default",
			Class:  class,
			Budget: 0,
		}
	}

	budget := tc.MaxFirstResponseMs

	if r.profile == nil || len(r.levelMap) == 0 {
		return PickResult{
			Level:   r.config.DefaultLevel,
			Model:   r.config.DefaultModel,
			Source:  "default",
			Class:   class,
			Budget:  budget,
			Matched: false,
		}
	}

	// Buscar primer nivel (orden calidad) cuyo p95 <= budget
	for _, level := range tc.Levels {
		model := r.levelMap[level]
		if model == "" {
			continue
		}
		// Buscar stats para este modelo
		if stats := r.findStats(model); stats != nil {
			if stats.FirstResponseMsP95 <= float64(budget) {
				return PickResult{
					Level:   level,
					Model:   model,
					Source:  "measured",
					Class:   class,
					Budget:  budget,
					Matched: true,
				}
			}
		}
	}

	// Ningún nivel medido cumple: devolver el más rápido disponible
	fastest := r.findFastestModel(tc.Levels)
	if fastest != "" {
		return PickResult{
			Level:   r.inferLevelFromModel(fastest),
			Model:   fastest,
			Source:  "measured",
			Class:   class,
			Budget:  budget,
			Matched: false, // marcó que no cumplió presupuesto
		}
	}

	// Fallback total
	return PickResult{
		Level:   r.config.DefaultLevel,
		Model:   r.config.DefaultModel,
		Source:  "default",
		Class:   class,
		Budget:  budget,
		Matched: false,
	}
}

// findStats busca stats agregados para un modelo en el perfil.
func (r *Router) findStats(model string) *AggregatedStats {
	if r.profile == nil {
		return nil
	}
	for _, cr := range r.profile.Results {
		if cr.Config.Model == model {
			return &cr.Stats
		}
	}
	return nil
}

// findFastestModel devuelve el modelo con menor p50 de first_response_ms entre todos los medidos.
func (r *Router) findFastestModel(levels []ModelLevel) string {
	var bestModel string
	var bestTime float64
	if r.profile == nil {
		return ""
	}
	for _, cr := range r.profile.Results {
		if stats := &cr.Stats; stats.ValidRepetitions > 0 {
			if bestModel == "" || stats.FirstResponseMsP50 < bestTime {
				bestModel = cr.Config.Model
				bestTime = stats.FirstResponseMsP50
			}
		}
	}
	return bestModel
}

func (r *Router) inferLevelFromModel(model string) ModelLevel {
	if strings.Contains(model, "1b") || strings.Contains(model, "1B") {
		return ModelLevelSmall
	}
	return ModelLevelMedium
}

// DefaultRouterConfig devuelve configuración sugerida según hardware actual.
func DefaultRouterConfig(defaultModel string) RouterConfig {
	return RouterConfig{
		DefaultModel: defaultModel,
		DefaultLevel: ModelLevelMedium,
		PerTaskClass: map[TaskClass]TaskClassConfig{
			TaskClassInterpretation: {
				MaxFirstResponseMs: 2000, // 2s para interpretación
				Levels:             []ModelLevel{ModelLevelSmall, ModelLevelMedium, ModelLevelLarge},
			},
			TaskClassGeneration: {
				MaxFirstResponseMs: 15000, // 15s para redacción
				Levels:             []ModelLevel{ModelLevelMedium, ModelLevelLarge, ModelLevelSmall},
			},
			TaskClassProposal: {
				MaxFirstResponseMs: 30000, // 30s para propuestas
				Levels:             []ModelLevel{ModelLevelMedium, ModelLevelLarge, ModelLevelSmall},
			},
		},
	}
}

// Validate verifica que la configuración es coherente.
func (r *Router) Validate() error {
	if r.config.DefaultModel == "" {
		return errors.New("default model required")
	}
	for class, tc := range r.config.PerTaskClass {
		if tc.MaxFirstResponseMs <= 0 {
			return fmt.Errorf("class %s: max first response ms must be > 0", class)
		}
		if len(tc.Levels) == 0 {
			return fmt.Errorf("class %s: at least one level required", class)
		}
	}
	return nil
}

// GetStatsForModel devuelve stats para un modelo (para debug/observabilidad).
func (r *Router) GetStatsForModel(model string) *AggregatedStats {
	return r.findStats(model)
}

// LevelsAvailable devuelve qué niveles tienen modelo medido.
func (r *Router) LevelsAvailable() map[ModelLevel]bool {
	result := make(map[ModelLevel]bool)
	for level, model := range r.levelMap {
		result[level] = model != ""
	}
	return result
}

// SortLevelsByQuality ordena niveles por calidad (mayor a menor).
func SortLevelsByQuality(levels []ModelLevel) {
	order := map[ModelLevel]int{
		ModelLevelLarge:  3,
		ModelLevelMedium: 2,
		ModelLevelSmall:  1,
	}
	sort.SliceStable(levels, func(i, j int) bool {
		return order[levels[i]] > order[levels[j]]
	})
}
