package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	"ahmelia/gateway"
)

// ============================================================================
// KNOWLEDGE_VALIDATION.GO — Bloque 53, Fase 4.1
// ============================================================================
//
// Knowledge Gateway (51) descarga y normaliza. Knowledge Discovery (52)
// decide DE DÓNDE. Ninguno de los dos juzga si lo que se trajo es
// confiable. Knowledge Validation es el primero que evalúa los
// gateway.Document reales que produce el Gateway — y, por eso mismo,
// el primero que de verdad ENCADENA los tres bloques de Fase 4.1
// (Discover → Fetch → Validate), no solo uno.
//
// No inventa un juicio de "calidad editorial" que no puede fundamentar
// — valida señales medibles de verdad: ¿la respuesta fue HTTP 2xx?
// ¿trajo contenido de texto real después de normalizar (no una página
// vacía por JavaScript, un bloqueo, o un error)? ¿hay más de una fuente
// AUTORIZADA distinta que corrobora lo mismo?
//
// LÍMITE HONESTO, documentado a propósito: esto NO valida antigüedad
// del contenido, tal como el roadmap original pedía ("¿es reciente?").
// gateway.Document (Bloque 51) no trae fecha de publicación ni de
// última actualización — normalize() solo extrae título y texto plano,
// nunca metadata de fecha. Lo único que Document tiene es FetchedAt,
// que es CUÁNDO Ahmelia lo descargó, no cuándo se escribió. Simular una
// fecha de publicación a partir de eso sería inventar un dato que no
// existe — Knowledge Gateway necesitaría extraer metadata real
// primero, y eso no es parte de este bloque.

// MinContentLength es el umbral por debajo del cual un documento
// probablemente no cargó contenido real — páginas que requieren
// JavaScript, bloqueos, o respuestas de error suelen normalizar a casi
// nada de texto plano.
const MinContentLength = 200

// SourceValidation es el resultado de validar UN gateway.Document.
type SourceValidation struct {
	Document   *gateway.Document
	StatusOK   bool
	HasContent bool
	Issues     []string
}

// IsReliable es true solo si pasó las dos señales medibles — mismo
// patrón que IsReliable() en learning_validator.go (Bloque 48) y
// HasLead()/HasAnalogy() (25/28): un booleano claro, sin que quien
// llame tenga que interpretar Issues a mano.
func (sv *SourceValidation) IsReliable() bool {
	return sv.StatusOK && sv.HasContent
}

// ValidationSet es el resultado de validar TODOS los documentos
// traídos para un objetivo.
type ValidationSet struct {
	Goal         string
	Validations  []*SourceValidation
	Corroborated bool // true si 2+ documentos de fuentes AUTORIZADAS DISTINTAS resultaron confiables
}

// SourceValidator es el Knowledge Validation en sí. Se llama
// SourceValidator, no Validator — ese nombre ya lo usa
// learning_validator.go (Bloque 48, valida ActionConfidence
// estadística, un concepto completamente distinto). Sin estado propio.
type SourceValidator struct{}

// NewSourceValidator arma el validador de fuentes.
func NewSourceValidator() *SourceValidator {
	return &SourceValidator{}
}

// Validate evalúa UN documento. nil si doc es nil — mismo criterio
// defensivo de todo el proyecto.
func (sv *SourceValidator) Validate(doc *gateway.Document) *SourceValidation {
	if doc == nil {
		return nil
	}

	v := &SourceValidation{Document: doc}

	v.StatusOK = doc.StatusCode >= 200 && doc.StatusCode < 300
	if !v.StatusOK {
		v.Issues = append(v.Issues, fmt.Sprintf("respuesta HTTP %d, no 2xx", doc.StatusCode))
	}

	v.HasContent = len(strings.TrimSpace(doc.Text)) >= MinContentLength
	if !v.HasContent {
		v.Issues = append(v.Issues, "muy poco contenido de texto tras normalizar — puede necesitar JavaScript, estar bloqueada, o ser una página de error")
	}

	return v
}

// ValidateAll evalúa varios documentos y calcula Corroborated —
// requiere que al menos DOS fuentes AUTORIZADAS DISTINTAS (Document.Source,
// el Name de gateway.AuthorizedSource que matcheó en Bloque 51, no la
// URL) hayan resultado confiables. Dos documentos de la misma fuente no
// corroboran nada entre sí — es la misma fuente diciendo lo mismo dos
// veces.
func (sv *SourceValidator) ValidateAll(goal string, docs []*gateway.Document) *ValidationSet {
	vs := &ValidationSet{Goal: goal}

	reliableSources := make(map[string]bool)
	for _, doc := range docs {
		v := sv.Validate(doc)
		if v == nil {
			continue
		}
		vs.Validations = append(vs.Validations, v)
		if v.IsReliable() {
			reliableSources[doc.Source] = true
		}
	}
	vs.Corroborated = len(reliableSources) > 1

	return vs
}

// Describe arma la narrativa del conjunto de validaciones.
func (vs *ValidationSet) Describe() string {
	if len(vs.Validations) == 0 {
		return fmt.Sprintf("No hay ningún documento para validar sobre \"%s\" todavía.", vs.Goal)
	}

	reliable := 0
	var issues []string
	for _, v := range vs.Validations {
		if v.IsReliable() {
			reliable++
		} else {
			issues = append(issues, fmt.Sprintf("%s (%s)", v.Document.URL, strings.Join(v.Issues, "; ")))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d de %d documento(s) confiables para \"%s\".", reliable, len(vs.Validations), vs.Goal)

	switch {
	case vs.Corroborated:
		b.WriteString(" Corroborado por más de una fuente autorizada independiente.")
	case reliable > 0:
		b.WriteString(" Sin corroboración — solo una fuente confiable, no hay con qué compararla.")
	}

	if len(issues) > 0 {
		fmt.Fprintf(&b, " Problemas: %s.", strings.Join(issues, " | "))
	}

	return b.String()
}

// ── Trigger de consulta ──────────────────────────────────────────────

var sourceValidationTriggers = []string{
	"esa fuente es confiable",
	"puedo confiar en esa documentacion",
	"esa informacion de internet es confiable",
	"valida esas fuentes sobre",
	"corrobora la informacion sobre",
}

// ParseSourceValidationQuery detecta si se pregunta por la fiabilidad
// de fuentes de INTERNET (Gateway/Discovery, Fase 4.1) — distinta de
// ParseValidatorQuery (learning_validator.go, Bloque 48, que pregunta
// por la fiabilidad ESTADÍSTICA de ActionConfidence, un dato interno
// que no tiene nada que ver con documentos externos).
func ParseSourceValidationQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingSourceValidation)
}
