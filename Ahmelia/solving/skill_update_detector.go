package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"
)

// ============================================================================
// SKILL_UPDATE_DETECTOR.GO — Bloque 58, Fase 4.1
// ============================================================================
//
// Skill Builder (54) persiste una nota. Knowledge Retrieval (56) evita
// salir a buscar de nuevo si ya hay una guardada. La consecuencia,
// nunca antes señalada: una nota, una vez guardada, NUNCA se vuelve a
// comparar contra la fuente real — puede quedar desactualizada para
// siempre sin que nada lo detecte. Skill Update Detector es el primero
// que compara: vuelve a construir lo que SkillBuilder.Build() (54)
// armaría HOY con documentos frescos, y lo compara contra lo que ya
// está guardado.
//
// "Integrar, no crear": no agrega ningún mecanismo nuevo de fetch ni de
// comparación de hashes — reusa SkillBuilder.Build (54) tal cual para
// construir el candidato, y compara Content byte a byte contra la nota
// existente. Build() ya es determinístico (orden estable por Source,
// documentado en Bloque 54): dos fetches del mismo contenido real
// producen el mismo Content, así que una diferencia real SÍ significa
// que algo cambió en el origen, no es ruido de orden de iteración.
//
// LÍMITE HONESTO: esto NO actualiza la nota — solo DETECTA. Decidir
// qué hacer con una nota desactualizada (pisarla, guardar las dos
// versiones, avisar y esperar confirmación) es una decisión de
// política que este bloque no toma a propósito: eso es Skill
// Versioning (59) y Knowledge Conflict Resolver (60), que dependen de
// que esta detección exista primero — sin saber que algo cambió, nunca
// hay una "versión 2" que versionar ni un conflicto que resolver.

// UpdateCheck es el resultado de comparar una Skill guardada
// contra lo que se construiría con documentos frescos ahora mismo.
type UpdateCheck struct {
	SkillID   string
	Existing  *Skill
	Candidate *Skill // lo que Build() armaría HOY; nil si no hubo documentos confiables en este intento
	Changed   bool
	Reason    string

	// Delta — Integration Fix: antes, Changed solo decía SÍ/NO, sin
	// decir QUÉ cambió — un límite real para decidir algo con esa
	// información (¿son 2 pasos nuevos, o cambió el State entero de
	// "solo conocimiento" a "procedural"? son casos muy distintos).
	// Se computa comparando los campos estructurados de Existing vs
	// Candidate (54: Steps/Prerequisites/Pitfalls/FailureRecovery/
	// Operations/State) — no agrega ninguna comparación nueva, solo
	// describe con más detalle la MISMA diferencia que ya detectó
	// Content != Content. Vacío si Changed es false, o si Candidate es
	// nil.
	Delta string

	// StructuralChange — Integration Fix, Bloque 59: distingue "el
	// Content difiere" (Changed) de "y ese cambio movió al menos un
	// campo estructurado contable" — false cuando el único motivo del
	// Delta es la explicación de "probablemente texto reformulado sin
	// cambio de sustancia". Skill Versioning (59) usa esto para NO
	// archivar una versión nueva por cada reformulación cosmética de
	// la misma fuente — sin este campo, cualquier retoque de
	// redacción en el sitio original generaría una entrada de
	// historial idéntica en sustancia a la anterior, exactamente lo
	// que el propio 59 documenta que quiere evitar.
	StructuralChange bool
}

// NeedsUpdate es el atajo booleano — mismo patrón que IsReliable() (48,
// 53) / HasLead() (25) / HasAnalogy() (28): un solo booleano claro, sin
// que quien llame tenga que interpretar Reason a mano.
func (uc *UpdateCheck) NeedsUpdate() bool {
	return uc.Changed
}

// UpdateDetector es el módulo en sí. Sin estado propio, mismo criterio
// que el resto de Fase 3/4.
type UpdateDetector struct{}

// NewUpdateDetector arma el detector de cambios.
func NewUpdateDetector() *UpdateDetector {
	return &UpdateDetector{}
}

// Check compara existing contra lo que se construiría ahora mismo con
// vs (un ValidationSet recién calculado por Bloque 53 sobre un fetch
// nuevo). existing nil no rompe, pero significa que no hay nada contra
// qué comparar — eso es trabajo de Skill Builder (54), no de este
// bloque, así que NeedsUpdate() queda en false (no es "no cambió", es
// "no aplica"; el Reason distingue los dos casos).
func (ud *UpdateDetector) Check(item LearningItem, existing *Skill, vs *ValidationSet) *UpdateCheck {
	check := &UpdateCheck{SkillID: item.TaskID, Existing: existing}

	if existing == nil {
		check.Reason = "no hay ninguna nota guardada para este tema todavía — comparar no aplica, hace falta construirla primero."
		return check
	}

	builder := NewSkillBuilder()
	candidate := builder.Build(item, vs)
	check.Candidate = candidate

	if candidate == nil {
		check.Reason = "no encontré ningún documento confiable en este intento — no puedo confirmar si la fuente cambió, mantengo la nota guardada tal como está."
		return check
	}

	if candidate.Content != existing.Content {
		check.Changed = true
		check.Delta, check.StructuralChange = describeDelta(existing, candidate)
		check.Reason = "el contenido que traje ahora es distinto del que tenía guardado — la fuente pudo haber cambiado desde que armé la nota."
		return check
	}

	check.Reason = "el contenido que traje ahora coincide con lo que ya tenía guardado — sin cambios detectados."
	return check
}

// describeDelta compara los campos estructurados (54) de existing
// contra candidate y arma una lista corta de qué cambió — SOLO
// conteos y transiciones de estado, nunca el contenido en sí (eso ya
// vive en Candidate.Content para quien lo necesite). El segundo
// valor de retorno es false cuando NINGÚN campo estructurado contable
// varió pese a que Content sí cambió — Skill Versioning (59) lo usa
// para no archivar una versión nueva por una reformulación cosmética.
func describeDelta(existing, candidate *Skill) (string, bool) {
	var parts []string

	stateLabel := func(s SkillState) string {
		if s == "" {
			return "sin estructura reconocible"
		}
		return string(s)
	}
	if existing.State != candidate.State {
		parts = append(parts, fmt.Sprintf("estado: %s → %s", stateLabel(existing.State), stateLabel(candidate.State)))
	}
	if len(existing.Steps) != len(candidate.Steps) {
		parts = append(parts, fmt.Sprintf("pasos: %d → %d", len(existing.Steps), len(candidate.Steps)))
	}
	if len(existing.Prerequisites) != len(candidate.Prerequisites) {
		parts = append(parts, fmt.Sprintf("prerrequisitos: %d → %d", len(existing.Prerequisites), len(candidate.Prerequisites)))
	}
	if len(existing.Pitfalls) != len(candidate.Pitfalls) {
		parts = append(parts, fmt.Sprintf("advertencias: %d → %d", len(existing.Pitfalls), len(candidate.Pitfalls)))
	}
	if len(existing.FailureRecovery) != len(candidate.FailureRecovery) {
		parts = append(parts, fmt.Sprintf("recuperación de errores: %d → %d", len(existing.FailureRecovery), len(candidate.FailureRecovery)))
	}
	if len(existing.Operations) != len(candidate.Operations) {
		parts = append(parts, fmt.Sprintf("operaciones detectadas: %d → %d", len(existing.Operations), len(candidate.Operations)))
	}

	if len(parts) == 0 {
		return "el contenido crudo cambió, pero ningún campo estructurado contable varió (probablemente texto reformulado sin cambio de sustancia)", false
	}
	return strings.Join(parts, "; "), true
}

// Describe arma la narrativa del chequeo.
func (uc *UpdateCheck) Describe() string {
	if uc.Existing == nil {
		return fmt.Sprintf("%s: %s", uc.SkillID, uc.Reason)
	}
	status := "sin cambios"
	if uc.Changed {
		status = "cambió"
	}
	if uc.Changed && uc.Delta != "" {
		return fmt.Sprintf("\"%s\" (%s — %s): %s", uc.Existing.Label, status, uc.Delta, uc.Reason)
	}
	return fmt.Sprintf("\"%s\" (%s): %s", uc.Existing.Label, status, uc.Reason)
}

// ── Trigger de consulta ──────────────────────────────────────────────

var updateCheckTriggers = []string{
	"sigue actualizada la nota sobre",
	"cambio la fuente de informacion sobre",
	"detecta si cambio algo sobre",
	"la nota de estudio sigue vigente sobre",
}

// ParseUpdateCheckQuery detecta si se pregunta si una nota guardada
// sigue vigente — distinta de ParseSkillBuildQuery (54, que construye/
// pisa una nota sin comparar contra la anterior) y de
// ParseSkillContextQuery (55, que solo informa si hay una nota, sin
// volver a tocar la red).
func ParseUpdateCheckQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSkillUpdateCheck)
}
