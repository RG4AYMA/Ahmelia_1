package cognition

import "ahmelia/interaction"

// B96.33.10 — CONVERSATIONAL RESPONSE STRATEGY
//
// Selects how Ahmelia should formulate her conversational participation after
// B96.33.9 has selected the abstract conversational action. This is a strategy
// contract, not generated language: it describes response posture, structure,
// directness, initiative, and whether an operational request should be framed
// as a downstream request rather than executed here.

type ConversationalResponseMode string

const (
	ResponseModeUnknown          ConversationalResponseMode = "unknown"
	ResponseModeAnswer           ConversationalResponseMode = "answer"
	ResponseModeClarify          ConversationalResponseMode = "clarify"
	ResponseModeDiscuss          ConversationalResponseMode = "discuss"
	ResponseModePropose          ConversationalResponseMode = "propose"
	ResponseModeCorrect          ConversationalResponseMode = "correct"
	ResponseModeDisagree         ConversationalResponseMode = "disagree"
	ResponseModeConfirm          ConversationalResponseMode = "confirm"
	ResponseModeRequestExecution ConversationalResponseMode = "request_execution"
	ResponseModeWait             ConversationalResponseMode = "wait"
	ResponseModeContinue         ConversationalResponseMode = "continue"
	ResponseModeChangeTopic      ConversationalResponseMode = "change_topic"
)

type ResponseDirectness string

const (
	DirectnessUnknown     ResponseDirectness = "unknown"
	DirectnessDirect      ResponseDirectness = "direct"
	DirectnessGuided      ResponseDirectness = "guided"
	DirectnessExploratory ResponseDirectness = "exploratory"
	DirectnessCautious    ResponseDirectness = "cautious"
)

type ResponseStructure string

const (
	StructureUnknown        ResponseStructure = "unknown"
	StructureConcise        ResponseStructure = "concise"
	StructureExplanation    ResponseStructure = "explanation"
	StructureClarifying     ResponseStructure = "clarifying_question"
	StructureDiscussion     ResponseStructure = "discussion"
	StructureProposal       ResponseStructure = "proposal"
	StructureCorrection     ResponseStructure = "correction"
	StructureActionBoundary ResponseStructure = "action_boundary"
)

type ResponseInitiative string

const (
	InitiativeUnknown ResponseInitiative = "unknown"
	InitiativeMinimal ResponseInitiative = "minimal"
	InitiativeHelpful ResponseInitiative = "helpful"
	InitiativeGuiding ResponseInitiative = "guiding"
)

type ConversationalResponseStrategy struct {
	Version                string
	Mode                   ConversationalResponseMode
	Directness             ResponseDirectness
	Structure              ResponseStructure
	Initiative             ResponseInitiative
	Confidence             float64
	Ambiguous              bool
	NeedsClarification     bool
	PreserveUserAgency     bool
	ExecutionBoundary      bool
	ShouldGenerateLanguage bool
	Evidence               []string
	Reason                 string
	Adaptation             ConversationalAdaptation
}

// SelectConversationalResponseStrategy derives a response strategy from the
// already-established conversational cognition chain. It does not generate
// text, call the LLM, authorize work, select a capability/provider, or mutate
// conversation state.
func SelectConversationalResponseStrategy(
	model ConversationalCognitiveModel,
	intent CommunicativeIntentEvaluation,
	goal ConversationalGoalInference,
	relevance ConversationalRelevance,
	interpretation ConversationalInterpretation,
	decision ConversationalDecision,
	uncertainty ConversationalUncertainty,
	conflict ConversationalConflict,
	action ConversationalActionSelection,
) ConversationalResponseStrategy {
	out := ConversationalResponseStrategy{
		Version:                "B96.33.10",
		Mode:                   ResponseModeUnknown,
		Directness:             DirectnessDirect,
		Structure:              StructureConcise,
		Initiative:             InitiativeHelpful,
		Confidence:             clamp01(minFloat(model.Signals.Confidence, intent.Confidence, goal.Confidence, relevance.Confidence, interpretation.Confidence, decision.Confidence)),
		PreserveUserAgency:     true,
		ExecutionBoundary:      true,
		ShouldGenerateLanguage: true,
	}

	// Action Selection is authoritative for the immediate conversational
	// posture. Strategy refines presentation; it must not invent another action.
	switch action.Primary {
	case ActionRespond:
		out.Mode = ResponseModeAnswer
		out.Structure = StructureExplanation
		out.Evidence = append(out.Evidence, "action:respond")
	case ActionClarify:
		out.Mode = ResponseModeClarify
		out.Directness = DirectnessCautious
		out.Structure = StructureClarifying
		out.Initiative = InitiativeGuiding
		out.NeedsClarification = true
		out.Evidence = append(out.Evidence, "action:clarify")
	case ActionDiscuss:
		out.Mode = ResponseModeDiscuss
		out.Directness = DirectnessExploratory
		out.Structure = StructureDiscussion
		out.Evidence = append(out.Evidence, "action:discuss")
	case ActionPropose:
		out.Mode = ResponseModePropose
		out.Directness = DirectnessGuided
		out.Structure = StructureProposal
		out.Evidence = append(out.Evidence, "action:propose", "boundary:proposal_is_not_execution")
	case ActionCorrect:
		out.Mode = ResponseModeCorrect
		out.Structure = StructureCorrection
		out.Evidence = append(out.Evidence, "action:correct")
	case ActionDisagree:
		out.Mode = ResponseModeDisagree
		out.Directness = DirectnessDirect
		out.Structure = StructureExplanation
		out.Evidence = append(out.Evidence, "action:disagree")
	case ActionConfirm:
		out.Mode = ResponseModeConfirm
		out.Structure = StructureConcise
		out.Evidence = append(out.Evidence, "action:confirm")
	case ActionRequestExecution:
		out.Mode = ResponseModeRequestExecution
		out.Directness = DirectnessDirect
		out.Structure = StructureActionBoundary
		out.Initiative = InitiativeMinimal
		out.Evidence = append(out.Evidence, "action:request_execution", "boundary:authorization_and_execution_are_downstream")
	case ActionWait:
		out.Mode = ResponseModeWait
		out.Initiative = InitiativeMinimal
		out.Evidence = append(out.Evidence, "action:wait")
	case ActionContinueTopic:
		out.Mode = ResponseModeContinue
		out.Structure = StructureDiscussion
		out.Evidence = append(out.Evidence, "action:continue_topic")
	case ActionChangeTopic:
		out.Mode = ResponseModeChangeTopic
		out.Structure = StructureConcise
		out.Evidence = append(out.Evidence, "action:change_topic")
	default:
		out.Mode = ResponseModeUnknown
		out.Directness = DirectnessCautious
		out.Initiative = InitiativeMinimal
		out.NeedsClarification = true
		out.Evidence = append(out.Evidence, "action:unknown")
	}

	// Cognitive gates can refine strategy, but cannot manufacture an execution
	// posture. A blocked/ambiguous action always becomes a clarification strategy.
	if action.Primary == ActionRequestExecution &&
		(action.Blocked || action.Ambiguous || action.NeedsClarification || uncertainty.RequiresClarificationBeforeAction(decision) || conflict.RequiresClarificationBeforeAction(decision)) {
		out.Mode = ResponseModeClarify
		out.Directness = DirectnessCautious
		out.Structure = StructureClarifying
		out.Initiative = InitiativeGuiding
		out.NeedsClarification = true
		out.Evidence = append(out.Evidence, "gate:execution_strategy_replaced_by_clarification")
	}
	if interpretation.HasUnresolvedRefs {
		out.NeedsClarification = true
		if action.Primary == ActionClarify || action.Primary == ActionRequestExecution {
			out.Mode = ResponseModeClarify
			out.Structure = StructureClarifying
		}
		out.Evidence = append(out.Evidence, "interpretation:unresolved_reference")
	}
	if interpretation.NeedsClarification || uncertainty.NeedsClarification || conflict.NeedsClarification {
		out.NeedsClarification = true
	}

	// A proposal remains a proposal even when active work exists. Strategy may
	// make the contribution explicit, but never turns it into an execution.
	if action.Primary == ActionPropose || intent.Primary == IntentShareIdea {
		out.PreserveUserAgency = true
		out.Evidence = append(out.Evidence, "agency:proposal_remains_optional")
	}

	// The strategy layer always describes language realization; it does not
	// itself contain the generated response text. phi4-mini remains an adapter
	// for language realization, not the owner of this cognitive decision.
	out.Ambiguous = action.Ambiguous || decision.Ambiguous || interpretation.Ambiguous
	out.Evidence = uniqueStrings(out.Evidence)

	if out.Mode == ResponseModeClarify {
		out.Reason = "clarification strategy selected because the current conversational posture is not sufficiently grounded"
	} else if out.Mode == ResponseModeRequestExecution {
		out.Reason = "execution request strategy selected conceptually; authorization and execution remain downstream"
	} else if out.Mode == ResponseModePropose {
		out.Reason = "proposal strategy preserves optionality and user agency rather than implying execution"
	} else {
		out.Reason = "response strategy derived from the selected conversational action and established cognitive evidence"
	}
	return out
}

// Keep the shared interaction contract visible in this block: response
// strategy consumes established semantics and does not create another parser.
var _ interaction.SemanticRequest
