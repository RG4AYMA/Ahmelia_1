package interaction

import (
	"strings"

	"ahmelia/ownership"
	"ahmelia/shared"
)

// InferSemanticRelations adds a conservative, language-level relation
// projection when the richer linguistic adapter could not provide one.
// It deliberately extracts only relations whose surface grammar is explicit:
// future/commitment morphology and explicit proposal constructions. It never
// chooses an action or authorizes execution.
func InferSemanticRelations(r SemanticRequest) SemanticRequest {
	// Linguistic relations and this conservative grammar projection share the
	// same SemanticRequest.Relations contract. Do not short-circuit just
	// because the linguistic adapter supplied one relation: it may have found
	// an unrelated relation while missing an explicit future/commitment one.
	n := strings.TrimSpace(shared.Normalize(r.Normalized))
	if n == "" {
		n = strings.TrimSpace(shared.Normalize(r.Raw))
	}
	if n == "" {
		return r
	}

	add := func(source, predicate, target, value, evidence string, confidence float64) {
		if strings.TrimSpace(source) == "" || strings.TrimSpace(predicate) == "" || strings.TrimSpace(target) == "" {
			return
		}
		for _, existing := range r.Relations {
			if strings.EqualFold(existing.Source, source) && strings.EqualFold(existing.Predicate, predicate) && strings.EqualFold(existing.Target, target) && strings.EqualFold(strings.TrimSpace(existing.Value), strings.TrimSpace(value)) {
				return
			}
		}
		r.Relations = append(r.Relations, SemanticRelation{Source: source, Predicate: predicate, Target: target, Value: strings.TrimSpace(value), Confidence: confidence, Evidence: evidence})
	}

	// Attribute semantics are resolved at the interaction boundary. Structured
	// linguistic relations win; deterministic grammar is the fallback when the
	// optional language model did not provide an explicit relation. The key is
	// always extracted from the utterance itself, never selected from a list of
	// supported attributes.
	if attribute, ok := ResolveSemanticAttribute(r); ok {
		// A structured linguistic relation can refine an ownership result that
		// remained unresolved during the cheap pass. This is especially important
		// when discourse address ("tu/tú") and the queried possessor ("mi") both
		// appear: the relation source is evidence for the semantic referent, not
		// a new decision authority. Never overwrite an already-resolved owner.
		if r.Subject == UserObservationSubject("") &&
			(attribute.Subject == UserObservationSubjectUser || attribute.Subject == UserObservationSubjectSelf) {
			r.Subject = attribute.Subject
			r.SubjectConfidence = attribute.Confidence
			r.SubjectEvidence = append(r.SubjectEvidence, ownershipEvidenceFromAttribute(attribute))
		}

		predicate := "declares_attribute"
		if attribute.HasValue && SemanticAttributeDomainForKey(attribute.Key) == SemanticAttributeDomainPreference {
			predicate = "declares_preference"
		} else if !attribute.HasValue {
			predicate = "asks_about_attribute"
			if SemanticAttributeDomainForKey(attribute.Key) == SemanticAttributeDomainPreference {
				predicate = "asks_about_preference"
			}
		}
		if !attribute.HasValue {
			add(string(attribute.Subject), predicate, attribute.Key, "", attribute.Evidence, attribute.Confidence)
		} else {
			add(string(attribute.Subject), predicate, attribute.Key, attribute.Value, attribute.Evidence, attribute.Confidence)
		}
	}

	// Explicit first-person future/commitment forms. The target remains a
	// semantic future state rather than a capability or execution instruction.
	if hasFutureCommitment(n) {
		add("user", "commits_to", "ahmelia:future_state", "", "semantic:future_commitment", .82)
		add("user", "anticipates", "ahmelia:future_state", "", "semantic:future_commitment", .78)
	}
	if isProposalConstruction(n) {
		add("user", "proposes", "future_state", "", "semantic:proposal_construction", .80)
	}
	return r
}

func ownershipEvidenceFromAttribute(attribute SemanticAttributeReference) ownership.Evidence {
	signal := "semantic_attribute"
	if attribute.HasValue {
		signal = "semantic_attribute_declaration"
	}
	return ownership.Evidence{
		Signal:     signal,
		Token:      attribute.Key,
		Confidence: attribute.Confidence,
		Provenance: "interaction.semantic_attribute",
	}
}

func hasFutureCommitment(n string) bool {
	// shared.Normalize removes accents and punctuation before token matching,
	// so STT forms with or without diacritics reach the same semantic surface.
	t := " " + shared.Normalize(n) + " "
	future := []string{
		" hare ", " haremos ", " haras ", " hara ", " haran ",
		" podre ", " podras ", " podremos ", " podra ", " podran ",
		" voy a ", " vas a ", " vamos a ", " va a ", " van a ",
		" i will ", " we will ", " i can ", " we can ",
	}
	for _, marker := range future {
		if strings.Contains(t, marker) {
			return true
		}
	}
	return false
}

func isProposalConstruction(n string) bool {
	t := " " + strings.TrimSpace(n) + " "
	for _, marker := range []string{
		" te propongo ", " propongo ", " podriamos ",
		" deberiamos ", " we should ", " we could ", " i suggest ",
	} {
		if strings.Contains(t, marker) {
			return true
		}
	}
	return false
}
