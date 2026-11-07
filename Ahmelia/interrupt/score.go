package interrupt

import (
	"fmt"
	"time"
)

// Score is the four-factor reading. Total depends only on severity, urgency
// and consequence; cost is exposed for ranking and for the notice, never added
// to Total. That keeps monotonicity honest: raising cost cannot raise the score.
type Score struct {
	Severity    float64  `json:"severity"`
	Urgency     float64  `json:"urgency"`
	Consequence float64  `json:"consequence"`
	Cost        float64  `json:"cost"`
	Total       float64  `json:"total"`
	Critical    bool     `json:"critical"`
	Escalated   bool     `json:"escalated"`
	Reasons     []string `json:"reasons,omitempty"`
}

// Assess validates the event and computes the monotone score. It is pure: the
// same event always produces the same score, with no clock and no model.
func Assess(e Event) (Score, error) {
	if err := e.Validate(); err != nil {
		return Score{}, err
	}
	sevRank, _ := severityRank(e.Severity)
	conRank, _ := consequenceRank(e.Consequence)

	score := Score{
		Severity:    severityFraction(e.Severity),
		Urgency:     urgencyFraction(e.Urgency),
		Consequence: consequenceFraction(e.Consequence),
		Cost:        blastFraction(e.BlastRadius),
	}
	score.Total = 0.35*score.Severity + 0.30*score.Urgency + 0.35*score.Consequence

	// Predicate 1: an irreversible consequence is imminent and the label is at
	// least high. The label helps, but the consequence is what authorizes the
	// interruption.
	if e.Consequence == ConsequenceIrreversible && e.Urgency <= ImminentWindow && sevRank >= 3 {
		score.Critical = true
		score.Reasons = append(score.Reasons, "irreversible_imminent_high_severity")
	}

	// Predicate 2: a critical label can interrupt a compensable consequence
	// only when there is evidence that it is imminent. A label alone never
	// promotes a reversible event.
	if e.Severity == SeverityCritical && conRank >= 1 && len(e.Evidence) > 0 && e.Urgency <= ImminentWindow {
		score.Critical = true
		score.Reasons = append(score.Reasons, "critical_label_with_imminent_evidence")
	}

	// An irreversible event that does not meet the interruption predicate is
	// never discarded: it is escalated for review and must appear in the
	// digest or in a blocked record.
	if e.Consequence == ConsequenceIrreversible && !score.Critical {
		score.Escalated = true
		score.Reasons = append(score.Reasons, "irreversible_below_interrupt_threshold")
	}

	if !score.Critical && !score.Escalated {
		score.Reasons = append(score.Reasons, "below_interrupt_threshold")
	}
	return score, nil
}

// Priority orders competing critical events: consequence first, then urgency,
// then severity. Cost is not part of the order the specification requires.
func (s Score) Priority() float64 {
	return 0.4*s.Consequence + 0.3*s.Urgency + 0.3*s.Severity
}

func severityFraction(s Severity) float64 {
	switch s {
	case SeverityInfo:
		return 0.0
	case SeverityLow:
		return 0.1
	case SeverityMedium:
		return 0.3
	case SeverityHigh:
		return 0.65
	case SeverityCritical:
		return 1.0
	}
	return 0.0
}

func urgencyFraction(d time.Duration) float64 {
	switch {
	case d <= 5*time.Minute:
		// Zero or negative means the consequence is already happening.
		return 1.0
	case d <= ImminentWindow:
		return 0.85
	case d <= time.Hour:
		return 0.55
	case d <= 6*time.Hour:
		return 0.3
	default:
		return 0.1
	}
}

func consequenceFraction(c Consequence) float64 {
	switch c {
	case ConsequenceReversible:
		return 0.0
	case ConsequenceCompensable:
		return 0.45
	case ConsequenceIrreversible:
		return 1.0
	}
	return 0.0
}

func blastFraction(b BlastRadius) float64 {
	switch b {
	case BlastSelf:
		return 0.1
	case BlastProject:
		return 0.3
	case BlastOwner:
		return 0.7
	case BlastThirdParty:
		return 1.0
	}
	return 0.0
}

// String makes the score readable in traces without inventing prose.
func (s Score) String() string {
	state := "below"
	if s.Critical {
		state = "critical"
	} else if s.Escalated {
		state = "escalated"
	}
	return fmt.Sprintf("%s total=%.2f sev=%.2f urg=%.2f con=%.2f cost=%.2f", state, s.Total, s.Severity, s.Urgency, s.Consequence, s.Cost)
}
