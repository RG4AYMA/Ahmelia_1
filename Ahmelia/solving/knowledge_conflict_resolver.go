package solving

import (
	"ahmelia/interaction"
	"fmt"
	"sort"
	"strings"

	"ahmelia/shared"
)

// ============================================================================
// KNOWLEDGE_CONFLICT_RESOLVER.GO — Bloque 60, Fase 4.1 (cierra la Fase 4)
// ============================================================================
//
// ValidationSet.Corroborated (Bloque 53) hoy solo confirma que 2+
// fuentes AUTORIZADAS DISTINTAS resultaron confiables — HTTP 2xx y
// contenido real tras normalizar. Nunca compara SI ESE CONTENIDO DICE
// LO MISMO. Dos documentos pueden estar "corroborados" en el sentido
// de Bloque 53 y, aun así, hablar de aspectos distintos del tema o
// directamente contradecirse — el sistema, hasta este bloque, no tenía
// forma de distinguir eso de una coincidencia real. Knowledge Conflict
// Resolver es el primero que compara CONTENIDO entre fuentes, no solo
// su fiabilidad individual.
//
// "Integrar, no crear": reusa shared.ExtractKeywords (ya usado por
// goal_analyzer.go, knowledge_mapper.go, hypothesis_generator.go,
// problem_decomposer.go, constraint_solver.go) para tokenizar y
// filtrar stopwords, en vez de reimplementar esa normalización otra
// vez — shared/keywords.go ya documenta por qué existe un solo lugar
// para eso.
//
// LÍMITE HONESTO, el que importa de todo el bloque: esto NO entiende
// SIGNIFICADO. No hay forma real de que Ahmelia sepa si "requiere
// Windows 10" y "requiere Windows 11" se CONTRADICEN semánticamente —
// eso necesitaría comprensión de lenguaje que este proyecto, 100%
// determinista desde el Bloque 1, no tiene ni finge tener (mismo
// límite que Semantic Skill Search, 57, ya documentó sobre "Semantic"
// siendo la ASPIRACIÓN del nombre, no lo que el código hace hoy). Lo
// que SÍ es medible y real: superposición de palabras significativas
// (índice de Jaccard, sobre keywords ya filtradas de stopwords) entre
// el Content de cada par de fuentes confiables. Poca superposición es
// una SEÑAL de posible divergencia — nunca una prueba de contradicción;
// puede ser, con la misma probabilidad, que las dos fuentes simplemente
// cubran aspectos distintos del mismo tema. Por eso, tal como pedía el
// roadmap original ("decide cuál conservar o marcar para revisión"),
// este bloque SOLO hace la segunda mitad: marca para revisión manual.
// Nunca descarta ni prioriza una fuente en automático — eso exigiría
// un juicio de verdad que Ahmelia no puede fundamentar con lo que
// tiene. Con esto se cierra la Fase 4.1 completa: Discover (52) →
// Fetch (51) → Validate (53) → Build (54) → Integrate (55) →
// Retrieve (56) → Search (57) → Detect Update (58) → Version (59) →
// Resolve Conflict (60).

// ConflictThreshold es el piso de superposición de keywords por debajo
// del cual dos fuentes confiables se marcan como divergentes — mismo
// criterio de umbral medible y documentado que MinContentLength (53),
// MinSampleSize (48) y MaxNoteContentLength (54): un punto de partida
// razonable, no una ciencia exacta. 0.15 significa que comparten menos
// del 15% de sus palabras significativas combinadas (unión de Jaccard).
const ConflictThreshold = 0.15

// SourcePairComparison es la comparación de contenido entre dos
// fuentes confiables y distintas para el mismo objetivo.
type SourcePairComparison struct {
	SourceA, SourceB string
	Overlap          float64 // índice de Jaccard sobre keywords, 0.0–1.0
	Diverges         bool    // true si Overlap < ConflictThreshold
}

// ConflictReport es el resultado de comparar TODAS las fuentes
// confiables entre sí, par a par, para un mismo objetivo.
type ConflictReport struct {
	Goal    string
	Pairs   []SourcePairComparison
	Applies bool // false si hubo menos de 2 fuentes confiables y distintas para comparar
}

// HasConflict es true si algún par divergió — atajo booleano, mismo
// criterio que Corroborated/IsReliable()/HasLead()/NeedsUpdate() en el
// resto del proyecto: un solo booleano claro, sin que quien llame
// tenga que recorrer Pairs a mano.
func (cr *ConflictReport) HasConflict() bool {
	for _, p := range cr.Pairs {
		if p.Diverges {
			return true
		}
	}
	return false
}

// Describe arma la narrativa del reporte — tres casos, mismo criterio
// de "no mezclar categorías distintas" que el resto de Fase 4.1.
func (cr *ConflictReport) Describe() string {
	if !cr.Applies {
		return fmt.Sprintf("No hay al menos dos fuentes confiables y distintas sobre \"%s\" — comparar contenido entre fuentes no aplica todavía.", cr.Goal)
	}
	if !cr.HasConflict() {
		return fmt.Sprintf("Comparé %d par(es) de fuentes confiables sobre \"%s\": todas comparten vocabulario suficiente, sin señales de divergencia.", len(cr.Pairs), cr.Goal)
	}
	var diverging []string
	for _, p := range cr.Pairs {
		if p.Diverges {
			diverging = append(diverging, fmt.Sprintf("%s vs %s (%.0f%% de superposición)", p.SourceA, p.SourceB, p.Overlap*100))
		}
	}
	return fmt.Sprintf("Divergencia detectada sobre \"%s\" entre: %s — no decido cuál fuente es correcta, esto queda marcado para revisión manual.", cr.Goal, strings.Join(diverging, "; "))
}

// ConflictResolver compara el Content de las fuentes confiables de un
// ValidationSet (53) entre sí. Sin estado propio, mismo criterio que
// el resto de Fase 4.1.
type ConflictResolver struct{}

// NewConflictResolver arma el resolvedor de conflictos.
func NewConflictResolver() *ConflictResolver {
	return &ConflictResolver{}
}

// Resolve compara par a par el Content de cada fuente CONFIABLE Y
// DISTINTA — mismo criterio de "una entrada por Source" que
// SkillBuilder.Build (54): dos documentos de la misma fuente no se
// comparan entre sí, es la misma fuente hablando dos veces, no una
// segunda opinión. vs nil o con menos de 2 fuentes confiables →
// Applies=false, sin inventar una comparación que no tiene sentido.
func (cr *ConflictResolver) Resolve(vs *ValidationSet) *ConflictReport {
	report := &ConflictReport{}
	if vs == nil {
		return report
	}
	report.Goal = vs.Goal

	type sourceDoc struct{ source, text string }
	seen := make(map[string]bool)
	var docs []sourceDoc
	for _, v := range vs.Validations {
		if v == nil || !v.IsReliable() || seen[v.Document.Source] {
			continue
		}
		seen[v.Document.Source] = true
		docs = append(docs, sourceDoc{source: v.Document.Source, text: v.Document.Text})
	}

	// Orden estable por Source — mismo criterio de determinismo que
	// todo Fase 4.1: la salida no depende del orden en que FetchAll
	// devolvió los documentos.
	sort.Slice(docs, func(i, j int) bool { return docs[i].source < docs[j].source })

	if len(docs) < 2 {
		return report
	}
	report.Applies = true

	for i := 0; i < len(docs); i++ {
		for j := i + 1; j < len(docs); j++ {
			overlap := jaccardOverlap(docs[i].text, docs[j].text)
			report.Pairs = append(report.Pairs, SourcePairComparison{
				SourceA:  docs[i].source,
				SourceB:  docs[j].source,
				Overlap:  overlap,
				Diverges: overlap < ConflictThreshold,
			})
		}
	}

	return report
}

// jaccardOverlap mide qué proporción de las palabras significativas
// combinadas de a y b son compartidas — 1.0 si comparten todo su
// vocabulario significativo, 0.0 si no comparten ninguna palabra.
// Usa shared.ExtractKeywords (mismo tokenizador y stopwords que el
// resto del proyecto) para no reimplementar normalización acá.
func jaccardOverlap(a, b string) float64 {
	setA := toWordSet(shared.ExtractKeywords(a))
	setB := toWordSet(shared.ExtractKeywords(b))
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}

	sharedCount := 0
	for w := range setA {
		if setB[w] {
			sharedCount++
		}
	}
	union := len(setA) + len(setB) - sharedCount
	if union == 0 {
		return 0
	}
	return float64(sharedCount) / float64(union)
}

// toWordSet deduplica una lista de keywords en un set — ExtractKeywords
// no deduplica (puede repetir la misma palabra si aparece más de una
// vez en el texto), y Jaccard necesita conjuntos, no listas con
// repetición.
func toWordSet(words []string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// ── Trigger de consulta ──────────────────────────────────────────────

var conflictTriggers = []string{
	"hay conflicto entre las fuentes sobre",
	"las fuentes se contradicen sobre",
	"necesita revision manual la informacion sobre",
}

// ParseConflictQuery detecta si se pregunta si las fuentes disponibles
// sobre un objetivo se contradicen entre sí — distinta de
// ParseSourceValidationQuery (53, evalúa CONFIABILIDAD individual, no
// compara contenido entre fuentes) y de ParseUpdateCheckQuery (58, que
// compara una fuente contra SU PROPIA nota guardada en el tiempo, no
// contra OTRA fuente distinta ahora mismo).
func ParseConflictQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingConflict)
}
