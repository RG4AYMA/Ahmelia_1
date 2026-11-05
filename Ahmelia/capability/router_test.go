package capability

import (
	"testing"

	"ahmelia/interaction"
)

func TestResolverRoutesGeneralProductionWithoutDomainKeyword(t *testing.T) {
	r := NewResolver(NewRegistry())
	req := interaction.Analyze("Analiza una capacidad que actualmente te falte. Diseña un pequeño bloque, crea los archivos necesarios, prepara sus tests y entrégame todo para revisión.", interaction.Context{})
	decision := interaction.ResolveCapabilityInteraction(req)
	route := r.Resolve(decision, req)
	if route.Kind != IntentProduction {
		t.Fatalf("kind = %q, want production", route.Kind)
	}
}

func TestResolverConsumesAcceptedSemanticReasoningLane(t *testing.T) {
	r := NewResolver(NewRegistry())
	req := interaction.SemanticRequest{Kind: interaction.KindReasoning, RequestedAction: "answer", Confidence: .95}
	decision := interaction.CapabilityInteractionDecision{Executable: false}
	route := r.Resolve(decision, req)
	if route.Kind != IntentReasoning {
		t.Fatalf("kind = %q, want reasoning", route.Kind)
	}
}

func TestResolverKeepsExplicitSearchAsSearch(t *testing.T) {
	r := NewResolver(NewRegistry())
	req := interaction.Analyze("Busca información sobre Go", interaction.Context{})
	decision := interaction.ResolveCapabilityInteraction(req)
	route := r.Resolve(decision, req)
	if route.Kind != IntentSearch {
		t.Fatalf("kind = %q, want search", route.Kind)
	}
}

func TestResolverProductionUsesStructuredCandidateFields(t *testing.T) {
	registry := NewRegistry()
	registry.Register(Capability{ID: "programming.create_file", Domain: DomainLocal, Action: "create_file", Description: "crear archivos de código", Permission: PermissionAskFirst})
	r := NewResolver(registry)
	req := interaction.SemanticRequest{
		Kind: interaction.KindProduction, RequestedAction: "create", Topic: "test",
		Entities: []string{"programa", "test"}, Confidence: .95,
	}
	decision := interaction.CapabilityInteractionDecision{Executable: true, Request: interaction.CapabilityRequest{Operation: "produce"}}
	route := r.Resolve(decision, req)
	if route.Kind != IntentProduction {
		t.Fatalf("kind = %q, want production", route.Kind)
	}
	if len(route.Candidates) == 0 {
		t.Fatal("expected semantic capability candidates")
	}
}

func TestResolverPreservesStructuredNegativeSearchConstraint(t *testing.T) {
	r := NewResolver(NewRegistry())
	req := interaction.SemanticRequest{Kind: interaction.KindReasoning, Topic: "capability review", RequestedAction: "answer", Confidence: .95,
		Objective: interaction.Objective{Constraints: []string{"no_search"}}}
	decision := interaction.CapabilityInteractionDecision{Executable: true, Request: interaction.CapabilityRequest{Operation: "act"}}
	route := r.Resolve(decision, req)
	if route.Kind != IntentReasoning {
		t.Fatalf("kind=%q, want reasoning; G4 must not reclassify from negative constraint text", route.Kind)
	}
}
