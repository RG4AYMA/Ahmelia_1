// Package interrupt owns the deterministic triage of operational events.
//
// It answers two separate questions:
//
//  1. How urgent is this event, using severity, urgency, consequence and cost?
//  2. Does it qualify to interrupt the owner, and if not, must it still be
//     escalated or recorded?
//
// The package never writes prose and never calls a model. A five-part Notice
// is composed from facts the harness already owns; the model may rewrite it
// later, but cannot add facts, actions or options.
package interrupt

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Severity is the observed label, not the decision. A label alone never turns
// a reversible event into a critical one.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Consequence is what happens if nobody acts.
type Consequence string

const (
	ConsequenceReversible   Consequence = "reversible"
	ConsequenceCompensable  Consequence = "compensable"
	ConsequenceIrreversible Consequence = "irreversible"
)

// BlastRadius is the cost dimension: who pays if the consequence lands.
type BlastRadius string

const (
	BlastSelf       BlastRadius = "self"
	BlastProject    BlastRadius = "project"
	BlastOwner      BlastRadius = "owner"
	BlastThirdParty BlastRadius = "third_party"
)

// Privacy is the disclosure class of the event and its notice.
type Privacy string

const (
	PrivacyPrivate Privacy = "private"
	PrivacyShared  Privacy = "shared"
	PrivacyPublic  Privacy = "public"
)

// Event is one operational fact. Urgency is the time until the consequence,
// not the time since the event was observed.
type Event struct {
	ID          string        `json:"id"`
	Kind        string        `json:"kind"`
	Severity    Severity      `json:"severity"`
	Urgency     time.Duration `json:"urgency"`
	Consequence Consequence   `json:"consequence"`
	BlastRadius BlastRadius   `json:"blast_radius"`
	Privacy     Privacy       `json:"privacy"`
	Source      string        `json:"source"`
	ObservedAt  time.Time     `json:"observed_at"`
	ExpiresAt   time.Time     `json:"expires_at,omitempty"`
	Evidence    []string      `json:"evidence,omitempty"`
}

// ErrInvalid rejects malformed events before they reach the policy.
var ErrInvalid = errors.New("interrupt: evento inválido")

// Validate rejects an event whose facts cannot be scored honestly.
func (e Event) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("%w: id vacío", ErrInvalid)
	}
	if strings.TrimSpace(e.Kind) == "" {
		return fmt.Errorf("%w: kind vacío", ErrInvalid)
	}
	if _, ok := severityRank(e.Severity); !ok {
		return fmt.Errorf("%w: severity %q", ErrInvalid, e.Severity)
	}
	if _, ok := consequenceRank(e.Consequence); !ok {
		return fmt.Errorf("%w: consequence %q", ErrInvalid, e.Consequence)
	}
	if _, ok := blastRank(e.BlastRadius); !ok {
		return fmt.Errorf("%w: blast_radius %q", ErrInvalid, e.BlastRadius)
	}
	switch e.Privacy {
	case PrivacyPrivate, PrivacyShared, PrivacyPublic:
	default:
		return fmt.Errorf("%w: privacy %q", ErrInvalid, e.Privacy)
	}
	if e.ObservedAt.IsZero() {
		return fmt.Errorf("%w: observed_at vacío", ErrInvalid)
	}
	if !e.ExpiresAt.IsZero() && !e.ExpiresAt.After(e.ObservedAt) {
		return fmt.Errorf("%w: expires_at no es posterior a observed_at", ErrInvalid)
	}
	return nil
}

func severityRank(s Severity) (int, bool) {
	switch s {
	case SeverityInfo:
		return 0, true
	case SeverityLow:
		return 1, true
	case SeverityMedium:
		return 2, true
	case SeverityHigh:
		return 3, true
	case SeverityCritical:
		return 4, true
	}
	return 0, false
}

func consequenceRank(c Consequence) (int, bool) {
	switch c {
	case ConsequenceReversible:
		return 0, true
	case ConsequenceCompensable:
		return 1, true
	case ConsequenceIrreversible:
		return 2, true
	}
	return 0, false
}

func blastRank(b BlastRadius) (int, bool) {
	switch b {
	case BlastSelf:
		return 0, true
	case BlastProject:
		return 1, true
	case BlastOwner:
		return 2, true
	case BlastThirdParty:
		return 3, true
	}
	return 0, false
}

// ImminentWindow is the bound the specification gives for acting before an
// irreversible consequence lands.
const ImminentWindow = 15 * time.Minute

// Fingerprint identifies the facts of an event for duplicate handling. The same
// ID with different severity or evidence is a new notice; the same facts are a
// duplicate.
func (e Event) Fingerprint() string {
	evidence := append([]string(nil), e.Evidence...)
	sort.Strings(evidence)
	return fmt.Sprintf("%s|%s|%s", e.ID, e.Severity, strings.Join(evidence, ","))
}
