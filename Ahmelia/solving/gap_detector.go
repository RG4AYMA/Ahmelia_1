package solving

import (
	"fmt"
	"strings"
)

// ============================================================================
// GAP_DETECTOR.GO — Bloque 70 (Gap Detection)
// ============================================================================
//
// Motivación: Durante el cumplimiento de un objetivo ("Necesito hacer X"),
// Ahmelia puede fallar o detenerse por motivos fundamentalmente distintos.
// Sin este bloque, el sistema corre el riesgo de intentar "aprender de Internet"
// (Knowledge) un procedimiento cuando en realidad falta código local en el motor
// (Capability) o faltan binarios/permisos/archivos en el SO (Resource).
//
// Distinción de la taxonomía:
// 1. KNOWLEDGE GAP: No existe una Skill o procedimiento conocido para el objetivo.
//    → Acción: Aprender / Buscar tutorial / Registrar nueva Skill.
// 2. CAPABILITY GAP: Se conoce el procedimiento/acción, pero el motor ejecutor
//    no tiene cableada la función interna (Handler/Action) para realizarlo.
//    → Acción: Desarrollar/conectar código localmente en el motor de Go.
// 3. RESOURCE GAP: La Skill existe y la capacidad local está disponible, pero
//    falta un prerrequisito del entorno (archivo inexistente, app no instalada,
//    falta de permisos de sistema).
//    → Acción: Resolver requisito previo / Pedir intervención al entorno.

type GapType string

const (
	// GapNone — No hay brecha; el objetivo o acción es ejecutable con el estado actual.
	GapNone GapType = "ninguno"

	// KnowledgeGap — "No sé cómo hacerlo". No hay Skill registrada para la meta.
	KnowledgeGap GapType = "knowledge_gap"

	// CapabilityGap — "Sé cómo hacerlo pero el sistema no puede ejecutarlo".
	// La Skill/Acción no está soportada por el ejecutor de acciones locales.
	CapabilityGap GapType = "capability_gap"

	// ResourceGap — "Sé y puedo, pero falta un recurso previo".
	// Fallan validaciones de archivos, binarios externos o permisos del sistema.
	ResourceGap GapType = "resource_gap"
)

// GapAnalysis es el diagnóstico estricto de la brecha detectada antes o durante
// la ejecución de un objetivo.
type GapAnalysis struct {
	Type        GapType
	Action      string
	Description string
	Resolution  string
}

// HasGap confirma si existe alguna traba que impida la ejecución inmediata.
func (g *GapAnalysis) HasGap() bool {
	return g.Type != GapNone
}

// SystemCapabilities define la interfaz o contrato con el que comprobamos
// el estado del motor y del entorno sin acoplar innecesariamente este paquete.
type SystemCapabilities interface {
	HasSkill(action string) bool
	HasExecutableAction(action string) bool
	HasResource(resourceKey string) (bool, string) // Devuelve existencia y razón de fallo si aplica
}

// DetectGap evalúa la intención o acción solicitada frente a las capacidades,
// conocimientos y recursos disponibles en el sistema.
func DetectGap(action string, requiredResources []string, sys SystemCapabilities) *GapAnalysis {
	res := &GapAnalysis{Action: action}

	// 1. Evaluamos KNOWLEDGE GAP ("No sé cómo hacerlo")
	// Si no se tiene un registro conceptual o Skill sobre la acción.
	if sys != nil && !sys.HasSkill(action) {
		res.Type = KnowledgeGap
		res.Description = fmt.Sprintf("no existe una Skill o conocimiento registrado para la acción %q", action)
		res.Resolution = "Aprender: Buscar documentación o registrar un nuevo procedimiento/tutorial para esta acción."
		return res
	}

	// 2. Evaluamos CAPABILITY GAP ("Sé cómo hacerlo pero no puedo")
	// Existe el concepto/Skill, pero el motor local no tiene el Handler/Action implementado en Go.
	if sys != nil && !sys.HasExecutableAction(action) {
		res.Type = CapabilityGap
		res.Description = fmt.Sprintf("se reconoce la Skill %q, pero el motor local no tiene la capacidad técnica/función para ejecutarla", action)
		res.Resolution = "Implementar: Conectar o programar el handler/operación local en el código fuente de Go."
		return res
	}

	// 3. Evaluamos RESOURCE GAP ("Sé y puedo, pero falta un programa/archivo/permiso")
	// La Skill existe y la capacidad técnica está escrita, pero faltan elementos del entorno.
	if sys != nil {
		for _, resource := range requiredResources {
			if ok, reason := sys.HasResource(resource); !ok {
				res.Type = ResourceGap
				res.Description = fmt.Sprintf("la capacidad existe, pero falta el recurso %q: %s", resource, reason)
				res.Resolution = "Resolver requisito: Verificar la existencia del archivo, instalar la dependencia o solicitar permisos."
				return res
			}
		}
	}

	// Sin brechas detectadas
	res.Type = GapNone
	res.Description = fmt.Sprintf("acción %q completamente lista para ejecución", action)
	res.Resolution = "Ninguna: Proceder a la ejecución."
	return res
}

// Describe genera una representación clara en prosa para fines de auditoría o logging.
func (g *GapAnalysis) Describe() string {
	var b strings.Builder
	switch g.Type {
	case KnowledgeGap:
		fmt.Fprintf(&b, "[KNOWLEDGE GAP] %s. Sugerencia: %s", g.Description, g.Resolution)
	case CapabilityGap:
		fmt.Fprintf(&b, "[CAPABILITY GAP] %s. Sugerencia: %s", g.Description, g.Resolution)
	case ResourceGap:
		fmt.Fprintf(&b, "[RESOURCE GAP] %s. Sugerencia: %s", g.Description, g.Resolution)
	default:
		fmt.Fprintf(&b, "[SIN GAP] %s.", g.Description)
	}
	return b.String()
}
