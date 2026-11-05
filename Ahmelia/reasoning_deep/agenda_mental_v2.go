package reasoning_deep

import (
	"fmt"
	"time"
)

// CognitiveState es el estado cognitivo profundo
type CognitiveState struct {
	// Enfoque actual
	CurrentFocus string
	SubFocus     string

	// Estado emocional/cognitivo
	Confidence  float64 // 0.0-1.0
	Curiosity   float64 // Cuánta curiosidad tiene
	Uncertainty float64 // Cuánta incertidumbre tiene
	Frustration float64 // Cuánta frustración acumula

	// Lo que piensa
	ActiveThoughts []string // En qué está pensando ahora
	Wondering      []string // Cosas que se pregunta
	Doubts         []string // Dudas específicas
	Hypotheses     []string // Hipótesis propias

	// Lo que observa
	Observations []Observation // Lo que ha notado
	Patterns     []string      // Patrones detectados
	Anomalies    []string      // Cosas que no encajan

	// Lo que le importa
	Preoccupations []Concern // Lo que la preocupa
	Goals          []string  // Objetivos personales
	Values         []string  // Valores guía

	// Errores y aprendizaje
	RecentErrors []ErrorRecord // Errores recientes
	Lessons      []string      // Lecciones aprendidas

	// Relación con el usuario
	UserUnderstanding float64 // Cuánto entiende al usuario
	UserTrust         float64 // Confianza en lo que el usuario dice

	LastUpdated time.Time
}

// Observation es algo que Ahmelia ha notado
type Observation struct {
	Text       string
	Importance int // 1-5
	Confidence float64
	Timestamp  time.Time
}

// Concern es algo que preocupa
type Concern struct {
	Description string
	Severity    int      // 1-5
	Related     []string // IDs de concerns relacionados
	CreatedAt   time.Time
}

// ErrorRecord es un error aprendido
type ErrorRecord struct {
	What          string
	Why           string
	When          time.Time
	LessonLearned string
}

// MentalAgendaV2 es el sistema mejorado de agenda mental
type MentalAgendaV2 struct {
	state        *CognitiveState
	stateHistory []*CognitiveState // Para análisis de evolución
	maxHistory   int
}

// Snapshot returns a detached copy of the current cognitive state so other
// system layers can reason over live agenda evidence without owning or
// mutating the agenda.
func (ma *MentalAgendaV2) Snapshot() CognitiveState {
	if ma == nil || ma.state == nil {
		return CognitiveState{}
	}
	state := *ma.state
	state.ActiveThoughts = append([]string(nil), ma.state.ActiveThoughts...)
	state.Wondering = append([]string(nil), ma.state.Wondering...)
	state.Doubts = append([]string(nil), ma.state.Doubts...)
	state.Hypotheses = append([]string(nil), ma.state.Hypotheses...)
	state.Observations = append([]Observation(nil), ma.state.Observations...)
	state.Patterns = append([]string(nil), ma.state.Patterns...)
	state.Anomalies = append([]string(nil), ma.state.Anomalies...)
	state.Preoccupations = append([]Concern(nil), ma.state.Preoccupations...)
	for i := range state.Preoccupations {
		state.Preoccupations[i].Related = append([]string(nil), ma.state.Preoccupations[i].Related...)
	}
	state.Goals = append([]string(nil), ma.state.Goals...)
	state.Values = append([]string(nil), ma.state.Values...)
	state.RecentErrors = append([]ErrorRecord(nil), ma.state.RecentErrors...)
	state.Lessons = append([]string(nil), ma.state.Lessons...)
	return state
}

// NewMentalAgendaV2 crea una nueva agenda mental mejorada
func NewMentalAgendaV2() *MentalAgendaV2 {
	return &MentalAgendaV2{
		state: &CognitiveState{
			Confidence:        0.7,
			Curiosity:         0.8,
			Uncertainty:       0.3,
			Frustration:       0.0,
			ActiveThoughts:    []string{},
			Wondering:         []string{},
			Doubts:            []string{},
			Hypotheses:        []string{},
			Observations:      []Observation{},
			Patterns:          []string{},
			Anomalies:         []string{},
			Preoccupations:    []Concern{},
			Goals:             []string{},
			Values:            []string{},
			RecentErrors:      []ErrorRecord{},
			Lessons:           []string{},
			UserUnderstanding: 0.5,
			UserTrust:         0.7,
			LastUpdated:       time.Now(),
		},
		stateHistory: []*CognitiveState{},
		maxHistory:   10,
	}
}

// SetFocus establece el enfoque actual
func (ma *MentalAgendaV2) SetFocus(focus, subfocus string) {
	ma.state.CurrentFocus = focus
	ma.state.SubFocus = subfocus
	ma.state.LastUpdated = time.Now()
	ma.addThought(fmt.Sprintf("Ahora me enfoco en: %s / %s", focus, subfocus))
}

// AddThought añade un pensamiento activo
func (ma *MentalAgendaV2) addThought(thought string) {
	ma.state.ActiveThoughts = append(ma.state.ActiveThoughts, thought)
	if len(ma.state.ActiveThoughts) > 5 {
		// Mantener solo los últimos 5
		ma.state.ActiveThoughts = ma.state.ActiveThoughts[1:]
	}
	ma.state.LastUpdated = time.Now()
}

// Wonder añade algo en lo que está pensando
func (ma *MentalAgendaV2) Wonder(question string) {
	ma.state.Wondering = append(ma.state.Wondering, question)
	ma.state.Curiosity = min(1.0, ma.state.Curiosity+0.05)
	ma.state.LastUpdated = time.Now()
}

// Doubt añade una duda
func (ma *MentalAgendaV2) Doubt(doubt string) {
	ma.state.Doubts = append(ma.state.Doubts, doubt)
	ma.state.Uncertainty = min(1.0, ma.state.Uncertainty+0.1)
	ma.state.LastUpdated = time.Now()
}

// Hypothesize añade una hipótesis
func (ma *MentalAgendaV2) Hypothesize(hypothesis string) {
	ma.state.Hypotheses = append(ma.state.Hypotheses, hypothesis)
	ma.state.LastUpdated = time.Now()
}

// Observe registra una observación
func (ma *MentalAgendaV2) Observe(text string, importance int, confidence float64) {
	obs := Observation{
		Text:       text,
		Importance: importance,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}
	ma.state.Observations = append(ma.state.Observations, obs)
	ma.state.LastUpdated = time.Now()
}

// DetectPattern registra un patrón detectado
func (ma *MentalAgendaV2) DetectPattern(pattern string) {
	ma.state.Patterns = append(ma.state.Patterns, pattern)
	ma.state.LastUpdated = time.Now()
}

// NoticeAnomaly registra algo que no encaja
func (ma *MentalAgendaV2) NoticeAnomaly(anomaly string) {
	ma.state.Anomalies = append(ma.state.Anomalies, anomaly)
	ma.state.Uncertainty = min(1.0, ma.state.Uncertainty+0.15)
	ma.state.LastUpdated = time.Now()
}

// Worry registra una preocupación
func (ma *MentalAgendaV2) Worry(description string, severity int) {
	concern := Concern{
		Description: description,
		Severity:    severity,
		CreatedAt:   time.Now(),
	}
	ma.state.Preoccupations = append(ma.state.Preoccupations, concern)
	ma.state.Frustration = min(1.0, ma.state.Frustration+0.1)
	ma.state.LastUpdated = time.Now()
}

// ResolveWorry elimina una preocupación
func (ma *MentalAgendaV2) ResolveWorry(description string) {
	filtered := []Concern{}
	for _, c := range ma.state.Preoccupations {
		if c.Description != description {
			filtered = append(filtered, c)
		}
	}
	ma.state.Preoccupations = filtered
	ma.state.Frustration = max(0.0, ma.state.Frustration-0.1)
	ma.state.LastUpdated = time.Now()
}

// RecordError registra un error cometido
func (ma *MentalAgendaV2) RecordError(what, why, lesson string) {
	error := ErrorRecord{
		What:          what,
		Why:           why,
		When:          time.Now(),
		LessonLearned: lesson,
	}
	ma.state.RecentErrors = append(ma.state.RecentErrors, error)
	ma.state.Confidence = max(0.2, ma.state.Confidence-0.05)
	ma.LearnLesson(lesson)
	ma.state.LastUpdated = time.Now()
}

// LearnLesson registra algo aprendido
func (ma *MentalAgendaV2) LearnLesson(lesson string) {
	ma.state.Lessons = append(ma.state.Lessons, lesson)
	ma.state.Confidence = min(1.0, ma.state.Confidence+0.03)
	ma.state.LastUpdated = time.Now()
}

// GetMentalReport retorna un reporte completo del estado mental
func (ma *MentalAgendaV2) GetMentalReport() string {
	report := "🧠 ESTADO MENTAL PROFUNDO:\n\n"

	// Enfoque
	if ma.state.CurrentFocus != "" {
		report += fmt.Sprintf("🎯 ENFOQUE: %s / %s\n", ma.state.CurrentFocus, ma.state.SubFocus)
	}

	// Estado cognitivo
	report += fmt.Sprintf("\n💭 ESTADO COGNITIVO:\n")
	report += fmt.Sprintf("  • Confianza: %.0f%%\n", ma.state.Confidence*100)
	report += fmt.Sprintf("  • Curiosidad: %.0f%%\n", ma.state.Curiosity*100)
	report += fmt.Sprintf("  • Incertidumbre: %.0f%%\n", ma.state.Uncertainty*100)
	report += fmt.Sprintf("  • Frustración: %.0f%%\n", ma.state.Frustration*100)

	// Lo que piensa
	if len(ma.state.ActiveThoughts) > 0 {
		report += fmt.Sprintf("\n💡 PENSANDO EN:\n")
		for _, t := range ma.state.ActiveThoughts {
			report += fmt.Sprintf("  • %s\n", t)
		}
	}

	// Dudas
	if len(ma.state.Doubts) > 0 {
		report += fmt.Sprintf("\n❓ DUDAS:\n")
		for _, d := range ma.state.Doubts {
			report += fmt.Sprintf("  • %s\n", d)
		}
	}

	// Preocupaciones
	if len(ma.state.Preoccupations) > 0 {
		report += fmt.Sprintf("\n⚠️  PREOCUPACIONES:\n")
		for _, p := range ma.state.Preoccupations {
			report += fmt.Sprintf("  • %s (severidad: %d/5)\n", p.Description, p.Severity)
		}
	}

	// Patrones y anomalías
	if len(ma.state.Patterns) > 0 {
		report += fmt.Sprintf("\n🔍 PATRONES DETECTADOS:\n")
		for _, p := range ma.state.Patterns {
			report += fmt.Sprintf("  • %s\n", p)
		}
	}

	if len(ma.state.Anomalies) > 0 {
		report += fmt.Sprintf("\n⚡ ANOMALÍAS:\n")
		for _, a := range ma.state.Anomalies {
			report += fmt.Sprintf("  • %s\n", a)
		}
	}

	// Lecciones
	if len(ma.state.Lessons) > 0 {
		report += fmt.Sprintf("\n📚 HE APRENDIDO:\n")
		// Mostrar últimas 3
		start := len(ma.state.Lessons) - 3
		if start < 0 {
			start = 0
		}
		for i := start; i < len(ma.state.Lessons); i++ {
			report += fmt.Sprintf("  • %s\n", ma.state.Lessons[i])
		}
	}

	return report
}

// GetQuickMentalState retorna estado mental en 1-2 líneas
func (ma *MentalAgendaV2) GetQuickMentalState() string {
	return fmt.Sprintf(
		"Enfoque: %s | Confianza: %.0f%% | Dudas: %d | Preocupaciones: %d",
		ma.state.CurrentFocus,
		ma.state.Confidence*100,
		len(ma.state.Doubts),
		len(ma.state.Preoccupations),
	)
}

// SaveState guarda el estado actual para análisis histórico
func (ma *MentalAgendaV2) SaveState() {
	// Copiar estado
	stateCopy := *ma.state
	ma.stateHistory = append(ma.stateHistory, &stateCopy)

	if len(ma.stateHistory) > ma.maxHistory {
		ma.stateHistory = ma.stateHistory[1:]
	}
}

// GetMentalEvolution retorna cómo ha cambiado el estado mental
func (ma *MentalAgendaV2) GetMentalEvolution() string {
	if len(ma.stateHistory) < 2 {
		return "Sin datos históricos suficientes"
	}

	oldest := ma.stateHistory[0]
	newest := ma.stateHistory[len(ma.stateHistory)-1]

	evolution := fmt.Sprintf("EVOLUCIÓN MENTAL:\n\n")
	evolution += fmt.Sprintf("Confianza: %.0f%% → %.0f%% %s\n",
		oldest.Confidence*100, newest.Confidence*100,
		trend(oldest.Confidence, newest.Confidence))
	evolution += fmt.Sprintf("Curiosidad: %.0f%% → %.0f%% %s\n",
		oldest.Curiosity*100, newest.Curiosity*100,
		trend(oldest.Curiosity, newest.Curiosity))
	evolution += fmt.Sprintf("Frustración: %.0f%% → %.0f%% %s\n",
		oldest.Frustration*100, newest.Frustration*100,
		trend(oldest.Frustration, newest.Frustration))

	return evolution
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

func trend(old, new float64) string {
	if new > old+0.05 {
		return "📈 aumentando"
	}
	if new < old-0.05 {
		return "📉 disminuyendo"
	}
	return "➡️  estable"
}
