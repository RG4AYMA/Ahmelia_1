package reasoning

import (
	"fmt"
	"strings"
)

// trustTriggers detecta preguntas sobre confianza en la ÚLTIMA acción
// ejecutada — distinto de "por qué hiciste eso" (que pide la
// justificación) y de "estado mental"/Capa 5.5 (que es sobre
// foco/dudas/preocupaciones). Esto pregunta puntualmente "¿confiás en
// lo que acabás de hacer?", que es justo lo que Decider.DecideTrust ya
// sabía calcular y nadie llamaba (Bloque 14).
var trustTriggers = []string{
	"confias en eso",
	"confias en lo que hiciste",
	"que tan confiable fue eso",
	"que tanto confias en",
	"fue confiable eso",
}

// ParseTrustQuery detecta si la pregunta pide la evaluación de confianza
// de Decider sobre la última acción.
func ParseTrustQuery(normalizedText string) bool {
	for _, trigger := range trustTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}

// Alternative es una opción considerada
type Alternative struct {
	Option      string
	Pros        []string
	Cons        []string
	Confidence  float64
	Risk        float64 // 0.0 - 1.0
	Recommended bool
}

// DecisionTree representa una decisión con alternativas
type DecisionTree struct {
	Question      string        // ¿Qué decidir?
	Alternatives  []Alternative // Opciones
	Chosen        string        // Qué se eligió
	Justification string        // Por qué se eligió
	Confidence    float64       // Confianza en la decisión
}

// Decider construye árboles de decisión
type Decider struct {
	analyzer     *Analyzer
	prioritizer  *Prioritizer
	hypothesizer *Hypothesizer
}

// NewDecider crea un tomador de decisiones
func NewDecider(analyzer *Analyzer, prioritizer *Prioritizer, hypothesizer *Hypothesizer) *Decider {
	return &Decider{
		analyzer:     analyzer,
		prioritizer:  prioritizer,
		hypothesizer: hypothesizer,
	}
}

/* =======================
   DECIDIR QUÉ ACCIÓN EJECUTAR
   Dado un input, genera alternativas y elige la mejor
======================= */

func (d *Decider) DecideAction(input string) DecisionTree {
	// Aquí el motor ya ha clasificado el input
	// El Decider solo genera alternativas y prioriza

	tree := DecisionTree{
		Question: fmt.Sprintf("¿Qué acción ejecutar para '%s'?", input),
		Alternatives: []Alternative{
			{
				Option:      "Ejecutar acción principal",
				Pros:        []string{"Coincide exactamente", "Historial de éxito"},
				Cons:        []string{"Puede fallar si contexto cambió"},
				Confidence:  0.85,
				Risk:        0.15,
				Recommended: true,
			},
			{
				Option:      "Pedir confirmación",
				Pros:        []string{"Evita errores", "Aprende del usuario"},
				Cons:        []string{"Más lento", "Interrumpe el flujo"},
				Confidence:  0.70,
				Risk:        0.05,
				Recommended: false,
			},
			{
				Option:      "Buscar alternativa similar",
				Pros:        []string{"Flexible", "Aprende variaciones"},
				Cons:        []string{"Puede no ser exacto"},
				Confidence:  0.60,
				Risk:        0.30,
				Recommended: false,
			},
		},
		Chosen:        "Ejecutar acción principal",
		Justification: "Confianza alta en el mapeo input-acción",
		Confidence:    0.85,
	}

	return tree
}

/* =======================
   DECIDIR CÓMO RESPONDER A UNA PREGUNTA
   Ej: "¿Por qué hiciste eso?"
======================= */

func (d *Decider) DecideResponse(question string, analysisDepth string) DecisionTree {
	tree := DecisionTree{
		Question: fmt.Sprintf("¿Cómo responder '%s'?", question),
	}

	// Decisión depende de cuántos datos tenemos
	switch analysisDepth {
	case "insufficient":
		tree.Alternatives = []Alternative{
			{
				Option:      "Admitir que no hay suficientes datos",
				Pros:        []string{"Honesto", "Evita especular"},
				Cons:        []string{"Parece menos inteligente"},
				Confidence:  0.90,
				Risk:        0.0,
				Recommended: true,
			},
			{
				Option:      "Especular de todas formas",
				Pros:        []string{"Parece que sé"},
				Cons:        []string{"Puedo inventar", "Pierde confianza"},
				Confidence:  0.30,
				Risk:        0.95,
				Recommended: false,
			},
		}
		tree.Chosen = "Admitir que no hay suficientes datos"
		tree.Justification = "Más de 3 datos = especular. Menos = admitir ignorancia"
		tree.Confidence = 0.90

	case "partial":
		tree.Alternatives = []Alternative{
			{
				Option:      "Responder con contexto y limitaciones",
				Pros:        []string{"Equilibrado", "Honesto", "Informativo"},
				Cons:        []string{"Más complejo"},
				Confidence:  0.80,
				Risk:        0.10,
				Recommended: true,
			},
			{
				Option:      "Responder con confianza total",
				Pros:        []string{"Simple y directo"},
				Cons:        []string{"Puede estar equivocado"},
				Confidence:  0.60,
				Risk:        0.40,
				Recommended: false,
			},
		}
		tree.Chosen = "Responder con contexto y limitaciones"
		tree.Justification = "Tengo datos pero no suficientes para conclusiones finales"
		tree.Confidence = 0.80

	default: // sufficient
		tree.Alternatives = []Alternative{
			{
				Option:      "Responder con confianza y evidencia",
				Pros:        []string{"Datos sólidos", "Justificado"},
				Cons:        []string{"Puede haber excepciones"},
				Confidence:  0.95,
				Risk:        0.05,
				Recommended: true,
			},
		}
		tree.Chosen = "Responder con confianza y evidencia"
		tree.Justification = "Tengo datos suficientes para conclusión sólida"
		tree.Confidence = 0.95
	}

	return tree
}

/* =======================
   DECIDIR SI CONFIAR EN UNA ACCIÓN
======================= */

func (d *Decider) DecideTrust(action string, lastSuccessRate float64) DecisionTree {
	tree := DecisionTree{
		Question: fmt.Sprintf("¿Confiar en '%s'?", action),
	}

	// Basado en tasa de éxito histórica
	if lastSuccessRate >= 0.8 {
		tree.Alternatives = []Alternative{
			{
				Option:      "Ejecutar sin cuestionar",
				Confidence:  0.95,
				Risk:        0.05,
				Recommended: true,
				Pros:        []string{"Muy confiable", "Rápido"},
			},
		}
		tree.Chosen = "Ejecutar sin cuestionar"
		tree.Confidence = 0.95
	} else if lastSuccessRate >= 0.5 {
		tree.Alternatives = []Alternative{
			{
				Option:      "Ejecutar pero monitorear",
				Confidence:  0.70,
				Risk:        0.30,
				Recommended: true,
				Pros:        []string{"Flexible", "Aprende"},
			},
			{
				Option:      "Pedir confirmación primero",
				Confidence:  0.85,
				Risk:        0.10,
				Recommended: false,
				Pros:        []string{"Más seguro"},
			},
		}
		tree.Chosen = "Ejecutar pero monitorear"
		tree.Confidence = 0.70
	} else {
		tree.Alternatives = []Alternative{
			{
				Option:      "Pedir confirmación al usuario",
				Confidence:  0.90,
				Risk:        0.05,
				Recommended: true,
				Pros:        []string{"Evita errores", "Aprende"},
			},
			{
				Option:      "Rechazar y buscar alternativa",
				Confidence:  0.80,
				Risk:        0.20,
				Recommended: false,
				Pros:        []string{"Conservador"},
			},
		}
		tree.Chosen = "Pedir confirmación al usuario"
		tree.Confidence = 0.90
	}

	tree.Justification = fmt.Sprintf(
		"Tasa de éxito histórica: %.0f%%",
		lastSuccessRate*100,
	)

	return tree
}

/* =======================
   RESUMIR DECISIÓN
======================= */

func (d *Decider) SummarizeDecision(tree DecisionTree) string {
	return fmt.Sprintf(
		"Decidí '%s' con confianza %.0f%%. Razón: %s",
		tree.Chosen,
		tree.Confidence*100,
		tree.Justification,
	)
}
