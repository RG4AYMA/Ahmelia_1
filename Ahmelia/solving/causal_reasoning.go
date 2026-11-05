package solving

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
)

// ============================================================================
// CAUSAL_REASONING.GO — Bloque 43, Fase 3 Profundizada (Nivel III — Executive Reasoning)
// ============================================================================
//
// ACLARACIÓN IMPORTANTE, decidida antes de escribir esto: DependencyGraph
// (Bloque 24) es LINEAL — Order es una secuencia, no un árbol de causas
// ramificado. Cada Task bloqueada apunta a UN SOLO gap causante (el más
// cercano, BlockedByID), no a una cadena de varios pasos. No hay
// "cadenas causales multi-hop" que rastrear con este modelo de datos —
// pretender que las hay sería inventar estructura que no existe.
//
// Lo que sí es genuinamente nuevo, construido sobre la misma relación
// causa-efecto que Bloque 24 ya calculaba sin explotar del todo:
//
//   - ExplainBlock: la causa de UNA Task puntual, en lenguaje de
//     causa-efecto. Describe() (Bloque 24) da la vista de TODO el plan
//     de una — no permite preguntar por una Task específica.
//   - ProjectResolution: el efecto de resolver un gap puntual — "si
//     resuelvo X, ¿qué tareas dejan de estar bloqueadas?". Esto no
//     existía en ningún lado: Bloque 24 nunca simula un "y si...".
//
// RootGaps() (Bloque 24) ya identificaba las causas raíz — no se
// duplica acá, se reusa tal cual dentro de ProjectAllGaps.

// CausalLink es la explicación causa-efecto de UNA Task.
type CausalLink struct {
	TaskID    string
	TaskDesc  string
	IsBlocked bool
	CauseID   string // TaskID del gap causante; "" si IsBlocked==false
	CauseDesc string
}

// Describe arma la narrativa de un solo link causal.
func (c *CausalLink) Describe() string {
	if !c.IsBlocked {
		return fmt.Sprintf("%s (%s) no está bloqueada — nada la causa.", c.TaskID, c.TaskDesc)
	}
	return fmt.Sprintf("%s (%s) está bloqueada porque %s (%s) es territorio nuevo — resolvé esa primero.",
		c.TaskID, c.TaskDesc, c.CauseID, c.CauseDesc)
}

// CausalProjection es el efecto de resolver UN gap puntual.
type CausalProjection struct {
	GapID    string
	GapDesc  string
	Unblocks []*DependencyNode // Tasks que dejarían de estar bloqueadas si GapID se resolviera
}

// Describe arma la narrativa de una proyección puntual.
func (p *CausalProjection) Describe() string {
	if len(p.Unblocks) == 0 {
		return fmt.Sprintf("Resolver %s (%s) no desbloquearía ninguna otra tarea en este plan.", p.GapID, p.GapDesc)
	}
	names := make([]string, len(p.Unblocks))
	for i, n := range p.Unblocks {
		names[i] = n.TaskDesc
	}
	return fmt.Sprintf("Si resuelvo %s (%s), se desbloquean %d tarea(s): %s.",
		p.GapID, p.GapDesc, len(p.Unblocks), strings.Join(names, ", "))
}

// CausalReasoner es el Causal Reasoning en sí. Sin estado propio, mismo
// criterio que el resto de Fase 3.
type CausalReasoner struct{}

// NewCausalReasoner arma el Causal Reasoner.
func NewCausalReasoner() *CausalReasoner {
	return &CausalReasoner{}
}

// ExplainBlock arma la explicación causa-efecto de UNA Task, buscada
// por TaskID dentro de dg. dg nil, o taskID que no existe en el grafo,
// devuelve nil — no inventa una explicación sobre algo que no está.
func (cr *CausalReasoner) ExplainBlock(dg *DependencyGraph, taskID string) *CausalLink {
	if dg == nil {
		return nil
	}
	node, ok := dg.Nodes[taskID]
	if !ok {
		return nil
	}

	link := &CausalLink{TaskID: node.TaskID, TaskDesc: node.TaskDesc, IsBlocked: node.Blocked}
	if node.Blocked {
		link.CauseID = node.BlockedByID
		if cause, ok := dg.Nodes[node.BlockedByID]; ok {
			link.CauseDesc = cause.TaskDesc
		}
	}
	return link
}

// ExplainAllBlocks arma un CausalLink por cada Task bloqueada del plan
// — para "por qué está bloqueado esto" sin nombrar una Task puntual.
func (cr *CausalReasoner) ExplainAllBlocks(dg *DependencyGraph) []*CausalLink {
	if dg == nil {
		return nil
	}
	var out []*CausalLink
	for _, node := range dg.Blocked() {
		out = append(out, cr.ExplainBlock(dg, node.TaskID))
	}
	return out
}

// ProjectResolution simula resolver UN gap puntual: qué Tasks tienen
// BlockedByID == gapID hoy, y por lo tanto dejarían de estarlo si ese
// gap se resolviera. dg nil, o gapID que no es un gap real, -> nil.
func (cr *CausalReasoner) ProjectResolution(dg *DependencyGraph, gapID string) *CausalProjection {
	if dg == nil {
		return nil
	}
	gapNode, ok := dg.Nodes[gapID]
	if !ok || !gapNode.IsGap {
		return nil
	}

	proj := &CausalProjection{GapID: gapID, GapDesc: gapNode.TaskDesc}
	for _, id := range dg.Order {
		node := dg.Nodes[id]
		if node.Blocked && node.BlockedByID == gapID {
			proj.Unblocks = append(proj.Unblocks, node)
		}
	}
	return proj
}

// ProjectAllGaps arma un CausalProjection por cada gap raíz del plan
// (RootGaps(), Bloque 24) — "si resolviera cada problema por separado,
// ¿cuál conviene resolver primero?".
func (cr *CausalReasoner) ProjectAllGaps(dg *DependencyGraph) []*CausalProjection {
	if dg == nil {
		return nil
	}
	var out []*CausalProjection
	for _, gap := range dg.RootGaps() {
		out = append(out, cr.ProjectResolution(dg, gap.TaskID))
	}
	return out
}

// DescribeProjections arma la narrativa conjunta de varios
// CausalProjection, ordenados de MAYOR a menor impacto (más tareas
// desbloqueadas primero) — responde directo "cuál conviene resolver
// primero" sin que quien pregunte tenga que comparar números a mano.
// Orden estable: dos gaps con el mismo impacto quedan en el orden en
// que RootGaps() los entregó (el orden secuencial del plan).
func DescribeProjections(projections []*CausalProjection) string {
	if len(projections) == 0 {
		return "No hay ningún gap en este plan para proyectar."
	}

	sorted := make([]*CausalProjection, len(projections))
	copy(sorted, projections)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Unblocks) > len(sorted[j].Unblocks)
	})

	parts := make([]string, len(sorted))
	for i, p := range sorted {
		parts[i] = p.Describe()
	}
	return strings.Join(parts, " ")
}

// DescribeLinks arma la narrativa conjunta de varios CausalLink.
func DescribeLinks(links []*CausalLink) string {
	if len(links) == 0 {
		return "No hay ninguna tarea bloqueada en este plan."
	}
	parts := make([]string, len(links))
	for i, l := range links {
		parts[i] = l.Describe()
	}
	return strings.Join(parts, " ")
}

// ── Triggers de consulta ─────────────────────────────────────────────
// Dos preguntas distintas, dos triggers — mismo criterio que
// separar Comparator (Bloque 38) de Strategy Selector (Bloque 39):
// "explicar" y "proyectar" son operaciones distintas, no una sola con
// dos salidas.

var causalExplainTriggers = []string{
	"por que esta bloqueada",
	"que causa el bloqueo",
	"cual es la causa de esto",
}

var causalProjectTriggers = []string{
	"que se desbloquearia si resuelvo",
	"cual gap conviene resolver primero",
	"que pasaria si resuelvo",
}

// ParseCausalExplainQuery detecta si se pregunta por la CAUSA de un
// bloqueo ya existente.
func ParseCausalExplainQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingCausalExplain)
}

// ParseCausalProjectQuery detecta si se pregunta por el EFECTO de
// resolver algo que todavía no se resolvió.
func ParseCausalProjectQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingCausalProject)
}
