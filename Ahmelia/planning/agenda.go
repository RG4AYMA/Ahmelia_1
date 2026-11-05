package planning

import (
	"fmt"
	"time"
)

// MentalState representa el estado mental actual de Ahmelia sobre sus objetivos
type MentalState struct {
	CurrentFocus    *Goal                 // En qué objetivo está enfocada
	PendingGoals    []*Goal               // Objetivos por empezar
	ActiveGoals     []*Goal               // Objetivos en progreso
	CompletedGoals  []*Goal               // Objetivos logrados
	BlockedGoals    []*Goal               // Objetivos bloqueados
	
	LastUpdateTime  time.Time
	Confidence      float64               // Confianza general (0.0-1.0)
	MentalCapacity  float64               // Carga mental (0.0-1.0)
	
	Preoccupations  []string              // Cosas que la preocupan
	Observations    []string              // Cosas que ha notado
	Hypotheses      []string              // Cosas que hipotesiza
}

// AgendaManager gestiona la agenda mental de Ahmelia
type AgendaManager struct {
	mentality       *MentalState
	goalMap         map[string]*Goal
	activeScheduler *Scheduler
}

// NewAgendaManager crea un nuevo gestor de agenda
func NewAgendaManager() *AgendaManager {
	return &AgendaManager{
		mentality: &MentalState{
			PendingGoals:   []*Goal{},
			ActiveGoals:    []*Goal{},
			CompletedGoals: []*Goal{},
			BlockedGoals:   []*Goal{},
			Confidence:     0.7,
			MentalCapacity: 0.8,
			Preoccupations: []string{},
			Observations:   []string{},
			Hypotheses:     []string{},
			LastUpdateTime: time.Now(),
		},
		goalMap:         make(map[string]*Goal),
		activeScheduler: NewScheduler(),
	}
}

// RegisterGoal registra un nuevo objetivo en la agenda mental
func (am *AgendaManager) RegisterGoal(goal *Goal) {
	am.goalMap[goal.ID] = goal

	if goal.Owner == "ahmelia" {
		// Si es propuesto por Ahmelia, expresar conciencia
		am.mentality.Observations = append(
			am.mentality.Observations,
			fmt.Sprintf("He detectado que debería: %s", goal.Name),
		)
	}

	// Categorizar
	switch goal.Status {
	case GoalActive:
		am.mentality.ActiveGoals = append(am.mentality.ActiveGoals, goal)
	case GoalCompleted:
		am.mentality.CompletedGoals = append(am.mentality.CompletedGoals, goal)
	case GoalBlocked:
		am.mentality.BlockedGoals = append(am.mentality.BlockedGoals, goal)
	default:
		am.mentality.PendingGoals = append(am.mentality.PendingGoals, goal)
	}

	// Actualizar capacidad mental
	am.updateMentalCapacity()
}

// SetCurrentFocus establece el objetivo en el que Ahmelia está enfocada
func (am *AgendaManager) SetCurrentFocus(goalID string) error {
	if goal, ok := am.goalMap[goalID]; ok {
		am.mentality.CurrentFocus = goal
		am.mentality.Observations = append(
			am.mentality.Observations,
			fmt.Sprintf("Ahora me enfoco en: %s", goal.Name),
		)
		return nil
	}
	return fmt.Errorf("goal not found")
}

// GetMentalState retorna el estado mental actual
func (am *AgendaManager) GetMentalState() *MentalState {
	return am.mentality
}

// updateMentalCapacity recalcula la capacidad mental
func (am *AgendaManager) updateMentalCapacity() {
	// Capacidad afectada por:
	// - Número de objetivos activos
	// - Número de bloqueadores
	// - Complejidad general

	activeCount := len(am.mentality.ActiveGoals)
	blockedCount := len(am.mentality.BlockedGoals)

	// Fórmula: 1.0 - (activeCount * 0.15) - (blockedCount * 0.25)
	capacity := 1.0 - (float64(activeCount) * 0.15) - (float64(blockedCount) * 0.25)

	if capacity < 0.0 {
		capacity = 0.0
	}
	if capacity > 1.0 {
		capacity = 1.0
	}

	am.mentality.MentalCapacity = capacity

	if am.mentality.MentalCapacity < 0.3 {
		am.mentality.Preoccupations = append(
			am.mentality.Preoccupations,
			"Estoy sobrecargada mentalmente",
		)
	}
}

// AddPreoccupation añade una preocupación
func (am *AgendaManager) AddPreoccupation(concern string) {
	am.mentality.Preoccupations = append(am.mentality.Preoccupations, concern)
}

// ResolvePreoccupation elimina una preocupación
func (am *AgendaManager) ResolvePreoccupation(concern string) {
	filtered := []string{}
	for _, c := range am.mentality.Preoccupations {
		if c != concern {
			filtered = append(filtered, c)
		}
	}
	am.mentality.Preoccupations = filtered
}

// AddObservation registra una observación
func (am *AgendaManager) AddObservation(observation string) {
	am.mentality.Observations = append(am.mentality.Observations, observation)
}

// AddHypothesis añade una hipótesis
func (am *AgendaManager) AddHypothesis(hypothesis string) {
	am.mentality.Hypotheses = append(am.mentality.Hypotheses, hypothesis)
}

// GetStatusReport retorna un reporte completo del estado mental
func (am *AgendaManager) GetStatusReport() string {
	report := "📋 ESTADO MENTAL DE AHMELIA:\n\n"

	// Enfoque actual
	if am.mentality.CurrentFocus != nil {
		report += fmt.Sprintf("🎯 Enfoque actual: %s\n", am.mentality.CurrentFocus.Name)
		report += fmt.Sprintf("   Estado: %v (Progreso: %.0f%%)\n", 
			am.mentality.CurrentFocus.Status,
			am.mentality.CurrentFocus.Progress*100,
		)
	} else {
		report += "🎯 Sin enfoque actual\n"
	}

	report += "\n"

	// Objetivos activos
	report += fmt.Sprintf("⚙️  ACTIVOS (%d):\n", len(am.mentality.ActiveGoals))
	for i, goal := range am.mentality.ActiveGoals {
		report += fmt.Sprintf("  %d. %s (%.0f%%)\n", i+1, goal.Name, goal.Progress*100)
	}

	// Objetivos bloqueados
	if len(am.mentality.BlockedGoals) > 0 {
		report += fmt.Sprintf("\n🔴 BLOQUEADOS (%d):\n", len(am.mentality.BlockedGoals))
		for _, goal := range am.mentality.BlockedGoals {
			report += fmt.Sprintf("  • %s\n", goal.Name)
			for _, blocker := range goal.Blockers {
				report += fmt.Sprintf("    └─ %s\n", blocker)
			}
		}
	}

	// Completados
	if len(am.mentality.CompletedGoals) > 0 {
		report += fmt.Sprintf("\n✅ COMPLETADOS (%d):\n", len(am.mentality.CompletedGoals))
		for i, goal := range am.mentality.CompletedGoals {
			if i >= 3 { // Mostrar solo los últimos 3
				report += fmt.Sprintf("  ... y %d más\n", len(am.mentality.CompletedGoals)-3)
				break
			}
			report += fmt.Sprintf("  • %s\n", goal.Name)
		}
	}

	report += "\n"

	// Capacidad mental
	report += fmt.Sprintf("🧠 Capacidad mental: %.0f%%\n", am.mentality.MentalCapacity*100)
	report += fmt.Sprintf("💪 Confianza: %.0f%%\n", am.mentality.Confidence*100)

	// Preocupaciones
	if len(am.mentality.Preoccupations) > 0 {
		report += "\n⚠️  PREOCUPACIONES:\n"
		for _, p := range am.mentality.Preoccupations {
			report += fmt.Sprintf("  • %s\n", p)
		}
	}

	// Observaciones recientes
	if len(am.mentality.Observations) > 0 {
		report += "\n💭 HE NOTADO:\n"
		// Mostrar las últimas 3
		start := len(am.mentality.Observations) - 3
		if start < 0 {
			start = 0
		}
		for i := start; i < len(am.mentality.Observations); i++ {
			report += fmt.Sprintf("  • %s\n", am.mentality.Observations[i])
		}
	}

	// Hipótesis
	if len(am.mentality.Hypotheses) > 0 {
		report += "\n🔍 HIPÓTESIS:\n"
		// Mostrar las últimas 2
		start := len(am.mentality.Hypotheses) - 2
		if start < 0 {
			start = 0
		}
		for i := start; i < len(am.mentality.Hypotheses); i++ {
			report += fmt.Sprintf("  • %s\n", am.mentality.Hypotheses[i])
		}
	}

	report += "\n"

	return report
}

// GetQuickStatus retorna un estado rápido (1-2 líneas)
func (am *AgendaManager) GetQuickStatus() string {
	if am.mentality.CurrentFocus == nil {
		return fmt.Sprintf("Sin enfoque. Tengo %d objetivos pendientes.", len(am.mentality.PendingGoals))
	}

	return fmt.Sprintf(
		"Enfocada en: %s (%.0f%% completado). %d activos, %d bloqueados.",
		am.mentality.CurrentFocus.Name,
		am.mentality.CurrentFocus.Progress*100,
		len(am.mentality.ActiveGoals),
		len(am.mentality.BlockedGoals),
	)
}

// ShouldProposedNewGoal decide si Ahmelia debería proponer un nuevo objetivo
func (am *AgendaManager) ShouldProposedNewGoal() bool {
	// Si tiene capacidad mental y ha completado algo recientemente
	return am.mentality.MentalCapacity > 0.5 && len(am.mentality.CompletedGoals) > 0
}

// UpdateConfidence actualiza la confianza general basada en éxito
func (am *AgendaManager) UpdateConfidence(successRate float64) {
	// Ajustar confianza basado en tasa de éxito
	if successRate > 0.8 {
		am.mentality.Confidence = min(1.0, am.mentality.Confidence+0.05)
	} else if successRate < 0.5 {
		am.mentality.Confidence = max(0.2, am.mentality.Confidence-0.1)
	}
}

// helpers
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
