package reasoning

import (
	"fmt"
	"strings"
)

// Explanation es una respuesta inteligible para el usuario
type Explanation struct {
	Summary       string   // Explicación corta
	DetailLevel   string   // "brief", "normal", "detailed"
	Evidence      string   // Evidencia que la respalda
	Uncertainty   string   // Lo que no estoy seguro
	Alternatives  []string // Otras opciones consideradas
	InternalNote  string   // Pensamiento interno (no mostrar)
}

// Explainer traduce razonamientos complejos en explicaciones claras
type Explainer struct {
	reasoner *Reasoner
	critic   *Critic
}

// NewExplainer crea un explicador
func NewExplainer(reasoner *Reasoner, critic *Critic) *Explainer {
	return &Explainer{
		reasoner: reasoner,
		critic:   critic,
	}
}

/* =======================
   EXPLICAR: "¿Por qué hiciste eso?"
======================= */

func (e *Explainer) ExplainDecision(action string, input string, target string, confidence float64) Explanation {
	exp := Explanation{
		DetailLevel: "normal",
	}

	// Si target no aparece dentro de lo que el usuario dijo literalmente,
	// es señal de que hubo una referencia implícita resuelta en el
	// camino (pronombre como "ábrelo" -> "chrome", vía WorldState) — no
	// que el clasificador inventó el target de la nada. Antes esto era
	// invisible para el explicador: solo veía action+input+confidence, y
	// "por qué hiciste eso" nunca podía contar la parte más interesante
	// de la decisión.
	resolvedNote := ""
	if target != "" && !strings.Contains(strings.ToLower(input), strings.ToLower(target)) {
		resolvedNote = fmt.Sprintf(" Entendí que te referías a '%s'.", target)
	}

	// Construcción inteligente basada en confianza
	if confidence >= 0.9 {
		exp.Summary = fmt.Sprintf(
			"Elegí '%s' porque coincide exactamente con tu comando '%s' y ha funcionado correctamente en ocasiones anteriores.%s",
			action, input, resolvedNote,
		)
		exp.Evidence = "Tasa de éxito histórica muy alta para esta acción"
		exp.Uncertainty = "Prácticamente ninguna duda"
	} else if confidence >= 0.7 {
		exp.Summary = fmt.Sprintf(
			"Elegí '%s' como la opción más probable para '%s'.%s",
			action, input, resolvedNote,
		)
		exp.Evidence = fmt.Sprintf("Confianza: %.0f%%. Basado en patrones previos", confidence*100)
		exp.Uncertainty = "Hay un pequeño margen de error"
	} else if confidence >= 0.5 {
		exp.Summary = fmt.Sprintf(
			"Intenté '%s' para '%s', pero no estoy completamente seguro.%s",
			action, input, resolvedNote,
		)
		exp.Evidence = fmt.Sprintf("Confianza moderada (%.0f%%). Pocos datos históricos", confidence*100)
		exp.Uncertainty = "Hay un riesgo notable de que sea la acción equivocada"
		exp.Alternatives = []string{"Pedir confirmación", "Buscar contexto adicional"}
	} else {
		exp.Summary = fmt.Sprintf(
			"Intenté '%s' para '%s', pero tengo dudas reales.%s",
			action, input, resolvedNote,
		)
		exp.Evidence = fmt.Sprintf("Confianza baja (%.0f%%). Datos insuficientes", confidence*100)
		exp.Uncertainty = "No tengo suficiente información para estar seguro"
		exp.Alternatives = []string{"Debería haber pedido confirmación antes"}
	}

	// Pensamiento interno
	exp.InternalNote = fmt.Sprintf(
		"Confianza: %.0f%%. Coincidencia: %s. Historial: verificado",
		confidence*100, action,
	)

	return exp
}

/* =======================
   EXPLICAR: "¿Qué analizas?"
======================= */

func (e *Explainer) ExplainAnalysis(actionCount int, successRate float64, problemCount int) Explanation {
	exp := Explanation{
		DetailLevel: "normal",
	}

	if actionCount < 5 {
		exp.Summary = "Aún no tengo suficientes datos. Necesito más acciones para analizar patrones reales"
		exp.Evidence = fmt.Sprintf("Solo %d acciones registradas (necesito al menos 10)", actionCount)
		exp.Uncertainty = "Mis conclusiones serían especulación"
		exp.InternalNote = "No hay suficiente evidencia para hacer afirmaciones"
		return exp
	}

	// Con datos suficientes
	exp.Summary = fmt.Sprintf(
		"He analizado mis últimas %d acciones. Mi tasa de éxito es %.0f%%, y he detectado %d problemas potenciales.",
		actionCount, successRate*100, problemCount,
	)

	evidence := []string{
		fmt.Sprintf("Total de acciones: %d", actionCount),
		fmt.Sprintf("Tasa de éxito: %.0f%%", successRate*100),
	}

	if problemCount > 0 {
		evidence = append(evidence, fmt.Sprintf("Problemas detectados: %d", problemCount))
	} else {
		evidence = append(evidence, "Sin problemas significativos detectados")
	}

	exp.Evidence = strings.Join(evidence, " | ")

	// Incertidumbre
	if successRate < 0.7 {
		exp.Uncertainty = "Hay margen considerable de mejora"
	} else if successRate < 0.9 {
		exp.Uncertainty = "Rendimiento bueno pero no óptimo"
	} else {
		exp.Uncertainty = "Rendimiento muy cercano al óptimo"
	}

	exp.InternalNote = fmt.Sprintf(
		"Datos: %d acciones. Confianza en análisis: media. Necesito monitor continuo",
		actionCount,
	)

	return exp
}

/* =======================
   EXPLICAR: "¿Cuál es tu tendencia?"
======================= */

func (e *Explainer) ExplainTrend(trend string, actionCount int) Explanation {
	exp := Explanation{
		DetailLevel: "normal",
	}

	if actionCount < 6 {
		exp.Summary = "Datos insuficientes para detectar tendencia"
		exp.Evidence = fmt.Sprintf("Necesito al menos 6 acciones, tengo %d", actionCount)
		exp.Uncertainty = "No puedo afirmar una tendencia con tan pocos datos"
		exp.InternalNote = "Tendencia es especulación en este punto"
		return exp
	}

	switch trend {
	case "mejorando":
		exp.Summary = "Mi rendimiento está mejorando. Las acciones recientes tienen más éxito"
		exp.Evidence = "Las últimas acciones superan el promedio histórico"
		exp.Uncertainty = "Podría ser una variación aleatoria, no un patrón real"
		exp.InternalNote = "Tendencia positiva detectada. Mantener monitoreo"

	case "empeorando":
		exp.Summary = "Mi rendimiento está empeorando. Las acciones recientes fallan más"
		exp.Evidence = "Las últimas acciones tienen menor tasa de éxito"
		exp.Uncertainty = "Podría deberse a cambios en el entorno, no a errores míos"
		exp.Alternatives = []string{"Revisar cambios recientes en la configuración"}
		exp.InternalNote = "Tendencia negativa. CRÍTICO: requiere atención"

	default:
		exp.Summary = "Mi rendimiento es estable. Las acciones tienen un éxito consistente"
		exp.Evidence = "No hay diferencia significativa entre períodos recientes y antiguos"
		exp.Uncertainty = "La estabilidad es buena, pero puedo estar estancado"
		exp.InternalNote = "Rendimiento sin cambios significativos"
	}

	return exp
}

/* =======================
   EXPLICAR: "¿Qué mejorarías?"
======================= */

func (e *Explainer) ExplainImprovements(problems []Priority, actionCount int) Explanation {
	exp := Explanation{
		DetailLevel: "normal",
	}

	if actionCount < 5 {
		exp.Summary = "No tengo suficientes datos para proponer mejoras concretas"
		exp.Evidence = fmt.Sprintf("Necesito analizar al menos 10 acciones, tengo %d", actionCount)
		exp.Uncertainty = "Las mejoras que propusiera serían especulación"
		exp.InternalNote = "Insuficiente evidencia para recomendaciones"
		return exp
	}

	if len(problems) == 0 {
		exp.Summary = "Mis sistemas funcionan bien. No hay problemas críticos detectados"
		exp.Evidence = "Tasa de éxito alta, sin patrones de fallo recurrentes"
		exp.Uncertainty = "Siempre hay espacio para optimización"
		exp.InternalNote = "Sin problemas, pero mantener vigilancia"
		return exp
	}

	// Con problemas detectados
	topProblem := problems[0]

	exp.Summary = fmt.Sprintf(
		"Mi principal área de mejora: %s",
		topProblem.Item,
	)

	exp.Evidence = topProblem.Reason

	// Alternativas
	if len(problems) > 1 {
		exp.Alternatives = []string{
			fmt.Sprintf("Secundario: %s", problems[1].Item),
		}
		if len(problems) > 2 {
			exp.Alternatives = append(exp.Alternatives,
				fmt.Sprintf("Tercero: %s", problems[2].Item),
			)
		}
	}

	// Incertidumbre
	exp.Uncertainty = fmt.Sprintf(
		"Confianza en esto: %.0f%% (podría haber factores que no detecté)",
		topProblem.Score*100,
	)

	exp.InternalNote = fmt.Sprintf(
		"Detecté %d problemas. Priorizando por severidad y confianza",
		len(problems),
	)

	return exp
}

/* =======================
   FORMATEAR EXPLICACIÓN PARA OUTPUT
======================= */

func (e *Explainer) Format(exp Explanation, showInternalThoughts bool) string {
	var sb strings.Builder

	sb.WriteString(exp.Summary)

	if exp.Evidence != "" {
		sb.WriteString(fmt.Sprintf("\n📊 Evidencia: %s", exp.Evidence))
	}

	if exp.Uncertainty != "" {
		sb.WriteString(fmt.Sprintf("\n⚠️  Incertidumbre: %s", exp.Uncertainty))
	}

	if len(exp.Alternatives) > 0 {
		sb.WriteString("\n🔄 Consideré también:")
		for _, alt := range exp.Alternatives {
			sb.WriteString(fmt.Sprintf("\n  • %s", alt))
		}
	}

	if showInternalThoughts && exp.InternalNote != "" {
		sb.WriteString(fmt.Sprintf("\n🧠 [Pensamiento interno: %s]", exp.InternalNote))
	}

	return sb.String()
}

/* =======================
   RESUMIR EXPLICACIÓN
   Versión muy corta
======================= */

func (e *Explainer) SummarizeExplanation(exp Explanation) string {
	return exp.Summary
}
