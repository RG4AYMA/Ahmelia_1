package interaction

import "strings"

// NeedsLinguisticDepth determines whether the deterministic semantic boundary
// has enough structured evidence to let the interaction continue without an
// additional linguistic interpretation pass.
//
// This is deliberately a boundary-completeness check, not a phrase matcher.
// The fast grammar may recognize known constructions, but an unrecognized
// surface form must remain eligible for the linguistic interpreter instead of
// becoming an implicit semantic rejection. The interpreter only enriches the
// semantic contract; it never acquires response, memory, capability, or
// execution authority.
func NeedsLinguisticDepth(r SemanticRequest) bool {
	if r.CommunicativeActs.Primary == ActUnknown || r.Confidence < 0.75 {
		return true
	}

	if len(r.References) > 0 ||
		r.CommunicativeActs.Has(ActSocial) ||
		r.CommunicativeActs.Has(ActProposal) ||
		r.CommunicativeActs.Has(ActCorrection) {
		return true
	}

	// Questions are the most important fallback boundary. A high-confidence
	// classifier can correctly identify "question" while still leaving the
	// semantic target incomplete. Known deterministic constructions (for
	// example, a generic user attribute query) already produce the required
	// semantic evidence and stay on the fast path. A question that reaches
	// this point without such evidence is escalated to the linguistic
	// interpreter, regardless of its exact wording, verb, clause structure or
	// attribute vocabulary.
	if r.CommunicativeActs.Has(ActQuestion) && !questionHasSemanticGrounding(r) {
		return true
	}

	return false
}

// ShouldDeferLinguisticDepth decides whether an incomplete linguistic pass is
// safe to continue after the live turn.  Deferred interpretation is optional
// enrichment: it must never compete with an already executable user command.
//
// This is deliberately expressed in terms of the canonical semantic contract
// rather than phrases or domains.  Once the request already has a confident
// command act and resolves to an executable capability, the execution path is
// the owner of the turn.  A late linguistic result cannot improve that turn
// enough to justify consuming the model again in the background.
func ShouldDeferLinguisticDepth(r SemanticRequest) bool {
	if !NeedsLinguisticDepth(r) {
		return false
	}
	acts := CanonicalActs(r)
	if !acts.Has(ActCommand) || r.Confidence < 0.75 || r.NeedsClarification {
		return true
	}
	decision := ResolveCapabilityInteraction(r)
	if decision.Executable {
		return false
	}
	return true
}

func questionHasSemanticGrounding(r SemanticRequest) bool {
	if _, ok := ResolveSemanticAttribute(r); ok {
		return true
	}
	if strings.TrimSpace(r.Topic) != "" ||
		strings.TrimSpace(r.ExplicitDomain) != "" ||
		len(r.Relations) > 0 ||
		len(r.Entities) > 0 ||
		len(r.References) > 0 {
		return true
	}

	// A specialized question type is already a semantic classification even
	// when the target itself is not represented as an entity/topic. For the
	// generic/unknown case there is no comparable grounding, so the deeper
	// linguistic layer is allowed to enrich it.
	qtype := strings.ToLower(strings.TrimSpace(r.QuestionType))
	return qtype != "" && qtype != "general" && qtype != "unknown"
}
