package cognition

import (
	"strings"
	"time"

	"ahmelia/interaction"
)

// ============================================================================
// B96.33.1 — CONVERSATIONAL COGNITIVE MODEL
// ============================================================================
//
// This block defines the cognitive contract that sits AFTER B96.32
// Conversational State and BEFORE later conversational cognition blocks.
//
// B96.32 answers:
//   "What is happening in the interaction?"
//
// B96.33.1 answers:
//   "What kind of cognitive situation is this?"
//
// It deliberately does NOT:
//   - generate language;
//   - execute capabilities;
//   - choose a final response;
//   - create durable user memory;
//   - replace ConversationState;
//   - replace reasoning/planning;
//   - reparse raw language through a second classifier.
//
// The model is an assessment/coordination contract. Later B96.33 blocks
// enrich this contract with intent evaluation, goal inference, relevance,
// interpretation, uncertainty, conflict, decision and response strategy.
//
// The source of semantic truth remains interaction.SemanticRequest.
// The source of conversational state remains interaction.ConversationSnapshot.
// Cognition consumes those contracts; it does not take ownership of them.

// ConversationalCognitiveModel is the domain-neutral cognitive projection
// for one conversational turn.
//
// It is intentionally descriptive. Producing this value must not mutate
// ConversationState, execute work, or emit a response.
type ConversationalCognitiveModel struct {
	Version   string
	CreatedAt time.Time

	// Current semantic evidence. This is a value copy, not ownership.
	Request interaction.SemanticRequest

	// Current interaction state. ConversationState remains authoritative.
	Context interaction.ConversationSnapshot

	// The model's initial cognitive framing of the turn.
	Situation ConversationalSituation

	// Stable high-level signals exposed for later B96.33 blocks.
	Signals ConversationalCognitiveSignals

	// Provenance makes the contract auditable: later cognition should be
	// able to explain which existing layer supplied the evidence.
	Provenance CognitiveProvenance
}

// ConversationalSituation describes what cognitive space the current turn
// occupies without deciding the final response or action.
type ConversationalSituation struct {
	Kind           SituationKind
	Topic          string
	ActiveGoal     string
	HasActiveWork  bool
	HasHistory     bool
	IsFollowUp     bool
	IsContinuation bool
}

// SituationKind is intentionally coarse in B96.33.1. Fine-grained
// communicative intent belongs to B96.33.2.
type SituationKind string

const (
	SituationUnknown      SituationKind = "unknown"
	SituationConversation SituationKind = "conversation"
	SituationQuestion     SituationKind = "question"
	SituationAction       SituationKind = "action"
	SituationProduction   SituationKind = "production"
	SituationCorrection   SituationKind = "correction"
	SituationProposal     SituationKind = "proposal"
	SituationLearning     SituationKind = "learning"
	SituationPlanning     SituationKind = "planning"
	SituationReasoning    SituationKind = "reasoning"
	SituationFeedback     SituationKind = "feedback"
	SituationControl      SituationKind = "control"
)

// ConversationalCognitiveSignals are observations, not decisions.
//
// In particular, NeedsAction does NOT authorize execution. It only exposes
// that the semantic request belongs to an action/production family. The
// authorization/execution contract remains downstream.
type ConversationalCognitiveSignals struct {
	HasSemanticEvidence bool
	HasReferences       bool
	HasUnresolvedRefs   bool
	HasActiveContext    bool
	NeedsClarification  bool
	NeedsAction         bool
	GoalDirected        bool
	Confidence          float64
}

// CognitiveProvenance records the existing architectural sources used to
// construct the model. This prevents B96.33.1 from becoming an opaque,
// parallel interpretation path.
type CognitiveProvenance struct {
	SemanticSource string
	StateSource    string
	DerivedOnly    bool
}

// NewConversationalCognitiveModel creates the B96.33.1 contract from the
// existing semantic request and B96.32 snapshot.
//
// It performs only deterministic framing. It does not call an LLM, mutate
// state, write memory, execute tools, or select a response.
func NewConversationalCognitiveModel(
	req interaction.SemanticRequest,
	ctx interaction.ConversationSnapshot,
) ConversationalCognitiveModel {
	return ConversationalCognitiveModel{
		Version:   "B96.33.1",
		CreatedAt: time.Now(),
		Request:   cloneSemanticRequest(req),
		Context:   cloneConversationSnapshot(ctx),
		Situation: buildConversationalSituation(req, ctx),
		Signals:   buildConversationalSignals(req, ctx),
		Provenance: CognitiveProvenance{
			SemanticSource: "interaction.SemanticRequest",
			StateSource:    "interaction.ConversationSnapshot",
			DerivedOnly:    true,
		},
	}
}

func buildConversationalSituation(
	req interaction.SemanticRequest,
	ctx interaction.ConversationSnapshot,
) ConversationalSituation {
	kind := situationKindFor(req)
	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		topic = strings.TrimSpace(ctx.ActiveTopic)
	}

	goal := strings.TrimSpace(req.Objective.Text)
	if goal == "" {
		goal = strings.TrimSpace(ctx.ActiveObjective)
	}

	return ConversationalSituation{
		Kind:           kind,
		Topic:          topic,
		ActiveGoal:     goal,
		HasActiveWork:  strings.TrimSpace(ctx.ActiveWorkID) != "",
		HasHistory:     len(ctx.Turns) > 0,
		IsFollowUp:     req.FollowUp,
		IsContinuation: req.Continues || interaction.CanonicalActs(req).Has(interaction.ActContinuation),
	}
}

func situationKindFor(req interaction.SemanticRequest) SituationKind {
	a := interaction.CanonicalActs(req)
	switch {
	case a.Has(interaction.ActControl):
		return SituationControl
	case a.Has(interaction.ActCorrection):
		return SituationCorrection
	case a.Has(interaction.ActQuestion), a.Has(interaction.ActOpinionRequest):
		return SituationQuestion
	case a.Has(interaction.ActProposal):
		return SituationProposal
	case a.Has(interaction.ActLearning):
		return SituationLearning
	case a.Has(interaction.ActCommand):
		if req.Action.Name != "" {
			return SituationAction
		}
		return SituationProduction
	case a.Has(interaction.ActDecision):
		return SituationPlanning
	case a.Has(interaction.ActFeedback):
		return SituationFeedback
	case a.Has(interaction.ActExploration), a.Has(interaction.ActIdea):
		return SituationConversation
	case a.Has(interaction.ActContinuation), a.Has(interaction.ActInform), a.Has(interaction.ActDeclaration), a.Has(interaction.ActObservation), a.Has(interaction.ActSocial):
		return SituationConversation
	default:
		return SituationUnknown
	}
}

func buildConversationalSignals(
	req interaction.SemanticRequest,
	ctx interaction.ConversationSnapshot,
) ConversationalCognitiveSignals {
	unresolved := false
	for _, ref := range req.References {
		if strings.TrimSpace(ref.Expression) == "" || ref.Resolved {
			continue
		}
		if ref.Role == interaction.ReferenceRoleRequirement || ref.Role == interaction.ReferenceRoleRecoveryTarget {
			continue
		}
		unresolved = true
		break
	}

	hasContext := strings.TrimSpace(ctx.ActiveTopic) != "" ||
		strings.TrimSpace(ctx.ActiveObjective) != "" ||
		strings.TrimSpace(ctx.ActiveThreadID) != "" ||
		strings.TrimSpace(ctx.ActiveWorkID) != ""

	goalDirected := strings.TrimSpace(req.Objective.Text) != "" &&
		!interaction.CanonicalActs(req).Has(interaction.ActSocial) &&
		!interaction.CanonicalActs(req).Has(interaction.ActInform)

	needsAction := interaction.CanonicalActs(req).Has(interaction.ActCommand)

	return ConversationalCognitiveSignals{
		HasSemanticEvidence: strings.TrimSpace(req.Normalized) != "" &&
			interaction.CanonicalActs(req).Primary != interaction.ActUnknown,
		HasReferences:      len(req.References) > 0,
		HasUnresolvedRefs:  unresolved,
		HasActiveContext:   hasContext,
		NeedsClarification: req.NeedsClarification || unresolved,
		NeedsAction:        needsAction,
		GoalDirected:       goalDirected,
		Confidence:         clamp01(req.Confidence),
	}
}

// IsCognitivelyReady reports whether the B96.33 pipeline has enough basic
// structure to continue to the next block. "Ready" does not mean "correct"
// or "safe to act"; later evaluation/uncertainty/authority layers still apply.
func (m ConversationalCognitiveModel) IsCognitivelyReady() bool {
	if !m.Signals.HasSemanticEvidence {
		return false
	}
	if m.Situation.Kind == SituationUnknown {
		return false
	}
	return true
}

func cloneSemanticRequest(in interaction.SemanticRequest) interaction.SemanticRequest {
	out := in
	out.References = append([]interaction.Reference(nil), in.References...)
	out.Entities = append([]string(nil), in.Entities...)
	out.Objective.Constraints = append([]string(nil), in.Objective.Constraints...)
	out.Action.Parameters = cloneStringMap(in.Action.Parameters)
	return out
}

func cloneConversationSnapshot(in interaction.ConversationSnapshot) interaction.ConversationSnapshot {
	out := in
	out.Turns = append([]interaction.TurnRecord(nil), in.Turns...)
	out.OpenThreads = append([]interaction.ConversationThread(nil), in.OpenThreads...)
	out.Decisions = append([]interaction.ConversationDecision(nil), in.Decisions...)
	out.Commitments = append([]interaction.ConversationCommitment(nil), in.Commitments...)
	out.Objectives = append([]interaction.InteractionObjective(nil), in.Objectives...)
	out.WorkLinks = append([]interaction.ConversationWorkLink(nil), in.WorkLinks...)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
