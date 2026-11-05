package capability

import (
	"ahmelia/interaction"
)

// IntentKind describes the broad execution lane selected before a domain is
// chosen. It is deliberately domain-agnostic: Programming, Unity, Blender,
// files, etc. are providers behind this decision, not special cases in the
// Engine.
type IntentKind string

const (
	IntentUnknown      IntentKind = "unknown"
	IntentConversation IntentKind = "conversation"
	IntentSearch       IntentKind = "search"
	IntentReasoning    IntentKind = "reasoning"
	IntentAction       IntentKind = "action"
	IntentProduction   IntentKind = "production"
)

// Route is the result of the general intent resolver. Candidates are registry
// capabilities that may satisfy the objective; the resolver never executes or
// authorizes anything.
type Route struct {
	Kind       IntentKind
	Candidates []Capability
	Score      int
}

// Resolver is the general front door for capability/production intent. It is
// intentionally small and deterministic so future semantic resolvers can be
// substituted without changing the Engine contract.
type Resolver struct {
	registry  *Registry
	discovery *Discovery
}

func NewResolver(registry *Registry) *Resolver {
	return &Resolver{registry: registry, discovery: NewDiscovery(registry)}
}

// Resolve consumes the CapabilityInteractionDecision already accepted by the
// Engine authority together with the SemanticRequest that produced it. It
// does not inspect or classify raw language. Its only remaining job is to map
// the accepted semantic lane to a Route and, for production, match registry
// capabilities from structured semantic fields.
func (r *Resolver) Resolve(decision interaction.CapabilityInteractionDecision, req interaction.SemanticRequest) Route {
	if !decision.Executable || !decision.Request.HasOperation() {
		return routeForSemanticKind(req.Kind)
	}

	route := routeForSemanticKind(req.Kind)
	if route.Kind == IntentUnknown {
		route.Kind = routeKindForOperation(decision.Request.Operation)
	}
	if route.Kind != IntentProduction && route.Kind != IntentAction {
		return route
	}

	if r.discovery != nil {
		results, err := r.discovery.DiscoverSemantic(req)
		if err == nil {
			for _, result := range results {
				if result.Score <= 0 || !capabilityLaneMatches(result.Capability, route.Kind) {
					continue
				}
				route.Candidates = append(route.Candidates, result.Capability)
				route.Score += result.Score
			}
		}
	}
	return route
}

func routeForSemanticKind(kind interaction.RequestKind) Route {
	switch kind {
	case interaction.KindSearch:
		return Route{Kind: IntentSearch}
	case interaction.KindReasoning:
		return Route{Kind: IntentReasoning}
	case interaction.KindAction:
		return Route{Kind: IntentAction}
	case interaction.KindProduction:
		return Route{Kind: IntentProduction}
	case interaction.KindConversation, interaction.KindQuestion, interaction.KindInform,
		interaction.KindProposal, interaction.KindCorrection, interaction.KindContinuation,
		interaction.KindClarification, interaction.KindLearning, interaction.KindPlanning,
		interaction.KindFeedback, interaction.KindStop, interaction.KindPause:
		return Route{Kind: IntentConversation}
	default:
		return Route{Kind: IntentUnknown}
	}
}

func routeKindForOperation(operation string) IntentKind {
	switch operation {
	case "produce", "create", "modify", "repair":
		return IntentProduction
	default:
		return IntentAction
	}
}

func capabilityLaneMatches(c Capability, kind IntentKind) bool {
	switch kind {
	case IntentAction:
		return c.Lane == LaneAction || c.Lane == LaneUnspecified
	case IntentProduction:
		return c.Lane == LaneProduction || c.Lane == LaneUnspecified
	default:
		return true
	}
}
