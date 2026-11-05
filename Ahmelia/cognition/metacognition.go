package cognition

import (
	"fmt"
	"strings"

	"ahmelia/interaction"
)

// B96.40.7 — METACOGNITIVE DIAGNOSTIC BOUNDARY
//
// Subsystems remain independently authoritative over their own state/policy.
// They expose typed DiagnosticResult values. Cognition decides how those
// diagnostics should participate in the current conversational turn; it does
// not ask the subsystem for a ready-made sentence.

type MetacognitiveDecision struct {
	Intent      interaction.ResponseIntent
	Move        interaction.ResponseMove
	Diagnostics []interaction.DiagnosticResult
	Evidence    []interaction.ResponseEvidence
	Reason      string
}

func EvaluateMetacognition(req interaction.SemanticRequest, snap interaction.ConversationSnapshot, diagnostics []interaction.DiagnosticResult) (MetacognitiveDecision, bool) {
	if len(diagnostics) == 0 || !isMetacognitiveQuestion(req) {
		return MetacognitiveDecision{}, false
	}

	intent := interaction.DetermineResponseIntent(req, snap)
	if intent.Kind == interaction.ResponseIntentNone || !intent.ShouldSpeak {
		intent.Kind = interaction.ResponseIntentAnswer
		intent.ShouldSpeak = true
		intent.ShouldContinue = true
	}
	intent.Reason = "metacognitive response derived from subsystem diagnostics"

	move := interaction.MoveAnswer
	if !interaction.ResponseMoveIsValid(move) {
		return MetacognitiveDecision{}, false
	}

	evidence := make([]interaction.ResponseEvidence, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if !diagnostic.Valid() {
			continue
		}
		for _, fact := range diagnostic.Facts {
			value := strings.TrimSpace(fact.Value)
			if value == "" {
				continue
			}
			// This representation is intentionally machine/semantic shaped:
			// Language receives facts, not a completed reply.
			text := fmt.Sprintf("diagnostic[%s] %s=%s", diagnostic.Kind, fact.Key, value)
			if diagnostic.Source == "reasoning_deep.mental_agenda" {
				labels := map[string]string{"focus": "Enfoque actual", "doubt_1": "Duda activa", "concern_1": "Preocupación activa", "thought_1": "Pensamiento activo", "confidence": "Confianza actual"}
				label := labels[fact.Key]
				if label == "" {
					label = strings.ReplaceAll(strings.TrimSuffix(fact.Key, "_"), "_", " ")
				}
				text = label + ": " + value
			}
			confidence := diagnostic.Confidence
			if fact.Confidence > 0 && fact.Confidence < confidence {
				confidence = fact.Confidence
			}
			if confidence <= 0 {
				confidence = 0.5
			}
			evidence = append(evidence, interaction.ResponseEvidence{
				Text: text, Source: diagnostic.Source, Relevant: true, Confidence: confidence,
			})
		}
	}
	if len(evidence) == 0 {
		return MetacognitiveDecision{}, false
	}

	return MetacognitiveDecision{
		Intent: intent, Move: move, Diagnostics: append([]interaction.DiagnosticResult(nil), diagnostics...),
		Evidence: evidence, Reason: "Cognition selected diagnostic evidence as the semantic basis for Language",
	}, true
}

func isMetacognitiveQuestion(req interaction.SemanticRequest) bool {
	acts := interaction.CanonicalActs(req)
	return acts.Has(interaction.ActQuestion) || acts.Has(interaction.ActOpinionRequest) ||
		interaction.HasSemanticQuery(req, interaction.QueryMentalState)
}
