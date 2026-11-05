package reasoning

import (
	"fmt"
)

// Hypothesis es una posible explicación para un evento
type Hypothesis struct {
	Explanation string
	Likelihood  float64 // 0.0 - 1.0
	Evidence    string
	Testable    bool // Se puede verificar
}

// Hypothesizer genera explicaciones posibles para los hechos
type Hypothesizer struct {
	analyzer *Analyzer
}

// NewHypothesizer crea un generador de hipótesis
func NewHypothesizer(analyzer *Analyzer) *Hypothesizer {
	return &Hypothesizer{analyzer: analyzer}
}

/* =======================
   GENERAR HIPÓTESIS PARA UN FALLO
   "¿Por qué falló 'cambiar_ventana'?"
======================= */

func (h *Hypothesizer) ExplainFailure(action string) []Hypothesis {
	hypotheses := []Hypothesis{}

	// Hipótesis genéricas según el tipo de acción
	switch action {
	case "abrir_programa":
		hypotheses = append(hypotheses,
			Hypothesis{
				Explanation: "El programa no existe en el índice actual",
				Likelihood:  0.65,
				Evidence:    "Acciones similares también fallan",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "El nombre del programa es ambiguo",
				Likelihood:  0.20,
				Evidence:    "Hay múltiples programas con nombres parecidos",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "El índice está desactualizado",
				Likelihood:  0.10,
				Evidence:    "Hace mucho tiempo que no se actualiza",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Otro error desconocido",
				Likelihood:  0.05,
				Evidence:    "Sin datos específicos",
				Testable:    false,
			},
		)

	case "cambiar_ventana":
		hypotheses = append(hypotheses,
			Hypothesis{
				Explanation: "La ventana no está visible o activa",
				Likelihood:  0.55,
				Evidence:    "Cambios de ventana frecuentemente fallan cuando hay cambios de contexto",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "El nombre de la ventana es impreciso",
				Likelihood:  0.25,
				Evidence:    "Ambigüedad en la identificación de ventanas",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Ventana minimizada o fuera del escritorio",
				Likelihood:  0.15,
				Evidence:    "Error típico en sistemas multi-ventana",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Fallo del sistema operativo",
				Likelihood:  0.05,
				Evidence:    "Sin evidencia específica",
				Testable:    false,
			},
		)

	case "buscar_archivo":
		hypotheses = append(hypotheses,
			Hypothesis{
				Explanation: "El archivo no existe en el sistema",
				Likelihood:  0.70,
				Evidence:    "Ruta inválida o nombre incorrecto",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Permisos insuficientes",
				Likelihood:  0.15,
				Evidence:    "Sin acceso a directorios específicos",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Ruta relativa incorrecta",
				Likelihood:  0.10,
				Evidence:    "El contexto de directorios cambió",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Otro problema",
				Likelihood:  0.05,
				Evidence:    "Sin información adicional",
				Testable:    false,
			},
		)

	default:
		hypotheses = append(hypotheses,
			Hypothesis{
				Explanation: "Fallo en el reconocimiento del comando",
				Likelihood:  0.40,
				Evidence:    "Sin contexto específico",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Cambio en el estado del sistema",
				Likelihood:  0.30,
				Evidence:    "Condiciones externas afectaron la ejecución",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Error no capturado",
				Likelihood:  0.20,
				Evidence:    "Sistema respondió de forma inesperada",
				Testable:    false,
			},
			Hypothesis{
				Explanation: "Otro factor desconocido",
				Likelihood:  0.10,
				Evidence:    "Sin datos suficientes",
				Testable:    false,
			},
		)
	}

	return hypotheses
}

/* =======================
   GENERAR HIPÓTESIS PARA UN ÉXITO
   "¿Por qué funcionó 'bajar_volumen'?"
======================= */

func (h *Hypothesizer) ExplainSuccess(action string) []Hypothesis {
	hypotheses := []Hypothesis{}

	switch action {
	case "volumen_bajar":
		hypotheses = append(hypotheses,
			Hypothesis{
				Explanation: "El comando se correspondió exactamente con la API de audio",
				Likelihood:  0.95,
				Evidence:    "El sistema de audio responde consistentemente a este comando",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "El dispositivo de audio estaba disponible",
				Likelihood:  0.98,
				Evidence:    "No hay conflictos de hardware",
				Testable:    true,
			},
		)

	case "abrir_youtube":
		hypotheses = append(hypotheses,
			Hypothesis{
				Explanation: "La URL es simple y bien conocida",
				Likelihood:  0.90,
				Evidence:    "Aplicación registrada en el sistema",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "Conexión a Internet disponible",
				Likelihood:  0.85,
				Evidence:    "Sin errores de red",
				Testable:    true,
			},
		)

	default:
		hypotheses = append(hypotheses,
			Hypothesis{
				Explanation: "El comando se reconoció correctamente",
				Likelihood:  0.85,
				Evidence:    "Coincidencia exacta entre entrada y acción",
				Testable:    true,
			},
			Hypothesis{
				Explanation: "El sistema estaba en estado óptimo",
				Likelihood:  0.75,
				Evidence:    "Sin limitaciones de recursos",
				Testable:    true,
			},
		)
	}

	return hypotheses
}

/* =======================
   RESUMIR HIPÓTESIS CON CONFIANZA
======================= */

func (h *Hypothesizer) SummarizeHypotheses(hypotheses []Hypothesis) string {
	if len(hypotheses) == 0 {
		return "Sin hipótesis disponibles"
	}

	// La más probable
	best := hypotheses[0]
	for _, hyp := range hypotheses {
		if hyp.Likelihood > best.Likelihood {
			best = hyp
		}
	}

	return fmt.Sprintf(
		"Lo más probable (%.0f%%): %s. %s",
		best.Likelihood*100,
		best.Explanation,
		best.Evidence,
	)
}
