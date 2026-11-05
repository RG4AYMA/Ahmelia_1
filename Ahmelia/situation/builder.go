package situation

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/interaction"
	"ahmelia/ownership"
)

// Input contains only already-authoritative domain projections. The builder
// reconciles them into one derived Situation; it does not mutate any source.
type Input struct {
	Request           interaction.SemanticRequest
	SubjectResolution *ownership.Resolution
	Conversation      interaction.ConversationSnapshot
	Work              interaction.WorkContext
	HasWork           bool
	World             WorldEvidence
	Observations      []Observation
	Now               time.Time
}

type WorldEvidence struct {
	Available  bool
	App        string
	Activity   string
	Project    string
	File       string
	Workspace  string
	Unsaved    bool
	ObservedAt time.Time
	Source     string
}

func Build(in Input) Situation {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	s := Situation{
		ID:         situationID(in.Request, in.Conversation, in.Work, now),
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
		Scope:      "interaction",
		Temporal:   TemporalCurrent,
		Relevance:  1,
		Confidence: clamp(in.Request.Confidence),
	}

	if s.Confidence == 0 {
		s.Confidence = 0.5
	}

	r := strings.TrimSpace(in.Request.Raw)
	if r == "" {
		r = strings.TrimSpace(in.Request.Normalized)
	}
	s.Anchor = Anchor{Kind: anchorKind(in), Description: r, Source: "interaction.SemanticRequest", ObservedAt: now}

	addRequestEvidence(&s, in, now)
	addConversationEvidence(&s, in, now)
	addWorkEvidence(&s, in, now)
	addWorldEvidence(&s, in, now)
	addObservationEvidence(&s, in, now)
	deriveIntentionalSocialEvidence(&s, in, now)
	deriveRelations(&s)
	deriveUncertainty(&s, in)
	return s
}

func anchorKind(in Input) string {
	if strings.TrimSpace(in.Request.Raw) != "" {
		if in.Request.CommunicativeActs.Primary != "" {
			return string(in.Request.CommunicativeActs.Primary)
		}
		return string(in.Request.SpeechAct)
	}
	return "observation"
}

func addRequestEvidence(s *Situation, in Input, now time.Time) {
	r := in.Request
	subject := r.Subject
	subjectConfidence := r.SubjectConfidence
	var subjectResolution *ownership.Resolution
	if in.SubjectResolution != nil {
		subjectResolution = in.SubjectResolution
		subject = interaction.UserObservationSubject(in.SubjectResolution.Subject)
		subjectConfidence = in.SubjectResolution.Confidence
	} else if len(r.SubjectEvidence) > 0 || r.SubjectConfidence > 0 {
		subjectResolution = &ownership.Resolution{Subject: ownership.Subject(r.Subject), Confidence: r.SubjectConfidence, Evidence: append([]ownership.Evidence(nil), r.SubjectEvidence...)}
	}
	if subjectConfidence <= 0 && subject != "" {
		subjectConfidence = clamp(r.Confidence)
	}
	addEvidence(s, Evidence{ID: "semantic.turn", Authority: "interaction.SemanticRequest", Source: "B96.32", Subject: string(subject), Predicate: "communicates", Object: string(r.SpeechAct), Value: r.Normalized, ObservedAt: now, Temporal: TemporalCurrent, Relevance: 1, Confidence: clamp(r.Confidence), Epistemic: EpistemicObserved, Authoritative: true})
	if subject != "" && subjectResolution != nil {
		evidenceConfidence := clamp(subjectResolution.Confidence)
		if evidenceConfidence == 0 {
			evidenceConfidence = clamp(subjectConfidence)
		}
		for i, evidence := range subjectResolution.Evidence {
			if strings.TrimSpace(evidence.Token) == "" {
				continue
			}
			addEvidence(s, Evidence{
				ID: fmt.Sprintf("ownership.%d", i), Authority: "ownership.Resolver", Source: evidence.Provenance,
				Subject: string(subject), Predicate: evidence.Signal, Object: evidence.Token, Value: evidence.Provenance,
				ObservedAt: now, Temporal: TemporalCurrent, Relevance: .95, Confidence: evidenceConfidence,
				Epistemic: EpistemicDerived, Authoritative: false,
			})
		}
	}
	if strings.TrimSpace(r.Topic) != "" {
		addEvidence(s, Evidence{ID: "semantic.topic", Authority: "interaction.SemanticRequest", Source: "B96.32", Subject: "turn", Predicate: "about", Object: r.Topic, ObservedAt: now, Temporal: TemporalCurrent, Relevance: .95, Confidence: clamp(r.Confidence), Epistemic: EpistemicDerived, Authoritative: true})
	}
	if strings.TrimSpace(r.Objective.Text) != "" {
		addEvidence(s, Evidence{ID: "semantic.objective", Authority: "interaction.SemanticRequest", Source: "B96.32", Subject: "user", Predicate: "pursues", Object: r.Objective.Text, ObservedAt: now, Temporal: TemporalCurrent, Relevance: .95, Confidence: clamp(r.Confidence), Epistemic: EpistemicObserved, Authoritative: true})
	}
	for i, ref := range r.References {
		if strings.TrimSpace(ref.Value) == "" && !ref.Resolved {
			s.Uncertainty = append(s.Uncertainty, fmt.Sprintf("referencia no resuelta: %s", ref.Expression))
			continue
		}
		value := ref.Value
		if value == "" {
			value = ref.Expression
		}
		addEvidence(s, Evidence{ID: fmt.Sprintf("semantic.reference.%d", i), Authority: "contextintelligence", Source: "B96.36", Subject: "turn", Predicate: "refers_to", Object: value, ObservedAt: now, Temporal: TemporalCurrent, Relevance: .9, Confidence: .9, Epistemic: EpistemicDerived, Authoritative: true})
	}
}

func addConversationEvidence(s *Situation, in Input, now time.Time) {
	c := in.Conversation
	if strings.TrimSpace(c.ActiveTopic) != "" {
		addEvidence(s, Evidence{ID: "conversation.topic", Authority: "interaction.ConversationState", Source: "ConversationSnapshot", Subject: "conversation", Predicate: "active_topic", Object: c.ActiveTopic, ObservedAt: c.LastTurnAt, Temporal: TemporalCurrent, Relevance: .85, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if strings.TrimSpace(c.ActiveObjective) != "" {
		addEvidence(s, Evidence{ID: "conversation.objective", Authority: "interaction.ConversationState", Source: "ConversationSnapshot", Subject: "conversation", Predicate: "active_objective", Object: c.ActiveObjective, ObservedAt: c.LastTurnAt, Temporal: TemporalCurrent, Relevance: .85, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if c.ActiveWorkID != "" {
		addEvidence(s, Evidence{ID: "conversation.work", Authority: "interaction.ConversationState", Source: "ConversationSnapshot", Subject: "conversation", Predicate: "linked_to_work", Object: c.ActiveWorkID, ObservedAt: c.LastTurnAt, Temporal: TemporalCurrent, Relevance: .8, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if len(c.Decisions) > 0 {
		d := c.Decisions[len(c.Decisions)-1]
		addEvidence(s, Evidence{ID: "conversation.decision.latest", Authority: "interaction.ConversationState", Source: "ConversationSnapshot", Subject: "conversation", Predicate: "latest_decision", Object: d.Decision, ObservedAt: d.At, Temporal: TemporalRecent, Relevance: .65, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
}

func addWorkEvidence(s *Situation, in Input, now time.Time) {
	if !in.HasWork || strings.TrimSpace(in.Work.ID) == "" {
		return
	}
	at := in.Work.UpdatedAt
	if at.IsZero() {
		at = now
	}
	addEvidence(s, Evidence{ID: "work.identity", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: "situation", Predicate: "involves_work", Object: in.Work.ID, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .9, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	if in.Work.Status != "" {
		addEvidence(s, Evidence{ID: "work.status", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "status", Object: in.Work.Status, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .9, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.Objective != "" {
		addEvidence(s, Evidence{ID: "work.objective", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "pursues", Object: in.Work.Objective, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .9, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.CurrentStep != "" {
		addEvidence(s, Evidence{ID: "work.step", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "current_step", Object: in.Work.CurrentStep, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .85, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.LastResult != "" {
		addEvidence(s, Evidence{ID: "work.result", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_result", Object: in.Work.LastResult, ObservedAt: at, Temporal: TemporalRecent, Relevance: .85, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	// Preserve the epistemic state of the latest attempt as distinct facts.
	// These describe what the canonical WorkStore records; they do not turn a
	// provider claim into independent proof that the external effect occurred.
	if in.Work.OutcomeStatus != "" {
		addEvidence(s, Evidence{ID: "work.outcome_status", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_outcome_status", Object: in.Work.OutcomeStatus, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .95, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
		if in.Work.OutcomeCode != "" {
			addEvidence(s, Evidence{ID: "work.outcome_code", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_outcome_code", Object: in.Work.OutcomeCode, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .9, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
		}
		observedValue := "false"
		if in.Work.Observed {
			observedValue = "true"
		}
		addEvidence(s, Evidence{ID: "work.effect_observed", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_effect_observed", Object: observedValue, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .95, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
		verifiedValue := "false"
		if in.Work.LastActionVerified {
			verifiedValue = "true"
		}
		addEvidence(s, Evidence{ID: "work.effect_verified", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_effect_verified", Object: verifiedValue, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .95, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.VerificationSource != "" {
		addEvidence(s, Evidence{ID: "work.verification_source", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_verification_source", Object: in.Work.VerificationSource, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .85, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.VerificationReason != "" {
		addEvidence(s, Evidence{ID: "work.verification_reason", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_verification_reason", Object: in.Work.VerificationReason, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .85, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.FailureReason != "" {
		addEvidence(s, Evidence{ID: "work.failure_reason", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_failure_reason", Object: in.Work.FailureReason, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .9, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.AttemptNumber > 0 {
		addEvidence(s, Evidence{ID: "work.attempt_number", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_number", Object: fmt.Sprint(in.Work.AttemptNumber), ObservedAt: at, Temporal: TemporalCurrent, Relevance: .9, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.AttemptStatus != "" {
		addEvidence(s, Evidence{ID: "work.attempt_status", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "last_attempt_status", Object: in.Work.AttemptStatus, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .9, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.Work.ReconciliationPending {
		addEvidence(s, Evidence{ID: "work.reconciliation_pending", Authority: "production.WorkStore", Source: "interaction.WorkContext", Subject: in.Work.ID, Predicate: "requires_reconciliation", Object: "true", Value: "outcome is unknown or attempt is unverified; do not infer success or failure", ObservedAt: at, Temporal: TemporalCurrent, Relevance: 1, Confidence: .99, Epistemic: EpistemicObserved, Authoritative: true})
	}
}

func addObservationEvidence(s *Situation, in Input, now time.Time) {
	for i, o := range in.Observations {
		at := o.ObservedAt
		if at.IsZero() {
			at = now
		}
		confidence := clamp(o.Confidence)
		if confidence == 0 {
			confidence = 0.5
		}
		id := o.ID
		if id == "" {
			id = fmt.Sprintf("observation.%d", i)
		}
		addEvidence(s, Evidence{ID: id, Authority: o.Source, Source: o.Source, Subject: o.Subject, Predicate: o.Predicate, Object: o.Object, Value: o.Value, ObservedAt: at, Temporal: observationTemporal(o), Relevance: 0.9, Confidence: confidence, Epistemic: o.Epistemic, Authoritative: false})
	}
}

func deriveIntentionalSocialEvidence(s *Situation, in Input, now time.Time) {
	for i, rel := range in.Request.Relations {
		if strings.TrimSpace(rel.Source) == "" || strings.TrimSpace(rel.Predicate) == "" || strings.TrimSpace(rel.Target) == "" {
			continue
		}
		conf := clamp(rel.Confidence)
		if conf == 0 {
			conf = clamp(in.Request.Confidence)
		}
		if conf == 0 {
			conf = .5
		}
		id := fmt.Sprintf("semantic.relation.%d", i)
		addEvidence(s, Evidence{ID: id, Authority: "interaction.SemanticRequest", Source: "linguistics", Subject: rel.Source, Predicate: rel.Predicate, Object: rel.Target, Value: rel.Evidence, ObservedAt: now, Temporal: TemporalCurrent, Relevance: .9, Confidence: conf, Epistemic: EpistemicDerived, Authoritative: false})
	}
	if in.Request.Subject == interaction.UserObservationSubjectSelf {
		s.Entities = appendUniqueEntity(s.Entities, Entity{ID: "user", Kind: "actor", Name: "user", Authority: "interaction.SemanticRequest", Confidence: clamp(in.Request.Confidence)})
		s.Entities = appendUniqueEntity(s.Entities, Entity{ID: "ahmelia", Kind: "actor", Name: "Ahmelia", Authority: "self_knowledge", Confidence: 1})
		addEvidence(s, Evidence{ID: "social.address", Authority: "interaction.SemanticRequest", Source: "B96.32", Subject: "user", Predicate: "addresses", Object: "ahmelia", ObservedAt: now, Temporal: TemporalCurrent, Relevance: .9, Confidence: clamp(in.Request.Confidence), Epistemic: EpistemicDerived, Authoritative: true})
	}
	acts := interaction.CanonicalActs(in.Request)
	if in.Request.Subject == interaction.UserObservationSubjectSelf && acts.Has(interaction.ActQuestion) && isSelfStateQuestion(in.Request) {
		s.Entities = appendUniqueEntity(s.Entities, Entity{ID: "user", Kind: "actor", Name: "user", Authority: "interaction.SemanticRequest", Confidence: clamp(in.Request.Confidence)})
		s.Entities = appendUniqueEntity(s.Entities, Entity{ID: "ahmelia", Kind: "actor", Name: "Ahmelia", Authority: "self_knowledge", Confidence: 1})
		addEvidence(s, Evidence{ID: "self.state.question", Authority: "interaction.SemanticRequest", Source: "B96.32", Subject: "user", Predicate: "asks_about_state", Object: "ahmelia", ObservedAt: now, Temporal: TemporalCurrent, Relevance: .95, Confidence: clamp(in.Request.Confidence), Epistemic: EpistemicDerived, Authoritative: true})
	}

	if acts.Has(interaction.ActProposal) || in.Request.SpeechAct == interaction.SpeechPropose {
		s.Entities = appendUniqueEntity(s.Entities, Entity{ID: "user", Kind: "actor", Name: "user", Authority: "interaction.SemanticRequest", Confidence: clamp(in.Request.Confidence)})
		s.Entities = appendUniqueEntity(s.Entities, Entity{ID: "ahmelia", Kind: "actor", Name: "Ahmelia", Authority: "self_knowledge", Confidence: 1})
		addEvidence(s, Evidence{ID: "intent.proposal", Authority: "interaction.SemanticRequest", Source: "B96.32", Subject: "user", Predicate: "proposes", Object: "future_state", ObservedAt: now, Temporal: TemporalCurrent, Relevance: .9, Confidence: clamp(in.Request.Confidence), Epistemic: EpistemicDerived, Authoritative: true})
	}
	if in.Request.SpeechAct == interaction.SpeechFeedback {
		s.Entities = appendUniqueEntity(s.Entities, Entity{ID: "user", Kind: "actor", Name: "user", Authority: "interaction.SemanticRequest", Confidence: clamp(in.Request.Confidence)})
		s.Entities = appendUniqueEntity(s.Entities, Entity{ID: "ahmelia", Kind: "actor", Name: "Ahmelia", Authority: "self_knowledge", Confidence: 1})
		addEvidence(s, Evidence{ID: "social.feedback", Authority: "interaction.SemanticRequest", Source: "B96.32", Subject: "user", Predicate: "gives_feedback_to", Object: "ahmelia", ObservedAt: now, Temporal: TemporalCurrent, Relevance: .95, Confidence: clamp(in.Request.Confidence), Epistemic: EpistemicObserved, Authoritative: true})
	}
}

func appendUniqueEntity(items []Entity, e Entity) []Entity {
	for _, existing := range items {
		if existing.ID == e.ID {
			return items
		}
	}
	return append(items, e)
}

func addWorldEvidence(s *Situation, in Input, now time.Time) {
	if !in.World.Available {
		return
	}
	at := in.World.ObservedAt
	if at.IsZero() {
		at = now
	}
	if in.World.App != "" {
		addEvidence(s, Evidence{ID: "world.app", Authority: "environment.WorldState", Source: in.World.Source, Subject: "world", Predicate: "active_app", Object: in.World.App, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .55, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.World.Activity != "" {
		addEvidence(s, Evidence{ID: "world.activity", Authority: "environment.WorldState", Source: in.World.Source, Subject: "world", Predicate: "activity", Object: in.World.Activity, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .6, Confidence: .9, Epistemic: EpistemicDerived, Authoritative: true})
	}
	if in.World.Project != "" {
		addEvidence(s, Evidence{ID: "world.project", Authority: "environment.WorldState", Source: in.World.Source, Subject: "world", Predicate: "project", Object: in.World.Project, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .8, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.World.File != "" {
		addEvidence(s, Evidence{ID: "world.file", Authority: "environment.WorldState", Source: in.World.Source, Subject: "world", Predicate: "file", Object: in.World.File, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .65, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.World.Workspace != "" {
		addEvidence(s, Evidence{ID: "world.workspace", Authority: "environment.WorldState", Source: in.World.Source, Subject: "world", Predicate: "workspace", Object: in.World.Workspace, ObservedAt: at, Temporal: TemporalCurrent, Relevance: .8, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
	if in.World.Unsaved {
		addEvidence(s, Evidence{ID: "world.unsaved", Authority: "environment.WorldState", Source: in.World.Source, Subject: "world", Predicate: "has_unsaved_changes", Object: "true", ObservedAt: at, Temporal: TemporalCurrent, Relevance: .75, Confidence: .95, Epistemic: EpistemicObserved, Authoritative: true})
	}
}

func isSelfStateQuestion(r interaction.SemanticRequest) bool {
	acts := interaction.CanonicalActs(r)
	if !acts.Has(interaction.ActQuestion) {
		return false
	}
	// Social questions and questions with a process/explanation form are
	// runtime-state candidates when the owner is already established as self.
	// A stored self fact, such as a preference or name, is still allowed to
	// take its dedicated self-knowledge route.
	if acts.Has(interaction.ActSocial) || r.QuestionType == "procedure_or_explanation" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.QuestionType), "state")
}

func deriveRelations(s *Situation) {
	for _, e := range s.Evidence {
		if e.Subject == "" || e.Object == "" {
			continue
		}
		s.Relations = append(s.Relations, Relation{Source: e.Subject, Target: e.Object, Type: e.Predicate, EvidenceIDs: []string{e.ID}, Provenance: e.Authority, Temporal: e.Temporal, Relevance: e.Relevance, Confidence: e.Confidence, Epistemic: e.Epistemic})
	}
	if hasEvidence(s, "semantic.objective") && hasEvidence(s, "work.objective") {
		a, _ := evidence(s, "semantic.objective")
		b, _ := evidence(s, "work.objective")
		if strings.TrimSpace(a.Object) != strings.TrimSpace(b.Object) {
			s.Conflicts = append(s.Conflicts, "el objetivo inmediato de la interacción difiere del objetivo del trabajo activo")
		}
	}
}

func deriveUncertainty(s *Situation, in Input) {
	if in.HasWork && in.Work.ReconciliationPending {
		s.Uncertainty = append(s.Uncertainty, "el último intento del trabajo requiere reconciliación; no debe inferirse éxito ni fracaso del efecto")
	}
	if in.Request.Confidence > 0 && in.Request.Confidence < .6 {
		s.Uncertainty = append(s.Uncertainty, "interpretación semántica de baja confianza")
	}
	if in.Request.NeedsClarification {
		s.Uncertainty = append(s.Uncertainty, "el turno requiere aclaración")
	}
	if len(s.Relations) == 0 {
		s.Uncertainty = append(s.Uncertainty, "no hay relaciones situacionales suficientes")
	}
}

func addEvidence(s *Situation, e Evidence)     { s.Evidence = append(s.Evidence, e) }
func hasEvidence(s *Situation, id string) bool { _, ok := evidence(s, id); return ok }
func evidence(s *Situation, id string) (Evidence, bool) {
	for _, e := range s.Evidence {
		if e.ID == id {
			return e, true
		}
	}
	return Evidence{}, false
}
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
func situationID(r interaction.SemanticRequest, c interaction.ConversationSnapshot, w interaction.WorkContext, now time.Time) string {
	anchor := strings.TrimSpace(r.Normalized)
	if anchor == "" {
		anchor = strings.TrimSpace(r.Raw)
	}
	base := c.ActiveThreadID + "|" + w.ID + "|" + anchor
	return fmt.Sprintf("sit-%x-%d", simpleHash(base), now.UnixNano())
}
func simpleHash(s string) uint64 {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
