package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"
)

// ============================================================================
// SKILL_INTEGRATION.GO — Bloque 55, Fase 4.1 (cierre real del ciclo)
// ============================================================================
//
// Hasta acá, Curriculum Builder (47) y Skill Library (54) nunca se
// hablaron entre sí — Ahmelia podía tener una nota de estudio
// corroborada sobre Unreal Engine guardada en disco y, en la misma
// conversación, seguir diciendo "esto hay que aprenderlo de cero" para
// un objetivo de Unreal Engine, porque Curriculum Builder no sabía que
// esa nota existía. Skill Integration cierra ese hueco: antes de
// presentar un curriculum, pregunta si ya hay algo guardado sobre el
// dominio técnico completo del objetivo.
//
// LÍMITE HONESTO, más importante que el resto del bloque: esto NO
// filtra los LearningItem individuales del curriculum. Hacerlo
// requeriría cruzar cada gap puntual (ej. "Implementar la lógica de
// juego") contra un subtema técnico específico (ej. "Blueprints"), y
// esa granularidad no existe en ningún lado del proyecto todavía —
// Curriculum Builder arma subgoals de FASE genéricos (domainTemplates,
// goal_analyzer.go), no subtemas técnicos, y Knowledge Discovery (52)
// solo reconoce TECNOLOGÍAS completas (techDomains), no partes de
// ellas dentro de una tecnología. Inventar ese cruce fino sería
// fabricar una relación que no está fundamentada en los datos reales
// — el mismo límite honesto que Curriculum Builder (47) ya documentó
// sobre el árbol de prerrequisitos, aplicado acá a la integración con
// Skill Library.
//
// Lo que SÍ hace, con datos reales: detecta el dominio técnico
// completo del objetivo (mismo Discoverer de Bloque 52, sin
// reimplementar el matching de techDomains), consulta si Skill Library
// (54) ya tiene una nota guardada para ese dominio exacto, y lo
// reporta junto al curriculum sin tocar sus Items — la nota es
// referencia de estudio, no evidencia de que los gaps del plan ya
// están resueltos.
//
// Fuera de alcance a propósito, documentado acá para que quede
// explícito y no se asuma por descuido en un bloque futuro:
//   — Learning Planner (50) no se toca: sus KindLearn ya reflejan
//     Curriculum.Items tal cual, que este bloque no modifica.
//   — Knowledge Memory (49) no se toca: valida ActionConfidence
//     ESTADÍSTICA interna, un concepto sin relación real con notas de
//     estudio de Internet más allá del nombre parecido — mezclarlos
//     sería una coincidencia de nombres, no una necesidad
//     arquitectónica.

// SkillContext es el resultado de cruzar un Curriculum (47) contra
// Skill Library (54) para el dominio técnico completo del objetivo.
type SkillContext struct {
	Goal       string
	Domain     string // "" si Discovery no reconoció un dominio técnico
	KnownSkill *Skill // nil si no hay nota guardada para Domain
	Curriculum *Curriculum
}

// HasKnownSkill es true si ya hay una nota de estudio guardada para el
// dominio detectado — atajo booleano, mismo criterio que
// HasSources()/HasLead()/HasAnalogy() en el resto del proyecto.
func (sc *SkillContext) HasKnownSkill() bool {
	return sc.KnownSkill != nil
}

// Describe arma la narrativa completa: primero qué se sabe del
// dominio (nada reconocido / reconocido sin nota / reconocido con
// nota), después el curriculum tal cual — nunca se pisan entre sí, ni
// se insinúa que la nota resuelve los gaps del plan.
func (sc *SkillContext) Describe() string {
	var b strings.Builder

	switch {
	case sc.Domain == "":
		fmt.Fprintf(&b, "No reconozco un dominio técnico conocido para \"%s\" — no hay ninguna nota de estudio que consultar. ", sc.Goal)
	case sc.KnownSkill == nil:
		fmt.Fprintf(&b, "Detecté el dominio %s, pero todavía no tengo ninguna nota de estudio guardada sobre él. ", sc.Domain)
	default:
		// Integration Fix — Bloque 55, misma filosofía de siempre: NO
		// filtra LearningItems ni afirma que la nota resuelve gaps del
		// plan (ver límite honesto documentado arriba, sin cambios).
		// Lo que sí cambia: antes esto reconstruía a mano una versión
		// más pobre de la narrativa (solo Corroborated+Sources) — como
		// Skill.Describe() (54) ya construye la versión rica
		// (EffectiveLabel/State/Steps/Corroborated/Sources), reusarla
		// acá evita tener dos lugares narrando lo mismo de forma
		// distinta. Lo único que se agrega ACÁ, específico de cruzar
		// contra el curriculum, es la distinción Operations — cuántas
		// de las operaciones que el procedimiento necesita ya
		// reconoce Ahmelia — la misma pregunta "conocimiento vs.
		// capacidad respaldada" que motivó todo el rediseño del 54,
		// ahora visible también al integrar con el plan de
		// aprendizaje.
		var opNote string
		if sc.KnownSkill.HasProcedure() {
			wired, total := sc.KnownSkill.CapabilityCoverage()
			switch sc.KnownSkill.OperabilityLabel() {
			case "OperationsFullyMapped":
				opNote = fmt.Sprintf(" Las %d operación(es) que necesita ya están todas reconocidas por Ahmelia.", total)
			case "OperationsPartiallyMapped":
				opNote = fmt.Sprintf(" %d de %d operación(es) que necesita ya están reconocidas por Ahmelia — el resto todavía no.", wired, total)
			default: // KnowledgeOnly
				opNote = " Ninguna de sus operaciones está reconocida todavía por Ahmelia — es procedimiento, no algo que pueda ejecutar."
			}
		}
		fmt.Fprintf(&b, "Detecté el dominio %s: %s%s Esto es solo referencia de estudio — no reduce los gaps genuinos del plan, que siguen igual de pendientes. ",
			sc.Domain, sc.KnownSkill.Describe(), opNote)
	}

	if sc.Curriculum != nil {
		b.WriteString(sc.Curriculum.Describe())
	} else {
		fmt.Fprintf(&b, "No hay curriculum armado para \"%s\".", sc.Goal)
	}

	return b.String()
}

// SkillIntegrator cruza Curriculum (47) y Skill Library (54) usando
// Discoverer (52) para identificar el dominio — sin estado propio,
// mismo criterio que el resto de Fase 3/4.1.
type SkillIntegrator struct{}

// NewSkillIntegrator arma el integrador.
func NewSkillIntegrator() *SkillIntegrator {
	return &SkillIntegrator{}
}

// Integrate arma el SkillContext para un Curriculum ya construido.
// c, discoverer y skills pueden ser nil — defensivo, mismo criterio de
// todo el proyecto: sin datos, se reporta lo que falta, nunca se
// inventa un dominio o una nota que no existen.
func (si *SkillIntegrator) Integrate(c *Curriculum, discoverer *Discoverer, skills *SkillLibrary) *SkillContext {
	sc := &SkillContext{Curriculum: c}
	if c != nil {
		sc.Goal = c.Goal
	}
	if discoverer == nil || sc.Goal == "" {
		return sc
	}

	discovery := discoverer.Discover(sc.Goal)
	sc.Domain = discovery.Domain
	if sc.Domain == "" || skills == nil {
		return sc
	}

	if note, ok := skills.Get(sc.Domain); ok {
		sc.KnownSkill = note
	}
	return sc
}

// ── Trigger de consulta ──────────────────────────────────────────────

var skillContextTriggers = []string{
	"que te falta aprender realmente para",
	"actualiza el curriculum para",
	"revisa mi progreso de aprendizaje para",
}

// ParseSkillContextQuery detecta si se pide el curriculum YA cruzado
// contra lo que hay guardado en Skill Library — distinto de
// ParseCurriculumQuery (47, el curriculum crudo, sin consultar Skill
// Library) y de ParseSkillBuilderQuery (54, consulta la biblioteca
// sola, sin ningún curriculum de por medio).
func ParseSkillContextQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSkillContext)
}
