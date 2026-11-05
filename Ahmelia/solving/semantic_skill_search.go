package solving

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"
)

// ============================================================================
// SEMANTIC_SKILL_SEARCH.GO — Bloque 57, Fase 4.1
// ============================================================================
//
// Hasta acá, encontrar una nota en Skill Library (54) requiere saber
// la clave EXACTA: el Domain tal cual lo arma Discoverer (52) —
// "Unity", no "motor de videojuegos", no "renderizado". Si alguien
// pregunta "¿qué sabes sobre renderizado?", `SkillLibrary.Get("renderizado")`
// no encuentra nada, aunque Ahmelia sí tenga guardada una nota sobre
// Unity cuyo contenido real habla extensamente de rendering.
//
// LÍMITE HONESTO, el más importante de todo el bloque: esto NO es
// búsqueda semántica de verdad. No hay embeddings, no hay un modelo
// de lenguaje calculando similitud de significado — ni acá ni en
// ningún otro punto del proyecto, mismo criterio 100% determinista
// que rige desde el Bloque 1. Lo que hace es más simple y 100% real:
// cuenta cuántas veces aparece literalmente el término buscado
// (case-insensitive) dentro del Content y el Label de cada Skill
// ya guardada — el mismo texto que se fetcheó de verdad de una fuente
// autorizada (Gateway, 51) y se validó (53). Si la nota sobre Unity
// menciona "rendering" varias veces, buscar "rendering" la encuentra
// y la prioriza. Buscar "renderizado" (español) NO la encuentra si el
// contenido fetcheado está en inglés — no hay traducción ni
// normalización de idioma en ningún punto de este bloque. Documentar
// esto acá, en vez de fingir que funciona en cualquier idioma o que
// entiende sinónimos, es la misma honestidad que ya aplicó Bloque 53
// sobre no inventar fecha de publicación, y Bloque 56 sobre no
// inventar vencimiento de una nota.
//
// El nombre del bloque ("Semantic") describe la ASPIRACIÓN del
// roadmap, no lo que el código hace hoy — igual que "Causal
// Reasoning" (43) no rastrea cadenas multi-hop o "Learning Validator"
// (48) no entiende el dominio de lo que valida. Si en algún momento
// existe un componente real de embeddings, este archivo es el punto
// de reemplazo — la firma de Search() no debería necesitar cambiar,
// solo la implementación interna.

// SkillMatch es una nota que matcheó una búsqueda, con cuántas veces
// apareció el término — permite ordenar por relevancia REAL (conteo
// real de apariciones en texto real), no por un score inventado.
type SkillMatch struct {
	Domain string
	Note   *Skill
	Hits   int
}

// SkillSearchResult es el resultado completo de una búsqueda.
type SkillSearchResult struct {
	Query   string
	Matches []SkillMatch // ordenados por Hits desc, después por Domain para estabilidad
}

// HasMatches es true si la búsqueda encontró al menos una nota.
func (sr *SkillSearchResult) HasMatches() bool {
	return len(sr.Matches) > 0
}

// Describe arma la narrativa del resultado.
func (sr *SkillSearchResult) Describe() string {
	if !sr.HasMatches() {
		return fmt.Sprintf("No encontré ninguna nota guardada que mencione \"%s\".", sr.Query)
	}
	var parts []string
	for _, m := range sr.Matches {
		// Integration Fix — EffectiveLabel() (54) en vez de Domain
		// crudo: si la Skill tiene ProcedureTitle (el texto fuente
		// nombraba la tarea concreta), la lista ya lo muestra en vez
		// del dominio genérico — mismo criterio que Skill.Describe() y
		// SkillContext.Describe() (55) ya aplican.
		//
		// Note nil-safe a propósito: Search() (más abajo) siempre lo
		// puebla en uso real (salta explícitamente note == nil antes
		// de agregar un match), pero Describe() no debería asumirlo —
		// mismo compromiso de "nunca panics" que ya documenta Search().
		label := m.Domain
		if m.Note != nil {
			label = m.Note.EffectiveLabel()
		}
		parts = append(parts, fmt.Sprintf("%s (%d mención(es))", label, m.Hits))
	}
	return fmt.Sprintf("Encontré %d nota(s) que mencionan \"%s\": %s.",
		len(sr.Matches), sr.Query, strings.Join(parts, ", "))
}

// SemanticSkillSearcher busca sobre el contenido ya guardado en Skill
// Library (54). Sin estado propio — la biblioteca real vive en
// SkillLibrary, esto solo la recorre.
type SemanticSkillSearcher struct{}

// NewSemanticSkillSearcher arma el buscador.
func NewSemanticSkillSearcher() *SemanticSkillSearcher {
	return &SemanticSkillSearcher{}
}

// Search busca query (literal, case-insensitive, sin normalizar
// idioma) en el Content y el Label de cada nota guardada. query vacío
// o skills nil → resultado sin matches, defensivo — nunca panics.
func (s *SemanticSkillSearcher) Search(query string, skills *SkillLibrary) *SkillSearchResult {
	result := &SkillSearchResult{Query: query}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || skills == nil {
		return result
	}

	for domain, note := range skills.Skills {
		if note == nil {
			continue
		}
		hits := strings.Count(strings.ToLower(note.Content), q) + strings.Count(strings.ToLower(note.Label), q)
		if hits > 0 {
			result.Matches = append(result.Matches, SkillMatch{Domain: domain, Note: note, Hits: hits})
		}
	}

	// Orden estable: más menciones primero; a igual conteo, alfabético
	// por Domain — mismo criterio de determinismo que Skill Builder
	// (54) usa para no depender del orden de iteración de un map.
	sort.Slice(result.Matches, func(i, j int) bool {
		if result.Matches[i].Hits != result.Matches[j].Hits {
			return result.Matches[i].Hits > result.Matches[j].Hits
		}
		return result.Matches[i].Domain < result.Matches[j].Domain
	})

	return result
}

// ── Trigger de consulta ──────────────────────────────────────────────

var semanticSkillSearchTriggers = []string{
	"que sabes sobre",
	"que notas mencionan",
	"en que apuntes aparece",
	"donde mencione",
	// Integration Fix — "sabes usar X"/"sabes hacer X" es la forma
	// natural en que alguien pregunta por una nota ya guardada
	// ("¿sabes usar Blender?"), pero antes solo "que sabes sobre X"
	// disparaba la búsqueda — un fraseo igual de común quedaba sin
	// cubrir y caía en el fallback genérico de confianza baja (mismo
	// camino que una acción no registrada en shared.ActionPatterns).
	// Esto es una mejora de MATCHING de triggers, no una capacidad
	// nueva — ver la nota de límite honesto más abajo.
	"sabes usar",
	"sabes hacer",
}

// ParseSemanticSkillSearchQuery detecta si se pregunta por notas que
// mencionen un término — distinto de ParseSkillBuilderQuery (54, lista
// TODA la biblioteca) y de ParseSkillContextQuery (55, cruza un
// curriculum específico contra un dominio exacto).
func ParseSemanticSkillSearchQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSemanticSkillSearch)
}
