package interaction

import (
	"strings"
	"time"
)

// WorkRelation describes how a conversational turn relates to operational
// work. It is deliberately not a production status and does not execute or
// mutate work.
type WorkRelation string

const (
	WorkRelationNone     WorkRelation = "none"
	WorkRelationAttach   WorkRelation = "attach"
	WorkRelationContinue WorkRelation = "continue"
	WorkRelationResume   WorkRelation = "resume"
	WorkRelationPreserve WorkRelation = "preserve"
	WorkRelationStartNew WorkRelation = "start_new"
)

// WorkContext is the small, domain-neutral view of operational work that the
// interaction layer is allowed to see. The production package owns the real
// WorkSession; this type prevents an import cycle and keeps Interaction
// capability-agnostic.
type WorkContext struct {
	ID                    string
	Status                string
	Objective             string
	Provider              string
	CurrentStep           string
	LastResult            string
	HasMaterializedResult bool
	LastActionSuccess     bool
	LastActionVerified    bool
	OutcomeStatus         string
	OutcomeCode           string
	Observed              bool
	VerificationSource    string
	VerificationReason    string
	FailureReason         string
	AttemptNumber         int
	AttemptStatus         string
	ReconciliationPending bool
	UpdatedAt             time.Time
}

// ConversationWorkLink records the semantic relationship between a
// conversation/thread and operational work. It is a link, not ownership of
// the work itself.
type ConversationWorkLink struct {
	ThreadID     string       `json:"thread_id,omitempty"`
	WorkID       string       `json:"work_id,omitempty"`
	Relation     WorkRelation `json:"relation"`
	SourceTurnID uint64       `json:"source_turn_id,omitempty"`
	At           time.Time    `json:"at"`
	Confidence   float64      `json:"confidence"`
	Reason       string       `json:"reason,omitempty"`
}

// WorkBridgeDecision is the result of B96.22.23. The decision tells the
// caller how conversation and work are related, while leaving execution,
// authorization and lifecycle changes to the work system.
type WorkBridgeDecision struct {
	Relation        WorkRelation
	WorkID          string
	ThreadID        string
	Objective       string
	AppendObjective bool
	Confidence      float64
	Reason          string
	ShouldCreate    bool
}

// WorkBridge is the explicit boundary between conversational continuity and
// operational work. It is intentionally stateless: durable state remains in
// ConversationState and the real work store.
type WorkBridge struct{}

func NewWorkBridge() *WorkBridge {
	return &WorkBridge{}
}

// Resolve determines how the current semantic turn relates to the current
// work context.
//
// Rules:
//   - production requests without work attach to a new work session;
//   - explicit/recent-work references resume the current work;
//   - continuation/correction turns addressed to the current context continue it;
//   - an unrelated conversational/question turn preserves work in the background;
//   - a new production objective while work exists starts a separate work
//     session rather than silently rewriting the existing one.
//
// The bridge never creates, pauses, completes, or executes work.
func (b *WorkBridge) Resolve(req SemanticRequest, conversation ConversationSnapshot, work WorkContext, hasWork bool) WorkBridgeDecision {
	threadID := conversation.ActiveThreadID
	objective := strings.TrimSpace(req.Objective.Text)
	if objective == "" {
		objective = strings.TrimSpace(conversation.ActiveObjective)
	}

	if !hasWork || strings.TrimSpace(work.ID) == "" {
		if CanonicalActs(req).Has(ActCommand) || req.Kind == KindProduction || req.Kind == KindAction {
			return WorkBridgeDecision{
				Relation: WorkRelationAttach, WorkID: "", ThreadID: threadID,
				Objective: objective, Confidence: 0.96, Reason: "production request has no active work; downstream work runtime may attach it",
				ShouldCreate: true,
			}
		}
		return WorkBridgeDecision{
			Relation: WorkRelationNone, ThreadID: threadID, Objective: objective,
			Confidence: 0.90, Reason: "conversation has no operational work to bridge",
		}
	}

	workID := strings.TrimSpace(work.ID)
	recentWorkRef := hasReference(req, ReferenceRecentWork)
	if recentWorkRef || explicitWorkResume(req.Normalized) {
		return WorkBridgeDecision{
			Relation: WorkRelationResume, WorkID: workID, ThreadID: threadID,
			Objective:  chooseWorkObjective(objective, work.Objective),
			Confidence: 0.97, Reason: "explicit or semantic reference to recent work",
		}
	}

	if CanonicalActs(req).Has(ActCommand) || req.Kind == KindProduction || req.Kind == KindAction {
		// A pending work session is also an objective-drafting boundary.
		// Voice/text input can arrive as several utterances, so a production
		// turn that elaborates an unfinished objective must remain attached to
		// the same Work instead of silently creating a second owner. A genuinely
		// new objective still wins when the user marks a topic/work boundary.
		if isPendingWorkSpecification(req, work) {
			return WorkBridgeDecision{
				Relation: WorkRelationContinue, WorkID: workID, ThreadID: threadID,
				Objective: chooseWorkObjective(objective, work.Objective), AppendObjective: true, Confidence: 0.96,
				Reason: "production turn continues an unfinished work objective",
			}
		}
		return WorkBridgeDecision{
			Relation: WorkRelationStartNew, WorkID: workID, ThreadID: threadID,
			Objective: objective, Confidence: 0.94,
			Reason:       "new production objective must not overwrite the existing work session",
			ShouldCreate: true,
		}
	}

	if CanonicalActs(req).Has(ActContinuation) || CanonicalActs(req).Has(ActCorrection) ||
		req.Continues || req.FollowUp || req.AddressesCurrent ||
		hasReference(req, ReferenceRecentArtifact) {
		appendObjective := !CanonicalActs(req).Has(ActCorrection) && isPendingWorkSpecification(req, work)
		return WorkBridgeDecision{
			Relation: WorkRelationContinue, WorkID: workID, ThreadID: threadID,
			Objective: chooseWorkObjective(objective, work.Objective), AppendObjective: appendObjective,
			Confidence: 0.94, Reason: "turn is context-dependent and addresses current operational work",
		}
	}

	// Questions and social conversation do not cancel or mutate work. Keeping
	// the relationship explicit is what lets a later "volvamos al proyecto"
	// resume the same work instead of creating a duplicate.
	return WorkBridgeDecision{
		Relation: WorkRelationPreserve, WorkID: workID, ThreadID: threadID,
		Objective: work.Objective, Confidence: 0.88,
		Reason: "current turn is conversationally unrelated to active work; preserve work for later resumption",
	}
}

func chooseWorkObjective(current, work string) string {
	if strings.TrimSpace(current) != "" {
		return strings.TrimSpace(current)
	}
	return strings.TrimSpace(work)
}

// HasReference exposes the existing reference predicate to orchestration
// boundaries that need to distinguish explicit resume from implicit history.
func HasReference(req SemanticRequest, kind ReferenceKind) bool {
	return hasReference(req, kind)
}

func hasReference(req SemanticRequest, kind ReferenceKind) bool {
	for _, ref := range req.References {
		if ref.Kind == kind {
			return true
		}
	}
	return false
}

// IsExplicitWorkResume exposes the existing work-resume boundary to higher
// layers without duplicating its lexical authority.
func IsExplicitWorkResume(n string) bool {
	return explicitWorkResume(n)
}

func explicitWorkResume(n string) bool {
	n = strings.ToLower(strings.TrimSpace(n))
	for _, marker := range []string{
		"volvamos al trabajo", "volvamos al proyecto", "retomemos el trabajo",
		"retomemos el proyecto", "sigamos con el proyecto", "sigamos con el trabajo",
		"back to the work", "back to the project", "resume the work", "resume the project",
	} {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

func isPendingWorkSpecification(req SemanticRequest, work WorkContext) bool {
	status := strings.ToLower(strings.TrimSpace(work.Status))
	if status != "pending" && status != "blocked" {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(req.Normalized))
	if n == "" || explicitNewWorkBoundary(n) {
		return false
	}
	if req.FollowUp || req.Continues || req.AddressesCurrent {
		return true
	}
	if req.Kind == KindProduction || req.Kind == KindAction {
		return true
	}
	return objectiveSpecificationMarker(n)
}

func objectiveSpecificationMarker(n string) bool {
	if strings.HasSuffix(strings.TrimSpace(n), ":") {
		return true
	}
	markers := []string{
		"objetivo:", "condiciones:", "requisitos:", "restricciones:", "criterios:",
		"ademas ", "tambien ", "también ", "debe ", "deben ",
		"asegurate ", "asegúrate ", "verifica ", "ejecuta ", "comprueba ",
		"si encuentras ", "if you find ", "make sure ", "requirements:", "constraints:",
	}
	for _, marker := range markers {
		if strings.HasPrefix(n, marker) || strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

// IsExplicitNewObjective exposes the existing conversational boundary to
// higher layers without duplicating its lexical authority.
func IsExplicitNewObjective(n string) bool {
	return explicitNewWorkBoundary(strings.ToLower(strings.TrimSpace(n)))
}

func explicitNewWorkBoundary(n string) bool {
	for _, marker := range []string{
		"nuevo objetivo", "nueva tarea", "otra tarea", "otro proyecto",
		"nuevo proyecto", "empecemos otra", "hagamos otra cosa",
		"new objective", "new task", "another task", "another project",
	} {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

// ComposeObjective merges a new conversational specification into the active
// objective without creating a second work owner. It is intentionally
// domain-neutral: the current utterance becomes additional objective text
// only when it is semantically part of the same unfinished work.
func ComposeObjective(base, addition string) string {
	base = strings.TrimSpace(base)
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return base
	}
	addition = strings.TrimSpace(strings.TrimLeft(addition, "-*•\t "))
	if isObjectiveLabel(addition) {
		return base
	}
	if base == "" {
		return addition
	}
	if strings.EqualFold(base, addition) || strings.Contains(strings.ToLower(base), strings.ToLower(addition)) {
		return base
	}
	return base + "\n" + addition
}

func isObjectiveLabel(text string) bool {
	n := strings.ToLower(strings.TrimSpace(text))
	n = strings.TrimSpace(strings.TrimRight(n, ":"))
	switch n {
	case "objetivo", "condiciones", "requisitos", "restricciones", "criterios",
		"objective", "conditions", "requirements", "constraints", "criteria":
		return true
	default:
		return false
	}
}
