package interaction

import "strings"

// CapabilityRequest is the domain-neutral executable contract emitted by the
// interaction layer. It describes what the user wants an executable system
// to do without selecting a provider, tool, domain implementation, or
// execution mechanism.
//
// The interaction layer owns meaning. Capability/production layers own
// resolution, authorization, execution and verification.
const CapabilitySourceAutonomyPlan = "autonomy.plan"

type CapabilityRequest struct {
	Operation  string
	Target     string
	Parameters map[string]string
	Confidence float64
	Source     string
	Repair     *PostExecutionRepairRequest
}

// PostExecutionRepairRequest is a provider-neutral execution proposal emitted
// after Cognition establishes that a correction targets already materialized work.
// It contains no provider selection or physical execution details.
type PostExecutionRepairRequest struct {
	WorkID      string
	Target      string
	Replacement string
	Confidence  float64
	Reason      string
}

// HasOperation reports whether the request contains a usable executable
// operation. An empty request must remain conversational and must never be
// treated as an executable capability.
func (r CapabilityRequest) HasOperation() bool {
	return strings.TrimSpace(r.Operation) != ""
}

// ToCapabilityRequest adapts the existing deterministic ActionIntent to the
// capability-agnostic boundary. ActionIntent remains a surface-level parser
// contract for compatibility; this adapter deliberately does not resolve a
// provider or capability implementation.
func ToCapabilityRequest(a ActionIntent, raw string) CapabilityRequest {
	if strings.TrimSpace(a.Name) == "" {
		return CapabilityRequest{}
	}
	params := map[string]string{}
	for k, v := range a.Parameters {
		params[k] = v
	}
	if strings.TrimSpace(raw) != "" && params["raw"] == "" {
		params["raw"] = raw
	}
	return CapabilityRequest{
		Operation:  strings.TrimSpace(a.Name),
		Target:     strings.TrimSpace(a.Target),
		Parameters: params,
		Confidence: a.Confidence,
		Source:     "interaction",
	}
}

// CapabilityInteractionDecision is the result of the capability-agnostic
// boundary. It explicitly separates "the user requested an executable
// operation" from "some provider can execute it".
type CapabilityInteractionDecision struct {
	Executable bool
	Request    CapabilityRequest
	Reason     string
}

// ResolveCapabilityInteraction converts semantic interaction evidence into a
// provider-neutral executable request. It never checks whether the operation
// exists, never selects a tool, and never authorizes execution.
func ResolveCapabilityInteraction(r SemanticRequest) CapabilityInteractionDecision {
	if !CanonicalActs(r).Has(ActCommand) {
		return CapabilityInteractionDecision{
			Executable: false,
			Reason:     "turn is not an executable interaction request",
		}
	}
	if strings.TrimSpace(r.Action.Name) != "" {
		req := ToCapabilityRequest(r.Action, r.Raw)
		return CapabilityInteractionDecision{
			Executable: req.HasOperation(),
			Request:    req,
			Reason:     "interaction produced a provider-neutral executable request",
		}
	}

	// Production goals are executable interaction requests even when no
	// concrete action vocabulary exists at the semantic boundary. The
	// interaction layer therefore emits the generic operation "produce" as a
	// provider-neutral contract. Capability resolution remains downstream in
	// the single Engine execution authority. This is the missing bridge that
	// previously forced ProductionRuntime to become a parallel execution door.
	if isProduction(r.Normalized) {
		return CapabilityInteractionDecision{
			Executable: true,
			Request: CapabilityRequest{
				Operation:  "produce",
				Parameters: map[string]string{"raw": r.Raw},
				Confidence: r.Confidence,
				Source:     "interaction.production_intent",
			},
			Reason: "interaction produced a provider-neutral production request",
		}
	}

	return CapabilityInteractionDecision{
		Executable: false,
		Reason:     "command has no provider-neutral executable operation",
	}
}

// ResolvePostExecutionRepair converts Cognition's repair assessment into the
// same provider-neutral capability boundary used by ordinary execution.
func ResolvePostExecutionRepair(r PostExecutionRepairRequest) CapabilityInteractionDecision {
	if strings.TrimSpace(r.WorkID) == "" || strings.TrimSpace(r.Replacement) == "" {
		return CapabilityInteractionDecision{Executable: false, Reason: "repair request lacks work or replacement"}
	}
	return CapabilityInteractionDecision{
		Executable: true,
		Request: CapabilityRequest{
			Operation:  "repair",
			Parameters: map[string]string{},
			Confidence: r.Confidence,
			Source:     "cognition.post_execution_repair",
			Repair:     &r,
		},
		Reason: r.Reason,
	}
}
