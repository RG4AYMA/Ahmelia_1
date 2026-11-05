package reasoning_deep

import (
	"fmt"
	"sort"
	"time"
)

// Perspective representa un ángulo de análisis
type Perspective struct {
	ID          string
	Name        string        // "rendimiento", "mantenimiento", "escalabilidad"
	Argument    string        // La posición que defiende
	Evidence    []string      // Evidencia que la respalda
	Score       float64       // 0.0-1.0 (cuán válida es)
	Risks       []string      // Riesgos de esta perspectiva
	Benefits    []string      // Beneficios
	Confidence  float64       // Cuánta certeza hay en esta perspectiva
	CreatedAt   time.Time
}

// PerspectiveAnalysis es el resultado de analizar desde múltiples ángulos
type PerspectiveAnalysis struct {
	Question      string
	Perspectives  []*Perspective
	Winner        *Perspective      // La perspectiva con mejor score
	Consensus     float64            // Acuerdo entre perspectivas (0-1)
	Synthesis     string             // Síntesis de todas
	Tradeoffs     []string           // Compromisos identificados
	AnalyzedAt    time.Time
}

// PerspectiveEngine analiza desde múltiples ángulos
type PerspectiveEngine struct {
	perspectives map[string]*Perspective
}

// NewPerspectiveEngine crea un nuevo motor de perspectivas
func NewPerspectiveEngine() *PerspectiveEngine {
	return &PerspectiveEngine{
		perspectives: make(map[string]*Perspective),
	}
}

// NewPerspective crea una nueva perspectiva
func NewPerspective(name, argument string) *Perspective {
	return &Perspective{
		ID:         generatePerspectiveID(),
		Name:       name,
		Argument:   argument,
		Evidence:   []string{},
		Score:      0.5,
		Risks:      []string{},
		Benefits:   []string{},
		Confidence: 0.5,
		CreatedAt:  time.Now(),
	}
}

// AddEvidence añade evidencia a una perspectiva
func (p *Perspective) AddEvidence(evidence string) {
	p.Evidence = append(p.Evidence, evidence)
}

// AddRisk añade un riesgo
func (p *Perspective) AddRisk(risk string) {
	p.Risks = append(p.Risks, risk)
}

// AddBenefit añade un beneficio
func (p *Perspective) AddBenefit(benefit string) {
	p.Benefits = append(p.Benefits, benefit)
}

// AnalyzeFromPerspectives analiza una pregunta desde múltiples ángulos
func (pe *PerspectiveEngine) AnalyzeFromPerspectives(question string, perspectives []*Perspective) *PerspectiveAnalysis {
	analysis := &PerspectiveAnalysis{
		Question:     question,
		Perspectives: perspectives,
		AnalyzedAt:   time.Now(),
	}

	// Calcular scores basado en evidencia y confianza
	pe.calculateScores(perspectives)

	// Encontrar la perspectiva ganadora
	analysis.Winner = pe.findWinner(perspectives)

	// Calcular consenso
	analysis.Consensus = pe.calculateConsensus(perspectives)

	// Sintetizar
	analysis.Synthesis = pe.synthesize(perspectives)

	// Identificar tradeoffs
	analysis.Tradeoffs = pe.identifyTradeoffs(perspectives)

	return analysis
}

// calculateScores calcula el score de cada perspectiva
func (pe *PerspectiveEngine) calculateScores(perspectives []*Perspective) {
	for _, p := range perspectives {
		// Score = (evidencia + confianza) / 2
		evidenceScore := float64(len(p.Evidence)) / 10.0 // Máx 10 evidencias
		if evidenceScore > 1.0 {
			evidenceScore = 1.0
		}

		p.Score = (evidenceScore*0.7 + p.Confidence*0.3)

		// Ajustar por riesgos
		riskPenalty := float64(len(p.Risks)) * 0.05
		p.Score = p.Score - riskPenalty
		if p.Score < 0 {
			p.Score = 0
		}
	}
}

// findWinner encuentra la perspectiva con mejor score
func (pe *PerspectiveEngine) findWinner(perspectives []*Perspective) *Perspective {
	if len(perspectives) == 0 {
		return nil
	}

	winner := perspectives[0]
	for _, p := range perspectives {
		if p.Score > winner.Score {
			winner = p
		}
	}
	return winner
}

// calculateConsensus calcula el acuerdo entre perspectivas (0-1)
func (pe *PerspectiveEngine) calculateConsensus(perspectives []*Perspective) float64 {
	if len(perspectives) < 2 {
		return 1.0
	}

	// Calcular varianza de scores
	var sum, sumSq float64
	for _, p := range perspectives {
		sum += p.Score
		sumSq += p.Score * p.Score
	}

	mean := sum / float64(len(perspectives))
	variance := (sumSq / float64(len(perspectives))) - (mean * mean)

	// Consenso inverso a varianza
	// Alta varianza = bajo consenso
	consensus := 1.0 - (variance * 2.0)
	if consensus < 0 {
		consensus = 0
	}
	if consensus > 1 {
		consensus = 1
	}

	return consensus
}

// synthesize sintetiza todas las perspectivas en texto
func (pe *PerspectiveEngine) synthesize(perspectives []*Perspective) string {
	if len(perspectives) == 0 {
		return "Sin perspectivas para analizar"
	}

	synthesis := ""

	// Ordenar por score
	sorted := make([]*Perspective, len(perspectives))
	copy(sorted, perspectives)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	// Síntesis narrativa
	synthesis += fmt.Sprintf("Desde %s (score: %.2f): %s\n",
		sorted[0].Name, sorted[0].Score, sorted[0].Argument)

	if len(sorted) > 1 {
		synthesis += fmt.Sprintf("\nPero desde %s (score: %.2f): %s\n",
			sorted[1].Name, sorted[1].Score, sorted[1].Argument)
	}

	if len(sorted) > 2 {
		synthesis += fmt.Sprintf("\nTambién desde %s (score: %.2f): %s\n",
			sorted[2].Name, sorted[2].Score, sorted[2].Argument)
	}

	return synthesis
}

// identifyTradeoffs identifica compromisos entre perspectivas
func (pe *PerspectiveEngine) identifyTradeoffs(perspectives []*Perspective) []string {
	tradeoffs := []string{}

	// Comparar beneficios y riesgos entre perspectivas
	for i := 0; i < len(perspectives); i++ {
		for j := i + 1; j < len(perspectives); j++ {
			p1 := perspectives[i]
			p2 := perspectives[j]

			// Si una tiene beneficios que la otra no, es un tradeoff
			for _, b1 := range p1.Benefits {
				found := false
				for _, b2 := range p2.Benefits {
					if b1 == b2 {
						found = true
						break
					}
				}
				if !found && len(p2.Risks) > 0 {
					tradeoff := fmt.Sprintf(
						"Elegir '%s' sacrifica beneficio '%s' pero evita riesgos de '%s'",
						p1.Name, b1, p2.Name,
					)
					tradeoffs = append(tradeoffs, tradeoff)
				}
			}
		}
	}

	return tradeoffs
}

// GetReport retorna un reporte narrativo
func (analysis *PerspectiveAnalysis) GetReport() string {
	report := fmt.Sprintf("ANÁLISIS DESDE MÚLTIPLES PERSPECTIVAS:\n\n")
	report += fmt.Sprintf("Pregunta: %s\n\n", analysis.Question)

	// Listar todas las perspectivas
	report += "PERSPECTIVAS ANALIZADAS:\n"
	sort.Slice(analysis.Perspectives, func(i, j int) bool {
		return analysis.Perspectives[i].Score > analysis.Perspectives[j].Score
	})

	for i, p := range analysis.Perspectives {
		report += fmt.Sprintf("\n%d. %s (score: %.2f, confianza: %.0f%%)\n",
			i+1, p.Name, p.Score, p.Confidence*100)
		report += fmt.Sprintf("   %s\n", p.Argument)

		if len(p.Evidence) > 0 {
			report += fmt.Sprintf("   Evidencia:\n")
			for _, e := range p.Evidence {
				report += fmt.Sprintf("     • %s\n", e)
			}
		}

		if len(p.Benefits) > 0 {
			report += fmt.Sprintf("   Beneficios: %v\n", p.Benefits)
		}

		if len(p.Risks) > 0 {
			report += fmt.Sprintf("   Riesgos: %v\n", p.Risks)
		}
	}

	report += fmt.Sprintf("\n\nGANADORA: %s (%.2f)\n", analysis.Winner.Name, analysis.Winner.Score)
	report += fmt.Sprintf("Consenso entre perspectivas: %.0f%%\n\n", analysis.Consensus*100)

	if len(analysis.Tradeoffs) > 0 {
		report += "TRADEOFFS IDENTIFICADOS:\n"
		for _, t := range analysis.Tradeoffs {
			report += fmt.Sprintf("  • %s\n", t)
		}
	}

	return report
}

// GetSimplifiedReport retorna solo el resumen
func (analysis *PerspectiveAnalysis) GetSimplifiedReport() string {
	report := fmt.Sprintf(
		"Mi análisis considera:\n"+
			"  • %s: %s\n"+
			"  • %s: %s\n",
		analysis.Perspectives[0].Name, analysis.Perspectives[0].Argument,
		analysis.Perspectives[1].Name, analysis.Perspectives[1].Argument,
	)

	if len(analysis.Perspectives) > 2 {
		report += fmt.Sprintf(
			"  • %s: %s\n",
			analysis.Perspectives[2].Name, analysis.Perspectives[2].Argument,
		)
	}

	report += fmt.Sprintf(
		"\nMi conclusión: %s es la mejor opción (%.0f%% confianza, consenso: %.0f%%)",
		analysis.Winner.Name,
		analysis.Winner.Score*100,
		analysis.Consensus*100,
	)

	return report
}

// helper
func generatePerspectiveID() string {
	return fmt.Sprintf("persp-%d", time.Now().UnixNano())
}
