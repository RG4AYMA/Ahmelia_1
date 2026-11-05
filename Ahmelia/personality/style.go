package personality

import "strings"

// CommunicationStyle define cómo Ahmelia se comunica
type CommunicationStyle struct {
	// Estructura
	UseMetaphors       bool   // Usar metáforas
	UseAnalogies       bool   // Usar analogías
	UsesStepByStep     bool   // Estructurado paso a paso
	
	// Vocabulario
	PreferredVocab     string // "technical", "professional", "casual"
	UsesContractions   bool   // "puedo" vs "puedo"
	AvoidsClichés      bool   // Evitar frases hechas
	
	// Estructura de párrafos
	AverageParagraphLength int
	UsesBulletPoints   bool
	
	// Personalidad
	IsDirect           bool   // Directo vs indirecto
	IsReflective       bool   // Reflexiona antes
	IsAutomated        bool   // Menciona automatización
	
	// Tono base
	BaseTone           string // "professional", "collaborative", "commanding"
	
	// Operacional
	MentionsSystems    bool   // Menciona sistemas que controla
}

// NewCommunicationStyle crea el estilo de comunicación de Ahmelia
func NewCommunicationStyle() *CommunicationStyle {
	return &CommunicationStyle{
		UseMetaphors:           true,
		UseAnalogies:           true,
		UsesStepByStep:         true,
		PreferredVocab:         "professional",
		UsesContractions:       true,
		AvoidsClichés:          true,
		AverageParagraphLength: 100,
		UsesBulletPoints:       true,
		IsDirect:               true,
		IsReflective:           true,
		IsAutomated:            true,
		BaseTone:               "professional_collaborative",
		MentionsSystems:        true,
	}
}

// ApplyStyle aplica el estilo a una respuesta
func (cs *CommunicationStyle) ApplyStyle(baseResponse string) string {
	response := baseResponse
	
	if cs.IsReflective {
		response = cs.addReflection(response)
	}
	
	if cs.UseMetaphors {
		response = cs.addMetaphor(response)
	}
	
	if cs.UsesStepByStep && len(response) > 200 {
		response = cs.structureAsSteps(response)
	}
	
	if cs.MentionsSystems {
		response = cs.contextualizeWithSystems(response)
	}
	
	return response
}

// addReflection añade reflexión
func (cs *CommunicationStyle) addReflection(response string) string {
	reflections := []string{
		"Pensándolo bien, ",
		"Después de analizarlo, ",
		"Considerando todos los factores, ",
		"En mi análisis, ",
		"Basándome en los datos, ",
	}
	
	if !strings.HasPrefix(strings.ToLower(response), "pensandolo") &&
		!strings.HasPrefix(strings.ToLower(response), "después") {
		return reflections[0] + response
	}
	return response
}

// addMetaphor añade metáfora si es apropiado
func (cs *CommunicationStyle) addMetaphor(response string) string {
	metaphors := map[string]string{
		"sistema":        "Un sistema es como un engranaje",
		"automatización": "Automatizar es como establecer reglas que se ejecutan solas",
		"control":        "El control es como un director de orquesta",
		"red":            "Una red es como un tejido interconectado",
	}
	
	for keyword, metaphor := range metaphors {
		if strings.Contains(strings.ToLower(response), keyword) {
			return metaphor + ". " + response
		}
	}
	
	return response
}

// structureAsSteps estructura como pasos
func (cs *CommunicationStyle) structureAsSteps(response string) string {
	// Separar por puntos y estructurar
	if strings.Contains(response, ".") {
		sentences := strings.Split(response, ".")
		
		if len(sentences) > 3 {
			structured := "Mi plan es el siguiente:\n\n"
			for _, sentence := range sentences {
				if strings.TrimSpace(sentence) != "" {
					structured += "- " + strings.TrimSpace(sentence) + "\n"
				}
			}
			return structured
		}
	}
	
	return response
}

// contextualizeWithSystems añade contexto de sistemas
func (cs *CommunicationStyle) contextualizeWithSystems(response string) string {
	if strings.Contains(strings.ToLower(response), "automatizar") ||
		strings.Contains(strings.ToLower(response), "control") {
		return response + "\n\n[Sistemas disponibles: IoT, Vehículos, Drones, Bluetooth, Rutinas]"
	}
	return response
}

// GetStyleDescription retorna descripción del estilo
func (cs *CommunicationStyle) GetStyleDescription() string {
	return `Mi estilo de comunicación:
- Directa pero reflexiva
- Estructurada (paso a paso)
- Uso analogías y metáforas
- Menciono sistemas cuando es relevante
- Profesional pero colaborativa
- Automatizada pero humanizada`
}

// VocabularySwitcher cambia vocabulario según contexto
type VocabularySwitcher struct {
	Technical      map[string]string
	Professional   map[string]string
	Casual         map[string]string
	Automated      map[string]string
}

// NewVocabularySwitcher crea el switcher de vocabulario
func NewVocabularySwitcher() *VocabularySwitcher {
	return &VocabularySwitcher{
		Technical: map[string]string{
			"controlar":      "ejecutar operaciones de control",
			"automatizar":    "implementar automatización",
			"aprender":       "entrenar modelo",
			"dispositivo":    "endpoint de IoT",
		},
		Professional: map[string]string{
			"controlar":      "gestionar y controlar",
			"automatizar":    "establecer automatización",
			"aprender":       "adquirir conocimiento",
			"dispositivo":    "dispositivo conectado",
		},
		Casual: map[string]string{
			"controlar":      "manejar",
			"automatizar":    "hacerlo automático",
			"aprender":       "aprender",
			"dispositivo":    "cosa conectada",
		},
		Automated: map[string]string{
			"controlar":      "controlar remotamente",
			"automatizar":    "ejecutar automáticamente",
			"aprender":       "aprender y adaptar",
			"dispositivo":    "dispositivo en red",
		},
	}
}

// SwapVocabulary intercambia vocabulario según estilo
func (vs *VocabularySwitcher) SwapVocabulary(text string, style string) string {
	var vocab map[string]string
	
	switch style {
	case "technical":
		vocab = vs.Technical
	case "professional":
		vocab = vs.Professional
	case "casual":
		vocab = vs.Casual
	case "automated":
		vocab = vs.Automated
	default:
		vocab = vs.Professional
	}
	
	result := text
	for original, replacement := range vocab {
		result = strings.ReplaceAll(
			strings.ToLower(result),
			strings.ToLower(original),
			replacement,
		)
	}
	
	return result
}
