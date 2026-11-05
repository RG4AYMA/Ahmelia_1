package cognition

import (
	"strings"

	"ahmelia/interaction"
	"ahmelia/situation"
)

// B96.40.2 — SITUATION → COGNITION INTEGRATION
//
// Situation and specialist Contributions are evidence for cognition. They do
// not become a second intent authority, response owner, planner, or executor.
// The integration is deliberately activated only when the structured
// conversational cognition result is unknown or below the established
// confidence threshold. Normal high-confidence cognition remains untouched.

type SituationalCognitionContext struct {
	Iteration             situation.IterationContext
	Situation             situation.Situation
	Contributions         []situation.Contribution
	Work                  interaction.WorkContext
	HasWork               bool
	AuthoritativeEvidence []situation.Evidence
}

type SituationalResolution struct {
	Used        bool
	Perspective string
	Intent      CommunicativeIntentKind
	Confidence  float64
	Evidence    []string
	Reason      string
}

func resolveFromSituation(intent CommunicativeIntentEvaluation, evidence situation.DecisionEvidence) (CommunicativeIntentEvaluation, SituationalResolution) {
	resolution := SituationalResolution{Reason: "situation not consulted because structured intent is sufficiently grounded"}
	if intent.Primary != IntentUnknown && intent.Confidence >= 0.60 {
		return intent, resolution
	}

	type candidate struct {
		kind        CommunicativeIntentKind
		score       float64
		perspective string
		evidence    []string
	}
	var best *candidate
	consider := func(kind CommunicativeIntentKind, score float64, perspective string, evidence ...string) {
		if score <= 0 {
			return
		}
		c := candidate{kind: kind, score: clamp01(score), perspective: perspective, evidence: uniqueStrings(evidence)}
		if best == nil || c.score > best.score {
			best = &c
		}
	}

	for _, f := range evidence.Findings {
		switch strings.ToLower(strings.TrimSpace(f.Kind)) {
		case "proposes", "anticipates", "commits_to":
			consider(IntentShareIdea, f.Confidence, f.Perspective, "decision_evidence:finding="+strings.ToLower(f.Kind), "situation="+evidence.SituationID)
		case "gives_feedback_to", "evaluates":
			consider(IntentFeedback, f.Confidence, f.Perspective, "decision_evidence:finding="+strings.ToLower(f.Kind), "situation="+evidence.SituationID)
		case "addresses", "asks_about_state":
			consider(IntentSocial, f.Confidence, f.Perspective, "decision_evidence:finding="+strings.ToLower(f.Kind), "situation="+evidence.SituationID)
		case "asks_about_opinion":
			consider(IntentAskOpinion, f.Confidence, f.Perspective, "decision_evidence:finding=asks_about_opinion", "situation="+evidence.SituationID)
		case "communicates":
			if strings.EqualFold(f.Target, "social") || strings.EqualFold(f.Target, string(interaction.SpeechSocial)) {
				consider(IntentSocial, f.Confidence, f.Perspective, "decision_evidence:finding=communicates:social", "situation="+evidence.SituationID)
			}
		}
	}

	if best == nil {
		resolution.Reason = "situation consulted but no structured contribution could safely resolve the turn"
		return intent, resolution
	}

	resolved := intent
	resolved.Primary = best.kind
	resolved.Confidence = best.score
	resolved.Ambiguous = false
	resolved.NeedsClarification = false
	resolved.ActionOriented = best.kind == IntentRequestAction
	resolved.DecisionOriented = best.kind == IntentDecide
	resolved.ProposalLike = best.kind == IntentShareIdea
	resolved.QuestionLike = best.kind == IntentQuestion || best.kind == IntentAskOpinion
	resolved.Evidence = uniqueStrings(append(append([]string(nil), intent.Evidence...), best.evidence...))
	resolved.Reason = "communicative intent resolved from structured Situation contribution"

	resolution.Used = true
	resolution.Perspective = best.perspective
	resolution.Intent = best.kind
	resolution.Confidence = best.score
	resolution.Evidence = append([]string(nil), best.evidence...)
	resolution.Reason = "structured Situation contribution supplied an interpretation after normal cognition was insufficient"
	return resolved, resolution
}

func minContributionConfidence(c situation.Contribution, r situation.Relation) float64 {
	score := c.Confidence
	if r.Confidence > 0 && (score == 0 || r.Confidence < score) {
		score = r.Confidence
	}
	if score == 0 {
		score = 0.60
	}
	return clamp01(score)
}
