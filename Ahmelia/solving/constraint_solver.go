package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	"ahmelia/shared"
)

// ============================================================================
// CONSTRAINT_SOLVER.GO — Bloque 26, Fase 3 (Cognitive Mind), Capa 3.6
// ============================================================================
//
// Goal Analyzer (Bloque 21) ya detecta Requirements — "rápido", "que
// sea escalable", "sin conexión a Internet" — pero nadie los vuelve a
// mirar después: Problem Decomposer, Knowledge Mapper, Dependency
// Solver y Hypothesis Generator arman el plan sin chequear si ese plan
// en verdad puede cumplir lo que el usuario pidió. Constraint Solver
// cierra ese círculo: cruza cada Requirement contra lo que las capas
// anteriores YA calcularon, y dice si hay señal para confirmarlo, para
// dudar de él, o si simplemente no hay forma de saberlo con lo que
// Ahmelia tiene hoy.
//
// Primera pasada, mismo criterio que el resto de Fase 3: HOY solo hay
// señal real para UN tipo de Requirement — los de velocidad/urgencia,
// que se pueden cruzar contra Gaps() (Bloque 23) y Blocked() (Bloque
// 24): territorio nuevo y tareas bloqueadas son, literalmente, motivos
// concretos de demora. El resto de los Requirements (escalabilidad,
// seguridad, offline, costo, plataforma) quedan honestamente marcados
// como "sin señal para verificar" — inventar una respuesta ahí sería
// peor que admitir que el plan actual no trae esa información. Eso se
// va resolver capa por capa, a medida que existan módulos que sí sepan
// algo de recursos/tiempo/plataforma — no acá, no en este bloque.
//
// El tipo se llama ConstraintSolver (no Solver) porque solving.Solver
// ya lo usa Dependency Solver (Bloque 24, Capa 3.4) — dos "solvers"
// distintos en el mismo paquete, dos nombres distintos.

// ConstraintStatus es el veredicto sobre UN Requirement.
type ConstraintStatus string

const (
	// StatusSatisfiable: las capas anteriores no encontraron nada que
	// ponga en riesgo este Requirement.
	StatusSatisfiable ConstraintStatus = "cumplible"
	// StatusAtRisk: hay señal concreta (gaps, bloqueos) de que este
	// Requirement puede no cumplirse tal como está el plan hoy.
	StatusAtRisk ConstraintStatus = "en riesgo"
	// StatusUnverifiable: Ahmelia no tiene, todavía, ninguna fuente que
	// le permita confirmar o descartar este Requirement.
	StatusUnverifiable ConstraintStatus = "sin señal para verificar"
)

// ConstraintCheck es el resultado de evaluar UN Requirement.
type ConstraintCheck struct {
	Requirement string
	Status      ConstraintStatus
	Reason      string
}

// ConstraintReport es el resultado de evaluar TODOS los Requirements de
// un GoalAnalysis.
type ConstraintReport struct {
	Goal   string
	Checks []ConstraintCheck
}

// ConstraintSolver es el Constraint Solver en sí. Sin estado propio,
// mismo criterio que el resto de Fase 3.
type ConstraintSolver struct{}

// NewConstraintSolver arma el Constraint Solver.
func NewConstraintSolver() *ConstraintSolver {
	return &ConstraintSolver{}
}

// Check evalúa cada Requirement de ga contra lo que km (Bloque 23) y dg
// (Bloque 24) ya calcularon. km y dg pueden ser nil — mismo criterio
// defensivo que el resto de Fase 3: sin esos datos, un Requirement de
// velocidad queda Unverifiable en vez de arriesgar un veredicto sin
// base.
func (cs *ConstraintSolver) Check(ga *GoalAnalysis, km *KnowledgeMap, dg *DependencyGraph) *ConstraintReport {
	report := &ConstraintReport{Goal: ga.MainGoal}

	for _, req := range ga.Requirements {
		report.Checks = append(report.Checks, checkRequirement(req, km, dg))
	}

	return report
}

// checkRequirement decide, para UN Requirement, si hay una regla
// conocida para evaluarlo (hoy, solo velocidad/urgencia) o si queda sin
// señal.
func checkRequirement(req Requirement, km *KnowledgeMap, dg *DependencyGraph) ConstraintCheck {
	norm := shared.Normalize(req.Description)

	if strings.Contains(norm, "velocidad") || strings.Contains(norm, "urgencia") {
		return checkSpeedRequirement(req, km, dg)
	}

	return ConstraintCheck{
		Requirement: req.Description,
		Status:      StatusUnverifiable,
		Reason:      "El plan actual no trae información de recursos, plataforma o costo — no hay forma de confirmar esto automáticamente, hay que revisarlo a mano.",
	}
}

// checkSpeedRequirement cruza un Requirement de velocidad/urgencia
// contra Gaps() (Bloque 23) y Blocked() (Bloque 24): territorio nuevo
// sin resolver y tareas bloqueadas son, en la práctica, las dos fuentes
// de demora que el plan ya conoce.
func checkSpeedRequirement(req Requirement, km *KnowledgeMap, dg *DependencyGraph) ConstraintCheck {
	if km == nil {
		return ConstraintCheck{
			Requirement: req.Description,
			Status:      StatusUnverifiable,
			Reason:      "Todavía no se corrió Knowledge Mapper sobre este plan — sin eso no se puede saber si hay territorio nuevo que demore la entrega.",
		}
	}

	gapCount := km.GapCount()
	if gapCount == 0 {
		return ConstraintCheck{
			Requirement: req.Description,
			Status:      StatusSatisfiable,
			Reason:      "No hay territorio nuevo en el plan — todas las tareas ya son capacidad conocida, no debería haber demoras por aprendizaje.",
		}
	}

	blockedCount := 0
	if dg != nil {
		blockedCount = dg.BlockedCount()
	}

	if blockedCount > 0 {
		return ConstraintCheck{
			Requirement: req.Description,
			Status:      StatusAtRisk,
			Reason: fmt.Sprintf(
				"%d tarea(s) son territorio nuevo y dejan %d tarea(s) bloqueadas esperándolas — eso es tiempo que no se puede estimar de antemano.",
				gapCount, blockedCount,
			),
		}
	}

	return ConstraintCheck{
		Requirement: req.Description,
		Status:      StatusAtRisk,
		Reason: fmt.Sprintf(
			"%d tarea(s) son territorio nuevo — no bloquean nada más, pero el tiempo real depende de qué tan rápido se resuelvan.",
			gapCount,
		),
	}
}

// RiskCount cuenta cuántos Requirements quedaron en riesgo, sin que
// quien llame recorra Checks a mano.
func (cr *ConstraintReport) RiskCount() int {
	n := 0
	for _, c := range cr.Checks {
		if c.Status == StatusAtRisk {
			n++
		}
	}
	return n
}

// Risks devuelve solo los Checks en riesgo, en orden.
func (cr *ConstraintReport) Risks() []ConstraintCheck {
	var risks []ConstraintCheck
	for _, c := range cr.Checks {
		if c.Status == StatusAtRisk {
			risks = append(risks, c)
		}
	}
	return risks
}

// Describe arma la narrativa: por cada Requirement, su veredicto y por
// qué.
func (cr *ConstraintReport) Describe() string {
	if len(cr.Checks) == 0 {
		return fmt.Sprintf("No detecté requisitos implícitos para \"%s\" — nada que Constraint Solver tenga que evaluar todavía.", cr.Goal)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Requisitos para \"%s\": ", cr.Goal)

	parts := make([]string, 0, len(cr.Checks))
	for _, c := range cr.Checks {
		parts = append(parts, fmt.Sprintf("%s [%s] — %s", c.Requirement, c.Status, c.Reason))
	}
	b.WriteString(strings.Join(parts, "; "))
	b.WriteString(".")

	return b.String()
}

// ── Trigger de consulta ──────────────────────────────────────────────

var constraintTriggers = []string{
	"que restricciones tiene",
	"esto cumple con",
	"se puede cumplir con",
	"hay riesgo de no cumplir",
	"que requisitos estan en riesgo en",
}

// ParseConstraintQuery detecta si el usuario pregunta si el plan
// cumple sus requisitos — distinto de las cinco capas anteriores:
// ninguna de ellas volvía a mirar los Requirements que Goal Analyzer
// (Bloque 21) ya había detectado; esta es la primera que los cruza
// contra el resto del plan.
func ParseConstraintQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingConstraint)
}
