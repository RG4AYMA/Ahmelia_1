package cognition

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"ahmelia/interaction"
	"ahmelia/situation"
)

// B96.33.8 — CONVERSATIONAL CONFLICT
//
// Detects incompatibilities between the semantic/intent/goal/relevance/
// interpretation/decision assessments already produced by B96.33.
//
// Conflict is not the same as disagreement with the user. It means that two
// pieces of the current cognitive evidence cannot safely be treated as one
// coherent interpretation without resolving the tension. This layer does not
// reparse language, authorize actions, mutate conversation state, or execute.

type ConversationalConflictKind string

const (
	ConflictNone                  ConversationalConflictKind = "none"
	ConflictIntentGoal            ConversationalConflictKind = "intent_goal"
	ConflictIntentInterpret       ConversationalConflictKind = "intent_interpretation"
	ConflictGoalRelevance         ConversationalConflictKind = "goal_relevance"
	ConflictInterpretDecision     ConversationalConflictKind = "interpretation_decision"
	ConflictContextCurrent        ConversationalConflictKind = "context_current_turn"
	ConflictCorrectionAction      ConversationalConflictKind = "correction_action"
	ConflictTopicContinuity       ConversationalConflictKind = "topic_continuity"
	ConflictAuthoritativeEvidence ConversationalConflictKind = "authoritative_evidence"
)

type ConflictEvidence struct {
	Kind        ConversationalConflictKind
	Score       float64
	Reason      string
	Critical    bool
	EvidenceIDs []string
}

type ConversationalConflict struct {
	AuthoritativeFindings []situation.DecisionFinding
	Version               string
	Primary               ConversationalConflictKind
	Confidence            float64
	HasConflict           bool
	Ambiguous             bool
	NeedsClarification    bool
	BlocksAction          bool
	Severity              float64
	Evidence              []ConflictEvidence
	Reason                string
}

// EvaluateConversationalConflict compares established B96.33 outputs without
// inventing a second semantic interpretation. Strong conflicts are promoted
// to action blockers; weaker tensions remain observable so later strategy can
// choose whether clarification is worthwhile.
func EvaluateConversationalConflict(
	model ConversationalCognitiveModel,
	intent CommunicativeIntentEvaluation,
	goal ConversationalGoalInference,
	relevance ConversationalRelevance,
	interpretation ConversationalInterpretation,
	decision ConversationalDecision,
	authoritativeEvidence ...[]situation.Evidence,
) ConversationalConflict {
	evidence := make([]ConflictEvidence, 0, 8)
	add := func(kind ConversationalConflictKind, score float64, reason string, critical bool) {
		if score <= 0 {
			return
		}
		evidence = append(evidence, ConflictEvidence{Kind: kind, Score: clamp01(score), Reason: reason, Critical: critical})
	}

	var authoritative []situation.Evidence
	if len(authoritativeEvidence) > 0 {
		authoritative = authoritativeEvidence[0]
	}
	var authoritativeFindings []situation.DecisionFinding
	for _, rel := range model.Request.Relations {
		predicate := strings.ToLower(strings.TrimSpace(rel.Predicate))
		if rel.Source != "" && !strings.EqualFold(strings.TrimSpace(rel.Source), "user") {
			continue
		}
		if predicate != "declares_attribute" && predicate != "declares_preference" {
			continue
		}
		key := strings.TrimSpace(rel.Target)
		value := strings.TrimSpace(rel.Value)
		if key == "" || value == "" || rel.Confidence < 0.70 {
			continue
		}
		for _, fact := range authoritative {
			if !fact.Authoritative || !strings.EqualFold(strings.TrimSpace(fact.Subject), "user") {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(fact.Predicate), predicate) || !strings.EqualFold(strings.TrimSpace(fact.Object), key) {
				continue
			}
			confirmed := strings.TrimSpace(fact.Value)
			if confirmed == "" || strings.EqualFold(confirmed, value) {
				continue
			}
			conf := minFloat(rel.Confidence, fact.Confidence)
			if conf < 0.70 {
				continue
			}
			turnEvidenceID := semanticTurnEvidenceID(model.Request, rel)
			evidenceIDs := []string{turnEvidenceID}
			if strings.TrimSpace(fact.ID) != "" {
				evidenceIDs = append(evidenceIDs, strings.TrimSpace(fact.ID))
			}
			reason := fmt.Sprintf("authoritative:%s:%s:%s", predicate, key, fact.ID)
			addWithEvidence := ConflictEvidence{Kind: ConflictAuthoritativeEvidence, Score: clamp01(conf), Reason: reason, Critical: false, EvidenceIDs: uniqueStrings(evidenceIDs)}
			evidence = append(evidence, addWithEvidence)
			authoritativeFindings = append(authoritativeFindings, situation.DecisionFinding{
				Kind:         "contradicts_authoritative_evidence",
				Source:       "cognition.B96.33.8",
				Perspective:  "authoritative_evidence",
				RelationType: "contradiction",
				Subject:      "user",
				Target:       key,
				EvidenceIDs:  uniqueStrings(evidenceIDs),
				Claims:       []string{value, confirmed},
				Confidence:   conf,
				Relevance:    1,
				Epistemic:    situation.EpistemicDerived,
				Provenance:   []string{"cognition.B96.33.8", fact.Source},
			})
		}
	}

	// A request for action cannot coherently coexist with an interpretation
	// that explicitly says the turn is only for discussion/consideration.
	if intent.Primary == IntentRequestAction &&
		(interpretation.ActionPosture == ActionPostureDiscuss || interpretation.ActionPosture == ActionPostureConsider) {
		add(ConflictIntentInterpret, 0.93, "intent:request_action_vs_interpretation:non_request", true)
	}

	// A correction is a repair of meaning. If the same turn is still carrying
	// a direct execution posture, the repaired meaning must be resolved first.
	if (intent.Primary == IntentCorrect || model.Request.Correction.IsCorrection || interaction.CanonicalActs(model.Request).Has(interaction.ActCorrection)) &&
		(interpretation.ActionOriented || decision.ActionCandidate || interpretation.ActionPosture == ActionPostureClarify || strings.TrimSpace(model.Request.RequestedAction) != "" || strings.TrimSpace(model.Request.Action.Name) != "") {
		add(ConflictCorrectionAction, 0.97, "correction:action_candidate_after_repair", true)
	}

	// A stop goal and an action decision are mutually incompatible for the
	// same turn. This is a cognitive contradiction, not an authorization rule.
	if goal.Primary == GoalStop && decision.Primary == DecisionAct {
		add(ConflictIntentGoal, 0.99, "goal:stop_vs_decision:act", true)
	}

	// An explicit new topic should not simultaneously be treated as a strict
	// continuation of the current topic.
	if relevance.Primary == RelevanceNewTopic &&
		(model.Request.Continues || model.Request.FollowUp || intent.Primary == IntentContinue || interpretation.Relevance == RelevanceCurrentTopic) {
		add(ConflictTopicContinuity, 0.91, "relevance:new_topic_vs_continuation", true)
	}

	// An explicit topic different from the active topic combined with a
	// continuation/follow-up signal is a direct boundary contradiction. This
	// check uses the structured topic fields so it does not depend on whether
	// an earlier goal layer carried the old objective forward.
	currentTopic := strings.TrimSpace(model.Request.Topic)
	activeTopic := strings.TrimSpace(model.Context.ActiveTopic)
	if currentTopic != "" && activeTopic != "" &&
		!strings.EqualFold(currentTopic, activeTopic) &&
		(model.Request.Continues || model.Request.FollowUp) {
		add(ConflictTopicContinuity, 0.94, "topic:explicit_topic_differs_from_active_topic_while_continuing", true)
	}

	// A decision to change topic while interpretation still says the turn is
	// specifically about the current active work indicates a boundary tension.
	if decision.Primary == DecisionChangeTopic && relevance.Primary == RelevanceCurrentWork {
		add(ConflictGoalRelevance, 0.82, "decision:change_topic_vs_relevance:current_work", false)
	}

	// Unknown/weak semantic structure combined with a confident operational
	// posture is an internal inconsistency worth surfacing. It does not itself
	// claim that the user's request is wrong.
	if !model.Signals.HasSemanticEvidence && decision.ActionCandidate {
		add(ConflictInterpretDecision, 0.95, "semantic:evidence_missing_vs_action", true)
	}
	if interpretation.HasUnresolvedRefs && interpretation.ActionOriented {
		add(ConflictInterpretDecision, 0.98, "interpretation:unresolved_reference_vs_action", true)
	}

	// Keep a context/current-turn mismatch visible when a continuation carries
	// an old objective while the current semantic request names a different
	// objective. This is not automatically a conflict when the user explicitly
	// changed topic; relevance already owns that boundary.
	currentObjective := strings.TrimSpace(model.Request.Objective.Text)
	activeObjective := strings.TrimSpace(model.Context.ActiveObjective)
	if model.Request.Continues && currentObjective != "" && activeObjective != "" &&
		!strings.EqualFold(currentObjective, activeObjective) && relevance.Primary != RelevanceNewTopic {
		add(ConflictContextCurrent, 0.79, "continuation:objective_differs_from_active_context", false)
	}

	out := ConversationalConflict{
		Version:               "B96.33.8",
		Evidence:              evidence,
		AuthoritativeFindings: authoritativeFindings,
		Confidence:            clamp01(minFloat(model.Signals.Confidence, intent.Confidence, goal.Confidence, relevance.Confidence, interpretation.Confidence, decision.Confidence)),
	}
	if len(evidence) == 0 {
		out.Primary = ConflictNone
		out.Severity = 0
		out.Reason = "no material conversational conflict detected"
		return out
	}

	best := evidence[0]
	for _, e := range evidence[1:] {
		if e.Score > best.Score {
			best = e
		}
	}
	out.Primary = best.Kind
	out.Severity = best.Score
	out.HasConflict = true
	out.Ambiguous = intent.Ambiguous || goal.Ambiguous || relevance.Ambiguous || interpretation.Ambiguous || decision.Ambiguous
	for _, e := range evidence {
		if e.Critical {
			out.BlocksAction = true
			break
		}
	}
	out.NeedsClarification = out.BlocksAction || interpretation.NeedsClarification || decision.NeedsClarification
	if decision.ActionCandidate && out.Severity >= 0.70 {
		out.BlocksAction = true
		out.NeedsClarification = true
	}
	if out.BlocksAction {
		out.Reason = "conflicting conversational evidence must be resolved before an action posture can be trusted"
	} else if out.NeedsClarification {
		out.Reason = "conversational evidence contains a recoverable coherence conflict"
	} else {
		out.Reason = "a non-blocking conversational coherence conflict was detected"
	}
	return out
}

func semanticTurnEvidenceID(req interaction.SemanticRequest, rel interaction.SemanticRelation) string {
	// Provenance identity is derived only from structured semantics. It is not
	// used as a classification signal and intentionally does not hash Raw or
	// Normalized user text.
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s", req.Kind, req.Language, req.Topic, rel.Predicate, rel.Target, rel.Value)
	h := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("semantic_turn:%x", h[:8])
}

// RequiresClarificationBeforeAction is the explicit downstream gate exposed
// by B96.33.8. It does not authorize or execute anything.
func (c ConversationalConflict) RequiresClarificationBeforeAction(d ConversationalDecision) bool {
	return c.BlocksAction && (d.ActionCandidate || d.Primary == DecisionAskClarify)
}

// Keep the shared interaction contract visible in this block: conflict is
// derived from established semantic state, never from a second parser.
var _ interaction.SemanticRequest
