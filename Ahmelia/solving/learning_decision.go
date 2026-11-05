package solving

import (
	"fmt"
	"strings"
)

// ============================================================================
// LEARNING_DECISION.GO — Bloque 71 (Learning Decision)
// ============================================================================
//
// Motivación: Evitar la reacción impulsiva de navegar por Internet ante
// cualquier falta de respuesta. Ante un objetivo, el agente debe evaluar
// conscientemente si realmente corresponde aprender, si alcanza con su memoria
// local, si no tiene la capacidad técnica de software para ejecutar el resultado,
// o si el riesgo de la acción exige confirmación humana antes de investigar.
//
// Reglas de Decisión:
// 1. Si no hay conexión o se exige modo estricto offline -> UseMemory.
// 2. Si la acción/operación local NO existe (CapabilityGap) -> ReportCapabilityGap
//    (no tiene sentido buscar tutoriales de algo que Go no sabe ejecutar).
// 3. Si requiere acciones destructivas o de alto impacto -> RequestConfirmation.
// 4. Si hay un KnowledgeGap, la red está disponible y la investigación vale la pena -> ExecuteResearch.
// 5. Si la información existente es suficiente o el costo de aprender supera el beneficio -> UseMemory.

type DecisionType string

const (
	// DecisionUseMemory — Usar la memoria/conocimiento existente. No ir a red.
	DecisionUseMemory DecisionType = "usar_memoria"

	// DecisionExecuteResearch — Ir a Internet a investigar o aprender la Skill.
	DecisionExecuteResearch DecisionType = "investigar_red"

	// DecisionReportCapabilityGap — Informar que el motor no soporta la acción técnica.
	DecisionReportCapabilityGap DecisionType = "reportar_falta_capacidad"

	// DecisionRequestConfirmation — Acción de alto riesgo. Detenerse y pedir confirmación.
	DecisionRequestConfirmation DecisionType = "solicitar_confirmacion"
)

// LearningContext agrupa las variables de entorno, brechas y configuración de red.
type LearningContext struct {
	Action               string
	IsOnline             bool
	IsHighRisk           bool
	WorthLearning        bool // Evaluación de costo/beneficio o relevancia
	Gap                  *GapAnalysis
	HasRelatedSkill      bool
	HasEnoughInformation bool
}

// LearningDecision es el dictamen estructurado de B71.
type LearningDecision struct {
	Type       DecisionType
	Action     string
	Reason     string
	Actionable string
}

// Decide evaluate la situación y emite un fallo estricto sobre cómo proceder.
func Decide(ctx LearningContext) *LearningDecision {
	res := &LearningDecision{Action: ctx.Action}

	// Fail-safe / Cero supuestos: Si no hay análisis de brecha (Gap), no asumimos nada.
	if ctx.Gap == nil {
		res.Type = DecisionUseMemory
		res.Reason = "no se proporcionó un análisis de brecha (Gap nil); se recurre al comportamiento local seguro"
		res.Actionable = "Proceder únicamente con memoria local o solicitar re-evaluación de brechas."
		return res
	}

	// 1. CAPABILITY GAP: Si el motor de Go no tiene la función programada,
	// aprender de Internet no soluciona el problema de ejecución.
	if ctx.Gap.Type == CapabilityGap {
		res.Type = DecisionReportCapabilityGap
		res.Reason = fmt.Sprintf("la acción %q requiere una capacidad en el código local que el motor aún no tiene", ctx.Action)
		res.Actionable = "Informar al desarrollador o usuario que se requiere implementar soporte técnico en Go."
		return res
	}

	// 2. RIESGO ALTO: Si involucra operaciones peligrosas, se debe pedir confirmación antes de cualquier paso.
	if ctx.IsHighRisk {
		res.Type = DecisionRequestConfirmation
		res.Reason = fmt.Sprintf("la acción %q implica un impacto o riesgo elevado en el sistema", ctx.Action)
		res.Actionable = "Pedir confirmación explícita al usuario antes de proceder a investigar o ejecutar."
		return res
	}

	// 3. MODO OFFLINE: Si no hay Internet, forzosamente se usa la memoria disponible.
	if !ctx.IsOnline {
		res.Type = DecisionUseMemory
		res.Reason = "el entorno está offline o sin conectividad disponible"
		res.Actionable = "Resolver la solicitud utilizando únicamente la memoria y Skills registradas previamente."
		return res
	}

	// 4. KNOWLEDGE GAP + ONLINE + CUMPLE CRITERIOS DE APRENDIZAJE
	if ctx.Gap.Type == KnowledgeGap {
		if !ctx.WorthLearning {
			res.Type = DecisionUseMemory
			res.Reason = "falta conocimiento, pero la evaluación determinó que el aprendizaje no vale la pena o es irrelevante"
			res.Actionable = "Responder con la mejor aproximación disponible en memoria sin consultar red."
			return res
		}

		res.Type = DecisionExecuteResearch
		res.Reason = "falta la Skill/conocimiento, hay red disponible y el aprendizaje aporta valor directo al objetivo"
		res.Actionable = "Navegar o consultar fuentes externas para adquirir el procedimiento y registrar la nueva Skill."
		return res
	}

	// 5. RESOURCE GAP / SIN GAP / OTROS: Se intenta proceder localmente o resolver el recurso.
	if ctx.HasEnoughInformation || ctx.Gap.Type == GapNone {
		res.Type = DecisionUseMemory
		res.Reason = "existe suficiente conocimiento local y capacidad para completar la tarea"
		res.Actionable = "Ejecutar directamente usando la memoria y los recursos locales disponibles."
		return res
	}

	// Default seguro
	res.Type = DecisionUseMemory
	res.Reason = "no se cumplieron las condiciones para investigación externa"
	res.Actionable = "Usar memoria interna."
	return res
}

// Describe genera un resumen claro para auditoría en texto.
func (d *LearningDecision) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Decisión [%s]: %s. Recomendación: %s", d.Type, d.Reason, d.Actionable)
	return b.String()
}
