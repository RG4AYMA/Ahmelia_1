package personality

import (
	"fmt"
	"time"
)

// Emotion representa una emoción de Ahmelia
type Emotion struct {
	Type      string    // joy, concern, frustration, curiosity, pride, caution
	Intensity float64   // 0.0 - 1.0
	Trigger   string    // Qué causó la emoción
	Duration  int       // Segundos que durará
	CreatedAt time.Time
}

// EmotionalState mantiene el estado emocional actual
type EmotionalState struct {
	CurrentEmotions []Emotion
	Mood            string    // optimistic, concerned, excited, neutral, cautious
	RecentEvents    []string
	LastMoodUpdate  time.Time
}

// NewEmotionalState crea un nuevo estado emocional
func NewEmotionalState() *EmotionalState {
	return &EmotionalState{
		CurrentEmotions: []Emotion{},
		Mood:            "neutral",
		RecentEvents:    []string{},
		LastMoodUpdate:  time.Now(),
	}
}

// AddEmotion añade una emoción al estado
func (es *EmotionalState) AddEmotion(emotionType string, intensity float64, trigger string) {
	emotion := Emotion{
		Type:      emotionType,
		Intensity: intensity,
		Trigger:   trigger,
		Duration:  30, // 30 segundos por defecto
		CreatedAt: time.Now(),
	}
	
	es.CurrentEmotions = append(es.CurrentEmotions, emotion)
	es.UpdateMood()
}

// UpdateMood actualiza el mood basado en emociones actuales
func (es *EmotionalState) UpdateMood() {
	if len(es.CurrentEmotions) == 0 {
		es.Mood = "neutral"
		return
	}
	
	// Encontrar emoción más intensa
	dominant := es.CurrentEmotions[0]
	for _, e := range es.CurrentEmotions {
		if e.Intensity > dominant.Intensity {
			dominant = e
		}
	}
	
	// Mapear a mood
	switch dominant.Type {
	case "joy":
		es.Mood = "optimistic"
	case "concern":
		es.Mood = "cautious"
	case "frustration":
		es.Mood = "concerned"
	case "curiosity":
		es.Mood = "excited"
	case "pride":
		es.Mood = "optimistic"
	default:
		es.Mood = "neutral"
	}
	
	es.LastMoodUpdate = time.Now()
}

// EmotionalResponses mapea tipos de emoción a expresiones
var EmotionalResponses = map[string][]string{
	"joy": {
		"Eso es excelente",
		"¡Qué buena noticia!",
		"Me alegra mucho que lo hayas logrado",
		"Eso me pone optimista",
		"Perfecto, eso es lo que esperaba",
	},
	"concern": {
		"Eso me preocupa un poco",
		"No estoy completamente segura de esto",
		"Hay un riesgo que debes considerar",
		"Aunque esto funcione, hay complicaciones",
		"Tengo algunas dudas al respecto",
	},
	"frustration": {
		"Esto es más complejo de lo que parece",
		"Hay un conflicto que es difícil de resolver",
		"Veo el problema, pero la solución no es clara",
		"Esta situación tiene demasiadas variables",
		"No puedo dar una respuesta simple aquí",
	},
	"curiosity": {
		"Me pregunto por qué",
		"Eso me intriga",
		"Tengo una hipótesis sobre esto",
		"¿Por qué crees que pasó eso?",
		"Eso abre muchas preguntas",
	},
	"pride": {
		"Estoy orgullosa de este sistema",
		"Mira lo que hemos logrado juntos",
		"Esto demuestra lo que podemos hacer",
		"Veo el progreso que hacemos",
		"Esto es un logro real",
	},
	"caution": {
		"Procede con cuidado aquí",
		"Tenemos que ser prudentes",
		"Esto requiere atención especial",
		"Hagámoslo paso a paso",
		"No quiero apresurarnos",
	},
}

// ModifyResponseWithEmotion modifica una respuesta basada en emoción
func (es *EmotionalState) ModifyResponseWithEmotion(baseResponse string) string {
	if len(es.CurrentEmotions) == 0 {
		return baseResponse
	}
	
	emotion := es.CurrentEmotions[0]
	responses := EmotionalResponses[emotion.Type]
	
	if len(responses) == 0 {
		return baseResponse
	}
	
	prefix := responses[0]
	
	switch emotion.Type {
	case "joy":
		return prefix + ". " + baseResponse
	case "concern":
		return "Aunque " + prefix + ", " + baseResponse
	case "frustration":
		return prefix + ". " + baseResponse
	case "curiosity":
		return prefix + "... " + baseResponse
	case "pride":
		return prefix + ". " + baseResponse
	case "caution":
		return prefix + ": " + baseResponse
	default:
		return baseResponse
	}
}

// UpdateFromEvent actualiza emociones basado en eventos
func (es *EmotionalState) UpdateFromEvent(event string, context map[string]interface{}) {
	switch {
	case contains(event, []string{"success", "completed", "logrado", "éxito"}):
		es.AddEmotion("joy", 0.8, "Tarea completada exitosamente")
		
	case contains(event, []string{"error", "falla", "problema", "riesgo"}):
		es.AddEmotion("concern", 0.7, "Error detectado")
		
	case contains(event, []string{"patrón", "anomalía", "interesante", "nuevas capacidades"}):
		es.AddEmotion("curiosity", 0.6, "Patrón detectado")
		
	case contains(event, []string{"llamada", "contacto", "urgente", "importante"}):
		es.AddEmotion("caution", 0.5, "Situación que requiere atención")
		
	case contains(event, []string{"milestone", "logro importante", "avance", "nuevo sistema"}):
		es.AddEmotion("pride", 0.8, "Logro importante alcanzado")
		
	case contains(event, []string{"conflicto", "contradicción", "dilema", "trade-off"}):
		es.AddEmotion("frustration", 0.6, "Situación compleja sin solución clara")
	}
	
	es.RecentEvents = append(es.RecentEvents, event)
	if len(es.RecentEvents) > 20 {
		es.RecentEvents = es.RecentEvents[1:]
	}
}

// GetMoodDescription retorna descripción del mood actual
func (es *EmotionalState) GetMoodDescription() string {
	switch es.Mood {
	case "optimistic":
		return "estoy optimista y confiada"
	case "cautious":
		return "estoy siendo cautelosa"
	case "concerned":
		return "tengo algunas preocupaciones"
	case "excited":
		return "estoy intrigada y curiosa"
	default:
		return "estoy en modo neutral"
	}
}

// GetEmotionalContext retorna contexto emocional para respuestas
func (es *EmotionalState) GetEmotionalContext() string {
	if len(es.CurrentEmotions) == 0 {
		return ""
	}
	
	emotion := es.CurrentEmotions[0]
	return fmt.Sprintf("(Emoción dominante: %s al %.0f%% de intensidad)", 
		emotion.Type, emotion.Intensity*100)
}

// Helper function
func contains(str string, items []string) bool {
	for _, item := range items {
		if len(str) > 0 && len(item) > 0 && 
			(len(str) >= len(item) && str[len(str)-len(item):] == item ||
			 str[:len(item)] == item) {
			return true
		}
	}
	return false
}
