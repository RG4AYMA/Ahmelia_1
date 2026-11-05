package comprehension

import (
	"strings"
)

// IntentType define los 5 tipos de intención
type IntentType string

const (
	IntentCommand     IntentType = "command"     // Quiere que Ahmelia haga algo
	IntentQuestion    IntentType = "question"    // Pregunta algo
	IntentLearning    IntentType = "learning"    // Está enseñando algo a Ahmelia
	IntentFeedback    IntentType = "feedback"    // Da feedback sobre una respuesta
	IntentCorrection  IntentType = "correction"  // Corrige algo que Ahmelia hizo
	IntentUnknown     IntentType = "unknown"     // No se pudo determinar
)

// Intent representa la intención detectada
type Intent struct {
	Type       IntentType
	Confidence float64 // 0.0 - 1.0
	Indicators []string // Palabras que llevaron a esta conclusión
}

// intentPatterns define patrones para cada tipo de intención
var intentPatterns = map[IntentType][]string{
	IntentCommand: {
		"abre", "abrir", "ejecuta", "lanza", "inicia",
		"busca", "encuentra", "mueve", "cierra", "minimiza",
		"sube", "baja", "silencia", "reproduce", "pon",
		"cambia", "crea", "borra", "elimina", "escribe",
	},
	IntentQuestion: {
		"que", "qué", "cual", "cuál", "como", "cómo",
		"donde", "dónde", "cuando", "cuándo", "por que", "por qué",
		"quien", "quién", "cuanto", "cuánto", "sabes", "puedes",
		"tienes", "existe", "hay", "es", "son",
	},
	IntentLearning: {
		"aprende", "recuerda", "guarda", "cuando diga", "si digo",
		"esto significa", "esto es", "deberías", "tienes que",
		"a partir de ahora", "en el futuro", "memoriza",
	},
	IntentFeedback: {
		"bien", "mal", "perfecto", "incorrecto", "correcto",
		"gracias", "eso está bien", "eso está mal", "no era eso",
		"exacto", "equivocado", "fallaste", "acertaste",
	},
	IntentCorrection: {
		"no", "no es", "es al revés", "corrige", "corrígete",
		"me equivoqué", "te equivocaste", "olvídalo", "cancela",
		"no lo hagas", "para", "detente",
	},
}

// DetectIntent analiza el input y retorna la intención más probable
func DetectIntent(input string) Intent {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return Intent{Type: IntentUnknown, Confidence: 0}
	}

	scores := make(map[IntentType]float64)
	matches := make(map[IntentType][]string)

	for intentType, patterns := range intentPatterns {
		for _, pattern := range patterns {
			if strings.Contains(normalized, pattern) {
				scores[intentType]++
				matches[intentType] = append(matches[intentType], pattern)
			}
		}
	}

	// Encontrar la intención con mayor score
	bestType := IntentUnknown
	bestScore := 0.0

	for intentType, score := range scores {
		if score > bestScore {
			bestScore = score
			bestType = intentType
		}
	}

	if bestType == IntentUnknown || bestScore == 0 {
		return Intent{Type: IntentUnknown, Confidence: 0}
	}

	// Normalizar confianza: más matches = más confianza, tope en 1.0
	// Base: primer match = 0.6, cada match adicional suma 0.1, máx 1.0
	confidence := 0.6 + (bestScore-1)*0.1
	if confidence > 1.0 {
		confidence = 1.0
	}

	return Intent{
		Type:       bestType,
		Confidence: confidence,
		Indicators: matches[bestType],
	}
}

// IsCommand es un helper para verificar rápido si es un comando
func IsCommand(input string) bool {
	intent := DetectIntent(input)
	return intent.Type == IntentCommand && intent.Confidence >= 0.6
}

// IsCorrection verifica si el usuario está cancelando o corrigiendo
func IsCorrection(input string) bool {
	intent := DetectIntent(input)
	return intent.Type == IntentCorrection
}

// IsQuestion verifica si es una pregunta
func IsQuestion(input string) bool {
	intent := DetectIntent(input)
	return intent.Type == IntentQuestion
}

// NeedsLearning verifica si el usuario está intentando enseñar algo
func NeedsLearning(input string) bool {
	intent := DetectIntent(input)
	return intent.Type == IntentLearning
}

// String retorna representación legible del intent
func (i Intent) String() string {
	return string(i.Type)
}

// DetectIntentWithContext usa el tema activo para mejorar la detección
// Por ejemplo, en tema "sistema_audio" "sube" es un comando, no ambiguo
func DetectIntentWithContext(input, activeTopic string) Intent {
	intent := DetectIntent(input)

	// Si ya tiene alta confianza, el contexto no cambia nada
	if intent.Confidence >= 0.8 {
		return intent
	}

	normalized := strings.ToLower(strings.TrimSpace(input))

	// Ajustar según tema activo
	switch activeTopic {
	case "sistema_audio":
		audioCommands := []string{"sube", "baja", "mas", "menos", "silencio", "mute"}
		for _, cmd := range audioCommands {
			if strings.Contains(normalized, cmd) {
				return Intent{
					Type:       IntentCommand,
					Confidence: 0.85,
					Indicators: []string{cmd, "contexto:sistema_audio"},
				}
			}
		}

	case "navegacion_ventanas":
		winCommands := []string{"otra", "siguiente", "cambia", "vuelve"}
		for _, cmd := range winCommands {
			if strings.Contains(normalized, cmd) {
				return Intent{
					Type:       IntentCommand,
					Confidence: 0.85,
					Indicators: []string{cmd, "contexto:navegacion_ventanas"},
				}
			}
		}

	case "archivos":
		fileCommands := []string{"abre", "mueve", "renombra", "borra"}
		for _, cmd := range fileCommands {
			if strings.Contains(normalized, cmd) {
				return Intent{
					Type:       IntentCommand,
					Confidence: 0.85,
					Indicators: []string{cmd, "contexto:archivos"},
				}
			}
		}
	}

	return intent
}
