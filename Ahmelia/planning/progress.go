package planning

import (
	"fmt"
	"time"
)

// Progress rastrea el progreso de un objetivo
type Progress struct {
	GoalID      string
	StartTime   time.Time
	LastUpdated time.Time

	// Contadores
	TasksCompleted int
	TasksTotal     int
	TasksFailed    int

	// Métricas
	PercentComplete float64 // 0.0 - 1.0
	Velocity        float64 // tasks/hour
	BurndownRate    float64 // tasks completadas por hora

	// Tiempo
	TimeElapsed   time.Duration
	TimeRemaining time.Duration
	TimeEstimated time.Duration

	// Calidad
	ErrorCount      int
	SuccessRate     float64 // 0.0 - 1.0
	EfficiencyScore float64 // 0.0 - 1.0 (cuán eficiente es)

	// Bloqueos
	Blockers  []string
	BlockTime time.Duration

	// Historial
	Snapshots []ProgressSnapshot
}

// ProgressSnapshot es un punto en el tiempo del progreso
type ProgressSnapshot struct {
	Timestamp       time.Time
	TasksCompleted  int
	PercentComplete float64
	ErrorCount      int
}

// NewProgress crea un nuevo rastreador de progreso
func NewProgress(goalID string, totalTasks int, timeEstimated time.Duration) *Progress {
	return &Progress{
		GoalID:          goalID,
		StartTime:       time.Now(),
		LastUpdated:     time.Now(),
		TasksCompleted:  0,
		TasksTotal:      totalTasks,
		TasksFailed:     0,
		PercentComplete: 0.0,
		Velocity:        0.0,
		BurndownRate:    0.0,
		TimeElapsed:     0,
		TimeRemaining:   timeEstimated,
		TimeEstimated:   timeEstimated,
		ErrorCount:      0,
		SuccessRate:     1.0,
		EfficiencyScore: 1.0,
		Blockers:        []string{},
		BlockTime:       0,
		Snapshots:       []ProgressSnapshot{},
	}
}

// UpdateTaskCompleted registra que una tarea se completó
func (p *Progress) UpdateTaskCompleted() {
	p.TasksCompleted++
	p.updateMetrics()
	p.takeSnapshot()
}

// UpdateTaskFailed registra que una tarea falló
func (p *Progress) UpdateTaskFailed() {
	p.TasksFailed++
	p.ErrorCount++
	p.updateMetrics()
	p.takeSnapshot()
}

// AddBlocker añade un bloqueador
func (p *Progress) AddBlocker(blocker string) {
	p.Blockers = append(p.Blockers, blocker)
	p.updateMetrics()
}

// RemoveBlocker elimina un bloqueador
func (p *Progress) RemoveBlocker(blocker string) {
	filtered := []string{}
	for _, b := range p.Blockers {
		if b != blocker {
			filtered = append(filtered, b)
		}
	}
	p.Blockers = filtered
	p.updateMetrics()
}

// updateMetrics recalcula todas las métricas
func (p *Progress) updateMetrics() {
	p.LastUpdated = time.Now()
	p.TimeElapsed = time.Since(p.StartTime)

	// Porcentaje completado
	if p.TasksTotal > 0 {
		p.PercentComplete = float64(p.TasksCompleted) / float64(p.TasksTotal)
	}

	// Velocidad de ejecución (tareas/hora)
	hours := p.TimeElapsed.Hours()
	if hours > 0 {
		p.BurndownRate = float64(p.TasksCompleted) / hours
		p.Velocity = p.BurndownRate
	}

	// Tiempo restante estimado
	if p.BurndownRate > 0 {
		tasksRemaining := p.TasksTotal - p.TasksCompleted
		hoursRemaining := float64(tasksRemaining) / p.BurndownRate
		p.TimeRemaining = time.Duration(int64(hoursRemaining * float64(time.Hour)))
	} else {
		p.TimeRemaining = p.TimeEstimated
	}

	// Tasa de éxito
	if p.TasksTotal > 0 {
		totalAttempts := p.TasksCompleted + p.TasksFailed
		if totalAttempts > 0 {
			p.SuccessRate = float64(p.TasksCompleted) / float64(totalAttempts)
		}
	}

	// Eficiencia: (tiempo estimado vs tiempo real) * tasa de éxito
	if p.TimeElapsed > 0 {
		ratioEfficiency := p.TimeEstimated.Hours() / p.TimeElapsed.Hours()
		if ratioEfficiency > 1.0 {
			ratioEfficiency = 1.0
		}
		p.EfficiencyScore = ratioEfficiency * p.SuccessRate
	}
}

// takeSnapshot guarda un punto de progreso
func (p *Progress) takeSnapshot() {
	snapshot := ProgressSnapshot{
		Timestamp:       p.LastUpdated,
		TasksCompleted:  p.TasksCompleted,
		PercentComplete: p.PercentComplete,
		ErrorCount:      p.ErrorCount,
	}
	p.Snapshots = append(p.Snapshots, snapshot)
}

// IsOnTrack verifica si el progreso está en línea con el tiempo estimado
func (p *Progress) IsOnTrack() bool {
	if p.TimeElapsed == 0 {
		return true
	}

	expectedProgress := p.TimeElapsed.Hours() / p.TimeEstimated.Hours()
	return p.PercentComplete >= expectedProgress*0.8 // 80% de tolerancia
}

// GetStatus retorna un resumen del estado
func (p *Progress) GetStatus() string {
	status := fmt.Sprintf(
		"PROGRESO: %d/%d tareas (%.0f%%)\n"+
			"Velocidad: %.2f tasks/hora\n"+
			"Tiempo: %.1f/%.1f horas\n"+
			"Eficiencia: %.0f%%\n"+
			"Tasa de éxito: %.0f%%",
		p.TasksCompleted,
		p.TasksTotal,
		p.PercentComplete*100,
		p.BurndownRate,
		p.TimeElapsed.Hours(),
		p.TimeEstimated.Hours(),
		p.EfficiencyScore*100,
		p.SuccessRate*100,
	)

	if len(p.Blockers) > 0 {
		status += fmt.Sprintf("\n⚠️  Bloqueadores: %v", p.Blockers)
	}

	if !p.IsOnTrack() {
		status += "\n⚠️  ALERTA: Atraso en el cronograma"
	}

	return status
}

// GetVelocityTrend retorna la tendencia de velocidad
func (p *Progress) GetVelocityTrend() string {
	if len(p.Snapshots) < 2 {
		return "insuficientes datos"
	}

	firstHalf := len(p.Snapshots) / 2
	oldVelocity := float64(p.Snapshots[firstHalf].TasksCompleted) /
		(p.Snapshots[firstHalf].Timestamp.Sub(p.StartTime).Hours())

	currentVelocity := float64(p.TasksCompleted) /
		(p.TimeElapsed.Hours() + 0.001) // Evitar división por cero

	if currentVelocity > oldVelocity*1.1 {
		return "acelerando"
	} else if currentVelocity < oldVelocity*0.9 {
		return "desacelerando"
	}
	return "estable"
}

// ProjectedCompletion retorna la fecha estimada de finalización
func (p *Progress) ProjectedCompletion() time.Time {
	return time.Now().Add(p.TimeRemaining)
}

// GetHealthScore retorna un score de 0-100 sobre la salud del proyecto
func (p *Progress) GetHealthScore() int {
	score := 100

	// Reducir por atraso
	if !p.IsOnTrack() {
		score -= 20
	}

	// Reducir por errores
	if p.ErrorCount > 0 {
		score -= int(float64(p.ErrorCount) * 5)
	}

	// Reducir por bloqueadores
	if len(p.Blockers) > 0 {
		score -= len(p.Blockers) * 10
	}

	// Reducir por eficiencia baja
	if p.EfficiencyScore < 0.7 {
		score -= 15
	}

	if score < 0 {
		score = 0
	}

	return score
}

// GetRecommendations retorna recomendaciones basadas en el progreso
func (p *Progress) GetRecommendations() []string {
	recommendations := []string{}

	// Si está atrasado
	if !p.IsOnTrack() {
		recommendations = append(recommendations, "⚠️  Estás atrasado. Considera aumentar recursos o priorizar tasks críticas.")
	}

	// Si hay muchos errores
	if p.SuccessRate < 0.7 {
		recommendations = append(recommendations, "⚠️  Alta tasa de fallos. Revisa la calidad de las estimaciones.")
	}

	// Si hay bloqueadores
	if len(p.Blockers) > 0 {
		recommendations = append(recommendations, "🔴 Hay bloqueadores activos. Resuelve estos primero.")
	}

	// Si es muy lento
	if p.BurndownRate < 0.5 {
		recommendations = append(recommendations, "📉 Velocidad muy baja. ¿Necesitas ayuda o más contexto?")
	}

	// Si está bien
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "✅ El proyecto está en buen camino.")
	}

	return recommendations
}
