package situation

import "time"

// B96.40.1 — Transversal Situation Core.
// Situation is a derived, temporal representation. It is not a memory store,
// truth authority, decision engine, planner, dialogue owner, or executor.

type TemporalStatus string

const (
	TemporalCurrent    TemporalStatus = "current"
	TemporalRecent     TemporalStatus = "recent"
	TemporalHistorical TemporalStatus = "historical"
)

type EpistemicStatus string

const (
	EpistemicObserved  EpistemicStatus = "observed"
	EpistemicDerived   EpistemicStatus = "derived"
	EpistemicInferred  EpistemicStatus = "inferred"
	EpistemicUncertain EpistemicStatus = "uncertain"
)

type Evidence struct {
	ID            string
	Authority     string
	Source        string
	Subject       string
	Predicate     string
	Object        string
	Value         string
	ObservedAt    time.Time
	Temporal      TemporalStatus
	Relevance     float64
	Confidence    float64
	Epistemic     EpistemicStatus
	Authoritative bool
}

type Entity struct {
	ID         string
	Kind       string
	Name       string
	Authority  string
	Confidence float64
}

type Relation struct {
	Source      string
	Target      string
	Type        string
	EvidenceIDs []string
	Provenance  string
	Temporal    TemporalStatus
	Relevance   float64
	Confidence  float64
	Epistemic   EpistemicStatus
}

type Anchor struct {
	Kind        string
	Description string
	Source      string
	ObservedAt  time.Time
}

type Situation struct {
	ID          string
	Version     uint64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Scope       string
	Anchor      Anchor
	Entities    []Entity
	Relations   []Relation
	Evidence    []Evidence
	Temporal    TemporalStatus
	Relevance   float64
	Confidence  float64
	Uncertainty []string
	Conflicts   []string
}

// Contribution is a specialist's independent reading of one Situation.
// Contributions do not vote and do not mutate the Situation's source owners.
type Contribution struct {
	ID            string
	Perspective   string
	Claims        []string
	EvidenceIDs   []string
	Relations     []Relation
	Implications  []string
	Risks         []string
	Opportunities []string
	Conflicts     []string
	Confidence    float64
	Uncertainty   []string
	Provenance    []string
	CreatedAt     time.Time
}
