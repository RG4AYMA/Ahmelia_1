package reasoning

import (
	"fmt"
	"strings"
)

// Insight representa un descubrimiento automático sobre el comportamiento de Ahmelia
type Insight struct {
	Type        string  // "pattern", "trend", "problem", "opportunity"
	Title       string
	Description string
	Confidence  float64 // 0.0 - 1.0
	Priority    int     // 1-5 (5 = más urgente)
	Action      string  // Qué hacer al respecto
}

// InsightGenerator crea insights a partir del análisis
type InsightGenerator struct {
	analyzer *Analyzer
}

// NewInsightGenerator crea un generador de insights
func NewInsightGenerator(analyzer *Analyzer) *InsightGenerator {
	return &InsightGenerator{analyzer: analyzer}
}

/* =======================
   GENERAR TODOS LOS INSIGHTS
======================= */

func (ig *InsightGenerator) GenerateInsights() []Insight {
	insights := []Insight{}

	insights = append(insights, ig.insightsFromSuccessPatterns()...)
	insights = append(insights, ig.insightsFromFailurePatterns()...)
	insights = append(insights, ig.insightFromTrend())
	insights = append(insights, ig.insightsFromPerformance()...)
	insights = append(insights, ig.insightsFromAmbiguity()...)

	// Ordenar por prioridad descendente
	for i := 0; i < len(insights)-1; i++ {
		for j := i + 1; j < len(insights); j++ {
			if insights[i].Priority < insights[j].Priority {
				insights[i], insights[j] = insights[j], insights[i]
			}
		}
	}

	return insights
}

/* =======================
   INSIGHT: PATRONES EXITOSOS
======================= */

func (ig *InsightGenerator) insightsFromSuccessPatterns() []Insight {
	insights := []Insight{}
	patterns := ig.analyzer.AnalyzeSuccessPatterns()

	for i, p := range patterns {
		if i >= 2 {
			break
		}
		if p.IsReliable && p.Count > 2 {
			insights = append(insights, Insight{
				Type:        "pattern",
				Title:       fmt.Sprintf("'%s' es muy confiable", p.Action),
				Description: fmt.Sprintf("Funcionó el %.0f%% de las veces (%d intentos)", p.SuccessRate, p.Count),
				Confidence:  p.SuccessRate / 100,
				Priority:    3,
				Action:      "Puedes confiar en este comando",
			})
		}
	}
	return insights
}

/* =======================
   INSIGHT: PATRONES DE FALLO
======================= */

func (ig *InsightGenerator) insightsFromFailurePatterns() []Insight {
	insights := []Insight{}
	failures := ig.analyzer.AnalyzeFailurePatterns()

	for i, f := range failures {
		if i >= 2 {
			break
		}
		if f.Count >= 2 {
			insights = append(insights, Insight{
				Type:        "problem",
				Title:       fmt.Sprintf("'%s' falla con frecuencia", f.Action),
				Description: fmt.Sprintf("Falló %d veces. Causa probable: %s", f.Count, f.Reason),
				Confidence:  float64(f.Count) / 10,
				Priority:    5,
				Action:      fmt.Sprintf("Revisar la lógica de '%s'", f.Action),
			})
		}
	}
	return insights
}

/* =======================
   INSIGHT: TENDENCIA
======================= */

func (ig *InsightGenerator) insightFromTrend() Insight {
	trend := ig.analyzer.DetectTrend()

	switch trend {
	case "mejorando":
		return Insight{
			Type:        "trend",
			Title:       "Rendimiento mejorando",
			Description: "Las últimas acciones tienen más éxito que las anteriores",
			Confidence:  0.8,
			Priority:    3,
			Action:      "Mantén la configuración actual",
		}
	case "empeorando":
		return Insight{
			Type:        "trend",
			Title:       "Rendimiento bajando",
			Description: "Las últimas acciones fallan más que las anteriores",
			Confidence:  0.75,
			Priority:    5,
			Action:      "Revisar cambios recientes que puedan haber causado esto",
		}
	default:
		return Insight{
			Type:        "trend",
			Title:       "Rendimiento estable",
			Description: "Nivel de éxito consistente",
			Confidence:  0.6,
			Priority:    1,
			Action:      "Busca oportunidades de mejora",
		}
	}
}

/* =======================
   INSIGHT: RENDIMIENTO
======================= */

func (ig *InsightGenerator) insightsFromPerformance() []Insight {
	insights := []Insight{}
	perf := ig.analyzer.AnalyzePerformance()

	slowest := perf["slowest_action"].(string)
	slowestTime := perf["slowest_time_ms"].(int)
	avg := perf["avg_duration_ms"].(int)

	if slowest != "" && avg > 0 && slowestTime > avg*2 {
		insights = append(insights, Insight{
			Type:        "opportunity",
			Title:       fmt.Sprintf("'%s' es lento", slowest),
			Description: fmt.Sprintf("Tarda %dms (promedio: %dms)", slowestTime, avg),
			Confidence:  0.7,
			Priority:    2,
			Action:      fmt.Sprintf("Optimizar '%s' para mejorar velocidad de respuesta", slowest),
		})
	}
	return insights
}

/* =======================
   INSIGHT: AMBIGÜEDAD
======================= */

func (ig *InsightGenerator) insightsFromAmbiguity() []Insight {
	insights := []Insight{}
	ambiguous := ig.analyzer.DetectAmbiguous()

	for _, input := range ambiguous {
		insights = append(insights, Insight{
			Type:        "problem",
			Title:       "Comando ambiguo detectado",
			Description: fmt.Sprintf("'%s' produjo acciones diferentes en distintos momentos", input),
			Confidence:  0.65,
			Priority:    3,
			Action:      "Agregar más contexto para desambiguar ese comando",
		})
	}
	return insights
}

/* =======================
   RESUMEN EN TEXTO
   Para que SelfQuery pueda responder preguntas de razonamiento
======================= */

func (ig *InsightGenerator) Summarize() string {
	insights := ig.GenerateInsights()

	if len(insights) == 0 {
		return "Aún no tengo suficientes datos para analizar patrones. Continúa usándome"
	}

	var sb strings.Builder
	sb.WriteString("He analizado mis últimas acciones. ")

	limit := 2
	if len(insights) < limit {
		limit = len(insights)
	}

	for i := 0; i < limit; i++ {
		ins := insights[i]
		if i > 0 {
			sb.WriteString(" También: ")
		}
		sb.WriteString(ins.Description)
	}

	return sb.String()
}

// TopInsight retorna el insight más urgente
func (ig *InsightGenerator) TopInsight() *Insight {
	insights := ig.GenerateInsights()
	if len(insights) == 0 {
		return nil
	}
	return &insights[0]
}
