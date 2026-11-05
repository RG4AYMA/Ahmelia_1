package interaction

import "strings"

// SemanticEvaluationStatus describes whether the current semantic
// interpretation is safe to pass downstream as-is.
type SemanticEvaluationStatus string

const (
	SemanticAccepted SemanticEvaluationStatus = "accepted"
	SemanticReview   SemanticEvaluationStatus = "review"
	SemanticRejected SemanticEvaluationStatus = "rejected"
)

// SemanticEvaluation is a domain-neutral quality gate over meaning already
// extracted by Analyze (and optionally enriched by a linguistic adapter).
// It evaluates the interpretation, not the truth of the answer and not the
// quality of any provider/tool that may eventually act on it.
type SemanticEvaluation struct {
	Status       SemanticEvaluationStatus
	Score        float64
	Confidence   float64
	Issues       []string
	ResolvedRefs int
	OpenRefs     int
	Reason       string
}

// EvaluateSemantic evaluates semantic coherence, contextual grounding and
// completeness. It never reparses the raw input into a second interpretation.
// This keeps B96.22.29 as an evaluator of the shared semantic contract rather
// than another competing classifier.
func EvaluateSemantic(req SemanticRequest, ctx Context) SemanticEvaluation {
	acts := CanonicalActs(req)
	score := maxFloat(clamp01(req.Confidence), clamp01(acts.Confidence))
	issues := make([]string, 0, 5)
	resolved, open := 0, 0

	if strings.TrimSpace(req.Normalized) == "" && strings.TrimSpace(req.Raw) == "" {
		issues = append(issues, "empty semantic input")
		score = 0
	}

	if acts.Primary == ActUnknown {
		issues = append(issues, "communicative act is unknown")
		score -= 0.25
	}

	if acts.Has(ActQuestion) || acts.Has(ActOpinionRequest) {
		if strings.TrimSpace(req.QuestionType) == "" {
			issues = append(issues, "question type is missing")
			score -= 0.12
		}
	}

	if acts.Has(ActCommand) {
		if strings.TrimSpace(req.Objective.Text) == "" {
			issues = append(issues, "objective is missing for a goal-directed turn")
			score -= 0.16
		}
	}

	if acts.Has(ActCorrection) {
		if strings.TrimSpace(req.CorrectionTarget) == "" && !req.Correction.IsCorrection {
			issues = append(issues, "correction has no identifiable target")
			score -= 0.14
		}
	}

	for _, ref := range req.References {
		if strings.TrimSpace(ref.Expression) == "" {
			continue
		}
		if ref.Resolved {
			resolved++
		} else {
			open++
		}
	}
	if open > 0 {
		// An unresolved reference is not automatically an error: it may be
		// exactly why a clarification is needed. Penalize conservatively.
		score -= semanticMinFloat(0.20, 0.08*float64(open))
		issues = append(issues, "one or more references remain unresolved")
	}

	if req.Continues || req.FollowUp || req.AddressesCurrent {
		if strings.TrimSpace(ctx.ActiveObjective) == "" && strings.TrimSpace(ctx.CurrentTopic) == "" {
			issues = append(issues, "turn claims continuity without active context")
			score -= 0.12
		}
	}

	// Canonical acts may legitimately be compound; coexistence is evidence, not a contradiction.
	// Conflicts between meaning and action authorization are evaluated later by B96.33.8.

	score = clamp01(score)
	status := SemanticAccepted
	switch {
	case score < 0.40:
		status = SemanticRejected
	case score < 0.70 || len(issues) >= 2:
		status = SemanticReview
	}

	reason := "semantic interpretation is coherent enough for downstream decisions"
	if status == SemanticReview {
		reason = "semantic interpretation is usable but should be treated as uncertain"
	} else if status == SemanticRejected {
		reason = "semantic evidence is too weak or contradictory for downstream decisions"
	}

	return SemanticEvaluation{
		Status: status, Score: score, Confidence: score,
		Issues: issues, ResolvedRefs: resolved, OpenRefs: open, Reason: reason,
	}
}

func semanticMinFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
