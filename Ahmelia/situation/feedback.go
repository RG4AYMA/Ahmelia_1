package situation

import "time"

// Observation is a post-cycle fact supplied to the next Situation build.
// It records what changed or what was observed; it does not decide what the
// change means globally and it never owns the originating state.
type Observation struct {
	ID         string
	Kind       string
	Source     string
	Subject    string
	Predicate  string
	Object     string
	Value      string
	ObservedAt time.Time
	// Temporal preserves the producer's freshness classification when an
	// observation is rehydrated from durable history. Empty retains the
	// historical default of Recent for newly supplied cycle feedback.
	Temporal   TemporalStatus
	Confidence float64
	Epistemic  EpistemicStatus
}

// Trigger marks why a new cognitive Situation was requested. A trigger is a
// scheduling/causal signal, not a behavioral decision.
type Trigger struct {
	ID             string
	Reason         string
	ObservationIDs []string
	CreatedAt      time.Time
}

// SituationDelta is a compact representation of what changed between two
// derived Situations. Source ownership remains outside this package.
type SituationDelta struct {
	FromVersion uint64
	ToVersion   uint64
	Added       []Relation
	Removed     []Relation
	Changed     []Relation
	Trigger     Trigger
}

// ApplyObservation derives the next Situation from an existing Situation.
// This is useful for feedback-driven cycles where a fresh full convergence is
// not yet required; normal Engine turns still rebuild Situation from source
// projections, which is the authoritative convergence path.
func ApplyObservation(previous Situation, observations []Observation, trigger Trigger) Situation {
	next := previous
	next.Version++
	next.UpdatedAt = time.Now()
	next.Temporal = TemporalCurrent
	for _, o := range observations {
		id := o.ID
		if id == "" {
			continue
		}
		temporal := observationTemporal(o)
		next.Evidence = append(next.Evidence, Evidence{ID: id, Authority: o.Source, Source: o.Source, Subject: o.Subject, Predicate: o.Predicate, Object: o.Object, Value: o.Value, ObservedAt: o.ObservedAt, Temporal: temporal, Relevance: .9, Confidence: clamp01(o.Confidence), Epistemic: o.Epistemic})
		if o.Subject != "" && o.Object != "" {
			next.Relations = append(next.Relations, Relation{Source: o.Subject, Target: o.Object, Type: o.Predicate, EvidenceIDs: []string{id}, Provenance: o.Source, Temporal: temporal, Relevance: .9, Confidence: clamp01(o.Confidence), Epistemic: o.Epistemic})
		}
	}
	return next
}

func observationTemporal(o Observation) TemporalStatus {
	if o.Temporal != "" {
		return o.Temporal
	}
	return TemporalRecent
}
