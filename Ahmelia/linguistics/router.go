package linguistics

import (
	"strings"

	"ahmelia/interaction"
	"ahmelia/shared"
)

// RouteDecision contiene la decisión del router lingüístico.
//
// El router no interpreta el lenguaje ni toma decisiones cognitivas.
// Únicamente determina si una entrada debería recibir análisis
// lingüístico adicional mediante el LLM.
type RouteDecision struct {
	UseLLM bool
	Reason string
}

// Router decide cuándo Ahmelia necesita apoyo del intérprete lingüístico.
type Router struct{}

// NewRouter crea el único router de la capa lingüística.
func NewRouter() *Router {
	return &Router{}
}

// Decide determina si una entrada debería ser enviada al LLM.
//
// IMPORTANTE:
// Una decisión positiva NO significa que el LLM tenga autoridad.
// Solo significa que la estructura lingüística de la entrada podría
// beneficiarse de análisis adicional.
func (r *Router) Decide(input string) RouteDecision {
	req := interaction.Analyze(strings.TrimSpace(input), interaction.Context{})
	return r.DecideRequest(req)
}

// DecideRequest is the live routing boundary. It consumes the same semantic
// request already produced by Interaction rather than maintaining a second
// word-count/phrase-count routing system inside linguistics.
func (r *Router) DecideRequest(req interaction.SemanticRequest) RouteDecision {
	if strings.TrimSpace(req.Raw) == "" {
		return RouteDecision{UseLLM: false, Reason: "entrada vacía"}
	}
	if interaction.NeedsLinguisticDepth(req) {
		return RouteDecision{UseLLM: true, Reason: "representación semántica incompleta"}
	}
	normalized := strings.TrimSpace(shared.Normalize(req.Normalized))
	if containsContextReference(normalized) {
		return RouteDecision{UseLLM: true, Reason: "posible referencia contextual"}
	}
	if containsConstraint(normalized) {
		return RouteDecision{UseLLM: true, Reason: "posible restricción o condición"}
	}
	return RouteDecision{UseLLM: false, Reason: "representación semántica suficiente"}
}

// containsContextReference busca indicadores lingüísticos simples de
// referencias cuyo significado podría depender del contexto previo.
//
// No intenta resolverlas. Solo detecta que podrían existir.
func containsContextReference(input string) bool {
	references := []string{
		"eso",
		"esto",
		"aquello",
		"lo de ayer",
		"lo que hicimos",
		"lo que hacíamos",
		"haz lo mismo",
		"el anterior",
		"la anterior",
		"como antes",
		"igual que antes",
	}

	for _, reference := range references {
		if strings.Contains(input, reference) {
			return true
		}
	}

	return false
}

// containsConstraint busca señales de posibles restricciones.
//
// Esto no determina la restricción exacta; esa tarea sigue siendo del
// intérprete lingüístico cuando UseLLM sea true.
func containsConstraint(input string) bool {
	indicators := []string{
		"pero no",
		"sin ",
		"solo ",
		"antes de ",
		"después de ",
		"excepto ",
		"a menos que ",
	}

	for _, indicator := range indicators {
		if strings.Contains(input, indicator) {
			return true
		}
	}

	return false
}
