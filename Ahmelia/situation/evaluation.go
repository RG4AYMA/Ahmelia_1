package situation

import (
	"strings"
	"time"
)

// Criterion is a contextual property that matters when interpreting or
// handing a Situation to a downstream authority. It is not a rule engine and
// it does not select an action.
type Criterion struct {
	ID          string
	Name        string
	Kind        string
	Reason      string
	EvidenceIDs []string
	Confidence  float64
	Relevance   float64
}

// Evaluation is the transversal reading of a Situation. It describes what is
// relevant, constrained, uncertain, or consequential without choosing what to
// do. Domain authorities remain responsible for their own decisions.
type Evaluation struct {
	RelevantFindings []DecisionFinding
	Criteria         []Criterion
	Implications     []string
	Risks            []string
	Opportunities    []string
	Conflicts        []string
	Uncertainty      []string
	Confidence       float64
	CreatedAt        time.Time
}

// DecisionFinding is a normalized observation extracted from Situation and
// specialist contributions. Consumers inspect findings instead of reaching
// into raw Relations themselves.
type DecisionFinding struct {
	Kind         string
	Source       string
	Perspective  string
	RelationType string
	Subject      string
	Target       string
	EvidenceIDs  []string
	Claims       []string
	Confidence   float64
	Relevance    float64
	Epistemic    EpistemicStatus
	Provenance   []string
}

// DecisionEvidence is the shared, read-only decision surface for downstream
// behavioral authorities. It carries evaluation and criteria but never
// chooses a response, initiative, capability, provider, or action.
type DecisionEvidence struct {
	IterationID      string
	SituationID      string
	SituationVersion uint64
	Findings         []DecisionFinding
	Criteria         []Criterion
	Evaluation       Evaluation
	Conflicts        []string
	Uncertainty      []string
	Confidence       float64
	Relevant         bool
	CreatedAt        time.Time
}

// IterationContext is the immutable, derived view carried through one pass of
// the system. It groups the common Situation, specialist contributions, and
// their single evaluation without taking ownership of any source state.
// Separate triggers create separate contexts; this is not durable memory.
type IterationContext struct {
	ID            string
	Trigger       string
	CreatedAt     time.Time
	Situation     Situation
	Contributions []Contribution
	Evidence      DecisionEvidence
}

// NewIterationContext assembles the shared read surface once so downstream
// dimensions consume the same interpretation rather than rebuilding it.
func NewIterationContext(trigger string, s Situation, contributions []Contribution) IterationContext {
	createdAt := time.Now()
	id := strings.TrimSpace(s.ID) + ":iteration:" + createdAt.UTC().Format("20060102T150405.000000000Z07:00")
	evidence := Evaluate(s, contributions)
	evidence.IterationID = id
	return IterationContext{
		ID: id, Trigger: strings.TrimSpace(trigger), CreatedAt: createdAt,
		Situation: s, Contributions: append([]Contribution(nil), contributions...), Evidence: evidence,
	}
}

// Evaluate is the single situation-level evaluation boundary. It aggregates
// already-derived evidence and specialist contributions into normalized
// findings and contextual criteria. It never mutates Situation or any source
// authority and never decides what a downstream authority must do.
func Evaluate(s Situation, contributions []Contribution) DecisionEvidence {
	now := time.Now()
	out := DecisionEvidence{
		SituationID: s.ID, SituationVersion: s.Version,
		Conflicts:   append([]string(nil), s.Conflicts...),
		Uncertainty: append([]string(nil), s.Uncertainty...),
		Confidence:  s.Confidence, Relevant: s.Relevance > 0, CreatedAt: now,
	}
	for _, c := range contributions {
		// A specialist's uncertainty/conflict is part of the shared decision
		// surface, not a private note. Preserve it even when the contribution
		// has no relation: otherwise a warning can be silently lost.
		out.Conflicts = appendUnique(out.Conflicts, c.Conflicts...)
		out.Uncertainty = appendUnique(out.Uncertainty, c.Uncertainty...)
		for _, r := range c.Relations {
			if strings.TrimSpace(r.Type) == "" {
				continue
			}
			out.Findings = append(out.Findings, DecisionFinding{
				Kind: strings.ToLower(strings.TrimSpace(r.Type)), Source: "situation",
				Perspective: c.Perspective, RelationType: r.Type, Subject: r.Source,
				Target: r.Target, EvidenceIDs: append([]string(nil), r.EvidenceIDs...),
				Claims: append([]string(nil), c.Claims...), Confidence: min01(c.Confidence, r.Confidence),
				Relevance: r.Relevance, Epistemic: r.Epistemic,
				Provenance: append([]string(nil), c.Provenance...),
			})
		}
	}
	// Criteria and Evaluation are derived from the situation plus the
	// contributions just merged. Use a copy so the source Situation remains
	// immutable while the criteria see the complete epistemic picture.
	evaluationSituation := s
	evaluationSituation.Conflicts = out.Conflicts
	evaluationSituation.Uncertainty = out.Uncertainty
	out.Criteria = selectCriteria(evaluationSituation, out.Findings)
	out.Evaluation = buildEvaluation(evaluationSituation, out.Findings, out.Criteria, now)
	out.Conflicts = appendUnique(out.Conflicts, out.Evaluation.Conflicts...)
	out.Uncertainty = appendUnique(out.Uncertainty, out.Evaluation.Uncertainty...)
	if out.Evaluation.Confidence > 0 {
		out.Confidence = out.Evaluation.Confidence
	}
	return out
}

func selectCriteria(s Situation, findings []DecisionFinding) []Criterion {
	var out []Criterion
	add := func(id, name, kind, reason string, ev []string, conf, rel float64) {
		for _, c := range out {
			if c.ID == id {
				return
			}
		}
		out = append(out, Criterion{ID: id, Name: name, Kind: kind, Reason: reason, EvidenceIDs: append([]string(nil), ev...), Confidence: min01(conf, 1), Relevance: clamp01(rel)})
	}
	if len(s.Uncertainty) > 0 {
		add("uncertainty", "preserve uncertainty", "epistemic", "the situation contains unresolved or low-certainty information", nil, .9, .95)
	}
	if len(s.Conflicts) > 0 {
		add("conflict", "preserve conflict", "epistemic", "source or interpretation conflict must remain visible", nil, .95, 1)
	}
	for _, f := range findings {
		switch strings.ToLower(f.Kind) {
		case "proposes", "anticipates", "commits_to":
			add("agency", "preserve user agency", "social", "a future-oriented relation expresses possibility or commitment, not execution authorization", f.EvidenceIDs, f.Confidence, f.Relevance)
		case "gives_feedback_to", "evaluates":
			add("feedback", "address feedback", "social", "the turn evaluates or comments on the addressed system or behavior", f.EvidenceIDs, f.Confidence, f.Relevance)
		case "addresses", "asks_about_state", "asks_about_opinion":
			add("direct_turn", "address current participant", "social", "the current turn explicitly targets Ahmelia or requests her participation", f.EvidenceIDs, f.Confidence, f.Relevance)
		case "supports", "enables", "affects", "depends_on":
			add("relational_context", "preserve relevant relation", "semantic", "the interpretation depends on a stated relation among elements", f.EvidenceIDs, f.Confidence, f.Relevance)
		case "involves_work", "status", "current_step", "last_result",
			"last_attempt_outcome_status", "last_attempt_outcome_code", "last_attempt_effect_observed", "last_attempt_effect_verified",
			"last_attempt_verification_source", "last_attempt_verification_reason", "last_attempt_failure_reason",
			"last_attempt_number", "last_attempt_status", "requires_reconciliation":
			add("operational_state", "respect current operational state", "operational", "work and attempt epistemic state is relevant to downstream planning or execution decisions", f.EvidenceIDs, f.Confidence, f.Relevance)
		}
	}
	return out
}

func buildEvaluation(s Situation, findings []DecisionFinding, criteria []Criterion, now time.Time) Evaluation {
	ev := Evaluation{RelevantFindings: append([]DecisionFinding(nil), findings...), Criteria: append([]Criterion(nil), criteria...), Conflicts: append([]string(nil), s.Conflicts...), Uncertainty: append([]string(nil), s.Uncertainty...), Confidence: s.Confidence, CreatedAt: now}
	for _, f := range findings {
		if f.Relevance < .5 {
			continue
		}
		switch strings.ToLower(f.Kind) {
		case "proposes", "anticipates", "commits_to":
			ev.Opportunities = append(ev.Opportunities, "existe una posibilidad o compromiso futuro relevante para la situación")
		case "gives_feedback_to", "evaluates":
			ev.Implications = append(ev.Implications, "el turno aporta evaluación sobre Ahmelia o su comportamiento y debe reconocerse sin convertirla en ejecución")
		case "addresses", "asks_about_state", "asks_about_opinion":
			ev.Implications = append(ev.Implications, "el usuario está dirigiendo el turno hacia Ahmelia o solicitando su participación")
		case "supports", "enables", "affects", "depends_on":
			ev.Implications = append(ev.Implications, "la relación declarada conecta elementos que pueden condicionar la interpretación posterior")
		case "status", "current_step", "last_result", "last_attempt_outcome_status", "last_attempt_outcome_code",
			"last_attempt_effect_observed", "last_attempt_effect_verified", "last_attempt_verification_source",
			"last_attempt_verification_reason", "last_attempt_failure_reason", "last_attempt_number",
			"last_attempt_status", "requires_reconciliation":
			ev.Implications = append(ev.Implications, "el estado operativo y epistemológico del último intento puede condicionar las decisiones posteriores")
		}
	}
	if len(s.Uncertainty) > 0 {
		ev.Risks = append(ev.Risks, "una interpretación no suficientemente respaldada no debe convertirse en hecho")
	}
	return ev
}

func appendUnique(dst []string, values ...string) []string {
	seen := map[string]bool{}
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range values {
		if strings.TrimSpace(v) != "" && !seen[v] {
			dst = append(dst, v)
			seen[v] = true
		}
	}
	return dst
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
func min01(a, b float64) float64 {
	if a == 0 {
		a = 1
	}
	if b == 0 {
		b = a
	}
	if a < b {
		return a
	}
	return b
}
func (e DecisionEvidence) HasKind(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	for _, f := range e.Findings {
		if strings.ToLower(strings.TrimSpace(f.Kind)) == kind {
			return true
		}
	}
	return false
}
