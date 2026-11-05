package planning

import (
	"fmt"
	"time"
)

// Review representa una evaluación de un plan completado
type Review struct {
	ID              string
	GoalID          string
	PlanID          string
	ReviewedAt      time.Time
	
	// Evaluación básica
	WasPlanAccurate bool         // ¿Se ajustó a las estimaciones?
	WasSuccessful   bool         // ¿Se logró el objetivo?
	LessonsLearned  []string
	
	// Análisis de tiempo
	EstimatedTime   time.Duration
	ActualTime      time.Duration
	Variance        float64      // % de diferencia
	
	// Análisis de tareas
	TasksCompleted  int
	TasksFailed     int
	TasksBlocked    int
	SuccessRate     float64
	
	// Calidad
	ErrorsFound     int
	FatalErrors     int
	QualityScore    float64      // 0.0 - 1.0
	
	// Reflexión
	WhatWentWell    []string
	WhatWentWrong   []string
	ShouldRepeat    bool         // ¿Repetir este proceso?
	Improvements    []string
}

// Reviewer realiza evaluaciones de planes
type Reviewer struct {
	reviews map[string]*Review
}

// NewReviewer crea un nuevo revisor
func NewReviewer() *Reviewer {
	return &Reviewer{
		reviews: make(map[string]*Review),
	}
}

// ReviewGoal evalúa un objetivo completado
func (r *Reviewer) ReviewGoal(goal *Goal, plan *Plan, progress *Progress) *Review {
	review := &Review{
		ID:              fmt.Sprintf("review-%d", time.Now().Unix()),
		GoalID:          goal.ID,
		PlanID:          plan.ID,
		ReviewedAt:      time.Now(),
		EstimatedTime:   plan.TotalEstimate,
		ActualTime:      progress.TimeElapsed,
		TasksCompleted:  progress.TasksCompleted,
		TasksFailed:     progress.TasksFailed,
		SuccessRate:     progress.SuccessRate,
		LessonsLearned:  []string{},
		WhatWentWell:    []string{},
		WhatWentWrong:   []string{},
		Improvements:    []string{},
	}

	// Análisis de tiempo
	review.analyzeTime()

	// Análisis de tareas
	review.analyzeTasks()

	// Análisis de calidad
	review.analyzeQuality(progress)

	// Reflexión
	review.reflect(goal, progress)

	r.reviews[review.ID] = review
	return review
}

// analyzeTime analiza si se ajustó a las estimaciones de tiempo
func (r *Review) analyzeTime() {
	if r.EstimatedTime == 0 {
		return
	}

	hours := r.ActualTime.Hours()
	estimatedHours := r.EstimatedTime.Hours()

	r.Variance = ((hours - estimatedHours) / estimatedHours) * 100

	// Decidir si fue preciso
	if r.Variance < 20 && r.Variance > -20 {
		r.WasPlanAccurate = true
		r.WhatWentWell = append(r.WhatWentWell, fmt.Sprintf("Las estimaciones fueron precisas (%.0f%% de varianza)", r.Variance))
	} else if r.Variance < 0 {
		r.WhatWentWell = append(r.WhatWentWell, fmt.Sprintf("Completamos más rápido de lo esperado (%.0f%% más rápido)", -r.Variance))
	} else {
		r.WhatWentWrong = append(r.WhatWentWrong, fmt.Sprintf("Tardamos más (%.0f%% más lento)", r.Variance))
		r.Improvements = append(r.Improvements, "Mejorar estimaciones de tiempo")
	}
}

// analyzeTasks analiza el desempeño con las tareas
func (r *Review) analyzeTasks() {
	total := r.TasksCompleted + r.TasksFailed
	if total == 0 {
		return
	}

	r.WasSuccessful = r.TasksFailed == 0 || (r.SuccessRate >= 0.8)

	if r.SuccessRate == 1.0 {
		r.WhatWentWell = append(r.WhatWentWell, "100% de tareas completadas exitosamente")
	} else if r.SuccessRate >= 0.8 {
		r.WhatWentWell = append(r.WhatWentWell, fmt.Sprintf("Alta tasa de éxito (%.0f%%)", r.SuccessRate*100))
	} else if r.SuccessRate >= 0.6 {
		r.WhatWentWrong = append(r.WhatWentWrong, fmt.Sprintf("Tasa de éxito moderada (%.0f%%)", r.SuccessRate*100))
		r.Improvements = append(r.Improvements, "Revisar por qué algunas tareas fallan")
	} else {
		r.WhatWentWrong = append(r.WhatWentWrong, fmt.Sprintf("Baja tasa de éxito (%.0f%%)", r.SuccessRate*100))
	}
}

// analyzeQuality analiza la calidad del trabajo
func (r *Review) analyzeQuality(progress *Progress) {
	r.ErrorsFound = progress.ErrorCount

	// Calcular score de calidad
	// Basado en: ausencia de errores, tasa de éxito, tiempo
	r.QualityScore = progress.EfficiencyScore
	r.FatalErrors = progress.TasksFailed

	if r.QualityScore >= 0.8 {
		r.WhatWentWell = append(r.WhatWentWell, fmt.Sprintf("Calidad excepcional (score: %.0f)", r.QualityScore*100))
	} else if r.QualityScore >= 0.6 {
		r.Improvements = append(r.Improvements, "Mejorar consistencia y reducir errores")
	}
}

// reflect genera reflexión profunda
func (r *Review) reflect(goal *Goal, progress *Progress) {
	// ¿Debería repetirse el proceso?
	r.ShouldRepeat = r.WasPlanAccurate && r.WasSuccessful

	// Lecciones aprendidas
	if r.Variance > 0 {
		r.LessonsLearned = append(r.LessonsLearned, "Las estimaciones tienden a ser optimistas")
	}

	if r.SuccessRate < 1.0 {
		r.LessonsLearned = append(r.LessonsLearned, "Hay dependencias no anticipadas")
	}

	if len(progress.Blockers) > 0 {
		r.LessonsLearned = append(r.LessonsLearned, "Se encontraron más bloqueadores de los esperado")
	}

	// Mejoras concretas
	if !r.WasPlanAccurate {
		r.Improvements = append(r.Improvements, "Ajustar metodología de estimación")
	}

	if !r.WasSuccessful {
		r.Improvements = append(r.Improvements, "Identificar y resolver causas raíz de fallos")
	}
}

// GetSummary retorna un resumen de la revisión
func (r *Review) GetSummary() string {
	summary := fmt.Sprintf("REVISIÓN DEL OBJETIVO:\n\n")

	// Resultado general
	if r.WasSuccessful {
		summary += "✅ OBJETIVO LOGRADO\n\n"
	} else {
		summary += "❌ OBJETIVO NO COMPLETAMENTE LOGRADO\n\n"
	}

	// Datos clave
	summary += fmt.Sprintf("Tiempo estimado: %.1f horas\n", r.EstimatedTime.Hours())
	summary += fmt.Sprintf("Tiempo real: %.1f horas\n", r.ActualTime.Hours())
	summary += fmt.Sprintf("Varianza: %.0f%%\n", r.Variance)
	summary += fmt.Sprintf("Tasa de éxito: %.0f%%\n", r.SuccessRate*100)
	summary += fmt.Sprintf("Calidad: %.0f%%\n\n", r.QualityScore*100)

	// Qué fue bien
	if len(r.WhatWentWell) > 0 {
		summary += "✅ QUÉ FUNCIONÓ:\n"
		for _, item := range r.WhatWentWell {
			summary += fmt.Sprintf("  • %s\n", item)
		}
		summary += "\n"
	}

	// Qué fue mal
	if len(r.WhatWentWrong) > 0 {
		summary += "❌ QUÉ NO FUNCIONÓ:\n"
		for _, item := range r.WhatWentWrong {
			summary += fmt.Sprintf("  • %s\n", item)
		}
		summary += "\n"
	}

	// Lecciones
	if len(r.LessonsLearned) > 0 {
		summary += "📚 LECCIONES APRENDIDAS:\n"
		for _, lesson := range r.LessonsLearned {
			summary += fmt.Sprintf("  • %s\n", lesson)
		}
		summary += "\n"
	}

	// Mejoras
	if len(r.Improvements) > 0 {
		summary += "🚀 MEJORAS PARA PRÓXIMAS VECES:\n"
		for _, imp := range r.Improvements {
			summary += fmt.Sprintf("  • %s\n", imp)
		}
		summary += "\n"
	}

	// Recomendación
	if r.ShouldRepeat {
		summary += "✅ Este proceso funcionó bien y puede repetirse.\n"
	} else {
		summary += "⚠️  Revisar y ajustar la metodología antes de repetir.\n"
	}

	return summary
}

// GetMetrics retorna métricas detalladas
func (r *Review) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"estimated_hours":  r.EstimatedTime.Hours(),
		"actual_hours":     r.ActualTime.Hours(),
		"time_variance":    r.Variance,
		"success_rate":     r.SuccessRate,
		"quality_score":    r.QualityScore,
		"tasks_completed":  r.TasksCompleted,
		"tasks_failed":     r.TasksFailed,
		"errors_found":     r.ErrorsFound,
		"was_successful":   r.WasSuccessful,
		"plan_accurate":    r.WasPlanAccurate,
		"should_repeat":    r.ShouldRepeat,
	}
}
