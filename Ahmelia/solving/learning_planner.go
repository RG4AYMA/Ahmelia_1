package solving

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
)

// ============================================================================
// LEARNING_PLANNER.GO — Bloque 50, Fase 4 (Knowledge Acquisition Mind)
// Último bloque de la "Opción A.5"
// ============================================================================
//
// Curriculum Builder (47) dice QUÉ aprender de cero (gaps sin lead ni
// analogía). Knowledge Memory (49) dice QUÉ ya es confiable y desde
// cuándo — pero sus entradas StatusTentative (Bloque 48) quedan sin
// usar: Actions que Ahmelia YA sabe hacer (tienen Action real, no son
// gaps) pero con pocas muestras como para confiar del todo todavía. Ese
// hueco intermedio — "no es territorio nuevo, pero tampoco está
// consolidado" — no lo cubre ni Curriculum (solo mira gaps genuinos) ni
// Knowledge Memory por sí solo (solo narra lo YA validado). Learning
// Planner integra los dos en un solo plan con DOS tipos de ítem.
//
// "Integrar, no crear", una vez más: cero cálculos nuevos de confianza
// o prioridad — Reforzar usa Samples/MinSampleSize (48), Aprender usa
// Priority tal cual ya lo ordena Curriculum (47). Cierra la "Opción
// A.5": Learning Validation (48) → Knowledge Memory (49) → Learning
// Planner (50), sin ninguna dependencia de Internet Gateway (4.1,
// todavía pendiente).

// PlanItemKind distingue los dos tipos de ítem del plan.
type PlanItemKind string

const (
	KindReinforce PlanItemKind = "reforzar" // ya es Action real, pocas muestras todavía (StatusTentative, 48)
	KindLearn     PlanItemKind = "aprender" // gap genuino, sin lead ni analogía (Curriculum, 47)
)

// PlanItem es un ítem del plan de aprendizaje.
type PlanItem struct {
	Kind        PlanItemKind
	Label       string
	Description string
	Priority    int // solo aplica a KindLearn (viene de LearningItem.Priority, 47); 0 en KindReinforce
	Samples     int // solo aplica a KindReinforce (viene de MemoryEntry.Samples, 49); 0 en KindLearn
}

// LearningPlan es el resultado de combinar Knowledge Memory (49) y
// Curriculum Builder (47) en un plan único.
type LearningPlan struct {
	Goal  string
	Items []PlanItem
}

// Planner es el Learning Planner en sí. Sin estado propio — recibe
// Knowledge Memory y Curriculum ya calculados, no los reconstruye ni
// depende de ellos directamente.
type Planner struct{}

// NewPlanner arma el planner.
func NewPlanner() *Planner {
	return &Planner{}
}

// Plan combina km (Bloque 49) y c (Bloque 47) en un LearningPlan.
// Reforzar sale de las entradas StatusTentative de Knowledge Memory,
// ordenadas por Samples descendente (más cerca de MinSampleSize
// primero — el avance más inmediato). Aprender son los Items de
// Curriculum tal cual, en el mismo orden que Curriculum ya los
// prioriza. Reforzar va PRIMERO como grupo: consolidar algo que ya
// funciona es una inversión más chica que aprender de cero. km/c
// pueden ser nil — mismo criterio defensivo de todo Fase 3/4: sin
// datos, esa mitad del plan sale vacía, no inventada.
func (p *Planner) Plan(km *KnowledgeMemory, c *Curriculum) *LearningPlan {
	plan := &LearningPlan{}
	if c != nil {
		plan.Goal = c.Goal
	}

	var reinforce []PlanItem
	if km != nil {
		for _, entry := range km.Entries {
			if entry.Status != StatusTentative {
				continue
			}
			reinforce = append(reinforce, PlanItem{
				Kind:        KindReinforce,
				Label:       entry.Label,
				Description: fmt.Sprintf("consolidar con más intentos (tiene %d de %d necesarios)", entry.Samples, MinSampleSize),
				Samples:     entry.Samples,
			})
		}
	}
	sort.SliceStable(reinforce, func(i, j int) bool {
		return reinforce[i].Samples > reinforce[j].Samples
	})

	var learn []PlanItem
	if c != nil {
		for _, item := range c.Items {
			learn = append(learn, PlanItem{
				Kind:        KindLearn,
				Label:       item.TaskDesc,
				Description: item.Reason,
				Priority:    item.Priority,
			})
		}
	}

	plan.Items = append(plan.Items, reinforce...)
	plan.Items = append(plan.Items, learn...)
	return plan
}

// Describe arma la narrativa agrupada por tipo — mismo criterio de "no
// mezclar categorías distintas en la misma lista" que DescribeValidations (48).
func (lp *LearningPlan) Describe() string {
	if len(lp.Items) == 0 {
		return "No tengo nada pendiente en el plan de aprendizaje — sin tentativas para reforzar y sin gaps genuinos en Curriculum."
	}

	var reinforceParts, learnParts []string
	for _, item := range lp.Items {
		switch item.Kind {
		case KindReinforce:
			reinforceParts = append(reinforceParts, fmt.Sprintf("%s (%s)", item.Label, item.Description))
		case KindLearn:
			learnParts = append(learnParts, fmt.Sprintf("%s (%s)", item.Label, item.Description))
		}
	}

	var b strings.Builder
	if len(reinforceParts) > 0 {
		b.WriteString("Para reforzar (ya lo sé hacer, me falta práctica): ")
		b.WriteString(strings.Join(reinforceParts, "; "))
		b.WriteString(". ")
	}
	if len(learnParts) > 0 {
		b.WriteString("Para aprender de cero, en orden de prioridad: ")
		b.WriteString(strings.Join(learnParts, "; "))
		b.WriteString(".")
	}
	return strings.TrimSpace(b.String())
}

// ── Trigger de consulta ──────────────────────────────────────────────

var plannerTriggers = []string{
	"que deberias reforzar",
	"cual es tu plan de aprendizaje",
	"en que deberias enfocarte para mejorar",
	"que te conviene practicar",
}

// ParsePlannerQuery detecta si se pregunta por el plan combinado —
// distinta de Curriculum (47, solo gaps genuinos) y de Knowledge
// Memory (49, solo lo ya consolidado): esta pregunta por AMBOS juntos,
// priorizados en un solo plan.
func ParsePlannerQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingPlanner)
}
