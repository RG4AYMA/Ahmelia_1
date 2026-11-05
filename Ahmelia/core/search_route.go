package core

import (
	"fmt"
	"strings"

	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/knowledge"
)

// consumeSearchRoute is the single Core consumer for IntentSearch. Search is
// not a physical capability execution lane: it delegates external retrieval to
// KnowledgeEngine and therefore never calls Engine.execute or a provider.
func (e *Engine) consumeSearchRoute(req interaction.SemanticRequest, decision interaction.CapabilityInteractionDecision, route capability.Route) bool {
	if e == nil || route.Kind != capability.IntentSearch {
		return false
	}

	if e.flowTrace != nil {
		e.flowTrace.Mark("SEARCH ROUTE", fmt.Sprintf("intent=search candidates=%d score=%d", len(route.Candidates), route.Score))
	}

	result := (*knowledge.Result)(nil)
	if e.knowledgeEngine != nil {
		result = e.knowledgeEngine.QuerySemantic(req, e.lastDecisionEvidence)
	}
	if result == nil || strings.TrimSpace(result.Content) == "" || result.Kind == knowledge.KindUnknown {
		result = &knowledge.Result{
			Content:    "No pude completar la búsqueda con una fuente externa autorizada y habilitada.",
			Kind:       knowledge.KindStructured,
			Confidence: 0.4,
			Source:     "search_route",
		}
	}

	if e.renderSpecialistResponseResults(req, SpecialistResponseResults{Knowledge: result}) {
		return true
	}
	return e.renderCanonicalTextResult(req, result.Source, result.Content, result.Confidence, interaction.MoveAnswer)
}
