package cognition

import (
	"strings"

	"ahmelia/interaction"
	"ahmelia/situation"
)

// B96.33.12 — CONVERSATIONAL COGNITION E2E
//
// This is the single deterministic orchestration boundary for the B96.33
// conversational cognition chain. It runs the established cognitive stages in
// order and returns their auditable intermediate contracts together with the
// final transient adaptation.
//
// It deliberately does not parse language a second time, call an LLM, mutate
// conversation state, persist a user model, authorize execution, select a
// capability/provider, execute work, or generate final language.

type ConversationalCognitionE2E struct {
	Version               string
	Model                 ConversationalCognitiveModel
	Intent                CommunicativeIntentEvaluation
	Goal                  ConversationalGoalInference
	Relevance             ConversationalRelevance
	Interpretation        ConversationalInterpretation
	Decision              ConversationalDecision
	Uncertainty           ConversationalUncertainty
	Conflict              ConversationalConflict
	Action                ConversationalActionSelection
	Strategy              ConversationalResponseStrategy
	Adaptation            ConversationalAdaptation
	Behavior              ConversationalBehaviorDecision
	Situation             situation.Situation
	Contributions         []situation.Contribution
	DecisionEvidence      situation.DecisionEvidence
	SituationalResolution SituationalResolution
	Repair                PostExecutionRepairAssessment
	Valid                 bool
	Errors                []string
}

// RunConversationalCognitionE2E is the canonical B96.33 orchestration entry
// point. Every stage consumes the semantic/state contracts produced by the
// preceding stages; no stage reparses the raw user utterance.
func RunConversationalCognitionE2E(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot, situational ...SituationalCognitionContext) ConversationalCognitionE2E {
	out := ConversationalCognitionE2E{Version: "B96.33.12"}

	out.Model = NewConversationalCognitiveModel(req, ctx)
	if len(situational) > 0 {
		shared := situational[0]
		out.Situation = shared.Situation
		out.Contributions = append([]situation.Contribution(nil), shared.Contributions...)
		if strings.TrimSpace(shared.Iteration.ID) != "" {
			out.Situation = shared.Iteration.Situation
			out.Contributions = append([]situation.Contribution(nil), shared.Iteration.Contributions...)
		}
	}
	out.Intent = EvaluateCommunicativeIntent(out.Model)
	if len(situational) > 0 && strings.TrimSpace(situational[0].Iteration.ID) != "" &&
		situational[0].Iteration.Evidence.SituationID == out.Situation.ID &&
		situational[0].Iteration.Evidence.SituationVersion == out.Situation.Version {
		out.DecisionEvidence = situational[0].Iteration.Evidence
	} else {
		out.DecisionEvidence = situation.Evaluate(out.Situation, out.Contributions)
	}
	var authoritative []situation.Evidence
	if len(situational) > 0 {
		authoritative = append(authoritative, situational[0].AuthoritativeEvidence...)
	}
	out.Intent, out.SituationalResolution = resolveFromSituation(out.Intent, out.DecisionEvidence)
	out.Goal = InferConversationalGoal(out.Model, out.Intent)
	out.Relevance = EvaluateConversationalRelevance(out.Model, out.Intent, out.Goal)
	out.Interpretation = InterpretConversation(out.Model, out.Intent, out.Goal, out.Relevance)
	out.Decision = DecideConversationalParticipation(out.Model, out.Intent, out.Goal, out.Relevance, out.Interpretation)
	out.Uncertainty = EvaluateConversationalUncertainty(out.Model, out.Intent, out.Goal, out.Relevance, out.Interpretation, out.Decision)
	out.Conflict = EvaluateConversationalConflict(out.Model, out.Intent, out.Goal, out.Relevance, out.Interpretation, out.Decision, authoritative)
	if len(out.Conflict.AuthoritativeFindings) > 0 {
		out.DecisionEvidence.Findings = append(out.DecisionEvidence.Findings, out.Conflict.AuthoritativeFindings...)
	}
	out.Action = SelectConversationalAction(out.Model, out.Intent, out.Goal, out.Relevance, out.Interpretation, out.Decision, out.Uncertainty, out.Conflict)
	out.Strategy = SelectConversationalResponseStrategy(out.Model, out.Intent, out.Goal, out.Relevance, out.Interpretation, out.Decision, out.Uncertainty, out.Conflict, out.Action)
	out.Adaptation = AdaptConversationalParticipation(out.Model, out.Intent, out.Goal, out.Relevance, out.Interpretation, out.Decision, out.Uncertainty, out.Conflict, out.Action, out.Strategy, ctx)
	// State-derived participation changes are made before the final behavioral
	// decision. Language receives the resulting action; it is not asked to infer
	// user state and choose a different conduct on its own.
	out.Action = RefineActionForAdaptation(out.Action, out.Adaptation)
	out.Strategy = SelectConversationalResponseStrategy(out.Model, out.Intent, out.Goal, out.Relevance, out.Interpretation, out.Decision, out.Uncertainty, out.Conflict, out.Action)
	// Strategy carries the already-derived transient adaptation as expression
	// guidance. The final BehavioralDecision below owns the conduct/language gate.
	out.Strategy.Adaptation = out.Adaptation
	out.Behavior = ResolveConversationalBehavior(out.Action, out.Strategy, interaction.DetermineResponseIntent(req, ctx))
	if len(situational) > 0 {
		out.Repair = DetectPostExecutionRepair(req, ctx, situational[0].Work, situational[0].HasWork)
	}

	out.Valid, out.Errors = validateConversationalCognitionE2E(out)
	return out
}

func validateConversationalCognitionE2E(e ConversationalCognitionE2E) (bool, []string) {
	errors := make([]string, 0, 8)

	if e.Model.Version != "B96.33.1" {
		errors = append(errors, "cognitive model version mismatch")
	}
	if e.Intent.Version != "B96.33.2" {
		errors = append(errors, "intent evaluation version mismatch")
	}
	if e.Goal.Version != "B96.33.3" {
		errors = append(errors, "goal inference version mismatch")
	}
	if e.Relevance.Version != "B96.33.4" {
		errors = append(errors, "relevance version mismatch")
	}
	if e.Interpretation.Version != "B96.33.5" {
		errors = append(errors, "interpretation version mismatch")
	}
	if e.Decision.Version != "B96.33.6" {
		errors = append(errors, "decision version mismatch")
	}
	if e.Uncertainty.Version != "B96.33.7" {
		errors = append(errors, "uncertainty version mismatch")
	}
	if e.Conflict.Version != "B96.33.8" {
		errors = append(errors, "conflict version mismatch")
	}
	if e.Action.Version != "B96.33.9" {
		errors = append(errors, "action selection version mismatch")
	}
	if e.Strategy.Version != "B96.33.10" {
		errors = append(errors, "response strategy version mismatch")
	}
	if e.Adaptation.Version != "B96.33.11" {
		errors = append(errors, "adaptation version mismatch")
	}
	if e.Behavior.Version != "B96.40.1" || e.Behavior.Action != e.Action.Primary {
		errors = append(errors, "behavioral decision does not converge on the selected cognition action")
	}

	// B96.33 must never convert a proposal into execution.
	if e.Intent.Primary == IntentShareIdea && e.Action.Primary == ActionRequestExecution {
		errors = append(errors, "proposal was incorrectly converted into execution")
	}

	// Unresolved references must remain visible and cannot silently become an
	// executable action without clarification.
	if e.Interpretation.HasUnresolvedRefs && e.Action.Primary == ActionRequestExecution && !e.Action.Blocked {
		errors = append(errors, "unresolved reference bypassed action block")
	}

	// The E2E layer may expose an execution request conceptually, but it cannot
	// remove the authorization/execution boundary owned by downstream layers.
	if e.Action.Primary == ActionRequestExecution && !e.Strategy.ExecutionBoundary {
		errors = append(errors, "execution boundary was removed")
	}
	if e.Action.Primary == ActionPropose && !e.Strategy.PreserveUserAgency {
		errors = append(errors, "proposal lost user agency")
	}

	// Adaptation is explicitly transient; B96.34 owns persistent user modeling.
	if !e.Adaptation.Transient {
		errors = append(errors, "adaptation became persistent")
	}

	return len(errors) == 0, errors
}
